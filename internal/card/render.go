package card

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrTooLong refuses a card that does not fit, instead of cutting it: a cut
// card would show the person less than what runs.
var ErrTooLong = errors.New("card text is longer than the dialog can show")

// ErrDialogTooSmall refuses a card the approval dialog cannot show whole: a
// dialog that does not show the command must not be able to approve it.
var ErrDialogTooSmall = errors.New("the card is too long to show whole in the approval dialog")

// Render writes the card for a person, one fixed layout:
//
//	ROOT REQUEST <ID> (until HH:MM)
//	!! <warning>                      one line per warning
//	Run: "<path>"
//	  "<argv[0]>"                     one line per argument
//	In: "<cwd>"
//	Asked by: "<comm>" (pid N) <- "<parent>" <- ...
//	Risk: <labels> | none detected
//	The agent says: "<why>"
//
// Everything that came from outside onceroot (arguments, folder, process
// names, the agent's text) is Go-quoted, so control, format and bidi runes
// appear escaped and no outside text can start a line of its own.
func Render(c Card, warnings []string, maxRunes int) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "ROOT REQUEST %s (until %s)", c.ID, c.Expires.Format("15:04"))
	for _, w := range warnings {
		b.WriteString("\n!! " + w)
	}
	b.WriteString("\nRun: " + quote(c.Path))
	for _, a := range c.Argv {
		b.WriteString("\n  " + quote(a))
	}
	b.WriteString("\nIn: " + quote(c.Req.CWD))
	fmt.Fprintf(&b, "\nAsked by: %s (pid %d)", quote(c.Req.Comm), c.Req.PID)
	for _, p := range c.Req.Chain {
		b.WriteString(" <- " + quote(p))
	}
	risk := "none detected"
	if len(c.Risks) > 0 {
		risk = strings.Join(c.Risks, "; ")
	}
	b.WriteString("\nRisk: " + risk)
	b.WriteString("\nThe agent says: " + quote(c.Why))
	text := b.String()
	if utf8.RuneCountInString(text) > maxRunes {
		return "", ErrTooLong
	}
	return text, nil
}

// maxCompactChain is how many parents the dialog card names.
const maxCompactChain = 3

// agentMore starts each line the agent's words go on in. quote escapes the
// mark, so outside text never shows it, and a line of the agent's words
// cannot pass for one of the card's own.
const agentMore = "  » "

// DialogWidth and DialogLines bound the card a polkit dialog is given.
// Omarchy's dialog centres every line and lets a wide one run off both edges
// of the screen, so the program and the last arguments go unseen (Omarchy
// 4.0 in a VM, 2026-10-27). 96 runes fit a 700-pixel-wide screen in its font,
// and twelve lines fit above its password field.
const (
	DialogWidth = 96
	DialogLines = 12
)

// RenderDialog is the card for a polkit dialog: six lines plus warnings, at
// most three parent processes, and no line wider than DialogWidth runes.
// Arguments and the agent's words wrap onto lines indented by two spaces; an
// argument is never split inside its quotes. A card that does not fit in
// DialogLines lines that way is ErrDialogTooSmall, never shown in part.
func RenderDialog(c Card, warnings []string, maxRunes int) (string, error) {
	head := []string{fmt.Sprintf("ROOT REQUEST %s until %s", c.ID, c.Expires.Format("15:04"))}
	for _, w := range warnings {
		lines, _ := fill("!! ", "  ", strings.Fields(w))
		head = append(head, lines...)
	}
	run := []string{quote(c.Path)}
	for _, a := range c.Argv[min(1, len(c.Argv)):] {
		run = append(run, quote(a))
	}
	if len(c.Argv) > 0 && c.Argv[0] != filepath.Base(c.Path) {
		run = append(run, "(as "+quote(c.Argv[0])+")")
	}
	by := []string{quote(c.Req.Comm), fmt.Sprintf("(pid %d)", c.Req.PID)}
	for _, p := range c.Req.Chain[:min(maxCompactChain, len(c.Req.Chain))] {
		by = append(by, "<- "+quote(p))
	}
	risk := "none detected"
	if len(c.Risks) > 0 {
		risk = strings.Join(c.Risks, "; ")
	}
	lines, fits := head, true
	for _, field := range []struct {
		label, more string
		tokens      []string
	}{
		{"Run: ", "  ", run},
		{"In: ", "  ", []string{quote(c.Req.CWD)}},
		{"By: ", "  ", by},
		{"Risk: ", "  ", strings.Fields(risk)},
		{"Agent says: ", agentMore, strings.Split(quote(c.Why), " ")},
	} {
		more, ok := fill(field.label, field.more, field.tokens)
		lines, fits = append(lines, more...), fits && ok
	}
	text := strings.Join(lines, "\n")
	if utf8.RuneCountInString(text) > maxRunes {
		return "", ErrTooLong
	}
	if !fits || len(lines) > DialogLines {
		return "", ErrDialogTooSmall
	}
	return text, nil
}

// fill lays tokens out after label, as many to a line as fit DialogWidth,
// going on in lines that start with more. ok is false when a token alone is
// wider than a line.
func fill(label, more string, tokens []string) (lines []string, ok bool) {
	if len(tokens) == 0 {
		tokens = []string{""}
	}
	line, ok := label+tokens[0], true
	for _, t := range tokens[1:] {
		if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(t) <= DialogWidth {
			line += " " + t
			continue
		}
		lines, line = append(lines, line), more+t
	}
	lines = append(lines, line)
	for _, l := range lines {
		ok = ok && utf8.RuneCountInString(l) <= DialogWidth
	}
	return lines, ok
}

// quote writes outside text the way Go quotes a string, except that only
// printable ASCII and the letters of the Latin, Greek and Cyrillic scripts
// appear as themselves: each is one column wide in a monospace font, so a
// line's runes are its columns on screen. Wide, invisible, combining and
// look-alike characters (a full-width quote, a Hangul filler, a combining
// accent) are escaped, so no argument can pass for several or push the
// command off the screen.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case shown(r):
			b.WriteRune(r)
		default:
			q := strconv.QuoteToASCII(s[i : i+n])
			b.WriteString(q[1 : len(q)-1])
		}
		i += n
	}
	b.WriteByte('"')
	return b.String()
}

// shown reports whether r appears on a card as itself.
func shown(r rune) bool {
	switch {
	case r >= 0x20 && r < 0x7f:
		return true
	case r >= 0xc0 && r <= 0x24f: // Latin-1 letters, Latin Extended-A and -B
		return true
	case r >= 0x370 && r <= 0x4ff: // Greek and Cyrillic, letters only
		return unicode.IsLetter(r)
	}
	return false
}
