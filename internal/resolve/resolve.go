//go:build unix

// Package resolve finds the program a request names the way root would, and
// fingerprints it. A program that anyone but root could change is refused:
// approving it would approve whatever its owner writes into it later.
package resolve

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// RootPATH is the only search path onceroot uses; the caller's PATH never is.
const RootPATH = "/usr/local/sbin:/usr/local/bin:/usr/bin"

// trustedOwner owns the programs onceroot runs: root. Tests in this package
// set it to the test user.
var trustedOwner uint32 = 0

// trustTop is where the ownership walk stops: "/" in oncerootd, so every
// directory above a program counts. Tests stop it at their own directory,
// whose parents (such as a shared /tmp) they cannot vouch for.
var trustTop = "/"

// Resolve returns the absolute, symlink-free path of the program and the
// SHA-256 of its content. A bare name is searched in rootPATH only; a name
// with a slash must be absolute.
func Resolve(name, rootPATH string) (string, [32]byte, error) {
	var none [32]byte
	switch {
	case name == "":
		return "", none, errors.New("resolve: empty program name")
	case strings.Contains(name, "/") && !filepath.IsAbs(name):
		return "", none, fmt.Errorf("resolve: %q is a relative path; name the program or give its absolute path", name)
	}
	candidate := name
	if !strings.Contains(name, "/") {
		candidate = ""
		for _, dir := range filepath.SplitList(rootPATH) {
			p := filepath.Join(dir, name)
			if _, err := os.Lstat(p); err == nil {
				candidate = p
				break
			}
		}
		if candidate == "" {
			return "", none, fmt.Errorf("resolve: %q is not in root's PATH (%s)", name, rootPATH)
		}
	}
	final, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", none, fmt.Errorf("resolve: %w", err)
	}
	if err := trusted(final); err != nil {
		return "", none, err
	}
	sum, err := hashFile(final)
	if err != nil {
		return "", none, err
	}
	return final, sum, nil
}

// Unchanged checks, just before the run, that the program is still trusted
// and still has the content the card was approved for.
func Unchanged(path string, want [32]byte) error {
	if err := trusted(path); err != nil {
		return err
	}
	got, err := hashFile(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("resolve: %s changed since the card was made", path)
	}
	return nil
}

// trusted requires an executable regular file, and the file and every
// directory above it owned by root (or the trusted owner) and writable by
// nobody else.
func trusted(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("resolve: %s is not a regular file", path)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("resolve: %s is not executable", path)
	}
	for p := path; ; p = filepath.Dir(p) {
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("resolve: %w", err)
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("resolve: cannot read the owner of %s", p)
		}
		if st.Uid != 0 && st.Uid != trustedOwner {
			return fmt.Errorf("resolve: %s belongs to uid %d, not root", p, st.Uid)
		}
		if fi.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("resolve: %s is writable by group or others", p)
		}
		if p == trustTop || p == filepath.Dir(p) {
			return nil
		}
	}
}

func hashFile(path string) ([32]byte, error) {
	var sum [32]byte
	f, err := os.Open(path)
	if err != nil {
		return sum, fmt.Errorf("resolve: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return sum, fmt.Errorf("resolve: %w", err)
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
