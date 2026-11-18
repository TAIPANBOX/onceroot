package gates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func pluginDir(t *testing.T) string { return filepath.Join(repoRoot(t), PluginDir) }

func TestOmarchyPluginManifest(t *testing.T) {
	if p := checkPluginManifest(pluginDir(t)); len(p) > 0 {
		t.Fatalf("plugin manifest:\n%s", strings.Join(p, "\n"))
	}
}

func TestOmarchyPluginTextIsPlain(t *testing.T) {
	if p := checkPluginPlainText(pluginDir(t)); len(p) > 0 {
		t.Fatalf("plugin QML:\n%s", strings.Join(p, "\n"))
	}
}

func TestOmarchyPluginTranslations(t *testing.T) {
	if p := checkPluginTranslations(pluginDir(t)); len(p) > 0 {
		t.Fatalf("plugin strings:\n%s", strings.Join(p, "\n"))
	}
}

func TestOmarchyPluginShipsInPackage(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "PKGBUILD"))
	if err != nil {
		t.Fatal(err)
	}
	if p := checkPluginPackaged(b, pluginDir(t)); len(p) > 0 {
		t.Fatalf("PKGBUILD:\n%s", strings.Join(p, "\n"))
	}
}

// TestOmarchyPluginModel runs the widget's model tests (node's own test
// runner, no packages). Where node is missing it skips, except in CI, where a
// skip would let the gate report nothing as a pass.
func TestOmarchyPluginModel(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("node is required in CI to run the plugin model tests")
		}
		t.Skip("node not installed")
	}
	tests, _ := filepath.Glob(filepath.Join(pluginDir(t), "test", "*.test.js"))
	if len(tests) == 0 {
		t.Fatal("no plugin model tests: measured nothing")
	}
	// TAP on purpose: node's default reporter depends on its version and on
	// whether stdout is a terminal, and only TAP has a fixed summary to read.
	cmd := exec.Command(node, append([]string{"--test", "--test-reporter=tap"}, tests...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin model tests failed: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "\n# fail 0\n") || strings.Contains(s, "\n# pass 0\n") || !strings.Contains(s, "\n# pass ") {
		t.Fatalf("plugin model tests did not report passes and 0 failures:\n%s", out)
	}
}

const goodManifest = `{"schemaVersion":1,"id":"acme.thing","name":"Thing","version":"1.0.0",
"kinds":["bar-widget"],"entryPoints":{"barWidget":"W.qml"},
"barWidget":{"displayName":"Thing","allowMultiple":false}}`

func TestPluginManifestGateHasTeeth(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		fail  bool
	}{
		{"clean", map[string]string{"manifest.json": goodManifest, "W.qml": "Item {}"}, false},
		{"no manifest: measured nothing", map[string]string{"W.qml": "Item {}"}, true},
		{"reserved id", map[string]string{"manifest.json": strings.Replace(goodManifest, "acme.thing", "omarchy.thing", 1), "W.qml": ""}, true},
		{"schema 2", map[string]string{"manifest.json": strings.Replace(goodManifest, `"schemaVersion":1`, `"schemaVersion":2`, 1), "W.qml": ""}, true},
		{"entry point missing", map[string]string{"manifest.json": goodManifest}, true},
		{"entry point escapes", map[string]string{"manifest.json": strings.Replace(goodManifest, `"W.qml"`, `"../W.qml"`, 1)}, true},
		{"no bar-widget kind", map[string]string{"manifest.json": strings.Replace(goodManifest, `["bar-widget"]`, `["panel"]`, 1), "W.qml": ""}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(checkPluginManifest(writeTree(t, c.files))) > 0; got != c.fail {
				t.Fatalf("fail=%v, want %v", got, c.fail)
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		dir := writeTree(t, map[string]string{"manifest.json": goodManifest, "W.qml": ""})
		if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
		if len(checkPluginManifest(dir)) == 0 {
			t.Fatal("a symlink passed")
		}
	})
}

func TestPluginPlainTextGateHasTeeth(t *testing.T) {
	cases := []struct {
		name string
		qml  map[string]string
		fail bool
	}{
		{"clean", map[string]string{"W.qml": "Item {\n  Text {\n    textFormat: Text.PlainText\n    text: x\n  }\n  Button { onClicked: Util.execArgv([\"a\", id]) }\n}\n"}, false},
		{"no QML: measured nothing", map[string]string{"x.js": ""}, true},
		{"AutoText", map[string]string{"W.qml": "Item {\n  Text {\n    text: card\n  }\n}\n"}, true},
		{"PlainText only in a child", map[string]string{"W.qml": "Text {\n  text: card\n  Item { property string f: \"textFormat: Text.PlainText\" }\n}\n"}, true},
		{"one Text plain, the next not", map[string]string{"W.qml": "Item {\n  Text { textFormat: Text.PlainText; text: a }\n  Text { text: card }\n}\n"}, true},
		{"shell string", map[string]string{"W.qml": "Item { Component.onCompleted: Util.execDetached(\"onceroot approve \" + id) }"}, true},
		{"bar.run", map[string]string{"W.qml": "Item { function f() { root.bar.run(\"x\") } }"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(checkPluginPlainText(writeTree(t, c.qml))) > 0; got != c.fail {
				t.Fatalf("fail=%v, want %v", got, c.fail)
			}
		})
	}
}

func TestPluginTranslationsGateHasTeeth(t *testing.T) {
	qml := "Item { Text { textFormat: Text.PlainText; text: strings.t(\"a\") + strings.tn(\"b\", n) } }"
	cases := []struct {
		name  string
		files map[string]string
		fail  bool
	}{
		{"clean", map[string]string{"W.qml": qml, "i18n/en.json": `{"a":"A","b":{"one":"1","other":"n"}}`, "i18n/uk.json": `{"a":"А","b":{"one":"1","other":"n"}}`}, false},
		{"no calls: measured nothing", map[string]string{"W.qml": "Item {}", "i18n/en.json": `{}`, "i18n/uk.json": `{}`}, true},
		{"key missing in English", map[string]string{"W.qml": qml, "i18n/en.json": `{"a":"A"}`, "i18n/uk.json": `{"a":"А"}`}, true},
		{"key missing in Ukrainian", map[string]string{"W.qml": qml, "i18n/en.json": `{"a":"A","b":"B"}`, "i18n/uk.json": `{"a":"А"}`}, true},
		{"extra Ukrainian key", map[string]string{"W.qml": qml, "i18n/en.json": `{"a":"A","b":"B"}`, "i18n/uk.json": `{"a":"А","b":"Б","c":"В"}`}, true},
		{"no uk.json", map[string]string{"W.qml": qml, "i18n/en.json": `{"a":"A","b":"B"}`}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(checkPluginTranslations(writeTree(t, c.files))) > 0; got != c.fail {
				t.Fatalf("fail=%v, want %v", got, c.fail)
			}
		})
	}
}

func TestPluginPackagedGateHasTeeth(t *testing.T) {
	dir := writeTree(t, map[string]string{"manifest.json": "{}", "i18n/en.json": "{}", "test/m.test.js": ""})
	good := `  install -Dm644 omarchy-plugin/manifest.json "$pkgdir/usr/share/onceroot/omarchy-plugin/manifest.json"
  install -Dm644 omarchy-plugin/i18n/en.json "$pkgdir/usr/share/onceroot/omarchy-plugin/i18n/en.json"
`
	if p := checkPluginPackaged([]byte(good), dir); len(p) > 0 {
		t.Fatalf("clean PKGBUILD failed: %v", p)
	}
	missing := strings.SplitN(good, "\n", 2)[0] + "\n"
	if len(checkPluginPackaged([]byte(missing), dir)) == 0 {
		t.Fatal("a PKGBUILD without i18n/en.json passed")
	}
	if len(checkPluginPackaged([]byte(good), t.TempDir())) == 0 {
		t.Fatal("an empty plugin folder passed: should say measured nothing")
	}
}

func TestWorkflowsInstallNodeForThePlugin(t *testing.T) {
	if p := checkWorkflowsInstallNode(repoRoot(t)); len(p) > 0 {
		t.Fatalf("workflows:\n%s", strings.Join(p, "\n"))
	}
}

func TestWorkflowsInstallNodeGateHasTeeth(t *testing.T) {
	build := "      - run: su builder -c \"scripts/package.sh\"\n"
	cases := []struct {
		name  string
		files map[string]string
		fail  bool
	}{
		{"clean", map[string]string{".github/workflows/a.yml": "      - run: pacman -Syu --noconfirm git go nodejs\n" + build}, false},
		{"no nodejs", map[string]string{".github/workflows/a.yml": "      - run: pacman -Syu --noconfirm git go\n" + build}, true},
		{"nodejs only in a comment", map[string]string{".github/workflows/a.yml": "      - run: pacman -Syu --noconfirm git go  # nodejs: the plugin tests\n" + build}, true},
		{"nodejs-lts is another package", map[string]string{".github/workflows/a.yml": "      - run: pacman -Syu --noconfirm git go nodejsx\n" + build}, true},
		{"one of two lacks it", map[string]string{
			".github/workflows/a.yml": "      - run: pacman -Syu --noconfirm git go nodejs\n" + build,
			".github/workflows/b.yml": "      - run: pacman -Syu --noconfirm git go\n" + build,
		}, true},
		{"no package build: measured nothing", map[string]string{".github/workflows/a.yml": "      - run: echo hi\n"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(checkWorkflowsInstallNode(writeTree(t, c.files))) > 0; got != c.fail {
				t.Fatalf("fail=%v, want %v", got, c.fail)
			}
		})
	}
}
