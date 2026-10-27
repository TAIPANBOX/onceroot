// Package gates holds the repository's own checks: what the code may depend
// on and whether every scenario is bound to a test. The checkers are plain
// functions so each gate's tests can plant the fault it exists to catch.
package gates

import (
	"bufio"
	"bytes"
	"debug/elf"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// allowedModules is the whole dependency allowlist (I15).
var allowedModules = map[string]bool{
	"golang.org/x/sys": true,
}

// checkDeps reports every require outside the allowlist and every replace
// whose target is not a local directory.
func checkDeps(goMod []byte) []string {
	var problems []string
	block := ""
	sc := bufio.NewScanner(bytes.NewReader(goMod))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if block != "" {
			if line == ")" {
				block = ""
				continue
			}
			problems = append(problems, checkDirective(block, line, n)...)
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "require", "replace":
			if len(fields) == 2 && fields[1] == "(" {
				block = fields[0]
				continue
			}
			problems = append(problems, checkDirective(fields[0], strings.Join(fields[1:], " "), n)...)
		}
	}
	return problems
}

func checkDirective(kind, rest string, line int) []string {
	switch kind {
	case "require":
		mod := strings.Fields(rest)[0]
		if !allowedModules[mod] {
			return []string{fmt.Sprintf("go.mod:%d: require %s is not in the allowlist", line, mod)}
		}
	case "replace":
		parts := strings.SplitN(rest, "=>", 2)
		if len(parts) != 2 {
			return []string{fmt.Sprintf("go.mod:%d: unreadable replace %q", line, rest)}
		}
		target := strings.Fields(parts[1])
		if len(target) == 0 || !isLocalPath(target[0]) {
			return []string{fmt.Sprintf("go.mod:%d: replace to a non-local path %q", line, strings.TrimSpace(parts[1]))}
		}
	}
	return nil
}

func isLocalPath(p string) bool {
	return strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/")
}

var (
	testFuncRE = regexp.MustCompile(`(?m)^func ((?:Test|Fuzz)[A-Za-z0-9_]*)\(`)
	scenarioRE = regexp.MustCompile(`^\s*Scenario(?: Outline)?:`)
)

// checkBindings reports every scenario in root/features/*.feature that has
// no @test tag, every tag naming a test that does not exist, every feature
// file without a scenario, and the absence of feature files, which would
// otherwise read as a clean run that measured nothing.
func checkBindings(root string) []string {
	features, _ := filepath.Glob(filepath.Join(root, "features", "*.feature"))
	if len(features) == 0 {
		return []string{"measured nothing: no features/*.feature files"}
	}
	tests, err := testFuncs(root)
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	for _, f := range features {
		problems = append(problems, checkFeature(f, tests)...)
	}
	return problems
}

func checkFeature(path string, tests map[string]bool) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	name := filepath.Base(path)
	var problems []string
	var pending []string
	scenarios := 0
	for n, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@") {
			for tag := range strings.FieldsSeq(trimmed) {
				if name, ok := strings.CutPrefix(tag, "@test:"); ok {
					pending = append(pending, name)
				}
			}
			continue
		}
		if !scenarioRE.MatchString(line) {
			continue
		}
		scenarios++
		if len(pending) == 0 {
			problems = append(problems, fmt.Sprintf("%s:%d: scenario without an @test tag", name, n+1))
		}
		for _, t := range pending {
			if !tests[t] {
				problems = append(problems, fmt.Sprintf("%s:%d: @test:%s names no test", name, n+1, t))
			}
		}
		pending = nil
	}
	if scenarios == 0 {
		problems = append(problems, fmt.Sprintf("%s: no scenarios", name))
	}
	sort.Strings(problems)
	return problems
}

// testFuncs collects the names of Test and Fuzz functions in every _test.go
// file under root, skipping VCS and tool state directories.
func testFuncs(root string) (map[string]bool, error) {
	found := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".superpowers", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range testFuncRE.FindAllSubmatch(b, -1) {
			found[string(m[1])] = true
		}
		return nil
	})
	return found, err
}

// policyAction is the only action onceroot's polkit policy may define.
const policyAction = "org.taipanbox.onceroot.run"

type policyXML struct {
	Actions []struct {
		ID       string `xml:"id,attr"`
		Defaults struct {
			Any      string `xml:"allow_any"`
			Inactive string `xml:"allow_inactive"`
			Active   string `xml:"allow_active"`
		} `xml:"defaults"`
	} `xml:"action"`
}

// checkPolicy requires exactly onceroot's action, with auth_admin for every
// subject class: never auth_admin_keep, never yes (I13).
func checkPolicy(b []byte) []string {
	var p policyXML
	if err := xml.Unmarshal(b, &p); err != nil {
		return []string{"policy is not readable XML: " + err.Error()}
	}
	if len(p.Actions) != 1 || p.Actions[0].ID != policyAction {
		return []string{fmt.Sprintf("policy must define exactly the action %s", policyAction)}
	}
	d := p.Actions[0].Defaults
	var problems []string
	for name, v := range map[string]string{"allow_any": d.Any, "allow_inactive": d.Inactive, "allow_active": d.Active} {
		if strings.TrimSpace(v) != "auth_admin" {
			problems = append(problems, fmt.Sprintf("%s is %q, must be auth_admin", name, v))
		}
	}
	sort.Strings(problems)
	return problems
}

// execAllowed are the only packages that may start processes. internal/gates
// is skipped because it holds the pattern itself.
var execAllowed = []string{"internal/runner/", "internal/polkit/", "internal/gates/"}

var execUse = regexp.MustCompile(`"os/exec"|os\.StartProcess\(|syscall\.(Exec|ForkExec|StartProcess)\(`)

// checkExec reports non-test Go files outside runner and polkit that import
// os/exec or call os.StartProcess, syscall.Exec or syscall.ForkExec.
func checkExec(root string) []string {
	var problems []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".superpowers" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		for _, a := range execAllowed {
			if strings.HasPrefix(rel, a) {
				return nil
			}
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if execUse.Match(b) {
			problems = append(problems, rel+": starts processes")
		}
		return nil
	})
	sort.Strings(problems)
	return problems
}

// unitWant are the [Service] directives oncerootd.service must carry. The
// daemon runs as root and must run programs that write /usr and /etc, so
// ProtectSystem and NoNewPrivileges are deliberately absent. UMask is root's
// usual 0022 because approved commands inherit it: under 0077 a file an
// approved `cp` puts in /etc came out 0600 (VM run 2026-10-27). oncerootd sets
// the modes of its own files explicitly.
var unitWant = map[string]string{
	"ExecStart":   "/usr/bin/oncerootd",
	"UMask":       "0022",
	"ProtectHome": "read-only",
	"PrivateTmp":  "yes",
	"Restart":     "on-failure",
}

func checkUnit(b []byte) []string {
	got := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			got[k] = v
		}
	}
	var problems []string
	for k, v := range unitWant {
		if got[k] != v {
			problems = append(problems, fmt.Sprintf("%s=%q, want %q", k, got[k], v))
		}
	}
	for _, k := range []string{"User", "Group", "DynamicUser"} {
		if _, ok := got[k]; ok {
			problems = append(problems, k+" must not be set: oncerootd runs as root")
		}
	}
	sort.Strings(problems)
	return problems
}

// buildWant are what build() in PKGBUILD must pass to go: PIE and a trimmed
// path for every build. namcap asks PIE and BIND_NOW of every Arch binary;
// onceroot gets them without cgo, so the root daemon links no C and loads no
// NSS modules (namcap 3.6.0 in the Omarchy VM, 2026-10-27). The binaries
// themselves are measured by checkELF in check().
var buildWant = []string{"-buildmode=pie", "-trimpath"}

var (
	// linkStrip finds -s or -w in any spelling (-s, --s, -s=true):
	// stripping at link time leaves the debug package with links to nothing,
	// and makepkg strips and keeps the symbols itself.
	linkStrip = regexp.MustCompile(`(?:^|[\s'"=])--?[sw](?:=\S*)?(?:[\s'"]|$)`)
	buildMode = regexp.MustCompile(`-buildmode[= ]['"]?([^\s'"]+)`)
	cgoSet    = regexp.MustCompile(`CGO_ENABLED=['"]?([^\s'"]*)`)
)

func checkPKGBUILD(b []byte) []string {
	src := string(b)
	build, check := shellFunc(src, "build"), shellFunc(src, "check")
	var problems []string
	for _, w := range buildWant {
		if !strings.Contains(build, w) {
			problems = append(problems, "build() lacks "+w)
		}
	}
	if set := cgoSet.FindAllStringSubmatch(build, -1); len(set) == 0 || set[len(set)-1][1] != "0" {
		problems = append(problems, "build() does not end with CGO_ENABLED=0")
	}
	builds := 0
	for _, line := range strings.Split(build, "\n") {
		if !strings.Contains(line, "go build") {
			continue
		}
		builds++
		for _, m := range buildMode.FindAllStringSubmatch(line, -1) {
			if m[1] != "pie" {
				problems = append(problems, "a go build line sets -buildmode="+m[1])
			}
		}
		if !strings.Contains(line, "-bindnow") {
			problems = append(problems, "a go build line lacks -bindnow")
		}
	}
	if builds == 0 {
		problems = append(problems, "build() runs no go build")
	}
	if linkStrip.MatchString(build) {
		problems = append(problems, "build() strips at link time (-s or -w)")
	}
	if !strings.Contains(check, "go test ./...") {
		problems = append(problems, "check() does not run go test ./...")
	}
	if !strings.Contains(check, "ONCEROOT_CHECK_BIN=") || !strings.Contains(check, "TestBuiltBinariesAreHardened") {
		problems = append(problems, "check() does not measure the built binaries")
	}
	if !regexp.MustCompile(`(?m)^depends=\(.*'polkit'`).MatchString(src) {
		problems = append(problems, "depends lacks polkit: oncerootd runs pkcheck")
	}
	return problems
}

// shellFunc returns the body of the last PKGBUILD function written as
// name() { ... } with its closing brace alone on a line, the one bash keeps,
// or "" when there is none.
func shellFunc(src, name string) string {
	head := "\n" + name + "() {\n"
	i := strings.LastIndex(src, head)
	if i < 0 {
		return ""
	}
	body, _, _ := strings.Cut(src[i+len(head):], "\n}")
	return body
}

// checkELF measures a built binary: position independent (ET_DYN with
// DF_1_PIE), bound at load time (BIND_NOW), with a RELRO segment, and not
// stripped at the link, so makepkg can split its debug package.
func checkELF(path string) []string {
	f, err := elf.Open(path)
	if err != nil {
		return []string{"cannot read it as ELF: " + err.Error()}
	}
	defer f.Close()
	has := func(tag elf.DynTag, bit uint64) bool {
		vals, _ := f.DynValue(tag)
		for _, v := range vals {
			if v&bit != 0 {
				return true
			}
		}
		return false
	}
	var problems []string
	if f.Type != elf.ET_DYN || !has(elf.DT_FLAGS_1, uint64(elf.DF_1_PIE)) {
		problems = append(problems, "not position independent (PIE)")
	}
	if !has(elf.DT_FLAGS, uint64(elf.DF_BIND_NOW)) && !has(elf.DT_FLAGS_1, uint64(elf.DF_1_NOW)) {
		problems = append(problems, "not bound at load time (BIND_NOW)")
	}
	relro := false
	for _, p := range f.Progs {
		relro = relro || p.Type == elf.PT_GNU_RELRO
	}
	if !relro {
		problems = append(problems, "no RELRO segment")
	}
	if f.Section(".symtab") == nil || f.Section(".debug_info") == nil {
		problems = append(problems, "stripped at link time: no symbols or no DWARF")
	}
	return problems
}
