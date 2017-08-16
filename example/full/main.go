package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/quintans/maze"
	"github.com/quintans/toolkit/log"
	"github.com/quintans/toolkit/web"
)

func init() {
	maze.SetLogger(log.LoggerFor("github.com/quintans/maze"))
}

const (
	COUNTER = "counter"
)

// authorization filter
func UnauthorizedFilter(ctx maze.IContext) error {
	logger.Debugf("executing UnauthorizedFilter()")
	http.Error(ctx.GetResponse(), "Unauthorized Access", http.StatusUnauthorized)
	return nil
}

// end point
func counter(c maze.IContext) error {
	logger.Debugf("executing counter()")

	var ctx = c.(*AppCtx)
	session := ctx.Session
	var count int = 0
	cnt := session.Get(COUNTER)
	if cnt != nil {
		count = cnt.(int) + 1
	}
	session.Put(COUNTER, count)

	fmt.Fprintln(ctx.GetResponse(), "Count: ", count)
	return nil
}

// hello world
func hello(ctx maze.IContext) error {
	logger.Debugf("executing hello()")

	return ctx.JSON("Hello World!")
}

// dummy test
func mark(ctx maze.IContext) error {
	logger.Debug("requesting", ctx.GetRequest().URL.Path)
	return ctx.Proceed()
}

func hasRole(roles ...string) func(ctx maze.IContext) error {
	return func(ctx maze.IContext) error {
		logger.Debugf("executing hasRole(%s)", roles)

		fmt.Println(fmt.Sprintf(">>> checking for role(s) %s <<<\n", roles))
		return ctx.Proceed()
	}
}

// redirects / to homte.html
func HomeHandler(ctx maze.IContext) error {
	w := ctx.GetResponse()
	r := ctx.GetRequest()
	logger.Debugf("executing HomeHandler() on %s", r.URL.Path)

	logger.Debugf("redirecting...")
	http.Redirect(w, r, "/static/home.html", http.StatusMovedPermanently)
	return nil
}

// 16MB
const post_limit = 1 << 24

// limits the body of a post
func limit(ctx maze.IContext) error {
	logger.Debugf("executing limit()")

	ctx.GetRequest().Body = http.MaxBytesReader(ctx.GetResponse(), ctx.GetRequest().Body, post_limit)
	return ctx.Proceed()
}

// file upload
func upload(ctx maze.IContext) error {
	logger.Debugf("executing upload()")

	w := ctx.GetResponse()
	r := ctx.GetRequest()
	fn, _, err := r.FormFile("content")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	defer fn.Close()

	//data, err := ioutil.ReadAll(file)
	//if err != nil {
	//	return err
	//}

	f, _ := ioutil.TempFile("./upload", "upload")
	defer f.Close()
	defer os.Remove(f.Name())
	io.Copy(f, fn)

	// do something with file, like storing in a database

	fmt.Fprintln(ctx.GetResponse(), "uploaded to ", f.Name())
	return nil
}

type AddIn struct {
	A int64
	B int64
}

type InfoOut struct {
	Name string
	Age  int
}

type GreetingService struct {
}

func (this *GreetingService) SayHello(name string) string {
	return "Hello " + name
}

func (this *GreetingService) Add(in AddIn) int64 {
	return in.A + in.B
}

func (this *GreetingService) Info() InfoOut {
	return InfoOut{"Paulo", 41}
}

func (this *GreetingService) SayHi(ctx maze.IContext) error {
	var q struct {
		Id   int    `schema:"id"`
		Name string `schema:"name"`
	}
	if err := ctx.Vars(&q); err != nil {
		return err
	}

	return ctx.JSON("Hi " + q.Name + ". Your ID is " + strconv.Itoa(q.Id))
}

var logger = log.LoggerFor("main")

func init() {
	/*
	 * ===================
	 * BEGIN CONFIGURATION
	 * ===================
	 */
	logLevel := flag.Int("logLevel", int(log.DEBUG), "log level. values between DEBUG=0, INFO, WARN, ERROR, FATAL, NONE=6. default: DEBUG")
	flag.Parse()
	var show = *logLevel <= int(log.INFO)
	log.Register("/", log.LogLevel(*logLevel), log.NewConsoleAppender(false)).ShowCaller(show)

	//log.SetLevel("pqp", log.DEBUG)

	/*
	 * ===================
	 * END CONFIGURATION
	 * ===================
	 */
}

type AppCtx struct {
	*maze.Context

	Session web.ISession
}

// THIS IS IMPORTANT.
// this way in the handlers we can cast to the specialized context
func (this *AppCtx) Proceed() error {
	return this.Next(this)
}

func (this *AppCtx) Reply(value interface{}) error {
	result, err := json.Marshal(value)
	if err == nil {
		w := this.GetResponse()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Expires", "-1")

		_, err = this.Response.Write(result)
	}
	return err
}

func main() {
	defer func() {
		// give time for the loggers to write
		time.Sleep(100 * time.Millisecond)

		err := recover()
		if err != nil {
			fmt.Printf("%s\n%s\n", err, debug.Stack())
		}
	}()

	// creates maze with context factory.
	var mz = maze.NewMaze(func(w http.ResponseWriter, r *http.Request, filters []*maze.Filter) maze.IContext {
		var ctx = new(AppCtx)
		ctx.Context = maze.NewContext(w, r, filters)
		return ctx
	})
	// limits size
	mz.Push("/*", limit)
	// logs request path
	mz.Push("/*", mark)

	// handles server sessions
	sessions := web.NewSessions(web.SessionsConfig{
		Timeout:  2 * time.Minute,
		Interval: time.Minute,
	})
	// if this filter is used it has to be the first to write to response
	mz.Push("/app/*", func(c maze.IContext) error {
		logger.Debugf("executing SessionFilter()")

		var ctx = c.(*AppCtx)
		// (re)writes the session cookie to the response
		ctx.Session = sessions.GetOrCreate(ctx.GetResponse(), ctx.GetRequest(), true)
		return ctx.Proceed()
	})

	mz.Push("/app/cnt/*", counter)
	//.PushF("/app/xxx*", hello)

	// secure content
	mz.Push("/static/private/*", UnauthorizedFilter)

	// delivering static content and preventing malicious access
	fs := web.OnlyFilesFS{http.Dir("./")}
	fileServer := http.FileServer(fs)
	//http.Handle("/static/", http.FileServer(fs))
	mz.GET("/static/*", func(ctx maze.IContext) error {
		logger.Debugf("executing static()")
		fileServer.ServeHTTP(ctx.GetResponse(), ctx.GetRequest())
		return nil
	})
	// or
	// mz.Static("/static/*", "./")

	mz.Push("/upload/*", upload)
	// JSON-RPC services
	var greetingsService = new(GreetingService)
	var rpc = maze.NewJsonRpc(greetingsService)
	rpc.SetActionFilters("SayHello", hasRole("user", "admin")) // filters specific action of the service
	mz.Add(rpc.Build("/json/greeting")...)

	mz.Push("/rest/greet/*", hasRole("super"))
	// the applied rule will be "/rest/greet/sayhi/:Id"
	mz.GET("sayhi/:Id", greetingsService.SayHi)
	// guard - if this valid and it reached here it means the service endpoint is invalid
	mz.Push("/rest/greet/*", func(c maze.IContext) error {
		http.Error(c.GetResponse(), "Unknown Service "+c.GetRequest().URL.Path, http.StatusNotFound)
		return nil
	})

	// redirects to the homepage if uri = '/'
	mz.Push("/", HomeHandler) // homepage

	// 404
	mz.Push("/*", func(c maze.IContext) error {
		http.NotFound(c.GetResponse(), c.GetRequest())
		return nil
	})

	fmt.Println("Listening at port 8888")
	if err := mz.ListenAndServe(":8888"); err != nil {
		panic(err)
	}
}
