# ProgramIndex language-adapter SDK

This is the cold-start path for adding a language when a reliable compiler or
extractor already exists. You should not need repository history or knowledge
of a language-specific semantic pipeline. The adapter emits one immutable
`programindex.Input`; `programindex.New` owns the canonical graph.

## Exact files to touch

For a new language named `jvm`, make only these product changes:

1. Add an adapter package, for example `internal/jvmprogramindex/`. It owns the
   extractor-to-`programindex.Input` translation and its focused tests.
2. Add or extend the one cumulative real-language fixture at
   `testdata/repositories/jvm/`, including exact inventory expectations.
3. Add one command-layer descriptor in
   `cmd/repomap/repository_target_registry.go` and its discovery/runtime
   callbacks in a language-owned `cmd/repomap/jvm_target_runtime.go`. The
   descriptor declares selector prefixes, display text, and every
   `AllowedLanguages` value the adapter may actually emit.
4. Add the language's dependency-catalog producer if its package authority is
   not already expressible by the shared dependency input.
5. Add one conformance test that calls
   `adaptertest.AssertConforms` on the real fixture adapter.
6. Update the source-bound inventories under `testdata/contracts/` for every
   new production or test file.

Do **not** add a language branch to documentation reduction, ProgramIndex
categorization, target-local grouping, cross-target matching, report schemas,
templates, CSS, or browser JavaScript merely to admit the language. If one of
those changes seems necessary, stop: either the adapter has not filled the
portable contract or the contract has exposed a language-neutral gap that must
be reviewed as such.

## One atomic snapshot, five logical tables

Implement one operation with this shape:

```go
func BuildProgramInput(facts CompilerSnapshot) (programindex.Input, error)
```

Capture `CompilerSnapshot` once. Do not expose `GetObjects`, `GetRelations`, or
other stateful getters that can observe different compiler states. Fill these
five logical tables inside the returned value:

| table | `programindex.Input` field | required authority |
|---|---|---|
| Target | `Target` | language, kind, stable selector, owned sources, anchor |
| Object | `Objects` | adapter-local `SourceRef`, neutral kind, visibility, optional owner/container/location |
| Relation | `Relations` | refs to objects, neutral kind, explicit resolution, complete observed/omitted counts, complete witnesses and optional nested syntax patterns |
| Seed | `Target.Seeds` | adapter-proved callable/surface roots; never naming heuristics alone |
| Coverage | `Coverage` | complete extractor ledger, including omissions/frontiers |

Refs are temporary joins within this one input. Do not construct canonical IDs,
sort rows, deduplicate rows, restore joins, or seal JSON in the adapter. The
common builder owns all of that:

```go
input, err := BuildProgramInput(facts)
if err != nil { return err }
index, err := programindex.New(input)
```

`Object.Name` is presentation text, not identity. For a non-module declaration,
emit its leaf name or owner-qualified display name without copying a repository
path into it; keep the exact path in `Location`. Equal names are legitimate:
`SourceRef` and the derived object ID keep declarations distinct inside one
target, while `SymbolLinkIdentities` are the only optional cross-target join
authority. Shared consumers must never split `Name` on a language-specific
delimiter to reconstruct any of these fields. Logical module and package names
may retain their language-native path-like spelling.

For an ordinary run, every `Target.Sources[].FileRef` and
`Target.AnchorFileRef` must be the exact `corpus.FileID` for the same path in
the run's immutable corpus. `programindex.New` validates joins inside the
adapter snapshot; the later shared categorization and grouping compilers also
validate target source refs against that corpus. A made-up adapter-local file
ref can therefore pass ProgramIndex conformance and still fail the ordinary
graph path.

Callable objects also need portable module context. Give a callable an
`OwnerRef`/`ContainerRef` chain that reaches an `ObjectModule` or
`ObjectPackage`; an exact source location alone is insufficient for shared
categorization, grouping, and matching context. A typical JVM snapshot is
package -> type -> method, with the method owned by the type and contained by
the package.

A minimal copyable reference is
`internal/programindex/adaptertest.ReferenceAdapter`.

## Resolution and external symbols

- `exact`: the extractor proved every retained target and no target was
  omitted.
- `alternatives`: the extractor proved a closed candidate set but not the
  runtime choice. Keep the observed/retained/omitted counts exact.
- `unresolved`: the runtime target is unknown. Do not manufacture a `ToRef` by
  name.
- For `invokes_external`, use `ObjectExternalSymbol` and fill the structured
  raw `External.PackagePath`, required `External.AuthorityKind`, optional
  receiver, and symbol name. `package` means ordinary external package
  authority; `platform` means standard-library or language-runtime authority.
  Derive this from the language's exact compiler/runtime inventory and fail
  closed when it is unknown. Never infer it from a path prefix. Presentation
  text in `Object.Name` is never package authority.
- Witness kinds and source expressions are adapter-owned source facts. Shared
  layers do not parse a language from them; an adapter-specific preparer may
  validate stricter syntax before constructing a domain input.

## Optional nested call patterns and values (ProgramIndex v11)

When a compiler can retain the structure of a call without assigning product
meaning to it, attach `RelationPatternInput` rows to the existing
`RelationInput.Patterns` field. A pattern is not a sixth table or a second
graph. It is neutral syntax owned by that relation. ProgramIndex does not know
that a package is an HTTP client, that a decorator is a route, or that two
paths should match.

Retain every source-distinct pattern that fits the closed shape. There is no
per-relation sample or truncation cap. Complete ProgramIndex row totals and
the former 64 MiB aggregate semantic-text and 128 MiB canonical-JSON sizes are
diagnostics only; identity, closed-shape validation, and machine
representation remain the actual authorities. Duplicate compiler witnesses at the same source
position remain witness accounting; they do not increase `PatternsObserved`
and are not pattern omissions.

Set `RelationInput.PatternsObserved` to the number of syntactically eligible
candidates the adapter actually observed. A normal relation that was not
inspected for patterns keeps zero and seals to a non-nil empty `Patterns`
array. If a candidate has a computed terminal selector and therefore cannot be
materialized, count it as observed without inventing a selector; the sealed
relation records the omission.

The closed forms are `PatternCall` (`call`) and `PatternDecoratorCall`
(`decorator_call`). A retained candidate supplies:

```go
programindex.RelationPatternInput{
    SourceRef: "call:src/service.ts:12:3", // unique only within this relation
    Form:      programindex.PatternCall,
    Selector:  "get",                     // exact terminal selector; never a sentinel
    Location:  &programindex.Location{Path: "src/service.ts", Line: 12, Column: 3},
    ResultRef: "object:call-result",       // optional exact result object
    ReceiverRef: "object:client",          // optional exact local receiver object
    ReceiverOriginRefs: []string{"object:client-constructor"},
    ReceiverOriginResolution: programindex.ResolutionExact,
    ReceiverOriginsObserved: 1,
    ArgumentsObserved: 1,
    Arguments: []programindex.PatternArgumentInput{{
        Position: 1,
        Kind:     programindex.PatternLiteralString,
        Value:    "/api/levels",
    }},
}
```

`Location` is the exact source position of this pattern, not a copy of the
compacted relation's representative location. Downstream navigation prefers
it so several retained calls in one relation remain distinguishable and
openable.

`ResultRef` and `ReceiverRef` are optional object joins and resolve to
`ResultID` and `ReceiverID`. A language adapter may materialize a neutral call
result object, cite it as one pattern's result, then cite the same object as a
later pattern's receiver. This records only the source/type-proved chained-call
provenance; it assigns no framework meaning and invents no runtime call.
Receiver origins are a separate optional object set with the same
exact/alternatives/unresolved meaning as relation targets. Do not invent an
object for an import binding or repeat an external callee already present in
the relation target. With no object authority, leave refs empty, resolution
empty, and observed count zero. Use unresolved with a positive observed count
only when the adapter really observed object candidates that it could not
materialize.

Each argument is keyed by exactly one of a one-based `Position` or a non-empty
`Keyword`. ProgramIndex derives its stable ID from the owning pattern and that
key, resolves optional `ObjectRefs`, and orders positional keys before lexical
keyword keys. The value shape is closed:

- `PatternLiteralString` (`literal_string`): `Value` contains the exact string
  and `Parts` is empty. Empty and whitespace-bearing source strings remain
  legitimate literals. Local artifact-size thresholds are warning-only and
  never bound, truncate, or reject them.
- `PatternStringTemplate` (`string_template`): `Value` is empty and `Parts`
  contains ordered `PatternPartLiteral`/`PatternPartHole` rows. Only literal
  parts carry `Text`; hole text is always empty. At least one hole is required.
- `PatternDynamic` (`dynamic`): both `Value` and `Parts` are empty. This records
  the frontier without reproducing an expression.

A dynamic argument may additionally retain every locally reconstructed string
as `ValueCandidates`. This is how a constant, local initializer, parameter
forwarding chain, or another compiler-proved value flow reaches shared
categorization, grouping, and matching without asking a model to recover the
value from source text. Each candidate is a literal or template and carries:

- `Resolution`: `exact` when the value is exact at this use, or `possible` when
  it is only one possible runtime value;
- `SourceKind`: `initializer` or `actual_argument`;
- complete `SourceObjectRefs` or `SourceArgumentRefs` provenance, with exact
  observed counts.

`initializer` requires at least one source object and no source argument.
`actual_argument` requires at least one exact nested source-argument ref, no
source object, and always remains `possible`. Set
`ValueCandidatesObserved == len(ValueCandidates)`; value reconstruction is
complete or the ProgramIndex fails closed. Exact provenance does not promote a
`possible` candidate into an exact runtime value or occurrence. Value
candidates are valid only on an argument whose direct `Kind` is `dynamic`.

For `*args`, `**kwargs`, computed selectors, or another observed form that
cannot honestly use these closed keys, increase the relevant observed count
without adding a fabricated retained row. The builder derives omitted counts;
it never truncates or repairs the adapter input. Pattern, argument,
receiver-origin, and argument-object totals are aggregated into `Coverage`, so
downstream coverage checks do not need language-specific accounting.

When an exact `passes_callback` relation came from one retained call argument,
set its structured `RelationInput.SourceArgument` to the owning relation
`SourceRef`, nested pattern `SourceRef`, and that argument's one-based
`Position` or `Keyword`. The builder resolves this tuple after all relations,
seals `SourceArgumentID`, and verifies that the argument's object IDs,
resolution, and observed/omitted authority exactly equal the callback
relation's targets. Argument position is part of the authority: passing the
same callable in two positions must remain two distinguishable provenance
rows, not one ambiguous singular source-argument join.

## Optional exact cross-target symbol identity

If the extractor can prove that a local public declaration and an external
symbol are the same callable identity, attach the same normalized tuple to both
objects:

```go
SymbolLinkIdentities: []programindex.SymbolLinkIdentityInput{{
    Domain:  "jvm-public-callable-v1",
    Parts:   []string{"method", "com.example.Service", "run", "(Request)Response"},
    Display: "Service.run",
}}
```

`Domain` namespaces the adapter and identity scheme. `Parts` are ordered,
non-empty, already normalized exact facts. `Display` is optional and has no
authority. The common builder seals a key; consumers compare only exact
`(Domain, Key)` and never parse `Parts`, `Display`, object names, or language.
The sealed row retains only `part_count` for warning-only scale diagnostics;
the raw parts are not persisted. Retain every exact alias/re-export identity;
former per-object and per-identity thresholds are warning-only. Do not emit
identity from a similar name or signature guess.

## Conformance test

```go
func TestAdapterConforms(t *testing.T) {
    index := adaptertest.AssertConforms(t, adaptertest.AdapterFunc(func() (programindex.Input, error) {
        return jvmprogramindex.BuildProgramInput(loadFixtureCompilerSnapshot(t))
    }))
    if index.Target.Language != "java" {
        t.Fatalf("language = %q", index.Target.Language)
    }
}
```

Run:

```text
go test -count=1 -p=4 -timeout=5m ./internal/jvmprogramindex
go test -count=1 -p=4 -timeout=5m ./internal/programindex/adaptertest
```

`AssertConforms` builds twice, validates the sealed index, performs a canonical
encode/decode round trip, and rejects nondeterministic output.

That structural check does not execute semantics or presentation. The smallest
end-to-end adapter conformance slice uses the generic command registry, then
runs the same shared sequence as every other target:

1. reduce repository documentation once and persist its exact artifact;
2. categorize the adapter's sealed ProgramIndex and reseal the same index type;
3. build and persist one `GroupsIndex` from the enriched index;
4. include that index in repository-level group matching;
5. bind the complete matched graph to `report.ReportData`, project the typed
   browser group graph, and render the existing Canvas.

Keep this as a synthetic test with local, request-bound provider responses. A
previously unseen language passing that sequence proves admission to the
ordinary shared graph/report contract without a model or presentation
allowlist.

## Common failures and what they mean

- `unknown object ref`: a Relation, Seed, owner, or container points outside
  this atomic snapshot.
- `observed count mismatch` or invalid coverage: the adapter hid a compiler
  frontier or supplied incomplete accounting. Record it; do not truncate.
- `not canonical`, duplicate, or identity mismatch after decode: the adapter
  attempted to own ordering/IDs, or output changed between builds.
- invalid symbol-link identity: domain/parts are empty, malformed, or two rows
  claim the same exact tuple with conflicting display text.
- aggregate/index byte warning: retain the complete target unchanged. These
  thresholds are diagnostic only; never split target ownership or truncate the
  graph because one is crossed.
- categorization, grouping, matching, or report projection rejects an otherwise
  valid synthetic language: treat that as a shared-contract bug. Do not add the
  language to a downstream allowlist.

## Definition of done

The real fixture discovers and selects the target, emits a sealed ProgramIndex
and dependency catalog, passes the conformance kit, and enters the ordinary
shared categorization/grouping/matching/report path without language switches
outside registry dispatch or adapter-private code. The generated report may
omit optional adapter extensions, but it must not require a special report
schema to exist.
