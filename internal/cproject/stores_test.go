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
	if err := os.WriteFile(filepath.Join(packages, "pkg", "pkg.h"), []byte("int pkg_run(const char *name, void (*job)(void));\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Makefile": "CFLAGS = -I" + packages + "\napp: app.o\n\t$(CC) -o app app.o -lpkg\n.c.o:\n\t$(CC) -c $(CFLAGS) $<\n",
		"app.c":    "#include <stdio.h>\n#include <pkg/pkg.h>\n\nstatic void job(void) { puts(\"job\"); }\n\nint main(void) { return pkg_run(\"nightly\", job); }\n",
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
	layer, _ := buildPlaces(t, x.repository, x.index)
	var registrations []string
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		registrations = append(registrations, fact.Key+" "+fact.Symbol+" "+fact.Text)
	}
	if !slices.Equal(registrations, []string{"pkg_run job pkg/pkg.h.pkg_run"}) {
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
