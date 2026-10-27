Feature: The person sees waiting requests in Omarchy's bar
  @decided 2026-10-27: made to fit Omarchy first: approval at the desk goes
  through the desktop's own dialog, and waiting requests are visible.

  @test:TestBarSilentWhenNothingWaits
  Scenario: the bar shows nothing while no request waits
    Then the bar module prints nothing, because Omarchy shows the raw output of a module whose text is empty

  @test:TestDoBarIdleAndUnreachable
  Scenario: the bar stays empty when idle and says when oncerootd is gone
    Then an idle bar prints nothing, and an unreachable oncerootd shows "root ?" highlighted

  @test:TestBarShowsCountAndActive
  Scenario: waiting requests show in the bar with their cards
    Given two held requests and one rejected
    Then the bar shows the padlock of Omarchy's own dialog and 2, highlighted, and the tooltip says 2 requests ask for root, with both cards and not the rejected one

  @test:TestReviewApprovesAndRejects
  Scenario: the review window acts only on what the person types
    Given three held requests and one that already ran
    When the person types a, r and s
    Then the first is sent for approval through polkit, the second is rejected, the third is skipped, each says so, and the finished one is not offered

  @test:TestReviewNeedsAnExplicitAnswer
  Scenario: an unclear answer does nothing
    Then "yes please" or an empty line skips the request

  @test:TestReviewSaysWhenAnActionFails
  Scenario: a failed approval is not reported as done
    Given the person dismisses the polkit dialog the review opened
    Then the review says the approval failed and never that it was approved
