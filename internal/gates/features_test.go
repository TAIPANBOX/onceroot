package gates

import (
	"strings"
	"testing"
)

func TestScenariosBoundToTests(t *testing.T) {
	if p := checkBindings(repoRoot(t)); len(p) > 0 {
		t.Fatalf("scenario bindings broken:\n%s", strings.Join(p, "\n"))
	}
}

func TestBindingGateHasTeeth(t *testing.T) {
	const testFile = "package x\n\nfunc TestReal(t *testing.T) {}\n"
	cases := []struct {
		name  string
		files map[string]string
		fail  bool
	}{
		{"untagged scenario", map[string]string{
			"features/a.feature": "Feature: a\n\n  Scenario: nothing binds me\n    Given x\n",
			"x_test.go":          testFile,
		}, true},
		{"tag naming nothing", map[string]string{
			"features/a.feature": "Feature: a\n\n  @test:TestGone\n  Scenario: bound to a ghost\n    Given x\n",
			"x_test.go":          testFile,
		}, true},
		{"feature without scenarios", map[string]string{
			"features/a.feature": "Feature: a\n",
			"x_test.go":          testFile,
		}, true},
		{"no feature files: measured nothing", map[string]string{
			"x_test.go": testFile,
		}, true},
		{"clean", map[string]string{
			"features/a.feature": "Feature: a\n\n  @test:TestReal\n  Scenario: bound\n    Given x\n",
			"x_test.go":          testFile,
		}, false},
	}
	for _, c := range cases {
		dir := writeTree(t, c.files)
		got := len(checkBindings(dir)) > 0
		if got != c.fail {
			t.Errorf("%s: failed=%v, want %v (%v)", c.name, got, c.fail, checkBindings(dir))
		}
	}
}
