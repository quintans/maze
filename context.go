package maze

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gorilla/schema"

	tk "github.com/quintans/toolkit"
)

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

var _ IContext = &MazeContext{}

type MazeContext struct {
	logger     Logger
	decoder    *schema.Decoder
	Response   http.ResponseWriter
	Request    *http.Request
	Attributes map[interface{}]interface{} // attributes only valid in this request
	filters    []*Filter
	filterPos  int
	values     Values
	pathValues Values
}

func NewContext(logger Logger, w http.ResponseWriter, r *http.Request, filters []*Filter) *MazeContext {
	decoder := schema.NewDecoder()
	decoder.SetAliasTag("json")
	c := &MazeContext{
		logger:     logger,
		decoder:    decoder,
		Response:   w,
		Request:    r,
		filterPos:  -1,
		filters:    filters,
		Attributes: map[interface{}]interface{}{},
	}
	return c
}

func (c *MazeContext) nextFilter() *Filter {
	c.filterPos++
	if c.filterPos < len(c.filters) {
		return c.filters[c.filterPos]
	}
	// don't let it go higher than the max
	c.filterPos = len(c.filters)

	return nil
}

// Proceed proceeds to the next valid rule
// This method should be reimplemented in specialized Context,
// extending this one
func (c *MazeContext) Proceed() error {
	return c.Next(c)
}

func (c *MazeContext) Next(mc IContext) error {
	next := c.nextFilter()
	if next != nil {
		if next.route == "" {
			c.logger.Debugf("executing filter without rule")
			return next.handler(mc)
		} else {
			// go to the next valid filter.
			for i := c.filterPos; i < len(c.filters); i++ {
				n := c.filters[i]
				if n.IsValid(mc.GetRequest()) {
					c.filterPos = i
					c.logger.Debugf("executing filter %s", n)
					return n.handler(mc)
				}
			}
		}
	}

	return nil
}

func (c *MazeContext) GetResponse() http.ResponseWriter {
	return c.Response
}

func (c *MazeContext) SetResponse(w http.ResponseWriter) {
	c.Response = w
}

func (c *MazeContext) GetRequest() *http.Request {
	return c.Request
}

func (c *MazeContext) GetAttribute(key interface{}) interface{} {
	return c.Attributes[key]
}

func (c *MazeContext) SetAttribute(key interface{}, value interface{}) {
	c.Attributes[key] = value
}

func (c *MazeContext) CurrentFilter() *Filter {
	if c.filterPos < len(c.filters) {
		return c.filters[c.filterPos]
	}
	return nil
}

func (c *MazeContext) Payload(value interface{}) error {
	if c.Request.Body != nil {
		payload, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			return err
		}

		return json.Unmarshal(payload, value)
	}

	return nil
}

// PathVars put the path parameters in a url into the struct passed as an interface{}
func (c *MazeContext) PathVars(value interface{}) error {
	values := c.PathValues()
	if len(values) > 0 {
		return c.decoder.Decode(value, values)
	}

	return nil
}

// QueryVars put the parameters in the query part of a url into the struct passed as an interface{}
func (c *MazeContext) QueryVars(value interface{}) error {
	values := c.GetRequest().URL.Query()
	if len(values) > 0 {
		return c.decoder.Decode(value, values)
	}

	return nil
}

func (c *MazeContext) Vars(value interface{}) error {
	values := c.Values()
	if len(values) > 0 {
		return c.decoder.Decode(value, values)
	}

	return nil
}

func (c *MazeContext) Load(value interface{}) error {
	if err := c.Vars(value); err != nil {
		return err
	}

	if err := c.Payload(value); err != nil {
		return err
	}

	return nil
}

func (c *MazeContext) Values() Values {
	if c.values != nil {
		return c.values
	}

	c.values = make(Values)

	// path parameters
	for k, v := range c.PathValues() {
		c.values[k] = v
	}

	// query parameters
	query := c.GetRequest().URL.Query()
	for k, v := range query {
		c.values[k] = v
	}

	return c.values
}

func (c *MazeContext) PathValues() Values {
	if c.pathValues != nil {
		return c.pathValues
	}

	c.pathValues = make(Values)
	path := c.GetRequest().URL.Path
	parts := strings.Split(path, "/")

	template := c.CurrentFilter().template

	if len(parts) == len(template) {
		for k, v := range template {
			if strings.HasPrefix(v, ":") {
				c.pathValues[v[1:]] = []string{parts[k]}
			}
		}
	}

	return c.pathValues
}

// TEXT transforms value to text and send it as text content type
// with the specified status (eg: http.StatusOK)
func (c *MazeContext) TEXT(status int, value interface{}) error {
	w := c.GetResponse()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Expires", "-1")
	w.WriteHeader(status)
	if value != nil {
		s := tk.ToString(value)
		if _, err := w.Write([]byte(s)); err != nil {
			return err
		}
	}

	return nil
}

func (c *MazeContext) JSON(status int, value interface{}) error {
	w := c.GetResponse()
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
