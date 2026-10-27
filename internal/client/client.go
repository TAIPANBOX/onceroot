// Package client talks to oncerootd over its socket for the onceroot CLI and
// the MCP tool.
package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/TAIPANBOX/onceroot/internal/wire"
)

// DefaultSocket is where oncerootd listens.
const DefaultSocket = "/run/onceroot/onceroot.sock"

// ErrRefused means the request did not run: denied, rejected, expired or
// refused by a limit. Its message carries onceroot's reason.
var ErrRefused = errors.New("onceroot: not run")

// Run asks for argv to run as root and streams its output. While a person
// decides, the card goes to stderr so the agent knows what it waits for.
// It returns the command's exit code.
func Run(sock string, argv []string, why string, stdout, stderr io.Writer) (int, error) {
	return RunContext(context.Background(), sock, argv, why, stdout, stderr)
}

// RunContext is Run that gives up when ctx ends: it closes the connection,
// and oncerootd withdraws the card, so an abandoned request never runs later.
func RunContext(ctx context.Context, sock string, argv []string, why string, stdout, stderr io.Writer) (int, error) {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return 0, fmt.Errorf("onceroot: daemon not reachable at %s: %w", sock, err)
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	if err := wire.WriteRequest(c, wire.Request{Op: "run", Argv: argv, Why: why}); err != nil {
		return 0, err
	}
	r := bufio.NewReader(c)
	for {
		e, err := wire.ReadEvent(r)
		if err != nil {
			if ctx.Err() != nil {
				return 0, fmt.Errorf("%w: cancelled, the request was withdrawn", ErrRefused)
			}
			return 0, fmt.Errorf("onceroot: connection ended before the command did: %w", err)
		}
		switch e.Ev {
		case "held":
			fmt.Fprintf(stderr, "onceroot: waiting for a person to approve %s:\n%s\n", e.ID, e.Card)
		case "note":
			fmt.Fprintf(stderr, "onceroot: %s\n", e.Reason)
		case "decided":
			switch e.Verdict {
			case "allow", "approve":
				fmt.Fprintf(stderr, "onceroot: %s (%s)\n", e.Verdict, e.Reason)
			default:
				return 0, fmt.Errorf("%w: %s: %s", ErrRefused, e.Verdict, e.Reason)
			}
		case "out":
			w := stdout
			if e.FD == 2 {
				w = stderr
			}
			w.Write(e.Data)
		case "exit":
			return e.Code, nil
		case "error":
			return 0, fmt.Errorf("%w: %s", ErrRefused, e.Reason)
		}
	}
}

// Do sends one non-run request and returns the daemon's one answer.
func Do(sock string, req wire.Request) (wire.Event, error) {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return wire.Event{}, fmt.Errorf("onceroot: daemon not reachable at %s: %w", sock, err)
	}
	defer c.Close()
	if err := wire.WriteRequest(c, req); err != nil {
		return wire.Event{}, err
	}
	e, err := wire.ReadEvent(bufio.NewReader(c))
	if err != nil {
		return wire.Event{}, err
	}
	if e.Ev == "error" {
		return e, fmt.Errorf("%w: %s", ErrRefused, e.Reason)
	}
	return e, nil
}
