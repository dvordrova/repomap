package main

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestFreshPrimaryIntentKeyIsCandidateSpecificAndStable(t *testing.T) {
	t.Parallel()
	base := freshPrimaryIntentInput{
		RepositoryNamespace: "example.test/repo",
		Question:            "How does the monitor dispatch work?",
		Kind:                semanticdiscovery.ArtifactMechanism,
		Scope: semanticdiscovery.MechanismScope{
			Kind:  semanticdiscovery.MechanismScopeGoPackage,
			Value: "example.test/repo/core",
		},
		CentralAnchorIDs: []string{"anchor-b", "anchor-a"},
	}
	want := freshPrimaryIntentKey(base)
	reordered := base
	reordered.CentralAnchorIDs = []string{"anchor-a", "anchor-b"}
	if got := freshPrimaryIntentKey(reordered); got != want {
		t.Fatalf("reordered anchors changed identity: got %q want %q", got, want)
	}
	changedQuestion := base
	changedQuestion.Question = "How does restore dispatch work?"
	if got := freshPrimaryIntentKey(changedQuestion); got == want {
		t.Fatal("changed question did not change identity")
	}
	changedAnchors := base
	changedAnchors.CentralAnchorIDs = []string{"anchor-c"}
	if got := freshPrimaryIntentKey(changedAnchors); got == want {
		t.Fatal("changed candidate anchors did not change identity")
	}
	changedRepository := base
	changedRepository.RepositoryNamespace = "example.test/other"
	if got := freshPrimaryIntentKey(changedRepository); got == want {
		t.Fatal("changed repository namespace did not change identity")
	}
	// Model titles and unrelated repository facts are deliberately absent from
	// the identity input, so changing either cannot affect this hash.
	if got := freshPrimaryIntentKey(base); got != want {
		t.Fatalf("unrelated state changed identity: got %q want %q", got, want)
	}
}

func TestFreshPrimaryAspectsRequireInputCoreAndEffect(t *testing.T) {
	t.Parallel()
	aspects := freshPrimaryAspects("How does the selected operation work?")
	if len(aspects) != 3 {
		t.Fatalf("aspect count = %d, want 3", len(aspects))
	}
	want := []freshPrimaryAspectRole{
		freshPrimaryRoleInput,
		freshPrimaryRoleCore,
		freshPrimaryRoleEffect,
	}
	for index := range aspects {
		if aspects[index].Role != want[index] || !aspects[index].Key {
			t.Fatalf("aspect[%d] = %+v", index, aspects[index])
		}
	}
}

func TestDeriveFreshPrimaryEligibility(t *testing.T) {
	t.Parallel()
	boundary := &freshPrimaryEffectBoundary{
		EvidenceID: "evidence-effect",
		Kind:       freshBoundaryFileWrite,
	}
	plan := &freshPrimaryProbePlan{
		RootAnchors:    []freshPrimaryAnchor{{ID: "anchor"}},
		EffectBoundary: boundary,
	}
	fact := func(
		id string,
		aspect string,
		statement string,
		path string,
		symbol string,
		capabilities []semanticdiscovery.Capability,
		evidenceID string,
	) semanticdiscovery.Fact {
		return semanticdiscovery.Fact{
			ID: id, Statement: statement,
			Keywords:     []string{"answer_aspect:" + aspect},
			Capabilities: capabilities,
			Source: &semanticdiscovery.FactSource{
				Path: path, EnclosingSymbol: symbol,
			},
			Evidence: []semanticdiscovery.EvidenceRef{{ID: evidenceID}},
		}
	}
	input := fact("input", freshPrimaryAspectInput, "entry", "entry.go", "Command.Run",
		[]semanticdiscovery.Capability{semanticdiscovery.CapabilityEntry}, "entry-evidence")
	core := fact("core", freshPrimaryAspectCore, "repository operation", "core.go", "Worker.Apply",
		[]semanticdiscovery.Capability{
			semanticdiscovery.CapabilityBehavior,
			semanticdiscovery.CapabilityDirectCall,
		}, "core-evidence")
	effect := fact("effect", freshPrimaryAspectEffect, "file write", "core.go", "Worker.Apply",
		[]semanticdiscovery.Capability{semanticdiscovery.CapabilityOutputEffect}, "evidence-effect")

	tests := []struct {
		name      string
		facts     []semanticdiscovery.Fact
		collision bool
		want      freshPrimaryPlanStatus
		reason    string
	}{
		{name: "complete", facts: []semanticdiscovery.Fact{input, core, effect}, want: freshPrimaryPlanReady},
		{name: "branch cannot replace effect", facts: []semanticdiscovery.Fact{input, core}, want: freshPrimaryPlanInsufficient, reason: "observable_effect_fact_missing"},
		{name: "collision", facts: []semanticdiscovery.Fact{input, core, effect}, collision: true, want: freshPrimaryPlanInsufficient, reason: "intent_key_collision"},
	}
	loggerCore := core
	loggerCore.Statement = "The function calls its logger."
	tests = append(tests, struct {
		name      string
		facts     []semanticdiscovery.Fact
		collision bool
		want      freshPrimaryPlanStatus
		reason    string
	}{name: "logger cannot satisfy core", facts: []semanticdiscovery.Fact{input, loggerCore, effect}, want: freshPrimaryPlanInsufficient, reason: "core_work_fact_missing"})
	localEffect := effect
	localEffect.Evidence = append([]semanticdiscovery.EvidenceRef(nil), effect.Evidence...)
	localEffect.Evidence[0].ID = "local-call"
	tests = append(tests, struct {
		name      string
		facts     []semanticdiscovery.Fact
		collision bool
		want      freshPrimaryPlanStatus
		reason    string
	}{name: "local call cannot satisfy effect", facts: []semanticdiscovery.Fact{input, core, localEffect}, want: freshPrimaryPlanInsufficient, reason: "observable_effect_fact_missing"})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := deriveFreshPrimaryEligibility(plan, test.facts, test.collision)
			if got.Status != test.want {
				t.Fatalf("status = %q, want %q; reasons=%v", got.Status, test.want, got.Reasons)
			}
			if test.reason != "" && !containsFreshString(got.Reasons, test.reason) {
				t.Fatalf("reasons = %v, want %q", got.Reasons, test.reason)
			}
		})
	}
}

func TestFreshPrimaryPlannerReachesTypedBoundaryWithinBounds(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	commandSource := `package demo
type JobCommand struct { job *Job }
func (c *JobCommand) Run() { c.job.Apply() }
`
	jobSource := `package demo
type BackendClient interface { Write() error }
type Job struct { Client BackendClient }
func (j *Job) Apply() { _ = j.Client.Write() }
`
	writeTestFile(t, repoRoot, "command.go", commandSource)
	writeTestFile(t, repoRoot, "job.go", jobSource)
	source := testFreshSourceFunction(t, "command.go", commandSource, "JobCommand.Run")
	candidate := semanticdiscovery.OpportunityCandidate{
		ID:               "semantic-candidate-1234567890abcdef",
		Kind:             semanticdiscovery.ArtifactMechanism,
		Title:            "Model-written title",
		QuestionAnswered: "How does the job command apply records to the backend?",
		SupportIDs:       []string{source.Fact.ID},
		ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
		Confidence:       semanticdiscovery.ConfidenceHigh,
	}
	data := &report.ReportData{
		RepoName: "demo",
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{{
			CanonicalPath: "example.test/demo", Name: "demo",
			ModulePath: "example.test/demo", Locality: "local",
			Files: []string{"command.go", "job.go"},
		}}},
	}
	work := planFreshPrimaryCandidate(repoRoot, data, candidate, []freshSourceFunction{source})
	primary := work.Plan.Primary
	if primary.Status != freshPrimaryPlanReady {
		t.Fatalf("status = %q, reasons=%v", primary.Status, primary.StatusReasons)
	}
	if len(primary.SelectedFrontiers) != 1 {
		t.Fatalf("selected frontiers = %d, want 1", len(primary.SelectedFrontiers))
	}
	if got := primary.SelectedFrontiers[0].TargetSymbol; got != "Job.Apply" {
		t.Fatalf("selected target = %q, want Job.Apply", got)
	}
	if primary.EffectBoundary == nil || primary.EffectBoundary.Kind != freshBoundaryBackendInterface {
		t.Fatalf("effect boundary = %+v", primary.EffectBoundary)
	}
	if len(primary.AdditionalFilesRead) > freshPrimaryMaxAdditionalFiles ||
		len(primary.SelectedFrontiers) > freshPrimaryMaxPathFrontiers ||
		primary.RetainedSourceBytes > freshPrimaryMaxRetainedBytes {
		t.Fatalf("planner exceeded bounds: %+v", primary)
	}
	if primary.Limits.MaxFiles != 8 || primary.Limits.MaxFunctions != 10 ||
		primary.Limits.MaxDepth != 3 || primary.Limits.MaxFrontierExpansions != 2 ||
		primary.Limits.MaxAdditionalFiles != 4 || primary.Limits.MaxAdditionalFuncs != 6 ||
		primary.Limits.MaxRetainedBytes != 128<<10 || primary.Limits.Timeout != 5*time.Second {
		t.Fatalf("planner limits = %+v", primary.Limits)
	}
	if work.Plan.Probe.Limits.MaxFiles > 6 || work.Plan.Probe.Limits.MaxFunctions > 15 ||
		work.Plan.Probe.Limits.MaxDepth > 3 || work.Plan.Probe.Limits.MaxSourceBytes > 128<<10 ||
		work.Plan.Probe.Limits.Timeout <= 0 || work.Plan.Probe.Limits.Timeout > 5*time.Second {
		t.Fatalf("probe limits = %+v", work.Plan.Probe.Limits)
	}
	if work.Plan.Probe.Limits.Timeout >= freshPrimaryTimeout {
		t.Fatalf("probe timeout = %s, want planner time deducted from %s", work.Plan.Probe.Limits.Timeout, freshPrimaryTimeout)
	}
}

func TestFreshPrimaryPlannerResolvesRangeReceiverAcrossPackage(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "cmd", "demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	commandSource := `package command
import core "example.test/demo"
type Command struct { store *core.Store }
func (c *Command) Run() { c.runOnce() }
func (c *Command) runOnce() {
	for _, db := range c.store.DBs() {
		_ = db.Sync()
	}
}
`
	storeSource := `package demo
type Store struct { dbs []*DB }
func (s *Store) DBs() []*DB { return s.dbs }
`
	dbSource := `package demo
type Backend interface { Write([]byte) error }
type DB struct{ backend Backend }
func (db *DB) Sync() error { return db.backend.Write(nil) }
`
	writeTestFile(t, repoRoot, "cmd/demo/run.go", commandSource)
	writeTestFile(t, repoRoot, "store.go", storeSource)
	writeTestFile(t, repoRoot, "db.go", dbSource)
	source := testFreshSourceFunction(t, "cmd/demo/run.go", commandSource, "Command.Run")
	candidate := semanticdiscovery.OpportunityCandidate{
		ID:               "semantic-candidate-range-cross-package",
		Kind:             semanticdiscovery.ArtifactMechanism,
		QuestionAnswered: "How does the command sync database changes?",
		SupportIDs:       []string{source.Fact.ID},
		ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
		Confidence:       semanticdiscovery.ConfidenceHigh,
	}
	data := &report.ReportData{
		RepoName: "demo",
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{
			{
				CanonicalPath: "example.test/demo", Name: "demo",
				ModulePath: "example.test/demo", Locality: "local",
				Files: []string{"db.go", "store.go"},
			},
			{
				CanonicalPath: "example.test/demo/cmd/demo", Name: "command",
				ModulePath: "example.test/demo", Locality: "local",
				Files: []string{"cmd/demo/run.go"},
			},
		}},
	}

	work := planFreshPrimaryCandidate(repoRoot, data, candidate, []freshSourceFunction{source})
	primary := work.Plan.Primary
	if primary.Status != freshPrimaryPlanReady {
		t.Fatalf("status = %q, reasons=%v, frontiers=%+v", primary.Status, primary.StatusReasons, primary.EnumeratedFrontiers)
	}
	if primary.EffectBoundary == nil || primary.EffectBoundary.Kind != freshBoundaryBackendInterface {
		t.Fatalf("effect boundary = %+v, want backend interface", primary.EffectBoundary)
	}
	if primary.EffectResolution != "resolved_typed_boundary" || primary.EffectFailureClass != "" {
		t.Fatalf("effect resolution = %q/%q", primary.EffectResolution, primary.EffectFailureClass)
	}
	var syncFrontier *freshPrimaryFrontier
	for index := range primary.SelectedFrontiers {
		frontier := &primary.SelectedFrontiers[index]
		if frontier.TargetPath == "db.go" && frontier.TargetSymbol == "DB.Sync" {
			syncFrontier = frontier
			break
		}
	}
	if syncFrontier == nil {
		t.Fatalf("selected frontiers = %+v, want exact db.go DB.Sync target", primary.SelectedFrontiers)
	}
	if syncFrontier.ReceiverType != "core.DB" || syncFrontier.Resolution != "resolved_local" {
		t.Fatalf("sync frontier = %+v", syncFrontier)
	}
	if len(primary.SelectedFrontiers) > freshPrimaryMaxPathFrontiers || len(primary.AdditionalFilesRead) > 4 ||
		len(primary.ProjectedFacts) == 0 || primary.Limits.MaxFunctions != 10 ||
		primary.Limits.MaxAdditionalFuncs != 6 {
		t.Fatalf("planner bounds/result = %+v", primary)
	}
}

func TestFreshPrimaryPlannerFollowsOnlyTwoFrontiersAfterExactBridgeStart(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "cmd", "demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	commandSource := `package command
import core "example.test/demo"
type Command struct { store *core.Store }
func (c *Command) Run() { c.runOnce() }
func (c *Command) runOnce() {
	for _, db := range c.store.DBs() {
		_ = db.Replica.Sync()
	}
}
`
	storeSource := `package demo
type Store struct { dbs []*DB }
func (s *Store) DBs() []*DB { return s.dbs }
`
	dbSource := "package demo\n" +
		strings.Repeat("// unrelated production declaration padding\n", 2200) +
		"type DB struct{ Replica *Replica }\n"
	replicaSource := `package demo
type Replica struct{ Client DestinationClient }
func (r *Replica) Sync() error { return r.syncOnce() }
func (r *Replica) syncOnce() error { return r.upload() }
func (r *Replica) upload() error { return r.Client.Write([]byte("data")) }
`
	clientSource := `package demo
type DestinationClient interface { Write([]byte) error }
`
	writeTestFile(t, repoRoot, "cmd/demo/run.go", commandSource)
	writeTestFile(t, repoRoot, "store.go", storeSource)
	writeTestFile(t, repoRoot, "db.go", dbSource)
	writeTestFile(t, repoRoot, "replica.go", replicaSource)
	writeTestFile(t, repoRoot, "client.go", clientSource)
	entry := testFreshSourceFunction(t, "cmd/demo/run.go", commandSource, "Command.Run")
	core := testFreshSourceFunction(t, "cmd/demo/run.go", commandSource, "Command.runOnce")
	candidate := semanticdiscovery.OpportunityCandidate{
		ID:               "semantic-candidate-two-additional-frontiers",
		Kind:             semanticdiscovery.ArtifactMechanism,
		QuestionAnswered: "How does the command sync data to its destination?",
		SupportIDs:       []string{entry.Fact.ID, core.Fact.ID},
		ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
		Confidence:       semanticdiscovery.ConfidenceHigh,
	}
	data := &report.ReportData{
		RepoName: "demo",
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{
			{
				CanonicalPath: "example.test/demo", Name: "demo",
				ModulePath: "example.test/demo", Locality: "local",
				Files: []string{"client.go", "db.go", "replica.go", "store.go"},
			},
			{
				CanonicalPath: "example.test/demo/cmd/demo", Name: "command",
				ModulePath: "example.test/demo", Locality: "local",
				Files: []string{"cmd/demo/run.go"},
			},
		}},
	}

	work := planFreshPrimaryCandidate(
		repoRoot,
		data,
		candidate,
		[]freshSourceFunction{entry, core},
	)
	primary := work.Plan.Primary
	if primary.Status != freshPrimaryPlanReady {
		t.Fatalf("status = %q, reasons=%v, frontiers=%+v", primary.Status, primary.StatusReasons, primary.EnumeratedFrontiers)
	}
	if primary.EffectBoundary == nil || primary.EffectBoundary.Kind != freshBoundaryBackendInterface ||
		primary.EffectBoundary.FunctionSymbol != "Replica.upload" {
		t.Fatalf("effect boundary = %+v", primary.EffectBoundary)
	}
	if got := len(primary.SelectedFrontiers); got != freshPrimaryMaxPathFrontiers {
		t.Fatalf("selected frontiers = %d, want bridge start + two = %d: %+v", got, freshPrimaryMaxPathFrontiers, primary.SelectedFrontiers)
	}
	wantSymbols := []string{"Replica.Sync", "Replica.syncOnce", "Replica.upload"}
	for index, want := range wantSymbols {
		if got := primary.SelectedFrontiers[index].TargetSymbol; got != want {
			t.Fatalf("selected frontier[%d] = %q, want %q", index, got, want)
		}
	}
	if got := work.Plan.Probe.ExpansionAllowlist; !reflect.DeepEqual(
		got,
		[]string{"Replica.syncOnce", "Replica.upload"},
	) {
		t.Fatalf("expansion allowlist = %v", got)
	}
	if primary.Limits.MaxFrontierExpansions != 2 || primary.Limits.MaxFunctions != 10 ||
		len(primary.AdditionalFilesRead) > 4 {
		t.Fatalf("bounded plan = %+v", primary)
	}
}

func TestFreshPrimaryPlannerDoesNotTreatConcreteSyncNameAsEffect(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "cmd", "demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	commandSource := `package command
import core "example.test/demo"
type Command struct { db *core.DB }
func (c *Command) Run() { c.runOnce() }
func (c *Command) runOnce() { _ = c.db.Sync() }
`
	dbSource := `package demo
type StorageClient struct{}
func (client *StorageClient) Write(data []byte) error { return nil }
type DB struct{ client *StorageClient }
func (db *DB) Sync() error { return db.client.Write(nil) }
`
	writeTestFile(t, repoRoot, "cmd/demo/run.go", commandSource)
	writeTestFile(t, repoRoot, "db.go", dbSource)
	source := testFreshSourceFunction(t, "cmd/demo/run.go", commandSource, "Command.Run")
	candidate := semanticdiscovery.OpportunityCandidate{
		ID:               "semantic-candidate-concrete-sync-near-miss",
		Kind:             semanticdiscovery.ArtifactMechanism,
		QuestionAnswered: "How does the command sync database changes?",
		SupportIDs:       []string{source.Fact.ID},
		ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
		Confidence:       semanticdiscovery.ConfidenceHigh,
	}
	data := &report.ReportData{RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{
		{
			CanonicalPath: "example.test/demo", Name: "demo",
			ModulePath: "example.test/demo", Locality: "local", Files: []string{"db.go"},
		},
		{
			CanonicalPath: "example.test/demo/cmd/demo", Name: "command",
			ModulePath: "example.test/demo", Locality: "local", Files: []string{"cmd/demo/run.go"},
		},
	}}}

	work := planFreshPrimaryCandidate(repoRoot, data, candidate, []freshSourceFunction{source})
	primary := work.Plan.Primary
	if primary.EffectBoundary != nil {
		t.Fatalf("effect boundary = %+v, want none for no-op concrete DB.Sync", primary.EffectBoundary)
	}
	if primary.Status != freshPrimaryPlanInsufficient ||
		!containsFreshString(primary.StatusReasons, "observable_effect_fact_missing") {
		t.Fatalf("status = %q, reasons=%v", primary.Status, primary.StatusReasons)
	}
}

func TestFreshPrimaryBoundaryClassificationRequiresExactTypedOperation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		call freshPrimaryCall
		want freshPrimaryBoundaryKind
	}{
		{
			name: "os file write",
			call: freshPrimaryCall{
				Terminal: "WriteAt", ReceiverType: "os.File", ReceiverImportPath: "os",
			},
			want: freshBoundaryFileWrite,
		},
		{
			name: "fmt stdout print",
			call: freshPrimaryCall{Target: "fmt.Println", Terminal: "Println", ImportPath: "fmt"},
			want: freshBoundaryPublicOutput,
		},
		{
			name: "os stdout write",
			call: freshPrimaryCall{
				Target: "os.Stdout.Write", Terminal: "Write", ReceiverChain: "os.Stdout", ImportPath: "os",
			},
			want: freshBoundaryPublicOutput,
		},
		{
			name: "http response writer",
			call: freshPrimaryCall{
				Terminal: "Write", ReceiverType: "http.ResponseWriter", ReceiverImportPath: "net/http",
			},
			want: freshBoundaryPublicOutput,
		},
		{
			name: "sql exec",
			call: freshPrimaryCall{
				Terminal: "ExecContext", ReceiverType: "sql.DB", ReceiverImportPath: "database/sql",
			},
			want: freshBoundaryDatabaseWrite,
		},
		{
			name: "http client do",
			call: freshPrimaryCall{
				Terminal: "Do", ReceiverType: "http.Client", ReceiverImportPath: "net/http",
			},
			want: freshBoundaryNetworkSend,
		},
		{
			name: "untyped writer name is not enough",
			call: freshPrimaryCall{Target: "writer.Write", Terminal: "Write", ReceiverChain: "writer"},
		},
		{
			name: "shadowed fmt name is not enough",
			call: freshPrimaryCall{Target: "fmt.Println", Terminal: "Println"},
		},
		{
			name: "concrete client name is not enough",
			call: freshPrimaryCall{Terminal: "Write", ReceiverType: "StorageClient"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := freshPrimaryBoundaryForCall(test.call); got != test.want {
				t.Fatalf("boundary = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFreshPrimaryBoundaryInfersOSFileConstructor(t *testing.T) {
	t.Parallel()
	source := []byte(`package demo
import "os"
func write(path string) error {
	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	_, err = f.Write([]byte("data"))
	return err
}
`)
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "write.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := &freshPrimaryParsedFile{
		Path: "write.go", Data: source, File: parsed, FSet: fset,
		Functions: make(map[string]freshPrimaryFunction),
		Fields:    freshPrimaryStructFields(parsed),
		Imports:   freshPrimaryImports(parsed),
	}
	declaration := parsed.Decls[1].(*ast.FuncDecl)
	function := freshPrimaryFunction{
		Function:    sourcewindowfacts.Function{Path: "write.go", Symbol: "write"},
		Declaration: declaration,
	}
	function.Calls = freshPrimaryCalls(file, function)
	for _, call := range function.Calls {
		if call.Target != "f.Write" {
			continue
		}
		if call.ReceiverType != "os.File" || call.ReceiverImportPath != "os" ||
			freshPrimaryBoundaryForCall(call) != freshBoundaryFileWrite {
			t.Fatalf("write call = %+v", call)
		}
		return
	}
	t.Fatal("f.Write call not found")
}

func TestFreshPrimaryBackendBoundaryDoesNotBypassLookupLimit(t *testing.T) {
	t.Parallel()
	state := freshPrimaryPlannerState{
		ctx: context.Background(),
		data: &report.ReportData{RepositoryGraph: &report.RepositoryGraph{
			Packages: []report.PackageInfo{{
				CanonicalPath: "example.test/demo",
				Name:          "demo",
				Locality:      "local",
				Files:         []string{"client.go", "worker.go"},
			}},
		}},
		files:           make(map[string]*freshPrimaryParsedFile),
		functions:       make(map[string]freshPrimaryFunction),
		additionalFiles: []string{"a.go", "b.go", "c.go", "d.go"},
		limitReasons:    make(map[string]struct{}),
	}
	function := freshPrimaryFunction{
		Function: sourcewindowfacts.Function{Path: "worker.go", Symbol: "Worker.Run"},
		Calls: []freshPrimaryCall{{
			ID: "call-write", Path: "worker.go", Function: "Worker.Run",
			Target: "client.Write", Terminal: "Write", ReceiverType: "DestinationClient",
		}},
	}
	if boundary := state.bestBoundary(function); boundary != nil {
		t.Fatalf("boundary = %+v, want bounded lookup failure", boundary)
	}
	if _, exists := state.limitReasons["additional_file_limit"]; !exists {
		t.Fatalf("limit reasons = %v, want additional_file_limit", state.limitReasons)
	}
	resolution, class := state.effectResolution(nil)
	if resolution != "unresolved" || class != "bounded_static_analysis_limit" {
		t.Fatalf("resolution = %q/%q", resolution, class)
	}
}

func TestFreshPrimaryEffectResolutionExplainsBoundedFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		state     freshPrimaryPlannerState
		wantClass string
	}{
		{
			name: "dynamic receiver",
			state: freshPrimaryPlannerState{frontiers: []freshPrimaryFrontier{{
				Resolution: "unresolved", ResolutionReason: "receiver_type_unresolved",
			}}},
			wantClass: "unresolved_dynamic_dispatch",
		},
		{
			name: "exact package but target absent",
			state: freshPrimaryPlannerState{frontiers: []freshPrimaryFrontier{{
				Resolution: "unresolved", ResolutionReason: "exact_local_target_not_found",
			}}},
			wantClass: "insufficient_cross_package_connectivity",
		},
		{
			name:      "bounded file limit",
			state:     freshPrimaryPlannerState{limitReasons: map[string]struct{}{"additional_file_limit": {}}},
			wantClass: "bounded_static_analysis_limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolution, class := test.state.effectResolution(nil)
			if resolution != "unresolved" || class != test.wantClass {
				t.Fatalf("resolution = %q/%q, want unresolved/%q", resolution, class, test.wantClass)
			}
		})
	}
}

func TestFreshPrimaryPlannerUsesProductCentralAnchors(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	sourceText := `package demo
func entry() { _ = helper() }
func helper() string { return "done" }
`
	writeTestFile(t, repoRoot, "demo.go", sourceText)
	entry := testFreshSourceFunction(t, "demo.go", sourceText, "entry")
	helper := testFreshSourceFunction(t, "demo.go", sourceText, "helper")
	candidate := semanticdiscovery.OpportunityCandidate{
		ID:               "semantic-candidate-1234567890abcdef",
		Kind:             semanticdiscovery.ArtifactMechanism,
		QuestionAnswered: "How does the demo entry run its work?",
		SupportIDs:       []string{entry.Fact.ID, helper.Fact.ID},
		ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
		Confidence:       semanticdiscovery.ConfidenceHigh,
		ProductIntent: &semanticdiscovery.OpportunityProductIntent{
			OpportunityKind:  semanticdiscovery.OpportunityKindCentralBehavior,
			TargetUserJob:    semanticdiscovery.OpportunityUserJobFirstContact,
			CentralAnchorIDs: []string{entry.Fact.ID},
		},
	}
	data := &report.ReportData{
		DocumentedPurpose: "The demo entry runs work.",
		RepositoryGraph: &report.RepositoryGraph{Packages: []report.PackageInfo{{
			CanonicalPath: "example.test/demo",
			ModulePath:    "example.test/demo",
			Name:          "demo",
			Locality:      "local",
			Files:         []string{"demo.go"},
		}}},
	}
	work := planFreshPrimaryCandidate(
		repoRoot,
		data,
		candidate,
		[]freshSourceFunction{entry, helper},
	)
	if len(work.Plan.Primary.RootAnchors) != 1 {
		t.Fatalf("root anchors = %+v, want only product central anchor", work.Plan.Primary.RootAnchors)
	}
	if got := work.Plan.Primary.RootAnchors[0].Symbol; got != "entry" {
		t.Fatalf("root symbol = %q, want entry", got)
	}
	if work.Plan.ProductIntent != candidate.ProductIntent {
		t.Fatal("candidate plan did not retain product intent")
	}

	legacy := candidate
	legacy.ProductIntent = nil
	legacyWork := planFreshPrimaryCandidate(
		repoRoot,
		data,
		legacy,
		[]freshSourceFunction{entry, helper},
	)
	if len(legacyWork.Plan.Primary.RootAnchors) != 2 {
		t.Fatalf("legacy root anchors = %+v, want support-id fallback", legacyWork.Plan.Primary.RootAnchors)
	}
}

func TestFreshPrimaryPlanningAnchorsReserveInputCoreAndEffect(t *testing.T) {
	t.Parallel()
	candidate := semanticdiscovery.OpportunityCandidate{
		SupportIDs: []string{"input-a", "core-a", "effect-a", "central-a", "central-b"},
		ProductIntent: &semanticdiscovery.OpportunityProductIntent{
			CentralAnchorIDs: []string{"central-a", "central-b"},
			ExpectedPath: semanticdiscovery.OpportunityExpectedPath{
				InputTrigger: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{"input-a"},
				},
				CoreWork: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{"core-a", "core-b"},
				},
				ObservableEffect: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{"effect-a", "effect-b"},
				},
			},
		},
	}
	got := freshCandidatePlanningAnchorIDs(candidate)
	wantPrefix := []string{"input-a", "core-a", "effect-a", "central-a"}
	if len(got) < len(wantPrefix) {
		t.Fatalf("planning anchors = %v", got)
	}
	for index, want := range wantPrefix {
		if got[index] != want {
			t.Fatalf("planning anchors = %v, want prefix %v", got, wantPrefix)
		}
	}
}

func TestFreshPrimaryResolveRootsSkipsAnchorOutsideRepositoryGraph(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	exampleText := "package example\nfunc run() { helper() }\nfunc helper() {}\n"
	coreText := "package demo\nfunc monitor() { tick() }\nfunc tick() {}\n"
	writeTestFile(t, repoRoot, "example.go", exampleText)
	writeTestFile(t, repoRoot, "core.go", coreText)
	example := testFreshSourceFunction(t, "example.go", exampleText, "run")
	core := testFreshSourceFunction(t, "core.go", coreText, "monitor")
	candidate := semanticdiscovery.OpportunityCandidate{
		SupportIDs: []string{example.Fact.ID, core.Fact.ID},
		ProductIntent: &semanticdiscovery.OpportunityProductIntent{
			CentralAnchorIDs: []string{example.Fact.ID, core.Fact.ID},
			ExpectedPath: semanticdiscovery.OpportunityExpectedPath{
				InputTrigger: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{example.Fact.ID},
				},
				CoreWork: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{core.Fact.ID},
				},
			},
		},
	}
	data := &report.ReportData{RepositoryGraph: &report.RepositoryGraph{
		Packages: []report.PackageInfo{{
			CanonicalPath: "example.test/demo",
			ModulePath:    "example.test/demo",
			Name:          "demo",
			Locality:      "local",
			Files:         []string{"core.go"},
		}},
	}}
	reader, err := reporead.New(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	state := freshPrimaryPlannerState{
		ctx: context.Background(), reader: reader, data: data, candidate: candidate,
		sources: []freshSourceFunction{example, core},
		files:   make(map[string]*freshPrimaryParsedFile), functions: make(map[string]freshPrimaryFunction),
	}

	roots, sources, err := state.resolveRoots()
	if err != nil {
		t.Fatalf("resolveRoots() error = %v", err)
	}
	if len(roots) != 1 || roots[0].Path != "core.go" || roots[0].Symbol != "monitor" {
		t.Fatalf("resolved roots = %#v, want only repository-graph core root", roots)
	}
	if len(sources) != 1 || sources[0].Fact.ID != core.Fact.ID {
		t.Fatalf("resolved source facts = %#v, want core fact", sources)
	}
}

func TestFreshPrimaryResolveRootsKeepsExactAnchorsWhenEntryAlreadyPresent(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	sourceText := `package demo
type Command struct { worker *Worker }
func (c *Command) Run() { c.worker.Sync() }
type Worker struct{}
func (w *Worker) Start() {}
func (w *Worker) Sync() { w.Write() }
func (w *Worker) Write() { persist() }
func (w *Worker) monitor() { observe() }
func persist() {}
func observe() {}
`
	writeTestFile(t, repoRoot, "demo.go", sourceText)
	entry := testFreshSourceFunction(t, "demo.go", sourceText, "Command.Run")
	core := testFreshSourceFunction(t, "demo.go", sourceText, "Worker.Sync")
	effect := testFreshSourceFunction(t, "demo.go", sourceText, "Worker.Write")
	monitor := testFreshSourceFunction(t, "demo.go", sourceText, "Worker.monitor")
	candidate := semanticdiscovery.OpportunityCandidate{
		SupportIDs: []string{entry.Fact.ID, core.Fact.ID, effect.Fact.ID, monitor.Fact.ID},
		ProductIntent: &semanticdiscovery.OpportunityProductIntent{
			CentralAnchorIDs: []string{monitor.Fact.ID},
			ExpectedPath: semanticdiscovery.OpportunityExpectedPath{
				InputTrigger: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{entry.Fact.ID},
				},
				CoreWork: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{core.Fact.ID},
				},
				ObservableEffect: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{effect.Fact.ID},
				},
			},
		},
	}
	data := &report.ReportData{RepositoryGraph: &report.RepositoryGraph{
		Packages: []report.PackageInfo{{
			CanonicalPath: "example.test/demo",
			ModulePath:    "example.test/demo",
			Name:          "demo",
			Locality:      "local",
			Files:         []string{"demo.go"},
		}},
	}}
	reader, err := reporead.New(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	state := freshPrimaryPlannerState{
		ctx: context.Background(), reader: reader, data: data, candidate: candidate,
		sources: []freshSourceFunction{entry, core, effect, monitor},
		files:   make(map[string]*freshPrimaryParsedFile), functions: make(map[string]freshPrimaryFunction),
	}

	roots, sources, err := state.resolveRoots()
	if err != nil {
		t.Fatalf("resolveRoots() error = %v", err)
	}
	if len(roots) != 4 || len(sources) != 4 {
		t.Fatalf("resolved roots = %#v; source count = %d, want four exact roots", roots, len(sources))
	}
	want := map[string]bool{
		"Command.Run":    false,
		"Worker.Sync":    false,
		"Worker.Write":   false,
		"Worker.monitor": false,
	}
	for _, root := range roots {
		if root.Symbol == "Worker.Start" {
			t.Fatal("same-receiver entry companion displaced an exact candidate anchor")
		}
		if _, ok := want[root.Symbol]; ok {
			want[root.Symbol] = true
		}
	}
	for symbol, found := range want {
		if !found {
			t.Fatalf("exact root %q was not retained: %#v", symbol, roots)
		}
	}
}

func TestFreshPrimaryFrontierScorePrefersEffectAndCoreAnchors(t *testing.T) {
	t.Parallel()

	state := freshPrimaryPlannerState{candidate: semanticdiscovery.OpportunityCandidate{
		QuestionAnswered: "How does the repository replicate records?",
		ProductIntent: &semanticdiscovery.OpportunityProductIntent{
			ExpectedPath: semanticdiscovery.OpportunityExpectedPath{
				InputTrigger: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{"fact-input"},
				},
				CoreWork: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{"fact-core"},
				},
				ObservableEffect: semanticdiscovery.OpportunityExpectation{
					SupportIDs: []string{"fact-effect"},
				},
			},
		},
	}}
	call := freshPrimaryCall{Path: "worker.go", Terminal: "Apply", Target: "w.Apply"}
	target := freshPrimaryFunction{Function: sourcewindowfacts.Function{Path: "worker.go", Symbol: "Worker.Apply"}}
	score := func(factID string) int {
		return state.frontierScore(freshPrimaryAnchor{OriginFactID: factID}, call, target)
	}
	inputScore := score("fact-input")
	coreScore := score("fact-core")
	effectScore := score("fact-effect")
	if !(effectScore > coreScore && coreScore > inputScore) {
		t.Fatalf("aspect-aware scores = input %d, core %d, effect %d", inputScore, coreScore, effectScore)
	}
	if got := score("fact-other"); got >= inputScore {
		t.Fatalf("unscoped anchor score = %d, want below input score %d", got, inputScore)
	}
}

func TestFreshPrimaryFirstContactRequiresProductThresholds(t *testing.T) {
	t.Parallel()
	centralCandidate := semanticdiscovery.OpportunityCandidate{
		ProductIntent: &semanticdiscovery.OpportunityProductIntent{
			OpportunityKind: semanticdiscovery.OpportunityKindCentralBehavior,
			TargetUserJob:   semanticdiscovery.OpportunityUserJobFirstContact,
		},
	}
	newWork := func(centrality freshCandidateCentrality) freshCandidateWork {
		return freshCandidateWork{
			Candidate: centralCandidate,
			Plan: freshRepoCandidatePlan{
				Centrality: centrality,
				Primary: &freshPrimaryProbePlan{
					Status: freshPrimaryPlanReady,
					Eligibility: freshPrimaryEligibility{
						Status: freshPrimaryPlanReady,
					},
				},
			},
		}
	}

	below := newWork(freshCandidateCentrality{
		PurposeAlignment:  freshPrimaryMinPurposeAlignment - 1,
		ExplanatoryValue:  freshPrimaryMinExplanation - 1,
		NavigationValue:   freshPrimaryMinNavigation - 1,
		EvidenceReadiness: freshPrimaryMinEvidence - 1,
	})
	freshApplyPrimaryProductEligibility(&below)
	if below.Plan.Primary.Status != freshPrimaryPlanInsufficient {
		t.Fatalf("below-threshold status = %q", below.Plan.Primary.Status)
	}
	for _, reason := range []string{
		"purpose_alignment_below_primary_threshold",
		"explanatory_value_below_primary_threshold",
		"navigation_value_below_primary_threshold",
		"evidence_readiness_below_primary_threshold",
	} {
		if !containsFreshString(below.Plan.Primary.StatusReasons, reason) {
			t.Fatalf("reasons = %v, want %q", below.Plan.Primary.StatusReasons, reason)
		}
	}

	eligible := newWork(freshCandidateCentrality{
		PurposeAlignment:  freshPrimaryMinPurposeAlignment,
		ExplanatoryValue:  freshPrimaryMinExplanation,
		NavigationValue:   freshPrimaryMinNavigation,
		EvidenceReadiness: freshPrimaryMinEvidence,
	})
	freshApplyPrimaryProductEligibility(&eligible)
	if eligible.Plan.Primary.Status != freshPrimaryPlanReady ||
		!freshCandidateIsEligibleFirstContact(eligible) {
		t.Fatalf("eligible work was demoted: %+v", eligible.Plan.Primary)
	}

	legacy := newWork(freshCandidateCentrality{})
	legacy.Candidate.ProductIntent = nil
	freshApplyPrimaryProductEligibility(&legacy)
	if legacy.Plan.Primary.Status != freshPrimaryPlanReady {
		t.Fatalf("legacy candidate was product-gated: %+v", legacy.Plan.Primary)
	}
}

func TestValidateFreshPrimaryArtifactRetainsAndAnswersQuestion(t *testing.T) {
	t.Parallel()

	question := "How does the request write a snapshot?"
	plan := &freshPrimaryProbePlan{
		Question: question,
		Status:   freshPrimaryPlanReady,
	}
	facts := []semanticdiscovery.Fact{
		{ID: "input", Statement: "The request enters the handler."},
		{ID: "core", Statement: "The handler calls the snapshot operation."},
		{ID: "effect", Statement: "The operation writes the snapshot file."},
	}
	artifact := semanticdiscovery.Artifact{
		Question:           question,
		Summary:            "The request invokes the snapshot operation, which writes the snapshot file.",
		CoveredAspectIDs:   []string{freshPrimaryAspectInput, freshPrimaryAspectCore, freshPrimaryAspectEffect},
		UncoveredAspectIDs: []string{},
		Statements: []semanticdiscovery.Statement{
			{ID: "s1", Text: facts[0].Statement, Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{"input"}},
			{ID: "s2", Text: facts[1].Statement, Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{"core"}},
			{ID: "s3", Text: facts[2].Statement, Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{"effect"}},
		},
		Steps: []semanticdiscovery.Step{
			{Title: "Receive request", Evidence: []semanticdiscovery.EvidenceRef{{ID: "e1"}}},
			{Title: "Build snapshot", Evidence: []semanticdiscovery.EvidenceRef{{ID: "e2"}}},
			{Title: "Write snapshot", Evidence: []semanticdiscovery.EvidenceRef{{ID: "e3"}}},
		},
	}
	if err := validateFreshPrimaryArtifact(artifact, plan, facts); err != nil {
		t.Fatalf("valid artifact rejected: %v", err)
	}

	unrelated := artifact
	unrelated.Summary = "Widgets are collected."
	unrelated.Statements = append([]semanticdiscovery.Statement(nil), artifact.Statements...)
	for index := range unrelated.Statements {
		unrelated.Statements[index].Text = "A widget is collected."
	}
	if err := validateFreshPrimaryArtifact(unrelated, plan, facts); err == nil ||
		!strings.Contains(err.Error(), "not aligned") {
		t.Fatalf("unrelated answer error = %v", err)
	}

	trivial := artifact
	trivial.Statements = append([]semanticdiscovery.Statement(nil), artifact.Statements...)
	for index := range trivial.Statements {
		trivial.Statements[index].SupportIDs = []string{"input"}
	}
	if err := validateFreshPrimaryArtifact(trivial, plan, facts); err == nil ||
		!strings.Contains(err.Error(), "non-trivial") {
		t.Fatalf("single-operation answer error = %v", err)
	}
}

func TestFreshPrimaryPlannerEnforcesFileAndFunctionBounds(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	functionSymbols := make([]string, 0, 16)
	var source strings.Builder
	source.WriteString("package demo\n")
	for index := range 16 {
		symbol := fmt.Sprintf("f%02d", index)
		functionSymbols = append(functionSymbols, symbol)
		fmt.Fprintf(&source, "func %s() int { return %d }\n", symbol, index)
	}
	paths := make([]string, 0, freshPrimaryMaxFiles+1)
	for index := 0; index <= freshPrimaryMaxFiles; index++ {
		path := fmt.Sprintf("file%d.go", index)
		paths = append(paths, path)
		writeTestFile(t, repoRoot, path, source.String())
	}
	data := &report.ReportData{RepositoryGraph: &report.RepositoryGraph{
		Packages: []report.PackageInfo{{
			CanonicalPath: "example.test/demo",
			ModulePath:    "example.test/demo",
			Name:          "demo",
			Locality:      "local",
			Files:         paths,
		}},
	}}
	reader, err := reporead.New(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	state := freshPrimaryPlannerState{
		ctx:       context.Background(),
		reader:    reader,
		data:      data,
		files:     make(map[string]*freshPrimaryParsedFile),
		functions: make(map[string]freshPrimaryFunction),
	}
	for _, path := range paths[:freshPrimaryMaxFiles] {
		if _, err := state.readFile(path, false, functionSymbols[0]); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
	}
	if _, err := state.readFile(paths[freshPrimaryMaxFiles], false, functionSymbols[0]); err == nil || !strings.Contains(err.Error(), "file limit reached") {
		t.Fatalf("ninth file error = %v, want file limit", err)
	}
	if err := state.indexPreferredFunctions(state.files[paths[0]], functionSymbols); err == nil || !strings.Contains(err.Error(), "function limit reached") {
		t.Fatalf("function overflow error = %v, want function limit", err)
	}
	if got := len(state.functions); got != freshPrimaryMaxFunctions {
		t.Fatalf("indexed functions = %d, want %d", got, freshPrimaryMaxFunctions)
	}
}

func TestAttemptFreshCandidateSkipsProviderWhenPrimaryEvidenceIsInsufficient(t *testing.T) {
	t.Parallel()
	provider := &semanticDiscoveryEditorStub{calls: make(map[string]int)}
	work := freshCandidateWork{
		Candidate: semanticdiscovery.OpportunityCandidate{
			ID:               "semantic-candidate-1234567890abcdef",
			QuestionAnswered: "How does the bounded operation work?",
		},
		Plan: freshRepoCandidatePlan{
			CandidateID: "semantic-candidate-1234567890abcdef",
			Question:    "How does the bounded operation work?",
			Primary: &freshPrimaryProbePlan{
				Status:        freshPrimaryPlanInsufficient,
				StatusReasons: []string{"observable_effect_fact_missing"},
				Eligibility: freshPrimaryEligibility{
					Status:  freshPrimaryPlanInsufficient,
					Reasons: []string{"observable_effect_fact_missing"},
				},
			},
		},
	}
	attempt, _, _, err := attemptFreshCandidate(
		context.Background(), t.TempDir(), t.TempDir(), nil, work, provider,
	)
	if !errors.Is(err, errFreshPrimaryEvidenceInsufficient) {
		t.Fatalf("error = %v, want insufficient sentinel", err)
	}
	if attempt.State != string(freshPrimaryPlanInsufficient) {
		t.Fatalf("attempt state = %q", attempt.State)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("provider calls = %v, want none", provider.calls)
	}
}

func testFreshSourceFunction(
	t *testing.T,
	path string,
	source string,
	symbol string,
) freshSourceFunction {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(source, "\n"), "\n")
	window, err := sourcewindowfacts.NewWindow("window-1", path, 1, lines)
	if err != nil {
		t.Fatal(err)
	}
	function, err := sourcewindowfacts.ExtractGoFunction(window, symbol)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := freshWindowFunctionFact(function)
	if err != nil {
		t.Fatal(err)
	}
	return freshSourceFunction{Function: function, Fact: fact}
}

func writeTestFile(t *testing.T, root string, path string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsFreshString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
