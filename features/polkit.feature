Feature: At the desk, the person approves in Omarchy's own polkit dialog
  @decided 2026-10-27: approval at the desk uses what the desktop already has:
  the polkit dialog with fingerprint, FIDO2 or password, showing the card.

  @test:TestArgsCarryStartTimeAndMessage
  Scenario: polkit is asked about the requesting process, with the card as the message
    Then pkcheck gets onceroot's action, the process as pid, start time and uid, user interaction allowed, and the card as polkit.message

  @test:TestCheckPassesTheMessageVerbatim
  Scenario: the card reaches the dialog unchanged
    Given a card of several lines with quotes
    Then pkcheck receives it as one argument, byte for byte

  @test:TestOnlyExitZeroApproves
  Scenario: only a clear yes from polkit approves
    Then exit status 0 approves, and 1, 2, 3, 4, 127 and a missing pkcheck do not

  @test:TestCancelledCheckIsNotApproval
  Scenario: a check that is cut off is not an approval
    Given the card expires while the dialog is open
    Then the check ends and nothing is approved

  @test:TestPolicyIsAuthAdminEverywhere
  Scenario: every approval needs an administrator's authentication, every time
    Then the shipped policy says auth_admin for any, inactive and active sessions

  @test:TestPolicyGateHasTeeth
  Scenario: the policy gate catches a weaker policy
    Then auth_admin_keep, yes, a missing class, another action id or a second action fail the gate

  @test:TestNoAgentIsNotARefusal
  Scenario: no dialog to ask is not a refusal
    Given pkcheck exits 2, which it does when no authentication agent can ask for the process
    Then onceroot reads it as "nobody was asked", never as a person saying no
