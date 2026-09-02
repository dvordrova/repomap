package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestGroupGraphViewOwnsCompleteMatchedSetWithoutReconstruction(t *testing.T) {
	left := reportGroupIndexFixture(t, "api", "go:./cmd/api", "cmd/api/main.go")
	right := reportGroupIndexFixture(t, "worker", "python:worker", "worker.py")
	matched, diagnostics, err := groupindex.WithConnections(
		[]groupindex.Index{left, right},
		[]groupindex.ConnectionInput{{
			From:         groupindex.Endpoint{TargetID: left.Target.ID, GroupID: left.Groups[0].ID},
			To:           groupindex.Endpoint{TargetID: right.Target.ID, GroupID: right.Groups[0].ID},
			SemanticKind: "dispatches_to", Label: "dispatches to", Summary: "API work is dispatched to the worker.",
			SupportResolution: programindex.PatternValueExact,
		}},
	)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("WithConnections: diagnostics=%#v err=%v", diagnostics, err)
	}
	view, err := NewGroupGraphView(matched, left.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Indexes) != 2 || len(view.Indexes[0].Connections)+len(view.Indexes[1].Connections) != 1 {
		t.Fatalf("group graph = %#v", view)
	}
	paths, err := view.SourcePaths()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths, []string{"cmd/api/main.go", "worker.py"}) {
		t.Fatalf("source paths = %#v", paths)
	}
	snapshot := view.Snapshot()
	snapshot.Indexes[0].Groups[0].Title = "changed"
	if view.Indexes[0].Groups[0].Title == "changed" {
		t.Fatal("GroupGraphView snapshot aliases graph authority")
	}
}

func TestReadRunDirDefersForeignEndpointsUntilCompleteGraphBinding(t *testing.T) {
	leftProgram, left := reportCategorizedGroupFixture(
		t, "api", "go:./cmd/api", "cmd/api/main.go",
		[]programindex.Category{programindex.CategoryInbound, programindex.CategoryBackgroundActivity},
		groupindex.LaneTriggers,
	)
	_, right := reportCategorizedGroupFixture(
		t, "worker", "python:worker", "worker.py",
		[]programindex.Category{programindex.CategoryDependency}, groupindex.LaneDependencies,
	)
	matched, diagnostics, err := groupindex.WithConnections(
		[]groupindex.Index{left, right},
		[]groupindex.ConnectionInput{{
			From:         groupindex.Endpoint{TargetID: left.Target.ID, GroupID: left.Groups[0].ID},
			To:           groupindex.Endpoint{TargetID: right.Target.ID, GroupID: right.Groups[0].ID},
			SemanticKind: "dispatches_to", Label: "dispatches to",
			Summary:           "API work is dispatched to the worker.",
			SupportResolution: programindex.PatternValuePossible,
		}},
	)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("WithConnections: diagnostics=%#v err=%v", diagnostics, err)
	}
	local := matchedGroupIndex(t, matched, left.Target.ID)
	runDir := t.TempDir()
	writeReportProgramIndexArtifacts(t, runDir, leftProgram)
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"fixture"}`))
	if err := documentationreduce.Persist(runDir, reportReducedDocumentationFixture(t)); err != nil {
		t.Fatal(err)
	}
	if err := groupindex.Persist(runDir, local); err != nil {
		t.Fatal(err)
	}
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir with foreign endpoint: %v", err)
	}
	if data.localGroupsIndex == nil || data.localGroupsIndex.SHA256 != local.SHA256 {
		t.Fatal("page-local GroupsIndex was not restored")
	}
	if data.GroupGraph != nil {
		t.Fatal("foreign endpoint gained singleton graph authority")
	}
	if err := BindGroupGraphView(data, matched); err != nil {
		t.Fatalf("BindGroupGraphView: %v", err)
	}
	if err := collectOpenablePaths(data); err != nil {
		t.Fatal(err)
	}
	data.CapturedRevision = strings.Repeat("a", 40)
	data.TargetOutcomePortfolio = reportTargetOutcomeViewFixture(t, []TargetNavigationPage{{
		RunID:            "run-fixture",
		ProgramTarget:    leftProgram.Target.Snapshot(),
		ArtifactFilename: programindex.ArtifactFilename,
	}}, leftProgram.Target.ID)
	// The complete matched set reaches the page: both groups become sections
	// content and the cross-target connection keeps its model sentence.
	view, err := buildPageView(data, strings.Repeat("b", 64), nil)
	if err != nil {
		t.Fatalf("buildPageView: %v", err)
	}
	if len(view.Sections) != 2 {
		t.Fatalf("page sections = %d, want one per analyzed target", len(view.Sections))
	}
	crossTarget := 0
	for _, section := range view.Sections {
		for _, group := range append(append([]pageGroup(nil), section.Triggers...), section.Core...) {
			for _, connection := range group.Connections {
				if connection.OtherTarget == "" {
					continue
				}
				crossTarget++
				if connection.Summary == "" || connection.Href == "" {
					t.Fatalf("cross-target connection lost its sentence or link: %#v", connection)
				}
			}
		}
	}
	if crossTarget == 0 {
		t.Fatal("cross-target connection did not reach the page")
	}
}

func TestBindRunAuthorityGroupGraphRequiresEveryProjectedSourcePath(t *testing.T) {
	repository := t.TempDir()
	writeTestFile(t, repository, "api.go", "package fixture\n")
	writeTestFile(t, repository, "worker.py", "def run():\n    pass\n")
	writeTestFile(t, repository, "worker_config.py", "QUEUE = 'jobs'\n")
	runManifestGit(t, repository, "init", "--quiet")
	runManifestGit(t, repository, "add", "api.go", "worker.py", "worker_config.py")
	runManifestGit(t, repository,
		"-c", "user.name=repomap test", "-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture",
	)
	state := captureRunManifestRepositoryState(t, repository)
	authority, err := ConfirmRunAuthorityScoped(
		context.Background(), repository, state, []string{"api.go", "worker.py"},
	)
	if err != nil {
		t.Fatal(err)
	}
	left := reportGroupIndexFixture(t, "api", "fixture:api", "api.go")
	right := reportGroupIndexWithUngroupedSource(t)
	if _, err := BindRunAuthorityGroupGraph(authority, []groupindex.Index{left, right}); err == nil ||
		!strings.Contains(err.Error(), "worker_config.py") {
		t.Fatalf("missing group source authorization error = %v", err)
	}
	extended, err := ExtendRunAuthority(
		context.Background(), authority, []string{"api.go", "worker.py", "worker_config.py"},
	)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindRunAuthorityGroupGraph(extended, []groupindex.Index{left, right})
	if err != nil {
		t.Fatal(err)
	}
	if !bound.groupGraphBound || len(bound.groupGraphIndexes) != 2 {
		t.Fatalf("bound graph authority = %#v", bound)
	}
}

func reportGroupIndexWithUngroupedSource(t *testing.T) groupindex.Index {
	t.Helper()
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("d", 64), SourceSHA256: strings.Repeat("e", 64),
		Target: programindex.TargetInput{
			Language: "fixture", Kind: "service", Name: "worker", Selector: "fixture:worker",
			Sources: []programindex.TargetSource{
				{FileRef: "f1", Path: "worker.py"},
				{FileRef: "f2", Path: "worker_config.py"},
			},
			AnchorFileRef: "f1",
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "worker", Kind: programindex.ObjectFunction, Name: "run",
				Visibility: programindex.VisibilityPublic,
				Location:   &programindex.Location{Path: "worker.py", Line: 1, Column: 1}},
			{SourceRef: "config", Kind: programindex.ObjectModule, Name: "config",
				Visibility: programindex.VisibilityInternal,
				Location:   &programindex.Location{Path: "worker_config.py", Line: 1, Column: 1}},
		},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	enriched, err := programindex.Enrich(
		base, strings.Repeat("f", 64),
		[]programindex.CategoryAssignment{{
			SubjectID:  base.Objects[0].ID,
			Categories: []programindex.Category{programindex.CategoryDependency},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	index, diagnostics, err := groupindex.Build(enriched, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
		Key: "worker", Title: "Worker", Summary: "Processes queued work.", Lane: groupindex.LaneDependencies,
		MemberSubjectIDs: []string{base.Objects[0].ID}, EvidenceSubjectIDs: []string{},
	}}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("groupindex.Build: diagnostics=%#v err=%v", diagnostics, err)
	}
	return index
}

func TestManifestBindsExactLocalGroupsIndexBeforeMatchedReportSet(t *testing.T) {
	program, local := reportCategorizedGroupFixture(
		t, "api", "fixture:api", "api.go",
		[]programindex.Category{programindex.CategoryCore}, groupindex.LaneCore,
	)
	right := reportGroupIndexFixture(t, "worker", "fixture:worker", "worker.py")
	matched, diagnostics, err := groupindex.WithConnections(
		[]groupindex.Index{local, right},
		[]groupindex.ConnectionInput{{
			From:         groupindex.Endpoint{TargetID: local.Target.ID, GroupID: local.Groups[0].ID},
			To:           groupindex.Endpoint{TargetID: right.Target.ID, GroupID: right.Groups[0].ID},
			SemanticKind: "dispatches_to", Label: "dispatches to", Summary: "Dispatches work.",
			SupportResolution: programindex.PatternValueExact,
		}},
	)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("WithConnections: diagnostics=%#v err=%v", diagnostics, err)
	}
	local = matchedGroupIndex(t, matched, local.Target.ID)
	runDir := t.TempDir()
	writeReportProgramIndexArtifacts(t, runDir, program)
	documentation := reportReducedDocumentationFixture(t)
	if err := documentationreduce.Persist(runDir, documentation); err != nil {
		t.Fatal(err)
	}
	if err := groupindex.Persist(runDir, local); err != nil {
		t.Fatal(err)
	}
	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&program.Target)
	if err != nil {
		t.Fatal(err)
	}
	setRaw, err := os.ReadFile(filepath.Join(runDir, programindex.ArtifactSetFilename))
	if err != nil {
		t.Fatal(err)
	}
	groupsRaw, err := os.ReadFile(filepath.Join(runDir, groupindex.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramIndexSetSHA256 = manifestSHA256(setRaw)
	documentationRaw, err := os.ReadFile(filepath.Join(runDir, documentationreduce.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ReducedDocumentationSHA256 = manifestSHA256(documentationRaw)
	manifest.MaterialInputs.GroupsIndexSHA256 = manifestSHA256(groupsRaw)
	view, err := NewGroupGraphView(matched, local.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := NewProgramPortfolio(program.Target.ID, []programindex.Index{program})
	if err != nil {
		t.Fatal(err)
	}
	reportData := ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "fixture",
		CapturedRevision:   manifest.RepositoryState.Head,
		CapturedInputCount: len(manifest.CapturedInputs),
		OpenablePaths:      []string{"api.go", "worker.py"},
		ProgramPortfolio:   portfolio,
		GroupGraph:         view,
	}
	reportData.TargetOutcomePortfolio = reportTargetOutcomeViewFixture(t, []TargetNavigationPage{{
		RunID:            "run-fixture",
		ProgramTarget:    program.Target.Snapshot(),
		ArtifactFilename: programindex.ArtifactFilename,
	}}, program.Target.ID)
	manifest.OpenablePaths = append([]string(nil), reportData.OpenablePaths...)
	if err := manifest.verifyReportData(reportData); err != nil {
		t.Fatalf("verify graph-only report projection: %v", err)
	}
	reportJSON, err := json.Marshal(&reportData)
	if err != nil {
		t.Fatal(err)
	}
	verify := func(candidate RunManifest) error {
		suite, suiteErr := newManifestVerificationSuiteWithValidation(candidate, runDir, false)
		if suiteErr != nil {
			return suiteErr
		}
		defer suite.Close()
		programs, suiteErr := suite.programIndexes()
		if suiteErr != nil {
			return suiteErr
		}
		if suiteErr = suite.verifyReducedDocumentationArtifact(programs); suiteErr != nil {
			return suiteErr
		}
		return suite.verifyGroupsIndexArtifact(programs, reportJSON)
	}
	if err := verify(manifest); err != nil {
		t.Fatalf("verify exact GroupsIndex: %v", err)
	}
	unboundDocumentation := manifest
	unboundDocumentation.MaterialInputs.ReducedDocumentationSHA256 = ""
	if err := verify(unboundDocumentation); err == nil ||
		!strings.Contains(err.Error(), "unbound reduced documentation") {
		t.Fatalf("unbound reduced documentation verification error = %v", err)
	}
	tampered := append([]byte(nil), groupsRaw...)
	tampered[len(tampered)-2] ^= 1
	writeReportProgramFile(t, filepath.Join(runDir, groupindex.ArtifactFilename), tampered)
	if err := verify(manifest); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered GroupsIndex verification error = %v", err)
	}
}

func reportGroupIndexFixture(t *testing.T, name, selector, sourcePath string) groupindex.Index {
	t.Helper()
	_, index := reportCategorizedGroupFixture(
		t, name, selector, sourcePath,
		[]programindex.Category{programindex.CategoryCore}, groupindex.LaneCore,
	)
	return index
}

func reportCategorizedGroupFixture(
	t *testing.T,
	name, selector, sourcePath string,
	categories []programindex.Category,
	lane groupindex.Lane,
) (programindex.Index, groupindex.Index) {
	t.Helper()
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "fixture", Kind: "service", Name: name, Selector: selector,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: sourcePath}}, AnchorFileRef: "f1",
		},
		Objects: []programindex.ObjectInput{{
			SourceRef: "root", Kind: programindex.ObjectFunction, Name: name,
			Visibility: programindex.VisibilityPublic,
			Location:   &programindex.Location{Path: sourcePath, Line: 1, Column: 1},
		}},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	documentation := reportReducedDocumentationFixture(t)
	enriched, err := programindex.Enrich(base, documentation.ReductionSHA256, []programindex.CategoryAssignment{{
		SubjectID: base.Objects[0].ID, Categories: categories,
	}})
	if err != nil {
		t.Fatal(err)
	}
	index, diagnostics, err := groupindex.Build(enriched, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
		Key: "group", Title: name + " group", Summary: "Owns " + name + " work.", Lane: lane,
		MemberSubjectIDs: []string{base.Objects[0].ID}, EvidenceSubjectIDs: []string{},
	}}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("groupindex.Build: diagnostics=%#v err=%v", diagnostics, err)
	}
	return enriched, index
}

func reportFinalGraphFixture(
	t *testing.T,
	index programindex.Index,
) (programindex.Index, groupindex.Index, documentationreduce.Result) {
	t.Helper()
	base, err := programindex.Base(index)
	if err != nil {
		t.Fatal(err)
	}
	documentation := reportReducedDocumentationFixture(t)
	assignments := make([]programindex.CategoryAssignment, 0, len(base.Objects))
	members := make([]string, 0, len(base.Objects))
	for _, object := range base.Objects {
		assignments = append(assignments, programindex.CategoryAssignment{
			SubjectID: object.ID, Categories: []programindex.Category{programindex.CategoryCore},
		})
		members = append(members, object.ID)
	}
	enriched, err := programindex.Enrich(base, documentation.ReductionSHA256, assignments)
	if err != nil {
		t.Fatal(err)
	}
	groups, diagnostics, err := groupindex.Build(enriched, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
		Key: "program", Title: "Program", Summary: "Owns the target program.", Lane: groupindex.LaneCore,
		MemberSubjectIDs: members, EvidenceSubjectIDs: []string{},
	}}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("groupindex.Build: diagnostics=%#v err=%v", diagnostics, err)
	}
	return enriched, groups, documentation
}

func writeReportFinalGraphArtifacts(
	t *testing.T,
	runDir string,
	index programindex.Index,
) (programindex.Index, groupindex.Index, documentationreduce.Result) {
	t.Helper()
	enriched, groups, documentation := reportFinalGraphFixture(t, index)
	writeReportProgramIndexArtifacts(t, runDir, enriched)
	if err := documentationreduce.Persist(runDir, documentation); err != nil {
		t.Fatal(err)
	}
	if err := groupindex.Persist(runDir, groups); err != nil {
		t.Fatal(err)
	}
	return enriched, groups, documentation
}

func reportReducedDocumentationFixture(t *testing.T) documentationreduce.Result {
	t.Helper()
	result := documentationreduce.Result{
		GuidanceSHA256: strings.Repeat("c", 64),
		Overview:       "Explains the fixture service.",
		Sources: []documentationreduce.Source{{
			Path: "README.md", Kind: readmetargetscout.GuidanceReadme,
			Claims: []string{"The fixture processes work."}, Concepts: []string{"Work"},
		}},
	}
	wire, err := json.Marshal(struct {
		Version        int                          `json:"version"`
		GuidanceSHA256 string                       `json:"guidance_sha256"`
		Overview       string                       `json:"overview"`
		Sources        []documentationreduce.Source `json:"sources"`
	}{
		Version: documentationreduce.Version, GuidanceSHA256: result.GuidanceSHA256,
		Overview: result.Overview, Sources: result.Sources,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	result.ReductionSHA256 = hex.EncodeToString(digest[:])
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func matchedGroupIndex(t *testing.T, indexes []groupindex.Index, targetID string) groupindex.Index {
	t.Helper()
	for _, index := range indexes {
		if index.Target.ID == targetID {
			return index
		}
	}
	t.Fatalf("matched GroupsIndex %q is absent", targetID)
	return groupindex.Index{}
}
