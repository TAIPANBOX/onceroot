package card

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func sample() Card {
	req := Requester{UID: 1000, PID: 4242, StartTime: 987654, Comm: "claude",
		Chain: []string{"claude", "ghostty"}, CWD: "/home/u/Work/app"}
	bin := [32]byte{1, 2, 3}
	return New("KQWPT", req, "/usr/bin/pacman", bin,
		[]string{"pacman", "-S", "--noconfirm", "ripgrep"}, "install ripgrep",
		[]string{"installs a package"}, t0, 10*time.Minute)
}

func TestHashCoversEveryField(t *testing.T) {
	base := sample()
	variants := map[string]func(*Card){
		"id":         func(c *Card) { c.ID = "AAAAA" },
		"uid":        func(c *Card) { c.Req.UID = 1001 },
		"pid":        func(c *Card) { c.Req.PID = 1 },
		"start time": func(c *Card) { c.Req.StartTime = 1 },
		"comm":       func(c *Card) { c.Req.Comm = "codex" },
		"chain":      func(c *Card) { c.Req.Chain = []string{"claude"} },
		"cwd":        func(c *Card) { c.Req.CWD = "/" },
		"path":       func(c *Card) { c.Path = "/usr/local/bin/pacman" },
		"binary":     func(c *Card) { c.BinarySHA256 = [32]byte{9} },
		"argv":       func(c *Card) { c.Argv = []string{"pacman", "-R", "--noconfirm", "ripgrep"} },
		"why":        func(c *Card) { c.Why = "remove ripgrep" },
		"risks":      func(c *Card) { c.Risks = nil },
		"created":    func(c *Card) { c.Created = c.Created.Add(time.Second) },
		"expires":    func(c *Card) { c.Expires = c.Expires.Add(time.Second) },
	}
	for name, mutate := range variants {
		c := base
		mutate(&c)
		if computeHash(c) == base.Hash {
			t.Errorf("changing %s does not change the hash", name)
		}
	}
	if computeHash(base) != base.Hash {
		t.Fatal("New stores a hash computeHash does not reproduce")
	}
}

func TestHashSeparatesArguments(t *testing.T) {
	a := sample()
	a.Argv = []string{"echo", "a b"}
	b := sample()
	b.Argv = []string{"echo", "a", "b"}
	c := sample()
	c.Argv = []string{"echo", "ab", ""}
	if computeHash(a) == computeHash(b) || computeHash(b) == computeHash(c) || computeHash(a) == computeHash(c) {
		t.Fatal("different argument lists hash the same")
	}
}

func TestApproveNeedsMatchingHash(t *testing.T) {
	r := Record{Card: sample()}
	wrong := r.Card.Hash
	wrong[0] ^= 1
	if err := r.Approve(wrong, "polkit", t0.Add(time.Minute)); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("wrong hash: got %v, want ErrHashMismatch", err)
	}
	if r.Status != Held {
		t.Fatalf("a refused approval moved the status to %v", r.Status)
	}
	if err := r.Approve(r.Card.Hash, "polkit", t0.Add(time.Minute)); err != nil {
		t.Fatalf("matching hash refused: %v", err)
	}
	if r.Status != Approved || r.By != "polkit" {
		t.Fatalf("status %v by %q after approval", r.Status, r.By)
	}
}

func TestApprovedCardRunsOnce(t *testing.T) {
	r := Record{Card: sample()}
	if err := r.Approve(r.Card.Hash, "polkit", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkRan(t0.Add(2 * time.Minute)); err != nil {
		t.Fatalf("first run refused: %v", err)
	}
	if err := r.MarkRan(t0.Add(3 * time.Minute)); !errors.Is(err, ErrAlreadyRan) {
		t.Fatalf("second run: got %v, want ErrAlreadyRan", err)
	}
}

func TestHeldCardCannotRun(t *testing.T) {
	r := Record{Card: sample()}
	if err := r.MarkRan(t0.Add(time.Minute)); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("running a held card: got %v, want ErrNotApproved", err)
	}
}

func TestExpiredCardCannotBeApproved(t *testing.T) {
	r := Record{Card: sample()}
	if err := r.Approve(r.Card.Hash, "polkit", r.Card.Expires); !errors.Is(err, ErrExpired) {
		t.Fatalf("approval at expiry: got %v, want ErrExpired", err)
	}
}

func TestExpiredCardCannotRun(t *testing.T) {
	r := Record{Card: sample()}
	if err := r.Approve(r.Card.Hash, "polkit", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkRan(r.Card.Expires); !errors.Is(err, ErrExpired) {
		t.Fatalf("run at expiry: got %v, want ErrExpired", err)
	}
}

func TestStatusMovesOnce(t *testing.T) {
	now := t0.Add(time.Minute)

	rejected := Record{Card: sample()}
	if err := rejected.Reject("polkit", now); err != nil {
		t.Fatal(err)
	}
	if err := rejected.Approve(rejected.Card.Hash, "phone", now); !errors.Is(err, ErrNotHeld) {
		t.Fatalf("approving a rejected card: got %v, want ErrNotHeld", err)
	}

	withdrawn := Record{Card: sample()}
	if err := withdrawn.Withdraw(now); err != nil {
		t.Fatal(err)
	}
	for name, step := range map[string]func() error{
		"approve": func() error { return withdrawn.Approve(withdrawn.Card.Hash, "polkit", now) },
		"reject":  func() error { return withdrawn.Reject("polkit", now) },
		"expire":  func() error { return withdrawn.Expire(now) },
		"run":     func() error { return withdrawn.MarkRan(now) },
	} {
		if err := step(); err == nil {
			t.Errorf("%s moved a withdrawn card", name)
		}
	}
	if withdrawn.Status != Withdrawn {
		t.Fatalf("withdrawn card ended as %v", withdrawn.Status)
	}

	approved := Record{Card: sample()}
	if err := approved.Approve(approved.Card.Hash, "polkit", now); err != nil {
		t.Fatal(err)
	}
	if err := approved.Withdraw(now); err != nil {
		t.Fatalf("an approved card that has not run must be withdrawable: %v", err)
	}
	if err := approved.MarkRan(now); err == nil {
		t.Fatal("a withdrawn approval still ran")
	}
}

func TestStatusNames(t *testing.T) {
	want := map[Status]string{Held: "held", Approved: "approved", Ran: "ran", Rejected: "rejected", Expired: "expired", Withdrawn: "withdrawn", Status(99): "unknown"}
	for s, name := range want {
		if s.String() != name {
			t.Errorf("%d: %q, want %q", s, s.String(), name)
		}
	}
}
