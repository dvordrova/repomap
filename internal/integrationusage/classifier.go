// Package integrationusage selects concrete product integration and side-effect
// uses from adapter-owned external-operation candidates. The shared cube owns
// model selection and restoration; language adapters alone interpret their
// exact ProgramIndex relation, witness, external-symbol, and importer facts.
package integrationusage

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version = 6

	MaxAdvertisedOperationsPerRequest = MaxSelectedUsesPerRequest
	MaxSelectedUsesPerRequest         = 256
	MaxLabelBytes                     = 120
	MaxMechanismBytes                 = 120

	maxRequestBytes  = 4 * 1024 * 1024
	maxResponseBytes = 256 * 1024
	maxOutputTokens  = 8192
)

const (
	AuthoritySyntacticUnresolved = "syntactic_unresolved"
	AuthorityExactExternalSymbol = "exact_external_symbol"
	pythonCallsiteCandidate      = "callsite_candidate"
)

//go:embed prompts/classifier.md
var prompt string

// Operation is locally restored evidence for one advertised external-call
// witness. Authority states whether the adapter proved the external symbol or
// retained only a syntactic candidate; neither value invents product meaning.
type Operation struct {
	Language         string                  `json:"language"`
	DependencyID     string                  `json:"dependency_id"`
	RelationID       string                  `json:"relation_id"`
	WitnessIndex     int                     `json:"witness_index"`
	CallerID         string                  `json:"caller_id"`
	CallerKind       programindex.ObjectKind `json:"caller_kind"`
	CallerName       string                  `json:"caller_name"`
	CallerLocation   programindex.Location   `json:"caller_location"`
	Callsite         programindex.Location   `json:"callsite"`
	CallExpression   string                  `json:"call_expression"`
	CanonicalCallee  string                  `json:"canonical_callee"`
	ExternalSymbolID string                  `json:"external_symbol_id"`
	Invocation       string                  `json:"invocation,omitempty"`
	Authority        string                  `json:"authority"`
}

// Use is one model-selected operation with presentation-ready semantics. The
// exact operation is restored locally; the model supplies only Label and the
// deliberately free, possibly "unknown", interaction Mechanism.
type Use struct {
	Operation Operation `json:"operation"`
	Label     string    `json:"label"`
	Mechanism string    `json:"mechanism"`
}

// Coverage partitions every RelationInvokesExternal witness observed by the
// ProgramIndex. Retained exact/syntactic operations may be advertised;
// adapter-recorded unresolved or representative-sampling frontiers remain
// explicit in CallsiteCandidatesOmitted and are never promoted by the cube.
type Coverage struct {
	DependenciesObserved       int  `json:"dependencies_observed"`
	DependenciesWithOperations int  `json:"dependencies_with_operations"`
	ExternalRelationsObserved  int  `json:"external_relations_observed"`
	CallsiteCandidatesObserved int  `json:"callsite_candidates_observed"`
	CallsiteCandidatesOmitted  int  `json:"callsite_candidates_omitted"`
	OperationsAdvertised       int  `json:"operations_advertised"`
	OutOfScopeCandidates       int  `json:"out_of_scope_candidates"`
	ExactExternalRelations     int  `json:"exact_external_relations"`
	UnresolvedRuntimeRelations int  `json:"unresolved_runtime_relations"`
	Selected                   int  `json:"selected"`
	ModelCalled                bool `json:"model_called"`
}

// Result is the canonical standalone integration-usage artifact. It binds both
// material inputs by their canonical SHA-256 identities.
type Result struct {
	Version                       int      `json:"version"`
	ProgramIndexSHA256            string   `json:"program_index_sha256"`
	IntegrationDependenciesSHA256 string   `json:"integration_dependencies_sha256"`
	Uses                          []Use    `json:"uses"`
	Coverage                      Coverage `json:"coverage"`
}

type wireDependency struct {
	Ref         string            `json:"ref"`
	Kind        dependencies.Kind `json:"kind"`
	Name        string            `json:"name"`
	ModulePath  string            `json:"module_path,omitempty"`
	PackagePath string            `json:"package_path"`
}

type wireCaller struct {
	Kind     programindex.ObjectKind `json:"kind"`
	Name     string                  `json:"name"`
	Location programindex.Location   `json:"location"`
}

type wireOperation struct {
	Ref             string                `json:"ref"`
	DependencyRef   string                `json:"dependency_ref"`
	Caller          wireCaller            `json:"caller"`
	Callsite        programindex.Location `json:"callsite"`
	CallExpression  string                `json:"call_expression"`
	CanonicalCallee string                `json:"canonical_callee"`
	Invocation      string                `json:"invocation"`
	Authority       string                `json:"authority"`
}

type wireTarget struct {
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Selector string `json:"selector"`
}

type request struct {
	BatchIndex   int              `json:"batch_index"`
	BatchCount   int              `json:"batch_count"`
	Observed     int              `json:"observed"`
	Omitted      int              `json:"omitted"`
	Target       wireTarget       `json:"target"`
	Dependencies []wireDependency `json:"dependencies"`
	Operations   []wireOperation  `json:"operations"`
}

type response struct {
	Uses []wireUse `json:"uses"`
}

type wireUse struct {
	OperationRef string `json:"operation_ref"`
	Label        string `json:"label"`
	Mechanism    string `json:"mechanism"`
}

type dependencyCandidate struct {
	ref      string
	selected integrationdependency.SelectedDependency
	keys     []string
}

type operationCandidate struct {
	ref           string
	dependencyRef string
	operation     Operation
}

// Run prepares a complete disjoint request partition over every operation
// attributable to the selected dependencies. An input omission, unsupported
// witness, ambiguous association, or batch failure fails closed.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	index programindex.Index,
	selected integrationdependency.Result,
) (Result, error) {
	dependenciesSHA256, candidates, coverage, err := prepare(index, selected)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Version: Version, ProgramIndexSHA256: index.SHA256,
		IntegrationDependenciesSHA256: dependenciesSHA256,
		Uses:                          []Use{}, Coverage: coverage,
	}
	if len(candidates.operations) == 0 {
		if err := result.ValidateAgainst(index, selected); err != nil {
			return Result{}, err
		}
		return result, nil
	}

	totalBatches := completeBatchCount(len(candidates.operations), MaxAdvertisedOperationsPerRequest)
	calls := make([]llm.Call[response], 0, totalBatches)
	allowedByBatch := make([]map[string]operationCandidate, 0, totalBatches)
	for batchIndex, start := 0, 0; start < len(candidates.operations); batchIndex, start = batchIndex+1, start+MaxAdvertisedOperationsPerRequest {
		end := min(start+MaxAdvertisedOperationsPerRequest, len(candidates.operations))
		operations := candidates.operations[start:end]
		dependencies := dependenciesForOperations(candidates.dependencies, operations)
		batch := preparedCandidates{dependencies: dependencies, operations: operations}
		payload := request{
			BatchIndex: batchIndex + 1, BatchCount: totalBatches,
			Observed: len(candidates.operations), Omitted: 0,
			Target: wireTarget{
				Language: index.Target.Language, Kind: index.Target.Kind,
				Name: index.Target.Name, Selector: index.Target.Selector,
			},
			Dependencies: make([]wireDependency, 0, len(dependencies)),
			Operations:   make([]wireOperation, 0, len(operations)),
		}
		for _, candidate := range dependencies {
			dependency := candidate.selected.Dependency
			payload.Dependencies = append(payload.Dependencies, wireDependency{
				Ref: candidate.ref, Kind: dependency.Kind, Name: dependency.Name,
				ModulePath: dependency.ModulePath, PackagePath: dependency.PackagePath,
			})
		}
		allowed := make(map[string]operationCandidate, len(operations))
		for _, candidate := range operations {
			operation := candidate.operation
			payload.Operations = append(payload.Operations, wireOperation{
				Ref: candidate.ref, DependencyRef: candidate.dependencyRef,
				Caller: wireCaller{
					Kind: operation.CallerKind, Name: operation.CallerName,
					Location: operation.CallerLocation,
				},
				Callsite: operation.Callsite, CallExpression: operation.CallExpression,
				CanonicalCallee: operation.CanonicalCallee, Invocation: operation.Invocation,
				Authority: operation.Authority,
			})
			allowed[candidate.ref] = candidate
		}
		user, err := json.Marshal(payload)
		if err != nil {
			return Result{}, fmt.Errorf("integration usage: encode request batch %d: %w", batchIndex+1, err)
		}
		state, err := classifierState(
			index.SHA256, dependenciesSHA256, batch,
			batchIndex+1, totalBatches, len(candidates.operations),
		)
		if err != nil {
			return Result{}, fmt.Errorf("integration usage: state batch %d: %w", batchIndex+1, err)
		}
		batchAllowed := allowed
		allowedByBatch = append(allowedByBatch, batchAllowed)
		calls = append(calls, llm.Call[response]{
			State: state,
			Prompt: llm.Prompt{
				System: strings.TrimSpace(prompt), User: string(user), ResponseFormatJSON: true,
			},
			Limits: llm.Limits{
				MaxRequestBytes: maxRequestBytes, MaxResponseBytes: maxResponseBytes,
				MaxOutputTokens: maxOutputTokens,
			},
			Validate: func(value response) error { return validateResponse(value, batchAllowed) },
		})
	}
	outcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
	if err != nil {
		return Result{}, fmt.Errorf("integration usage: model cube: %w", err)
	}
	if len(outcomes) != len(allowedByBatch) {
		return Result{}, fmt.Errorf("integration usage: model cube returned an incomplete batch outcome set")
	}
	selectedByRef := make(map[string]wireUse)
	for batchIndex, outcome := range outcomes {
		for _, value := range outcome.Value.Uses {
			if _, known := allowedByBatch[batchIndex][value.OperationRef]; !known {
				continue
			}
			selectedByRef[value.OperationRef] = value
		}
	}
	for _, candidate := range candidates.operations {
		semantic, ok := selectedByRef[candidate.ref]
		if !ok {
			continue
		}
		result.Uses = append(result.Uses, Use{
			Operation: candidate.operation, Label: semantic.Label, Mechanism: semantic.Mechanism,
		})
	}
	result.Coverage.Selected = len(result.Uses)
	if err := result.ValidateAgainst(index, selected); err != nil {
		return Result{}, err
	}
	return result, nil
}

type preparedCandidates struct {
	dependencies []dependencyCandidate
	operations   []operationCandidate
}

func dependenciesForOperations(
	dependencies []dependencyCandidate,
	operations []operationCandidate,
) []dependencyCandidate {
	needed := make(map[string]struct{})
	for _, operation := range operations {
		needed[operation.dependencyRef] = struct{}{}
	}
	result := make([]dependencyCandidate, 0, len(needed))
	for _, dependency := range dependencies {
		if _, ok := needed[dependency.ref]; ok {
			result = append(result, dependency)
		}
	}
	return result
}

func prepare(
	index programindex.Index,
	selected integrationdependency.Result,
) (string, preparedCandidates, Coverage, error) {
	switch index.Target.Language {
	case "python":
		return preparePython(index, selected)
	case "go":
		return prepareGo(index, selected)
	case "javascript", "typescript":
		return prepareJavaScriptTypeScript(index, selected)
	default:
		return "", preparedCandidates{}, Coverage{}, fmt.Errorf(
			"integration usage: no operation adapter for ProgramIndex language %q", index.Target.Language,
		)
	}
}

func preparePython(
	index programindex.Index,
	selected integrationdependency.Result,
) (string, preparedCandidates, Coverage, error) {
	var empty preparedCandidates
	if err := index.Validate(); err != nil {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: ProgramIndex: %w", err)
	}
	if index.Target.Language != "python" {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: ProgramIndex target language is %q, want python", index.Target.Language,
		)
	}
	if index.Coverage.ObjectsOmitted != 0 || index.Coverage.RelationsOmitted != 0 ||
		index.Coverage.WitnessesOmitted != 0 {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: ProgramIndex authority is incomplete (%d objects, %d relations, %d witnesses omitted)",
			index.Coverage.ObjectsOmitted, index.Coverage.RelationsOmitted,
			index.Coverage.WitnessesOmitted,
		)
	}
	if err := selected.Validate(); err != nil {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: integration dependencies: %w", err)
	}
	if selected.Coverage.Omitted != 0 {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: integration dependency authority omitted %d candidates",
			selected.Coverage.Omitted,
		)
	}
	dependenciesSHA256, err := selected.ArtifactSHA256()
	if err != nil {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: integration dependency identity: %w", err)
	}

	dependencyCandidates := make([]dependencyCandidate, 0, len(selected.Dependencies))
	for position, value := range selected.Dependencies {
		dependency := value.Dependency
		if dependency.Language != "python" {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: selected dependency %q has language %q", dependency.ID, dependency.Language,
			)
		}
		keys, err := dependencyKeys(dependency)
		if err != nil {
			return "", empty, Coverage{}, err
		}
		for _, importer := range value.Importers {
			if importer.Language != "python" {
				return "", empty, Coverage{}, fmt.Errorf(
					"integration usage: selected dependency %q has non-Python importer", dependency.ID,
				)
			}
		}
		dependencyCandidates = append(dependencyCandidates, dependencyCandidate{
			ref: fmt.Sprintf("d%d", position+1), selected: cloneSelectedDependency(value), keys: keys,
		})
	}

	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	resolver := moduleResolver{objects: objects}
	coverage := Coverage{DependenciesObserved: len(dependencyCandidates)}
	operations := make([]operationCandidate, 0)
	dependenciesWithOperations := make(map[string]struct{})
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		coverage.ExternalRelationsObserved++
		if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 ||
			relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: external relation %q is not one retained Python runtime candidate", relation.ID,
			)
		}
		coverage.UnresolvedRuntimeRelations++
		externalSymbol, ok := objects[relation.ToIDs[0]]
		if !ok || externalSymbol.Kind != programindex.ObjectExternalSymbol {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: external relation %q has no exact external-symbol candidate", relation.ID,
			)
		}
		if relation.Location == nil || relation.WitnessesOmitted != 0 || len(relation.Witnesses) == 0 ||
			relation.WitnessesObserved != len(relation.Witnesses) {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: external relation %q has incomplete callsite authority", relation.ID,
			)
		}
		caller, ok := objects[relation.FromID]
		if !ok || caller.Location == nil || caller.Name == "" {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: external relation %q has no exact caller declaration", relation.ID,
			)
		}
		module, err := resolver.moduleFor(caller.ID)
		if err != nil {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: external relation %q caller: %w", relation.ID, err,
			)
		}
		if !validPythonImportKey(externalSymbol.Name) {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: external relation %q has an unsupported canonical target", relation.ID,
			)
		}
		for witnessIndex, witness := range relation.Witnesses {
			coverage.CallsiteCandidatesObserved++
			if witness.Kind != pythonCallsiteCandidate || witness.Location == nil ||
				!sameSourceLine(*witness.Location, *relation.Location) ||
				!validDottedExpression(witness.SourceExpression) {
				return "", empty, Coverage{}, fmt.Errorf(
					"integration usage: external relation %q has unsupported witness %d", relation.ID, witnessIndex,
				)
			}
			dependency, matched, err := matchDependency(externalSymbol.Name, module, dependencyCandidates)
			if err != nil {
				return "", empty, Coverage{}, fmt.Errorf(
					"integration usage: external relation %q witness %d: %w", relation.ID, witnessIndex, err,
				)
			}
			if !matched {
				coverage.OutOfScopeCandidates++
				continue
			}
			operation := Operation{
				Language:     "python",
				DependencyID: dependency.selected.Dependency.ID, RelationID: relation.ID,
				WitnessIndex: witnessIndex, CallerID: caller.ID, CallerKind: caller.Kind,
				CallerName: caller.Name, CallerLocation: *caller.Location,
				Callsite: *witness.Location, CallExpression: witness.SourceExpression,
				CanonicalCallee: externalSymbol.Name, ExternalSymbolID: externalSymbol.ID,
				Invocation: relation.Invocation, Authority: AuthoritySyntacticUnresolved,
			}
			operations = append(operations, operationCandidate{
				dependencyRef: dependency.ref, operation: operation,
			})
			dependenciesWithOperations[dependency.selected.Dependency.ID] = struct{}{}
		}
	}
	if coverage.CallsiteCandidatesObserved != len(operations)+coverage.OutOfScopeCandidates+
		coverage.CallsiteCandidatesOmitted {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: callsite coverage is not a complete partition")
	}
	sort.Slice(operations, func(left, right int) bool {
		return operationLess(operations[left].operation, operations[right].operation)
	})
	for position := range operations {
		operations[position].ref = fmt.Sprintf("o%d", position+1)
	}
	coverage.DependenciesWithOperations = len(dependenciesWithOperations)
	coverage.OperationsAdvertised = len(operations)
	coverage.ModelCalled = len(operations) > 0
	return dependenciesSHA256, preparedCandidates{
		dependencies: dependencyCandidates, operations: operations,
	}, coverage, nil
}

func (result Result) Validate() error {
	if result.Version != Version || !validSHA256(result.ProgramIndexSHA256) ||
		!validSHA256(result.IntegrationDependenciesSHA256) || result.Uses == nil {
		return fmt.Errorf("integration usage: invalid result identity")
	}
	if err := validateCoverage(result.Coverage); err != nil {
		return err
	}
	if len(result.Uses) != result.Coverage.Selected {
		return fmt.Errorf("integration usage: selected use count is outside authority")
	}
	for position, use := range result.Uses {
		if err := validateOperation(use.Operation); err != nil {
			return fmt.Errorf("integration usage: use %d: %w", position, err)
		}
		if !validBoundedLine(use.Label, MaxLabelBytes) ||
			!validBoundedLine(use.Mechanism, MaxMechanismBytes) {
			return fmt.Errorf("integration usage: use %d has invalid model semantics", position)
		}
		if position > 0 && !operationLess(result.Uses[position-1].Operation, use.Operation) {
			return fmt.Errorf("integration usage: uses are not canonical or contain duplicates")
		}
	}
	return nil
}

// ValidateAgainst reconstructs the complete candidate catalog from both exact
// inputs and proves every retained operation, coverage count, and input digest.
func (result Result) ValidateAgainst(
	index programindex.Index,
	selected integrationdependency.Result,
) error {
	if err := result.Validate(); err != nil {
		return err
	}
	dependenciesSHA256, candidates, coverage, err := prepare(index, selected)
	if err != nil {
		return err
	}
	coverage.Selected = len(result.Uses)
	if result.ProgramIndexSHA256 != index.SHA256 ||
		result.IntegrationDependenciesSHA256 != dependenciesSHA256 ||
		result.Coverage != coverage {
		return fmt.Errorf("integration usage: result authority mismatch")
	}
	exactByKey := make(map[string]Operation, len(candidates.operations))
	for _, candidate := range candidates.operations {
		exactByKey[operationKey(candidate.operation)] = candidate.operation
	}
	for _, use := range result.Uses {
		exact, ok := exactByKey[operationKey(use.Operation)]
		if !ok || !reflect.DeepEqual(exact, use.Operation) {
			return fmt.Errorf("integration usage: selected operation authority mismatch")
		}
	}
	return nil
}

func validateResponse(value response, allowed map[string]operationCandidate) error {
	if value.Uses == nil {
		return fmt.Errorf("integration usage: selected use count is outside bounds")
	}
	seen := make(map[string]wireUse, len(value.Uses))
	for _, use := range value.Uses {
		if _, ok := allowed[use.OperationRef]; !ok {
			continue
		}
		if !validBoundedLine(use.Label, MaxLabelBytes) ||
			!validBoundedLine(use.Mechanism, MaxMechanismBytes) {
			return fmt.Errorf("integration usage: invalid label or mechanism for %q", use.OperationRef)
		}
		if previous, duplicate := seen[use.OperationRef]; duplicate {
			if previous != use {
				return fmt.Errorf("integration usage: conflicting assignment for request-local ref %q", use.OperationRef)
			}
			continue
		}
		seen[use.OperationRef] = use
	}
	if len(seen) > MaxSelectedUsesPerRequest {
		return fmt.Errorf("integration usage: selected use count is outside bounds")
	}
	return nil
}

func validateCoverage(value Coverage) error {
	if value.DependenciesObserved < 0 || value.DependenciesWithOperations < 0 ||
		value.DependenciesWithOperations > value.DependenciesObserved ||
		value.ExternalRelationsObserved < 0 || value.CallsiteCandidatesObserved < 0 ||
		value.OperationsAdvertised < 0 ||
		value.CallsiteCandidatesOmitted < 0 || value.OutOfScopeCandidates < 0 ||
		value.OperationsAdvertised > value.CallsiteCandidatesObserved ||
		value.OutOfScopeCandidates > value.CallsiteCandidatesObserved-value.OperationsAdvertised ||
		value.CallsiteCandidatesOmitted != value.CallsiteCandidatesObserved-
			value.OperationsAdvertised-value.OutOfScopeCandidates ||
		value.ExactExternalRelations < 0 || value.UnresolvedRuntimeRelations < 0 ||
		value.ExactExternalRelations > value.ExternalRelationsObserved ||
		value.UnresolvedRuntimeRelations != value.ExternalRelationsObserved-value.ExactExternalRelations ||
		value.Selected < 0 ||
		value.Selected > value.OperationsAdvertised ||
		value.ModelCalled != (value.OperationsAdvertised > 0) {
		return fmt.Errorf("integration usage: invalid coverage")
	}
	return nil
}

func validateOperation(value Operation) error {
	if value.Language == "" || value.DependencyID == "" || value.RelationID == "" || value.WitnessIndex < 0 ||
		value.CallerID == "" || !value.CallerKind.Valid() || value.CallerName == "" ||
		value.ExternalSymbolID == "" ||
		!validLocation(value.CallerLocation) || !validLocation(value.Callsite) ||
		!validBoundedText(value.CallerName, programindex.MaxTextBytes) ||
		!validBoundedText(value.CanonicalCallee, programindex.MaxTextBytes) ||
		!validOptionalLine(value.Invocation, programindex.MaxTextBytes) {
		return fmt.Errorf("invalid exact operation")
	}
	switch value.Language {
	case "python":
		if value.Authority != AuthoritySyntacticUnresolved ||
			!validDottedExpression(value.CallExpression) || !validPythonImportKey(value.CanonicalCallee) {
			return fmt.Errorf("invalid Python syntactic operation")
		}
	case "go":
		if value.Authority != AuthorityExactExternalSymbol || value.CallExpression != "" {
			return fmt.Errorf("invalid Go exact operation")
		}
	case "javascript":
		if value.Authority != AuthoritySyntacticUnresolved ||
			!validOptionalLine(value.CallExpression, programindex.MaxTextBytes) {
			return fmt.Errorf("invalid JavaScript operation")
		}
	case "typescript":
		if (value.Authority != AuthorityExactExternalSymbol &&
			value.Authority != AuthoritySyntacticUnresolved) ||
			!validOptionalLine(value.CallExpression, programindex.MaxTextBytes) {
			return fmt.Errorf("invalid TypeScript operation")
		}
	default:
		return fmt.Errorf("unsupported operation language %q", value.Language)
	}
	return nil
}

func matchDependency(
	canonical string,
	module programindex.Object,
	candidates []dependencyCandidate,
) (dependencyCandidate, bool, error) {
	bestLength := -1
	matches := make(map[string]dependencyCandidate)
	for _, candidate := range candidates {
		matchedKeyLength := -1
		for _, key := range candidate.keys {
			if canonical == key || strings.HasPrefix(canonical, key+".") {
				if len(key) > matchedKeyLength {
					matchedKeyLength = len(key)
				}
			}
		}
		if matchedKeyLength < 0 || !moduleMatchesImporters(module, candidate.selected.Importers) {
			continue
		}
		if matchedKeyLength > bestLength {
			bestLength = matchedKeyLength
			matches = map[string]dependencyCandidate{
				candidate.selected.Dependency.ID: candidate,
			}
		} else if matchedKeyLength == bestLength {
			matches[candidate.selected.Dependency.ID] = candidate
		}
	}
	if len(matches) == 0 {
		// A selected top-level package and an unselected submodule may share a
		// lexical prefix. Without an exact importer binding for this caller the
		// call is out of scope for the selected dependency, not corrupt input.
		return dependencyCandidate{}, false, nil
	}
	if len(matches) != 1 {
		return dependencyCandidate{}, false, fmt.Errorf(
			"canonical candidate %q has ambiguous longest-prefix dependency authority", canonical,
		)
	}
	for _, candidate := range matches {
		return candidate, true, nil
	}
	panic("unreachable")
}

func moduleMatchesImporters(module programindex.Object, importers []dependencies.Importer) bool {
	if module.Location == nil {
		return false
	}
	for _, importer := range importers {
		if importer.Language == "python" && importer.Name == module.Name &&
			importer.ModulePath == firstPythonNamePart(module.Name) &&
			importer.PackagePath == module.Name && importer.RepositoryPath == path.Dir(module.Location.Path) {
			return true
		}
	}
	return false
}

func firstPythonNamePart(value string) string {
	if position := strings.IndexByte(value, '.'); position >= 0 {
		return value[:position]
	}
	return value
}

type moduleResolver struct {
	objects map[string]programindex.Object
}

func (resolver moduleResolver) moduleFor(objectID string) (programindex.Object, error) {
	seen := make(map[string]struct{})
	for objectID != "" {
		if _, duplicate := seen[objectID]; duplicate {
			return programindex.Object{}, fmt.Errorf("object containment contains a cycle")
		}
		seen[objectID] = struct{}{}
		object, ok := resolver.objects[objectID]
		if !ok {
			return programindex.Object{}, fmt.Errorf("unknown object %q", objectID)
		}
		if object.Kind == programindex.ObjectModule || object.Kind == programindex.ObjectPackage {
			if object.Location == nil {
				return programindex.Object{}, fmt.Errorf("module %q has no exact location", object.Name)
			}
			return object, nil
		}
		if object.ContainerID != "" {
			objectID = object.ContainerID
		} else {
			objectID = object.OwnerID
		}
	}
	return programindex.Object{}, fmt.Errorf("object has no exact module container")
}

func dependencyKeys(value dependencies.Dependency) ([]string, error) {
	if !validPythonImportKey(value.PackagePath) {
		return nil, fmt.Errorf(
			"integration usage: selected dependency %q has invalid Python package path %q",
			value.ID,
			value.PackagePath,
		)
	}
	// PackagePath is the Python import authority. Name is presentation text in
	// the shared dependency contract and must never become an alternate match.
	return []string{value.PackagePath}, nil
}

func validPythonImportKey(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		for position, character := range part {
			if character == '_' || character >= 'a' && character <= 'z' ||
				character >= 'A' && character <= 'Z' ||
				position > 0 && character >= '0' && character <= '9' {
				continue
			}
			return false
		}
	}
	return true
}

func validDottedExpression(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		for _, character := range part {
			if unicode.IsSpace(character) || unicode.IsControl(character) || character == '/' ||
				character == '\\' || character == '>' || character == '<' {
				return false
			}
		}
	}
	return true
}

func validLocation(value programindex.Location) bool {
	return value.Path != "" && path.Clean(value.Path) == value.Path && value.Path != "." &&
		!path.IsAbs(value.Path) && !strings.Contains(value.Path, "\\") &&
		value.Line > 0 && value.Column > 0
}

// A Python call relation is anchored at the complete call expression, while
// each witness is anchored at its exact callee token. Their columns may differ.
func sameSourceLine(left, right programindex.Location) bool {
	return left.Path == right.Path && left.Line == right.Line
}

func validBoundedText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validBoundedLine(value string, limit int) bool {
	return validBoundedText(value, limit) && !strings.ContainsAny(value, "\r\n")
}

func validOptionalLine(value string, limit int) bool {
	return value == "" || validBoundedLine(value, limit)
}

func operationLess(left, right Operation) bool {
	return operationKey(left) < operationKey(right)
}

func operationKey(value Operation) string {
	return value.DependencyID + "\x00" + value.RelationID + "\x00" +
		fmt.Sprintf("%08d", value.WitnessIndex) + "\x00" + value.ExternalSymbolID
}

func classifierState(
	programIndexSHA256 string,
	dependenciesSHA256 string,
	candidates preparedCandidates,
	batchIndex int,
	batchCount int,
	operationsObserved int,
) ([]byte, error) {
	type dependencyAuthority struct {
		Ref string `json:"ref"`
		ID  string `json:"id"`
	}
	type operationAuthority struct {
		Ref          string `json:"ref"`
		DependencyID string `json:"dependency_id"`
		RelationID   string `json:"relation_id"`
		WitnessIndex int    `json:"witness_index"`
	}
	authority := struct {
		Dependencies []dependencyAuthority `json:"dependencies"`
		Operations   []operationAuthority  `json:"operations"`
	}{
		Dependencies: make([]dependencyAuthority, 0, len(candidates.dependencies)),
		Operations:   make([]operationAuthority, 0, len(candidates.operations)),
	}
	for _, candidate := range candidates.dependencies {
		authority.Dependencies = append(authority.Dependencies, dependencyAuthority{
			Ref: candidate.ref, ID: candidate.selected.Dependency.ID,
		})
	}
	for _, candidate := range candidates.operations {
		authority.Operations = append(authority.Operations, operationAuthority{
			Ref: candidate.ref, DependencyID: candidate.operation.DependencyID,
			RelationID: candidate.operation.RelationID, WitnessIndex: candidate.operation.WitnessIndex,
		})
	}
	authorityJSON, err := json.Marshal(authority)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Contract                      string `json:"contract"`
		Preparation                   int    `json:"preparation"`
		ResponseSchema                int    `json:"response_schema"`
		PromptSHA256                  string `json:"prompt_sha256"`
		ProgramIndexSHA256            string `json:"program_index_sha256"`
		IntegrationDependenciesSHA256 string `json:"integration_dependencies_sha256"`
		BatchIndex                    int    `json:"batch_index"`
		BatchCount                    int    `json:"batch_count"`
		OperationsObserved            int    `json:"operations_observed"`
		AuthoritySHA256               string `json:"authority_sha256"`
	}{
		Contract: "repomap.integrationusage.v6", Preparation: 5, ResponseSchema: 3,
		PromptSHA256:                  sha256Hex([]byte(strings.TrimSpace(prompt))),
		ProgramIndexSHA256:            programIndexSHA256,
		IntegrationDependenciesSHA256: dependenciesSHA256,
		BatchIndex:                    batchIndex,
		BatchCount:                    batchCount,
		OperationsObserved:            operationsObserved,
		AuthoritySHA256:               sha256Hex(authorityJSON),
	})
}

func completeBatchCount(total, perBatch int) int {
	if total == 0 {
		return 0
	}
	return 1 + (total-1)/perBatch
}

func cloneSelectedDependency(value integrationdependency.SelectedDependency) integrationdependency.SelectedDependency {
	result := value
	result.Dependency.ImporterRefs = append([]string(nil), value.Dependency.ImporterRefs...)
	if value.Dependency.Replacement != nil {
		replacement := *value.Dependency.Replacement
		result.Dependency.Replacement = &replacement
	}
	result.Importers = append([]dependencies.Importer(nil), value.Importers...)
	return result
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
