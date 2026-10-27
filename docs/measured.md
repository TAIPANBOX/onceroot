# What has been run, and what has not

Until 2026-10-27 the project was called askroot; the runs below were made
under that name and are shown with the current names of commands and files.

## 2026-10-27, MacBook (darwin/arm64), go1.27.3, branch feat/v1

| What | Command | Result |
|---|---|---|
| Unit and scenario tests | `go test -race -count=1 ./...` | 162 test runs pass, 0 fail |
| Scenarios bound to tests | `go test ./internal/gates/ -run TestScenariosBoundToTests` | 16 feature files, 151 scenarios, each bound to an existing test |
| Daemon stability | `go test -race -count=5 ./internal/daemon/` | pass |
| Fuzzing, 60 s each | `go test -fuzz FuzzRender`, `FuzzParse`, `FuzzDecode` | 7.6 M, 1.5 M and (60 s) executions, no failure |
| Gate teeth | `scripts/gates-have-teeth.sh` | 7 of 7: clean copy green; foreign require, auth_admin_keep, os/exec in daemon, PrivateTmp removed, a tag naming no test, no feature files: each red |
| Mutation testing | `python3 -I scripts/mutate.py` | 35 caught, 0 survived, 0 invalid (list below; four added after the final review) |
| Coverage | `go test -coverprofile` | 77.3 % of statements in total; packages 77 to 95 %; cmd/onceroot 39 %; cmd/oncerootd 0 % (glue, see below) |
| Linux build | `GOOS=linux GOARCH=amd64/arm64 go build ./cmd/...` | builds; nothing run on Linux |

### Mutants and the tests that caught them

card: approval ignores the hash (TestApproveNeedsMatchingHash); approval ignores expiry (TestExpiredCardCannotBeApproved); run ignores expiry (TestExpiredCardCannotRun); run leaves the card approved (TestApprovedCardRunsOnce); hash skips argv (TestHashCoversEveryField, TestHashSeparatesArguments); argv shown unquoted (FuzzRender, TestRenderQuotesAwkwardArgs).
rules: built-in deny skipped (TestBuiltinDenyBeatsAllowRule); argument count ignored (TestArgCountMustMatchWithoutEllipsis); path ignored (TestRuleMatchesTheResolvedPathOnly).
store: open limit off by one (TestOpenLimit); pause off by one (TestPauseAfterRejectionsInARow).
factor: PIN not checked (TestWrongPINDoesNotSpendTheCode); spent step accepted again (TestCodeSpentOnce, TestSpentStepSurvivesRestart); lockout off by one (TestLockoutAfterFiveFailures and two more).
journal: signature not checked (TestVerifyFindsAChangedByte, TestVerifyWithAnotherKeyFails); sequence not checked (TestVerifyRefusesASignedSequenceGap); chain link not checked (TestVerifyRefusesASignedBrokenLink); secret keys allowed (TestDetailNeverHoldsSecrets).
polkit: any exit approves (TestOnlyExitZeroApproves).
daemon: approve skips polkit (TestSocketCannotApprove); uid not checked (TestForeignUIDRefused); binary not re-checked (TestBinaryChangedBeforeRunRefuses); journal errors ignored (TestJournalFailureRefuses); desk refusal rejects with the phone on (TestNoDeskAgentFallsBackToPhone); broken rules ignored (TestBrokenRulesRefuse).
resolve: writable files trusted (TestResolveRefusesWhatOthersCanWrite). runner: requester env passed (TestRunnerEnvIsFixed).
daemon (after review): dialog opens in review mode (TestReviewModeNeverOpensTheDialogOnRun); list shows every uid (TestListShowsOnlyOwnCards); reject ignores the uid (TestRejectRefusesAnotherUsersCard). mcp: cancellation ignored (TestCancelledCallStopsTheRelay).
phone: any chat heard (TestOnlyConfiguredChatIsRead); secret message kept (TestSecretMessageDeletedFirst); stale card hash accepted (TestStaleHashRefused). wire: NUL bytes accepted (TestDecodeRefusesOutOfBounds).

The first run had two survivors, the journal's sequence and link checks,
which covered for each other; two tests that break exactly one of them were
added and the second run caught both.

## 2026-10-27, Omarchy 4.0.6 in a QEMU/KVM VM on a Debian 13 host

ISO `omarchy-4.0.6.iso` (6.45 GB) from iso.omarchy.org, signature checked with
gpg against key 40DFB630FF42BCFFB047046CF0134EE680CAC571 (GOODSIG), installed
unattended from a `cidata` drive; guest kernel 7.2.8-9-omarchy, 2 vCPU, 4 GB.
onceroot from feat/v1 at f8866c0 (binaries cross-built on the Mac), installed by
the install steps of the PKGBUILD's package(), not by makepkg.

| What | Result |
|---|---|
| All 18 test binaries (`go test -c` for linux/amd64), run as a normal user | all pass, including TestFromConnReadsPeerCreds against the real kernel |
| `systemctl enable --now oncerootd` | active; socket 0660 root:onceroot; journal 0640 root:onceroot; journal.pub 0644; journal.key 0600 |
| `onceroot status` | passwordless sudo off, phone off, 0 open |
| `onceroot run -- pacman -Q linux-omarchy` | allowed by the shipped rule, ran without asking, exit 0 |
| `onceroot run -- bash -c id`, `onceroot run -- ./x.sh` | refused with the built-in and relative-path reasons, exit 126 |
| A request typed in a terminal (foot) inside the Omarchy session | Omarchy's polkit dialog showed the whole card: program, arguments, folder, the chain onceroot <- bash <- foot <- Hyprland, the risk label and the agent's words |
| Approve in the dialog (test password) | "approve (approved by polkit)", the command ran once, file created by root, `onceroot list` shows `ran` |
| Escape in the dialog | "rejected", nothing ran |
| `onceroot journal verify` as a member of group onceroot | verified (18 entries at the end) |

Found by the VM run and fixed:

- With binaries built before the review fixes, journal and journal.pub came
  out 0600 under the service's UMask: review finding 3, reproduced live; the
  fixed build gives 0640 and 0644.
- Approved commands inherited UMask=0077, so a file they created was 0600 (a
  `cp` of a config into /etc would be unreadable for non-root services). The
  unit now sets UMask=0022 (gate TestUnitHardening, red first); a new file
  came out 0644 in the VM.
- The polkit dialog does not grow with a multi-line message: the last card
  lines overlap the password field. Fixed in the second pass below.
- The installer of Omarchy 4.0.6 wants kernel `linux-omarchy` in the cidata
  configuration; the older lab format with `linux` stops at "no kernel
  headers". Under Incus the live ISO stalled in cloud-init; plain QEMU works.

## 2026-10-27, second pass: package, bar, review, dialog and MCP

Same VM, onceroot from feat/v1, built by `makepkg` from `git archive HEAD`
(the PKGBUILD with the source and checksum pointed at that local tarball).

| What | Command | Result |
|---|---|---|
| Tests on the Mac | `go test -count=1 ./...` | 211 test runs pass, 0 fail; `TMPDIR=/tmp` too |
| Tests in the VM | `go test -race ./...` as the user; `go test -coverpkg=./... ./...` as root | all pass, including the end-to-end daemon test as root |
| Scenarios bound to tests | `TestScenariosBoundToTests` | 17 feature files, 196 scenarios |
| Fuzzing the dialog card | `go test -fuzz FuzzRenderDialog -fuzztime 45s` | 9.1 M executions, no failure |
| Gate teeth | `scripts/gates-have-teeth.sh` | 8 of 8, the new one: a PKGBUILD that strips at link time goes red |
| Mutation testing | `python3 -I scripts/mutate.py` | 47 caught, 0 survived, 0 invalid |
| Coverage | `-coverpkg=./...` | 88.6 % as root in the VM, 85.7 % on the Mac (Linux-only code does not run there); below 70 % only the two `main` functions and `WritePublicKey` |
| Package build | `makepkg -f` | build, `check()` (19 test packages) and package pass |
| Package lint | `namcap` | PKGBUILD clean; package: "polkit and glibc may not be needed" (both are: oncerootd runs `pkcheck`, and the binaries load through glibc's `ld-linux`); debug package: build-id links "point to nothing" on their own, and resolve once the main package is installed |
| Hardening | `readelf` | both binaries PIE, GNU_RELRO, BIND_NOW, built without cgo |
| Install | `pacman -U` over the earlier manual install | 31 files, 0 altered (`pacman -Qkk`); group and folders created by pacman's own hooks; the earlier journal carried on and verifies |
| Bar module | module from docs/omarchy.md in `~/.config/omarchy/shell.json` | `root? 2`, highlighted; the tooltip shows both cards; a click opens `onceroot review` in Omarchy's floating terminal; nothing shows while no request waits |
| Review | `a`, then the polkit dialog; `r` | approved and ran once (file 0644 root); rejected; each says so |
| Dialog, a short card | `desk_prompt = auto`, request typed in foot | the dialog opens at once with the card above the password field |
| Dialog, fourteen long arguments | same | arguments wrap within the screen, the program and every argument visible |
| Dialog, an argument of 150 characters | same | no dialog; the agent is told to approve from `onceroot review`; the dialog that review opens says the card is too long for it and to approve only after reading it there |
| MCP | a JSON-RPC client over `onceroot mcp` in foot | initialize and tools/list answer; an allowed read returns exit 0; a call cancelled with notifications/cancelled gets no answer, its card is withdrawn and nothing ran; an approved call runs and returns exit 0 |
| A request over ssh, `desk_prompt = auto` | `onceroot run` in an ssh session | no dialog can open there; the card waits, the agent is told to approve it from `onceroot review`, and approving it from the bar runs it |
| Journal | `onceroot journal verify` | 61 entries verified at the end |

Found by this pass and fixed:

- Omarchy's polkit dialog centres every line on a one-line band. A long Run
  line ran off both edges of the screen, hiding the program and the last
  arguments, and lines below the middle covered the password field. The
  dialog card now wraps between quoted arguments within 96 characters and 12
  lines; a card that does not fit is not shown in part; and on Omarchy the text
  ends in as many empty lines as the card has, which keeps it above the field.
- Omarchy's bar shows the raw output of a command module whose JSON text is
  empty, so the idle bar showed `{"class":"idle",...}`. The module now prints
  nothing while no request waits, and uses the class Omarchy highlights.
- `makepkg check()` runs with `TMPDIR=/tmp`, whose parent anyone can write, and
  three resolve tests failed there. They now stop the ownership walk at their
  own folder; oncerootd still walks to `/`.
- A request from a process no authentication dialog can open for (over ssh)
  was rejected at once "by polkit", although nobody was asked, and counted
  toward the pause. pkcheck's exit status 2 is now told apart: the card waits
  for the review or the phone.
- namcap: binaries without PIE and full RELRO, and a debug package emptied by
  stripping at link time. Fixed and gated (TestPackageBuildHardening).

## 2026-10-27, third pass: after an independent review

A fresh reviewer read c74be26..2513af3 and found one critical and two
important defects. Each fix has a test that failed on the code before it.

| What | Command | Result |
|---|---|---|
| Tests on the Mac | `go test -count=1 ./...` | 220 test runs pass, 0 fail |
| Tests in the VM | `go test -race ./...` as the user; `go test -coverpkg=./... ./...` as root | all pass; coverage 89.2 % as root |
| Scenarios bound to tests | `TestScenariosBoundToTests` | 17 feature files, 206 scenarios |
| Fuzzing | `FuzzRender`, `FuzzRenderDialog`, 40 s each | no failure; every rune on a card is one of the shown set |
| Gate teeth | `scripts/gates-have-teeth.sh` | 9 of 9 |
| Mutation testing | `python3 -I scripts/mutate.py` | 55 caught, 0 survived, 0 invalid |
| Package | `makepkg -f`; `check()` now also measures the built binaries | 20 test packages pass, `TestBuiltBinariesAreHardened` ran (not skipped) on both binaries |
| An argument of 150 characters, over ssh | `onceroot run` | refused at once, exit 126, the reason names the approval dialog |
| An argument with full-width quotes, typed in foot | `desk_prompt = auto` | the dialog and the terminal show the quotes as `\uff02`, one argument |
| `onceroot review`, `a`, then Escape in the dialog | from the bar | "approve failed ... exit status 3; the request is rejected"; the agent is refused; nothing ran |
| `onceroot review`, `a`, then the password | from the bar | ran once, file created by root |
| Journal | `onceroot journal verify` | 73 entries verified |

Fixed in this pass:

- An agent could call `onceroot approve` on a card the dialog could not show
  whole, and the dialog asked for a password without the command. A card the
  dialog cannot show whole is now refused when it is asked for; approve is
  limited to the requester's own user; a dismissed approval dialog rejects the
  card, so one dialog per card is all anyone gets.
- Card width was counted in characters, not screen columns, and look-alike
  quotes could make one argument read as several. Outside text now shows only
  printable ASCII and Latin, Greek and Cyrillic letters as themselves; the rest
  is escaped.
- The PKGBUILD gate let four edits through. It now checks each go build line,
  the last build() and the last CGO_ENABLED, and check() measures the binaries.
- Lines the agent's words go on in carry a mark outside text cannot show.

## Not run yet

- polkit dialogs other than Omarchy's (GNOME, KDE, lxqt): with desk_prompt=auto
  they are not used; approval goes through `onceroot review`.
- A fingerprint or FIDO2 approval (the VM has neither).
- `makepkg` against the published release tarball: the PKGBUILD's checksum
  stays `SKIP` until the tag exists, and must be set then.
- The Telegram module against the real Bot API.
- Any agent (Claude Code, Codex) using the skill or the MCP tool for real; the
  MCP server was driven by a scripted client.
