Feature: The approved command runs exactly as approved and cannot outlive its limit
  @decided 2026-10-27: what runs is what the person saw: that argument list,
  in that folder, with an environment the agent does not control.

  @test:TestRunnerEnvIsFixed
  Scenario: the agent's environment never reaches the command
    Given the agent has a variable AGENT_SECRET set
    Then the command sees only PATH, LANG and HOME as onceroot sets them

  @test:TestRunnerPassesArgvVerbatim
  Scenario: arguments arrive byte for byte
    Then arguments with spaces, newlines, quotes, an empty one, "--" and "-n" reach the program unchanged

  @test:TestRunnerStdinIsEmpty
  Scenario: a command that waits for input does not hang
    Given a command that reads its input
    Then it reads nothing and ends at once

  @test:TestRunnerExitCodeAndStderr
  Scenario: the exit code and error output come back
    Then exit status 7 and the error output reach the agent

  @test:TestRunnerUsesTheCardsCwd
  Scenario: the command runs in the folder shown on the card
    Then the working directory is the card's folder

  @test:TestRunnerRefusesAMissingCwd
  Scenario: a folder that is gone is not replaced by another
    Then the command does not run when the card's folder no longer exists

  @test:TestRunnerStreamsLargeOutput
  Scenario: large output is streamed, not held in memory
    Given a command printing ten megabytes
    Then all of it arrives in pieces of at most 32 KiB

  @test:TestRunnerKillsProcessGroupOnTimeout
  Scenario: the time limit stops the command and everything it started
    Given a command that starts a background child and waits
    When the time limit passes
    Then both are killed and the run ends with an error

  @test:TestOnlyRunnerAndPolkitImportExec
  Scenario: only two places in onceroot can start a process
    Then no code outside runner and polkit imports os/exec or calls a process-starting system call

  @test:TestExecGateHasTeeth
  Scenario: the process-start gate catches a stray process start
    Then os/exec in the daemon, os.StartProcess or syscall.Exec elsewhere fail the gate, and runner and tests do not
