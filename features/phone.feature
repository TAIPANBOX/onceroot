Feature: Approval from the phone, when the owner turns it on
  @decided 2026-10-27: approval away from the desk ships in the first version
  as an optional Telegram module, off by default; the core works without it.

  @test:TestNoParseMode
  Scenario: the card goes to the phone as plain text to one chat
    Then the message has no parse_mode, link previews are off, and it goes to the configured chat only

  @test:TestApproveNeedsPINAndCode
  Scenario: Approve asks for PIN and code before anything runs
    Given the person pressed Approve
    Then nothing is approved until a message with the right PIN and code arrives

  @test:TestWrongFactorsDoNotApprove
  Scenario: a wrong PIN or a PIN alone does not approve
    Then a wrong PIN with the right code, or only a PIN, leaves the request open

  @test:TestRejectNeedsNoFactors
  Scenario: Reject takes one press
    Then pressing Reject rejects at once, because rejecting cannot give root

  @test:TestOnlyConfiguredChatIsRead
  Scenario: nobody but the configured private chat is heard
    Then buttons and messages from another chat change nothing and get no answer

  @test:TestStaleHashRefused
  Scenario: a button for another version of the card does nothing
    Then a button whose card hash does not match is refused

  @test:TestSecretMessageDeletedFirst
  Scenario: the message with PIN and code is deleted before anything else
    Then deleteMessage is the first call after the secret arrives

  @test:TestCodeReplayRefused
  Scenario: a code that approved one request cannot approve another
    Then the same code on a second request is refused

  @test:TestDeskDecisionWithdrawsPhoneCard
  Scenario: a request decided at the desk closes on the phone
    Then the phone's message says the request is closed and its buttons do nothing

  @test:TestTokenNeverInErrors
  Scenario: the bot token never shows up in errors or printouts
    Then a failing API call's error and every print form of the bot omit the token

  @test:TestBotRefusesOtherHosts
  Scenario: the bot talks to Telegram only
    Then the client refuses to connect to any other host

  @test:TestLoadRefusesLooseFilePerms
  Scenario: the phone module needs its secret files private
    Then a token or factors file readable by others stops the module

  @test:TestFactorsFileRoundTrip
  Scenario: the PIN is stored hashed, never in clear
    Then a written factors file checks the PIN and code and does not contain the PIN

  @test:TestNewBotWithoutAClientIsGuarded
  Scenario: the default client of the bot talks to Telegram only
    Then a bot made without an explicit client refuses to connect to another host
