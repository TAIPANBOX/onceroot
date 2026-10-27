package wire

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func decode(s string) (Request, error) {
	return Decode(bufio.NewReader(strings.NewReader(s)))
}

func TestDecodeAcceptsEachOp(t *testing.T) {
	for _, line := range []string{
		`{"op":"run","argv":["pacman","-S","x"],"why":"install x"}`,
		`{"op":"list"}`,
		`{"op":"status"}`,
		`{"op":"approve","id":"KQWPT"}`,
		`{"op":"reject","id":"KQWPT"}`,
	} {
		if _, err := decode(line + "\n"); err != nil {
			t.Errorf("%s: %v", line, err)
		}
	}
}

func TestDecodeRefusesOutOfBounds(t *testing.T) {
	long := strings.Repeat("a", 4097)
	many := `"` + strings.Repeat(`a","`, 256) + `a"`
	cases := map[string]string{
		"unknown op":        `{"op":"exec","argv":["x"]}`,
		"unknown field":     `{"op":"list","env":{"A":"1"}}`,
		"empty argv":        `{"op":"run","argv":[]}`,
		"empty program":     `{"op":"run","argv":[""]}`,
		"257 arguments":     `{"op":"run","argv":[` + many + `]}`,
		"argument too long": `{"op":"run","argv":["x","` + long + `"]}`,
		"NUL in argument":   `{"op":"run","argv":["x","a\u0000b"]}`,
		"why too long":      `{"op":"run","argv":["x"],"why":"` + strings.Repeat("я", 501) + `"}`,
		"argv on list":      `{"op":"list","argv":["x"]}`,
		"bad id":            `{"op":"approve","id":"KQWPI"}`,
		"id on run":         `{"op":"run","argv":["x"],"id":"KQWPT"}`,
		"two objects":       `{"op":"list"}{"op":"status"}`,
		"not json":          `run pacman`,
	}
	for name, line := range cases {
		if _, err := decode(line + "\n"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestDecodeRefusesALineOver64KiB(t *testing.T) {
	line := `{"op":"run","argv":["x"],"why":"` + strings.Repeat("a", 70*1024) + `"}` + "\n"
	if _, err := decode(line); err == nil {
		t.Fatal("a 70 KiB line was read")
	}
}

func TestEventRoundTrip(t *testing.T) {
	var b bytes.Buffer
	in := Event{Ev: "out", FD: 2, Data: []byte{0, 1, 0xff, '\n'}}
	if err := WriteEvent(&b, in); err != nil {
		t.Fatal(err)
	}
	if strings.Count(b.String(), "\n") != 1 {
		t.Fatalf("an event is not exactly one line: %q", b.String())
	}
	out, err := ReadEvent(bufio.NewReader(&b))
	if err != nil {
		t.Fatal(err)
	}
	if out.Ev != "out" || out.FD != 2 || !bytes.Equal(out.Data, in.Data) {
		t.Fatalf("got %+v", out)
	}
}
