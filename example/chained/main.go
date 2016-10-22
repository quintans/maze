package main

import (
	"fmt"
	"net/http"

	"github.com/quintans/maze"
)

// logs request path
func trace(c maze.IContext) error {
	fmt.Println("requesting", c.GetRequest().URL.Path)
	return c.Proceed()
}

// Hello World filter
func helloWorld(c maze.IContext) error {
	return c.JSON("Hello World!")
}

func main() {
	// creates maze with the default context factory.
	var mz = maze.NewMaze(nil)

	mz.GET("/*", trace, helloWorld)

	mux := http.NewServeMux()
	mux.Handle("/", mz)

	fmt.Println("Listening at port 8888")
	if err := http.ListenAndServe(":8888", mux); err != nil {
		panic(err)
	}
}
