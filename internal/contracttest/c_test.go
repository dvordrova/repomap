package contracttest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/cproject"
)

// The cumulative C fixture is a small key-value server (kvd) and its client
// (kvcli), built by the root Makefile, plus a dump tool the Makefile does not
// build. These checks run the real clang: discovery reads `make -n -B`, and
// every unit is parsed with the build's own flags.

// cFixture is the fixture materialized once per test with its three
// programs parsed through one store, so a unit two programs link is parsed
// once.
type cFixture struct {
	root       string
	repository *corpus.Corpus
	project    *cproject.Project
	parsed     map[string]*cproject.Parsed
}

func loadCFixture(t *testing.T) cFixture {
	t.Helper()
	root, repository := materializeFixtureRepository(t, "c")
	project, err := cproject.Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if project == nil || project.Toolchain.Err != "" {
		t.Fatalf("the C fixture needs clang on PATH: %+v", project)
	}
	fixture := cFixture{root: root, repository: repository, project: project, parsed: map[string]*cproject.Parsed{}}
	store := cproject.NewStore()
	for _, program := range project.Programs {
		parsed, err := cproject.Parse(t.Context(), root, repository, program, store)
		if err != nil {
			t.Fatalf("parse %s: %v", program.Selector, err)
		}
		fixture.parsed[program.Selector] = parsed
	}
	return fixture
}

func (fixture cFixture) program(t *testing.T, selector string) cproject.Program {
	t.Helper()
	for _, program := range fixture.project.Programs {
		if program.Selector == selector {
			return program
		}
	}
	t.Fatalf("no C program %s", selector)
	return cproject.Program{}
}

func (fixture cFixture) unit(t *testing.T, selector, path string) *cproject.Unit {
	t.Helper()
	for _, unit := range fixture.parsed[selector].Units {
		if unit.Path == path {
			return unit
		}
	}
	t.Fatalf("%s has no unit %s", selector, path)
	return nil
}

func (fixture cFixture) source(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// at is the line and column of the first occurrence of needle in path, and
// of within it when within is not empty.
func (fixture cFixture) at(t *testing.T, path, needle, within string) (int, int) {
	t.Helper()
	text := string(fixture.source(t, path))
	offset := strings.Index(text, needle)
	if offset < 0 {
		t.Fatalf("%s has no %q", path, needle)
	}
	if within != "" {
		inner := strings.Index(needle, within)
		if inner < 0 {
			t.Fatalf("%q is not in %q", within, needle)
		}
		offset += inner
	}
	line := strings.Count(text[:offset], "\n") + 1
	return line, offset - strings.LastIndex(text[:offset], "\n")
}

func walkCNodes(nodes []*cproject.Node, visit func(node *cproject.Node, parents []*cproject.Node)) {
	var walk func(node *cproject.Node, parents []*cproject.Node)
	walk = func(node *cproject.Node, parents []*cproject.Node) {
		visit(node, parents)
		parents = append(parents, node)
		for _, child := range node.Inner {
			walk(child, parents)
		}
		if node.ArrayFiller != nil {
			walk(node.ArrayFiller, parents)
		}
	}
	for _, node := range nodes {
		walk(node, nil)
	}
}

// references are the DeclRefExpr nodes naming name, with their enclosing
// top-level declaration.
func cReferences(unit *cproject.Unit, name string) map[*cproject.Node]*cproject.Node {
	found := map[*cproject.Node]*cproject.Node{}
	walkCNodes(unit.Decls, func(node *cproject.Node, parents []*cproject.Node) {
		if node.Kind == "DeclRefExpr" && node.ReferencedDecl != nil && node.ReferencedDecl.Name == name && len(parents) > 0 {
			found[node] = parents[0]
		}
	})
	return found
}

func cDeclarations(unit *cproject.Unit, kind, name string) []*cproject.Node {
	var found []*cproject.Node
	for _, node := range unit.Decls {
		if node.Kind == kind && node.Name == name {
			found = append(found, node)
		}
	}
	return found
}

func cSpecPaths(units []cproject.UnitSpec) []string {
	var paths []string
	for _, unit := range units {
		paths = append(paths, unit.Path)
	}
	return paths
}

func cUnitPaths(units []*cproject.Unit) []string {
	var paths []string
	for _, unit := range units {
		paths = append(paths, unit.Path)
	}
	return paths
}

func TestCFixtureProgramsComeFromTheMakefile(t *testing.T) {
	fixture := loadCFixture(t)
	project := fixture.project
	if project.Build.Kind != cproject.BuildMake || project.Build.Path != "Makefile" || project.Build.Err != "" {
		t.Fatalf("build description: %+v", project.Build)
	}
	var selectors []string
	for _, program := range project.Programs {
		selectors = append(selectors, program.Selector)
		if err := program.ValidateAgainst(fixture.repository); err != nil {
			t.Fatal(err)
		}
	}
	// Each link line is a program; tools/dump.c, which no link line links, is
	// one through its own main. Every unit belongs to a program, so there is
	// no library.
	if !reflect.DeepEqual(selectors, []string{"c:kvcli", "c:kvd", "c:tools/dump.c"}) {
		t.Fatalf("programs: %v", selectors)
	}

	server := fixture.program(t, "c:kvd")
	serverRule, _ := fixture.at(t, "Makefile", "kvd: kvd.o", "")
	if server.Kind != cproject.ProgramExecutable || server.Closure || server.Anchor != (cproject.Site{Path: "Makefile", Line: serverRule}) ||
		!reflect.DeepEqual(cSpecPaths(server.Units), []string{"kvd.c", "loop.c", "strbuf.c"}) || !reflect.DeepEqual(server.LinkArgs, []string{"-pthread"}) {
		t.Fatalf("kvd: %+v", server)
	}
	if len(server.Evidence) != 1 || server.Evidence[0].Kind != "c_link" || server.Evidence[0].Fields["output"] != "kvd" {
		t.Fatalf("kvd evidence: %+v", server.Evidence)
	}
	client := fixture.program(t, "c:kvcli")
	clientRule, _ := fixture.at(t, "Makefile", "kvcli: kvcli.o", "")
	if client.Anchor != (cproject.Site{Path: "Makefile", Line: clientRule}) || !reflect.DeepEqual(cSpecPaths(client.Units), []string{"kvcli.c", "strbuf.c"}) {
		t.Fatalf("kvcli: %+v", client)
	}
	// Every unit keeps the flags that change what clang reads and drops
	// optimisation, debug and warnings.
	for _, unit := range slices.Concat(server.Units, client.Units) {
		if !unit.Built || !reflect.DeepEqual(unit.Args, []string{"-std=c99", "-D_DEFAULT_SOURCE", "-DLOOP_POLL"}) ||
			!reflect.DeepEqual(unit.Dropped, []string{"-O2", "-g", "-Wall"}) {
			t.Fatalf("%s flags: %+v", unit.Path, unit)
		}
	}
	// The backends are parts of loop.c, never units of their own.
	pollLine, _ := fixture.at(t, "loop.c", `#include "loop_poll.c"`, "")
	epollLine, _ := fixture.at(t, "loop.c", `#include "loop_epoll.c"`, "")
	want := []cproject.IncludedSource{
		{Path: "loop_epoll.c", Includers: []cproject.Site{{Path: "loop.c", Line: epollLine}}},
		{Path: "loop_poll.c", Includers: []cproject.Site{{Path: "loop.c", Line: pollLine}}},
	}
	if !reflect.DeepEqual(project.Included, want) {
		t.Fatalf("included sources: %+v", project.Included)
	}
	for _, unit := range project.Units {
		if strings.HasPrefix(unit.Path, "loop_") {
			t.Fatalf("an included backend became a unit: %+v", unit)
		}
	}
	dump := fixture.program(t, "c:tools/dump.c")
	dumpMain, _ := fixture.at(t, "tools/dump.c", "int main", "")
	if !dump.Closure || dump.Anchor != (cproject.Site{Path: "tools/dump.c", Line: dumpMain}) || dump.Units[0].Built || dump.Units[0].Main == nil {
		t.Fatalf("dump: %+v", dump)
	}
}

func TestCFixtureParsesEachProgramInTheBuildsView(t *testing.T) {
	fixture := loadCFixture(t)
	server, client, dump := fixture.parsed["c:kvd"], fixture.parsed["c:kvcli"], fixture.parsed["c:tools/dump.c"]
	serverMain, _ := fixture.at(t, "kvd.c", "int main", "")
	clientMain, _ := fixture.at(t, "kvcli.c", "int main", "")
	if server.Main == nil || server.Main.Unit != "kvd.c" || server.Main.At.Line != serverMain ||
		client.Main == nil || client.Main.Unit != "kvcli.c" || client.Main.At.Line != clientMain {
		t.Fatalf("mains: %+v / %+v", server.Main, client.Main)
	}
	// strbuf.c is linked into both programs and parsed once.
	if fixture.unit(t, "c:kvd", "strbuf.c") != fixture.unit(t, "c:kvcli", "strbuf.c") {
		t.Fatal("a unit linked into two programs was parsed twice")
	}
	// The build asks for poll: the epoll backend is outside this build on
	// every host, and only kvd, which links loop.c, reports it.
	if !reflect.DeepEqual(server.Outside, []string{"loop_epoll.c"}) || len(client.Outside) != 0 || len(dump.Outside) != 0 {
		t.Fatalf("outside the build: %v / %v / %v", server.Outside, client.Outside, dump.Outside)
	}
	loop := fixture.unit(t, "c:kvd", "loop.c")
	if polls := cDeclarations(loop, "FunctionDecl", "loopApiPoll"); len(polls) != 1 || polls[0].Loc.Site().File != "loop_poll.c" || polls[0].StorageClass != "static" {
		t.Fatalf("the active backend: %+v", polls)
	}
	// The linker's closure gives the dump tool strbuf.c and nothing else.
	if !reflect.DeepEqual(cUnitPaths(dump.Units), []string{"strbuf.c", "tools/dump.c"}) || dump.Main == nil || dump.Main.Unit != "tools/dump.c" {
		t.Fatalf("dump: %v main %+v", cUnitPaths(dump.Units), dump.Main)
	}

	// The command table keeps each row's name beside the function it names.
	daemon := fixture.unit(t, "c:kvd", "kvd.c")
	tables := cDeclarations(daemon, "VarDecl", "cmdTable")
	if len(tables) != 1 || tables[0].StorageClass != "static" {
		t.Fatalf("command table: %+v", tables)
	}
	getRow, getColumn := fixture.at(t, "kvd.c", `{"get", getCommand, 2}`, "getCommand")
	var rowReference *cproject.Node
	for node, top := range cReferences(daemon, "getCommand") {
		if top == tables[0] {
			rowReference = node
		}
	}
	if rowReference == nil || rowReference.Begin.Site().Line != getRow || rowReference.Begin.Site().Col != getColumn {
		t.Fatalf("the get row names getCommand at %+v", rowReference)
	}

	// A call written in a macro argument keeps its own place; the call the
	// macro body writes is found at the macro name.
	nonBlockLine, nonBlockColumn := fixture.at(t, "kvd.c", "kvAssert(setNonBlocking(cfd) == 0)", "setNonBlocking")
	_, assertColumn := fixture.at(t, "kvd.c", "kvAssert(setNonBlocking(cfd) == 0)", "")
	source := fixture.source(t, "kvd.c")
	var inArgument, inBody []cproject.Position
	for node, top := range cReferences(daemon, "setNonBlocking") {
		if top.Name == "acceptHandler" {
			if !node.Begin.MacroArg {
				t.Fatalf("setNonBlocking in kvAssert's argument: %+v", node.Begin)
			}
			inArgument = append(inArgument, node.Begin.Site())
		}
	}
	for node, top := range cReferences(daemon, "kvAssertFail") {
		if top.Name == "acceptHandler" && node.Begin.InMacroBody() {
			inBody = append(inBody, node.Begin.Site())
		}
	}
	if len(inArgument) != 1 || inArgument[0].Line != nonBlockLine || inArgument[0].Col != nonBlockColumn || cproject.TokenText(source, inArgument[0]) != "setNonBlocking" {
		t.Fatalf("setNonBlocking sites: %+v", inArgument)
	}
	if len(inBody) != 1 || inBody[0].Line != nonBlockLine || inBody[0].Col != assertColumn || cproject.TokenText(source, inBody[0]) != "kvAssert" {
		t.Fatalf("kvAssertFail sites: %+v", inBody)
	}

	// Calls nested on one line keep their own columns: the same address
	// handed to two different calls, and one of them called twice.
	const scheme = `return strncmp(host, "kvd://", strlen("kvd://")) == 0 ? host + strlen("kvd://") : host;`
	schemeLine, compare := fixture.at(t, "kvcli.c", scheme, "strncmp")
	_, measure := fixture.at(t, "kvcli.c", scheme, `strlen("kvd://")) == 0`)
	_, skip := fixture.at(t, "kvcli.c", scheme, `strlen("kvd://") : host`)
	cli := fixture.unit(t, "c:kvcli", "kvcli.c")
	cliSource := fixture.source(t, "kvcli.c")
	var calls []string
	for _, name := range []string{"strncmp", "strlen"} {
		for node, top := range cReferences(cli, name) {
			if site := node.Begin.Site(); top.Name == "withoutScheme" && site.Line == schemeLine && cproject.TokenText(cliSource, site) == name {
				calls = append(calls, fmt.Sprintf("%s@%d", name, site.Col))
			}
		}
	}
	slices.Sort(calls)
	if want := []string{fmt.Sprintf("strlen@%d", measure), fmt.Sprintf("strlen@%d", skip), fmt.Sprintf("strncmp@%d", compare)}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls on one line: %v, want %v", calls, want)
	}

	// Platform declarations are named by the header the fixture includes,
	// whichever internal header declares them on this host; fortification
	// is off, so libc calls keep their written names.
	packages := map[string]string{}
	for _, unit := range slices.Concat(server.Units, client.Units) {
		for _, declaration := range unit.External {
			if declaration.Kind == "FunctionDecl" && declaration.Class == cproject.FilePlatform {
				packages[declaration.Name] = declaration.Package
			}
		}
	}
	for name, header := range map[string]string{
		"getenv": "stdlib.h", "system": "stdlib.h", "qsort": "stdlib.h", "snprintf": "stdio.h", "memcpy": "string.h", "strncmp": "string.h",
		"pthread_create": "pthread.h", "sigaction": "signal.h", "fork": "unistd.h", "socket": "sys/socket.h", "poll": "poll.h",
	} {
		if packages[name] != header {
			t.Errorf("%s is declared for the fixture by %q, want %q", name, packages[name], header)
		}
	}
	for _, unit := range server.Units {
		walkCNodes(unit.Decls, func(node *cproject.Node, _ []*cproject.Node) {
			if node.Kind == "DeclRefExpr" && node.ReferencedDecl != nil && strings.HasPrefix(node.ReferencedDecl.Name, "__builtin___") {
				t.Errorf("%s calls the fortified %s", unit.Path, node.ReferencedDecl.Name)
			}
		})
	}

	// A header's static inline function appears in every unit including it,
	// at its one definition; same-named statics of two units stay apart.
	avail := map[string]cproject.Position{}
	oom := map[string]cproject.Position{}
	for _, unit := range server.Units {
		for _, node := range cDeclarations(unit, "FunctionDecl", "sbAvail") {
			avail[unit.Path] = node.Loc.Expansion
		}
		for _, node := range cDeclarations(unit, "FunctionDecl", "oom") {
			if node.StorageClass != "static" {
				t.Fatalf("oom in %s is not static", unit.Path)
			}
			oom[unit.Path] = node.Loc.Expansion
		}
	}
	availLine, _ := fixture.at(t, "strbuf.h", "static inline size_t sbAvail", "")
	if len(avail) != 2 || avail["kvd.c"] != avail["strbuf.c"] || avail["kvd.c"].File != "strbuf.h" || avail["kvd.c"].Line != availLine {
		t.Fatalf("sbAvail definitions: %+v", avail)
	}
	if len(oom) != 2 || oom["loop.c"].File != "loop.c" || oom["strbuf.c"].File != "strbuf.c" {
		t.Fatalf("oom definitions: %+v", oom)
	}

	// typedef struct {...} strbuf declares a record with no tag of its own.
	var anonymous int
	for _, node := range fixture.unit(t, "c:kvd", "strbuf.c").Decls {
		if node.Kind == "RecordDecl" && node.Name == "" && node.CompleteDefinition && node.Loc.Site().File == "strbuf.h" {
			anonymous++
		}
	}
	if anonymous != 1 || len(cDeclarations(fixture.unit(t, "c:kvd", "strbuf.c"), "TypedefDecl", "strbuf")) != 1 {
		t.Fatalf("anonymous strbuf records: %d", anonymous)
	}
}

// Nested in a bigger repository, the fixture's Makefile is not at the root,
// so nothing gives its flags: each main is a program whose files the linker
// closure decides, parsed with clang's defaults. loop.c then takes the host's
// own backend, and every program still parses on every host. (Under a
// testdata directory, as in repomap's own repository, its files are no units
// at all.)
func TestCFixtureParsesWithoutItsMakefile(t *testing.T) {
	isolateFixtureGitEnvironment(t)
	const prefix = "services/kv/"
	root := filepath.Join(t.TempDir(), "repository")
	copyFixtureTree(t, filepath.Join(repositoryRoot(t), "testdata", "repositories", "c"), filepath.Join(root, filepath.FromSlash(prefix)))
	runFixtureGit(t, root, "init", "--quiet")
	runFixtureGit(t, root, "add", "--all", "--")
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	project, err := cproject.Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if project == nil || project.Toolchain.Err != "" {
		t.Fatalf("the C fixture needs clang on PATH: %+v", project)
	}
	if project.Build.Kind != cproject.BuildNone {
		t.Fatalf("build description: %+v", project.Build)
	}
	for _, observation := range project.Observations {
		if observation.Kind == "c_unit_error" {
			t.Fatalf("a unit does not parse with clang's defaults: %+v", observation)
		}
	}
	mains := map[string]bool{}
	store := cproject.NewStore()
	for _, program := range project.Programs {
		parsed, err := cproject.Parse(t.Context(), root, repository, program, store)
		if err != nil {
			t.Fatalf("parse %s: %v", program.Selector, err)
		}
		if parsed.Main == nil {
			continue
		}
		mains[program.Selector] = true
		if program.Selector != "c:"+prefix+"kvd.c" {
			continue
		}
		// The host decides the backend when the build does not.
		outside := prefix + "loop_epoll.c"
		if runtime.GOOS == "linux" {
			outside = prefix + "loop_poll.c"
		}
		if units := cUnitPaths(parsed.Units); !reflect.DeepEqual(units, []string{prefix + "kvd.c", prefix + "loop.c", prefix + "strbuf.c"}) ||
			!reflect.DeepEqual(parsed.Outside, []string{outside}) {
			t.Fatalf("kvd.c without its Makefile: units %v, outside %v", units, parsed.Outside)
		}
	}
	for _, main := range []string{"kvcli.c", "kvd.c", "tools/dump.c"} {
		if !mains["c:"+prefix+main] {
			t.Fatalf("no program for %s%s: %v", prefix, main, mains)
		}
	}
}
