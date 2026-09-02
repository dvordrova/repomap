package analysistarget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestBindExactRootsTelebotShapedPublicAPI(t *testing.T) {
	repository := t.TempDir()
	writeExactRootsFixture(t, repository, "go.mod", "module gopkg.in/telebot.v3\n\ngo 1.25\n")
	writeExactRootsFixture(t, repository, "telebot.go", `package telebot

type Bot struct{}
func NewBot() *Bot { return &Bot{} }
func (*Bot) Raw() {}
func (*Bot) Download() {}
func File() {}
func hidden() {}
type hiddenType struct{}
func (*hiddenType) Exported() {}
func (*hiddenType) private() {}
type hiddenGeneric[T any] struct{}
func (*hiddenGeneric[T]) Convert() {}
func (*hiddenGeneric[T]) private() {}
`)
	writeExactRootsFixture(t, repository, "layout/layout.go", `package layout
func Open() {}
`)

	target := requireResolvedExactRootsTarget(t, syntheticFacts(
		"module-root", "gopkg.in/telebot.v3",
		[]syntheticPackage{
			{path: "gopkg.in/telebot.v3", dir: "."},
			{path: "gopkg.in/telebot.v3/layout", dir: "layout"},
		},
	))
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(),
		surfacediscovery.DefaultOptions(repository, runtime.GOOS+"/"+runtime.GOARCH),
		surfacediscovery.Input{
			ModuleDirs: []string{"."},
			Packages: []surfacediscovery.PackageInput{
				{Path: "gopkg.in/telebot.v3", ModuleDir: "."},
				{Path: "gopkg.in/telebot.v3/layout", ModuleDir: "."},
			},
			AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
				TargetRef: target.Ref, Kind: surfacediscovery.AnalysisTargetModuleLibrary,
				ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
				TargetPackages: directCallIndexTargetPackagePaths(target),
			},
		},
	)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.DirectCallIndex == nil {
		t.Fatal("Analyze returned no DirectCallIndex")
	}

	roots, err := BindExactRoots(target, result.DirectCallIndex)
	if err != nil {
		t.Fatalf("BindExactRoots: %v", err)
	}
	want := []TargetRoot{
		{Path: "layout/layout.go", Line: 2, Symbol: "gopkg.in/telebot.v3/layout.Open", Package: "gopkg.in/telebot.v3/layout"},
		{Path: "telebot.go", Line: 4, Symbol: "gopkg.in/telebot.v3.NewBot", Package: "gopkg.in/telebot.v3"},
		{Path: "telebot.go", Line: 5, Symbol: "gopkg.in/telebot.v3.(*Bot).Raw", Package: "gopkg.in/telebot.v3"},
		{Path: "telebot.go", Line: 6, Symbol: "gopkg.in/telebot.v3.(*Bot).Download", Package: "gopkg.in/telebot.v3"},
		{Path: "telebot.go", Line: 7, Symbol: "gopkg.in/telebot.v3.File", Package: "gopkg.in/telebot.v3"},
		{Path: "telebot.go", Line: 10, Symbol: "gopkg.in/telebot.v3.(*hiddenType).Exported", Package: "gopkg.in/telebot.v3"},
		{Path: "telebot.go", Line: 13, Symbol: "gopkg.in/telebot.v3.(*hiddenGeneric).Convert", Package: "gopkg.in/telebot.v3"},
	}
	if len(roots.Roots) != len(want) || roots.OmittedRoots != 0 {
		t.Fatalf("roots = %#v omitted=%d, want complete module API roots", roots.Roots, roots.OmittedRoots)
	}
	for index, root := range roots.Roots {
		if root.Symbol != want[index].Symbol || root.Package != want[index].Package ||
			root.NodeID == "" || root.Path != want[index].Path || root.Line != want[index].Line {
			t.Fatalf("root[%d] = %#v, want exact %#v", index, root, want[index])
		}
		if strings.Contains(root.Symbol, ".main") {
			t.Fatalf("off-target or synthetic main root leaked: %#v", root)
		}
	}
	encoded, err := json.Marshal(roots)
	if err != nil {
		t.Fatalf("marshal private roots: %v", err)
	}
	if string(encoded) != "{}" {
		t.Fatalf("private target roots leaked to JSON: %s", encoded)
	}

	cloneRoots := func() TargetRoots {
		result := roots
		result.Scenario.Tags = append([]string(nil), roots.Scenario.Tags...)
		result.Roots = append([]TargetRoot(nil), roots.Roots...)
		return result
	}
	snapshot := cloneRoots()
	snapshot.Scenario.Tags = append(snapshot.Scenario.Tags, "mutated")
	snapshot.Roots[0].Path = "mutated.go"
	if roots.Roots[0].Path != "layout/layout.go" || len(roots.Scenario.Tags) != 0 {
		t.Fatalf("Snapshot mutation changed producer envelope: %#v", roots)
	}

}

func TestBindExactRootsExecutableUsesOnlyExactMainLocator(t *testing.T) {
	repository := t.TempDir()
	writeExactRootsFixture(t, repository, "go.mod", "module example.com/tool\n\ngo 1.25\n")
	writeExactRootsFixture(t, repository, "cmd/tool/main.go", `package main

func helper() {}
func main() { helper() }
`)
	target := requireResolvedExactRootsTarget(t, syntheticFacts(
		"module-root", "example.com/tool",
		[]syntheticPackage{{path: "example.com/tool/cmd/tool", dir: "cmd/tool", executable: true, line: 4}},
	))
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(),
		surfacediscovery.DefaultOptions(repository, runtime.GOOS+"/"+runtime.GOARCH),
		surfacediscovery.Input{
			ModuleDirs: []string{"."},
			Packages: []surfacediscovery.PackageInput{{
				Path: target.PackagePath, ModuleDir: ".",
			}},
			AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
				TargetRef: target.Ref, Kind: surfacediscovery.AnalysisTargetExecutablePackage,
				ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
				PackagePath: target.PackagePath, TargetPackages: []string{target.PackagePath},
				Roots: []surfacediscovery.AnalysisTargetRootInput{{Path: "cmd/tool/main.go", Line: 4}},
			},
		},
	)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	roots, err := BindExactRoots(target, result.DirectCallIndex)
	if err != nil {
		t.Fatalf("BindExactRoots: %v", err)
	}
	if len(roots.Roots) != 1 || roots.Roots[0].Symbol != "example.com/tool/cmd/tool.main" ||
		roots.Roots[0].Path != "cmd/tool/main.go" || roots.Roots[0].Line != 4 {
		t.Fatalf("executable roots = %#v, want only exact package main", roots.Roots)
	}
}

func TestBindExactRootsRejectsNilDirectCallIndex(t *testing.T) {
	target := requireResolvedExactRootsTarget(t, syntheticFacts(
		"module-root", "example.com/library",
		[]syntheticPackage{{path: "example.com/library", dir: "."}},
	))
	if _, err := BindExactRoots(target, nil); err == nil ||
		!strings.Contains(err.Error(), "direct call index is nil") {
		t.Fatalf("BindExactRoots nil index error = %v", err)
	}
}

func TestBoundExactRootCandidatesRetainsPastFormerNodeThreshold(t *testing.T) {
	const formerNodeThreshold = 65_536
	candidates := make([]targetRootCandidate, formerNodeThreshold+3)
	for index := range candidates {
		candidates[index] = targetRootCandidate{root: TargetRoot{
			NodeID: fmt.Sprintf("node-%05d", index), Path: "api.go", Line: index + 1,
			Symbol: fmt.Sprintf("example.com/library.API%05d", index), Package: "example.com/library",
		}}
	}
	roots, omitted := boundExactRootCandidates(candidates)
	if len(roots) != len(candidates) || omitted != 0 {
		t.Fatalf("retained roots=%d omitted=%d, want %d and 0", len(roots), omitted, len(candidates))
	}
	if roots[0] != candidates[0].root || roots[len(roots)-1] != candidates[len(candidates)-1].root {
		t.Fatalf("complete roots were ranked, reordered, or truncated")
	}
}

func requireResolvedExactRootsTarget(t *testing.T, facts gofacts.Facts) Target {
	t.Helper()
	candidates, err := Candidates(facts)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("exact target candidates = %#v, %v", candidates, err)
	}
	return candidates[0].Target.Snapshot()
}

func writeExactRootsFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
