package maze

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gorilla/schema"
	tk "github.com/quintans/toolkit"
	"github.com/quintans/toolkit/log"
	"github.com/quintans/toolkit/web"
)

var decoder = schema.NewDecoder()

func init() {
	logger = log.LoggerFor("github.com/quintans/maze")
	decoder.SetAliasTag("json")
}

var logger log.ILogger

func SetLogger(lgr log.ILogger) {
	logger = lgr
}

// NewMaze creates maze with context factory. If nil, it uses a default context factory
func NewMaze(contextFactory func(w http.ResponseWriter, r *http.Request, filters []*Filter) IContext) *Maze {
	this := new(Maze)
	this.filters = make([]*Filter, 0)
	this.contextFactory = contextFactory
	return this
}

type Maze struct {
	filters        []*Filter
	contextFactory func(w http.ResponseWriter, r *http.Request, filters []*Filter) IContext
	lastRule       string
}

func (this *Maze) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(this.filters) > 0 {
		var ctx IContext
		if this.contextFactory == nil {
			// default
			ctx = NewContext(w, r, this.filters)
		} else {
			ctx = this.contextFactory(w, r, this.filters)
		}
		err := ctx.Proceed()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (this *Maze) GET(rule string, filters ...Handler) {
	this.PushMethod([]string{"GET"}, rule, filters...)
}

func (this *Maze) POST(rule string, filters ...Handler) {
	this.PushMethod([]string{"POST"}, rule, filters...)
}

func (this *Maze) PUT(rule string, filters ...Handler) {
	this.PushMethod([]string{"PUT"}, rule, filters...)
}

func (this *Maze) DELETE(rule string, filters ...Handler) {
	this.PushMethod([]string{"DELETE"}, rule, filters...)
}

func (this *Maze) Push(rule string, filters ...Handler) {
	this.PushMethod(nil, rule, filters...)
}

// PushMethod adds the filters to the end of the last filters.
// If the rule does NOT start with '/' the applied rule will be
// the concatenation of the last rule that started with '/' and ended with a '*'
// with this one (the '*' is omitted).
// ex: /greet/* + sayHi/:Id = /greet/sayHi/:Id
func (this *Maze) PushMethod(methods []string, rule string, handlers ...Handler) {
	if strings.HasPrefix(rule, "/") {
		if strings.HasSuffix(rule, "*") {
			this.lastRule = rule[:len(rule)-1]
			logger.Tracef("Last main rule set as %s", this.lastRule)
		} else {
			// resets lastRule
			this.lastRule = ""
		}
	} else if !strings.HasPrefix(rule, "*") {
		if this.lastRule == "" {
			rule = "/" + rule
		} else {
			rule = this.lastRule + rule
		}
	}

	if len(handlers) > 0 {
		f := ConvertHandlers(handlers...)
		// rule is only set for the first filter
		f[0].setRule(methods, rule)
		this.filters = append(this.filters, f...)
	}
}

func (this *Maze) Add(filters ...*Filter) {
	this.filters = append(this.filters, filters...)
}

// Static serves static content.
// rule defines the rule and dir the relative path
func (this *Maze) Static(rule string, dir string) {
	// delivering static content and preventing malicious access
	var fs = web.OnlyFilesFS{http.Dir(dir)}
	var fileServer = http.FileServer(fs)
	this.GET(rule, func(ctx IContext) error {
		fileServer.ServeHTTP(ctx.GetResponse(), ctx.GetRequest())
		return nil
	})
}

func (this *Maze) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/", this)

	logger.Infof("Listening http at %s", addr)
	return http.ListenAndServe(addr, mux)
}

type IContext interface {
	Proceed() error
	GetResponse() http.ResponseWriter
	SetResponse(http.ResponseWriter)
	GetRequest() *http.Request
	GetAttribute(interface{}) interface{}
	SetAttribute(interface{}, interface{})
	CurrentFilter() *Filter

	// Payload put the json string in the request body into the struct passed as an interface{}
	Payload(interface{}) error
	// PathVars put the path parameters in a url into the struct passed as an interface{}
	PathVars(interface{}) error
	// QueryVars put the parameters in the query part of a url into the struct passed as an interface{}
	QueryVars(interface{}) error
	// Vars
	Vars(interface{}) error
	// Values gets a path parameter converter
	PathValues() Values
	// Values gets a parameter converter (path + query)
	Values() Values
	// Load calls Vars and Payload
	Load(value interface{}) error
	// TEXT converts to string the interface{} value and sends it into the response with a status code
	TEXT(int, interface{}) error
	// JSON marshals the interface{} value into a json string and sends it into the response with a status code
	JSON(int, interface{}) error
}

var _ IContext = &Context{}

type Context struct {
	Response   http.ResponseWriter
	Request    *http.Request
	Attributes map[interface{}]interface{} // attributes only valid in this request
	filters    []*Filter
	filterPos  int
	values     Values
	pathValues Values
}

func NewContext(w http.ResponseWriter, r *http.Request, filters []*Filter) *Context {
	var this = new(Context)
	this.Response = w
	this.Request = r
	this.filterPos = -1
	this.filters = filters
	this.Attributes = make(map[interface{}]interface{})
	return this
}

func (this *Context) nextFilter() *Filter {
	this.filterPos++
	if this.filterPos < len(this.filters) {
		return this.filters[this.filterPos]
	}
	// don't let ir go higher than the max
	this.filterPos = len(this.filters)

	return nil
}

// Proceed proceeds to the next valid rule
// This method should be reimplemented in specialized Context,
// extending this one
func (this *Context) Proceed() error {
	return this.Next(this)
}

func (this *Context) Next(c IContext) error {
	var next = this.nextFilter()
	if next != nil {
		if next.route == "" {
			logger.Debugf("executing filter without rule")
			return next.handler(this)
		} else {
			// go to the next valid filter.
			// I don't use recursivity for this, because it can be very deep
			for i := this.filterPos; i < len(this.filters); i++ {
				var n = this.filters[i]
				if n.IsValid(c.GetRequest()) {
					this.filterPos = i
					logger.Debugf("executing filter %s", n)
					return n.handler(this)
				}
			}
		}
	}

	return nil
}

func (this *Context) GetResponse() http.ResponseWriter {
	return this.Response
}

func (this *Context) SetResponse(w http.ResponseWriter) {
	this.Response = w
}

func (this *Context) GetRequest() *http.Request {
	return this.Request
}

func (this *Context) GetAttribute(key interface{}) interface{} {
	return this.Attributes[key]
}

func (this *Context) SetAttribute(key interface{}, value interface{}) {
	this.Attributes[key] = value
}

func (this *Context) CurrentFilter() *Filter {
	if this.filterPos < len(this.filters) {
		return this.filters[this.filterPos]
	}
	return nil
}

func (this *Context) Payload(value interface{}) error {
	if this.Request.Body != nil {
		payload, err := ioutil.ReadAll(this.Request.Body)
		if err != nil {
			return err
		}

		return json.Unmarshal(payload, value)
	}

	return nil
}

// PathVars put the path parameters in a url into the struct passed as an interface{}
func (this *Context) PathVars(value interface{}) error {
	var values = this.PathValues()
	if len(values) > 0 {
		return decoder.Decode(value, values)
	}

	return nil
}

// QueryVars put the parameters in the query part of a url into the struct passed as an interface{}
func (this *Context) QueryVars(value interface{}) error {
	var values = this.GetRequest().URL.Query()
	if len(values) > 0 {
		return decoder.Decode(value, values)
	}

	return nil
}

func (this *Context) Vars(value interface{}) error {
	var values = this.Values()
	if len(values) > 0 {
		return decoder.Decode(value, values)
	}

	return nil
}

func (this *Context) Load(value interface{}) error {
	if err := this.Vars(value); err != nil {
		return err
	}

	if err := this.Payload(value); err != nil {
		return err
	}

	return nil
}

func (this *Context) Values() Values {
	if this.values != nil {
		return this.values
	}

	this.values = make(Values)

	// path parameters
	for k, v := range this.PathValues() {
		this.values[k] = v
	}

	// query parameters
	var query = this.GetRequest().URL.Query()
	if query != nil {
		for k, v := range query {
			this.values[k] = v
		}
	}

	return this.values
}

func (this *Context) PathValues() Values {
	if this.pathValues != nil {
		return this.pathValues
	}

	this.pathValues = make(Values)
	var path = this.GetRequest().URL.Path
	var parts = strings.Split(path, "/")

	var template = this.CurrentFilter().template

	if len(parts) == len(template) {
		for k, v := range template {
			if strings.HasPrefix(v, ":") {
				this.pathValues[v[1:]] = []string{parts[k]}
			}
		}
	}

	return this.pathValues
}

// TEXT transforms value to text and send it as text content type
// with the specified status (eg: http.StatusOK)
func (this *Context) TEXT(status int, value interface{}) error {
	var w = this.GetResponse()
	//w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	//w.Header().Set("Expires", "-1")
	if value != nil {
		var s = tk.ToString(value)
		if _, err := w.Write([]byte(s)); err != nil {
			return err
		}
	}
	// eg: http.StatusOK
	w.WriteHeader(status)

	return nil
}

func (this *Context) JSON(status int, value interface{}) error {
	var w = this.GetResponse()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Expires", "-1")
	if value != nil {
		result, err := json.Marshal(value)
		if err != nil {
			return err
		}
		w.WriteHeader(status)
		_, err = w.Write(result)
		if err != nil {
			return err
		}
	} else {
		w.WriteHeader(status)
	}

	return nil
}

type Filterer interface {
	Handle(ctx IContext) error
}

type Handler func(IContext) error

const (
	WILDCARD        = "*"
	WILDCARD_BEFORE = -1
	WILDCARD_AFTER  = 1
)

type Filter struct {
	route          string
	wildcard       int
	template       []string
	allowedMethods []string

	handler Handler
}

func (this *Filter) setRule(methods []string, rule string) {
	if rule != "" {
		logger.Infof("registering rule %s", rule)

		if strings.HasPrefix(rule, WILDCARD) {
			this.route = rule[1:]
			this.wildcard = WILDCARD_BEFORE
		} else if strings.HasSuffix(rule, WILDCARD) {
			this.route = rule[:len(rule)-1]
			this.wildcard = WILDCARD_AFTER
		}

		this.route = rule
		if i := strings.Index(rule, ":"); i != -1 {
			this.template = strings.Split(rule, "/")
		}
	}
	this.allowedMethods = methods
}

func (this *Filter) String() string {
	var str string
	if this.wildcard == WILDCARD_BEFORE {
		str = "*"
	}
	str += this.route
	if this.wildcard == WILDCARD_AFTER {
		str += "*"
	}
	return str
}

func NewFilter(rule string, handler Handler) *Filter {
	var this = new(Filter)
	this.setRule(nil, rule)
	this.handler = handler
	return this
}

func (this *Filter) IsValid(request *http.Request) bool {
	if this.route == "" {
		return false
	}

	// verify if method is allowed
	var allowed bool
	if this.allowedMethods == nil {
		allowed = true
	} else {
		var method = request.Method
		if method == "" {
			method = "GET"
		}
		for _, v := range this.allowedMethods {
			if method == v {
				allowed = true
				break
			}
		}
	}

	if allowed {
		var path = request.URL.Path
		if this.wildcard == WILDCARD_BEFORE {
			return strings.HasSuffix(path, this.route)
		} else if this.wildcard == WILDCARD_AFTER {
			return strings.HasPrefix(path, this.route)
		} else if this.template != nil {
			return this.validate(path)
		} else {
			return path == this.route
		}
	}

	return false
}

// validate checks if its a valid match with the url template
func (this *Filter) validate(path string) bool {
	var parts = strings.Split(path, "/")

	if len(parts) != len(this.template) {
		return false
	}

	for k, v := range this.template {
		if !strings.HasPrefix(v, ":") && v != parts[k] {
			return false
		}
	}

	return true
}

func ConvertHandlers(handlers ...Handler) []*Filter {
	var filters = make([]*Filter, len(handlers))
	for k, v := range handlers {
		filters[k] = &Filter{handler: v}
	}
	return filters
}
