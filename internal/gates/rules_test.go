package gates

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vettedAllows are the only allow rules the shipped rules file may carry:
// each lets through read-only invocations and nothing that changes state.
// A new default allow rule must be added here, on purpose, with its reason.
var vettedAllows = map[string]string{
	"allow /usr/bin/pacman -Q* ...":       "pacman takes one operation; -Q queries the local database only",
	"allow /usr/bin/systemctl status ...": "status only reads unit state",
}

func TestShippedRulesAllowOnlyVettedReads(t *testing.T) {
	f, err := os.Open(filepath.Join(repoRoot(t), "packaging", "rules.example"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.Join(strings.Fields(strings.SplitN(sc.Text(), "#", 2)[0]), " ")
		if strings.HasPrefix(line, "allow ") {
			if _, ok := vettedAllows[line]; !ok {
				t.Errorf("shipped allow rule %q is not vetted", line)
			}
		}
	}
}
