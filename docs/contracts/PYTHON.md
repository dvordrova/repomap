# Python native authority

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Catalogue and shared parsing

The checked Python catalogue validates once when built or decoded and retains
exact native-target, selector and scoped-module lookups. Explicit Validate
still checks a supplied edited catalogue completely; lookups do not rescan it.
Selected targets with the same root/module inventory enter the existing
BuildMany parser core together. It parses each source AST once, then produces
each distinct package/alias view independently. Identical views share one
immutable common input and one stored facts payload. Each target retains its
original scope, launch seeds, complete dependencies and model analysis.
A bad launch projection refuses only that target while its neighbours keep
their already parsed inputs. A failed shared parser preparation retains the
existing exact-target fallback; cancellation stops dispatch immediately.

A module-level function, class or variable is public unless its name starts
with an underscore; a module that declares a literal `__all__` list or tuple
of strings exports exactly the names it lists, so an unlisted module-level
name is internal. Methods and attributes keep the underscore rule. The map of
parts shows the signatures of public names only
(`testdata/repositories/python/src/fixture_app/exports.py`). A class's methods
are always declared in its body: Python has no method outside its class for
the map of parts to move.

## Imports and callable identity

The Python adapter owns package/module scope, import restoration, call and
registration facts, decorators, arguments, target seeds, external origins,
and complete observed/omitted coverage. A name, import, alias, decorator, base
class, read or write that resolves to one known declaration is `exact`; several
known targets are `alternatives`, and a rebound or unknown name is `unresolved`.
Shared stages never repair resolution by matching names.
It maps an exact top-level import root in `sys.stdlib_module_names` to
`platform` and every other external root to `package`; an invalid or missing
authority kind fails the adapter boundary.

An `import ccxt` stays an outside module in every module that writes it.
Until 2026-09-29 only the first module (in path order) to import a package
read it as outside: the symbol that import made was taken for a repository
name by every later one, so `ccxt.Exchange` in `exchange.py` and 220 other
freqtrade outside symbols (numpy 41, asyncio 33, torch 30, …) resolved
only where an earlier module had resolved the same attribute.

Repeated aliases in one import retain one witness for the same declaration
at the same source site; the observed count includes that witness once.
Distinct import statements and calls through each alias retain their own
locations. The 2026-09-09 ordinary Airflow check exposed a false omission:
`BaseFacet`, `BaseFacet as DatasetFacet`, and `BaseFacet as RunFacet` produced
one witness but an observed count of three, refusing the Python dependency
catalog and target. Counting only distinct witnesses fixes the producer while
keeping the complete-coverage check. Cumulative Python, Go, TypeScript and
JavaScript examples exercise native imports, distinct calls and complete
dependency coverage.

The Python adapter also keeps an existing callable candidate consistent
between an argument and the callback transfer that cites that exact argument.
Aliases assigned to a function or lambda and inline lambdas are exact, while
unknown or overwritten aliases, and aliases assigned under a branch, gain no
callback.
The Airflow Edge3, Azure and Vertica libraries exposed the earlier mismatch:
the argument named the assignment variable while the transfer named its callable.
Local native extraction does not establish ordinary full-repository acceptance. Cumulative Python,
Go, TypeScript and JavaScript examples retain their native authority rules.

An inline lambda in a store target's receiver or index (the pandas
`df.loc[reduce(lambda …), "exit"] = 1` idiom, also in annotated-assignment and
`for` targets) is declared and passed like any other; Freqtrade's example
strategy once failed its whole target on it. A lambda in a definition header
(a parameter or return annotation such as FastAPI's `Depends(lambda: …)`, a
type-parameter bound, or a lambda's default) belongs to the defining scope,
which passes it where a call receives it.

A call belongs to the scope in which it runs, in every language. A decorator's
arguments, a default and an annotation run once, where the function or class
is defined, so their calls, reads and lambdas belong to the defining scope:
the module for a class decorator or a top-level function's, the class for a
method's decorator and default. The `decorates` relation stays the decorated
declaration's. `LevelRoutes` in `models.py` checks it. TypeScript decorators
and Clojure metadata and attr-maps follow the same rule
([JSTS](JSTS.md#callable-jsx-and-declaration-headers),
[Clojure](CLOJURE.md#calls-that-run-when-a-namespace-loads)); Go and C write
nothing a definition runs.

Nested Python calls now use their complete native AST span in local relation
and argument-pattern identities. A chain such as `push().map(first).map(second)`
shares its starting position but retains two distinct calls, each with its own
arguments and callback candidates. Original source locations are unchanged;
the source-argument consistency check is unchanged. This fixes the three
Airflow task-sdk targets that previously failed with an argument-authority
mismatch. Current acceptance status belongs in [CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work).
Cumulative Python, Go, TypeScript and JavaScript regressions preserve each
language's existing resolution strength and both callbacks. Go and JS/TS
already distinguished the calls and needed no production change.

The Python adapter retains each declared package's native directory in
ProgramIndex. A namespace package without `__init__.py` keeps no source
location; its exact directory supplies the existing workspace dependency row.
Relative named and wildcard imports into another explicitly named portion of
an advertised namespace retain their original external import boundary. A
nearer ordinary package/module does not authorize an unknown child. Importing
an unknown member directly from a known namespace keeps only that known
boundary; it does not invent a declaration or a callable.

A target indexes only its own project, so another project's package it
imports stays an outside `package` symbol there, named by its dotted path.
The places graph joins such a call or import to the one file another target
declares under that name (a module, or a module's function, type or
variable; `places/package_members.go`): Python resolves the import there once
both are installed. A name the importing target declares itself, or two
files declare, joins nothing, and a package's shorter re-export
(`freqtrade_client.FtRestClient`) names no module member and stays outside.
Each file stays its own target's, so the call is a joint between the two:
freqtrade's `scripts/rest_client.py` calling
`freqtrade_client.ft_client.main` had drawn no arrow
(`TestACallIntoAnotherTargetsPackageJoinsTheirFiles`, the fixture's
`levels.py` into `client/fixture_client/rest.py`). Go and JS/TS index another
module's or workspace package's source themselves; the Clojure and C
fixtures import no other target's code.

## Typed parameters and explicit re-exports

Python HTTP facts follow observed single base-class chains to external methods,
stopping at local overrides and incomplete or multiple bases. TypeScript uses
the compiler-resolved original external class method. These facts do not add
native call edges; Go's promoted embedded methods use existing compiler evidence.
Direct written Python parameter types also retain possible receiver origins,
resolved in the defining scope. This preserves router mounts through typed
parameters and function-local imports. Untyped or reassigned parameters and
unrelated local classes do not acquire framework authority from a method name.
The Python adapter also follows unconditional explicit re-exports through
indexed package/module imports, retaining each written import boundary. A
factory's declared return type may then supply the original instance method
as the callback recipient. Reassigned, conflicting, conditional,
deleted, wildcard or cyclic export bindings remain unresolved, and unrelated
same-named classes gain no receiver authority. A factory with no declared
return type gives a repository class none either; the outside call its one
return statement returns does give its outside symbol (owner, 2026-09-16:
no hedged resolution; [What a call produces](#what-a-call-produces)). No
module is imported or executed to discover exports. TypeScript uses the compiler's
existing barrel-export and declared-return resolution for the comparable case.

A module-level star import (`from m import *`) may bind any name where it
runs, under a branch too, and its names are not followed. The adapter records
where each star import and each export binding of a module is written, as
statement positions (line, then column). A member of a module with star
imports (`pkg.name`, `from pkg import name`, `import pkg as alias` then
`alias.name`) resolves as it would without the stars only when the module
writes it once, unconditionally, in a statement that starts after its last
star import: a `def`, a `class`, an assignment or an explicit import there is
the module's own. A name only a star could bind, and a name written before a
later star (which may rebind it), stay unresolved, as does a child module
of such a package that the package does not bind itself. pykrx's
`krx/__init__.py` star-imports four subpackages and then defines
`datetime2string` and `get_nearest_business_day_in_a_week`: their 77 calls
through `krx` are exact, while the 75 calls to names only its stars bind stay
unresolved. The cumulative fixture's `import_facades/star_facade` and
`star_consumer.py` pin both sides, beside `star.py`'s `StarOnly`, which stays
unresolved. The native equivalents resolve the same shape already: TypeScript
`export *` plus the module's own export, through `import * as`
(`src/facade-exports/star-*.ts`), and Clojure `:refer :all` plus the
namespace's own `defn`, through an alias (`example.facade`), each with its
expectation; a C header that includes another and declares its own function
is the fixture's `kvd.h` and `kvAssertFail`, called exactly from `kvd.c`
(C contract tests). Go has no wildcard re-export, so a package member is
always a declaration of that package: not applicable.

A source-ordered, directly annotated parameter may supply an existing locally resolved class method as the native target, including an explicitly imported facade class. Annotations and literal values retain distinct provenance. Reassignment or conditional assignment clears that binding; unknown, union and unresolved quoted types remain unresolved. This adds no executed import, body analysis, framework inference or exact runtime dispatch.

Synchronous iteration over a directly annotated homogeneous collection retains
its locally resolved element class as a possible receiver inside the loop body.
This applies to parameters, local annotations and declared receiver fields,
including unshadowed `sorted(collection)` with its ordinary key/reverse options.
Known built-in and typing/collections container annotations supply this evidence;
an arbitrary wrapper, shadowed `sorted`, heterogeneous tuple, union, unknown
collection or replaced binding does not. A loop may run zero times, so its
receiver origin does not escape into the else clause or subsequent statements.
An asynchronous loop does not borrow a synchronous container annotation.
Method calls and direct field writes keep their original source locations and
possible status; this is not runtime dispatch or proof that an input executes
the write.

The cumulative iteration fixture checks these positive cases and negative
controls. Real Go range, TypeScript for-of and JavaScript JSDoc-array examples
retain their existing compiler-resolved method identities, with untyped JS/TS
controls remaining unresolved. Clojure has no corresponding generic receiver
type evidence in its current adapter; Java instance dispatch remains unresolved.

## Constructing a repository class

A call of a repository class (`Worker(args)`) constructs an instance. It is
a `calls` relation to the class with the `construct` invocation, keeping its
pattern: arguments, result and the record its `__init__` fields make, as
before. It also runs the class's `__init__`: a second exact `calls`
relation at the same site, also `construct`, to the `__init__` the class
declares or inherits along a chain of single repository bases
(`QuietWorker(name)` runs `BaseWorker.__init__`), witnessed as
`constructor` and with no pattern of its own. The arguments stay on the
class's call, so the facts stage still reads the instance as the class's
(its own and inherited members), and destination reading binds the
constructor's formals from that call (PROGRAM_INDEX). A class with several
bases, whose order the runtime decides, or a chain that reaches a base
outside the repository or unknown before an `__init__`, names no
`__init__`; a class with none anywhere in the chain (`Plain()`) is the
class call alone. A metaclass or `__new__` is not read, and no class is
executed.

A name bound to that call's result holds the instance, source-ordered like
every local binding: a call on it (`worker.run()`) is the method the class
declares or inherits along the same chain, exact (`BaseWorker.run`). The
inherited lookup serves every receiver whose class the adapter knows,
including a directly annotated parameter, a factory's declared return
type, a class call's direct result (`Worker(name, 1).run()`, in
`outside_results.py`) and `self` in a method: `self.helper()` is the method
the class declares or inherits along its chain of single repository bases,
so freqtrade's `Discord` calling `self._send_msg(payload)` calls
`Webhook._send_msg` (`inherited_clients.py`: `Discord.notify` calls
`Webhook.send`; `MixedPrices`, with two bases, and `Heartbeat`, whose base
is `threading.Thread`, name none;
`TestCumulativePythonSelfCallsAndFieldsFollowTheBaseChain`). A store of
`None` does not count: None has no method, so a call on the name is made on
its other value; freqtrade's `start_trading` writes
`worker = None` before `worker = Worker(args)`, and `worker.run()` is
`Worker.run`. A second store of
anything else leaves the class unknown and the call unresolved, as before.
The cumulative fixture's `src/fixture_app/workers.py` checks each case
(`TestCumulativePythonConstructorCallsRunInitAndTypeTheirName`).

Native equivalents:

- TypeScript: `new` is a `construct` call of the constructor the compiler
  resolves, the class's own or the one it inherits, and a call on the
  result is its declared type's method, inherited ones included
  (`src/workers.ts`, `TestCumulativeJSTSConstructorCallsReachTheirConstructor`).
  A class with no constructor anywhere gives the compiler no declaration:
  `new Plain()` stays an unresolved `construct` call, where Python calls
  the class (a recorded difference).
- Go runs no constructor: `NewWorker` is an ordinary exact call, a struct
  literal is no call, and a method promoted from an embedded struct is exact
  (`internal/storefixture/workers.go`, `assertGoConstructedWorker`).
- Clojure: a record's constructor (`->Worker`) is an ordinary function and a
  protocol call on the record is protocol dispatch, unresolved (CLOJURE); no
  equivalent.
- C has no constructors; its `construct` is a record a table row or a field
  store builds (C), and no code runs for it.

## Inherited members and fields

A class's field holds what its one store gives it, wherever the class, a
class deriving from it or a base method reads it. The store is a plain
assignment `self.<field> = value` in any method, and the value is:

- an outside call's result (`self.parser = argparse.ArgumentParser(...)`,
  [Inputs](#inputs-a-calls-words-declare-and-what-python-does-not-have-yet));
- a repository function's result whose declared return type is an outside
  class written as a name or an attribute: freqtrade's
  `self._api = self._init_ccxt(...)`, with `_init_ccxt(...) -> ccxt.Exchange`,
  makes `self._api.create_order(...)` `ccxt.Exchange.create_order`;
- a parameter of the storing def annotated with an outside class and never
  reassigned there: `ExchangeWS(config, ccxt_object: ccxt.Exchange)` storing
  `self._ccxt_object = ccxt_object`;
- a repository function's result whose one return statement returns an
  outside call ([What a call produces](#what-a-call-produces)).

A union, a container, a quoted type, `typing.Any` (no type) and
`typing.Self` (the repository class itself) name no outside class, and a
coroutine function returns a coroutine, not its declared type. A subclass
sees its base's field: the stores it reads are those of the first class of
its chain of single repository bases that stores the field, itself first,
so `Binance` reading `self._api` reads `Exchange`'s store. A store in a
class deriving from the reading class is a second store, since self may be
that class's instance: a second store of any kind leaves the field unknown.
`inherited_clients.py` checks each case: `Prices.ask` and its subclass's
`FuturesPrices.funding` call `httpx.Client.get` through the factory
`_make_client(...) -> httpx.Client`, `StreamedPrices.poll` through the
annotated parameter; `Quotes.last` stays unresolved because `AsyncQuotes`
stores its own client, and `MaybePrices.ask` because its factory declares
`httpx.Client | None`
(`TestCumulativePythonSelfCallsAndFieldsFollowTheBaseChain`).

A method reading a field that the classes deriving from its class also
store reads one of those stores. The field's source value keeps the store
of the reading class, or the base it inherits the field from, and the
`__init__` store of each class deriving from it as `alternatives`, each a
`field_value` at its own line and none chosen; a class storing the field
only outside `__init__` is one more alternative, `unknown`. freqtrade's
`Webhook._send_msg` posts to `self._url`, which `Webhook` stores at
webhook.py:34 and `Discord` at discord.py:20: destination reading gives
both addresses. `inherited_clients.py`'s `Webhook.send` and `Discord` check
it (`TestCumulativePythonBaseReadTakesEachSubclassStore`).

Native equivalents:

- Go and TypeScript fields carry the compiler's declared type, so a call on
  a field, an inherited one included, resolves by type already (above);
  Go's promoted methods (`workers.go`) and TypeScript's inherited methods
  (`workers.ts`) are the `self.helper()` case. Go has no class a base method
  runs for, so the subclass-store case does not arise; TypeScript's
  `this.url` source value takes only the enclosing class's constructor
  record (JSTS), a recorded missing equivalent of the subclass stores.
- Clojure keeps no fields and dispatches protocol calls unresolved; C has
  no classes: no equivalent.

## What a call produces

A call on a call's result is a member of what that call produces, as a
call on a name bound to it is: `Path(name).open()` is `pathlib.Path.open`, a
repository class's call gives that class's method (`Worker(name, 1).run()`
is `BaseWorker.run`), and a member of that result, or a call on a call's
result, continues the outside symbol:
`scheduler.every().day.at("00:07").do(job)` is
`schedule.Scheduler.every.day.at.do` and
`Application.builder().token(token).build()` is
`telegram.ext.Application.builder.token.build`. What a call produces is the
outside symbol it calls, the outside class a repository function declares
it returns, or, for a repository function with no declared return type
that is neither a coroutine nor a generator function, the outside call its
one return statement returns. The same holds for a field stored from that
call (above) and a local name bound to it once. freqtrade's
`_init_telegram_app` returns `Application.builder().token(...).build()`, so
`self._app.bot.send_message(...)` is
`telegram.ext.Application.builder.token.build.bot.send_message`. A function
with a second return statement, a bare `return` included, gives nothing,
and a returned repository class's call gives no class. `outside_results.py`
checks it: `Bot._app` resolves, `Bot._fallback`, from a function with two
returns, does not (`TestCumulativePythonCallsOnCallResultsAndPartial`).

`functools.partial(f, ...)` given as an argument hands `f` over, as a bare
`f` would: the argument's authority is `f`, a `passes_callback` cites it,
and a call outside the repository receiving it is a registration handing
`f` over. freqtrade's
`CommandHandler(["forcebuy", "forcelong"], partial(self._force_enter, …))`
hands `Telegram._force_enter`; the partial call keeps its own callback of
`f` too. `outside_results.py`'s `Bot.register` checks it.

Native equivalents: the Go and TypeScript compilers type a call's result,
chains included. Go's `router.HandleFunc(...).Methods("GET")` (`cmd/app/main.go`)
and `exec.Command(hook, args...).Run()` (`storefixture/destinations.go`) are
the outside types' methods; TypeScript's
`createConsumer().on(...).on(...)` (`src/server.ts`,
`TestCumulativeJSTSChainedCallsKeepTheirOwnPositions`) is `Consumer.on`
twice, named by the declared return type where Python names the call path
(`kafka.KafkaConsumer.subscribe.subscribe`, events.py), a recorded
difference. C has no member calls and Clojure types no call result.
`Function.prototype.bind` in JS/TS and Clojure's `partial` hand over their
call's result, not the function: a recorded missing equivalent. Go has no
partial application; a method value (`s.handle`) is already a bare
callable.

## Source values of rebound names, class attributes and entered objects

The value a name holds where it is read (a pattern's source value) is its
source-ordered binding (2026-09-30, destinations through objects,
READING; skeptic-reviewed):

- A plain assignment or with item rebinding a name in the statement list of
  its previous binding, or in an arm of an if statement standing there,
  whose value was known, holds the new value for the reads that follow
  (`statement = update(Ledger)...; execute(statement); statement =
  update(Archive)...; execute(statement)`). Each arm of an if statement
  starts from the bindings before it, and after it a name an arm bound
  holds each path's last value, the value before it for a path that left
  it alone: one value, or their `alternatives`, none chosen, as Go's SSA
  joins them (freqtrade's `get_trades_query` returns
  `select(Trade).filter(...)`, `select(Trade)` or the `.options(...)` made
  on either); a class a call on it reaches stays unknown when two bindings
  meet. A rebinding in a loop, try, with or match body of a name bound
  outside it, or of a `global` or `nonlocal` name, or after an unknown
  value, leaves the name unknown as before; a nested def or lambda reading
  an enclosing name bound more than once reads it unknown (it runs later).
  Literal-initializer authority is unchanged. `reassigned_address` now
  reads its replacement parameter, never the literal it replaced.
- `with X as name` binds `name` to an `entered` value whose one part is
  X's value: what entering X gives, which need not be X
  (`with engine.begin() as connection`). A tuple or attribute target, an
  `except ... as` name, a match capture and a nested def or class rebinding
  a name make it unknown; the earlier value no longer leaks
  (`conn = a.connect()` then `with b.begin() as conn:` had read
  `a.connect()`).
- A class attribute stored once by a plain assignment through its class's
  name anywhere in the program (`Trade.session = scoped_session(...)` in
  freqtrade's `init_db`) holds that store's value wherever the class is read
  so: the call's result, or, stored from another class attribute
  (`Order.session = Trade.session`), what that one holds. A second store of
  any kind in the class, the base it inherits the attribute from or a class
  deriving from it (a class-body value, `self.name`, `cls.name`,
  `type(self).name`, `setattr`) leaves it unknown; a store in a configured
  test source (`PairLock.session = MagicMock()`) is not the program's (the
  parser request marks test sources). The value-less annotation
  `session: ClassVar[...]` is no store. Which symbol `Trade.session.execute`
  calls is not changed.

`destinations.py`'s ledger checks each case
(`TestCumulativePythonStatementsSentThroughObjectsEndAtTheirEngine`):
`Ledger.session` and `Archive.session` read scoped_session's result,
`Journal.session` (a class-body store too) a field, both `connection`s an
`entered` value, the second `statement` its own update, the lambda's
`statement` nothing, `ledger_query` the alternatives of its arms; and every
statement's destination ends where create_engine's walk does, the
subquery a column's `not_in` takes and the CTE whose column another select
reads included.

A list field stored once as an empty list (annotated or not) whose every
use in the program is the class's own `self.<field>`, the one store, an
append of a construction of a repository class or of a local bound once to
one, `pop`, `remove`, `clear`, iteration, a truth test, `len` and an
element read, holds objects of those classes: a closed set, which wins over
its annotation. Another receiver's `.<field>`, an alias, `extend`, `insert`,
`+=`, a slice or element store, handing or returning the list and a base or
deriving class using it leave it open; a class unrelated to it using its own
field of that name does not. A method call on an element of such a list is a
`calls` relation to each class's method of that name (its own or
inherited), `alternatives` with dispatch `interface` when there are several,
`exact` when one; a class with no such method leaves it unresolved.
freqtrade's `RPCManager.registered_modules` (Telegram, Discord, Webhook,
ApiServer) makes `mod.send_msg(msg)` their four `send_msg`s.
`inherited_clients.py`'s `Notifier` and `OpenNotifier` check it
(`TestCumulativePythonSelfCallsAndFieldsFollowTheBaseChain`).

Native equivalents: Go reads each use's SSA value (a φ is the
`alternatives` of its values, GO), so rebinding in place and the join of an
if statement's arms are already exact;
Go package variables written in a function carry no stored value (GO), a
missing equivalent of class attributes stored through a name; Go interface
calls are already `alternatives` of the implementations (`interface`). JS/TS
reads a reassigned `let` as unknown (JSTS, missing equivalent), and a
static class attribute stored through its class and a `using` declaration's
value are not recorded (missing equivalents); its typed method calls resolve
by the compiler. Clojure's adapter records no call results as origins and
follows no local's value (CLOJURE), so none of these applies; its protocol
calls stay unresolved (missing equivalent of registered lists). C has no
context managers or classes; a file-scope variable's field write carries
its value (C, PROGRAM_INDEX).

## Variable reads and attribute writes

Underscore is an ordinary Python parameter/local name. Its declaration,
annotation and initializer retain the same source identity as other names;
typed `_` parameters and annotated call results must not abort extraction.
Unannotated receivers remain unresolved. The cumulative JS/TS examples retain
their native underscore parameter authority, and Clojure retains an unresolved
local underscore callback. Go's blank identifier is intentionally different:
the cumulative range example never creates a named `_` variable.

Native variable reads retain the original declared slot and each source site,
including imported aliases, module-qualified values, receiver fields and reads
of receivers/indices on assignment targets. A read names its declaration, not a
runtime value. A callable reading its own parameter or local is not a relation;
that value's origin stays on the patterns that use it. Lexical parameters/locals, comprehensions, nonlocal/global
declarations and class-body versus method scope cannot borrow a same-named
outer value. Unbound with/except/match targets stay unresolved. Replaced or
untyped receivers do not acquire field authority. The cumulative examples test
these controls and preserve read locations through GroupsIndex.

The places graph keeps these reads, with every exact decoration, as the
declaration's `uses` (READING): `TestCumulativePythonMapOfParts` checks that
`read_level_data` uses `levels.py`'s `READ_VALUES` and `READ_LIMIT`, and that
`traced_level` uses `traced`, a decorator written as a bare name, which
leaves no pattern and so no lifted call. The role split's helper question
(READING) shows those constants with the functions that read them, and
counts every exact decoration as a use. In the split check, `exports.py`'s
`format_score`, which `__all__` leaves out and only `render_level` calls, is
a helper, so the file keeps one declaration that is none and stays whole.

Clojure already emits comparable native var reads; its cumulative example now
checks an imported var and a shadowing local. JS/TS emits its compiler-bound
declared value references (JSTS), and C a function's reads of file-scope
variables and tables, one per site (C). Go does not currently emit general
variable reads (GO).

Source-ordered receiver origins may bind a direct attribute write to an existing
native class field. Method receivers, their local aliases, directly annotated
parameters and local constructor results retain the field's exact lexical owner.
Assignments, augmented assignments, annotated writes and deletes keep every
source site. Nested classes keep their own receiver identity; a captured outer
receiver keeps its original owner. Rebinding, an untyped receiver, a static
parameter merely named `self`, nested receiver expressions and dynamic `setattr`
remain unresolved. A resolved field write is exact. No class is executed to infer the result.

The cumulative MutableCounter fixture checks each write and its GroupsIndex
source projection, with read-only and replaced-receiver controls. A resolved
read or write targets the class's field object, so one field's readers and
writers gather by their target, as C's do. Python sets no `field_path`
(PROGRAM_INDEX): the written expression (`self.count`, `counter.count`) is
the witness's detail, a receiver is always a typed value and never a
module-level variable, so the path would always be `Class.field`, and a
chain (`self.a.b = x`) stays unresolved, where C follows every field of a
chain and names its root, as Go does (GO). The current
JS/TS and Clojure adapters do not emit comparable target-bound field-write
relations; their mutation-tracing equivalent remains unavailable rather than
being inferred from call or field-initializer evidence. A Python write
carries no stored value (C's does, PROGRAM_INDEX): a field's stores reach
the destination walk only as a field value's initializer, and a field
stored twice (the fixture's `MutableAdapter.url`, a default and a
replacement) is unknown, so a file read from such a field is a path not
established (READING, files a program keeps; 2026-09-29 files pass).

## Handler tables and stored callbacks

These are the Python equivalents of the C adapter's command table, its
callbacks stored under a branch and its calls through function-pointer fields.
The cumulative `src/fixture_app/stored_callbacks.py` checks what the adapter
supports:

- A handler stored into one of two attributes under a branch keeps each store
  as an exact write of its attribute at its own line. The calls through
  `self.on_read` and `self.on_write` stay unresolved and gain no handler. Each
  handler keeps its exact callback at its `register` call.
- A local name bound once to a function (`handler = accept_client`) makes
  `handler()` an exact call of that function.
- A name reassigned under a branch leaves the call through it unresolved,
  never its last assignment and never alternatives, as the C adapter does.
  After `handler = flush_replies` and `if readable: handler = accept_client`
  (`run_chosen_handler`), `handler()` names each function stored in the name
  as a `function_value_store` witness at the stored value:
  `flush_replies stored in handler` and
  `accept_client stored in handler under a condition`, each naming its
  function by identity too, so the map draws the call's possible arrows. In `models.py`,
  `register_callback_aliases` passes its parameter `handler` after
  `if replace_handler: handler = handle_delivery`; the argument keeps the
  variable and gains no callback of `handle_delivery`.

A branch is the body of an `if`, a loop, a `try` (not its `finally`), a `with`
or a `match` case, an arm of a conditional expression, an operand of a boolean
operator after the first, or a comprehension, in the name's own scope. The
condition, the subject and the first operand always run, as the C adapter
walks an if's condition and the left of `&&`: after
`if (handler := accept_client) and ready:`, `handler()` stays exact. A function
declared under a branch keeps the exact calls of its own body, and an enclosed
function reading the name sees the same unresolved value. A `def`, `class` or
`import` of the name in that scope is one more witness, and once the name is
also assigned there, one under a branch makes it conditional too
(`handler = flush_replies`, `if readable: def handler(): ...`). A call of an
attribute of such a name is unresolved and names each module or class stored
in it: after `codec = json` and `if flag: codec = pickle`, `codec.dumps()`
names `json stored in codec` and `pickle stored in codec under a condition`,
never `pickle.dumps`.

Missing equivalents, recorded rather than fabricated:

- The fixture has no input of its own, so GroupsIndex's reach is checked by
  probing declarations as inputs (`flowtest.Probe`: `read_level_data` reads
  `READ_VALUES` and `READ_LIMIT`, `traced_level` does not reach `traced`); a
  call through an attribute is never resolved as alternatives (below), so
  there is no dispatch site.
- No Python function is proven `unreachable` (the C adapter's per-program
  fact, PROGRAM_INDEX): `getattr`, `importlib`, entry points, decorators that
  register, special methods the interpreter calls and `eval` reach functions
  no call names. A boundary in a module several targets import stays with
  every target, and a part leaves no target's map as code that target never
  runs (READING). Two targets importing one module therefore list nothing
  either never runs, and no declaration is shown "run by" the other
  (REPORT).
- A dict or list of handlers (`{"get": get_command}`,
  `[("del", del_command, 2)]`), at module level or in a function, keeps no
  binding, key or other relation to its handlers. A call through a looked-up
  entry (`COMMANDS[name](args)`) is unresolved.
- With no table-row registration, the C rule that a row storing two
  callables is one input has nothing to apply to.
- A call through an attribute is never resolved from its stores, even from a
  single store in `__init__`. The C adapter makes one store exact and several
  stores alternatives. The adapter sets no dispatch word, so no call says that
  it runs a function value.
- Assignments without a branch still resolve to the last one in the scope,
  even at a call written before it: after `handler = accept_client`,
  `handler()`, `handler = flush_replies`, the call is an exact call of
  `flush_replies`. The C adapter gives unconditional stores alternatives.
- A function, class or import bound under a branch with no assignment of that
  name (`try: from fast import loads`, `except ImportError: def loads(...)`)
  still resolves to its last binding.

## Generic declarations

Class and function signatures keep PEP 695 type parameters as written after
the name: `class Crate[T]`, `class Keyed[K: str, V: (int, str)](Box[V])`,
`first[T](items: list[T]) -> T`. A `Generic[T]` base already stays in the
class header (`class Box(Generic[T])`). The cumulative Python repository's
`src/fixture_app/generic_types.py` asserts all four, so that fixture needs
Python 3.12 or later; the parser itself still reads older interpreters' trees,
which have no type-parameter field. A PEP 695 `type Pair[T] = ...` statement is
not indexed as a declaration yet; that gap remains open. See
[Go](GO.md#owned-declarations) for the equivalents.

Functions and classes carry `code_lines`: the lines of `lineno..end_lineno`
holding a `tokenize` token that is not a comment, outside every docstring
statement's span (decorator lines are outside the range). A module counts its
whole file; a module or class variable counts its assignment statement. The
same file's `pick` has two `@overload` stubs and an implementation, three
declarations of one name the map of parts reads as one unit; the
implementation's docstring, comment and blank line leave it 3 code lines.

## Framework-neutral registrations

The original AST call site, result identity, positional/keyword arguments and callback targets remain separate. `Thread(target=...)`, async-task and supported schedule registrations preserve their written activation evidence. A later `start`, `join` or liveness check on that same result does not invent a callback call. Lifespan setup and finite retry loops remain negative controls; final scheduled/continuous roles belong to [operation review](READING.md#operation-ownership). A coroutine call handed to another call carries the `async_task` invocation; with one exact repository callee it is a registration handing that coroutine over, its word the call it is handed to (`create_task`), asked on its own what it starts (PROGRAM_INDEX, READING starting statements): `runtime_registrations.py`'s `start_background_tasks` starts the polling loop `poll_prices` and the one-shot `announce_start`, and `submit_candles` hands `refresh_candles` a stream (`TestCumulativePythonStartsAreAskedPerStatement`). `tool_cli.py`'s `run_init`, init's handler, compares a field of the namespace it was handed with `init-*` through `fnmatch.fnmatch`: init's sub-argument, never asked (READING, K3).

## Inputs a call's words declare, and what Python does not have yet

A call of an outside symbol given words is asked on its own what they
become (READING, the `atlas_api` per-call question), beside its symbol's
row with `result_receives`, the calls made on what the call returns. The
fixture's `src/fixture_app/tool_cli.py` and `dispatch.py` ask
`argparse.ArgumentParser` (`add_argument ×1`, `add_subparsers ×4`),
`add_argument`, `add_subparsers` (`add_parser ×5`) and `add_parser`
(`add_subparsers ×1`, `set_defaults ×2`);
`set_defaults(func=…)` hands a callable and is asked what it becomes. A subcommand named by one
call and handled through another is one input (READING, J1): `init`,
named by `commands.add_parser("init")`, is handled by `run_init`, which
`init.set_defaults(func=run_init)` hands over on that call's own result.
The inputs an object's calls declare are one catalogue of that object:
`--verbose` is declared on `argparse.ArgumentParser("tool")` and `--force`
on init's own parser, whose catalogue names `init` as what its members are
declared on (`TestCumulativePythonInputsJoinAndCatalogue`). A helper
declaring on the parser it is handed (`add_common(command:
argparse.ArgumentParser)`: a call whose receiver value is its own
`parameter`) is declared on no object of its own; GroupsIndex nests it
under each subcommand whose `add_parser` call made the parser a call hands
it, and a row of a table looked up with a list of keys handed beside such
a parser under that subcommand (`dispatch.py`'s `build_serve`:
`build_args(optionlist=ARGS_SERVE, parser=serve)`; READING, options;
`TestASubcommandsOptionsAreNestedUnderIt`). Not followed yet: rows
through `*X` spreads, the elements of a list literal handed as an
argument, and `parents=[...]` (a parser whose options another takes).

A class's field stored exactly once, by a plain assignment of a call's
result in `__init__` or any method (`self.parser =
argparse.ArgumentParser("service")`), holds that result wherever the class
reads it, whichever method comes first in the file: a call on it
(`self.parser.add_subparsers(...)`) is the outside call's own member
(`argparse.ArgumentParser.add_subparsers`) with the field as its receiver
and the outside symbol as the receiver's origin, and the field's source
value is the storing call's result, as a local name bound to a call's
result is. So `ServiceCommands` in `tool_cli.py` keeps its parser and its
subcommand collection in fields, and `serve`, named by
`self._subparsers.add_parser("serve")`, is one input handled by
`run_serve`, declared on `self.parser.add_subparsers(dest="cmd")`
(`TestCumulativePythonFieldStoredOnceFromACallKeepsItsOrigin`,
`TestCumulativePythonInputsJoinAndCatalogue`). A second store of any kind
leaves the field unknown: another assignment, an augmented, deleted,
unpacked, loop or `with` target, and any class attribute of that name (a
dataclass's `field(default_factory=set)` or a model's `Column(...)` is not
what an instance holds). `RebuiltParser` stores its parser twice, and its
`self.parser.add_argument("--again")` stays unresolved and is no input. A
field stored once from a repository class's constructor keeps the existing
typed-field rule (receiver fields, above). The one store may also give the
field an outside type without an outside call
([Inherited members and fields](#inherited-members-and-fields)). freqtrade's `Arguments` stores
`self.parser = ArgumentParser(...)` once, in `_build_subcommands`: its
`subparsers = self.parser.add_subparsers(...)` is argparse's, and each of
its 34 subcommands (`trade`, `backtesting`, …) is an `add_parser` call
given words whose result receives the `set_defaults(func=…)` that hands its
handler over. Go and TypeScript fields carry the compiler's declared type,
so a call on a field resolves by type already; Clojure keeps no fields
(CLOJURE). Not recorded yet:

- `sys.argv` carries no argument vector origin, and a lone comparison of
  it or of a parsed argument (`args.cmd == "init"`) is no fact: an operator
  is no call, so the per-call contrast of an option comparison with a
  comparison of data (C's `strcasecmp(argv[1], "--raw")` beside
  `strcasecmp(cmd->name, "bgsave")`) has no Python equivalent (two or more
  words compared with one value are a comparison, below);
- a field stored more than once, or from anything but a call or a
  parameter annotated with an outside type (another field, an
  unannotated parameter), carries no origin, and a chain through a
  field of a field of a repository class (`self.a.b.c()`) stays
  unresolved;
- dict registries (`handlers[name] = fn`);
- a callable the repository's own function keeps (S1) is not enabled;
- settings a structure names (GO, the tagged-field question): a dataclass,
  a typed dict or a model class a configuration file is decoded into names
  its keys by its field names, and a field's key alias is a call argument
  (`Field(alias="dbs")`), not an object alias; the adapter records neither
  as a key, so no Python field is asked what its key is;
- spellings of one value (PROGRAM_INDEX `same_value_as`): `q.get("a") or
  q.get("b")`, `os.environ.get("A") or os.environ.get("B")` and an
  `if`/`elif` chain whose arms read one call's words and are written the
  same are the equivalent of Go's `||` and if/else-if chain, and the
  adapter records neither, so two spellings stay two inputs.

## Words a value is compared with, and tables of names

An `if`/`elif` chain comparing one expression with a string (`==`, or `in`
a written tuple, list or set of strings) and a `match` whose cases match
string values are one comparison per expression and scope when two or more
different words are compared in two or more cases (PROGRAM_INDEX
`comparisons`): comparisons in one condition, through `and`/`or`, are one
case, whose branch is the `if`'s body; a match case's `a | b` is one case,
its branch the case's body. The value's origin is its source value where it
is compared, and a parallel assignment binds each name to its own value,
all read before any is bound (`command, rest = argv[0], argv[1:]`: element
"0" of parameter `argv`). A module body's comparisons are the module's; a
class body's are none. The fixture's `src/fixture_app/dispatch.py` holds
`dispatch` (three cases), `describe` (`match`) and `is_default`'s lone
comparison, which is none; init's case calls `run_init`, so dispatch handles
init there (READING).

A module-level list, tuple, set or dict written once whose elements share
one shape (every element a string, a call to one callee, or a tuple of
constants of one length; a dict's keys strings and its values of one such
shape or constants) is a table of names (PROGRAM_INDEX `rows`): each
element a row of the string literals it writes in order, a dict's key
first and a call's keyword words named by their keyword. Only a table a
function, method or lambda outside the tests reads keeps its rows
(`keepReadTables`, over the `reads` relations): `dispatch.py`'s `OPTIONS`
(two `Opt(...)` rows) and `REQUIRED` are tables and `FORMATS`, which
nothing reads, is none (`TestPythonTablesOfNamesAreTheOnesAFunctionReads`).
A nested dict (freqtrade's `CONF_SCHEMA`) and a mixed collection are none;
a table a module body reads alone is none. freqtrade records 64 tables
read by a function (60 with a place outside tests: a table whose name
starts with `_` is no declaration), `AVAILABLE_CLI_OPTIONS` with its 124
`Arg(...)` rows among them, and 46 comparisons outside tests.

How a function reads a table is recorded on the read (PROGRAM_INDEX's
shared `membership` and `keys` witnesses), so the reading can tell a table
naming another's rows from one declaring its own (READING):

- `membership`: a module-level or field variable on the right of `in` or
  `not in` (`parsed_arg.command in NO_CONF_REQURIED`). A scope testing the
  same expression in another case, against another table or written words
  (`if command in READ_ONLY: … elif command in WRITES:`), compares it case
  by case as an if/elif chain does, so neither read is a membership test.
- `keys`: a subscript of a module-level variable X, by name or as a
  module's attribute, whose index is the element of a loop over the table:
  a `for` whose target is that name until it is bound again, or a
  comprehension with one generator and no condition. The loop iterates the
  table where it is read, or a parameter of the function, and then each
  call of that function (resolved, not through a class) handing the table
  by keyword, or by position before any starred argument, meets the
  parameter by name or by position as a call site counts, the receiver
  excluded (`self._build_args(optionlist=ARGS_TRADE, parser=trade_cmd)`).
  A method called through its class is handed its receiver first, so only
  its keyword arguments meet parameters. The subscript must run on every
  pass: in a plain statement of the loop's own body before any `continue`,
  in a body with no `break`, `return` or `raise`, outside a conditional
  expression's branches, the later operands of `and`/`or`, a lambda or a
  comprehension, and the loop outside the body of a `try` with handlers.
  A table looked up with its own rows (`HELP[name] for name in HELP`)
  holds no keys of another.

The fixture's `dispatch.py` holds each: `ARGS_SERVE` handed to
`build_args` by keyword and `ARGS_INIT` by position and looked up by
`init_flags`' comprehension (keys of `OPTIONS`), `NO_CONFIG` and `KNOWN`
tested alone (membership, `KNOWN` before a `getattr` dispatch), and the
counter-cases `HELP` (its own keys, and a membership test beside a plain
lookup), `COMMANDS` (looked up only under a condition, handed through a
class), `READ_ONLY` and `WRITES` (tested case by case)
(`TestPythonTablesNamingAnotherTablesRowsAreNoInputs`). freqtrade records
keys of `AVAILABLE_CLI_OPTIONS` on the reads of its 28 `ARGS_*` tables
(through `_build_args`) and membership on 7 tables, 3 of them tested
alone (`NO_CONF_REQURIED`, `NO_CONF_ALLOWED`, `SUPPORTED_EXCHANGES`). Not
recorded: `X.get(v)` and `v in X` before `X[v]` (a lookup under its own
test is no keys read), and keys a program writes into X at run time (X's
rows are its initializer's).

## Programs a call starts

`revision` in `tool_cli.py` runs `subprocess.run(["git", "rev-parse",
"HEAD"], check=True, capture_output=True)`: asked `talks` with the call as
written, the reading's `runs_program` (READING). Not recorded yet: the
strings inside a list or tuple literal are no call words (a call's words
are its literal arguments), so this call gives the program question no
word, is not asked, and its program stays not established. `os.system`
and `subprocess.Popen` given a string are asked like any word-given call.
Missing equivalent (2026-09-28): a name assigned on both branches of an
if/else (`proc = subprocess.Popen(…)` in each) is a reassigned binding,
so its read is `unknown` with the name as text, not the `alternatives` of
both `call_result`s that Go records; `proc.communicate()` on it stays its
own launch boundary instead of folding into both launches (READING).

## Test sources

A resolved pytest table in `pyproject.toml` (`[tool.pytest]` or
`[tool.pytest.ini_options]`) owns the Python files below it. Its
`python_files` patterns select test modules. Without that setting, pytest's
default patterns apply only when pytest is a declared dependency. Every
`conftest.py` under a resolved table is test code too: it is pytest's own
plugin file, the equivalent of a JS runner's config. The nearest table owns a
file, and an unresolved one classifies nothing.

A test directory is test code whole (2026-09-30): the topmost directory
below a resolved table's own directory that holds a selected test module
while no file under it is a module the project's build declares
(`pythontarget.Target.DeclaresModule`) or any program's launch file, in a
project whose build declares its packages. A distribution ships its
declared packages; a directory beside them holding its tests holds their
helpers and the modules they load by path too. freqtrade's root
distribution folds into `freqtrade` (DISCOVERY), so `tests/` is its; its
`conftest_trades.py` and the strategies under `tests/strategy/strats`,
selected by no pattern, had drawn "Strategy test fixtures" and "Test
fixtures" parts on the product map (READING's test-only part rule reads
this fact). A test module inside a declared package (`pandas/tests`) is
selected alone, and a directory holding a program's launch file is none
(its test-module subdirectory may be). The cumulative fixture's `tests/`
beside `src/` is a test directory: `tests/__init__.py` and
`tests/sample_orders.py`, which no pattern names, are test code;
`src/fixture_app/test_market.py` inside the declared package is selected
alone (`TestATestDirectoryBesideTheDeclaredPackagesIsTestCode`).

Equivalents that are not derived:

- `testpaths` is a collection root, not a test-only directory. Large
  projects point it at their production package (pandas sets
  `testpaths = "pandas"`), so a file under it is not test code by that
  fact. Only the `python_files` matches, `conftest.py` below it and the
  test directories above are.
- `unittest` discovery has no manifest declaration. Its `discover -s/-p`
  arguments live in Makefiles, tox or CI, which the adapter does not read.
- `pytest.ini`, `tox.ini` and `setup.cfg` pytest sections are not read.

## No execution for inference

No Python module, dynamic setup expression, factory or imported package is executed to infer target ownership or method authority. Unknown, overwritten, conflicting or conditional bindings remain unresolved. All extraction changes require the real cumulative Python fixture and applicable equivalents in the other languages; see [development](DEVELOPMENT.md).
