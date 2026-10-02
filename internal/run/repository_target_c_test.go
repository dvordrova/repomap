package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/cproject"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/targetoutcome"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// cTestRepository is a small C repository: two programs the Makefile links
// share strbuf.c, and tools/dump.c has a main the Makefile never builds.
var cTestRepository = map[string]string{
	"Makefile": "CFLAGS = -std=c99 -O2 -g -Wall -DKV_TEST\n\n" +
		"all: kvd kvcli\n\n" +
		"kvd: kvd.o strbuf.o\n\t$(CC) -o kvd kvd.o strbuf.o\n\n" +
		"kvcli: kvcli.o strbuf.o\n\t$(CC) -o kvcli kvcli.o strbuf.o\n\n" +
		"%.o: %.c\n\t$(CC) $(CFLAGS) -c $<\n",
	"strbuf.h":     "int sb_len(const char *s);\n",
	"strbuf.c":     "#include \"strbuf.h\"\nint sb_len(const char *s) { int n = 0; while (s[n]) n++; return n; }\n",
	"kvd.c":        "#include \"strbuf.h\"\nint main(void) { return sb_len(\"kvd\"); }\n",
	"kvcli.c":      "#include \"strbuf.h\"\nint main(int argc, char **argv) { return sb_len(argv[argc - 1]); }\n",
	"tools/dump.c": "#include <stdio.h>\nint main(void) { puts(\"dump\"); return 0; }\n",
}

func writeCTestRepository(t *testing.T, files map[string]string) (string, *corpus.Corpus) {
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

func cTestFileRef(t *testing.T, repository *corpus.Corpus, path string) corpus.FileID {
	t.Helper()
	ref, ok := repository.ID(path)
	if !ok {
		t.Fatalf("%s is not in the corpus", path)
	}
	return ref
}

func TestCRepositoryTargetsFromLinkLines(t *testing.T) {
	root, repository := writeCTestRepository(t, cTestRepository)
	discovery, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root, NoModel: true})
	if err != nil || !enabled {
		t.Fatalf("discovery: %v %v", enabled, err)
	}
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := discovery.validate(registry); err != nil {
		t.Fatal(err)
	}
	ref := func(path string) corpus.FileID { return cTestFileRef(t, repository, path) }

	// Each program offers the files only it compiles; the shared strbuf.c and
	// the Makefile name several programs and are no candidate of their own.
	var offered []corpus.FileID
	for _, candidate := range discovery.Candidates {
		offered = append(offered, candidate.FileRef)
	}
	slices.Sort(offered)
	want := []corpus.FileID{ref("kvcli.c"), ref("kvd.c"), ref("tools/dump.c")}
	slices.Sort(want)
	if !slices.Equal(offered, want) || !slices.Equal(discovery.RequiredFileRefs, want) {
		t.Fatalf("candidates %v, required %v, want %v", offered, discovery.RequiredFileRefs, want)
	}
	for _, path := range []string{"strbuf.c", "Makefile", "kvd.c", "tools/dump.c"} {
		if !discovery.ResolvesFile(ref(path)) {
			t.Fatalf("%s restores no C program", path)
		}
	}
	if discovery.ResolvesFile(ref("strbuf.h")) {
		t.Fatal("a header no build line names restored a program")
	}
	restored, err := discovery.RestoreFiles([]corpus.FileID{ref("strbuf.c")})
	if err != nil {
		t.Fatal(err)
	}
	var shared []string
	for _, row := range restored {
		shared = append(shared, row.Target.Selector)
		if !slices.Equal(row.FileRefs, []corpus.FileID{ref("strbuf.c")}) {
			t.Fatalf("%s restored from %v", row.Target.Selector, row.FileRefs)
		}
	}
	if !slices.Equal(shared, []string{"c:kvcli", "c:kvd"}) {
		t.Fatalf("strbuf.c restores %v", shared)
	}
	if group, err := discovery.ChoiceGroup(); err != nil || group.Language != "C" || group.Choices != "c:kvcli, c:kvd, c:tools/dump.c" {
		t.Fatalf("choices %+v %v", group, err)
	}

	targets, err := discovery.ResolveExplicit(repository, "c:kvd")
	if err != nil || len(targets) != 1 {
		t.Fatalf("selection: %v %v", targets, err)
	}
	target := targets[0]
	if target.Scope != targetoutcome.ScopeExecutable || target.Display != "kvd" || !slices.Equal(target.AllowedLanguages, []string{"c"}) {
		t.Fatalf("target %+v", target)
	}
	if missing, err := discovery.ResolveExplicit(repository, "c:kvd.c"); err != nil || len(missing) != 0 {
		t.Fatalf("a unit path selected a linked program: %v %v", missing, err)
	}

	// The link line is the program's evidence, and the portfolio prompt
	// defines every kind a C program brings.
	evidence, err := discovery.NativeEvidence(target)
	if err != nil {
		t.Fatal(err)
	}
	makefile := cTestRepository["Makefile"]
	linkLine := strings.Count(makefile[:strings.Index(makefile, "kvd:")], "\n") + 1
	if evidence.Root != "." || len(evidence.Observations) != 1 || evidence.Observations[0].Kind != "c_link" ||
		evidence.Observations[0].Path != "Makefile" || evidence.Observations[0].Line != linkLine ||
		evidence.Observations[0].Fields["output"] != "kvd" || !slices.Equal(evidence.Observations[0].Values, []string{"kvd.c", "strbuf.c"}) {
		t.Fatalf("evidence %+v", evidence)
	}
	dump, err := discovery.ResolveExplicit(repository, "c:tools/dump.c")
	if err != nil || len(dump) != 1 {
		t.Fatalf("closure program: %v %v", dump, err)
	}
	dumpEvidence, err := discovery.NativeEvidence(dump[0])
	if err != nil || dumpEvidence.Root != "tools" {
		t.Fatalf("closure evidence %+v %v", dumpEvidence, err)
	}
	prompt, err := os.ReadFile(filepath.Join("..", "targetportfolio", "prompts", "system.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{evidence.Observations[0].Kind, dumpEvidence.Observations[0].Kind, "c_library"} {
		if !regexp.MustCompile(`(?m)^- (?:[a-z_]+, )*` + kind + `[,:]`).Match(prompt) {
			t.Fatalf("the target portfolio prompt does not define %s", kind)
		}
	}

	// Dispatch parses the program's units once per plan and finds its main.
	cli, err := discovery.ResolveExplicit(repository, "c:kvcli")
	if err != nil || len(cli) != 1 {
		t.Fatalf("selection: %v %v", cli, err)
	}
	adapter := cRepositoryTargetAdapterDescriptor()
	options := repositoryTargetDispatchOptions{Repo: root, Corpus: repository}
	store, err := adapter.PrepareDispatchPlan(repositoryTargetPlan{}, []repositoryTypedTarget{target, cli[0]})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := adapter.PrepareDispatchTarget(t.Context(), options, target, store)
	if err != nil {
		t.Fatal(err)
	}
	facts, ok := binding.ProgramFacts.(*cRepositoryProgramFacts)
	if !ok || !binding.ProgramFactsBound || facts.Parsed.Main == nil || facts.Parsed.Main.Unit != "kvd.c" || len(facts.Parsed.Units) != 2 {
		t.Fatalf("parsed %+v", binding.ProgramFacts)
	}
	if err := binding.Target.validateWith(registry); err != nil || !sameRepositoryPlannedTarget(target, binding.Target) {
		t.Fatalf("prepared target changed: %v", err)
	}
	if _, err := cRepositoryFacts(dump[0], facts); err == nil {
		t.Fatal("another program's parse bound to tools/dump.c")
	}
	if _, err := adapter.BuildDependencies(repositoryDependencyBuildRequest{Target: target, Facts: facts}); err == nil {
		t.Fatal("dependencies were returned before the program was projected")
	}
	unit := func(parsed *cproject.Parsed, path string) *cproject.Unit {
		for _, unit := range parsed.Units {
			if unit.Path == path {
				return unit
			}
		}
		t.Fatalf("%s does not link %s", parsed.Program.Selector, path)
		return nil
	}
	// Projecting kvd is the last read of its units: kvd.c is released, and
	// strbuf.c stays for kvcli, which still links it.
	_, _ = adapter.BuildProgramInput(repositoryProgramBuildRequest{Context: t.Context(), Corpus: repository, Target: target, Facts: facts})
	cliBinding, err := adapter.PrepareDispatchTarget(t.Context(), options, cli[0], store)
	if err != nil {
		t.Fatal(err)
	}
	cliFacts := cliBinding.ProgramFacts.(*cRepositoryProgramFacts)
	if unit(facts.Parsed, "strbuf.c") != unit(cliFacts.Parsed, "strbuf.c") {
		t.Fatal("strbuf.c was parsed once per program, not once per plan")
	}
	// Once no planned program needs a unit, the plan holds none of them: a
	// later parse reads the sources again.
	_, _ = adapter.BuildProgramInput(repositoryProgramBuildRequest{Context: t.Context(), Corpus: repository, Target: cli[0], Facts: cliFacts})
	again, err := adapter.PrepareDispatchTarget(t.Context(), options, target, store)
	if err != nil {
		t.Fatal(err)
	}
	reparsed := again.ProgramFacts.(*cRepositoryProgramFacts).Parsed
	if unit(reparsed, "kvd.c") == unit(facts.Parsed, "kvd.c") || unit(reparsed, "strbuf.c") == unit(cliFacts.Parsed, "strbuf.c") {
		t.Fatal("the plan kept the units of programs it already projected")
	}
	native := target.native.(cproject.Program)
	program := programindex.Target{Language: "c", Selector: native.Selector, Name: native.Name, AnchorFileRef: native.AnchorFileRef}
	if !adapter.MatchProgramTarget(target, program) {
		t.Fatal("the program's own ProgramTarget does not match")
	}
	program.AnchorFileRef = string(ref("kvd.c"))
	if adapter.MatchProgramTarget(target, program) {
		t.Fatal("a ProgramTarget with another anchor matched")
	}
}

func TestCRepositoryDiscoveryIsOrdinary(t *testing.T) {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := registry.descriptor(repositoryTargetAdapterC)
	if !ok || descriptor.Rank != 4 || descriptor.Label != "C" || !explicitNonGoRepositoryTargetSelector("c:kvd") {
		t.Fatalf("C adapter %+v registered %v", descriptor, ok)
	}

	root, repository := writeCTestRepository(t, cTestRepository)
	result, err := discoverRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root, NoModel: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := result.byKey[repositoryTargetAdapterC]; !found || len(result.adapters) != 1 {
		t.Fatalf("adapters %+v", result.adapters)
	}
	plan, err := resolveExplicitRepositoryTarget(repository, result, "c:kvcli", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 || plan.Targets[0].Selector != "c:kvcli" || plan.Default != plan.Targets[0].Key {
		t.Fatalf("plan %+v", plan.Targets)
	}

	// Without the root, a corpus with C sources cannot be discovered; one
	// without them never asks for it.
	if _, _, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository}); err == nil {
		t.Fatal("C discovery ran without the repository root")
	}
	_, python := writeCTestRepository(t, map[string]string{"app.py": "print('hi')\n", "native.h": "int f(void);\n"})
	if _, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: python}); err != nil || enabled {
		t.Fatalf("a corpus without .c files enabled C: %v %v", enabled, err)
	}
}

// Two programs linked from the same files have no file of their own: the
// makefile that links them represents both.
func TestCRepositoryProgramsSharingEveryFile(t *testing.T) {
	root, repository := writeCTestRepository(t, map[string]string{
		"Makefile": "all: fast slow\n\n" +
			"fast: main.o\n\t$(CC) -o fast main.o\n\n" +
			"slow: main.o\n\t$(CC) -o slow main.o\n\n" +
			"%.o: %.c\n\t$(CC) -c $<\n",
		"main.c": "int main(void) { return 0; }\n",
	})
	discovery, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root})
	if err != nil || !enabled {
		t.Fatalf("discovery: %v %v", enabled, err)
	}
	makefile := cTestFileRef(t, repository, "Makefile")
	if len(discovery.Candidates) != 1 || discovery.Candidates[0].FileRef != makefile || discovery.Candidates[0].Hypotheses[0] != cLinkFileHypothesis ||
		!slices.Equal(discovery.RequiredFileRefs, []corpus.FileID{makefile}) {
		t.Fatalf("candidates %+v required %v", discovery.Candidates, discovery.RequiredFileRefs)
	}
	restored, err := discovery.RestoreFiles([]corpus.FileID{makefile})
	if err != nil || len(restored) != 2 || restored[0].Target.Selector != "c:fast" || restored[1].Target.Selector != "c:slow" {
		t.Fatalf("restored %+v %v", restored, err)
	}
}

// TestCRepositoryTargetWithoutClang leaves the program not analyzed because
// the required tool is missing, not because its sources are wrong.
func TestCRepositoryTargetWithoutClang(t *testing.T) {
	root, repository := writeCTestRepository(t, map[string]string{"main.c": "int main(void) { return 0; }\n"})
	t.Setenv("PATH", t.TempDir())
	discovery, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root})
	if err != nil || !enabled {
		t.Fatalf("discovery: %v %v", enabled, err)
	}
	targets, err := discovery.ResolveExplicit(repository, "c:./")
	if err != nil || len(targets) != 1 {
		t.Fatalf("selection: %v %v", targets, err)
	}
	adapter := cRepositoryTargetAdapterDescriptor()
	plan, err := adapter.PrepareDispatchPlan(repositoryTargetPlan{}, targets)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.PrepareDispatchTarget(t.Context(), repositoryTargetDispatchOptions{Repo: root, Corpus: repository}, targets[0], plan)
	if !errors.Is(err, cproject.ErrClangUnavailable) {
		t.Fatalf("error %v", err)
	}
	stage, reason := classifyRepositoryTargetFailure(targetoutcome.StageTargetPreparation, err)
	if stage != targetoutcome.StageTargetPreparation || reason != targetoutcome.ReasonRequiredToolUnavailable {
		t.Fatalf("classified %s %s", stage, reason)
	}
}

// TestCRepositoryOrdinaryRun selects a C program through the ordinary command:
// the run hands discovery the repository root, and the selected program
// reaches its own target page with its C identity.
func TestCRepositoryOrdinaryRun(t *testing.T) {
	root, _ := writeCTestRepository(t, cTestRepository)
	ordinaryGraphGit(t, root, "init", "--quiet")
	ordinaryGraphGit(t, root, "config", "user.email", "repomap@example.test")
	ordinaryGraphGit(t, root, "config", "user.name", "repomap fixture")
	ordinaryGraphGit(t, root, "add", ".")
	ordinaryGraphGit(t, root, "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "C programs")
	debugDir := t.TempDir()
	var console strings.Builder
	runErr := runDefaultWithDeps(root, []string{"--no-model", "--target", "c:kvd", "--no-open", "--debug-dir", debugDir}, defaultRunDeps{
		ctx: t.Context(), stdout: &console, stderr: &console,
		serveReport: func(context.Context, reportserver.Options) error { return nil },
		openReport:  func(string) error { return nil },
	})
	if !strings.Contains(console.String(), "build: make -n -B -w -o Makefile") {
		t.Fatalf("the run did not read the C build (%v):\n%s", runErr, console.String())
	}
	// The run's own directory; "latest" links to it.
	portfolios, err := filepath.Glob(filepath.Join(debugDir, "*", targetoutcome.ArtifactFilename))
	portfolios = slices.DeleteFunc(portfolios, func(found string) bool { return filepath.Base(filepath.Dir(found)) == "latest" })
	if err != nil || len(portfolios) != 1 {
		t.Fatalf("outcome portfolios %v %v (run: %v)", portfolios, err, runErr)
	}
	raw, err := os.ReadFile(portfolios[0])
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := targetoutcome.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(portfolio.Outcomes) != 1 {
		t.Fatalf("outcomes %+v", portfolio.Outcomes)
	}
	selected := portfolio.Outcomes[0].SelectedTarget
	if selected.Selector != "c:kvd" || selected.LanguageGroup != "c" || selected.ScopeKind != targetoutcome.ScopeExecutable ||
		!slices.Equal(selected.AllowedProgramLanguages, []string{"c"}) {
		t.Fatalf("selected %+v", selected)
	}
	if failure := portfolio.Outcomes[0].Failure; failure != nil && failure.Stage == targetoutcome.StageTargetPreparation {
		t.Fatalf("the program's sources did not parse: %+v", failure)
	}

	// The platform view the program was read in is in the run's metadata
	// (owner decision D3): clang and its target, the fortify override, and
	// each unit's kept and dropped build flags. The page carries none of it.
	runDir := filepath.Dir(portfolios[0])
	raw, err = os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	view := metadata.CPlatform
	if view == nil || !strings.Contains(view.Clang, "clang") || view.Target == "" ||
		!slices.Contains(view.Overrides, "-U_FORTIFY_SOURCE") || !slices.Contains(view.Overrides, "-D_FORTIFY_SOURCE=0") {
		t.Fatalf("metadata platform view %+v", view)
	}
	flags := debugdump.CPlatformUnit{Built: true, Kept: []string{"-std=c99", "-DKV_TEST"}, Dropped: []string{"-O2", "-g", "-Wall"}}
	kvd, strbuf := flags, flags
	kvd.Path, strbuf.Path = "kvd.c", "strbuf.c"
	if !reflect.DeepEqual(view.Units, []debugdump.CPlatformUnit{kvd, strbuf}) || len(view.Outside) != 0 {
		t.Fatalf("metadata units %+v outside %v", view.Units, view.Outside)
	}
	for _, name := range []string{"report.html", "report.json"} {
		page, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, label := range []string{view.Clang, view.Target, "_FORTIFY_SOURCE"} {
			if strings.Contains(string(page), label) {
				t.Fatalf("%s shows the platform view: %q", name, label)
			}
		}
	}
}

// A Main flow step citing a registration reads as the callable it
// registers, never as the registrar, with where the walk found it
// registered, as orientation saved it (version 3): kvd's main registers
// acceptHandler with loopCreateFileEvent, acceptHandler registers
// readQueryFromClient, and loopProcessEvents may call each through
// fe->rfileProc, so readQueryFromClient, reached as one of those, reads
// "loopProcessEvents may call acceptHandler registers it" (owner,
// 2026-09-29: three of redis-server's six steps read aeCreateFileEvent, and
// aeMain, aeProcessEvents and createClient were gone).
func TestCMainFlowReadsARegistrationAsTheCallableItRegisters(t *testing.T) {
	root, _ := cumulativeEvidenceRepository(t, "c")
	debugDir := t.TempDir()
	var console strings.Builder
	runErr := runDefaultWithDeps(root, []string{"--no-model", "--target", "c:kvd", "--no-open", "--debug-dir", debugDir}, defaultRunDeps{
		ctx: t.Context(), stdout: &console, stderr: &console,
		serveReport: func(context.Context, reportserver.Options) error { return nil },
		openReport:  func(string) error { return nil },
	})
	if runErr != nil {
		t.Fatalf("run: %v\n%s", runErr, console.String())
	}
	latest := filepath.Join(debugDir, "latest")
	restored, err := report.ReadRunReceipt(latest)
	if err != nil {
		t.Fatal(err)
	}
	data := restored.Data()
	program, err := programindex.ReadFile(filepath.Join(latest, programindex.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(filepath.Join(latest, groupindex.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	index, err := groupindex.Decode(encoded, program)
	if err != nil {
		t.Fatal(err)
	}
	// A reader choosing the event loop, then the client's read handler.
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		for _, choice := range []string{"loopMain", "loopProcessEvents", "readQueryFromClient"} {
			if slices.ContainsFunc(question.Options, func(option llm.Option) bool { return option.Name == choice }) {
				return llm.Verdict{Choice: choice, Probabilities: map[string]float64{choice: 0.9}}, true
			}
		}
		return llm.Verdict{}, false
	}}
	flowWalked, _, err := orientation.WalkFlow(t.Context(), llm.Executor{}, categorizer, orientation.Input{Facts: *data.Facts, Groups: []groupindex.Index{index}}, index.Target.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	data.Orientation = &orientation.Result{MainFlow: flowWalked}
	html, err := report.RenderHTMLWithOptions(data, restored.RenderOptions())
	if err != nil {
		t.Fatal(err)
	}
	flow := mainFlowSection(t, string(html))
	var said []string
	for _, item := range strings.Split(flow, `<li class="flow-step"`)[1:] {
		step := regexp.MustCompile(`^[^>]*><span class="flow-what"><code>([^<]+)</code></span>.*?<span class="flow-how">(.*?)</span><span class="flow-via">`).FindStringSubmatch(item)
		if step == nil {
			continue
		}
		how := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(step[2], "")
		how = regexp.MustCompile(` (registers|runs) it`).ReplaceAllString(how, " $1 it; ")
		said = append(said, step[1]+": "+strings.TrimSuffix(how, "; "))
	}
	if want := []string{"readQueryFromClient: loopProcessEvents may call acceptHandler registers it"}; !reflect.DeepEqual(said, want) {
		t.Fatalf("the Main flow reads %q\nwant %q\n%s", said, want, flow)
	}
	if strings.Contains(flow, "<code>loopCreateFileEvent</code>") {
		t.Fatalf("a step reads the registrar:\n%s", flow)
	}
	// A step's own place is its callable's declaration, never the call
	// registering it (owner, 2026-09-29: readQueryFromClient's link had
	// opened createClient's registering line).
	source, err := os.ReadFile(filepath.Join(root, "kvd.c"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(source), "\n")
	for _, step := range regexp.MustCompile(`<code>(\w+)</code></span> <span class="anchor">kvd\.c:(\d+)<`).FindAllStringSubmatch(flow, -1) {
		line := 0
		for _, digit := range step[2] {
			line = line*10 + int(digit-'0')
		}
		if line < 1 || line > len(lines) || !strings.Contains(lines[line-1], step[1]+"(") || strings.Contains(lines[line-1], "loopCreateFileEvent") {
			t.Fatalf("the step %s is placed at kvd.c:%d, not its declaration:\n%s", step[1], line, flow)
		}
	}
}

// mainFlowSection is the page's Main flow section, its source for the
// component's reading.
func mainFlowSection(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `<section class="component-flow" hidden>`)
	if start < 0 {
		t.Fatal("the page has no Main flow")
	}
	return html[start : start+strings.Index(html[start:], "</section>")]
}

// The client links the fixture's net.c but never listens: the page says
// where the listener went, in the client's own list of what its entrypoints
// never reach on the "What is missing" page, as it lists what the traversal
// never reached before. Read
// together, each program's list names the other program that runs a
// declaration it never runs: the server's event loop, listener and
// sbConsume are the server's, and the server never connects. Neither list
// claims the code is unused.
func TestCRepositoryPageListsWhatAProgramNeverRuns(t *testing.T) {
	root, _ := cumulativeEvidenceRepository(t, "c")
	debugDir := t.TempDir()
	var console strings.Builder
	runErr := runDefaultWithDeps(root, []string{"--no-model", "--target", "c:kvd,c:kvcli", "--no-open", "--debug-dir", debugDir}, defaultRunDeps{
		ctx: t.Context(), stdout: &console, stderr: &console,
		serveReport: func(context.Context, reportserver.Options) error { return nil },
		openReport:  func(string) error { return nil },
	})
	if runErr != nil {
		t.Fatalf("run: %v\n%s", runErr, console.String())
	}
	page, err := os.ReadFile(filepath.Join(debugDir, "latest", "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	neverRuns := func(program string) []string {
		t.Helper()
		section := string(page)
		at := strings.Index(section, `<h4>`+program+`</h4>`)
		if at < 0 || !strings.Contains(section[:at], `class="component-gaps"`) {
			t.Fatalf("the What is missing page has no program %s", program)
		}
		section = section[at:]
		start := strings.Index(section, `-dead">Not reachable from the entrypoints</h5>`)
		if start < 0 {
			t.Fatalf("%s has no list of what its entrypoints never reach", program)
		}
		section = section[start:]
		section = section[:strings.Index(section, "</section>")]
		if !strings.Contains(section, "Nothing this program runs reaches the declarations below. This does not establish that the code is unused.") {
			t.Fatalf("%s's list does not say that it does not establish unused code:\n%s", program, section)
		}
		var listed []string
		for _, match := range regexp.MustCompile(`<li><code>([^<]+)</code> · (.*?)</li>`).FindAllStringSubmatch(section, -1) {
			for _, chip := range regexp.MustCompile(`>([A-Za-z_]\w*)<span class="ln">:\d+</span></(?:a|span)>( <span class="meta run-by">.*?</span>)?`).FindAllStringSubmatch(match[2], -1) {
				entry := match[1] + ":" + chip[1]
				if runBy := regexp.MustCompile(`<a href="#t\d+">([^<]+)</a>`).FindAllStringSubmatch(chip[2], -1); len(runBy) > 0 {
					var programs []string
					for _, name := range runBy {
						programs = append(programs, name[1])
					}
					entry += " run by " + strings.Join(programs, ", ")
				}
				listed = append(listed, entry)
			}
		}
		return listed
	}
	// The client links the server's event loop and runs none of it; the
	// server runs all of it.
	want := []string{"loop.c:oom run by kvd", "loop.c:loopCreate run by kvd", "loop.c:loopCreateFileEvent run by kvd", "loop.c:loopDeleteFileEvent run by kvd",
		"loop.c:loopSetBeforeSleep run by kvd", "loop.c:loopProcessEvents run by kvd", "loop.c:loopMain run by kvd", "loop.c:loopStop run by kvd",
		"loop_poll.c:loopApiCreate run by kvd", "loop_poll.c:loopApiAddEvent run by kvd", "loop_poll.c:loopApiPoll run by kvd",
		"net.c:netListen run by kvd", "strbuf.c:sbConsume run by kvd"}
	if listed := neverRuns("kvcli"); !reflect.DeepEqual(listed, want) {
		t.Fatalf("the client lists %v as never run, want %v", listed, want)
	}
	// The server never connects; the client does.
	if listed, want := neverRuns("kvd"), []string{"net.c:netConnect run by kvcli"}; !reflect.DeepEqual(listed, want) {
		t.Fatalf("the server lists %v as never run, want %v", listed, want)
	}
}

// A backend an #ifdef keeps out on this host is outside this platform's
// build, and the run says so beside the program it belongs to.
func TestCRepositoryReportsSourcesOutsideThisPlatform(t *testing.T) {
	root, repository := writeCTestRepository(t, map[string]string{
		"Makefile": "loop: loop.o\n\t$(CC) -o loop loop.o\n\n%.o: %.c\n\t$(CC) -c $<\n",
		"loop.c": "#ifdef LOOP_FAST\n#include \"loop_fast.c\"\n#else\n#include \"loop_plain.c\"\n#endif\n" +
			"int main(void) { return backend(); }\n",
		"loop_fast.c":  "static int backend(void) { return 1; }\n",
		"loop_plain.c": "static int backend(void) { return 0; }\n",
	})
	discovery, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root})
	if err != nil || !enabled {
		t.Fatalf("discovery: %v %v", enabled, err)
	}
	targets, err := discovery.ResolveExplicit(repository, "c:loop")
	if err != nil || len(targets) != 1 {
		t.Fatalf("selection: %v %v", targets, err)
	}
	adapter := cRepositoryTargetAdapterDescriptor()
	plan, err := adapter.PrepareDispatchPlan(repositoryTargetPlan{}, targets)
	if err != nil {
		t.Fatal(err)
	}
	var console strings.Builder
	binding, err := adapter.PrepareDispatchTarget(t.Context(), repositoryTargetDispatchOptions{Repo: root, Corpus: repository, Output: newRunOutput(&console)}, targets[0], plan)
	if err != nil {
		t.Fatal(err)
	}
	if outside := binding.ProgramFacts.(*cRepositoryProgramFacts).Parsed.Outside; !slices.Equal(outside, []string{"loop_fast.c"}) {
		t.Fatalf("outside %v", outside)
	}
	// The platform view the run records names it too.
	if view := binding.CPlatform; view == nil || !slices.Equal(view.Outside, []string{"loop_fast.c"}) ||
		!reflect.DeepEqual(view.Units, []debugdump.CPlatformUnit{{Path: "loop.c", Built: true}}) {
		t.Fatalf("platform view %+v", binding.CPlatform)
	}
	if !strings.Contains(console.String(), "program: c:loop") || !strings.Contains(console.String(), "outside this platform's build: loop_fast.c") {
		t.Fatalf("console:\n%s", console.String())
	}
}

// cToolLog puts logging clang and make first on PATH: each records its
// arguments in the returned log file and runs the real tool.
func cToolLog(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "tools.log")
	for _, tool := range []string{"clang", "make"} {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s is not installed: %v", tool, err)
		}
		script := "#!/bin/sh\necho \"" + tool + " $*\" >> '" + log + "'\nexec '" + real + "' \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func cToolCalls(t *testing.T, log string) string {
	t.Helper()
	raw, err := os.ReadFile(log)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// repomap's own repository has C files only in testdata/repositories/c, the
// C fixture: they are inputs of the tests around them, never a target, and C
// discovery neither dry-runs repomap's Makefile nor probes clang for them.
func TestCDiscoveryLeavesRepomapsOwnFixtureAlone(t *testing.T) {
	log := cToolLog(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	fixture := 0
	for _, entry := range repository.Entries() {
		if path.Ext(entry.Path) != ".c" {
			continue
		}
		if !corpus.ToolingPath(entry.Path) {
			t.Fatalf("repomap has a C file outside its tooling directories: %s", entry.Path)
		}
		if strings.HasPrefix(entry.Path, "testdata/repositories/c/") {
			fixture++
		}
	}
	if fixture == 0 {
		t.Fatal("the C fixture is not in repomap's corpus")
	}
	if _, ok := repository.ID("Makefile"); !ok {
		t.Fatal("repomap's own Makefile is not in its corpus")
	}
	discovery, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root, NoModel: true})
	if err != nil || enabled || len(discovery.Candidates) != 0 {
		t.Fatalf("C discovery offered repomap's fixture: enabled=%v candidates=%v err=%v", enabled, discovery.Candidates, err)
	}
	project, err := cproject.Discover(t.Context(), root, repository)
	if err != nil || project != nil {
		t.Fatalf("cproject found programs in repomap: %+v %v", project, err)
	}
	if calls := cToolCalls(t, log); calls != "" {
		t.Fatalf("C discovery ran tools for repomap's fixture:\n%s", calls)
	}
}

// Beside a program of its own, a repository's tooling C files are never
// units: no clang probe reads them and no target is theirs.
func TestCDiscoveryProbesNoToolingSources(t *testing.T) {
	root, repository := writeCTestRepository(t, map[string]string{
		"src/main.c":                  "int main(void) { return 0; }\n",
		"testdata/fixture/main.c":     "int main(void) { return 1; }\n",
		".github/actions/check/run.c": "int main(void) { return 2; }\n",
	})
	log := cToolLog(t)
	discovery, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root, NoModel: true})
	if err != nil || !enabled {
		t.Fatalf("discovery: %v %v", enabled, err)
	}
	group, err := discovery.ChoiceGroup()
	if err != nil || group.Choices != "c:src/main.c" {
		t.Fatalf("choices %+v %v", group, err)
	}
	calls := cToolCalls(t, log)
	if !strings.Contains(calls, "src/main.c") || strings.Contains(calls, "testdata/") || strings.Contains(calls, ".github/") {
		t.Fatalf("tool calls:\n%s", calls)
	}
}

// An explicit --target another adapter owns names no C program: C discovery
// does not run make or clang for it. A C selector among them still does.
func TestCDiscoveryWaitsForItsOwnExplicitTarget(t *testing.T) {
	root, repository := writeCTestRepository(t, cTestRepository)
	log := cToolLog(t)
	for _, override := range []string{
		"jsts:package.json",
		"python:.:guard:src/app",
		"clojure:deps.edn",
		"example.com/m@.::example.com/m/cmd/api",
		"jsts:package.json, example.com/m@.::example.com/m/cmd/api",
	} {
		_, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root, NoModel: true, TargetOverride: override})
		if err != nil || enabled {
			t.Fatalf("--target %q enabled C discovery: %v %v", override, enabled, err)
		}
	}
	if calls := cToolCalls(t, log); calls != "" {
		t.Fatalf("C discovery ran tools for another adapter's target:\n%s", calls)
	}
	for _, override := range []string{"c:kvd", "jsts:package.json,c:kvd", "kvd"} {
		_, enabled, err := discoverCRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, Root: root, NoModel: true, TargetOverride: override})
		if err != nil || !enabled {
			t.Fatalf("--target %q disabled C discovery: %v %v", override, enabled, err)
		}
	}
	if calls := cToolCalls(t, log); !strings.Contains(calls, "make ") || !strings.Contains(calls, "clang ") {
		t.Fatalf("C discovery for its own target ran no tools:\n%s", calls)
	}
}
