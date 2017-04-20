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

type Child struct {
	Name     string
	Function func(context.Context) error
}

// Passed on exit to supervisor process for every child that terminates.
type exit struct {
	child *Child
	err   error
}

// Supervise creates a supervisor process as part of a supervision tree.
// The created supervisor process is configured with a restart strategy,
// a maximum restart intensity, and a list of child processes.
//
// Supervise returns only after all child processes terminated or once the passed context is canceled.
// Children are passed a context that is derived from the passed context
//
// All child processes are started asynchronously.
func Supervise(name string, ctx context.Context, flags Flags, children ...Child) (err error) {
	if name == "" {
		name = "unnamed"
	}
	defer func(e *error) { log_(Info, "%s exited with '%v'.", name, *e) }(&err)

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
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Non-zero as long as children are still running
	nChildren := len(children)
	childrenWg := &sync.WaitGroup{}
	childrenWg.Add(nChildren)

	for _, child := range children {
		c := child
		log_(Info, "%s starting child '%s' [%v]...", name, c.Name, c.Function)
		go runChild(name, childCtx, childrenWg, exits, &c)
	}

	for {
		lastErrorAt := time.Now()

		select {
		case <-ctx.Done(): // -> childCtx cancelled too
			log_(Debug, "%s context closed.", name)
			flush(exits)
			childrenWg.Wait()
			err = ctx.Err()
			return

		case exit := <-exits:
			nChildren--

			if exit.err == nil {
				if nChildren == 0 {
					childrenWg.Wait() // Exit may be received before call to wait group from runChild
					return
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
				err = exit.err
				return
			}

			switch flags.Strategy {
			case OneForOne:
				log_(Info, "%s restarting single child %s [%v]...", name, exit.child.Name, exit.child.Function)
				childrenWg.Add(1) // Responsibility to decrement on exit is with runChild
				nChildren++
				go runChild(name, childCtx, childrenWg, exits, exit.child)
				continue

			case OneForAll:
				log_(Info, "%s restarting all children...", name)
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

func runChild(parent string, ctx context.Context, childrenWg *sync.WaitGroup, exits chan *exit, child *Child) {
	var err error
	defer func(e *error) {
		log_(Info, "%s child %v [%v] exited with '%v'.", parent, child.Name, child.Function, *e)
	}(&err)

	errs := make(chan error, 1)
	defer close(errs)

	go func() { errs <- child.Function(ctx) }()

	// Pass exit status to supervisor, signal termination on the children wait group
	select {
	case <-ctx.Done():
		err = <-errs // Child context cancelled as well, wait for termination
		exits <- &exit{child: child, err: ctx.Err()}
		childrenWg.Done()
	case err = <-errs:
		exits <- &exit{child: child, err: err}
		childrenWg.Done()
	}
}
