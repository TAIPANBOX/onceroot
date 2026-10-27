package card

import (
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func FuzzRender(f *testing.F) {
	f.Add("a b", "two\nlines", "why\x00", "claude")
	f.Add("\u202E", "\u2066x\u2069", "", "\x1b[2J")
	f.Fuzz(func(t *testing.T, a1, a2, why, comm string) {
		c := sample()
		c.Argv = []string{"cmd", a1, a2}
		c.Why = why
		c.Req.Comm = comm
		warnings := []string{"passwordless sudo is on"}
		text, err := Render(c, warnings, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range text {
			if r == '\n' {
				continue
			}
			if r < 0x20 || r == 0x7f || isBidi(r) || unicode.Is(unicode.Cf, r) || !shown(r) {
				t.Fatalf("raw rune %U in card", r)
			}
		}
		// title, Run, In, Asked by, Risk, The agent says
		want := 6 + len(c.Argv) + len(warnings)
		if got := strings.Count(text, "\n") + 1; got != want {
			t.Fatalf("card has %d lines, want %d", got, want)
		}
	})
}

func isBidi(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200E || r == 0x200F || r == 0x061C
}

func FuzzRenderDialog(f *testing.F) {
	f.Add("a b", "two\nlines", "why\x00", "claude")
	f.Add("\u202E", "\u2066x\u2069", "", "\x1b[2J")
	f.Add(strings.Repeat("w", 120), "x", strings.Repeat("long why ", 40), "c")
	f.Fuzz(func(t *testing.T, a1, a2, why, comm string) {
		c := sample()
		c.Argv = []string{"cmd", a1, a2}
		c.Why = why
		c.Req.Comm = comm
		text, err := RenderDialog(c, nil, 1<<20)
		if errors.Is(err, ErrDialogTooSmall) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(text, "\n") {
			if n := utf8.RuneCountInString(l); n > DialogWidth {
				t.Fatalf("a line of %d runes: %q", n, l)
			}
			for _, r := range l {
				if r < 0x20 || r == 0x7f || isBidi(r) || unicode.Is(unicode.Cf, r) || !shown(r) && r != '\u00bb' {
					t.Fatalf("raw rune %U in dialog card", r)
				}
			}
		}
		if n := strings.Count(text, "\n") + 1; n > DialogLines {
			t.Fatalf("%d lines", n)
		}
		for _, a := range c.Argv[1:] {
			if !strings.Contains(text, quote(a)) {
				t.Fatalf("argument %q missing from the dialog card", a)
			}
		}
	})
}
