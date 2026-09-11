# Corpus, documentation and targets

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Corpus

- Extract deterministic repository facts locally, then send bounded,
  request-local evidence to the configured model provider. The initial
  repository-guidance file-role classifier may send the complete names-only
  safe-corpus dictionary as a lossless prefix-compressed path tree with
  compact `f*` leaves, the complete closed set of prose-file refs derived from
  that same dictionary, plus complete textual README and AGENTS.md documents;
  it sends no other source-file contents. Before any `f*` identity exists, exclude
  `.npmrc`, every `.env*`, installed dependency subtrees such as `node_modules`, and every `*.tsbuildinfo` file from the
  shared corpus, freshness state, model input, debug output, and publication.

- The selected repository is trusted and the tool is not a security boundary.
  Nothing is scanned or redacted, the run directory is as sensitive as the
  repository, the run manifest is a record and is never verified against the
  artifacts, and the report server opens the files a page names under the
  analysed root. The provider key is read from the environment and never
  enters a request body or a cache record.

- Repository changes during a run do not fail publication. Do not reintroduce a
  freshness gate or strict-snapshot mode.
- `--no-serve` requires resolvable GitHub or GitLab source links and fails in
  preflight with corrective flag guidance otherwise. A corpus file absent
  from the captured revision or changed locally never blocks HTML publication:
  keep its path and line as plain text with a `No source` hover explanation.
  Preserve the code cube, explanation and navigation, and every unaffected
  permalink. The outer run checks path availability once; its standalone
  manifest retains that list for repository-free saved rendering. Served
  reports may only
  add manifest-authorized local editor opening; do not add browser APIs for
  workspace reads, investigation, symbols, source context, or run selection.

## File identity

Every later stage derives authority from one immutable repository corpus.
Corpus collection now inventories current source files, recognized project
manifests and documentation on disk, including new and Git-ignored generated
sources. File refs and executable permissions come from that working directory,
independently of Git index membership. Later stages select from the same corpus.
The root `.repomapignore` lists literal repository-relative files or directories
to exclude; it is separate from `.gitignore`. Installed dependency trees,
virtual environments and known caches are excluded. A directory named `build`,
`dist` or `coverage` alone is not grounds to omit its sources. The current run's
output directory is excluded when it is inside the analyzed directory.

Remaining integration work: ordinary repository-state capture still requires
a Git HEAD, and claims extraction and report validation still require a
revision. A filesystem corpus works without Git; the complete ordinary command
does not yet. Those stages need optional Git metadata and content identities
for untracked inputs before Git-free execution is complete. Standalone GitHub
and GitLab publication checks every readable corpus path against the captured
revision once, using a NUL-delimited Git tree inventory and the analysis root's
exact repository-relative prefix. Absent paths, including submodule descendants,
and paths in the captured tracked changes retain their report content but have
no remote link. The owner requested plain path text with a `No source` hover
explanation, without interrupting publication or requiring extra actions.
Existing code cubes and their selection use source identity independently of
link availability. Unchanged sources keep their original permalinks. Child
targets reuse the outer check; the standalone manifest stores the unavailable
path list, so saved HTML rendering never reopens the repository or a provider.
This is publication metadata, not model input or an analysis exclusion. Local
serving still opens the analyzed working tree, including these paths. Corpus
tests cover identical inventory before Git init, before the
first commit and after a commit; they do not stand in for that integration.

## Repository guidance

The initial guidance classifier may send the complete names-only safe-corpus
dictionary as a lossless prefix-compressed path tree with compact `f*` leaves,
the complete closed set of prose-file refs derived from that dictionary, and
complete textual README and AGENTS.md documents. It sends no other source-file
contents.

The classifier restores accepted rows only to advertised file refs. Its
repository-guidance result supplies exact documentation inputs and optional
target evidence. It does not create program objects, program edges, or target
identity.

`documentation_reduce` consumes the complete validated guidance snapshot and
produces one sealed `reduced-documentation.json` handoff:

- an optional repository overview;
- source-bound claims and concepts;
- the exact guidance digest;
- the exact reduction digest.

Sparse reduced documentation is legitimate, including the canonical empty
result when there is no guidance authority. Every retained source must restore
to the exact guidance snapshot. The reduction is repository context for every
selected target's atlas context; it is not copied into adapter facts
or treated as source-code authority.

The 2026-09-09 validation correction accepts guidance files, classifications
and hypotheses independently. Invalid members retain a recorded reason without
erasing valid neighbours or the native target inventory. Documentation likewise
keeps valid source claims and concepts beside malformed ones. Independent source
and merge requests use the shared worker pool without cancelling accepted
siblings; real resource refusals split every affected request in the round.
Successful input snapshots stay unchanged for exact warm-cache reuse, including
after children are added. A failed merge preserves its already accepted input
claims, and an incomplete merge supplies no invented whole-repository overview.
Optional terms are collected only from fully accepted file/source scopes.

Source and merge packing find the largest complete request prefix by probing
exponentially growing windows, then searching within the last fit/refusal
bracket. This avoids re-encoding every growing prefix, without repeatedly
encoding the entire remaining reservoir when only a small window fits. Exact
provider preparation still decides fit; UTF-8 splitting, worst-case ordinal
reservations, materialization, request bytes and adaptive execution are unchanged.
An indivisible merge candidate is rejected before materialization even when it
follows a valid window. Other preparation errors retain their original cause.
Regression comparisons preserve every materialized byte while checking that repeated preparation is reduced.

## Target selection

- Analyze every eligible target by default. `--target` selects an explicit
  target; `--force-platform GOOS/GOARCH` overrides the normal Go platform
  selection.
- Run every active Go and Python target scout plus the JavaScript/TypeScript
  package-target catalog scout over the same repository corpus. Merge their
  exact file candidates and resolvable repository-guidance candidates into one
  repository-wide `TargetPortfolio` request, leaving out any candidate under a
  `.claude`, `.github` or `.vscode` directory (hooks, workflows and editor
  settings are never a product); the presence of one supported
  language must never suppress another. Bind one canonical required file
  representative for every exact native target, deduplicating a shared
  representative and never requiring every alternative file for the same
  target. Each native target has a separate closed target ref, even when several
  targets share a file. Every native ref must receive a placement decision:
  `standalone`, `seed_of:<ref>`, `shared_code`, `tool`, or `example`. A Go
  module library placed `shared_code` whose only consumer is one standalone
  executable is folded into that executable (`folded_into:<ref>` beside the
  model's decision in the journal): the executable's page claims the module
  root, so `internal/` and `pkg/` packages are its own boxes instead of a
  second "shared code" component. Two or more consumers keep the shared
  library.
  Missing or invalid decisions retain `standalone` with the original evidence
  and rejection reason in `target-placements.json`; there is no default seed
  owner. Exact equivalent argument-free Python launch forms receive one
  `launch_groups` owner choice in the same portfolio call. The group retains
  every original member, complete callable identity and source evidence in one
  provider window. A positive advertised owner choice restores one standalone
  owner and all original `seed_of` placements; missing or invalid choices keep
  every member standalone with their refusal, regardless of independent member
  answers. An explicit `separate` choice uses ordinary per-member classifications,
  including tool/example; equivalent utility forms do not become a product.
  Other targets retain the ordinary individual decision contract. A positive
  guard-to-library decision may also fold a launch into an advertised mandatory
  owner with an explicit `standalone` decision. Chains
  and cycles are invalid. Python ownership uses static declared packages,
  including `packages.find` where/include/exclude, separately from the full
  importable inventory. Launch bases, Go own-main consumers and other-module
  imports, and source-addressed fenced README commands/imports reach this
  decision before page analysis. Author commands remain documentation claims.
  Tools, examples and shared code keep their own complete analysis; shared code
  appears in a counted catalogue with links from its observed consumers.
  The owner accepts an author-written shebang as a native launch candidate.
  A fallback standalone role is not an accepted model decision and must not be explained as one. Original launch mode, executable bit and anchored relative imports inform the model; they never remove the shebang candidate locally or force a product count.
  Seeds keep their original source anchors inside the owner's ProgramIndex,
  without narrowing its library API. JS/TS package ownership stays separate.
  File refs remain exact restoration addresses, and positively supported
  guidance-only candidates may additionally be selected. Restore each file
  through exactly one adapter and apply validated placements to the typed plan.
  The portfolio chooses a retained repository default. An exact `--target`
  bypasses the model portfolio but must still
  resolve unambiguously through that same typed adapter boundary. Target scouts
  may not execute an adapter's page-local ProgramIndex, dependency, or semantic
  path. A compiler projection used to build that page, including the JSTS
  TypeScript Compiler API projection, likewise belongs only to selected typed
  targets, so an unselected language's page prerequisite cannot block an exact
  target owned by another adapter.

## Python package and launch observations

The Python adapter records static package lists in setup.py, pyproject.toml
and setup.cfg and package discovery where/include/exclude rules. Full package
names are matched, and namespace settings are respected. A project-name match
requires a corresponding real package. Dynamic setup calls are not evaluated
and supply no package declaration. This authority is separate from the complete
importable module inventory and does not itself classify a guard as an example.
For library ownership, only a guard inside the declared distribution is
advertised as eligible for a seed owner. Independently, executable forms with
the same complete argument-free launch callable can belong to one launch group:
console scripts, module launches and simple direct guards retain their original
anchors while the model chooses one owner. A shebang alone supplies no callable
equivalence. JS/TS source-owning package targets retain their
existing independent compiler boundaries, including packages without start/bin.
Exact Python launch files also supply their corpus executable bit and direct
module-level relative imports, including wildcard imports, with original source
anchors. These observations do not change the native candidate's identity or
remove an author-written shebang candidate or change its native identity.

## Typed adapter boundary

Each language adapter owns syntax, compiler/type-system mechanics, native
target interpretation, and exact local fact restoration. It returns one
atomic, language-neutral `programindex.Input` for its selected target. The
shared `programindex.New` boundary validates and seals that snapshot.

The adapter input contains:

- the exact `ProgramTarget`, its source refs, anchor, selector, and structural
  execution seeds;
- program objects with kind, display name, visibility, ownership,
  containment, exact source location, and optional signature;
- external symbols with their exact raw package origins and required,
  language-neutral `package` or `platform` authority kind;
- language-neutral relations with resolution, targets, witnesses, neutral
  patterns, and complete observed/omitted coverage;
- target-scoped deterministic dependency authority.

Adapter-private indexes may retain richer language facts, but shared semantic
and report code depends only on the sealed ProgramIndex and target-scoped
dependency authority. Adapters do not contain framework, protocol, or product
role allowlists.

Every eligible selected target runs its real adapter. Fixture or preset paths
may replace only provider responses in focused tests; they never replace
deterministic adapter execution.

## Scope in documentation evidence

Markdown sections retain the complete heading ancestry and original source anchors. An existing file-role hint is labelled as a model hypothesis, not native authority. A nested dependency README can explain that dependency without becoming project configuration. Context is preserved rather than hidden by a path blacklist. Terraform/HCL implementation extraction is not supplied merely by retaining such documentation; unsupported configuration remains an explicit evidence gap.
