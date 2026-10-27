Feature: An agent cannot wear the person down
  @decided 2026-10-27: requests are bounded so a person is not flooded into
  approving without reading; the values are onceroot's defaults, in its config.

  @test:TestDefaultLimits
  Scenario: the defaults are the published ones
    Then a card lives 10 minutes, a uid has at most 3 open cards and 30 per hour, and 3 rejections in a row pause it for 15 minutes

  @test:TestOpenLimit
  Scenario: at most three open requests per user
    Given three requests from one user wait for an answer
    When a fourth arrives
    Then it is refused until one of them is answered
    And another user's requests are not affected

  @test:TestHourlyLimitSlides
  Scenario: the hourly limit counts the last sixty minutes
    Given the hourly limit is reached
    When the oldest request falls out of the hour
    Then a new request is accepted

  @test:TestPauseAfterRejectionsInARow
  Scenario: three rejections in a row pause the user's requests
    Given a person rejected three requests in a row
    Then new requests from that user are refused for fifteen minutes
    And an approval between rejections starts the count again

  @test:TestSweepExpires
  Scenario: a request nobody answered expires
    Given a held request
    When ten minutes pass without an answer
    Then it expires and can no longer be approved

  @test:TestDecideRefusesWrongHash
  Scenario: a decision must name the card and its hash
    Then a decision for another hash or an unknown id changes nothing

  @test:TestMarkRanOnce
  Scenario: the store lets an approved card run once
    Then a held card cannot run, and an approved one runs once

  @test:TestNewIDAlphabet
  Scenario: request ids are easy to read aloud
    Then every id is five capital letters without I or O

  @test:TestChangedFiresOnDecision
  Scenario: whoever waits on a card hears about its decision
    Given a client waiting on a card
    When a person approves it
    Then the waiting client is woken

  @test:TestWithdrawEndsAnApprovedCard
  Scenario: an approved card whose requester left never runs
    Given an approved card
    When the agent that asked goes away before it runs
    Then the card is withdrawn and cannot run
