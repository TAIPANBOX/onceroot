package client

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/wire"
)

// serve answers one connection with the given events after reading one request.
func serve(t *testing.T, events ...wire.Event) (string, chan wire.Request) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ar")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	got := make(chan wire.Request, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		req, err := wire.Decode(bufio.NewReader(c))
		if err != nil {
			return
		}
		got <- req
		for _, e := range events {
			wire.WriteEvent(c, e)
		}
	}()
	return sock, got
}

func TestRunStreamsOutputAndReturnsTheExitCode(t *testing.T) {
	sock, got := serve(t,
		wire.Event{Ev: "held", ID: "KQWPT", Card: "ROOT REQUEST KQWPT"},
		wire.Event{Ev: "decided", Verdict: "approve"},
		wire.Event{Ev: "out", FD: 1, Data: []byte("hello\n")},
		wire.Event{Ev: "out", FD: 2, Data: []byte("warn\n")},
		wire.Event{Ev: "exit", Code: 3},
	)
	var out, errb bytes.Buffer
	code, err := Run(sock, []string{"pacman", "-S", "x"}, "install x", &out, &errb)
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 || out.String() != "hello\n" || !strings.Contains(errb.String(), "warn\n") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	if !strings.Contains(errb.String(), "ROOT REQUEST KQWPT") {
		t.Fatalf("the agent was not told it waits for a person: %q", errb.String())
	}
	req := <-got
	if req.Op != "run" || req.Why != "install x" || strings.Join(req.Argv, " ") != "pacman -S x" {
		t.Fatalf("daemon received %+v", req)
	}
}

func TestRunReportsARefusal(t *testing.T) {
	sock, _ := serve(t, wire.Event{Ev: "decided", Verdict: "deny", Reason: "built-in: bash is a shell"})
	var out, errb bytes.Buffer
	code, err := Run(sock, []string{"bash", "-c", "id"}, "", &out, &errb)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "built-in: bash") {
		t.Fatalf("got %d, %v", code, err)
	}
}

func TestRunWithoutAnExitIsAnError(t *testing.T) {
	sock, _ := serve(t, wire.Event{Ev: "out", FD: 1, Data: []byte("x")})
	var out, errb bytes.Buffer
	if _, err := Run(sock, []string{"true"}, "", &out, &errb); err == nil {
		t.Fatal("a stream that ends without exit was taken as success")
	}
}

func TestDoReturnsTheAnswer(t *testing.T) {
	sock, got := serve(t, wire.Event{Ev: "list", Cards: []wire.Listed{{ID: "KQWPT", Status: "held", Card: "c"}}})
	e, err := Do(sock, wire.Request{Op: "list"})
	if err != nil || e.Ev != "list" || len(e.Cards) != 1 {
		t.Fatalf("got %+v, %v", e, err)
	}
	if (<-got).Op != "list" {
		t.Fatal("wrong op sent")
	}
}

func TestRunContextClosesTheConnectionOnCancel(t *testing.T) {
	dir, _ := os.MkdirTemp("", "ar")
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	closed := make(chan struct{})
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		r := bufio.NewReader(c)
		wire.Decode(r)
		wire.WriteEvent(c, wire.Event{Ev: "held", ID: "KQWPT", Card: "c"})
		r.ReadByte() // returns when the client closes
		close(closed)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	var out, errb bytes.Buffer
	if _, err := RunContext(ctx, sock, []string{"pacman"}, "", &out, &errb); err == nil {
		t.Fatal("a cancelled run reported success")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("the daemon never saw the connection close")
	}
}
