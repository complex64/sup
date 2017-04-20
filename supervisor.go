package sup

import (
	"golang.org/x/net/context"
	"math"
	"sync"
	"time"
)

// Flags configures supervisor's behavior in case a child processes terminates with an error.
// It limits the number of restarts which can occur in a given time interval.
// This is specified by the two elements Intensity and Duration.
// If more than Intensity number of child processes terminate with an error within Duration, then the supervisor
// terminates all children.
type Flags struct {
	Strategy  Strategy      // Default: OneForOne
	Intensity int           // Default: 1
	Duration  time.Duration // Default: 1*time.Second
}

// Strategy configures how children are restarted in case they terminate with an error
type Strategy int

const (
	// OneForOne restarts only the child process that terminated
	OneForOne Strategy = iota
	// OneForAll restarts all other child processes including the one that terminated
	// Child processes that previously finished are restarted as well
	OneForAll
)

// Passed on exit to supervisor process for every child that terminates.
type exit struct {
	fun func(context.Context) error
	err error
}

// Supervise creates a supervisor process as part of a supervision tree.
// The created supervisor process is configured with a restart strategy,
// a maximum restart intensity, and a list of child processes.
// Supervise returns only after all child processes terminated.
// All child processes are started asynchronously.
func Supervise(parentCtx context.Context, flags Flags, children ...func(context.Context) error) error {
	// Channel to monitor child exits
	exits := make(chan *exit, 1)

	// Apply defaults
	if flags.Duration == 0 {
		flags.Duration = time.Second
	}
	if flags.Intensity == 0 {
		flags.Intensity = 1
	}

	failureRate := 0.0

restart:
	childCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	// Non-zero as long as children are still running
	nChildren := len(children)
	childrenWg := &sync.WaitGroup{}
	childrenWg.Add(nChildren)

	for _, childF := range children {
		f := childF
		go runChild(childCtx, childrenWg, exits, f)
	}

	for {
		lastErrorAt := time.Now()

		select {
		case <-parentCtx.Done(): // -> childCtx cancelled too
			flush(exits)
			childrenWg.Wait()
			return parentCtx.Err()

		case exit := <-exits:
			nChildren--

			if exit.err == nil {
				if nChildren == 0 {
					childrenWg.Wait() // Exit may be received before call to wait group from runChild
					return nil
				}
				continue
			}

			// Decay and increment failure rate
			since := time.Now().Sub(lastErrorAt)
			intvs := float64(since / flags.Duration)
			failureRate = failureRate*math.Pow(0.5, intvs) + 1

			// Threshold reached: Terminate all children and return error that triggered threshold
			if int(failureRate) > flags.Intensity {
				cancel()
				flush(exits)
				childrenWg.Wait()
				return exit.err
			}

			switch flags.Strategy {
			case OneForOne:
				childrenWg.Add(1) // Responsibility to decrement on exit is with runChild
				nChildren++
				go runChild(childCtx, childrenWg, exits, exit.fun)
				continue

			case OneForAll:
				cancel()
				childrenWg.Wait()
				goto restart
			}
		}
	}
}

func flush(exits chan *exit) {
	go func() {
		for {
			_, ok := <-exits
			if !ok {
				return
			}
		}
	}()
}

func runChild(ctx context.Context, childrenWg *sync.WaitGroup, exits chan *exit, childF func(context.Context) error) {
	errs := make(chan error, 1)
	defer close(errs)
	go func() { errs <- childF(ctx) }()

	// Pass exit status to supervisor, signal termination on the children wait group
	select {
	case <-ctx.Done():
		<-errs // Child context cancelled as well, wait for termination
		exits <- &exit{fun: childF, err: ctx.Err()}
		childrenWg.Done()
	case err := <-errs:
		exits <- &exit{fun: childF, err: err}
		childrenWg.Done()
	}
}
