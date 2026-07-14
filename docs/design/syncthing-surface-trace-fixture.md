# Syncthing surface-to-trace fixture diagnosis

This record captures the blind product diagnosis for saved run
`20260713-191035-syncthing`. The diagnosis used only persisted local artifacts
and saved provider responses. No provider call was repeated.

## Saved-run outcome

The repository snapshot and model stages completed, but generic surface
discovery failed as one repository-wide operation:

```text
surface discovery unavailable: package loading failed with 3 error(s); first:
/Users/dvordrova/git/syncthing/cmd/infra/strelaypoolsrv/main.go:289:17:
undefined: auto.Assets
```

The surface stage therefore persisted no trigger catalog or coverage artifact.
The report projected an empty unified catalog: zero surfaces, zero inspected
packages, and zero considered entrypoints. This is not evidence that Syncthing
has no runtime surfaces.

The snapshot independently retained one Go module, 91 build-selected packages,
and 18 exact top-level `func main()` declarations. A package-local load failure
incorrectly erased usable facts from all other executables.

## Broken package diagnosis

The first failed package is
`github.com/syncthing/syncthing/cmd/infra/strelaypoolsrv`.
Its build-selected files are:

- `cmd/infra/strelaypoolsrv/main.go`
- `cmd/infra/strelaypoolsrv/stats.go`

The imported `cmd/infra/strelaypoolsrv/auto` package selected only `doc.go`.
Tracked file `cmd/infra/strelaypoolsrv/auto/noassets.go` was not build-selected,
and the saved selected files expose no definition for `auto.Assets`. The
artifacts therefore support a missing generated/build-selected source
diagnosis. They do not support pretending the package compiled. The loader
reported three errors but persisted only the first, so the other two cannot be
reconstructed from this run.

The generic recovery boundary is package or executable availability. A typed
analyzer may retain only packages whose type information and dependency closure
are safe. Exact syntax-level entry declarations remain usable even when deeper
typed analysis for that executable is unavailable.

## Exact process entrypoints

These are local build-selected package facts with exact receiverless,
parameterless top-level `func main()` declarations. They are not model guesses
or filename-only hints.

| Package | Exact declaration | Initial product role |
|---|---|---|
| `cmd/syncthing` | `cmd/syncthing/main.go:212` | primary application |
| `cmd/stdiscosrv` | `cmd/stdiscosrv/main.go:87` | secondary service |
| `cmd/strelaysrv` | `cmd/strelaysrv/main.go:82` | secondary service |
| `cmd/infra/stcrashreceiver` | `cmd/infra/stcrashreceiver/main.go:50` | secondary service |
| `cmd/infra/strelaypoolsrv` | `cmd/infra/strelaypoolsrv/main.go:129` | unavailable secondary executable |
| `cmd/infra/stupgrades` | `cmd/infra/stupgrades/main.go:41` | secondary service |
| `cmd/infra/ursrv` | `cmd/infra/ursrv/main.go:23` | secondary service |
| `cmd/strelaysrv/testutil` | `cmd/strelaysrv/testutil/main.go:23` | test/helper |
| `cmd/dev/stcompdirs` | `cmd/dev/stcompdirs/main.go:20` | tooling |
| `cmd/dev/stdisco` | `cmd/dev/stdisco/main.go:43` | tooling |
| `cmd/dev/stevents` | `cmd/dev/stevents/main.go:26` | tooling |
| `cmd/dev/stfileinfo` | `cmd/dev/stfileinfo/main.go:20` | tooling |
| `cmd/dev/stfinddevice` | `cmd/dev/stfinddevice/main.go:27` | tooling |
| `cmd/dev/stfindignored` | `cmd/dev/stfindignored/main.go:19` | tooling |
| `cmd/dev/stgenfiles` | `cmd/dev/stgenfiles/main.go:20` | tooling |
| `cmd/dev/stsigtool` | `cmd/dev/stsigtool/main.go:19` | tooling |
| `cmd/dev/stvanity` | `cmd/dev/stvanity/main.go:38` | tooling |
| `cmd/dev/stwatchfile` | `cmd/dev/stwatchfile/main.go:18` | tooling |

The snapshot also contains heuristic path `cmd/syncthing/cli/main.go`, but that
package is named `cli` and has no exact process-entry anchor. It must not become
a process surface.

The role labels above are fixture expectations, not permission to encode these
paths in production. Generic evidence must distinguish the repository-named
main executable, independent non-tool services, developer-tool areas,
test/helper areas, and executables whose deeper analysis is unavailable.

## Current directions and why none can start a trace

All six accepted directions produced `local_only`, `evidence_only` bundles.
None contains a FlowProof session or saved trace.

| Direction | Exact entrypoint candidate | Current failure |
|---|---|---|
| Syncthing daemon startup and continuous sync | `cmd/syncthing/main.go:212` | no locally resolvable starting callable; command dispatch and core retrieval remain incomplete |
| REST API request handling | `cmd/syncthing/main.go:212` | no locally resolvable starting callable or exact route seed in the saved result |
| Discovery server operation | `cmd/stdiscosrv/main.go:87` | exact process declaration exists, but the proof assembler accepts only current command traces |
| Relay server operation | `cmd/strelaysrv/main.go:82` | exact process declaration exists, but the proof assembler accepts only current command traces |
| Crash receiver background maintenance | `cmd/infra/stcrashreceiver/main.go:50` | exact process declaration and local HTTP signals exist, but no surface survived and no generic entry seed is accepted |
| Background loop — periodic ticker created | parser-selected `cmd/dev/stvanity/main.go:38` | aggregate source-signal hypothesis spans multiple executables and has no singular exact owner/callback |

Every suggestion currently reports:

```text
current_grounding: exact_member
investigation_available: false
can_start_trace: false
unavailable_reason: no locally resolvable starting callable
```

`exact_member` here means package/file membership. It does not provide an exact
starting callable. The current proof assembler is Cobra-command-oriented and
does not accept exact process-entry declarations. A useful process trace may be
partial: exact `main` seed, exact local calls, then an explicit unresolved
composition or service-start frontier.

The periodic-ticker direction is not automatically a top-level flow. Its saved
evidence spans developer tooling, crash receiver, upgrades, and relay-pool
packages. Until an exact parent/callback join exists it remains a suggested
investigation or frontier, not a saved trace.

## Existing architecture and exact suggestion ownership

Architecture synthesis succeeded over `package_landscape`: three model-named
subsystems, ten model-named components, all 91 exact package members, and 60
exact package-import facts. No behavior-grounding artifact was available.
Component names and conceptual grouping are model interpretation; member IDs,
package paths, and import edges are local facts.

Exact package membership supports these suggestion mappings:

| Suggestion | Owning component(s) supported by exact members |
|---|---|
| Syncthing daemon startup | Main Entry (`cmd/syncthing`); Core Library for its separately cited library members |
| REST API handling | Main Entry (`cmd/syncthing`) and Core Library (`lib/api`) |
| Discovery server | Discovery Server (`cmd/stdiscosrv`) |
| Relay server | Relay Server (`cmd/strelaysrv`) |
| Crash receiver | Usage Reporting & Upgrades (`cmd/infra/stcrashreceiver`) |
| Periodic ticker | no honest singular owner; evidence spans Dev Tools, Usage Reporting & Upgrades, and Relay Server |

The report currently leaves all suggestion component mappings empty because the
ownership index does not join package-member declarations through the exact
repository package graph. Mapping must use canonical package/member IDs and
remain unassigned when more than one owner is supported.

## Evidence bundles are not traces

Saved-run accounting is:

| Object | Count |
|---|---:|
| accepted suggested investigations | 6 |
| local evidence bundles | 6 |
| persisted FlowProof traces | 0 |
| usable surface records | 0 |
| failed package-load stage | 1 repository-wide failure, reporting 3 package errors |

`report.run.saved_flow_count` incorrectly reports six because it uses the number
of evidence bundles. The six bundles remain useful under each suggested
investigation as supporting files/packages/tests, but they are not saved traces.

## Focused research failure

Two saved DeepSeek research rounds were performed. Both questions were
reasonable; their evidence selection was not.

### Relay client round

Question: which method under `lib/relay/client` discovers and connects to
relays?

Selected source windows:

- `lib/relay/client/client.go:1-28`
- `lib/relay/client/dynamic.go:1-28`
- `lib/relay/client/methods.go:1-28`
- `lib/relay/client/static.go` received only a file summary

The model correctly returned an unsupported bounded finding because it mostly
saw headers and imports. Saved local evidence already had a more useful exact
line at `lib/relay/client/dynamic.go:43` for `dynamicClient.serve`, plus relevant
signals in `static.go`, but the research planner did not center on them.

### Remote-index round

Question: which function in `lib/model/model.go` processes remote index
messages and updates the local database?

Selected source windows:

- `lib/model/model.go:1-28`
- `internal/db/olddb/set.go:1-28`
- `internal/db/olddb/lowlevel.go:1-28`

The first window is only the header/import block; the others end at unrelated
type or constructor declarations. The unsupported finding is a statement about
the supplied windows, not the repository. Tracked files
`lib/model/indexhandler.go` and `lib/model/indexhandler_test.go` were not
selected.

All windows began at line 1 because research candidates carried file paths but
no exact focus line. Missing focus was converted to line 1 and the reader took
the first 28 lines. File-summary evidence was sufficient to pass the old
value-of-information count gate even when no code-bearing source window was
available.

The corrected priority is exact surface, trace frontier, behavior anchor,
callsite, target declaration, and containing syntax before file start. A round
must be skipped with `no_code_bearing_bounded_window` when bounded retrieval
cannot produce code-bearing evidence.

## Fact versus interpretation

| Result | Authority |
|---|---|
| one module, 91 packages, 18 exact `func main()` anchors | deterministic local fact |
| undefined `auto.Assets` at the saved location | deterministic package-load diagnostic |
| zero saved surfaces | deterministic failed-stage result, not repository absence |
| exact paths, declaration lines, package IDs, and import edges | deterministic local facts |
| six candidate names, purposes, and semantic flow descriptions | model interpretation over bounded facts |
| architecture member IDs | deterministic local facts |
| architecture subsystem/component names and conceptual grouping | validated model interpretation |
| six local-only flow bundles | deterministic supporting evidence, not traces |
| research unsupported findings | model interpretation limited to supplied evidence IDs/windows |
| static registrations or calls | static evidence, not observed runtime order or execution |

## Initial trace preference

The densest saved exact evidence belongs to the crash-receiver executable:

- exact process declaration at `cmd/infra/stcrashreceiver/main.go:50`;
- local HTTP registration signals at lines 85, 86, and 92;
- exact `crashReceiver.ServeHTTP` declaration at
  `cmd/infra/stcrashreceiver/stcrashreceiver.go:24`;
- periodic maintenance signals in `diskstore.go:55-56`.

This is an evaluation preference, not a production special case. The generic
implementation should choose the highest-quality available entry seed and may
instead select the primary daemon when its exact bounded call chain is stronger
in a fresh analysis.

An honest first trace may end at an unresolved service/registration frontier.
It must preserve exact evidence, distinguish inferred static reachability from
runtime observation, and must not promote the periodic task into an unrelated
top-level trace.
