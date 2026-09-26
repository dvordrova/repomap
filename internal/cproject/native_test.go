package cproject

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompileFlagsKeepOnlyWhatChangesTheParse(t *testing.T) {
	args := strings.Fields("-c -std=c99 -pedantic -O2 -Wall -W -g -ggdb -rdynamic -D _GNU_SOURCE -DKV_SMALL=1 -I include -I../vendor -isystem /opt/x/include -iquote q " +
		"-include config.h -isysroot /sdk -arch x86_64 -arch arm64 -pthread -fno-strict-aliasing -fconserve-stack -fsigned-char -fno-builtin-malloc " +
		"-msse4.2 -mno-red-zone -MD -MF deps/x.d -D_FORTIFY_SOURCE=2 -o out/x.o src/x.c")
	kept, dropped, inputs, output, compileOnly, preprocessOnly := compileFlags(args)
	wantKept := strings.Fields("-std=c99 -D _GNU_SOURCE -DKV_SMALL=1 -I include -I../vendor -isystem /opt/x/include -iquote q -include config.h -isysroot /sdk -arch x86_64 -pthread -fsigned-char -fno-builtin-malloc -msse4.2 -D_FORTIFY_SOURCE=2")
	if !reflect.DeepEqual(kept, wantKept) {
		t.Fatalf("kept:\n got %q\nwant %q", kept, wantKept)
	}
	wantDropped := strings.Fields("-pedantic -O2 -Wall -W -g -ggdb -rdynamic -arch arm64 -fno-strict-aliasing -fconserve-stack -mno-red-zone -MD -MF deps/x.d")
	if !reflect.DeepEqual(dropped, wantDropped) {
		t.Fatalf("dropped:\n got %q\nwant %q", dropped, wantDropped)
	}
	if !reflect.DeepEqual(inputs, []string{"src/x.c"}) || output != "out/x.o" || !compileOnly || preprocessOnly {
		t.Fatalf("inputs %q output %q compile %v preprocess %v", inputs, output, compileOnly, preprocessOnly)
	}
	// The adapter's own overrides come after the build's flags, so a build
	// that turns fortification on still parses with the written names.
	args2 := clangArgs(UnitSpec{Source: "x.c", Args: kept}, Toolchain{Overrides: fortifyOff}, "-H")
	if at := slicesIndex(args2, "-D_FORTIFY_SOURCE=0"); at < slicesIndex(args2, "-D_FORTIFY_SOURCE=2") || args2[len(args2)-1] != "x.c" {
		t.Fatalf("clang arguments: %q", args2)
	}
}

func slicesIndex(values []string, value string) int {
	for i, v := range values {
		if v == value {
			return i
		}
	}
	return -1
}

func TestParseSearchListClassifiesPlatformAndPackages(t *testing.T) {
	darwin := `Apple clang version 17.0.0 (clang-1700.6.3.2)
InstalledDir: /Library/Developer/CommandLineTools/usr/bin
 "/Library/Developer/CommandLineTools/usr/bin/clang" -cc1 -triple x86_64-apple-macosx15.0.0 -isysroot /Library/Developer/CommandLineTools/SDKs/MacOSX.sdk -I/usr/local/include -x c /dev/null
#include "..." search starts here:
#include <...> search starts here:
 /usr/local/include
 /Library/Developer/CommandLineTools/usr/lib/clang/17/include
 /Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/usr/include
 /Library/Developer/CommandLineTools/usr/include
 /Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/System/Library/Frameworks (framework directory)
End of search list.
`
	list := parseSearchList(darwin)
	if list.sysroot != "/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk" || list.installed != "/Library/Developer/CommandLineTools/usr/bin" || len(list.dirs) != 5 || !list.dirs[4].Framework {
		t.Fatalf("darwin: %+v", list)
	}
	roots := platformRoots("/Library/Developer/CommandLineTools/usr/lib/clang/17", list.sysroot, list.installed)
	var platform []bool
	for _, dir := range list.dirs {
		platform = append(platform, underAny(roots, dir.Path))
	}
	if !reflect.DeepEqual(platform, []bool{false, true, true, true, true}) {
		t.Fatalf("darwin platform directories: %v", platform)
	}
	linux := `clang version 18.1.3
InstalledDir: /usr/bin
 "/usr/lib/llvm-18/bin/clang" -cc1 -triple x86_64-pc-linux-gnu -x c /dev/null
#include "..." search starts here:
#include <...> search starts here:
 /usr/lib/llvm-18/lib/clang/18/include
 /usr/local/include
 /usr/include/x86_64-linux-gnu
 /usr/include
End of search list.
`
	list = parseSearchList(linux)
	roots = platformRoots("/usr/lib/llvm-18/lib/clang/18", list.sysroot, list.installed)
	platform = nil
	for _, dir := range list.dirs {
		platform = append(platform, underAny(roots, dir.Path))
	}
	if list.sysroot != "" || !reflect.DeepEqual(platform, []bool{true, false, true, true}) {
		t.Fatalf("linux platform directories: %v %+v", platform, list)
	}
}

func TestParseIncludeTreeSeparatesDiagnostics(t *testing.T) {
	names := &fileNames{cwd: "/repo", roots: []string{"/repo"}, corpus: map[string]bool{"a.c": true, "a.h": true}, cache: map[string]fileName{}}
	stderr := strings.Join([]string{
		". ./a.h",
		".. /sdk/usr/include/stdlib.h",
		"... /sdk/usr/include/_stdlib.h",
		".. /sdk/usr/include/stdio.h",
		". /sdk/usr/include/signal.h",
		"a.c:9:1: error: unknown type name 'foo'",
		"1 error generated.",
	}, "\n")
	includes, diagnostics := parseIncludeTree(stderr, names)
	want := []Include{
		{Path: "a.h", Depth: 1, Parent: -1},
		{Path: "/sdk/usr/include/stdlib.h", Depth: 2, Parent: 0},
		{Path: "/sdk/usr/include/_stdlib.h", Depth: 3, Parent: 1},
		{Path: "/sdk/usr/include/stdio.h", Depth: 2, Parent: 0},
		{Path: "/sdk/usr/include/signal.h", Depth: 1, Parent: -1},
	}
	if !reflect.DeepEqual(includes, want) {
		t.Fatalf("includes: %+v", includes)
	}
	if len(diagnostics) != 2 || !strings.Contains(diagnostics[0], "error:") {
		t.Fatalf("diagnostics: %q", diagnostics)
	}
}

func TestPackageOfIsTheNearestHeaderTheCorpusNames(t *testing.T) {
	c := &classifier{root: "/repo", corpus: map[string]bool{"a.c": true, "a.h": true}, search: []string{"/sdk/usr/include"}, frameworks: map[string]bool{}}
	unit := &Unit{UnitSpec: UnitSpec{Path: "a.c"}, Includes: []Include{
		{Path: "a.h", Class: FileCorpus, Depth: 1, Parent: -1},
		{Path: "/sdk/usr/include/stdlib.h", Class: FilePlatform, Depth: 2, Parent: 0},
		{Path: "/sdk/usr/include/_stdlib.h", Class: FilePlatform, Depth: 3, Parent: 1},
		{Path: "/sdk/usr/include/sys/wait.h", Class: FilePlatform, Depth: 3, Parent: 1},
		{Path: "/sdk/usr/include/sys/signal.h", Class: FilePlatform, Depth: 4, Parent: 3},
		{Path: "/sdk/usr/include/arpa/inet.h", Class: FilePlatform, Depth: 1, Parent: -1},
		{Path: "/sdk/usr/include/sys/socket.h", Class: FilePlatform, Depth: 2, Parent: 5},
	}}
	// The repository's own stdio.h shadows the platform's for "stdio.h":
	// the platform stdio.h below is reached through stdlib.h.
	c.corpus["stdio.h"] = true
	unit.Includes = append(unit.Includes,
		Include{Path: "stdio.h", Class: FileCorpus, Depth: 1, Parent: -1},
		Include{Path: "/sdk/usr/include/stdio.h", Class: FilePlatform, Depth: 3, Parent: 1},
		Include{Path: "/sdk/usr/include/_stdio.h", Class: FilePlatform, Depth: 4, Parent: 8})
	directives := map[string][]directive{
		"a.c": {{line: 1, spelled: "a.h", quoted: true}, {line: 2, spelled: "arpa/inet.h"}, {line: 3, spelled: "sys/wait.h"}, {line: 4, spelled: "signal.h"}, {line: 5, spelled: "stdio.h", quoted: true}},
		"a.h": {{line: 3, spelled: "stdlib.h"}},
	}
	store := NewStore()
	store.directives = directives
	named := store.namedByCorpus(nil, unit, c)
	for file, want := range map[string]string{
		"/sdk/usr/include/_stdlib.h":    "stdlib.h",   // declared in _stdlib.h, included through stdlib.h
		"/sdk/usr/include/sys/signal.h": "sys/wait.h", // <signal.h> is not sys/signal.h
		"/sdk/usr/include/sys/socket.h": "arpa/inet.h",
		"/sdk/usr/include/stdlib.h":     "stdlib.h",
		"/sdk/usr/include/_stdio.h":     "stdlib.h",
		"/elsewhere.h":                  "",
	} {
		if got := c.packageOf(unit, named, file); got != want {
			t.Errorf("%s: package %q, want %q", file, got, want)
		}
	}
}

func TestShellCommandsFollowRecipeSyntax(t *testing.T) {
	commands, ok := shellCommands(`cd sub && CC=x ccache gcc -c -DNAME="a b" 'q.c' 2>&1 >log.txt; echo "done;" | tee x`)
	want := [][]string{{"cd", "sub"}, {"CC=x", "ccache", "gcc", "-c", "-DNAME=a b", "q.c"}, {"echo", "done;"}, {"tee", "x"}}
	if !ok || !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands: %q", commands)
	}
	if words := commandWords([]string{"@CC=x", "ccache", "gcc", "-c"}); !reflect.DeepEqual(words, []string{"gcc", "-c"}) {
		t.Fatalf("compiler words: %q", words)
	}
	// A bare archiver word is not an archive command.
	parseDryRun(parseEnv{roots: []string{"/repo"}}, "ar\nar -t\n")
	if _, ok := shellCommands(`echo "unterminated`); ok {
		t.Fatal("an unterminated quote split")
	}
	for name, want := range map[string]bool{"cc": true, "gcc-12": true, "x86_64-linux-gnu-gcc": true, "clang": true, "clang-17": true, "clang++": false, "g++": false, "ccache": false, "c99": true} {
		if compilerName.MatchString(name) != want {
			t.Errorf("compiler %s: %v", name, !want)
		}
	}
}

func TestMatchDirectivesFollowsEachParentInOrder(t *testing.T) {
	// An X-macro table entered twice by one parent: each entry is its own
	// directive.
	c := &classifier{root: "/repo", corpus: map[string]bool{"a.c": true, "ops.def": true, "a.h": true}, frameworks: map[string]bool{}}
	unit := &Unit{UnitSpec: UnitSpec{Path: "a.c"}, Includes: []Include{
		{Path: "ops.def", Class: FileCorpus, Depth: 1, Parent: -1},
		{Path: "a.h", Class: FileCorpus, Depth: 1, Parent: -1},
		{Path: "ops.def", Class: FileCorpus, Depth: 1, Parent: -1},
	}}
	store := NewStore()
	store.directives = map[string][]directive{"a.c": {
		{line: 2, spelled: "ops.def", quoted: true}, {line: 4, spelled: "a.h", quoted: true}, {line: 9, spelled: "ops.def", quoted: true},
	}}
	store.matchDirectives(nil, unit, c)
	var lines []int
	for _, include := range unit.Includes {
		lines = append(lines, include.Line)
	}
	if !reflect.DeepEqual(lines, []int{2, 4, 9}) {
		t.Fatalf("directive lines: %v", lines)
	}
}
