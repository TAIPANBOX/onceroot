package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnitHardening(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "oncerootd.service"))
	if err != nil {
		t.Fatal(err)
	}
	if p := checkUnit(b); len(p) > 0 {
		t.Fatalf("oncerootd.service:\n%s", strings.Join(p, "\n"))
	}
}

func TestUnitGateHasTeeth(t *testing.T) {
	good := "[Service]\nExecStart=/usr/bin/oncerootd\nUMask=0022\nProtectHome=read-only\nPrivateTmp=yes\nRestart=on-failure\n"
	if p := checkUnit([]byte(good)); len(p) > 0 {
		t.Fatalf("a good unit failed: %v", p)
	}
	for name, bad := range map[string]string{
		"no PrivateTmp":       strings.Replace(good, "PrivateTmp=yes\n", "", 1),
		"home writable":       strings.Replace(good, "ProtectHome=read-only", "ProtectHome=no", 1),
		"open umask":          strings.Replace(good, "UMask=0022", "UMask=0000", 1),
		"0077 umask":          strings.Replace(good, "UMask=0022", "UMask=0077", 1),
		"runs another binary": strings.Replace(good, "/usr/bin/oncerootd", "/usr/bin/oncerootd --debug", 1),
		"runs as a user":      good + "User=nobody\n",
		"empty file":          "",
	} {
		if p := checkUnit([]byte(bad)); len(p) == 0 {
			t.Errorf("%s passed", name)
		}
	}
}
