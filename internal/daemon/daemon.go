// Package daemon is oncerootd's request flow. It owns every decision: the
// socket takes requests, never approvals; an approval comes only from a
// person through polkit or, when enabled, the phone. Every step is journalled
// before the matching reply goes out, and a journal that fails refuses.
package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/card"
	"github.com/TAIPANBOX/onceroot/internal/polkit"
	"github.com/TAIPANBOX/onceroot/internal/rules"
	"github.com/TAIPANBOX/onceroot/internal/store"
	"github.com/TAIPANBOX/onceroot/internal/wire"
)

// Decision is a person's answer from a channel other than the desk.
type Decision struct {
	Approve bool
	By      string
}

// Phone offers a held card to a person away from the desk.
type Phone interface {
	Offer(ctx context.Context, rec card.Record, text string) (<-chan Decision, error)
	Withdraw(id string)
}

// Journal records each step.
type Journal interface {
	Append(kind, cardID string, hash [32]byte, detail map[string]string) error
}

// Config is what oncerootd reads from its config file.
type Config struct {
	AllowedUIDs  []uint32
	Limits       store.Limits
	CommandLimit time.Duration
	MaxCardRunes int
	RootPATH     string
	SudoersDir   string
	// DeskDialog opens the polkit dialog as soon as a card is held. Without
	// it, a card waits until the person opens it (onceroot review or the
	// bar) and approves from there, where the whole card is on screen
	// whatever the polkit agent shows.
	DeskDialog bool
	// DialogLift puts empty lines under the text polkit shows: Omarchy's
	// dialog centres it on a one-line band just above the password field,
	// and the lines below the middle would cover the field.
	DialogLift bool
}

// Deps are the daemon's collaborators, real in oncerootd and fakes in tests.
type Deps struct {
	Peer      func(*net.UnixConn) (card.Requester, error)
	StillSame func(card.Requester) error
	Resolve   func(name, rootPATH string) (string, [32]byte, error)
	Unchanged func(path string, sha [32]byte) error
	Rules     func() ([]rules.Rule, error)
	Polkit    polkit.Checker
	Run       func(ctx context.Context, path string, argv []string, cwd string, limit time.Duration, out func(int, []byte)) (int, error)
	Journal   Journal
	Store     *store.Store
	Phone     Phone // nil when the phone module is off
	Clock     func() time.Time
}

// Daemon serves onceroot's socket.
type Daemon struct {
	cfg Config
	d   Deps

	// logged carries, per held card, the result of journalling the decision
	// on it, so approve and reject answer only after the journal has it (I9).
	loggedMu sync.Mutex
	logged   map[string]chan error
}

// New returns a daemon.
func New(cfg Config, d Deps) *Daemon {
	return &Daemon{cfg: cfg, d: d, logged: map[string]chan error{}}
}

// decisionLog is where the requester of a held card reports whether its
// decision reached the journal.
func (dm *Daemon) decisionLog(id string) chan error {
	dm.loggedMu.Lock()
	defer dm.loggedMu.Unlock()
	return dm.logged[id]
}

// awaitLogged waits for the decision on id to reach the journal.
func awaitLogged(logged chan error, expires time.Time) error {
	if logged == nil {
		return errors.New("the request is no longer waiting")
	}
	select {
	case err := <-logged:
		return err
	case <-time.After(time.Until(expires) + 10*time.Second):
		return errors.New("the decision was not journalled in time")
	}
}

// Serve accepts connections until the listener closes.
func (dm *Daemon) Serve(l *net.UnixListener) error {
	for {
		c, err := l.AcceptUnix()
		if err != nil {
			return err
		}
		go dm.handle(c)
	}
}

// events writes one connection's answer, one event per line.
type events struct {
	mu sync.Mutex
	c  net.Conn
}

func (e *events) send(ev wire.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	wire.WriteEvent(e.c, ev)
}

func (e *events) refuse(verdict, reason string) {
	e.send(wire.Event{Ev: "decided", Verdict: verdict, Reason: reason})
}

func (dm *Daemon) handle(c *net.UnixConn) {
	defer c.Close()
	ev := &events{c: c}
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	req, err := wire.Decode(bufio.NewReader(c))
	if err != nil {
		ev.send(wire.Event{Ev: "error", Reason: err.Error()})
		return
	}
	c.SetReadDeadline(time.Time{})
	r, err := dm.d.Peer(c)
	if err != nil {
		ev.send(wire.Event{Ev: "error", Reason: "cannot identify the caller: " + err.Error()})
		return
	}
	if !slices.Contains(dm.cfg.AllowedUIDs, r.UID) {
		var none [32]byte
		dm.d.Journal.Append("refused", "", none, map[string]string{"uid": strconv.Itoa(int(r.UID)), "reason": "uid not allowed"})
		ev.refuse("refuse", fmt.Sprintf("uid %d may not use onceroot", r.UID))
		return
	}
	switch req.Op {
	case "run":
		dm.run(c, ev, r, req)
	case "approve":
		dm.approve(ev, r, req.ID)
	case "reject":
		dm.reject(ev, r, req.ID)
	case "list":
		dm.list(ev, r.UID)
	case "status":
		dm.status(ev)
	}
}

func (dm *Daemon) journal(kind, id string, h [32]byte, detail map[string]string) error {
	return dm.d.Journal.Append(kind, id, h, detail)
}

func quoteArgv(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = strconv.Quote(a)
	}
	return strings.Join(q, " ")
}

func (dm *Daemon) run(c *net.UnixConn, ev *events, r card.Requester, req wire.Request) {
	var none [32]byte
	if err := dm.journal("request", "", none, map[string]string{
		"uid": strconv.Itoa(int(r.UID)), "pid": strconv.Itoa(r.PID), "comm": r.Comm,
		"cwd": r.CWD, "argv": quoteArgv(req.Argv), "why": req.Why,
	}); err != nil {
		ev.send(wire.Event{Ev: "error", Reason: "journal unavailable, request refused: " + err.Error()})
		return
	}
	refuse := func(id string, h [32]byte, reason string) {
		if err := dm.journal("refused", id, h, map[string]string{"reason": reason}); err != nil {
			ev.send(wire.Event{Ev: "error", Reason: "journal unavailable: " + err.Error()})
			return
		}
		ev.refuse("refuse", reason)
	}

	rs, err := dm.d.Rules()
	if err != nil {
		refuse("", none, "the rules file cannot be read, so nothing runs: "+err.Error())
		return
	}
	path, sha, err := dm.d.Resolve(req.Argv[0], dm.cfg.RootPATH)
	if err != nil {
		refuse("", none, err.Error())
		return
	}
	verdict, reason := rules.Decide(rs, path, req.Argv)
	id := dm.d.Store.NewID()
	cd := card.New(id, r, path, sha, req.Argv, req.Why, rules.Risks(path, req.Argv), dm.d.Clock(), dm.cfg.Limits.TTL)
	text, err := card.Render(cd, dm.warnings(), dm.cfg.MaxCardRunes)
	if err != nil {
		refuse(id, cd.Hash, err.Error())
		return
	}

	switch verdict {
	case rules.Deny:
		if err := dm.journal("denied", id, cd.Hash, map[string]string{"reason": reason}); err != nil {
			ev.send(wire.Event{Ev: "error", Reason: "journal unavailable: " + err.Error()})
			return
		}
		ev.refuse("deny", reason)
		return
	case rules.Allow:
		if err := dm.journal("allowed", id, cd.Hash, map[string]string{"reason": reason}); err != nil {
			ev.send(wire.Event{Ev: "error", Reason: "journal unavailable: " + err.Error()})
			return
		}
		ev.send(wire.Event{Ev: "decided", ID: id, Verdict: "allow", Reason: reason})
		dm.execute(ev, cd, false)
		return
	}

	// The polkit dialog has little room: it gets the dialog card. A card it
	// cannot show whole is refused, so no dialog can approve a command it
	// does not show.
	dialog, err := card.RenderDialog(cd, dm.warnings(), dm.cfg.MaxCardRunes)
	if err != nil {
		refuse(id, cd.Hash, err.Error()+": ask again with shorter or fewer arguments (paths relative to the folder) or a shorter reason")
		return
	}
	if err := dm.d.Store.Hold(cd); err != nil {
		refuse(id, cd.Hash, err.Error())
		return
	}
	logged := make(chan error, 1)
	dm.loggedMu.Lock()
	dm.logged[id] = logged
	dm.loggedMu.Unlock()
	defer func() {
		dm.loggedMu.Lock()
		delete(dm.logged, id)
		dm.loggedMu.Unlock()
	}()
	if err := dm.journal("held", id, cd.Hash, nil); err != nil {
		dm.d.Store.Withdraw(id)
		ev.send(wire.Event{Ev: "error", Reason: "journal unavailable: " + err.Error()})
		return
	}
	ev.send(wire.Event{Ev: "held", ID: id, Card: text})

	ctx, cancel := context.WithDeadline(context.Background(), cd.Expires)
	defer cancel()
	go func() { // the requester leaving withdraws its card
		c.Read(make([]byte, 1))
		dm.d.Store.Withdraw(id)
	}()
	if dm.cfg.DeskDialog {
		go func() { // the desk
			ok, err := dm.d.Polkit.Check(ctx, r, dm.lift(dialog))
			switch {
			case ok:
				dm.d.Store.Decide(id, cd.Hash, true, "polkit")
			case errors.Is(err, polkit.ErrNoAgent):
				// Nobody was asked: the card waits for the review or the phone.
				dm.journal("no_dialog", id, cd.Hash, nil)
				ev.send(wire.Event{Ev: "note", ID: id, Reason: "no authentication dialog can open for the asking process: approve it from onceroot review"})
			case dm.d.Phone == nil && ctx.Err() == nil:
				dm.d.Store.Decide(id, cd.Hash, false, "polkit")
			}
		}()
	}
	if dm.d.Phone != nil {
		rec, _ := dm.d.Store.Get(id)
		if ch, err := dm.d.Phone.Offer(ctx, rec, text); err == nil {
			go func() {
				select {
				case d := <-ch:
					dm.d.Store.Decide(id, cd.Hash, d.Approve, d.By)
				case <-ctx.Done():
				}
			}()
		}
		defer dm.d.Phone.Withdraw(id)
	}
	expiry := time.AfterFunc(time.Until(cd.Expires), func() { dm.d.Store.Sweep() })
	defer expiry.Stop()

	for {
		changed := dm.d.Store.Changed(id)
		rec, ok := dm.d.Store.Get(id)
		if !ok || rec.Status != card.Held {
			break
		}
		<-changed
	}
	cancel()
	rec, _ := dm.d.Store.Get(id)
	switch rec.Status {
	case card.Approved:
		err := dm.journal("approved", id, cd.Hash, map[string]string{"by": rec.By})
		logged <- err
		if err != nil {
			dm.d.Store.Withdraw(id)
			ev.send(wire.Event{Ev: "error", Reason: "journal unavailable: " + err.Error()})
			return
		}
		ev.send(wire.Event{Ev: "decided", ID: id, Verdict: "approve", Reason: "approved by " + rec.By})
		dm.execute(ev, cd, true)
	case card.Rejected:
		err := dm.journal("rejected", id, cd.Hash, map[string]string{"by": rec.By})
		logged <- err
		if err != nil {
			ev.send(wire.Event{Ev: "error", Reason: "journal unavailable: " + err.Error()})
			return
		}
		ev.refuse("reject", "rejected by "+rec.By)
	case card.Expired:
		if err := dm.journal("expired", id, cd.Hash, nil); err != nil {
			ev.send(wire.Event{Ev: "error", Reason: "journal unavailable: " + err.Error()})
			return
		}
		ev.refuse("expire", "nobody answered before the card expired")
	case card.Withdrawn:
		dm.journal("withdrawn", id, cd.Hash, nil)
	}
}

// execute re-checks, marks the card ran (when it came through the store),
// runs it and journals the result before the exit reaches the client.
func (dm *Daemon) execute(ev *events, cd card.Card, viaStore bool) {
	fail := func(reason string) {
		if err := dm.journal("refused", cd.ID, cd.Hash, map[string]string{"reason": reason}); err != nil {
			reason += "; journal unavailable: " + err.Error()
		}
		ev.send(wire.Event{Ev: "error", Reason: reason})
	}
	if err := dm.d.StillSame(cd.Req); err != nil {
		fail(err.Error())
		return
	}
	if err := dm.d.Unchanged(cd.Path, cd.BinarySHA256); err != nil {
		fail(err.Error())
		return
	}
	if viaStore {
		if err := dm.d.Store.MarkRan(cd.ID); err != nil {
			fail(err.Error())
			return
		}
	}
	code, err := dm.d.Run(context.Background(), cd.Path, cd.Argv, cd.Req.CWD, dm.cfg.CommandLimit, func(fd int, p []byte) {
		ev.send(wire.Event{Ev: "out", FD: fd, Data: p})
	})
	detail := map[string]string{"exit": strconv.Itoa(code)}
	if err != nil {
		detail["error"] = err.Error()
	}
	if jerr := dm.journal("ran", cd.ID, cd.Hash, detail); jerr != nil {
		ev.send(wire.Event{Ev: "error", Reason: "the command ran, but the journal failed: " + jerr.Error()})
		return
	}
	if err != nil {
		ev.send(wire.Event{Ev: "error", Reason: err.Error()})
		return
	}
	ev.send(wire.Event{Ev: "exit", ID: cd.ID, Code: code})
}

// approve asks polkit about the approver's own process; the socket itself
// never approves (I2). A user approves only their own requests.
func (dm *Daemon) approve(ev *events, r card.Requester, id string) {
	rec, ok := dm.d.Store.Get(id)
	if !ok || rec.Status != card.Held || rec.Card.Req.UID != r.UID {
		ev.send(wire.Event{Ev: "error", Reason: "no held request " + id})
		return
	}
	text, err := card.RenderDialog(rec.Card, dm.warnings(), dm.cfg.MaxCardRunes)
	if err != nil {
		ev.send(wire.Event{Ev: "error", Reason: err.Error()})
		return
	}
	ctx, cancel := context.WithDeadline(context.Background(), rec.Card.Expires)
	defer cancel()
	approved, err := dm.d.Polkit.Check(ctx, r, dm.lift(text))
	if !approved {
		reason := "not approved"
		if err != nil {
			reason += ": " + err.Error()
		}
		// A dismissed or failed dialog is the person's no, and one dialog per
		// card is all anyone gets. A check that asked nobody is not a no.
		if !errors.Is(err, polkit.ErrNoAgent) && ctx.Err() == nil {
			dm.d.Store.Decide(id, rec.Card.Hash, false, "polkit:"+r.Comm)
			reason += "; the request is rejected"
		}
		ev.send(wire.Event{Ev: "error", Reason: reason})
		return
	}
	logged := dm.decisionLog(id)
	if err := dm.d.Store.Decide(id, rec.Card.Hash, true, "polkit:"+r.Comm); err != nil {
		ev.send(wire.Event{Ev: "error", Reason: err.Error()})
		return
	}
	if err := awaitLogged(logged, rec.Card.Expires); err != nil {
		ev.send(wire.Event{Ev: "error", Reason: "approved, but the journal did not take it, so nothing runs: " + err.Error()})
		return
	}
	ev.send(wire.Event{Ev: "decided", ID: id, Verdict: "approve"})
}

// reject needs no authentication: it can only take root away. A user
// rejects only their own requests.
func (dm *Daemon) reject(ev *events, r card.Requester, id string) {
	rec, ok := dm.d.Store.Get(id)
	if !ok || rec.Card.Req.UID != r.UID {
		ev.send(wire.Event{Ev: "error", Reason: "no request " + id})
		return
	}
	logged := dm.decisionLog(id)
	if err := dm.d.Store.Decide(id, rec.Card.Hash, false, "cli:"+r.Comm); err != nil {
		ev.send(wire.Event{Ev: "error", Reason: err.Error()})
		return
	}
	if err := awaitLogged(logged, rec.Card.Expires); err != nil {
		ev.send(wire.Event{Ev: "error", Reason: "rejected, but the journal did not take it: " + err.Error()})
		return
	}
	ev.send(wire.Event{Ev: "decided", ID: id, Verdict: "reject"})
}

// list shows the caller's own requests only.
func (dm *Daemon) list(ev *events, uid uint32) {
	var out []wire.Listed
	for _, rec := range dm.d.Store.All() {
		if rec.Card.Req.UID != uid {
			continue
		}
		text, err := card.Render(rec.Card, nil, dm.cfg.MaxCardRunes)
		if err != nil {
			text = "(card too long to show)"
		}
		out = append(out, wire.Listed{ID: rec.Card.ID, Status: rec.Status.String(), Card: text})
	}
	slices.SortFunc(out, func(a, b wire.Listed) int { return strings.Compare(a.ID, b.ID) })
	ev.send(wire.Event{Ev: "list", Cards: out})
}

func (dm *Daemon) status(ev *events) {
	sudo, phone := "off", "off"
	if dm.passwordless() {
		sudo = "on"
	}
	if dm.d.Phone != nil {
		phone = "on"
	}
	open := 0
	for _, rec := range dm.d.Store.All() {
		if rec.Status == card.Held || rec.Status == card.Approved {
			open++
		}
	}
	ev.send(wire.Event{Ev: "status", Status: []string{
		"passwordless sudo: " + sudo,
		"phone approval: " + phone,
		"open requests: " + strconv.Itoa(open),
	}})
}

// lift adds as many empty lines as the text has when DialogLift is set.
func (dm *Daemon) lift(text string) string {
	if !dm.cfg.DialogLift {
		return text
	}
	return text + strings.Repeat("\n", strings.Count(text, "\n")+1)
}

func (dm *Daemon) warnings() []string {
	if dm.passwordless() {
		return []string{"passwordless sudo is on: any process of this user can go around onceroot"}
	}
	return nil
}

// passwordless reports an active Omarchy passwordless sudo grant.
func (dm *Daemon) passwordless() bool {
	m, _ := filepath.Glob(filepath.Join(dm.cfg.SudoersDir, "99-omarchy-*nopasswd*"))
	return len(m) > 0
}
