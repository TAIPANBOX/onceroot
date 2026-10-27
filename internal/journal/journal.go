// Package journal is onceroot's append-only record: one JSON object per line,
// each Ed25519-signed and carrying the hash of the entry before it, so a
// changed, removed or reordered entry shows up in Verify.
//
// Its limit, said plainly: root holds the key, so root can write a whole new
// chain. The journal shows tampering by anyone else, not by root.
package journal

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/sig"
)

// signKind separates journal signatures from any other onceroot signature.
const signKind = "journal-entry"

// maxLine bounds one entry; a longer line is corruption, not an entry.
const maxLine = 1 << 20

// Kinds are the events onceroot journals.
var Kinds = []string{"request", "held", "allowed", "denied", "approved", "rejected", "expired", "withdrawn", "ran", "refused"}

// secretKeys may never appear as detail keys (I12).
var secretKeys = []string{"pin", "code", "token", "secret", "otp", "password", "totp", "key"}

// Entry is one line of the journal.
type Entry struct {
	Seq      uint64            `json:"seq"`
	Time     time.Time         `json:"time"`
	Kind     string            `json:"kind"`
	CardID   string            `json:"card"`
	CardHash string            `json:"card_hash"`
	Detail   map[string]string `json:"detail,omitempty"`
	Prev     string            `json:"prev"`
	Sig      string            `json:"sig"`
}

// Journal appends signed entries to one file.
type Journal struct {
	mu   sync.Mutex
	f    *os.File
	priv ed25519.PrivateKey
	seq  uint64
	prev [32]byte
}

// Open verifies the existing chain with the key's public half, then opens
// the file for appending. A broken chain is refused: onceroot does not write
// on top of a record it cannot trust.
func Open(path string, priv ed25519.PrivateKey) (*Journal, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o640)
	if err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	// Group onceroot reads the journal for verify; the service's umask would
	// otherwise leave it 0600.
	if err := f.Chmod(0o640); err != nil {
		f.Close()
		return nil, fmt.Errorf("journal: %w", err)
	}
	seq, prev, err := verify(f, priv.Public().(ed25519.PublicKey))
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Journal{f: f, priv: priv, seq: seq, prev: prev}, nil
}

// Append signs and writes one entry, then syncs the file. An error means
// the entry may not be on disk and the caller must refuse what it records.
func (j *Journal) Append(kind, cardID string, cardHash [32]byte, detail map[string]string) error {
	if !slices.Contains(Kinds, kind) {
		return fmt.Errorf("journal: unknown kind %q", kind)
	}
	for k := range detail {
		if slices.Contains(secretKeys, strings.ToLower(k)) {
			return fmt.Errorf("journal: detail key %q is reserved for secrets and never journalled", k)
		}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	e := Entry{
		Seq:      j.seq + 1,
		Time:     time.Now().UTC(),
		Kind:     kind,
		CardID:   cardID,
		CardHash: hex.EncodeToString(cardHash[:]),
		Detail:   detail,
		Prev:     hex.EncodeToString(j.prev[:]),
	}
	signed := canonical(e)
	s := sig.Sign(j.priv, signKind, signed)
	e.Sig = hex.EncodeToString(s)
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if _, err := j.f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if err := j.f.Sync(); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	j.seq = e.Seq
	j.prev = link(signed, s)
	return nil
}

// Close closes the file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.f.Close()
}

// Verify checks every entry of the journal at path: sequence, link to the
// previous entry and signature. The error names the first bad entry by its
// position. It returns the number of entries checked.
func Verify(path string, pub ed25519.PublicKey) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("journal: %w", err)
	}
	defer f.Close()
	n, _, err := verify(f, pub)
	return int(n), err
}

func verify(f *os.File, pub ed25519.PublicKey) (uint64, [32]byte, error) {
	var prev [32]byte
	if _, err := f.Seek(0, 0); err != nil {
		return 0, prev, fmt.Errorf("journal: %w", err)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	var n uint64
	for sc.Scan() {
		n++
		bad := func(format string, a ...any) (uint64, [32]byte, error) {
			return 0, prev, fmt.Errorf("journal: entry %d: %s", n, fmt.Sprintf(format, a...))
		}
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		var e Entry
		if err := dec.Decode(&e); err != nil {
			return bad("unreadable: %v", err)
		}
		if e.Seq != n {
			return bad("sequence number %d where %d belongs (an entry was removed, added or moved)", e.Seq, n)
		}
		if e.Prev != hex.EncodeToString(prev[:]) {
			return bad("does not follow the entry before it")
		}
		s, err := hex.DecodeString(e.Sig)
		if err != nil {
			return bad("unreadable signature")
		}
		signed := canonical(e)
		if !sig.Verify(pub, signKind, signed, s) {
			return bad("signature does not match its content")
		}
		prev = link(signed, s)
	}
	if err := sc.Err(); err != nil {
		return 0, prev, fmt.Errorf("journal: entry %d: %w", n+1, err)
	}
	return n, prev, nil
}

// canonical is the signed form of an entry: every field but Sig, length
// prefixed, detail keys sorted.
func canonical(e Entry) []byte {
	var b bytes.Buffer
	u := func(v uint64) {
		var x [8]byte
		binary.BigEndian.PutUint64(x[:], v)
		b.Write(x[:])
	}
	s := func(v string) {
		u(uint64(len(v)))
		b.WriteString(v)
	}
	u(e.Seq)
	s(e.Time.UTC().Format(time.RFC3339Nano))
	s(e.Kind)
	s(e.CardID)
	s(e.CardHash)
	keys := make([]string, 0, len(e.Detail))
	for k := range e.Detail {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	u(uint64(len(keys)))
	for _, k := range keys {
		s(k)
		s(e.Detail[k])
	}
	s(e.Prev)
	return b.Bytes()
}

// link is what the next entry names as its predecessor.
func link(signed, s []byte) [32]byte {
	return sha256.Sum256(append(append([]byte(nil), signed...), s...))
}

// LoadKey reads the journal's 32-byte Ed25519 seed, or creates one with mode
// 0600. A key file anyone but its owner can read or write is refused.
func LoadKey(path string) (ed25519.PrivateKey, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_, priv, gerr := ed25519.GenerateKey(nil)
		if gerr == nil {
			_, gerr = f.Write(priv.Seed())
		}
		if cerr := f.Close(); gerr == nil {
			gerr = cerr
		}
		if gerr != nil {
			os.Remove(path)
			return nil, fmt.Errorf("journal: key: %w", gerr)
		}
		return priv, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("journal: key: %w", err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("journal: key %s is readable or writable by others (mode %o)", path, fi.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("journal: key: %w", err)
	}
	if len(b) != ed25519.SeedSize {
		return nil, fmt.Errorf("journal: key %s is not a 32-byte seed", path)
	}
	return ed25519.NewKeyFromSeed(b), nil
}

// WritePublicKey writes the journal's public key for `onceroot journal verify`,
// readable by everyone whatever the umask.
func WritePublicKey(path string, pub ed25519.PublicKey) error {
	if err := os.WriteFile(path, pub, 0o644); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	return os.Chmod(path, 0o644)
}
