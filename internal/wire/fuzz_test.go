package wire

import (
	"bufio"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzDecode(f *testing.F) {
	f.Add(`{"op":"run","argv":["pacman","-S","x"],"why":"w"}` + "\n")
	f.Add(`{"op":"approve","id":"KQWPT"}` + "\n")
	f.Add(`{"op":"list"}`)
	f.Fuzz(func(t *testing.T, s string) {
		r, err := Decode(bufio.NewReader(strings.NewReader(s)))
		if err != nil {
			return
		}
		switch r.Op {
		case "run":
			if len(r.Argv) < 1 || len(r.Argv) > MaxArgs || r.Argv[0] == "" || r.ID != "" {
				t.Fatalf("bad run accepted: %+v", r)
			}
			for _, a := range r.Argv {
				if len(a) > MaxArg || strings.IndexByte(a, 0) >= 0 {
					t.Fatalf("bad argument accepted: %q", a)
				}
			}
			if utf8.RuneCountInString(r.Why) > MaxWhy || !utf8.ValidString(r.Why) {
				t.Fatalf("bad why accepted: %q", r.Why)
			}
		case "list", "status":
			if r.Argv != nil || r.ID != "" || r.Why != "" {
				t.Fatalf("fields on %s: %+v", r.Op, r)
			}
		case "approve", "reject":
			if !ValidID(r.ID) || r.Argv != nil {
				t.Fatalf("bad %s accepted: %+v", r.Op, r)
			}
		default:
			t.Fatalf("op %q accepted", r.Op)
		}
	})
}
