#!/bin/sh
# Plants each gate's own fault in a throwaway copy of the repository and
# requires that gate to fail; then requires every gate to pass on the clean
# copy. A gate that cannot go red is reported, not trusted.
set -u
cd "$(dirname "$0")/.."
root=$(pwd)
fail=0

fresh() {
  dir=$(mktemp -d "${TMPDIR:-/tmp}/onceroot-teeth.XXXXXX")
  (cd "$root" && git ls-files -z --cached --others --exclude-standard | xargs -0 -I{} sh -c 'mkdir -p "$1/$(dirname "{}")" && cp "{}" "$1/{}"' _ "$dir")
  echo "$dir"
}

expect() { # expect red|green <test regex> <dir> <label>
  want=$1 test=$2 dir=$3 label=$4
  if (cd "$dir" && go test -count=1 -run "$test" ./internal/gates/ >/dev/null 2>&1); then got=green; else got=red; fi
  if [ "$got" = "$want" ]; then echo "ok    $label: $got"; else echo "FAIL  $label: $got, want $want"; fail=1; fi
}

d=$(fresh); expect green '.' "$d" "clean copy, every gate"; rm -rf "$d"

d=$(fresh); printf '\nrequire github.com/evil/thing v1.0.0\n' >> "$d/go.mod"
expect red '^TestDependencyAllowlist$' "$d" "deps: foreign require"; rm -rf "$d"

d=$(fresh); sed -i.bak 's|<allow_active>auth_admin</allow_active>|<allow_active>auth_admin_keep</allow_active>|' "$d/packaging/org.taipanbox.onceroot.policy"
expect red '^TestPolicyIsAuthAdminEverywhere$' "$d" "policy: auth_admin_keep"; rm -rf "$d"

d=$(fresh); printf 'package daemon\n\nimport "os/exec"\n\nvar _ = exec.Command\n' > "$d/internal/daemon/stray.go"
expect red '^TestOnlyRunnerAndPolkitImportExec$' "$d" "exec: os/exec in daemon"; rm -rf "$d"

d=$(fresh); sed -i.bak '/^PrivateTmp=yes$/d' "$d/packaging/oncerootd.service"
expect red '^TestUnitHardening$' "$d" "unit: PrivateTmp removed"; rm -rf "$d"

d=$(fresh); sed -i.bak "s|-ldflags '-bindnow'|-ldflags '-s -w -bindnow'|" "$d/packaging/PKGBUILD"
expect red '^TestPackageBuildHardening$' "$d" "pkgbuild: stripped at link time"; rm -rf "$d"

d=$(fresh); sed -i.bak "s|go build -ldflags '-bindnow' -o oncerootd|go build -buildmode=exe -ldflags '-bindnow' -o oncerootd|" "$d/packaging/PKGBUILD"
expect red '^TestPackageBuildHardening$' "$d" "pkgbuild: one build not PIE"; rm -rf "$d"

d=$(fresh); printf '\n  @test:TestNobodyWroteThis\n  Scenario: ghost\n    Given nothing\n' >> "$d/features/card.feature"
expect red '^TestScenariosBoundToTests$' "$d" "bindings: tag naming no test"; rm -rf "$d"

d=$(fresh); rm -f "$d"/features/*.feature
expect red '^TestScenariosBoundToTests$' "$d" "bindings: no features measured nothing"; rm -rf "$d"

d=$(fresh); sed -i.bak 's|"id": "taipanbox.onceroot"|"id": "omarchy.onceroot"|' "$d/omarchy-plugin/manifest.json"
expect red '^TestOmarchyPluginManifest$' "$d" "plugin: reserved omarchy id"; rm -rf "$d"

d=$(fresh); awk '/textFormat: Text.PlainText/ && !done { done=1; next } { print }' "$d/omarchy-plugin/BarWidget.qml" > "$d/w" && mv "$d/w" "$d/omarchy-plugin/BarWidget.qml"
expect red '^TestOmarchyPluginTextIsPlain$' "$d" "plugin: one Text left as AutoText"; rm -rf "$d"

d=$(fresh); sed -i.bak 's|Util.execArgv(argv)|Util.execDetached(argv.join(" "))|' "$d/omarchy-plugin/BarWidget.qml"
expect red '^TestOmarchyPluginTextIsPlain$' "$d" "plugin: command through a shell string"; rm -rf "$d"

d=$(fresh); sed -i.bak '/"card.reject"/d' "$d/omarchy-plugin/i18n/uk.json"
expect red '^TestOmarchyPluginTranslations$' "$d" "plugin: Ukrainian string missing"; rm -rf "$d"

d=$(fresh); sed -i.bak '/omarchy-plugin\/Model.js/d' "$d/packaging/PKGBUILD"
expect red '^TestOmarchyPluginShipsInPackage$' "$d" "plugin: package leaves out Model.js"; rm -rf "$d"

d=$(fresh); sed -i.bak 's| nodejs  # polkit| # polkit|' "$d/.github/workflows/release.yml"
expect red '^TestWorkflowsInstallNodeForThePlugin$' "$d" "plugin: release workflow without nodejs"; rm -rf "$d"

if command -v node >/dev/null 2>&1; then
  d=$(fresh); sed -i.bak 's|^var ID = .*|var ID = /^[A-Z]{5}$/|' "$d/omarchy-plugin/Model.js"
  expect red '^TestOmarchyPluginModel$' "$d" "plugin: id check lets I and O through"; rm -rf "$d"
  d=$(fresh); sed -i.bak 's|if (r.status !== "held" \|\| !validId(r.id)) continue|if (!validId(r.id)) continue|' "$d/omarchy-plugin/Model.js"
  expect red '^TestOmarchyPluginModel$' "$d" "plugin: finished requests counted as waiting"; rm -rf "$d"
else
  echo "FAIL  plugin model: node not installed, measured nothing"; fail=1
fi

exit $fail
