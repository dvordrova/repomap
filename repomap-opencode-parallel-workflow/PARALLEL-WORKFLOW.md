# Parallel workflow roles

## Source truth

`repo-fact-oracle` reads one actual repository independently of repomap output and records
expected entrypoints, surfaces, useful flows, architecture responsibilities, and honest
frontiers.

Oracles are cached by fixture revision and reused until the source repository changes.

## Isolated execution

`fixture-runner` runs one repository using an isolated output/cache directory. Several
fixtures may run in parallel, but normal iteration uses saved provider responses.

## Independent comparison

`fixture-auditor` compares one generated report to its source oracle.

At the same time:

- `semantic-contract-auditor` checks cross-artifact meaning and counts;
- `performance-auditor` compares stage timings, provider budgets, and cache behavior.

## Synthesis

`cross-fixture-synthesizer` groups symptoms by shared root cause and returns one smallest
generic implementation batch.

## One writer

`feature-builder` is the only production-code writer. This avoids contract races and
merge-conflict-driven architecture.

## Browser product review

`browser-fixture-reviewer` performs a real onboarding journey for one report. Several
fixtures can be reviewed in parallel on distinct ports.

`product-acceptance-reviewer` issues the final PASS/BLOCKED verdict after all evidence is
available.
