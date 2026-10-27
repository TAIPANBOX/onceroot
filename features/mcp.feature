Feature: Agents that prefer tools ask through MCP
  @decided 2026-10-27: onceroot works for every agent Omarchy launches without
  depending on one agent's hooks: a CLI for shells, an MCP tool for tool users.

  @test:TestInitializeEchoesProtocol
  Scenario: the MCP handshake works
    Then initialize answers with the client's protocol version and the server name onceroot

  @test:TestToolsListHasOneTool
  Scenario: there is exactly one tool
    Then tools/list shows run_as_root, which needs argv and why

  @test:TestCallRelaysExitCode
  Scenario: a call runs through onceroot and returns output and exit code
    Then the agent receives stdout, stderr and the exit code, marked as an error when the code is not 0

  @test:TestCallReportsARefusal
  Scenario: a refused call says it did not run, and why
    Then the result starts with NOT RUN and onceroot's reason

  @test:TestCallRefusesBadArguments
  Scenario: malformed calls never reach oncerootd
    Then an empty argv, another tool's name or a shell string instead of a list is an error and nothing is relayed

  @test:TestUnknownMethodIsJSONRPCError
  Scenario: unknown methods and broken JSON get the standard errors
    Then an unknown method gets -32601 and unreadable JSON gets -32700

  @test:TestNotificationGetsNoAnswer
  Scenario: notifications are not answered
    Then notifications/initialized gets no answer and the next request does

  @test:TestCancelledCallStopsTheRelay
  Scenario: an agent that cancels a call withdraws its request
    Given a tool call waiting for a person
    When the agent sends notifications/cancelled for it
    Then the relay is stopped, the call gets no answer, and other requests are still answered

  @test:TestRunContextClosesTheConnectionOnCancel
  Scenario: a cancelled run closes the connection, so oncerootd withdraws the card
    Then the daemon sees the connection close as soon as the run is cancelled
