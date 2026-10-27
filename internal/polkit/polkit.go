// Package polkit asks the desktop's polkit agent for a person's approval of
// one card, through pkcheck. Only exit status 0 is an approval; every other
// status, a missing pkcheck and a cancelled check are not. Exit status 2,
// no agent to ask, is told apart: nobody said no.
package polkit

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/TAIPANBOX/onceroot/internal/card"
)

// ActionID is onceroot's one polkit action.
const ActionID = "org.taipanbox.onceroot.run"

// DefaultBin is where Arch and Debian install pkcheck.
const DefaultBin = "/usr/bin/pkcheck"

// Args are pkcheck's arguments for one card. The subject carries pid,
// start time and uid so a reused pid is not asked about. polkit.message is
// honoured only for a caller running as root, which oncerootd is.
func Args(r card.Requester, message string) []string {
	return []string{
		"--action-id", ActionID,
		"--process", fmt.Sprintf("%d,%d,%d", r.PID, r.StartTime, r.UID),
		"--allow-user-interaction",
		"--detail", "polkit.message", message,
	}
}

// ErrNoAgent is pkcheck's exit status 2: no authentication agent can ask
// anyone, as for a process outside a desktop session.
var ErrNoAgent = errors.New("polkit: no authentication agent can ask for this process")

// Checker asks a person.
type Checker interface {
	Check(ctx context.Context, r card.Requester, message string) (bool, error)
}

// PK runs pkcheck.
type PK struct{ Bin string }

// Check runs pkcheck and approves only on exit status 0.
func (p PK) Check(ctx context.Context, r card.Requester, message string) (bool, error) {
	bin := p.Bin
	if bin == "" {
		bin = DefaultBin
	}
	cmd := exec.CommandContext(ctx, bin, Args(r, message)...)
	cmd.Env = []string{"PATH=/usr/bin", "LANG=C.UTF-8"}
	err := cmd.Run()
	if ctx.Err() != nil {
		return false, fmt.Errorf("polkit: check ended: %w", ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 2 {
		return false, ErrNoAgent
	}
	if err != nil {
		return false, fmt.Errorf("polkit: not approved: %w", err)
	}
	return true, nil
}
