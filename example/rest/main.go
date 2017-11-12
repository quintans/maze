package main

import (
	"fmt"
	"net/http"

	"github.com/quintans/maze"
	"github.com/quintans/toolkit/log"
)

func init() {
	maze.SetLogger(log.LoggerFor("github.com/quintans/maze"))
}

// logs request path
func trace(c maze.IContext) error {
	fmt.Println("==> requesting", c.GetRequest().URL.Path)
	return c.Proceed()
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
// This is a demonstrative example of how we can extend Context.
func (this *AppCtx) Reply(value interface{}) error {
	return this.JSON(http.StatusOK, value)
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
	mz.Push("/rest/greet/*", trace)

	// since the rule does not start with '/' and the last rule ended with '*'
	// the applied rule will be the concatenation of the previous one
	// with this one resulting in "/rest/greet/sayhi/:Id"
	mz.GET("sayhi/:Id", greetingsService.SayHi)

	mz.GET("/*", func(ctx maze.IContext) error {
		ctx.TEXT(
			http.StatusBadRequest,
			"invalid URI.\nUse /rest/greet/sayhi/:Id[?name=Name] eg: /rest/greet/sayhi/123?name=Quintans",
		)
		return nil
	})

	fmt.Println("Listening at port 8888")
	if err := mz.ListenAndServe(":8888"); err != nil {
		panic(err)
	}
}
