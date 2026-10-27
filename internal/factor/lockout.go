package factor

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrLockedOut = errors.New("factor: locked after too many wrong tries")
	ErrWrong     = errors.New("factor: wrong PIN or code")
)

// Lockout counts failed tries and locks after max of them for d.
type Lockout struct {
	mu    sync.Mutex
	max   int
	d     time.Duration
	fails int
	until time.Time
}

// NewLockout locks after max failures for d.
func NewLockout(max int, d time.Duration) *Lockout { return &Lockout{max: max, d: d} }

// Allowed returns ErrLockedOut, naming its end, while locked.
func (l *Lockout) Allowed(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.allowedLocked(now)
}

func (l *Lockout) allowedLocked(now time.Time) error {
	if now.Before(l.until) {
		return fmt.Errorf("%w until %s", ErrLockedOut, l.until.Format("15:04"))
	}
	return nil
}

// Try records one check. A success resets the count; the failure that
// reaches max starts the lockout and returns ErrLockedOut.
func (l *Lockout) Try(now time.Time, ok bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.allowedLocked(now); err != nil {
		return err
	}
	if ok {
		l.fails = 0
		return nil
	}
	l.fails++
	if l.fails >= l.max {
		l.fails = 0
		l.until = now.Add(l.d)
		return fmt.Errorf("%w until %s", ErrLockedOut, l.until.Format("15:04"))
	}
	return ErrWrong
}

// Factors are what an approval from the phone needs.
type Factors struct {
	PINSalt []byte
	PINHash []byte
	TOTP    *TOTP
	Lock    *Lockout
}

// Check verifies PIN and code. The PIN is checked first, so a wrong PIN
// never spends the code; a lockout refuses even the right factors.
func (f *Factors) Check(pin, code string, now time.Time) error {
	if err := f.Lock.Allowed(now); err != nil {
		return err
	}
	if !CheckPIN(pin, f.PINHash, f.PINSalt) {
		if err := f.Lock.Try(now, false); err != nil {
			return err
		}
		return ErrWrong
	}
	if !f.TOTP.Check(code, now) {
		if err := f.Lock.Try(now, false); err != nil {
			return err
		}
		return ErrWrong
	}
	return f.Lock.Try(now, true)
}
