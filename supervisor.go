package sup

import (
	"golang.org/x/net/context"
	"math"
	"sync"
	"time"
)

type Flags struct {
	Strategy  Strategy
	Intensity int
	Duration  time.Duration
}

type Strategy int

const (
	OneForOne Strategy = iota
	OneForAll
)

type Child func(context.Context) error
type exit struct {
	fun Child
	err error
}

func Supervise(pctx context.Context, flags Flags, children ... Child) error {
	exits := make(chan *exit, 1)
	defer close(exits)

	started := make(chan struct{}, 1)
	defer close(started)

	if flags.Duration == 0 {
		flags.Duration = time.Second
	}
	if flags.Intensity == 0 {
		flags.Intensity = 1
	}

	failures := 0.0

restart:
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	wg := &sync.WaitGroup{}
	ccount := len(children)
	wg.Add(ccount)

	for _, f := range children {
		childf := f
		go exec(ctx, wg, exits, childf)
	}

	for {
		lastErrorAt := time.Now()

		select {
		case <-ctx.Done():
			flush(exits)
			wg.Wait()
			return ctx.Err()

		case exit := <-exits:
			ccount--
			if exit.err == nil {
				if (ccount == 0) {
					return nil
				}
				continue
			}

			since := time.Now().Sub(lastErrorAt)
			intvs := float64(since / flags.Duration)
			failures = failures*math.Pow(0.5, intvs) + 1

			if failures > float64(flags.Intensity) {
				flush(exits)
				wg.Wait()
				return exit.err
			}

			if flags.Strategy == OneForOne {
				wg.Add(1)
				ccount++
				go exec(ctx, wg, exits, exit.fun)
				continue
			}

			if flags.Strategy == OneForAll {
				cancel()
				wg.Wait()
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

func exec(ctx context.Context, wg *sync.WaitGroup, exits chan *exit, childf Child) {
	errs := make(chan error, 1)
	defer close(errs)
	go func() { errs <- childf(ctx) }()
	select {
	case <-ctx.Done():
		<-errs
		exits <- &exit{fun: childf, err: ctx.Err()}
		wg.Done()
	case err := <-errs:
		exits <- &exit{fun: childf, err: err}
		wg.Done()
	}
}
