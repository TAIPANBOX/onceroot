package sig

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func key(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestSignatureVerifies(t *testing.T) {
	pub, priv := key(t)
	s := Sign(priv, "journal-entry", []byte("m"))
	if !Verify(pub, "journal-entry", []byte("m"), s) {
		t.Fatal("own signature refused")
	}
	if Verify(pub, "journal-entry", []byte("n"), s) {
		t.Fatal("signature accepted for another message")
	}
}

func TestSignatureIsDomainSeparated(t *testing.T) {
	pub, priv := key(t)
	s := Sign(priv, "journal-entry", []byte("m"))
	if Verify(pub, "other-kind", []byte("m"), s) {
		t.Fatal("a signature of one kind passes as another")
	}
	if ed25519.Verify(pub, []byte("rollcall/v1/journal-entry\x00m"), s) {
		t.Fatal("an onceroot signature passes in rollcall's domain")
	}
}

func TestSmallOrderKeyRefused(t *testing.T) {
	identity, _ := hex.DecodeString("0100000000000000000000000000000000000000000000000000000000000000")
	if ValidPublicKey(identity) != ErrSmallOrderKey {
		t.Fatal("the identity point was accepted as a key")
	}
	if Verify(identity, "journal-entry", []byte("m"), make([]byte, 64)) {
		t.Fatal("verify accepted a signature under a small-order key")
	}
	if ValidPublicKey(make([]byte, 31)) != ErrKeyLength {
		t.Fatal("a 31-byte key was accepted")
	}
}

func TestSignRefusesAnAmbiguousKind(t *testing.T) {
	_, priv := key(t)
	for _, kind := range []string{"", "a\x00b"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Sign accepted kind %q", kind)
				}
			}()
			Sign(priv, kind, []byte("m"))
		}()
	}
}
