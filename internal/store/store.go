// Package store keeps the cards that wait for a person, in memory only: a
// restart drops every one, which errs toward asking again.
//
// The limits and the pause after rejections are ported from the held-action
// store of openwrt-mcp (approval.go, MIT, Ian Williams and Yurii Kostiuk),
// without its UCI configuration: there a router change waits, here a root
// command does.
package store

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/card"
)

// Limits bound how many cards one uid can put in front of a person.
type Limits struct {
	TTL        time.Duration
	MaxOpen    int // held or approved, not yet run
	MaxPerHour int
	PauseAfter int // rejections in a row before a pause
	Pause      time.Duration
}

// DefaultLimits are the values of spec 6.3.
func DefaultLimits() Limits {
	return Limits{TTL: 10 * time.Minute, MaxOpen: 3, MaxPerHour: 30, PauseAfter: 3, Pause: 15 * time.Minute}
}

var (
	ErrTooManyOpen = errors.New("too many open requests; wait for the person to answer")
	ErrHourlyLimit = errors.New("hourly request limit reached")
	ErrPaused      = errors.New("paused after rejections in a row")
	ErrUnknown     = errors.New("no such request")
)

// keepFinished is how long a decided card stays listable.
const keepFinished = time.Hour

// idAlphabet leaves out I and O, which read as 1 and 0.
const idAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ"

type entry struct {
	rec     card.Record
	changed chan struct{}
}

type uidState struct {
	asked       []time.Time
	rejectRun   int
	pausedUntil time.Time
}

// Store holds records by id. Every method is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	l     Limits
	clock func() time.Time
	recs  map[string]*entry
	uids  map[uint32]*uidState
}

// NewStore returns an empty store.
func NewStore(l Limits, clock func() time.Time) *Store {
	return &Store{l: l, clock: clock, recs: map[string]*entry{}, uids: map[uint32]*uidState{}}
}

// Limits returns the limits the store applies.
func (s *Store) Limits() Limits { return s.l }

// NewID returns a 5-letter id no record in the store carries.
func (s *Store) NewID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		var b [5]byte
		if _, err := rand.Read(b[:]); err != nil {
			panic("onceroot: crypto/rand failed: " + err.Error())
		}
		for i := range b {
			b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
		}
		if _, taken := s.recs[string(b[:])]; !taken {
			return string(b[:])
		}
	}
}

// Hold puts a new card in front of a person, within the limits.
func (s *Store) Hold(c card.Card) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock()
	u := s.uid(c.Req.UID)
	if now.Before(u.pausedUntil) {
		return ErrPaused
	}
	open := 0
	for _, e := range s.recs {
		if e.rec.Card.Req.UID == c.Req.UID && (e.rec.Status == card.Held || e.rec.Status == card.Approved) {
			open++
		}
	}
	if open >= s.l.MaxOpen {
		return ErrTooManyOpen
	}
	recent := u.asked[:0]
	for _, t := range u.asked {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	u.asked = recent
	if len(u.asked) >= s.l.MaxPerHour {
		return ErrHourlyLimit
	}
	u.asked = append(u.asked, now)
	s.recs[c.ID] = &entry{rec: card.Record{Card: c, Status: card.Held, At: now}, changed: make(chan struct{})}
	return nil
}

// Get returns a copy of a record.
func (s *Store) Get(id string) (card.Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.recs[id]
	if !ok {
		return card.Record{}, false
	}
	return e.rec, true
}

// Decide approves (for the card's own hash) or rejects a held card. Three
// rejections in a row pause the uid; an approval ends the run.
func (s *Store) Decide(id string, h [32]byte, approve bool, by string) error {
	return s.update(id, func(r *card.Record, now time.Time) error {
		u := s.uid(r.Card.Req.UID)
		if approve {
			if err := r.Approve(h, by, now); err != nil {
				return err
			}
			u.rejectRun = 0
			return nil
		}
		if err := r.Reject(by, now); err != nil {
			return err
		}
		u.rejectRun++
		if u.rejectRun >= s.l.PauseAfter {
			u.pausedUntil = now.Add(s.l.Pause)
			u.rejectRun = 0
		}
		return nil
	})
}

// Withdraw ends a card whose requester left.
func (s *Store) Withdraw(id string) error {
	return s.update(id, func(r *card.Record, now time.Time) error { return r.Withdraw(now) })
}

// MarkRan records the one execution of an approved card.
func (s *Store) MarkRan(id string) error {
	return s.update(id, func(r *card.Record, now time.Time) error { return r.MarkRan(now) })
}

// Sweep expires held cards past their time and forgets decided ones older
// than an hour. It returns the ids it expired.
func (s *Store) Sweep() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock()
	var expired []string
	for id, e := range s.recs {
		switch {
		case e.rec.Status == card.Held && !now.Before(e.rec.Card.Expires):
			if e.rec.Expire(now) == nil {
				expired = append(expired, id)
				s.signal(e)
			}
		case e.rec.Status != card.Held && e.rec.Status != card.Approved && now.Sub(e.rec.At) > keepFinished:
			delete(s.recs, id)
		}
	}
	return expired
}

// Open lists the uid's held and approved cards.
func (s *Store) Open(uid uint32) []card.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []card.Record
	for _, e := range s.recs {
		if e.rec.Card.Req.UID == uid && (e.rec.Status == card.Held || e.rec.Status == card.Approved) {
			out = append(out, e.rec)
		}
	}
	return out
}

// All lists every record the store still keeps.
func (s *Store) All() []card.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]card.Record, 0, len(s.recs))
	for _, e := range s.recs {
		out = append(out, e.rec)
	}
	return out
}

// Changed returns a channel closed at the record's next status change.
func (s *Store) Changed(id string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.recs[id]; ok {
		return e.changed
	}
	closed := make(chan struct{})
	close(closed)
	return closed
}

func (s *Store) update(id string, f func(*card.Record, time.Time) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.recs[id]
	if !ok {
		return ErrUnknown
	}
	if err := f(&e.rec, s.clock()); err != nil {
		return err
	}
	s.signal(e)
	return nil
}

func (s *Store) signal(e *entry) {
	close(e.changed)
	e.changed = make(chan struct{})
}

func (s *Store) uid(uid uint32) *uidState {
	u, ok := s.uids[uid]
	if !ok {
		u = &uidState{}
		s.uids[uid] = u
	}
	return u
}
