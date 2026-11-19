package gates

import (
	"strings"
	"testing"
)

func TestInstallStepsStopBeforePacmanOnFailure(t *testing.T) {
	if p := checkInstallChains(repoRoot(t), InstallDocs); len(p) > 0 {
		t.Fatalf("install docs:\n%s", strings.Join(p, "\n"))
	}
}

func TestInstallChainGateHasTeeth(t *testing.T) {
	good := "```sh\ncd \"$(mktemp -d)\" &&\ngh release download v1 &&\ngh attestation verify x \\\n  --repo r &&\necho 'h  x' | sha256sum -c - &&\nsudo pacman -U ./x\n```\n"
	cases := []struct {
		name string
		doc  string
		fail bool
	}{
		{"clean", good, false},
		{"checksum on its own line", strings.Replace(good, "sha256sum -c - &&", "sha256sum -c -", 1), true},
		{"attestation on its own line", strings.Replace(good, "--repo r &&", "--repo r", 1), true},
		{"pacman outside any block: measured nothing", "sudo pacman -U ./x\n", true},
		{"a block without pacman is not checked", "```sh\necho a\necho b\n```\n" + good, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeTree(t, map[string]string{"README.md": c.doc})
			if got := len(checkInstallChains(dir, []string{"README.md"})) > 0; got != c.fail {
				t.Fatalf("fail=%v, want %v", got, c.fail)
			}
		})
	}
}
