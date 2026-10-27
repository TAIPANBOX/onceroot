package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TAIPANBOX/onceroot/internal/journal"
	"github.com/TAIPANBOX/onceroot/internal/wire"
)

// fakeDaemon answers every connection with the events reply gives for the request.
func fakeDaemon(t *testing.T, reply func(wire.Request) []wire.Event) (string, *[]wire.Request) {
	t.Helper()
	dir, _ := os.MkdirTemp("", "ark")
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	var seen []wire.Request
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			req, err := wire.Decode(bufio.NewReader(c))
			if err == nil {
				seen = append(seen, req)
				for _, e := range reply(req) {
					wire.WriteEvent(c, e)
				}
			}
			c.Close()
		}
	}()
	return sock, &seen
}

func run(t *testing.T, sock string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	inv, err := parseArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := do(inv, env{sock: sock, stdin: strings.NewReader(stdin), stdout: &out, stderr: &errb,
		journal: filepath.Join(t.TempDir(), "none"), pub: filepath.Join(t.TempDir(), "none.pub")})
	return code, out.String(), errb.String()
}

func TestDoRunReturnsTheCommandsCode(t *testing.T) {
	sock, _ := fakeDaemon(t, func(wire.Request) []wire.Event {
		return []wire.Event{{Ev: "decided", Verdict: "allow"}, {Ev: "out", FD: 1, Data: []byte("x\n")}, {Ev: "exit", Code: 4}}
	})
	if code, out, _ := run(t, sock, "", "run", "--", "id"); code != 4 || out != "x\n" {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestDoRunRefusedExits126(t *testing.T) {
	sock, _ := fakeDaemon(t, func(wire.Request) []wire.Event {
		return []wire.Event{{Ev: "decided", Verdict: "deny", Reason: "built-in"}}
	})
	if code, _, errs := run(t, sock, "", "run", "--", "bash"); code != exitRefused || !strings.Contains(errs, "built-in") {
		t.Fatalf("code %d stderr %q", code, errs)
	}
}

func TestDoWithoutDaemonFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.sock")
	for _, args := range [][]string{{"run", "--", "id"}, {"list"}, {"status"}, {"approve", "KQWPT"}, {"review"}} {
		if code, _, _ := run(t, missing, "", args...); code != 1 {
			t.Errorf("%v without a daemon: code %d, want 1", args, code)
		}
	}
	if code, out, _ := run(t, missing, "", "bar"); code != 0 || !strings.Contains(out, "not reachable") {
		t.Errorf("bar without a daemon: code %d out %q", code, out)
	}
}

func TestDoListStatusApproveReject(t *testing.T) {
	sock, seen := fakeDaemon(t, func(r wire.Request) []wire.Event {
		switch r.Op {
		case "list":
			return []wire.Event{{Ev: "list", Cards: []wire.Listed{{ID: "KQWPT", Status: "held", Card: "CARD"}}}}
		case "status":
			return []wire.Event{{Ev: "status", Status: []string{"passwordless sudo: off"}}}
		}
		return []wire.Event{{Ev: "decided", ID: r.ID, Verdict: r.Op}}
	})
	if _, out, _ := run(t, sock, "", "list"); !strings.Contains(out, "KQWPT held\nCARD") {
		t.Fatalf("list %q", out)
	}
	_, out, _ := run(t, sock, "", "list", "--json")
	var cards []wire.Listed
	if json.Unmarshal([]byte(out), &cards) != nil || len(cards) != 1 {
		t.Fatalf("list --json %q", out)
	}
	if _, out, _ := run(t, sock, "", "status"); !strings.Contains(out, "passwordless sudo: off") {
		t.Fatalf("status %q", out)
	}
	if code, out, _ := run(t, sock, "", "approve", "KQWPT"); code != 0 || !strings.Contains(out, "KQWPT approved") {
		t.Fatalf("approve %d %q", code, out)
	}
	if code, out, _ := run(t, sock, "", "reject", "KQWPT"); code != 0 || !strings.Contains(out, "KQWPT rejected") {
		t.Fatalf("reject %d %q", code, out)
	}
	ops := []string{}
	for _, r := range *seen {
		ops = append(ops, r.Op)
	}
	if strings.Join(ops, ",") != "list,list,status,approve,reject" {
		t.Fatalf("ops %v", ops)
	}
}

func TestDoBarIdleAndUnreachable(t *testing.T) {
	sock, _ := fakeDaemon(t, func(r wire.Request) []wire.Event {
		return []wire.Event{{Ev: "list", Cards: []wire.Listed{{ID: "KQWPT", Status: "ran", Card: "CARD"}}}}
	})
	if code, out, _ := run(t, sock, "", "bar"); code != 0 || out != "" {
		t.Fatalf("idle bar %d %q, want nothing", code, out)
	}
	if code, out, _ := run(t, filepath.Join(t.TempDir(), "absent.sock"), "", "bar"); code != 0 || !strings.Contains(out, `"text":"root ?"`) || !strings.Contains(out, `"class":"active"`) {
		t.Fatalf("unreachable bar %d %q", code, out)
	}
}

func TestDoBarAndReview(t *testing.T) {
	sock, seen := fakeDaemon(t, func(r wire.Request) []wire.Event {
		if r.Op == "list" {
			return []wire.Event{{Ev: "list", Cards: []wire.Listed{{ID: "KQWPT", Status: "held", Card: "CARD"}}}}
		}
		return []wire.Event{{Ev: "decided", ID: r.ID, Verdict: r.Op}}
	})
	if _, out, _ := run(t, sock, "", "bar"); !strings.Contains(out, "root? 1") {
		t.Fatalf("bar %q", out)
	}
	if code, out, _ := run(t, sock, "r\n", "review"); code != 0 || !strings.Contains(out, "KQWPT: rejected") {
		t.Fatalf("review %d %q", code, out)
	}
	if last := (*seen)[len(*seen)-1]; last.Op != "reject" || last.ID != "KQWPT" {
		t.Fatalf("review sent %+v", last)
	}
}

func TestDoJournalVerify(t *testing.T) {
	dir := t.TempDir()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	j, _ := journal.Open(filepath.Join(dir, "journal"), priv)
	j.Append("request", "", [32]byte{}, nil)
	j.Close()
	journal.WritePublicKey(filepath.Join(dir, "journal.pub"), pub)
	inv, _ := parseArgs([]string{"journal", "verify"})
	var out, errb bytes.Buffer
	e := env{stdin: strings.NewReader(""), stdout: &out, stderr: &errb, journal: filepath.Join(dir, "journal"), pub: filepath.Join(dir, "journal.pub")}
	if code := do(inv, e); code != 0 || !strings.Contains(out.String(), "1 entries") {
		t.Fatalf("verify %d %q %q", code, out.String(), errb.String())
	}
	os.WriteFile(filepath.Join(dir, "journal"), []byte("{\"seq\":9}\n"), 0o640)
	if code := do(inv, e); code != 1 {
		t.Fatal("a broken journal verified")
	}
	e.pub = filepath.Join(dir, "absent")
	if code := do(inv, e); code != 1 {
		t.Fatal("verify without a public key succeeded")
	}
}

func TestDoMCPAnswersOverStdio(t *testing.T) {
	sock, _ := fakeDaemon(t, func(wire.Request) []wire.Event { return nil })
	code, out, _ := run(t, sock, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n", "mcp")
	if code != 0 || !strings.Contains(out, "run_as_root") {
		t.Fatalf("mcp %d %q", code, out)
	}
}
