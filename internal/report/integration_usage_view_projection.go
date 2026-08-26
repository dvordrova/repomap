package report

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	IntegrationUsageViewVersion  = 3
	MaxIntegrationUsageViewBytes = 16 << 20
)

// IntegrationUsageView is the report-owned projection of model-selected
// integration operations for one exact ProgramIndex. It deliberately
// excludes every high-recall dependency or operation candidate the model did
// not select.
type IntegrationUsageView struct {
	Version                       int                             `json:"version"`
	ProgramTargetID               string                          `json:"program_target_id"`
	ProgramIndexSHA256            string                          `json:"program_index_sha256"`
	DependencyCatalogSHA256       string                          `json:"dependency_catalog_sha256,omitempty"`
	IntegrationDependenciesSHA256 string                          `json:"integration_dependencies_sha256,omitempty"`
	IntegrationUsageSHA256        string                          `json:"integration_usage_sha256,omitempty"`
	Dependencies                  []IntegrationUsageDependency    `json:"dependencies"`
	DeclaredCandidates            []IntegrationDeclaredCandidate  `json:"declared_candidates"`
	DeclarationCoverage           *IntegrationDeclarationCoverage `json:"declaration_coverage,omitempty"`
	Coverage                      IntegrationUsageViewCoverage    `json:"coverage"`
}

// IntegrationDeclaredCandidate is a package-manager candidate selected by the
// high-recall dependency cube. It deliberately carries no import, call, or
// runtime-use claim; a later resolver may connect it to exact code evidence.
type IntegrationDeclaredCandidate struct {
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

type IntegrationDeclarationCoverage struct {
	Input      dependencydeclaration.Coverage `json:"input"`
	Advertised int                            `json:"advertised"`
	Selected   int                            `json:"selected"`
}

// IntegrationUsageDependency is published only when at least one exact use
// for this DependencyID was selected by the model.
type IntegrationUsageDependency struct {
	DependencyID string                `json:"dependency_id"`
	Language     string                `json:"language"`
	Kind         dependencies.Kind     `json:"kind"`
	Name         string                `json:"name"`
	ModulePath   string                `json:"module_path,omitempty"`
	PackagePath  string                `json:"package_path"`
	Uses         []IntegrationUsageUse `json:"uses"`
}

type IntegrationUsageUse struct {
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
	Label            string                  `json:"label"`
	Mechanism        string                  `json:"mechanism"`
}

// IntegrationUsageViewCoverage is copied field-for-field from the validated
// producer ledger. Rendered counts are checked against the same Dependencies
// and Uses arrays the browser receives; the report derives no semantic rows.
type IntegrationUsageViewCoverage struct {
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

// NewIntegrationUsageView first revalidates the producer artifact against both
// exact inputs, then groups only selected uses by exact DependencyID.
func NewIntegrationUsageView(
	usage integrationusage.Result,
	index programindex.Index,
	selected integrationdependency.Result,
) (*IntegrationUsageView, error) {
	if err := usage.ValidateAgainst(index, selected); err != nil {
		return nil, fmt.Errorf("integration usage view: producer authority: %w", err)
	}
	if !programSemanticLanguage(index.Target.Language) {
		return nil, fmt.Errorf(
			"integration usage view: unsupported ProgramIndex language %q", index.Target.Language,
		)
	}
	selectedSHA256, err := selected.ArtifactSHA256()
	if err != nil {
		return nil, fmt.Errorf("integration usage view: integration dependency identity: %w", err)
	}
	usageSHA256, err := usage.ArtifactSHA256()
	if err != nil {
		return nil, fmt.Errorf("integration usage view: integration usage identity: %w", err)
	}
	dependenciesByID := make(map[string]dependencies.Dependency, len(selected.Dependencies))
	for _, value := range selected.Dependencies {
		dependenciesByID[value.Dependency.ID] = cloneIntegrationUsageDependency(value.Dependency)
	}

	view := &IntegrationUsageView{
		Version: IntegrationUsageViewVersion, ProgramTargetID: index.Target.ID,
		ProgramIndexSHA256:            index.SHA256,
		DependencyCatalogSHA256:       selected.DependencyCatalogSHA256,
		IntegrationDependenciesSHA256: selectedSHA256,
		IntegrationUsageSHA256:        usageSHA256,
		Dependencies:                  []IntegrationUsageDependency{},
		DeclaredCandidates:            []IntegrationDeclaredCandidate{},
		Coverage:                      projectIntegrationUsageCoverage(usage.Coverage),
	}
	if selected.Declarations != nil {
		view.DeclarationCoverage = &IntegrationDeclarationCoverage{
			Input:      selected.Declarations.Coverage.Input,
			Advertised: selected.Declarations.Coverage.Advertised,
			Selected:   selected.Declarations.Coverage.Selected,
		}
		for _, candidate := range selected.Declarations.Packages {
			view.DeclaredCandidates = append(
				view.DeclaredCandidates, projectIntegrationDeclaredCandidate(candidate),
			)
		}
	}
	groupsByID := make(map[string]int)
	for _, selectedUse := range usage.Uses {
		dependency, ok := dependenciesByID[selectedUse.Operation.DependencyID]
		if !ok {
			return nil, fmt.Errorf("integration usage view: selected use has no exact dependency")
		}
		position, exists := groupsByID[dependency.ID]
		if !exists {
			position = len(view.Dependencies)
			groupsByID[dependency.ID] = position
			view.Dependencies = append(view.Dependencies, IntegrationUsageDependency{
				DependencyID: dependency.ID, Language: dependency.Language,
				Kind: dependency.Kind, Name: dependency.Name, ModulePath: dependency.ModulePath,
				PackagePath: dependency.PackagePath, Uses: []IntegrationUsageUse{},
			})
		}
		view.Dependencies[position].Uses = append(
			view.Dependencies[position].Uses,
			projectIntegrationUsageUse(selectedUse),
		)
	}
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("integration usage view: invalid projection: %w", err)
	}
	return view, nil
}

// ValidateAgainst re-derives the complete report projection from the exact
// producer inputs and rejects any changed grouping, semantics, or source fact.
func (view IntegrationUsageView) ValidateAgainst(
	usage integrationusage.Result,
	index programindex.Index,
	selected integrationdependency.Result,
) error {
	if err := view.Validate(); err != nil {
		return err
	}
	expected, err := NewIntegrationUsageView(usage, index, selected)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(view, *expected) {
		return fmt.Errorf("integration usage view: projection does not match exact producer authority")
	}
	return nil
}

// Validate checks the standalone browser handoff. Exact producer equality is
// separately enforced by ValidateAgainst and the manifest verifier.
func (view IntegrationUsageView) Validate() error {
	if view.Version != IntegrationUsageViewVersion ||
		!validCubeMapViewText(view.ProgramTargetID, false) ||
		!validCubeMapViewSHA256(view.ProgramIndexSHA256) ||
		!validCubeMapViewSHA256(view.DependencyCatalogSHA256) ||
		!validCubeMapViewSHA256(view.IntegrationDependenciesSHA256) ||
		!validCubeMapViewSHA256(view.IntegrationUsageSHA256) ||
		view.Dependencies == nil || view.DeclaredCandidates == nil {
		return fmt.Errorf("integration usage view: invalid identity")
	}
	if err := validateIntegrationUsageViewCoverage(view.Coverage); err != nil {
		return err
	}
	if len(view.Dependencies) > view.Coverage.Selected ||
		len(view.Dependencies) > view.Coverage.DependenciesWithOperations {
		return fmt.Errorf("integration usage view: dependency groups exceed selected producer authority")
	}
	if err := validateIntegrationDeclarationProjection(
		view.DeclaredCandidates, view.DeclarationCoverage,
	); err != nil {
		return err
	}
	selectedUses := 0
	previousDependencyID := ""
	seenOperations := make(map[string]struct{})
	for dependencyPosition, dependency := range view.Dependencies {
		if !validCubeMapViewText(dependency.DependencyID, false) ||
			!programSemanticLanguage(dependency.Language) ||
			(dependency.Kind != dependencies.KindExternal && dependency.Kind != dependencies.KindStdlib) ||
			!validCubeMapViewText(dependency.Name, false) ||
			!validCubeMapViewText(dependency.ModulePath, true) ||
			!validCubeMapViewText(dependency.PackagePath, false) ||
			dependency.Uses == nil || len(dependency.Uses) == 0 {
			return fmt.Errorf("integration usage view: invalid published dependency %d", dependencyPosition)
		}
		if previousDependencyID != "" && previousDependencyID >= dependency.DependencyID {
			return fmt.Errorf("integration usage view: dependency groups are not canonical")
		}
		previousDependencyID = dependency.DependencyID
		previousUseKey := ""
		for usePosition, use := range dependency.Uses {
			if err := validateIntegrationUsageViewUse(use); err != nil {
				return fmt.Errorf("integration usage view: dependency %d use %d: %w", dependencyPosition, usePosition, err)
			}
			if use.DependencyID != dependency.DependencyID {
				return fmt.Errorf("integration usage view: selected use dependency join is inconsistent")
			}
			key := integrationUsageViewUseKey(dependency.DependencyID, use)
			if previousUseKey != "" && previousUseKey >= key {
				return fmt.Errorf("integration usage view: uses are not canonical")
			}
			previousUseKey = key
			if _, duplicate := seenOperations[key]; duplicate {
				return fmt.Errorf("integration usage view: duplicate selected operation")
			}
			seenOperations[key] = struct{}{}
			selectedUses++
		}
	}
	if selectedUses != view.Coverage.Selected {
		return fmt.Errorf("integration usage view: rendered uses do not match producer selected coverage")
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("integration usage view: encode bound check: %w", err)
	}
	if len(encoded) > MaxIntegrationUsageViewBytes {
		return fmt.Errorf(
			"integration usage view: JSON size %d exceeds projection limit %d",
			len(encoded), MaxIntegrationUsageViewBytes,
		)
	}
	return nil
}

func validateIntegrationDeclarationProjection(
	values []IntegrationDeclaredCandidate,
	coverage *IntegrationDeclarationCoverage,
) error {
	if coverage == nil {
		if len(values) != 0 {
			return fmt.Errorf("integration usage view: declared candidates have no coverage authority")
		}
		return nil
	}
	if err := validateIntegrationDeclarationCoverageInput(coverage.Input); err != nil {
		return err
	}
	if coverage.Advertised != coverage.Input.PackagesRetained ||
		coverage.Advertised < 0 || coverage.Advertised > integrationdependency.MaxAdvertisedDeclaredPackages ||
		coverage.Selected != len(values) || coverage.Selected > coverage.Advertised ||
		coverage.Selected > integrationdependency.MaxSelectedDeclaredPackages {
		return fmt.Errorf("integration usage view: invalid declared candidate coverage")
	}
	previous := ""
	for position, value := range values {
		if !validDeclaredPackageID(value.PackageID) ||
			!validCubeMapViewText(value.Ecosystem, false) ||
			!validCubeMapViewText(value.Name, false) ||
			!validCubeMapViewText(value.NormalizedName, false) ||
			value.Names == nil || value.SourcePaths == nil || value.Roles == nil ||
			value.Groups == nil || value.LocatorKinds == nil || value.Sections == nil ||
			len(value.Names) == 0 || value.Names[0] != value.Name ||
			len(value.SourcePaths) == 0 || len(value.Roles) == 0 ||
			len(value.LocatorKinds) == 0 || len(value.Sections) == 0 ||
			value.Statements < 1 || value.ConditionalStatements < 0 ||
			value.Statements > dependencydeclaration.MaxStatements ||
			value.ConstraintStatements < 0 ||
			value.ConditionalStatements > value.Statements ||
			value.ConstraintStatements >= value.Statements {
			return fmt.Errorf("integration usage view: invalid declared candidate %d", position)
		}
		if !canonicalDeclaredTextValues(value.Names, false) {
			return fmt.Errorf("integration usage view: invalid declared candidate names")
		}
		if !canonicalDeclaredTextValues(value.SourcePaths, true) {
			return fmt.Errorf("integration usage view: invalid declared candidate source paths")
		}
		if !canonicalDeclaredRoles(value.Roles) {
			return fmt.Errorf("integration usage view: invalid declared candidate roles")
		}
		if !canonicalDeclaredTextValues(value.Groups, false) {
			return fmt.Errorf("integration usage view: invalid declared candidate groups")
		}
		if !canonicalDeclaredLocatorKinds(value.LocatorKinds) {
			return fmt.Errorf("integration usage view: invalid declared candidate locators")
		}
		if !canonicalDeclaredTextValues(value.Sections, false) {
			return fmt.Errorf("integration usage view: invalid declared candidate sections")
		}
		key := value.Ecosystem + "\x00" + value.NormalizedName + "\x00" + value.PackageID
		if previous != "" && previous >= key {
			return fmt.Errorf("integration usage view: declared candidates are not canonical")
		}
		previous = key
	}
	return nil
}

func validateIntegrationDeclarationCoverageInput(value dependencydeclaration.Coverage) error {
	if !value.State.Valid() ||
		value.SourcesObserved < 0 || value.SourcesObserved > dependencydeclaration.MaxSources ||
		value.SourcesParsed < 0 || value.SourcesParsed > dependencydeclaration.MaxSources ||
		value.SourcesFrontier < 0 || value.SourcesFrontier > dependencydeclaration.MaxSources ||
		value.SourcesObserved != value.SourcesParsed+value.SourcesFrontier ||
		value.PackagesRetained < 0 || value.PackagesRetained > dependencydeclaration.MaxPackages ||
		value.StatementsObserved < 0 ||
		value.StatementsObserved > dependencydeclaration.MaxStatements+dependencydeclaration.MaxFrontiers ||
		value.StatementsRetained < 0 || value.StatementsRetained > dependencydeclaration.MaxStatements ||
		value.StatementsFrontier < 0 || value.StatementsFrontier > dependencydeclaration.MaxFrontiers ||
		value.StatementsObserved != value.StatementsRetained+value.StatementsFrontier ||
		value.IncludesObserved < 0 || value.IncludesObserved > dependencydeclaration.MaxIncludes ||
		value.IncludesResolved < 0 || value.IncludesResolved > dependencydeclaration.MaxIncludes ||
		value.IncludesFrontier < 0 || value.IncludesFrontier > dependencydeclaration.MaxIncludes ||
		value.IncludesObserved != value.IncludesResolved+value.IncludesFrontier ||
		value.Boundaries < value.IncludesFrontier+value.SourcesFrontier+value.StatementsFrontier ||
		value.Boundaries > dependencydeclaration.MaxFrontiers+dependencydeclaration.MaxIncludes ||
		value.Boundaries-value.IncludesFrontier > dependencydeclaration.MaxFrontiers ||
		(value.State == dependencydeclaration.CoverageComplete) !=
			(value.SourcesFrontier == 0 && value.Boundaries == 0) {
		return fmt.Errorf("integration usage view: invalid declaration input coverage")
	}
	return nil
}

func validDeclaredPackageID(value string) bool {
	const prefix = "ddpkg-"
	if len(value) != len(prefix)+24 || value[:len(prefix)] != prefix {
		return false
	}
	for _, character := range value[len(prefix):] {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}

func canonicalDeclaredTextValues(values []string, paths bool) bool {
	previous := ""
	for position, value := range values {
		valid := validCubeMapViewText(value, false)
		if paths {
			valid = validCubeMapViewPath(value)
		}
		if !valid || position > 0 && previous >= value {
			return false
		}
		previous = value
	}
	return true
}

func canonicalDeclaredRoles(values []dependencydeclaration.Role) bool {
	previous := ""
	for position, value := range values {
		if !value.Valid() || position > 0 && previous >= string(value) {
			return false
		}
		previous = string(value)
	}
	return true
}

func canonicalDeclaredLocatorKinds(values []dependencydeclaration.LocatorKind) bool {
	previous := ""
	for position, value := range values {
		if !value.Valid() || position > 0 && previous >= string(value) {
			return false
		}
		previous = string(value)
	}
	return true
}

func validateIntegrationUsageViewCoverage(value IntegrationUsageViewCoverage) error {
	if value.DependenciesObserved < 0 || value.DependenciesWithOperations < 0 ||
		value.DependenciesObserved > integrationdependency.MaxSelectedDependencies ||
		value.DependenciesWithOperations > value.DependenciesObserved ||
		value.ExternalRelationsObserved < 0 || value.CallsiteCandidatesObserved < 0 ||
		value.OperationsAdvertised < 0 ||
		value.DependenciesWithOperations > value.OperationsAdvertised ||
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
		return fmt.Errorf("integration usage view: invalid producer coverage")
	}
	return nil
}

func validateIntegrationUsageViewUse(value IntegrationUsageUse) error {
	if !validCubeMapViewText(value.DependencyID, false) ||
		!validCubeMapViewText(value.RelationID, false) || value.WitnessIndex < 0 ||
		!validCubeMapViewText(value.CallerID, false) || !value.CallerKind.Valid() ||
		!validCubeMapViewText(value.CallerName, false) ||
		!validCubeMapViewLocation(CubeMapViewLocation{
			Path: value.CallerLocation.Path, Line: value.CallerLocation.Line, Column: value.CallerLocation.Column,
		}, true) ||
		!validCubeMapViewLocation(CubeMapViewLocation{
			Path: value.Callsite.Path, Line: value.Callsite.Line, Column: value.Callsite.Column,
		}, true) ||
		!validCubeMapViewText(value.CallExpression, true) ||
		!validCubeMapViewText(value.CanonicalCallee, false) ||
		!validCubeMapViewText(value.ExternalSymbolID, false) ||
		!validCubeMapViewText(value.Invocation, true) ||
		(value.Authority != integrationusage.AuthoritySyntacticUnresolved &&
			value.Authority != integrationusage.AuthorityExactExternalSymbol) ||
		!validCubeMapViewText(value.Label, false) ||
		!validCubeMapViewText(value.Mechanism, false) {
		return fmt.Errorf("invalid selected use")
	}
	return nil
}

func projectIntegrationUsageUse(value integrationusage.Use) IntegrationUsageUse {
	operation := value.Operation
	return IntegrationUsageUse{
		DependencyID: operation.DependencyID,
		RelationID:   operation.RelationID, WitnessIndex: operation.WitnessIndex,
		CallerID: operation.CallerID, CallerKind: operation.CallerKind,
		CallerName: operation.CallerName, CallerLocation: operation.CallerLocation,
		Callsite: operation.Callsite, CallExpression: operation.CallExpression,
		CanonicalCallee: operation.CanonicalCallee, ExternalSymbolID: operation.ExternalSymbolID,
		Invocation: operation.Invocation, Authority: operation.Authority,
		Label: value.Label, Mechanism: value.Mechanism,
	}
}

func projectIntegrationUsageCoverage(value integrationusage.Coverage) IntegrationUsageViewCoverage {
	return IntegrationUsageViewCoverage{
		DependenciesObserved:       value.DependenciesObserved,
		DependenciesWithOperations: value.DependenciesWithOperations,
		ExternalRelationsObserved:  value.ExternalRelationsObserved,
		CallsiteCandidatesObserved: value.CallsiteCandidatesObserved,
		CallsiteCandidatesOmitted:  value.CallsiteCandidatesOmitted,
		OperationsAdvertised:       value.OperationsAdvertised,
		OutOfScopeCandidates:       value.OutOfScopeCandidates,
		ExactExternalRelations:     value.ExactExternalRelations,
		UnresolvedRuntimeRelations: value.UnresolvedRuntimeRelations,
		Selected:                   value.Selected, ModelCalled: value.ModelCalled,
	}
}

func projectIntegrationDeclaredCandidate(
	value integrationdependency.SelectedDeclaredPackage,
) IntegrationDeclaredCandidate {
	return IntegrationDeclaredCandidate{
		PackageID: value.PackageID, Ecosystem: value.Ecosystem, Name: value.Name,
		NormalizedName: value.NormalizedName,
		Names:          append([]string{}, value.Names...), SourcePaths: append([]string{}, value.SourcePaths...),
		Roles:        append([]dependencydeclaration.Role{}, value.Roles...),
		Groups:       append([]string{}, value.Groups...),
		LocatorKinds: append([]dependencydeclaration.LocatorKind{}, value.LocatorKinds...),
		Sections:     append([]string{}, value.Sections...), Statements: value.Statements,
		ConditionalStatements: value.ConditionalStatements,
		ConstraintStatements:  value.ConstraintStatements,
	}
}

func integrationUsageViewUseKey(dependencyID string, value IntegrationUsageUse) string {
	return fmt.Sprintf(
		"%s\x00%s\x00%08d\x00%s",
		dependencyID, value.RelationID, value.WitnessIndex, value.ExternalSymbolID,
	)
}

func cloneIntegrationUsageDependency(value dependencies.Dependency) dependencies.Dependency {
	result := value
	result.ImporterRefs = append([]string(nil), value.ImporterRefs...)
	if value.Replacement != nil {
		replacement := *value.Replacement
		result.Replacement = &replacement
	}
	return result
}
