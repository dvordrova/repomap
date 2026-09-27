package groupindex

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestAtlasOutboundSurvivesIncomingLaneAndRebindsAcrossTargets(t *testing.T) {
	library := atlasTestProgram(t, "library", "api/proxy.go")
	app := atlasTestProgram(t, "executable", "api/proxy.go")
	rebound := rebindTestTargets(t, library, app)
	library, app = rebound[0], rebound[1]
	address := "https://시세.example/가격?q=%EC%9B%90"
	makeTarget := func(p programindex.Index) atlas.Target {
		return atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "go", Kind: p.Target.Kind,
			Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
			Boxes: []atlas.Box{{ID: "api", Dir: "api", Title: "Proxy", Line: "Receives and forwards requests.", Side: atlas.SideIn, MemberIDs: []string{p.Objects[0].ID},
				Files: []atlas.File{{Path: "api/proxy.go", Source: atlas.SourceModel, Line: "Proxy implementation.", Symbols: []atlas.Symbol{}}}}},
			Boundaries: []atlas.Boundary{
				{ID: "send-a", ObjectID: library.Objects[0].ID, BoxID: "api", Direction: atlas.DirectionOut, Kind: atlas.BoundaryClientRequest,
					Destination: "Price service", Address: address, Basis: "dispatch", External: "http.Client.Do", Source: "model",
					Path: "api/proxy.go", LineNo: 12, Column: 20, Values: []string{address}, Line: "Forwards the price request."},
				{ID: "send-b", ObjectID: library.Objects[0].ID, BoxID: "api", Direction: atlas.DirectionOut, Kind: atlas.BoundarySDK,
					Destination: "Trace collector", Basis: "configuration", External: "otlp.New", Source: "model",
					Path: "api/proxy.go", LineNo: 12, Column: 48, Values: []string{}, Line: "Configures trace export."},
				{ID: "config", BoxID: "api", Direction: atlas.DirectionOut, Kind: atlas.BoundaryConfig,
					Path: "api/proxy.go", LineNo: 8, Column: 1, Values: []string{"PRICE_URL"}, Line: "Reads configuration."},
				{ID: "receive", BoxID: "api", Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest,
					Path: "api/proxy.go", LineNo: 10, Column: 1, Method: "GET", Values: []string{"/proxy"}, Name: "GET /proxy", Line: "Receives requests.", FactID: "route"},
			},
		}
	}
	value := atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{makeTarget(library), makeTarget(app)}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{library.Target.ID: library, app.Target.ID: app}, value)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range indexes {
		if len(index.Groups) != 1 || index.Groups[0].Lane != LaneTriggers || len(index.Outbound) != 2 {
			t.Fatalf("incoming lane lost outgoing communication or promoted non-communication: %+v", index.Outbound)
		}
		program := map[string]programindex.Index{library.Target.ID: library, app.Target.ID: app}[index.Target.ID]
		for _, call := range index.Outbound {
			if call.SubjectID != program.Objects[0].ID || call.GroupID != index.Groups[0].ID || call.Source != "model" {
				t.Fatalf("call lost target-local subject, group or provenance: %+v", call)
			}
		}
		if index.Outbound[0].Address != address || index.Outbound[0].Location.Column != 20 || index.Outbound[1].Location.Column != 48 || index.Outbound[1].Address != "" || index.Outbound[1].Basis != "configuration" {
			t.Fatalf("two calls at one line merged, address rewritten or configuration became dispatch: %+v", index.Outbound)
		}
		copy := index.Snapshot()
		copy.Outbound[0].Values[0] = "changed"
		if index.Outbound[0].Values[0] != address {
			t.Fatal("outbound snapshot shares observed values")
		}
		raw, err := Encode(index)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := Decode(raw, program)
		if err != nil || !reflect.DeepEqual(restored.Outbound, index.Outbound) {
			t.Fatalf("saved outbound observations changed: %+v, %v", restored.Outbound, err)
		}
	}
}

func TestOutboundCustomProtocolDoesNotRequireDependencyLane(t *testing.T) {
	program := atlasTestProgram(t, "executable", "peer.go")
	boundary := atlas.Boundary{ID: "custom", Kind: atlas.BoundaryOther, Direction: atlas.DirectionOut,
		Destination: "Replication peer", Basis: "dispatch", Source: "model",
		Path: "peer.go", LineNo: 20, Column: 7, Line: "Sends replication records to a peer."}
	calls := projectOutbound(program, atlas.Target{Boundaries: []atlas.Boundary{boundary}}, nil, nil)
	if len(calls) != 1 || calls[0].Destination != "Replication peer" || calls[0].SubjectID != "" {
		t.Fatalf("custom communication was lost or attached to an unrelated declaration: %+v", calls)
	}
	if err := (Index{Outbound: calls}).validateOutbound(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOutboundUseStepsPassThroughDeclarationsOffTheMap(t *testing.T) {
	program := atlasTestProgram(t, "executable", "peer.go")
	object := program.Objects[0]
	boundary := atlas.Boundary{ID: "send", Kind: atlas.BoundaryOther, Direction: atlas.DirectionOut,
		Destination: "Replication peer", Source: "model", Path: "peer.go", LineNo: 20, Column: 7,
		Uses: []atlas.DestinationUse{{Steps: []atlas.DestinationStep{{SubjectID: "s1", Name: object.Name, Path: "peer.go", Line: 20}}}}}
	// A target whose map failed holds every declaration off the map.
	target := atlas.Target{Boundaries: []atlas.Boundary{boundary},
		OffMap: []atlas.OffMapFile{{ID: "f1", Reason: "map_failure", File: atlas.File{Path: "peer.go", Symbols: []atlas.Symbol{{ID: "s1", ObjectID: "o1", Name: object.Name}}}}}}
	calls := projectOutbound(program, target, nil, map[string]string{"o1": declarationKey(object)})
	if len(calls) != 1 || calls[0].Uses[0].Steps[0].SubjectID != object.ID {
		t.Fatalf("a use step through an off-map declaration lost its subject: %+v", calls)
	}
}
