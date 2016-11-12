package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/quintans/maze"
)

// JSONProducer adds the headers for a json reply
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

	return ctx.JSON("Hi " + q.Name + ". Your ID is " + strconv.Itoa(q.Id))
}

type AppCtx struct {
	*maze.Context
}

// Reply writes in JSON format.
// It overrides Context.Reply()
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
		// this is important.
		// this way in the handlers we can cast to the specialized context
		ctx.Overrider = ctx

		return ctx
	})

	var greetingsService = new(GreetingService)

	mz.Push("/rest/greet/*", JSONProducer)
	// the applied rule will be "/rest/greet/sayhi/{Id}"
	mz.GET("sayhi/{Id}", greetingsService.SayHi)

	mux := http.NewServeMux()
	mux.Handle("/", mz)

	fmt.Println("Listening at port 8888")
	if err := http.ListenAndServe(":8888", mux); err != nil {
		panic(err)
	}
}
