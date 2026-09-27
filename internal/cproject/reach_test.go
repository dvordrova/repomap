package cproject

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// unreachable are the names of the functions a program's index proves it
// never runs, sorted.
func (x indexed) unreachable() []string {
	var names []string
	for _, object := range x.index.Objects {
		if object.Unreachable {
			names = append(names, object.Name)
		}
	}
	slices.Sort(names)
	return names
}

const reachSource = `#include <stdlib.h>

static void neverCalled(void);
static void calledOnlyByNeverCalled(void) {}
static void neverCalled(void) { calledOnlyByNeverCalled(); }
static void storedOnlyByDeadCode(void) {}
static void deadStorer(void) { void (*f)(void) = storedOnlyByDeadCode; f(); }

static void castToInteger(void) {}
static void inTable(void) {}
static void compared(void) {}
static void cleanup(int *p) { (void)p; }
static void stored(void) {}
static void beforeMain(void) __attribute__((constructor));
static void beforeMain(void) {}
static void inAssembly(void) {}

static unsigned long symbols[] = { (unsigned long)castToInteger };
static void (*handlers[])(void) = { inTable };

/* The platform declares abs; its library may call this one instead. */
int abs(int x) { return x < 0 ? -x : x; }
/* Names reserved for the implementation, which alone calls them. */
int __kv_hook(void) { return 0; }
int _kv_start(void) { return 0; }

int main(void) {
    int x __attribute__((cleanup(cleanup))) = 0;
    void (*s)(void) = stored;
    if (s == compared) return 1;
    __asm__ volatile("" : : "r"(&x));
    __asm__ volatile("# _inAssembly");
    s();
    return x + (int)symbols[0] + (handlers[0] != 0);
}
`

// A C function runs only when code that runs names it, by a call or by its
// address. What main, a constructor, a file-scope initializer, the
// platform's library, the implementation or assembly can name runs; what
// only dead code names does not.
func TestIndexProvesWhatAProgramNeverRuns(t *testing.T) {
	x := indexProgram(t, map[string]string{"reach.c": reachSource}, "c:reach.c")
	want := []string{"calledOnlyByNeverCalled", "deadStorer", "neverCalled", "storedOnlyByDeadCode"}
	if got := x.unreachable(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unreachable = %v, want %v", got, want)
	}
	for _, name := range []string{"main", "castToInteger", "inTable", "compared", "cleanup", "stored", "beforeMain", "inAssembly", "abs", "__kv_hook", "_kv_start"} {
		if x.object(t, name, "reach.c").Unreachable {
			t.Errorf("%s runs, yet the index says nothing reaches it", name)
		}
	}
}

// A static inline function in a header is one object whichever unit includes
// it, but each unit's copy calls that unit's own static functions: every
// copy is read.
func TestIndexReadsEveryUnitsCopyOfASharedStatic(t *testing.T) {
	header := "static void helper(void);\nstatic inline void step(void) { helper(); }\nvoid second(void);\n"
	x := indexProgram(t, map[string]string{
		"Makefile": "all: prog\nprog: a.o b.o\n\t$(CC) -o prog a.o b.o\n",
		"step.h":   header,
		"a.c":      "#include \"step.h\"\nstatic void helper(void) {}\nint main(void) { step(); second(); return 0; }\n",
		"b.c":      "#include \"step.h\"\nstatic void helper(void) {}\nvoid second(void) { step(); }\n",
	}, "c:prog")
	if got := x.unreachable(); len(got) != 0 {
		t.Fatalf("unreachable = %v, want none: b.c's helper runs through b.c's copy of step", got)
	}
	if helpers := x.objects(t, "helper"); len(helpers) != 2 {
		t.Fatalf("helpers = %+v, want one per unit", helpers)
	}
}

// Code the adapter does not read can call any function with external
// linkage by its name: code loaded or looked up at run time, a library other
// than the C runtime's own, and a link input no compile line built. Then only
// static functions are proven. A link line that names another entry proves
// nothing, and a library, called from outside, marks nothing.
func TestIndexProvesOnlyStaticsWhereOtherCodeCanCallByName(t *testing.T) {
	const source = "#include <dlfcn.h>\n" +
		"void orphan(void) {}\n" +
		"static void hidden(void) {}\n" +
		"int main(void) { return 0; }\n"
	lookup := indexProgram(t, map[string]string{"lookup.c": strings.Replace(source, "return 0;", "return dlsym(RTLD_DEFAULT, \"orphan\") != 0;", 1)}, "c:lookup.c")
	if got := lookup.unreachable(); !reflect.DeepEqual(got, []string{"hidden"}) {
		t.Fatalf("a program calling dlsym marks %v unreachable, want only the static hidden", got)
	}
	makefile := func(link string) map[string]string {
		return map[string]string{
			"Makefile": "all: prog\nprog: prog.o\n\t$(CC) -o prog prog.o " + link + "\n",
			"prog.c":   source,
		}
	}
	for _, test := range []struct {
		link string
		want []string
	}{
		{"-lm -pthread", []string{"hidden", "orphan"}},
		{"-lfoo", []string{"hidden"}},
		{"vendor/prebuilt.o", []string{"hidden"}},
		{"start.S", []string{"hidden"}},
		{"gen/generated.c", []string{"hidden"}},
		{"-e orphan", nil},
		{"-Wl,-e,orphan", nil},
		{"-Xlinker --entry=orphan", nil},
		{"-nostartfiles", nil},
	} {
		if got := indexProgram(t, makefile(test.link), "c:prog").unreachable(); !reflect.DeepEqual(got, test.want) {
			t.Errorf("linked with %s: unreachable = %v, want %v", test.link, got, test.want)
		}
	}
	library := indexProgram(t, map[string]string{"lib/util.c": "static int twice(int x) { return 2 * x; }\nint unused(void) { return 0; }\n"}, "c:lib/")
	if library.result.Program.Kind != ProgramLibrary {
		t.Fatalf("lib/ is a %s", library.result.Program.Kind)
	}
	if got := library.unreachable(); len(got) != 0 {
		t.Fatalf("a library marks %v unreachable", got)
	}
}
