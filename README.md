# sup

Simple Supervised Process Trees for Go with Contexts

[![GoDoc](https://godoc.org/github.com/johannesh/sup?status.svg)](https://godoc.org/github.com/johannesh/sup)
[![Build Status](https://travis-ci.org/johannesh/sup.svg?branch=master)](https://travis-ci.org/johannesh/sup)
[![codecov](https://codecov.io/gh/johannesh/sup/branch/master/graph/badge.svg)](https://codecov.io/gh/johannesh/sup)
[![Go Report Card](https://goreportcard.com/badge/github.com/johannesh/sup)](https://goreportcard.com/report/github.com/johannesh/sup)


## Example

In this example we start a supervisor from main and wire things up for orderly shutdown.
The supervisor has two children, _left_ and _right_. Left keeps crashing once every second, while right continues to serve HTTP requests.

Prelude with imports:

```go
package main

import (
	"github.com/johannesh/sup"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)
```

Main handles SIGINT and SIGTERM and cancels the root context:

```go
func main() {
	// Handle signals for orderly shutdown.
	exit := make(chan os.Signal, 1)
	defer close(exit)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-exit // On exit signal cancel root context.
		cancel()
		log.Println("Terminating...")
	}()

	log.Println("Starting...")

	err := runMainSupervisor(ctx)
	if err != nil {
		os.Exit(1)
	}
}
```

For more on contexts see [Go Concurrency Patterns: Context](https://blog.golang.org/context).

Execute main supervisor via `runMainSupervisor`. The supervisor performs an orderly shutdown whenever the parent context is cancelled.

```go
func runMainSupervisor(ctx context.Context) error {
	return sup.Supervise(ctx,
		sup.Flags{
			Duration:  500 * time.Millisecond,
			Intensity: 1,
		},
		runLeftChild,
		runRightChild,
	)
}
```

Keep crashing (i.e. return an error) once every second. The application continues to run since errors occur at half the maximum rate; Once per second vs. twice per second.

```go
func runLeftChild(ctx context.Context) error {
	log.Println("left child started")
	time.Sleep(time.Second)
	return errors.New("left child error")
}
```

The right child continues to serve HTTP requests independent of the left child.

```go
func runRightChild(ctx context.Context) error {
	lis, err := net.Listen("tcp", ":8899")
	if err != nil {
		return err
	}
	defer lis.Close()

	errs := make(chan error, 1)
	go func() { errs <- http.Serve(lis, &handler{}) }()

	select {
	case <-ctx.Done():
		lis.Close()
		<-errs
		return ctx.Err()
	case err := <-errs:
		return err
	}
}

type handler struct{}

func (_ *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}
```

If the left child were to crash twice per second then the parent supervisor would shutdown as the maximum error threshold is reached.
