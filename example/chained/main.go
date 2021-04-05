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
	fmt.Println("requesting", c.GetRequest().URL.Path)
	return c.Proceed()
}

// Hello World filter
func helloWorld(c maze.IContext) error {
	return c.JSON(http.StatusOK, "Hello World!")
}

func main() {
	// creates maze with the default context factory.
	mz := maze.NewMaze()

	mz.GET("/*", trace, helloWorld)

	fmt.Println("Listening at port 8888")
	if err := mz.ListenAndServe(":8888"); err != nil {
		panic(err)
	}
}
