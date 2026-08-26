// Package integrationdependency classifies exact observed dependency rows and,
// for declaration-aware language paths, exact package-manager declarations
// that may provide meaningful integration or side-effect capabilities. The two
// authorities remain separate: a declaration is never promoted to an import or
// use, and the model selects only request-local refs.
package integrationdependency

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
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version = 4

	MaxAdvertisedImporters = 16

	MaxDependencyBatches          = 16
	MaxAdvertisedDependencies     = 4096
	MaxAdvertisedDeclaredPackages = dependencydeclaration.MaxPackages

	// Selection is a subset of the complete advertised authority, not a
	// separately quota-limited semantic product. These defensive artifact
	// bounds therefore match the corresponding input-authority bounds.
	MaxSelectedDependencies     = MaxAdvertisedDependencies
	MaxSelectedDeclaredPackages = MaxAdvertisedDeclaredPackages
	MaxSelectedCandidates       = MaxSelectedDependencies + MaxSelectedDeclaredPackages

	maxSerializedUserBytes = 1 * 1024 * 1024
	maxRequestBytes        = 4 * 1024 * 1024
	maxResponseBytes       = 128 * 1024
	maxOutputTokens        = 4096
)

//go:embed prompts/classifier.md
var observedPrompt string

//go:embed prompts/classifier_with_declarations.md
var declarationPrompt string

// Coverage accounts for every external or standard-library dependency in the
// exact input catalog. Declaration coverage, when present, is recorded in the
// separate Declarations section rather than being folded into these counts.
type Coverage struct {
	Observed    int  `json:"observed"`
	Advertised  int  `json:"advertised"`
	Omitted     int  `json:"omitted"`
	ModelCalled bool `json:"model_called"`
}

// SelectedDependency restores one model-selected d* ref to the complete exact
// observed dependency row and all of its exact importers.
type SelectedDependency struct {
	Dependency dependencies.Dependency `json:"dependency"`
	Importers  []dependencies.Importer `json:"importers"`
}

// SelectedDeclaredPackage is a deterministic, exact projection of one
// package-manager package authority. It intentionally contains no import or
// call claim. The projection keeps the context useful to later cubes without
// duplicating every declaration statement into this artifact.
type SelectedDeclaredPackage struct {
	PackageID             string                              `json:"package_id"`
	Ecosystem             string                              `json:"ecosystem"`
	Name                  string                              `json:"name"`
	NormalizedName        string                              `json:"normalized_name"`
	Names                 []string                            `json:"names"`
	SourcePaths           []string                            `json:"source_paths"`
	Roles                 []dependencydeclaration.Role        `json:"roles"`
	Groups                []string                            `json:"groups"`
	LocatorKinds          []dependencydeclaration.LocatorKind `json:"locator_kinds"`
	Sections              []string                            `json:"sections"`
	Statements            int                                 `json:"statements"`
	ConditionalStatements int                                 `json:"conditional_statements"`
	ConstraintStatements  int                                 `json:"constraint_statements"`
}

// DeclarationCoverage retains the declaration adapter's exact input ledger.
// Frontier is legitimate incomplete context: retained positive packages may
// be classified, while absence beyond the recorded boundaries proves nothing.
type DeclarationCoverage struct {
	Input      dependencydeclaration.Coverage `json:"input"`
	Advertised int                            `json:"advertised"`
	Selected   int                            `json:"selected"`
}

// DeclarationSelection exists only for the explicit declaration-aware API.
// ArtifactSHA256 binds every p* selection and coverage count to one canonical
// dependencydeclaration artifact.
type DeclarationSelection struct {
	ArtifactSHA256 string                    `json:"artifact_sha256"`
	TargetID       string                    `json:"target_id"`
	Packages       []SelectedDeclaredPackage `json:"packages"`
	Coverage       DeclarationCoverage       `json:"coverage"`
}

// Result is the stable, language-neutral result of one complete classifier
// run. Dependencies and Declarations are deliberately separate authorities.
type Result struct {
	Version                 int                   `json:"version"`
	DependencyCatalogSHA256 string                `json:"dependency_catalog_sha256"`
	Dependencies            []SelectedDependency  `json:"dependencies"`
	Declarations            *DeclarationSelection `json:"declarations,omitempty"`
	Coverage                Coverage              `json:"coverage"`
}

type wireImporter struct {
	PackagePath    string `json:"package_path"`
	RepositoryPath string `json:"repository_path"`
}

type wireDependency struct {
	Ref              string            `json:"ref"`
	Kind             dependencies.Kind `json:"kind"`
	Name             string            `json:"name"`
	ModulePath       string            `json:"module_path"`
	ModuleVersion    string            `json:"module_version,omitempty"`
	PackagePath      string            `json:"package_path"`
	Importers        []wireImporter    `json:"importers"`
	ImportersOmitted int               `json:"importers_omitted"`
}

type wireDeclaredPackage struct {
	Ref                   string                              `json:"ref"`
	Ecosystem             string                              `json:"ecosystem"`
	Name                  string                              `json:"name"`
	NormalizedName        string                              `json:"normalized_name"`
	Names                 []string                            `json:"names"`
	SourcePaths           []string                            `json:"source_paths"`
	Roles                 []dependencydeclaration.Role        `json:"roles"`
	Groups                []string                            `json:"groups"`
	LocatorKinds          []dependencydeclaration.LocatorKind `json:"locator_kinds"`
	Sections              []string                            `json:"sections"`
	Statements            int                                 `json:"statements"`
	ConditionalStatements int                                 `json:"conditional_statements"`
	ConstraintStatements  int                                 `json:"constraint_statements"`
}

type observedRequest struct {
	BatchIndex int              `json:"batch_index"`
	BatchCount int              `json:"batch_count"`
	Observed   int              `json:"observed"`
	Omitted    int              `json:"omitted"`
	Catalog    []wireDependency `json:"catalog"`
}

type declarationRequest struct {
	BatchIndex          int                            `json:"batch_index"`
	BatchCount          int                            `json:"batch_count"`
	Target              wireTargetContext              `json:"target"`
	Observed            int                            `json:"observed"`
	Omitted             int                            `json:"omitted"`
	DeclaredObserved    int                            `json:"declared_observed"`
	DeclarationCoverage dependencydeclaration.Coverage `json:"declaration_coverage"`
	Catalog             []wireDependency               `json:"observed_dependencies"`
	DeclaredPackages    []wireDeclaredPackage          `json:"declared_packages"`
}

type wireTargetContext struct {
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Selector string `json:"selector"`
}

type response struct {
	IntegrationDependencyRefs []string `json:"integration_dependency_refs"`
	DeclaredPackageRefs       []string `json:"integration_declared_package_refs"`
}

type candidate struct {
	ref        string
	dependency dependencies.Dependency
	importers  []dependencies.Importer
	wire       wireDependency
	wireBytes  int
}

type declaredCandidate struct {
	ref        string
	projection SelectedDeclaredPackage
	wire       wireDeclaredPackage
	wireBytes  int
}

type preparedBatch struct {
	observed  []candidate
	declared  []declaredCandidate
	userBytes int
}

type responseAuthority struct {
	observed map[string]struct{}
	declared map[string]struct{}
}

// Run is the explicit observed-only API used by the Go dependency path. Empty
// complete catalogs return an authoritative empty result without a model call;
// partial catalogs and global-bound overflow fail closed.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	catalog dependencies.Catalog,
) (Result, error) {
	return run(ctx, executor, provider, catalog, nil, nil)
}

// RunWithDeclarations is the explicit declaration-aware API. It classifies the
// bounded union of exact observed d* candidates and retained declaration p*
// candidates, but restores the two selections into separate result sections.
// A declaration frontier is sent as incomplete context and never interpreted
// as evidence that an unrepresented package is absent.
func RunWithDeclarations(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	catalog dependencies.Catalog,
	declarations dependencydeclaration.Result,
	target programindex.Target,
) (Result, error) {
	owned := declarations.Snapshot()
	ownedTarget := target.Snapshot()
	return run(ctx, executor, provider, catalog, &owned, &ownedTarget)
}

func run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	catalog dependencies.Catalog,
	declarations *dependencydeclaration.Result,
	target *programindex.Target,
) (Result, error) {
	if (declarations == nil) != (target == nil) {
		return Result{}, fmt.Errorf("integration dependency: declaration and target input mode mismatch")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := catalog.Validate(); err != nil {
		return Result{}, fmt.Errorf("integration dependency: catalog: %w", err)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete {
		return Result{}, fmt.Errorf(
			"integration dependency: dependency authority is incomplete: %s",
			omissionSummary(catalog.Coverage.Omissions),
		)
	}
	catalogSHA256, err := catalogSHA(catalog)
	if err != nil {
		return Result{}, err
	}
	observedCandidates, observed, err := buildCandidates(catalog)
	if err != nil {
		return Result{}, err
	}

	var declaredCandidates []declaredCandidate
	var declarationSHA256 string
	if declarations != nil {
		if err := declarations.Validate(); err != nil {
			return Result{}, fmt.Errorf("integration dependency: declarations: %w", err)
		}
		if err := validateAuthorityJoin(catalog, *declarations, *target); err != nil {
			return Result{}, err
		}
		declarationSHA256, err = declarations.ArtifactSHA256()
		if err != nil {
			return Result{}, fmt.Errorf("integration dependency: declaration identity: %w", err)
		}
		declaredCandidates, err = buildDeclaredCandidates(*declarations)
		if err != nil {
			return Result{}, err
		}
	}

	result := Result{
		Version: Version, DependencyCatalogSHA256: catalogSHA256,
		Dependencies: []SelectedDependency{},
		Coverage: Coverage{
			Observed: observed, Advertised: len(observedCandidates), Omitted: 0,
			ModelCalled: len(observedCandidates)+len(declaredCandidates) > 0,
		},
	}
	if declarations != nil {
		result.Declarations = &DeclarationSelection{
			ArtifactSHA256: declarationSHA256,
			TargetID:       target.ID,
			Packages:       []SelectedDeclaredPackage{},
			Coverage: DeclarationCoverage{
				Input: declarations.Coverage, Advertised: len(declaredCandidates), Selected: 0,
			},
		}
	}
	if !result.Coverage.ModelCalled {
		if err := validateResultAgainst(result, catalog, declarations, target); err != nil {
			return Result{}, err
		}
		return result, nil
	}

	batches, err := partitionCandidates(observedCandidates, declaredCandidates, observed, declarations, target)
	if err != nil {
		return Result{}, err
	}
	calls := make([]llm.Call[response], 0, len(batches))
	authorities := make([]responseAuthority, 0, len(batches))
	for batchIndex, batch := range batches {
		user, err := marshalRequest(batch, batchIndex+1, len(batches), observed, declarations, target)
		if err != nil {
			return Result{}, fmt.Errorf("integration dependency: encode request batch %d: %w", batchIndex+1, err)
		}
		if len(user) > maxSerializedUserBytes {
			return Result{}, fmt.Errorf(
				"integration dependency: serialized request batch %d is %d bytes, limit is %d",
				batchIndex+1, len(user), maxSerializedUserBytes,
			)
		}
		state, err := classifierState(
			batch, batchIndex+1, len(batches), observed,
			catalogSHA256, declarationSHA256, targetID(target), declarations != nil,
		)
		if err != nil {
			return Result{}, fmt.Errorf("integration dependency: state batch %d: %w", batchIndex+1, err)
		}
		authority := buildResponseAuthority(batch)
		authorities = append(authorities, authority)
		declarationMode := declarations != nil
		calls = append(calls, llm.Call[response]{
			State: state,
			Prompt: llm.Prompt{
				System: classifierPrompt(declarationMode), User: string(user), ResponseFormatJSON: true,
			},
			Limits: llm.Limits{
				MaxRequestBytes: maxRequestBytes, MaxResponseBytes: maxResponseBytes,
				MaxOutputTokens: maxOutputTokens,
			},
			Validate: func(value response) error {
				return validateResponse(value, declarationMode)
			},
		})
	}
	outcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
	if err != nil {
		return Result{}, fmt.Errorf("integration dependency: model cube: %w", err)
	}
	selectedObserved := make(map[string]struct{})
	selectedDeclared := make(map[string]struct{})
	for batchIndex, outcome := range outcomes {
		authority := authorities[batchIndex]
		for ref := range normalizeSelectedRefs(outcome.Value.IntegrationDependencyRefs, authority.observed) {
			selectedObserved[ref] = struct{}{}
		}
		for ref := range normalizeSelectedRefs(outcome.Value.DeclaredPackageRefs, authority.declared) {
			selectedDeclared[ref] = struct{}{}
		}
	}
	if len(selectedObserved) > MaxSelectedDependencies ||
		len(selectedDeclared) > MaxSelectedDeclaredPackages ||
		len(selectedObserved)+len(selectedDeclared) > MaxSelectedCandidates {
		return Result{}, fmt.Errorf("integration dependency: total selected candidate bound exceeded")
	}
	for _, value := range observedCandidates {
		if _, selected := selectedObserved[value.ref]; !selected {
			continue
		}
		result.Dependencies = append(result.Dependencies, SelectedDependency{
			Dependency: cloneDependency(value.dependency),
			Importers:  append([]dependencies.Importer(nil), value.importers...),
		})
	}
	if result.Declarations != nil {
		for _, value := range declaredCandidates {
			if _, selected := selectedDeclared[value.ref]; !selected {
				continue
			}
			result.Declarations.Packages = append(
				result.Declarations.Packages, cloneDeclaredProjection(value.projection),
			)
		}
		result.Declarations.Coverage.Selected = len(result.Declarations.Packages)
	}
	if err := validateResultAgainst(result, catalog, declarations, target); err != nil {
		return Result{}, err
	}
	if _, err := Encode(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

// Validate checks the standalone result contract, exact observed dependency
// identities, declaration projection shape, canonical order, and coverage.
func (result Result) Validate() error {
	if result.Version != Version || !validSHA256(result.DependencyCatalogSHA256) || result.Dependencies == nil {
		return fmt.Errorf("integration dependency: invalid result identity")
	}
	if len(result.Dependencies) > MaxSelectedDependencies {
		return fmt.Errorf("integration dependency: selected dependency bound exceeded")
	}
	if len(result.Dependencies) > result.Coverage.Advertised {
		return fmt.Errorf("integration dependency: selected dependencies exceed advertised authority")
	}
	if err := validateObservedSelections(result.Dependencies); err != nil {
		return err
	}
	declaredAdvertised := 0
	declaredSelected := 0
	if result.Declarations != nil {
		if err := validateDeclarationSelection(*result.Declarations); err != nil {
			return err
		}
		declaredAdvertised = result.Declarations.Coverage.Advertised
		declaredSelected = len(result.Declarations.Packages)
	}
	if len(result.Dependencies)+declaredSelected > MaxSelectedCandidates {
		return fmt.Errorf("integration dependency: selected candidate bound exceeded")
	}
	if err := validateCoverage(result.Coverage, declaredAdvertised); err != nil {
		return err
	}
	return nil
}

// ValidateAgainst is deliberately observed-only. A declaration-aware result
// must be checked with ValidateAgainstDeclarations so declaration authority can
// never be silently skipped by an older consumer.
func (result Result) ValidateAgainst(catalog dependencies.Catalog) error {
	if result.Declarations != nil {
		return fmt.Errorf("integration dependency: declaration-aware result requires declaration authority")
	}
	return validateResultAgainst(result, catalog, nil, nil)
}

// ValidateAgainstDeclarations proves that coverage and every d*/p* selection
// came from the exact catalog and exact declaration artifact supplied.
func (result Result) ValidateAgainstDeclarations(
	catalog dependencies.Catalog,
	declarations dependencydeclaration.Result,
	target programindex.Target,
) error {
	if result.Declarations == nil {
		return fmt.Errorf("integration dependency: result has no declaration authority")
	}
	owned := declarations.Snapshot()
	ownedTarget := target.Snapshot()
	return validateResultAgainst(result, catalog, &owned, &ownedTarget)
}

func validateResultAgainst(
	result Result,
	catalog dependencies.Catalog,
	declarations *dependencydeclaration.Result,
	target *programindex.Target,
) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if (result.Declarations == nil) != (declarations == nil) ||
		(declarations == nil) != (target == nil) {
		return fmt.Errorf("integration dependency: result input mode mismatch")
	}
	if err := catalog.Validate(); err != nil {
		return fmt.Errorf("integration dependency: catalog: %w", err)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete {
		return fmt.Errorf("integration dependency: dependency authority is incomplete")
	}
	sha, err := catalogSHA(catalog)
	if err != nil {
		return err
	}
	candidates, observed, err := buildCandidates(catalog)
	if err != nil {
		return err
	}
	declaredAdvertised := 0
	if declarations != nil {
		if err := declarations.Validate(); err != nil {
			return fmt.Errorf("integration dependency: declarations: %w", err)
		}
		if err := validateAuthorityJoin(catalog, *declarations, *target); err != nil {
			return err
		}
		declarationSHA256, err := declarations.ArtifactSHA256()
		if err != nil {
			return fmt.Errorf("integration dependency: declaration identity: %w", err)
		}
		declaredCandidates, err := buildDeclaredCandidates(*declarations)
		if err != nil {
			return err
		}
		declaredAdvertised = len(declaredCandidates)
		wantCoverage := DeclarationCoverage{
			Input: declarations.Coverage, Advertised: declaredAdvertised,
			Selected: len(result.Declarations.Packages),
		}
		if result.Declarations.ArtifactSHA256 != declarationSHA256 ||
			result.Declarations.TargetID != target.ID ||
			!reflect.DeepEqual(result.Declarations.Coverage, wantCoverage) {
			return fmt.Errorf("integration dependency: declaration result authority mismatch")
		}
		exactByID := make(map[string]SelectedDeclaredPackage, len(declaredCandidates))
		for _, value := range declaredCandidates {
			exactByID[value.projection.PackageID] = value.projection
		}
		for _, selected := range result.Declarations.Packages {
			exact, exists := exactByID[selected.PackageID]
			if !exists || !reflect.DeepEqual(exact, selected) {
				return fmt.Errorf("integration dependency: selected declaration authority mismatch")
			}
		}
	}
	wantCoverage := Coverage{
		Observed: observed, Advertised: len(candidates), Omitted: 0,
		ModelCalled: len(candidates)+declaredAdvertised > 0,
	}
	if result.DependencyCatalogSHA256 != sha || result.Coverage != wantCoverage {
		return fmt.Errorf("integration dependency: result authority mismatch")
	}
	candidateByID := make(map[string]candidate, len(candidates))
	for _, value := range candidates {
		candidateByID[value.dependency.ID] = value
	}
	for _, selected := range result.Dependencies {
		exact, exists := candidateByID[selected.Dependency.ID]
		if !exists || !reflect.DeepEqual(exact.dependency, selected.Dependency) ||
			!reflect.DeepEqual(exact.importers, selected.Importers) {
			return fmt.Errorf("integration dependency: selected dependency authority mismatch")
		}
	}
	return nil
}

func buildCandidates(catalog dependencies.Catalog) ([]candidate, int, error) {
	importers := make(map[string]dependencies.Importer, len(catalog.Importers))
	for _, importer := range catalog.Importers {
		importers[importer.Ref] = importer
	}
	values := make([]candidate, 0)
	observed := 0
	for _, dependency := range catalog.Dependencies {
		if dependency.Kind != dependencies.KindExternal && dependency.Kind != dependencies.KindStdlib {
			continue
		}
		observed++
		if observed > MaxAdvertisedDependencies {
			return nil, observed, fmt.Errorf(
				"integration dependency: %d observed candidates exceed the complete run bound %d",
				observed, MaxAdvertisedDependencies,
			)
		}
		value := candidate{dependency: cloneDependency(dependency)}
		for _, ref := range dependency.ImporterRefs {
			importer, exists := importers[ref]
			if !exists {
				return nil, 0, fmt.Errorf("integration dependency: dependency has unavailable importer %q", ref)
			}
			value.importers = append(value.importers, importer)
		}
		value.ref = fmt.Sprintf("d%d", len(values)+1)
		value.wire = wireDependency{
			Ref: value.ref, Kind: dependency.Kind, Name: dependency.Name,
			ModulePath: dependency.ModulePath, ModuleVersion: dependency.ModuleVersion,
			PackagePath: dependency.PackagePath, Importers: []wireImporter{},
		}
		for position, importer := range value.importers {
			if position >= MaxAdvertisedImporters {
				value.wire.ImportersOmitted++
				continue
			}
			value.wire.Importers = append(value.wire.Importers, wireImporter{
				PackagePath: importer.PackagePath, RepositoryPath: importer.RepositoryPath,
			})
		}
		encodedWire, err := json.Marshal(value.wire)
		if err != nil {
			return nil, 0, fmt.Errorf("integration dependency: encode observed candidate: %w", err)
		}
		value.wireBytes = len(encodedWire)
		values = append(values, value)
	}
	return values, observed, nil
}

func buildDeclaredCandidates(declarations dependencydeclaration.Result) ([]declaredCandidate, error) {
	if len(declarations.Packages) > MaxAdvertisedDeclaredPackages {
		return nil, fmt.Errorf(
			"integration dependency: %d declared candidates exceed the complete run bound %d",
			len(declarations.Packages), MaxAdvertisedDeclaredPackages,
		)
	}
	sourcePaths := make(map[string]string, len(declarations.Sources))
	for _, source := range declarations.Sources {
		sourcePaths[source.ID] = source.Path
	}
	values := make([]declaredCandidate, 0, len(declarations.Packages))
	for position, pkg := range declarations.Packages {
		projection, err := projectDeclaredPackage(pkg, sourcePaths)
		if err != nil {
			return nil, fmt.Errorf("integration dependency: declared package %d: %w", position, err)
		}
		ref := fmt.Sprintf("p%d", position+1)
		value := declaredCandidate{
			ref: ref, projection: projection,
			wire: wireDeclaredPackage{
				Ref: ref, Ecosystem: projection.Ecosystem, Name: projection.Name,
				NormalizedName:        projection.NormalizedName,
				Names:                 append([]string{}, projection.Names...),
				SourcePaths:           append([]string{}, projection.SourcePaths...),
				Roles:                 append([]dependencydeclaration.Role{}, projection.Roles...),
				Groups:                append([]string{}, projection.Groups...),
				LocatorKinds:          append([]dependencydeclaration.LocatorKind{}, projection.LocatorKinds...),
				Sections:              append([]string{}, projection.Sections...),
				Statements:            projection.Statements,
				ConditionalStatements: projection.ConditionalStatements,
				ConstraintStatements:  projection.ConstraintStatements,
			},
		}
		encodedWire, err := json.Marshal(value.wire)
		if err != nil {
			return nil, fmt.Errorf("integration dependency: encode declared candidate: %w", err)
		}
		value.wireBytes = len(encodedWire)
		values = append(values, value)
	}
	return values, nil
}

func projectDeclaredPackage(
	pkg dependencydeclaration.Package,
	sourcePathByID map[string]string,
) (SelectedDeclaredPackage, error) {
	roles := make(map[dependencydeclaration.Role]struct{})
	groups := make(map[string]struct{})
	locators := make(map[dependencydeclaration.LocatorKind]struct{})
	paths := make(map[string]struct{})
	sections := make(map[string]struct{})
	projection := SelectedDeclaredPackage{
		PackageID: pkg.ID, Ecosystem: pkg.Ecosystem, Name: pkg.Name,
		NormalizedName: pkg.NormalizedName, Names: append([]string(nil), pkg.Names...),
		Statements: len(pkg.Statements),
	}
	for _, statement := range pkg.Statements {
		sourcePath, ok := sourcePathByID[statement.SourceRef]
		if !ok {
			return SelectedDeclaredPackage{}, fmt.Errorf("statement has unavailable source %q", statement.SourceRef)
		}
		roles[statement.Role] = struct{}{}
		locators[statement.Locator.Kind] = struct{}{}
		paths[sourcePath] = struct{}{}
		sections[statement.Section] = struct{}{}
		if statement.Group != "" {
			groups[statement.Group] = struct{}{}
		}
		if statement.Conditional {
			projection.ConditionalStatements++
		}
		if statement.Kind == dependencydeclaration.StatementConstraint {
			projection.ConstraintStatements++
		}
	}
	projection.SourcePaths = sortedSet(paths)
	projection.Groups = sortedSet(groups)
	projection.Sections = sortedSet(sections)
	projection.Roles = sortedRoles(roles)
	projection.LocatorKinds = sortedLocatorKinds(locators)
	if err := validateDeclaredPackage(projection); err != nil {
		return SelectedDeclaredPackage{}, err
	}
	return projection, nil
}

func partitionCandidates(
	observed []candidate,
	declared []declaredCandidate,
	observedTotal int,
	declarations *dependencydeclaration.Result,
	target *programindex.Target,
) ([]preparedBatch, error) {
	if len(observed)+len(declared) == 0 {
		return []preparedBatch{}, nil
	}
	empty := preparedBatch{observed: []candidate{}, declared: []declaredCandidate{}}
	emptyRequest, err := marshalRequest(
		empty, MaxDependencyBatches, MaxDependencyBatches,
		observedTotal, declarations, target,
	)
	if err != nil {
		return nil, err
	}
	if len(emptyRequest) > maxSerializedUserBytes {
		return nil, fmt.Errorf(
			"integration dependency: request metadata is %d serialized bytes, limit is %d",
			len(emptyRequest), maxSerializedUserBytes,
		)
	}
	batches := make([]preparedBatch, 0)
	newBatch := func() preparedBatch {
		return preparedBatch{
			observed: []candidate{}, declared: []declaredCandidate{}, userBytes: len(emptyRequest),
		}
	}
	current := newBatch()
	var appendCandidate func(*candidate, *declaredCandidate) error
	appendCandidate = func(observedCandidate *candidate, declaredValue *declaredCandidate) error {
		delta := 0
		if observedCandidate != nil {
			delta = observedCandidate.wireBytes
			if len(current.observed) > 0 {
				delta++ // exact JSON array comma
			}
		} else {
			delta = declaredValue.wireBytes
			if len(current.declared) > 0 {
				delta++ // exact JSON array comma
			}
		}
		if current.userBytes+delta <= maxSerializedUserBytes {
			if observedCandidate != nil {
				current.observed = append(current.observed, *observedCandidate)
			} else {
				current.declared = append(current.declared, *declaredValue)
			}
			current.userBytes += delta
			return nil
		}
		if len(current.observed)+len(current.declared) == 0 {
			return fmt.Errorf(
				"integration dependency: one candidate atom is %d serialized bytes, request limit is %d",
				current.userBytes+delta, maxSerializedUserBytes,
			)
		}
		batches = append(batches, current)
		if len(batches) >= MaxDependencyBatches {
			return fmt.Errorf(
				"integration dependency: serialized candidate partition exceeds %d model batches",
				MaxDependencyBatches,
			)
		}
		current = newBatch()
		return appendCandidate(observedCandidate, declaredValue)
	}
	for observedIndex, declaredIndex := 0, 0; observedIndex < len(observed) || declaredIndex < len(declared); {
		if observedIndex < len(observed) {
			value := observed[observedIndex]
			if err := appendCandidate(&value, nil); err != nil {
				return nil, err
			}
			observedIndex++
		}
		if declaredIndex < len(declared) {
			value := declared[declaredIndex]
			if err := appendCandidate(nil, &value); err != nil {
				return nil, err
			}
			declaredIndex++
		}
	}
	if len(current.observed)+len(current.declared) > 0 {
		batches = append(batches, current)
	}
	if len(batches) > MaxDependencyBatches {
		return nil, fmt.Errorf(
			"integration dependency: serialized candidate partition exceeds %d model batches",
			MaxDependencyBatches,
		)
	}
	return batches, nil
}

func marshalRequest(
	batch preparedBatch,
	batchIndex, batchCount, observedTotal int,
	declarations *dependencydeclaration.Result,
	target *programindex.Target,
) ([]byte, error) {
	observed := make([]wireDependency, 0, len(batch.observed))
	for _, value := range batch.observed {
		observed = append(observed, value.wire)
	}
	if declarations == nil {
		return json.Marshal(observedRequest{
			BatchIndex: batchIndex, BatchCount: batchCount,
			Observed: observedTotal, Omitted: 0, Catalog: observed,
		})
	}
	declared := make([]wireDeclaredPackage, 0, len(batch.declared))
	for _, value := range batch.declared {
		declared = append(declared, value.wire)
	}
	return json.Marshal(declarationRequest{
		BatchIndex: batchIndex, BatchCount: batchCount,
		Target: wireTargetContext{
			Language: target.Language, Kind: target.Kind, Name: target.Name, Selector: target.Selector,
		},
		Observed: observedTotal, Omitted: 0,
		DeclaredObserved: len(declarations.Packages), DeclarationCoverage: declarations.Coverage,
		Catalog: observed, DeclaredPackages: declared,
	})
}

func validateResponse(
	value response,
	declarationMode bool,
) error {
	if value.IntegrationDependencyRefs == nil {
		return fmt.Errorf("integration dependency: observed selected-ref array is missing")
	}
	if declarationMode && value.DeclaredPackageRefs == nil {
		return fmt.Errorf("integration dependency: declared selected-ref array is missing")
	}
	if !declarationMode && value.DeclaredPackageRefs != nil {
		return fmt.Errorf("integration dependency: declaration refs are not valid in observed-only mode")
	}
	return nil
}

func buildResponseAuthority(batch preparedBatch) responseAuthority {
	authority := responseAuthority{
		observed: make(map[string]struct{}, len(batch.observed)),
		declared: make(map[string]struct{}, len(batch.declared)),
	}
	for _, value := range batch.observed {
		authority.observed[value.ref] = struct{}{}
	}
	for _, value := range batch.declared {
		// A constraint-only declaration limits another requirement's version;
		// it does not establish that this distribution is installed. Keep it
		// visible as exact manifest context, but do not give it selection
		// authority.
		if value.projection.ConstraintStatements < value.projection.Statements {
			authority.declared[value.ref] = struct{}{}
		}
	}
	return authority
}

// normalizeSelectedRefs keeps only exact request-local refs before treating
// the response as a set. Unknown refs carry no authority: they are ignored,
// never guessed, repaired, or used to trigger another provider call.
func normalizeSelectedRefs(refs []string, allowed map[string]struct{}) map[string]struct{} {
	selected := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, exists := allowed[ref]; !exists {
			continue
		}
		selected[ref] = struct{}{}
	}
	return selected
}

func validateObservedSelections(values []SelectedDependency) error {
	importersByRef := make(map[string]dependencies.Importer)
	dependenciesForSeal := make([]dependencies.Dependency, 0, len(values))
	seenDependencies := make(map[string]struct{}, len(values))
	for position, selected := range values {
		if selected.Dependency.Kind != dependencies.KindExternal &&
			selected.Dependency.Kind != dependencies.KindStdlib {
			return fmt.Errorf("integration dependency: selected dependency is not an integration candidate")
		}
		if len(selected.Importers) != len(selected.Dependency.ImporterRefs) || selected.Importers == nil {
			return fmt.Errorf("integration dependency: selected dependency importer binding mismatch")
		}
		if _, duplicate := seenDependencies[selected.Dependency.ID]; duplicate {
			return fmt.Errorf("integration dependency: duplicate selected dependency")
		}
		seenDependencies[selected.Dependency.ID] = struct{}{}
		if position > 0 && !selectionLess(values[position-1], selected) {
			return fmt.Errorf("integration dependency: selected dependencies are not canonical")
		}
		for importerPosition, importer := range selected.Importers {
			if importer.Ref != selected.Dependency.ImporterRefs[importerPosition] {
				return fmt.Errorf("integration dependency: selected dependency importer order mismatch")
			}
			if previous, exists := importersByRef[importer.Ref]; exists && previous != importer {
				return fmt.Errorf("integration dependency: conflicting importer authority")
			}
			importersByRef[importer.Ref] = importer
		}
		dependenciesForSeal = append(dependenciesForSeal, cloneDependency(selected.Dependency))
	}
	importers := make([]dependencies.Importer, 0, len(importersByRef))
	for _, importer := range importersByRef {
		importers = append(importers, importer)
	}
	sealed, err := dependencies.BuildWithOmissions(importers, dependenciesForSeal, nil)
	if err != nil {
		return fmt.Errorf("integration dependency: selected authority: %w", err)
	}
	if len(sealed.Dependencies) != len(values) {
		return fmt.Errorf("integration dependency: selected dependency authority was merged")
	}
	sealedByID := make(map[string]dependencies.Dependency, len(sealed.Dependencies))
	for _, value := range sealed.Dependencies {
		sealedByID[value.ID] = value
	}
	for _, selected := range values {
		if !reflect.DeepEqual(sealedByID[selected.Dependency.ID], selected.Dependency) {
			return fmt.Errorf("integration dependency: selected dependency identity mismatch")
		}
	}
	return nil
}

func validateDeclarationSelection(value DeclarationSelection) error {
	if !validSHA256(value.ArtifactSHA256) || !validBoundedPlain(value.TargetID) || value.Packages == nil {
		return fmt.Errorf("integration dependency: invalid declaration selection identity")
	}
	if len(value.Packages) > MaxSelectedDeclaredPackages ||
		len(value.Packages) != value.Coverage.Selected ||
		len(value.Packages) > value.Coverage.Advertised {
		return fmt.Errorf("integration dependency: invalid declaration selection count")
	}
	if err := validateDeclarationCoverage(value.Coverage); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(value.Packages))
	for position, selected := range value.Packages {
		if err := validateDeclaredPackage(selected); err != nil {
			return fmt.Errorf("integration dependency: declared package %d: %w", position, err)
		}
		if selected.ConstraintStatements == selected.Statements {
			return fmt.Errorf("integration dependency: declared package %d is constraint-only", position)
		}
		if _, duplicate := seen[selected.PackageID]; duplicate {
			return fmt.Errorf("integration dependency: duplicate selected declared package")
		}
		seen[selected.PackageID] = struct{}{}
		if position > 0 && !declaredPackageLess(value.Packages[position-1], selected) {
			return fmt.Errorf("integration dependency: selected declared packages are not canonical")
		}
	}
	return nil
}

func validateDeclarationCoverage(value DeclarationCoverage) error {
	input := value.Input
	counts := []int{
		input.SourcesObserved, input.SourcesParsed, input.SourcesFrontier,
		input.PackagesRetained, input.StatementsObserved, input.StatementsRetained,
		input.StatementsFrontier, input.IncludesObserved, input.IncludesResolved,
		input.IncludesFrontier, input.Boundaries, value.Advertised, value.Selected,
	}
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("integration dependency: invalid declaration coverage")
		}
	}
	if !input.State.Valid() || input.SourcesObserved != input.SourcesParsed+input.SourcesFrontier ||
		input.StatementsObserved != input.StatementsRetained+input.StatementsFrontier ||
		input.IncludesObserved != input.IncludesResolved+input.IncludesFrontier ||
		value.Advertised != input.PackagesRetained || value.Advertised > MaxAdvertisedDeclaredPackages ||
		value.Selected > value.Advertised || value.Selected > MaxSelectedDeclaredPackages ||
		(input.State == dependencydeclaration.CoverageComplete &&
			(input.SourcesFrontier != 0 || input.Boundaries != 0)) ||
		(input.State == dependencydeclaration.CoverageFrontier &&
			input.SourcesFrontier == 0 && input.Boundaries == 0) {
		return fmt.Errorf("integration dependency: invalid declaration coverage")
	}
	return nil
}

func validateDeclaredPackage(value SelectedDeclaredPackage) error {
	if !validPrefixedIdentity(value.PackageID, "ddpkg-") {
		return fmt.Errorf("invalid declared package identity")
	}
	if !validBoundedPlain(value.Ecosystem) || !validBoundedPlain(value.Name) ||
		!validBoundedPlain(value.NormalizedName) {
		return fmt.Errorf("invalid declared package naming")
	}
	if value.Names == nil || len(value.Names) == 0 || value.Name != value.Names[0] {
		return fmt.Errorf("invalid declared package names")
	}
	if value.SourcePaths == nil || len(value.SourcePaths) == 0 {
		return fmt.Errorf("invalid declared package source paths")
	}
	if value.Roles == nil || len(value.Roles) == 0 {
		return fmt.Errorf("invalid declared package roles")
	}
	if value.Groups == nil {
		return fmt.Errorf("declared package groups inventory is absent")
	}
	if value.LocatorKinds == nil || len(value.LocatorKinds) == 0 {
		return fmt.Errorf("invalid declared package locator kinds")
	}
	if value.Sections == nil || len(value.Sections) == 0 {
		return fmt.Errorf("invalid declared package sections")
	}
	if value.Statements <= 0 || value.Statements > dependencydeclaration.MaxStatements ||
		value.ConditionalStatements < 0 || value.ConditionalStatements > value.Statements ||
		value.ConstraintStatements < 0 || value.ConstraintStatements > value.Statements {
		return fmt.Errorf("invalid declared package statement counts")
	}
	if err := validateCanonicalStrings(value.Names, validBoundedPlain); err != nil {
		return err
	}
	if err := validateCanonicalStrings(value.SourcePaths, validRepositoryPath); err != nil {
		return err
	}
	if err := validateCanonicalStrings(value.Groups, validBoundedPlain); err != nil {
		return err
	}
	if err := validateCanonicalStrings(value.Sections, validBoundedPlain); err != nil {
		return err
	}
	for position, role := range value.Roles {
		if !role.Valid() || (position > 0 && string(value.Roles[position-1]) >= string(role)) {
			return fmt.Errorf("invalid declared package roles")
		}
	}
	for position, kind := range value.LocatorKinds {
		if !kind.Valid() || (position > 0 && string(value.LocatorKinds[position-1]) >= string(kind)) {
			return fmt.Errorf("invalid declared package locator kinds")
		}
	}
	return nil
}

func validateCoverage(value Coverage, declaredAdvertised int) error {
	if value.Observed < 0 || value.Observed > MaxAdvertisedDependencies ||
		value.Advertised != value.Observed || value.Omitted != 0 ||
		value.ModelCalled != (value.Advertised+declaredAdvertised > 0) {
		return fmt.Errorf("integration dependency: invalid coverage")
	}
	return nil
}

func validateAuthorityJoin(
	catalog dependencies.Catalog,
	declarations dependencydeclaration.Result,
	target programindex.Target,
) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("integration dependency: target: %w", err)
	}
	if declarations.TargetID != target.ID || declarations.Scope.Language != target.Language {
		return fmt.Errorf("integration dependency: declaration and target authorities differ")
	}
	language := declarations.Scope.Language
	for _, importer := range catalog.Importers {
		if importer.Language != language {
			return fmt.Errorf("integration dependency: declaration and observed dependency languages differ")
		}
	}
	for _, dependency := range catalog.Dependencies {
		if dependency.Language != language {
			return fmt.Errorf("integration dependency: declaration and observed dependency languages differ")
		}
	}
	return nil
}

func classifierState(
	batch preparedBatch,
	batchIndex, batchTotal, observed int,
	catalogSHA256, declarationSHA256, targetID string,
	declarationMode bool,
) ([]byte, error) {
	type authorityRow struct {
		Ref  string `json:"ref"`
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	authority := make([]authorityRow, 0, len(batch.observed)+len(batch.declared))
	for _, value := range batch.observed {
		authority = append(authority, authorityRow{Ref: value.ref, Kind: "observed", ID: value.dependency.ID})
	}
	for _, value := range batch.declared {
		authority = append(authority, authorityRow{Ref: value.ref, Kind: "declared", ID: value.projection.PackageID})
	}
	authorityJSON, err := json.Marshal(authority)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Contract                  string `json:"contract"`
		Preparation               int    `json:"preparation"`
		ResponseSchema            int    `json:"response_schema"`
		PromptSHA256              string `json:"prompt_sha256"`
		BatchIndex                int    `json:"batch_index"`
		BatchCount                int    `json:"batch_count"`
		Observed                  int    `json:"observed"`
		DependencyCatalogSHA256   string `json:"dependency_catalog_sha256"`
		DeclarationArtifactSHA256 string `json:"declaration_artifact_sha256,omitempty"`
		TargetID                  string `json:"target_id,omitempty"`
		AuthoritySHA256           string `json:"authority_sha256"`
	}{
		Contract: "repomap.integrationdependency.v5", Preparation: 5, ResponseSchema: 5,
		PromptSHA256:              sha256Hex([]byte(classifierPrompt(declarationMode))),
		BatchIndex:                batchIndex,
		BatchCount:                batchTotal,
		Observed:                  observed,
		DependencyCatalogSHA256:   catalogSHA256,
		DeclarationArtifactSHA256: declarationSHA256,
		TargetID:                  targetID,
		AuthoritySHA256:           sha256Hex(authorityJSON),
	})
}

func targetID(target *programindex.Target) string {
	if target == nil {
		return ""
	}
	return target.ID
}

func classifierPrompt(declarationMode bool) string {
	if declarationMode {
		return strings.TrimSpace(declarationPrompt)
	}
	return strings.TrimSpace(observedPrompt)
}

func catalogSHA(catalog dependencies.Catalog) (string, error) {
	encoded, err := dependencies.Encode(catalog)
	if err != nil {
		return "", fmt.Errorf("integration dependency: encode catalog authority: %w", err)
	}
	return sha256Hex(encoded), nil
}

func cloneDependency(value dependencies.Dependency) dependencies.Dependency {
	result := value
	result.ImporterRefs = append([]string(nil), value.ImporterRefs...)
	if value.Replacement != nil {
		replacement := *value.Replacement
		result.Replacement = &replacement
	}
	return result
}

func cloneDeclaredProjection(value SelectedDeclaredPackage) SelectedDeclaredPackage {
	result := value
	result.Names = append([]string{}, value.Names...)
	result.SourcePaths = append([]string{}, value.SourcePaths...)
	result.Roles = append([]dependencydeclaration.Role{}, value.Roles...)
	result.Groups = append([]string{}, value.Groups...)
	result.LocatorKinds = append([]dependencydeclaration.LocatorKind{}, value.LocatorKinds...)
	result.Sections = append([]string{}, value.Sections...)
	return result
}

func selectionLess(left, right SelectedDependency) bool {
	return dependencyKey(left.Dependency) < dependencyKey(right.Dependency)
}

func dependencyKey(value dependencies.Dependency) string {
	rank := "3"
	switch value.Kind {
	case dependencies.KindWorkspace:
		rank = "0"
	case dependencies.KindStdlib:
		rank = "1"
	case dependencies.KindExternal:
		rank = "2"
	}
	return rank + "\x00" + value.PackagePath + "\x00" + value.ModulePath + "\x00" + value.ID
}

func declaredPackageLess(left, right SelectedDeclaredPackage) bool {
	return left.Ecosystem+"\x00"+left.NormalizedName+"\x00"+left.PackageID <
		right.Ecosystem+"\x00"+right.NormalizedName+"\x00"+right.PackageID
}

func omissionSummary(omissions []dependencies.Omission) string {
	if len(omissions) == 0 {
		return "partial coverage has no classified omission"
	}
	first := omissions[0]
	summary := fmt.Sprintf("%s for %s", first.Reason, first.PackagePath)
	if len(omissions) > 1 {
		summary += fmt.Sprintf(" and %d more", len(omissions)-1)
	}
	return summary
}

func sortedSet[T ~string](values map[T]struct{}) []T {
	result := make([]T, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return string(result[left]) < string(result[right]) })
	return result
}

func sortedRoles(values map[dependencydeclaration.Role]struct{}) []dependencydeclaration.Role {
	return sortedSet(values)
}

func sortedLocatorKinds(values map[dependencydeclaration.LocatorKind]struct{}) []dependencydeclaration.LocatorKind {
	return sortedSet(values)
}

func validateCanonicalStrings(values []string, validate func(string) bool) error {
	for position, value := range values {
		if !validate(value) || (position > 0 && values[position-1] >= value) {
			return fmt.Errorf("invalid canonical declared package strings")
		}
	}
	return nil
}

func validBoundedPlain(value string) bool {
	if value == "" || len(value) > dependencydeclaration.MaxStringBytes ||
		!utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

func validRepositoryPath(value string) bool {
	if !validBoundedPlain(value) || path.IsAbs(value) || strings.Contains(value, "\\") || path.Clean(value) != value {
		return false
	}
	return value != ".." && !strings.HasPrefix(value, "../")
}

func validPrefixedIdentity(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+24 {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil && len(decoded) == 12 && value == strings.ToLower(value)
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func sha256Hex(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}
