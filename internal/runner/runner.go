//go:build unix

// Package runner runs one approved command as root: the argv exactly as
// approved, a fixed environment, the card's folder, no stdin, its own process
// group, and a time limit that kills the whole group.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// MaxChunk bounds one piece of output handed to out.
const MaxChunk = 32 * 1024

// Env is the whole environment of a command (I8). Nothing of the
// requester's environment reaches it.
func Env() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/bin",
		"LANG=C.UTF-8",
		"HOME=/root",
	}
}

// Run starts path with argv in cwd and streams its stdout (fd 1) and stderr
// (fd 2) to out, one call at a time. It returns the exit code. When limit
// passes or ctx ends, it kills the process group and returns an error.
func Run(ctx context.Context, path string, argv []string, cwd string, limit time.Duration, out func(fd int, p []byte)) (int, error) {
	if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return -1, fmt.Errorf("runner: working directory %q is not available", cwd)
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	outR, outW, err := os.Pipe()
	if err != nil {
		return -1, err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return -1, err
	}
	cmd := &exec.Cmd{
		Path:        path,
		Args:        argv,
		Env:         Env(),
		Dir:         cwd,
		Stdout:      outW,
		Stderr:      errW,
		SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
	}
	if err := cmd.Start(); err != nil {
		outR.Close()
		outW.Close()
		errR.Close()
		errW.Close()
		return -1, fmt.Errorf("runner: %w", err)
	}
	outW.Close()
	errW.Close()
	pgid := cmd.Process.Pid

	var mu sync.Mutex
	emit := func(fd int, p []byte) {
		mu.Lock()
		defer mu.Unlock()
		out(fd, p)
	}
	var readers sync.WaitGroup
	for fd, r := range map[int]*os.File{1: outR, 2: errR} {
		readers.Add(1)
		go func() {
			defer readers.Done()
			defer r.Close()
			buf := make([]byte, MaxChunk)
			for {
				n, err := r.Read(buf)
				if n > 0 {
					emit(fd, append([]byte(nil), buf[:n]...))
				}
				if err != nil {
					return
				}
			}
		}()
	}

	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	var werr error
	select {
	case werr = <-waited:
	case <-ctx.Done():
		syscall.Kill(-pgid, syscall.SIGKILL)
		<-waited
		werr = ctx.Err()
	}
	// Whatever the command left behind in its group goes too, so no
	// background child keeps root or the output pipes.
	syscall.Kill(-pgid, syscall.SIGKILL)
	readers.Wait()

	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return -1, fmt.Errorf("runner: killed after the %s limit", limit)
		}
		return -1, fmt.Errorf("runner: stopped: %w", ctx.Err())
	}
	var exit *exec.ExitError
	if werr != nil && !errors.As(werr, &exit) {
		return -1, fmt.Errorf("runner: %w", werr)
	}
	return cmd.ProcessState.ExitCode(), nil
}
