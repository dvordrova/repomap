package cproject

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

const indexMakefile = `CFLAGS = -std=c99 -O2 -Wall

all: kvd kvcli

kvd: kvd.o loop.o strbuf.o
	$(CC) -o kvd kvd.o loop.o strbuf.o -pthread

kvcli: kvcli.o strbuf.o
	$(CC) -o kvcli kvcli.o strbuf.o

.c.o:
	$(CC) -c $(CFLAGS) $<
`

const indexHeader = `#ifndef KV_H
#define KV_H
typedef struct kvClient { int fd; const char *argv0; } kvClient;
typedef void kvCommandProc(kvClient *c);
struct kvCommand { const char *name; kvCommandProc *proc; int arity; };
typedef struct { int hits; } kvStats;
typedef enum { KV_OK, KV_ERR } kvStatus;
typedef char *kvString;
extern kvStats stats;
void kvFail(const char *expr);
#define kvAssert(e) ((e) ? (void)0 : kvFail(#e))
static inline int kvShared(void) { return 1; }
#endif
`

const indexLoopHeader = `typedef void fileProc(int fd);
typedef struct fileEvent { int mask; fileProc *readProc; fileProc *writeProc; } fileEvent;
void loopWatch(int fd, int mask, fileProc *proc);
void loopTimer(void (*proc)(void));
void loopRun(void);
void loopStop(void);
`

const indexLoop = `#include "loop.h"

static fileEvent events[16];
static void (*timer)(void);
static int stop;

void loopWatch(int fd, int mask, fileProc *proc) {
    fileEvent *fe = &events[fd];
    fe->mask |= mask;
    if (mask & 1) fe->readProc = proc;
    if (mask & 2) fe->writeProc = proc;
}

void loopTimer(void (*proc)(void)) { timer = proc; }

void loopRun(void) {
    while (!stop) {
        for (int fd = 0; fd < 16; fd++) {
            fileEvent *fe = &events[fd];
            if (fe->mask & 1) fe->readProc(fd);
        }
        timer();
    }
}

void loopStop(void) { stop = 1; }
`

const indexServer = `#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "kv.h"
#include "loop.h"
#include "strbuf.h"
#include "util.c"

kvStats stats;
static void getCommand(kvClient *c);
static void setCommand(kvClient *c);
static void pingCommand(kvClient *c) { (void)c; }

static struct kvCommand cmdTable[] = {
    {"get", getCommand, 2},
    {"set", setCommand, 3},
    {"ping", pingCommand, 1},
    {NULL, NULL, 0}
};

/* Addresses kept as numbers for backtraces, not callbacks. */
static struct { const char *name; unsigned long pointer; } symbols[] = {
    {"getCommand", (unsigned long)getCommand},
};

static struct kvCommand *lookupCommand(const char *name) {
    for (int i = 0; cmdTable[i].name; i++)
        if (!strcmp(cmdTable[i].name, name)) return &cmdTable[i];
    return NULL;
}

static void call(kvClient *c, struct kvCommand *cmd) {
    if (cmd->proc == pingCommand) return;
    cmd->proc(c);
}

static void getCommand(kvClient *c) { printf("get %d\n", c->fd); }
static void setCommand(kvClient *c) { printf("set %d\n", clampInt(c->fd)); }

static int compare(const void *a, const void *b) { return strcmp(a, b); }
static void readClient(int fd) { kvClient c = {fd, "x"}; call(&c, lookupCommand("get")); }
static void writeClient(int fd) { (void)fd; }
static void tick(void) { loopStop(); }
static void crash(int sig, siginfo_t *info, void *ctx) { (void)sig; (void)info; (void)ctx; exit(1); }
static void apply(void (*each)(int), int n) { each(n); }

static void once(void) {
    struct kvCommand local = {"local", pingCommand, 0};
    local.proc(NULL);
}

int main(int argc, char **argv) {
    struct sigaction act;
    const char *names[2] = {"b", "a"};
    act.sa_sigaction = crash;
    sigaction(SIGSEGV, &act, NULL);
    qsort(names, 2, sizeof names[0], compare);
    kvAssert(sbLen(argv[0]) > argc);
    loopWatch(0, 1, readClient);
    loopWatch(1, 2, writeClient);
    loopTimer(tick);
    apply(writeClient, 3);
    once();
    loopRun();
    return kvShared() + (int)(symbols[0].pointer == 0);
}
`

const indexStrbuf = `#include <string.h>
#include "kv.h"
#include "strbuf.h"
#include "util.c"

static int compare(const char *a, const char *b) { return strcmp(a, b); }

int sbLen(const char *s) { return clampInt((int)strlen(s)) + compare(s, s); }
void kvFail(const char *expr) { (void)expr; }
`

func indexFiles() map[string]string {
	return map[string]string{
		"Makefile": indexMakefile,
		"kv.h":     indexHeader,
		"loop.h":   indexLoopHeader,
		"loop.c":   indexLoop,
		"kvd.c":    indexServer,
		"strbuf.h": "int sbLen(const char *s);\n",
		"strbuf.c": indexStrbuf,
		// Included by two units of one program: one definition.
		"util.c":  "static int clampInt(int v) { return v < 0 ? 0 : v; }\n",
		"kvcli.c": "#include \"kv.h\"\n#include \"strbuf.h\"\n\nstatic struct kvCommand cmdTable[] = {{\"get\", NULL, 2}};\n\nint main(void) { return sbLen(cmdTable[0].name); }\n",
	}
}

type indexed struct {
	repository *corpus.Corpus
	result     *Result
	index      programindex.Index
}

func indexProgram(t *testing.T, files map[string]string, selector string) indexed {
	t.Helper()
	root, repository := writeRepository(t, files)
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(t.Context(), root, repository, programBySelector(t, project, selector), NewStore())
	if err != nil {
		t.Fatal(err)
	}
	result, err := Index(repository, parsed)
	if err != nil {
		t.Fatal(err)
	}
	index := adaptertest.AssertConforms(t, adaptertest.AdapterFunc(func() (programindex.Input, error) {
		again, err := Index(repository, parsed)
		if err != nil {
			return programindex.Input{}, err
		}
		return again.Input, nil
	}))
	adaptertest.AssertSharedArtifact(t, result.Input, index)
	return indexed{repository: repository, result: result, index: index}
}

func (x indexed) objects(t *testing.T, name string) []programindex.Object {
	t.Helper()
	var found []programindex.Object
	for _, object := range x.index.Objects {
		if object.Name == name {
			found = append(found, object)
		}
	}
	return found
}

func (x indexed) object(t *testing.T, name, path string) programindex.Object {
	t.Helper()
	for _, object := range x.objects(t, name) {
		if path == "" || object.Location != nil && object.Location.Path == path {
			return object
		}
	}
	t.Fatalf("no object %s in %s", name, path)
	return programindex.Object{}
}

func (x indexed) byID(id string) programindex.Object {
	for _, object := range x.index.Objects {
		if object.ID == id {
			return object
		}
	}
	return programindex.Object{}
}

func (x indexed) names(ids []string) []string {
	var names []string
	for _, id := range ids {
		names = append(names, x.byID(id).Name)
	}
	slices.Sort(names)
	return names
}

// relations are the relations of a kind from an object whose first pattern,
// when there is one, has the selector.
func (x indexed) relations(kind programindex.RelationKind, from, selector string) []programindex.Relation {
	var found []programindex.Relation
	for _, relation := range x.index.Relations {
		if relation.Kind != kind || relation.FromID != from {
			continue
		}
		if selector != "" && (len(relation.Patterns) == 0 || relation.Patterns[0].Selector != selector) {
			continue
		}
		found = append(found, relation)
	}
	return found
}

func (x indexed) one(t *testing.T, kind programindex.RelationKind, from, selector string) programindex.Relation {
	t.Helper()
	found := x.relations(kind, from, selector)
	if len(found) != 1 {
		t.Fatalf("%s %s from %s: %d relations: %+v", kind, selector, x.byID(from).Name, len(found), found)
	}
	return found[0]
}

func witnessKinds(relation programindex.Relation) []string {
	var kinds []string
	for _, witness := range relation.Witnesses {
		kinds = append(kinds, witness.Kind+": "+witness.Detail)
	}
	return kinds
}

func TestIndexKeepsLinkerIdentities(t *testing.T) {
	x := indexProgram(t, indexFiles(), "c:kvd")
	if x.index.Target.Language != "c" || x.index.Target.Kind != "executable" || x.index.Target.Name != "kvd" || x.index.Target.Selector != "c:kvd" {
		t.Fatalf("target: %+v", x.index.Target)
	}
	// A function is found at its definition, never its prototype.
	get := x.object(t, "getCommand", "kvd.c")
	if get.Kind != programindex.ObjectFunction || get.Visibility != programindex.VisibilityInternal ||
		get.Location.Line != lineOf(t, indexServer, "static void getCommand(kvClient *c) {") || get.Signature != "void getCommand(kvClient *c)" {
		t.Fatalf("getCommand: %+v", get)
	}
	main := x.object(t, "main", "kvd.c")
	if len(x.index.Target.Seeds) != 1 || x.index.Target.Seeds[0].ObjectID != main.ID || x.index.Target.Seeds[0].Kind != programindex.SeedCallable || main.ID != "n1" || main.Visibility != programindex.VisibilityPublic {
		t.Fatalf("main %+v seeds %+v", main, x.index.Target.Seeds)
	}
	// Same-named statics of two units stay two; a header's static inline
	// function and a .c file two units include are one definition each.
	if compare := x.objects(t, "compare"); len(compare) != 2 || compare[0].Location.Path == compare[1].Location.Path {
		t.Fatalf("compare: %+v", compare)
	}
	if shared := x.objects(t, "kvShared"); len(shared) != 1 || shared[0].Location.Path != "kv.h" {
		t.Fatalf("kvShared: %+v", shared)
	}
	if clamp := x.objects(t, "clampInt"); len(clamp) != 1 || clamp[0].Location.Path != "util.c" {
		t.Fatalf("clampInt: %+v", clamp)
	}
	// A tentative definition is the variable; the extern declaration is not.
	if stats := x.objects(t, "stats"); len(stats) != 1 || stats[0].Location.Path != "kvd.c" || stats[0].Visibility != programindex.VisibilityPublic {
		t.Fatalf("stats: %+v", stats)
	}
	// Types: a typedef names the record it defines, an anonymous record takes
	// its typedef's name, a typedef of another type is a type of its own.
	client := x.object(t, "kvClient", "kv.h")
	command := x.object(t, "kvCommand", "kv.h")
	statsType := x.object(t, "kvStats", "kv.h")
	status := x.object(t, "kvStatus", "kv.h")
	text := x.object(t, "kvString", "kv.h")
	if client.Kind != programindex.ObjectType || client.Signature != "struct" || len(x.objects(t, "kvClient")) != 1 ||
		statsType.Signature != "struct" || status.Signature != "enum" || text.Signature != "char *" || command.Visibility != programindex.VisibilityPublic {
		t.Fatalf("types: %+v %+v %+v %+v", client, statsType, status, text)
	}
	if fd := x.object(t, "fd", "kv.h"); fd.OwnerID != client.ID || fd.ContainerID != client.ID || fd.Kind != programindex.ObjectVariable || fd.Signature != "int" {
		t.Fatalf("field: %+v", fd)
	}
	if ok := x.object(t, "KV_OK", "kv.h"); ok.OwnerID != status.ID {
		t.Fatalf("enum constant: %+v", ok)
	}
	// Signatures carry the repository types through pointers and const.
	if !reflect.DeepEqual(get.Parameters, []programindex.TypedName{{Name: "c", Type: "kvClient *", TypeID: client.ID}}) {
		t.Fatalf("parameters: %+v", get.Parameters)
	}
	lookup := x.object(t, "lookupCommand", "kvd.c")
	if !reflect.DeepEqual(lookup.Results, []programindex.TypedName{{Type: "struct kvCommand *", TypeID: command.ID}}) ||
		lookup.Signature != "struct kvCommand *lookupCommand(const char *name)" {
		t.Fatalf("lookupCommand: %+v", lookup)
	}
	module := x.object(t, "kvd.c", "kvd.c")
	if module.Kind != programindex.ObjectModule || get.ContainerID != module.ID {
		t.Fatalf("module: %+v, getCommand in %s", module, get.ContainerID)
	}
}

func TestIndexRefusesTwoExternalDefinitions(t *testing.T) {
	root, repository := writeRepository(t, map[string]string{
		"Makefile": "app: a.o b.o\n\tcc -o app a.o b.o\n",
		"a.c":      "int twice(void) { return 1; }\nint main(void) { return twice(); }\n",
		"b.c":      "int twice(void) { return 2; }\n",
	})
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:app"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Index(repository, parsed); err == nil || !strings.Contains(err.Error(), "function twice is defined twice: a.c:1 and b.c:1") {
		t.Fatalf("two definitions: %v", err)
	}
}

func TestIndexResolvesCallsAcrossUnitsAndThePlatform(t *testing.T) {
	x := indexProgram(t, indexFiles(), "c:kvd")
	main := x.object(t, "main", "kvd.c")
	sbLen := x.one(t, programindex.RelationCalls, main.ID, "sbLen")
	if x.byID(sbLen.ToIDs[0]).Location.Path != "strbuf.c" || sbLen.Resolution != programindex.ResolutionExact {
		t.Fatalf("sbLen: %+v", sbLen)
	}
	// A call written in a macro argument keeps its own column.
	assertLine := lineOf(t, indexServer, "kvAssert(sbLen")
	if sbLen.Location.Line != assertLine || sbLen.Location.Column != strings.Index(indexServer[strings.Index(indexServer, "kvAssert(sbLen"):], "sbLen")+5 {
		t.Fatalf("sbLen site: %+v", sbLen.Location)
	}
	// A call a macro body writes is found at the macro, with the body's place.
	fail := x.one(t, programindex.RelationCalls, main.ID, "kvAssert")
	if x.byID(fail.ToIDs[0]).Name != "kvFail" || fail.Location.Line != assertLine || fail.Location.Column != 5 {
		t.Fatalf("kvAssert: %+v", fail)
	}
	if !slices.ContainsFunc(fail.Witnesses, func(w programindex.Witness) bool {
		return w.Kind == "macro_expansion" && w.Location != nil && w.Location.Path == "kv.h" && w.Location.Line == lineOf(t, indexHeader, "#define kvAssert")
	}) {
		t.Fatalf("kvAssert witnesses: %v", witnessKinds(fail))
	}
	// A platform function is invokes_external under the header the
	// repository includes, not the SDK header that declares it.
	qsort := x.one(t, programindex.RelationInvokesExternal, main.ID, "qsort")
	if external := x.byID(qsort.ToIDs[0]).External; external == nil || *external != (programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPlatform, PackagePath: "stdlib.h", Name: "qsort"}) {
		t.Fatalf("qsort: %+v", external)
	}
	// Arguments: literals, source text, and a function named as a callback
	// bound to its argument.
	pattern := qsort.Patterns[0]
	if len(pattern.Arguments) != 4 || pattern.Arguments[0].Origin.Text != "names" || pattern.Arguments[3].Kind != programindex.PatternDynamic || len(pattern.Arguments[3].ObjectIDs) != 1 {
		t.Fatalf("qsort arguments: %+v", pattern.Arguments)
	}
	var compare programindex.Relation
	for _, relation := range x.relations(programindex.RelationPassesCallback, main.ID, "") {
		if relation.SourceArgumentID == pattern.Arguments[3].ID {
			compare = relation
		}
	}
	if len(compare.ToIDs) != 1 || x.byID(compare.ToIDs[0]).Location.Path != "kvd.c" || x.byID(compare.ToIDs[0]).Name != "compare" || compare.Witnesses[0].Kind != "c_callback_argument" {
		t.Fatalf("compare callback: %+v", compare)
	}
	get := x.one(t, programindex.RelationCalls, x.object(t, "readClient", "kvd.c").ID, "lookupCommand")
	if arguments := get.Patterns[0].Arguments; len(arguments) != 1 || arguments[0].Kind != programindex.PatternLiteralString || arguments[0].Value != "get" {
		t.Fatalf("lookupCommand arguments: %+v", arguments)
	}
	// Imports and dependencies: repository headers are the workspace,
	// platform headers the standard library.
	imports := map[string]string{}
	for _, relation := range x.relations(programindex.RelationImports, x.object(t, "kvd.c", "kvd.c").ID, "") {
		target := x.byID(relation.ToIDs[0])
		imports[target.Name] = string(target.Kind)
	}
	if imports["kv.h"] != "module" || imports["stdlib.h"] != "external_symbol" || imports["util.c"] != "module" || len(imports) != 8 {
		t.Fatalf("imports: %v", imports)
	}
	kinds := map[string]dependencies.Kind{}
	for _, dependency := range x.result.Dependencies.Dependencies {
		kinds[dependency.Name] = dependency.Kind
		if dependency.Language != "c" {
			t.Fatalf("dependency language: %+v", dependency)
		}
	}
	if kinds["kv.h"] != dependencies.KindWorkspace || kinds["stdlib.h"] != dependencies.KindStdlib || kinds["signal.h"] != dependencies.KindStdlib {
		t.Fatalf("dependencies: %v", kinds)
	}
}

func TestIndexResolvesCallsThroughStoredFunctions(t *testing.T) {
	x := indexProgram(t, indexFiles(), "c:kvd")
	// Every row of the table stores its handler into kvCommand.proc.
	dispatch := x.one(t, programindex.RelationCalls, x.object(t, "call", "kvd.c").ID, "proc")
	if dispatch.Dispatch != programindex.DispatchFunctionValue || dispatch.Resolution != programindex.ResolutionAlternatives ||
		!reflect.DeepEqual(x.names(dispatch.ToIDs), []string{"getCommand", "pingCommand", "setCommand"}) {
		t.Fatalf("cmd->proc: %+v %v", dispatch, x.names(dispatch.ToIDs))
	}
	// A comparison with a function is no call and no callback.
	for _, relation := range x.relations(programindex.RelationPassesCallback, x.object(t, "call", "kvd.c").ID, "") {
		t.Fatalf("comparison became a callback: %+v", relation)
	}
	// A function kept as a number is no store: symbols stores nothing.
	for _, relation := range x.relations(programindex.RelationPassesCallback, x.object(t, "symbols", "kvd.c").ID, "") {
		t.Fatalf("an address cast to an integer became a callback: %+v", relation)
	}
	// A store under a branch leaves the call unresolved, the candidates its
	// witnesses: readProc is set only when mask says so.
	read := x.one(t, programindex.RelationCalls, x.object(t, "loopRun", "loop.c").ID, "readProc")
	var candidates []string
	for _, witness := range read.Witnesses {
		if witness.Kind == "c_function_pointer_store" {
			candidates = append(candidates, fmt.Sprintf("%s@%s:%d", witness.Detail, witness.Location.Path, witness.Location.Line))
		}
	}
	want := []string{
		fmt.Sprintf("readClient stored in fileEvent.readProc by loopWatch under a condition@kvd.c:%d", lineOf(t, indexServer, "loopWatch(0, 1, readClient)")),
		fmt.Sprintf("writeClient stored in fileEvent.readProc by loopWatch under a condition@kvd.c:%d", lineOf(t, indexServer, "loopWatch(1, 2, writeClient)")),
	}
	if read.Resolution != programindex.ResolutionUnresolved || len(read.ToIDs) != 0 || !reflect.DeepEqual(candidates, want) {
		t.Fatalf("fe->readProc: %s %v\n candidates %q\n want %q", read.Resolution, x.names(read.ToIDs), candidates, want)
	}
	// An unconditional store of a parameter joins what callers pass: one
	// function is exact, through a variable and through a parameter.
	timer := x.one(t, programindex.RelationCalls, x.object(t, "loopRun", "loop.c").ID, "timer")
	each := x.one(t, programindex.RelationCalls, x.object(t, "apply", "kvd.c").ID, "each")
	if timer.Resolution != programindex.ResolutionExact || x.names(timer.ToIDs)[0] != "tick" || each.Resolution != programindex.ResolutionExact || x.names(each.ToIDs)[0] != "writeClient" {
		t.Fatalf("timer %+v each %+v", timer, each)
	}
	// The callback handed to loopWatch is a store into the loop's fields.
	main := x.object(t, "main", "kvd.c")
	var stored []string
	for _, relation := range x.relations(programindex.RelationPassesCallback, main.ID, "") {
		if x.names(relation.ToIDs)[0] == "readClient" {
			stored = witnessKinds(relation)
		}
	}
	// A branch decides which field: readClient goes into one of them.
	if !reflect.DeepEqual(stored, []string{"c_function_pointer_store: readClient stored in fileEvent.readProc or fileEvent.writeProc by loopWatch under a condition"}) {
		t.Fatalf("readClient callback: %v", stored)
	}
	// Loop context on calls: the body of while and for.
	if context := read.Patterns[0].Context; len(context) != 2 || context[0].Detail != "while body" || context[0].Location.Line != lineOf(t, indexLoop, "while (!stop)") ||
		context[1].Detail != "for body" || context[1].Location.Line != lineOf(t, indexLoop, "for (int fd") {
		t.Fatalf("readProc context: %+v", context)
	}
}

func TestIndexKeepsTableRowsAndPlatformFieldsAsRegistrations(t *testing.T) {
	x := indexProgram(t, indexFiles(), "c:kvd")
	table := x.object(t, "cmdTable", "kvd.c")
	command := x.object(t, "kvCommand", "kv.h")
	get := x.object(t, "getCommand", "kvd.c")
	getLine := lineOf(t, indexServer, `{"get", getCommand, 2}`)
	adaptertest.AssertRegistration(t, x.index, adaptertest.Registration{
		Name: "get row",
		Registration: adaptertest.Relation{Kind: programindex.RelationCalls, FromID: table.ID, ToIDs: []string{command.ID}, Resolution: programindex.ResolutionExact,
			Invocation: programindex.InvocationConstruct, Path: "kvd.c", Line: getLine, TargetsObserved: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{Form: programindex.PatternCall, Selector: "kvCommand", Path: "kvd.c", Line: getLine, Observed: 2, Arguments: []adaptertest.Argument{
				{Keyword: "name", Kind: programindex.PatternLiteralString, Value: "get"},
				{Keyword: "proc", Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{IDs: []string{get.ID}, Resolution: programindex.ResolutionExact, Observed: 1}},
			}}}},
		Callbacks: []adaptertest.Callback{{ArgumentKeyword: "proc", Relation: adaptertest.Relation{Kind: programindex.RelationPassesCallback, FromID: table.ID, ToIDs: []string{get.ID},
			Resolution: programindex.ResolutionExact, Path: "kvd.c", Line: getLine, TargetsObserved: 1, WitnessesObserved: 2}}},
		RequireComplete: true,
	})
	var callback programindex.Relation
	for _, relation := range x.relations(programindex.RelationPassesCallback, table.ID, "") {
		if relation.ToIDs[0] == get.ID {
			callback = relation
		}
	}
	if kinds := witnessKinds(callback); !reflect.DeepEqual(kinds, []string{"c_function_pointer_store: getCommand stored in kvCommand.proc by a row of cmdTable", `callable_receiver_field: name = "get"`}) {
		t.Fatalf("row witnesses: %v", kinds)
	}
	// A row a function builds for itself binds its function to that function,
	// without the construction a table's row is.
	local := x.one(t, programindex.RelationPassesCallback, x.object(t, "once", "kvd.c").ID, "")
	if local.SourceArgumentID != "" || !slices.Contains(witnessKinds(local), "c_function_pointer_store: pingCommand stored in kvCommand.proc by once") {
		t.Fatalf("local row: %+v", local)
	}
	// A platform record's field is named as written: sa_sigaction is a macro
	// for a member of a union, and the record is the variable's own type.
	main := x.object(t, "main", "kvd.c")
	crash := x.object(t, "crash", "kvd.c")
	storeLine := lineOf(t, indexServer, "act.sa_sigaction = crash")
	construct := x.one(t, programindex.RelationInvokesExternal, main.ID, "struct sigaction")
	if external := x.byID(construct.ToIDs[0]).External; construct.Invocation != programindex.InvocationConstruct || external == nil ||
		*external != (programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPlatform, PackagePath: "signal.h", Name: "struct sigaction"}) ||
		construct.Location.Line != storeLine || construct.Patterns[0].Arguments[0].Keyword != "sa_sigaction" || !reflect.DeepEqual(construct.Patterns[0].Arguments[0].ObjectIDs, []string{crash.ID}) {
		t.Fatalf("sigaction: %+v", construct)
	}
}

// buildPlaces reads programs the way the ordinary pipeline does after the
// adapter: facts, then places.
func buildPlaces(t *testing.T, repository *corpus.Corpus, indexes ...programindex.Index) (facts.Result, atlas.Graph) {
	t.Helper()
	indexes, err := programindex.RebindTargetSet(indexes)
	if err != nil {
		t.Fatal(err)
	}
	var factTargets []facts.TargetInput
	var placeTargets []places.TargetInput
	for _, index := range indexes {
		factTargets = append(factTargets, facts.TargetInput{Index: index, Root: "."})
		placeTargets = append(placeTargets, places.TargetInput{Index: index})
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: factTargets})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Revision: strings.Repeat("a", 40), Repository: repository, Targets: placeTargets, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := atlas.EncodeGraph(graph); err != nil {
		t.Fatal(err)
	}
	return layer, graph
}

// The owner's D1: a function a row of a repository table names by a string
// literal is a registration, and places shows it on the function as a binding
// carrying the literal. A binding with only field witnesses would give
// places no row at all.
func TestTableRowsReachPlacesAsRegistrations(t *testing.T) {
	x := indexProgram(t, indexFiles(), "c:kvd")
	layer, graph := buildPlaces(t, x.repository, x.index)
	var registrations []string
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		registrations = append(registrations, fmt.Sprintf("%s %v -> %s", fact.Key, fact.Values, fact.Symbol))
	}
	slices.Sort(registrations)
	// The row a function builds in its own body, and kvcli's row without a
	// function, are not registrations; getCommand kept as a number is not.
	want := []string{"kvCommand [get] -> getCommand", "kvCommand [ping] -> pingCommand", "kvCommand [set] -> setCommand", "qsort [] -> compare", "struct sigaction [] -> crash"}
	if !reflect.DeepEqual(registrations, want) {
		t.Fatalf("registrations:\n got %q\nwant %q", registrations, want)
	}
	boundaries := 0
	for _, place := range graph.Places {
		if place.Boundary != nil && place.Path == "kvd.c" && place.LineNo == lineOf(t, indexServer, `{"get", getCommand, 2}`) {
			boundaries++
			if place.Boundary.Direction != atlas.DirectionIn || !reflect.DeepEqual(place.Boundary.Values, []string{"get"}) {
				t.Fatalf("get boundary: %+v", place.Boundary)
			}
		}
	}
	if boundaries != 1 {
		t.Fatalf("get row boundaries: %d", boundaries)
	}
	adaptertest.AssertRegistrationArgument(t, graph, "kvd.c", "getCommand", map[int]string{lineOf(t, indexServer, `{"get", getCommand, 2}`): "get"})
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Symbol.Decl.Name != "getCommand" {
			continue
		}
		for _, binding := range place.Symbol.Bindings {
			if binding.Detail != "getCommand stored in kvCommand.proc by a row of cmdTable" || binding.From != "cmdTable" ||
				!slices.ContainsFunc(binding.Evidence, func(e atlas.EdgeEvidence) bool {
					return e.Extractor == "callable_receiver_field" && e.Label == `name = "get"`
				}) {
				t.Fatalf("getCommand binding: %+v", binding)
			}
		}
	}
	adaptertest.AssertCallControls(t, x.index, graph, "loop.c", "timer", map[int][]adaptertest.Control{
		lineOf(t, indexLoop, "timer();"): {{Line: lineOf(t, indexLoop, "while (!stop)"), Kind: "while body"}},
	})
	// Each row is its own registration at the function it stores.
	adaptertest.AssertCallSiteBoundaries(t, x.repository, x.result.Input, "kvd.c", []adaptertest.CallSite{
		{Line: lineOf(t, indexServer, `{"get", getCommand, 2}`), Column: len(`    {"get", `) + 1, Key: "kvCommand", Symbol: "getCommand"},
		{Line: lineOf(t, indexServer, `{"set", setCommand, 3}`), Column: len(`    {"set", `) + 1, Key: "kvCommand", Symbol: "setCommand"},
		{Line: lineOf(t, indexServer, `{"ping", pingCommand, 1}`), Column: len(`    {"ping", `) + 1, Key: "kvCommand", Symbol: "pingCommand"},
	})
}
