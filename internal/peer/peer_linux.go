//go:build linux

package peer

import (
	"fmt"
	"net"
	"syscall"

	"github.com/TAIPANBOX/onceroot/internal/card"
)

// FromConn names the process on the other end of c: uid and pid from
// SO_PEERCRED, the rest from /proc.
func FromConn(c *net.UnixConn) (card.Requester, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return card.Requester{}, fmt.Errorf("peer: %w", err)
	}
	var cred *syscall.Ucred
	var cerr error
	if err := raw.Control(func(fd uintptr) {
		cred, cerr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return card.Requester{}, fmt.Errorf("peer: %w", err)
	}
	if cerr != nil {
		return card.Requester{}, fmt.Errorf("peer: SO_PEERCRED: %w", cerr)
	}
	return describe("/proc", cred.Uid, int(cred.Pid))
}

// StillSame checks that the requester's pid still names the same process.
func StillSame(r card.Requester) error { return stillSame("/proc", r) }
