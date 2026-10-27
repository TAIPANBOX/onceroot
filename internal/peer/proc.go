// Package peer says who is on the other end of onceroot's socket, from the
// kernel's peer credentials and /proc, never from what the process claims.
package peer

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TAIPANBOX/onceroot/internal/card"
)

// maxChain is how many parent names a card shows.
const maxChain = 6

// ErrUnsupported is returned where the kernel cannot name a socket peer.
var ErrUnsupported = errors.New("peer: socket peer credentials are not supported on this system")

// parseStat reads ppid (field 4) and start time (field 22) from a
// /proc/<pid>/stat line. comm (field 2) may hold spaces and parentheses, so
// the fields are counted from the last ')'.
func parseStat(b []byte) (ppid int, start uint64, err error) {
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, 0, errors.New("peer: stat without a closing parenthesis")
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 {
		return 0, 0, errors.New("peer: stat has too few fields")
	}
	ppid, err = strconv.Atoi(f[1])
	if err != nil {
		return 0, 0, fmt.Errorf("peer: ppid: %w", err)
	}
	start, err = strconv.ParseUint(f[19], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("peer: start time: %w", err)
	}
	return ppid, start, nil
}

func readStat(procRoot string, pid int) (int, uint64, error) {
	b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, 0, fmt.Errorf("peer: %w", err)
	}
	return parseStat(b)
}

func readComm(procRoot string, pid int) string {
	b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "comm"))
	if err != nil {
		return "?"
	}
	return strings.TrimSuffix(string(b), "\n")
}

// describe builds the requester from /proc under procRoot.
func describe(procRoot string, uid uint32, pid int) (card.Requester, error) {
	ppid, start, err := readStat(procRoot, pid)
	if err != nil {
		return card.Requester{}, err
	}
	cwd, err := os.Readlink(filepath.Join(procRoot, strconv.Itoa(pid), "cwd"))
	if err != nil {
		return card.Requester{}, fmt.Errorf("peer: working directory of pid %d: %w", pid, err)
	}
	r := card.Requester{UID: uid, PID: pid, StartTime: start, Comm: readComm(procRoot, pid), CWD: cwd}
	for p := ppid; p > 0 && len(r.Chain) < maxChain; {
		r.Chain = append(r.Chain, readComm(procRoot, p))
		next, _, err := readStat(procRoot, p)
		if err != nil {
			break
		}
		p = next
	}
	return r, nil
}

// stillSame checks that the pid still names the process that asked.
func stillSame(procRoot string, r card.Requester) error {
	_, start, err := readStat(procRoot, r.PID)
	if err != nil {
		return fmt.Errorf("peer: the requesting process is gone: %w", err)
	}
	if start != r.StartTime {
		return fmt.Errorf("peer: pid %d now names another process", r.PID)
	}
	return nil
}
