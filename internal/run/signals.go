package run

import "os"

// watchSignals drives the interrupt policy. The first signal invokes
// onFirst (graceful: cancel in-flight work, `docker stop` so the
// entrypoint's trap can salvage a bundle); the second invokes onSecond
// (abort: the operator wants out now, salvage is abandoned). Closing done
// ends the watch on normal completion — signal.Stop only stops future
// relaying, it does not close the channel.
//
// The channel keeps being drained after both callbacks so a repeated
// signal is never silently dropped into the buffered channel while nobody
// is listening (which would otherwise make the process unkillable short of
// SIGKILL). Production's onSecond never returns (os.Exit), but the drain
// must not rely on that.
func watchSignals(sigCh <-chan os.Signal, done <-chan struct{}, onFirst, onSecond func()) {
	select {
	case <-sigCh:
	case <-done:
		return
	}
	onFirst()
	select {
	case <-sigCh:
		onSecond()
	case <-done:
		return
	}
	for {
		select {
		case <-sigCh:
		case <-done:
			return
		}
	}
}
