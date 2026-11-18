package gates

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PluginDir is where the Omarchy shell plugin lives in the repository.
const PluginDir = "omarchy-plugin"

// PluginInstallDir is where the package puts it.
const PluginInstallDir = "usr/share/onceroot/omarchy-plugin"

// checkPluginManifest checks the parts of Omarchy's plugin contract that
// `omarchy plugin validate` enforces at load time: schema 1, an id outside
// the reserved omarchy namespace, the bar-widget kind with an entry point
// that exists inside the folder, and no symlinks anywhere.
func checkPluginManifest(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return []string{"manifest.json: measured nothing: " + err.Error()}
	}
	var m struct {
		SchemaVersion int               `json:"schemaVersion"`
		ID            string            `json:"id"`
		Name          string            `json:"name"`
		Version       string            `json:"version"`
		Kinds         []string          `json:"kinds"`
		EntryPoints   map[string]string `json:"entryPoints"`
		BarWidget     *struct {
			DisplayName   string `json:"displayName"`
			AllowMultiple *bool  `json:"allowMultiple"`
		} `json:"barWidget"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return []string{"manifest.json: " + err.Error()}
	}
	var p []string
	if m.SchemaVersion != 1 {
		p = append(p, fmt.Sprintf("schemaVersion is %d, want 1", m.SchemaVersion))
	}
	if !regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9-]+$`).MatchString(m.ID) {
		p = append(p, fmt.Sprintf("id %q is not <owner>.<name>", m.ID))
	}
	if strings.HasPrefix(m.ID, "omarchy.") {
		p = append(p, fmt.Sprintf("id %q is in the reserved omarchy namespace", m.ID))
	}
	if m.Name == "" || m.Version == "" {
		p = append(p, "name and version are required")
	}
	hasKind := false
	for _, k := range m.Kinds {
		if k == "bar-widget" {
			hasKind = true
		}
	}
	if !hasKind {
		p = append(p, "kinds lacks bar-widget")
	}
	entry := m.EntryPoints["barWidget"]
	switch {
	case entry == "":
		p = append(p, "entryPoints.barWidget is missing")
	case filepath.IsAbs(entry) || strings.Contains(entry, ".."):
		p = append(p, fmt.Sprintf("entryPoints.barWidget %q is not a safe relative path", entry))
	default:
		if _, err := os.Stat(filepath.Join(dir, entry)); err != nil {
			p = append(p, fmt.Sprintf("entryPoints.barWidget %q does not exist", entry))
		}
	}
	if m.BarWidget == nil || m.BarWidget.DisplayName == "" || m.BarWidget.AllowMultiple == nil {
		p = append(p, "barWidget needs displayName and allowMultiple")
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(dir, path)
			p = append(p, "symlink in the plugin folder: "+rel)
		}
		return nil
	})
	return p
}

var (
	textElement   = regexp.MustCompile(`\bText\s*\{`)
	shellRunners  = regexp.MustCompile(`\b(execDetached|bar\.run|Quickshell\.execDetached)\s*\(`)
	stringKeyCall = regexp.MustCompile(`strings\.tn?\(\s*"([^"]+)"`)
)

// block returns the text from the brace at open to its matching brace.
func block(s string, open int) string {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[open : i+1]
			}
		}
	}
	return s[open:]
}

// ownProps keeps only the text at the element's own level of a block from
// block(), so a property set on a child does not count for its parent.
func ownProps(b string) string {
	var out strings.Builder
	depth := 0
	for _, r := range b {
		switch r {
		case '{':
			depth++
			continue
		case '}':
			depth--
			continue
		}
		if depth == 1 {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// checkPluginPlainText requires every QML Text element to set
// textFormat: Text.PlainText (Qt's default AutoText reads markup, and a card
// carries whatever the agent sent) and forbids running commands through a
// shell string: commands go through Util.execArgv, whose arguments a shell
// never re-reads.
func checkPluginPlainText(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.qml"))
	if len(files) == 0 {
		return []string{"no QML files: measured nothing"}
	}
	var p []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		s := string(b)
		name := filepath.Base(f)
		for _, loc := range textElement.FindAllStringIndex(s, -1) {
			if !strings.Contains(ownProps(block(s, loc[1]-1)), "textFormat: Text.PlainText") {
				line := strings.Count(s[:loc[0]], "\n") + 1
				p = append(p, fmt.Sprintf("%s:%d: Text without textFormat: Text.PlainText", name, line))
			}
		}
		for _, loc := range shellRunners.FindAllStringIndex(s, -1) {
			line := strings.Count(s[:loc[0]], "\n") + 1
			p = append(p, fmt.Sprintf("%s:%d: command through a shell string; use Util.execArgv", name, line))
		}
	}
	return p
}

func jsonKeys(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for k := range m {
		keys[k] = true
	}
	return keys, nil
}

// checkPluginTranslations requires every key the QML asks for to be in
// en.json, and uk.json to carry exactly the keys en.json has.
func checkPluginTranslations(dir string) []string {
	used := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.qml"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, m := range stringKeyCall.FindAllStringSubmatch(string(b), -1) {
			used[m[1]] = true
		}
	}
	if len(used) == 0 {
		return []string{"no strings.t/tn calls in the QML: measured nothing"}
	}
	en, err := jsonKeys(filepath.Join(dir, "i18n", "en.json"))
	if err != nil {
		return []string{"i18n/en.json: " + err.Error()}
	}
	uk, err := jsonKeys(filepath.Join(dir, "i18n", "uk.json"))
	if err != nil {
		return []string{"i18n/uk.json: " + err.Error()}
	}
	var p []string
	for k := range used {
		if !en[k] {
			p = append(p, "en.json lacks "+k)
		}
	}
	for k := range en {
		if !uk[k] {
			p = append(p, "uk.json lacks "+k)
		}
	}
	for k := range uk {
		if !en[k] {
			p = append(p, "uk.json has "+k+", which en.json does not")
		}
	}
	sort.Strings(p)
	return p
}

// checkPluginPackaged requires the PKGBUILD to install every plugin file
// under PluginInstallDir.
func checkPluginPackaged(pkgbuild []byte, dir string) []string {
	var rels []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "test/") {
			return nil
		}
		rels = append(rels, rel)
		return nil
	})
	if len(rels) == 0 {
		return []string{"no plugin files: measured nothing"}
	}
	s := string(pkgbuild)
	var p []string
	for _, rel := range rels {
		if !strings.Contains(s, fmt.Sprintf("%s/%s ", PluginDir, rel)) || !strings.Contains(s, "/"+PluginInstallDir+"/"+rel+`"`) {
			p = append(p, "PKGBUILD does not install "+PluginDir+"/"+rel)
		}
	}
	return p
}
