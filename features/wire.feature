Feature: The socket takes only well-formed, bounded requests
  @decided 2026-10-27: the agent calls onceroot instead of sudo; onceroot reads
  its request strictly and tells it plainly what is happening.

  @test:TestDecodeAcceptsEachOp
  Scenario: the five requests a client can make are understood
    Then run, list, status, approve and reject are read

  @test:TestDecodeRefusesOutOfBounds
  Scenario: anything outside the protocol is refused
    Then an unknown op or field, an empty or oversized argument list, a NUL byte, an overlong reason, a malformed id, extra fields or two objects on one line are refused

  @test:TestDecodeRefusesALineOver64KiB
  Scenario: a request line is at most 64 KiB
    Then a 70 KiB line is refused before it is parsed

  @test:FuzzDecode
  Scenario: no input yields a request outside the bounds
    Then every request that is read satisfies the bounds of its op

  @test:TestEventRoundTrip
  Scenario: command output survives the trip byte for byte
    Then binary output with NUL and newline bytes arrives unchanged, as one line per event

  @test:TestRunStreamsOutputAndReturnsTheExitCode
  Scenario: the agent sees the card while it waits, then the command's output and exit code
    Given a request that a person approves
    Then the agent's stderr shows the card while it waits
    And the command's stdout and stderr reach the agent's stdout and stderr
    And onceroot exits with the command's exit code

  @test:TestRunReportsARefusal
  Scenario: a refusal reaches the agent with onceroot's reason
    Then the agent learns the request was not run and why

  @test:TestRunWithoutAnExitIsAnError
  Scenario: a stream cut short is never taken for success
    Then a connection that ends before the exit event is an error

  @test:TestDoReturnsTheAnswer
  Scenario: list, status, approve and reject get one answer each
    Then the client returns the daemon's one answer
