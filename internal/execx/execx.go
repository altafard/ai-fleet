// Package execx is the single doorway to subprocesses: capture or stream,
// never panic on non-zero exits.
package execx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// waitDelay bounds how long Wait keeps waiting for I/O after the context is
// done or the process exits — without it, a lingering descendant (ssh, a
// credential helper) that inherited the pipe keeps Wait blocked forever.
const waitDelay = 3 * time.Second

// maxStreamLine caps a single output line in Stream. Claude stream-json
// events embed whole tool results, so the cap is generous. Overridable in
// tests.
var maxStreamLine = 64 * 1024 * 1024

// Result captures a finished subprocess: whitespace-trimmed stdout and
// stderr, and the exit code (zero unless the process ran and failed).
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Cause selects the failure description for a call that did not succeed:
// when the process could not run at all, Stderr is empty (there was no
// process to write it), so only err carries the cause — formatting Stderr
// unconditionally would report "…failed: " with nothing after the colon.
func Cause(r Result, err error) string {
	if err != nil {
		return err.Error()
	}
	return r.Stderr
}

// Run executes and captures. A non-zero exit code is reported in Result,
// not as an error; error means the process could not run at all.
func Run(dir, name string, args ...string) (Result, error) {
	return RunCtx(context.Background(), dir, nil, name, args...)
}

// RunCtx is Run with a context (timeout/cancel) and extra env entries
// appended to os.Environ(). A context-killed process surfaces as a
// non-zero ExitCode, not an error.
func RunCtx(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = waitDelay
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := Result{Stdout: strings.TrimSpace(out.String()), Stderr: strings.TrimSpace(errb.String())}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// Process exited but a descendant held the pipe open past the grace
		// window; the captured output is still valid.
		return res, nil
	}
	return res, err
}

// Stream executes with stdout and stderr merged into one ordered pipe,
// invoking onLine for every line. env entries extend os.Environ().
func Stream(ctx context.Context, dir string, env []string, onLine func(string), name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.WaitDelay = waitDelay
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return -1, err
	}
	done := make(chan struct{})
	var scanErr error
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), maxStreamLine)
		for sc.Scan() {
			onLine(sc.Text())
		}
		scanErr = sc.Err()
		// Keep draining even after a scan error (e.g. a line beyond the cap):
		// the process's writer must never block on a dead reader, or
		// cmd.Wait below would deadlock forever.
		io.Copy(io.Discard, pr)
		close(done)
	}()
	err := cmd.Wait()
	pw.Close()
	<-done
	if scanErr != nil {
		return -1, fmt.Errorf("output stream: %w", scanErr)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}
