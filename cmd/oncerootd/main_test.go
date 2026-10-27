package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TAIPANBOX/onceroot/internal/phone"
)

var me = uint32(os.Getuid())

func TestLoadRulesMissingFileMeansNoRules(t *testing.T) {
	rs, err := loadRules(filepath.Join(t.TempDir(), "absent"), me)
	if err != nil || rs != nil {
		t.Fatalf("got %v, %v; want no rules and no error", rs, err)
	}
}

func TestLoadRulesParsesAPrivateFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules")
	os.WriteFile(p, []byte("allow /usr/bin/id\n"), 0o644)
	rs, err := loadRules(p, me)
	if err != nil || len(rs) != 1 {
		t.Fatalf("got %v, %v", rs, err)
	}
}

func TestLoadRulesRefusesAFileOthersCanWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules")
	os.WriteFile(p, []byte("allow /usr/bin/id\n"), 0o644)
	os.Chmod(p, 0o666)
	if _, err := loadRules(p, me); err == nil {
		t.Fatal("a rules file anyone can write was used")
	}
}

func TestLoadRulesRefusesAnotherOwner(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules")
	os.WriteFile(p, []byte("allow /usr/bin/id\n"), 0o644)
	if _, err := loadRules(p, me+1); err == nil {
		t.Fatal("a rules file owned by someone else was used")
	}
}

func TestLoadRulesRefusesABrokenFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules")
	os.WriteFile(p, []byte("permit /usr/bin/id\n"), 0o644)
	if _, err := loadRules(p, me); err == nil {
		t.Fatal("a rules file that does not parse was used")
	}
}

func TestSetupPhoneWritesFactorsAndPrintsTheLink(t *testing.T) {
	dir := t.TempDir()
	fac, used := filepath.Join(dir, "factors"), filepath.Join(dir, "totp.used")
	os.WriteFile(used, []byte("123\n"), 0o600)
	var out bytes.Buffer
	if err := setupPhone(strings.NewReader("472913\n"), &out, fac, used, "box"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "otpauth://totp/") || strings.Contains(out.String(), "472913") {
		t.Fatalf("output %q", out.String())
	}
	if _, err := os.Stat(used); !os.IsNotExist(err) {
		t.Fatal("the old spent-step file was kept for a new secret")
	}
	os.WriteFile(filepath.Join(dir, "tok"), []byte("1:x"), 0o600)
	if _, _, err := phone.Load(filepath.Join(dir, "tok"), fac, used); err != nil {
		t.Fatalf("the written factors do not load: %v", err)
	}
}

func TestSetupPhoneRefusesAShortPIN(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := setupPhone(strings.NewReader("1234\n"), &out, filepath.Join(dir, "f"), filepath.Join(dir, "u"), "box"); err == nil {
		t.Fatal("a 4-digit PIN was accepted")
	}
	if err := setupPhone(strings.NewReader(""), &out, filepath.Join(dir, "f"), filepath.Join(dir, "u"), "box"); err == nil {
		t.Fatal("an empty stdin was accepted")
	}
}
