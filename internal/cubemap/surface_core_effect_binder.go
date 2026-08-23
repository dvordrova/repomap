package cubemap

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/activitysurface"
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	SurfaceCoreEffectBindingsVersion = 1

	maxSurfaceCoreEffectBindings = 256
)

//go:embed prompts/surface-core-effects.md
var surfaceCoreEffectsPrompt string

// AnchorCoreRelation is a local, exact topology fact between one accepted
// surface/effect anchor and the representative symbols of one refined core
// block. Unconnected means only that the bounded exact index establishes no
// path; a core represented only by non-graph ProgramIndex declarations also
// has no graph path to establish. It is never proof that a runtime
// relationship is absent.
type AnchorCoreRelation string

const (
	AnchorCoreSameSymbol  AnchorCoreRelation = "same_symbol"
	AnchorReachesCore     AnchorCoreRelation = "anchor_reaches_core"
	CoreReachesAnchor     AnchorCoreRelation = "core_reaches_anchor"
	AnchorCoreUnconnected AnchorCoreRelation = "unconnected"
)

func (relation AnchorCoreRelation) valid() bool {
	switch relation {
	case AnchorCoreSameSymbol, AnchorReachesCore, CoreReachesAnchor, AnchorCoreUnconnected:
		return true
	default:
		return false
	}
}

// SurfaceCoreBinding is one model-selected semantic association restored to
// the stable IDs owned by the activity-surface and refined-core cubes.
type SurfaceCoreBinding struct {
	SurfaceID   string             `json:"surface_id"`
	CoreBlockID string             `json:"core_block_id"`
	Relation    AnchorCoreRelation `json:"relation"`
	MinHops     *int               `json:"min_hops,omitempty"`
}

// EffectCoreBinding restores one selected external operation and its exact
// repository caller to one refined core responsibility.
type EffectCoreBinding struct {
	ExternalCallFamilyID string             `json:"external_call_family_id"`
	CallerNodeID         string             `json:"caller_node_id"`
	CoreBlockID          string             `json:"core_block_id"`
	Relation             AnchorCoreRelation `json:"relation"`
	MinHops              *int               `json:"min_hops,omitempty"`
}

type SurfaceCoreEffectCoverage struct {
	Surfaces            int  `json:"surfaces"`
	CoreBlocks          int  `json:"core_blocks"`
	Effects             int  `json:"effects"`
	SurfaceCorePairs    int  `json:"surface_core_pairs"`
	EffectCorePairs     int  `json:"effect_core_pairs"`
	SelectedSurfaceCore int  `json:"selected_surface_core"`
	SelectedEffectCore  int  `json:"selected_effect_core"`
	ModelCalled         bool `json:"model_called"`
}

// SurfaceCoreEffectBindings is the locally restored output of the binder.
// AuthoritySHA256 binds its complete request-local ref authority without
// persisting any request-local refs.
type SurfaceCoreEffectBindings struct {
	Version          int                       `json:"version"`
	TargetRef        string                    `json:"target_ref"`
	DirectCallSHA256 string                    `json:"direct_call_sha256"`
	AuthoritySHA256  string                    `json:"authority_sha256"`
	SurfaceCore      []SurfaceCoreBinding      `json:"surface_core"`
	EffectCore       []EffectCoreBinding       `json:"effect_core"`
	Coverage         SurfaceCoreEffectCoverage `json:"coverage"`
}

type binderTargetRow struct {
	Kind           analysistarget.Kind            `json:"kind"`
	ModulePath     string                         `json:"module_path"`
	PackagePath    string                         `json:"package_path,omitempty"`
	PublicPackages []analysistarget.TargetPackage `json:"public_packages,omitempty"`
}

type binderSymbolRow struct {
	Ref     string `json:"ref"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Package string `json:"package"`
	Symbol  string `json:"symbol"`
}

type binderCoreRelationRow struct {
	CoreRef  string             `json:"core_ref"`
	Relation AnchorCoreRelation `json:"relation"`
	MinHops  *int               `json:"min_hops,omitempty"`
}

type binderSurfaceRow struct {
	Ref           string                  `json:"ref"`
	AnchorRef     string                  `json:"anchor_ref"`
	Kind          string                  `json:"kind"`
	Role          string                  `json:"role"`
	Form          string                  `json:"form"`
	Registration  Location                `json:"registration"`
	Identity      string                  `json:"identity,omitempty"`
	Method        string                  `json:"method,omitempty"`
	Path          string                  `json:"path,omitempty"`
	Handler       string                  `json:"handler,omitempty"`
	CoreRelations []binderCoreRelationRow `json:"core_relations"`
}

type binderCoreRow struct {
	Ref                string                        `json:"ref"`
	Name               string                        `json:"name"`
	Purpose            string                        `json:"purpose"`
	RepresentativeRefs []string                      `json:"representative_refs"`
	Representatives    []binderCoreRepresentativeRow `json:"representatives"`
	Objects            []binderCoreObjectRow         `json:"objects"`
}

// binderCoreRepresentativeRow preserves every exact CoreMap declaration in
// the binder request. GraphRef is present only when that declaration is also
// an exact DirectCall node. ProgramIndex-only declarations such as interface
// and type declarations remain useful structural evidence, but they never
// acquire invented direct-call topology.
type binderCoreRepresentativeRow struct {
	GraphRef           string                  `json:"graph_ref,omitempty"`
	Kind               programindex.ObjectKind `json:"kind,omitempty"`
	Package            string                  `json:"package"`
	Name               string                  `json:"name"`
	Path               string                  `json:"path"`
	Line               int                     `json:"line"`
	Column             int                     `json:"column,omitempty"`
	Exported           bool                    `json:"exported"`
	IncomingCalls      int                     `json:"incoming_calls"`
	OutgoingCalls      int                     `json:"outgoing_calls"`
	UnresolvedOutgoing int                     `json:"unresolved_outgoing,omitempty"`
}

type binderCoreObjectRow struct {
	Role      CoreObjectBindingRole `json:"role"`
	Kind      string                `json:"kind"`
	Package   string                `json:"package"`
	Name      string                `json:"name"`
	Receiver  string                `json:"receiver,omitempty"`
	Signature string                `json:"signature,omitempty"`
	Path      string                `json:"path"`
	Line      int                   `json:"line"`
}

type binderEffectRow struct {
	Ref               string                                `json:"ref"`
	AnchorRef         string                                `json:"anchor_ref"`
	DependencyName    string                                `json:"dependency_name"`
	DependencyPackage string                                `json:"dependency_package"`
	Receiver          string                                `json:"receiver,omitempty"`
	Operation         string                                `json:"operation"`
	Dispatch          surfacediscovery.ExternalCallDispatch `json:"dispatch"`
	Invocation        surfacediscovery.DirectCallInvocation `json:"invocation"`
	CoreRelations     []binderCoreRelationRow               `json:"core_relations"`
}

type surfaceCoreEffectRequest struct {
	Target   binderTargetRow    `json:"target"`
	Symbols  []binderSymbolRow  `json:"symbols"`
	Cores    []binderCoreRow    `json:"cores"`
	Surfaces []binderSurfaceRow `json:"surfaces"`
	Effects  []binderEffectRow  `json:"effects"`
}

type binderSurfaceCoreSelection struct {
	SurfaceRef string `json:"surface_ref"`
	CoreRef    string `json:"core_ref"`
}

type binderEffectCoreSelection struct {
	EffectRef string `json:"effect_ref"`
	CoreRef   string `json:"core_ref"`
}

type surfaceCoreEffectResponse struct {
	SurfaceCore []binderSurfaceCoreSelection `json:"surface_core"`
	EffectCore  []binderEffectCoreSelection  `json:"effect_core"`
}

type binderEffectAuthority struct {
	caller    Symbol
	operation IntegrationOperation
}

type binderPairAuthority struct {
	relation AnchorCoreRelation
	minHops  *int
}

type surfaceCoreEffectCompilation struct {
	request          surfaceCoreEffectRequest
	requestWire      []byte
	targetRef        string
	directCallSHA256 string
	authoritySHA256  string
	surfaces         map[string]activitysurface.Surface
	cores            map[string]coremap.Block
	effects          map[string]binderEffectAuthority
	surfacePairs     map[string]binderPairAuthority
	effectPairs      map[string]binderPairAuthority
}

func compileSurfaceCoreEffectBinder(
	core coremap.Result,
	objects CoreObjectProjection,
	activities activitysurface.Result,
	dependencies []IntegrationDependency,
	integrations []IntegrationSymbol,
	index surfacediscovery.DirectCallIndex,
) (surfaceCoreEffectCompilation, error) {
	if err := core.Validate(); err != nil {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder core: %w", err)
	}
	if err := objects.Validate(); err != nil {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder objects: %w", err)
	}
	if objects.CoreObjectIndexSHA256 != core.CoreObjectSHA256 {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder object authority mismatch")
	}
	if err := activities.Validate(); err != nil {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder activities: %w", err)
	}
	if err := index.Validate(); err != nil {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder index: %w", err)
	}
	if index.State != surfacediscovery.DirectCallIndexReady || core.DirectCallSHA256 != index.SHA256 {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder source authority mismatch")
	}
	if index.Scope.TargetRef != core.Target.Ref {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder target authority mismatch")
	}

	dependencyByID := make(map[string]IntegrationDependency, len(dependencies))
	for _, dependency := range dependencies {
		if err := validateIntegrationDependency(dependency); err != nil {
			return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder dependency: %w", err)
		}
		if _, duplicate := dependencyByID[dependency.ID]; duplicate {
			return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder duplicate dependency")
		}
		dependencyByID[dependency.ID] = dependency
	}

	wantedNodeIDs := make(map[string]struct{})
	for _, surface := range activities.Surfaces {
		wantedNodeIDs[surface.RootNodeID] = struct{}{}
	}
	coreGraphNodeIDs := make(map[string]struct{})
	for _, block := range core.Refined {
		for _, symbol := range block.Symbols {
			node, exists := index.Node(symbol.NodeID)
			if !exists {
				if core.ProgramTarget == nil || core.ProgramTarget.Language != "go" || !symbol.Kind.Valid() {
					return surfaceCoreEffectCompilation{}, fmt.Errorf(
						"cubemap: surface-core-effect binder core symbol is absent from exact graph without Go ProgramIndex authority",
					)
				}
				continue
			}
			if !directNodeMatchesCoreSymbol(node, symbol) {
				return surfaceCoreEffectCompilation{}, fmt.Errorf(
					"cubemap: surface-core-effect binder core symbol authority mismatch",
				)
			}
			wantedNodeIDs[symbol.NodeID] = struct{}{}
			coreGraphNodeIDs[symbol.NodeID] = struct{}{}
		}
	}
	for _, integration := range integrations {
		wantedNodeIDs[integration.Symbol.NodeID] = struct{}{}
	}

	symbolRefByNodeID := make(map[string]string, len(wantedNodeIDs))
	symbols := make([]binderSymbolRow, 0, len(wantedNodeIDs))
	for _, node := range index.Nodes {
		if _, wanted := wantedNodeIDs[node.ID]; !wanted {
			continue
		}
		ref := fmt.Sprintf("n%d", len(symbols)+1)
		symbolRefByNodeID[node.ID] = ref
		symbols = append(symbols, binderSymbolRow{
			Ref: ref, Path: node.Declaration.Path, Line: node.Declaration.Line,
			Package: node.Package, Symbol: node.Symbol.Name,
		})
	}
	if len(symbolRefByNodeID) != len(wantedNodeIDs) {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder has an unknown exact anchor")
	}

	callableByID := make(map[string]gocoreobject.CallableDeclaration, len(objects.Callables))
	for _, callable := range objects.Callables {
		callableByID[callable.ID] = callable
	}
	typeByID := make(map[string]gocoreobject.TypeDeclaration, len(objects.ReceiverTypes))
	for _, declaration := range objects.ReceiverTypes {
		typeByID[declaration.ID] = declaration
	}
	objectRowsByBlock := make(map[string][]binderCoreObjectRow)
	for _, binding := range objects.Bindings {
		switch binding.Role {
		case CoreObjectRepresentativeCallable:
			callable := callableByID[binding.ObjectID]
			objectRowsByBlock[binding.CoreBlockID] = append(objectRowsByBlock[binding.CoreBlockID], binderCoreObjectRow{
				Role: binding.Role, Kind: string(callable.Kind), Package: callable.Package,
				Name: callable.Name, Receiver: callable.Receiver, Signature: callable.Signature,
				Path: callable.Location.Path, Line: callable.Location.Line,
			})
		case CoreObjectReceiverType:
			declaration := typeByID[binding.ObjectID]
			objectRowsByBlock[binding.CoreBlockID] = append(objectRowsByBlock[binding.CoreBlockID], binderCoreObjectRow{
				Role: binding.Role, Kind: string(declaration.Kind), Package: declaration.Package,
				Name: declaration.Name, Path: declaration.Location.Path, Line: declaration.Location.Line,
			})
		}
	}
	coreByRef := make(map[string]coremap.Block, len(core.Refined))
	coreRows := make([]binderCoreRow, 0, len(core.Refined))
	coreNodeIDs := make(map[string][]string, len(core.Refined))
	seenCoreIDs := make(map[string]struct{}, len(core.Refined))
	for position, block := range core.Refined {
		if _, duplicate := seenCoreIDs[block.ID]; duplicate {
			return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder core IDs are not unique")
		}
		seenCoreIDs[block.ID] = struct{}{}
		ref := fmt.Sprintf("c%d", position+1)
		row := binderCoreRow{
			Ref: ref, Name: block.Name, Purpose: block.Purpose,
			RepresentativeRefs: []string{},
			Representatives:    []binderCoreRepresentativeRow{},
			Objects:            append([]binderCoreObjectRow{}, objectRowsByBlock[block.ID]...),
		}
		for _, symbol := range block.Symbols {
			anchorRef := ""
			if _, graphNode := coreGraphNodeIDs[symbol.NodeID]; graphNode {
				anchorRef = symbolRefByNodeID[symbol.NodeID]
				if anchorRef == "" {
					return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder core has an unknown exact graph symbol")
				}
				row.RepresentativeRefs = append(row.RepresentativeRefs, anchorRef)
				coreNodeIDs[ref] = append(coreNodeIDs[ref], symbol.NodeID)
			}
			row.Representatives = append(row.Representatives, binderCoreRepresentativeRow{
				GraphRef: anchorRef, Kind: symbol.Kind, Package: symbol.Package,
				Name: symbol.Symbol.Name, Path: symbol.Declaration.Path,
				Line: symbol.Declaration.Line, Column: symbol.Declaration.Column,
				Exported: symbol.Exported, IncomingCalls: symbol.IncomingCalls,
				OutgoingCalls: symbol.OutgoingCalls, UnresolvedOutgoing: symbol.UnresolvedOutgoing,
			})
		}
		if len(row.Representatives) == 0 {
			return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder core has no exact declaration")
		}
		coreRows = append(coreRows, row)
		coreByRef[ref] = block
	}

	adjacency := buildBinderAdjacency(index)
	relationsByAnchor := make(map[string][]binderCoreRelationRow)
	relationAuthorityByAnchor := make(map[string]map[string]binderPairAuthority)
	compileRelations := func(anchorNodeID string) ([]binderCoreRelationRow, map[string]binderPairAuthority) {
		if rows, exists := relationsByAnchor[anchorNodeID]; exists {
			return rows, relationAuthorityByAnchor[anchorNodeID]
		}
		forward := binderDistances(anchorNodeID, adjacency.outgoing)
		reverse := binderDistances(anchorNodeID, adjacency.incoming)
		rows := make([]binderCoreRelationRow, 0, len(coreRows))
		authority := make(map[string]binderPairAuthority, len(coreRows))
		for _, coreRow := range coreRows {
			relation := aggregateAnchorCoreRelation(anchorNodeID, coreNodeIDs[coreRow.Ref], forward, reverse)
			rows = append(rows, binderCoreRelationRow{
				CoreRef: coreRow.Ref, Relation: relation.relation, MinHops: cloneInt(relation.minHops),
			})
			authority[coreRow.Ref] = relation
		}
		relationsByAnchor[anchorNodeID] = rows
		relationAuthorityByAnchor[anchorNodeID] = authority
		return rows, authority
	}

	surfaceByRef := make(map[string]activitysurface.Surface, len(activities.Surfaces))
	surfaceRows := make([]binderSurfaceRow, 0, len(activities.Surfaces))
	surfacePairs := make(map[string]binderPairAuthority, len(activities.Surfaces)*len(coreRows))
	for position, surface := range activities.Surfaces {
		ref := fmt.Sprintf("s%d", position+1)
		relations, authority := compileRelations(surface.RootNodeID)
		row := binderSurfaceRow{
			Ref: ref, AnchorRef: symbolRefByNodeID[surface.RootNodeID], Kind: surface.Kind,
			Role: surface.Role, Form: string(surface.Form), Registration: locationFromEntryCall(surface.Registration),
			Identity: surfaceValueText(surface.Identity), Method: surfaceValueText(surface.Method),
			Path: surfaceValueText(surface.Path), Handler: surfaceValueText(surface.Handler),
			CoreRelations: cloneBinderRelations(relations),
		}
		surfaceRows = append(surfaceRows, row)
		surfaceByRef[ref] = surface
		for coreRef, relation := range authority {
			surfacePairs[binderPairKey(ref, coreRef)] = relation
		}
	}

	effectByRef := make(map[string]binderEffectAuthority)
	effectRows := make([]binderEffectRow, 0)
	effectPairs := make(map[string]binderPairAuthority)
	seenEffects := make(map[string]struct{})
	for _, integration := range integrations {
		if err := validateSymbol(integration.Symbol); err != nil {
			return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder integration symbol: %w", err)
		}
		node, exists := index.Node(integration.Symbol.NodeID)
		if !exists || symbolFromNode(node) != integration.Symbol {
			return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder integration symbol authority mismatch")
		}
		relations, authority := compileRelations(integration.Symbol.NodeID)
		for _, operation := range integration.Operations {
			dependency, exists := dependencyByID[operation.DependencyID]
			if !exists {
				return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder effect has an unknown dependency")
			}
			if err := validateIntegrationOperation(operation, dependencyByID); err != nil {
				return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder effect: %w", err)
			}
			effectKey := binderPairKey(integration.Symbol.NodeID, operation.ExternalCallFamilyID)
			if _, duplicate := seenEffects[effectKey]; duplicate {
				return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder effects are not unique")
			}
			seenEffects[effectKey] = struct{}{}
			ref := fmt.Sprintf("e%d", len(effectRows)+1)
			effectRows = append(effectRows, binderEffectRow{
				Ref: ref, AnchorRef: symbolRefByNodeID[integration.Symbol.NodeID],
				DependencyName: dependency.Name, DependencyPackage: dependency.PackagePath,
				Receiver: operation.Receiver, Operation: operation.Name,
				Dispatch: operation.Dispatch, Invocation: operation.Invocation,
				CoreRelations: cloneBinderRelations(relations),
			})
			effectByRef[ref] = binderEffectAuthority{caller: integration.Symbol, operation: operation}
			for coreRef, relation := range authority {
				effectPairs[binderPairKey(ref, coreRef)] = relation
			}
		}
	}

	request := surfaceCoreEffectRequest{
		Target: binderTargetRow{
			Kind: core.Target.Kind, ModulePath: core.Target.ModulePath, PackagePath: core.Target.PackagePath,
			PublicPackages: append([]analysistarget.TargetPackage(nil), core.Target.LibraryPackages...),
		},
		Symbols: symbols, Cores: coreRows, Surfaces: surfaceRows, Effects: effectRows,
	}
	requestWire, err := json.Marshal(request)
	if err != nil {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder encode request: %w", err)
	}
	if len(requestWire)+len(strings.TrimSpace(surfaceCoreEffectsPrompt)) > maxRequestBytes {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder request exceeds bounded envelope")
	}

	authoritySHA256, err := surfaceCoreEffectAuthoritySHA(
		core.Target.Ref, index.SHA256, requestWire, surfaceByRef, coreByRef, effectByRef,
	)
	if err != nil {
		return surfaceCoreEffectCompilation{}, fmt.Errorf("cubemap: surface-core-effect binder encode authority: %w", err)
	}
	return surfaceCoreEffectCompilation{
		request: request, requestWire: requestWire, targetRef: core.Target.Ref,
		directCallSHA256: index.SHA256, authoritySHA256: authoritySHA256,
		surfaces: surfaceByRef, cores: coreByRef, effects: effectByRef,
		surfacePairs: surfacePairs, effectPairs: effectPairs,
	}, nil
}

// directNodeMatchesCoreSymbol joins on the DirectCall node identity and every
// user-meaningful declaration fact. Symbol.ID is deliberately not compared:
// a ProgramIndex-backed Go CoreMap retains its ProgramIndex object identity in
// the nested source symbol while NodeID is locally rebound to the exact
// DirectCall identity. Treating that redundant producer-local ID as graph
// authority would reject an otherwise exact join.
func directNodeMatchesCoreSymbol(node surfacediscovery.DirectCallNode, symbol coremap.SymbolFact) bool {
	return node.ID == symbol.NodeID &&
		node.Package == symbol.Package &&
		node.Exported == symbol.Exported &&
		node.Declaration == symbol.Declaration &&
		node.Symbol.Package == symbol.Symbol.Package &&
		node.Symbol.Name == symbol.Symbol.Name &&
		node.Symbol.Location == symbol.Symbol.Location
}

type binderEffectAuthorityWire struct {
	CallerNodeID         string `json:"caller_node_id"`
	ExternalCallFamilyID string `json:"external_call_family_id"`
}

func binderEffectAuthorityForWire(values map[string]binderEffectAuthority) map[string]binderEffectAuthorityWire {
	result := make(map[string]binderEffectAuthorityWire, len(values))
	for ref, value := range values {
		result[ref] = binderEffectAuthorityWire{
			CallerNodeID: value.caller.NodeID, ExternalCallFamilyID: value.operation.ExternalCallFamilyID,
		}
	}
	return result
}

func surfaceCoreEffectAuthoritySHA(
	targetRef string,
	directCallSHA256 string,
	requestWire []byte,
	surfaces map[string]activitysurface.Surface,
	cores map[string]coremap.Block,
	effects map[string]binderEffectAuthority,
) (string, error) {
	authorityWire, err := json.Marshal(struct {
		TargetRef  string                               `json:"target_ref"`
		DirectCall string                               `json:"direct_call_sha256"`
		RequestSHA string                               `json:"request_sha256"`
		Surfaces   map[string]activitysurface.Surface   `json:"surfaces"`
		Cores      map[string]coremap.Block             `json:"cores"`
		Effects    map[string]binderEffectAuthorityWire `json:"effects"`
	}{
		TargetRef: targetRef, DirectCall: directCallSHA256, RequestSHA: sha256Hex(requestWire),
		Surfaces: surfaces, Cores: cores, Effects: binderEffectAuthorityForWire(effects),
	})
	if err != nil {
		return "", err
	}
	return sha256Hex(authorityWire), nil
}

func runSurfaceCoreEffectBinder(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation surfaceCoreEffectCompilation,
) (SurfaceCoreEffectBindings, error) {
	if err := validateSurfaceCoreEffectCompilation(compilation); err != nil {
		return SurfaceCoreEffectBindings{}, err
	}
	if len(compilation.cores) == 0 || len(compilation.surfaces)+len(compilation.effects) == 0 {
		return emptySurfaceCoreEffectBindings(compilation), nil
	}
	state, err := cubeState("surface-core-effects", surfaceCoreEffectsPrompt, struct {
		AuthoritySHA256 string `json:"authority_sha256"`
	}{AuthoritySHA256: compilation.authoritySHA256})
	if err != nil {
		return SurfaceCoreEffectBindings{}, fmt.Errorf("cubemap: surface-core-effect binder state: %w", err)
	}
	outcome, err := llm.ExecuteJSON(ctx, executorForStage(executor, StageSurfaceCoreEffects), provider, llm.Call[SurfaceCoreEffectBindings]{
		State: state,
		Prompt: llm.Prompt{
			System: strings.TrimSpace(surfaceCoreEffectsPrompt), User: string(compilation.requestWire), ResponseFormatJSON: true,
		},
		Limits: cubeLimits(),
		DecodeValidate: func(raw []byte) (SurfaceCoreEffectBindings, error) {
			return reduceSurfaceCoreEffectBindings(compilation, raw)
		},
	})
	if err != nil {
		return SurfaceCoreEffectBindings{}, fmt.Errorf("cubemap: surface-core-effect binder cube: %w", err)
	}
	return outcome.Value, nil
}

func reduceSurfaceCoreEffectBindings(
	compilation surfaceCoreEffectCompilation,
	raw []byte,
) (SurfaceCoreEffectBindings, error) {
	if err := validateSurfaceCoreEffectCompilation(compilation); err != nil {
		return SurfaceCoreEffectBindings{}, err
	}
	response, err := decodeSurfaceCoreEffectResponse(raw)
	if err != nil {
		return SurfaceCoreEffectBindings{}, err
	}
	if response.SurfaceCore == nil || response.EffectCore == nil ||
		len(response.SurfaceCore)+len(response.EffectCore) > maxSurfaceCoreEffectBindings {
		return SurfaceCoreEffectBindings{}, fmt.Errorf("cubemap: surface-core-effect binder response exceeds selection bounds")
	}
	result := emptySurfaceCoreEffectBindings(compilation)
	result.Coverage.ModelCalled = true
	seenSurface := make(map[string]struct{}, len(response.SurfaceCore))
	for _, selected := range response.SurfaceCore {
		key := binderPairKey(selected.SurfaceRef, selected.CoreRef)
		relation, exists := compilation.surfacePairs[key]
		if !exists {
			return SurfaceCoreEffectBindings{}, fmt.Errorf("cubemap: surface-core-effect binder response cites an unknown surface/core pair")
		}
		if _, duplicate := seenSurface[key]; duplicate {
			return SurfaceCoreEffectBindings{}, fmt.Errorf("cubemap: surface-core-effect binder response repeats a surface/core pair")
		}
		seenSurface[key] = struct{}{}
		result.SurfaceCore = append(result.SurfaceCore, SurfaceCoreBinding{
			SurfaceID:   compilation.surfaces[selected.SurfaceRef].ID,
			CoreBlockID: compilation.cores[selected.CoreRef].ID,
			Relation:    relation.relation, MinHops: cloneInt(relation.minHops),
		})
	}
	seenEffect := make(map[string]struct{}, len(response.EffectCore))
	for _, selected := range response.EffectCore {
		key := binderPairKey(selected.EffectRef, selected.CoreRef)
		relation, exists := compilation.effectPairs[key]
		if !exists {
			return SurfaceCoreEffectBindings{}, fmt.Errorf("cubemap: surface-core-effect binder response cites an unknown effect/core pair")
		}
		if _, duplicate := seenEffect[key]; duplicate {
			return SurfaceCoreEffectBindings{}, fmt.Errorf("cubemap: surface-core-effect binder response repeats an effect/core pair")
		}
		seenEffect[key] = struct{}{}
		effect := compilation.effects[selected.EffectRef]
		result.EffectCore = append(result.EffectCore, EffectCoreBinding{
			ExternalCallFamilyID: effect.operation.ExternalCallFamilyID,
			CallerNodeID:         effect.caller.NodeID, CoreBlockID: compilation.cores[selected.CoreRef].ID,
			Relation: relation.relation, MinHops: cloneInt(relation.minHops),
		})
	}
	sortSurfaceCoreEffectBindings(&result)
	result.Coverage.SelectedSurfaceCore = len(result.SurfaceCore)
	result.Coverage.SelectedEffectCore = len(result.EffectCore)
	if err := result.ValidateAgainst(compilation); err != nil {
		return SurfaceCoreEffectBindings{}, err
	}
	return result, nil
}

func decodeSurfaceCoreEffectResponse(raw []byte) (surfaceCoreEffectResponse, error) {
	var response surfaceCoreEffectResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return surfaceCoreEffectResponse{}, fmt.Errorf("cubemap: surface-core-effect binder decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return surfaceCoreEffectResponse{}, fmt.Errorf("cubemap: surface-core-effect binder response contains trailing JSON")
	}
	return response, nil
}

func emptySurfaceCoreEffectBindings(compilation surfaceCoreEffectCompilation) SurfaceCoreEffectBindings {
	return SurfaceCoreEffectBindings{
		Version: SurfaceCoreEffectBindingsVersion, TargetRef: compilation.targetRef,
		DirectCallSHA256: compilation.directCallSHA256, AuthoritySHA256: compilation.authoritySHA256,
		SurfaceCore: []SurfaceCoreBinding{}, EffectCore: []EffectCoreBinding{},
		Coverage: SurfaceCoreEffectCoverage{
			Surfaces: len(compilation.surfaces), CoreBlocks: len(compilation.cores), Effects: len(compilation.effects),
			SurfaceCorePairs: len(compilation.surfacePairs), EffectCorePairs: len(compilation.effectPairs),
		},
	}
}

func (bindings SurfaceCoreEffectBindings) Validate() error {
	if bindings.Version != SurfaceCoreEffectBindingsVersion || strings.TrimSpace(bindings.TargetRef) == "" ||
		!validSHA256(bindings.DirectCallSHA256) || !validSHA256(bindings.AuthoritySHA256) ||
		bindings.SurfaceCore == nil || bindings.EffectCore == nil {
		return fmt.Errorf("cubemap: invalid surface-core-effect binding identity")
	}
	coverage := bindings.Coverage
	counts := []int{
		coverage.Surfaces, coverage.CoreBlocks, coverage.Effects, coverage.SurfaceCorePairs,
		coverage.EffectCorePairs, coverage.SelectedSurfaceCore, coverage.SelectedEffectCore,
	}
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("cubemap: invalid surface-core-effect binding coverage")
		}
	}
	if coverage.SurfaceCorePairs != coverage.Surfaces*coverage.CoreBlocks ||
		coverage.EffectCorePairs != coverage.Effects*coverage.CoreBlocks ||
		coverage.SelectedSurfaceCore != len(bindings.SurfaceCore) ||
		coverage.SelectedEffectCore != len(bindings.EffectCore) ||
		coverage.SelectedSurfaceCore+coverage.SelectedEffectCore > maxSurfaceCoreEffectBindings ||
		coverage.ModelCalled != (coverage.CoreBlocks > 0 && coverage.Surfaces+coverage.Effects > 0) {
		return fmt.Errorf("cubemap: invalid surface-core-effect binding accounting")
	}
	for position, binding := range bindings.SurfaceCore {
		if strings.TrimSpace(binding.SurfaceID) == "" || strings.TrimSpace(binding.CoreBlockID) == "" ||
			!validAnchorCoreDistance(binding.Relation, binding.MinHops) {
			return fmt.Errorf("cubemap: invalid surface/core binding")
		}
		if position > 0 && surfaceCoreBindingKey(bindings.SurfaceCore[position-1]) >= surfaceCoreBindingKey(binding) {
			return fmt.Errorf("cubemap: surface/core bindings are not canonical")
		}
	}
	for position, binding := range bindings.EffectCore {
		if strings.TrimSpace(binding.ExternalCallFamilyID) == "" || strings.TrimSpace(binding.CallerNodeID) == "" ||
			strings.TrimSpace(binding.CoreBlockID) == "" || !validAnchorCoreDistance(binding.Relation, binding.MinHops) {
			return fmt.Errorf("cubemap: invalid effect/core binding")
		}
		if position > 0 && effectCoreBindingKey(bindings.EffectCore[position-1]) >= effectCoreBindingKey(binding) {
			return fmt.Errorf("cubemap: effect/core bindings are not canonical")
		}
	}
	return nil
}

func (bindings SurfaceCoreEffectBindings) ValidateAgainst(compilation surfaceCoreEffectCompilation) error {
	if err := validateSurfaceCoreEffectCompilation(compilation); err != nil {
		return err
	}
	if err := bindings.Validate(); err != nil {
		return err
	}
	if bindings.TargetRef != compilation.targetRef || bindings.DirectCallSHA256 != compilation.directCallSHA256 ||
		bindings.AuthoritySHA256 != compilation.authoritySHA256 {
		return fmt.Errorf("cubemap: surface-core-effect binding authority mismatch")
	}
	wantCoverage := emptySurfaceCoreEffectBindings(compilation).Coverage
	wantCoverage.SelectedSurfaceCore = len(bindings.SurfaceCore)
	wantCoverage.SelectedEffectCore = len(bindings.EffectCore)
	wantCoverage.ModelCalled = len(compilation.cores) > 0 && len(compilation.surfaces)+len(compilation.effects) > 0
	if bindings.Coverage != wantCoverage {
		return fmt.Errorf("cubemap: surface-core-effect binding coverage authority mismatch")
	}
	for _, binding := range bindings.SurfaceCore {
		matched := false
		for surfaceRef, surface := range compilation.surfaces {
			if surface.ID != binding.SurfaceID {
				continue
			}
			for coreRef, block := range compilation.cores {
				if block.ID != binding.CoreBlockID {
					continue
				}
				relation := compilation.surfacePairs[binderPairKey(surfaceRef, coreRef)]
				matched = relation.relation == binding.Relation && equalOptionalInt(relation.minHops, binding.MinHops)
			}
		}
		if !matched {
			return fmt.Errorf("cubemap: surface/core binding does not restore from exact authority")
		}
	}
	for _, binding := range bindings.EffectCore {
		matched := false
		for effectRef, effect := range compilation.effects {
			if effect.caller.NodeID != binding.CallerNodeID || effect.operation.ExternalCallFamilyID != binding.ExternalCallFamilyID {
				continue
			}
			for coreRef, block := range compilation.cores {
				if block.ID != binding.CoreBlockID {
					continue
				}
				relation := compilation.effectPairs[binderPairKey(effectRef, coreRef)]
				matched = relation.relation == binding.Relation && equalOptionalInt(relation.minHops, binding.MinHops)
			}
		}
		if !matched {
			return fmt.Errorf("cubemap: effect/core binding does not restore from exact authority")
		}
	}
	return nil
}

func validateSurfaceCoreEffectCompilation(compilation surfaceCoreEffectCompilation) error {
	if strings.TrimSpace(compilation.targetRef) == "" || !validSHA256(compilation.directCallSHA256) ||
		!validSHA256(compilation.authoritySHA256) || len(compilation.requestWire) == 0 ||
		compilation.surfaces == nil || compilation.cores == nil || compilation.effects == nil ||
		compilation.surfacePairs == nil || compilation.effectPairs == nil {
		return fmt.Errorf("cubemap: invalid surface-core-effect compilation")
	}
	wire, err := json.Marshal(compilation.request)
	if err != nil || !bytes.Equal(wire, compilation.requestWire) ||
		len(compilation.request.Surfaces) != len(compilation.surfaces) ||
		len(compilation.request.Cores) != len(compilation.cores) ||
		len(compilation.request.Effects) != len(compilation.effects) ||
		len(compilation.surfacePairs) != len(compilation.surfaces)*len(compilation.cores) ||
		len(compilation.effectPairs) != len(compilation.effects)*len(compilation.cores) {
		return fmt.Errorf("cubemap: surface-core-effect compilation binding mismatch")
	}
	wantAuthority, err := surfaceCoreEffectAuthoritySHA(
		compilation.targetRef, compilation.directCallSHA256, compilation.requestWire,
		compilation.surfaces, compilation.cores, compilation.effects,
	)
	if err != nil || wantAuthority != compilation.authoritySHA256 {
		return fmt.Errorf("cubemap: surface-core-effect compilation authority mismatch")
	}
	for _, core := range compilation.request.Cores {
		if _, exists := compilation.cores[core.Ref]; !exists || len(core.Representatives) == 0 {
			return fmt.Errorf("cubemap: surface-core-effect compilation core authority mismatch")
		}
		graphRefs := make([]string, 0, len(core.Representatives))
		for _, representative := range core.Representatives {
			if representative.Name == "" || representative.Package == "" || representative.Path == "" ||
				representative.Line < 1 || representative.Column < 0 || representative.IncomingCalls < 0 ||
				representative.OutgoingCalls < 0 || representative.UnresolvedOutgoing < 0 ||
				representative.Kind != "" && !representative.Kind.Valid() {
				return fmt.Errorf("cubemap: surface-core-effect compilation has an invalid core declaration")
			}
			if representative.GraphRef != "" {
				graphRefs = append(graphRefs, representative.GraphRef)
			}
		}
		if !slices.Equal(graphRefs, core.RepresentativeRefs) {
			return fmt.Errorf("cubemap: surface-core-effect compilation graph representative mismatch")
		}
	}
	for _, surface := range compilation.request.Surfaces {
		if _, exists := compilation.surfaces[surface.Ref]; !exists || len(surface.CoreRelations) != len(compilation.cores) {
			return fmt.Errorf("cubemap: surface-core-effect compilation surface authority mismatch")
		}
		seenCoreRefs := make(map[string]struct{}, len(surface.CoreRelations))
		for _, relation := range surface.CoreRelations {
			if _, duplicate := seenCoreRefs[relation.CoreRef]; duplicate {
				return fmt.Errorf("cubemap: surface-core-effect compilation repeats a surface relation")
			}
			seenCoreRefs[relation.CoreRef] = struct{}{}
			exact, exists := compilation.surfacePairs[binderPairKey(surface.Ref, relation.CoreRef)]
			if !exists || exact.relation != relation.Relation || !equalOptionalInt(exact.minHops, relation.MinHops) {
				return fmt.Errorf("cubemap: surface-core-effect compilation surface relation mismatch")
			}
		}
	}
	for _, effect := range compilation.request.Effects {
		if _, exists := compilation.effects[effect.Ref]; !exists || len(effect.CoreRelations) != len(compilation.cores) {
			return fmt.Errorf("cubemap: surface-core-effect compilation effect authority mismatch")
		}
		seenCoreRefs := make(map[string]struct{}, len(effect.CoreRelations))
		for _, relation := range effect.CoreRelations {
			if _, duplicate := seenCoreRefs[relation.CoreRef]; duplicate {
				return fmt.Errorf("cubemap: surface-core-effect compilation repeats an effect relation")
			}
			seenCoreRefs[relation.CoreRef] = struct{}{}
			exact, exists := compilation.effectPairs[binderPairKey(effect.Ref, relation.CoreRef)]
			if !exists || exact.relation != relation.Relation || !equalOptionalInt(exact.minHops, relation.MinHops) {
				return fmt.Errorf("cubemap: surface-core-effect compilation effect relation mismatch")
			}
		}
	}
	return nil
}

type binderAdjacency struct {
	outgoing map[string][]string
	incoming map[string][]string
}

func buildBinderAdjacency(index surfacediscovery.DirectCallIndex) binderAdjacency {
	result := binderAdjacency{outgoing: make(map[string][]string), incoming: make(map[string][]string)}
	for _, edge := range index.Edges {
		result.outgoing[edge.CallerID] = append(result.outgoing[edge.CallerID], edge.CalleeID)
		result.incoming[edge.CalleeID] = append(result.incoming[edge.CalleeID], edge.CallerID)
	}
	return result
}

func binderDistances(start string, adjacency map[string][]string) map[string]int {
	distances := map[string]int{start: 0}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if _, seen := distances[next]; seen {
				continue
			}
			distances[next] = distances[current] + 1
			queue = append(queue, next)
		}
	}
	return distances
}

func aggregateAnchorCoreRelation(
	anchor string,
	coreNodeIDs []string,
	forward map[string]int,
	reverse map[string]int,
) binderPairAuthority {
	forwardDistance, forwardFound := minimumBinderDistance(coreNodeIDs, forward)
	reverseDistance, reverseFound := minimumBinderDistance(coreNodeIDs, reverse)
	for _, nodeID := range coreNodeIDs {
		if nodeID == anchor {
			zero := 0
			return binderPairAuthority{relation: AnchorCoreSameSymbol, minHops: &zero}
		}
	}
	if forwardFound && (!reverseFound || forwardDistance <= reverseDistance) {
		return binderPairAuthority{relation: AnchorReachesCore, minHops: intPointer(forwardDistance)}
	}
	if reverseFound {
		return binderPairAuthority{relation: CoreReachesAnchor, minHops: intPointer(reverseDistance)}
	}
	return binderPairAuthority{relation: AnchorCoreUnconnected}
}

func minimumBinderDistance(nodeIDs []string, distances map[string]int) (int, bool) {
	minimum := 0
	found := false
	for _, nodeID := range nodeIDs {
		distance, exists := distances[nodeID]
		if !exists || distance == 0 {
			continue
		}
		if !found || distance < minimum {
			minimum = distance
			found = true
		}
	}
	return minimum, found
}

func surfaceValueText(value *activitysurface.Value) string {
	if value == nil {
		return ""
	}
	return value.Text
}

func locationFromEntryCall(value struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}) Location {
	return Location{Path: value.Path, Line: value.Line, Column: value.Column}
}

func cloneBinderRelations(values []binderCoreRelationRow) []binderCoreRelationRow {
	result := make([]binderCoreRelationRow, len(values))
	for position, value := range values {
		result[position] = value
		result[position].MinHops = cloneInt(value.MinHops)
	}
	return result
}

func binderPairKey(left, right string) string { return left + "\x00" + right }

func intPointer(value int) *int { return &value }

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func equalOptionalInt(left, right *int) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func validAnchorCoreDistance(relation AnchorCoreRelation, distance *int) bool {
	if !relation.valid() {
		return false
	}
	switch relation {
	case AnchorCoreSameSymbol:
		return distance != nil && *distance == 0
	case AnchorReachesCore, CoreReachesAnchor:
		return distance != nil && *distance > 0
	case AnchorCoreUnconnected:
		return distance == nil
	default:
		return false
	}
}

func sortSurfaceCoreEffectBindings(value *SurfaceCoreEffectBindings) {
	sort.Slice(value.SurfaceCore, func(i, j int) bool {
		return surfaceCoreBindingKey(value.SurfaceCore[i]) < surfaceCoreBindingKey(value.SurfaceCore[j])
	})
	sort.Slice(value.EffectCore, func(i, j int) bool {
		return effectCoreBindingKey(value.EffectCore[i]) < effectCoreBindingKey(value.EffectCore[j])
	})
}

func surfaceCoreBindingKey(value SurfaceCoreBinding) string {
	return value.SurfaceID + "\x00" + value.CoreBlockID
}

func effectCoreBindingKey(value EffectCoreBinding) string {
	return value.CallerNodeID + "\x00" + value.ExternalCallFamilyID + "\x00" + value.CoreBlockID
}
