package sup

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"sync"
	"testing"
	"time"
)

func Test_execReturn(t *testing.T) {
	exits := make(chan *exit, 1)
	f := func(ctx context.Context) error { return nil }
	wg := &sync.WaitGroup{}
	wg.Add(1)

	runChild(context.Background(), wg, exits, f)

	wg.Wait()
	e := <-exits
	assert.Nil(t, e.err)
	assert.ObjectsAreEqual(f, e.fun)
}

func Test_execCancel(t *testing.T) {
	exits := make(chan *exit, 2)
	continuef := make(chan struct{}, 1)
	f := func(ctx context.Context) error {
		<-continuef
		return errors.New("errors")
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { runChild(ctx, wg, exits, f) }()

	cancel()
	continuef <- struct{}{}

	wg.Wait()
	exit := <-exits

	assert.EqualError(t, exit.err, "context canceled")
}

func Test_superviseSingleReturn(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	err := Supervise(context.Background(), Flags{}, func(ctx context.Context) error {
		defer wg.Done()
		return nil
	})
	wg.Wait()
	assert.Nil(t, err)
}

func Test_superviseDoubleReturn(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	err := Supervise(context.Background(), Flags{},
		func(ctx context.Context) error {
			defer wg.Done()
			return nil
		},
		func(ctx context.Context) error {
			defer wg.Done()
			return nil
		},
	)
	wg.Wait()
	assert.Nil(t, err)
}

func Test_superviseSingleCrash_withRestart(t *testing.T) {
	errs := make(chan error, 2)
	errs <- errors.New("first")
	errs <- nil

	wg := &sync.WaitGroup{}
	wg.Add(2)

	err := Supervise(context.Background(), Flags{}, func(ctx context.Context) error {
		defer wg.Done()
		return <-errs
	})

	wg.Wait()
	assert.Nil(t, err)
}

func Test_superviseTwoChildren_withSingleRestart(t *testing.T) {
	errs := make(chan error, 2)
	errs <- errors.New("first")
	errs <- nil

	wg := &sync.WaitGroup{}
	wg.Add(3)

	err := Supervise(context.Background(), Flags{},
		func(ctx context.Context) error {
			defer wg.Done()
			return <-errs
		},
		func(ctx context.Context) error {
			defer wg.Done()
			return nil
		},
	)

	wg.Wait()
	assert.Nil(t, err)
}

func Test_superviseTwoChildren_oneForAllRestart(t *testing.T) {
	errs := make(chan error, 2)
	errs <- errors.New("first")
	errs <- nil

	wg := &sync.WaitGroup{}
	wg.Add(4)

	err := Supervise(context.Background(),
		Flags{
			Strategy: OneForAll,
		},
		func(ctx context.Context) error {
			defer wg.Done()
			return <-errs
		},
		func(ctx context.Context) error {
			defer wg.Done()
			return nil
		},
	)

	wg.Wait()
	if err != nil {
		assert.EqualError(t, err, "context canceled")
	}
}

func Test_superviseTwoChildren_withCancelOfContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err := Supervise(ctx, Flags{},
		func(ctx context.Context) error {
			return nil
		},
		func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	)

	assert.EqualError(t, err, "context canceled")
}

func Test_superviseTwoChildren_withOneCrashingAboveThreshold(t *testing.T) {
	returnFromChild2 := make(chan struct{}, 1)

	started := make(chan struct{}, 1)
	errs := make(chan error)
	go func() {
		started <- struct{}{}
		errs <- errors.New("first")
		time.Sleep(5 * time.Millisecond)
		errs <- errors.New("second")
		time.Sleep(5 * time.Millisecond)
		errs <- errors.New("third")
	}()
	<-started

	wg := &sync.WaitGroup{}
	wg.Add(3)

	done := make(chan struct{}, 1)
	go func() {
		err := Supervise(context.Background(),
			Flags{
				Intensity: 2,
				Duration:  10 * time.Millisecond,
			},
			func(ctx context.Context) error {
				defer wg.Done()
				return <-errs
			},
			func(ctx context.Context) error {
				<-returnFromChild2
				return nil
			},
		)

		assert.EqualError(t, err, "third")
		done <- struct{}{}
	}()

	wg.Wait()
	returnFromChild2 <- struct{}{}

	<-done
}
