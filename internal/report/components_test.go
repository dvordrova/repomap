package report

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

func TestBuildComponentsGroupsAnchorsAndBuildsGroundedRelation(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		OpenablePaths: []string{"batch.go", "wal/wal.go"},
		HighLevelMap: []Subsystem{
			{
				Name:         "Batch Operations",
				Role:         componentmap.RoleDomain,
				WhyItMatters: "groups writes",
				Evidence:     []string{"batch.go defines Batch", "batch.go:395 discusses fsync"},
			},
			{
				Name:         "Write-Ahead Log",
				WhyItMatters: "persists writes",
				Evidence:     []string{"wal/wal.go is the WAL implementation"},
			},
		},
		CandidateDirections: []CandidateDirection{{
			ID:          "write-batch",
			LikelyFiles: []string{"batch.go", "wal/wal.go"},
		}},
		RepositoryGraph: &RepositoryGraph{
			Modules:      []ModuleInfo{{Path: "example.com/project"}},
			PackageEdges: []EdgeInfo{{From: "example.com/project", To: "example.com/project/wal"}},
		},
		sourceSignals: []SourceSignal{{
			Path:     "batch.go",
			Line:     395,
			Category: "storage_durability",
			Snippet:  "// waiting for WAL fsync",
			Reason:   "fsync/sync call for durability",
		}},
	}

	buildComponents(data)
	if len(data.Components) != 2 {
		t.Fatalf("components = %d, want 2", len(data.Components))
	}
	batch := data.Components[0]
	if batch.ID == "" || batch.Name != "Batch Operations" || batch.PrimaryPackage != "example.com/project" {
		t.Fatalf("batch component = %#v", batch)
	}
	if batch.Role != componentmap.RoleDomain || batch.RoleBasis != evidence.CertaintyHypothesis {
		t.Fatalf("batch role = %q (%q), want domain hypothesis", batch.Role, batch.RoleBasis)
	}
	if len(batch.AnchorGroups) != 1 {
		t.Fatalf("batch anchors = %#v", batch.AnchorGroups)
	}
	anchor := batch.AnchorGroups[0]
	if anchor.Path != "batch.go" || anchor.Grounding != anchorGroundingLine ||
		len(anchor.Locations) != 1 || anchor.Locations[0].Line != 395 ||
		len(anchor.ModelNotes) != 2 || len(anchor.LocalContext) != 1 || !anchor.CanListSymbols {
		t.Fatalf("batch anchor = %#v", anchor)
	}
	if !reflect.DeepEqual(batch.RelatedFlowIDs, []string{"write-batch"}) {
		t.Fatalf("batch related flows = %v", batch.RelatedFlowIDs)
	}
	if len(data.ComponentRelations) != 1 {
		t.Fatalf("relations = %#v", data.ComponentRelations)
	}
	relation := data.ComponentRelations[0]
	if relation.From != data.Components[0].ID || relation.To != data.Components[1].ID ||
		relation.Kind != "package_import" || len(relation.Evidence) != 1 {
		t.Fatalf("relation = %#v", relation)
	}

	firstComponentID := data.Components[0].ID
	firstAnchorID := data.Components[0].AnchorGroups[0].ID
	buildComponents(data)
	if data.Components[0].ID != firstComponentID || data.Components[0].AnchorGroups[0].ID != firstAnchorID {
		t.Fatal("component or anchor ID changed for identical input")
	}
}

func TestBuildComponentRelationsUsesEveryComponentPackage(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepositoryGraph: &RepositoryGraph{PackageEdges: []EdgeInfo{{
			From: "example.com/project/adapter",
			To:   "example.com/project/state",
		}}},
		Components: []Component{
			{ID: "surface", PrimaryPackage: "example.com/project/cmd", Packages: []string{
				"example.com/project/cmd",
				"example.com/project/adapter",
			}},
			{ID: "storage", PrimaryPackage: "example.com/project", Packages: []string{
				"example.com/project",
				"example.com/project/state",
			}},
		},
	}

	relations := buildComponentRelations(data)
	if len(relations) != 1 {
		t.Fatalf("relations = %#v, want one relation through secondary packages", relations)
	}
	if relations[0].From != "surface" || relations[0].To != "storage" ||
		len(relations[0].Evidence) != 1 || relations[0].Certainty != evidence.CertaintyStatic {
		t.Fatalf("relation = %#v", relations[0])
	}
}

func TestComponentRelationRequiresEndpointWitnesses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		components []Component
	}{
		{
			name: "ambiguous source package owner",
			components: []Component{
				{ID: "surface", Packages: []string{"example.com/project/adapter"}},
				{ID: "domain", Packages: []string{"example.com/project/adapter"}},
				{ID: "storage", Packages: []string{"example.com/project/state"}},
			},
		},
		{
			name: "ambiguous target package owner",
			components: []Component{
				{ID: "surface", Packages: []string{"example.com/project/adapter"}},
				{ID: "domain", Packages: []string{"example.com/project/state"}},
				{ID: "storage", Packages: []string{"example.com/project/state"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rawEdge := EdgeInfo{
				From: "example.com/project/adapter",
				To:   "example.com/project/state",
			}
			data := &ReportData{
				RepositoryGraph: &RepositoryGraph{PackageEdges: []EdgeInfo{rawEdge}},
				Components:      test.components,
			}

			if relations := buildComponentRelations(data); len(relations) != 0 {
				t.Fatalf("relations = %#v, want no promotion without unique endpoint owners", relations)
			}
			if !reflect.DeepEqual(data.RepositoryGraph.PackageEdges, []EdgeInfo{rawEdge}) {
				t.Fatalf("raw package edges = %#v, want %#v", data.RepositoryGraph.PackageEdges, []EdgeInfo{rawEdge})
			}
		})
	}
}

func TestRepositoryPackageForFileUsesLongestModuleDirectory(t *testing.T) {
	t.Parallel()

	graph := &RepositoryGraph{Modules: []ModuleInfo{
		{Path: "example.com/project"},
		{Path: "example.com/project/server/v2", Dir: "server"},
	}}
	tests := map[string]string{
		"main.go":                     "example.com/project",
		"client/client.go":            "example.com/project/client",
		"server/main.go":              "example.com/project/server/v2",
		"server/runtime/worker.go":    "example.com/project/server/v2/runtime",
		"serverish/runtime/worker.go": "example.com/project/serverish/runtime",
	}
	for file, want := range tests {
		if got := repositoryPackageForFile(graph, file); got != want {
			t.Errorf("repositoryPackageForFile(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestRepositoryPackageForFileUsesExactPackageOwnership(t *testing.T) {
	t.Parallel()

	graph := &RepositoryGraph{
		Version: 2,
		Modules: []ModuleInfo{
			{ID: "outer", Path: "example.com/foo", Dir: ""},
			{ID: "nested", Path: "example.com/foobar/v2", Dir: "server"},
		},
		Packages: []PackageInfo{
			{CanonicalPath: "example.com/foo/serverish", ModuleID: "outer", Dir: "serverish", DisplayPath: "serverish", Locality: "local"},
			{CanonicalPath: "example.com/foobar/v2/internal/worker", ModuleID: "nested", Dir: "server/internal/worker", DisplayPath: "internal/worker", Locality: "local"},
		},
	}
	if got := repositoryPackageForFile(graph, "server/internal/worker/work.go"); got != "example.com/foobar/v2/internal/worker" {
		t.Fatalf("nested package = %q", got)
	}
	if got := repositoryPackageForFile(graph, "serverish/file.go"); got != "example.com/foo/serverish" {
		t.Fatalf("prefix-collision package = %q", got)
	}
	if got := repositoryPackageForFile(graph, "server/unowned/file.go"); got != "" {
		t.Fatalf("unowned package = %q", got)
	}
}

func TestBuildComponentsAddsSymbolAnchorsOnlyFromSemanticallyRelatedSavedDirection(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		OpenablePaths: []string{
			"api/rpc.proto",
			"contrib/raftexample/raft.go",
			"server/api/rafthttp/peer.go",
			"server/api/v3rpc/grpc.go",
			"server/main.go",
			"server/raft.go",
			"server/txn/put.go",
			"tools/check-grpc/main.go",
		},
		HighLevelMap: []Subsystem{
			{
				Name:         "Raft Consensus Module",
				WhyItMatters: "replication",
				Evidence:     []string{"server/api/rafthttp/ provides transport"},
			},
			{
				Name:         "gRPC API Layer",
				WhyItMatters: "client requests",
				Evidence:     []string{"api/rpc.proto defines the service"},
			},
		},
		CandidateDirections: []CandidateDirection{
			{
				ID:          "raft-election",
				Name:        "Raft Leader Election Flow",
				LikelyFiles: []string{"server/main.go", "server/api/rafthttp/peer.go"},
			},
			{
				ID:          "grpc-put",
				Name:        "gRPC Put Request Flow",
				LikelyFiles: []string{"api/rpc.proto", "server/main.go"},
			},
		},
		Flows: []FlowData{
			{
				ID: "raft-election",
				BundleFiles: []FileItem{
					{Path: "server/raft.go"},
					{Path: "contrib/raftexample/raft.go"},
				},
			},
			{
				ID: "grpc-put",
				BundleFiles: []FileItem{
					{Path: "server/api/v3rpc/grpc.go"},
					{Path: "server/txn/put.go"},
					{Path: "tools/check-grpc/main.go"},
				},
			},
		},
	}

	buildComponents(data)
	if len(data.Components) != 2 {
		t.Fatalf("components = %d", len(data.Components))
	}
	raft := data.Components[0]
	if !containsString(raft.RelatedFlowIDs, "raft-election") || !componentHasAnchor(raft, "server/raft.go") ||
		!componentHasAnchor(raft, "server/api/rafthttp/peer.go") {
		t.Fatalf("raft component did not receive related verified anchors: %#v", raft)
	}
	if componentHasAnchor(raft, "server/main.go") {
		t.Fatalf("generic main.go became a Raft anchor: %#v", raft.AnchorGroups)
	}
	grpc := data.Components[1]
	if !containsString(grpc.RelatedFlowIDs, "grpc-put") || !componentHasAnchor(grpc, "api/rpc.proto") ||
		!componentHasAnchor(grpc, "server/api/v3rpc/grpc.go") || !componentHasAnchor(grpc, "server/txn/put.go") {
		t.Fatalf("gRPC component did not retain proto and related Go anchors: %#v", grpc)
	}
	if componentHasAnchor(grpc, "server/main.go") || componentHasAnchor(grpc, "tools/check-grpc/main.go") {
		t.Fatalf("generic/tool main.go became a gRPC anchor: %#v", grpc.AnchorGroups)
	}
}

func componentHasAnchor(component Component, want string) bool {
	for _, anchor := range component.AnchorGroups {
		if anchor.Path == want {
			return true
		}
	}
	return false
}
