package gates

import (
	"strings"
	"testing"
)

// Only internal/runner and internal/polkit start processes.
func TestOnlyRunnerAndPolkitImportExec(t *testing.T) {
	if p := checkExec(repoRoot(t)); len(p) > 0 {
		t.Fatalf("process starts outside runner and polkit:\n%s", strings.Join(p, "\n"))
	}
}

func TestExecGateHasTeeth(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		fail  bool
	}{
		"os/exec in daemon": {map[string]string{"internal/daemon/d.go": "package d\nimport \"os/exec\"\nvar _ = exec.Command\n"}, true},
		"StartProcess":      {map[string]string{"cmd/x/main.go": "package main\nimport \"os\"\nfunc main(){ os.StartProcess(\"/bin/sh\", nil, nil) }\n"}, true},
		"syscall.Exec":      {map[string]string{"internal/x/x.go": "package x\nimport \"syscall\"\nfunc f(){ syscall.Exec(\"/bin/sh\", nil, nil) }\n"}, true},
		"runner allowed":    {map[string]string{"internal/runner/r.go": "package runner\nimport \"os/exec\"\nvar _ = exec.Command\n"}, false},
		"tests allowed":     {map[string]string{"internal/x/x_test.go": "package x\nimport \"os/exec\"\nvar _ = exec.Command\n"}, false},
	}
	for name, c := range cases {
		got := len(checkExec(writeTree(t, c.files))) > 0
		if got != c.fail {
			t.Errorf("%s: failed=%v, want %v", name, got, c.fail)
		}
	}
}
