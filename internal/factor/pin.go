package factor

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
)

const (
	pinIterations = 200_000
	pinKeyLen     = 32
)

// ErrBadPIN refuses a PIN that is not 6 to 8 ASCII digits. Six is the
// floor: the lockout stops guessing, and a longer PIN costs one tap more.
var ErrBadPIN = errors.New("factor: a PIN is 6 to 8 digits")

// ValidPIN accepts 6 to 8 ASCII digits.
func ValidPIN(pin string) error {
	if len(pin) < 6 || len(pin) > 8 {
		return ErrBadPIN
	}
	for i := 0; i < len(pin); i++ {
		if pin[i] < '0' || pin[i] > '9' {
			return ErrBadPIN
		}
	}
	return nil
}

// HashPIN derives the stored form of a PIN: PBKDF2-HMAC-SHA256.
func HashPIN(pin string, salt []byte) []byte {
	k, err := pbkdf2.Key(sha256.New, pin, salt, pinIterations, pinKeyLen)
	if err != nil {
		panic("factor: pbkdf2: " + err.Error()) // fixed parameters; cannot fail
	}
	return k
}

// CheckPIN compares in constant time.
func CheckPIN(pin string, stored, salt []byte) bool {
	return subtle.ConstantTimeCompare(HashPIN(pin, salt), stored) == 1
}
