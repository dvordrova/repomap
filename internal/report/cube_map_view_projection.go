package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

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

const (
	CubeMapViewVersion = 3

	MaxCubeMapViewCoreObjects           = 4_096
	MaxCubeMapViewCoreObjectBindings    = 8_192
	MaxCubeMapViewActivitySurfaces      = 2_048
	MaxCubeMapViewEntrypoints           = 4_096
	MaxCubeMapViewDependencies          = 4_096
	MaxCubeMapViewDependencyImporters   = 16_384
	MaxCubeMapViewIntegrationSymbols    = 4_096
	MaxCubeMapViewIntegrationOperations = 16_384
	MaxCubeMapViewIntegrationCallsites  = 65_536
	MaxCubeMapViewReversePaths          = 4_096
	MaxCubeMapViewReversePathNodes      = 65_536
	MaxCubeMapViewSurfaceCoreBindings   = 8_192
	MaxCubeMapViewEffectCoreBindings    = 8_192
	MaxCubeMapViewJSONBytes             = 16 << 20
	maxCubeMapViewTextBytes             = 16 << 10
)

// CubeMapView is the report-owned, browser-safe projection of one validated
// CubeMap. It carries the producer's human names and exact local joins only;
// presentation does not infer a responsibility, entrypoint, integration, or
// path from spelling or order.
type CubeMapView struct {
	Version         int                   `json:"version"`
	ProgramTargetID string                `json:"program_target_id"`
	Target          analysistarget.Target `json:"target"`

	SourceIndexSHA256          string `json:"source_index_sha256,omitempty"`
	ExternalCallIndexSHA256    string `json:"external_call_index_sha256,omitempty"`
	DependencyCatalogSHA256    string `json:"dependency_catalog_sha256,omitempty"`
	CoreObjectIndexSHA256      string `json:"core_object_index_sha256,omitempty"`
	CoreObjectProjectionSHA256 string `json:"core_object_projection_sha256,omitempty"`
	ActivitySubstrateSHA256    string `json:"activity_substrate_sha256,omitempty"`

	BaselineCore  []CubeMapViewCoreBlock `json:"baseline_core"`
	RefinedCore   []CubeMapViewCoreBlock `json:"refined_core"`
	RefinedGroups []CoreMapViewGroup     `json:"refined_groups"`

	CoreObjects        []CubeMapViewCoreObject        `json:"core_objects"`
	CoreObjectBindings []CubeMapViewCoreObjectBinding `json:"core_object_bindings"`

	ActivitySurfaces        []CubeMapViewActivitySurface       `json:"activity_surfaces"`
	Entrypoints             []CubeMapViewSymbol                `json:"entrypoints"`
	IntegrationDependencies []CubeMapViewIntegrationDependency `json:"integration_dependencies"`
	IntegrationSymbols      []CubeMapViewIntegrationSymbol     `json:"integration_symbols"`
	ReversePaths            []CubeMapViewReversePath           `json:"reverse_paths"`

	SurfaceCoreEffects *CubeMapViewSurfaceCoreEffects `json:"surface_core_effects,omitempty"`
	Coverage           CubeMapViewCoverage            `json:"coverage"`
}

type CubeMapViewLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type CubeMapViewFile struct {
	FileRef string `json:"file_ref"`
	Path    string `json:"path"`
}

type CubeMapViewSymbol struct {
	NodeID   string              `json:"node_id"`
	Package  string              `json:"package"`
	Name     string              `json:"name"`
	Location CubeMapViewLocation `json:"location"`
}

type CubeMapViewCoreSymbol struct {
	Symbol        CubeMapViewSymbol `json:"symbol"`
	Exported      bool              `json:"exported"`
	IncomingCalls int               `json:"incoming_calls"`
	OutgoingCalls int               `json:"outgoing_calls"`
}

// CubeMapViewCoreBlock keeps baseline hierarchy and refined representative
// symbols in one closed shape. Baseline blocks have Children but no Symbols;
// refined blocks have Symbols but no Children.
type CubeMapViewCoreBlock struct {
	ID       string                  `json:"id"`
	Name     string                  `json:"name"`
	Purpose  string                  `json:"purpose"`
	Files    []CubeMapViewFile       `json:"files"`
	Symbols  []CubeMapViewCoreSymbol `json:"representative_symbols"`
	Children []CubeMapViewCoreBlock  `json:"children"`
}

type CubeMapViewCoreObjectCategory string

const (
	CubeMapViewCoreCallable     CubeMapViewCoreObjectCategory = "callable"
	CubeMapViewCoreReceiverType CubeMapViewCoreObjectCategory = "receiver_type"
)

// CubeMapViewCoreObject is a declaration selected by the exact CoreObject
// projection. DeclarationKind is producer-owned (function/method or the
// concrete Go type kind); Category only keeps the two source collections
// unambiguous for binding validation.
type CubeMapViewCoreObject struct {
	ID               string                        `json:"id"`
	Category         CubeMapViewCoreObjectCategory `json:"category"`
	DeclarationKind  string                        `json:"declaration_kind"`
	Package          string                        `json:"package"`
	Name             string                        `json:"name"`
	Receiver         string                        `json:"receiver,omitempty"`
	Signature        string                        `json:"signature,omitempty"`
	Exported         bool                          `json:"exported"`
	Location         CubeMapViewLocation           `json:"location"`
	DirectCallNodeID string                        `json:"direct_call_node_id,omitempty"`
}

type CubeMapViewCoreObjectBinding struct {
	CoreBlockID string                        `json:"core_block_id"`
	ObjectID    string                        `json:"object_id"`
	Role        cubemap.CoreObjectBindingRole `json:"role"`
}

type CubeMapViewSurfaceValue struct {
	Kind     string              `json:"kind"`
	Text     string              `json:"text"`
	Location CubeMapViewLocation `json:"location"`
}

type CubeMapViewActivitySurface struct {
	ID           string                   `json:"id"`
	RootNodeID   string                   `json:"root_node_id"`
	Kind         string                   `json:"kind"`
	Role         string                   `json:"role"`
	Form         string                   `json:"form"`
	Registration CubeMapViewLocation      `json:"registration"`
	Identity     *CubeMapViewSurfaceValue `json:"identity,omitempty"`
	Method       *CubeMapViewSurfaceValue `json:"method,omitempty"`
	Path         *CubeMapViewSurfaceValue `json:"path,omitempty"`
	Handler      *CubeMapViewSurfaceValue `json:"handler,omitempty"`
}

type CubeMapViewImporter struct {
	PackagePath    string `json:"package_path"`
	RepositoryPath string `json:"repository_path"`
}

type CubeMapViewIntegrationDependency struct {
	ID            string                `json:"id"`
	Kind          dependencies.Kind     `json:"kind"`
	Name          string                `json:"name"`
	ModulePath    string                `json:"module_path,omitempty"`
	ModuleVersion string                `json:"module_version,omitempty"`
	PackagePath   string                `json:"package_path"`
	Importers     []CubeMapViewImporter `json:"importers"`
}

type CubeMapViewIntegrationOperation struct {
	ExternalCallFamilyID string                                `json:"external_call_family_id"`
	DependencyID         string                                `json:"dependency_id"`
	PackagePath          string                                `json:"package_path"`
	Receiver             string                                `json:"receiver,omitempty"`
	Name                 string                                `json:"name"`
	Dispatch             surfacediscovery.ExternalCallDispatch `json:"dispatch"`
	Invocation           surfacediscovery.DirectCallInvocation `json:"invocation"`
	WitnessCount         int                                   `json:"witness_count"`
	Callsites            []CubeMapViewLocation                 `json:"callsites"`
	CallsitesOmitted     int                                   `json:"callsites_omitted"`
}

type CubeMapViewIntegrationSymbol struct {
	Symbol        CubeMapViewSymbol                 `json:"symbol"`
	DependencyIDs []string                          `json:"dependency_ids"`
	Operations    []CubeMapViewIntegrationOperation `json:"operations"`
}

// CubeMapViewReversePath is the exact producer path presented from an
// integration caller upward to its activity entrypoint. Nodes are the exact
// input nodes in reverse order; this is a navigation direction, not a claim
// that calls execute in reverse.
type CubeMapViewReversePath struct {
	IntegrationNodeID string              `json:"integration_node_id"`
	EntrypointNodeID  string              `json:"entrypoint_node_id"`
	Nodes             []CubeMapViewSymbol `json:"nodes"`
}

type CubeMapViewSurfaceCoreBinding struct {
	SurfaceID   string                     `json:"surface_id"`
	CoreBlockID string                     `json:"core_block_id"`
	Relation    cubemap.AnchorCoreRelation `json:"relation"`
	MinHops     *int                       `json:"min_hops,omitempty"`
}

type CubeMapViewEffectCoreBinding struct {
	ExternalCallFamilyID string                     `json:"external_call_family_id"`
	CallerNodeID         string                     `json:"caller_node_id"`
	CoreBlockID          string                     `json:"core_block_id"`
	Relation             cubemap.AnchorCoreRelation `json:"relation"`
	MinHops              *int                       `json:"min_hops,omitempty"`
}

type CubeMapViewSurfaceCoreEffects struct {
	AuthoritySHA256 string                            `json:"authority_sha256,omitempty"`
	SurfaceCore     []CubeMapViewSurfaceCoreBinding   `json:"surface_core"`
	EffectCore      []CubeMapViewEffectCoreBinding    `json:"effect_core"`
	Coverage        cubemap.SurfaceCoreEffectCoverage `json:"coverage"`
}

// CubeMapViewCoverage preserves every producer coverage ledger and adds a
// report projection ledger. This projection never truncates: each collection
// is either present in full or NewCubeMapView returns an explicit error.
type CubeMapViewCoverage struct {
	Cube                 cubemap.Coverage                     `json:"cube"`
	Core                 coremap.Coverage                     `json:"core"`
	CoreObjects          cubemap.CoreObjectProjectionCoverage `json:"core_objects"`
	ActivityState        entrycall.State                      `json:"activity_state"`
	ActivityClosedReason entrycall.ClosedReason               `json:"activity_closed_reason,omitempty"`
	ActivitySurfaces     activitysurface.Coverage             `json:"activity_surfaces"`
	SurfaceCoreEffects   *cubemap.SurfaceCoreEffectCoverage   `json:"surface_core_effects,omitempty"`
	Projection           CubeMapViewProjectionCoverage        `json:"projection"`
}

type CubeMapViewProjectionCoverage struct {
	BaselineCore          CubeMapViewCollectionCoverage `json:"baseline_core"`
	RefinedCore           CubeMapViewCollectionCoverage `json:"refined_core"`
	RefinedGroups         CubeMapViewCollectionCoverage `json:"refined_groups"`
	CoreFiles             CubeMapViewCollectionCoverage `json:"core_files"`
	CoreSymbols           CubeMapViewCollectionCoverage `json:"core_symbols"`
	CoreObjects           CubeMapViewCollectionCoverage `json:"core_objects"`
	CoreObjectBindings    CubeMapViewCollectionCoverage `json:"core_object_bindings"`
	ActivitySurfaces      CubeMapViewCollectionCoverage `json:"activity_surfaces"`
	Entrypoints           CubeMapViewCollectionCoverage `json:"entrypoints"`
	Dependencies          CubeMapViewCollectionCoverage `json:"dependencies"`
	DependencyImporters   CubeMapViewCollectionCoverage `json:"dependency_importers"`
	IntegrationSymbols    CubeMapViewCollectionCoverage `json:"integration_symbols"`
	IntegrationOperations CubeMapViewCollectionCoverage `json:"integration_operations"`
	IntegrationCallsites  CubeMapViewCollectionCoverage `json:"integration_callsites"`
	ReversePaths          CubeMapViewCollectionCoverage `json:"reverse_paths"`
	ReversePathNodes      CubeMapViewCollectionCoverage `json:"reverse_path_nodes"`
	SurfaceCoreBindings   CubeMapViewCollectionCoverage `json:"surface_core_bindings"`
	EffectCoreBindings    CubeMapViewCollectionCoverage `json:"effect_core_bindings"`
}

type CubeMapViewCollectionCoverage struct {
	Eligible int `json:"eligible"`
	Shown    int `json:"shown"`
	Omitted  int `json:"omitted"`
}

type cubeMapViewCounts struct {
	baselineCore, refinedCore, refinedGroups, coreFiles, coreSymbols, coreObjects, coreObjectBindings int
	activitySurfaces, entrypoints, dependencies, dependencyImporters                                  int
	integrationSymbols, integrationOperations, integrationCallsites                                   int
	reversePaths, reversePathNodes, surfaceCoreBindings, effectCoreBindings                           int
}

// NewCubeMapView first accepts the complete producer validation contract,
// then performs only exact copies and ID joins into the presentation shape.
func NewCubeMapView(
	value cubemap.Map,
	target programindex.Target,
	programIndexSHA256 string,
) (*CubeMapView, error) {
	if err := cubemap.Validate(value); err != nil {
		return nil, fmt.Errorf("cube map view: invalid cube map: %w", err)
	}
	if err := validateCubeMapProgramTarget(value.Core.Target, target); err != nil {
		return nil, fmt.Errorf("cube map view: %w", err)
	}
	if value.Core.ProgramTarget == nil || !reflect.DeepEqual(*value.Core.ProgramTarget, target.Snapshot()) {
		return nil, fmt.Errorf("cube map view: core map program target authority mismatch")
	}
	if !validCubeMapViewSHA256(programIndexSHA256) || value.Core.ProgramIndexSHA256 != programIndexSHA256 {
		return nil, fmt.Errorf("cube map view: core map ProgramIndex authority mismatch")
	}
	return projectValidatedCubeMap(value, target.ID)
}

func projectValidatedCubeMap(value cubemap.Map, programTargetID string) (*CubeMapView, error) {
	if value.Core.CoreObjectSHA256 != value.CoreObjects.CoreObjectIndexSHA256 {
		return nil, fmt.Errorf("cube map view: core object source authority mismatch")
	}
	if value.SurfaceCoreEffects != nil &&
		(value.SurfaceCoreEffects.TargetRef != value.Core.Target.Ref ||
			value.SurfaceCoreEffects.DirectCallSHA256 != value.SourceIndexSHA256) {
		return nil, fmt.Errorf("cube map view: surface/core/effect source authority mismatch")
	}
	counts := countCubeMapViewSource(value)
	if err := validateCubeMapViewLimits(counts); err != nil {
		return nil, err
	}

	view := &CubeMapView{
		Version: CubeMapViewVersion, ProgramTargetID: programTargetID, Target: value.Core.Target.Snapshot(),
		SourceIndexSHA256: value.SourceIndexSHA256, ExternalCallIndexSHA256: value.ExternalCallIndexSHA256,
		DependencyCatalogSHA256:    value.DependencyCatalogSHA256,
		CoreObjectIndexSHA256:      value.CoreObjects.CoreObjectIndexSHA256,
		CoreObjectProjectionSHA256: value.CoreObjects.SHA256,
		ActivitySubstrateSHA256:    value.ActivitySurfaces.SubstrateSHA256,
		BaselineCore:               []CubeMapViewCoreBlock{}, RefinedCore: []CubeMapViewCoreBlock{},
		RefinedGroups: []CoreMapViewGroup{},
		CoreObjects:   []CubeMapViewCoreObject{}, CoreObjectBindings: []CubeMapViewCoreObjectBinding{},
		ActivitySurfaces: []CubeMapViewActivitySurface{}, Entrypoints: []CubeMapViewSymbol{},
		IntegrationDependencies: []CubeMapViewIntegrationDependency{},
		IntegrationSymbols:      []CubeMapViewIntegrationSymbol{}, ReversePaths: []CubeMapViewReversePath{},
	}
	for _, block := range value.Core.Baseline {
		view.BaselineCore = append(view.BaselineCore, projectCubeMapCoreBlock(block))
	}
	for _, block := range value.Core.Refined {
		view.RefinedCore = append(view.RefinedCore, projectCubeMapCoreBlock(block))
	}
	for _, group := range value.Core.RefinedGroups {
		view.RefinedGroups = append(view.RefinedGroups, CoreMapViewGroup{
			ID: group.ID, Name: group.Name, Purpose: group.Purpose,
			CoreBlockIDs: append([]string(nil), group.BlockIDs...),
		})
	}
	for _, callable := range value.CoreObjects.Callables {
		view.CoreObjects = append(view.CoreObjects, projectCubeMapCallable(callable))
	}
	for _, declaration := range value.CoreObjects.ReceiverTypes {
		view.CoreObjects = append(view.CoreObjects, projectCubeMapReceiverType(declaration))
	}
	sort.Slice(view.CoreObjects, func(i, j int) bool { return view.CoreObjects[i].ID < view.CoreObjects[j].ID })
	for _, binding := range value.CoreObjects.Bindings {
		view.CoreObjectBindings = append(view.CoreObjectBindings, CubeMapViewCoreObjectBinding{
			CoreBlockID: binding.CoreBlockID, ObjectID: binding.ObjectID, Role: binding.Role,
		})
	}
	for _, surface := range value.ActivitySurfaces.Surfaces {
		view.ActivitySurfaces = append(view.ActivitySurfaces, projectCubeMapActivitySurface(surface))
	}
	for _, symbol := range value.Entrypoints {
		view.Entrypoints = append(view.Entrypoints, projectCubeMapSymbol(symbol))
	}
	for _, dependency := range value.IntegrationDependencies {
		view.IntegrationDependencies = append(view.IntegrationDependencies, projectCubeMapDependency(dependency))
	}
	for _, integration := range value.IntegrationSymbols {
		view.IntegrationSymbols = append(view.IntegrationSymbols, projectCubeMapIntegration(integration))
	}
	for _, path := range value.Paths {
		view.ReversePaths = append(view.ReversePaths, projectCubeMapReversePath(path))
	}
	if value.SurfaceCoreEffects != nil {
		view.SurfaceCoreEffects = projectCubeMapSurfaceCoreEffects(*value.SurfaceCoreEffects)
	}
	view.Coverage = CubeMapViewCoverage{
		Cube: value.Coverage, Core: value.Core.Coverage, CoreObjects: value.CoreObjects.Coverage,
		ActivityState: value.ActivitySurfaces.State, ActivityClosedReason: value.ActivitySurfaces.ClosedReason,
		ActivitySurfaces: value.ActivitySurfaces.Coverage,
		Projection:       cubeMapViewProjectionCoverage(counts),
	}
	if value.SurfaceCoreEffects != nil {
		coverage := value.SurfaceCoreEffects.Coverage
		view.Coverage.SurfaceCoreEffects = &coverage
	}
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("cube map view: invalid projection: %w", err)
	}
	return view, nil
}

func projectCubeMapCoreBlock(block coremap.Block) CubeMapViewCoreBlock {
	result := CubeMapViewCoreBlock{
		ID: block.ID, Name: block.Name, Purpose: block.Purpose,
		Files: []CubeMapViewFile{}, Symbols: []CubeMapViewCoreSymbol{}, Children: []CubeMapViewCoreBlock{},
	}
	for _, file := range block.Files {
		result.Files = append(result.Files, CubeMapViewFile{FileRef: string(file.FileRef), Path: file.Path})
	}
	for _, symbol := range block.Symbols {
		result.Symbols = append(result.Symbols, CubeMapViewCoreSymbol{
			Symbol: CubeMapViewSymbol{
				NodeID: symbol.NodeID, Package: symbol.Package, Name: symbol.Symbol.Name,
				Location: cubeMapLocationFromSurface(symbol.Declaration),
			},
			Exported: symbol.Exported, IncomingCalls: symbol.IncomingCalls, OutgoingCalls: symbol.OutgoingCalls,
		})
	}
	for _, child := range block.Children {
		result.Children = append(result.Children, projectCubeMapCoreBlock(child))
	}
	return result
}

func projectCubeMapCallable(value gocoreobject.CallableDeclaration) CubeMapViewCoreObject {
	return CubeMapViewCoreObject{
		ID: value.ID, Category: CubeMapViewCoreCallable, DeclarationKind: string(value.Kind),
		Package: value.Package, Name: value.Name, Receiver: value.Receiver, Signature: value.Signature,
		Exported: value.Exported, Location: cubeMapLocationFromCoreObject(value.Location),
		DirectCallNodeID: value.DirectCallNodeID,
	}
}

func projectCubeMapReceiverType(value gocoreobject.TypeDeclaration) CubeMapViewCoreObject {
	return CubeMapViewCoreObject{
		ID: value.ID, Category: CubeMapViewCoreReceiverType, DeclarationKind: string(value.Kind),
		Package: value.Package, Name: value.Name, Exported: value.Exported,
		Location: cubeMapLocationFromCoreObject(value.Location),
	}
}

func projectCubeMapActivitySurface(value activitysurface.Surface) CubeMapViewActivitySurface {
	return CubeMapViewActivitySurface{
		ID: value.ID, RootNodeID: value.RootNodeID, Kind: value.Kind, Role: value.Role, Form: string(value.Form),
		Registration: cubeMapLocationFromEntryCall(value.Registration),
		Identity:     projectCubeMapSurfaceValue(value.Identity), Method: projectCubeMapSurfaceValue(value.Method),
		Path: projectCubeMapSurfaceValue(value.Path), Handler: projectCubeMapSurfaceValue(value.Handler),
	}
}

func projectCubeMapSurfaceValue(value *activitysurface.Value) *CubeMapViewSurfaceValue {
	if value == nil {
		return nil
	}
	return &CubeMapViewSurfaceValue{
		Kind: string(value.Kind), Text: value.Text, Location: cubeMapLocationFromEntryCall(value.Location),
	}
}

func projectCubeMapSymbol(value cubemap.Symbol) CubeMapViewSymbol {
	return CubeMapViewSymbol{
		NodeID: value.NodeID, Package: value.Package, Name: value.Name,
		Location: CubeMapViewLocation{Path: value.Path, Line: value.Line, Column: value.Column},
	}
}

func projectCubeMapDependency(value cubemap.IntegrationDependency) CubeMapViewIntegrationDependency {
	result := CubeMapViewIntegrationDependency{
		ID: value.ID, Kind: value.Kind, Name: value.Name, ModulePath: value.ModulePath,
		ModuleVersion: value.ModuleVersion, PackagePath: value.PackagePath, Importers: []CubeMapViewImporter{},
	}
	for _, importer := range value.Importers {
		result.Importers = append(result.Importers, CubeMapViewImporter{
			PackagePath: importer.PackagePath, RepositoryPath: importer.RepositoryPath,
		})
	}
	return result
}

func projectCubeMapIntegration(value cubemap.IntegrationSymbol) CubeMapViewIntegrationSymbol {
	result := CubeMapViewIntegrationSymbol{
		Symbol:        projectCubeMapSymbol(value.Symbol),
		DependencyIDs: append([]string(nil), value.DependencyIDs...),
		Operations:    []CubeMapViewIntegrationOperation{},
	}
	for _, operation := range value.Operations {
		projected := CubeMapViewIntegrationOperation{
			ExternalCallFamilyID: operation.ExternalCallFamilyID, DependencyID: operation.DependencyID,
			PackagePath: operation.PackagePath, Receiver: operation.Receiver, Name: operation.Name,
			Dispatch: operation.Dispatch, Invocation: operation.Invocation, WitnessCount: operation.WitnessCount,
			Callsites: []CubeMapViewLocation{}, CallsitesOmitted: operation.CallsitesOmitted,
		}
		for _, callsite := range operation.Callsites {
			projected.Callsites = append(projected.Callsites, CubeMapViewLocation{
				Path: callsite.Path, Line: callsite.Line, Column: callsite.Column,
			})
		}
		result.Operations = append(result.Operations, projected)
	}
	return result
}

func projectCubeMapReversePath(value cubemap.Path) CubeMapViewReversePath {
	result := CubeMapViewReversePath{
		IntegrationNodeID: value.IntegrationNodeID, EntrypointNodeID: value.EntrypointNodeID,
		Nodes: make([]CubeMapViewSymbol, len(value.Nodes)),
	}
	for position := range value.Nodes {
		result.Nodes[len(value.Nodes)-position-1] = projectCubeMapSymbol(value.Nodes[position])
	}
	return result
}

func projectCubeMapSurfaceCoreEffects(value cubemap.SurfaceCoreEffectBindings) *CubeMapViewSurfaceCoreEffects {
	result := &CubeMapViewSurfaceCoreEffects{
		AuthoritySHA256: value.AuthoritySHA256,
		SurfaceCore:     []CubeMapViewSurfaceCoreBinding{}, EffectCore: []CubeMapViewEffectCoreBinding{},
		Coverage: value.Coverage,
	}
	for _, binding := range value.SurfaceCore {
		result.SurfaceCore = append(result.SurfaceCore, CubeMapViewSurfaceCoreBinding{
			SurfaceID: binding.SurfaceID, CoreBlockID: binding.CoreBlockID,
			Relation: binding.Relation, MinHops: cloneCubeMapViewInt(binding.MinHops),
		})
	}
	for _, binding := range value.EffectCore {
		result.EffectCore = append(result.EffectCore, CubeMapViewEffectCoreBinding{
			ExternalCallFamilyID: binding.ExternalCallFamilyID, CallerNodeID: binding.CallerNodeID,
			CoreBlockID: binding.CoreBlockID, Relation: binding.Relation,
			MinHops: cloneCubeMapViewInt(binding.MinHops),
		})
	}
	return result
}

func cubeMapLocationFromSurface(value surfacediscovery.Location) CubeMapViewLocation {
	return CubeMapViewLocation{Path: value.Path, Line: value.Line, Column: value.Column}
}

func cubeMapLocationFromCoreObject(value gocoreobject.Location) CubeMapViewLocation {
	return CubeMapViewLocation{Path: value.Path, Line: value.Line, Column: value.Column}
}

func cubeMapLocationFromEntryCall(value entrycall.Location) CubeMapViewLocation {
	return CubeMapViewLocation{Path: value.Path, Line: value.Line, Column: value.Column}
}

func cloneCubeMapViewInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func countCubeMapViewSource(value cubemap.Map) cubeMapViewCounts {
	counts := cubeMapViewCounts{
		baselineCore: len(value.Core.Baseline), refinedCore: len(value.Core.Refined),
		refinedGroups:      len(value.Core.RefinedGroups),
		coreObjects:        len(value.CoreObjects.Callables) + len(value.CoreObjects.ReceiverTypes),
		coreObjectBindings: len(value.CoreObjects.Bindings), activitySurfaces: len(value.ActivitySurfaces.Surfaces),
		entrypoints: len(value.Entrypoints), dependencies: len(value.IntegrationDependencies),
		integrationSymbols: len(value.IntegrationSymbols), reversePaths: len(value.Paths),
	}
	var visit func([]coremap.Block)
	visit = func(blocks []coremap.Block) {
		for _, block := range blocks {
			counts.coreFiles += len(block.Files)
			counts.coreSymbols += len(block.Symbols)
			counts.baselineCore += len(block.Children)
			visit(block.Children)
		}
	}
	visit(value.Core.Baseline)
	for _, block := range value.Core.Refined {
		counts.coreFiles += len(block.Files)
		counts.coreSymbols += len(block.Symbols)
	}
	for _, dependency := range value.IntegrationDependencies {
		counts.dependencyImporters += len(dependency.Importers)
	}
	for _, integration := range value.IntegrationSymbols {
		counts.integrationOperations += len(integration.Operations)
		for _, operation := range integration.Operations {
			counts.integrationCallsites += len(operation.Callsites)
		}
	}
	for _, path := range value.Paths {
		counts.reversePathNodes += len(path.Nodes)
	}
	if value.SurfaceCoreEffects != nil {
		counts.surfaceCoreBindings = len(value.SurfaceCoreEffects.SurfaceCore)
		counts.effectCoreBindings = len(value.SurfaceCoreEffects.EffectCore)
	}
	return counts
}

func validateCubeMapViewLimits(counts cubeMapViewCounts) error {
	limits := []struct {
		name  string
		value int
		limit int
	}{
		{"core objects", counts.coreObjects, MaxCubeMapViewCoreObjects},
		{"core object bindings", counts.coreObjectBindings, MaxCubeMapViewCoreObjectBindings},
		{"activity surfaces", counts.activitySurfaces, MaxCubeMapViewActivitySurfaces},
		{"entrypoints", counts.entrypoints, MaxCubeMapViewEntrypoints},
		{"integration dependencies", counts.dependencies, MaxCubeMapViewDependencies},
		{"dependency importers", counts.dependencyImporters, MaxCubeMapViewDependencyImporters},
		{"integration symbols", counts.integrationSymbols, MaxCubeMapViewIntegrationSymbols},
		{"integration operations", counts.integrationOperations, MaxCubeMapViewIntegrationOperations},
		{"integration callsites", counts.integrationCallsites, MaxCubeMapViewIntegrationCallsites},
		{"reverse paths", counts.reversePaths, MaxCubeMapViewReversePaths},
		{"reverse path nodes", counts.reversePathNodes, MaxCubeMapViewReversePathNodes},
		{"surface/core bindings", counts.surfaceCoreBindings, MaxCubeMapViewSurfaceCoreBindings},
		{"effect/core bindings", counts.effectCoreBindings, MaxCubeMapViewEffectCoreBindings},
	}
	for _, item := range limits {
		if item.value > item.limit {
			return fmt.Errorf("cube map view: %d %s exceed projection limit %d", item.value, item.name, item.limit)
		}
	}
	return nil
}

func cubeMapViewProjectionCoverage(counts cubeMapViewCounts) CubeMapViewProjectionCoverage {
	complete := func(count int) CubeMapViewCollectionCoverage {
		return CubeMapViewCollectionCoverage{Eligible: count, Shown: count, Omitted: 0}
	}
	return CubeMapViewProjectionCoverage{
		BaselineCore: complete(counts.baselineCore), RefinedCore: complete(counts.refinedCore),
		RefinedGroups: complete(counts.refinedGroups),
		CoreFiles:     complete(counts.coreFiles), CoreSymbols: complete(counts.coreSymbols),
		CoreObjects: complete(counts.coreObjects), CoreObjectBindings: complete(counts.coreObjectBindings),
		ActivitySurfaces: complete(counts.activitySurfaces), Entrypoints: complete(counts.entrypoints),
		Dependencies: complete(counts.dependencies), DependencyImporters: complete(counts.dependencyImporters),
		IntegrationSymbols:    complete(counts.integrationSymbols),
		IntegrationOperations: complete(counts.integrationOperations),
		IntegrationCallsites:  complete(counts.integrationCallsites), ReversePaths: complete(counts.reversePaths),
		ReversePathNodes: complete(counts.reversePathNodes), SurfaceCoreBindings: complete(counts.surfaceCoreBindings),
		EffectCoreBindings: complete(counts.effectCoreBindings),
	}
}

// Validate checks the closed browser handoff without consulting source files,
// a language tool, or a model. All cross-collection relationships resolve by
// exact IDs already present in the projection.
func (view CubeMapView) Validate() error {
	if view.Version != CubeMapViewVersion {
		return fmt.Errorf("cube map view: unsupported version %d", view.Version)
	}
	if !validCubeMapViewText(view.ProgramTargetID, false) {
		return fmt.Errorf("cube map view: invalid program target identity")
	}
	if err := view.Target.Validate(); err != nil {
		return fmt.Errorf("cube map view: target: %w", err)
	}
	for _, identity := range []string{
		view.SourceIndexSHA256, view.ExternalCallIndexSHA256,
		view.DependencyCatalogSHA256, view.CoreObjectIndexSHA256,
		view.CoreObjectProjectionSHA256, view.ActivitySubstrateSHA256,
	} {
		if !validCubeMapViewSHA256(identity) {
			return fmt.Errorf("cube map view: invalid source identity")
		}
	}
	if view.BaselineCore == nil || view.RefinedCore == nil || view.RefinedGroups == nil || view.CoreObjects == nil ||
		view.CoreObjectBindings == nil || view.ActivitySurfaces == nil || view.Entrypoints == nil ||
		view.IntegrationDependencies == nil || view.IntegrationSymbols == nil || view.ReversePaths == nil {
		return fmt.Errorf("cube map view: missing collection")
	}

	baselineIDs := make(map[string]struct{})
	if err := validateCubeMapCoreBlocks(view.BaselineCore, true, 0, baselineIDs); err != nil {
		return err
	}
	refinedIDs := make(map[string]struct{})
	if len(view.RefinedCore) == 0 {
		return fmt.Errorf("cube map view: refined core is empty")
	}
	if err := validateCubeMapCoreBlocks(view.RefinedCore, false, 0, refinedIDs); err != nil {
		return err
	}
	if err := validateCoreMapViewGroups(view.RefinedGroups, refinedIDs); err != nil {
		return err
	}
	if err := validateCubeMapCoreFilePairs(view.BaselineCore, view.RefinedCore); err != nil {
		return err
	}

	objectsByID, err := validateCubeMapCoreObjects(view.CoreObjects)
	if err != nil {
		return err
	}
	if err := validateCubeMapCoreObjectBindings(view.CoreObjectBindings, objectsByID, view.RefinedCore, refinedIDs); err != nil {
		return err
	}

	surfacesByID, err := validateCubeMapActivitySurfaces(view.ActivitySurfaces)
	if err != nil {
		return err
	}
	entrypointsByID, err := validateCubeMapSymbols("entrypoint", view.Entrypoints, true)
	if err != nil {
		return err
	}
	dependenciesByID, err := validateCubeMapDependencies(view.IntegrationDependencies)
	if err != nil {
		return err
	}
	integrationsByID, effects, err := validateCubeMapIntegrations(view.IntegrationSymbols, dependenciesByID)
	if err != nil {
		return err
	}
	if err := validateCubeMapReversePaths(view.ReversePaths, entrypointsByID, integrationsByID); err != nil {
		return err
	}
	if err := validateCubeMapSymbolAuthority(view); err != nil {
		return err
	}
	if err := validateCubeMapSurfaceCoreEffects(view.SurfaceCoreEffects, refinedIDs, surfacesByID, effects); err != nil {
		return err
	}
	if err := validateCubeMapViewCoverage(view); err != nil {
		return err
	}

	encoded, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("cube map view: encode bound check: %w", err)
	}
	if len(encoded) > MaxCubeMapViewJSONBytes {
		return fmt.Errorf("cube map view: JSON size %d exceeds projection limit %d", len(encoded), MaxCubeMapViewJSONBytes)
	}
	return nil
}

func validateCubeMapCoreFilePairs(groups ...[]CubeMapViewCoreBlock) error {
	pathByRef := make(map[string]string)
	refByPath := make(map[string]string)
	var visit func([]CubeMapViewCoreBlock) error
	visit = func(blocks []CubeMapViewCoreBlock) error {
		for _, block := range blocks {
			for _, file := range block.Files {
				if path, exists := pathByRef[file.FileRef]; exists && path != file.Path {
					return fmt.Errorf("cube map view: core file ref maps to conflicting paths")
				}
				if ref, exists := refByPath[file.Path]; exists && ref != file.FileRef {
					return fmt.Errorf("cube map view: core file path maps to conflicting refs")
				}
				pathByRef[file.FileRef] = file.Path
				refByPath[file.Path] = file.FileRef
			}
			if err := visit(block.Children); err != nil {
				return err
			}
		}
		return nil
	}
	for _, blocks := range groups {
		if err := visit(blocks); err != nil {
			return err
		}
	}
	return nil
}

func validateCubeMapCoreBlocks(
	blocks []CubeMapViewCoreBlock,
	baseline bool,
	depth int,
	seen map[string]struct{},
) error {
	if baseline && depth > 1 && len(blocks) != 0 {
		return fmt.Errorf("cube map view: baseline hierarchy exceeds its producer depth")
	}
	for _, block := range blocks {
		if !validCubeMapViewText(block.ID, false) || !validCubeMapViewText(block.Name, false) ||
			!validCubeMapViewText(block.Purpose, false) || block.Files == nil || block.Symbols == nil || block.Children == nil {
			return fmt.Errorf("cube map view: invalid core block")
		}
		if _, duplicate := seen[block.ID]; duplicate {
			return fmt.Errorf("cube map view: duplicate core block %q", block.ID)
		}
		seen[block.ID] = struct{}{}
		if baseline && len(block.Symbols) != 0 || !baseline && len(block.Children) != 0 {
			return fmt.Errorf("cube map view: invalid core stage shape")
		}
		if len(block.Files)+len(block.Symbols)+len(block.Children) == 0 {
			return fmt.Errorf("cube map view: core block %q has no exact evidence", block.ID)
		}
		seenFiles := make(map[string]struct{}, len(block.Files))
		for _, file := range block.Files {
			if !validCubeMapViewText(file.FileRef, false) || !validCubeMapViewPath(file.Path) {
				return fmt.Errorf("cube map view: invalid core file")
			}
			key := file.FileRef + "\x00" + file.Path
			if _, duplicate := seenFiles[key]; duplicate {
				return fmt.Errorf("cube map view: duplicate core file")
			}
			seenFiles[key] = struct{}{}
		}
		seenSymbols := make(map[string]struct{}, len(block.Symbols))
		for _, symbol := range block.Symbols {
			if err := validateCubeMapSymbol(symbol.Symbol); err != nil || symbol.IncomingCalls < 0 || symbol.OutgoingCalls < 0 {
				return fmt.Errorf("cube map view: invalid core representative")
			}
			if _, duplicate := seenSymbols[symbol.Symbol.NodeID]; duplicate {
				return fmt.Errorf("cube map view: duplicate core representative")
			}
			seenSymbols[symbol.Symbol.NodeID] = struct{}{}
		}
		if err := validateCubeMapCoreBlocks(block.Children, baseline, depth+1, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateCubeMapCoreObjects(values []CubeMapViewCoreObject) (map[string]CubeMapViewCoreObject, error) {
	objects := make(map[string]CubeMapViewCoreObject, len(values))
	previous := ""
	for _, object := range values {
		if !validCubeMapViewText(object.ID, false) || !validCubeMapViewText(object.Package, false) ||
			!validCubeMapViewText(object.Name, false) || !validCubeMapViewLocation(object.Location, true) {
			return nil, fmt.Errorf("cube map view: invalid core object")
		}
		if previous != "" && previous >= object.ID {
			return nil, fmt.Errorf("cube map view: core objects are not canonical")
		}
		previous = object.ID
		if _, duplicate := objects[object.ID]; duplicate {
			return nil, fmt.Errorf("cube map view: duplicate core object %q", object.ID)
		}
		switch object.Category {
		case CubeMapViewCoreCallable:
			kind := gocoreobject.CallableKind(object.DeclarationKind)
			if !kind.Valid() || !validCubeMapViewText(object.Signature, false) ||
				!validCubeMapViewText(object.DirectCallNodeID, false) ||
				(kind == gocoreobject.CallableFunction && object.Receiver != "") ||
				(kind == gocoreobject.CallableMethod && !validCubeMapViewText(object.Receiver, false)) {
				return nil, fmt.Errorf("cube map view: invalid callable core object")
			}
		case CubeMapViewCoreReceiverType:
			if !gocoreobject.TypeKind(object.DeclarationKind).Valid() || object.Receiver != "" ||
				object.Signature != "" || object.DirectCallNodeID != "" {
				return nil, fmt.Errorf("cube map view: invalid receiver-type core object")
			}
		default:
			return nil, fmt.Errorf("cube map view: invalid core object category %q", object.Category)
		}
		objects[object.ID] = object
	}
	return objects, nil
}

func validateCubeMapCoreObjectBindings(
	bindings []CubeMapViewCoreObjectBinding,
	objects map[string]CubeMapViewCoreObject,
	blocks []CubeMapViewCoreBlock,
	blockIDs map[string]struct{},
) error {
	representatives := make(map[string]map[string]CubeMapViewSymbol, len(blocks))
	for _, block := range blocks {
		nodes := make(map[string]CubeMapViewSymbol, len(block.Symbols))
		for _, symbol := range block.Symbols {
			nodes[symbol.Symbol.NodeID] = symbol.Symbol
		}
		representatives[block.ID] = nodes
	}
	seenObjects := make(map[string]struct{}, len(objects))
	previous := ""
	for _, binding := range bindings {
		key := binding.CoreBlockID + "\x00" + string(binding.Role) + "\x00" + binding.ObjectID
		if previous != "" && previous >= key {
			return fmt.Errorf("cube map view: core object bindings are not canonical")
		}
		previous = key
		if _, exists := blockIDs[binding.CoreBlockID]; !exists {
			return fmt.Errorf("cube map view: core object binding has unknown block")
		}
		object, exists := objects[binding.ObjectID]
		if !exists {
			return fmt.Errorf("cube map view: core object binding has unknown object")
		}
		switch binding.Role {
		case cubemap.CoreObjectRepresentativeCallable:
			if object.Category != CubeMapViewCoreCallable {
				return fmt.Errorf("cube map view: callable binding has the wrong object category")
			}
			representative, exact := representatives[binding.CoreBlockID][object.DirectCallNodeID]
			if !exact || representative.Package != object.Package || representative.Name != object.Name ||
				representative.Location != object.Location {
				return fmt.Errorf("cube map view: callable binding does not join an exact representative")
			}
		case cubemap.CoreObjectReceiverType:
			if object.Category != CubeMapViewCoreReceiverType {
				return fmt.Errorf("cube map view: receiver binding has the wrong object category")
			}
		default:
			return fmt.Errorf("cube map view: invalid core object binding role")
		}
		seenObjects[object.ID] = struct{}{}
	}
	if len(seenObjects) != len(objects) {
		return fmt.Errorf("cube map view: unbound core object")
	}
	return nil
}

func validateCubeMapActivitySurfaces(values []CubeMapViewActivitySurface) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	previous := ""
	for _, surface := range values {
		if !validCubeMapViewText(surface.ID, false) || !validCubeMapViewText(surface.RootNodeID, false) ||
			!validCubeMapViewText(surface.Kind, false) || !validCubeMapViewText(surface.Role, false) ||
			!entrycall.SurfaceCandidateForm(surface.Form).Valid() || !validCubeMapViewLocation(surface.Registration, false) {
			return nil, fmt.Errorf("cube map view: invalid activity surface")
		}
		for _, value := range []*CubeMapViewSurfaceValue{surface.Identity, surface.Method, surface.Path, surface.Handler} {
			if value != nil && (!entrycall.SurfaceFactKind(value.Kind).Valid() ||
				!validCubeMapViewText(value.Text, false) || !validCubeMapViewLocation(value.Location, false)) {
				return nil, fmt.Errorf("cube map view: invalid activity surface value")
			}
		}
		if surface.Handler != nil && surface.Handler.Kind != string(entrycall.SurfaceFactCallable) {
			return nil, fmt.Errorf("cube map view: activity handler is not callable")
		}
		switch surface.Kind {
		case entrycall.SurfaceKindCLICommand:
			if surface.Form != string(entrycall.SurfaceCandidateKeyedComposite) || surface.Role != entrycall.SurfaceRoleDescriptor ||
				surface.Identity == nil || surface.Identity.Kind != string(entrycall.SurfaceFactString) ||
				surface.Method != nil || surface.Path != nil {
				return nil, fmt.Errorf("cube map view: invalid CLI activity surface")
			}
		case entrycall.SurfaceKindHTTPRoute:
			if surface.Form != string(entrycall.SurfaceCandidateDirectCall) || surface.Identity != nil ||
				surface.Path == nil || surface.Path.Kind != string(entrycall.SurfaceFactString) ||
				!strings.HasPrefix(surface.Path.Text, "/") || surface.Method != nil && !validCubeMapViewHTTPMethod(*surface.Method) ||
				!validCubeMapViewHandlerRole(surface) {
				return nil, fmt.Errorf("cube map view: invalid HTTP activity surface")
			}
		case entrycall.SurfaceKindScheduledJob:
			if surface.Form != string(entrycall.SurfaceCandidateDirectCall) || surface.Identity == nil ||
				surface.Identity.Kind != string(entrycall.SurfaceFactString) || surface.Method != nil || surface.Path != nil ||
				!validCubeMapViewHandlerRole(surface) {
				return nil, fmt.Errorf("cube map view: invalid scheduled activity surface")
			}
		default:
			return nil, fmt.Errorf("cube map view: unknown activity kind %q", surface.Kind)
		}
		if _, duplicate := result[surface.ID]; duplicate {
			return nil, fmt.Errorf("cube map view: duplicate activity surface %q", surface.ID)
		}
		key := strings.Join([]string{
			surface.RootNodeID, surface.Kind, surface.Registration.Path,
			fmt.Sprintf("%010d", surface.Registration.Line), fmt.Sprintf("%010d", surface.Registration.Column), surface.ID,
		}, "\x00")
		if previous != "" && previous >= key {
			return nil, fmt.Errorf("cube map view: activity surfaces are not canonical")
		}
		previous = key
		result[surface.ID] = struct{}{}
	}
	return result, nil
}

func validCubeMapViewHandlerRole(surface CubeMapViewActivitySurface) bool {
	if surface.Handler == nil {
		return surface.Role == entrycall.SurfaceRoleDescriptor
	}
	return surface.Role == entrycall.SurfaceRoleEntrySurface
}

func validCubeMapViewHTTPMethod(value CubeMapViewSurfaceValue) bool {
	if value.Kind != string(entrycall.SurfaceFactToken) && value.Kind != string(entrycall.SurfaceFactString) {
		return false
	}
	switch strings.ToUpper(value.Text) {
	case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE":
		return true
	default:
		return false
	}
}

func validateCubeMapSymbolAuthority(view CubeMapView) error {
	seen := make(map[string]CubeMapViewSymbol)
	add := func(symbol CubeMapViewSymbol) error {
		if previous, exists := seen[symbol.NodeID]; exists && previous != symbol {
			return fmt.Errorf("cube map view: exact symbol ID has conflicting facts")
		}
		seen[symbol.NodeID] = symbol
		return nil
	}
	var visitBlocks func([]CubeMapViewCoreBlock) error
	visitBlocks = func(blocks []CubeMapViewCoreBlock) error {
		for _, block := range blocks {
			for _, symbol := range block.Symbols {
				if err := add(symbol.Symbol); err != nil {
					return err
				}
			}
			if err := visitBlocks(block.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visitBlocks(view.RefinedCore); err != nil {
		return err
	}
	for _, symbol := range view.Entrypoints {
		if err := add(symbol); err != nil {
			return err
		}
	}
	for _, integration := range view.IntegrationSymbols {
		if err := add(integration.Symbol); err != nil {
			return err
		}
	}
	for _, path := range view.ReversePaths {
		for _, symbol := range path.Nodes {
			if err := add(symbol); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCubeMapSymbols(label string, values []CubeMapViewSymbol, canonical bool) (map[string]CubeMapViewSymbol, error) {
	result := make(map[string]CubeMapViewSymbol, len(values))
	previous := ""
	for _, symbol := range values {
		if err := validateCubeMapSymbol(symbol); err != nil {
			return nil, fmt.Errorf("cube map view: invalid %s: %w", label, err)
		}
		if _, duplicate := result[symbol.NodeID]; duplicate {
			return nil, fmt.Errorf("cube map view: duplicate %s %q", label, symbol.NodeID)
		}
		key := cubeMapViewSymbolKey(symbol)
		if canonical && previous != "" && previous >= key {
			return nil, fmt.Errorf("cube map view: %ss are not canonical", label)
		}
		previous = key
		result[symbol.NodeID] = symbol
	}
	return result, nil
}

func validateCubeMapSymbol(symbol CubeMapViewSymbol) error {
	if !validCubeMapViewText(symbol.NodeID, false) || !validCubeMapViewText(symbol.Package, false) ||
		!validCubeMapViewText(symbol.Name, false) || !validCubeMapViewLocation(symbol.Location, false) {
		return fmt.Errorf("invalid exact symbol")
	}
	return nil
}

func validateCubeMapDependencies(values []CubeMapViewIntegrationDependency) (map[string]CubeMapViewIntegrationDependency, error) {
	result := make(map[string]CubeMapViewIntegrationDependency, len(values))
	previous := ""
	for _, dependency := range values {
		if !validCubeMapViewText(dependency.ID, false) ||
			(dependency.Kind != dependencies.KindExternal && dependency.Kind != dependencies.KindStdlib) ||
			!validCubeMapViewText(dependency.Name, false) || !validCubeMapViewText(dependency.PackagePath, false) ||
			dependency.Importers == nil {
			return nil, fmt.Errorf("cube map view: invalid integration dependency")
		}
		if !validCubeMapViewText(dependency.ModulePath, true) || !validCubeMapViewText(dependency.ModuleVersion, true) {
			return nil, fmt.Errorf("cube map view: invalid integration dependency metadata")
		}
		if _, duplicate := result[dependency.ID]; duplicate {
			return nil, fmt.Errorf("cube map view: duplicate integration dependency %q", dependency.ID)
		}
		key := dependency.PackagePath + "\x00" + dependency.ModulePath + "\x00" + dependency.ID
		if previous != "" && previous >= key {
			return nil, fmt.Errorf("cube map view: integration dependencies are not canonical")
		}
		previous = key
		previousImporter := ""
		for _, importer := range dependency.Importers {
			if !validCubeMapViewText(importer.PackagePath, false) || !validCubeMapViewDirectory(importer.RepositoryPath) {
				return nil, fmt.Errorf("cube map view: invalid dependency importer")
			}
			importerKey := importer.PackagePath + "\x00" + importer.RepositoryPath
			if previousImporter != "" && previousImporter >= importerKey {
				return nil, fmt.Errorf("cube map view: dependency importers are not canonical")
			}
			previousImporter = importerKey
		}
		result[dependency.ID] = dependency
	}
	return result, nil
}

func validateCubeMapIntegrations(
	values []CubeMapViewIntegrationSymbol,
	dependenciesByID map[string]CubeMapViewIntegrationDependency,
) (map[string]CubeMapViewSymbol, map[string]struct{}, error) {
	integrations := make(map[string]CubeMapViewSymbol, len(values))
	effects := make(map[string]struct{})
	previous := ""
	for _, integration := range values {
		if err := validateCubeMapSymbol(integration.Symbol); err != nil || integration.DependencyIDs == nil ||
			len(integration.DependencyIDs) == 0 || integration.Operations == nil || len(integration.Operations) == 0 {
			return nil, nil, fmt.Errorf("cube map view: invalid integration symbol")
		}
		if _, duplicate := integrations[integration.Symbol.NodeID]; duplicate {
			return nil, nil, fmt.Errorf("cube map view: duplicate integration symbol")
		}
		key := cubeMapViewSymbolKey(integration.Symbol)
		if previous != "" && previous >= key {
			return nil, nil, fmt.Errorf("cube map view: integration symbols are not canonical")
		}
		previous = key
		claimedDependencies := make(map[string]struct{}, len(integration.DependencyIDs))
		previousDependency := ""
		for _, dependencyID := range integration.DependencyIDs {
			if previousDependency != "" && previousDependency >= dependencyID {
				return nil, nil, fmt.Errorf("cube map view: integration dependency IDs are not canonical")
			}
			if _, exists := dependenciesByID[dependencyID]; !exists {
				return nil, nil, fmt.Errorf("cube map view: integration symbol has unknown dependency")
			}
			claimedDependencies[dependencyID] = struct{}{}
			previousDependency = dependencyID
		}
		operationDependencies := make(map[string]struct{}, len(claimedDependencies))
		previousOperation := ""
		for _, operation := range integration.Operations {
			dependency, exists := dependenciesByID[operation.DependencyID]
			if !exists || dependency.PackagePath != operation.PackagePath ||
				!validCubeMapViewText(operation.ExternalCallFamilyID, false) ||
				!validCubeMapViewText(operation.Name, false) || !validCubeMapViewText(operation.Receiver, true) ||
				!operation.Dispatch.Valid() || !operation.Invocation.Valid() || operation.WitnessCount < 1 ||
				operation.Callsites == nil || operation.CallsitesOmitted < 0 ||
				operation.WitnessCount != len(operation.Callsites)+operation.CallsitesOmitted {
				return nil, nil, fmt.Errorf("cube map view: invalid integration operation")
			}
			operationKey := strings.Join([]string{
				operation.DependencyID, operation.PackagePath, operation.Receiver, operation.Name,
				string(operation.Dispatch), string(operation.Invocation), operation.ExternalCallFamilyID,
			}, "\x00")
			if previousOperation != "" && previousOperation >= operationKey {
				return nil, nil, fmt.Errorf("cube map view: integration operations are not canonical")
			}
			previousOperation = operationKey
			previousCallsite := ""
			for _, callsite := range operation.Callsites {
				if !validCubeMapViewLocation(callsite, false) {
					return nil, nil, fmt.Errorf("cube map view: invalid integration callsite")
				}
				callsiteKey := cubeMapViewLocationKey(callsite)
				if previousCallsite != "" && previousCallsite >= callsiteKey {
					return nil, nil, fmt.Errorf("cube map view: integration callsites are not canonical")
				}
				previousCallsite = callsiteKey
			}
			operationDependencies[operation.DependencyID] = struct{}{}
			effectKey := integration.Symbol.NodeID + "\x00" + operation.ExternalCallFamilyID
			if _, duplicate := effects[effectKey]; duplicate {
				return nil, nil, fmt.Errorf("cube map view: duplicate exact integration effect")
			}
			effects[effectKey] = struct{}{}
		}
		if len(operationDependencies) != len(claimedDependencies) {
			return nil, nil, fmt.Errorf("cube map view: integration dependency claims do not match operations")
		}
		for dependencyID := range claimedDependencies {
			if _, exists := operationDependencies[dependencyID]; !exists {
				return nil, nil, fmt.Errorf("cube map view: integration dependency claim has no operation")
			}
		}
		integrations[integration.Symbol.NodeID] = integration.Symbol
	}
	return integrations, effects, nil
}

func validateCubeMapReversePaths(
	values []CubeMapViewReversePath,
	entrypoints map[string]CubeMapViewSymbol,
	integrations map[string]CubeMapViewSymbol,
) error {
	previous := ""
	for _, path := range values {
		if _, exists := entrypoints[path.EntrypointNodeID]; !exists {
			return fmt.Errorf("cube map view: reverse path has unknown entrypoint")
		}
		if _, exists := integrations[path.IntegrationNodeID]; !exists {
			return fmt.Errorf("cube map view: reverse path has unknown integration")
		}
		if len(path.Nodes) == 0 || path.Nodes[0].NodeID != path.IntegrationNodeID ||
			path.Nodes[len(path.Nodes)-1].NodeID != path.EntrypointNodeID {
			return fmt.Errorf("cube map view: reverse path endpoints do not match")
		}
		if path.Nodes[0] != integrations[path.IntegrationNodeID] ||
			path.Nodes[len(path.Nodes)-1] != entrypoints[path.EntrypointNodeID] {
			return fmt.Errorf("cube map view: reverse path endpoint facts do not match exact symbols")
		}
		if previous != "" && previous >= path.IntegrationNodeID {
			return fmt.Errorf("cube map view: reverse paths are not canonical")
		}
		previous = path.IntegrationNodeID
		seen := make(map[string]struct{}, len(path.Nodes))
		for _, symbol := range path.Nodes {
			if err := validateCubeMapSymbol(symbol); err != nil {
				return fmt.Errorf("cube map view: invalid reverse path node")
			}
			if _, duplicate := seen[symbol.NodeID]; duplicate {
				return fmt.Errorf("cube map view: reverse path contains a cycle")
			}
			seen[symbol.NodeID] = struct{}{}
		}
	}
	return nil
}

func validateCubeMapSurfaceCoreEffects(
	value *CubeMapViewSurfaceCoreEffects,
	coreBlocks map[string]struct{},
	surfaces map[string]struct{},
	effects map[string]struct{},
) error {
	if value == nil {
		return nil
	}
	if !validCubeMapViewSHA256(value.AuthoritySHA256) || value.SurfaceCore == nil || value.EffectCore == nil {
		return fmt.Errorf("cube map view: invalid surface/core/effect identity")
	}
	previous := ""
	for _, binding := range value.SurfaceCore {
		if _, exists := surfaces[binding.SurfaceID]; !exists {
			return fmt.Errorf("cube map view: surface/core binding has unknown surface")
		}
		if _, exists := coreBlocks[binding.CoreBlockID]; !exists || !validCubeMapViewAnchorDistance(binding.Relation, binding.MinHops) {
			return fmt.Errorf("cube map view: invalid surface/core binding")
		}
		key := binding.SurfaceID + "\x00" + binding.CoreBlockID
		if previous != "" && previous >= key {
			return fmt.Errorf("cube map view: surface/core bindings are not canonical")
		}
		previous = key
	}
	previous = ""
	for _, binding := range value.EffectCore {
		if _, exists := effects[binding.CallerNodeID+"\x00"+binding.ExternalCallFamilyID]; !exists {
			return fmt.Errorf("cube map view: effect/core binding has unknown effect")
		}
		if _, exists := coreBlocks[binding.CoreBlockID]; !exists || !validCubeMapViewAnchorDistance(binding.Relation, binding.MinHops) {
			return fmt.Errorf("cube map view: invalid effect/core binding")
		}
		key := strings.Join([]string{binding.CallerNodeID, binding.ExternalCallFamilyID, binding.CoreBlockID}, "\x00")
		if previous != "" && previous >= key {
			return fmt.Errorf("cube map view: effect/core bindings are not canonical")
		}
		previous = key
	}
	coverage := value.Coverage
	if coverage.Surfaces != len(surfaces) || coverage.CoreBlocks != len(coreBlocks) || coverage.Effects != len(effects) ||
		coverage.SurfaceCorePairs != coverage.Surfaces*coverage.CoreBlocks ||
		coverage.EffectCorePairs != coverage.Effects*coverage.CoreBlocks ||
		coverage.SelectedSurfaceCore != len(value.SurfaceCore) || coverage.SelectedEffectCore != len(value.EffectCore) ||
		coverage.ModelCalled != (coverage.CoreBlocks > 0 && coverage.Surfaces+coverage.Effects > 0) {
		return fmt.Errorf("cube map view: invalid surface/core/effect coverage")
	}
	return nil
}

func validateCubeMapViewCoverage(view CubeMapView) error {
	counts := countCubeMapView(view)
	if err := validateCubeMapViewLimits(counts); err != nil {
		return err
	}
	wantProjection := cubeMapViewProjectionCoverage(counts)
	if view.Coverage.Projection != wantProjection {
		return fmt.Errorf("cube map view: projection coverage mismatch")
	}
	core := view.Coverage.Core
	if core.TrackedFiles < 0 || core.BaselineRoleFiles < 0 || core.SymbolsAvailable < 0 ||
		core.BaselineBlocks != counts.baselineCore || core.RefinedBlocks != counts.refinedCore ||
		core.RefinedGroups != counts.refinedGroups ||
		core.RefinedGroupCalls != cubeMapViewBoolCount(counts.refinedCore >= 2) ||
		core.BaselineFilesSelected != countCubeMapUniqueFiles(view.BaselineCore) ||
		core.RefinedFilesSelected != countCubeMapUniqueFiles(view.RefinedCore) ||
		core.RefinedSymbolsSelected != countCubeMapUniqueCoreSymbols(view.RefinedCore) ||
		core.SymbolsAvailable < core.RefinedSymbolsSelected || !core.DirectCallState.Valid() {
		return fmt.Errorf("cube map view: core coverage mismatch")
	}
	objectCoverage := view.Coverage.CoreObjects
	callables := countCubeMapCoreObjectCategory(view.CoreObjects, CubeMapViewCoreCallable)
	receiverTypes := countCubeMapCoreObjectCategory(view.CoreObjects, CubeMapViewCoreReceiverType)
	callableBindings := countCubeMapCoreObjectBindingRole(view.CoreObjectBindings, cubemap.CoreObjectRepresentativeCallable)
	receiverBindings := countCubeMapCoreObjectBindingRole(view.CoreObjectBindings, cubemap.CoreObjectReceiverType)
	if objectCoverage.CoreBlocksObserved != counts.refinedCore ||
		objectCoverage.RepresentativeCallablesMatched != callables || objectCoverage.ReceiverTypesMatched != receiverTypes ||
		objectCoverage.CallableBindings != callableBindings || objectCoverage.ReceiverTypeBindings != receiverBindings ||
		objectCoverage.RepresentativeNodesObserved != objectCoverage.RepresentativeCallablesMatched+objectCoverage.RepresentativeNodesUnmatched ||
		objectCoverage.ReceiverMethodsObserved < objectCoverage.ReceiverMethodsOmitted ||
		objectCoverage.GenericReceiverMethodsOmitted > objectCoverage.ReceiverMethodsOmitted {
		return fmt.Errorf("cube map view: core object coverage mismatch")
	}
	if view.Coverage.ActivitySurfaces.Selected != len(view.ActivitySurfaces) {
		return fmt.Errorf("cube map view: activity surface coverage mismatch")
	}
	if err := validateCubeMapActivityCoverage(view.Coverage.ActivitySurfaces); err != nil {
		return err
	}
	switch view.Coverage.ActivityState {
	case entrycall.StateReady:
		if view.Coverage.ActivityClosedReason != "" {
			return fmt.Errorf("cube map view: ready activity surface coverage has a closed reason")
		}
	case entrycall.StateUnavailable:
		if !validCubeMapViewActivityClosedReason(view.Coverage.ActivityClosedReason) || len(view.ActivitySurfaces) != 0 ||
			view.Coverage.ActivitySurfaces != (activitysurface.Coverage{}) {
			return fmt.Errorf("cube map view: invalid unavailable activity surface coverage")
		}
	default:
		return fmt.Errorf("cube map view: invalid activity surface state")
	}
	if err := validateCubeMapCandidateCoverage(view.Coverage.Cube.Entrypoints); err != nil {
		return err
	}
	if err := validateCubeMapDependencyCoverage(view.Coverage.Cube.DependencyCatalog); err != nil {
		return err
	}
	if err := validateCubeMapCandidateCoverage(view.Coverage.Cube.IntegrationDependencies); err != nil {
		return err
	}
	if err := validateCubeMapCandidateCoverage(view.Coverage.Cube.IntegrationSymbols); err != nil {
		return err
	}
	if view.Coverage.Cube.UnconnectedSymbols != len(view.IntegrationSymbols)-len(view.ReversePaths) ||
		view.Coverage.Cube.GoFilesObserved < 0 || view.Coverage.Cube.GoFilesParsed < 0 ||
		view.Coverage.Cube.GoFilesSkipped < 0 ||
		view.Coverage.Cube.GoFilesParsed+view.Coverage.Cube.GoFilesSkipped != view.Coverage.Cube.GoFilesObserved ||
		view.Coverage.Cube.ExternalCallFamiliesObserved < 0 ||
		view.Coverage.Cube.ExternalCallFamiliesMatched < 0 ||
		view.Coverage.Cube.ExternalCallFamiliesMatched > view.Coverage.Cube.ExternalCallFamiliesObserved {
		return fmt.Errorf("cube map view: cube coverage mismatch")
	}
	if err := validateCubeMapExternalCallCoverage(view.Coverage.Cube); err != nil {
		return err
	}
	if (view.SurfaceCoreEffects == nil) != (view.Coverage.SurfaceCoreEffects == nil) {
		return fmt.Errorf("cube map view: surface/core/effect coverage presence mismatch")
	}
	if view.SurfaceCoreEffects != nil && *view.Coverage.SurfaceCoreEffects != view.SurfaceCoreEffects.Coverage {
		return fmt.Errorf("cube map view: surface/core/effect coverage mismatch")
	}
	return nil
}

func validateCubeMapDependencyCoverage(coverage cubemap.DependencyCatalogCoverage) error {
	if coverage.ImportsObserved < 0 || coverage.ImportsRetained < 0 || coverage.Omissions < 0 ||
		coverage.ImportsObserved != coverage.ImportsRetained+coverage.Omissions {
		return fmt.Errorf("cube map view: invalid dependency catalog coverage")
	}
	sum := 0
	previous := dependencies.OmissionReason("")
	for _, reason := range coverage.Reasons {
		if reason.Count < 1 || (previous != "" && previous >= reason.Reason) {
			return fmt.Errorf("cube map view: invalid dependency omission coverage")
		}
		previous = reason.Reason
		sum += reason.Count
	}
	if sum != coverage.Omissions {
		return fmt.Errorf("cube map view: dependency omission coverage mismatch")
	}
	switch coverage.State {
	case dependencies.CoverageComplete:
		if coverage.Omissions != 0 || len(coverage.Reasons) != 0 {
			return fmt.Errorf("cube map view: complete dependency coverage has omissions")
		}
	default:
		return fmt.Errorf("cube map view: dependency coverage is not complete")
	}
	return nil
}

func validateCubeMapActivityCoverage(coverage activitysurface.Coverage) error {
	values := []int{
		coverage.Candidates.ConsideredCandidates, coverage.Candidates.AdvertisedCandidates,
		coverage.Candidates.OmittedCandidates, coverage.Candidates.ConsideredFacts,
		coverage.Candidates.AdvertisedFacts, coverage.Candidates.OmittedFacts,
		coverage.Candidates.UnsafeFactsExcluded, coverage.Candidates.UnreachableCandidatesExcluded,
		coverage.Selected, coverage.Rejected,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("cube map view: negative activity surface coverage")
		}
	}
	if coverage.Candidates.AdvertisedCandidates > entrycall.MaxSurfaceCandidates ||
		coverage.Candidates.AdvertisedCandidates+coverage.Candidates.OmittedCandidates != coverage.Candidates.ConsideredCandidates ||
		coverage.Candidates.AdvertisedFacts > entrycall.MaxSurfaceFacts ||
		coverage.Candidates.AdvertisedFacts+coverage.Candidates.OmittedFacts != coverage.Candidates.ConsideredFacts ||
		coverage.Selected+coverage.Rejected > coverage.Candidates.AdvertisedCandidates ||
		coverage.ModelCalled != (coverage.Candidates.AdvertisedCandidates > 0) {
		return fmt.Errorf("cube map view: invalid activity surface coverage")
	}
	return nil
}

func validCubeMapViewActivityClosedReason(reason entrycall.ClosedReason) bool {
	return reason == entrycall.ClosedNoEntrypoints
}

func validateCubeMapExternalCallCoverage(coverage cubemap.Coverage) error {
	values := []int{
		coverage.ExternalCalls.PackagesIndexed, coverage.ExternalCalls.CallersIndexed,
		coverage.ExternalCalls.FamiliesIndexed, coverage.ExternalCalls.ExternalStaticWitnesses,
		coverage.ExternalCalls.ExternalInterfaceInvokeWitnesses,
		coverage.ExternalCalls.RepresentativeCallsites, coverage.ExternalCalls.RepresentativeCallsitesOmitted,
		coverage.ExternalCalls.DynamicInvokesExcluded, coverage.ExternalCalls.NonStaticCallsExcluded,
		coverage.ExternalCalls.UnnamedStaticCalleesExcluded, coverage.ExternalCalls.InvalidCallsitesExcluded,
		coverage.ExternalCalls.SyntheticCallerWitnessesExcluded, coverage.ExternalCalls.InvalidCallerWitnessesExcluded,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("cube map view: negative external-call coverage")
		}
	}
	if coverage.ExternalCalls.FamiliesIndexed != coverage.ExternalCallFamiliesObserved {
		return fmt.Errorf("cube map view: external-call family coverage mismatch")
	}
	return nil
}

func countCubeMapView(view CubeMapView) cubeMapViewCounts {
	counts := cubeMapViewCounts{
		baselineCore: countCubeMapBlocks(view.BaselineCore), refinedCore: countCubeMapBlocks(view.RefinedCore),
		refinedGroups: len(view.RefinedGroups),
		coreObjects:   len(view.CoreObjects), coreObjectBindings: len(view.CoreObjectBindings),
		activitySurfaces: len(view.ActivitySurfaces), entrypoints: len(view.Entrypoints),
		dependencies: len(view.IntegrationDependencies), integrationSymbols: len(view.IntegrationSymbols),
		reversePaths: len(view.ReversePaths),
	}
	for _, block := range append(append([]CubeMapViewCoreBlock{}, view.BaselineCore...), view.RefinedCore...) {
		countCubeMapBlockEvidence(block, &counts)
	}
	for _, dependency := range view.IntegrationDependencies {
		counts.dependencyImporters += len(dependency.Importers)
	}
	for _, integration := range view.IntegrationSymbols {
		counts.integrationOperations += len(integration.Operations)
		for _, operation := range integration.Operations {
			counts.integrationCallsites += len(operation.Callsites)
		}
	}
	for _, path := range view.ReversePaths {
		counts.reversePathNodes += len(path.Nodes)
	}
	if view.SurfaceCoreEffects != nil {
		counts.surfaceCoreBindings = len(view.SurfaceCoreEffects.SurfaceCore)
		counts.effectCoreBindings = len(view.SurfaceCoreEffects.EffectCore)
	}
	return counts
}

func cubeMapViewBoolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func countCubeMapBlocks(blocks []CubeMapViewCoreBlock) int {
	result := len(blocks)
	for _, block := range blocks {
		result += countCubeMapBlocks(block.Children)
	}
	return result
}

func countCubeMapBlockEvidence(block CubeMapViewCoreBlock, counts *cubeMapViewCounts) {
	counts.coreFiles += len(block.Files)
	counts.coreSymbols += len(block.Symbols)
	for _, child := range block.Children {
		countCubeMapBlockEvidence(child, counts)
	}
}

func countCubeMapUniqueFiles(blocks []CubeMapViewCoreBlock) int {
	seen := make(map[string]struct{})
	var visit func([]CubeMapViewCoreBlock)
	visit = func(values []CubeMapViewCoreBlock) {
		for _, block := range values {
			for _, file := range block.Files {
				seen[file.FileRef+"\x00"+file.Path] = struct{}{}
			}
			visit(block.Children)
		}
	}
	visit(blocks)
	return len(seen)
}

func countCubeMapUniqueCoreSymbols(blocks []CubeMapViewCoreBlock) int {
	seen := make(map[string]struct{})
	for _, block := range blocks {
		for _, symbol := range block.Symbols {
			seen[symbol.Symbol.NodeID] = struct{}{}
		}
	}
	return len(seen)
}

func countCubeMapCoreObjectCategory(values []CubeMapViewCoreObject, category CubeMapViewCoreObjectCategory) int {
	count := 0
	for _, value := range values {
		if value.Category == category {
			count++
		}
	}
	return count
}

func countCubeMapCoreObjectBindingRole(values []CubeMapViewCoreObjectBinding, role cubemap.CoreObjectBindingRole) int {
	count := 0
	for _, value := range values {
		if value.Role == role {
			count++
		}
	}
	return count
}

func validateCubeMapCandidateCoverage(value cubemap.CandidateCoverage) error {
	if value.Observed < 0 || value.Advertised < 0 || value.Omitted < 0 ||
		value.Advertised > value.Observed || value.Omitted != value.Observed-value.Advertised ||
		value.Omitted != 0 || value.ModelCalled && value.Advertised == 0 {
		return fmt.Errorf("cube map view: invalid candidate coverage")
	}
	return nil
}

func validCubeMapViewAnchorDistance(relation cubemap.AnchorCoreRelation, distance *int) bool {
	switch relation {
	case cubemap.AnchorCoreSameSymbol:
		return distance != nil && *distance == 0
	case cubemap.AnchorReachesCore, cubemap.CoreReachesAnchor:
		return distance != nil && *distance > 0
	case cubemap.AnchorCoreUnconnected:
		return distance == nil
	default:
		return false
	}
}

func validCubeMapViewSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validCubeMapViewText(value string, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	if value != strings.TrimSpace(value) || len(value) > maxCubeMapViewTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCubeMapViewPath(value string) bool {
	return value != "." && fs.ValidPath(value) && !strings.Contains(value, "\\")
}

func validCubeMapViewDirectory(value string) bool {
	return value == "." || validCubeMapViewPath(value)
}

func validCubeMapViewLocation(value CubeMapViewLocation, requireColumn bool) bool {
	if !validCubeMapViewPath(value.Path) || value.Line < 1 || value.Column < 0 {
		return false
	}
	return !requireColumn || value.Column > 0
}

func cubeMapViewSymbolKey(value CubeMapViewSymbol) string {
	return strings.Join([]string{
		value.Location.Path, fmt.Sprintf("%010d", value.Location.Line), value.Package, value.Name, value.NodeID,
	}, "\x00")
}

func cubeMapViewLocationKey(value CubeMapViewLocation) string {
	return strings.Join([]string{
		value.Path, fmt.Sprintf("%010d", value.Line), fmt.Sprintf("%010d", value.Column),
	}, "\x00")
}
