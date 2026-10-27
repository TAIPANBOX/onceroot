package peer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stat builds a /proc/<pid>/stat line: pid (comm) state ppid ... with the
// start time as field 22.
func stat(pid int, comm string, ppid int, start uint64) string {
	f := make([]string, 50)
	for i := range f {
		f[i] = "0"
	}
	f[0] = "S"
	f[1] = fmt.Sprint(ppid)
	f[19] = fmt.Sprint(start)
	return fmt.Sprintf("%d (%s) %s\n", pid, comm, strings.Join(f, " "))
}

type proc struct {
	pid, ppid int
	comm      string
	start     uint64
	cwd       string
}

func fakeProc(t *testing.T, ps ...proc) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range ps {
		d := filepath.Join(root, fmt.Sprint(p.pid))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(d, "stat"), []byte(stat(p.pid, p.comm, p.ppid, p.start)), 0o644)
		os.WriteFile(filepath.Join(d, "comm"), []byte(p.comm+"\n"), 0o644)
		if p.cwd != "" {
			os.Symlink(p.cwd, filepath.Join(d, "cwd"))
		}
	}
	return root
}

func TestParseStatHandlesParensInComm(t *testing.T) {
	ppid, start, err := parseStat([]byte(stat(4242, "a) b (c", 77, 987654)))
	if err != nil || ppid != 77 || start != 987654 {
		t.Fatalf("got %d %d %v", ppid, start, err)
	}
}

func TestParseStatRefusesHostile(t *testing.T) {
	for _, s := range []string{"", "1 (x", "1 (x) S", "1 (x) S notanumber", strings.Repeat("(", 100)} {
		if _, _, err := parseStat([]byte(s)); err == nil {
			t.Errorf("%q parsed", s)
		}
	}
}

func TestDescribeReadsAFakeProc(t *testing.T) {
	root := fakeProc(t,
		proc{pid: 100, ppid: 50, comm: "claude", start: 9000, cwd: "/home/u/Work/app"},
		proc{pid: 50, ppid: 1, comm: "ghostty", start: 8000},
		proc{pid: 1, ppid: 0, comm: "systemd", start: 1},
	)
	r, err := describe(root, 1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if r.UID != 1000 || r.PID != 100 || r.StartTime != 9000 || r.Comm != "claude" || r.CWD != "/home/u/Work/app" {
		t.Fatalf("got %+v", r)
	}
	if strings.Join(r.Chain, ",") != "ghostty,systemd" {
		t.Fatalf("chain %v", r.Chain)
	}
}

func TestChainStopsAtSix(t *testing.T) {
	var ps []proc
	for pid := 20; pid >= 1; pid-- {
		ps = append(ps, proc{pid: pid, ppid: pid - 1, comm: fmt.Sprint("p", pid), start: uint64(pid)})
	}
	ps[0].cwd = "/"
	r, err := describe(fakeProc(t, ps...), 1000, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chain) != 6 || r.Chain[0] != "p19" {
		t.Fatalf("chain %v", r.Chain)
	}
}

func TestStillSameSeesPidReuse(t *testing.T) {
	root := fakeProc(t, proc{pid: 100, ppid: 1, comm: "claude", start: 9000, cwd: "/w"}, proc{pid: 1, comm: "init", start: 1})
	r, err := describe(root, 1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := stillSame(root, r); err != nil {
		t.Fatalf("same process: %v", err)
	}
	os.WriteFile(filepath.Join(root, "100", "stat"), []byte(stat(100, "evil", 1, 9999)), 0o644)
	if err := stillSame(root, r); err == nil {
		t.Fatal("a reused pid passed as the same process")
	}
	os.RemoveAll(filepath.Join(root, "100"))
	if err := stillSame(root, r); err == nil {
		t.Fatal("a vanished process passed as the same process")
	}
}
