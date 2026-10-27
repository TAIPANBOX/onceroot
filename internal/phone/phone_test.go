package phone

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/card"
	"github.com/TAIPANBOX/onceroot/internal/daemon"
	"github.com/TAIPANBOX/onceroot/internal/factor"
)

const token = "123456:SECRET-CANARY-token"
const chat int64 = 4242

var totpSecret = []byte("12345678901234567890")

type call struct {
	method string
	body   map[string]any
}

type fakeTG struct {
	mu      sync.Mutex
	calls   []call
	updates []map[string]any
	nextMsg int64
	fail    bool
}

func (f *fakeTG) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/bot"+token+"/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(b, &body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail {
			w.WriteHeader(500)
			fmt.Fprintf(w, `{"ok":false,"description":"boom %s"}`, token)
			return
		}
		f.calls = append(f.calls, call{method, body})
		switch method {
		case "getUpdates":
			ups := f.updates
			f.updates = nil
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": ups})
		case "sendMessage":
			f.nextMsg++
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": f.nextMsg}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		}
	})
}

func (f *fakeTG) push(u map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u["update_id"] = float64(len(f.calls) + len(f.updates) + 1000)
	f.updates = append(f.updates, u)
}

func (f *fakeTG) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var m []string
	for _, c := range f.calls {
		if c.method != "getUpdates" {
			m = append(m, c.method)
		}
	}
	return m
}

func (f *fakeTG) sent() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if c.method == "sendMessage" {
			out = append(out, c)
		}
	}
	return out
}

func callback(fromChat int64, data string) map[string]any {
	return map[string]any{"callback_query": map[string]any{"id": "cb1", "data": data,
		"message": map[string]any{"message_id": 1, "chat": map[string]any{"id": float64(fromChat), "type": "private"}}}}
}

func message(fromChat int64, id int64, text string) map[string]any {
	return map[string]any{"message": map[string]any{"message_id": float64(id), "text": text,
		"chat": map[string]any{"id": float64(fromChat), "type": "private"}}}
}

type rig struct {
	tg    *fakeTG
	p     *Phone
	now   time.Time
	cards int
}

func newRig(t *testing.T) *rig {
	t.Helper()
	tg := &fakeTG{}
	srv := httptest.NewServer(tg.handler(t))
	t.Cleanup(srv.Close)
	tp, err := factor.NewTOTP(totpSecret, filepath.Join(t.TempDir(), "used"))
	if err != nil {
		t.Fatal(err)
	}
	salt := []byte("0123456789abcdef")
	f := &factor.Factors{PINSalt: salt, PINHash: factor.HashPIN("4711", salt), TOTP: tp, Lock: factor.NewLockout(5, 15*time.Minute)}
	r := &rig{tg: tg, now: time.Unix(1_000_000, 0)}
	r.p = New(NewBot(srv.URL, token, chat, srv.Client()), f, func() time.Time { return r.now })
	r.p.pollEvery = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go r.p.Run(ctx)
	return r
}

func (r *rig) offer(t *testing.T) (card.Record, <-chan daemon.Decision) {
	t.Helper()
	r.cards++
	req := card.Requester{UID: 1000, PID: 1, StartTime: 1, Comm: "claude", CWD: "/w"}
	c := card.New(fmt.Sprintf("KQWP%c", 'A'+r.cards), req, "/usr/bin/pacman", [32]byte{1}, []string{"pacman", "-S", "x"}, "", nil, r.now, 10*time.Minute)
	rec := card.Record{Card: c, Status: card.Held}
	ch, err := r.p.Offer(context.Background(), rec, "ROOT REQUEST "+c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return rec, ch
}

func data(verb string, rec card.Record) string {
	return verb + ":" + rec.Card.ID + ":" + hex.EncodeToString(rec.Card.Hash[:8])
}

func code(now time.Time) string { return factor.CodeAt(totpSecret, uint64(now.Unix()/30)) }

func expect(t *testing.T, ch <-chan daemon.Decision, approve bool) {
	t.Helper()
	select {
	case d := <-ch:
		if d.Approve != approve {
			t.Fatalf("decision approve=%v, want %v", d.Approve, approve)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no decision")
	}
}

func expectNone(t *testing.T, ch <-chan daemon.Decision) {
	t.Helper()
	select {
	case d := <-ch:
		t.Fatalf("unexpected decision %+v", d)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestNoParseMode(t *testing.T) {
	r := newRig(t)
	r.offer(t)
	s := r.tg.sent()
	if len(s) != 1 {
		t.Fatalf("%d messages sent", len(s))
	}
	if _, ok := s[0].body["parse_mode"]; ok {
		t.Fatal("parse_mode set")
	}
	lp, _ := s[0].body["link_preview_options"].(map[string]any)
	if lp["is_disabled"] != true {
		t.Fatalf("link previews not disabled: %v", s[0].body)
	}
	if s[0].body["chat_id"] != float64(chat) {
		t.Fatalf("sent to %v", s[0].body["chat_id"])
	}
}

func TestApproveNeedsPINAndCode(t *testing.T) {
	r := newRig(t)
	rec, ch := r.offer(t)
	r.tg.push(callback(chat, data("a", rec)))
	expectNone(t, ch)
	r.tg.push(message(chat, 77, "4711 "+code(r.now)))
	expect(t, ch, true)
}

func TestWrongFactorsDoNotApprove(t *testing.T) {
	r := newRig(t)
	rec, ch := r.offer(t)
	r.tg.push(callback(chat, data("a", rec)))
	r.tg.push(message(chat, 77, "0000 "+code(r.now)))
	expectNone(t, ch)
	r.tg.push(message(chat, 78, "4711"))
	expectNone(t, ch)
}

func TestRejectNeedsNoFactors(t *testing.T) {
	r := newRig(t)
	rec, ch := r.offer(t)
	r.tg.push(callback(chat, data("r", rec)))
	expect(t, ch, false)
}

func TestOnlyConfiguredChatIsRead(t *testing.T) {
	r := newRig(t)
	rec, ch := r.offer(t)
	r.tg.push(callback(chat+1, data("r", rec)))
	r.tg.push(callback(chat+1, data("a", rec)))
	r.tg.push(message(chat+1, 5, "4711 "+code(r.now)))
	expectNone(t, ch)
	for _, c := range r.tg.sent()[1:] {
		if c.body["chat_id"] != float64(chat) {
			t.Fatalf("answered another chat: %v", c.body)
		}
	}
}

func TestStaleHashRefused(t *testing.T) {
	r := newRig(t)
	rec, ch := r.offer(t)
	r.tg.push(callback(chat, "r:"+rec.Card.ID+":0000000000000000"))
	expectNone(t, ch)
}

func TestSecretMessageDeletedFirst(t *testing.T) {
	r := newRig(t)
	rec, ch := r.offer(t)
	r.tg.push(callback(chat, data("a", rec)))
	time.Sleep(50 * time.Millisecond)
	before := len(r.tg.methods())
	r.tg.push(message(chat, 77, "4711 "+code(r.now)))
	expect(t, ch, true)
	after := r.tg.methods()[before:]
	if len(after) == 0 || after[0] != "deleteMessage" {
		t.Fatalf("calls after the secret: %v; deleteMessage must come first", after)
	}
}

func TestCodeReplayRefused(t *testing.T) {
	r := newRig(t)
	rec1, ch1 := r.offer(t)
	r.tg.push(callback(chat, data("a", rec1)))
	c := code(r.now)
	r.tg.push(message(chat, 77, "4711 "+c))
	expect(t, ch1, true)
	rec2, ch2 := r.offer(t)
	r.tg.push(callback(chat, data("a", rec2)))
	r.tg.push(message(chat, 78, "4711 "+c))
	expectNone(t, ch2)
}

func TestDeskDecisionWithdrawsPhoneCard(t *testing.T) {
	r := newRig(t)
	rec, ch := r.offer(t)
	r.p.Withdraw(rec.Card.ID)
	r.tg.push(callback(chat, data("r", rec)))
	expectNone(t, ch)
	if m := r.tg.methods(); m[len(m)-1] != "editMessageText" && !contains(m, "editMessageText") {
		t.Fatalf("the phone card was not updated: %v", m)
	}
}

func TestTokenNeverInErrors(t *testing.T) {
	r := newRig(t)
	r.tg.mu.Lock()
	r.tg.fail = true
	r.tg.mu.Unlock()
	_, err := r.p.Offer(context.Background(), card.Record{Card: card.Card{ID: "KQWPZ"}}, "x")
	if err == nil {
		t.Fatal("a failing API reported no error")
	}
	if strings.Contains(err.Error(), "SECRET-CANARY") || strings.Contains(fmt.Sprintf("%v %+v %#v", r.p.bot, r.p.bot, r.p.bot), "SECRET-CANARY") {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestBotRefusesOtherHosts(t *testing.T) {
	hc := GuardedClient("api.telegram.org")
	if _, err := hc.Get("https://example.com/"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("got %v, want a refusal to dial example.com", err)
	}
}

func TestLoadRefusesLooseFilePerms(t *testing.T) {
	dir := t.TempDir()
	tok, fac := filepath.Join(dir, "telegram.token"), filepath.Join(dir, "factors")
	if err := WriteFactors(fac, "471123", totpSecret); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(tok, []byte(token+"\n"), 0o600)
	if _, _, err := Load(tok, fac, filepath.Join(dir, "used")); err != nil {
		t.Fatalf("private files refused: %v", err)
	}
	os.Chmod(tok, 0o644)
	if _, _, err := Load(tok, fac, filepath.Join(dir, "used")); err == nil {
		t.Fatal("a token readable by others was accepted")
	}
	os.Chmod(tok, 0o600)
	os.Chmod(fac, 0o640)
	if _, _, err := Load(tok, fac, filepath.Join(dir, "used")); err == nil {
		t.Fatal("a factors file readable by the group was accepted")
	}
}

func TestFactorsFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fac := filepath.Join(dir, "factors")
	if err := WriteFactors(fac, "4711", totpSecret); err == nil {
		t.Fatal("a 4-digit PIN was written")
	}
	if err := WriteFactors(fac, "471123", totpSecret); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "tok"), []byte(token), 0o600)
	_, f, err := Load(filepath.Join(dir, "tok"), fac, filepath.Join(dir, "used"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_000_000, 0)
	if err := f.Check("471123", code(now), now); err != nil {
		t.Fatalf("written factors do not check: %v", err)
	}
	b, _ := os.ReadFile(fac)
	if strings.Contains(string(b), "471123") {
		t.Fatal("the PIN is stored in clear")
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestNewBotWithoutAClientIsGuarded(t *testing.T) {
	b := NewBot("https://api.telegram.org", token, chat, nil)
	b.base = "https://example.com"
	if _, err := b.Send(context.Background(), "x", nil); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("got %v, want a refusal to dial another host", err)
	}
}
