package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type sink struct {
	mu       sync.Mutex
	out, err bytes.Buffer
	maxChunk int
}

func (s *sink) write(fd int, p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(p) > s.maxChunk {
		s.maxChunk = len(p)
	}
	if fd == 2 {
		s.err.Write(p)
	} else {
		s.out.Write(p)
	}
}

func run(t *testing.T, limit time.Duration, cwd string, path string, argv ...string) (int, *sink, error) {
	t.Helper()
	s := &sink{}
	code, err := Run(context.Background(), path, argv, cwd, limit, s.write)
	return code, s, err
}

func TestRunnerEnvIsFixed(t *testing.T) {
	t.Setenv("AGENT_SECRET", "leak")
	_, s, err := run(t, 5*time.Second, "/", "/usr/bin/env", "env")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(s.out.String()), "\n")
	if strings.Join(got, "|") != strings.Join(Env(), "|") {
		t.Fatalf("child env %q, want %q", got, Env())
	}
	if Env()[0] != "PATH=/usr/local/sbin:/usr/local/bin:/usr/bin" {
		t.Fatalf("PATH is %q", Env()[0])
	}
}

func TestRunnerPassesArgvVerbatim(t *testing.T) {
	args := []string{"a b", "x\ny", "", `q"uote`, "--", "-n"}
	_, s, err := run(t, 5*time.Second, "/", "/usr/bin/printf", append([]string{"printf", `%s\0`}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(s.out.String(), "\x00"), "\x00")
	if strings.Join(got, "|") != strings.Join(args, "|") {
		t.Fatalf("got %q, want %q", got, args)
	}
}

func TestRunnerStdinIsEmpty(t *testing.T) {
	start := time.Now()
	code, s, err := run(t, 10*time.Second, "/", "/bin/cat", "cat")
	if err != nil || code != 0 || s.out.Len() != 0 {
		t.Fatalf("code %d, out %q, err %v", code, s.out.String(), err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("cat waited on stdin")
	}
}

func TestRunnerExitCodeAndStderr(t *testing.T) {
	code, s, err := run(t, 5*time.Second, "/", "/bin/sh", "sh", "-c", "echo oops >&2; exit 7")
	if err != nil || code != 7 || s.err.String() != "oops\n" {
		t.Fatalf("code %d, stderr %q, err %v", code, s.err.String(), err)
	}
}

func TestRunnerUsesTheCardsCwd(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	_, s, err := run(t, 5*time.Second, dir, "/bin/pwd", "pwd", "-P")
	if err != nil || strings.TrimSpace(s.out.String()) != dir {
		t.Fatalf("pwd %q, want %q, err %v", s.out.String(), dir, err)
	}
}

func TestRunnerStreamsLargeOutput(t *testing.T) {
	_, s, err := run(t, 30*time.Second, "/", "/usr/bin/head", "head", "-c", "10485760", "/dev/zero")
	if err != nil {
		t.Fatal(err)
	}
	if s.out.Len() != 10<<20 {
		t.Fatalf("got %d bytes", s.out.Len())
	}
	if s.maxChunk > MaxChunk {
		t.Fatalf("a chunk of %d bytes, limit %d", s.maxChunk, MaxChunk)
	}
}

func TestRunnerKillsProcessGroupOnTimeout(t *testing.T) {
	start := time.Now()
	code, s, err := run(t, 500*time.Millisecond, "/", "/bin/sh", "sh", "-c", "/bin/sleep 60 & echo $!; wait")
	if err == nil {
		t.Fatalf("a command past its limit ended without an error (code %d)", code)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("the limit did not stop the command")
	}
	pid, perr := strconv.Atoi(strings.TrimSpace(s.out.String()))
	if perr != nil {
		t.Fatalf("no grandchild pid in %q", s.out.String())
	}
	time.Sleep(100 * time.Millisecond)
	if alive(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("grandchild %d outlived the limit", pid)
	}
}

// alive reports whether pid is a running process. A killed process stays a
// zombie until something reaps it, and where PID 1 never does (a container
// whose first process is not an init, as in CI) kill(pid, 0) still finds it.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true // no /proc here: kill(pid, 0) is the answer
	}
	i := bytes.LastIndexByte(b, ')') // the state follows the command name
	return !(i >= 0 && i+2 < len(b) && b[i+2] == 'Z')
}

func TestRunnerRefusesAMissingCwd(t *testing.T) {
	if _, _, err := run(t, time.Second, filepath.Join(os.TempDir(), "no-such-dir-onceroot"), "/bin/pwd", "pwd"); err == nil {
		t.Fatal("ran in a directory that does not exist")
	}
}
