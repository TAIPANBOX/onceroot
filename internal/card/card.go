// Package card holds a root request as the person approving it sees it: an
// immutable card with a hash over every field, and a record whose status
// moves one way only (held, then approved, rejected, expired or withdrawn;
// an approved card then runs once).
package card

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"time"
)

// Requester is the process that asked, as the daemon observed it, never as
// the process described itself.
type Requester struct {
	UID       uint32
	PID       int
	StartTime uint64 // field 22 of /proc/<pid>/stat, guards against pid reuse
	Comm      string
	Chain     []string // parent process names, nearest first
	CWD       string
}

// Card is one root request. It is never changed after New: Hash covers
// every other field, and an approval names that hash.
type Card struct {
	ID           string
	Req          Requester
	Path         string // absolute, resolved by root
	BinarySHA256 [32]byte
	Argv         []string
	Why          string // the agent's own words, shown only as quoted text
	Risks        []string
	Created      time.Time
	Expires      time.Time
	Hash         [32]byte
}

// New builds a card that expires ttl after now and stores its hash.
func New(id string, req Requester, path string, bin [32]byte, argv []string, why string, risks []string, now time.Time, ttl time.Duration) Card {
	c := Card{
		ID:           id,
		Req:          req,
		Path:         path,
		BinarySHA256: bin,
		Argv:         append([]string(nil), argv...),
		Why:          why,
		Risks:        append([]string(nil), risks...),
		Created:      now,
		Expires:      now.Add(ttl),
	}
	c.Req.Chain = append([]string(nil), req.Chain...)
	c.Hash = computeHash(c)
	return c
}

// computeHash covers every field but Hash. Strings and lists are length
// prefixed, so ["a b"], ["a", "b"] and ["ab", ""] never collide.
func computeHash(c Card) [32]byte {
	h := sha256.New()
	writeString(h, "onceroot card v1")
	writeString(h, c.ID)
	writeUint(h, uint64(c.Req.UID))
	writeUint(h, uint64(c.Req.PID))
	writeUint(h, c.Req.StartTime)
	writeString(h, c.Req.Comm)
	writeStrings(h, c.Req.Chain)
	writeString(h, c.Req.CWD)
	writeString(h, c.Path)
	h.Write(c.BinarySHA256[:])
	writeStrings(h, c.Argv)
	writeString(h, c.Why)
	writeStrings(h, c.Risks)
	writeUint(h, uint64(c.Created.UnixNano()))
	writeUint(h, uint64(c.Expires.UnixNano()))
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func writeUint(h hash.Hash, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	h.Write(b[:])
}

func writeString(h hash.Hash, s string) {
	writeUint(h, uint64(len(s)))
	h.Write([]byte(s))
}

func writeStrings(h hash.Hash, ss []string) {
	writeUint(h, uint64(len(ss)))
	for _, s := range ss {
		writeString(h, s)
	}
}

// Status is where a record stands. It moves one way only.
type Status int

const (
	Held Status = iota
	Approved
	Ran
	Rejected
	Expired
	Withdrawn
)

func (s Status) String() string {
	switch s {
	case Held:
		return "held"
	case Approved:
		return "approved"
	case Ran:
		return "ran"
	case Rejected:
		return "rejected"
	case Expired:
		return "expired"
	case Withdrawn:
		return "withdrawn"
	}
	return "unknown"
}

var (
	ErrNotHeld      = errors.New("card is not held")
	ErrNotApproved  = errors.New("card is not approved")
	ErrExpired      = errors.New("card has expired")
	ErrHashMismatch = errors.New("approval names a different card hash")
	ErrAlreadyRan   = errors.New("card has already run")
)

// Record is a card and where it stands.
type Record struct {
	Card   Card
	Status Status
	By     string // who moved it last: "polkit", "phone", "rule:<line>", ...
	At     time.Time
}

// Approve moves a held, unexpired card to approved, only for its own hash.
func (r *Record) Approve(h [32]byte, by string, now time.Time) error {
	if r.Status != Held {
		return ErrNotHeld
	}
	if !now.Before(r.Card.Expires) {
		return ErrExpired
	}
	if h != r.Card.Hash {
		return ErrHashMismatch
	}
	r.move(Approved, by, now)
	return nil
}

// Reject moves a held card to rejected.
func (r *Record) Reject(by string, now time.Time) error {
	if r.Status != Held {
		return ErrNotHeld
	}
	r.move(Rejected, by, now)
	return nil
}

// Expire moves a held card to expired.
func (r *Record) Expire(now time.Time) error {
	if r.Status != Held {
		return ErrNotHeld
	}
	r.move(Expired, "ttl", now)
	return nil
}

// Withdraw ends a card whose requester left, held or approved but not yet run.
func (r *Record) Withdraw(now time.Time) error {
	if r.Status != Held && r.Status != Approved {
		return ErrNotHeld
	}
	r.move(Withdrawn, "requester", now)
	return nil
}

// MarkRan records the one execution of an approved, unexpired card.
func (r *Record) MarkRan(now time.Time) error {
	switch {
	case r.Status == Ran:
		return ErrAlreadyRan
	case r.Status != Approved:
		return ErrNotApproved
	case !now.Before(r.Card.Expires):
		return ErrExpired
	}
	r.move(Ran, r.By, now)
	return nil
}

func (r *Record) move(s Status, by string, now time.Time) {
	r.Status, r.By, r.At = s, by, now
}
