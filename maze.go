package maze

import (
	"net/http"
	"strings"

	"github.com/quintans/toolkit/log"
	"github.com/quintans/toolkit/web"
)

func init() {
	logger = log.LoggerFor("github.com/quintans/maze")
}

var logger log.ILogger

func SetLogger(lgr log.ILogger) {
	logger = lgr
}

type ContextFactory func(w http.ResponseWriter, r *http.Request, filters []*Filter) IContext

type Option func(m *Maze)

func WithContextFactory(cf ContextFactory) Option {
	return func(m *Maze) {
		m.contextFactory = cf
	}
}

// NewMaze creates maze with context factory. If nil, it uses a default context factory
func NewMaze(options ...Option) *Maze {
	m := &Maze{}
	for _, o := range options {
		o(m)
	}
	return m
}

type Maze struct {
	filters        []*Filter
	contextFactory ContextFactory
	lastRule       string
}

func (m *Maze) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(m.filters) > 0 {
		var ctx IContext
		if m.contextFactory == nil {
			// default
			ctx = NewContext(w, r, m.filters)
		} else {
			ctx = m.contextFactory(w, r, m.filters)
		}
		err := ctx.Proceed()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (m *Maze) GET(rule string, filters ...Handler) {
	m.PushMethod([]string{"GET"}, rule, filters...)
}

func (m *Maze) POST(rule string, filters ...Handler) {
	m.PushMethod([]string{"POST"}, rule, filters...)
}

func (m *Maze) PUT(rule string, filters ...Handler) {
	m.PushMethod([]string{"PUT"}, rule, filters...)
}

func (m *Maze) DELETE(rule string, filters ...Handler) {
	m.PushMethod([]string{"DELETE"}, rule, filters...)
}

func (m *Maze) PATCH(rule string, filters ...Handler) {
	m.PushMethod([]string{"PATCH"}, rule, filters...)
}

func (m *Maze) Push(rule string, filters ...Handler) {
	m.PushMethod(nil, rule, filters...)
}

// PushMethod adds the filters to the end of the last filters.
// If the rule does NOT start with '/' the applied rule will be
// the concatenation of the last rule that started with '/' and ended with a '*'
// with this one (the '*' is omitted).
// ex: /greet/* + sayHi/:Id = /greet/sayHi/:Id
func (m *Maze) PushMethod(methods []string, rule string, handlers ...Handler) {
	if strings.HasPrefix(rule, "/") {
		if strings.HasSuffix(rule, "*") {
			m.lastRule = rule[:len(rule)-1]
			logger.Tracef("Last main rule set as %s", m.lastRule)
		} else {
			// resets lastRule
			m.lastRule = ""
		}
	} else if !strings.HasPrefix(rule, "*") {
		if m.lastRule == "" {
			rule = "/" + rule
		} else {
			rule = m.lastRule + rule
		}
	}

	if len(handlers) > 0 {
		f := convertHandlers(handlers...)
		// rule is only set for the first filter
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

	logger.Infof("Listening http at %s", addr)
	return http.ListenAndServe(addr, mux)
}
