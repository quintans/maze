package main

import (
	"fmt"
	"net/http"

	"github.com/quintans/maze"
)

func main() {
	// creates maze with the default context factory.
	var mz = maze.NewMaze(nil)

	// Hello World filter
	mz.GET("/*", func(c maze.IContext) error {
		_, err := c.GetResponse().Write([]byte("Hello World!"))
		return err
	})

	mux := http.NewServeMux()
	mux.Handle("/", mz)

	fmt.Println("Listening at port 8888")
	if err := http.ListenAndServe(":8888", mux); err != nil {
		panic(err)
	}
}