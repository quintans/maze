package maze

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gorilla/schema"
	tk "github.com/quintans/toolkit"
	"github.com/quintans/toolkit/log"
	"github.com/quintans/toolkit/web"
)

var logger = log.LoggerFor("github.com/quintans/maze")

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

func (this *Maze) GET(rule string, filters ...func(ctx IContext) error) {
	this.PushMethod([]string{"GET"}, rule, filters...)
}

func (this *Maze) POST(rule string, filters ...func(ctx IContext) error) {
	this.PushMethod([]string{"POST"}, rule, filters...)
}

func (this *Maze) PUT(rule string, filters ...func(ctx IContext) error) {
	this.PushMethod([]string{"PUT"}, rule, filters...)
}

func (this *Maze) DELETE(rule string, filters ...func(ctx IContext) error) {
	this.PushMethod([]string{"DELETE"}, rule, filters...)
}

func (this *Maze) Push(rule string, filters ...func(ctx IContext) error) {
	this.PushMethod(nil, rule, filters...)
}

// PushMethod adds the filters to the end of the last filters.
// If the rule does NOT start with '/' the applied rule will be
// the concatenation of the last rule that started with '/' and ended with a '*'
// with this one (the '*' is omitted).
// ex: /greet/* + sayHi/{Id} = /greet/sayHi/{Id}
func (this *Maze) PushMethod(methods []string, rule string, handlers ...func(ctx IContext) error) {
	if strings.HasPrefix(rule, "/") {
		if strings.HasSuffix(rule, "*") {
			this.lastRule = rule[:len(rule)-1]
			logger.Debug("Last main rule set as", this.lastRule)
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
		if rule != "" {
			logger.Debug("registering rule", rule)
			f[0].rule = rule
			if i := strings.Index(rule, "{"); i != -1 {
				f[0].template = strings.Split(rule, "/")
			}
		}
		f[0].allowedMethods = methods

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
	// TEXT converts to string the interface{} value and sends it into the response
	TEXT(interface{}) error
	// JSON marshals the interface{} value into a json string and sends it into the response
	JSON(interface{}) error
}

var _ IContext = &Context{}

type Context struct {
	Response   http.ResponseWriter
	Request    *http.Request
	Attributes map[interface{}]interface{} // attributes only valid in this request
	filters    []*Filter
	filterPos  int
	// Enable to call methods of the extended struct
	// or to cast the IContext parameter of the handler function
	// to the right context struct
	Overrider IContext
}

func NewContext(w http.ResponseWriter, r *http.Request, filters []*Filter) *Context {
	var this = new(Context)
	this.Overrider = this
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
func (this *Context) Proceed() error {
	var c = this.Overrider
	var next = this.nextFilter()
	if next != nil {
		if next.rule == "" {
			logger.Debug("executing filter without rule")
			return next.handler(c)
		} else {
			// go to the next valid filter.
			// I don't use recursivity for this, because it can be very deep
			for i := this.filterPos; i < len(this.filters); i++ {
				var n = this.filters[i]
				if n.rule != "" && n.IsValid(c) {
					this.filterPos = i
					logger.Debugf("executing filter %s", n.rule)
					return n.handler(c)
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
	var filter = this.CurrentFilter()
	if filter.jsonPath != "" {
		return json.Unmarshal([]byte(filter.jsonPath), value)
	}

	return nil
}

var decoder = schema.NewDecoder()

// QueryVars put the parameters in the query part of a url into the struct passed as an interface{}
func (this *Context) QueryVars(value interface{}) error {
	// create a json string and then unmarshal
	var values = this.GetRequest().URL.Query()
	if len(values) > 0 {
		return decoder.Decode(value, this.GetRequest().URL.Query())
	}

	return nil
}

func (this *Context) Vars(value interface{}) error {
	if err := this.QueryVars(value); err != nil {
		return err
	}
	if err := this.PathVars(value); err != nil {
		return err
	}

	return nil
}

func (this *Context) TEXT(value interface{}) error {
	var w = this.GetResponse()
	//w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if value != nil {
		var s = tk.ToString(value)
		if _, err := w.Write([]byte(s)); err != nil {
			return err
		}
	}

	w.WriteHeader(http.StatusOK)
	return nil
}

func (this *Context) JSON(value interface{}) error {
	var w = this.GetResponse()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if value != nil {
		result, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = w.Write(result)
		if err != nil {
			return err
		}
	}

	w.WriteHeader(http.StatusOK)
	return nil
}

type Filterer interface {
	Handle(ctx IContext) error
}

type Filter struct {
	rule           string
	template       []string
	jsonPath       string
	allowedMethods []string

	handler func(ctx IContext) error
}

func NewFilter(rule string, handler func(c IContext) error) *Filter {
	var this = new(Filter)
	this.rule = rule
	this.handler = handler
	return this
}

func (this *Filter) IsValid(ctx IContext) bool {
	// verify if method is allowed
	var allowed bool
	if this.allowedMethods == nil {
		allowed = true
	} else {
		var method = ctx.GetRequest().Method
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
		var path = ctx.GetRequest().URL.Path
		if strings.HasPrefix(this.rule, "*") {
			return strings.HasSuffix(path, this.rule[1:])
		} else if strings.HasSuffix(this.rule, "*") {
			return strings.HasPrefix(path, this.rule[:len(this.rule)-1])
		} else if this.template != nil {
			var ok bool
			this.jsonPath, ok = this.parseToJson(path)
			return ok
		} else {
			return path == this.rule
		}
	}

	return false
}

// parseToJson converts path vars into json string
// and also checks if its a valid match with the url template
func (this *Filter) parseToJson(path string) (string, bool) {
	var json = ""
	var parts = strings.Split(path, "/")

	if len(parts) != len(this.template) {
		return "", false
	}

	for k, v := range this.template {
		if strings.HasPrefix(v, "{") {
			var name = v[1 : len(v)-1]
			var nameType = strings.Split(name, ":")
			name = nameType[0]
			var typ string
			if len(nameType) > 1 {
				typ = nameType[1]
			}

			var val = toJsonVal(parts[k], typ)
			if len(json) > 0 {
				json += ", "
			}
			json += fmt.Sprintf(`"%s": %s`, name, val)

		} else if v != parts[k] {
			return "", false
		}
	}

	return "{" + json + "}", true
}

func toJsonVal(ori string, typ string) string {
	var val = ori
	switch typ {
	case "number":
	case "boolean":
		if val == "1" || val == "true" || val == "t" {
			val = "true"
		} else {
			val = "false"
		}
	default:
		val = "\"" + val + "\""
	}

	return val
}

func ConvertHandlers(handlers ...func(ctx IContext) error) []*Filter {
	var filters = make([]*Filter, len(handlers))
	for k, v := range handlers {
		filters[k] = &Filter{handler: v}
	}
	return filters
}
