# C adapter

The ordinary C adapter reads the C programs the repository's own build
describes, its root's and its directories' own makefiles, through `clang`
run as a subprocess. It runs whenever the corpus
has a `.c` file outside the tooling directories;
`internal/run/repository_target_c.go` registers it with the selector prefix
`c:` and rank 4 after Go, Python, JS/TS and Clojure. Nothing in it knows a
particular project, framework or protocol.

## Build description and targets

A `.c` file under a tooling directory (`.claude`, `.github`, `.vscode`,
`testdata`: `corpus.ToolingPath`) is an input of the tests or tools around it
and never a unit: a corpus whose only `.c` files are there runs no C
discovery, no dry run and no clang probe, and beside other sources no clang
default unit is made of one. An explicit `--target` whose every selector
another adapter owns runs no C discovery either.

`internal/cproject` takes compile and link lines from the first of:

1. a root `compile_commands.json`;
2. a dry run of the root `GNUmakefile`, `makefile` or `Makefile`:
   `make -n -B -w -o <makefile>` on the default goal, with `LC_ALL=C`, the
   caller's `MAKEFLAGS` and related variables removed, and a 30 s limit that
   stops make's whole process group;

then from the makefiles of the directories below the root that no dry run
entered (Nested makefiles, below), and gives clang's defaults to every `.c`
file no line compiles and no other file includes. Such a unit that clang
cannot parse says why first: "no build line compiles src/litestream-vfs.c;
parsed with clang's defaults (exit status 1): src/litestream-vfs.c:1:10:
fatal error: 'litestream-vfs.h' file not found" (litestream's header is the
one `make vfs` has `go build -buildmode=c-archive` write, a goal its default
does not run), never as if the unit itself were broken
(`TestDiscoverWithoutBuildDescription`).

A failed dry run is recorded (`c_build_error`, naming its makefile) and the
units it would have compiled keep clang's defaults; a unit that then fails to
parse reports both errors. `make -n` is not a pure dry run: GNU make still
runs `$(shell ...)`, recipes that start with `+` or call `$(MAKE)`, and rules
that remake the makefiles it reads, and `-B` makes all of them out of date.
`-o <makefile>` keeps the makefile a run reads as it is; makefiles it
includes are still remade and may be written. The selected repository is
trusted, so this is recorded rather than prevented. macOS ships GNU make 3.81.

### Nested makefiles

A developer runs `make` in a directory whose own makefile the root's never
reaches (Lua's `testes/libs/makefile` builds the test libraries with
`-I../../`; Lua 5.1.5's `etc/Makefile` builds `min.c` with `-I../src`), and
so does discovery (`nested.go`). A directory counts when the corpus holds a
makefile there (`GNUmakefile`, `makefile`, `Makefile`, the first of make's
names) and `.c` units whose nearest makefile is that one that no line
compiled yet, outside the tooling directories; a directory a dry run entered
(`make[N]: Entering directory`, the root's recursive `$(MAKE)`) is that
run's and is not run again. Directories run shallow first, each in its own
directory with the root's command on its own makefile:

1. Its default goal's compile and link lines join the build's, as the
   root's do: Lua's `testes/libs` gives five shared libraries,
   `c:testes/libs/lib1.so` …, anchored on its makefile's rules.
2. When that goal compiles none of its units and the makefile has an `all`
   rule, `make -n -B -w -o <makefile> all` is taken as the default goal's
   lines are: compile, archive and link lines. Lua 5.1.5's `src/Makefile`
   asks for a platform by default and builds `liblua.a`, `lua` and `luac` on
   `all`: `c:src/liblua.a`, `c:src/lua`, `c:src/luac`. No other named goal is
   run: a platform's goal (`make macosx`) is a choice the build leaves to its
   reader, so Lua 5.1.5's platform flags (`-DLUA_USE_MACOSX`, readline) are
   not read; that is a known limit.
3. When neither compiles any of its units, they are compiled as that
   makefile compiles their objects: one `make -n -B -k -w -o <makefile>
   x.o …` naming each unit's object relative to the directory, its own rule
   or make's built-in rule fed its `CFLAGS` and `CPPFLAGS`. Only their
   compile lines are taken, never a link line: a program is then found by
   its main (`c:etc/min.c`). Lua 5.1.5's `etc/Makefile` only prints "Please
   choose a target" by default and has no `all`. A goal compiling some of its
   units leaves the others out, as the platform's build does (D3).

Each unit records the makefile that compiled it and whether its flags come
from that makefile's object rule (`UnitSpec.Makefile`, `ObjectRule`); the
root's units name none. A run that fails is a `c_build_error` on its
makefile, and the next step still runs after one fails; with `-k`, an object
no rule makes fails the run while the others print, so a unit only the
output leaves without a line is a `c_unit_unbuilt` on its makefile, not a
build error. A program whose units kept clang's defaults because their
makefile's runs failed carries that failure (`BuildErr`). The root's own
units are never read by steps 2 and 3: what its default goal leaves out is
the platform's choice. Measured 2026-10-02 on moby, deploy, go, ghidra and kubernetes: 0, 0,
2, 2 and 2 nested runs, none failing, discovery 2–9 s.

Each link line is one program (`c:<output>`, anchored on its makefile rule)
whose files are the units it links; a `-shared`/`-dynamiclib` line is a shared
library. Each archive the archiver writes (`ar` with an `r` or `q`
operation) is a library of its own, `c:<archive>` (`ProgramLibrary`), whose
files are its members, expanded as a link's inputs are, eight deep, a member
no compile line produced going to its `Missing` (external review and owner,
2026-10-02: the library is often the product, and Lua's C API reads as its
entries, never as code `lua`'s main does not reach). Lua's `makefile` writes
`liblua.a` and links it into `lua`: `c:liblua.a` (its 33 files) and `c:lua`
(`lua.c` and the same 33). The archive is not folded into the programs that
link it; each keeps its whole analysis, and a link line's `c_link` names the
archives it links (`fields.archives`). An archive with no member a compile
line produced is a `c_archive_without_units` observation and no target. A
partial link (`-o x.o`) is expanded like an archive and is no target. A link
expands an archive whole, not member by member in link order as a linker
pulls them: Lua 5.1.5's `luac` links all of `liblua.a`'s members, a known
limit. A rule names its output as written, else through a variable the same
makefile assigns it to (`LUA_T= lua` and `$(LUA_T): …` anchor `c:lua` on
`makefile:124`), one level deep, else line 0. Compile lines without `-o` map
`x.o` to `x.c` through make's working directory. Main is located when the program is parsed, not during discovery.
A unit no link line links (every unit without a build description, and with
a `compile_commands.json`, which has no link lines) that has an exact,
non-static `main` with a body (a filtered clang parse, since
`-ast-dump-filter` matches substrings) is a program `c:<path>` whose files the
linker closure decides: each unresolved external name goes to the one unit
that defines it. Of several units defining it, the build decides: the one a
build output (a link line's output or an archive, expanded through archives
as a link's inputs are, never a `c:<dir>/` grouping: `UnitSpec.Outputs`)
takes together with the unit needing it, when exactly one is; a unit the
build takes is never preferred over one it does not, and where a unit is
written never decides (external review, 2026-10-02: a directory's own
`fixedclock.c` may be the alternative its makefile does not link). Lua
5.1.5's `src/lstate.c` calls `luaX_init`, which `src/llex.c` and
`etc/noparser.c` define: `liblua.a` archives `lstate.c` with `llex.c`, so
`c:etc/min.c` takes `llex.c`. Where the build says nothing, and no unit the
program needs for another name defines it, every definer stays as an
alternative (owner, 2026-10-02: alternatives, as an interface's
implementations are, never a reason to fail): the program holds them all,
records "{name} is defined in a, b; the build does not say which is linked"
(`Parsed.Alternatives`, printed beside the program when it is parsed), marks
the units it holds only as such a definition, or for what one needs, as
alternatives, never linked members (`Parsed.AlternativeUnits`), and projects
each definition as an object at its place, a call of the name having them all
as its targets (`ResolutionAlternatives`); a function value naming it, and a
read of a variable several alternatives define, name none of them. Two units
the program links
(not alternatives) defining one name differently still fail it. Units no
program links and no archive takes and that define no main are their
directory's library (`c:<dir>/`). A `.c` file another file `#include`s belongs
to its includer; one no parsed unit enters on this platform (an `#ifdef` chose
another backend) is outside this platform's build, and the run prints it
beside its program when the program is parsed. Shared units stay complete in
every program that links them.

Discovery offers the target portfolio the files only one program compiles
(a program whose every file is shared offers its link line's file); every
unit of a program and its link-line file restore it. Native evidence rows are
`c_link` (the makefile line, `fields.output`, `fields.archives`, `values` =
linked files), `c_archive` (the archive's rule, `fields.output`,
`fields.consumers` = the link outputs that link it, `values` = its files),
`c_main` and `c_library`; the portfolio prompt defines them.

### A library's exports

A library has no main: a program using it calls in through its API, so its
entries are its exports (ProgramIndex `target.exports`, `exports.go`; owner
2026-10-02, after Lua's `liblua.a` read with no entry). Of the functions with
external linkage its units define, an export is one that a repository
header declares which a program the build links with the library includes
directly from its own units (`Program.Consumers`, each such link line's
objects; `Parsed.APIHeaders`, from the same parse their programs read): basis
`consumer_headers`. Lua's `lua.c` includes `lua.h`, `lauxlib.h` and
`lualib.h`, so `liblua.a` exports its 156 `lua_*`, `luaL_*` and `luaopen_*`
functions, and the 205 `luaD_*`, `luaK_*` and other functions with external
linkage that its internal headers declare (`LUAI_FUNC`, "not to be exported
to outside modules") stay its own. The fixture's `wirecat` includes
`wire.h`: `libwire.a` exports `wireEncode` and `wireEscape`, never
`wireNeedsEscape`, which its own files share through `wire_internal.h`.
When no program the build links takes the library (a shared object a program
loads, `upper.so` and Lua's `testes/libs`; a directory's units no link line
links; an archive nothing links), every function with external linkage is an
export, basis `linkage`: any program may call it by name, and a loaded
module's functions need no header. Lua 5.1.5's `luac.c` includes internal
headers directly: what they declare is its API too, as `luac` enters it. An
executable exports nothing.

## Native tooling and platform view

Compile flags pass an allowlist of what changes the parse: defines and
undefines, include paths (`-I`, `-isystem`, `-iquote`, `-idirafter`,
`-include`, `-imacros`), the language standard, sysroot and target, one
`-arch`, `-pthread` and language `-f`/`-m` flags. Warnings, optimisation,
debug, dependency output and flags only gcc accepts are dropped and recorded
per unit. After the build's flags every unit gets `-U_FORTIFY_SOURCE
-D_FORTIFY_SOURCE=0`, so libc calls keep their written names (the macOS SDK
fortifies at every optimisation level), and the diagnostics clang 16 and later
make errors for pre-C99 code stay warnings that `-w` silences.

Each unit runs `clang -fsyntax-only -w -H -Xclang -ast-dump=json` once per run
store, at most four at a time; a unit several programs link is parsed once.
The plan's store keeps a decoded unit only while a planned program that has
not been projected yet still links it: once a program is projected, or its
parse fails, the units no remaining program needs are released, so decoded
units do not stay alive through the pages' model work. The dump is decoded as
it streams, one top-level declaration at a time. clang leaves out `file` and
`line` when they have not changed, so that state is replayed through every
location in document order, including inside dropped system declarations:
spelling precedes expansion, and `includedFrom` and presumed locations never
move it. Only declarations whose expansion location is a corpus file are kept.
`-H` gives the active include tree.

The view is the host platform's build as the build description gives it (owner
decision D3): on macOS a Makefile that picks kqueue under `HAVE_KQUEUE` is
read with kqueue, and the epoll and select backends are outside this
platform's build. `Toolchain` records clang's version, target triple, sysroot,
resource directory, system include directories and the overrides, and the run
prints clang's version and target beside the build description and each
program's sources outside this platform's build beside the program. Each C
target's run records its view in that run's `metadata.json` as `c_platform`,
where a reader of the saved run finds it: clang's version line, target triple
and sysroot, the overrides every unit gets after its own flags (the fortify
override and the pre-C99 diagnostics), the build description's failure when
the units fell back to clang's defaults, each unit's kept and dropped flags
and whether the build or clang's defaults gave them (and which directory's
makefile, by its default goal or its object rule), and the included sources
outside this platform's build (an epoll backend beside a kqueue build on
macOS). The page and the report JSON carry no label for it.

The clang resource directory and the sysroot's headers and frameworks are the
platform; any other include directory (`/usr/local/include`,
`/opt/homebrew/include`) is a package. A platform or package declaration is
named by the first header on its `-H` chain that a corpus file includes, as
that directive spells it: `qsort` declared in `_stdlib.h` is `stdlib.h`.

Without clang the C targets are not analyzed, with the reason that a required
tool is unavailable (`cproject.ErrClangUnavailable`). A unit that exits
non-zero or prints `error:` fails every program that links it; there is no
partial graph, and targets of other adapters are unaffected.

## ProgramIndex shape

The projection follows the linker. A name with external linkage is one object
per program; two external definitions of one name fail the program. A static
function, variable or type is identified by its definition location, so a
header's `static inline` function or a `.c` file several units include is one
object, and same-named statics in different units stay apart. An anonymous
`typedef struct {...} Name` is the type `Name`.

Objects are a module per file, functions at their definitions (prototypes are
declaration witnesses; `static` is internal visibility), a type per struct,
union and enum with fields and enum constants contained by it, and file-scope
variables. Signatures are clang's native text; parameter and result type ids
follow pointers and qualifiers to repository types, and a field's `types`
name where the repository type its declared type names is declared
(`strbuf query` names strbuf.h's strbuf; a scalar or a platform type names
none), as Go's `DBs []*DBConfig` names DBConfig. `main` is the callable
seed.

- A direct call is `calls`, exact after resolving names across units. A call
  of a platform or package function is `invokes_external`; `__builtin_*` calls
  belong to the platform.
- A call written in a macro argument keeps its own spelling location; a call
  the macro body writes is sited at the macro use, with the outermost macro
  name as written as its selector and the body's spelling as a
  `macro_expansion` witness (no place when the body is the platform's:
  `tools/dump.c`'s `assert(keys.len == 0)`, whose builtins are in the
  `builtin` package). A function's flow (REPORT) reads the calls of one
  macro as that macro, once, as the code writes it (`assert`, not
  `__assert_rtn` and `__builtin_expect`), and never shows a compiler
  builtin as a call. Go, Python and JS/TS have no macros; Clojure records
  no call a macro's expansion makes (CLOJURE), so it has no internals to
  hide.
- A call written in an if statement's condition carries the lines that
  condition guards as its pattern's `branch` (PROGRAM_INDEX): kvd's
  `strcasecmp(argv[0], "port")` guards its own line, `"persist"` its block,
  and the report names the fields a setting's branch writes from it.
- A function passed as an argument is `passes_callback` bound to that argument.
  An assignment target's index and receiver are read like any expression, so
  a function passed to a call there (`marks[fold(c, n, both)] = 1`, `+=`,
  `rowFor(both)->exit = 1`) is a callback of the assigning function. C has no
  lambdas, so Python's store-target and header lambdas have no equivalent.
- A function stored into a field, variable or array (initializer rows,
  assignments, and a parameter a function stores joined with what its callers
  pass) is `passes_callback` with a primary `c_function_pointer_store` witness,
  so each store is a binding row. A row of a repository-owned table that names
  the function by a string literal is a registration fact the model classifies
  (owner decision D1). Its registrar is the row's record type and the field
  the function is stored in (`kvd.h.kvCommand.proc`), which the registrar
  table asks once what its callables become: a command a client sends is a
  `request`. The entry is named by the word the model chooses among the row's
  own (`get`), like any entry of any protocol. A row that stores a second
  callable (`kvd.c`'s `{"get", getCommand, 2, preloadKey}`, Redis's
  `vm_preload_proc`) is still one registration, of the callable it writes
  first; `preloadKey` stays a callback the row stores and is no input named
  `get` (PROGRAM_INDEX). The literal is the row's name:
  `{"get", getCommand}` states no HTTP method, since nothing in the row is an
  address for a verb to qualify; a route row `{"GET", "/health", health}`
  states GET beside its path. A store into a platform record (`act.sa_sigaction`) is
  keyed on the root value's type and the field as written, not on the union a
  platform macro expands it to.
- A call through a field, parameter or variable has the stored functions as
  its targets. A store under a branch, or of a value the index cannot name,
  leaves the call unresolved with the candidates as witnesses; it never
  produces alternatives the program cannot reach. Each candidate witness
  names its function by identity as well as by words, so `loop.c`'s
  `fe->rfileProc(...)` reaches the map as possible arrows to `acceptHandler`,
  `readQueryFromClient` and `sendReplyToClient` (READING), and stays
  unresolved. A parameter's stored functions are what the direct callers pass
  there: `once(first)` makes `once`'s `job()` exact, `either(first)` and
  `either(second)` make `either`'s its alternatives (cproject
  `TestIndexCallsWhatCallersPassAParameter`; the fixture has no such call).
- A function cast to an integer is an address used as data, never a callable.
- A function body that names a file-scope variable of the program reads it:
  one exact `reads` relation per place it names it (a `c_variable_read`
  witness; one a macro body writes is sited at the macro use with a
  `macro_expansion` witness), whether it takes the value, a member or an
  element (`symsTable[i].pointer`), or the address. The linker's identity
  names the variable: a static by its definition, an external name, also
  through a block-scope `extern` declaration, by its one definition in the
  program. The variable itself as the destination of `=` (`progname =
  argv[0]`) is written, not read, while `x += 1`, `x++` and `server.port =
  p` read it; an operand `sizeof` or `_Alignof` never evaluates reads
  nothing; a parameter, a local, a static local and a platform variable
  (`stderr`, `environ`) are no program variable. A file-scope initializer
  naming another variable (`&table`) is a link-time address, not a read.
  Redis's `findFuncName` reads `staticsymbols.h`'s `symsTable`; the
  fixture's `printSymbols` reads `staticsyms.h`'s.
- A function body that names a field of a repository record reads or
  writes that field: one exact `reads` or `writes` relation per site, whose
  target is the field object (`redisDb.expires`, contained by its record
  type) and whose `field_path` is the field as the code reaches it
  (PROGRAM_INDEX): the file-scope variable the chain starts from, or, from
  any other value (a parameter, a local, a call's result, a dereference),
  the record holding the chain's first field, then each named field of the
  chain with elements left out. So `server.masterhost = sdsnew(...)` in
  Redis's `loadServerConfig` writes `server.masterhost`, `db->expires` in
  `expireIfNeeded` reads `redisDb.expires`, `c->db->expires` reads
  `redisClient.db` and `redisClient.db.expires`, and `server.db[j].expires`
  reads `server.db.expires`; the readers and writers of one field gather by
  its target whatever root each reaches it from. The site is the field's
  name as written (`c_field_read` / `c_field_write` witnesses, "write of
  server.masterhost"); a field a macro body names is sited at the macro use
  with a `macro_expansion` witness (`dictSize(d)`'s `(d)->used`). The
  destination of `=`, of a compound assignment and of `++`/`--`, and an
  element of an array member there (`server.buf[0] = 'a'`), is written;
  one fact stands for the site, though a compound assignment also reads
  the old value. A plain `=` to the member itself keeps the value it
  stores as the relation's `value` (ProgramIndex 23), recorded as any
  argument's origin: `server.dbfilename = "dump.rdb"` stores the literal,
  `server.dbfilename = zstrdup(argv[1])` the call's result; a compound
  assignment, `++`/`--` and an element of an array member store none. The
  files a program keeps read a field's values from them (READING);
  `TestCFixtureFieldWritesKeepTheValueTheyStore` checks kvd's. Everything else reads: the value, the address
  (`&c->reply`), an element of a pointer member, whose pointer is read
  (`c->argv[0] = o` reads `redisClient.argv`), and the pointer a `->`
  member is taken from. A record a `.` member is taken from, and an array
  member indexed on the way to an element's field (`server.db[j].key`), is
  passed through and has no fact of its own. `sizeof` and `_Alignof`
  operands read no field; a member of a platform or package record
  (`act.sa_handler`) or of an anonymous struct or union has no repository
  field object and no fact. The whole-variable read above stays: `server.port
  = p` reads `server` and writes `server.port`. `TestIndexReadsAndWritesRecordFields`
  checks each case; `TestCFixtureReadsAndWritesRecordFields` the fixture's:
  kvd's `server.shutdown` has one writer, `onSignal`, and one reader,
  `beforeSleep`; `server.dbfile` is written by `main` and `loadConfig` and
  read by `bgsaveCommand`, as Redis's `server.masterhost` is written by
  `initServerConfig`, `loadServerConfig` and `slaveofCommand`; and
  `kvEntry.value` is written through `setCommand`'s local `e` and read
  through `getCommand`'s and, as `server.db.value`, by `saveSnapshot`.
- A call belongs to the function whose body runs it. C has no equivalent of
  a call a definition runs once (a Python decorator's arguments or defaults,
  a TypeScript decorator, Clojure metadata): there are no decorators or
  default arguments, and a file-scope initializer is a constant expression
  that calls nothing.
- Every active `#include` is one `imports` relation and one dependency: a
  repository header is a workspace dependency, a platform header the standard
  library, any other header a package.

String-literal arguments are literal patterns (clang joins adjacent
literals); loop context reuses the existing `control_context` words.

Every object with a located range carries `code_lines`, counted by the
adapter's own lexer: `//` and `/* */` comments are not code, a
backslash-newline continues a `//` comment, string and character literals are
code and end at an unescaped newline, and preprocessor lines are code; a
module counts its whole file. C has no in-file repeated declaration name for
the map of parts to merge: only a function's body is its declaration, a
typedef merges with its record, a tentative definition is one variable and an
`#if` alternative parses one branch. `kvd.c`'s `bgsaveCommand` (17 of its 18
lines; one comment line) asserts the count.

## What a program never runs

A C function runs only when code that runs names it: calls it directly, or
uses its address (stores, passes, returns or compares it, or casts it to an
integer that can be cast back). For a program with a `main`, the adapter
starts from `main`; from functions the runtime calls without a name in the
code (a `constructor` or `destructor` attribute); from every function a
file-scope initializer names, since those tables exist before `main` (Redis's
command table and `staticsymbols.h`'s integer casts); from an external
function whose name a platform or package header also declares, which that
library may call instead of its own; and from a name the C standard reserves
for the implementation (a file-scope name beginning with `_`). It follows
what each definition's body names, in every unit's copy of it (a header's
`static inline` function reaches each unit's own statics), including a
`cleanup` attribute's function. Assembly statements, file-scope assembly,
asm labels and `alias`/`ifunc` attributes name functions by text: every
repository function their written text, or its macro's, names is named there
(a call of `extern void f(void) __asm__("impl")` runs `impl`), and a function whose
own symbol an asm label renames is called by that symbol from outside, so it
is where running may start. A label a platform header writes names a
platform symbol and is not read. Each function
nothing reached is `unreachable` in that program's index: a deterministic
fact about that program as its link line (or, without one, the linker
closure) gives it, not about the file, so `anet.c`'s `anetTcpServer` and
`anetAccept` are unreachable in redis-cli and redis-benchmark and reached in
redis-server, and redis-cli's list names redis-server beside them ("run by",
REPORT). An address taken only inside code that never runs is never
taken. A part of a program's map whose every function is unreachable there
leaves that program's map (READING): redis-cli links `adlist.c` and calls
none of its thirteen functions, so its "Linked list" is listed under "Not
reachable from the entrypoints" and not drawn.

Code the adapter does not read can call any function with external linkage
by its name: a link input no compile line produced (`Missing`: a prebuilt
object or archive, an assembly or C++ source, a `.c` file outside the
corpus, a response file `@file`), a library other than the C runtime's own
(`-lc`, `-lm`, `-lpthread`, `-ldl`, `-lrt`; `-pthread` is a flag), a
framework, other programs calling a shared object (`-shared`) that also
defines `main`, and code the program loads or looks up at run time
(`dlopen`, `dlmopen`, `dlsym`, `dlvsym`, `dlfunc`). Then every external
function is where running may start, and only static functions are proven.
A link line that names another entry (`-e`, `--entry`, `-init`, `-fini`, a
linker script `-T`/`--script` with its `ENTRY`), defines one symbol as
another (`--defsym`, `--wrap`, Apple's `-alias`/`-alias_list`) or reads
linker options from a file (`-Wl,@file`), through `-Wl,` or `-Xlinker` too,
or drops the runtime's start files (`-nostartfiles`, `-nostdlib`), and
assembly text the adapter cannot read, prove nothing; a library (no `main`)
is called from outside and marks nothing.

The fixture's `net.c` is linked into kvd and kvcli like `anet.c`: kvd never
runs `netConnect`, kvcli never runs `netListen` (nor `strbuf.c`'s
`sbConsume`), and the backlog `netListen` reads (`KVD_BACKLOG`) is kvd's
setting alone (PROGRAM_INDEX, READING). kvcli also links `loop.o`, the
server's event loop, as redis-cli links `adlist.o`, and runs none of it: its
`loop.c` and `loop_poll.c` parts leave kvcli's map, while `loop.h`'s types,
which run nothing of their own, keep theirs.

## Facts and claims

### A makefile as the build's manifest

A C program's makefile, the one its link or archive rule stands on, is its
manifest, as a go.mod, a package.json or a deps.edn is (facts
`deriveManifest`; owner 2026-10-02, after Lua's "How to run" said "the
repository ships no manifest" beside the makefile whose dry run made
`liblua.a` and `lua`). The facts quote it through `internal/makefile` as
`manifest` rows a run recipe can cite, each at its line:
- `default_goal`: what `make` builds when given no goal (`.DEFAULT_GOAL`,
  else GNU make's first rule naming no special target or pattern), its
  prerequisites read through the makefile's own variables and its commands
  (Lua: `all: liblua.a lua`, makefile:113; the fixture: `all: kvd kvcli`,
  Makefile:7);
- `rule.<target>`: each other rule a reader may run (`make test`, with
  what it needs and runs), never an object's (`x.o`) or a special target;
- `variable.<NAME>`: the assignments of the variables the goal's commands and
  make's own compiles and links use (CC, CFLAGS, CPPFLAGS, LDFLAGS, LDLIBS),
  followed through the variables their values name, each under its
  conditional (`variable.CFLAGS when ifeq ($(uname_S),SunOS)` and `when not
  ifeq …` in redis-1.3.6; Lua's `MYCFLAGS … -DLUA_USE_LINUX`, `CC= gcc`).
A makefile several programs name as their manifest (Lua's root makefile
for `lua` and `liblua.a`, 5.1.5's src/Makefile for liblua.a, lua and luac,
the fixture's Makefile for kvd and kvcli) files each `rule.<target>` under
the program whose output it builds (the rule's target beside the makefile
is that program's name) and every row building no single program's output
(the default goal, `all`, `test`, a platform goal, a variable) under none:
a run recipe still cites it, no program's role reads it as its own (5.1.5's
liblua.a had read "links the lua and luac executables" from rules filed
under it). A rule named through a variable the makefile does not set
(`$(BUILD)/lua`) keeps the first owner.
A makefile is read as text, as the other manifests are; what the build does
is the dry run's. A makefile elsewhere in a repository of another language
(a docs/Makefile) is no manifest. A program built from a main unit with no
build line (a closure program, every unit with clang's defaults) has none
(`TestKvdOrientationReadsTheMakefileAsTheBuildsManifest`).

Config reads, SQL statements and registrations are the language-neutral facts
over those patterns: `getenv("KEY")` is a config read, an SQL literal passed to
a library call is an SQL statement, and an external call that receives a
repository function (`pthread_create`, `qsort`, `signal`) is a registration.
C has no statement that starts a function on its own, so no call carries the
`goroutine` or `async_task` invocation; a thread's function is handed to
`pthread_create` and asked `binds`. kvd's `setCommand`, the handler of set's
table row, compares an element of the client's argument vector it was handed
with `nx`: set's sub-argument, never asked (READING, K3), as Redis's SORT,
DEBUG and SLAVEOF words are.
A C file has no dynamic execution: C has no builtin that evaluates code, so
a repository function named `eval` or `exec` is the repository's own code,
and `system` and `popen` start another program, which the reading asks
about (`runs_program`, READING) instead of the fact layer naming them. The database extractor
does not read `.c` files, so C SQL literals give no table entities.

C comments enter the ordinary claims layer. A `/* */` block or a run of `//`
lines that ends directly above a top-level declaration (a line starting at
column 0 with a name) is that declaration's docstring. Its declaration line is
where the declaration ends its header (`static int` / `foo(void)` ends on
`foo`'s line). Places and the page's symbol cards read it through one rule,
`claims.CDocstring`: it describes only the declaration located on that line or
up to two lines above it (an Allman brace), so a comment above a prototype
describes no neighbouring declaration. A comment before the file's first line
of code that no declaration follows directly is the file's description, which
describes no declaration; places shows it as the file's own documentation.

A licence, copyright or version-control stamp before the first line of code is
not a claim (owner decision D5), even when it also states the file's purpose.
Section banners, a decoration run such as `====` with a title of at most eight
words, are the author's layout and neither a claim nor structure (D4). So is
an undecorated title of one or two plain words on one line directly above a
declaration (`Implementation`, `Global vars`), unless one of its words names a
part of the declaration, ignoring case, C keywords, the header's own comments
and a plural s (`Timer events` above `struct timerEvent`); a longer comment, a
sentence and a comment written as code (`db->expires`) stay its docstring. A
comment a blank line separates from the declaration below it was never its
docstring. `NOTE:`, `WARNING:`, `IMPORTANT:` and `DEPRECATED` comment lines
are quoted from every other comment; comment markers inside strings and
character constants open nothing. C names follow the existing code-name
glossary rule (D6).

## Test sources and missing equivalents

C has no standard test runner, so Target.TestSources stays empty and a test
program is an ordinary program the model may place as a tool. Out of scope
for this first adapter:

- macros as declarations or named constants, and macro uses that are not
  calls;
- preprocessor views other than the host's (no cross sysroot or platform
  flag);
- C++, Objective-C and assembly; generating `compile_commands.json` from CMake
  or autotools;
- callbacks handed from one parameter to another;
- code that runs only in a forked child, and communication decided by where a
  file descriptor came from;
- Makefile rules as run recipes, test scripts in other languages, and
  configuration files the corpus does not admit;
- parts smaller than one file (D2), protocol commands as their own entry kind
  and joints inside one target (D7, D8);
- a command handler that re-enters the dispatch: no kvd handler calls back
  into `processCommand`, so no input of the fixture reaches its dispatch site
  directly (Redis's `exec` reaches `call`); GroupsIndex's unit test covers it.
  The request arrives there from `acceptHandler`, which registers
  `readQueryFromClient` (below).

## Callables the program's own functions keep, and tables of names (pass 2)

A callable handed to a repository function that stores that parameter in a
field or a file-scope variable is a registration with its registrar
(ProgramIndex `ParameterStores`, facts `Registrar`): kvd's
`loopCreateFileEvent(..., acceptHandler, ...)` keeps `acceptHandler` in the
loop's file event, as Redis's `aeCreateFileEvent` and `aeCreateTimeEvent`
keep `acceptHandler` and `serverCron`. Each registrar and callable is asked
once (`atlas_inputs` stored) with the call as written and while what it is
kept (`registered_during`: from the program's start at `main`, or while the
callables another registration hands over run, each route listed). The kvd
preset answers `acceptHandler` request and `readQueryFromClient`,
`sendReplyToClient` and `beforeSleep` none: `acceptHandler` is an input with
its handler, and the request dispatched at `processCommand` arrives from it
through the reader it registers (GroupsIndex outer inputs).

A file-scope table whose rows write string literals and store no repository
callable (ProgramIndex `Rows`) is asked once (`atlas_inputs` table) with its
rows, its file and the functions reading it, each with its reading lines
and its callers' calling lines as written: kvcli's `cmdTable`, read by
`lookupCommand`, which `main` calls with `argv[first]`, makes six commands whose handler is not established, one
catalogue declared by the table and looked up in `lookupCommand`; kvd's
`symsTable` is answered none. When kvcli has a confirmed integration into
kvd (its `connect` to kvd's listening socket), each row is asked which of
kvd's inputs it sends (the joints peers question): the chosen input is kept
on the row (`Operation.Sends`) and never drawn as an arrow, and a row the
answer leaves unmatched names none, whatever words the two share.

The fixture has no HTTP route table: a row whose verb qualifies the path beside
it is covered by the facts tests alone, beside the Go-shaped record
`{Method: "GET", Path: "/users", Handler: h}` of an outside type, which the Go
fixture cannot build without a module dependency. Python's `methods=["GET"]`
and JavaScript's `{method: "GET"}` are no literals in the index and state no
method.

## Inputs a call's words declare, and what C does not have yet

Every language asks each call that gives an outside symbol words what the
words become (READING, the `atlas_api` per-call question). In C that is a
call such as `strcmp(argv[i], "-h")` or `fprintf(stderr, "usage: …")`: the
kvd fixture asks `strcmp(argv[1], "--symbols")` in `main`, which its preset
answers command, `loadConfig`'s `strcasecmp(argv[0], "port")` and
`strcasecmp(argv[0], "dbfilename")`, answered setting, and `fprintf`'s and
the other word calls, answered none; `persist`'s values `never` and
`always`, compared with `argv[1]` after `argv[0]` was compared with
`persist` (answered setting), are its values and are not asked (READING,
a key's values; Go, Python and JavaScript compare a line's words with
operators, one comparison question each, so the rule has no equivalent
there); a `getenv` call is a setting read the
facts already name and is not asked. The answer is the call's, so
kvcli's `strcasecmp(argv[1], "--raw")` is an option while its
`strcasecmp(cmd->name, "bgsave")` is not. The item shows where each
argument comes from as the index records it (`internal/cproject`
`originOf`, `locals.go`):

- a string, number or character literal as written (a number a macro
  name expands to stays that name, not followed);
- a parameter of the function, by position and name;
- the result of a call written there, with the call as written;
- `index` for an element, `argv[i]`: the text as written, then the origin
  of what is indexed and of the index;
- `field` for a member, `c->argv`: the field's name, then the origin of
  the value it is read from;
- a local variable followed to the writes that reach the read in the same
  function, the rule the Go, Python and JS adapters apply to their locals:
  the nearest write every path to the read passes through (its initializer,
  or a write its code always evaluates before the read, with no label a
  jump could enter by between them), with every write that can come between
  it and the read and every later write of a loop holding the read but not
  that write; one value is that value, several are `alternatives`, each
  once, in source order, as the Go adapter records a join. With no such
  write the writes before the read stand beside the value of a path that
  writes none, which is not followed. A static local, an array and a local
  whose address the function takes are not followed; `x += 1` and `x++`
  are writes whose value is not followed.

So Redis's `strcasecmp(argv[0], "timeout")` in `loadServerConfig` reads
element 0 of the result of `sdssplitlen(line, …)`, a line of the file it
reads, and `strcmp(argv[i], "-h")` in redis-cli's `parseOptions` element
`one of: 1 | i++` of parameter #2 `argv`. The cproject test
`TestArgumentOriginsFollowElementsFieldsAndLocals` holds the four shapes,
and the fixture holds them on real calls: kvd's `loadConfig` reads its
directives as element "0" of the result of `splitLine(line, &words)`
through the local `argv`, `main`'s `--symbols` as element "1" of parameter
#2 `argv`, and kvcli's `bgsave` comparison as field `name` of the result
of `lookupCommand(argv[first])` through the local `cmd`. Tables of names
(S3) and callables the repository's own functions keep (S1) are asked in
pass 2 (above). A table an entry's handler looks up with what it was
handed lists its rows under that entry (READING); kvcli's `cmdTable` is
looked up from `main`, the program's start and no entry's handler, and a
table storing callables is a registration of each row (D1), so the C
fixture has no such lookup. The object an input is declared on (K2) and J1, a word
entry joined with the hand-over made on its result, have no C equivalent:
C's option parsing compares words and makes no object options are declared
on. A `switch` whose case labels are character literals compares its
value with those letters: two or more letters in two or more cases are one
comparison of the function (PROGRAM_INDEX `comparisons`), labels stacked on
one statement one case, a case's branch from its label to the line before
the next label or the block's end, its value's origin the switch
condition's (kvcli's `shortOption`: `switch (arg[1])` with `'h'`/`'?'` and
`'V'`; V's case calls `printVersion`, so shortOption handles V there,
READING). A number, an enum or a range label is no word. C compares strings
by calls, so a `strcmp` chain stays per-call facts with their branches.
Redis records one in each program: redis-server's glob matcher
`stringmatchlen` (`*`, `?`, `[`, `\`) and redis-cli's reply reader
`cliReadReply` (`-`, `+`, `:`, `$`, `*`). Not recorded yet, and so asked
nothing:

- `==` on an argument's characters (`argv[1][0] == '-'`); a `switch` on
  character literals is a comparison (below);
- an option string handed with the whole argument vector
  (`getopt_long(argc, argv, "hvc:o:", …)`), deferred and never asked;
- a table of names declared inside a function (`long_opts` in `main`) or a
  file-level one no call compares with;
- how a function reads a table (PROGRAM_INDEX's `membership` and `keys`
  witnesses; PYTHON): C has no membership operator (a loop comparing each
  row with `strcmp` may return a flag or the row) and no subscript by a
  string (a row handed to a lookup function is a call), so every table read
  is a plain `c_variable_read` and every table is asked (missing
  equivalent, 2026-09-29);
- an element of the argument vector handed to a repository function with no
  literal beside it (a configuration file's path in `argv[1]`);
- a registry filled element by element (`t[i].proc = fn`);
- a comparison inside a `bsearch`/`qsort` comparator;
- settings a structure names (GO, the tagged-field question): a C structure
  has no tags, so its fields name no key. A configuration directive is a
  word a call compares (`strcasecmp(argv[0], "timeout")` in Redis's
  `loadServerConfig`, asked per call above) or a row of a table of names.

Spellings of one value are recorded (PROGRAM_INDEX `same_value_as`,
`internal/cproject/spellings.go`): calls of one function written the same
but for their string literals, as the operands of one `||` written the
same around their call, or as the conditions of one if/else-if chain's
arms whose statements are written the same; each later call names the
first. kvd's `loadConfig` compares `argv[0]` with `"dbfilename"` or its
older spelling `"dbfile"` in one condition: both are asked, answered
setting, and are one input `dbfilename` with the alias `dbfile` (READING;
`TestEveryLanguageKeepsTheSpellingsOfOneValue`,
`TestCFixturePresetReadingTurnsTableRowsIntoNamedRequests`). A call
written through a macro and a function-pointer call join nothing; `&&` is
none.


## Programs a call starts

`tools/dump.c` pipes its keys through `popen("sort -u", "w")`: one word,
the whole command line, which the program question offers as written.
kvd's `system(hook)` takes its command from `KVD_START_HOOK`: no word, so
its program stays not established. `fork()` in `bgsaveCommand` runs this
program's own code and is no launch.

## Environment

Install clang on the normal PATH: on macOS the Command Line Tools
(`xcode-select --install`), on Linux the distribution's `clang` package.
`make` is needed only for a repository with a makefile.

## Verification

`testdata/repositories/c` is the cumulative executable repository and
`testdata/contracts/c.files.json` binds its exact inventory. Its `upper/`,
`util/` and `wire/` have makefiles the root never enters
(`TestCFixtureReadsADirectorysOwnMakefile`): `c:upper/upper.so`, a shared
library its makefile links with `-I..`; `c:util/ping.c`, compiled by
`util/Makefile`'s object rule; `c:util/watch.c`, whose `loopNowMs` `loop.c`
and `util/fixedclock.c` both define with no build line deciding (`make watch`
and `make watch-replay` each link one, and neither is read): both stay as
alternatives, its program read, and `util/fixedclock.c` is also `c:util/`;
and `wire/Makefile`, which asks for a platform by default and builds on
`all` as Lua 5.1.5's `src/Makefile` does: `c:wire/libwire.a`, an archive
anchored on its `$(LIB):` rule, `c:wire/wirecat`, which links it, and
`c:wire/selftest.c`, built by hand, whose `wireEscape` is `escape.c`'s,
which `libwire.a` archives with `encode.c`, never `escape_none.c` beside it
(`c:wire/`). `TestCFixtureNestedInARepositoryReadsItsMakefile` reads the
whole fixture below a root without a makefile. `TestDiscoverReadsADirectorysOwnMakefile`,
`TestAClosureTakesTheDefinitionTheBuildLinks` and
`TestAnArchiveIsALibraryOfItsMembers` cover a makefile that fails, an object
no rule makes, the build deciding or not among definitions, an archive with
no member a compile line produced and a rule named through a variable. The
native equivalents exist: Go reads every `go.mod`'s module, JS/TS the deepest
`package.json`, Clojure each `deps.edn` or `project.clj` up to the nested
ones, Python every project manifest with the deepest root deciding a file.
Several definitions of one name kept as alternatives have no equivalent in
Go, Python or JS/TS: Go refuses two package-level definitions of a name at
compile time and chooses among files by build constraints, which `go list`
evaluates for the host before the Go adapter reads anything; Python binds a
name when its module runs, and an import resolves to one module along the
search path, the nearest case (two roots providing one module) being the
package views' (PYTHON); JS/TS resolve each import to one file through
`package.json` and the compiler. Clojure's nearest case, two files declaring
one namespace on the classpath, is a missing equivalent, recorded rather than
fabricated. A library a build packages for others to link has its analogue
in each adapter's library targets, whose rules are their contracts'.
The run tests cover discovery from link lines, repomap's own repository offering no C target
and running no tool for its fixture, tooling sources beside a program, another
adapter's explicit target, the files that restore each program, one parse per
plan for a shared unit and its release after the last projection that needs it,
a backend outside this platform's build, a missing clang, an ordinary
offline run selecting a C program, whose metadata records the platform view and
whose page shows none of it, and kvd and kvcli read together: kvcli's page
lists the event loop's functions, `netListen` and `sbConsume` under "Not
reachable from the entrypoints", each run by kvd, and kvd's lists
`netConnect`, run by kvcli, both under the note that this does not establish
unused code. Facts tests cover C
config reads, SQL, dynamic execution, command rows that state no HTTP method
and route rows that state one, each with its record field as registrar.
`TestIndexProvesWhatAProgramNeverRuns` checks each way a function is named
and every unit's copy of a shared static;
`TestIndexProvesOnlyStaticsWhereOtherCodeCanCallByName` checks run-time
loading, link lines (entries, linker scripts, symbol definitions, response
files, shared objects) and libraries;
`TestCFixtureProvesWhatEachProgramNeverRuns` checks the fixture's programs,
their facts and places, and
`TestCFixturePresetReadingKeepsSharedSocketsWithTheProgramThatRunsThem` reads
kvd and kvcli together: the listener stays kvd's, the connect kvcli's, and the
event loop both link is a part of kvd's map and leaves kvcli's. The kvd
preset reading
(`TestCFixturePresetReadingTurnsTableRowsIntoNamedRequests`) answers each
registrar and each entry from its row alone, without captions, and checks the
inputs: six requests named `get`, `set`, `del`, `keys`, `ping` and `bgsave`,
the stats thread as continuous work, and neither the signal handler nor the
`qsort` comparator; claims, places and report tests cover docstrings, licence blocks, banners,
section titles and the file description at their consuming boundaries, and the
fixture's own docstrings reach its declarations. `TestIndexReadsFileScopeVariables` checks each read and non-read case of a
function body, and `TestCFixtureReadsFileScopeVariables` the fixture's:
`printSymbols` reads `symsTable`, each program's `lookupCommand` its own
`cmdTable`, `onSignal` reads `server` through `server.shutdown = 1`, the dump
tool's `main` reads `progname` in its error message and not where it assigns
it, and kvd's reads reach GroupsIndex at their sites. The places graph keeps
them as declarations' `uses` (READING): `TestCumulativeCMapOfParts` checks that
`printSymbols` uses `symsTable` (a read), `keysCommand` hands `compareKeys` to
`qsort` and `cmdTable` hands `getCommand` over. Field reads and writes are
no `uses`: a field is no declaration of its own. They reach GroupsIndex as
relation-target edges with their `field_path` and the places graph as the
declaration's `fields` (READING), keyed by the record type's place and the
field's name, which `TestCFixtureReadsAndWritesRecordFields` checks for
`onSignal`'s write of `server.shutdown`; the role split does not read them,
so no model request changes with them. The role split's helper
question (READING) shows `symsTable` with its reader, and the split check
places `saveSnapshot` with `bgsaveCommand` and keeps `staticsyms.h` out of
the box of `printSymbols`: the check takes its type `kvSymbol` for no
helper, so the header is no file of helpers. The fixture's map of parts is
checked like every other language's (`TestCumulativeCMapOfParts`), and its
split puts `netConnect`, which kvd never runs, alone in a role part that
leaves kvd's map; `TestCFixtureClientMapLeavesTheLoopItNeverRuns` checks that
kvcli's `loop.c` and `loop_poll.c` parts leave its map, undescribed, and are
listed off it by their declarations, while `loop.h`, `net.c` and `strbuf.c`
stay. `TestCFixtureOrientationReadsMainsCallsInWrittenOrder` checks that the
orientation request carries kvd's `main` with its calls in the order `main`
writes them.
