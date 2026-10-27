package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// I13: the polkit action is auth_admin for every subject class.
func TestPolicyIsAuthAdminEverywhere(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "org.taipanbox.onceroot.policy"))
	if err != nil {
		t.Fatal(err)
	}
	if p := checkPolicy(b); len(p) > 0 {
		t.Fatalf("policy:\n%s", strings.Join(p, "\n"))
	}
}

func TestPolicyGateHasTeeth(t *testing.T) {
	good := `<policyconfig><action id="org.taipanbox.onceroot.run"><defaults><allow_any>auth_admin</allow_any><allow_inactive>auth_admin</allow_inactive><allow_active>auth_admin</allow_active></defaults></action></policyconfig>`
	if p := checkPolicy([]byte(good)); len(p) > 0 {
		t.Fatalf("a correct policy failed: %v", p)
	}
	for name, bad := range map[string]string{
		"keep":           strings.Replace(good, "<allow_active>auth_admin<", "<allow_active>auth_admin_keep<", 1),
		"yes":            strings.Replace(good, "<allow_any>auth_admin<", "<allow_any>yes<", 1),
		"missing class":  strings.Replace(good, "<allow_inactive>auth_admin</allow_inactive>", "", 1),
		"other action":   strings.Replace(good, "org.taipanbox.onceroot.run", "org.example.run", 1),
		"second action":  strings.Replace(good, "</policyconfig>", `<action id="org.taipanbox.onceroot.other"><defaults><allow_active>yes</allow_active></defaults></action></policyconfig>`, 1),
		"not xml at all": "nothing here",
	} {
		if p := checkPolicy([]byte(bad)); len(p) == 0 {
			t.Errorf("%s passed", name)
		}
	}
}
