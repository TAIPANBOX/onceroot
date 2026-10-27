# Contributing

Pull requests are welcome: a fix, a desktop or distribution we have not tried, a
better page. This file says how to build and check what CI builds and checks,
and what a pull request is held to.

A security problem goes through [SECURITY.md](SECURITY.md), not a pull request
or an issue.

## The flow

1. Fork the repository and branch from `main`.
2. Make the change with its scenario and its test (below).
3. Run the checks locally, then open a pull request against `main`.
4. CI runs every check on the pull request, the root tests and the package
   build included.
5. A maintainer reviews and merges.

## Building and checking locally

You need Go 1.27. On a Mac or Linux:

```sh
make test       # every test; the root and Linux-only ones skip elsewhere
make teeth      # each gate must fail on its own planted fault
make mutate     # each mutant of the decision path must be caught
make fuzz       # 60 seconds per fuzzer
```

On Arch Linux, the package the way a user builds it, with the suite in `check()`:

```sh
scripts/package.sh
```

The end-to-end daemon test needs root, a normal user to ask as (`tester`, or
`ONCEROOT_E2E_USER`) and the `onceroot` group:

```sh
sudo env "PATH=$PATH" ONCEROOT_E2E_USER="$USER" go test -run 'TestDaemonEndToEnd' -v ./cmd/oncerootd/
```

## What a pull request is held to

- **A scenario and a test.** New behaviour gets a scenario in
  [features/](features/) whose `@test:` tag names the test that proves it. The
  gate `TestScenariosBoundToTests` fails otherwise.
- **The test fails first.** A test for a fix is run against the code before the
  fix and must fail there. Say so in the pull request, with the failure.
- **Gates keep their teeth.** A new gate gets a case in
  `scripts/gates-have-teeth.sh` that plants its fault and requires it to fail.
- **The decision path keeps its mutants.** A change to how a request is
  decided, approved or run gets a mutant in `scripts/mutate.py` that the tests
  must catch.
- **No new dependencies.** Go's standard library only; `go.mod` is gated.
- **`gofmt`, `go vet`**, and the invariants in [CLAUDE.md](CLAUDE.md#invariants)
  still hold, each with what holds it.
- **English** in code, comments and documents. No long dashes in documents.

## Where things are

| Path | What |
|---|---|
| `cmd/onceroot`, `cmd/oncerootd` | the CLI (also the bar, review and MCP) and the service |
| `internal/card` | the card, its hash and how it is rendered |
| `internal/rules` | the rules file, the built-in list, risk labels |
| `internal/daemon` | the decision path: from a request to a run |
| `internal/resolve`, `internal/peer`, `internal/runner` | finding the program, who asks, running it |
| `internal/journal`, `internal/sig` | the signed, hash-chained journal |
| `internal/polkit`, `internal/phone`, `internal/factor` | the ways a person approves |
| `internal/gates` | the gates |
| `packaging/` | PKGBUILD, unit, polkit action, sysusers, tmpfiles, defaults |
| `features/` | the scenarios |
| `docs/` | the pages and the drawings |
