package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestValidateArchitectureGroundingAcceptsPersistedVersions(t *testing.T) {
	t.Parallel()

	for _, version := range []int{
		1,
		legacyArchitectureGroundingVersion,
		previousArchitectureGroundingVersion,
		typedArchitectureGroundingVersion,
		ArchitectureGroundingVersion,
	} {
		grounding := ArchitectureGrounding{
			Version:             version,
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingPackages,
		}
		if version == ArchitectureGroundingVersion {
			grounding.Coverage.Complete = true
			grounding.Coverage.Reasons = []surfacediscovery.GroundingCoverageReason{}
			grounding.Coverage.EntryHandoffs = emptyArchitectureEntryHandoffCoverage()
			grounding.EntryHandoffs = []ArchitectureEntryHandoff{}
		} else if version == typedArchitectureGroundingVersion {
			grounding.Coverage.Complete = true
		}
		if err := validateArchitectureGrounding(grounding); err != nil {
			t.Errorf("version %d: %v", version, err)
		}
	}
}

func TestValidateArchitectureGroundingRequiresTypedProofModeInV4AndV5(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithProofMode(componentmap.AnchorProofCallTarget)
	if err := validateArchitectureGrounding(grounding); err != nil {
		t.Fatalf("valid v5 proof mode: %v", err)
	}

	grounding.BehaviorAnchors[0].ProofMode = ""
	if err := validateArchitectureGrounding(grounding); err == nil {
		t.Fatal("missing v5 proof mode must be rejected")
	}

	grounding = architectureGroundingWithProofMode(componentmap.AnchorProofCallTarget)
	grounding.Version = typedArchitectureGroundingVersion
	grounding.EntryHandoffs = nil
	grounding.Coverage.EntryHandoffs = ArchitectureEntryHandoffCoverage{}
	if err := validateArchitectureGrounding(grounding); err != nil {
		t.Fatalf("valid v4 proof mode: %v", err)
	}
}

func TestValidateArchitectureGroundingRejectsLegacyUntypedAnchors(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithProofMode(componentmap.AnchorProofCallTarget)
	grounding.Version = previousArchitectureGroundingVersion
	grounding.BehaviorAnchors[0].ProofMode = ""
	grounding.Coverage = ArchitectureGroundingCoverage{}
	if err := validateArchitectureGrounding(grounding); err == nil {
		t.Fatal("legacy untyped behavior anchor must not be reinterpreted")
	}
}

func TestValidateArchitectureGroundingV5AcceptsExactEntryHandoff(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithEntryHandoff()
	if err := validateArchitectureGrounding(grounding); err != nil {
		t.Fatalf("valid entry handoff: %v", err)
	}
	if got := grounding.EntryHandoffs[0].ID; !strings.HasPrefix(got, "entry-handoff-") {
		t.Fatalf("entry handoff id = %q", got)
	}
}

func TestValidateArchitectureGroundingV5RejectsTamperedEntryHandoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ArchitectureEntryHandoff)
	}{
		{name: "id", mutate: func(h *ArchitectureEntryHandoff) { h.ID = "entry-handoff-tampered" }},
		{name: "entry", mutate: func(h *ArchitectureEntryHandoff) { h.ProcessEntrypoint.Name = "Main" }},
		{name: "callee", mutate: func(h *ArchitectureEntryHandoff) { h.Callee.ID = h.ProcessEntrypoint.ID }},
		{name: "callsite", mutate: func(h *ArchitectureEntryHandoff) { h.RepresentativeCallsite.Path = "other.go" }},
		{name: "package", mutate: func(h *ArchitectureEntryHandoff) { h.TargetPackage = "example.com/wrong" }},
		{name: "scenario", mutate: func(h *ArchitectureEntryHandoff) { h.Scenario.ID = "go:wrong" }},
		{name: "certainty", mutate: func(h *ArchitectureEntryHandoff) { h.Certainty = evidence.CertaintyObserved }},
		{name: "producer", mutate: func(h *ArchitectureEntryHandoff) { h.Producer.Operation = "classify" }},
		{name: "limitation", mutate: func(h *ArchitectureEntryHandoff) { h.Limitations[0] = "Runtime path." }},
		{name: "witness", mutate: func(h *ArchitectureEntryHandoff) { h.WitnessCount = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grounding := architectureGroundingWithEntryHandoff()
			test.mutate(&grounding.EntryHandoffs[0])
			if err := validateArchitectureGrounding(grounding); err == nil {
				t.Fatal("tampered entry handoff must be rejected")
			}
		})
	}
}

func TestValidateArchitectureGroundingV5RejectsTamperedEntryHandoffCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ArchitectureGrounding)
	}{
		{name: "digest", mutate: func(g *ArchitectureGrounding) {
			g.Coverage.EntryHandoffs.CandidateSetSHA256 = strings.Repeat("0", 64)
		}},
		{name: "published count", mutate: func(g *ArchitectureGrounding) {
			g.Coverage.EntryHandoffs.CandidatesPublished++
		}},
		{name: "witness count", mutate: func(g *ArchitectureGrounding) {
			g.Coverage.EntryHandoffs.WitnessesConsidered--
		}},
		{name: "unexplained partial", mutate: func(g *ArchitectureGrounding) {
			g.Coverage.Complete = false
			g.Coverage.EntryHandoffs.Complete = false
		}},
		{name: "missing parent reason", mutate: func(g *ArchitectureGrounding) {
			g.Coverage.Complete = false
			g.Coverage.EntryHandoffs.Complete = false
			g.Coverage.EntryHandoffs.CandidatesCollected++
			g.Coverage.EntryHandoffs.CandidatesConsidered++
			g.Coverage.EntryHandoffs.Reasons = []surfacediscovery.GroundingCoverageReason{
				surfacediscovery.GroundingCoveragePersistenceLimit,
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grounding := architectureGroundingWithEntryHandoff()
			test.mutate(&grounding)
			if err := validateArchitectureGrounding(grounding); err == nil {
				t.Fatal("tampered entry handoff coverage must be rejected")
			}
		})
	}
}

func TestValidateArchitectureGroundingV5AcceptsTruthfulBoundedEntryHandoffs(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithEntryHandoff()
	grounding.Coverage.Complete = false
	grounding.Coverage.Reasons = []surfacediscovery.GroundingCoverageReason{
		surfacediscovery.GroundingCoveragePersistenceLimit,
	}
	grounding.Coverage.EntryHandoffs = ArchitectureEntryHandoffCoverage{
		Complete: false,
		Reasons: []surfacediscovery.GroundingCoverageReason{
			surfacediscovery.GroundingCoveragePersistenceLimit,
		},
		CandidateSetSHA256:   strings.Repeat("a", 64),
		CandidatesConsidered: 2,
		CandidatesCollected:  2,
		CandidatesPublished:  1,
		WitnessesConsidered:  grounding.EntryHandoffs[0].WitnessCount + 1,
	}
	if err := validateArchitectureGrounding(grounding); err != nil {
		t.Fatalf("truthful persistence-limited entry handoffs: %v", err)
	}
}

func TestValidateArchitectureGroundingV4CannotCarryD210EntryHandoffs(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithEntryHandoff()
	grounding.Version = typedArchitectureGroundingVersion
	if err := validateArchitectureGrounding(grounding); err == nil {
		t.Fatal("v4 entry handoffs must not be reinterpreted")
	}
}

func TestArchitectureEntryHandoffsRemainOutsideCanvasAndAreOpenable(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithEntryHandoff()
	builder := newArchitectureCandidateBuilder(nil, &grounding)
	builder.addArchitectureGrounding(&grounding)
	if bundle := builder.bundle(); len(bundle.Candidates) != 0 || len(bundle.Relations) != 0 {
		t.Fatalf("entry handoff leaked into Architecture Canvas: %#v", bundle)
	}

	data := &ReportData{ArchitectureGrounding: &grounding}
	collectOpenablePaths(data)
	for _, want := range []string{"cmd/app/main.go", "internal/run/run.go"} {
		if !containsString(data.OpenablePaths, want) {
			t.Fatalf("entry handoff path %q is not openable: %#v", want, data.OpenablePaths)
		}
	}
	callsiteFound := false
	for _, target := range overviewSourceTargets(data) {
		if target.path == "cmd/app/main.go" && target.line == 14 {
			callsiteFound = true
			break
		}
	}
	if !callsiteFound {
		t.Fatalf("entry handoff callsite is absent from source coverage: %#v", overviewSourceTargets(data))
	}
}

func TestReadRunDirAcceptsExactProducerEntryHandoffArtifact(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	writeArchitectureGroundingFixtureFile(t, filepath.Join(repository, "go.mod"), `module example.com/app

go 1.24
`)
	writeArchitectureGroundingFixtureFile(t, filepath.Join(repository, "cmd/app/main.go"), `package main

import "example.com/app/internal/run"

func main() { run.Begin() }
`)
	writeArchitectureGroundingFixtureFile(t, filepath.Join(repository, "internal/run/run.go"), `package run

func Begin() {}
`)
	result, err := surfacediscovery.AnalyzeWithInput(
		surfacediscovery.DefaultOptions(repository),
		surfacediscovery.Input{
			RepositoryName: "app",
			ModuleDirs:     []string{"."},
			Entrypoints: []surfacediscovery.EntrypointInput{{
				Package: "example.com/app/cmd/app", PackageDir: "cmd/app", ModuleDir: ".",
				Kind: "primary_binary",
				Anchors: []surfacediscovery.EntrypointAnchorInput{{
					Kind: surfacediscovery.ProcessEntryAnchorGoMain, Path: "cmd/app/main.go", Line: 5,
				}},
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Grounding.EntryHandoffs) != 1 {
		t.Fatalf("producer entry handoffs = %#v", result.Grounding.EntryHandoffs)
	}
	runDir := t.TempDir()
	if err := surfacediscovery.WriteArtifacts(runDir, result); err != nil {
		t.Fatal(err)
	}
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureGrounding == nil || len(data.ArchitectureGrounding.EntryHandoffs) != 1 {
		t.Fatalf("validated producer grounding = %#v", data.ArchitectureGrounding)
	}
	for _, warning := range data.Warnings {
		if strings.Contains(warning, "architecture grounding:") {
			t.Fatalf("exact producer artifact was rejected: %s", warning)
		}
	}
	for _, want := range []string{"cmd/app/main.go", "internal/run/run.go"} {
		if !containsString(data.OpenablePaths, want) {
			t.Fatalf("producer handoff path %q is not openable: %#v", want, data.OpenablePaths)
		}
	}
}

func TestValidateArchitectureGroundingKeepsProofModeOrthogonalToNonProcessKind(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithProofMode(componentmap.AnchorProofDeclarationFamily)
	grounding.BehaviorAnchors[0].Kind = componentmap.AnchorLifecycleStart
	if err := validateArchitectureGrounding(grounding); err != nil {
		t.Fatalf("declaration-family lifecycle anchor: %v", err)
	}

	grounding.BehaviorAnchors[0].Kind = componentmap.AnchorProcessEntry
	if err := validateArchitectureGrounding(grounding); err == nil {
		t.Fatal("process entry must use process-entry proof mode")
	}
}

func architectureGroundingWithProofMode(mode componentmap.AnchorProofMode) ArchitectureGrounding {
	location := evidence.Location{Path: "service/start.go", Line: 12, Column: 1}
	declarationFamilyMembers := 0
	if mode == componentmap.AnchorProofDeclarationFamily {
		declarationFamilyMembers = 1
	}
	return ArchitectureGrounding{
		Version:             ArchitectureGroundingVersion,
		RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
		GroundingMode:       componentmap.GroundingBehavior,
		BehaviorAnchors: []ArchitectureBehaviorAnchor{{
			ID: "anchor-1", Kind: componentmap.AnchorLifecycleStart, ProofMode: mode,
			Label: "service start", Location: location,
			Scenario:          architectureGroundingScenario{ID: "scenario-1", GOOS: "linux", GOARCH: "amd64"},
			Certainty:         evidence.CertaintyStatic,
			AssociatedMembers: []ArchitectureAnchorMember{{ID: "member-1", Location: location}},
			Limitations:       []string{"Static local proof only."},
		}},
		Coverage: ArchitectureGroundingCoverage{
			Complete: true, AnchorsConsidered: 1, AnchorsPublished: 1,
			DeclarationFamilyMembersConsidered: declarationFamilyMembers,
			DeclarationFamilyMembersPublished:  declarationFamilyMembers,
			Reasons:                            []surfacediscovery.GroundingCoverageReason{},
			EntryHandoffs:                      emptyArchitectureEntryHandoffCoverage(),
		},
		EntryHandoffs: []ArchitectureEntryHandoff{},
	}
}

func architectureGroundingWithEntryHandoff() ArchitectureGrounding {
	entry := ArchitectureAnchorMember{
		ID: "example.com/app.main", Package: "example.com/app", Name: "main",
		Location: evidence.Location{Path: "cmd/app/main.go", Line: 10, Column: 6},
	}
	callee := ArchitectureAnchorMember{
		ID: "example.com/app/internal/run.Begin", Package: "example.com/app/internal/run", Name: "Begin",
		Location: evidence.Location{Path: "internal/run/run.go", Line: 8, Column: 6},
	}
	handoff := ArchitectureEntryHandoff{
		ProcessEntrypoint:      entry,
		Callee:                 callee,
		RepresentativeCallsite: evidence.Location{Path: "cmd/app/main.go", Line: 14, Column: 2},
		WitnessCount:           2,
		TargetPackage:          callee.Package,
		Scenario: architectureGroundingScenario{
			ID: "go:linux/amd64:tags=", GOOS: "linux", GOARCH: "amd64", Tags: []string{},
		},
		Certainty: evidence.CertaintyStatic,
		Producer: evidence.Provenance{
			Provider: "go_ssa", Version: surfacediscovery.AnalyzerVersion,
			Operation: "collect_entry_direct_static_handoff",
		},
		Limitations: []string{architectureEntryHandoffLimitation},
	}
	handoff.ID = architectureEntryHandoffID(handoff)
	grounding := ArchitectureGrounding{
		Version:             ArchitectureGroundingVersion,
		RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
		GroundingMode:       componentmap.GroundingPackages,
		BehaviorAnchors:     []ArchitectureBehaviorAnchor{},
		Relationships:       []ArchitectureBehaviorHandoff{},
		EntryHandoffs:       []ArchitectureEntryHandoff{handoff},
		Coverage: ArchitectureGroundingCoverage{
			Complete: true,
			Reasons:  []surfacediscovery.GroundingCoverageReason{},
		},
	}
	grounding.Coverage.EntryHandoffs = ArchitectureEntryHandoffCoverage{
		Complete:             true,
		Reasons:              []surfacediscovery.GroundingCoverageReason{},
		CandidateSetSHA256:   architectureEntryHandoffCandidateSetSHA256(grounding.EntryHandoffs),
		CandidatesConsidered: 1,
		CandidatesCollected:  1,
		CandidatesPublished:  1,
		WitnessesConsidered:  handoff.WitnessCount,
	}
	return grounding
}

func writeArchitectureGroundingFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
