package maze

import (
	"net/http"
	"strings"
)

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

func (f *Filter) setRule(methods []string, rule string) {
	if rule != "" {
		if strings.HasPrefix(rule, WILDCARD) {
			f.route = rule[1:]
			f.wildcard = WILDCARD_BEFORE
		} else if strings.HasSuffix(rule, WILDCARD) {
			f.route = rule[:len(rule)-1]
			f.wildcard = WILDCARD_AFTER
		} else {
			f.route = rule
			if i := strings.Index(rule, ":"); i != -1 {
				f.template = strings.Split(rule, "/")
			}
		}
	}
	f.allowedMethods = methods
}

func (f *Filter) String() string {
	var str string
	if f.wildcard == WILDCARD_BEFORE {
		str = WILDCARD
	}
	str += f.route
	if f.wildcard == WILDCARD_AFTER {
		str += WILDCARD
	}
	return str
}

func NewFilter(rule string, handler Handler) *Filter {
	f := new(Filter)
	f.setRule(nil, rule)
	f.handler = handler
	return f
}

func (f *Filter) IsValid(request *http.Request) bool {
	if f.route == "" {
		return false
	}

	// verify if method is allowed
	var allowed bool
	if f.allowedMethods == nil {
		allowed = true
	} else {
		method := request.Method
		if method == "" {
			method = http.MethodGet
		}
		for _, v := range f.allowedMethods {
			if method == v {
				allowed = true
				break
			}
		}
	}

	if allowed {
		path := request.URL.Path
		if f.wildcard == WILDCARD_BEFORE {
			return strings.HasSuffix(path, f.route)
		} else if f.wildcard == WILDCARD_AFTER {
			return strings.HasPrefix(path, f.route)
		} else if f.template != nil {
			return f.validate(path)
		} else {
			return path == f.route
		}
	}

	return false
}

// validate checks if its a valid match with the url template
func (f *Filter) validate(path string) bool {
	parts := strings.Split(path, "/")

	if len(parts) != len(f.template) {
		return false
	}

	for k, v := range f.template {
		if !strings.HasPrefix(v, ":") && v != parts[k] {
			return false
		}
	}

	return true
}

func convertHandlers(handlers ...Handler) []*Filter {
	filters := make([]*Filter, len(handlers))
	for k, v := range handlers {
		filters[k] = &Filter{handler: v}
	}
	return filters
}
