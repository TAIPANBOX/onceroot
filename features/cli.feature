Feature: The onceroot command for agents and for the person
  @decided 2026-10-27: everything is checked and covered by tests before the
  tool goes further.

  @test:TestDoRunReturnsTheCommandsCode
  Scenario: onceroot run exits with the command's own exit code
    Then output passes through and the exit code is the command's

  @test:TestDoRunRefusedExits126
  Scenario: a refused request exits 126 with onceroot's reason
    Then the agent sees the reason on stderr and exit code 126

  @test:TestDoWithoutDaemonFails
  Scenario: without oncerootd every command fails plainly, and the bar says so
    Then run, list, status, approve and review exit 1, and the bar shows that oncerootd is not reachable

  @test:TestDoListStatusApproveReject
  Scenario: list, status, approve and reject say what they did
    Then list prints each card, list --json prints JSON, status prints its lines, and approve and reject confirm in plain words

  @test:TestDoBarAndReview
  Scenario: the bar and the review window talk to oncerootd
    Then the bar shows the waiting count, and review sends the person's answer for that card and says what was done

  @test:TestDoJournalVerify
  Scenario: journal verify checks the journal with its public key
    Then a good journal verifies, a broken one fails, and a missing key fails

  @test:TestDoMCPAnswersOverStdio
  Scenario: onceroot mcp is an MCP server on stdin and stdout
    Then tools/list over stdin lists run_as_root on stdout
