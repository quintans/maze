package main

import (
	"fmt"

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
		_, err := c.GetResponse().Write([]byte("Hello World!"))
		return err
	})

	fmt.Println("Listening at port 8888")
	if err := mz.ListenAndServe(":8888"); err != nil {
		panic(err)
	}
}
