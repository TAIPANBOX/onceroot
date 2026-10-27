# Scenarios

Each scenario carries an `@test:<TestName>` tag on the line above it, naming
the Go test that proves it. `TestScenariosBoundToTests` in `internal/gates`
fails on a scenario without a tag, on a tag naming no test, on a feature file
with no scenarios, and when there are no feature files at all.

Decisions are written as `@decided YYYY-MM-DD` paraphrases. This repository
carries no quotes of its owner and no attribution of decisions.
