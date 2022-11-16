package worker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func Test_Limiter_Run(t *testing.T) {
	ignorefunc := "go.opencensus.io/stats/view.(*worker).start"
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction(ignorefunc))

	limiter := NewLimiter(5)

	returnCh := make(chan struct{})

	count := 3
	for i := 0; i < count; i++ {
		err := limiter.Dispatch(func() {
			returnCh <- struct{}{}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < count; i++ {
		<-returnCh
	}

	limiter.StopWait()
}

func Test_Limiter_Run_limits(t *testing.T) {
	ignorefunc := "go.opencensus.io/stats/view.(*worker).start"
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction(ignorefunc))

	limiter := NewLimiter(3)

	returnCh := make(chan struct{})

	count := 3
	for i := 0; i < count; i++ {
		err := limiter.Dispatch(func() {
			<-returnCh
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// add another func exceeding concurrency limit of 3
	err := limiter.Dispatch(func() {
		t.Fatal("expected limiter to limit concurrency")
	})
	if err == nil {
		t.Fatal("expected limiter to limit concurrency, by returning an error")
	}

	assert.ErrorIs(t, err, ErrLimiterConcurrency)

	// unblock routines
	for i := 0; i < count; i++ {
		returnCh <- struct{}{}
	}

	limiter.StopWait()
}

func Test_Limiter_Active(t *testing.T) {
	ignorefunc := "go.opencensus.io/stats/view.(*worker).start"
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction(ignorefunc))

	limiter := NewLimiter(5)

	// release causes the job to return
	releaseCh := make(chan struct{})

	count := 3
	for i := 0; i < count; i++ {
		err := limiter.Dispatch(func() {
			<-releaseCh
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// test active jobs are as expected
	assert.Equal(t, count, limiter.ActiveCount())

	for i := 0; i < count; i++ {
		// cause job to return
		releaseCh <- struct{}{}
	}

	limiter.StopWait()

	assert.Equal(t, 0, limiter.ActiveCount())
}

func Test_Limiter_StopWait(t *testing.T) {
	ignorefunc := "go.opencensus.io/stats/view.(*worker).start"
	defer goleak.VerifyNone(t, goleak.IgnoreTopFunction(ignorefunc))

	limiter := NewLimiter(5)

	returnCh := make(chan struct{})

	count := 3
	for i := 0; i < count; i++ {
		err := limiter.Dispatch(func() {
			time.Sleep(1 * time.Second)
			returnCh <- struct{}{}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	go limiter.StopWait()

	// give a few ms for StopWait to run
	time.Sleep(10 * time.Millisecond)

	// asert drain was set
	assert.Equal(t, true, limiter.drain)

	err := limiter.Dispatch(func() {
		t.Fatal("expected limiter to not accept methods in after StopWait()")
	})
	if err == nil {
		t.Fatal("expected limiter to not accept methods in after StopWait()")
	}

	assert.ErrorIs(t, err, ErrLimiterDrain)

	for i := 0; i < count; i++ {
		<-returnCh
	}
}
