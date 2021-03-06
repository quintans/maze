package maze

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"unicode"
)

// Makes a struct responsible for handling json-rpc calls.
// The endpoint will be composet by the service name and action name. ex order/item
// The rules for the action parameters are:
// * can have at most two parameters
// * if it has two parameters, the first must be of the type web.IContext
//
// The rules for the action return values are:
// * can have at most two return values
// * if it has two parameters, the last must be of the type error
//
// valid signature:  MyStruct.MyAction([web.IContext][any]) [any][error]

const (
	UNKNOWN_SRV = "JSONRPC01"
	UNKNOWN_ACT = "JSONRPC02"
)

type Action struct {
	name       string
	callFilter *Filter
	filters    []*Filter
}

func NewAction(name string) *Action {
	return &Action{
		name:    name,
		filters: make([]*Filter, 0),
	}
}

func (a *Action) SetFilters(filters ...Handler) {
	a.filters = convertHandlers(filters...)
}

type JsonRpc struct {
	servicePath string
	filters     []*Filter
	actions     []*Action
	logger      Logger
}

func NewJsonRpc(logger Logger, svc interface{}, filters ...Handler) (*JsonRpc, error) {
	v := reflect.ValueOf(svc)
	t := v.Type()
	if t.Kind() != reflect.Ptr {
		panic("Supplied instance must be a pointer.")
	}

	// Only structs are supported
	if t.Elem().Kind() != reflect.Struct {
		panic("Supplied instance is not a struct.")
	}

	rpc := &JsonRpc{
		servicePath: t.Name(),
		actions:     make([]*Action, 0),
		filters:     convertHandlers(filters...),
		logger:      logger,
	}

	// loop through the struct's fields and set the map
	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		if isExported(method.Name) {
			action := NewAction(method.Name)

			logger.Debugf("Registering JSON-RPC %s/%s", rpc.servicePath, method.Name)

			// validate argument types
			size := method.Type.NumIn()
			if size > 3 {
				return nil, fmt.Errorf("invalid service %s.%s. Service actions can only have at the most two  parameters",
					t.Elem().Name(), method.Name)
			} else if size > 2 {
				t := method.Type.In(1)
				if t != contextType {
					return nil, fmt.Errorf("invalid service %s.%s. In a two paramater action the first must be the interface web.IContext",
						t.Elem().Name(), method.Name)
				}
			}

			var payloadType reflect.Type
			var hasContext bool
			if size == 3 {
				payloadType = method.Type.In(2)
				hasContext = true
			} else if size == 2 {
				t := method.Type.In(1)
				if t != contextType {
					payloadType = t
				} else {
					hasContext = true
				}
			}

			// logger.Debugf("Has Contex: %t; Payload Type: %s", hasContext, payloadType)

			// validate return types
			size = method.Type.NumOut()
			if size > 2 {
				return nil, fmt.Errorf("invalid service %s.%s. Service actions can only have at the most two return values",
					t.Elem().Name(), method.Name)
			} else if size > 1 && errorType != method.Type.Out(1) {
				return nil, fmt.Errorf("invalid service %s.%s. In a two return values actions the second can only be an error. Found %s",
					t.Elem().Name(), method.Name, method.Type.Out(1))
			}

			action.callFilter = &Filter{handler: createCallHandler(logger, payloadType, hasContext, v.Method(i))}
			rpc.actions = append(rpc.actions, action)
		}
	}

	return rpc, nil
}

func (r *JsonRpc) SetFilters(filters ...Handler) {
	r.filters = convertHandlers(filters...)
}

func (r *JsonRpc) GetAction(actionName string) *Action {
	for _, v := range r.actions {
		if v.name == actionName {
			return v
		}
	}

	panic("The action " + actionName + " was not found in service")
}

func (r *JsonRpc) SetActionFilters(actionName string, filters ...Handler) {
	action := r.GetAction(actionName)
	action.SetFilters(filters...)
}

func (r *JsonRpc) Build(servicePath string) []*Filter {
	var prefix string
	if servicePath == "" {
		prefix = r.servicePath
	} else {
		prefix = servicePath
	}
	prefix += "/"
	var filters []*Filter

	if len(r.filters) > 0 {
		filters = r.filters
		filters[0].setRule(nil, prefix+WILDCARD)
	} else {
		filters = make([]*Filter, 0)
	}

	for _, v := range r.actions {
		f := v.filters
		f = append(f, v.callFilter)
		// apply rule to the first one
		f[0].setRule(nil, prefix+v.name)
		filters = append(filters, f...)
	}

	// guard
	f := NewFilter(prefix+WILDCARD, func(c IContext) error {
		http.Error(c.GetResponse(), "Unknown Service "+c.GetRequest().URL.Path, http.StatusNotFound)
		return nil
	})
	filters = append(filters, f)

	return filters
}

var (
	errorType   = reflect.TypeOf((*error)(nil)).Elem()    // interface type
	contextType = reflect.TypeOf((*IContext)(nil)).Elem() // interface type
)

func createCallHandler(logger Logger, payloadType reflect.Type, hasContext bool, method reflect.Value) Handler {
	return func(ctx IContext) error {
		w := ctx.GetResponse()
		r := ctx.GetRequest()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Expires", "-1")

		var payload []byte
		if r.Body != nil {
			var err error
			if payload, err = ioutil.ReadAll(r.Body); err != nil {
				return err
			}
		}

		var param reflect.Value
		var err error
		if payloadType != nil {
			// get pointer
			param = reflect.New(payloadType)
			// TODO: what happens if args is "null" ???
			err = json.Unmarshal(payload, param.Interface())
			if err != nil {
				logger.Errorf("An error occurred when unmarshalling the call for %s\n\tinput: %s\n\terror: %s", ctx.GetRequest().URL.Path, payload, err)
				return err
			}
		}
		params := make([]reflect.Value, 0)
		if hasContext {
			params = append(params, reflect.ValueOf(ctx))
		}
		if payloadType != nil {
			params = append(params, param.Elem())
		}

		results := method.Call(params)

		ok := true
		// check for error
		for k, v := range results {
			if v.Type() == errorType {
				if !v.IsNil() {
					return v.Interface().(error)
				}
				break
			} else {
				ok = false
				// stores the result to return at the end of the check
				data := results[k].Interface()
				result, err := json.Marshal(data)
				if err == nil {
					_, err = ctx.GetResponse().Write(result)
				}
				if err != nil {
					logger.Errorf("An error occurred when marshalling the response from %s\n\tresponse: %v\n\terror: %s", ctx.GetRequest().URL.Path, data, err)
					return err
				}
			}
		}
		if ok {
			// make sure the status is OK, to prevent the case where there is no result
			ctx.GetResponse().WriteHeader(http.StatusOK)
		}

		return nil
	}
}

func isExported(name string) bool {
	return unicode.IsUpper(rune(name[0]))
}
