package gopls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	symbolbundle "github.com/dvordrova/repomap/internal/symbol"
)

type fakeRunner struct {
	outputs map[string]string
}

func TestCommandTimeoutDefaultAndOverride(t *testing.T) {
	if got := newWithRunner(Options{}, fakeRunner{}).opts.CommandTimeout; got != DefaultCommandTimeout {
		t.Fatalf("default command timeout = %s, want %s", got, DefaultCommandTimeout)
	}
	if got := newWithRunner(
		Options{CommandTimeout: time.Second},
		fakeRunner{},
	).opts.CommandTimeout; got != time.Second {
		t.Fatalf("explicit command timeout = %s, want %s", got, time.Second)
	}
}

func (f fakeRunner) Run(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	output, ok := f.outputs[key]
	if key == "version" && !ok {
		return []byte("golang.org/x/tools/gopls v0.test\n"), nil
	}
	if !ok {
		return nil, fmt.Errorf("unexpected command %q", key)
	}
	return []byte(output), nil
}

func TestAnalyzeMarksPossibleAndStaticRelations(t *testing.T) {
	runner := fakeRunner{outputs: map[string]string{
		"version": "golang.org/x/tools/gopls v0.21.0\n",
		"workspace_symbol -matcher fuzzy Put": strings.Join([]string{
			"/repo/key.go:10:6-9 Put Function",
			"/repo/server.go:20:18-21 kv.Put Method",
		}, "\n"),
		"call_hierarchy /repo/key.go:10:6": strings.Join([]string{
			"caller[0]: ranges 30:2-5 in /repo/main.go from/to function main in /repo/main.go:25:6-10",
			"identifier: function Put in /repo/key.go:10:6-9",
			"callee[0]: ranges 11:2-5 in /repo/key.go from/to function Put in /repo/key.go:10:6-9",
			"callee[0]: ranges 12:2-7 in /repo/key.go from/to function Apply in /repo/apply.go:8:6-11",
		}, "\n"),
		"call_hierarchy /repo/server.go:20:18": "identifier: method kv.Put in /repo/server.go:20:18-21\n",
	}}
	analyzer := newWithRunner(Options{
		MaxSymbols:     10,
		MaxCallRoots:   2,
		CommandTimeout: time.Second,
	}, runner)

	graph, err := analyzer.Analyze(context.Background(), analysis.Request{RepoPath: "/repo", Query: "Put"})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	summary := graph.Summary()
	if summary.ByCertainty[evidence.CertaintyPossible] != 2 {
		t.Fatalf("possible relations = %d, want 2", summary.ByCertainty[evidence.CertaintyPossible])
	}
	if summary.ByCertainty[evidence.CertaintyStatic] != 3 {
		t.Fatalf("static relations = %d, want 3", summary.ByCertainty[evidence.CertaintyStatic])
	}
	assertRelation(t, graph, evidence.RelationResolvesTo, "Put")
}

func assertRelation(t *testing.T, graph evidence.Graph, kind evidence.RelationKind, targetName string) {
	t.Helper()

	entities := make(map[string]evidence.Entity, len(graph.Entities))
	for _, entity := range graph.Entities {
		entities[entity.ID] = entity
	}
	for _, relation := range graph.Relations {
		if relation.Kind == kind && entities[relation.To].Name == targetName {
			return
		}
	}
	t.Fatalf("relation %q to %q not found", kind, targetName)
}

func TestResolveExactSymbol(t *testing.T) {
	t.Parallel()

	symbols := []symbol{
		{Name: "KVServer.Put", Location: evidence.Location{Path: "generated.go", Line: 1}},
		{Name: "kvServer.Put", Location: evidence.Location{Path: "key.go", Line: 90}},
	}

	resolved, ok := resolveExactSymbol("kvServer.Put", symbols)
	if !ok {
		t.Fatal("resolveExactSymbol() did not resolve unique exact symbol")
	}
	if resolved.Location.Path != "key.go" {
		t.Fatalf("resolved path = %q, want key.go", resolved.Location.Path)
	}
}

func TestResolveExactSymbolRejectsAmbiguousName(t *testing.T) {
	t.Parallel()

	symbols := []symbol{
		{Name: "Run", Location: evidence.Location{Path: "a.go", Line: 1}},
		{Name: "Run", Location: evidence.Location{Path: "b.go", Line: 2}},
	}

	if _, ok := resolveExactSymbol("Run", symbols); ok {
		t.Fatal("resolveExactSymbol() resolved ambiguous symbol")
	}
}

func TestAnalyzeExactSymbolDisambiguatesSameNameByLocation(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"a.go": "package fixture\n\nfunc Run() {}\n",
		"b.go": "package fixture\n\n\n\nfunc Run() {}\n",
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{outputs: map[string]string{
		"call_hierarchy " + filepath.Join(resolvedRepo, "a.go") + ":3:6": strings.Join([]string{
			"identifier: function Run in " + filepath.Join(resolvedRepo, "a.go") + ":3:6-9",
			"caller[0]: ranges 5:12-15 in " + filepath.Join(resolvedRepo, "b.go") + " from/to function Run in " + filepath.Join(resolvedRepo, "b.go") + ":5:6-9",
		}, "\n"),
		"call_hierarchy " + filepath.Join(resolvedRepo, "b.go") + ":5:6": "identifier: function Run in " + filepath.Join(resolvedRepo, "b.go") + ":5:6-9\n",
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)

	for _, test := range []struct {
		path string
		line int
	}{
		{path: "a.go", line: 3},
		{path: "b.go", line: 5},
	} {
		t.Run(test.path, func(t *testing.T) {
			graph, err := analyzer.AnalyzeExactSymbol(context.Background(), analysis.ExactSymbolRequest{
				RepoPath: repo,
				Symbol: evidence.Entity{
					Kind:     evidence.EntityFunction,
					Name:     "Run",
					Language: "go",
					Location: &evidence.Location{Path: test.path, Line: test.line, Column: 6},
				},
			})
			if err != nil {
				t.Fatalf("AnalyzeExactSymbol() error = %v", err)
			}
			if err := graph.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			target := exactResolutionTarget(t, graph)
			if target.Location == nil || target.Location.Path != test.path || target.Location.Line != test.line {
				t.Fatalf("resolved target = %#v, want %s:%d", target, test.path, test.line)
			}
			bundle, err := symbolbundle.Build(graph, symbolbundle.Options{})
			if err != nil {
				t.Fatalf("symbol.Build() error = %v", err)
			}
			if bundle.Target.Entity.Location == nil || bundle.Target.Entity.Location.Path != test.path {
				t.Fatalf("bundle target = %#v, want %s", bundle.Target.Entity, test.path)
			}
			for _, relation := range graph.Relations {
				if relation.Certainty != evidence.CertaintyStatic {
					t.Fatalf("relation certainty = %q, want static", relation.Certainty)
				}
				for _, provenance := range relation.Provenance {
					if provenance.Operation != "call_hierarchy" {
						t.Fatalf("provenance operation = %q, want call_hierarchy", provenance.Operation)
					}
				}
			}
		})
	}
}

func TestAnalyzeExactSymbolRanksProductionCallersAndReportsBounds(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	for _, name := range []string{"target.go", "caller_a_test.go", "caller_b.go", "callee_a.go", "callee_b.go"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("package fixture\n\nfunc Symbol() {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(resolvedRepo, "target.go")
	line := func(direction, file string) string {
		path := filepath.Join(resolvedRepo, file)
		return direction + ": ranges 3:6-12 in " + path + " from/to function Symbol in " + path + ":3:6-12"
	}
	runner := fakeRunner{outputs: map[string]string{
		"call_hierarchy " + targetPath + ":3:6": strings.Join([]string{
			"identifier: function Symbol in " + targetPath + ":3:6-12",
			line("caller[0]", "caller_a_test.go"),
			line("caller[1]", "caller_b.go"),
			line("callee[0]", "callee_a.go"),
			line("callee[1]", "callee_b.go"),
		}, "\n"),
	}}
	analyzer := newWithRunner(Options{MaxCallers: 1, MaxCallees: 1, CommandTimeout: time.Second}, runner)

	graph, err := analyzer.AnalyzeExactSymbol(context.Background(), analysis.ExactSymbolRequest{
		RepoPath: repo,
		Symbol: evidence.Entity{
			Kind:     evidence.EntityFunction,
			Name:     "Symbol",
			Language: "go",
			Location: &evidence.Location{Path: "target.go", Line: 3, Column: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	entities := make(map[string]evidence.Entity, len(graph.Entities))
	for _, entity := range graph.Entities {
		entities[entity.ID] = entity
	}
	var callerPath string
	for _, relation := range graph.Relations {
		if relation.Kind == evidence.RelationCalls {
			calls++
			if target := entities[relation.To]; target.Location != nil && target.Location.Path == "target.go" {
				callerPath = entities[relation.From].Location.Path
			}
		}
	}
	if calls != 2 {
		t.Fatalf("call relations = %d, want one caller and one callee", calls)
	}
	if callerPath != "caller_b.go" {
		t.Fatalf("retained caller = %q, want production caller_b.go", callerPath)
	}
	warnings := strings.Join(graph.Warnings, "\n")
	for _, want := range []string{
		"omitted 1 incoming calls at analyzer limit 1",
		"omitted 1 outgoing calls at analyzer limit 1",
	} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("warnings = %v, want %q", graph.Warnings, want)
		}
	}
}

func TestAnalyzeExactSymbolPreservesSelectedMethodIdentity(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(repo, "worker.go"),
		[]byte("package fixture\n\ntype Worker struct{}\n\nfunc (w *Worker) Run() {}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{outputs: map[string]string{
		"call_hierarchy " + filepath.Join(resolvedRepo, "worker.go") + ":5:18": "identifier: function Run in " + filepath.Join(resolvedRepo, "worker.go") + ":5:18-21\n",
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)

	graph, err := analyzer.AnalyzeExactSymbol(context.Background(), analysis.ExactSymbolRequest{
		RepoPath: repo,
		Symbol: evidence.Entity{
			Kind:     evidence.EntityMethod,
			Name:     "(*Worker).Run",
			Language: "go",
			Location: &evidence.Location{Path: "worker.go", Line: 5, Column: 18},
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeExactSymbol() error = %v", err)
	}
	target := exactResolutionTarget(t, graph)
	if target.Kind != evidence.EntityMethod || target.Name != "(*Worker).Run" {
		t.Fatalf("resolved target = %#v, want selected method identity", target)
	}
}

func TestAnalyzeExactSymbolRejectsBodyPosition(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(repo, "run.go"),
		[]byte("package fixture\n\nfunc Run() {\n\tprintln(\"running\")\n}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{outputs: map[string]string{
		"call_hierarchy " + filepath.Join(resolvedRepo, "run.go") + ":4:2": "identifier: function Run in " + filepath.Join(resolvedRepo, "run.go") + ":3:6-9\n",
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)

	_, err = analyzer.AnalyzeExactSymbol(context.Background(), analysis.ExactSymbolRequest{
		RepoPath: repo,
		Symbol: evidence.Entity{
			Kind:     evidence.EntityFunction,
			Name:     "Run",
			Language: "go",
			Location: &evidence.Location{Path: "run.go", Line: 4, Column: 2},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match selected declaration") {
		t.Fatalf("AnalyzeExactSymbol() error = %v, want declaration mismatch", err)
	}
}

func exactResolutionTarget(t *testing.T, graph evidence.Graph) evidence.Entity {
	t.Helper()

	entities := make(map[string]evidence.Entity, len(graph.Entities))
	for _, entity := range graph.Entities {
		entities[entity.ID] = entity
	}
	for _, relation := range graph.Relations {
		if relation.Kind == evidence.RelationResolvesTo {
			return entities[relation.To]
		}
	}
	t.Fatal("exact resolution relation not found")
	return evidence.Entity{}
}

func TestCanonicalizeHierarchyRootUsesWorkspaceSymbolIdentityAtSamePosition(t *testing.T) {
	t.Parallel()

	requested := symbol{
		Name:     "kvServer.Put",
		Kind:     evidence.EntityMethod,
		Location: evidence.Location{Path: "server/key.go", Line: 90, Column: 20},
	}
	reported := symbol{
		Name:     "Put",
		Kind:     evidence.EntityMethod,
		Location: evidence.Location{Path: "server/key.go", Line: 90, Column: 20},
	}

	canonical := canonicalizeHierarchyRoot(requested, reported)
	if canonical.Name != "kvServer.Put" {
		t.Fatalf("canonical name = %q, want kvServer.Put", canonical.Name)
	}
}

func TestParseWorkspaceSymbols(t *testing.T) {
	output := []byte(strings.Join([]string{
		"/repo/key.go:10:6-9 Put Function",
		"/repo/key.go:20:18-21 kv.Put Method",
		"Log: ignored diagnostic line",
	}, "\n"))

	symbols := parseWorkspaceSymbols(output)
	if len(symbols) != 2 {
		t.Fatalf("symbols = %d, want 2", len(symbols))
	}
	if symbols[1].Name != "kv.Put" || symbols[1].Kind != evidence.EntityMethod {
		t.Fatalf("symbols[1] = %#v", symbols[1])
	}
}

func TestResolveLocationRanksLeadingCommentAndPrecedingDeclaration(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	sourcePath := filepath.Join(repo, "batch.go")
	source := strings.Join([]string{
		"package fixture",
		"type Batch struct {",
		"\tother int",
		"",
		"",
		"\t// waits for fsync",
		"\t// before publishing",
		"\tcommit int",
		"}",
		"",
		"// DeleteSized deletes a key.",
		"func (b *Batch) DeleteSized() {}",
	}, "\n")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{outputs: map[string]string{
		"symbols " + filepath.Join(resolvedRepo, "batch.go"): strings.Join([]string{
			"Batch Struct 2:6-2:11",
			"\tother Field 3:2-3:7",
			"\tcommit Field 8:2-8:8",
			"(*Batch).DeleteSized Method 12:17-12:28",
		}, "\n"),
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)
	result, err := analyzer.ResolveLocation(context.Background(), analysis.LocationRequest{
		RepoPath: repo,
		Location: evidence.Location{Path: "batch.go", Line: 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) < 2 {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	first := result.Candidates[0]
	if first.Entity.Name != "Batch.commit" || first.Entity.Kind != evidence.EntityField ||
		first.Match != "leading_comment" || first.Certainty != evidence.CertaintyPossible || first.Investigable {
		t.Fatalf("first candidate = %#v", first)
	}
	second := result.Candidates[1]
	if second.Entity.Name != "Batch" || second.Match != "preceding_declaration" {
		t.Fatalf("second candidate = %#v", second)
	}
}

func TestResolveLocationMarksExactMethodInvestigable(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	sourcePath := filepath.Join(repo, "batch.go")
	if err := os.WriteFile(sourcePath, []byte("package fixture\n\nfunc (b *Batch) DeleteSized() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{outputs: map[string]string{
		"symbols " + filepath.Join(resolvedRepo, "batch.go"): "(*Batch).DeleteSized Method 3:17-3:28\n",
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)
	result, err := analyzer.ResolveLocation(context.Background(), analysis.LocationRequest{
		RepoPath: repo,
		Location: evidence.Location{Path: "batch.go", Line: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].Investigable ||
		result.Candidates[0].Match != "declaration" || result.Candidates[0].Certainty != evidence.CertaintyStatic {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
}

func TestResolveLocationRanksFileCallablesByComponentTerms(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	sourcePath := filepath.Join(repo, "batch.go")
	if err := os.WriteFile(sourcePath, []byte("package fixture\n\nfunc Apply() {}\nfunc Run() {}\nfunc Commit() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{outputs: map[string]string{
		"symbols " + filepath.Join(resolvedRepo, "batch.go"): strings.Join([]string{
			"Apply Function 3:6-3:11",
			"Run Function 4:6-4:9",
			"Commit Function 5:6-5:12",
		}, "\n"),
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)
	result, err := analyzer.ResolveLocation(context.Background(), analysis.LocationRequest{
		RepoPath:      repo,
		Location:      evidence.Location{Path: "batch.go", Line: 1},
		MaxCandidates: 3,
		RankTerms:     []string{"batch", "commit", "commit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 3 || result.Candidates[0].Entity.Name != "Commit" ||
		result.Candidates[0].Match != "file_declaration" || len(result.Candidates[0].RankReasons) != 1 {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
}

func TestParseImplementationLocations(t *testing.T) {
	locations := parseLocations([]byte(strings.Join([]string{
		"/repo/impl.go:14:6-12",
		"/repo/other.go:22:2-5",
		"Info: loading packages",
	}, "\n")))
	if len(locations) != 2 {
		t.Fatalf("locations = %d, want 2", len(locations))
	}
	if locations[0].Line != 14 || locations[0].Column != 6 {
		t.Fatalf("locations[0] = %#v", locations[0])
	}
}

func TestReferencesReturnsUniqueLocalLocations(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	for _, name := range []string{"key.go", "key_test.go"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("package fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(filepath.Dir(resolvedRepo), "outside.go")
	if err := os.WriteFile(outsidePath, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	referenceCommand := "references " + filepath.Join(resolvedRepo, "key.go") + ":10:6"
	runner := fakeRunner{outputs: map[string]string{
		referenceCommand: strings.Join([]string{
			filepath.Join(resolvedRepo, "key_test.go") + ":20:4-7",
			filepath.Join(resolvedRepo, "key_test.go") + ":20:4-7",
			filepath.Join(resolvedRepo, "key.go") + ":30:2-5",
			outsidePath + ":40:2-5",
		}, "\n"),
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)
	result, err := analyzer.References(context.Background(), repo, evidence.Location{
		Path:   "key.go",
		Line:   10,
		Column: 6,
	})
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("References().Validate() error = %v", err)
	}
	locations := result.Locations
	if len(locations) != 2 {
		t.Fatalf("locations = %#v", locations)
	}
	if locations[0].Path != "key.go" || locations[1].Path != "key_test.go" {
		t.Fatalf("locations = %#v", locations)
	}
}

func TestReferencesRequiresUsableLocation(t *testing.T) {
	t.Parallel()

	analyzer := newWithRunner(Options{}, fakeRunner{})
	if _, err := analyzer.References(context.Background(), t.TempDir(), evidence.Location{}); err == nil {
		t.Fatal("References() error = nil")
	}
}

func TestReferencesResolvesInputSymlinks(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	targetPath := filepath.Join(repo, "key.go")
	if err := os.WriteFile(targetPath, []byte("package fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(repo, "alias.go")
	if err := os.Symlink(targetPath, aliasPath); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}

	referenceCommand := "references " + filepath.Join(resolvedRepo, "key.go") + ":10:6"
	runner := fakeRunner{outputs: map[string]string{
		referenceCommand: targetPath + ":30:2-5\n",
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)
	result, err := analyzer.References(context.Background(), repo, evidence.Location{
		Path:   "alias.go",
		Line:   10,
		Column: 6,
	})
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}
	locations := result.Locations
	if len(locations) != 1 || locations[0].Path != "key.go" {
		t.Fatalf("locations = %#v, want key.go", locations)
	}
}

func TestReferencesRejectsInputSymlinkEscape(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(base, "outside.go")
	if err := os.WriteFile(outsidePath, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(repo, "escape.go")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, fakeRunner{})
	_, err := analyzer.References(context.Background(), repo, evidence.Location{
		Path:   "escape.go",
		Line:   1,
		Column: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("References() error = %v, want outside repository", err)
	}
}

func TestReferencesFiltersOutputSymlinkEscapes(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(repo, "key.go")
	localPath := filepath.Join(repo, "local_test.go")
	canonicalPath := filepath.Join(repo, "canonical_test.go")
	outsidePath := filepath.Join(base, "outside_test.go")
	for _, path := range []string{keyPath, localPath, canonicalPath, outsidePath} {
		if err := os.WriteFile(path, []byte("package fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	insideAlias := filepath.Join(repo, "inside_alias_test.go")
	if err := os.Symlink(canonicalPath, insideAlias); err != nil {
		t.Skipf("create inside symlink: %v", err)
	}
	escapeAlias := filepath.Join(repo, "escape_alias_test.go")
	if err := os.Symlink(outsidePath, escapeAlias); err != nil {
		t.Skipf("create escaping symlink: %v", err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}

	referenceCommand := "references " + filepath.Join(resolvedRepo, "key.go") + ":10:6"
	runner := fakeRunner{outputs: map[string]string{
		referenceCommand: strings.Join([]string{
			insideAlias + ":20:4-7",
			canonicalPath + ":20:4-7",
			escapeAlias + ":30:2-5",
			localPath + ":40:2-5",
		}, "\n"),
	}}
	analyzer := newWithRunner(Options{CommandTimeout: time.Second}, runner)
	result, err := analyzer.References(context.Background(), repo, evidence.Location{
		Path:   "key.go",
		Line:   10,
		Column: 6,
	})
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}
	locations := result.Locations
	if len(locations) != 2 {
		t.Fatalf("locations = %#v, want two local canonical locations", locations)
	}
	if locations[0].Path != "canonical_test.go" || locations[1].Path != "local_test.go" {
		t.Fatalf("locations = %#v", locations)
	}
}
