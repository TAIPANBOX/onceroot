package daemon

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/card"
	"github.com/TAIPANBOX/onceroot/internal/client"
	"github.com/TAIPANBOX/onceroot/internal/polkit"
	"github.com/TAIPANBOX/onceroot/internal/rules"
	"github.com/TAIPANBOX/onceroot/internal/store"
	"github.com/TAIPANBOX/onceroot/internal/wire"
)

// ---- fakes

type fakePolkit struct {
	mu      sync.Mutex
	answer  func(r card.Requester) (ok bool, block bool)
	calls   []string
	noAgent func(r card.Requester) bool // pkcheck's exit 2 for this subject
}

func (f *fakePolkit) Check(ctx context.Context, r card.Requester, msg string) (bool, error) {
	f.mu.Lock()
	f.calls = append(f.calls, msg)
	ok, block := f.answer(r)
	noAgent := f.noAgent != nil && f.noAgent(r)
	f.mu.Unlock()
	if noAgent {
		return false, fmt.Errorf("pkcheck: %w", polkit.ErrNoAgent)
	}
	if block {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return ok, nil
}

type fakeRunner struct {
	mu   sync.Mutex
	runs [][]string
}

func (f *fakeRunner) Run(ctx context.Context, path string, argv []string, cwd string, limit time.Duration, out func(int, []byte)) (int, error) {
	f.mu.Lock()
	f.runs = append(f.runs, append([]string{path}, argv...))
	f.mu.Unlock()
	out(1, []byte("ran "+strings.Join(argv, " ")+"\n"))
	return 0, nil
}

func (f *fakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs)
}

type memJournal struct {
	mu    sync.Mutex
	kinds []string
	ids   []string
	fail  bool
}

func (j *memJournal) Append(kind, id string, h [32]byte, d map[string]string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.fail {
		return errors.New("disk full")
	}
	j.kinds = append(j.kinds, kind)
	j.ids = append(j.ids, id)
	return nil
}

func (j *memJournal) list() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.kinds...)
}

type fakePhone struct {
	mu      sync.Mutex
	offered map[string]chan Decision
	got     chan string
}

func (p *fakePhone) Offer(ctx context.Context, rec card.Record, text string) (<-chan Decision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan Decision, 1)
	p.offered[rec.Card.ID] = ch
	p.got <- rec.Card.ID
	return ch, nil
}

func (p *fakePhone) Withdraw(id string) {}

func (p *fakePhone) decide(id string, approve bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offered[id] <- Decision{Approve: approve, By: "phone"}
}

// ---- harness

type harness struct {
	sock    string
	d       *Daemon
	polkit  *fakePolkit
	runner  *fakeRunner
	journal *memJournal
	store   *store.Store
	peers   chan card.Requester
	changed error
	sudoers string
}

const agentPID, approverPID = 100, 200

func agent(uid uint32) card.Requester {
	return card.Requester{UID: uid, PID: agentPID, StartTime: 1, Comm: "claude", CWD: "/w"}
}

func approver() card.Requester {
	return card.Requester{UID: 1000, PID: approverPID, StartTime: 2, Comm: "omarchy-shell", CWD: "/"}
}

func start(t *testing.T, rulesText string, phone Phone, ttl time.Duration) *harness {
	t.Helper()
	dir, err := os.MkdirTemp("", "ard")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	rs, err := rules.Parse(strings.NewReader(rulesText))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		sock:    filepath.Join(dir, "s"),
		polkit:  &fakePolkit{answer: func(card.Requester) (bool, bool) { return true, false }},
		runner:  &fakeRunner{},
		journal: &memJournal{},
		peers:   make(chan card.Requester, 16),
		sudoers: filepath.Join(dir, "sudoers.d"),
	}
	os.Mkdir(h.sudoers, 0o755)
	limits := store.DefaultLimits()
	limits.TTL = ttl
	h.store = store.NewStore(limits, time.Now)
	cfg := Config{AllowedUIDs: []uint32{1000, 1001}, Limits: limits, CommandLimit: time.Minute, MaxCardRunes: 4000,
		RootPATH: "/usr/bin", SudoersDir: h.sudoers, DeskDialog: true}
	deps := Deps{
		Peer: func(*net.UnixConn) (card.Requester, error) {
			select {
			case r := <-h.peers:
				return r, nil
			default:
				return agent(1000), nil
			}
		},
		StillSame: func(card.Requester) error { return nil },
		Resolve: func(name, _ string) (string, [32]byte, error) {
			return "/usr/bin/" + filepath.Base(name), [32]byte{7}, nil
		},
		Unchanged: func(string, [32]byte) error { return h.changed },
		Rules:     func() ([]rules.Rule, error) { return rs, nil },
		Polkit:    h.polkit,
		Run:       h.runner.Run,
		Journal:   h.journal,
		Store:     h.store,
		Phone:     phone,
		Clock:     time.Now,
	}
	h.d = New(cfg, deps)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: h.sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go h.d.Serve(l)
	return h
}

func (h *harness) run(argv ...string) (int, string, error) {
	var out, errb bytes.Buffer
	code, err := client.Run(h.sock, argv, "test", &out, &errb)
	return code, out.String() + errb.String(), err
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func heldID(t *testing.T, h *harness) string {
	t.Helper()
	var id string
	waitFor(t, "a held card", func() bool {
		for _, r := range h.store.All() {
			if r.Status == card.Held {
				id = r.Card.ID
				return true
			}
		}
		return false
	})
	return id
}

// ---- tests

func TestAllowRuleRunsWithoutAsking(t *testing.T) {
	h := start(t, "allow /usr/bin/pacman -Q* ...\n", nil, time.Minute)
	code, out, err := h.run("pacman", "-Qi", "ripgrep")
	if err != nil || code != 0 || !strings.Contains(out, "ran pacman -Qi ripgrep") {
		t.Fatalf("code %d out %q err %v", code, out, err)
	}
	if len(h.polkit.calls) != 0 {
		t.Fatal("polkit was asked about an allowed command")
	}
}

func TestDenyNeverRuns(t *testing.T) {
	h := start(t, "allow /usr/bin/bash ...\ndeny /usr/bin/pacman -R* ...\n", nil, time.Minute)
	for _, argv := range [][]string{{"bash", "-c", "id"}, {"pacman", "-Rns", "x"}} {
		if _, _, err := h.run(argv...); !errors.Is(err, client.ErrRefused) {
			t.Errorf("%v: got %v, want a refusal", argv, err)
		}
	}
	if h.runner.count() != 0 {
		t.Fatal("a denied command ran")
	}
}

func TestApprovedCardRuns(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	code, out, err := h.run("pacman", "-S", "--noconfirm", "ripgrep")
	if err != nil || code != 0 || !strings.Contains(out, "ran pacman -S --noconfirm ripgrep") {
		t.Fatalf("code %d out %q err %v", code, out, err)
	}
	if !strings.Contains(out, "ROOT REQUEST") {
		t.Fatalf("the agent was not shown the card: %q", out)
	}
	if len(h.polkit.calls) != 1 || !strings.Contains(h.polkit.calls[0], `"ripgrep"`) {
		t.Fatalf("polkit was not shown the card: %q", h.polkit.calls)
	}
}

func TestDeskRefusalRejectsWhenPhoneOff(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.polkit.answer = func(card.Requester) (bool, bool) { return false, false }
	if _, _, err := h.run("pacman", "-S", "x"); !errors.Is(err, client.ErrRefused) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if h.runner.count() != 0 {
		t.Fatal("a refused card ran")
	}
}

func TestRejectedCardNeverRuns(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.polkit.answer = func(r card.Requester) (bool, bool) { return false, r.PID == agentPID }
	done := make(chan error, 1)
	go func() { _, _, err := h.run("pacman", "-S", "x"); done <- err }()
	id := heldID(t, h)
	h.peers <- approver()
	if _, err := client.Do(h.sock, wire.Request{Op: "reject", ID: id}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, client.ErrRefused) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if h.runner.count() != 0 {
		t.Fatal("a rejected card ran")
	}
}

func TestSocketCannotApprove(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	approverSays := false
	var mu sync.Mutex
	h.polkit.answer = func(r card.Requester) (bool, bool) {
		if r.PID == agentPID {
			return false, true // the desk dialog of the run stays open
		}
		mu.Lock()
		defer mu.Unlock()
		return approverSays, false
	}
	done := make(chan error, 1)
	go func() { _, _, err := h.run("pacman", "-S", "x"); done <- err }()
	id := heldID(t, h)

	h.peers <- approver()
	if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: id}); err == nil {
		t.Fatal("an approve request without the person's yes was accepted")
	}
	if err := <-done; !errors.Is(err, client.ErrRefused) {
		t.Fatalf("the refused approval did not end the request: %v", err)
	}
	if h.runner.count() != 0 {
		t.Fatal("the socket approved a card")
	}
	mu.Lock()
	approverSays = true
	mu.Unlock()
	go func() { _, _, err := h.run("pacman", "-S", "y"); done <- err }()
	id = heldID(t, h)
	h.peers <- approver()
	if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: id}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if h.runner.count() != 1 {
		t.Fatal("the polkit-approved card did not run")
	}
}

func TestForeignUIDRefused(t *testing.T) {
	h := start(t, "allow /usr/bin/pacman ...\n", nil, time.Minute)
	h.peers <- agent(1002)
	if _, _, err := h.run("pacman", "-Q"); !errors.Is(err, client.ErrRefused) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if h.runner.count() != 0 {
		t.Fatal("a foreign uid's command ran")
	}
}

func TestBinaryChangedBeforeRunRefuses(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.changed = errors.New("changed")
	if _, _, err := h.run("pacman", "-S", "x"); err == nil {
		t.Fatal("a changed binary ran")
	}
	if h.runner.count() != 0 {
		t.Fatal("a changed binary ran")
	}
}

func TestPidReuseBeforeRunRefuses(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.d.d.StillSame = func(card.Requester) error { return errors.New("pid reused") }
	if _, _, err := h.run("pacman", "-S", "x"); err == nil {
		t.Fatal("ran for a process that is gone")
	}
	if h.runner.count() != 0 {
		t.Fatal("ran for a process that is gone")
	}
}

func TestJournalFailureRefuses(t *testing.T) {
	h := start(t, "allow /usr/bin/pacman ...\n", nil, time.Minute)
	h.journal.fail = true
	if _, _, err := h.run("pacman", "-Q"); err == nil {
		t.Fatal("a request ran without a journal")
	}
	if h.runner.count() != 0 {
		t.Fatal("a request ran without a journal")
	}
}

func TestCardWithdrawnWhenClientDisconnects(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.polkit.answer = func(r card.Requester) (bool, bool) { return r.PID != agentPID, r.PID == agentPID }
	c, err := net.Dial("unix", h.sock)
	if err != nil {
		t.Fatal(err)
	}
	wire.WriteRequest(c, wire.Request{Op: "run", Argv: []string{"pacman", "-S", "x"}})
	e, err := wire.ReadEvent(bufio.NewReader(c))
	if err != nil || e.Ev != "held" {
		t.Fatalf("got %+v %v", e, err)
	}
	c.Close()
	waitFor(t, "withdrawal", func() bool { r, _ := h.store.Get(e.ID); return r.Status == card.Withdrawn })
	h.peers <- approver()
	if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: e.ID}); err == nil {
		t.Fatal("a withdrawn card was approved")
	}
	if h.runner.count() != 0 {
		t.Fatal("a withdrawn card ran")
	}
}

func TestEveryStepJournalled(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	if _, _, err := h.run("pacman", "-S", "x"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the ran entry", func() bool { k := h.journal.list(); return len(k) > 0 && k[len(k)-1] == "ran" })
	if got := strings.Join(h.journal.list(), ","); got != "request,held,approved,ran" {
		t.Fatalf("journal %s", got)
	}
	h.journal.mu.Lock()
	ids := h.journal.ids
	h.journal.mu.Unlock()
	if ids[1] == "" || ids[1] != ids[2] || ids[2] != ids[3] {
		t.Fatalf("entries name different cards: %v", ids)
	}
}

func TestPasswordlessSudoWarning(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	os.WriteFile(filepath.Join(h.sudoers, "99-omarchy-nopasswd-1000"), []byte("u ALL=(ALL) NOPASSWD: ALL\n"), 0o440)
	_, out, err := h.run("pacman", "-S", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "!! passwordless sudo is on") {
		t.Fatalf("card lacks the warning: %q", out)
	}
	e, err := client.Do(h.sock, wire.Request{Op: "status"})
	if err != nil || !strings.Contains(strings.Join(e.Status, "\n"), "passwordless sudo: on") {
		t.Fatalf("status %v %v", e.Status, err)
	}
}

func TestNoDeskAgentFallsBackToPhone(t *testing.T) {
	phone := &fakePhone{offered: map[string]chan Decision{}, got: make(chan string, 4)}
	h := start(t, "", phone, time.Minute)
	h.polkit.answer = func(card.Requester) (bool, bool) { return false, false }
	done := make(chan error, 1)
	go func() { _, _, err := h.run("pacman", "-S", "x"); done <- err }()
	id := <-phone.got
	time.Sleep(50 * time.Millisecond)
	if r, _ := h.store.Get(id); r.Status != card.Held {
		t.Fatalf("desk refusal ended the card although the phone is on: %v", r.Status)
	}
	phone.decide(id, true)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if h.runner.count() != 1 {
		t.Fatal("the phone-approved card did not run")
	}
}

// A request from a process no dialog can open for (over ssh, say) is not a
// refusal: nobody was asked. The card waits for the review and counts
// toward no pause (VM 2026-10-27: it was rejected "by polkit" at once).
func TestNoDeskAgentLeavesTheCardForTheReview(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.polkit.noAgent = func(r card.Requester) bool { return r.PID == agentPID }
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() { _, out, err := h.run("pacman", "-S", "x"); done <- result{out, err} }()
	id := heldID(t, h)
	waitFor(t, "the desk check", func() bool {
		h.polkit.mu.Lock()
		defer h.polkit.mu.Unlock()
		return len(h.polkit.calls) == 1
	})
	time.Sleep(100 * time.Millisecond)
	if r, _ := h.store.Get(id); r.Status != card.Held {
		t.Fatalf("no dialog could open, and the card ended as %v", r.Status)
	}
	h.peers <- approver()
	if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: id}); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil || h.runner.count() != 1 {
		t.Fatalf("approved from the review but did not run: %v", r.err)
	}
	if !strings.Contains(r.out, "onceroot review") {
		t.Fatalf("the agent was not told why nothing opened: %q", r.out)
	}
}

// Only the requester's own user approves a card, as with list and reject.
func TestApproveRefusesAnotherUsersCard(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.d.cfg.DeskDialog = false
	go h.run("pacman", "-S", "x")
	id := heldID(t, h)
	other := approver()
	other.UID = 1001
	h.peers <- other
	if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: id}); err == nil {
		t.Fatal("another user approved the card")
	}
	h.polkit.mu.Lock()
	calls := len(h.polkit.calls)
	h.polkit.mu.Unlock()
	if r, _ := h.store.Get(id); calls != 0 || r.Status != card.Held {
		t.Fatalf("another user's approve asked polkit %d times, card %v", calls, r.Status)
	}
}

// A dismissed approval dialog rejects the card, so nobody can open it again
// and again until the person gives in.
func TestDismissedApprovalRejects(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.d.cfg.DeskDialog = false
	h.polkit.answer = func(card.Requester) (bool, bool) { return false, false }
	done := make(chan error, 1)
	go func() { _, _, err := h.run("pacman", "-S", "x"); done <- err }()
	id := heldID(t, h)
	for i := 0; i < 2; i++ {
		h.peers <- approver()
		if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: id}); err == nil {
			t.Fatal("a dismissed dialog approved")
		}
	}
	if err := <-done; !errors.Is(err, client.ErrRefused) {
		t.Fatalf("the agent got %v, want a refusal", err)
	}
	h.polkit.mu.Lock()
	calls := len(h.polkit.calls)
	h.polkit.mu.Unlock()
	if r, _ := h.store.Get(id); r.Status != card.Rejected || calls != 1 {
		t.Fatalf("card %v after %d dialogs, want rejected after one", r.Status, calls)
	}
}

// An approver with no authentication agent (over ssh) was not asked: the
// card stays held for an approval that can show it.
func TestApproveWithoutAnAgentKeepsTheCard(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.d.cfg.DeskDialog = false
	h.polkit.noAgent = func(r card.Requester) bool { return r.PID == approverPID }
	go h.run("pacman", "-S", "x")
	id := heldID(t, h)
	h.peers <- approver()
	if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: id}); err == nil {
		t.Fatal("approved without anyone asked")
	}
	if r, _ := h.store.Get(id); r.Status != card.Held {
		t.Fatalf("card %v, want it still held", r.Status)
	}
}

func TestExpiredCardNeverRuns(t *testing.T) {
	h := start(t, "", nil, 300*time.Millisecond)
	h.polkit.answer = func(card.Requester) (bool, bool) { return false, true }
	_, _, err := h.run("pacman", "-S", "x")
	if !errors.Is(err, client.ErrRefused) || !strings.Contains(err.Error(), "expire") {
		t.Fatalf("got %v, want an expiry", err)
	}
	if h.runner.count() != 0 {
		t.Fatal("an expired card ran")
	}
}

func TestListShowsHeldCards(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.polkit.answer = func(r card.Requester) (bool, bool) { return false, true }
	go h.run("pacman", "-S", "x")
	id := heldID(t, h)
	e, err := client.Do(h.sock, wire.Request{Op: "list"})
	if err != nil || len(e.Cards) != 1 || e.Cards[0].ID != id || e.Cards[0].Status != "held" || !strings.Contains(e.Cards[0].Card, "ROOT REQUEST "+id) {
		t.Fatalf("list %+v %v", e, err)
	}
}

func TestBrokenRulesRefuse(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.d.d.Rules = func() ([]rules.Rule, error) { return nil, errors.New("rules: line 3: unknown verb") }
	if _, _, err := h.run("pacman", "-Q"); !errors.Is(err, client.ErrRefused) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if h.runner.count() != 0 || len(h.polkit.calls) != 0 {
		t.Fatal("a request went ahead with an unreadable rules file")
	}
}

func TestListShowsOnlyOwnCards(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.polkit.answer = func(card.Requester) (bool, bool) { return false, true }
	go h.run("pacman", "-S", "x")
	heldID(t, h)
	h.peers <- agent(1001)
	e, err := client.Do(h.sock, wire.Request{Op: "list"})
	if err != nil || len(e.Cards) != 0 {
		t.Fatalf("uid 1001 saw uid 1000's cards: %+v %v", e.Cards, err)
	}
}

func TestRejectRefusesAnotherUsersCard(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.polkit.answer = func(card.Requester) (bool, bool) { return false, true }
	go h.run("pacman", "-S", "x")
	id := heldID(t, h)
	h.peers <- agent(1001)
	if _, err := client.Do(h.sock, wire.Request{Op: "reject", ID: id}); err == nil {
		t.Fatal("uid 1001 rejected uid 1000's card")
	}
	if r, _ := h.store.Get(id); r.Status != card.Held {
		t.Fatalf("status %v after a foreign reject", r.Status)
	}
}

func TestReviewModeNeverOpensTheDialogOnRun(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.d.cfg.DeskDialog = false
	done := make(chan error, 1)
	go func() { _, _, err := h.run("pacman", "-S", "x"); done <- err }()
	id := heldID(t, h)
	time.Sleep(100 * time.Millisecond)
	h.polkit.mu.Lock()
	calls := len(h.polkit.calls)
	h.polkit.mu.Unlock()
	if calls != 0 {
		t.Fatalf("polkit was asked %d times before anyone opened the review", calls)
	}
	h.peers <- approver()
	if _, err := client.Do(h.sock, wire.Request{Op: "approve", ID: id}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil || h.runner.count() != 1 {
		t.Fatalf("approved from the review but did not run: %v", err)
	}
}

func TestPolkitGetsTheDialogCard(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	_, out, err := h.run("pacman", "-S", "--noconfirm", "ripgrep")
	if err != nil {
		t.Fatal(err)
	}
	h.polkit.mu.Lock()
	msg := h.polkit.calls[0]
	h.polkit.mu.Unlock()
	if !strings.Contains(msg, `Run: "/usr/bin/pacman" "-S" "--noconfirm" "ripgrep"`) || strings.Count(msg, "\n")+1 != 6 {
		t.Fatalf("polkit was not given the dialog card:\n%s", msg)
	}
	if !strings.Contains(out, "\n  \"ripgrep\"") {
		t.Fatalf("the agent's terminal did not get the full card: %q", out)
	}
}

// A card the dialog cannot show whole is refused when it is asked for: a
// dialog that does not show the command must never be able to approve it.
// Allowed commands need no dialog and are not affected.
func TestCardTooLongForTheDialogIsRefused(t *testing.T) {
	h := start(t, "allow /usr/bin/ls ...\n", nil, time.Minute)
	long := "/" + strings.Repeat("x", 150)
	_, out, err := h.run("touch", long)
	if !errors.Is(err, client.ErrRefused) || !strings.Contains(err.Error(), "approval dialog") {
		t.Fatalf("got %v, want a refusal naming the dialog (%q)", err, out)
	}
	h.polkit.mu.Lock()
	calls := len(h.polkit.calls)
	h.polkit.mu.Unlock()
	if len(h.store.All()) != 0 || calls != 0 || h.runner.count() != 0 {
		t.Fatal("a card the dialog cannot show was held, shown or run")
	}
	if code, _, err := h.run("ls", long); err != nil || code != 0 || h.runner.count() != 1 {
		t.Fatalf("an allowed command needs no dialog, yet: %d %v", code, err)
	}
}

// Omarchy's dialog centres the text on a one-line band above the password
// field; as many empty lines below as the card has keep it above the field.
func TestDialogLiftsTheCardOnOmarchy(t *testing.T) {
	h := start(t, "", nil, time.Minute)
	h.d.cfg.DialogLift = true
	if _, _, err := h.run("pacman", "-S", "ripgrep"); err != nil {
		t.Fatal(err)
	}
	h.polkit.mu.Lock()
	msg := h.polkit.calls[0]
	h.polkit.mu.Unlock()
	body := strings.TrimRight(msg, "\n")
	lines := strings.Count(body, "\n") + 1
	if lines != 6 || len(msg)-len(body) != lines {
		t.Fatalf("%d card lines and %d empty ones after:\n%q", lines, len(msg)-len(body), msg)
	}
}

func TestCommandFailureIsReported(t *testing.T) {
	h := start(t, "allow /usr/bin/pacman ...\n", nil, time.Minute)
	h.d.d.Run = func(context.Context, string, []string, string, time.Duration, func(int, []byte)) (int, error) {
		return -1, errors.New("killed after the 30m limit")
	}
	if _, _, err := h.run("pacman", "-Q"); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("got %v, want the runner's error", err)
	}
	if k := h.journal.list(); k[len(k)-1] != "ran" {
		t.Fatalf("the failed run was not journalled: %v", k)
	}
}

func TestJournalFailureAfterTheRunIsReported(t *testing.T) {
	h := start(t, "allow /usr/bin/pacman ...\n", nil, time.Minute)
	h.d.d.Run = func(ctx context.Context, p string, a []string, c string, l time.Duration, out func(int, []byte)) (int, error) {
		h.journal.mu.Lock()
		h.journal.fail = true
		h.journal.mu.Unlock()
		return 0, nil
	}
	if _, _, err := h.run("pacman", "-Q"); err == nil || !strings.Contains(err.Error(), "journal failed") {
		t.Fatalf("got %v, want the journal failure", err)
	}
}
