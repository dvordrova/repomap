package surfacediscovery

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
)

func TestDynamicHandoffCaptureUsesExistingSSAAndKeepsRuntimeUncertainty(t *testing.T) {
	repository := t.TempDir()
	source := `package main

import "os"

type Runner interface { Run(func()) }
type worker struct{}
type alternate struct{}
func (worker) Run(func()) {}
func (alternate) Run(func()) {}
func alpha() {}
func beta() {}
func register(callback func()) { callback() }

func main() {
	var runner Runner
	if len(os.Args) > 2 { runner = worker{} } else { runner = alternate{} }
	runner.Run(alpha)
	var callback func()
	if len(os.Args) > 1 { callback = alpha } else { callback = beta }
	callback()
	register(alpha)
	register(callback)
}
`
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/handoff\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", source)
	mainLine := strings.Count(source[:strings.Index(source, "func main")], "\n") + 1
	options := defaultHostOptions(repository)
	options.CaptureDynamicHandoffIndex = true
	result, err := analyzeForTest(options, Input{
		ModuleDirs: []string{"."},
		Packages:   []PackageInput{{Path: "example.com/handoff", ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-main", Kind: AnalysisTargetExecutablePackage,
			ModuleID: "module-main", ModulePath: "example.com/handoff", ModuleDir: ".",
			PackagePath: "example.com/handoff", TargetPackages: []string{"example.com/handoff"},
			Roots: []AnalysisTargetRootInput{{Path: "main.go", Line: mainLine}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	index := result.DynamicHandoffIndex
	if index == nil {
		t.Fatal("dynamic handoff index is absent")
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	if index.SourceDirectCallSHA256 != result.DirectCallIndex.SHA256 ||
		index.Coverage.InterfaceInvokes != 1 || index.Coverage.FunctionValueCalls != 2 ||
		index.Coverage.CallbackTransfers != 3 || index.Coverage.HandoffsOmitted != 0 {
		t.Fatalf("dynamic handoff coverage = %#v", index.Coverage)
	}
	var interfaceAlternatives, valueAlternatives, callbackExact, interfaceCallbackExact bool
	for _, handoff := range index.Handoffs {
		switch {
		case handoff.Kind == godynamichandoff.InterfaceInvoke:
			interfaceAlternatives = handoff.Resolution == godynamichandoff.ResolutionAlternatives &&
				len(handoff.Candidates) == 2 && handoff.Slot.Method == "Run" &&
				handoff.Candidates[0].Evidence == godynamichandoff.EvidenceInterfaceValueAlternative &&
				handoff.Candidates[1].Evidence == godynamichandoff.EvidenceInterfaceValueAlternative
		case handoff.Kind == godynamichandoff.FunctionValueCall &&
			handoff.Resolution == godynamichandoff.ResolutionAlternatives:
			valueAlternatives = len(handoff.Candidates) == 2
		case handoff.Kind == godynamichandoff.CallbackTransfer &&
			handoff.Resolution == godynamichandoff.ResolutionExact:
			callbackExact = len(handoff.Candidates) == 1 && handoff.Slot.Parameter == 1
			if handoff.StaticTarget.Name == "Run" {
				interfaceCallbackExact = callbackExact && handoff.StaticTarget.Receiver == "Runner"
			}
		}
	}
	if !interfaceAlternatives || !valueAlternatives || !callbackExact || !interfaceCallbackExact {
		t.Fatalf("dynamic handoffs lost structural authority: %#v", index.Handoffs)
	}
}
