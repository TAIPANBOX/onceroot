Feature: oncerootd's flow from request to run
  @decided 2026-10-27: the agent asks for one exact command; a person sees it
  and approves it at the desk through polkit or, when enabled, on the phone;
  the command runs once; every step is journalled; nothing the agent or the
  socket says can stand in for the person's yes.

  @test:TestAllowRuleRunsWithoutAsking
  Scenario: an allowed command runs without asking anyone
    Given a rule "allow /usr/bin/pacman -Q* ..."
    When the agent asks for "pacman -Qi ripgrep"
    Then it runs and polkit is never asked

  @test:TestDenyNeverRuns
  Scenario: a denied command never runs
    Then "bash -c id" (built-in) and "pacman -Rns x" (a deny rule) are refused and nothing runs

  @test:TestApprovedCardRuns
  Scenario: the person approves at the desk and the command runs
    Given polkit shows the card and the person authenticates
    Then the command runs once and the agent gets its output
    And the agent was shown the card while it waited

  @test:TestDeskRefusalRejectsWhenPhoneOff
  Scenario: dismissing the dialog rejects the request when the phone is off
    Then the request is refused and nothing runs

  @test:TestRejectedCardNeverRuns
  Scenario: a rejected request never runs
    Given a held request
    When a person rejects it with "onceroot reject"
    Then the agent is told it was rejected and nothing runs

  @test:TestSocketCannotApprove
  Scenario: the socket alone cannot approve
    Given a held request
    When "onceroot approve" is sent and the person does not confirm in polkit
    Then nothing runs
    And only when the person confirms does the command run

  @test:TestForeignUIDRefused
  Scenario: only configured users may ask
    Then a request from a uid not in allowed_uids is refused, even for an allowed command

  @test:TestBinaryChangedBeforeRunRefuses
  Scenario: a program changed after approval does not run
    Then the check just before the run refuses it

  @test:TestPidReuseBeforeRunRefuses
  Scenario: a requester that is gone does not get its command run
    Then the check just before the run refuses it

  @test:TestJournalFailureRefuses
  Scenario: no journal, no root
    Given the journal cannot be written
    Then even an allowed command does not run

  @test:TestBrokenRulesRefuse
  Scenario: an unreadable rules file stops everything
    Then no request runs and polkit is not asked while the rules file cannot be read

  @test:TestCardWithdrawnWhenClientDisconnects
  Scenario: the agent leaving withdraws its request
    Given a held request
    When the agent disconnects
    Then the request is withdrawn and a later approval fails

  @test:TestEveryStepJournalled
  Scenario: the journal tells the whole story of an approved run
    Then the journal holds request, held, approved and ran, and the last three name the same card

  @test:TestPasswordlessSudoWarning
  Scenario: the person is told when passwordless sudo makes onceroot moot
    Given Omarchy's passwordless sudo grant is active
    Then the card warns about it right under the title, and status says so

  @test:TestNoDeskAgentFallsBackToPhone
  Scenario: without a desk dialog the phone still decides
    Given the phone module is on and polkit cannot ask anyone
    Then the request stays held for the phone, and the phone's approval runs it

  @test:TestExpiredCardNeverRuns
  Scenario: nobody answers and the request expires
    Then the agent is told it expired and nothing runs

  @test:TestListShowsHeldCards
  Scenario: held requests can be listed with their cards
    Then "onceroot list" shows each held request's id, status and card

  @test:TestParseReadsEveryKey
  Scenario: the config file sets every limit
    Then allowed users, card lifetime, limits, pause, command limit and the phone are read

  @test:TestParseFillsDefaults
  Scenario: what the config leaves out gets the published defaults
    Then an omitted key takes its default from the design

  @test:TestParseRefusesBadConfig
  Scenario: a config mistake is never quietly replaced by a default
    Then an unknown or repeated key, root as a user, a bad duration, a zero limit, or a phone without its chat stop oncerootd

  @test:TestLoadKeyCreatesOnceWith0600
  Scenario: the journal key is made once and kept private
    Then the first start creates a key readable by root only, and later starts reuse it

  @test:TestLoadKeyRefusesAWrongFile
  Scenario: a damaged or exposed key is refused
    Then a short key file or one others can read stops oncerootd

  @test:TestParseRun
  Scenario: the agent's command goes after --
    Then "onceroot run --why TEXT -- pacman -S ripgrep" asks for exactly "pacman -S ripgrep"

  @test:TestParseRunNeedsDoubleDash
  Scenario: without -- nothing is guessed
    Then a run without --, with nothing after it, or with an unknown option is refused

  @test:TestParseOtherCommands
  Scenario: the other commands take exactly their arguments
    Then list, status, approve ID, reject ID, journal verify and mcp are read, and malformed ones are refused

  @test:TestListShowsOnlyOwnCards
  Scenario: a user sees only their own requests
    Given two users may use onceroot
    Then one user's list does not show the other's requests

  @test:TestRejectRefusesAnotherUsersCard
  Scenario: a user cannot reject someone else's request
    Then rejecting another user's request is refused and the request stays held

  @test:TestReviewModeNeverOpensTheDialogOnRun
  Scenario: in review mode the dialog opens only from the review
    Given desk_prompt is review
    Then a new request opens no dialog, and approving it from onceroot review runs it

  @test:TestDeskPromptValues
  Scenario: the desk prompt is auto, dialog or review
    Then auto is the default and any other value stops oncerootd

  @test:TestAutoUsesTheDialogOnlyOnOmarchy
  Scenario: auto opens the dialog at once only on Omarchy
    Then auto means dialog where Omarchy is installed and review elsewhere, and the card is lifted above the password field wherever Omarchy's dialog shows it

  @test:TestDaemonEndToEnd
  Scenario: the real daemon, end to end, on Linux as root
    Given oncerootd running with its socket, journal and rules, and a user in group onceroot
    Then an allowed command runs as root, an approved one creates its file as root, a rejected one does not, a shell is refused
    And the journal verifies, and the socket, journal, public key and key file have their modes

  @test:TestLoadRulesMissingFileMeansNoRules
  Scenario: without a rules file every request asks a person
    Then a missing rules file gives no rules and no error

  @test:TestLoadRulesParsesAPrivateFile
  Scenario: a root-owned rules file is read on every request
    Then a rules file root owns and only root can write is used

  @test:TestLoadRulesRefusesAFileOthersCanWrite
  Scenario: a rules file others can write is not trusted
    Then such a file refuses every request

  @test:TestLoadRulesRefusesAnotherOwner
  Scenario: a rules file root does not own is not trusted
    Then such a file refuses every request

  @test:TestLoadRulesRefusesABrokenFile
  Scenario: a rules file that does not parse is not trusted
    Then such a file refuses every request

  @test:TestSetupPhoneWritesFactorsAndPrintsTheLink
  Scenario: phone setup writes the factors and shows the authenticator link once
    Then the factors file loads, the link is printed, the PIN is not, and spent codes of an old secret are forgotten

  @test:TestSetupPhoneRefusesAShortPIN
  Scenario: phone setup refuses a short PIN or none
    Then a four-digit PIN and an empty input are refused

  @test:TestPolkitGetsTheDialogCard
  Scenario: the dialog gets the dialog card, the agent the full one
    Then polkit is shown six lines with the arguments on one line, and the agent's terminal gets one argument per line

  @test:TestCardTooLongForTheDialogIsRefused
  Scenario: a request whose card the dialog cannot show whole is refused
    Given an argument wider than the dialog
    Then the request is refused with a reason naming the approval dialog, nothing is held, shown or run, and an allowed command with the same argument still runs

  @test:TestNoDeskAgentLeavesTheCardForTheReview
  Scenario: a request from outside the desktop session waits instead of being rejected
    Given the desk dialog is on and the asking process has no authentication agent, as over ssh
    Then the card stays held, the agent is told to approve it from onceroot review, and approving it there runs it

  @test:TestApproveRefusesAnotherUsersCard
  Scenario: only the requester's own user approves a card
    Then an approve from another allowed user is refused without a dialog, and the card stays held

  @test:TestDismissedApprovalRejects
  Scenario: a dismissed approval dialog rejects the card
    Given the person dismisses the dialog the review opened
    Then the card is rejected, the agent is told, and no second dialog opens for it

  @test:TestDecisionsAnswerAfterTheJournal
  Scenario: approving or rejecting is reported done only once the journal has it
    Given the journal refuses the decision
    Then the person approving or rejecting is told it failed, and nothing runs

  @test:TestApproveWithoutAnAgentKeepsTheCard
  Scenario: an approve that could ask nobody leaves the card held
    Given the approver has no authentication agent, as over ssh
    Then the approve fails and the card stays held

  @test:TestDialogLiftsTheCardOnOmarchy
  Scenario: on Omarchy the card stays above the password field
    Then the text polkit shows ends in as many empty lines as the card has, so the card sits above the field

  @test:TestServeRefusesABadSetup
  Scenario: oncerootd does not start on a setup it cannot trust
    Then a config others can write, a config that does not parse, a missing config, a missing group, a phone module without its files, or a missing state directory stop it

  @test:TestCommandFailureIsReported
  Scenario: a command that fails or is killed is reported and journalled
    Then the agent gets the runner's error and the journal records the run

  @test:TestJournalFailureAfterTheRunIsReported
  Scenario: a journal failure after the run is not hidden
    Then the agent is told the command ran but the journal failed
