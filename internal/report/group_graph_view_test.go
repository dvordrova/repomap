package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

// Three exact calls from one group to another were three identical rows on
// the card, each explaining its label with the label again.
func TestConnectionRowsAreSaidOnceAndDoNotEchoTheirLabel(t *testing.T) {
	row := pageConnection{Arrow: "←", Title: "Client IP middleware", Label: "provides client IP context", Summary: "Provides client IP context."}
	rows := collapseConnections([]pageConnection{row, row, row, {Arrow: "→", Title: "Timeout", Label: "uses context", Summary: "hands the request context on"}})
	if len(rows) != 2 || rows[0].Count != 3 || rows[1].Count != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].Summary != "" {
		t.Errorf("a summary repeating the label was kept: %q", rows[0].Summary)
	}
	if rows[1].Summary != "hands the request context on" {
		t.Errorf("a summary saying more than the label was lost: %q", rows[1].Summary)
	}
	if dropEcho("Compression middleware", "Compression middleware") != "" || dropEcho("gzip and brotli", "Compression middleware") == "" {
		t.Error("dropEcho keeps the echo or drops the explanation")
	}
}

// The graph keeps one group per title per lane; the page shows one per title.
// The largest slice lends its identity and its place in a zone, every slice's
// connections follow it, and a slice talking to its twin is dropped.
func TestPageFoldsOneTitleIntoOneGroup(t *testing.T) {
	target := "t1"
	indexes := []groupindex.Index{{
		Target: programindex.Target{ID: target},
		Groups: []groupindex.Group{
			{ID: "trig", Title: "Client IP middleware", Lane: groupindex.LaneTriggers, MemberSubjectIDs: []string{"s1", "s2"}},
			{ID: "core", Title: "Client IP middleware", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"s3", "s4", "s5"}, Summary: "reads the client address from headers"},
			{ID: "router", Title: "Router", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"s6"}},
		},
		Containers: []groupindex.Container{
			{ID: "z1", Title: "Middleware", GroupIDs: []string{"core", "router"}},
			{ID: "z2", Title: "Entry", GroupIDs: []string{"trig"}},
		},
		Connections: []groupindex.Connection{
			{From: groupindex.Endpoint{TargetID: target, GroupID: "trig"}, To: groupindex.Endpoint{TargetID: target, GroupID: "router"}, Label: "wraps"},
			{From: groupindex.Endpoint{TargetID: target, GroupID: "core"}, To: groupindex.Endpoint{TargetID: target, GroupID: "router"}, Label: "wraps"},
			{From: groupindex.Endpoint{TargetID: target, GroupID: "trig"}, To: groupindex.Endpoint{TargetID: target, GroupID: "core"}, Label: "builds"},
		},
	}}
	folded := foldIndexes(indexes)[0]
	if len(folded.Groups) != 2 {
		t.Fatalf("groups = %#v", folded.Groups)
	}
	var one groupindex.Group
	for _, group := range folded.Groups {
		if group.ID == "core" {
			one = group
		}
	}
	if one.ID != "core" || one.Lane != groupindex.LaneCore || len(one.MemberSubjectIDs) != 5 || one.Summary == "" {
		t.Errorf("folded group = %#v", one)
	}
	if len(folded.Containers) != 1 || len(folded.Containers[0].GroupIDs) != 2 {
		t.Errorf("containers = %#v", folded.Containers)
	}
	if len(folded.Connections) != 1 || folded.Connections[0].From.GroupID != "core" {
		t.Errorf("connections = %#v", folded.Connections)
	}
	if len(indexes[0].Groups) != 3 {
		t.Error("the graph itself was changed")
	}
}
