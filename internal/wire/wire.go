// Package wire is the protocol on onceroot's socket: one JSON object per
// line each way. The client sends one Request; the daemon answers with a
// stream of Events. Decode bounds every field, refuses unknown fields and
// refuses anything after the one object on the line.
package wire

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Bounds on a request.
const (
	MaxLine  = 64 * 1024
	MaxEvent = 1 << 20
	MaxArgs  = 256
	MaxArg   = 4096
	MaxWhy   = 500 // runes
)

// Request is what a client asks.
type Request struct {
	Op   string   `json:"op"`
	Argv []string `json:"argv,omitempty"`
	Why  string   `json:"why,omitempty"`
	ID   string   `json:"id,omitempty"`
}

// Event is one line of the daemon's answer. Ev is one of: held (Card holds
// the rendered card), note (Reason, for the person reading the agent's
// terminal), decided (Verdict allow|deny|approve|reject|expire and
// Reason), out (FD 1 or 2, Data), exit (Code), list (Cards), status
// (Status), error (Reason).
type Event struct {
	Ev      string   `json:"ev"`
	ID      string   `json:"id,omitempty"`
	Card    string   `json:"card,omitempty"`
	Verdict string   `json:"verdict,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	FD      int      `json:"fd,omitempty"`
	Data    []byte   `json:"data,omitempty"`
	Code    int      `json:"code,omitempty"`
	Cards   []Listed `json:"cards,omitempty"`
	Status  []string `json:"status,omitempty"`
}

// Listed is one card in a list answer.
type Listed struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Card   string `json:"card"`
}

// ErrTooLong is a line over MaxLine.
var ErrTooLong = errors.New("wire: line too long")

// idAlphabet matches store's: A-Z without I and O.
const idAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ"

// ValidID reports a five-letter card id.
func ValidID(id string) bool {
	return len(id) == 5 && strings.Trim(id, idAlphabet) == ""
}

// Decode reads and checks one request.
func Decode(r *bufio.Reader) (Request, error) {
	line, err := readLine(r, MaxLine)
	if err != nil {
		return Request{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	var req Request
	if err := dec.Decode(&req); err != nil {
		return Request{}, fmt.Errorf("wire: %w", err)
	}
	if dec.More() {
		return Request{}, errors.New("wire: more than one object on the line")
	}
	if rest, _ := io.ReadAll(dec.Buffered()); len(bytes.TrimSpace(rest)) > 0 {
		return Request{}, errors.New("wire: data after the object")
	}
	return req, check(req)
}

func check(r Request) error {
	switch r.Op {
	case "run":
		if r.ID != "" {
			return errors.New("wire: run carries no id")
		}
		if len(r.Argv) == 0 || len(r.Argv) > MaxArgs {
			return fmt.Errorf("wire: argv must hold 1 to %d items", MaxArgs)
		}
		if r.Argv[0] == "" {
			return errors.New("wire: empty program name")
		}
		for _, a := range r.Argv {
			if len(a) > MaxArg {
				return fmt.Errorf("wire: an argument is longer than %d bytes", MaxArg)
			}
			if strings.IndexByte(a, 0) >= 0 {
				return errors.New("wire: NUL byte in an argument")
			}
		}
		if !utf8.ValidString(r.Why) || utf8.RuneCountInString(r.Why) > MaxWhy {
			return fmt.Errorf("wire: why must be valid UTF-8, at most %d characters", MaxWhy)
		}
	case "list", "status":
		if r.Argv != nil || r.ID != "" || r.Why != "" {
			return fmt.Errorf("wire: %s takes no fields", r.Op)
		}
	case "approve", "reject":
		if r.Argv != nil || r.Why != "" || !ValidID(r.ID) {
			return fmt.Errorf("wire: %s takes one valid id", r.Op)
		}
	default:
		return fmt.Errorf("wire: unknown op %q", r.Op)
	}
	return nil
}

// readLine reads up to and without the newline, at most max bytes. A
// final line without a newline is accepted.
func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := r.ReadSlice('\n')
		out = append(out, chunk...)
		if len(out) > max+1 {
			return nil, ErrTooLong
		}
		switch {
		case err == nil:
			return bytes.TrimSuffix(out, []byte("\n")), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(out) > 0:
			return out, nil
		default:
			return nil, err
		}
	}
}

// WriteEvent writes one event as one line.
func WriteEvent(w io.Writer, e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// WriteRequest writes one request as one line.
func WriteRequest(w io.Writer, r Request) error {
	if err := check(r); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// ReadEvent reads one event line. Events come from the daemon, so they are
// bounded but not otherwise distrusted.
func ReadEvent(r *bufio.Reader) (Event, error) {
	line, err := readLine(r, MaxEvent)
	if err != nil {
		return Event{}, err
	}
	var e Event
	if err := json.Unmarshal(line, &e); err != nil {
		return Event{}, fmt.Errorf("wire: %w", err)
	}
	return e, nil
}
