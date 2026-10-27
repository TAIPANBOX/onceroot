Feature: onceroot runs the program root would find, and only one root controls
  @decided 2026-10-27: the person sees exactly what will run, and an approval
  holds for that command only, so the program cannot be one the agent can
  swap after the approval.

  @test:TestResolveFindsAProgramInRootPATH
  Scenario: a bare name is found in root's PATH
    Then "tool" resolves to its absolute path with a content hash

  @test:TestResolveRefusesRelativeAndMissing
  Scenario: relative paths and unknown programs are refused before any card
    Then "./tool", "../tool", "sub/tool", an unknown name and an empty name are refused

  @test:TestResolveUsesOnlyRootPATH
  Scenario: the agent's PATH is never searched
    Given a program only on the agent's PATH
    Then onceroot does not find it

  @test:TestResolveFollowsSymlinkToFinalFile
  Scenario: a symlink resolves to the real file
    Then the card names the file the symlink points to

  @test:TestResolveRefusesWhatOthersCanWrite
  Scenario: a program others can change is refused
    Then a world-writable program, or one in a world-writable directory, is refused

  @test:TestResolveRefusesAFileOfAnotherOwner
  Scenario: a program that root does not own is refused
    Then a program owned by another user is refused

  @test:TestResolveRefusesNonExecutableAndDirectories
  Scenario: only executable regular files run
    Then a data file and a directory are refused

  @test:TestUnchangedSeesAReplacedBinary
  Scenario: a program changed after the card was made does not run
    Given a card made for a program
    When the program's content changes
    Then the check before the run fails
