package maze

import (
	"net/http"
	"strings"

	"github.com/quintans/toolkit/web"
	"github.com/sirupsen/logrus"
)

type ContextFactory func(l Logger, w http.ResponseWriter, r *http.Request, filters []*Filter) IContext

type Option func(m *Maze)

func WithContextFactory(cf ContextFactory) Option {
	return func(m *Maze) {
		m.contextFactory = cf
	}
}

func WithLogger(l Logger) Option {
	return func(m *Maze) {
		m.logger = l
	}
}

// NewMaze creates maze with context factory. If nil, it uses a default context factory
func NewMaze(options ...Option) *Maze {
	m := &Maze{
		logger: NewLogrus(logrus.StandardLogger()),
	}
	for _, o := range options {
		o(m)
	}
	return m
}

type Maze struct {
	logger         Logger
	filters        []*Filter
	contextFactory ContextFactory
	lastRule       string
}

func (m *Maze) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(m.filters) > 0 {
		var ctx IContext
		if m.contextFactory == nil {
			// default
			ctx = NewContext(m.logger, w, r, m.filters)
		} else {
			ctx = m.contextFactory(m.logger, w, r, m.filters)
		}
		err := ctx.Proceed()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (m *Maze) GET(rule string, filters ...Handler) {
	m.PushMethod([]string{http.MethodGet}, rule, filters...)
}

func (m *Maze) POST(rule string, filters ...Handler) {
	m.PushMethod([]string{http.MethodPost}, rule, filters...)
}

func (m *Maze) PUT(rule string, filters ...Handler) {
	m.PushMethod([]string{http.MethodPut}, rule, filters...)
}

func (m *Maze) DELETE(rule string, filters ...Handler) {
	m.PushMethod([]string{http.MethodDelete}, rule, filters...)
}

func (m *Maze) PATCH(rule string, filters ...Handler) {
	m.PushMethod([]string{http.MethodPatch}, rule, filters...)
}

func (m *Maze) Push(rule string, filters ...Handler) {
	m.PushMethod(nil, rule, filters...)
}

// PushMethod adds the filters to the end of the last filters.
// If the current rule does NOT start with '/', the applied rule will be
// the concatenation of the last rule that started with '/' and ended with a '*'
// with this current one (the '*' is omitted).
// eg: /greet/* + sayHi/:Id = /greet/sayHi/:Id
func (m *Maze) PushMethod(methods []string, rule string, handlers ...Handler) {
	if strings.HasPrefix(rule, "/") {
		if strings.HasSuffix(rule, WILDCARD) {
			m.lastRule = rule[:len(rule)-1]
			m.logger.Debugf("Last main rule set as %s", m.lastRule)
		} else {
			// resets lastRule
			m.lastRule = ""
		}
	} else if !strings.HasPrefix(rule, WILDCARD) {
		if m.lastRule == "" {
			rule = "/" + rule
		} else {
			rule = m.lastRule + rule
		}
	}

	if len(handlers) > 0 {
		f := convertHandlers(handlers...)
		// rule is only set for the first filter
		m.logger.Infof("registering rule %s", rule)
		f[0].setRule(methods, rule)
		m.filters = append(m.filters, f...)
	}
}

func (m *Maze) Add(filters ...*Filter) {
	m.filters = append(m.filters, filters...)
}

// Static serves static content.
// rule defines the rule and dir the relative path
func (m *Maze) Static(rule string, dir string) {
	// delivering static content and preventing malicious access
	fs := web.OnlyFilesFS{Fs: http.Dir(dir)}
	fileServer := http.FileServer(fs)
	m.GET(rule, func(ctx IContext) error {
		fileServer.ServeHTTP(ctx.GetResponse(), ctx.GetRequest())
		return nil
	})
}

func (m *Maze) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/", m)

	m.logger.Infof("Listening http at %s", addr)
	return http.ListenAndServe(addr, mux)
}
