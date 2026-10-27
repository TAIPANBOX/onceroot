package gates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuiltBinariesAreHardened measures the binaries a build made, named in
// ONCEROOT_CHECK_BIN (paths joined by ':'); PKGBUILD's check() runs it on the
// two it built.
func TestBuiltBinariesAreHardened(t *testing.T) {
	list := os.Getenv("ONCEROOT_CHECK_BIN")
	if list == "" {
		t.Skip("ONCEROOT_CHECK_BIN names no binaries; PKGBUILD's check() sets it")
	}
	for _, p := range strings.Split(list, ":") {
		if problems := checkELF(p); len(problems) > 0 {
			t.Errorf("%s:\n%s", p, strings.Join(problems, "\n"))
		}
	}
}

func TestELFGateHasTeeth(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain to build the samples with")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module sample\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	build := func(name string, args ...string) string {
		t.Helper()
		out := filepath.Join(dir, name)
		cmd := exec.Command(gobin, append(append([]string{"build", "-trimpath", "-o", out}, args...), ".")...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOFLAGS=")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, b)
		}
		return out
	}
	if p := checkELF(build("good", "-buildmode=pie", "-ldflags=-bindnow")); len(p) > 0 {
		t.Fatalf("a hardened binary failed: %v", p)
	}
	for _, c := range []struct {
		name, want string
		args       []string
	}{
		{"not PIE", "position independent", []string{"-buildmode=exe", "-ldflags=-bindnow"}},
		{"no BIND_NOW", "BIND_NOW", []string{"-buildmode=pie"}},
		{"stripped at the link", "stripped", []string{"-buildmode=pie", "-ldflags=-bindnow -s -w"}},
	} {
		p := checkELF(build(strings.ReplaceAll(c.name, " ", "-"), c.args...))
		if !strings.Contains(strings.Join(p, "\n"), c.want) {
			t.Errorf("%s: problems %q do not name it", c.name, p)
		}
	}
	if p := checkELF(filepath.Join(dir, "absent")); len(p) == 0 {
		t.Error("a missing binary passed")
	}
}
