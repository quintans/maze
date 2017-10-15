# maze
HTTP router based on chained filter rules

Maze enable us to define a set of request interceptors, called filters, where we can do stuff,
like beginning a database transaction or checking the user credencials.

Lets start with the classic "Hello World"

```go
package main

import (
	"github.com/quintans/maze"
	"github.com/quintans/toolkit/log"
)

func init() {
	maze.SetLogger(log.LoggerFor("github.com/quintans/maze"))
}

func main() {
	// creates maze with the default context factory.
	var mz = maze.NewMaze(nil)

	// Hello World filter
	mz.GET("/*", func(c maze.IContext) error {
		return c.TEXT("Hello World!")
	})

	if err := mz.ListenAndServe(":8888"); err != nil {
		panic(err)
	}
}
```

In this example, any GET will return "Hello World!", because every url matches the filter "/*".
The asterisk means anything.
We can filter what ends with "*" (e.g: "/static/*") or what begins with "*" (e.g: "*.js").

We can chain filters. Inside a filter, if we want to call the next filter in the chain
we just use the Proceed() method of the context.

The following example shows the chaining of filters and the use of Proceed().
We move the Hello filter to a method and add another filter to trace the requests.

```go
// logs request path
func trace(c maze.IContext) error {
	logger.Debugf("requesting %s", c.GetRequest().URL.Path)
	return c.Proceed()
}

// Hello World filter
func helloWorld(c maze.IContext) error {
	return c.TEXT("Hello World!")
}

func main() {
	//...

	mz.GET("/*", trace, helloWorld)

	//...
}

```

In each filter we decide if we want to proceed or to return and ending the request.

We can also define rules to declare REST endpoints like this: "/rest/greet/sayhi/:Id"

Here is a complete example:

TODO: this example uses low level functions. Use high level functions and a simpler example.

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/quintans/maze"
	"github.com/quintans/toolkit/log"
)

func init() {
	maze.SetLogger(log.LoggerFor("github.com/quintans/maze"))
}

// JSONProducer adds the headers for a json reply
// This is a demonstrative example. Usually this is not needed.
func JSONProducer(ctx maze.IContext) error {
	w := ctx.GetResponse()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Expires", "-1")

	return ctx.Proceed()
}

type GreetingService struct{}

func (this *GreetingService) SayHi(ctx maze.IContext) error {
	var q struct {
		Id   int    `schema:"id"`
		Name string `schema:"name"`
	}
	if err := ctx.Vars(&q); err != nil {
		return err
	}

	return ctx.(*AppCtx).Reply(fmt.Sprintf("Hi %s. Your ID is %d", q.Name, q.Id))
}

type AppCtx struct {
	*maze.Context
}

func (this *AppCtx) Proceed() error {
	return this.Next(this)
}

// Reply writes in JSON format.
// It overrides Context.Reply()
// This is a demonstrative example. Usually we would use maze.IContext.JSON()
func (this *AppCtx) Reply(value interface{}) error {
	result, err := json.Marshal(value)
	if err == nil {
		_, err = this.Response.Write(result)
	}
	return err
}

func main() {
	// creates maze with specialized context factory.
	var mz = maze.NewMaze(func(w http.ResponseWriter, r *http.Request, filters []*maze.Filter) maze.IContext {
		var ctx = new(AppCtx)
		ctx.Context = maze.NewContext(w, r, filters)
		return ctx
	})

	var greetingsService = new(GreetingService)
	// we apply a filter to requests starting with /rest/greet/*
	mz.Push("/rest/greet/*", JSONProducer)

	// since the rule does not start with '/' and the last rule ended with '*'
	// the applied rule will be the concatenation of the previous one
	// with this one resulting in "/rest/greet/sayhi/:Id"
	mz.GET("sayhi/:Id", greetingsService.SayHi)

	fmt.Println("Listening at port 8888")
	if err := mz.ListenAndServe(":8888"); err != nil {
		panic(err)
	}
}
```

Run it and access [http://localhost:8888/rest/greet/sayhi/123?name=Quintans](http://localhost:8888/rest/greet/sayhi/123?name=Quintans)
