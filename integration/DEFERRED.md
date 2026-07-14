# Deferred audit work

Deferred means “still a known risk”, not “disproved”. These items are outside
the approved MVP increments because no selected regression test requires the
larger change or because the current fixture cannot prove a useful contract.

| Item | Reason | Revisit trigger |
| --- | --- | --- |
| Full deferred-call lifecycle (`localVss.DeleteSnapshots`) | the MVP callback fix must not silently generalize into a Go control-flow engine | a product flow depends on deferred cleanup and receives a focused failing fixture |
| General happens-before or control-flow solver | exact source order plus explicit task/cancel/join facts are sufficient for the restic fixture | a selected test cannot be expressed without branch or ordering constraints |
| Generic cross-language async task algebra | only the Go/restic fixture demonstrates a concrete concurrent lifecycle; Python is synchronous in the accepted fixture | a real Python async framework fixture needs the same consumer contract |
| External I/O discovery beyond the selected callsites | source target resolution proves identity, not external I/O or persistence role | a bounded adapter can produce a concrete external-resource witness |
| CLI process termination proof | return from `runBackup` proves handler completion only; `main`/`Exit` is outside the selected bounded proof | a user-facing scenario requires process-exit semantics |
| Python semantic onboarding parity (framework registrations, imports, resources) | current Pyright adapter only proves focused exact-symbol facts | a separate Python milestone selects one framework fixture and explicit acceptance criteria |
| Language-neutral focused investigation runner | existing investigation freshness/test discovery is Go-specific | a Python product action, not an analyzer experiment, requires resume/action support |
| Pyright total-work metrics or cancellation instrumentation | output is bounded and current runs are acceptable; no MVP failure is caused by missing work accounting | a reproducible latency/cancellation budget fails |
| Report-server service split | architectural hygiene proposal without a semantic regression | presentation starts mutating proof state in a way a focused test exposes |
| SSA, VTA, repository database, embeddings, framework registry | explicitly forbidden expansion for this integration | a future milestone approves one after bounded tools fail a concrete test |
| Exhaustive audit test inventory | would turn symptom lists into a brittle suite | promote one only when it protects a new accepted contract or a confident product lie |

The report renderer’s order-sensitive guided-path behavior remains a known risk.
This integration may remove a demonstrated false sequence, but it will not
redesign the browser or introduce a graph framework.
