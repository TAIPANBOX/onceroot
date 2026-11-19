package gates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallDocs are the documents that tell a person how to install onceroot.
var InstallDocs = []string{"README.md", "docs/install.md", "omarchy-plugin/README.md"}

// checkInstallChains requires every shell block that installs with pacman -U
// to reach pacman only if every command before it succeeded: each line before
// the pacman line ends in "&&" or continues with "\". Pasted into a shell,
// separate lines would install after a failed attestation or checksum.
func checkInstallChains(root string, docs []string) []string {
	var p []string
	blocks := 0
	for _, doc := range docs {
		b, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			p = append(p, doc+": "+err.Error())
			continue
		}
		lines := strings.Split(string(b), "\n")
		in, start := false, 0
		for i, line := range lines {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "```") {
				if !in && strings.HasPrefix(t, "```sh") {
					in, start = true, i+1
				} else if in {
					in = false
				}
				continue
			}
			if !in || !strings.Contains(t, "pacman -U") {
				continue
			}
			blocks++
			for j := start; j < i; j++ {
				prev := strings.TrimSpace(lines[j])
				if prev == "" || strings.HasSuffix(prev, "&&") || strings.HasSuffix(prev, `\`) {
					continue
				}
				p = append(p, fmt.Sprintf("%s:%d: %q does not chain into pacman -U on line %d", doc, j+1, prev, i+1))
			}
		}
	}
	if blocks == 0 {
		return []string{"no shell block installs with pacman -U: measured nothing"}
	}
	return p
}
