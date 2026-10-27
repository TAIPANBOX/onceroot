package factor

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

var rfcSecret = []byte("12345678901234567890")

func at(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

func TestRFC6238Vector(t *testing.T) {
	// RFC 6238 appendix B, SHA-1, T = 59 s: 94287082; six digits: 287082.
	if got := CodeAt(rfcSecret, 1); got != "287082" {
		t.Fatalf("got %s, want 287082", got)
	}
}

func newTOTP(t *testing.T) (*TOTP, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "totp.used")
	tp, err := NewTOTP(rfcSecret, path)
	if err != nil {
		t.Fatal(err)
	}
	return tp, path
}

func TestCodeSpentOnce(t *testing.T) {
	tp, _ := newTOTP(t)
	now := at(1_000_000)
	code := CodeAt(rfcSecret, uint64(now.Unix()/30))
	if !tp.Check(code, now) {
		t.Fatal("fresh code refused")
	}
	if tp.Check(code, now.Add(5*time.Second)) {
		t.Fatal("the same code passed twice")
	}
}

func TestSkewOneStep(t *testing.T) {
	now := at(1_000_000)
	c := uint64(now.Unix() / 30)
	for _, ctr := range []uint64{c - 1, c, c + 1} {
		tp, _ := newTOTP(t)
		if !tp.Check(CodeAt(rfcSecret, ctr), now) {
			t.Errorf("step %+d refused", int64(ctr)-int64(c))
		}
	}
	for _, ctr := range []uint64{c - 2, c + 2} {
		tp, _ := newTOTP(t)
		if tp.Check(CodeAt(rfcSecret, ctr), now) {
			t.Errorf("step %+d accepted", int64(ctr)-int64(c))
		}
	}
}

func TestOlderStepRefusedAfterNewer(t *testing.T) {
	tp, _ := newTOTP(t)
	now := at(1_000_000)
	c := uint64(now.Unix() / 30)
	if !tp.Check(CodeAt(rfcSecret, c), now) {
		t.Fatal("current code refused")
	}
	if tp.Check(CodeAt(rfcSecret, c-1), now) {
		t.Fatal("an older step passed after a newer one was spent")
	}
}

func TestSpentStepSurvivesRestart(t *testing.T) {
	tp, path := newTOTP(t)
	now := at(1_000_000)
	code := CodeAt(rfcSecret, uint64(now.Unix()/30))
	if !tp.Check(code, now) {
		t.Fatal("fresh code refused")
	}
	again, err := NewTOTP(rfcSecret, path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Check(code, now) {
		t.Fatal("a spent code passed after a restart")
	}
}

func TestMalformedCodeRefused(t *testing.T) {
	tp, _ := newTOTP(t)
	for _, c := range []string{"", "12345", "1234567", "28708a", " 287082"} {
		if tp.Check(c, at(59)) {
			t.Errorf("%q accepted", c)
		}
	}
}

func TestPINDigitsOnly6To8(t *testing.T) {
	for _, ok := range []string{"123456", "12345678"} {
		if err := ValidPIN(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"1234", "12345", "123456789", "12a456", "", "١٢٣٤٥٦"} {
		if err := ValidPIN(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestPINHashChecks(t *testing.T) {
	salt := []byte("0123456789abcdef")
	h := HashPIN("4711", salt)
	if !CheckPIN("4711", h, salt) {
		t.Fatal("right PIN refused")
	}
	if CheckPIN("4712", h, salt) {
		t.Fatal("wrong PIN accepted")
	}
	if string(HashPIN("4711", []byte("another salt...."))) == string(h) {
		t.Fatal("the salt does not change the hash")
	}
}

func TestLockoutAfterFiveFailures(t *testing.T) {
	l := NewLockout(5, 15*time.Minute)
	now := at(1000)
	for i := range 4 {
		if err := l.Try(now, false); errors.Is(err, ErrLockedOut) {
			t.Fatalf("locked out after %d failures", i+1)
		}
	}
	if err := l.Try(now, false); !errors.Is(err, ErrLockedOut) {
		t.Fatalf("fifth failure: got %v, want ErrLockedOut", err)
	}
	if err := l.Allowed(now.Add(time.Minute)); !errors.Is(err, ErrLockedOut) {
		t.Fatal("not locked a minute later")
	}
}

func TestLockoutEndsAfterFifteenMinutes(t *testing.T) {
	l := NewLockout(5, 15*time.Minute)
	now := at(1000)
	for range 5 {
		l.Try(now, false)
	}
	if err := l.Allowed(now.Add(15 * time.Minute)); err != nil {
		t.Fatalf("still locked after 15 minutes: %v", err)
	}
	if err := l.Try(now.Add(15*time.Minute), false); errors.Is(err, ErrLockedOut) {
		t.Fatal("the count did not start again after the lockout")
	}
}

func factors(t *testing.T) *Factors {
	t.Helper()
	tp, _ := newTOTP(t)
	salt := []byte("0123456789abcdef")
	return &Factors{PINSalt: salt, PINHash: HashPIN("4711", salt), TOTP: tp, Lock: NewLockout(5, 15*time.Minute)}
}

func TestWrongPINDoesNotSpendTheCode(t *testing.T) {
	f := factors(t)
	now := at(1_000_000)
	code := CodeAt(rfcSecret, uint64(now.Unix()/30))
	if err := f.Check("0000", code, now); !errors.Is(err, ErrWrong) {
		t.Fatalf("wrong PIN: got %v, want ErrWrong", err)
	}
	if err := f.Check("4711", code, now); err != nil {
		t.Fatalf("right PIN with the same code: %v", err)
	}
}

func TestLockedOutRefusesEvenTheRightFactors(t *testing.T) {
	f := factors(t)
	now := at(1_000_000)
	for range 5 {
		f.Check("0000", "000000", now)
	}
	code := CodeAt(rfcSecret, uint64(now.Unix()/30))
	if err := f.Check("4711", code, now); !errors.Is(err, ErrLockedOut) {
		t.Fatalf("right factors during a lockout: got %v, want ErrLockedOut", err)
	}
	later := now.Add(15 * time.Minute)
	if err := f.Check("4711", CodeAt(rfcSecret, uint64(later.Unix()/30)), later); err != nil {
		t.Fatalf("right factors after the lockout: %v", err)
	}
}
