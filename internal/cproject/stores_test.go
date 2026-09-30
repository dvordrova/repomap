package cproject

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

const storesSource = `#include <stdio.h>
#include <stdlib.h>
int puts(const char *s);

struct list { void (*free)(void *); void (*dup)(void *); };
struct owned { void (*release)(void *); };

static void freeValue(void *v) { (void)v; }
static void first(void) {}
static void second(void) {}
static void (*steps[])(void) = { first, second };
static void (*picked)(void);
static void (*pick(int n))(void) { return n ? first : second; }
int missing(void);

static void copy(struct list *to, struct list *from) {
    to->free = from->free;
    to->dup = from->free;
}

int main(int argc, char **argv) {
    struct list l;
    struct owned o;
    (void)argv;
    o.release = free;
    o.release(0);
    puts("again");
    l.free = freeValue;
    l.free(0);
    l.dup(0);
    steps[argc]();
    picked = pick(argc);
    picked();
    while (1) { second(); break; }
    do { first(); } while (0);
    if (__builtin_expect(argc > 1, 0)) return 2;
    return missing();
}
`

func TestIndexReadsStoresBeyondTables(t *testing.T) {
	x := indexProgram(t, map[string]string{"stores.c": storesSource}, "c:stores.c")
	main := x.object(t, "main", "stores.c")
	// A function assigned to a field is a callback of the assigning function
	// and the one target of calls through the field; copying the field into
	// itself adds nothing.
	free := x.one(t, programindex.RelationCalls, main.ID, "free")
	if free.Resolution != programindex.ResolutionExact || x.names(free.ToIDs)[0] != "freeValue" {
		t.Fatalf("l.free: %+v", free)
	}
	var stored []string
	for _, relation := range x.relations(programindex.RelationPassesCallback, main.ID, "") {
		stored = append(stored, witnessKinds(relation)...)
	}
	if !reflect.DeepEqual(stored, []string{"c_function_pointer_store: freeValue stored in list.free"}) {
		t.Fatalf("main's callbacks: %v", stored)
	}
	// A platform function stored and called through a field is that
	// function; one the repository declares again is still the platform's.
	release := x.one(t, programindex.RelationInvokesExternal, main.ID, "release")
	puts := x.one(t, programindex.RelationInvokesExternal, main.ID, "puts")
	for _, relation := range []programindex.Relation{release, puts} {
		if external := x.byID(relation.ToIDs[0]).External; relation.Resolution != programindex.ResolutionExact || external == nil || external.PackagePath != "stdlib.h" && external.PackagePath != "stdio.h" {
			t.Fatalf("%s: %+v %+v", relation.Patterns[0].Selector, relation, external)
		}
	}
	if release.Dispatch != programindex.DispatchFunctionValue || x.byID(release.ToIDs[0]).Name != "free" {
		t.Fatalf("o.release: %+v", release)
	}
	// Another field's value is a value the index cannot name.
	if dup := x.one(t, programindex.RelationCalls, main.ID, "dup"); dup.Resolution != programindex.ResolutionUnresolved {
		t.Fatalf("l.dup: %+v", dup)
	}
	// An array of functions: its elements are the alternatives.
	if steps := x.one(t, programindex.RelationCalls, main.ID, "steps"); steps.Resolution != programindex.ResolutionAlternatives ||
		!reflect.DeepEqual(x.names(steps.ToIDs), []string{"first", "second"}) {
		t.Fatalf("steps[argc](): %+v", steps)
	}
	// A call's result stored in a variable is unknown: no false alternatives.
	if picked := x.one(t, programindex.RelationCalls, main.ID, "picked"); picked.Resolution != programindex.ResolutionUnresolved || len(picked.ToIDs) != 0 {
		t.Fatalf("picked(): %+v", picked)
	}
	// A repository declaration no unit defines stays unresolved and says so.
	missing := x.one(t, programindex.RelationCalls, main.ID, "missing")
	if missing.Resolution != programindex.ResolutionUnresolved || missing.Witnesses[0].Detail != "missing is declared at stores.c:14 and defined in no unit of stores.c" {
		t.Fatalf("missing(): %+v", missing)
	}
	// clang's builtins are declared where first used; they are the platform's.
	expect := x.one(t, programindex.RelationInvokesExternal, main.ID, "__builtin_expect")
	if external := x.byID(expect.ToIDs[0]).External; *external != (programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPlatform, PackagePath: "builtin", Name: "__builtin_expect"}) {
		t.Fatalf("__builtin_expect: %+v", external)
	}
	contexts := map[string]string{}
	for _, selector := range []string{"first", "second"} {
		for _, relation := range x.relations(programindex.RelationCalls, main.ID, selector) {
			for _, witness := range relation.Patterns[0].Context {
				contexts[selector] = witness.Detail
			}
		}
	}
	if !reflect.DeepEqual(contexts, map[string]string{"second": "while body with constant true condition", "first": "do-while body"}) {
		t.Fatalf("loop contexts: %v", contexts)
	}
}

// A header from an include directory outside the platform is a package: its
// functions are invoked with package authority and it is an external
// dependency, while a platform header stays the standard library.
func TestIndexNamesPackageHeaders(t *testing.T) {
	packages := t.TempDir()
	if err := os.MkdirAll(filepath.Join(packages, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packages, "pkg", "pkg.h"), []byte("int pkg_run(const char *name, void (*job)(void));\nstruct pkg_hook { void (*run)(void); };\nvoid pkg_hook_add(struct pkg_hook *hook);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Makefile": "CFLAGS = -I" + packages + "\napp: app.o\n\t$(CC) -o app app.o -lpkg\n.c.o:\n\t$(CC) -c $(CFLAGS) $<\n",
		"app.c": "#include <stdio.h>\n#include <pkg/pkg.h>\n\n#define ON_RUN(h, f) ((h).run = (f))\n\nstatic void job(void) { puts(\"job\"); }\n\n" +
			"int main(void) {\n    struct pkg_hook hook;\n    ON_RUN(hook, job);\n    pkg_hook_add(&hook);\n    return pkg_run(\"nightly\", job);\n}\n",
	}
	x := indexProgram(t, files, "c:app")
	main := x.object(t, "main", "app.c")
	run := x.one(t, programindex.RelationInvokesExternal, main.ID, "pkg_run")
	if external := x.byID(run.ToIDs[0]).External; external == nil || *external != (programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "pkg/pkg.h", Name: "pkg_run"}) {
		t.Fatalf("pkg_run: %+v", external)
	}
	kinds := map[string]dependencies.Kind{}
	for _, dependency := range x.result.Dependencies.Dependencies {
		kinds[dependency.Name] = dependency.Kind
	}
	if !reflect.DeepEqual(kinds, map[string]dependencies.Kind{"pkg/pkg.h": dependencies.KindExternal, "stdio.h": dependencies.KindStdlib}) {
		t.Fatalf("dependencies: %v", kinds)
	}
	// A store a repository macro writes into a package record's field is
	// keyed on the field as the macro spells it, not on the macro's name.
	hook := x.one(t, programindex.RelationInvokesExternal, main.ID, "struct pkg_hook")
	if arguments := hook.Patterns[0].Arguments; hook.Invocation != programindex.InvocationConstruct || len(arguments) != 1 || arguments[0].Keyword != "run" {
		t.Fatalf("struct pkg_hook: %+v", hook)
	}
	layer, _ := buildPlaces(t, x.repository, x.index)
	var registrations []string
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		registrations = append(registrations, fact.Key+" "+fact.Symbol+" "+fact.Text)
	}
	slices.Sort(registrations)
	if !slices.Equal(registrations, []string{"pkg_run job pkg/pkg.h.pkg_run", "struct pkg_hook job pkg/pkg.h.struct pkg_hook"}) {
		t.Fatalf("registrations: %q", registrations)
	}
}

func TestCStringDecodesClangLiterals(t *testing.T) {
	for value, want := range map[string]string{
		`"get"`:           "get",
		`"a\nb\t\"q\""`:   "a\nb\t\"q\"",
		`"\033[0m\x41"`:   "\x1b[0mA",
		`L"wide"`:         "wide",
		`u8"caf\303\251"`: "café",
	} {
		if got, ok := cString(value); !ok || got != want {
			t.Errorf("cString(%s) = %q, %v; want %q", value, got, ok, want)
		}
	}
	// Bytes that are not text are no literal value.
	if _, ok := cString(`"\xff"`); ok {
		t.Error("an invalid UTF-8 literal decoded")
	}
}

const slotsSource = `static void first(void) {}
static void second(void) {}

/* Elements of a module-level array of functions are bound to the array. */
static void (*steps[])(void) = { first, second };

/* An unnamed bit-field is padding: the row's elements skip it. */
struct step { int order; unsigned :4; void (*run)(void); const char *name; };
static struct step plan[] = { {1, first, "first"} };

/* A tentative definition and the definition are one variable. */
static void (*hook)(void);
static void (*hook)(void) = first;

/* runWith is also kept in a pointer, so its direct callers are not all
   that reaches job. */
static void runWith(void (*job)(void)) { job(); }
static void (*runner)(void (*)(void)) = runWith;

/* choose writes through the address it is given. */
static void (*chosen)(void) = first;
static void choose(void (**slot)(void)) { *slot = second; }

/* fill writes into the array it is handed. */
static void (*later[])(void) = { first };
static void fill(void (**into)(void)) { into[0] = second; }

int main(int argc, char **argv) {
    (void)argv;
    steps[argc]();
    plan[0].run();
    hook();
    runWith(first);
    runner(second);
    choose(&chosen);
    chosen();
    fill(later);
    later[0]();
    return 0;
}
`

// Only what the index sees decides a call through a slot: a function also
// used as a value, or a slot whose address or array is handed over, may hold
// what no store shows, so those calls stay unresolved with the stores as
// witnesses.
func TestIndexKeepsSlotsTheIndexCannotSeeUnresolved(t *testing.T) {
	x := indexProgram(t, map[string]string{"slots.c": slotsSource}, "c:slots.c")
	main := x.object(t, "main", "slots.c")
	steps := x.object(t, "steps", "slots.c")
	var bound []string
	for _, relation := range x.relations(programindex.RelationPassesCallback, steps.ID, "") {
		bound = append(bound, witnessKinds(relation)...)
	}
	if !reflect.DeepEqual(bound, []string{"c_function_pointer_store: first stored in variable steps", "c_function_pointer_store: second stored in variable steps"}) {
		t.Fatalf("steps bindings: %v", bound)
	}
	if run := x.one(t, programindex.RelationCalls, main.ID, "run"); run.Resolution != programindex.ResolutionExact || x.names(run.ToIDs)[0] != "first" {
		t.Fatalf("plan[0].run(): %+v", run)
	}
	row := x.one(t, programindex.RelationCalls, x.object(t, "plan", "slots.c").ID, "step")
	if arguments := row.Patterns[0].Arguments; len(arguments) != 2 || arguments[0].Keyword != "name" || arguments[0].Value != "first" || arguments[1].Keyword != "run" {
		t.Fatalf("plan row: %+v", arguments)
	}
	hooks := x.objects(t, "hook")
	if len(hooks) != 1 || hooks[0].Location.Line != lineOf(t, slotsSource, "static void (*hook)(void) = first;") {
		t.Fatalf("hook objects: %+v", hooks)
	}
	if hook := x.one(t, programindex.RelationCalls, main.ID, "hook"); hook.Resolution != programindex.ResolutionExact || x.names(hook.ToIDs)[0] != "first" {
		t.Fatalf("hook(): %+v", hook)
	}
	for _, check := range []struct {
		from, selector, witness string
	}{
		{"runWith", "job", "c_function_pointer_store: first passed to runWith"},
		{"main", "chosen", "c_function_pointer_store: first stored in variable chosen"},
		{"main", "later", "c_function_pointer_store: first stored in variable later"},
	} {
		call := x.one(t, programindex.RelationCalls, x.object(t, check.from, "slots.c").ID, check.selector)
		if call.Resolution != programindex.ResolutionUnresolved || len(call.ToIDs) != 0 || !slices.Contains(witnessKinds(call), check.witness) {
			t.Fatalf("%s(): %s %v %v", check.selector, call.Resolution, x.names(call.ToIDs), witnessKinds(call))
		}
	}
}

// A call through a function's own pointer parameter calls what its direct
// callers pass there, as Python's call of a def's own parameter does: one
// function is its exact target, two are its alternatives.
func TestIndexCallsWhatCallersPassAParameter(t *testing.T) {
	x := indexProgram(t, map[string]string{"handed.c": `static void first(void) {}
static void second(void) {}
static void once(void (*job)(void)) { job(); }
static void either(void (*job)(void)) { job(); }
int main(void) {
    once(first);
    either(first);
    either(second);
    return 0;
}
`}, "c:handed.c")
	for _, check := range []struct {
		from       string
		resolution programindex.Resolution
		to         []string
	}{
		{"once", programindex.ResolutionExact, []string{"first"}},
		{"either", programindex.ResolutionAlternatives, []string{"first", "second"}},
	} {
		call := x.one(t, programindex.RelationCalls, x.object(t, check.from, "handed.c").ID, "job")
		if call.Resolution != check.resolution || !slices.Equal(x.names(call.ToIDs), check.to) || call.Dispatch != programindex.DispatchFunctionValue {
			t.Fatalf("%s's job(): %s %v %q, want %s %v through a function value", check.from, call.Resolution, x.names(call.ToIDs), call.Dispatch, check.resolution, check.to)
		}
	}
}
