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
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
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
	// one through its own main. Every root unit belongs to a program, so the
	// root has no library. upper/, util/ and wire/ have makefiles of their
	// own (the test below).
	if !reflect.DeepEqual(selectors, []string{"c:kvcli", "c:kvd", "c:tools/dump.c", "c:upper/upper.so", "c:util/", "c:util/ping.c", "c:util/watch.c",
		"c:wire/", "c:wire/libwire.a", "c:wire/selftest.c", "c:wire/wirecat"}) {
		t.Fatalf("programs: %v", selectors)
	}

	server := fixture.program(t, "c:kvd")
	serverRule, _ := fixture.at(t, "build/main.mk", "kvd: kvd.o", "")
	manifestRef, _ := fixture.repository.ID("Makefile")
	fragmentRef, _ := fixture.repository.ID("build/main.mk")
	if server.Kind != cproject.ProgramExecutable || server.Closure || server.Anchor != (cproject.Site{Path: "build/main.mk", Line: serverRule}) ||
		server.AnchorFileRef != string(fragmentRef) || server.Manifest != "Makefile" || server.ManifestFileRef != string(manifestRef) ||
		!reflect.DeepEqual(cSpecPaths(server.Units), []string{"kvd.c", "loop.c", "net.c", "strbuf.c"}) || !reflect.DeepEqual(server.LinkArgs, []string{"-pthread"}) {
		t.Fatalf("kvd: %+v", server)
	}
	if len(server.Evidence) != 1 || server.Evidence[0].Kind != "c_link" || server.Evidence[0].Fields["output"] != "kvd" || server.Evidence[0].Path != "build/main.mk" || server.Evidence[0].Line != serverRule {
		t.Fatalf("kvd evidence: %+v", server.Evidence)
	}
	client := fixture.program(t, "c:kvcli")
	clientRule, _ := fixture.at(t, "build/main.mk", "kvcli: kvcli.o", "")
	// The client links the server's event loop too, as redis-cli links
	// adlist.o, and never runs it.
	if client.Anchor != (cproject.Site{Path: "build/main.mk", Line: clientRule}) || client.Manifest != "Makefile" || client.ManifestFileRef != string(manifestRef) || !reflect.DeepEqual(cSpecPaths(client.Units), []string{"kvcli.c", "loop.c", "net.c", "repl.c", "strbuf.c"}) {
		t.Fatalf("kvcli: %+v", client)
	}
	projected, err := cproject.Index(fixture.repository, fixture.parsed[server.Selector])
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(projected.Input)
	if err != nil {
		t.Fatal(err)
	}
	if index.Target.AnchorFileRef != string(manifestRef) || !slices.ContainsFunc(index.Target.Sources, func(source programindex.TargetSource) bool {
		return source.Path == "Makefile" && source.FileRef == string(manifestRef)
	}) || !slices.ContainsFunc(index.Target.Sources, func(source programindex.TargetSource) bool {
		return source.Path == "build/main.mk" && source.FileRef == string(fragmentRef)
	}) {
		t.Fatalf("native source site replaced its owning target manifest: %+v", index.Target)
	}
	assertNativeAdjacentCommentOwners(t, fixture.root, fixture.repository, index, "kvd.c", "documentedNeighbor", "undocumentedNeighbor")
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(layer.Targets) != 1 || layer.Targets[0].Root != "." || layer.Targets[0].Manifest != "Makefile" {
		t.Fatalf("included source changed invocation root/manifest: %+v", layer.Targets)
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
	// every host, and the two programs that link loop.c report it.
	if !reflect.DeepEqual(server.Outside, []string{"loop_epoll.c"}) || !reflect.DeepEqual(client.Outside, []string{"loop_epoll.c"}) || len(dump.Outside) != 0 {
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
	getRow, getColumn := fixture.at(t, "kvd.c", `{"get", getCommand, 2, preloadKey}`, "getCommand")
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

// Without its makefiles, nothing gives the fixture's flags: each main is a
// program whose files the linker closure decides, parsed with clang's
// defaults. loop.c then takes the host's own backend, and every program
// still parses on every host. (upper/ and util/ need their makefiles' -I..
// and go with them. Under a testdata directory, as in repomap's own
// repository, its files are no units at all.)
func TestCFixtureParsesWithoutItsMakefile(t *testing.T) {
	isolateFixtureGitEnvironment(t)
	const prefix = "services/kv/"
	root := filepath.Join(t.TempDir(), "repository")
	copyFixtureTree(t, filepath.Join(repositoryRoot(t), "testdata", "repositories", "c"), filepath.Join(root, filepath.FromSlash(prefix)))
	for _, made := range []string{"Makefile", "upper", "util"} {
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(prefix), made)); err != nil {
			t.Fatal(err)
		}
	}
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
		if units := cUnitPaths(parsed.Units); !reflect.DeepEqual(units, []string{prefix + "kvd.c", prefix + "loop.c", prefix + "net.c", prefix + "strbuf.c"}) ||
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

// A directory's own makefile the root never reaches is read as a developer
// runs make there (C.md "Nested makefiles"): upper/makefile's default goal
// links upper.so, a shared library whose unit takes -I.. and -fPIC; util's
// Makefile only lists its examples by default and has no `all`, so its units
// are compiled as that makefile compiles their objects, their flags only,
// ping.c and watch.c programs through their own mains; wire's Makefile asks
// for a platform by default and builds on `all`, as Lua 5.1.5's src/Makefile
// does. All parse in the build's view: ping's closure takes the root's net.c;
// watch's loopNowMs, which loop.c and util/fixedclock.c define and no build
// line read decides between, keeps both as alternatives (owner, 2026-10-02),
// never the one beside it; selftest's wireEscape is escape.c's, which
// libwire.a archives with encode.c, never escape_none.c beside them.
func TestCFixtureReadsADirectorysOwnMakefile(t *testing.T) {
	// The expected flags come from this fixture's makefiles. Make's implicit
	// object rule also reads the developer's ambient preprocessor flags.
	t.Setenv("CPPFLAGS", "")
	fixture := loadCFixture(t)
	var runs []string
	for _, build := range fixture.project.Nested {
		runs = append(runs, build.Path+": "+strings.Join(build.Command, " "))
		if build.Err != "" {
			t.Fatalf("%s failed: %s", build.Path, build.Err)
		}
	}
	if want := []string{"upper/makefile: make -n -B -w -o makefile", "util/Makefile: make -n -B -w -o Makefile",
		"util/Makefile: make -n -B -k -w -o Makefile fixedclock.o ping.o watch.o", "wire/Makefile: make -n -B -w -o Makefile",
		"wire/Makefile: make -n -B -w -o Makefile all"}; !reflect.DeepEqual(runs, want) {
		t.Fatalf("nested runs %q, want %q", runs, want)
	}
	upper := fixture.program(t, "c:upper/upper.so")
	rule, _ := fixture.at(t, "upper/makefile", "upper.so: upper.c", "")
	if upper.Kind != cproject.ProgramShared || upper.Anchor != (cproject.Site{Path: "upper/makefile", Line: rule}) || !reflect.DeepEqual(cSpecPaths(upper.Units), []string{"upper/upper.c"}) ||
		!reflect.DeepEqual(upper.LinkArgs, []string{"-shared"}) {
		t.Fatalf("upper.so: %+v", upper)
	}
	if unit := upper.Units[0]; !unit.Built || unit.Makefile != "upper/makefile" || unit.ObjectRule || unit.Dir != "upper" || !reflect.DeepEqual(unit.Args, []string{"-I..", "-fPIC"}) {
		t.Fatalf("upper.c flags: %+v", unit)
	}
	ping := fixture.program(t, "c:util/ping.c")
	if unit := ping.Units[0]; !ping.Closure || !unit.Built || unit.Makefile != "util/Makefile" || !unit.ObjectRule || !reflect.DeepEqual(unit.Args, []string{"-I.."}) ||
		!reflect.DeepEqual(unit.Dropped, []string{"-O2", "-Wall"}) {
		t.Fatalf("ping.c: %+v", ping)
	}
	for _, observation := range fixture.project.Observations {
		if observation.Kind == "c_build_error" || observation.Kind == "c_unit_unbuilt" || observation.Kind == "c_unit_error" {
			t.Fatalf("the fixture's makefiles: %+v", observation)
		}
	}
	if units := cUnitPaths(fixture.parsed["c:util/ping.c"].Units); !reflect.DeepEqual(units, []string{"net.c", "util/ping.c"}) {
		t.Fatalf("ping links %v", units)
	}
	if units := cUnitPaths(fixture.parsed["c:upper/upper.so"].Units); !reflect.DeepEqual(units, []string{"upper/upper.c"}) {
		t.Fatalf("upper.so parses %v", units)
	}
	// The report has words for an executable and a library, not for the
	// adapter's own kinds: a shared library and an archive are indexed as
	// libraries.
	for _, selector := range []string{"c:upper/upper.so", "c:wire/libwire.a"} {
		indexed, err := cproject.Index(fixture.repository, fixture.parsed[selector])
		if err != nil {
			t.Fatal(err)
		}
		if kind := indexed.Input.Target.Kind; kind != "library" {
			t.Fatalf("%s is indexed as %q, want library", selector, kind)
		}
	}
	// watch's clock: two definitions, the build silent on which it links.
	watch := fixture.parsed["c:util/watch.c"]
	if units := cUnitPaths(watch.Units); !reflect.DeepEqual(units, []string{"loop.c", "net.c", "util/fixedclock.c", "util/watch.c"}) ||
		!reflect.DeepEqual(watch.Alternatives, []cproject.Alternative{{Name: "loopNowMs", Units: []string{"loop.c", "util/fixedclock.c"}}}) ||
		!reflect.DeepEqual(watch.AlternativeUnits, []string{"loop.c", "util/fixedclock.c"}) {
		t.Fatalf("watch: units %v, alternatives %+v, alternative units %v", units, watch.Alternatives, watch.AlternativeUnits)
	}
	if clock := fixture.program(t, "c:util/"); clock.Kind != cproject.ProgramLibrary || !reflect.DeepEqual(cSpecPaths(clock.Units), []string{"util/fixedclock.c"}) {
		t.Fatalf("util's library: %+v", clock)
	}
	// wire: the archive, the program linking it, and selftest's closure the
	// archive decides.
	library := fixture.program(t, "c:wire/libwire.a")
	libraryRule, _ := fixture.at(t, "wire/Makefile", "$(LIB): encode.o", "")
	if library.Kind != cproject.ProgramLibrary || library.Anchor != (cproject.Site{Path: "wire/Makefile", Line: libraryRule}) ||
		!reflect.DeepEqual(cSpecPaths(library.Units), []string{"wire/encode.c", "wire/escape.c"}) ||
		!reflect.DeepEqual(library.Evidence, []cproject.Observation{{Kind: "c_archive", Path: "wire/Makefile", Line: libraryRule,
			Fields: map[string]string{"output": "wire/libwire.a", "consumers": "wire/wirecat"}, Values: []string{"wire/encode.c", "wire/escape.c"}}}) {
		t.Fatalf("libwire.a: %+v", library)
	}
	wirecat := fixture.program(t, "c:wire/wirecat")
	if !reflect.DeepEqual(cSpecPaths(wirecat.Units), []string{"wire/encode.c", "wire/escape.c", "wire/wirecat.c"}) || wirecat.Evidence[0].Fields["archives"] != "wire/libwire.a" {
		t.Fatalf("wirecat: %+v", wirecat)
	}
	selftest := fixture.parsed["c:wire/selftest.c"]
	if units := cUnitPaths(selftest.Units); !reflect.DeepEqual(units, []string{"wire/encode.c", "wire/escape.c", "wire/selftest.c"}) || len(selftest.Alternatives) != 0 {
		t.Fatalf("selftest: units %v, alternatives %+v", units, selftest.Alternatives)
	}
	if raw := fixture.program(t, "c:wire/"); !reflect.DeepEqual(cSpecPaths(raw.Units), []string{"wire/escape_none.c"}) {
		t.Fatalf("wire's library: %+v", raw)
	}
}

// A library's entries are its API: libwire.a exports what wire.h, the header
// wirecat includes, declares, and never wireNeedsEscape, which its own files
// share through wire_internal.h. A library no program links (upper.so, which
// a program loads; wire's escape_none.c) exports every function with
// external linkage. An executable exports nothing.
func TestCFixtureLibrariesExportTheirAPI(t *testing.T) {
	fixture := loadCFixture(t)
	if headers := fixture.parsed["c:wire/libwire.a"].APIHeaders; !reflect.DeepEqual(headers, []string{"wire/wire.h"}) {
		t.Fatalf("libwire.a's API headers: %v", headers)
	}
	for _, want := range []struct {
		selector string
		basis    programindex.ExportBasis
		names    []string
	}{
		{"c:wire/libwire.a", programindex.ExportsConsumerHeaders, []string{"wireEncode", "wireEscape"}},
		{"c:wire/", programindex.ExportsLinkage, []string{"wireEscape"}},
		{"c:upper/upper.so", programindex.ExportsLinkage, []string{"upperValue"}},
		{"c:wire/wirecat", "", nil},
	} {
		indexed, err := cproject.Index(fixture.repository, fixture.parsed[want.selector])
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]string{}
		for _, object := range indexed.Input.Objects {
			names[object.SourceRef] = object.Name
		}
		var got []string
		for _, export := range indexed.Input.Target.Exports {
			got = append(got, names[export.ObjectRef])
		}
		slices.Sort(got)
		if basis := indexed.Input.Target.ExportBasis; basis != want.basis || !reflect.DeepEqual(got, want.names) {
			t.Fatalf("%s exports %v by %q, want %v by %q", want.selector, got, basis, want.names, want.basis)
		}
		if _, err := programindex.New(indexed.Input); err != nil {
			t.Fatalf("%s: %v", want.selector, err)
		}
	}
}

// Nested in a bigger repository whose root has no makefile, the fixture's
// Makefile is a directory's own: kvd and kvcli are its link lines, their
// units its flags, and upper/ and util/ read their own makefiles below it.
func TestCFixtureNestedInARepositoryReadsItsMakefile(t *testing.T) {
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
	if project.Build.Kind != cproject.BuildNone {
		t.Fatalf("root build: %+v", project.Build)
	}
	var selectors []string
	for _, program := range project.Programs {
		selectors = append(selectors, program.Selector)
		if program.Selector == "c:"+prefix+"kvd" {
			for _, unit := range program.Units {
				if !unit.Built || unit.Makefile != prefix+"Makefile" || !reflect.DeepEqual(unit.Args, []string{"-std=c99", "-D_DEFAULT_SOURCE", "-DLOOP_POLL"}) {
					t.Fatalf("%s: %+v", unit.Path, unit)
				}
			}
		}
	}
	var want []string
	for _, name := range []string{"kvcli", "kvd", "tools/dump.c", "upper/upper.so", "util/", "util/ping.c", "util/watch.c", "wire/", "wire/libwire.a", "wire/selftest.c", "wire/wirecat"} {
		want = append(want, "c:"+prefix+name)
	}
	if !reflect.DeepEqual(selectors, want) {
		t.Fatalf("programs %v, want %v", selectors, want)
	}
}
