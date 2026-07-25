# Decision: Operating Path Completeness Publication Gate

Status: Active for implementation-scope adjudication. Accepted by the product
supervisor and activated by the repository owner through the explicit
delegation of code ownership in the current session. Production and test
changes remain prohibited until the separate implementation authorization
required below.

## Clean baseline and preserved contracts

The sole eligible implementation baseline is clean commit
`ebeea99674cfe453b22d3289e659ea3428d2f316`.

Any future implementation of this decision must preserve report contract `26`,
semantic Search contract `5`, run manifest contract `4`, Architecture Canvas
contract `5`, component-map contract `5`, existing opaque IDs, exact source
authority, and manifest-bound view-only publication when a Source Catalog
cannot be formed.

## Product problem

The current Paved Path validators prove that selected commands, endpoints, and
references exist in bounded repository-owned evidence. They do not require a
candidate to contain an explicit prerequisite, the complete essential action
sequence, and an observable completion result before the report publishes it
as a runnable and copyable task.

The saved etcd baseline demonstrates the user-visible failure. `goreman start`
was published as the complete "Run a local etcd cluster" path even though its
selected evidence omitted the documented prerequisite and observable result.
A compile command was likewise presented as completion of a flaky-test task.
Both candidates contain exact evidence, but neither is a complete task.

## Proposed product outcome

Repomap publishes a runnable or copyable operating path only when the selected
repository-owned evidence closes:

`prerequisites -> essential actions -> observable result`

Exact but incomplete operational evidence may remain inspectable as provenance
or a view-only landmark. It must not be presented as a complete task, acquire a
Copy or Run affordance, or be editorially completed from plausible nearby
evidence.

## Proposed publication contract

1. Existing bundle, evidence, command, endpoint, source, safety, and opaque-ID
   validation remains necessary and unchanged. Passing those validators is not
   sufficient for user-visible task publication.
2. **Prerequisites:** a publishable path has at least one selected, exact,
   non-redacted repository-owned prerequisite reference. Its bounded evidence
   must explicitly state that a tool, environment, configuration, setup action,
   or other condition is required by the same procedure. A non-empty ID slice,
   a reused action ID, or a generally relevant environment/configuration item
   does not satisfy this role by itself. The current contracts do not encode a
   proven "no prerequisites" assertion, so an empty prerequisite selection
   fails closed in this slice.
3. **Essential actions:** a publishable path has the smallest complete ordered
   sequence selected from exact repository-owned evidence. Every action remains
   locally validated. The sequence must be a contiguous actionable sequence
   in one selected documented procedure or repository script, with no omitted
   actionable step between its first and last selected step. An
   `ordering_basis` value or editorial ordering alone cannot prove
   completeness.
4. **Observable result:** a publishable path has at least one existing typed
   operational result, derived from exact selected evidence, after its final
   essential action. A title, goal, generic expected reference, command
   existence, successful compilation, or an unevaluated verification command
   is not itself an observable result. An exact action-derived command output
   or generated artifact satisfies this role; a non-empty
   `expected_evidence_ids` slice alone does not.
5. All three roles must be closed by evidence already selected for that
   candidate. Directory proximity, a neighbouring README paragraph, an
   unselected collector item, model prose, a Study direction, or a landmark
   cannot fill a missing role.
6. The gate is deterministic and local. It runs after existing safety and
   authority validation but before report, Search, navigation, or copy
   projection can make the candidate user-visible as a task.
7. A candidate that passes the gate retains its existing ID, serialized fields,
   ordering, copy policy, source bindings, and presentation byte-for-byte.
   Relative ordering among complete paths is unchanged.

## Suppression behavior

An incomplete candidate is absent from public `operations.paths`, Search
targets, hash routes, primary operating-task navigation, and any Copy or Run
affordance. Its exact evidence may remain in the saved record, issue ledger,
debug artifacts, or existing landmark projection, but a public landmark
retained solely from an incomplete candidate is view-only and cannot imply an
ordered workflow.

Suppression records one deterministic internal reason identifying the first
missing role: prerequisite, essential action sequence, or observable result.
That reason remains in saved/debug diagnostics and does not become a new public
warning or DTO field. If no complete paths remain, the existing optional
Operate publication behavior applies; the renderer must not replace the
missing task with a fallback workflow.

## Truth boundary

Repository documentation states an intended procedure; scripts and
configuration state executable structure. Neither proves that a command
succeeds on the user's machine. This gate proves only that the saved guide has
complete repository-owned instructions and an exact documented observable. It
does not execute commands, install tools, infer environment readiness, or claim
runtime success.

## Focused acceptance tests

- A small complete fixture with an exact prerequisite, documented or scripted
  action order, and a typed result after the final action remains
  byte-for-byte identical at the report boundary, including IDs and ordering.
- Otherwise-valid candidates missing only the prerequisite, only a contiguous
  documented/scripted action sequence, or only the final observable result are
  suppressed with the corresponding deterministic internal reason.
- Unrelated prerequisite IDs, an `ordering_basis` assertion over a sequence
  that skips a documented step, and generic expected evidence cannot satisfy
  the gate.
- The saved etcd `goreman start`-only candidate is absent as a public task and
  exposes no Copy or Run affordance.
- A setup/change-directory/compile sequence without a documented observable
  flaky-test result is absent as a public task and exposes no Copy or Run
  affordance.
- A result attached before, rather than after, the final essential action does
  not satisfy completion.
- Safe neighbouring or unselected evidence cannot fill a missing role.
- Replayed complete and incomplete records produce deterministic output; small
  existing complete fixtures remain unchanged.
- Report `26`, Search `5`, manifest `4`, Architecture Canvas `5`,
  component-map `5`, source-open behavior, view-only fallback, opaque IDs, and
  saved investigation behavior remain unchanged.
- Focused checks use saved fixtures only. They make no provider or network
  calls and execute no command or test in a target repository.

## Explicit non-goals

- No G1 mechanism, Overview, navigation, click-path, Search, Source, Study,
  Architecture, warning, confidence, telemetry, or general UI redesign.
- No Python support, language-adapter, provider, prompt, retrieval, ranking,
  evidence-collector, or target-repository execution change.
- No new public DTO, universal workflow schema, report/Search/manifest/canvas
  format change, MCP, browser redesign, broad package move, or cleanup.
- No production-file selection or implementation estimate in this draft.
- No attempt to prove that a documented command succeeds or that every
  repository can publish an operating path.

## Approval and activation condition

The product supervisor accepted this publication contract, and the repository
owner activated it as the current decision. Implementation remains prohibited
until the supervisor separately authorizes an exact production-file budget,
acceptance commands, and stop condition from the clean docs-only activation
commit. The next step is that separate implementation adjudication, not code.
