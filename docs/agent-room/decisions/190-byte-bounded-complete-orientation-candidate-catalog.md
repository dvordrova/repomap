# Decision 190: Byte-bounded complete Orientation candidate catalog

## Status

Approved by the repository owner through the active supervisory instruction as
the immediate provider-free reliability checkpoint after Decision 189.
Decision 189 and earlier historical checkpoints are not rewritten.

## Problem

The ordinary Orientation path passed the research policy's file-count cap into
the generic bundle builder before request-byte fitting. A saved live Casdoor
run therefore reduced 834 locally eligible candidate rows to 250 solely under
`selection/max_files`, even though the resulting canonical bundle was only
92,649 bytes against a configured 1,032,192-byte bundle boundary. Byte fitting
made one attempt and was not applied.

That count-bound diversity selection retained low-ranked test files while
omitting higher-ranked production evidence. In the exact saved case,
`controllers/user.go` had score 130 but was omitted. A provider-free replay
with the complete 834-row catalog retained that exact path under stable ID
`file-5b366d4eacda4fd5`; the canonical bundle was 271,111 bytes and the typed
request was 322,963 bytes, both well below the existing request policy.

The loss was therefore an early selection policy artifact, not evidence that
the existing byte boundary required fewer grounded paths.

## Decision

Make the ordinary Orientation candidate catalog byte-bounded rather than
count-bounded. The normal command path passes zero for `MaxLLMFiles`, and
`orient.Run` resolves a non-positive value to the exact complete
`FilteredFiles` input count before calling the existing language-neutral bundle
builder. The `repomap orient --max-llm-files` default is zero. A positive value
is an explicit debug/test cap and retains the existing deterministic diversity
selection for that initial count-bound build.

Keep the configured canonical bundle-byte limit as the hard boundary. The
first build is always the complete or explicitly capped baseline. If it
exceeds the byte limit, search only the candidate-file limit with a bounded
binary search. Every retry uses the exact already-ranked prefix, and the
returned result is the largest prefix whose complete canonical bundle,
including the byte-fit warning, fits. The next ranked row therefore does not
fit unless the complete baseline already fit. Search attempts are bounded and
reported exactly.

The minimum retry prefix remains eight candidate rows. If that prefix still
exceeds the byte boundary, return the truthful bounded failure warning and
trace; do not shed another fact category. README, modules, entrypoints, edges,
source signals, known docs, command traces, and the byte boundary retain their
configured caps throughout byte fitting.

The original configured file limit remains the independent cap for Go
OrientationCandidates. A reduced byte-fit candidate-row limit is not reused
for that collection. After selecting the ranked prefix, candidate rows,
allowed paths, known docs, source signals, entrypoints, command traces,
OrientationCandidates, the private request-local reference catalog, and the
typed wire are rebuilt as one closed projection. No dangling candidate,
file, or evidence reference is permitted.

Candidate rows continue to contain only existing bounded path, kind, score,
signal, reason, and stable-ID metadata. Decision 190 adds no file contents,
snippets, full tree, or additional local scan. It changes no prompt, provider
request/response contract, provider identity, cache contract or key, Study,
canonical report, Architecture, Repository Atlas, localization, UI, retry,
legacy reader, migration, or live-model behavior.

## Proof

Provider-free focused tests establish that:

- a Casdoor-shaped 834-row catalog retains every candidate, including exact
  `controllers/user.go` score 130 and stable ID, under the configured bundle
  byte boundary with one build and no byte fit;
- the existing typed reference catalog and wire retain all 834 candidate/file
  mappings without duplicating `allowed_paths` or losing the exact user file;
- forced byte fitting returns the exact best-first prefix, changes only the
  candidate-file cap, and proves the selected `N` fits while `N+1` does not;
- allowed paths and typed file/evidence refs remain atomic with the fitted
  candidate rows and contain no dangling mappings;
- the reduced retry limit does not truncate the separately configured Go
  OrientationCandidate collection;
- both fitting and exhausted searches report the actual baseline plus binary
  probe count and reuse the already-tested minimum prefix;
- a small below-cap repository produces byte-identical bundle JSON when the
  ordinary complete-input bound replaces the old larger explicit cap; and
- all checks run without an API key, network request, or live model.
