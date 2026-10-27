package journal

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/sig"
)

func setup(t *testing.T, n int) (string, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "journal")
	j, err := Open(path, priv)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{"request", "held", "approved", "ran", "request"}
	for i := 0; i < n; i++ {
		if err := j.Append(kinds[i%len(kinds)], "KQWPT", [32]byte{byte(i)}, map[string]string{"argv": "pacman -S x", "by": "polkit"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	return path, pub, priv
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func rewrite(t *testing.T, path string, ls []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(ls, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyAcceptsAnUntouchedJournal(t *testing.T) {
	path, pub, _ := setup(t, 5)
	n, err := Verify(path, pub)
	if err != nil || n != 5 {
		t.Fatalf("got %d, %v; want 5 entries and no error", n, err)
	}
}

func TestVerifyFindsAChangedByte(t *testing.T) {
	path, pub, _ := setup(t, 5)
	ls := lines(t, path)
	ls[2] = strings.Replace(ls[2], "pacman -S x", "pacman -S y", 1)
	rewrite(t, path, ls)
	_, err := Verify(path, pub)
	if err == nil || !strings.Contains(err.Error(), "entry 3") {
		t.Fatalf("changed entry 3: got %v", err)
	}
}

func TestVerifyFindsARemovedEntry(t *testing.T) {
	path, pub, _ := setup(t, 5)
	ls := lines(t, path)
	rewrite(t, path, append(ls[:2:2], ls[3:]...))
	_, err := Verify(path, pub)
	if err == nil || !strings.Contains(err.Error(), "entry 3") {
		t.Fatalf("removed entry 3: got %v", err)
	}
}

func TestVerifyFindsReorderedEntries(t *testing.T) {
	path, pub, _ := setup(t, 5)
	ls := lines(t, path)
	ls[1], ls[2] = ls[2], ls[1]
	rewrite(t, path, ls)
	_, err := Verify(path, pub)
	if err == nil || !strings.Contains(err.Error(), "entry 2") {
		t.Fatalf("swapped entries 2 and 3: got %v", err)
	}
}

func TestVerifyWithAnotherKeyFails(t *testing.T) {
	path, _, _ := setup(t, 2)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := Verify(path, other); err == nil {
		t.Fatal("verified under a key that did not sign it")
	}
}

func TestReopenContinuesTheChain(t *testing.T) {
	path, pub, priv := setup(t, 2)
	j, err := Open(path, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append("ran", "KQWPT", [32]byte{9}, nil); err != nil {
		t.Fatal(err)
	}
	j.Close()
	if n, err := Verify(path, pub); err != nil || n != 3 {
		t.Fatalf("after reopen: %d, %v", n, err)
	}
}

func TestOpenRefusesABrokenJournal(t *testing.T) {
	path, _, priv := setup(t, 3)
	ls := lines(t, path)
	ls[1] = strings.Replace(ls[1], "polkit", "phone", 1)
	rewrite(t, path, ls)
	if _, err := Open(path, priv); err == nil {
		t.Fatal("opened a journal whose chain is broken")
	}
}

func TestDetailNeverHoldsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	j, err := Open(path, priv)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	for _, k := range []string{"pin", "code", "token", "secret", "otp", "password", "PIN"} {
		if err := j.Append("approved", "KQWPT", [32]byte{}, map[string]string{k: "1234"}); err == nil {
			t.Errorf("detail key %q accepted", k)
		}
	}
	if err := j.Append("bogus", "KQWPT", [32]byte{}, nil); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestLoadKeyCreatesOnceWith0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.key")
	k1, err := LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode %v, %v", fi.Mode().Perm(), err)
	}
	k2, err := LoadKey(path)
	if err != nil || !k1.Equal(k2) {
		t.Fatalf("the second load gave another key: %v", err)
	}
}

func TestLoadKeyRefusesAWrongFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.key")
	os.WriteFile(path, []byte("short"), 0o600)
	if _, err := LoadKey(path); err == nil {
		t.Fatal("a 5-byte key file was accepted")
	}
	os.WriteFile(path, make([]byte, 32), 0o600)
	os.Chmod(path, 0o644)
	if _, err := LoadKey(path); err == nil {
		t.Fatal("a key file others can read was accepted")
	}
}

// craft writes entries signed with the real key, so each verify check can be
// tested alone: an entry that is validly signed but breaks exactly one rule.
func craft(t *testing.T, priv ed25519.PrivateKey, entries []Entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journal")
	var b strings.Builder
	for _, e := range entries {
		e.Sig = hex.EncodeToString(sig.Sign(priv, signKind, canonical(e)))
		line, _ := json.Marshal(e)
		b.Write(line)
		b.WriteByte('\n')
	}
	os.WriteFile(path, []byte(b.String()), 0o600)
	return path
}

func chained(priv ed25519.PrivateKey, n int) []Entry {
	var out []Entry
	var prev [32]byte
	for i := 1; i <= n; i++ {
		e := Entry{Seq: uint64(i), Time: time.Unix(int64(i), 0).UTC(), Kind: "request", CardID: "KQWPT", CardHash: hex.EncodeToString(make([]byte, 32)), Prev: hex.EncodeToString(prev[:])}
		c := canonical(e)
		prev = link(c, sig.Sign(priv, signKind, c))
		out = append(out, e)
	}
	return out
}

func TestVerifyRefusesASignedSequenceGap(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	es := chained(priv, 3)
	es[2].Seq = 7 // validly signed, correctly linked, numbered wrong
	if _, err := Verify(craft(t, priv, es), pub); err == nil || !strings.Contains(err.Error(), "entry 3") {
		t.Fatalf("got %v, want a refusal naming entry 3", err)
	}
}

func TestVerifyRefusesASignedBrokenLink(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	es := chained(priv, 3)
	es[2].Prev = hex.EncodeToString(make([]byte, 32)) // validly signed, numbered right, linked wrong
	if _, err := Verify(craft(t, priv, es), pub); err == nil || !strings.Contains(err.Error(), "entry 3") {
		t.Fatalf("got %v, want a refusal naming entry 3", err)
	}
}

// The service runs with UMask=0077; the journal must still be readable by
// group onceroot, so its mode is set explicitly.
func TestJournalModesSurviveATightUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	dir := t.TempDir()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	j, err := Open(filepath.Join(dir, "journal"), priv)
	if err != nil {
		t.Fatal(err)
	}
	j.Close()
	if err := WritePublicKey(filepath.Join(dir, "journal.pub"), pub); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{"journal": 0o640, "journal.pub": 0o644} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v (%v)", name, fi.Mode().Perm(), want, err)
		}
	}
}
