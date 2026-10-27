//go:build linux

package peer

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestFromConnReadsPeerCreds(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := net.Dial("unix", sock)
		if err == nil {
			defer c.Close()
			c.Read(make([]byte, 1))
		}
	}()
	c, err := l.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r, err := FromConn(c)
	if err != nil {
		t.Fatal(err)
	}
	if r.UID != uint32(os.Getuid()) || r.PID != os.Getpid() || r.StartTime == 0 {
		t.Fatalf("got %+v", r)
	}
	if err := StillSame(r); err != nil {
		t.Fatal(err)
	}
}
