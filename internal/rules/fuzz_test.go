package rules

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add("allow /usr/bin/pacman -Q* ...\ndeny /usr/bin/rm ...\n")
	f.Add("# c\n\nallow /x\n")
	f.Add("deny /usr/bin/a b* c ...")
	f.Fuzz(func(t *testing.T, text string) {
		rs, err := Parse(strings.NewReader(text))
		if err != nil {
			return
		}
		for _, r := range rs {
			if r.Verdict != Allow && r.Verdict != Deny {
				t.Fatalf("rule with verdict %v accepted", r.Verdict)
			}
			if !strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, "*") {
				t.Fatalf("rule with path %q accepted", r.Path)
			}
			for _, p := range r.Args {
				if i := strings.Index(p, "*"); i >= 0 && i != len(p)-1 {
					t.Fatalf("pattern %q accepted", p)
				}
				if p == "..." {
					t.Fatal("ellipsis kept as a pattern")
				}
			}
		}
	})
}
