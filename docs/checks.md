# How onceroot is checked

Every check here runs in CI on each push, and can be run on your own machine.

## Scenarios bound to tests

Each behaviour is written as a scenario in [features/](../features/), and each
scenario names the test that proves it:

```gherkin
@test:TestCardTooLongForTheDialogIsRefused
Scenario: a request whose card the dialog cannot show whole is refused
  Given an argument wider than the dialog
  Then the request is refused with a reason naming the approval dialog, ...
```

The gate `TestScenariosBoundToTests` fails when a scenario names a test that
does not exist, so the scenarios cannot drift from the code.

## Gates

| Gate | What it holds |
|---|---|
| `TestDependencyAllowlist` | `go.mod` requires nothing outside the allowed list (today: nothing at all) |
| `TestOnlyRunnerAndPolkitImportExec` | only the runner and the polkit check start processes |
| `TestPolicyIsAuthAdminEverywhere` | the polkit action asks every time, for every kind of session |
| `TestUnitHardening` | the unit runs as root with `UMask=0022`, `ProtectHome=read-only`, `PrivateTmp=yes` |
| `TestShippedRulesAllowOnlyVettedReads` | the shipped rules allow only vetted read-only commands |
| `TestPackageBuildHardening` | every `go build` in the PKGBUILD makes PIE with BIND_NOW, without cgo or link-time stripping, and `check()` measures the binaries |
| `TestBuiltBinariesAreHardened` | the binaries a build made are PIE, bound at load time, have RELRO and keep their symbols (run by `check()`) |
| `TestScenariosBoundToTests` | every scenario names a test that exists |

## Gates have teeth

A gate that can never fail proves nothing. `scripts/gates-have-teeth.sh` copies
the repository, plants each gate's own fault (a foreign module, a remembered
polkit approval, `os/exec` in the daemon, `PrivateTmp` removed, a stripped or
non-PIE build, a scenario naming no test, no scenarios at all) and requires that
gate to fail; on the clean copy every gate must pass.

## Mutants

`scripts/mutate.py` breaks the decision path the way a silent wrong answer
would: an approval that skips polkit, an expired card that still runs, a rule
that ignores the program, a journal that skips its signature check, a dialog
card that shows what does not fit, and 50 more. An existing test must fail for
each one. A mutant that does not compile counts against the run.

## Fuzzing

`FuzzRender` and `FuzzRenderDialog` feed any arguments, reason and process name
to the card: no raw control or format character, and nothing outside the shown
set, may reach it, and a dialog card that is shown holds every argument.
`FuzzParse` and `FuzzDecode` do the same for the rules file and the socket's
wire format.

## Running them

```sh
make test       # every test
make teeth      # every gate must go red on its own fault
make mutate     # every mutant must be caught
make fuzz       # 60 seconds per fuzzer
```

The root and end-to-end tests skip without root on Linux. CI runs them as root,
and builds the package in an Arch Linux container with `scripts/package.sh`.
