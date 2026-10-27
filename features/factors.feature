Feature: Approving from the phone needs something you know and something you hold
  @decided 2026-10-27: approval away from the desk ships as an optional
  Telegram module, off by default; every approval there needs a PIN and a
  code from an authenticator app.

  @test:TestRFC6238Vector
  Scenario: codes are the ones authenticator apps show
    Given the RFC 6238 test secret at second 59
    Then the six-digit code is 287082

  @test:TestCodeSpentOnce
  Scenario: a code works once
    Given a code that approved a request
    When the same code arrives again
    Then it is refused

  @test:TestSkewOneStep
  Scenario: a phone a little out of time still works
    Then the code of the step before or after is accepted, and two steps away is not

  @test:TestOlderStepRefusedAfterNewer
  Scenario: an older code cannot follow a newer one
    Given the current code was spent
    Then the previous step's code is refused

  @test:TestSpentStepSurvivesRestart
  Scenario: a restart does not revive a spent code
    Given a code was spent and onceroot restarted
    Then the same code is refused

  @test:TestMalformedCodeRefused
  Scenario: only six digits count as a code
    Then empty, short, long, lettered or padded codes are refused

  @test:TestPINDigitsOnly6To8
  Scenario: a PIN is six to eight digits
    Then "123456" and "12345678" are valid, and four or five digits, nine digits, letters and non-ASCII digits are not

  @test:TestPINHashChecks
  Scenario: the PIN is stored salted and hashed
    Then the right PIN matches, a wrong one does not, and another salt gives another hash

  @test:TestLockoutAfterFiveFailures
  Scenario: five wrong tries lock the phone path
    Given five wrong tries in a row
    Then the phone path is locked

  @test:TestLockoutEndsAfterFifteenMinutes
  Scenario: the lock ends after fifteen minutes
    Then fifteen minutes later tries are accepted again and counted from zero

  @test:TestWrongPINDoesNotSpendTheCode
  Scenario: a wrong PIN does not waste the code
    Given a wrong PIN with the right code
    When the right PIN follows with the same code
    Then the approval goes through

  @test:TestLockedOutRefusesEvenTheRightFactors
  Scenario: during a lock even the right PIN and code are refused
    Then the right factors fail during the lock and pass after it
