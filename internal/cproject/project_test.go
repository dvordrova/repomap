package cproject

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
)

// writeRepository writes files under a new temporary root. The root keeps
// its symlinked spelling (/var on macOS) so both spellings are exercised.
func writeRepository(t *testing.T, files map[string]string) (string, *corpus.Corpus) {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return root, repository
}

func lineOf(t *testing.T, content, needle string) int {
	t.Helper()
	at := strings.Index(content, needle)
	if at < 0 {
		t.Fatalf("%q not found", needle)
	}
	return strings.Count(content[:at], "\n") + 1
}

func programBySelector(t *testing.T, project *Project, selector string) Program {
	t.Helper()
	for _, program := range project.Programs {
		if program.Selector == selector {
			return program
		}
	}
	t.Fatalf("no program %s in %+v", selector, project.Programs)
	return Program{}
}

func unitPaths[T interface{ ~[]UnitSpec | ~[]*Unit }](units T) []string {
	var paths []string
	switch values := any(units).(type) {
	case []UnitSpec:
		for _, unit := range values {
			paths = append(paths, unit.Path)
		}
	case []*Unit:
		for _, unit := range values {
			paths = append(paths, unit.Path)
		}
	}
	return paths
}

func walkNodes(nodes []*Node, visit func(*Node)) {
	for _, node := range nodes {
		visit(node)
		walkNodes(node.Inner, visit)
		if node.ArrayFiller != nil {
			walkNodes([]*Node{node.ArrayFiller}, visit)
		}
	}
}

const kvMakefile = `# Two programs share strbuf.o; loop.c picks its backend with KV_SMALL.
CFLAGS = -std=c99 -O2 -Wall -DKV_SMALL -fno-strict-aliasing -fconserve-stack -D_FORTIFY_SOURCE=2

all: kvd kvcli

kvd: kvd.o strbuf.o loop.o
	$(CC) -o kvd kvd.o strbuf.o loop.o -lm -pthread

kvcli: kvcli.o strbuf.o
	$(CC) -o kvcli kvcli.o strbuf.o

.c.o:
	$(CC) -c $(CFLAGS) $<

test:
	./kvd --self-test
`

const kvHeader = `#ifndef KV_H
#define KV_H
void kv_fail(const char *expr);
#define KV_ASSERT(e) ((e) ? (void)0 : kv_fail(#e))
static inline int kv_shared(void) { return 1; }
#endif
`

const kvServer = `#include <stdio.h>
#include <stdlib.h>
#include "kv.h"
#include "strbuf.h"

int loop_run(void);

static int compare(const void *a, const void *b) { return *(const int *)a - *(const int *)b; }
static int work(int x) { return x * 2; }

int main(int argc, char **argv) {
    int values[3] = {3, 1, 2};
    char out[32];
    (void)argv;
    KV_ASSERT(work(argc) > 0);
    qsort(values, 3, sizeof values[0], compare);
    snprintf(out, sizeof out, "%d", values[0]);
    return sb_len(out) + kv_shared() + loop_run();
}
`

func kvFiles() map[string]string {
	return map[string]string{
		"Makefile": kvMakefile,
		"kv.h":     kvHeader,
		"kvd.c":    kvServer,
		"strbuf.h": "int sb_len(const char *s);\n",
		"strbuf.c": "#include <string.h>\n#include \"strbuf.h\"\n#include \"kv.h\"\n\nint sb_len(const char *s) { return (int)strlen(s); }\nvoid kv_fail(const char *expr) { (void)expr; }\n",
		"loop.c":   "#include \"kv.h\"\n#ifdef KV_SMALL\n#include \"loop_small.c\"\n#else\n#include \"loop_large.c\"\n#endif\n\nint loop_run(void) { return backend_poll(); }\n",
		// loop_large.c would not even parse: only the active backend is read.
		"loop_small.c": "static int backend_poll(void) { return 1; }\n",
		"loop_large.c": "#include <sys/epoll_only_on_linux_or_nowhere.h>\nstatic int backend_poll(void) { return 2; }\n",
		"kvcli.c":      "#include \"strbuf.h\"\n\nint main(void) { return sb_len(\"x\"); }\n",
		"tools/dump.c": "#include \"../strbuf.h\"\n\nint main(void) { return sb_len(\"dump\"); }\n",
		"lib/extra.c":  "int extra_value(void) { return 7; }\n",
		"lib/unused.c": "int remainder_main(void) { return 0; }\nint main(void);\n",
		"README.md":    "# kv\n",
	}
}

func TestDiscoverProgramsFromMakeLinkLines(t *testing.T) {
	// A caller's make (make test) must not turn the dry run into a question.
	t.Setenv("MAKEFLAGS", "q")
	files := kvFiles()
	root, repository := writeRepository(t, files)
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if project.Toolchain.Err != "" {
		t.Fatalf("clang is required: %s", project.Toolchain.Err)
	}
	if project.Build.Kind != BuildMake || project.Build.Path != "Makefile" || project.Build.Err != "" {
		t.Fatalf("build: %+v", project.Build)
	}
	var selectors []string
	for _, program := range project.Programs {
		selectors = append(selectors, program.Selector)
		if err := program.ValidateAgainst(repository); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(selectors, []string{"c:kvcli", "c:kvd", "c:lib/", "c:tools/dump.c"}) {
		t.Fatalf("programs: %v", selectors)
	}
	server := programBySelector(t, project, "c:kvd")
	if server.Kind != ProgramExecutable || server.Closure || server.Anchor != (Site{Path: "Makefile", Line: lineOf(t, kvMakefile, "kvd: kvd.o")}) ||
		!reflect.DeepEqual(unitPaths(server.Units), []string{"kvd.c", "loop.c", "strbuf.c"}) || !reflect.DeepEqual(server.LinkArgs, []string{"-lm", "-pthread"}) {
		t.Fatalf("kvd: %+v", server)
	}
	if evidence := server.Evidence; len(evidence) != 1 || evidence[0].Kind != "c_link" || evidence[0].Fields["output"] != "kvd" || !reflect.DeepEqual(evidence[0].Values, []string{"kvd.c", "loop.c", "strbuf.c"}) {
		t.Fatalf("kvd evidence: %+v", server.Evidence)
	}
	// Flags: the build's language view, not its warnings, optimisation or
	// gcc-only options (clang rejects -fconserve-stack outright).
	unit := server.Units[0]
	if !reflect.DeepEqual(unit.Args, []string{"-std=c99", "-DKV_SMALL", "-D_FORTIFY_SOURCE=2"}) || !unit.Built || unit.Main != nil ||
		!reflect.DeepEqual(unit.Dropped, []string{"-O2", "-Wall", "-fno-strict-aliasing", "-fconserve-stack"}) {
		t.Fatalf("kvd.c flags: %+v", unit)
	}
	// Included backends belong to loop.c and are never units.
	if len(project.Included) != 2 || project.Included[0].Path != "loop_large.c" || project.Included[1].Includers[0] != (Site{Path: "loop.c", Line: 3}) {
		t.Fatalf("included: %+v", project.Included)
	}
	for _, spec := range project.Units {
		if strings.HasPrefix(spec.Path, "loop_") {
			t.Fatalf("an included .c file became a unit: %+v", spec)
		}
	}
	// No link line builds tools/dump.c: its exact main makes a closure program.
	dump := programBySelector(t, project, "c:tools/dump.c")
	if !dump.Closure || dump.Anchor != (Site{Path: "tools/dump.c", Line: 3}) || dump.Units[0].Built || dump.Units[0].Main == nil {
		t.Fatalf("dump: %+v", dump)
	}
	// Neither remainder_main nor a prototype of main is a main.
	library := programBySelector(t, project, "c:lib/")
	if library.Kind != ProgramLibrary || !reflect.DeepEqual(unitPaths(library.Units), []string{"lib/extra.c", "lib/unused.c"}) {
		t.Fatalf("library: %+v", library)
	}
}

func TestParseProgramsShareUnitsAndKeepWrittenSites(t *testing.T) {
	files := kvFiles()
	root, repository := writeRepository(t, files)
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	server, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:kvd"), store)
	if err != nil {
		t.Fatal(err)
	}
	client, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:kvcli"), store)
	if err != nil {
		t.Fatal(err)
	}
	if server.Main == nil || server.Main.Unit != "kvd.c" || server.Main.At.Line != lineOf(t, kvServer, "int main") || client.Main == nil || client.Main.Unit != "kvcli.c" {
		t.Fatalf("mains: %+v %+v", server.Main, client.Main)
	}
	if server.Units[2] != client.Units[1] || server.Units[2].Path != "strbuf.c" {
		t.Fatal("a unit linked into two programs was parsed twice")
	}
	// The inactive backend is outside this build, and only where its
	// includer is part of the program.
	if !reflect.DeepEqual(server.Outside, []string{"loop_large.c"}) || len(client.Outside) != 0 {
		t.Fatalf("outside: %v / %v", server.Outside, client.Outside)
	}
	loop := server.Units[1]
	var backend *Node
	for _, node := range loop.Decls {
		if node.Name == "backend_poll" {
			backend = node
		}
	}
	if backend == nil || backend.Loc.Site().File != "loop_small.c" || backend.StorageClass != "static" {
		t.Fatalf("included .c declaration: %+v", backend)
	}

	daemon := server.Units[0]
	source := []byte(files["kvd.c"])
	assertLine := lineOf(t, kvServer, "KV_ASSERT(work")
	var names []string
	var work, fail *Node
	walkNodes(daemon.Decls, func(node *Node) {
		if node.Kind != "DeclRefExpr" || node.ReferencedDecl == nil {
			return
		}
		names = append(names, node.ReferencedDecl.Name)
		switch node.ReferencedDecl.Name {
		case "work":
			work = node
		case "kv_fail":
			fail = node
		}
		// Every site outside a macro body shows the name it refers to.
		if site := node.Begin.Site(); site.File == "kvd.c" && !node.Begin.InMacroBody() {
			if got := TokenText(source, site); got != node.ReferencedDecl.Name {
				t.Errorf("%s:%d:%d reads %q, refers to %s", site.File, site.Line, site.Col, got, node.ReferencedDecl.Name)
			}
		}
	})
	// Fortification is off: libc calls keep their written names.
	if !contains(names, "snprintf") || containsPrefix(names, "__builtin___") {
		t.Fatalf("called names: %v", names)
	}
	// A call written in a macro argument keeps its own column; the macro
	// body's call is found at the macro name.
	if work == nil || !work.Begin.MacroArg || work.Begin.Site() != work.Begin.Spelling || work.Begin.Site().Line != assertLine || TokenText(source, work.Begin.Site()) != "work" {
		t.Fatalf("work: %+v", work)
	}
	if fail == nil || !fail.Begin.InMacroBody() || fail.Begin.Spelling.File != "kv.h" || TokenText(source, fail.Begin.Site()) != "KV_ASSERT" {
		t.Fatalf("kv_fail: %+v", fail)
	}
	// Platform declarations are named by the header the repository includes.
	var qsort *ExternalDecl
	for i := range daemon.External {
		if daemon.External[i].Name == "qsort" && daemon.External[i].Kind == "FunctionDecl" {
			qsort = &daemon.External[i]
		}
	}
	if qsort == nil || qsort.Package != "stdlib.h" || qsort.Class != FilePlatform || !filepath.IsAbs(qsort.Position.File) {
		t.Fatalf("qsort: %+v", qsort)
	}
	// The include tree carries the directive that entered each file.
	var header Include
	for _, include := range daemon.Includes {
		if include.Path == "kv.h" {
			header = include
		}
	}
	if header != (Include{Path: "kv.h", Class: FileCorpus, Depth: 1, Parent: -1, Line: 3, Spelled: "kv.h"}) {
		t.Fatalf("kv.h include: %+v", header)
	}
	// A header's static inline definition appears in every unit including
	// it at one definition location.
	shared := map[string]Position{}
	for _, unit := range server.Units {
		for _, node := range unit.Decls {
			if node.Name == "kv_shared" && hasBody(node) {
				shared[unit.Path] = node.Loc.Expansion
			}
		}
	}
	if len(shared) != 3 || shared["kvd.c"] != shared["strbuf.c"] || shared["kvd.c"].File != "kv.h" {
		t.Fatalf("header static definitions: %+v", shared)
	}
	if !contains(daemon.Command, "-D_FORTIFY_SOURCE=0") || daemon.JSONBytes == 0 {
		t.Fatalf("command: %q", daemon.Command)
	}
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func containsPrefix(values []string, prefix string) bool {
	for _, v := range values {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}

func TestParseClosureTakesWhatTheLinkerWould(t *testing.T) {
	root, repository := writeRepository(t, kvFiles())
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	dump, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:tools/dump.c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unitPaths(dump.Units), []string{"strbuf.c", "tools/dump.c"}) || dump.Main == nil || dump.Main.Unit != "tools/dump.c" {
		t.Fatalf("closure: %v main %+v", unitPaths(dump.Units), dump.Main)
	}
	library, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:lib/"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if library.Main != nil || len(library.Units) != 2 {
		t.Fatalf("library: %+v", library)
	}
}

func TestDiscoverWithoutBuildDescription(t *testing.T) {
	files := map[string]string{
		"app/main.c":    "int helper(void);\nextern int counter;\n\nint main(void) { return helper() + counter; }\n",
		"app/helper.c":  "int counter = 1;\nint helper(void) { return 2; }\n",
		"app/other.c":   "static int main(void) { return 0; }\nint other(void) { return 3; }\n",
		"clash/main.c":  "int twice(void);\nint main(void) { return twice(); }\n",
		"clash/one.c":   "int twice(void) { return 1; }\n",
		"clash/two.c":   "int twice(void) { return 2; }\n",
		"broken/main.c": "int main(void) { return missing_type x; }\n",
		// Host-specific units that do not parse here: one is never needed,
		// the other defines what need/main.c uses.
		"app/win32.c": "#include <windows_only_header.h>\nint win_only(void) { return 0; }\n",
		"need/main.c": "int needed(void);\nint main(void) { return needed(); }\n",
		"need/impl.c": "#include <windows_only_header.h>\nint needed(void) { return 1; }\n",
	}
	root, repository := writeRepository(t, files)
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if project.Build.Kind != BuildNone || project.Build.Err != "" {
		t.Fatalf("build: %+v", project.Build)
	}
	var selectors []string
	for _, program := range project.Programs {
		selectors = append(selectors, program.Selector)
	}
	// A static main is not a program; its unit joins its directory's library.
	if !reflect.DeepEqual(selectors, []string{"c:app/", "c:app/main.c", "c:broken/main.c", "c:clash/", "c:clash/main.c", "c:need/", "c:need/main.c"}) {
		t.Fatalf("programs: %v", selectors)
	}
	store := NewStore()
	app, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:app/main.c"), store)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unitPaths(app.Units), []string{"app/helper.c", "app/main.c"}) {
		t.Fatalf("closure took %v", unitPaths(app.Units))
	}
	_, err = Parse(t.Context(), root, repository, programBySelector(t, project, "c:need/main.c"), store)
	if err == nil || !strings.Contains(err.Error(), "need/impl.c") || !strings.Contains(err.Error(), "it may define needed") {
		t.Fatalf("a closure that needs a unit clang cannot parse: %v", err)
	}
	if _, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:app/"), store); err == nil || !strings.Contains(err.Error(), "app/win32.c") {
		t.Fatalf("a library with a unit clang cannot parse: %v", err)
	}
	if _, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:clash/main.c"), nil); err == nil || !strings.Contains(err.Error(), "twice is defined in several units: clash/one.c, clash/two.c") {
		t.Fatalf("two definitions of a needed name: %v", err)
	}
	// A unit clang cannot parse fails its program with clang's words.
	var observed bool
	for _, observation := range project.Observations {
		observed = observed || observation.Kind == "c_unit_error" && observation.Path == "broken/main.c"
	}
	_, err = Parse(t.Context(), root, repository, programBySelector(t, project, "c:broken/main.c"), nil)
	if !observed || err == nil || !strings.Contains(err.Error(), "broken/main.c") || !strings.Contains(err.Error(), "error:") {
		t.Fatalf("broken unit: observed %v, %v", observed, err)
	}
}

func TestFailedDryRunUsesDefaultsAndExplainsBoth(t *testing.T) {
	files := map[string]string{
		"Makefile": "$(error the dry run cannot read this makefile)\n",
		// The build would have passed -DCONFIGURED=1.
		"main.c": "int main(void) { return CONFIGURED; }\n",
	}
	root, repository := writeRepository(t, files)
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if project.Build.Kind != BuildNone || project.Build.Path != "Makefile" || !strings.Contains(project.Build.Err, "the dry run cannot read this makefile") {
		t.Fatalf("build: %+v", project.Build)
	}
	program := programBySelector(t, project, "c:main.c")
	if program.BuildErr == "" {
		t.Fatalf("program lost the build failure: %+v", program)
	}
	_, err = Parse(t.Context(), root, repository, program, nil)
	if err == nil || !strings.Contains(err.Error(), "CONFIGURED") || !strings.Contains(err.Error(), "the dry run cannot read this makefile") {
		t.Fatalf("parse error: %v", err)
	}
}

func TestDryRunThatHangsIsStopped(t *testing.T) {
	saved := dryRunTimeout
	dryRunTimeout = 300 * time.Millisecond
	t.Cleanup(func() { dryRunTimeout = saved })
	root, repository := writeRepository(t, map[string]string{
		"Makefile": "SLOW := $(shell sleep 30)\nall:\n\tcc -o app main.c\n",
		"main.c":   "int main(void) { return 0; }\n",
	})
	started := time.Now()
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second || !strings.Contains(project.Build.Err, "did not finish") {
		t.Fatalf("after %s: %+v", elapsed, project.Build)
	}
	if programBySelector(t, project, "c:main.c").BuildErr == "" {
		t.Fatal("the default-flag program lost the dry run failure")
	}
	// The $(shell sleep) child is stopped with make: otherwise it holds the
	// output pipe until cmd.WaitDelay gives up on it.
	if runtime.GOOS == "windows" {
		return
	}
	started = time.Now()
	if _, err := dryRun(t.Context(), root, []string{"make", "-n", "-B", "-w"}); err == nil {
		t.Fatal("a hanging dry run succeeded")
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("the dry run's children outlived it: stopped after %s", elapsed)
	}
}

func TestDryRunKeepsTheRootMakefile(t *testing.T) {
	// make remakes a makefile it reads for real even under -n, and -B makes
	// it out of date: a configured autotools tree would regenerate itself.
	root, repository := writeRepository(t, map[string]string{
		"Makefile":    "all: app\napp: main.o\n\tcc -o app main.o\nMakefile: Makefile.in\n\techo regenerated > regenerated.txt\n",
		"Makefile.in": "",
		"main.c":      "int main(void) { return 0; }\n",
	})
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "regenerated.txt")); err == nil {
		t.Fatal("the dry run remade the Makefile")
	}
	if project.Build.Kind != BuildMake || !reflect.DeepEqual(project.Build.Command, []string{"make", "-n", "-B", "-w", "-o", "Makefile"}) {
		t.Fatalf("build: %+v", project.Build)
	}
	if app := programBySelector(t, project, "c:app"); !reflect.DeepEqual(unitPaths(app.Units), []string{"main.c"}) {
		t.Fatalf("app: %+v", app)
	}
}

func TestParseClosureFollowsImplicitAndBlockScopeDeclarations(t *testing.T) {
	root, repository := writeRepository(t, map[string]string{
		// Pre-C99 code: clang 16 and later make implicit int, implicit
		// declarations and int/pointer conversions errors by default, -w
		// alone does not silence them, and gcc before 14 builds them.
		"old/main.c":   "main()\n{\n    return helper(2);\n}\n",
		"old/helper.c": "int helper(x)\nint x;\n{\n    char *p = x;\n    return p != 0;\n}\nnothing() { return; }\n",
		// Declarations inside a function body name external definitions; a
		// tentative definition is one; same-named statics never clash.
		"block/main.c":  "int main(void) {\n    int scoped(void);\n    extern int shared_count;\n    return scoped() + shared_count;\n}\n",
		"block/impl.c":  "static int local(void) { return 1; }\nint scoped(void) { return local(); }\n",
		"block/count.c": "static int local(void) { return 2; }\nint shared_count;\n",
	})
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Observations) != 0 {
		t.Fatalf("observations: %+v", project.Observations)
	}
	store := NewStore()
	old, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:old/main.c"), store)
	if err != nil {
		t.Fatal(err)
	}
	// clang does not dump the implicit declaration of helper; the call
	// still leads the closure to the unit defining it.
	if !reflect.DeepEqual(unitPaths(old.Units), []string{"old/helper.c", "old/main.c"}) || old.Main == nil || old.Main.At.Line != 1 {
		t.Fatalf("old: %v main %+v", unitPaths(old.Units), old.Main)
	}
	if !contains(old.Toolchain.Overrides, "-Wno-error=implicit-function-declaration") {
		t.Fatalf("overrides: %q", old.Toolchain.Overrides)
	}
	block, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:block/main.c"), store)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unitPaths(block.Units), []string{"block/count.c", "block/impl.c", "block/main.c"}) {
		t.Fatalf("block: %v", unitPaths(block.Units))
	}
}

func TestParseRefusesALinkWithTwoMains(t *testing.T) {
	root, repository := writeRepository(t, map[string]string{
		"Makefile": "app: a.o b.o\n\tcc -o app a.o b.o\n",
		"a.c":      "int main(void) { return 0; }\n",
		"b.c":      "int main(void) { return 1; }\n",
	})
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:app"), nil); err == nil || !strings.Contains(err.Error(), "defines main twice: a.c:1 and b.c:1") {
		t.Fatalf("two mains: %v", err)
	}
}

func TestMacroSitesFollowWhereTheReaderWroteTheCall(t *testing.T) {
	app := "#include \"check.h\"\n\nstatic int work(int x) { return x; }\n#define RUN(x) CHECK(work(x))\n\nint main(void) {\n    CHECK(work(1));\n    RUN(2);\n    return 0;\n}\n"
	root, repository := writeRepository(t, map[string]string{
		"check.h": "void check_fail(void);\n#define CHECK(e) ((e) ? (void)0 : check_fail())\n",
		"app.c":   app,
	})
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(t.Context(), root, repository, programBySelector(t, project, "c:app.c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var sites []string
	walkNodes(parsed.Units[0].Decls, func(node *Node) {
		if node.Kind == "DeclRefExpr" && node.ReferencedDecl != nil && node.ReferencedDecl.Name == "work" {
			site := node.Begin.Site()
			sites = append(sites, fmt.Sprintf("%s:%d %s macro_arg=%v", site.File, site.Line, TokenText([]byte(app), site), node.Begin.MacroArg))
		}
	})
	// work written in CHECK's argument keeps its own place; work written in
	// RUN's body, above in the same file, is found where RUN is used.
	want := []string{
		fmt.Sprintf("app.c:%d work macro_arg=true", lineOf(t, app, "CHECK(work(1))")),
		fmt.Sprintf("app.c:%d RUN macro_arg=true", lineOf(t, app, "RUN(2)")),
	}
	if !reflect.DeepEqual(sites, want) {
		t.Fatalf("work sites:\n got %q\nwant %q", sites, want)
	}
}

func TestCompileCommandsGiveFlagsAndMainsGivePrograms(t *testing.T) {
	root, repository := writeRepository(t, map[string]string{
		"src/main.c":  "int value(void);\nint main(void) { return value() + LEVEL; }\n",
		"src/value.c": "int value(void) { return 1; }\n",
		// A Makefile is ignored when compile_commands.json exists.
		"Makefile": "$(error not read)\n",
	})
	commands := `[
 {"directory": "` + root + `/build", "file": "../src/main.c", "arguments": ["cc", "-DLEVEL=2", "-O3", "-c", "../src/main.c", "-o", "main.o"]},
 {"directory": "` + root + `", "file": "src/value.c", "command": "gcc -Wall -I include -c src/value.c"}
]`
	if err := os.MkdirAll(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compile_commands.json"), []byte(commands), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if project.Build.Kind != BuildCompileCommands || len(project.Units) != 2 {
		t.Fatalf("build %+v units %+v", project.Build, project.Units)
	}
	main := project.Units[0]
	if main.Path != "src/main.c" || main.Dir != "build" || main.Source != "../src/main.c" || !reflect.DeepEqual(main.Args, []string{"-DLEVEL=2"}) || main.Main == nil {
		t.Fatalf("main unit: %+v", main)
	}
	program := programBySelector(t, project, "c:src/main.c")
	parsed, err := Parse(t.Context(), root, repository, program, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unitPaths(parsed.Units), []string{"src/main.c", "src/value.c"}) || parsed.Main == nil || parsed.Main.At.File != "src/main.c" {
		t.Fatalf("parsed: %v %+v", unitPaths(parsed.Units), parsed.Main)
	}
}

func TestParseDryRunFollowsDirectoriesAndArchives(t *testing.T) {
	files := map[string]string{"src/a.c": "", "src/b.c": "", "lib/c.c": "", "gen/d.c": ""}
	root, repository := writeRepository(t, files)
	env := parseEnv{root: root, roots: rootsOf(root), repository: repository, corpus: map[string]bool{"src/a.c": true, "src/b.c": true, "lib/c.c": true, "gen/d.c": true}}
	output := strings.Join([]string{
		"make: Entering directory `" + root + "'",
		"make -C lib",
		"make[1]: Entering directory '" + filepath.Join(root, "lib") + "'",
		// Older automake wraps the compile in if ...; then ...; fi.
		"if cc -c -Iinc c.c -o c.o; then mv -f .deps/c.Tpo .deps/c.Po; else rm -f .deps/c.Tpo; exit 1; fi",
		"ar -rcs libc.a c.o",
		"ranlib libc.a",
		"make[1]: Leaving directory '" + filepath.Join(root, "lib") + "'",
		"cd src && ccache cc -c -DA \\",
		"  a.c",
		"echo cc -c ignored.c",
		"cd src && cc -o ../bin/tool a.o b.c ../lib/libc.a prebuilt.o -lz",
		"cc -shared -o libgen.so gen/d.c",
		"make: Leaving directory `" + root + "'",
	}, "\n")
	description := parseDryRun(env, output)
	programs, linked, observations := linkPrograms(env, description)
	if len(observations) != 0 || len(programs) != 2 {
		t.Fatalf("programs %+v observations %+v", programs, observations)
	}
	tool, shared := programs[0], programs[1]
	if tool.Selector != "c:bin/tool" || !reflect.DeepEqual(unitPaths(tool.Units), []string{"lib/c.c", "src/a.c", "src/b.c"}) ||
		!reflect.DeepEqual(tool.Missing, []string{"src/prebuilt.o"}) || !reflect.DeepEqual(tool.LinkArgs, []string{"-lz"}) {
		t.Fatalf("tool: %+v", tool)
	}
	if tool.Units[0].Dir != "lib" || !reflect.DeepEqual(tool.Units[0].Args, []string{"-Iinc"}) || tool.Units[1].Dir != "src" || !reflect.DeepEqual(tool.Units[1].Args, []string{"-DA"}) {
		t.Fatalf("unit directories and flags: %+v", tool.Units)
	}
	if shared.Selector != "c:libgen.so" || shared.Kind != ProgramShared || len(linked) != 4 {
		t.Fatalf("shared: %+v linked %v", shared, linked)
	}
}

// TestRealRepositoryProbe reads a real C repository when
// REPOMAP_C_PROBE names it, checking every call site and function name
// against the source text.
func TestRealRepositoryProbe(t *testing.T) {
	root := os.Getenv("REPOMAP_C_PROBE")
	if root == "" {
		t.Skip("optional real repository probe")
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	project, err := Discover(t.Context(), root, repository)
	if err != nil || project == nil {
		t.Fatalf("discover: %v", err)
	}
	t.Logf("build %+v, %d units, observations %+v", project.Build, len(project.Units), project.Observations)
	store := NewStore()
	checked := 0
	for _, program := range project.Programs {
		parsed, err := Parse(t.Context(), root, repository, program, store)
		if err != nil {
			t.Fatalf("%s: %v", program.Selector, err)
		}
		t.Logf("%s: %d units, main %+v, outside %v", program.Selector, len(parsed.Units), parsed.Main, parsed.Outside)
		for _, unit := range parsed.Units {
			sources := map[string][]byte{}
			walkNodes(unit.Decls, func(node *Node) {
				site, want := node.Begin.Site(), ""
				switch {
				case node.Kind == "DeclRefExpr" && node.ReferencedDecl != nil && !node.Begin.InMacroBody():
					want = node.ReferencedDecl.Name
				case node.Kind == "FunctionDecl" && !node.Loc.FromMacro():
					site, want = node.Loc.Site(), node.Name
				default:
					return
				}
				if site.File == "" || filepath.IsAbs(site.File) || strings.HasPrefix(site.File, "<") {
					return
				}
				if _, ok := sources[site.File]; !ok {
					sources[site.File], _ = os.ReadFile(filepath.Join(root, site.File))
				}
				source := sources[site.File]
				checked++
				if got := TokenText(source, site); got != want || bytes.Count(source[:site.Offset], []byte("\n"))+1 != site.Line {
					t.Errorf("%s:%d:%d reads %q, want %q", site.File, site.Line, site.Col, got, want)
				}
			})
		}
	}
	t.Logf("checked %d sites against the source", checked)
}
