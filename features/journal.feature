Feature: Every step leaves a record nobody but root can rewrite quietly
  @decided 2026-10-27: each request, decision and execution goes into a signed
  journal that a separate command checks. Root holds the key, so root can
  write a whole new chain; the journal shows tampering by anyone else.

  @test:TestVerifyAcceptsAnUntouchedJournal
  Scenario: an untouched journal verifies
    Given a journal of five entries
    Then verify checks all five and finds nothing wrong

  @test:TestVerifyFindsAChangedByte
  Scenario: a changed entry is found
    Given the third entry's command is edited by hand
    Then verify fails and names entry 3

  @test:TestVerifyFindsARemovedEntry
  Scenario: a removed entry is found
    Given the third entry is deleted
    Then verify fails and names entry 3

  @test:TestVerifyFindsReorderedEntries
  Scenario: reordered entries are found
    Given the second and third entries are swapped
    Then verify fails and names entry 2

  @test:TestVerifyWithAnotherKeyFails
  Scenario: a journal signed by another key does not verify
    Then verify with a different public key fails

  @test:TestReopenContinuesTheChain
  Scenario: the chain continues across restarts
    Given onceroot restarts and appends to its journal
    Then the whole journal still verifies

  @test:TestOpenRefusesABrokenJournal
  Scenario: onceroot does not write on top of a broken record
    Given a journal edited by hand
    Then onceroot refuses to open it

  @test:TestDetailNeverHoldsSecrets
  Scenario: secrets never reach the journal
    Then an entry carrying a PIN, code, token, secret, OTP or password is refused
    And an entry of an unknown kind is refused

  @test:TestSignatureIsDomainSeparated
  Scenario: a signature made for something else does not pass as a journal entry
    Then a signature of another kind, or in rollcall's domain, is refused

  @test:TestSmallOrderKeyRefused
  Scenario: a public key under which forgeries verify is refused
    Then the identity point and a short key are refused as keys

  @test:TestVerifyRefusesASignedSequenceGap
  Scenario: a validly signed entry with the wrong number is refused
    Then verify names the entry whose sequence number is out of place

  @test:TestVerifyRefusesASignedBrokenLink
  Scenario: a validly signed entry that does not follow its predecessor is refused
    Then verify names the entry whose link to the one before is wrong

  @test:TestJournalModesSurviveATightUmask
  Scenario: the group can verify the journal although the service's umask is tight
    Given oncerootd runs with umask 0077
    Then the journal is 0640 and its public key 0644
