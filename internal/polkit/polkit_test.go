package polkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/card"
)

var req = card.Requester{UID: 1000, PID: 4242, StartTime: 987654, Comm: "claude", CWD: "/w"}

func fakePkcheck(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pkcheck")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestArgsCarryStartTimeAndMessage(t *testing.T) {
	got := strings.Join(Args(req, "MSG"), " ")
	want := "--action-id org.taipanbox.onceroot.run --process 4242,987654,1000 --allow-user-interaction --detail polkit.message MSG"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestOnlyExitZeroApproves(t *testing.T) {
	for _, code := range []string{"0", "1", "2", "3", "4", "127"} {
		ok, _ := PK{Bin: fakePkcheck(t, "exit "+code)}.Check(context.Background(), req, "m")
		if ok != (code == "0") {
			t.Errorf("exit %s: approved=%v", code, ok)
		}
	}
	if ok, err := (PK{Bin: "/nonexistent/pkcheck"}).Check(context.Background(), req, "m"); ok || err == nil {
		t.Errorf("missing pkcheck: approved=%v err=%v", ok, err)
	}
}

func TestCancelledCheckIsNotApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	ok, err := PK{Bin: fakePkcheck(t, "/bin/sleep 5; exit 0")}.Check(ctx, req, "m")
	if ok || err == nil {
		t.Fatalf("a cancelled check: approved=%v err=%v", ok, err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the check outlived its context")
	}
}

func TestCheckPassesTheMessageVerbatim(t *testing.T) {
	out := filepath.Join(t.TempDir(), "args")
	bin := fakePkcheck(t, `for a in "$@"; do printf '%s\0' "$a"; done > `+out+`; exit 0`)
	msg := "ROOT REQUEST KQWPT\nRun: \"/usr/bin/pacman\"\n  \"-S\""
	if ok, err := (PK{Bin: bin}).Check(context.Background(), req, msg); !ok || err != nil {
		t.Fatalf("approved=%v err=%v", ok, err)
	}
	b, _ := os.ReadFile(out)
	args := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
	if len(args) != 8 || args[6] != "polkit.message" || args[7] != msg {
		t.Fatalf("pkcheck received %q", args)
	}
}

// pkcheck exits 2 when no authentication agent can ask anyone, for instance
// for a process outside a desktop session; nobody said no.
func TestNoAgentIsNotARefusal(t *testing.T) {
	ok, err := PK{Bin: fakePkcheck(t, "exit 2")}.Check(context.Background(), req, "m")
	if ok || !errors.Is(err, ErrNoAgent) {
		t.Fatalf("exit 2: approved=%v err=%v, want ErrNoAgent", ok, err)
	}
	for _, code := range []string{"1", "3", "127"} {
		if _, err := (PK{Bin: fakePkcheck(t, "exit "+code)}).Check(context.Background(), req, "m"); errors.Is(err, ErrNoAgent) {
			t.Errorf("exit %s reads as no agent", code)
		}
	}
}
