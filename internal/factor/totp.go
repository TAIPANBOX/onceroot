// Package factor checks what a person types on the phone: a PIN they know
// and a TOTP code from a device they hold. Every approval asks for both; a
// spent code never passes again; five failures lock the factors for fifteen
// minutes.
//
// TOTP and PIN hashing are ported from openwrt-mcp (mfa.go, pin.go; MIT,
// Ian Williams and Yurii Kostiuk), without the unlock window: onceroot never
// opens a window, every approval is for one card.
package factor

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RFC 6238 as authenticator apps implement it: SHA-1, 6 digits, 30 s steps.
const (
	totpDigits = 6
	totpStep   = 30
	totpSkew   = 1
)

// CodeAt is the code for one time step.
func CodeAt(secret []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f // dynamic truncation, RFC 4226 5.4
	v := (uint32(sum[off]&0x7f) << 24) | (uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) | uint32(sum[off+3])
	return fmt.Sprintf("%06d", v%1_000_000)
}

// TOTP checks codes for one secret and remembers the last step spent, on
// disk, so a restart does not let a spent code pass again.
type TOTP struct {
	mu       sync.Mutex
	secret   []byte
	usedPath string
	last     uint64 // highest step spent; 0 means none
}

// NewTOTP reads the last spent step from usedPath, if the file exists.
func NewTOTP(secret []byte, usedPath string) (*TOTP, error) {
	t := &TOTP{secret: append([]byte(nil), secret...), usedPath: usedPath}
	b, err := os.ReadFile(usedPath)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, fmt.Errorf("factor: %w", err)
	default:
		n, perr := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
		if perr != nil {
			return nil, fmt.Errorf("factor: %s is unreadable", usedPath)
		}
		t.last = n
	}
	return t, nil
}

// Check accepts a 6-digit code for the current step or one step either side,
// newer than any step spent before, and spends it.
func (t *TOTP) Check(code string, now time.Time) bool {
	if len(code) != totpDigits || strings.Trim(code, "0123456789") != "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	cur := uint64(now.Unix()) / totpStep
	for d := -totpSkew; d <= totpSkew; d++ {
		ctr := uint64(int64(cur) + int64(d))
		if ctr <= t.last {
			continue
		}
		if hmac.Equal([]byte(CodeAt(t.secret, ctr)), []byte(code)) {
			if err := t.spend(ctr); err != nil {
				return false // a step we cannot record is a step we cannot refuse later
			}
			return true
		}
	}
	return false
}

func (t *TOTP) spend(ctr uint64) error {
	tmp := t.usedPath + ".new"
	if err := os.WriteFile(tmp, []byte(strconv.FormatUint(ctr, 10)+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, t.usedPath); err != nil {
		return err
	}
	t.last = ctr
	return nil
}

// NewSecret returns a random 20-byte TOTP secret.
func NewSecret() ([]byte, error) {
	b := make([]byte, 20)
	_, err := rand.Read(b)
	return b, err
}

// URI is the otpauth:// link an authenticator app scans.
func URI(secret []byte, account, issuer string) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {enc}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}
