package phone

import (
	"bufio"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/factor"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// WriteFactors stores a PIN (salted and hashed, never in clear) and a TOTP
// secret in a file only its owner can read.
func WriteFactors(path, pin string, totpSecret []byte) error {
	if err := factor.ValidPIN(pin); err != nil {
		return err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	body := fmt.Sprintf("pin_salt %s\npin_hash %s\ntotp %s\n",
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(factor.HashPIN(pin, salt)),
		b32.EncodeToString(totpSecret))
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads the bot token and the factors. Each file must be readable by
// its owner only.
func Load(tokenPath, factorsPath, usedPath string) (string, *factor.Factors, error) {
	for _, p := range []string{tokenPath, factorsPath} {
		fi, err := os.Stat(p)
		if err != nil {
			return "", nil, fmt.Errorf("phone: %w", err)
		}
		if fi.Mode().Perm()&0o077 != 0 {
			return "", nil, fmt.Errorf("phone: %s must be readable by its owner only (mode %o)", p, fi.Mode().Perm())
		}
	}
	tb, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", nil, fmt.Errorf("phone: %w", err)
	}
	token := strings.TrimSpace(string(tb))
	if token == "" || strings.ContainsAny(token, " \t\n/") {
		return "", nil, errors.New("phone: the bot token file does not hold one token")
	}
	f, err := os.Open(factorsPath)
	if err != nil {
		return "", nil, fmt.Errorf("phone: %w", err)
	}
	defer f.Close()
	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if ok {
			vals[k] = strings.TrimSpace(v)
		}
	}
	salt, err1 := base64.StdEncoding.DecodeString(vals["pin_salt"])
	hash, err2 := base64.StdEncoding.DecodeString(vals["pin_hash"])
	secret, err3 := b32.DecodeString(vals["totp"])
	if err1 != nil || err2 != nil || err3 != nil || len(salt) == 0 || len(hash) == 0 || len(secret) == 0 {
		return "", nil, errors.New("phone: the factors file is incomplete")
	}
	tp, err := factor.NewTOTP(secret, usedPath)
	if err != nil {
		return "", nil, err
	}
	return token, &factor.Factors{PINSalt: salt, PINHash: hash, TOTP: tp, Lock: factor.NewLockout(5, 15*time.Minute)}, nil
}
