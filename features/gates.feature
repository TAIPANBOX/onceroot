Feature: The repository's own gates
  @decided 2026-10-27: onceroot follows the house testing discipline: every
  scenario is bound to a test, and dependencies stay inside an allowlist.

  @test:TestDependencyAllowlist
  Scenario: go.mod requires only allowed modules
    Given the module's go.mod
    Then it requires nothing but golang.org/x/sys
    And it replaces modules only with local directories

  @test:TestDependencyGateHasTeeth
  Scenario: the dependency gate refuses a foreign module
    Given a go.mod that requires a module outside the allowlist
    Then the dependency gate reports it

  @test:TestBindingGateHasTeeth
  Scenario: a scenario without a test fails the binding gate
    Given a feature file with a scenario that has no @test tag
    Or a tag that names no test
    Or no feature files at all
    Then the binding gate reports it
