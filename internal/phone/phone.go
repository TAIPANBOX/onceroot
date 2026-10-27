package phone

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/card"
	"github.com/TAIPANBOX/onceroot/internal/daemon"
	"github.com/TAIPANBOX/onceroot/internal/factor"
)

// promptFor is how long the phone waits for PIN and code after Approve.
const promptFor = 3 * time.Minute

type offer struct {
	rec   card.Record
	msgID int64
	ch    chan daemon.Decision
}

// Phone implements daemon.Phone over Telegram.
type Phone struct {
	bot         *Bot
	f           *factor.Factors
	clock       func() time.Time
	pollEvery   time.Duration
	pollTimeout int

	mu         sync.Mutex
	offers     map[string]*offer
	awaiting   string
	awaitUntil time.Time
	offset     int64
}

var _ daemon.Phone = (*Phone)(nil)

// New returns a phone module; Run must be started for it to hear answers.
func New(bot *Bot, f *factor.Factors, clock func() time.Time) *Phone {
	return &Phone{bot: bot, f: f, clock: clock, pollEvery: time.Second, pollTimeout: 25, offers: map[string]*offer{}}
}

func hash8(rec card.Record) string { return hex.EncodeToString(rec.Card.Hash[:8]) }

// Offer sends the card with Approve and Reject buttons.
func (p *Phone) Offer(ctx context.Context, rec card.Record, text string) (<-chan daemon.Decision, error) {
	id := rec.Card.ID
	msgID, err := p.bot.Send(ctx, text+"\n\nApprove asks for your PIN and code. Reject takes nothing.", []Button{
		{Text: "Approve " + id, Data: "a:" + id + ":" + hash8(rec)},
		{Text: "Reject " + id, Data: "r:" + id + ":" + hash8(rec)},
	})
	if err != nil {
		return nil, err
	}
	o := &offer{rec: rec, msgID: msgID, ch: make(chan daemon.Decision, 1)}
	p.mu.Lock()
	p.offers[id] = o
	p.mu.Unlock()
	return o.ch, nil
}

// Withdraw ends the phone's part in a card decided elsewhere or expired.
func (p *Phone) Withdraw(id string) {
	p.mu.Lock()
	o, ok := p.offers[id]
	delete(p.offers, id)
	if p.awaiting == id {
		p.awaiting = ""
	}
	p.mu.Unlock()
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p.bot.Edit(ctx, o.msgID, "Request "+id+" is closed: decided at the desk, expired or withdrawn.")
	}
}

// Run polls for answers until ctx ends.
func (p *Phone) Run(ctx context.Context) {
	for ctx.Err() == nil {
		ups, err := p.bot.Updates(ctx, p.offset, p.pollTimeout)
		for _, u := range ups {
			if u.ID >= p.offset {
				p.offset = u.ID + 1
			}
			p.handle(ctx, u)
		}
		if err != nil || len(ups) == 0 {
			select {
			case <-ctx.Done():
			case <-time.After(p.pollEvery):
			}
		}
	}
}

func (p *Phone) ours(m *Message) bool {
	return m != nil && m.Chat.ID == p.bot.chat && m.Chat.Type == "private"
}

func (p *Phone) handle(ctx context.Context, u Update) {
	switch {
	case u.Callback != nil && p.ours(u.Callback.Message):
		p.button(ctx, u.Callback)
	case u.Message != nil && p.ours(u.Message):
		p.secret(ctx, u.Message)
	}
}

func (p *Phone) button(ctx context.Context, cb *Callback) {
	parts := strings.Split(cb.Data, ":")
	p.mu.Lock()
	var o *offer
	if len(parts) == 3 {
		o = p.offers[parts[1]]
	}
	if o == nil || parts[2] != hash8(o.rec) {
		p.mu.Unlock()
		p.bot.AnswerCallback(ctx, cb.ID, "This request is closed.")
		return
	}
	switch parts[0] {
	case "r":
		p.decideLocked(o, false)
		p.mu.Unlock()
		p.bot.AnswerCallback(ctx, cb.ID, "Rejected.")
		p.bot.Edit(ctx, o.msgID, "Rejected "+o.rec.Card.ID+".")
	case "a":
		p.awaiting = o.rec.Card.ID
		p.awaitUntil = p.clock().Add(promptFor)
		if p.awaitUntil.After(o.rec.Card.Expires) {
			p.awaitUntil = o.rec.Card.Expires
		}
		p.mu.Unlock()
		p.bot.AnswerCallback(ctx, cb.ID, "")
		p.bot.Send(ctx, "Send your PIN and the code from your authenticator for "+o.rec.Card.ID+" as one message: PIN CODE", nil)
	default:
		p.mu.Unlock()
	}
}

// secret handles the message that should carry PIN and code. It is deleted
// before anything else happens, and its text is never repeated.
func (p *Phone) secret(ctx context.Context, m *Message) {
	p.mu.Lock()
	id := p.awaiting
	p.mu.Unlock()
	if id == "" {
		return
	}
	p.bot.Delete(ctx, m.ID)

	now := p.clock()
	p.mu.Lock()
	o := p.offers[id]
	late := now.After(p.awaitUntil)
	if o == nil || late {
		p.awaiting = ""
		p.mu.Unlock()
		p.bot.Send(ctx, "Too late for "+id+"; press Approve again if it is still open.", nil)
		return
	}
	p.mu.Unlock()

	fields := strings.Fields(m.Text)
	if len(fields) != 2 {
		p.bot.Send(ctx, "Send exactly two things: PIN CODE.", nil)
		return
	}
	err := p.f.Check(fields[0], fields[1], now)
	switch {
	case err == nil:
		p.mu.Lock()
		p.decideLocked(o, true)
		p.mu.Unlock()
		p.bot.Edit(ctx, o.msgID, "Approved "+id+".")
	case errors.Is(err, factor.ErrLockedOut):
		p.mu.Lock()
		p.awaiting = ""
		p.mu.Unlock()
		p.bot.Send(ctx, "Too many wrong tries; approval from the phone is locked for a while.", nil)
	default:
		p.bot.Send(ctx, "Wrong PIN or code.", nil)
	}
}

func (p *Phone) decideLocked(o *offer, approve bool) {
	if _, open := p.offers[o.rec.Card.ID]; !open {
		return
	}
	delete(p.offers, o.rec.Card.ID)
	if p.awaiting == o.rec.Card.ID {
		p.awaiting = ""
	}
	o.ch <- daemon.Decision{Approve: approve, By: "phone"}
}
