# onceroot

The binding design is docs/superpowers/specs/2026-10-27-onceroot-design.md;
the plan is docs/superpowers/plans/2026-10-27-onceroot-v1.md.

## Rules for working here

- Go 1.27, stdlib plus golang.org/x/sys only.
- Only internal/runner and internal/polkit import os/exec. (gate: TestOnlyRunnerAndPolkitImportExec in internal/gates/imports_test.go, teeth TestExecGateHasTeeth)
- Every scenario in features/ names a test that exists, and every name points at one. (gate: TestScenariosBoundToTests in internal/gates/features_test.go)
- oncerootd.service runs as root with UMask 0022, ProtectHome read-only and PrivateTmp. (gate: TestUnitHardening in internal/gates/unit_test.go, teeth TestUnitGateHasTeeth)
- The shipped rules allow only vetted read-only commands. (gate: TestShippedRulesAllowOnlyVettedReads in internal/gates/rules_test.go)
- PKGBUILD builds PIE with BIND_NOW and no cgo, never strips at link time, runs the suite in check() and depends on polkit; check() measures the built binaries. (gate: TestPackageBuildHardening in internal/gates/pkgbuild_test.go, teeth TestPackageBuildGateHasTeeth; TestBuiltBinariesAreHardened run by check(), teeth TestELFGateHasTeeth)
- Every new test runs red against the unfixed code before the fix.
- English only; no quotes of the owner, no attribution of decisions
  (`@decided YYYY-MM-DD` paraphrases instead).

## Invariants

Spec section 6.4. The marker says what holds each one.

- I1. No request runs as root without a matching allow rule or an approval recorded for that card. (test: TestAllowRuleRunsWithoutAsking, TestDenyNeverRuns, TestDeskRefusalRejectsWhenPhoneOff, TestRejectedCardNeverRuns, TestExpiredCardNeverRuns, TestBrokenRulesRefuse in internal/daemon; rule matching in internal/rules)
- I2. The socket never accepts an approval; approvals come only from polkit or the Telegram path with PIN and TOTP. (test: TestSocketCannotApprove, TestApproveRefusesAnotherUsersCard, TestDismissedApprovalRejects in internal/daemon; mutant 'approve without polkit' caught 2026-10-27)
- I3. A card runs at most once. (test: TestApprovedCardRunsOnce, TestHeldCardCannotRun, TestStatusMovesOnce in internal/card/card_test.go)
- I4. A card past its expiry is never approved or run. (test: TestExpiredCardCannotBeApproved, TestExpiredCardCannotRun in internal/card/card_test.go)
- I5. The executed path and binary hash equal the card's; otherwise nothing runs. (test: TestBinaryChangedBeforeRunRefuses, TestPidReuseBeforeRunRefuses in internal/daemon; TestUnchangedSeesAReplacedBinary, TestResolveRefusesWhatOthersCanWrite in internal/resolve)
- I6. Card text contains no raw control or bidi characters and is never truncated silently; the dialog card has no line wider than the dialog, a card the dialog cannot show whole is refused, and only narrow letters appear unescaped. (test: TestRenderQuotesAwkwardArgs, TestRenderEscapesTheProcessName, TestRenderNeverTruncates, TestRenderLabelsTheAgentText, FuzzRender, TestRenderDialogWrapsArgumentsBetweenQuotes, TestRenderDialogRefusesACardThatDoesNotFit, TestQuoteShowsOnlyNarrowLetters, TestRenderDialogShowsNoWideOrLookAlikeRunes, TestRenderDialogLabelsTheAgentText, FuzzRenderDialog in internal/card; TestCardTooLongForTheDialogIsRefused in internal/daemon)
- I7. A built-in denied command is denied whatever the rules file says. (test: TestBuiltinDenyBeatsAllowRule, TestBuiltinDeniesWhatTheCardCannotShow in internal/rules)
- I8. The command's environment is the fixed set, never the requester's. (test: TestRunnerEnvIsFixed in internal/runner)
- I9. Every decision and execution is journalled before the reply goes out; a journal write failure refuses the request. (test: TestJournalFailureRefuses, TestEveryStepJournalled, TestDecisionsAnswerAfterTheJournal in internal/daemon)
- I10. `journal verify` fails on any changed, removed or reordered entry. (test: TestVerifyFindsAChangedByte, TestVerifyFindsARemovedEntry, TestVerifyFindsReorderedEntries, TestOpenRefusesABrokenJournal in internal/journal; limit: root can rewrite the whole chain, and a cut tail is not detected without an outside copy)
- I11. Requests from a uid outside the allowed list are refused. (test: TestForeignUIDRefused in internal/daemon)
- I12. Secrets (bot token, PIN hash, TOTP secret) never appear in card text, journal entries, error messages or logs. (test: TestDetailNeverHoldsSecrets in internal/journal; TestTokenNeverInErrors, TestSecretMessageDeletedFirst, TestFactorsFileRoundTrip, TestLoadRefusesLooseFilePerms in internal/phone)
- I13. The polkit action is auth_admin for every subject class, never auth_admin_keep or yes. (gate: TestPolicyIsAuthAdminEverywhere in internal/gates/policy_test.go, teeth TestPolicyGateHasTeeth)
- I14. Telegram messages carry no parse_mode and no link previews; only the configured private chat is answered. (test: TestNoParseMode, TestOnlyConfiguredChatIsRead, TestBotRefusesOtherHosts in internal/phone)
- I15. go.mod requires only allowed modules. (gate: TestDependencyAllowlist in internal/gates/deps_test.go, teeth TestDependencyGateHasTeeth)
