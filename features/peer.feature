Feature: onceroot names who asked from what the kernel says
  @decided 2026-10-27: the card shows which process asked and from which
  folder, as onceroot observed it, never as the agent described itself.

  @test:TestParseStatHandlesParensInComm
  Scenario: a process name with parentheses does not shift the fields
    Given a process named "a) b (c"
    Then its parent and start time are still read correctly

  @test:TestParseStatRefusesHostile
  Scenario: an unreadable stat line is refused
    Then empty, truncated and non-numeric stat lines are refused

  @test:TestDescribeReadsAFakeProc
  Scenario: the card gets the process, its folder and its parents
    Given claude in /home/u/Work/app started by ghostty started by systemd
    Then the requester is claude with that folder and the chain ghostty, systemd

  @test:TestChainStopsAtSix
  Scenario: the parent chain is bounded
    Then at most six parent names are shown

  @test:TestStillSameSeesPidReuse
  Scenario: a pid reused by another process is not taken for the requester
    Given the requesting process exited and its pid was reused
    Then the check before the run fails
    And it fails too when the process is simply gone

  @test:TestFromConnReadsPeerCreds
  Scenario: uid and pid come from the kernel
    Given a client connected to onceroot's socket
    Then onceroot reads its uid and pid with SO_PEERCRED and its start time from /proc
