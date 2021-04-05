package main

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/quintans/maze"
	"github.com/quintans/toolkit/log"
	"github.com/stretchr/testify/require"
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
func hiWorld(c maze.IContext) error {
	return c.JSON(http.StatusOK, "HI World!")
}

// Hello World filter
func helloWorld(c maze.IContext) error {
	return c.JSON(http.StatusOK, "Hello World!")
}

func TestChained(t *testing.T) {
	go server()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:8888/hello")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, `"Hello World!"`, string(body))
}

func server() {
	// creates maze with the default context factory.
	mz := maze.NewMaze()

	mz.GET("/hi", trace, hiWorld)
	mz.GET("/hello", trace, helloWorld)

	fmt.Println("Listening at port 8888")
	if err := mz.ListenAndServe(":8888"); err != nil {
		panic(err)
	}
}
