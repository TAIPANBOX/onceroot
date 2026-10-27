Feature: The package installs onceroot the way the design says
  @decided 2026-10-27: onceroot reaches Omarchy as an AUR package, the only way
  to install a root service there without a change to Omarchy itself.

  @test:TestUnitHardening
  Scenario: the service runs as root, cannot write home directories and has its own /tmp
    Then oncerootd.service runs /usr/bin/oncerootd with UMask 0022, ProtectHome read-only, PrivateTmp and restart on failure, and sets no User

  @test:TestUnitGateHasTeeth
  Scenario: the unit gate catches a weaker service
    Then a missing PrivateTmp, a writable home, a 0000 or 0077 umask, a changed command line, a User or an empty file fail the gate

  @test:TestShippedRulesAllowOnlyVettedReads
  Scenario: the default rules let through only vetted read-only commands
    Then every allow rule in the shipped rules file is on the vetted list, and journalctl is not

  @test:TestPackageBuildHardening
  Scenario: the package builds hardened binaries and tests them
    Then PKGBUILD builds position-independent binaries bound at load time, without cgo and without stripping at link time, runs the whole suite in check() and depends on polkit

  @test:TestPackageBuildGateHasTeeth
  Scenario: the package gate catches a weaker build
    Then no PIE, no BIND_NOW, -s or -w in any spelling, cgo on or turned on later, no trimpath, one go build line that sets another build mode or drops -bindnow, a later weaker build(), no check(), tests moved into build(), binaries never measured, no polkit dependency or an empty file fail the gate

  @test:TestBuiltBinariesAreHardened
  Scenario: the package's check() measures the binaries it built
    Then each built binary is position independent, bound at load time, has a RELRO segment and keeps its symbols and DWARF for the debug package

  @test:TestELFGateHasTeeth
  Scenario: the binary measurement catches a weaker binary
    Then a non-PIE binary, one without BIND_NOW, one stripped at the link, or a missing file fail it
