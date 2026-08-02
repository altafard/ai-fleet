// Package execx is the single doorway to subprocesses: capture or stream,
// never panic on non-zero exits.
package execx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Result captures a finished subprocess: whitespace-trimmed stdout and
// stderr, and the exit code (zero unless the process ran and failed).
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Run executes and captures. A non-zero exit code is reported in Result,
// not as an error; error means the process could not run at all.
func Run(dir, name string, args ...string) (Result, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := Result{Stdout: strings.TrimSpace(out.String()), Stderr: strings.TrimSpace(errb.String())}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
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
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return -1, err
	}
	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			onLine(sc.Text())
		}
		close(done)
	}()
	err := cmd.Wait()
	pw.Close()
	<-done
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}
