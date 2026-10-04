# Go native authority

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Build-selected scope

- The Go fact and target inventory excludes non-`DepOnly` `go list` root rows
  that have no build-selected `GoFiles` or `CgoFiles`. In particular, a
  directory containing only external `*_test.go` files is not an ordinary
  package when the product loads with `Tests=false`; its raw row may inform
  dependency metadata but must not enter package counts, target identity, or
  the typed ProgramIndex scope. A source-bearing package that fails type
  checking is not filtered and still fails its owning target closed.
- A main package that builds only with tags the repository's build
  descriptions give it is a target of its own tagged load (`TaggedBuild`,
  DISCOVERY "Go programs built with tags"); nothing else of the run's own
  build selection changes. cgo's generated wrappers (`_cgoexp_*`,
  `_cgo_cmalloc`) keep the column-less `//line` declaration the direct call
  index gives them, in the dynamic handoff index too.

The Go fact inventory also retains the complete build-selected package-origin
universe from every `go list -deps` row, including `DepOnly` rows. The Go
tool's exact `Standard` bit maps standard packages to `platform`; every other
known external package maps to `package`, and generated cgo `C` authority maps
to `platform`. An external target absent from that universe fails the adapter
closed rather than being guessed from its import path.

## A library's exports

A module library's entries are its API (PROGRAM_INDEX `target.exports`,
basis `visibility`, `goadapter` `libraryExports`): the exported functions,
and the exported methods of exported types, that its public packages
(`LibraryPackages`) declare outside `_test.go` files. A package under
`internal/` is none: only its own module may import it. A closure
(`Open$1`) is named after its function and exports nothing. The cumulative
fixture's library exports `PublishedRoot`, `ReadAliasedImports`,
`servicecfg.Address`, `PollIntervalSeconds` and `Limits.Describe`, never
`parsed.Raw` (its type is unexported), `internal/localstore.Get` or a test
(`assertGoLibraryExports`). An executable package exports nothing.

## Traversal and explicit narrowing

- The ordinary Go direct-call traversal is complete for the selected target:
  `--depth 0` and `--edges-limit 0` are the defaults and mean retain every
  exact call and edge across loaded repository declarations, including functions
  outside the launch tree. Positive values are explicit user-requested
  narrowing controls. Dynamic and unresolved call frontiers remain represented separately. Repository scale is neither a warning nor an implicit narrowing option.

## Packages imported for their effect

An import spec named `_` imports a package only to run its `init`, which
registers it with another library (a database driver with `database/sql`).
The dependency catalogue records the importing package in the dependency's
`effect_importer_refs` when every import of the path in the package's
build-selected files (`GoFiles`, `CgoFiles`, read by `go/parser` imports
only) is `_`; a file that cannot be read makes no claim. Only external
dependencies reach the systems question (the standard library's `_ "embed"`
or `_ "time/tzdata"` and the repository's own packages do not). The
published example imports its module's `driver` package so
(`assertGoEffectOnlyImports`). A repository package that registers itself
through a blank import (caddy's `modules/standard`) reaches a system only if
the program graph follows its `init`: a recorded gap.

## Dynamic receivers and callable transfers

The normal Go call-index pass now visits every loaded repository function,
including callbacks outside the launch call tree, and canonicalizes generic
origins. Anonymous functions retain their compiler signatures through the
Go adapter as well. Explicit depth/edge narrowing remains
explicit. Declaration candidates
include anonymous functions passed as callbacks; incidental closures are not
automatically added. Neutral callable bindings retain the source and destination
names, field/argument detail, invocation, resolution and source location. Calls
to external code retain qualified names, so `context.WithTimeout` does not lose
its identity before interpretation. The dynamic-handoff index also retains source
assignments to interface fields, keyed by the compiler's field declaration.
Local factory return values can resolve a stored implementation. The stores of
a field seen in the analyzed program are its values: one resolved value is an
exact call, several are alternatives, and only a store whose value cannot be
followed leaves an unknown. An implementation outside the repository, such as
`*sql.DB` stored in a sqlc `DBTX` field, is not an unknown: the call becomes an
`invokes_external` fact of that type's method (dispatch `interface`) with the
call site's arguments, and no unresolved `calls` relation is projected beside it.
Fields with the same name on unrelated types do not share candidates. Both the
call site and the constructor assignment survive projection; an assignment is
support for the call, not a second call at the constructor line. This recovers
`quotaKVServer.Put -> kvServer.Put -> EtcdServer.Put` in etcd without any
framework-specific rule.

A value stored under a branch is still a value of its field: a field's values
are every value its stores put there, one exact, several alternatives (owner,
2026-09-30: no cautious unknown; several targets are alternatives). A branch is
an if, a case of a switch or type switch, or a select clause of the storing
function; a loop body, a store after an early return and the body of a function
literal are not. litestream's `NewReplicaFromConfig` stores, in each case of a
switch on the replica's type, what that case's factory returns into
`Replica.Client`, so `Replica.Sync`'s `r.Client.WriteLTXFile` calls the file,
S3, GCS, ABS, SFTP, WebDAV, NATS and OSS clients' `WriteLTXFile` as
alternatives, each with an `interface_field_assignment` witness at its store
(`observed receiver assignment for …`); the store of a value no one follows
(a client its factory registry returns) keeps its unknown, so the call is
alternatives with that target omitted. The fixture's `replica.sync`
(`storefixture/command_table.go`) calls `bucketStore.Put` and `diskStore.Put`.
Only a parameter stored under a branch leaves its field open, as in the C
adapter: it joins every caller's argument whichever field the branch chose, so
a `register(readable, h)` that stores `h` into `read` or `write` would give
each field both handlers. What such a store brings becomes an
`interface_field_assignment` witness of the calls through the field at its
store (`X stored in T.field under a condition`), never a target, naming the
implementation by identity too (`object_id`), so the map draws the call's
possible arrows as the C adapter's stores do; an implementation outside the
repository gives no `invokes_external` fact there. A nil store puts nothing
callable there, so a branch around it opens nothing, as the C adapter keeps no
null store. When another package declares the field's interface, the call
keeps its `invokes_external` fact of that method, and the `calls` relation of
the repository implementations is projected beside it. A function-typed field
keeps its own rule (a call through it is exact after one store in the
allocating block, else unresolved). The other languages have not applied the
owner's 2026-09-30 decision yet, recorded missing rather than fabricated: the
C adapter keeps a function pointer stored under a branch a witness (C),
Python an attribute stored under a branch (a name so stored now calls its
stores' functions as alternatives, PYTHON, Handler tables), JS/TS a property
(a variable calls its stores' functions, JSTS), and Clojure's protocol
dispatch stays unresolved (CLOJURE); their fixtures keep today's witnesses.

Dynamic value traversal reuses immutable summaries within one root and exact
interface method. The function key also retains `throughFlow`; factory result
indices remain attached to their own SSA values. Only subtrees that completed
without an active-path cycle enter this local memo. Cyclic results propagate
their dependency on the current path and continue to use the original traversal.
Merging retains child-order evidence, every exact assignment location and the
number of unresolved paths per incoming edge. An integer representation overflow
is a terminal extraction error, including when the same callable resolver feeds
external-call argument facts. This adds no persistent cache, interface-method
cap or inferred candidate, and does not promise linear traversal of cyclic graphs.

Interface-valued arguments now retain the concrete methods of the declared
interface when their implementation is resolved from the actual value, local
factory return, or observed alternatives. The existing transfer slot records
the declared interface and method. An unrelated compatible type cannot supply
an implementation; extra concrete methods outside the interface are excluded.
These are object-registration observations, not callback executions: they are
`binds_implementation` relations, while a callable value passed as an argument
remains `passes_callback`. On etcd,
quotaKVServer.Put retains the RegisterKVServer argument at grpc.go:80 and the
separate local KvServerToKvClient adapter binding at v3client.go:33.

An interface value stored through a repository constructor parameter retains
the concrete implementations supplied by every actual static repository call
to that constructor. A non-call use, missing argument or caller outside the
selected repository scope leaves that path of the value open. The declared
method limits eligible observed implementations, and the cumulative Go fixture
requires the constructor-injected facade to retain `storedEngine.Put` as its
one observed target.

### What a call runs under

A call's guard (PROGRAM_INDEX) is read from the already loaded syntax and
types, once per target (`call_guard.go`): an if's body and else, a case or
select clause and the right operand of `&&` or `||` are arms; the arm taken
when an operand whose type is identical to the predeclared `error` is not
nil (the body of `!= nil`, also as an `&&` conjunct; the else of `== nil`),
what builtin `panic` is handed and an arm ending in `panic` are `error`; a
function literal's body starts afresh; a call in a condition, a tag or a
case's list is in no arm. A call edge folds its sites as it folds them into
one edge: one unguarded site leaves it unguarded, else the weakest stands,
kept out of the edge's identity; an interface invoke or an external call
takes its sites' guards. storefixture/handoff_flow.go's CheckedStore checks
each kind (`assertGoCallGuards`). `os.Exit` and `log.Fatal` are names, not
structure: not read.

### A value no observed flow gives

An interface call follows the repository's implementations (owner). On
2026-09-30 at 08:12:54 UTC the coordinator asked, as item 9: «Интерфейсные
вызовы. Идти по реализациям как по альтернативам?»; at 08:51:11 UTC the
owner answered: «"Это близко к «размытому разрешению», которое вы
запретили" - я никогда такого не запрещал». On 2026-09-16 he had ruled one
known target `exact`, several `alternatives`, none `unresolved`, with no
cautious unknowns. The September implementation followed the
implementations the field's stores put there (observed flow, above). A call through a value of an interface the
selected repository declares that no observed flow gives at all (no
candidate: a parameter of a function no repository code calls, a closure's
captured value, the result of another interface call) applies the same
approval: its targets are the methods the repository's named non-interface
types implementing the receiver's static interface declare, selected from
each pointer method set by `types.Implements` (the matcher of the
`implements` facts below, shared), one exact, several alternatives, no
unknown beside them. A method promoted from an embedded type is that type's
own, once; one promoted from an embedded interface is no implementation; a
generic type is not enumerated. The handoff candidate's evidence is
`interface_implementation`, and the ProgramIndex relation says
`basis: implements` (PROGRAM_INDEX), so a reader and the page never take it
for a traced runtime binding: "implemented by X in this repository". A call
with an observed value keeps that value's basis, an open path beside it
included (litestream's `WriteLTXFile`, above), and an interface declared
outside the repository (`error`, `io.Writer`) is filled by no repository
type. Stores under a branch that brought no candidate stop being witnesses
once the call is resolved. On etcd's server target, the gateway's
`server.Campaign` in `RegisterElectionHandlerServer`'s handler, which no
repository code calls (etcd registers the gateway in client mode), calls
`electionServer.Campaign`, `electionProxy.Campaign` and
`UnimplementedElectionServer.Campaign` as alternatives; its unresolved
interface calls fall from 836 to 86 (325 now exact and 425 alternatives on
the implements basis, 154 of them with a generated `Unimplemented*` stub
among the targets, 18 with the caller itself, a wrapper implementing the
interface it calls), the rest through `error` and anonymous interfaces or
with no repository implementation. The fixture's
`RegisterTicketHandlerServer` (two implementations, a type embedding one, a
type embedding the interface, a method of another signature) and
`RegisterReceiptHandlerServer` (one, exact) check it, with `unknownFacade.Put`
and the command table's `fire` through `write`
(`assertGoOpenInterfaceCallsFollowImplementations`).

Native equivalents, probed on one small repository per language (a base or
interface `Store` with two implementations, a `save(store)` called with one,
and a `register_store_handler(routes, store)` no code calls), are recorded
missing, not fabricated: Python resolves `store.put(key)` on a parameter
annotated with the repository's ABC exactly to the abstract `Store.put`, its
subclasses' overrides not alternatives, the called `save` included; JS/TS
leaves the call through an interface-typed parameter unresolved, the classes
that `implements` it not joined; Clojure resolves a protocol call `(put!
store k)` exactly to the protocol's `put!`, the `defrecord` implementations
not joined; C has no interfaces, and a function-pointer field keeps its
stores (C).

## Interface implementation matching

The exact Go analysis input owns `MatchInterfaceImplementations`. When enabled,
the core-object pass uses the already loaded `go/types` universe to enumerate
every selected-repository named non-interface type whose value or pointer method
set satisfies a selected-repository interface. An inverted method-name index
narrows candidates before `types.Implements` performs the authoritative check;
this adds no source read, package load, SSA build or call-graph traversal. The
ordinary Go adapter enables the option for every selected target, including
module libraries; direct cube callers may enable or disable it explicitly.

Each accepted type pair projects one exact `implements` relation from concrete
type to interface and each directly owned matching method projects another
exact `implements` relation from concrete method to interface method. Value and
pointer method-set evidence remains explicit. These compatibility facts do not
claim construction or assignment and never replace an observed binding or an
observed call's targets; a call no observed flow gives a value takes the
implementations they name as its own targets, on the `implements` basis
(above). The cumulative fixture requires `compatibleOnlyEngine` and its `Put`
method to match `FieldStore` despite never being assigned to that interface.

## Receiver fields and bindings

Callable bindings now retain literal assignments to other fields of the same
SSA receiver in that function. Referrer identity keeps two command/worker
objects of the same type separate; conditional or later stores remain separate
anchored observations, not final runtime values. Named and anonymous callbacks
share this mechanism. No field names or framework types drive extraction.
ProgramIndex carries these as `callable_receiver_field` witnesses, and the
atlas attaches them as binding evidence rather than additional registrations.
A callable bound into a field of a value whose type another package declares
(`&cobra.Command{Use: "serve", RunE: run}`) is also projected as the
construction of that value: an `invokes_external` relation to the type with
the `construct` invocation, whose one call pattern carries the string literals
stored beside the callable as keyword arguments and the bound field as the
keyword argument the `passes_callback` crosses by. Go constructs where other
languages call a constructor, and the facts stage reads both as one
registration shape. A value of a repository type is no such construction.
A callable the code assigns to the field of an outside value it already holds
(`fs.Usage = c.Usage`, `srv.Handler = mux`) constructs nothing: the dynamic
handoff marks the binding `assigned` (a selector names the field, where a
composite literal's element has none), and the relation names that field as
an outside symbol with the field's declared type as its signature,
`flag.FlagSet.Usage` (`func()`) and `net/http.Server.Handler`
(`http.Handler`), with no invocation, a `go_field_store` witness and one
relation per store; its pattern's word is the field and its keyword
arguments are the literals the same value received and the bound field. The
registration fact and the reading's outside symbol are then the field, as a
C table row names its record's field (`kvd.h.kvCommand.proc`), so the model
is asked what a callable stored in `flag.FlagSet.Usage` becomes, not what
one handed to `flag.FlagSet` does. `tool_cli.go`'s `toolCommand.Run` and
`server_state.go`'s `ServeStateStatus` are the fixture's cases
(`assertGoOutsideFieldStores`);
litestream's 20 `fs.Usage = c.Usage` stores (14 in cmd/litestream, 6 in
cmd/litestream-test) name `flag.FlagSet.Usage`. The C adapter still names a
store into a platform record by the record (`act.sa_handler = onSignal` is
handed to `struct sigaction`, C), and Python and JS/TS have no store of a
callable into an outside object's attribute as a registration: a recorded
difference, not an equivalent.
The own callback sees its object's fields; a neighbouring caller's registration
retains just the binding shape and source, so a shared error helper does not
inherit every command's help text. Canonical sealing, independent copies,
source anchors and a two-object fixture verify the underlying facts.
The atlas now canonically orders and exactly deduplicates each binding's
source-evidence set before deriving its identity key. The same Freqtrade Thread
registration occurred four times solely because target views ordered its
start/join/is_alive observations differently. Canonicalization produces one
binding while retaining every source anchor and the literal thread name.
Argument order, different field values, callback targets and source sites are
unchanged. A real cumulative Python extraction across four target views and
generic contrasting cases cover this correction. It changes affected graph
and exact-request hashes, independently of the byte-preserving loading change.

## What a function returns when it fails

A repository function's result value (`result_value`, surfacediscovery
`sourceReturn`) is the alternatives of its returns' first results, leaving
out each return that hands back constant zero values (`""`, `0`, `false`,
`nil`) beside an error that is not the nil constant (`return "", err`):
the function failing, whose caller uses no other result (2026-09-30). A
function that only fails keeps its returns. litestream's `expand` returned
`""` beside its error, so `db.path + "-wal"` walked to `-wal` alone; the
fixture's `destinationBase` fails the same way and
`DestinationThroughAFailingHelper`'s address is
`https://versioned.example/v1/items` alone (`assertGoSourceValues`).
Python, JS/TS and Clojure raise instead of returning an error beside a
value, and C's error returns (`-1`, `NULL`) are untyped conventions: no
equivalent is derived.

## Calls in package-level variable initializers

Go evaluates a package-level variable's initializer in the package's
synthetic initializer, which declares nothing, so a call written there
(`var schemaChanges = map[...]...{v: {addNewField(...)}}`) had no caller the
index could name and its callee looked unused. The direct-call index (version
15) now records each exact repository call the synthetic initializer makes
inside a package-level `var` specification's value as an edge whose caller is
that variable (`DirectCallVariable`: its name, type, exported state and whole
specification), at the call's own position. ProgramIndex projects the
variable as a `variable` of its package and the call as an ordinary exact
`calls` relation; the places graph lifts a package-level variable that owns a
call as a declaration, as it lifts a module body that owns one. A
specification of several names with one value gives the call to its first
name; a blank `var _ = f()` is the variable `_`. A call the synthetic
initializer makes outside any variable's value (an `init` function, an
imported package's initializer) is the runtime's own order and stays out, and
a call of an outside package there stays the package's unresolved
`synthetic_caller` frontier. The cumulative fixture's
`internal/storefixture/command_table.go` checks it: `defaultCommands` calls
`namedCommands` at line 164 (`TestCumulativeGoMapOfParts`), so
`namedCommands` has a user and goes with `defaultCommands` as a helper.

Native equivalents: a Python module-level assignment's call and a JS/TS
top-level `const` initializer's call are the module body's
(`src/fixture_app/cli.py:44`, `src/route-mounts.ts:3`), and a Clojure `def`
value's call is the var's, like Go's (`default-greeting` at the end of
`src/example/core.clj`); each fixture checks its call's owner. C has no
equivalent: a file-scope initializer holds constant expressions only, so no
call is written there, and a function it names is a hand-over from the
variable (C's command table).

## Inputs a call's words declare, and what Go does not have yet

A call of an outside symbol given words (`fs.String("config", "",
"config path")`, `flag.NewFlagSet("tool-list", …)`) is asked on its own what
the words become (READING, the `atlas_api` per-call question); the fixture
asks `flag.String`'s call at `internal/storefixture/destinations.go`. The
answer is the call's: a call answered with an entry kind is an input whose
handler is not established. `internal/storefixture/tool_cli.go`'s
`ToolCommand` holds the per-call contrast and the objects inputs are
declared on (`TestCumulativeGoInputsAreAskedPerCallAndCatalogued`):
`strings.EqualFold(os.Args[1], "check")` and `strings.EqualFold(level,
"default")` are asked apart, and `port` and `strict`, declared on the flag
sets `flag.NewFlagSet("serve", …)` and `flag.NewFlagSet("check", …)`
make, are two catalogues of one function. J1, a word entry joined with the
hand-over made on its own result, has no Go equivalent: the standard
library has no call naming an entry whose result another call hands a
handler to. Not recorded yet, and so asked nothing:

- a package-level variable's initializer calling an outside symbol
  (`var verbose = flag.Bool("verbose", …)` at the end of the same file):
  the synthetic initializer's outside calls stay its unresolved frontier
  (above), so the fixture's `flag.Bool` is not asked;
- package-level composite tables of names and tables inside functions, so
  a handler's lookup of a map with what it was handed (`commands[name]`, an
  index expression and no call) lists no values (READING, tables a handler
  looks up);
- `handlers[name] = fn` registries;
- a callable the repository's own function keeps (S1) is not enabled.

### Words a value is compared with

A value a function body compares with two or more different string words
in two or more cases is one comparison (PROGRAM_INDEX `comparisons`,
DirectCallIndex 17): the cases of a `switch` on a string value (a case's
constant expressions are its words, named constants included) and the `==`
comparisons of the same expression with a string constant, joined into one
comparison by the expression (a variable by its declaration, anything else
by its text). A case's branch is its clause; comparisons in one if
condition, through parentheses, `||` and `&&`, are one case whose branch
is the if's block, and one in a tagless switch's case its clause. A case
whose branch calls the program's own code is handled there (READING):
the fixture's `RunSubcommand` handles serve and check, runServe and
runCheck their reach; litestream's `Main.Run` each subcommand. The typed
syntax gives cases, words and lines; SSA, which compares a switch's tag at
each case expression and an `==` at its operator, gives the value's
origin. Source values follow a slice element (`IndexAddr`: litestream's
`cmd, args = args[0], args[1:]` is `one of: "" | element "0" of parameter
#2 args of Run`), write a constant index as its number and name a package
variable they do not follow (`os.Args`). The fixture's `RunSubcommand` in
`internal/storefixture/tool_cli.go` is litestream's `Main.Run`: a switch
with `"check", "verify"` in one case and a default branch comparing the
same `cmd` with the help words, one comparison of three cases;
`IsDefaultLevel`'s lone `==` is none, and `ToolCommand`'s
`strings.EqualFold` calls stay calls asked on their own
(`TestEveryLanguageRecordsAMultiWayDispatchAsOneComparison`,
`TestEveryLanguageAsksAComparisonOnceAndMakesAnInputPerCase`). Like field
accesses, comparisons are recorded where a node's calls are. A rune switch
(a lexer's `case '('`) is not recorded; C's character switch is.
litestream's cmd/litestream records 14 comparisons (the subcommand switch
of `Main.Run` with 17 cases) and cmd/litestream-test 3 (`fs.Arg(0)`'s
switch and its `help` check, 6 cases).

### Spellings of one value

A call that reads the same value as an earlier call names that call's
pattern (ProgramIndex 24 `same_value_as`, from the typed syntax in
`surfacediscovery/same_value_calls.go`): calls of one callee (go/types
`typeutil.Callee`), written the same but for the string literals the call
is given, either the operands of one `||` written the same around their one
call, or the arms of one if/else-if chain whose headers (init and condition)
are written the same but for their call's words and whose bodies are
written the same. Each later call names the first of its group; the
adapter keeps it only when the relation keeps both patterns, which one
caller's calls of one callee always share. `&&` is none (both values are
needed), and so are arms storing different fields and a header holding two
worded calls. litestream's `s3/replica_client.go` records
`query.Get("storage-class")` after `"storageClass"`, `"part-size"`, the
`sse-*` spellings, `"force-path-style"` in its `||`, and the environment
variables `LITESTREAM_ACCESS_KEY_ID` after `AWS_ACCESS_KEY_ID`;
cmd/litestream's `main.go:1522` its own `storage-class`. A condition naming
several words through calls (`strings.HasPrefix(host, "10.") ||
strings.HasPrefix(host, "172.16.")`) is the same fact, as `case "a", "b":`
is one case. The fixture's `ReplicaOptions`
(`internal/storefixture/replica_options.go`) holds both shapes and the
contrasts (`TestEveryLanguageKeepsTheSpellingsOfOneValue`,
`TestGoSpellingsOfOneSettingAreOneInput`).

### Settings a structure's tags name

A field whose struct tag names a key (the object aliases below,
`yaml:"dbs"`) is asked on its own what that key is (READING, the
`atlas_inputs` field question): a setting of the program's configuration
file, or none. The tag's format name is data as written; nothing in the
code names a library or a format. The question shows the field and its
type, its structure and file, the tag as written, and what the facts show
the structure is used for: each call of an outside symbol given a value of
it (with the declaration making the call and the call as written) and each
field of another structure typed with it, followed by that structure's own
use. Both come from go/types, never from names matched as text: the value
an outside call's argument is given records where the repository types its
static type names are declared (the source value's `Types`: `&config` of
type `*Config` names Config, `[]DatabaseInfo` names DatabaseInfo, an
`error` beside it names none, `oss.NewClient(cfg)` with an `*oss.Config`
names no repository type), and a field records the same for its declared
type (ProgramIndex `Object.Types`: `DBs []*DBConfig` names DBConfig; the
embedded `GetLevelsInfoResponse` of the cumulative fixture's
`EmbeddedLevelsInfoResponse` names it,
`TestCumulativeGoRepositoryDiscoveryAndProgramIndexContract`). A named
type is not looked into; pointers, slices, arrays, maps, channels, an
unnamed structure's fields and a generic type's arguments are. litestream's
`yaml.Unmarshal(buf, &config)` is the one call decoding `Config`; `DBConfig`
reads "the type of field DBs (yaml:"dbs") of Config, which is given to
gopkg.in/yaml.v2.Unmarshal …", and the CLI's JSON results read the
`json.MarshalIndent` call they are printed with. The echo fixture asks its
four JSON payload fields (the reply's with `Context.JSON`) and its preset
answers none (`TestEchoPresetReadingTurnsRegistrationsIntoOperations`); the
reading test `TestATaggedFieldIsAskedWithItsStructureAndAnsweredSettingIsAnEntry`
holds the setting shape, and the cumulative fixture holds it on real facts:
`ServerConfig`'s `listen` and `data_dir`, decoded by `json.Unmarshal(raw,
&config)` in `LoadServerConfig`, are settings declared on that call, while
the response structures' fields are asked and answered none. Not recorded yet: a key a structure's
`UnmarshalYAML`/`UnmarshalJSON` method reads itself, a map-typed field's
keys, an anonymous structure's fields (`var raw struct{…}`), which are no
type's members, and a value handed through a repository helper typed `any`
before an outside call (litestream's `writeJSON(w, resp)`: the call is
given an interface and names no type).

## Programs a call starts

`Revision` in `internal/storefixture/destinations.go` runs
`exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()`: the
launching call gives the words `git`, `rev-parse`, `HEAD`, and `Output` is
made on its `call_result`, so the reading keeps one boundary (READING).
`RunHook`'s `exec.Command(hook, args...).Run()` gives no word: its program
stays not established. `RevisionOf` builds its command on either branch of
an if/else (`cmd = exec.CommandContext(ctx, "git", "rev-parse", ref)` or
`… "HEAD")`) and calls `cmd.CombinedOutput()`: the receiver is the
`alternatives` of the two `call_result`s, so the call on it is both
launches', each naming git.

A value SSA joins at a control-flow merge (a φ) is the `alternatives` of its
incoming values, in edge order (a value that is the same on every edge is
that value): none is picked, and an edge whose value is not followed (a nil,
a load through memory) stays its `unknown` part, so the list is honest. A
join already expanded in the same recorded value (a later join reading an
earlier one on both edges, as conditional `q += …` appends do) stays the
`unknown` "conditional value" frontier where it comes again: a value tree
cannot share a node, and expanding it again would grow with the paths
through the code, 2^n for n appends, not with the code. A loop's join
reading itself is the "cyclic value" frontier as before. A field stored
through a pointer (`c.cmd = exec.CommandContext(…)` then `c.cmd.Start()`)
is still read as the `field`, not the value last stored there.

## Goroutines a function starts

A `go` statement's call carries the shared `goroutine` invocation. With one
exact repository callee it is a registration handing that callee over, its
word `go` (PROGRAM_INDEX); a closure written in the statement is handled by
the one repository function it calls itself. The reading asks each statement
what the started function becomes (READING, starting statements). The
cumulative fixture's worker service (`cmd/worker` → `StartBackground` in
`internal/storefixture/runtime_registrations.go`) starts `RunCommitWorker`
directly, the compactor inside a wait-group closure (handled by
`RunCompactor`) and a one-shot cache load (`loadCache`, answered none by the
preset); `time.AfterFunc` stays a registration of its outside symbol and the
finite retry none (`TestCumulativeGoStartsAreAskedPerStatement`). Its status
route (`cmd/worker/status.go`) compares a field of the request it was handed
with `HEAD` and makes `r.Header.Get("X-Verbose")`, the route's sub-arguments,
never asked, while `fmt.Fprintf(w, …)` is asked (READING, K3). Equivalents:
C's `pthread_create` and Python's `Thread`/`Process` hand a callable to an
outside symbol and are asked `binds`; Python's `asyncio.create_task` is this
registration (PYTHON); JS/TS has no statement that starts a function and
Clojure's `future`, `Thread.` and `core.async/go` are recorded as missing
(CLOJURE).

## Handler tables and stored callbacks

A function of the program's own that keeps a handed callable (`l.handlers = append(l.handlers, h)`, `s.onRead = fn`) is a registration's receiver by the general rule (PROGRAM_INDEX), but the Go adapter records no parameter stores, so such a call registers nothing: a recorded gap, as in Python, JS/TS and Clojure. C records them (C "Callables the program's own functions keep").

These are the Go equivalents of the C adapter's command table, its callbacks
stored under a branch and its calls through function-pointer fields. The
cumulative fixture's `internal/storefixture/command_table.go` checks them:

- A table of named handlers built inside a function
  (`[]commandRow{{Name: "get", Arity: 2, Run: getCommand}, ...}`) gives each
  row its own exact `passes_callback` binding. Its `go_ssa_dynamic_handoff`
  witness makes the binding row, and its `callable_receiver_field` witnesses
  carry that row's literals and no other row's (`Name = "get"`, `Arity = 2`).
  A value of a repository type is no construction, so a row stays a binding.
- A call through a function-typed field is exact when the value it reads was
  allocated in that function (or returned by a repository factory) with one
  store to the field, in the allocating block, before the call
  (`RunSingleHandler`). A field filled through a parameter under a branch
  (`eventLoop.fire`) and a store under a branch (`RunChosenHandler`) leave the
  call unresolved, as the C adapter does. The Go call lists no candidates,
  where the C adapter names each stored function as a witness. Each handler
  keeps its exact callback at its `register` call.
- The same loop with interface-typed fields (`readyLoop`): `register` stores
  its handler parameter into `read` or `write` under a branch, and
  `RunChosenReady` stores `acceptReady{}` into `read` under a branch. That
  value is `read`'s: every call through `read` calls `acceptReady.Handle` as
  an alternative, at its store (`observed receiver assignment for
  (acceptReady).Handle`), and `register`'s parameter leaves both fields open,
  its handlers their witnesses at its store (`(acceptReady).Handle stored in
  readyLoop.read under a condition`), joined from both of its calls as the C
  adapter joins what callers pass; the call through `write` stays
  unresolved. Each handler keeps its exact `binds_implementation` at its
  `register` call.
- The same choice for fields whose interface `fmt` declares (`namedLoop`): the
  call through the field a branch stored calls `acceptName.String`, exactly,
  beside its `invokes_external` fact of `fmt.Stringer.String`, and a field
  cleared to nil under a branch keeps the name stored before it as an
  alternative.

- A call of a function's own func-typed parameter (the call's SSA value is
  the parameter, so the function never rebinds it) calls what every static
  call of the function hands there, as the Python and C adapters join a
  parameter's callers: the functions, closures and method values the
  arguments are, one exact, several alternatives, dispatch `function_value`
  (`throttle`'s `step()`: `processRunning` and `processStopped`; `runOnce`'s
  `job()`: `acceptJob`). A caller handing any other value, and a function
  used as a value (handed over, stored, a method value or expression) or a
  method whose name an interface call invokes, leave the call unresolved;
  each function a call hands is then a `function_value_store` witness at
  that call (`runAny`: `flushJob passed to runAny`). Only the call through
  the parameter is joined: a parameter stored into a field or handed on keeps
  its own frontier, so `register(readable, h)` still gives neither field
  both handlers. A closure calling its enclosing function's parameter is not
  joined. Tests are not loaded, so no test hands anything.
- Go has no conditional expression. The equivalent of C's callee a condition
  chooses (C) is a local function value a branch chooses: after
  `tick := tickMillis` and `if seconds { tick = tickSeconds }`, `tick(ms)`
  calls the SSA phi of both functions, so both are its alternatives, dispatch
  `function_value` (`WatchTick`).
- A call through a variable a closure captures
  (`h := a; cb := func() { h() }; cb(); h = b`) is an unresolved
  `function_value` call with no candidates. The variable lives on the heap,
  and the SSA call through it names no function. The 2026-10-03 probe found
  this for a closure called before and between the stores, one returned,
  and one handed to a call. Go never claims a target there. JS/TS and
  Python instead order such calls by the closure's releases (JSTS, PYTHON,
  Handler tables).

Missing equivalents, recorded rather than fabricated:

- GroupsIndex derives no dispatch site on the fixture: `DispatchCommand`'s
  looked-up row call stays unresolved (below), so no input is dispatched from
  it, and only a field read (Field reads and writes, below) enters a part by
  a read: a package-level variable's read is none (READING, reach). The Echo
  preset checks the route's reach.
- No Go function is proven `unreachable` (the C adapter's per-program fact,
  PROGRAM_INDEX). The SSA call graph and its dynamic-call candidates do not
  see every way a function runs: reflection (`reflect.Value.Call`,
  `MethodByName`, `text/template` method calls), interface methods the
  standard library calls on values handed to it (`String`, `ServeHTTP`,
  `MarshalJSON`), `//go:linkname`, cgo `//export` and `plugin.Lookup`. A
  boundary in a Go package several commands link stays with every command,
  and a part leaves no command's map as code that command never runs
  (READING). Two commands sharing a package therefore list nothing either
  never runs, and no declaration is shown "run by" the other (REPORT); files
  are judged dead repository-wide, against every command's seeds.
- A call through the field of a row found by a lookup (`DispatchCommand`)
  stays unresolved: a function-typed field is followed only on one allocated
  value. The C adapter gives such a call every function stored into that field
  as its alternatives, so it reaches each handler of the table.
- A package-level table (`var commands = []commandRow{...}`) is filled by the
  synthetic package initializer, which the binding capture skips. A map of
  handlers (`map[string]func(){"ping": ping}`) is filled by map updates, not
  field stores. Neither keeps a binding or any other relation to its handlers,
  so neither gives the registration the C adapter makes of a table row that
  names its function by a string literal (the owner's decision of
  2026-09-26).
- Go emits no `reads` relation for a package-level variable or table: a
  function that indexes `var commands = []commandRow{...}` or reads a
  package-level `var symbols = map[string]uintptr{...}` has no relation to it.
  Python, JS/TS, Clojure and C emit one per read site (C's `printSymbols`
  reads `symsTable`, as Redis's `findFuncName` does), so a Go table or global
  has no users in the fact graph and the map places it by its file alone.
  A Go declaration's `uses` in the places graph (READING) therefore hold
  hand-overs only: `TestCumulativeGoMapOfParts` checks that
  `command_table.go`'s `commandTable` hands `getCommand` over and that
  `registerRouteDefinition` hands `http.HandleFunc` the closure
  `requireRouteToken` returns. A function value stored in a package
  variable, table or slice (`getCluster = srv.GetCluster`,
  `append(filters, filterNoPut)`) leaves no relation at all, so the function
  has no use there; recorded, not patched. The role split's helper question
  (READING) therefore never shows a Go declaration's `read_by`; a function
  used only as such a value has no user, so it is no helper by code rather
  than asked, while a Go variable, whose reads are never recorded, is always
  asked. The fixture's `lookupCommand`, called only by `DispatchCommand`,
  goes with it; `DispatchCommand`, which nothing calls, is not asked.
- A field a composite literal's element sets (`&Store{dbs: dbs}`) is no
  write, as a C designated initializer is none: only a selector names a field
  (Field reads and writes, below). A selector in a package-level variable's
  initializer is in no function body and reads nothing, where the calls
  written there have the variable as their caller. A method of an unexported
  type no code calls or converts is not in the SSA program's functions, so,
  like its calls, its field accesses are not recorded.
- A row storing two callables (`{Name: "get", Run: getCommand, Preload:
  preloadGet}`) keeps two bindings. No Go row is a registration, so the C
  rule that such a row is one input has nothing to apply to.
- An open interface field is decided at its stores, not at the calls that
  reach them. A helper that stores its parameter unconditionally
  (`func (l *L) setRead(h H) { l.read = h }`), called under a branch with a
  parameter its caller was given (`if readable { l.setRead(h) } else {
  l.setWrite(h) }`), still joins every argument of that caller's callers: the
  call through `l.read` keeps both handlers as false alternatives. The C
  adapter leaves a parameter passed on unresolved; Go follows it through its
  callers, as it does for the values a constructor chain hands to a field.
- A handler that a parameter store joins is a witness at that store
  (`readyLoop.register`), where the C adapter places it at the argument of each
  call that passes it (`X stored in S by F under a condition`).

## Field reads and writes

A function body that names a field of a struct type declared at package
level in one of the target's packages reads or writes that field: one exact
`reads` or `writes` relation per site, whose target is the field object
(contained by its type) and whose `field_path` is the field as the code
reaches it (PROGRAM_INDEX), with the C adapter's rules (C). The path starts
with the package variable the chain starts from (`serverState.shutdown`,
another package's `config.Default.Port` as `Default.Port`) or, from any other
value (a parameter, a local, a call's result, a map or range element), with
the struct type declaring the chain's first named field (`stateEntry.value`
for `e.value`, `Store.dbs` for `s.dbs`), then each named field; elements,
dereferences and implicit steps through an embedded field are left out
(`serverState.db[j].value` is `serverState.db.value`). The site is the
field's name as written, with a `go_field_read` or `go_field_write` witness
("write of serverState.shutdown"). The destination of `=`, of a compound
assignment, of `++`/`--` and of a range clause's `=`, and an element of an
array field there, is written, one fact per site. A struct value a further
field is taken from, and an array field indexed on the way to one, is passed
through and has no fact; everything else reads: the value, the address
(`&serverState.db[j]`), a method called on it (`s.mu.Lock()`), and the
pointer, slice or map it holds to reach an element or a further field
(`c.argv[1]` reads `stateClient.argv`). A field of an outside type
(`fs.Usage`) or of a type declared inside a function has no repository field
object and no fact.

The surface analysis records them where it records a node's calls, so an
explicit `--depth` narrows both alike (`DirectCallIndex.FieldAccesses`,
version 16): the SSA function owns the body, a closure its own, and what each
selector does and the path it reaches the field by are read from the body's
typed syntax, as the C adapter reads clang's. SSA lifts a local into the value
it holds (`e := &serverState.db[n]; e.key = k` would be reached as
`serverState.db.key`) and computes `x.f`'s address twice for `x.f += v`, so
neither the chain as written nor one fact per site survives in it. The
adapter joins each access to the core object field by package, type and
field name. The places graph keeps them as the declaration's `fields` and the
report's readings list each field's writers and readers (READING, REPORT);
the role split does not read them, so no model request changes with them.
A write carries no stored value, where C records it (PROGRAM_INDEX), so a
file read from a Go field has no established path (READING, files a program
keeps; recorded in the 2026-09-29 files pass).
GroupsIndex reach takes a field read as a terminal read (READING, reach).

The cumulative fixture's `internal/storefixture/server_state.go` is kvd's
server state in Go (`assertGoFieldAccesses`): `serverState.shutdown` has one
writer, `onStateSignal`, and one reader, `stateBeforeSleep`;
`serverState.dbfile` is written by `StartStateServer` and `loadStateConfig`
and read by `saveStateSnapshot`; `stateEntry.value` is written through
`setState`'s local `e`, read through `getState`'s and, as
`serverState.db.value`, by `saveStateSnapshot`; `serverState.dirty++` and
`serverState.stats.hits += 1` write (`stats` passed through);
`stateFind` reads `serverState.db` only where it takes an entry's address;
`toolCommand.Run` reads its receiver's `toolCommand.name`. litestream's
cmd/litestream (no model) records 2,843 field accesses (2,277 reads, 566
writes): `Store.dbs` written by `Store.RegisterDB` and `Store.UnregisterDB`
(`s.dbs = append(s.dbs, db)`; `NewStore`'s `&Store{dbs: dbs}` is a literal)
and read at 16 sites by 13 methods; `DB.MonitorInterval` written by
`NewDBFromConfig` and `ReplicateCommand.Run`, read by `DB.Open` and
`DB.monitor`.

## Owned declarations

Direct TypeScript interface property declarations retain their written type,
optional/readonly modifiers, exact source location and native owner. Go core
objects retain short type signatures and explicitly declared struct fields, including embedded
field declarations, under their native type. A struct tag is not signature text: each named format
becomes an object alias (`json:"count_label,omitempty"` is alias `json`/`count_label`; options and `-`
are not names). Both project into the existing
type-owned variable objects used by Python class fields. The same atlas members
and question evidence carry them onward; no field creates a runtime call or an
inherited declaration at a new owner. Comparable count-field examples live in the cumulative TypeScript, Python and Go testdata repositories.

A generic type's short signature keeps its whole type-parameter list, whose
constraints may hold spaces and brackets: `[T any] struct`, `[K comparable, V
map[string]int] struct`, `[T interface{~int | ~string}] interface`, and
`[T storefixture.Labeled[int]] []T` with the constraint's package path
shortened. Its fields and struct tags never enter that text; before this, the
form began at the first space, inside the parameters, and leaked every field
and tag. The header is written from go/types, not parsed back out of a
printed type: the type parameters with their constraints (consecutive ones
with one constraint share it, `[Item, Value any]`), then `struct`,
`interface`, the defined type, or `= T` for an alias, with packages named as
the code names them. A generic function's signature keeps its parameters
(`func[T any](items []T) T`). The cumulative Go repository's
`internal/storefixture/generic_types.go` asserts both. Equivalents: TypeScript
class headers and function signatures keep their parameters; a generic type
alias is written from its source parameters (`Keyed<K extends string, V =
number>`), because the compiler's rendering (`Keyed<K, V>`) drops constraints
and defaults. Regression examples are in `src/type-members.ts`. Python writes
PEP 695 parameters since the same change
([Python](PYTHON.md#generic-declarations)); Clojure declares no type parameters
and has no equivalent.

A call belongs to the scope in which it runs. Go has no equivalent of a call
a definition runs once (a Python decorator's arguments or defaults, a
TypeScript decorator, Clojure metadata): there are no decorators or default
parameters, parameter and result lists and type-parameter constraints hold
only types, and a struct tag is a string literal.

A Go method may be declared in another file than its type. Its native owner
is the type, so on the map of parts it goes with its type's file part, and
a file holding only such methods has no unit of its own
(`internal/localstore/ledger_append.go` in the cumulative Go fixture). A
closure (`f$1`) has no native parent in the index (its container is the
package); the map of parts finds it as a lexical child by source range. Its
native fact is the `anonymous` mark (ProgramIndex 25): the direct-call index
records each function go/ssa gives a parent (`DirectCallNode.Anonymous`) and
the adapter carries it to the object, so places hides a function literal by
that mark, never by the `$` go/ssa writes in its name
(`TestEveryLanguageDecidesByTheCalleeAStatedPrefixAndAnonymity`: `markExitRows$1`
is anonymous, `markExitRows` and `main` are not).

Go evaluates no code, and a repository function named like a setting read
or an evaluation is the program's own: `cmd/app/lookalike_names.go`'s
`eval`, `Getenv` and `statusLedger.exec` give no fact, its `os.Getenv` a
config read (`TestEveryLanguageDecidesByTheCalleeAStatedPrefixAndAnonymity`;
casdoor's `slavedb.exec("show slave status")` and etcd's `txn.eval()` were
dynamic executions by their names). Go writes no keyword argument, so no
keyword names a route prefix; a group's positional address composes as
before. A package-level variable is no reader declaration: the
`var GetVersionFromBinary = func(...)` form reaches the reader only as its
literal's place (READING "Declarations", a recorded gap).

Each type and callable carries `code_lines`: the lines from its name to its
end that hold a `go/scanner` token, so its doc comment, comment-only lines and
blank lines are not counted (a file the loader did not read as it is, such as
cgo's rewritten source, gives zero, unknown). A package may declare `init`
more than once; `internal/localstore/ledger.go` declares two, which the map of
parts reads as one unit, and asserts their code lines (4 and 1).

## Source-aware diagnostics

The etcd report exposed a shared-root ownership defect: the first target at a
root took every file from its library/executable sibling. Places now retains
all indexed owners tied at the deepest root; a nested target still owns its
own subtree. Input order does not decide ownership. Configuration reads no
longer put an entire directory in the outbound integration lane, and entry
seeds are checked against the current target's file membership.

Go TODO extraction scans comment tokens, preserving physical source lines;
`context.TODO()` and string literals are not comment markers. A TODO is a
comment's marker in a code file (owner, 2026-09-29: Redis's ten "TODOs" were
`<a name="TODO">` anchors of its doc/*.html): a file in a language an adapter
analyses, or in one no adapter does (the `unanalysed_file` languages), read by
that language's comment syntax (`//` and `/* */`, `#`, `;`, `--`) with its
strings skipped, and a language whose syntax is not known kept whole; a
document's, a page's or a data file's marker is none
(`TestTODOsAreCommentsOfCodeFiles`). Every readable code file is scanned;
the former whole-file 1 MiB cutoff silently lost all markers, even at the start
of an otherwise ordinary source file. Cumulative Go, Python and TypeScript
examples now retain source-distinct markers on both sides of that former size
boundary, including CRLF and Go physical-line anchors. Dependency facts retain the complete
imported package path instead of only the short package name.
Claims likewise read each eligible README/source file completely through the
existing corpus reader; a whole file larger than 1 MiB no longer silently
loses all its quotes. The existing quote selection, UTF-8 policy and physical
anchors are unchanged. Cumulative README/Go/Python/TypeScript regressions keep
their complete original claim sets, dates, ownership and seals after padding
moves the same text past that former byte boundary.

Listen address facts parse bracketed IPv6 host/port pairs, including scoped
addresses such as `[fe80::1%eth0]:8080`, with the same existing port rules.
Unix socket paths keep their existing handling. Native Go `net.Listen`
examples preserve distinct IPv4/IPv6/Unix call sites and exact source anchors.
The Python tuple/host-port and JS numeric-listen APIs do not use this string
address parser; this fix adds no new API recognition.
