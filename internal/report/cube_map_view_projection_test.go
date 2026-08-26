package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/activitysurface"
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/cubemap"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestCubeMapViewProjectsExactJoinsAndReverseNavigation(t *testing.T) {
	value := cubeMapViewFixture(t)
	programIndex := cubeMapProgramIndexFixture(t)
	view, err := NewCubeMapView(value, programIndex.Target, programIndex.SHA256)
	if err != nil {
		t.Fatalf("NewCubeMapView: %v", err)
	}
	if err := view.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(view.RefinedCore) != 2 || view.RefinedCore[0].Name != "Outbound delivery" ||
		len(view.RefinedGroups) != 2 || view.RefinedGroups[0].Authority != coremap.GroupAuthorityModel ||
		view.RefinedGroups[0].CoreBlockIDs[0] != view.RefinedCore[0].ID ||
		len(view.CoreObjects) != 1 || view.CoreObjects[0].DirectCallNodeID != "node-send" ||
		len(view.ActivitySurfaces) != 1 || view.ActivitySurfaces[0].Path == nil ||
		view.ActivitySurfaces[0].Path.Text != "/v1/send" {
		t.Fatalf("semantic projection = %#v", view)
	}
	if len(view.ReversePaths) != 1 || len(view.ReversePaths[0].Nodes) != 2 ||
		view.ReversePaths[0].Nodes[0].NodeID != "node-send" ||
		view.ReversePaths[0].Nodes[1].NodeID != "node-main" {
		t.Fatalf("reverse navigation path = %#v", view.ReversePaths)
	}
	if got := view.Coverage.Projection; got.IntegrationOperations.Omitted != 0 ||
		got.IntegrationOperations.Eligible != 1 || got.ReversePathNodes.Shown != 2 ||
		got.RefinedGroups.Eligible != 2 || got.RefinedGroups.Shown != 2 || got.RefinedGroups.Omitted != 0 ||
		got.SurfaceCoreBindings.Shown != 1 || got.EffectCoreBindings.Shown != 1 {
		t.Fatalf("projection coverage = %#v", got)
	}

	tampered := *view
	tampered.ReversePaths = append([]CubeMapViewReversePath(nil), view.ReversePaths...)
	tampered.ReversePaths[0].Nodes = append([]CubeMapViewSymbol(nil), view.ReversePaths[0].Nodes...)
	tampered.ReversePaths[0].Nodes[0].Location.Line++
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "endpoint facts") {
		t.Fatalf("tampered exact endpoint error = %v", err)
	}

	partial := *view
	partial.Coverage.Cube.Entrypoints.Observed++
	partial.Coverage.Cube.Entrypoints.Omitted = 1
	if err := partial.Validate(); err == nil || !strings.Contains(err.Error(), "candidate coverage") {
		t.Fatalf("partial candidate coverage error = %v", err)
	}

	overlappingGroups := *view
	overlappingGroups.RefinedGroups = append([]CoreMapViewGroup(nil), view.RefinedGroups...)
	for position := range overlappingGroups.RefinedGroups {
		overlappingGroups.RefinedGroups[position].CoreBlockIDs = append(
			[]string(nil), view.RefinedGroups[position].CoreBlockIDs...,
		)
	}
	overlappingGroups.RefinedGroups[1].CoreBlockIDs = append(
		overlappingGroups.RefinedGroups[1].CoreBlockIDs,
		overlappingGroups.RefinedGroups[0].CoreBlockIDs[0],
	)
	if err := overlappingGroups.Validate(); err != nil {
		t.Fatalf("overlapping refined group membership: %v", err)
	}

	badGroupCoverage := *view
	badGroupCoverage.Coverage.Projection.RefinedGroups.Shown--
	if err := badGroupCoverage.Validate(); err == nil || !strings.Contains(err.Error(), "projection coverage") {
		t.Fatalf("refined group projection coverage error = %v", err)
	}
}

func TestCubeMapViewRejectsInvalidSourceAndOverLimitProjection(t *testing.T) {
	if _, err := NewCubeMapView(cubemap.Map{}, programindex.Target{}, ""); err == nil || !strings.Contains(err.Error(), "invalid cube map") {
		t.Fatalf("invalid source error = %v", err)
	}

	value := cubeMapViewFixture(t)
	value.Entrypoints = make([]cubemap.Symbol, MaxCubeMapViewEntrypoints+1)
	if _, err := projectValidatedCubeMap(value, cubeMapProgramTargetFixture(t).ID); err == nil || !strings.Contains(err.Error(), "entrypoints exceed") {
		t.Fatalf("limit error = %v", err)
	}
}

func TestCubeMapViewAllowsOneBaselineChild(t *testing.T) {
	child := CubeMapViewCoreBlock{
		ID: "child", Name: "Child", Purpose: "Exact child evidence.",
		Files:   []CubeMapViewFile{{FileRef: "f-child", Path: "child.go"}},
		Symbols: []CubeMapViewCoreSymbol{}, Children: []CubeMapViewCoreBlock{},
	}
	parent := CubeMapViewCoreBlock{
		ID: "parent", Name: "Parent", Purpose: "Exact parent evidence.",
		Files: []CubeMapViewFile{}, Symbols: []CubeMapViewCoreSymbol{},
		Children: []CubeMapViewCoreBlock{child},
	}
	if err := validateCubeMapCoreBlocks(
		[]CubeMapViewCoreBlock{parent}, true, 0, make(map[string]struct{}),
	); err != nil {
		t.Fatalf("one producer-supported baseline child: %v", err)
	}
}

func cubeMapProgramTargetFixture(t *testing.T) programindex.Target {
	return cubeMapProgramIndexFixture(t).Target
}

func cubeMapProgramIndexFixture(t *testing.T) programindex.Index {
	t.Helper()
	location := &programindex.Location{Path: "cmd/app/main.go", Line: 10, Column: 1}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("7", 64),
		SourceSHA256:   strings.Repeat("8", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: "example.com/app/cmd/app",
			Selector:      "example.com/app/cmd/app",
			Sources:       []programindex.TargetSource{{FileRef: "f-main", Path: "cmd/app/main.go"}},
			AnchorFileRef: "f-main",
			Seeds:         []programindex.TargetSeedInput{{ObjectRef: "node-main", Kind: programindex.SeedCallable, Location: location}},
		},
		Objects: []programindex.ObjectInput{{
			SourceRef: "node-main", Kind: programindex.ObjectFunction, Name: "main",
			Visibility: programindex.VisibilityInternal, Location: location,
		}},
		Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 1, RelationsObserved: 0,
		},
	})
	if err != nil {
		t.Fatalf("cube program target fixture: %v", err)
	}
	return index
}

func cubeMapViewFixture(t *testing.T) cubemap.Map {
	t.Helper()
	target := cubeMapViewTarget(t)
	programIndex := cubeMapProgramIndexFixture(t)
	programTarget := programIndex.Target.Snapshot()
	entrypoint := cubemap.Symbol{
		NodeID: "node-main", Package: "example.com/app/cmd/app", Name: "main",
		Path: "cmd/app/main.go", Line: 10, Column: 1,
	}
	integration := cubemap.Symbol{
		NodeID: "node-send", Package: "example.com/app/internal/client", Name: "Send",
		Path: "internal/client/send.go", Line: 20, Column: 1,
	}
	baseline := coremap.Block{
		Name: "Runtime", Purpose: "Starts the application.",
		Files: []coremap.FileFact{{FileRef: "f-main", Path: "cmd/app/main.go"}},
	}
	refined := coremap.Block{
		Name: "Outbound delivery", Purpose: "Sends data to the remote API.",
		Files: []coremap.FileFact{{FileRef: "f-send", Path: "internal/client/send.go"}},
		Symbols: []coremap.SymbolFact{{
			NodeID: integration.NodeID, Package: integration.Package, Exported: true,
			Symbol: surfacediscovery.Symbol{Name: integration.Name},
			Declaration: surfacediscovery.Location{
				Path: integration.Path, Line: integration.Line, Column: integration.Column,
			},
			IncomingCalls: 1, OutgoingCalls: 1,
		}},
	}
	baseline.ID = cubeMapCoreBlockID(coremap.StageBaseline, programTarget.ID, baseline)
	refined.ID = cubeMapCoreBlockID(coremap.StageRefined, programTarget.ID, refined)
	coordination := coremap.Block{
		Name: "Request coordination", Purpose: "Coordinates outbound delivery from the application entrypoint.",
		Files:   []coremap.FileFact{{FileRef: "f-main", Path: "cmd/app/main.go"}},
		Symbols: append([]coremap.SymbolFact(nil), refined.Symbols...),
	}
	coordination.ID = cubeMapCoreBlockID(coremap.StageRefined, programTarget.ID, coordination)
	groups := []coremap.Group{
		{
			Authority: coremap.GroupAuthorityModel,
			Name:      "Delivery", Purpose: "Sends data to remote systems.",
			BlockIDs: []string{refined.ID},
		},
		{
			Authority: coremap.GroupAuthorityModel,
			Name:      "Coordination", Purpose: "Coordinates application work.",
			BlockIDs: []string{coordination.ID},
		},
	}
	for position := range groups {
		groups[position].ID = cubeMapCoreGroupID(programTarget.ID, groups[position])
	}
	operation := cubemap.IntegrationOperation{
		ExternalCallFamilyID: "family-http-do", DependencyID: "dep-net-http", PackagePath: "net/http",
		Receiver: "*Client", Name: "Do", Dispatch: surfacediscovery.ExternalCallStatic,
		Invocation: surfacediscovery.DirectCallSynchronous, WitnessCount: 1,
		Callsites: []cubemap.Location{{Path: "internal/client/send.go", Line: 25, Column: 4}},
	}
	surfaceID := "model-surface-" + strings.Repeat("1", 24)
	zero, one := 0, 1
	surfaceEffectCoverage := cubemap.SurfaceCoreEffectCoverage{
		Surfaces: 1, CoreBlocks: 2, Effects: 1, SurfaceCorePairs: 2, EffectCorePairs: 2,
		SelectedSurfaceCore: 1, SelectedEffectCore: 1, ModelCalled: true,
	}
	value := cubemap.Map{
		Version: cubemap.Version, SourceIndexSHA256: strings.Repeat("a", 64),
		ExternalCallIndexSHA256: strings.Repeat("b", 64), DependencyCatalogSHA256: strings.Repeat("c", 64),
		Core: coremap.Result{
			Version: coremap.Version, Repository: "app", CorpusRef: "corpus-1", Target: target,
			ProgramTarget: &programTarget, ProgramIndexSHA256: programIndex.SHA256,
			DirectCallSHA256: strings.Repeat("a", 64), CoreObjectSHA256: strings.Repeat("d", 64),
			Baseline: []coremap.Block{baseline}, Refined: []coremap.Block{refined, coordination},
			RefinedGroups: groups,
			Coverage: coremap.Coverage{
				TrackedFiles: 2, BaselineRoleFiles: 1, SymbolsAvailable: 1,
				BaselineBlocks: 1, BaselineFilesSelected: 1, RefinedBlocks: 2,
				RefinedFilesSelected: 2, RefinedSymbolsSelected: 1,
				RefinedGroups: 2, RefinedModelGroups: 2, RefinedGroupCalls: 1,
				SemanticFacts: 2, RefinedMapCalls: 1,
				DirectCallState: surfacediscovery.DirectCallIndexReady,
			},
			RequestSizes: coremap.RequestSizes{
				Baseline: coremap.StageRequestSize{
					Calls: 1, PayloadBytes: 1, ProviderBytes: 1, MaxPayloadBytes: 1, MaxProviderBytes: 1,
				},
				Refined: coremap.StageRequestSize{
					Calls: 1, PayloadBytes: 1, ProviderBytes: 1, MaxPayloadBytes: 1, MaxProviderBytes: 1,
				},
				Grouping: coremap.StageRequestSize{
					Calls: 1, PayloadBytes: 1, ProviderBytes: 1, MaxPayloadBytes: 1, MaxProviderBytes: 1,
				},
			},
		},
		CoreObjects: cubemap.CoreObjectProjection{
			Version: cubemap.CoreObjectProjectionVersion, CoreObjectIndexSHA256: strings.Repeat("d", 64),
			Callables: []gocoreobject.CallableDeclaration{{
				ID: "object-send", Kind: gocoreobject.CallableFunction, Package: integration.Package,
				Name: integration.Name, Signature: "func() error", Exported: true,
				Location:         gocoreobject.Location{Path: integration.Path, Line: integration.Line, Column: integration.Column},
				DirectCallNodeID: integration.NodeID,
			}},
			ReceiverTypes: []gocoreobject.TypeDeclaration{},
			Bindings: []cubemap.CoreObjectBinding{
				{CoreBlockID: refined.ID, ObjectID: "object-send", Role: cubemap.CoreObjectRepresentativeCallable},
				{CoreBlockID: coordination.ID, ObjectID: "object-send", Role: cubemap.CoreObjectRepresentativeCallable},
			},
			Coverage: cubemap.CoreObjectProjectionCoverage{
				CoreBlocksObserved: 2, RepresentativeSymbolClaims: 2, RepresentativeNodesObserved: 1,
				RepresentativeCallablesMatched: 1, CallableBindings: 2,
			},
			SHA256: strings.Repeat("e", 64),
		},
		ActivitySurfaces: activitysurface.Result{
			Version: activitysurface.Version, State: entrycall.StateReady, SubstrateSHA256: strings.Repeat("f", 64),
			Surfaces: []activitysurface.Surface{{
				ID: surfaceID, RootNodeID: entrypoint.NodeID, Kind: entrycall.SurfaceKindHTTPRoute,
				Role: entrycall.SurfaceRoleEntrySurface, Form: entrycall.SurfaceCandidateDirectCall,
				Registration: entrycall.Location{Path: "cmd/app/routes.go", Line: 12, Column: 2},
				Path: &activitysurface.Value{
					Kind: entrycall.SurfaceFactString, Text: "/v1/send",
					Location: entrycall.Location{Path: "cmd/app/routes.go", Line: 12, Column: 10},
				},
				Method: &activitysurface.Value{
					Kind: entrycall.SurfaceFactToken, Text: "POST",
					Location: entrycall.Location{Path: "cmd/app/routes.go", Line: 12, Column: 3},
				},
				Handler: &activitysurface.Value{
					Kind: entrycall.SurfaceFactCallable, Text: "main",
					Location: entrycall.Location{Path: entrypoint.Path, Line: entrypoint.Line, Column: entrypoint.Column},
				},
			}},
			Rejections: []activitysurface.RejectionCount{},
			Coverage: activitysurface.Coverage{
				Candidates: entrycall.SurfaceCandidateCoverage{
					ConsideredCandidates: 1, AdvertisedCandidates: 1,
					ConsideredFacts: 3, AdvertisedFacts: 3,
				},
				Selected: 1, ModelCalled: true,
			},
		},
		Entrypoints: []cubemap.Symbol{entrypoint},
		IntegrationDependencies: []cubemap.IntegrationDependency{{
			ID: "dep-net-http", Kind: dependencies.KindStdlib, Name: "http", PackagePath: "net/http",
			Importers: []cubemap.Importer{{PackagePath: integration.Package, RepositoryPath: "internal/client"}},
		}},
		IntegrationSymbols: []cubemap.IntegrationSymbol{{
			Symbol: integration, DependencyIDs: []string{"dep-net-http"}, Operations: []cubemap.IntegrationOperation{operation},
		}},
		Paths: []cubemap.Path{{
			EntrypointNodeID: entrypoint.NodeID, IntegrationNodeID: integration.NodeID,
			Nodes: []cubemap.Symbol{entrypoint, integration},
		}},
		SurfaceCoreEffects: &cubemap.SurfaceCoreEffectBindings{
			Version: cubemap.SurfaceCoreEffectBindingsVersion, TargetRef: target.Ref,
			DirectCallSHA256: strings.Repeat("a", 64), AuthoritySHA256: strings.Repeat("1", 64),
			SurfaceCore: []cubemap.SurfaceCoreBinding{{
				SurfaceID: surfaceID, CoreBlockID: refined.ID,
				Relation: cubemap.AnchorReachesCore, MinHops: &one,
			}},
			EffectCore: []cubemap.EffectCoreBinding{{
				ExternalCallFamilyID: operation.ExternalCallFamilyID, CallerNodeID: integration.NodeID,
				CoreBlockID: refined.ID, Relation: cubemap.AnchorCoreSameSymbol, MinHops: &zero,
			}},
			Coverage: surfaceEffectCoverage,
		},
		Coverage: cubemap.Coverage{
			DependencyCatalog: cubemap.DependencyCatalogCoverage{
				State: dependencies.CoverageComplete, Reasons: []cubemap.DependencyOmissionCount{},
			},
			Entrypoints:             cubemap.CandidateCoverage{Observed: 1, Advertised: 1, ModelCalled: true},
			IntegrationDependencies: cubemap.CandidateCoverage{Observed: 1, Advertised: 1, ModelCalled: true},
			IntegrationSymbols:      cubemap.CandidateCoverage{Observed: 1, Advertised: 1, ModelCalled: true},
			GoFilesObserved:         2, GoFilesParsed: 2,
			ExternalCallFamiliesObserved: 1, ExternalCallFamiliesMatched: 1,
			ExternalCalls: surfacediscovery.ExternalCallIndexCoverage{FamiliesIndexed: 1},
		},
	}
	sort.Slice(value.CoreObjects.Bindings, func(i, j int) bool {
		return value.CoreObjects.Bindings[i].CoreBlockID < value.CoreObjects.Bindings[j].CoreBlockID
	})
	coreObjectsForSeal := value.CoreObjects
	coreObjectsForSeal.SHA256 = ""
	coreObjectBytes, err := json.Marshal(coreObjectsForSeal)
	if err != nil {
		t.Fatal(err)
	}
	coreObjectDigest := sha256.Sum256(coreObjectBytes)
	value.CoreObjects.SHA256 = hex.EncodeToString(coreObjectDigest[:])
	return value
}

func cubeMapCoreBlockID(stage coremap.Stage, targetID string, block coremap.Block) string {
	keys := make([]string, 0, len(block.Files)+len(block.Symbols)+len(block.Children))
	for _, file := range block.Files {
		keys = append(keys, "f:"+string(file.FileRef))
	}
	for _, symbol := range block.Symbols {
		keys = append(keys, "s:"+symbol.NodeID)
	}
	for _, child := range block.Children {
		keys = append(keys, "c:"+child.ID)
	}
	sort.Strings(keys)
	wire, _ := json.Marshal(struct {
		Contract string        `json:"contract"`
		Stage    coremap.Stage `json:"stage"`
		Target   string        `json:"target"`
		Name     string        `json:"name"`
		Purpose  string        `json:"purpose"`
		Evidence []string      `json:"evidence"`
	}{
		Contract: "coremap-block-v2", Stage: stage, Target: targetID,
		Name: block.Name, Purpose: block.Purpose, Evidence: keys,
	})
	digest := sha256.Sum256(wire)
	return "core-" + hex.EncodeToString(digest[:8])
}

func cubeMapCoreGroupID(targetID string, group coremap.Group) string {
	keys := append([]string(nil), group.BlockIDs...)
	sort.Strings(keys)
	wire, _ := json.Marshal(struct {
		Contract    string   `json:"contract"`
		Target      string   `json:"target"`
		Name        string   `json:"name"`
		Purpose     string   `json:"purpose"`
		Memberships []string `json:"memberships"`
	}{
		Contract: "coremap-group-v2", Target: targetID,
		Name: group.Name, Purpose: group.Purpose, Memberships: keys,
	})
	digest := sha256.Sum256(wire)
	return "core-group-" + hex.EncodeToString(digest[:8])
}

func cubeMapViewTarget(t *testing.T) analysistarget.Target {
	t.Helper()
	target := analysistarget.Target{
		Version: analysistarget.Version, Kind: analysistarget.KindExecutablePackage,
		ModuleID: "module-1", ModulePath: "example.com/app", ModuleDir: ".",
		PackagePath: "example.com/app/cmd/app", PackageDir: "cmd/app",
		RootBoundary: analysistarget.RootBoundaryExactPackageMains,
		Roots:        []analysistarget.Root{{Path: "cmd/app/main.go", Line: 10}},
	}
	wire, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	target.Ref = "at-" + hex.EncodeToString(digest[:12])
	if err := target.Validate(); err != nil {
		t.Fatalf("target fixture: %v", err)
	}
	return target
}
