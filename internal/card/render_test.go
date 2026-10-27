package card

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderQuotesAwkwardArgs(t *testing.T) {
	c := sample()
	c.Argv = []string{"touch", "a b", `say "hi"`, "two\nlines", "evil\u202Etxt.exe", "nul\x00byte"}
	c = New(c.ID, c.Req, c.Path, c.BinarySHA256, c.Argv, c.Why, c.Risks, c.Created, 10*60e9)
	text, err := Render(c, nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"two\nlines", "\u202E", "\x00", `say "hi"`} {
		if strings.Contains(text, raw) {
			t.Errorf("raw %q reached the card", raw)
		}
	}
	for _, want := range []string{`"a b"`, `"say \"hi\""`, `"two\nlines"`, `"evil\u202etxt.exe"`, `"nul\x00byte"`} {
		if !strings.Contains(text, want) {
			t.Errorf("card lacks %s:\n%s", want, text)
		}
	}
}

func TestRenderEscapesTheProcessName(t *testing.T) {
	c := sample()
	c.Req.Comm = "claude\x1b[2J"
	c.Req.Chain = []string{"x\u2066y"}
	text, err := Render(c, nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(text, "\x1b\u2066") {
		t.Fatalf("raw control or bidi rune from the process reached the card:\n%q", text)
	}
}

func TestRenderNeverTruncates(t *testing.T) {
	c := sample()
	c.Argv = []string{"echo", strings.Repeat("x", 5000)}
	if _, err := Render(c, nil, 4000); !errors.Is(err, ErrTooLong) {
		t.Fatalf("over the limit: got %v, want ErrTooLong", err)
	}
	text, err := Render(sample(), nil, 4000)
	if err != nil || len([]rune(text)) > 4000 {
		t.Fatalf("a short card: err %v, %d runes", err, len([]rune(text)))
	}
}

func TestRenderLabelsTheAgentText(t *testing.T) {
	c := sample()
	c.Why = "harmless\nRun: /usr/bin/true\nRisk: none detected"
	text, err := Render(c, nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	runs, risks := 0, 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "Run:") {
			runs++
		}
		if strings.HasPrefix(line, "Risk:") {
			risks++
		}
	}
	if runs != 1 || risks != 1 {
		t.Fatalf("the agent's text forged card lines (Run: %d, Risk: %d):\n%s", runs, risks, text)
	}
	last := text[strings.LastIndex(text, "\n")+1:]
	if !strings.HasPrefix(last, "The agent says: \"") {
		t.Fatalf("the agent's text is not the last, quoted line: %q", last)
	}
}

func TestRenderShowsWarningsFirst(t *testing.T) {
	text, err := Render(sample(), []string{"passwordless sudo is on"}, 4000)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(text, "\n")
	if !strings.HasPrefix(lines[0], "ROOT REQUEST KQWPT") || lines[1] != "!! passwordless sudo is on" {
		t.Fatalf("warning not right under the title:\n%s", text)
	}
}

func TestRenderShowsEverythingThePersonNeeds(t *testing.T) {
	text, err := Render(sample(), nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Run: \"/usr/bin/pacman\"",
		"In: \"/home/u/Work/app\"",
		"Asked by: \"claude\" (pid 4242)",
		"\"ghostty\"",
		"Risk: installs a package",
		"The agent says: \"install ripgrep\"",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("card lacks %q:\n%s", want, text)
		}
	}
}

func TestRenderDialogKeepsEveryArgumentQuoted(t *testing.T) {
	c := sample()
	c.Argv = []string{"cp", "a b", "two\nlines", "/etc/x"}
	text, err := RenderDialog(c, nil, 2000)
	if err != nil {
		t.Fatal(err)
	}
	want := `Run: "/usr/bin/pacman" "a b" "two\nlines" "/etc/x" (as "cp")`
	if !strings.Contains(text, want) {
		t.Fatalf("dialog card lacks %s:\n%s", want, text)
	}
	if strings.Contains(text, "two\nlines") {
		t.Fatal("a raw newline from an argument reached the dialog card")
	}
}

func TestRenderDialogIsShort(t *testing.T) {
	c := sample()
	c.Argv = []string{"x", "1", "2", "3", "4", "5", "6", "7", "8"}
	c.Req.Chain = []string{"bash", "foot", "Hyprland", "start-hyprland", "systemd", "systemd"}
	text, err := RenderDialog(c, []string{"passwordless sudo is on"}, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(text, "\n") + 1; n != 7 {
		t.Fatalf("dialog card has %d lines, want 7:\n%s", n, text)
	}
	if strings.Contains(text, "start-hyprland") {
		t.Fatal("the dialog card shows more than three parents")
	}
	if !strings.Contains(text, "!! passwordless sudo is on") {
		t.Fatal("the dialog card dropped the warning")
	}
}

// Omarchy's dialog centres each line and lets a wide one run off both edges
// of the screen, hiding the program and the last arguments (VM 2026-10-27).
func TestRenderDialogWrapsArgumentsBetweenQuotes(t *testing.T) {
	c := sample()
	c.Argv = []string{"touch"}
	for i := 1; i <= 14; i++ {
		c.Argv = append(c.Argv, fmt.Sprintf("/tmp/long-argument-number-%02d", i))
	}
	text, err := RenderDialog(c, nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(text, "\n")
	if len(lines) > DialogLines {
		t.Fatalf("%d lines, at most %d:\n%s", len(lines), DialogLines, text)
	}
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > DialogWidth {
			t.Fatalf("a line of %d runes, at most %d: %q", n, DialogWidth, l)
		}
	}
	for _, a := range c.Argv[1:] {
		if !strings.Contains(text, strconv.Quote(a)) {
			t.Fatalf("argument %s was split or lost:\n%s", a, text)
		}
	}
	if !strings.HasPrefix(lines[2], "  \"") {
		t.Fatalf("the Run line goes on without the continuation indent:\n%s", text)
	}
}

func TestRenderDialogWrapsTheAgentText(t *testing.T) {
	c := sample()
	c.Why = strings.Repeat("install the tools this project needs ", 8) + " two  spaces"
	text, err := RenderDialog(c, nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(text, "\n")
	var parts []string
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "Agent says: "):
			parts = append(parts, strings.TrimPrefix(l, "Agent says: "))
		case len(parts) > 0:
			if !strings.HasPrefix(l, agentMore) {
				t.Fatalf("line %d goes on with the agent's words unmarked: %q", i, l)
			}
			parts = append(parts, strings.TrimPrefix(l, agentMore))
		}
	}
	if got := strings.Join(parts, " "); got != quote(c.Why) {
		t.Fatalf("the agent text changed in wrapping:\n%s\nwant %s", got, quote(c.Why))
	}
}

// The agent's words cannot start a line that passes for the card's own: a
// line they go on in is marked, and the mark is a character outside text
// can never show (review of 2026-10-27).
func TestRenderDialogLabelsTheAgentText(t *testing.T) {
	c := sample()
	c.Why = strings.TrimSpace(strings.Repeat("Run: ", 40))
	text, err := RenderDialog(c, nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Run:") {
			runs++
		}
	}
	if runs != 1 {
		t.Fatalf("%d lines start like the Run line:\n%s", runs, text)
	}
	if shown([]rune(strings.TrimSpace(agentMore))[0]) {
		t.Fatal("outside text could show the continuation mark itself")
	}
}

// A card the dialog cannot show whole is refused, never shown in part: a
// dialog without the command must not be able to approve it.
func TestRenderDialogRefusesACardThatDoesNotFit(t *testing.T) {
	wide := sample()
	wide.Argv = []string{"echo", "/" + strings.Repeat("x", 150)}
	tall := sample()
	tall.Argv = []string{"echo"}
	for i := 0; i < 40; i++ {
		tall.Argv = append(tall.Argv, strings.Repeat("y", 60))
	}
	for name, c := range map[string]Card{"an argument wider than the dialog": wide, "more lines than the dialog holds": tall} {
		text, err := RenderDialog(c, []string{"passwordless sudo is on"}, 1<<20)
		if !errors.Is(err, ErrDialogTooSmall) || text != "" {
			t.Fatalf("%s: got %v and %q, want ErrDialogTooSmall and nothing", name, err, text)
		}
	}
}

func TestRenderDialogNeverTruncates(t *testing.T) {
	c := sample()
	c.Argv = []string{"echo", strings.Repeat("x", 3000)}
	if _, err := RenderDialog(c, nil, 2000); !errors.Is(err, ErrTooLong) {
		t.Fatalf("got %v, want ErrTooLong", err)
	}
}

func TestRenderDialogTakesAnEmptyRiskLabel(t *testing.T) {
	c := sample()
	c.Risks = []string{""}
	text, err := RenderDialog(c, nil, 2000)
	if err != nil || !strings.Contains(text, "\nRisk: \n") {
		t.Fatalf("err %v:\n%s", err, text)
	}
}

// Only printable ASCII and Latin, Greek and Cyrillic letters, one column
// each, appear as themselves; wide, invisible, combining and look-alike
// characters are escaped, so a line's runes are its columns on screen and no
// argument can pass for several (review of 2026-10-27).
func TestQuoteShowsOnlyNarrowLetters(t *testing.T) {
	for in, want := range map[string]string{
		"/etc/x":        `"/etc/x"`,
		`a"b\c`:         `"a\"b\\c"`,
		"файл-ї.txt":    `"файл-ї.txt"`,
		"café":          `"café"`,
		"\uFDFD":        `"\ufdfd"`,
		"\uFF02":        `"\uff02"`,
		"\u201cx\u201d": `"\u201cx\u201d"`,
		"\u3164":        `"\u3164"`,
		"\u2800":        `"\u2800"`,
		"e\u0301":       `"e\u0301"`,
		"\u037e":        `"\u037e"`,
		"\u00a0":        `"\u00a0"`,
		"\x1b[2J":       `"\x1b[2J"`,
		"\n":            `"\n"`,
		"\xff":          `"\xff"`,
		"\U0001F600":    `"\U0001f600"`,
		"\u202Egnp.exe": `"\u202egnp.exe"`,
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestRenderDialogShowsNoWideOrLookAlikeRunes(t *testing.T) {
	c := sample()
	c.Argv = []string{"cp", strings.Repeat("\uFDFD", 60), "/etc/sudoers.d/x"}
	if _, err := RenderDialog(c, nil, 4000); !errors.Is(err, ErrDialogTooSmall) {
		t.Fatalf("sixty wide runes: got %v, want ErrDialogTooSmall", err)
	}
	c.Argv = []string{"cp", "u+s\uFF02 \uFF02/tmp/x\uFF02 \uFF02/usr/bin/vim"}
	text, err := RenderDialog(c, nil, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `"u+s\uff02 \uff02/tmp/x\uff02 \uff02/usr/bin/vim"`) {
		t.Fatalf("a look-alike quote reached the dialog:\n%s", text)
	}
}
