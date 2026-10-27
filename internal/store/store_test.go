package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/card"
)

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time          { return f.now }
func (f *fakeClock) Advance(d time.Duration) { f.now = f.now.Add(d) }

func newStore(l Limits) (*Store, *fakeClock) {
	clk := &fakeClock{now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	return NewStore(l, clk.Now), clk
}

func cardFor(s *Store, clk *fakeClock, uid uint32) card.Card {
	req := card.Requester{UID: uid, PID: 100, StartTime: 1, Comm: "claude", CWD: "/w"}
	return card.New(s.NewID(), req, "/usr/bin/pacman", [32]byte{1}, []string{"pacman", "-S", "x"}, "", nil, clk.Now(), s.Limits().TTL)
}

func TestDefaultLimits(t *testing.T) {
	l := DefaultLimits()
	if l.TTL != 10*time.Minute || l.MaxOpen != 3 || l.MaxPerHour != 30 || l.PauseAfter != 3 || l.Pause != 15*time.Minute {
		t.Fatalf("defaults %+v differ from spec 6.3", l)
	}
}

func TestOpenLimit(t *testing.T) {
	s, clk := newStore(DefaultLimits())
	for i := range 3 {
		if err := s.Hold(cardFor(s, clk, 1000)); err != nil {
			t.Fatalf("card %d: %v", i, err)
		}
	}
	if err := s.Hold(cardFor(s, clk, 1000)); !errors.Is(err, ErrTooManyOpen) {
		t.Fatalf("fourth open card: got %v, want ErrTooManyOpen", err)
	}
	if err := s.Hold(cardFor(s, clk, 1001)); err != nil {
		t.Fatalf("another uid is limited by the first one's cards: %v", err)
	}
}

func TestHourlyLimitSlides(t *testing.T) {
	l := DefaultLimits()
	l.MaxPerHour = 2
	s, clk := newStore(l)
	for range 2 {
		c := cardFor(s, clk, 1000)
		if err := s.Hold(c); err != nil {
			t.Fatal(err)
		}
		if err := s.Decide(c.ID, c.Hash, true, "polkit"); err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Minute)
	}
	if err := s.Hold(cardFor(s, clk, 1000)); !errors.Is(err, ErrHourlyLimit) {
		t.Fatalf("third card in an hour: got %v, want ErrHourlyLimit", err)
	}
	clk.Advance(59 * time.Minute)
	if err := s.Hold(cardFor(s, clk, 1000)); err != nil {
		t.Fatalf("after the first card left the hour: %v", err)
	}
}

func TestPauseAfterRejectionsInARow(t *testing.T) {
	s, clk := newStore(DefaultLimits())
	reject := func() {
		t.Helper()
		c := cardFor(s, clk, 1000)
		if err := s.Hold(c); err != nil {
			t.Fatal(err)
		}
		if err := s.Decide(c.ID, c.Hash, false, "polkit"); err != nil {
			t.Fatal(err)
		}
	}
	approve := func() {
		t.Helper()
		c := cardFor(s, clk, 1000)
		if err := s.Hold(c); err != nil {
			t.Fatal(err)
		}
		if err := s.Decide(c.ID, c.Hash, true, "polkit"); err != nil {
			t.Fatal(err)
		}
	}
	reject()
	reject()
	approve() // breaks the run
	reject()
	reject()
	if err := s.Hold(cardFor(s, clk, 1000)); err != nil {
		t.Fatalf("paused after two rejections in a row: %v", err)
	}
	s2, clk2 := newStore(DefaultLimits())
	s, clk = s2, clk2
	reject()
	reject()
	reject()
	if err := s.Hold(cardFor(s, clk, 1000)); !errors.Is(err, ErrPaused) {
		t.Fatalf("after three rejections in a row: got %v, want ErrPaused", err)
	}
	clk.Advance(15 * time.Minute)
	if err := s.Hold(cardFor(s, clk, 1000)); err != nil {
		t.Fatalf("pause did not end after 15 minutes: %v", err)
	}
}

func TestSweepExpires(t *testing.T) {
	s, clk := newStore(DefaultLimits())
	c := cardFor(s, clk, 1000)
	if err := s.Hold(c); err != nil {
		t.Fatal(err)
	}
	clk.Advance(9 * time.Minute)
	if got := s.Sweep(); len(got) != 0 {
		t.Fatalf("swept %v before expiry", got)
	}
	clk.Advance(time.Minute)
	if got := s.Sweep(); len(got) != 1 || got[0] != c.ID {
		t.Fatalf("swept %v, want [%s]", got, c.ID)
	}
	if rec, _ := s.Get(c.ID); rec.Status != card.Expired {
		t.Fatalf("status %v after sweep", rec.Status)
	}
	if err := s.Decide(c.ID, c.Hash, true, "polkit"); err == nil {
		t.Fatal("an expired card was approved")
	}
}

func TestDecideRefusesWrongHash(t *testing.T) {
	s, clk := newStore(DefaultLimits())
	c := cardFor(s, clk, 1000)
	if err := s.Hold(c); err != nil {
		t.Fatal(err)
	}
	h := c.Hash
	h[31] ^= 0xff
	if err := s.Decide(c.ID, h, true, "phone"); !errors.Is(err, card.ErrHashMismatch) {
		t.Fatalf("got %v, want ErrHashMismatch", err)
	}
	if err := s.Decide("ZZZZZ", c.Hash, true, "phone"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown id: got %v, want ErrUnknown", err)
	}
}

func TestMarkRanOnce(t *testing.T) {
	s, clk := newStore(DefaultLimits())
	c := cardFor(s, clk, 1000)
	if err := s.Hold(c); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRan(c.ID); !errors.Is(err, card.ErrNotApproved) {
		t.Fatalf("held card ran: %v", err)
	}
	if err := s.Decide(c.ID, c.Hash, true, "polkit"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRan(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRan(c.ID); !errors.Is(err, card.ErrAlreadyRan) {
		t.Fatalf("second run: got %v, want ErrAlreadyRan", err)
	}
}

func TestNewIDAlphabet(t *testing.T) {
	s, _ := newStore(DefaultLimits())
	seen := map[string]bool{}
	for range 2000 {
		id := s.NewID()
		if len(id) != 5 || strings.Trim(id, idAlphabet) != "" || strings.ContainsAny(id, "IO") {
			t.Fatalf("bad id %q", id)
		}
		seen[id] = true
	}
	if len(seen) < 1990 {
		t.Fatalf("only %d distinct ids in 2000", len(seen))
	}
}

func TestChangedFiresOnDecision(t *testing.T) {
	s, clk := newStore(DefaultLimits())
	c := cardFor(s, clk, 1000)
	if err := s.Hold(c); err != nil {
		t.Fatal(err)
	}
	ch := s.Changed(c.ID)
	select {
	case <-ch:
		t.Fatal("fired before any change")
	default:
	}
	if err := s.Decide(c.ID, c.Hash, true, "polkit"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("did not fire on approval")
	}
}

func TestWithdrawEndsAnApprovedCard(t *testing.T) {
	s, clk := newStore(DefaultLimits())
	c := cardFor(s, clk, 1000)
	if err := s.Hold(c); err != nil {
		t.Fatal(err)
	}
	if err := s.Decide(c.ID, c.Hash, true, "polkit"); err != nil {
		t.Fatal(err)
	}
	if err := s.Withdraw(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRan(c.ID); err == nil {
		t.Fatal("a withdrawn card ran")
	}
	if got := s.Open(1000); len(got) != 0 {
		t.Fatalf("withdrawn card still open: %v", got)
	}
}

func TestChangedForAnUnknownCardIsAlreadyClosed(t *testing.T) {
	s, _ := newStore(DefaultLimits())
	select {
	case <-s.Changed("NOSUCH"):
	default:
		t.Fatal("waiting on an unknown card would block forever")
	}
}
