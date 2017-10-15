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
