package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// I15: go.mod requires only allowed modules.
func TestDependencyAllowlist(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if p := checkDeps(b); len(p) > 0 {
		t.Fatalf("go.mod breaks the allowlist:\n%s", strings.Join(p, "\n"))
	}
}

func TestDependencyGateHasTeeth(t *testing.T) {
	cases := []struct {
		name   string
		goMod  string
		refuse bool
	}{
		{"foreign require", "module x\n\ngo 1.27\n\nrequire github.com/evil/thing v1.0.0\n", true},
		{"foreign require in a block", "module x\n\ngo 1.27\n\nrequire (\n\tgolang.org/x/sys v0.49.0\n\tgithub.com/a/b v1.0.0 // indirect\n)\n", true},
		{"replace to a remote module", "module x\n\ngo 1.27\n\nreplace golang.org/x/sys => github.com/evil/sys v1.0.0\n", true},
		{"replace block to a remote module", "module x\n\nreplace (\n\tgolang.org/x/sys => example.com/sys v0.1.0\n)\n", true},
		{"allowed require", "module x\n\ngo 1.27\n\nrequire golang.org/x/sys v0.49.0\n", false},
		{"allowed require block", "module x\n\ngo 1.27\n\nrequire (\n\tgolang.org/x/sys v0.49.0\n)\n", false},
		{"local replace", "module x\n\nreplace golang.org/x/sys => ../sys\n", false},
		{"no requires", "module x\n\ngo 1.27\n", false},
	}
	for _, c := range cases {
		got := len(checkDeps([]byte(c.goMod))) > 0
		if got != c.refuse {
			t.Errorf("%s: refused=%v, want %v", c.name, got, c.refuse)
		}
	}
}
