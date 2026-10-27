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

exit $fail
