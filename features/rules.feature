Feature: What happens before anyone is asked
  @decided 2026-10-27: safe commands run without asking, dangerous ones never
  run, and everything else waits for a person. Programs whose real effect a
  card cannot show are refused by onceroot itself, whatever the rules say.

  @test:TestParseRejectsBadLines
  Scenario: a broken rules line is refused with its line number
    Given a rules file with an unknown verb, a relative path or a * inside a pattern on line 3
    Then reading the rules fails and the error names line 3

  @test:TestParseSkipsCommentsAndBlanks
  Scenario: comments and blank lines are ignored
    Given a rules file with comments, blank lines and one rule with a trailing comment
    Then exactly that one rule is read, with its line number

  @test:TestFirstMatchWins
  Scenario: the first matching rule decides
    Given "deny pacman -R*" above "allow pacman ..."
    Then "pacman -Rns foo" is denied and "pacman -Qi foo" is allowed

  @test:TestNoMatchMeansAsk
  Scenario: no matching rule means a person is asked
    Given rules that allow only pacman queries
    When the agent asks for "systemctl restart sshd"
    Then a person is asked

  @test:TestArgCountMustMatchWithoutEllipsis
  Scenario: a rule without ... matches only the exact argument count
    Given "allow systemctl status sshd"
    Then "systemctl status sshd --no-pager" still asks a person

  @test:TestPrefixPattern
  Scenario: a pattern ending in * matches by prefix
    Given "allow pacman -Q* ..."
    Then "pacman -Qi ripgrep" runs without asking and "pacman -S ripgrep" asks

  @test:TestRuleMatchesTheResolvedPathOnly
  Scenario: a rule matches the program root resolved, not the name the agent used
    Given a rule for /usr/bin/pacman
    Then /usr/local/bin/pacman does not match it

  @test:TestBuiltinDenyBeatsAllowRule
  Scenario: the built-in deny list wins over an allow rule
    Given rules that allow bash and env
    When the agent asks for "bash -c id", "bash script.sh" or "env pacman -Q"
    Then each is denied by onceroot itself

  @test:TestBuiltinDeniesWhatTheCardCannotShow
  Scenario: shells, interpreters, wrappers, privilege tools and editors are refused
    Then sh, bash, python3, perl, env, xargs, timeout, systemd-run, sudo, su, pkexec, run0, visudo and vim are denied
    And busybox reached as sh is denied
    And pacman, systemctl, cp and mkdir are not

  @test:TestRiskLabels
  Scenario: the card labels risky commands in onceroot's own words
    Then installing a local package, touching sudo or polkit rules, recursive changes on /, raw disk writes, find -delete, account changes and writes under /usr are labelled
    And a pacman query gets no label

  @test:FuzzParse
  Scenario: no rules text yields a rule with a relative path or a misplaced *
    Given any rules text
    Then every accepted rule has an absolute path, a known verb and only trailing * patterns
