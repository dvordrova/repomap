package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

const (
	studyInvestigationTestRevision  = "0123456789abcdef0123456789abcdef01234567"
	studyInvestigationTestFreshness = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestValidateStudyInvestigationRepositoryFreshnessRejectsAnyDrift(t *testing.T) {
	initial := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: "/repo",
		Head:     studyInvestigationTestRevision,
		Dirty:    []freshness.DirtyFile{},
	}
	if err := validateStudyInvestigationRepositoryFreshness(initial, initial); err != nil {
		t.Fatalf("unchanged repository: %v", err)
	}

	changed := initial
	changed.Dirty = []freshness.DirtyFile{{
		Status:        "modified",
		Path:          "unrelated.txt",
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: strings.Repeat("a", 64),
	}}
	if err := validateStudyInvestigationRepositoryFreshness(initial, changed); err == nil ||
		!strings.Contains(err.Error(), "repository changed") {
		t.Fatalf("drift error = %v", err)
	}
}

type studyInvestigationRuntimeProvider struct {
	requests    []mechanismstudy.Request
	calls       int
	failCall    int
	failErr     error
	attempts    int
	failContent []byte
}

func (provider *studyInvestigationRuntimeProvider) MechanismStudyPromptJSON(
	prompt mechanismstudy.Prompt,
) ([]byte, error) {
	const marker = "Exact request bundle JSON:\n"
	position := strings.LastIndex(prompt.User, marker)
	if position < 0 {
		return nil, errors.New("missing exact request marker")
	}
	var request mechanismstudy.Request
	if err := json.Unmarshal([]byte(prompt.User[position+len(marker):]), &request); err != nil {
		return nil, err
	}
	provider.requests = append(provider.requests, request)
	return json.Marshal(struct {
		RequestRef string `json:"request_ref"`
	}{RequestRef: request.RequestRef})
}

func (provider *studyInvestigationRuntimeProvider) MechanismStudyBodyMeasured(
	_ context.Context,
	_ []byte,
) (modelresearch.ProviderResult, error) {
	provider.calls++
	if provider.failCall == provider.calls {
		failure := provider.failErr
		if failure == nil {
			failure = errors.New("fixture provider failed")
		}
		return modelresearch.ProviderResult{
			Content: provider.failContent, Attempts: provider.attempts,
			ResponseBytes: len(provider.failContent),
		}, failure
	}
	request := provider.requests[provider.calls-1]
	response := mechanismstudy.Response{
		Version:    mechanismstudy.ResultVersion,
		CatalogRef: request.CatalogRef, CatalogSHA256: request.CatalogSHA256,
		RequestRef: request.RequestRef,
		Cards:      make([]mechanismstudy.ResponseCard, 0, len(request.Cards)),
	}
	for _, card := range request.Cards {
		response.Cards = append(response.Cards, mechanismstudy.ResponseCard{
			CardRef:    card.Ref,
			Mechanisms: studyInvestigationRuntimeCandidates(card),
		})
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	return modelresearch.ProviderResult{
		Content: raw, Attempts: 1, ResponseBytes: len(raw),
	}, nil
}

func studyInvestigationRuntimeCandidates(card mechanismstudy.Card) []mechanismstudy.Candidate {
	readingRoots := make(map[string]struct{})
	for _, reading := range card.Readings {
		if reading.RootNodeRef != "" {
			readingRoots[reading.RootNodeRef] = struct{}{}
		}
	}
	adjacency := make(map[string][]mechanismstudy.Edge)
	incoming := make(map[string]bool)
	for _, edge := range card.Edges {
		adjacency[edge.CallerRef] = append(adjacency[edge.CallerRef], edge)
		incoming[edge.CalleeRef] = true
	}
	var search func(string, []string, map[string]bool, bool) []string
	search = func(node string, edges []string, seen map[string]bool, tied bool) []string {
		if _, currentTied := readingRoots[node]; currentTied {
			tied = true
		}
		if len(edges) >= 2 && tied {
			return edges
		}
		if len(edges) == mechanismstudy.MaxEdgesPerMechanism {
			return nil
		}
		for _, edge := range adjacency[node] {
			if seen[edge.CalleeRef] {
				continue
			}
			nextSeen := make(map[string]bool, len(seen)+1)
			for ref, present := range seen {
				nextSeen[ref] = present
			}
			nextSeen[edge.CalleeRef] = true
			if path := search(edge.CalleeRef, append(append([]string{}, edges...), edge.Ref), nextSeen, tied); len(path) > 0 {
				return path
			}
		}
		return nil
	}
	for _, node := range card.Nodes {
		if incoming[node.Ref] {
			continue
		}
		if path := search(node.Ref, nil, map[string]bool{node.Ref: true}, false); len(path) > 0 {
			return []mechanismstudy.Candidate{{EdgeRefs: path}}
		}
	}
	return []mechanismstudy.Candidate{}
}

func TestStudyInvestigationZeroGraphWritesCompletePreparedFamilyWithoutProvider(t *testing.T) {
	runDir, index, _, target := studyInvestigationRuntimeFixture(t, 2)
	rewriteStudyInvestigationReadings(t, runDir, index, "detached")
	factoryCalls := 0
	outcome, err := runStudyInvestigationForRun(
		context.Background(),
		runDir,
		index,
		target,
		studyInvestigationTestRevision,
		studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) {
			factoryCalls++
			return nil, errors.New("provider must not be configured")
		},
	)
	if err != nil {
		t.Fatalf("runStudyInvestigationForRun: %v", err)
	}
	if factoryCalls != 0 || outcome.Status.State != mechanismstudy.StatusComplete ||
		outcome.Status.ProviderCallCount != 0 || outcome.Status.PreparedCardCount != 2 ||
		len(outcome.ReportInput.Cards) != 2 {
		t.Fatalf("zero-graph outcome = %#v; factory calls=%d", outcome, factoryCalls)
	}
	for _, card := range outcome.ReportInput.Cards {
		if card.Outcome != "prepared_investigation" || len(card.Mechanisms) != 0 {
			t.Fatalf("prepared report card = %#v", card)
		}
	}
	assertStudyInvestigationArtifacts(t, runDir)
}

func TestStudyInvestigationRetainsAcceptedPrefixWhenSecondBatchFails(t *testing.T) {
	runDir, index, _, target := studyInvestigationRuntimeFixture(t, 5)
	provider := &studyInvestigationRuntimeProvider{failCall: 2, attempts: 1}
	factoryCalls := 0
	outcome, err := runStudyInvestigationForRun(
		context.Background(),
		runDir,
		index,
		target,
		studyInvestigationTestRevision,
		studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) {
			factoryCalls++
			return provider, nil
		},
	)
	if err != nil {
		t.Fatalf("runStudyInvestigationForRun: %v", err)
	}
	if factoryCalls != 1 || provider.calls != 2 ||
		outcome.Status.State != mechanismstudy.StatusPartial ||
		outcome.Status.AcceptedBatchCount != 1 || outcome.Status.FailedBatchCount != 1 ||
		outcome.Status.MechanismCardCount != 4 || outcome.Status.PreparedCardCount != 1 ||
		outcome.SemanticCalls != 2 || outcome.TransportAttempts != 2 {
		t.Fatalf("partial outcome = %#v; provider=%#v factory=%d", outcome.Status, provider, factoryCalls)
	}
	mechanismCards := 0
	for _, card := range outcome.ReportInput.Cards {
		if card.Outcome == "mechanism" {
			mechanismCards++
			if len(card.Mechanisms) != 1 || len(card.Mechanisms[0].Nodes) != 3 ||
				len(card.Mechanisms[0].Edges) != 2 {
				t.Fatalf("published mechanism card = %#v", card)
			}
		}
	}
	if mechanismCards != 4 {
		t.Fatalf("published mechanism cards = %d, want 4", mechanismCards)
	}
	assertStudyInvestigationArtifacts(t, runDir)
}

func TestStudyInvestigationConfigurationAndCancellationPersistFailedPreparedFamily(t *testing.T) {
	for _, test := range []struct {
		name        string
		context     func() context.Context
		factory     studyInvestigationClientFactory
		wantFactory int
		wantCalls   int
		wantAttempt int
	}{
		{
			name:    "configuration failure",
			context: context.Background,
			factory: func() (studyInvestigationClient, error) {
				return nil, errors.New("fixture configuration failed")
			},
			wantFactory: 1,
		},
		{
			name: "canceled before provider construction",
			context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			factory: func() (studyInvestigationClient, error) {
				return nil, errors.New("provider must not be constructed")
			},
		},
		{
			name:    "canceled attempted call with no transport attempt",
			context: context.Background,
			factory: func() (studyInvestigationClient, error) {
				return &studyInvestigationRuntimeProvider{
					failCall: 1, failErr: context.Canceled, attempts: 0,
				}, nil
			},
			wantFactory: 1,
			wantCalls:   1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir, index, _, target := studyInvestigationRuntimeFixture(t, 1)
			factoryCalls := 0
			outcome, err := runStudyInvestigationForRun(
				test.context(), runDir, index, target,
				studyInvestigationTestRevision, studyInvestigationTestFreshness,
				newRunOutput(io.Discard),
				func() (studyInvestigationClient, error) {
					factoryCalls++
					return test.factory()
				},
			)
			if err != nil {
				t.Fatalf("runStudyInvestigationForRun: %v", err)
			}
			if factoryCalls != test.wantFactory || outcome.SemanticCalls != test.wantCalls ||
				outcome.TransportAttempts != test.wantAttempt ||
				outcome.Status.State != mechanismstudy.StatusFailed ||
				outcome.Status.PreparedCardCount != 1 || outcome.Status.MechanismCardCount != 0 ||
				len(outcome.ReportInput.Cards) != 1 ||
				outcome.ReportInput.Cards[0].Outcome != report.StudyInvestigationOutcomePrepared {
				t.Fatalf("outcome = %#v; factory=%d", outcome, factoryCalls)
			}
			assertStudyInvestigationArtifacts(t, runDir)
		})
	}
}

func TestStudyInvestigationCancellationGetsOnlyBoundedPublicationContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ordinary, cancelOrdinary := studyInvestigationPublicationContext(
		parent,
		mechanismstudy.Status{State: mechanismstudy.StatusFailed},
	)
	defer cancelOrdinary()
	if !errors.Is(ordinary.Err(), context.Canceled) {
		t.Fatalf("ordinary context error = %v, want parent cancellation", ordinary.Err())
	}

	publication, cancelPublication := studyInvestigationPublicationContext(
		parent,
		mechanismstudy.Status{
			State:   mechanismstudy.StatusFailed,
			Batches: []mechanismstudy.BatchExecution{{State: mechanismstudy.BatchCanceled}},
		},
	)
	defer cancelPublication()
	if err := publication.Err(); err != nil {
		t.Fatalf("detached bounded publication context starts canceled: %v", err)
	}
	if _, hasDeadline := publication.Deadline(); !hasDeadline {
		t.Fatal("detached publication context has no deadline")
	}
}

func TestStudyInvestigationProviderFailureSecretKeepsExactCallJournaledAndRedacted(t *testing.T) {
	runDir, index, _, target := studyInvestigationRuntimeFixture(t, 1)
	const secret = "Bearer company-secret-token-value"
	provider := &studyInvestigationRuntimeProvider{
		failCall: 1, attempts: 1, failErr: errors.New("fixture provider failed"),
		failContent: []byte(`{"error":"` + secret + `"}`),
	}
	outcome, err := runStudyInvestigationForRun(
		context.Background(), runDir, index, target,
		studyInvestigationTestRevision, studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) { return provider, nil },
	)
	if err == nil || !strings.Contains(err.Error(), "credential scan") ||
		outcome.SemanticCalls != 1 || outcome.TransportAttempts != 1 {
		t.Fatalf("secret provider failure outcome = %#v, err=%v", outcome, err)
	}
	entries := readSemanticJournalEntries(t, runDir)
	if len(entries) != 1 || entries[0].record.Stage != debugdump.SemanticStageMechanismStudy ||
		entries[0].record.State != debugdump.SemanticStateRejected ||
		entries[0].record.ValidationCode != debugdump.SemanticValidationSecret ||
		entries[0].record.SemanticCalls != 1 || entries[0].record.TransportAttempts != 1 {
		t.Fatalf("secret exchange = %#v", entries)
	}
	if strings.Contains(string(entries[0].response), secret) {
		t.Fatal("provider credential leaked into semantic exchange response")
	}
}

func TestStudyInvestigationRuntimeRepomapTargetExcludesOtherExecutable(t *testing.T) {
	const modulePath = "example.com/repomap"
	target := resolveStudyInvestigationTarget(t, gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: modulePath, ModuleDir: ".", Main: true,
		}},
		Packages: []gofacts.PackageFact{
			{CanonicalPath: modulePath + "/cmd/repomap", Name: "main", ModuleID: "module-root", ModulePath: modulePath, PackageDir: "cmd/repomap", Locality: "local"},
			{CanonicalPath: modulePath + "/cmd/quality", Name: "main", ModuleID: "module-root", ModulePath: modulePath, PackageDir: "cmd/quality", Locality: "local"},
		},
		EntrypointPackages: []gofacts.Entrypoint{
			studyInvestigationEntrypoint(modulePath, modulePath+"/cmd/repomap", "cmd/repomap", "cmd/repomap/main.go", 3),
			studyInvestigationEntrypoint(modulePath, modulePath+"/cmd/quality", "cmd/quality", "cmd/quality/main.go", 3),
		},
	}, "cmd/repomap")
	_, index := analyzeStudyInvestigationRuntimeRepository(t, modulePath, target, []string{
		modulePath + "/cmd/repomap",
		modulePath + "/internal/app",
		modulePath + "/internal/report",
	}, map[string]string{
		"cmd/repomap/main.go": `package main
import "example.com/repomap/internal/app"
func main() { app.Run() }
`,
		"cmd/quality/main.go": `package main
import "example.com/repomap/internal/quality"
func main() { quality.Run() }
`,
		"internal/app/app.go": `package app
import "example.com/repomap/internal/report"
func Run() { report.Read() }
`,
		"internal/quality/quality.go": `package quality
import "example.com/repomap/internal/report"
func Run() { report.Read() }
`,
		"internal/report/report.go": `package report
func Read() { decode() }
func decode() { render() }
func render() {}
`,
	})
	runDir := writeStudyInvestigationRuntimeThemes(
		t, index, modulePath+"/internal/report.Read", 1,
	)
	provider := &studyInvestigationRuntimeProvider{}
	outcome, err := runStudyInvestigationForRun(
		context.Background(), runDir, index, target,
		studyInvestigationTestRevision, studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) { return provider, nil },
	)
	if err != nil {
		t.Fatalf("run target-rooted repomap Study: %v", err)
	}
	if provider.calls != 1 || outcome.Status.MechanismCount != 1 {
		t.Fatalf("repomap target outcome=%#v provider=%#v", outcome.Status, provider)
	}
	assertStudyInvestigationTargetBinding(t, runDir, target)
	for _, request := range provider.requests {
		for _, card := range request.Cards {
			for _, node := range card.Nodes {
				if strings.Contains(node.Label, "quality") {
					t.Fatalf("off-target quality executable reached provider: %#v", card.Nodes)
				}
			}
		}
	}
	path := outcome.ReportInput.Cards[0].Mechanisms[0]
	if len(path.Nodes) < 3 || path.Nodes[0].Symbol != modulePath+"/cmd/repomap.main" {
		t.Fatalf("repomap path does not start at selected exact main: %#v", path.Nodes)
	}
	for _, node := range path.Nodes {
		if strings.Contains(node.Symbol, "/cmd/quality.") || strings.Contains(node.Symbol, "/internal/quality.") {
			t.Fatalf("off-target executable entered restored path: %#v", path.Nodes)
		}
	}
}

func TestStudyInvestigationRuntimeTelebotLibraryUsesExactPublicAPIRoot(t *testing.T) {
	const modulePath = "gopkg.in/telebot.v3"
	target := resolveStudyInvestigationTarget(t, gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: modulePath, ModuleDir: ".", Main: true,
			PackagesCount: 1, RetainedPackagesCount: 1,
			Coverage: gofacts.ModuleCoverage{
				State: gofacts.CoverageComplete, PackagesDiscovered: 1, PackagesRetained: 1,
			},
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: modulePath, Name: "telebot", ModuleID: "module-root",
			ModulePath: modulePath, PackageDir: ".", ModuleRelativeDir: ".", Locality: "local",
			DeclarationsScanned: true, LoadCompleteness: completeGoPackageLoad(),
			Declarations: []gofacts.PackageDeclaration{
				{Kind: gofacts.PackageDeclarationType, Name: "Bot", Path: "bot.go", Line: 2, Column: 6},
				{Kind: gofacts.PackageDeclarationFunc, Name: "NewBot", Path: "bot.go", Line: 3, Column: 6},
			},
		}},
		PackagesCount: 1, RetainedPackagesCount: 1,
		Coverage: gofacts.Coverage{
			State: gofacts.CoverageComplete, ModulesDiscovered: 1, ModulesAvailable: 1,
			PackagesDiscovered: 1, PackagesRetained: 1,
		},
	}, modulePath)
	if target.Kind != analysistarget.KindModuleLibrary || len(target.LibraryPackages) != 1 {
		t.Fatalf("target=%#v, want one-package module library", target)
	}
	_, index := analyzeStudyInvestigationRuntimeRepository(t, modulePath, target, []string{modulePath}, map[string]string{
		"bot.go": `package telebot
type Bot struct{}
func NewBot() *Bot { return configure() }
func configure() *Bot { connect(); return &Bot{} }
func connect() {}
func (*Bot) Raw() { request() }
func request() {}
func hidden() {}
`,
	})
	if index.Scope.TargetKind != surfacediscovery.AnalysisTargetModuleLibrary ||
		index.Scope.TargetPackage != "" || len(index.Scope.TargetPackages) != 1 ||
		index.Scope.TargetPackages[0] != modulePath {
		t.Fatalf("Telebot DirectCallIndex lost plural module-library scope: %#v", index.Scope)
	}
	boundRoots, err := analysistarget.BindExactRoots(target, index)
	if err != nil {
		t.Fatalf("bind Telebot module-library roots: %v", err)
	}
	sawNewBot := false
	for _, root := range boundRoots.Roots {
		if root.Package != modulePath {
			t.Fatalf("Telebot root escaped selected module API package: %#v", root)
		}
		sawNewBot = sawNewBot || root.Symbol == modulePath+".NewBot"
	}
	if boundRoots.Version != analysistarget.TargetRootsVersion || !sawNewBot {
		t.Fatalf("Telebot plural module-library roots = %#v", boundRoots)
	}
	runDir := writeStudyInvestigationRuntimeThemes(t, index, modulePath+".configure", 1)
	provider := &studyInvestigationRuntimeProvider{}
	outcome, err := runStudyInvestigationForRun(
		context.Background(), runDir, index, target,
		studyInvestigationTestRevision, studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) { return provider, nil },
	)
	if err != nil {
		t.Fatalf("run target-rooted Telebot Study: %v", err)
	}
	if provider.calls != 1 || outcome.Status.MechanismCount != 1 {
		t.Fatalf("Telebot target outcome=%#v provider=%#v", outcome.Status, provider)
	}
	assertStudyInvestigationTargetBinding(t, runDir, target)
	path := outcome.ReportInput.Cards[0].Mechanisms[0]
	if len(path.Nodes) < 3 || path.Nodes[0].Symbol != modulePath+".NewBot" {
		t.Fatalf("Telebot path does not start at exact exported API: %#v", path.Nodes)
	}
	for _, node := range path.Nodes {
		if node.Symbol == modulePath+".main" || strings.HasSuffix(node.Symbol, ".hidden") {
			t.Fatalf("library path invented main/private API root: %#v", path.Nodes)
		}
	}
}

func TestStudyInvestigationRuntimeRejectsUnavailableDirectCallAuthority(t *testing.T) {
	runDir, _, _, target := studyInvestigationRuntimeFixture(t, 1)
	factoryCalls := 0
	_, err := runStudyInvestigationForRun(
		context.Background(), runDir, nil, target,
		studyInvestigationTestRevision, studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) {
			factoryCalls++
			return &studyInvestigationRuntimeProvider{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "direct call index is nil") || factoryCalls != 0 {
		t.Fatalf("unavailable index: err=%v factoryCalls=%d", err, factoryCalls)
	}
	for _, name := range mechanismstudy.ArtifactFilenames {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("unavailable target runtime retained %s: %v", name, statErr)
		}
	}
}

func assertStudyInvestigationTargetBinding(
	t *testing.T,
	runDir string,
	target analysistarget.Target,
) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runDir, mechanismstudy.FactsArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := mechanismstudy.DecodeFacts(raw)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Compilation.TargetTrailVersion != mechanismstudy.TargetTrailVersion ||
		facts.Compilation.AnalysisTargetRef != target.Ref ||
		facts.Compilation.TargetRootsSHA256 == "" {
		t.Fatalf("persisted runtime lost target authority: %#v", facts.Compilation)
	}
}

func analyzeStudyInvestigationRuntimeRepository(
	t *testing.T,
	modulePath string,
	target analysistarget.Target,
	admittedPackages []string,
	files map[string]string,
) (string, *surfacediscovery.DirectCallIndex) {
	t.Helper()
	repository := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(repository, "go.mod"),
		[]byte("module "+modulePath+"\n\ngo 1.25\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	for relative, content := range files {
		path := filepath.Join(repository, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	packageInputs := make([]surfacediscovery.PackageInput, 0, len(admittedPackages))
	for _, packagePath := range admittedPackages {
		packageInputs = append(packageInputs, surfacediscovery.PackageInput{
			Path: packagePath, ModuleDir: target.ModuleDir,
		})
	}
	targetInput := &surfacediscovery.AnalysisTargetInput{
		TargetRef: target.Ref, Kind: analysisTargetSurfaceKind(target),
		ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
		TargetPackages: analysisTargetRootPackagePaths(target),
	}
	if target.Kind == analysistarget.KindExecutablePackage {
		targetInput.PackagePath = target.PackagePath
		for _, root := range target.Roots {
			targetInput.Roots = append(targetInput.Roots, surfacediscovery.AnalysisTargetRootInput{
				Path: root.Path, Line: root.Line,
			})
		}
	}
	analysis, err := surfacediscovery.AnalyzeContextWithInput(
		context.Background(), surfacediscovery.DefaultOptions(repository), surfacediscovery.Input{
			RepositoryName: filepath.Base(modulePath), ModuleDirs: []string{target.ModuleDir},
			Packages: packageInputs, AnalysisTarget: targetInput,
		},
	)
	if err != nil {
		t.Fatalf("AnalyzeContext: %v", err)
	}
	if analysis.DirectCallIndex == nil {
		t.Fatal("AnalyzeContext returned no DirectCallIndex")
	}
	return repository, analysis.DirectCallIndex
}

func studyInvestigationEntrypoint(
	modulePath, importPath, packageDir, sourcePath string,
	line int,
) gofacts.Entrypoint {
	return gofacts.Entrypoint{
		ModulePath: modulePath, ImportPath: importPath, PackageDir: packageDir, ModuleDir: ".",
		Kind: "primary_binary",
		Anchors: []gofacts.EntrypointAnchor{{
			Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
			Path: sourcePath, Line: line,
		}},
	}
}

func resolveStudyInvestigationTarget(
	t *testing.T,
	facts gofacts.Facts,
	override string,
) analysistarget.Target {
	t.Helper()
	resolution, err := analysistarget.Resolve(facts, analysistarget.Options{Override: override})
	if err != nil || resolution.Selected == nil {
		t.Fatalf("resolve exact target %q: resolution=%#v err=%v", override, resolution, err)
	}
	return resolution.Selected.Snapshot()
}

func writeStudyInvestigationRuntimeThemes(
	t *testing.T,
	index *surfacediscovery.DirectCallIndex,
	readingSymbol string,
	cardCount int,
) string {
	t.Helper()
	var reading surfacediscovery.DirectCallNode
	for _, node := range index.Nodes {
		if node.Symbol.ID == readingSymbol {
			reading = node
			break
		}
	}
	if reading.ID == "" {
		t.Fatalf("reading %q absent from DirectCallIndex: %#v", readingSymbol, index.Nodes)
	}
	cards := make([]themestudy.ThemeCard, 0, cardCount)
	for ordinal := 1; ordinal <= cardCount; ordinal++ {
		cards = append(cards, themestudy.ThemeCard{
			Ordinal: ordinal, CanonicalID: "runtime-theme-" + string(rune('a'+ordinal-1)),
			FinalTitle: "Runtime path", FinalQuestion: "How does the selected product reach this code?",
			Readings: []themestudy.Reading{{
				Label: reading.Symbol.Name, Symbol: reading.Symbol.ID,
				Path: reading.Declaration.Path, Line: reading.Declaration.Line,
				Fit: themestudy.FitDirect, CanonicalSpanID: "runtime-span-" + string(rune('a'+ordinal-1)),
			}},
		})
	}
	raw, err := themestudy.EncodeStudyThemes(themestudy.StudyThemes{
		Version: themestudy.StudyThemesVersion, Revision: studyInvestigationTestRevision,
		ScoutSHA256: strings.Repeat("a", 64), AdjSHA256: strings.Repeat("b", 64),
		Cards: cards,
	})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := debugdump.NewWriter(t.TempDir(), "run", false)
	if err != nil {
		t.Fatal(err)
	}
	runDir := writer.RunDir()
	writer.Close()
	if err := os.WriteFile(filepath.Join(runDir, themestudy.StudyThemesArtifactFilename), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func studyInvestigationRuntimeFixture(
	t *testing.T,
	cardCount int,
) (string, *surfacediscovery.DirectCallIndex, surfacediscovery.DirectCallNode, analysistarget.Target) {
	t.Helper()
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example.com/investigation\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(`package main

func main() { entry() }
func entry() { service() }
func service() { persist() }
func persist() {}
func detached() {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	target := resolveStudyInvestigationExecutableTarget(t)
	targetInput := &surfacediscovery.AnalysisTargetInput{
		TargetRef: target.Ref, Kind: analysisTargetSurfaceKind(target),
		ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
		PackagePath: target.PackagePath, TargetPackages: analysisTargetRootPackagePaths(target),
	}
	for _, root := range target.Roots {
		targetInput.Roots = append(targetInput.Roots, surfacediscovery.AnalysisTargetRootInput{
			Path: root.Path, Line: root.Line,
		})
	}
	analysis, err := surfacediscovery.AnalyzeContextWithInput(
		context.Background(), surfacediscovery.DefaultOptions(repository), surfacediscovery.Input{
			RepositoryName: "investigation", ModuleDirs: []string{"."},
			Packages:       []surfacediscovery.PackageInput{{Path: target.PackagePath, ModuleDir: target.ModuleDir}},
			AnalysisTarget: targetInput,
		},
	)
	if err != nil {
		t.Fatalf("AnalyzeContext: %v", err)
	}
	if analysis.DirectCallIndex == nil {
		t.Fatal("AnalyzeContext returned no DirectCallIndex")
	}
	var root surfacediscovery.DirectCallNode
	for _, node := range analysis.DirectCallIndex.Nodes {
		if node.Symbol.Name == "entry" {
			root = node
			break
		}
	}
	if root.ID == "" {
		t.Fatalf("entry node absent: %#v", analysis.DirectCallIndex.Nodes)
	}
	cards := make([]themestudy.ThemeCard, 0, cardCount)
	for ordinal := 1; ordinal <= cardCount; ordinal++ {
		cards = append(cards, themestudy.ThemeCard{
			Ordinal: ordinal, CanonicalID: "fixture-theme-" + string(rune('a'+ordinal-1)),
			FinalTitle: "Bearer authentication", FinalQuestion: "How does startup reach persistence?",
			Readings: []themestudy.Reading{{
				Label: root.Symbol.Name, Symbol: root.Symbol.ID,
				Path: root.Declaration.Path, Line: root.Declaration.Line,
				Fit: themestudy.FitDirect,
			}},
		})
	}
	themesRaw, err := themestudy.EncodeStudyThemes(themestudy.StudyThemes{
		Version: themestudy.StudyThemesVersion, Revision: studyInvestigationTestRevision,
		ScoutSHA256: strings.Repeat("a", 64), AdjSHA256: strings.Repeat("b", 64),
		Cards: cards,
	})
	if err != nil {
		t.Fatalf("EncodeStudyThemes: %v", err)
	}
	writer, err := debugdump.NewWriter(t.TempDir(), "run", false)
	if err != nil {
		t.Fatal(err)
	}
	runDir := writer.RunDir()
	writer.Close()
	if err := os.WriteFile(
		filepath.Join(runDir, themestudy.StudyThemesArtifactFilename),
		themesRaw,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return runDir, analysis.DirectCallIndex, root, target
}

func rewriteStudyInvestigationReadings(
	t *testing.T,
	runDir string,
	index *surfacediscovery.DirectCallIndex,
	symbol string,
) {
	t.Helper()
	var selected surfacediscovery.DirectCallNode
	for _, node := range index.Nodes {
		if node.Symbol.Name == symbol {
			selected = node
			break
		}
	}
	if selected.ID == "" {
		t.Fatalf("reading symbol %q absent from DirectCallIndex", symbol)
	}
	path := filepath.Join(runDir, themestudy.StudyThemesArtifactFilename)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	themes, err := themestudy.DecodeStudyThemes(raw)
	if err != nil {
		t.Fatal(err)
	}
	for position := range themes.Cards {
		themes.Cards[position].Readings = []themestudy.Reading{{
			Label: selected.Symbol.Name, Symbol: selected.Symbol.ID,
			Path: selected.Declaration.Path, Line: selected.Declaration.Line,
			Fit: themestudy.FitDirect,
		}}
	}
	raw, err = themestudy.EncodeStudyThemes(themes)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func resolveStudyInvestigationExecutableTarget(t *testing.T) analysistarget.Target {
	t.Helper()
	const modulePath = "example.com/investigation"
	const moduleID = "module-root"
	resolution, err := analysistarget.Resolve(gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: moduleID, ModulePath: modulePath, ModuleDir: ".", Main: true,
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: modulePath, Name: "main", ModuleID: moduleID,
			ModulePath: modulePath, PackageDir: ".", Locality: "local",
		}},
		EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: modulePath, ImportPath: modulePath, PackageDir: ".", ModuleDir: ".",
			Kind: "primary_binary",
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion,
				Kind:    gofacts.EntrypointAnchorGoMain,
				Path:    "main.go", Line: 3,
			}},
		}},
	}, analysistarget.Options{})
	if err != nil || resolution.Selected == nil {
		t.Fatalf("resolve exact executable target: resolution=%#v err=%v", resolution, err)
	}
	return resolution.Selected.Snapshot()
}

func assertStudyInvestigationArtifacts(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range mechanismstudy.ArtifactFilenames {
		info, err := os.Stat(filepath.Join(runDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			t.Fatalf("artifact %s: info=%v err=%v", name, info, err)
		}
	}
}
