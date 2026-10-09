package groupindex

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestFullyUnheldDeclarationsKeepSourceAccessAndRefusedProposal(t *testing.T) {
	for _, reason := range []string{atlas.OffMapLeftOut, atlas.OffMapConflict, atlas.OffMapFailure} {
		t.Run(reason, func(t *testing.T) {
			p, err := programindex.New(programindex.Input{
				ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
				Target: programindex.TargetInput{Language: "go", Kind: "executable", Name: "app", Selector: "app", Sources: []programindex.TargetSource{{Path: "app.go", FileRef: "f1"}}, AnchorFileRef: "f1"},
				Objects: []programindex.ObjectInput{
					{SourceRef: "app", Kind: programindex.ObjectFunction, Name: "main", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: "app.go", Line: 3, Column: 1}},
					{SourceRef: "type", Kind: programindex.ObjectType, Name: "Utility", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: "helper.go", Line: 3, Column: 1}},
					{SourceRef: "method", Kind: programindex.ObjectMethod, Name: "Utility.Run", OwnerRef: "type", Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: "helper.go", Line: 5, Column: 1}},
					{SourceRef: "closure", Kind: programindex.ObjectFunction, Name: "Utility.Run.func1", ContainerRef: "method", Visibility: programindex.VisibilityInternal, Location: &programindex.Location{Path: "helper.go", Line: 6, Column: 1}},
				}, Relations: []programindex.RelationInput{}, Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 4},
			})
			if err != nil {
				t.Fatal(err)
			}
			before, err := programindex.Encode(p)
			if err != nil {
				t.Fatal(err)
			}
			target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: "executable", Root: ".", Zones: []atlas.Zone{}, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}}
			var refusedIDs []string
			for _, obj := range p.Objects {
				entry := atlas.OffMapFile{ID: obj.ID, Reason: reason, File: atlas.File{Path: obj.Location.Path, Source: atlas.SourceModel, Symbols: []atlas.Symbol{{ID: "s" + obj.ID, ObjectID: p.Target.ID + "." + obj.ID, Name: obj.Name, Kind: string(obj.Kind), LineNo: obj.Location.Line, Column: obj.Location.Column}}}}
				target.OffMap = append(target.OffMap, entry)
				if obj.Location.Path == "helper.go" {
					refusedIDs = append(refusedIDs, p.Target.ID+"."+obj.ID)
				}
			}
			if reason == atlas.OffMapFailure {
				target.MapFailure = atlas.MapFailureRefused
			}
			target.RefusedParts = []atlas.RefusedPart{{Name: "Standalone helpers", Holds: "Convenience helpers without a common job.", Reason: atlas.PartsIndependentJobs, MemberIDs: refusedIDs}}
			value := atlas.Atlas{Version: atlas.Version, Repository: "fixture", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
			indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, value)
			if err != nil {
				t.Fatal(err)
			}
			index := indexes[0]
			if len(index.Groups) != 0 || index.MapFailure != target.MapFailure {
				t.Fatalf("invented map: %+v", index.Groups)
			}
			var got []string
			for _, entry := range index.OffMap {
				if len(entry.SubjectIDs) != 1 {
					t.Fatalf("lost declaration: %+v", entry)
				}
				got = append(got, entry.SubjectIDs...)
			}
			var want []string
			for _, obj := range p.Objects {
				want = append(want, obj.ID)
			}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("native closure %v != %v", got, want)
			}
			if len(index.RefusedParts) != 1 || len(index.RefusedParts[0].SubjectIDs) != 3 || index.RefusedParts[0].Reason != atlas.PartsIndependentJobs {
				t.Fatalf("lost model refusal: %+v", index.RefusedParts)
			}
			encoded, err := Encode(index)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Decode(encoded, p)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(index.RefusedParts, restored.RefusedParts) || !reflect.DeepEqual(index.OffMap, restored.OffMap) {
				t.Fatal("persistence lost refused source access")
			}
			snap := index.Snapshot()
			snap.RefusedParts[0].SubjectIDs[0] = "changed"
			snap.OffMap[0].SubjectIDs[0] = "changed"
			if index.RefusedParts[0].SubjectIDs[0] == "changed" || index.OffMap[0].SubjectIDs[0] == "changed" {
				t.Fatal("snapshot shared mutable refusal members")
			}
			after, err := programindex.Encode(p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("native evidence changed")
			}
		})
	}
}

func TestWholeFileSourceAccessDoesNotHideUnknownDeclaration(t *testing.T) {
	p := atlasTestProgram(t, "app", "helper.go")
	target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: "executable", Root: ".", Zones: []atlas.Zone{}, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}, OffMap: []atlas.OffMapFile{{ID: "f1", Reason: atlas.OffMapLeftOut, File: atlas.File{Path: "helper.go", Source: atlas.SourceModel, Symbols: []atlas.Symbol{{ID: "s1", ObjectID: "unknown", Name: "Helper", Kind: "function"}}}}}}
	_, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown unheld declaration hidden: %v", err)
	}
}

func TestMapFailureSourceIDsRemainClosedAndUnitlessPathsStayPathOnly(t *testing.T) {
	subjects := map[string]Subject{"n1": {ID: "n1"}}
	if err := validateOffMap([]OffMapFile{{Path: "helper.go", Reason: "map_failure", SubjectIDs: []string{"n1"}}}, atlas.MapFailureRefused, subjects); err != nil {
		t.Fatal(err)
	}
	for _, row := range []OffMapFile{{Path: "helper.go", Reason: "map_failure", SubjectIDs: []string{"missing"}}, {Path: "helper.go", Reason: "no_units", SubjectIDs: []string{"n1"}}} {
		failure := ""
		if row.Reason == "map_failure" {
			failure = atlas.MapFailureRefused
		}
		if validateOffMap([]OffMapFile{row}, failure, subjects) == nil {
			t.Fatalf("invalid source binding accepted: %+v", row)
		}
	}
	if validateOffMap([]OffMapFile{{Path: "empty.go", Reason: "no_units"}}, "", subjects) != nil {
		t.Fatal("unitless path refused")
	}
	if validateOffMap([]OffMapFile{{Path: "helper.go", Reason: "map_failure"}}, atlas.MapFailureRefused, subjects) != nil {
		t.Fatal("legacy file-only map failure refused")
	}
}

func TestRefusedPartCannotNameMissingOrAcceptedNativeMember(t *testing.T) {
	subjects := map[string]Subject{"n1": {ID: "n1", Object: &ObjectFacts{Name: "Helper", Location: &programindex.Location{Path: "helper.go", Line: 3, Column: 1}}}}
	base := RefusedPart{Name: "Proposal", Holds: "Provisional purpose.", Reason: atlas.PartsNotEstablished, SubjectIDs: []string{"n1"}}
	if validateRefusedParts([]RefusedPart{base}, nil, subjects) != nil {
		t.Fatal("known model refusal refused")
	}
	if validateRefusedParts([]RefusedPart{base}, []Group{{MemberSubjectIDs: []string{"n1"}}}, subjects) == nil {
		t.Fatal("accepted declaration also refused")
	}
	for _, ids := range [][]string{{"missing"}, {"n1", "n1"}, nil} {
		row := base
		row.SubjectIDs = ids
		if validateRefusedParts([]RefusedPart{row}, nil, subjects) == nil {
			t.Fatalf("invalid members %v accepted", ids)
		}
	}
}
