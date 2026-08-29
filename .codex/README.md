# Codex workspace notes

This directory contains the internal review and implementation plan for improving the project.

- `CONTEXT.md`: architecture, scope, and working constraints.
- `REVIEW.md`: confirmed technical findings and risks.
- `PLAN.md`: phased implementation plan and acceptance criteria.
- `CONTRACTS.md`: decisions and target contracts established in Phase 2.
- `BENCHMARKS.md`: performance baselines captured before optimization.
- `STATE.md`: progress tracker and unresolved decisions.
- `check.sh`: repeatable core formatting, test, race, and vet checks; run it with `bash .codex/check.sh`.

Set `RUN_FUZZ=1` for bounded fuzzing and `RUN_BENCHMARKS=1` for benchmark smoke tests. Their durations can be changed with `FUZZ_TIME` and `BENCH_TIME`.

The original invalid examples were excluded from behavioral decisions. Phase 9 replaced them with supported programs derived from the finalized API and included them in full repository checks.
