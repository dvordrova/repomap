// Package cubemap builds the first domain-cube map of repository activity
// entrypoints, integration boundaries, and exact local call paths between them.
package cubemap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/activitysurface"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	Version          = 6
	ArtifactFilename = "cube-map.json"
)

// Symbol is a locally restored exact graph node. NodeID is never advertised to
// a model; model calls use request-local eN/uN references instead.
type Symbol struct {
	NodeID  string `json:"node_id"`
	Package string `json:"package"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column,omitempty"`
}

type Importer struct {
	PackagePath    string `json:"package_path"`
	RepositoryPath string `json:"repository_path"`
}

// IntegrationDependency is a model-selected external dependency restored from
// the exact language dependency catalog.
type IntegrationDependency struct {
	ID            string            `json:"id"`
	Kind          dependencies.Kind `json:"kind"`
	Name          string            `json:"name"`
	ModulePath    string            `json:"module_path,omitempty"`
	ModuleVersion string            `json:"module_version,omitempty"`
	PackagePath   string            `json:"package_path"`
	Importers     []Importer        `json:"importers"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

// IntegrationOperation is one exact external SSA call family restored after
// the model selects that operation as a strong integration use.
// Package and operation names are evidence, never backend-owned semantics.
type IntegrationOperation struct {
	ExternalCallFamilyID string                                `json:"external_call_family_id"`
	DependencyID         string                                `json:"dependency_id"`
	PackagePath          string                                `json:"package_path"`
	Receiver             string                                `json:"receiver,omitempty"`
	Name                 string                                `json:"name"`
	Dispatch             surfacediscovery.ExternalCallDispatch `json:"dispatch"`
	Invocation           surfacediscovery.DirectCallInvocation `json:"invocation"`
	WitnessCount         int                                   `json:"witness_count"`
	Callsites            []Location                            `json:"callsites"`
	CallsitesOmitted     int                                   `json:"callsites_omitted"`
}

// IntegrationSymbol groups model-selected exact integration operations by
// their repository caller. Incidental operations in the same caller are not
// restored merely because one operation was selected.
type IntegrationSymbol struct {
	Symbol        Symbol                 `json:"symbol"`
	DependencyIDs []string               `json:"dependency_ids"`
	Operations    []IntegrationOperation `json:"operations"`
}

// Path is the deterministic shortest exact direct-call path from a selected
// activity entrypoint down to a selected integration symbol.
type Path struct {
	EntrypointNodeID  string   `json:"entrypoint_node_id"`
	IntegrationNodeID string   `json:"integration_node_id"`
	Nodes             []Symbol `json:"nodes"`
}

type CandidateCoverage struct {
	Observed    int  `json:"observed"`
	Advertised  int  `json:"advertised"`
	Omitted     int  `json:"omitted"`
	ModelCalled bool `json:"model_called"`
}

// DependencyOmissionCount preserves why exact direct-import uses did not
// enter the typed dependency catalog without copying repository paths into a
// browser-facing coverage ledger.
type DependencyOmissionCount struct {
	Reason dependencies.OmissionReason `json:"reason"`
	Count  int                         `json:"count"`
}

// DependencyCatalogCoverage is the lossless count summary of the
// language-adapter dependency catalog. Integration selection must not turn a
// partial catalog into an apparently authoritative empty result.
type DependencyCatalogCoverage struct {
	State           dependencies.CoverageState `json:"state"`
	ImportsObserved int                        `json:"imports_observed"`
	ImportsRetained int                        `json:"imports_retained"`
	Omissions       int                        `json:"omissions"`
	Reasons         []DependencyOmissionCount  `json:"reasons"`
}

type Coverage struct {
	DependencyCatalog            DependencyCatalogCoverage                  `json:"dependency_catalog"`
	Entrypoints                  CandidateCoverage                          `json:"entrypoints"`
	IntegrationDependencies      CandidateCoverage                          `json:"integration_dependencies"`
	IntegrationSymbols           CandidateCoverage                          `json:"integration_symbols"`
	UnconnectedSymbols           int                                        `json:"unconnected_symbols"`
	GoFilesObserved              int                                        `json:"go_files_observed"`
	GoFilesParsed                int                                        `json:"go_files_parsed"`
	GoFilesSkipped               int                                        `json:"go_files_skipped"`
	ExternalCallFamiliesObserved int                                        `json:"external_call_families_observed"`
	ExternalCallFamiliesMatched  int                                        `json:"external_call_families_matched"`
	ExternalCalls                surfacediscovery.ExternalCallIndexCoverage `json:"external_calls"`
}

// Map is the locally restored pipeline result. It contains stable local
// identities; those identities never occur in provider requests.
type Map struct {
	Version                 int                        `json:"version"`
	SourceIndexSHA256       string                     `json:"source_index_sha256"`
	ExternalCallIndexSHA256 string                     `json:"external_call_index_sha256"`
	DependencyCatalogSHA256 string                     `json:"dependency_catalog_sha256"`
	Core                    coremap.Result             `json:"core"`
	CoreObjects             CoreObjectProjection       `json:"core_objects"`
	ActivitySurfaces        activitysurface.Result     `json:"activity_surfaces"`
	Entrypoints             []Symbol                   `json:"entrypoints"`
	IntegrationDependencies []IntegrationDependency    `json:"integration_dependencies"`
	IntegrationSymbols      []IntegrationSymbol        `json:"integration_symbols"`
	Paths                   []Path                     `json:"paths"`
	SurfaceCoreEffects      *SurfaceCoreEffectBindings `json:"surface_core_effects,omitempty"`
	Coverage                Coverage                   `json:"coverage"`
}

// Run executes the activity, entrypoint, dependency, and integration-use
// cubes and restores every model-selected short
// ref to exact local identities. Provider transport, JSON tolerance, caching,
// retries, and accounting stay in llm.Executor.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	coreCompilation coremap.Compilation,
	coreObjectIndex gocoreobject.Index,
	index surfacediscovery.DirectCallIndex,
	externalIndex surfacediscovery.ExternalCallIndex,
	activitySubstrate entrycall.Substrate,
	catalog dependencies.Catalog,
) (Map, error) {
	return run(ctx, executor, provider, coreCompilation, coreObjectIndex, index, externalIndex, activitySubstrate, catalog)
}

func Encode(value Map) ([]byte, error) {
	if err := Validate(value); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cubemap: encode: %w", err)
	}
	return append(encoded, '\n'), nil
}

func Decode(encoded []byte) (Map, error) {
	var value Map
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return Map{}, fmt.Errorf("cubemap: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Map{}, fmt.Errorf("cubemap: decode: trailing JSON value")
		}
		return Map{}, fmt.Errorf("cubemap: decode trailing data: %w", err)
	}
	if err := Validate(value); err != nil {
		return Map{}, err
	}
	return value, nil
}

func Validate(value Map) error {
	if value.Version != Version {
		return fmt.Errorf("cubemap: unsupported version %d", value.Version)
	}
	if !validSHA256(value.SourceIndexSHA256) || !validSHA256(value.ExternalCallIndexSHA256) ||
		!validSHA256(value.DependencyCatalogSHA256) {
		return fmt.Errorf("cubemap: invalid source identity")
	}
	if err := validateCoverage(value.Coverage); err != nil {
		return err
	}
	if err := value.ActivitySurfaces.Validate(); err != nil {
		return fmt.Errorf("cubemap: activity surfaces: %w", err)
	}
	if err := value.Core.Validate(); err != nil {
		return fmt.Errorf("cubemap: core map: %w", err)
	}
	if err := value.CoreObjects.Validate(); err != nil {
		return fmt.Errorf("cubemap: core objects: %w", err)
	}
	if value.Core.DirectCallSHA256 != value.SourceIndexSHA256 {
		return fmt.Errorf("cubemap: core map source identity mismatch")
	}
	coreBlockIDs := make(map[string]struct{}, len(value.Core.Refined))
	for _, block := range value.Core.Refined {
		coreBlockIDs[block.ID] = struct{}{}
	}
	for _, binding := range value.CoreObjects.Bindings {
		if _, exists := coreBlockIDs[binding.CoreBlockID]; !exists {
			return fmt.Errorf("cubemap: core object binding cites an unknown core block")
		}
	}
	entrypoints := make(map[string]struct{}, len(value.Entrypoints))
	for position, symbol := range value.Entrypoints {
		if err := validateSymbol(symbol); err != nil {
			return fmt.Errorf("cubemap: entrypoint %d: %w", position, err)
		}
		if _, exists := entrypoints[symbol.NodeID]; exists {
			return fmt.Errorf("cubemap: duplicate entrypoint node %q", symbol.NodeID)
		}
		entrypoints[symbol.NodeID] = struct{}{}
		if position > 0 && !symbolLess(value.Entrypoints[position-1], symbol) {
			return fmt.Errorf("cubemap: entrypoints are not canonical")
		}
	}
	dependenciesByID := make(map[string]IntegrationDependency, len(value.IntegrationDependencies))
	for position, dependency := range value.IntegrationDependencies {
		if err := validateIntegrationDependency(dependency); err != nil {
			return fmt.Errorf("cubemap: integration dependency %d: %w", position, err)
		}
		if _, exists := dependenciesByID[dependency.ID]; exists {
			return fmt.Errorf("cubemap: duplicate integration dependency %q", dependency.ID)
		}
		dependenciesByID[dependency.ID] = dependency
		if position > 0 && !integrationDependencyLess(value.IntegrationDependencies[position-1], dependency) {
			return fmt.Errorf("cubemap: integration dependencies are not canonical")
		}
	}
	integrationSymbols := make(map[string]struct{}, len(value.IntegrationSymbols))
	for position, symbol := range value.IntegrationSymbols {
		if err := validateSymbol(symbol.Symbol); err != nil {
			return fmt.Errorf("cubemap: integration symbol %d: %w", position, err)
		}
		if _, exists := integrationSymbols[symbol.Symbol.NodeID]; exists {
			return fmt.Errorf("cubemap: duplicate integration symbol %q", symbol.Symbol.NodeID)
		}
		integrationSymbols[symbol.Symbol.NodeID] = struct{}{}
		if len(symbol.DependencyIDs) == 0 || !sort.StringsAreSorted(symbol.DependencyIDs) || !uniqueStrings(symbol.DependencyIDs) {
			return fmt.Errorf("cubemap: integration symbol %q has invalid dependencies", symbol.Symbol.NodeID)
		}
		symbolDependencies := make(map[string]IntegrationDependency, len(symbol.DependencyIDs))
		for _, dependencyID := range symbol.DependencyIDs {
			dependency, exists := dependenciesByID[dependencyID]
			if !exists {
				return fmt.Errorf("cubemap: integration symbol %q has unknown dependency", symbol.Symbol.NodeID)
			}
			symbolDependencies[dependencyID] = dependency
		}
		if len(symbol.Operations) == 0 {
			return fmt.Errorf("cubemap: integration symbol %q has no exact operations", symbol.Symbol.NodeID)
		}
		operationDependencies := make(map[string]struct{}, len(symbol.DependencyIDs))
		for operationPosition, operation := range symbol.Operations {
			if err := validateIntegrationOperation(operation, symbolDependencies); err != nil {
				return fmt.Errorf("cubemap: integration symbol %q operation %d: %w", symbol.Symbol.NodeID, operationPosition, err)
			}
			if operationPosition > 0 && integrationOperationKey(symbol.Operations[operationPosition-1]) >= integrationOperationKey(operation) {
				return fmt.Errorf("cubemap: integration symbol %q operations are not canonical", symbol.Symbol.NodeID)
			}
			operationDependencies[operation.DependencyID] = struct{}{}
		}
		if len(operationDependencies) != len(symbol.DependencyIDs) {
			return fmt.Errorf("cubemap: integration symbol %q dependency claims do not match exact operations", symbol.Symbol.NodeID)
		}
		for _, dependencyID := range symbol.DependencyIDs {
			if _, exists := operationDependencies[dependencyID]; !exists {
				return fmt.Errorf("cubemap: integration symbol %q dependency claims do not match exact operations", symbol.Symbol.NodeID)
			}
		}
		if position > 0 && !integrationSymbolLess(value.IntegrationSymbols[position-1], symbol) {
			return fmt.Errorf("cubemap: integration symbols are not canonical")
		}
	}
	pathSymbols := make(map[string]struct{}, len(value.Paths))
	for position, path := range value.Paths {
		if _, exists := entrypoints[path.EntrypointNodeID]; !exists {
			return fmt.Errorf("cubemap: path %d has unknown entrypoint", position)
		}
		if _, exists := integrationSymbols[path.IntegrationNodeID]; !exists {
			return fmt.Errorf("cubemap: path %d has unknown integration symbol", position)
		}
		if len(path.Nodes) == 0 || path.Nodes[0].NodeID != path.EntrypointNodeID ||
			path.Nodes[len(path.Nodes)-1].NodeID != path.IntegrationNodeID {
			return fmt.Errorf("cubemap: path %d endpoints do not match nodes", position)
		}
		seenNodes := make(map[string]struct{}, len(path.Nodes))
		for _, symbol := range path.Nodes {
			if err := validateSymbol(symbol); err != nil {
				return fmt.Errorf("cubemap: path %d: %w", position, err)
			}
			if _, exists := seenNodes[symbol.NodeID]; exists {
				return fmt.Errorf("cubemap: path %d contains a cycle", position)
			}
			seenNodes[symbol.NodeID] = struct{}{}
		}
		if _, exists := pathSymbols[path.IntegrationNodeID]; exists {
			return fmt.Errorf("cubemap: duplicate path for integration symbol %q", path.IntegrationNodeID)
		}
		pathSymbols[path.IntegrationNodeID] = struct{}{}
		if position > 0 && value.Paths[position-1].IntegrationNodeID >= path.IntegrationNodeID {
			return fmt.Errorf("cubemap: paths are not canonical")
		}
	}
	if value.Coverage.UnconnectedSymbols != len(value.IntegrationSymbols)-len(value.Paths) {
		return fmt.Errorf("cubemap: unconnected symbol accounting mismatch")
	}
	if value.SurfaceCoreEffects != nil {
		if err := value.SurfaceCoreEffects.Validate(); err != nil {
			return fmt.Errorf("cubemap: surface/core/effect bindings: %w", err)
		}
		surfaces := make(map[string]struct{}, len(value.ActivitySurfaces.Surfaces))
		for _, surface := range value.ActivitySurfaces.Surfaces {
			surfaces[surface.ID] = struct{}{}
		}
		cores := make(map[string]struct{}, len(value.Core.Refined))
		for _, block := range value.Core.Refined {
			cores[block.ID] = struct{}{}
		}
		effects := make(map[string]struct{})
		for _, integration := range value.IntegrationSymbols {
			for _, operation := range integration.Operations {
				effects[operation.ExternalCallFamilyID+"\x00"+integration.Symbol.NodeID] = struct{}{}
			}
		}
		for _, binding := range value.SurfaceCoreEffects.SurfaceCore {
			if _, ok := surfaces[binding.SurfaceID]; !ok {
				return fmt.Errorf("cubemap: surface/core binding cites an unknown surface")
			}
			if _, ok := cores[binding.CoreBlockID]; !ok {
				return fmt.Errorf("cubemap: surface/core binding cites an unknown core block")
			}
		}
		for _, binding := range value.SurfaceCoreEffects.EffectCore {
			if _, ok := cores[binding.CoreBlockID]; !ok {
				return fmt.Errorf("cubemap: effect/core binding cites an unknown core block")
			}
			if _, ok := effects[binding.ExternalCallFamilyID+"\x00"+binding.CallerNodeID]; !ok {
				return fmt.Errorf("cubemap: effect/core binding cites an unknown external effect")
			}
		}
	}
	return nil
}

// ValidateAgainst restores the artifact's exact claims to the live local
// authorities. Validate checks the standalone shape; this check proves that
// every saved symbol and every adjacent path hop exists in the graph named by
// the artifact and that selected dependency rows come from its bound catalog.
func ValidateAgainst(
	value Map,
	coreCompilation coremap.Compilation,
	coreObjectIndex gocoreobject.Index,
	index surfacediscovery.DirectCallIndex,
	externalIndex surfacediscovery.ExternalCallIndex,
	activitySubstrate entrycall.Substrate,
	catalog dependencies.Catalog,
) error {
	if err := Validate(value); err != nil {
		return err
	}
	if err := value.Core.ValidateAgainst(coreCompilation); err != nil {
		return fmt.Errorf("cubemap: validate core map: %w", err)
	}
	if err := value.CoreObjects.ValidateAgainst(value.Core, coreObjectIndex); err != nil {
		return fmt.Errorf("cubemap: validate core objects: %w", err)
	}
	if err := index.Validate(); err != nil {
		return fmt.Errorf("cubemap: validate source index: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return fmt.Errorf("cubemap: validate dependency catalog: %w", err)
	}
	if err := externalIndex.Validate(); err != nil {
		return fmt.Errorf("cubemap: validate external-call index: %w", err)
	}
	if !sameScenario(index.Scenario, externalIndex.Scenario) {
		return fmt.Errorf("cubemap: call indexes describe different build scenarios")
	}
	if err := value.ActivitySurfaces.ValidateAgainst(activitySubstrate); err != nil {
		return fmt.Errorf("cubemap: validate activity surfaces: %w", err)
	}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("cubemap: encode dependency authority: %w", err)
	}
	if value.SourceIndexSHA256 != index.SHA256 || value.ExternalCallIndexSHA256 != externalIndex.SHA256 ||
		value.DependencyCatalogSHA256 != sha256Hex(catalogJSON) {
		return fmt.Errorf("cubemap: source authority mismatch")
	}
	if value.Coverage.ExternalCalls != externalIndex.Coverage {
		return fmt.Errorf("cubemap: external-call coverage authority mismatch")
	}
	if index.State == surfacediscovery.DirectCallIndexReady {
		if value.SurfaceCoreEffects == nil {
			return fmt.Errorf("cubemap: surface/core/effect bindings are unavailable for a ready exact graph")
		}
		bindingCompilation, compileErr := compileSurfaceCoreEffectBinder(
			value.Core, value.CoreObjects, value.ActivitySurfaces, value.IntegrationDependencies,
			value.IntegrationSymbols, index,
		)
		if compileErr != nil {
			return compileErr
		}
		if bindingErr := value.SurfaceCoreEffects.ValidateAgainst(bindingCompilation); bindingErr != nil {
			return fmt.Errorf("cubemap: validate surface/core/effect bindings: %w", bindingErr)
		}
	} else if value.SurfaceCoreEffects != nil {
		return fmt.Errorf("cubemap: surface/core/effect bindings require a ready exact graph")
	}
	for _, symbol := range value.Entrypoints {
		if err := validateExactGraphSymbol(index, symbol); err != nil {
			return err
		}
	}
	for _, integration := range value.IntegrationSymbols {
		if err := validateExactGraphSymbol(index, integration.Symbol); err != nil {
			return err
		}
		for _, operation := range integration.Operations {
			if err := validateExactExternalOperation(externalIndex, integration.Symbol.NodeID, operation); err != nil {
				return err
			}
		}
	}
	dependencyByID := make(map[string]dependencies.Dependency, len(catalog.Dependencies))
	for _, dependency := range catalog.Dependencies {
		dependencyByID[dependency.ID] = dependency
	}
	importerByRef := make(map[string]dependencies.Importer, len(catalog.Importers))
	for _, importer := range catalog.Importers {
		importerByRef[importer.Ref] = importer
	}
	for _, selected := range value.IntegrationDependencies {
		dependency, ok := dependencyByID[selected.ID]
		if !ok || dependency.Kind != selected.Kind ||
			(dependency.Kind != dependencies.KindExternal && dependency.Kind != dependencies.KindStdlib) ||
			dependency.Name != selected.Name ||
			dependency.ModulePath != selected.ModulePath || dependency.ModuleVersion != selected.ModuleVersion ||
			dependency.PackagePath != selected.PackagePath {
			return fmt.Errorf("cubemap: selected dependency authority mismatch")
		}
		expectedImporters := make([]Importer, 0, len(dependency.ImporterRefs))
		for _, ref := range dependency.ImporterRefs {
			importer, exists := importerByRef[ref]
			if !exists {
				return fmt.Errorf("cubemap: selected dependency importer is unavailable")
			}
			expectedImporters = append(expectedImporters, Importer{
				PackagePath: importer.PackagePath, RepositoryPath: importer.RepositoryPath,
			})
		}
		sort.Slice(expectedImporters, func(i, j int) bool {
			return importerKey(expectedImporters[i]) < importerKey(expectedImporters[j])
		})
		if len(expectedImporters) != len(selected.Importers) {
			return fmt.Errorf("cubemap: selected dependency importer authority mismatch")
		}
		for position := range expectedImporters {
			if expectedImporters[position] != selected.Importers[position] {
				return fmt.Errorf("cubemap: selected dependency importer authority mismatch")
			}
		}
	}
	for _, path := range value.Paths {
		for _, symbol := range path.Nodes {
			if err := validateExactGraphSymbol(index, symbol); err != nil {
				return err
			}
		}
		for position := 1; position < len(path.Nodes); position++ {
			callerID := path.Nodes[position-1].NodeID
			calleeID := path.Nodes[position].NodeID
			found := false
			for _, edge := range index.Outgoing(callerID) {
				if edge.CalleeID == calleeID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("cubemap: path contains a non-existent direct-call hop")
			}
		}
	}
	return nil
}

func validateExactGraphSymbol(index surfacediscovery.DirectCallIndex, symbol Symbol) error {
	node, ok := index.Node(symbol.NodeID)
	if !ok || symbolFromNode(node) != symbol {
		return fmt.Errorf("cubemap: exact symbol authority mismatch")
	}
	return nil
}

func validateCoverage(coverage Coverage) error {
	if err := validateDependencyCatalogCoverage(coverage.DependencyCatalog); err != nil {
		return err
	}
	for _, item := range []CandidateCoverage{
		coverage.Entrypoints, coverage.IntegrationDependencies, coverage.IntegrationSymbols,
	} {
		if item.Observed < 0 || item.Advertised < 0 || item.Omitted < 0 ||
			item.Advertised > item.Observed || item.Omitted != item.Observed-item.Advertised {
			return fmt.Errorf("cubemap: invalid candidate coverage")
		}
		if item.ModelCalled && item.Advertised == 0 {
			return fmt.Errorf("cubemap: model called with an empty catalog")
		}
		if item.Omitted != 0 {
			return fmt.Errorf("cubemap: incomplete semantic candidate coverage")
		}
	}
	if coverage.UnconnectedSymbols < 0 {
		return fmt.Errorf("cubemap: invalid unconnected symbol count")
	}
	if coverage.GoFilesObserved < 0 || coverage.GoFilesParsed < 0 || coverage.GoFilesSkipped < 0 ||
		coverage.GoFilesParsed+coverage.GoFilesSkipped != coverage.GoFilesObserved {
		return fmt.Errorf("cubemap: invalid Go file coverage")
	}
	if coverage.ExternalCallFamiliesObserved < 0 || coverage.ExternalCallFamiliesMatched < 0 ||
		coverage.ExternalCallFamiliesMatched > coverage.ExternalCallFamiliesObserved {
		return fmt.Errorf("cubemap: invalid external-call coverage")
	}
	externalCounts := []int{
		coverage.ExternalCalls.PackagesIndexed, coverage.ExternalCalls.CallersIndexed,
		coverage.ExternalCalls.FamiliesIndexed, coverage.ExternalCalls.ExternalStaticWitnesses,
		coverage.ExternalCalls.ExternalInterfaceInvokeWitnesses,
		coverage.ExternalCalls.RepresentativeCallsites, coverage.ExternalCalls.RepresentativeCallsitesOmitted,
		coverage.ExternalCalls.DynamicInvokesExcluded, coverage.ExternalCalls.NonStaticCallsExcluded,
		coverage.ExternalCalls.UnnamedStaticCalleesExcluded, coverage.ExternalCalls.InvalidCallsitesExcluded,
		coverage.ExternalCalls.SyntheticCallerWitnessesExcluded, coverage.ExternalCalls.InvalidCallerWitnessesExcluded,
	}
	for _, count := range externalCounts {
		if count < 0 {
			return fmt.Errorf("cubemap: invalid external-call frontier coverage")
		}
	}
	if coverage.ExternalCalls.FamiliesIndexed != coverage.ExternalCallFamiliesObserved {
		return fmt.Errorf("cubemap: external-call family coverage mismatch")
	}
	return nil
}

func validateDependencyCatalogCoverage(coverage DependencyCatalogCoverage) error {
	if coverage.ImportsObserved < 0 || coverage.ImportsRetained < 0 || coverage.Omissions < 0 ||
		coverage.ImportsObserved != coverage.ImportsRetained+coverage.Omissions {
		return fmt.Errorf("cubemap: invalid dependency catalog coverage")
	}
	sum := 0
	previous := dependencies.OmissionReason("")
	for _, reason := range coverage.Reasons {
		if reason.Count < 1 || !validDependencyOmissionReason(reason.Reason) ||
			(previous != "" && previous >= reason.Reason) {
			return fmt.Errorf("cubemap: invalid dependency omission coverage")
		}
		previous = reason.Reason
		sum += reason.Count
	}
	if sum != coverage.Omissions {
		return fmt.Errorf("cubemap: dependency omission reasons do not match coverage")
	}
	switch coverage.State {
	case dependencies.CoverageComplete:
		if coverage.Omissions != 0 || len(coverage.Reasons) != 0 {
			return fmt.Errorf("cubemap: complete dependency coverage has omissions")
		}
	default:
		return fmt.Errorf("cubemap: dependency coverage is not complete")
	}
	return nil
}

func validDependencyOmissionReason(reason dependencies.OmissionReason) bool {
	switch reason {
	case dependencies.OmissionImporterIdentityUnavailable,
		dependencies.OmissionDependencyMetadataMissing,
		dependencies.OmissionDependencyLoadUnavailable,
		dependencies.OmissionDependencyIdentityMissing,
		dependencies.OmissionModuleAuthorityMissing:
		return true
	default:
		return false
	}
}

func validateIntegrationOperation(value IntegrationOperation, dependenciesByID map[string]IntegrationDependency) error {
	if strings.TrimSpace(value.ExternalCallFamilyID) == "" ||
		strings.TrimSpace(value.DependencyID) == "" || strings.TrimSpace(value.PackagePath) == "" ||
		strings.TrimSpace(value.Name) == "" || !value.Dispatch.Valid() || !value.Invocation.Valid() || value.WitnessCount < 1 ||
		value.CallsitesOmitted != value.WitnessCount-len(value.Callsites) || value.CallsitesOmitted < 0 {
		return fmt.Errorf("invalid exact integration operation")
	}
	dependency, exists := dependenciesByID[value.DependencyID]
	if !exists {
		return fmt.Errorf("unknown integration dependency")
	}
	if dependency.PackagePath != value.PackagePath {
		return fmt.Errorf("integration operation package does not match its dependency")
	}
	for position, callsite := range value.Callsites {
		if !validRepositoryPath(callsite.Path) || callsite.Line < 1 || callsite.Column < 0 {
			return fmt.Errorf("invalid integration callsite")
		}
		if position > 0 && locationKey(value.Callsites[position-1]) >= locationKey(callsite) {
			return fmt.Errorf("integration callsites are not canonical")
		}
	}
	return nil
}

func validateExactExternalOperation(
	index surfacediscovery.ExternalCallIndex,
	callerID string,
	operation IntegrationOperation,
) error {
	for _, family := range index.Families {
		if family.ID != operation.ExternalCallFamilyID {
			continue
		}
		if family.CallerID != callerID || family.Target.PackagePath != operation.PackagePath ||
			family.Target.Receiver != operation.Receiver || family.Target.Name != operation.Name ||
			family.Dispatch != operation.Dispatch ||
			family.Invocation != operation.Invocation || family.WitnessCount != operation.WitnessCount ||
			family.CallsitesOmitted != operation.CallsitesOmitted || len(family.Callsites) != len(operation.Callsites) {
			return fmt.Errorf("cubemap: exact external-call authority mismatch")
		}
		for position, callsite := range family.Callsites {
			if locationFromSurface(callsite) != operation.Callsites[position] {
				return fmt.Errorf("cubemap: exact external-call authority mismatch")
			}
		}
		return nil
	}
	return fmt.Errorf("cubemap: exact external-call family is unavailable")
}

func validateSymbol(symbol Symbol) error {
	if strings.TrimSpace(symbol.NodeID) == "" || strings.TrimSpace(symbol.Package) == "" ||
		strings.TrimSpace(symbol.Name) == "" || !validRepositoryPath(symbol.Path) || symbol.Line < 1 || symbol.Column < 0 {
		return fmt.Errorf("invalid exact symbol")
	}
	return nil
}

func validateIntegrationDependency(value IntegrationDependency) error {
	if strings.TrimSpace(value.ID) == "" ||
		(value.Kind != dependencies.KindExternal && value.Kind != dependencies.KindStdlib) ||
		strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.PackagePath) == "" {
		return fmt.Errorf("invalid dependency")
	}
	for position, importer := range value.Importers {
		if strings.TrimSpace(importer.PackagePath) == "" || !validRepositoryDirectory(importer.RepositoryPath) {
			return fmt.Errorf("invalid importer")
		}
		if position > 0 && importerKey(value.Importers[position-1]) >= importerKey(importer) {
			return fmt.Errorf("importers are not canonical")
		}
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validRepositoryPath(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validRepositoryDirectory(value string) bool {
	if value == "." {
		return true
	}
	return validRepositoryPath(value)
}

func symbolLess(left, right Symbol) bool { return symbolKey(left) < symbolKey(right) }

func symbolKey(value Symbol) string {
	return value.Path + "\x00" + fmt.Sprintf("%010d", value.Line) + "\x00" + value.Package + "\x00" + value.Name + "\x00" + value.NodeID
}

func integrationDependencyLess(left, right IntegrationDependency) bool {
	return left.PackagePath+"\x00"+left.ModulePath+"\x00"+left.ID < right.PackagePath+"\x00"+right.ModulePath+"\x00"+right.ID
}

func integrationSymbolLess(left, right IntegrationSymbol) bool {
	return symbolLess(left.Symbol, right.Symbol)
}

func integrationOperationKey(value IntegrationOperation) string {
	return value.DependencyID + "\x00" + value.PackagePath + "\x00" + value.Receiver + "\x00" + value.Name + "\x00" +
		string(value.Dispatch) + "\x00" + string(value.Invocation) + "\x00" + value.ExternalCallFamilyID
}

func locationKey(value Location) string {
	return value.Path + "\x00" + fmt.Sprintf("%010d", value.Line) + "\x00" + fmt.Sprintf("%010d", value.Column)
}

func locationFromSurface(value surfacediscovery.Location) Location {
	return Location{Path: value.Path, Line: value.Line, Column: value.Column}
}

func importerKey(value Importer) string { return value.PackagePath + "\x00" + value.RepositoryPath }

func uniqueStrings(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] == values[index] {
			return false
		}
	}
	return true
}

func sameScenario(left, right surfacediscovery.Scenario) bool {
	if left.ID != right.ID || left.GOOS != right.GOOS || left.GOARCH != right.GOARCH ||
		left.GoFlags != right.GoFlags || len(left.Tags) != len(right.Tags) {
		return false
	}
	for position := range left.Tags {
		if left.Tags[position] != right.Tags[position] {
			return false
		}
	}
	return true
}
