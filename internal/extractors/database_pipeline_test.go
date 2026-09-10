package extractors

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativeDataSurvivesNativeGraphReadingAndSavedGroupsIndex(t *testing.T) {
	ctx := t.Context()
	request := cumulativeDataRequest(t, "python")
	repository, err := corpus.New(ctx, request.Root, gitfiles.Listing{Paths: request.Files, RegularPaths: request.Files})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	catalog, err := pythontarget.Discover(ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	var target pythontarget.Target
	for _, item := range catalog.Entries {
		if item.Kind == pythontarget.KindLibrary && item.ProjectDir == "." {
			target = item
			break
		}
	}
	if target.Ref == "" {
		t.Fatal("cumulative application package missing")
	}
	programs, err := pythonprogramindex.BuildMany(ctx, repository, []pythontarget.Target{target})
	if err != nil {
		t.Fatal(err)
	}
	program := programs[0]
	extracted, err := Run(ctx, request.Root, repository)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: program, Root: "."}}, Extractions: extracted.Extractions})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: program, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	dataSources := 0
	for _, edge := range graph.Edges {
		if edge.Kind == "data_source" {
			dataSources++
			if len(edge.Witnesses) != 0 || edge.Evidence.Path != "src/fixture_app/storage.py" {
				t.Fatal("schema association became call evidence")
			}
		}
	}
	if dataSources < 2 {
		t.Fatalf("native ORM source declarations did not bind: %d", dataSources)
	}
	wire, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = atlas.DecodeGraph(wire)
	if err != nil {
		t.Fatal(err)
	}
	anchored := false
	for _, row := range lines.QuestionRows(graph) {
		for ref, anchor := range row.Anchors {
			if anchor.Kind != "data_source" || anchor.Name != "trades" {
				continue
			}
			evidence := lines.AnchorEvidence(row, ref)["evidence"].([]map[string]any)
			if len(evidence) > 0 && evidence[0]["data"] != nil {
				anchored = true
			}
		}
	}
	if !anchored {
		t.Fatal("question evidence lost typed columns and source anchor")
	}
	result, err := reading.Read(ctx, reading.Options{Graph: graph, Repository: "cumulative", OwnerRunDir: t.TempDir(), Targets: []reading.TargetMeta{{ID: program.Target.ID, Name: program.Target.Name, Kind: program.Target.Kind, Language: "python", Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 {
		t.Fatal("data changed target authority")
	}
	index := indexes[0]
	owners := 0
	literalQueries := 0
	for _, row := range index.Data {
		if row.Path == "src/fixture_app/sql_literals.py" {
			if row.Data.Kind == "query" {
				literalQueries++
			} else if row.Data.Name != "orders" || row.Data.Schema != "public" {
				t.Fatalf("prose/value became a persisted table: %+v", row)
			}
		}
		if row.Data.Origin == "orm" {
			if row.OwnerSubjectID == "" {
				t.Fatal("native owner lost in GroupsIndex")
			}
			owners++
		}
	}
	if owners != 2 {
		t.Fatalf("ORM owners=%d", owners)
	}
	if literalQueries != 11 {
		t.Fatalf("persisted SQL literals=%d; expected eleven supported source statements", literalQueries)
	}
	copy := index.Snapshot()
	copy.Data[0].Data.Name = "changed"
	if index.Data[0].Data.Name == "changed" {
		t.Fatal("data snapshot shares source evidence")
	}
	raw, err := groupindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := groupindex.Decode(raw)
	if err != nil || !reflect.DeepEqual(restored.Data, index.Data) {
		t.Fatal("saved source schema changed", err)
	}
}
