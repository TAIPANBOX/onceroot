//go:build !linux

package peer

import (
	"net"

	"github.com/TAIPANBOX/onceroot/internal/card"
)

// FromConn is Linux only.
func FromConn(*net.UnixConn) (card.Requester, error) { return card.Requester{}, ErrUnsupported }

// StillSame is Linux only.
func StillSame(card.Requester) error { return ErrUnsupported }
