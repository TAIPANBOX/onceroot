//go:build linux

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/client"
	"github.com/TAIPANBOX/onceroot/internal/journal"
)

// The end-to-end test re-runs this binary as an unprivileged client.
func TestMain(m *testing.M) {
	if os.Getenv("ONCEROOT_E2E_CLIENT") != "" {
		var argv []string
		if err := json.Unmarshal([]byte(os.Getenv("ONCEROOT_E2E_ARGV")), &argv); err != nil {
			fmt.Fprintln(os.Stderr, "bad ONCEROOT_E2E_ARGV:", err)
			os.Exit(2)
		}
		code, err := client.Run(os.Getenv("ONCEROOT_E2E_SOCK"), argv, "e2e", os.Stdout, os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "REFUSED:", err)
			os.Exit(126)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestDaemonEndToEnd(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root on Linux: run on the test machine")
	}
	name := os.Getenv("ONCEROOT_E2E_USER")
	if name == "" {
		name = "tester"
	}
	u, err := user.Lookup(name)
	if err != nil {
		t.Skipf("no unprivileged user %q to ask as: %v", name, err)
	}
	g, err := user.LookupGroup("onceroot")
	if err != nil {
		t.Skip("group onceroot missing")
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	agid, _ := strconv.Atoi(g.Gid)

	dir, err := os.MkdirTemp("/tmp", "onceroot-e2e")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.Chmod(dir, 0o755)
	write := func(name, body string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		os.Chmod(p, mode)
		return p
	}
	l := layout{
		config:  write("onceroot.conf", fmt.Sprintf("allowed_uids = %d\ndesk_prompt = dialog\n", uid), 0o644),
		rules:   write("rules", "allow /usr/bin/id ...\n", 0o644),
		state:   filepath.Join(dir, "state"),
		sock:    filepath.Join(dir, "onceroot.sock"),
		pkcheck: write("pkcheck", "#!/bin/sh\nexit 0\n", 0o755),
		omarchy: filepath.Join(dir, "no-omarchy"),
		group:   "onceroot",
		owner:   0,
	}
	os.Mkdir(l.state, 0o750)
	done := make(chan struct{})
	served := make(chan error, 1)
	go func() { served <- serve(l, done) }()
	defer func() {
		close(done)
		if err := <-served; err != nil {
			t.Errorf("serve: %v", err)
		}
	}()
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(l.sock); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The test binary sits in a build directory only root can enter, so the
	// unprivileged client runs from a copy.
	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	clientBin := write("client.test", string(self), 0o755)
	ask := func(argv ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(clientBin, "-test.run=^$")
		js, _ := json.Marshal(argv)
		cmd.Env = []string{"ONCEROOT_E2E_CLIENT=1", "ONCEROOT_E2E_SOCK=" + l.sock, "ONCEROOT_E2E_ARGV=" + string(js)}
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{uint32(agid)}}}
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return out.String(), code
	}

	if out, code := ask("id", "-u"); code != 0 || !strings.Contains(out, "\n0\n") && !strings.HasSuffix(strings.TrimSpace(out), "0") {
		t.Fatalf("allowed command: code %d, output %q", code, out)
	}
	marker := filepath.Join(dir, "marker")
	if out, code := ask("touch", marker); code != 0 {
		t.Fatalf("approved command: code %d, output %q", code, out)
	}
	if fi, err := os.Stat(marker); err != nil || fi.Sys().(*syscall.Stat_t).Uid != 0 {
		t.Fatalf("the approved command did not create the file as root: %v", err)
	}
	write("pkcheck", "#!/bin/sh\nexit 1\n", 0o755)
	rejected := filepath.Join(dir, "rejected")
	if out, code := ask("touch", rejected); code != 126 {
		t.Fatalf("rejected command: code %d, output %q", code, out)
	}
	if _, err := os.Stat(rejected); err == nil {
		t.Fatal("a rejected command ran")
	}
	if out, code := ask("bash", "-c", "id"); code != 126 || !strings.Contains(out, "built-in") {
		t.Fatalf("shell: code %d, output %q", code, out)
	}

	pub, err := os.ReadFile(l.state + "/journal.pub")
	if err != nil {
		t.Fatal(err)
	}
	n, err := journal.Verify(l.state+"/journal", ed25519.PublicKey(pub))
	if err != nil || n < 10 {
		t.Fatalf("journal: %d entries, %v", n, err)
	}
	for p, want := range map[string]os.FileMode{l.sock: 0o660, l.state + "/journal": 0o640, l.state + "/journal.pub": 0o644, l.state + "/journal.key": 0o600} {
		fi, err := os.Stat(p)
		if err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v (%v)", filepath.Base(p), fi.Mode().Perm(), want, err)
		}
	}
	if fi, _ := os.Stat(l.sock); int(fi.Sys().(*syscall.Stat_t).Gid) != agid {
		t.Error("the socket does not belong to group onceroot")
	}
}

func TestServeRefusesABadSetup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root on Linux: run on the test machine")
	}
	dir, err := os.MkdirTemp("/tmp", "onceroot-bad")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	write := func(name, body string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), mode)
		os.Chmod(p, mode)
		return p
	}
	good := write("good.conf", "allowed_uids = 1000\n", 0o644)
	os.Mkdir(filepath.Join(dir, "state"), 0o750)
	base := layout{config: good, rules: filepath.Join(dir, "rules"), state: filepath.Join(dir, "state"),
		sock: filepath.Join(dir, "s"), pkcheck: "/bin/false", omarchy: dir, group: "onceroot",
		token: filepath.Join(dir, "absent-token"), factors: filepath.Join(dir, "absent-factors")}
	cases := map[string]func(l *layout){
		"config others can write":    func(l *layout) { l.config = write("loose.conf", "allowed_uids = 1000\n", 0o666) },
		"config that does not parse": func(l *layout) { l.config = write("bad.conf", "colour = red\n", 0o644) },
		"missing config":             func(l *layout) { l.config = filepath.Join(dir, "absent.conf") },
		"missing group":              func(l *layout) { l.group = "onceroot-no-such-group" },
		"phone without its files": func(l *layout) {
			l.config = write("phone.conf", "allowed_uids = 1000\nphone = telegram\ntelegram_chat = 42\n", 0o644)
		},
		"state directory missing": func(l *layout) { l.state = filepath.Join(dir, "no-state") },
	}
	for name, change := range cases {
		l := base
		change(&l)
		done := make(chan struct{})
		close(done)
		if err := serve(l, done); err == nil {
			t.Errorf("%s: serve started", name)
		}
	}
}
