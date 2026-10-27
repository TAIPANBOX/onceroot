// Package phone is onceroot's optional Telegram module: it offers a held card
// to one private chat, takes Reject at once and Approve only with the
// owner's PIN and a TOTP code, and never echoes either.
//
// The API client is ported from rollcall internal/channel/telegram.go and
// guard.go (MIT, Yurii Kostiuk): plain text, no parse_mode, link previews
// off, a client that dials only the Telegram host, and errors that never
// carry the token.
package phone

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BaseURL is the production API root.
const BaseURL = "https://api.telegram.org"

const maxAnswer = 1 << 20

// GuardedClient dials only the given hosts, takes no proxy from the
// environment and follows no redirect.
func GuardedClient(hosts ...string) *http.Client {
	allowed := map[string]bool{}
	for _, h := range hosts {
		allowed[strings.ToLower(h)] = true
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{
		Timeout: 70 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					host = addr
				}
				if !allowed[strings.ToLower(host)] {
					return nil, fmt.Errorf("phone: host %s is not allowed", host)
				}
				return dialer.DialContext(ctx, network, addr)
			},
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("phone: redirects are not followed") },
	}
}

// Bot calls the Bot API for one chat.
type Bot struct {
	base  string
	token string
	chat  int64
	hc    *http.Client
}

// NewBot returns a client for chat. A nil hc uses GuardedClient for base's host.
func NewBot(base, token string, chat int64, hc *http.Client) *Bot {
	if hc == nil {
		u, _ := url.Parse(base)
		hc = GuardedClient(u.Hostname())
	}
	return &Bot{base: strings.TrimSuffix(base, "/"), token: token, chat: chat, hc: hc}
}

// String never prints the token.
func (b Bot) String() string { return fmt.Sprintf("phone.Bot{chat: %d, token: <redacted>}", b.chat) }

// Format prints String for every verb, so no verb reaches the token.
func (b Bot) Format(f fmt.State, _ rune) { io.WriteString(f, b.String()) }

func (b *Bot) redact(s string) string {
	for _, t := range []string{b.token, url.PathEscape(b.token), url.QueryEscape(b.token)} {
		if t != "" {
			s = strings.ReplaceAll(s, t, "<redacted>")
		}
	}
	return s
}

func (b *Bot) call(ctx context.Context, method string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+"/bot"+url.PathEscape(b.token)+"/"+method, bytes.NewReader(raw))
	if err != nil {
		return errors.New(b.redact(fmt.Sprintf("telegram %s: %v", method, err)))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.hc.Do(req)
	if err != nil {
		return errors.New(b.redact(fmt.Sprintf("telegram %s: %v", method, err)))
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
	if err != nil {
		return errors.New(b.redact(fmt.Sprintf("telegram %s: %v", method, err)))
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram %s: HTTP %d", method, resp.StatusCode)
	}
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(answer, &env) != nil || !env.OK {
		return fmt.Errorf("telegram %s: the answer is not ok", method)
	}
	if out != nil && json.Unmarshal(env.Result, out) != nil {
		return fmt.Errorf("telegram %s: unexpected result", method)
	}
	return nil
}

// Button is one inline keyboard button.
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

type linkPreview struct {
	IsDisabled bool `json:"is_disabled"`
}

// send has no parse_mode field on purpose: every message is plain text.
type send struct {
	ChatID      int64       `json:"chat_id"`
	Text        string      `json:"text"`
	LinkPreview linkPreview `json:"link_preview_options"`
	ReplyMarkup *keyboard   `json:"reply_markup,omitempty"`
}

type keyboard struct {
	Inline [][]Button `json:"inline_keyboard"`
}

// Send posts plain text, optionally with one row of buttons.
func (b *Bot) Send(ctx context.Context, text string, buttons []Button) (int64, error) {
	body := send{ChatID: b.chat, Text: text, LinkPreview: linkPreview{IsDisabled: true}}
	if len(buttons) > 0 {
		body.ReplyMarkup = &keyboard{Inline: [][]Button{buttons}}
	}
	var msg struct {
		ID int64 `json:"message_id"`
	}
	err := b.call(ctx, "sendMessage", body, &msg)
	return msg.ID, err
}

// Edit replaces a message's text and drops its buttons.
func (b *Bot) Edit(ctx context.Context, id int64, text string) error {
	return b.call(ctx, "editMessageText", map[string]any{"chat_id": b.chat, "message_id": id, "text": text,
		"link_preview_options": linkPreview{IsDisabled: true}}, nil)
}

// Delete removes a message.
func (b *Bot) Delete(ctx context.Context, id int64) error {
	return b.call(ctx, "deleteMessage", map[string]any{"chat_id": b.chat, "message_id": id}, nil)
}

// AnswerCallback stops a button's spinner.
func (b *Bot) AnswerCallback(ctx context.Context, id, text string) error {
	return b.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, nil)
}

// Chat is the chat of a message.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Message is the part of a message onceroot reads.
type Message struct {
	ID   int64  `json:"message_id"`
	Text string `json:"text"`
	Chat Chat   `json:"chat"`
}

// Callback is a pressed button.
type Callback struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
}

// Update is one update.
type Update struct {
	ID       int64     `json:"update_id"`
	Message  *Message  `json:"message"`
	Callback *Callback `json:"callback_query"`
}

// Updates long-polls for updates after offset.
func (b *Bot) Updates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	var ups []Update
	err := b.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": timeout,
		"allowed_updates": []string{"message", "callback_query"}}, &ups)
	return ups, err
}
