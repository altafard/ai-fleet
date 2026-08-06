package run

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// step reports watcher callbacks in order; the watcher goroutine writes,
// the test reads with a timeout so a broken watcher fails instead of
// hanging the suite.
func awaitStep(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("callback order: got %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %q", want)
	}
}

func TestWatchSignalsFirstGracefulSecondAbort(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	defer close(done)
	steps := make(chan string, 2)

	go watchSignals(sigCh, done,
		func() { steps <- "first" },
		func() { steps <- "second" })

	sigCh <- syscall.SIGINT
	awaitStep(t, steps, "first")
	sigCh <- syscall.SIGINT
	awaitStep(t, steps, "second")
}

func TestWatchSignalsOneSignalDoesNotAbort(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	defer close(done)
	steps := make(chan string, 2)

	go watchSignals(sigCh, done,
		func() { steps <- "first" },
		func() { steps <- "second" })

	sigCh <- syscall.SIGTERM
	awaitStep(t, steps, "first")
	select {
	case got := <-steps:
		t.Fatalf("unexpected callback %q after a single signal", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWatchSignalsDoneEndsWatchWithoutCallbacks(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	steps := make(chan string, 2)
	returned := make(chan struct{})

	go func() {
		watchSignals(sigCh, done,
			func() { steps <- "first" },
			func() { steps <- "second" })
		close(returned)
	}()

	close(done)
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not return after done was closed")
	}
	select {
	case got := <-steps:
		t.Fatalf("unexpected callback %q", got)
	default:
	}
}

// After the second callback returns (it os.Exits in production, but must
// not be relied on to), late signals keep being drained so the buffered
// channel can never fill up and leave the process unkillable.
func TestWatchSignalsKeepsDrainingAfterSecond(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	steps := make(chan string, 2)

	go watchSignals(sigCh, done,
		func() { steps <- "first" },
		func() { steps <- "second" })

	sigCh <- syscall.SIGINT
	awaitStep(t, steps, "first")
	sigCh <- syscall.SIGINT
	awaitStep(t, steps, "second")
	for i := 0; i < 3; i++ {
		select {
		case sigCh <- syscall.SIGINT:
		case <-time.After(2 * time.Second):
			t.Fatal("signal channel not drained after second callback")
		}
	}
	close(done)
}
