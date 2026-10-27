package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

// In tests the files belong to the test user, so the user stands in for root.
func init() { trustedOwner = uint32(os.Getuid()) }

func tool(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func tdir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0o755); err != nil {
		t.Fatal(err)
	}
	saved := trustTop
	trustTop = d
	t.Cleanup(func() { trustTop = saved })
	return d
}

func TestResolveFindsAProgramInRootPATH(t *testing.T) {
	d := tdir(t)
	want := tool(t, d, "tool", "#!/bin/true\n")
	got, sha, err := Resolve("tool", d)
	wantReal, _ := filepath.EvalSymlinks(want)
	if err != nil || got != wantReal || sha == ([32]byte{}) {
		t.Fatalf("got %q %x %v, want %q", got, sha, err, wantReal)
	}
}

func TestResolveRefusesRelativeAndMissing(t *testing.T) {
	d := tdir(t)
	for _, name := range []string{"./tool", "../tool", "sub/tool", "nosuchcmd", ""} {
		if _, _, err := Resolve(name, d); err == nil {
			t.Errorf("%q resolved", name)
		}
	}
}

func TestResolveUsesOnlyRootPATH(t *testing.T) {
	callers := tdir(t)
	tool(t, callers, "tool", "x")
	t.Setenv("PATH", callers)
	if _, _, err := Resolve("tool", tdir(t)); err == nil {
		t.Fatal("found a program on the caller's PATH")
	}
}

func TestResolveFollowsSymlinkToFinalFile(t *testing.T) {
	d := tdir(t)
	real := tool(t, d, "real", "x")
	if err := os.Symlink(real, filepath.Join(d, "alias")); err != nil {
		t.Fatal(err)
	}
	got, _, err := Resolve("alias", d)
	wantReal, _ := filepath.EvalSymlinks(real)
	if err != nil || got != wantReal {
		t.Fatalf("got %q, %v; want %q", got, err, wantReal)
	}
}

func TestResolveRefusesWhatOthersCanWrite(t *testing.T) {
	d := tdir(t)
	p := tool(t, d, "tool", "x")
	if err := os.Chmod(p, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve("tool", d); err == nil {
		t.Fatal("a world-writable program resolved")
	}
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve(p, d); err == nil {
		t.Fatal("a program in a world-writable directory resolved")
	}
}

func TestResolveRefusesAFileOfAnotherOwner(t *testing.T) {
	d := tdir(t)
	p := tool(t, d, "tool", "x")
	if os.Getuid() == 0 {
		// root's own files are always trusted; give this one to nobody.
		if err := os.Chown(p, 65534, 65534); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Resolve("tool", d); err == nil {
			t.Fatal("a program owned by nobody resolved")
		}
		return
	}
	saved := trustedOwner
	trustedOwner = saved + 1
	defer func() { trustedOwner = saved }()
	if _, _, err := Resolve("tool", d); err == nil {
		t.Fatal("a program owned by someone else resolved")
	}
}

func TestResolveRefusesNonExecutableAndDirectories(t *testing.T) {
	d := tdir(t)
	p := filepath.Join(d, "data")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"data", "dir"} {
		if _, _, err := Resolve(name, d); err == nil {
			t.Errorf("%s resolved", name)
		}
	}
}

func TestUnchangedSeesAReplacedBinary(t *testing.T) {
	d := tdir(t)
	tool(t, d, "tool", "version 1")
	path, sha, err := Resolve("tool", d)
	if err != nil {
		t.Fatal(err)
	}
	if err := Unchanged(path, sha); err != nil {
		t.Fatalf("untouched binary: %v", err)
	}
	if err := os.WriteFile(path, []byte("version 2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Unchanged(path, sha); err == nil {
		t.Fatal("a replaced binary passed")
	}
}
