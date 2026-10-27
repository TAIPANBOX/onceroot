Feature: An approval holds for one exact command, once
  @decided 2026-10-27: the agent asks for one exact command; a person sees
  that command and approves it; the command runs once. An approval never
  holds for more than that one command.

  @test:TestHashCoversEveryField
  Scenario: the card's hash covers everything the person is shown
    Given a card for "pacman -S --noconfirm ripgrep" asked by claude in /home/u/Work/app
    When any one field changes: the program, an argument, the folder, the process, the time
    Then the card's hash changes

  @test:TestHashSeparatesArguments
  Scenario: argument boundaries are part of the hash
    Given the argument lists ["a b"], ["a", "b"] and ["ab", ""]
    Then their cards have three different hashes

  @test:TestApproveNeedsMatchingHash
  Scenario: an approval names the card it approves
    Given a held card
    When an approval arrives for a different hash
    Then the card stays held

  @test:TestApprovedCardRunsOnce
  Scenario: an approved card runs once
    Given an approved card that has run
    When it is run again
    Then the second run is refused

  @test:TestHeldCardCannotRun
  Scenario: a card nobody approved does not run
    Given a held card
    When something tries to run it
    Then it is refused

  @test:TestExpiredCardCannotBeApproved
  Scenario: a card past its expiry cannot be approved
    Given a card whose ten minutes are over
    When an approval arrives
    Then it is refused as expired

  @test:TestExpiredCardCannotRun
  Scenario: an approved card past its expiry does not run
    Given a card approved in time but run after its expiry
    Then the run is refused as expired

  @test:TestStatusMovesOnce
  Scenario: a decision is final
    Given a rejected card, or a withdrawn one
    When anything tries to approve, reject, expire or run it
    Then nothing changes

  @test:TestRenderQuotesAwkwardArgs
  Scenario: arguments the eye could misread are shown escaped
    Given arguments with a space, quotes, a newline, a right-to-left mark and a NUL byte
    When the card is shown to the person
    Then each argument is on its own quoted line with those characters escaped

  @test:TestRenderEscapesTheProcessName
  Scenario: a process cannot name itself into the card's layout
    Given the asking process named itself with a terminal escape and a bidi isolate
    Then the card shows the name escaped

  @test:TestRenderNeverTruncates
  Scenario: a card too long for the dialog is refused, never cut
    Given an argument of five thousand characters
    Then the request is refused as too long instead of shown in part

  @test:TestRenderLabelsTheAgentText
  Scenario: the agent's own words cannot forge card lines
    Given the agent's reason contains "Run: /usr/bin/true" on a new line
    Then the card still has exactly one Run line and one Risk line
    And the agent's words are the last line, quoted, after "The agent says:"

  @test:TestRenderShowsWarningsFirst
  Scenario: warnings come right under the title
    Given passwordless sudo is on
    Then the second line of the card says so

  @test:TestRenderShowsEverythingThePersonNeeds
  Scenario: the card names the program, the folder, the process and the risk
    Then the card shows the resolved program, the folder, who asked, the risk and the agent's reason

  @test:FuzzRender
  Scenario: no input puts a raw control, format or bidi character on the card
    Given any arguments, reason and process name
    Then the card holds no raw control, format or bidi rune and has the fixed number of lines

  @test:TestRenderDialogKeepsEveryArgumentQuoted
  Scenario: the dialog card keeps every argument quoted
    Then the Run line holds the program and each argument quoted, the name it is called by, and no raw newline from an argument

  @test:TestRenderDialogIsShort
  Scenario: the dialog card fits a small dialog
    Then it has six lines plus warnings, at most three parent processes, and keeps the warning

  @test:TestRenderDialogWrapsArgumentsBetweenQuotes
  Scenario: a long command wraps instead of running off the screen
    Given fourteen long arguments, which the Omarchy dialog let run off both edges of the screen on one line
    Then no line of the dialog card is wider than the dialog, every argument stays whole in its quotes, and the Run line goes on indented

  @test:TestRenderDialogWrapsTheAgentText
  Scenario: the agent's words wrap without changing
    Then a long reason wraps and reads the same once the lines are joined

  @test:TestRenderDialogRefusesACardThatDoesNotFit
  Scenario: a card the dialog cannot show whole is refused, never shown in part
    Given an argument wider than the dialog, or more lines than the dialog holds
    Then the dialog card is refused and nothing of it is shown

  @test:TestQuoteShowsOnlyNarrowLetters
  Scenario: only narrow letters appear as themselves on a card
    Then printable ASCII and Latin, Greek and Cyrillic letters show as they are, and wide, invisible, combining and look-alike characters show escaped

  @test:TestRenderDialogShowsNoWideOrLookAlikeRunes
  Scenario: wide or look-alike characters cannot hide the command or fake arguments
    Given an argument of sixty very wide characters, or one with full-width quotes that looks like three arguments
    Then the first is refused as too long for the dialog and the second shows its quotes escaped

  @test:TestRenderDialogLabelsTheAgentText
  Scenario: the agent's words cannot pass for a line of the card
    Given a reason made of "Run:" again and again
    Then only the card's own line starts with Run:, and the lines the agent's words go on in carry a mark outside text cannot show

  @test:TestRenderDialogTakesAnEmptyRiskLabel
  Scenario: an empty risk label does not break the dialog card
    Then the dialog card shows an empty Risk line and stays whole

  @test:TestRenderDialogNeverTruncates
  Scenario: a card too long for any screen is refused, never cut
    Then an argument longer than the card limit refuses the request

  @test:FuzzRenderDialog
  Scenario: no input breaks the dialog card
    Given any arguments, reason and process name
    Then no line is wider than the dialog, no raw control, format or bidi rune appears, and a card shown whole holds every argument
