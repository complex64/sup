```go
package main

import (
	"github.com/johannesh/sup"
	"golang.org/x/net/context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/pkg/errors"
)

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

func runMainSupervisor(ctx context.Context) error {
	return sup.Supervise(ctx, sup.Flags{},
		runLeftChild,
		runRightChild,
	)
}

// Error once a second.
func runLeftChild(ctx context.Context) error {
	log.Println("left child says hi")
	time.Sleep(time.Second)
	return errors.New("left child error")
}

// Example: Run a HTTP server, with orderly shutdown.
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
