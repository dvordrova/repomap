// Package goadapter projects already-captured Go facts into the sealed,
// language-neutral program index. It performs no source reads, package loads,
// type checking, SSA construction, or heuristic discovery.
package goadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/token"
	"path"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// Build adapts the exact Go producer snapshots into one sealed neutral program
// index. The common repository adapter path uses BuildInput and owns sealing;
// Build remains the package-level convenience entrypoint for direct callers.
func Build(
	repository *corpus.Corpus,
	target analysistarget.Target,
	packageOrigins []gofacts.PackageOrigin,
	direct surfacediscovery.DirectCallIndex,
	external surfacediscovery.ExternalCallIndex,
	core gocoreobject.Index,
	dynamic godynamichandoff.Index,
	testSources []gofacts.TestSource,
) (programindex.Index, error) {
	input, err := BuildInput(repository, target, packageOrigins, direct, external, core, dynamic, testSources)
	if err != nil {
		return programindex.Index{}, err
	}
	index, err := programindex.New(input)
	if err != nil {
		return programindex.Index{}, fmt.Errorf("Go program index adapter: seal projection: %w", err)
	}
	return index, nil
}

// BuildInput projects one complete, independently owned Go fact snapshot into
// the shared ProgramIndex input contract. Every relationship comes from an
// existing producer edge, declaration, target boundary, or closed frontier;
// BuildInput never infers a missing callee and does not seal the result.
func BuildInput(
	repository *corpus.Corpus,
	target analysistarget.Target,
	packageOrigins []gofacts.PackageOrigin,
	direct surfacediscovery.DirectCallIndex,
	external surfacediscovery.ExternalCallIndex,
	core gocoreobject.Index,
	dynamic godynamichandoff.Index,
	testSources []gofacts.TestSource,
) (programindex.Input, error) {
	if err := validateAuthority(repository, target, direct, external, core, dynamic); err != nil {
		return programindex.Input{}, err
	}
	externalAuthorityKinds, err := goExternalAuthorityKinds(packageOrigins, external)
	if err != nil {
		return programindex.Input{}, err
	}

	projection := goProjection{
		repository:           repository,
		target:               target,
		direct:               direct,
		external:             external,
		core:                 core,
		dynamic:              dynamic,
		externalAuthorities:  externalAuthorityKinds,
		objectRefs:           make(map[string]struct{}),
		moduleRefs:           make(map[string]string),
		packageRefs:          make(map[string]string),
		repositoryPackages:   make(map[string]bool),
		constructs:           make(map[string]int),
		typeRefs:             make(map[string]string),
		fieldRefs:            make(map[string]string),
		typeLocations:        make(map[string]*programindex.Location),
		methodRefs:           make(map[string]map[string]string),
		methodLocations:      make(map[string]*programindex.Location),
		directNodeObjectRefs: make(map[string]string),
		externalRefs:         make(map[string]string),
		callResultObjectRefs: make(map[string]string),
		unresolvedRelations:  make(map[string]int),
	}
	if err := projection.projectObjects(); err != nil {
		return programindex.Input{}, err
	}
	if err := projection.projectTestSources(testSources); err != nil {
		return programindex.Input{}, err
	}
	if err := projection.projectRelations(); err != nil {
		return programindex.Input{}, err
	}
	targetInput, err := projection.targetInput()
	if err != nil {
		return programindex.Input{}, err
	}
	for _, source := range testSources {
		targetInput.TestSources = append(targetInput.TestSources, source.Path)
	}
	scenarioSHA256, err := scenarioIdentity(direct.Scenario)
	if err != nil {
		return programindex.Input{}, fmt.Errorf("Go program index adapter: scenario identity: %w", err)
	}
	packageOriginsSHA256, err := canonicalSHA256(packageOrigins)
	if err != nil {
		return programindex.Input{}, fmt.Errorf("Go program index adapter: package-origin identity: %w", err)
	}
	sourceSHA256, err := canonicalSHA256(struct {
		CorpusSHA256         string               `json:"corpus_sha256"`
		TargetRef            string               `json:"target_ref"`
		PackageOriginsSHA256 string               `json:"package_origins_sha256"`
		DirectSHA256         string               `json:"direct_sha256"`
		ExternalSHA256       string               `json:"external_sha256"`
		CoreSHA256           string               `json:"core_sha256"`
		DynamicSHA256        string               `json:"dynamic_sha256"`
		TestSources          []gofacts.TestSource `json:"test_sources"`
	}{
		CorpusSHA256: repository.SHA256(), TargetRef: target.Ref,
		PackageOriginsSHA256: packageOriginsSHA256,
		DirectSHA256:         direct.SHA256, ExternalSHA256: external.SHA256, CoreSHA256: core.SHA256,
		DynamicSHA256: dynamic.SHA256, TestSources: testSources,
	})
	if err != nil {
		return programindex.Input{}, fmt.Errorf("Go program index adapter: source identity: %w", err)
	}
	objectsObserved, err := measuredCoverageCount(
		len(projection.objects),
		direct.Coverage.SyntheticFunctionsExcluded,
		direct.Coverage.InvalidFunctionsExcluded,
	)
	if err != nil {
		return programindex.Input{}, fmt.Errorf("Go program index adapter: object coverage: %w", err)
	}
	relationsObserved, err := measuredCoverageCount(
		len(projection.relations),
		direct.Coverage.InvalidEndpointCallsExcluded,
		direct.Coverage.InvalidCallsitesExcluded,
		dynamic.Coverage.HandoffsOmitted,
	)
	if err != nil {
		return programindex.Input{}, fmt.Errorf("Go program index adapter: relation coverage: %w", err)
	}

	var fieldWrites []programindex.FieldWrites
	for _, writes := range direct.FieldWrites {
		fieldWrites = append(fieldWrites, programindex.FieldWrites{Field: writes.Field, Value: writes.Value})
	}
	return programindex.Input{
		ScenarioSHA256: scenarioSHA256,
		SourceSHA256:   sourceSHA256,
		Target:         targetInput,
		Objects:        projection.objects,
		Relations:      projection.relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: objectsObserved, RelationsObserved: relationsObserved,
		},
		FieldWrites: fieldWrites,
	}, nil
}

func measuredCoverageCount(retained int, omitted ...int) (int, error) {
	if retained < 0 {
		return 0, fmt.Errorf("retained count is outside bounds")
	}
	result := retained
	maxInt := int(^uint(0) >> 1)
	for _, count := range omitted {
		if count < 0 || count > maxInt-result {
			return 0, fmt.Errorf("omission count is outside bounds")
		}
		result += count
	}
	return result, nil
}

func goExternalAuthorityKinds(
	packageOrigins []gofacts.PackageOrigin,
	external surfacediscovery.ExternalCallIndex,
) (map[string]programindex.ExternalAuthorityKind, error) {
	if err := gofacts.ValidatePackageOrigins(packageOrigins); err != nil {
		return nil, fmt.Errorf("Go program index adapter: package-origin authority: %w", err)
	}
	result := make(map[string]programindex.ExternalAuthorityKind, len(packageOrigins)+1)
	for _, origin := range packageOrigins {
		kind := programindex.ExternalAuthorityPackage
		if origin.Standard {
			kind = programindex.ExternalAuthorityPlatform
		}
		result[origin.PackagePath] = kind
	}
	for _, family := range external.Families {
		if generatedCgoTarget(family.Target) {
			result[family.Target.PackagePath] = programindex.ExternalAuthorityPlatform
			continue
		}
		if _, exists := result[family.Target.PackagePath]; !exists {
			return nil, fmt.Errorf(
				"Go program index adapter: external target package %q has no exact go-list origin authority",
				family.Target.PackagePath,
			)
		}
	}
	return result, nil
}

func scenarioIdentity(scenario surfacediscovery.Scenario) (string, error) {
	return canonicalSHA256(struct {
		ID      string   `json:"id"`
		GOOS    string   `json:"goos"`
		GOARCH  string   `json:"goarch"`
		GoFlags string   `json:"go_flags"`
		Tags    []string `json:"tags"`
	}{
		ID: scenario.ID, GOOS: scenario.GOOS, GOARCH: scenario.GOARCH,
		GoFlags: scenario.GoFlags, Tags: append([]string(nil), scenario.Tags...),
	})
}

type goProjection struct {
	// implementationFrontiers counts, per external-implementation call site,
	// the receiver values its folded SSA view left unknown.
	implementationFrontiers map[string]int
	repository              *corpus.Corpus
	target                  analysistarget.Target
	direct                  surfacediscovery.DirectCallIndex
	external                surfacediscovery.ExternalCallIndex
	core                    gocoreobject.Index
	dynamic                 godynamichandoff.Index
	externalAuthorities     map[string]programindex.ExternalAuthorityKind

	objects   []programindex.ObjectInput
	relations []programindex.RelationInput

	objectRefs  map[string]struct{}
	moduleRefs  map[string]string
	packageRefs map[string]string
	// repositoryPackages are the import paths this target's modules own; a
	// value of a type from any other package is constructed outside them.
	repositoryPackages map[string]bool
	// constructs indexes the construction relations by caller and literal so
	// two callables bound into one literal share it.
	constructs           map[string]int
	typeRefs             map[string]string
	fieldRefs            map[string]string // struct fields by type key and field name
	typeLocations        map[string]*programindex.Location
	methodRefs           map[string]map[string]string
	methodLocations      map[string]*programindex.Location
	directNodeObjectRefs map[string]string
	externalRefs         map[string]string
	callResultObjectRefs map[string]string
	unresolvedRelations  map[string]int
}

func (projection *goProjection) projectObjects() error {
	for _, pkg := range projection.core.Packages {
		moduleRef, exists := projection.moduleRefs[pkg.ModuleID]
		if !exists {
			moduleRef = pkg.ModuleID
			projection.moduleRefs[pkg.ModuleID] = moduleRef
			if err := projection.addObject(programindex.ObjectInput{
				SourceRef: moduleRef, Kind: programindex.ObjectModule, Name: pkg.Module,
				Visibility: programindex.VisibilityPublic,
			}); err != nil {
				return err
			}
		}
		packageRef := stableRef("go-package", pkg.ModuleID, pkg.Path)
		projection.packageRefs[packageKey(pkg.ModuleID, pkg.Path)] = packageRef
		projection.repositoryPackages[pkg.Path] = true
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: packageRef, Kind: programindex.ObjectPackage, Name: pkg.Path,
			Visibility: goPackageVisibility(pkg.Path), OwnerRef: moduleRef, ContainerRef: moduleRef,
		}); err != nil {
			return err
		}
	}

	for _, declaration := range projection.core.Types {
		packageRef, err := projection.packageRef(declaration.Package)
		if err != nil {
			return err
		}
		location, err := projection.coreLocation(declaration.Location)
		if err != nil {
			return err
		}
		projection.typeRefs[typeKey(declaration.Package, declaration.Name)] = declaration.ID
		projection.typeLocations[declaration.ID] = location
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: declaration.ID, Kind: programindex.ObjectType, Name: declaration.Name,
			Signature:  declaration.Signature,
			Visibility: visibility(declaration.Exported), OwnerRef: packageRef, ContainerRef: packageRef,
			Location: location, EndLine: declaration.EndLine, CodeLines: declaration.CodeLines,
		}); err != nil {
			return err
		}
		for _, field := range declaration.Fields {
			fieldLocation, err := projection.coreLocation(field.Location)
			if err != nil {
				return err
			}
			var fieldTypes []programindex.Location
			for _, declared := range field.Types {
				// A type declared outside the corpus is none of its types.
				if location, err := projection.coreLocation(declared); err == nil {
					fieldTypes = append(fieldTypes, *location)
				}
			}
			if err := projection.addObject(programindex.ObjectInput{
				SourceRef: field.ID, Kind: programindex.ObjectVariable, Name: field.Name,
				Signature: shortSignature(field.Signature), Aliases: tagAliases(field.Tag), Types: fieldTypes, Visibility: visibility(field.Exported),
				OwnerRef: declaration.ID, ContainerRef: declaration.ID, Location: fieldLocation,
			}); err != nil {
				return err
			}
			projection.fieldRefs[fieldKey(declaration.Package, declaration.Name, field.Name)] = field.ID
		}
	}

	for _, declaration := range projection.core.Callables {
		packageRef, err := projection.packageRef(declaration.Package)
		if err != nil {
			return err
		}
		location, err := projection.coreLocation(declaration.Location)
		if err != nil {
			return err
		}
		kind := programindex.ObjectFunction
		ownerRef, containerRef := packageRef, packageRef
		receiverName := ""
		if declaration.Kind == gocoreobject.CallableMethod {
			kind = programindex.ObjectMethod
			typeName, ok := receiverTypeName(declaration.Receiver, declaration.Package)
			if !ok {
				return fmt.Errorf(
					"Go program index adapter: method %q has unsupported exact receiver %q",
					declaration.ID, declaration.Receiver,
				)
			}
			typeRef, exists := projection.typeRefs[typeKey(declaration.Package, typeName)]
			if !exists {
				return fmt.Errorf(
					"Go program index adapter: method %q receiver type %q is absent from core objects",
					declaration.ID, typeName,
				)
			}
			ownerRef, containerRef = typeRef, typeRef
			receiverName = typeName
		}
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: declaration.ID, Kind: kind, Name: declaration.Name,
			Visibility: visibility(declaration.Exported), Signature: shortSignature(declaration.Signature),
			OwnerRef: ownerRef, ContainerRef: containerRef, Location: location, EndLine: declaration.EndLine, CodeLines: declaration.CodeLines,
			Parameters: projection.typedNames(declaration.Parameters), Results: projection.typedNames(declaration.Results),
		}); err != nil {
			return err
		}
		if kind == programindex.ObjectMethod {
			key := typeKey(declaration.Package, receiverName)
			if projection.methodRefs[key] == nil {
				projection.methodRefs[key] = make(map[string]string)
			}
			if _, duplicate := projection.methodRefs[key][declaration.Name]; duplicate {
				return fmt.Errorf("Go program index adapter: duplicate method %s.%s", key, declaration.Name)
			}
			projection.methodRefs[key][declaration.Name] = declaration.ID
			projection.methodLocations[declaration.ID] = location
		}
		if declaration.DirectCallNodeID != "" {
			if _, duplicate := projection.directNodeObjectRefs[declaration.DirectCallNodeID]; duplicate {
				return fmt.Errorf(
					"Go program index adapter: duplicate callable binding for direct node %q",
					declaration.DirectCallNodeID,
				)
			}
			projection.directNodeObjectRefs[declaration.DirectCallNodeID] = declaration.ID
		}
	}

	for _, node := range projection.direct.Nodes {
		if _, merged := projection.directNodeObjectRefs[node.ID]; merged {
			continue
		}
		packageRef, err := projection.packageRef(node.Package)
		if err != nil {
			return err
		}
		location, err := projection.surfaceLocation(node.Declaration)
		if err != nil {
			return err
		}
		// A function literal (Open$1) is anonymous: a reader looks up the
		// function holding it, never a name go/ssa made up.
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: node.ID, Kind: programindex.ObjectFunction, Name: node.Symbol.Name,
			Signature:  shortSignature(node.Signature),
			Visibility: visibility(node.Exported), OwnerRef: packageRef, ContainerRef: packageRef,
			Location: location, Anonymous: node.Anonymous,
		}); err != nil {
			return err
		}
		projection.directNodeObjectRefs[node.ID] = node.ID
	}

	// A package-level variable whose initializer calls repository code is
	// the caller of those calls (GO): a declaration of its package.
	for _, variable := range projection.direct.Variables {
		packageRef, err := projection.packageRef(variable.Package)
		if err != nil {
			return err
		}
		location, err := projection.surfaceLocation(variable.Declaration)
		if err != nil {
			return err
		}
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: variable.ID, Kind: programindex.ObjectVariable, Name: variable.Symbol.Name,
			Signature:  shortSignature(variable.Signature),
			Visibility: visibility(variable.Exported), OwnerRef: packageRef, ContainerRef: packageRef,
			Location: location, EndLine: variable.Body.End.Line, CodeLines: variable.CodeLines,
		}); err != nil {
			return err
		}
		projection.directNodeObjectRefs[variable.ID] = variable.ID
	}

	for _, family := range projection.external.Families {
		key := externalTargetKey(family.Target)
		if _, exists := projection.externalRefs[key]; exists {
			continue
		}
		ref := stableRef(
			"go-external-symbol", family.Target.PackagePath,
			family.Target.Receiver, family.Target.Name,
		)
		projection.externalRefs[key] = ref
		objectVisibility := visibility(token.IsExported(family.Target.Name))
		if generatedCgoTarget(family.Target) {
			// The target is an exact compiler-generated wrapper boundary, not a
			// portable repository or dependency declaration.
			objectVisibility = programindex.VisibilityUnknown
		}
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: ref, Kind: programindex.ObjectExternalSymbol,
			Name: externalTargetName(family.Target), Visibility: objectVisibility, Signature: shortSignature(family.Target.Signature),
			External: &programindex.ExternalSymbol{
				AuthorityKind: projection.externalAuthorities[family.Target.PackagePath],
				PackagePath:   family.Target.PackagePath,
				Receiver:      family.Target.Receiver,
				Name:          family.Target.Name,
			},
		}); err != nil {
			return err
		}
	}
	if err := projection.projectCallResultObjects(); err != nil {
		return err
	}
	return nil
}

// projectCallResultObjects materializes the intersection of call results cited
// as receivers and call results whose producer patterns are retained in this
// target projection. A cited result without its producer remains an unresolved
// receiver frontier when callPatterns projects the consumer. These are exact
// syntactic values, not declarations and not claims that either call executes.
func (projection *goProjection) projectCallResultObjects() error {
	used := make(map[string]struct{})
	visit := func(patterns []surfacediscovery.ExternalCallPattern) {
		for _, pattern := range patterns {
			for _, resultID := range pattern.ReceiverResultIDs {
				used[resultID] = struct{}{}
			}
		}
	}
	for _, edge := range projection.direct.Edges {
		visit(edge.Patterns)
	}
	for _, family := range projection.external.Families {
		visit(family.Patterns)
	}
	type resultFact struct {
		id        string
		kind      programindex.ObjectKind
		name      string
		signature string
		location  surfacediscovery.Location
	}
	facts := make(map[string]resultFact, len(used))
	collect := func(patterns []surfacediscovery.ExternalCallPattern) error {
		for _, pattern := range patterns {
			if _, needed := used[pattern.ResultID]; !needed || pattern.ResultID == "" {
				continue
			}
			fact := resultFact{
				id: pattern.ResultID, kind: programindex.ObjectVariable,
				name: "call result", signature: pattern.ResultType, location: pattern.Callsite,
			}
			if previous, exists := facts[fact.id]; exists && previous != fact {
				return fmt.Errorf("Go program index adapter: conflicting call result %q", fact.id)
			}
			facts[fact.id] = fact
		}
		return nil
	}
	for _, edge := range projection.direct.Edges {
		if err := collect(edge.Patterns); err != nil {
			return err
		}
	}
	for _, family := range projection.external.Families {
		if err := collect(family.Patterns); err != nil {
			return err
		}
	}
	ids := make([]string, 0, len(facts))
	for id := range facts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fact := facts[id]
		location, err := projection.surfaceLocation(fact.location)
		if err != nil {
			return err
		}
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: fact.id, Kind: fact.kind, Name: fact.name,
			Visibility: programindex.VisibilityInternal, Signature: shortSignature(fact.signature),
			Location: location,
		}); err != nil {
			return err
		}
		projection.callResultObjectRefs[id] = id
	}
	return nil
}

func (projection *goProjection) projectRelations() error {
	if err := projection.projectInterfaceImplementations(); err != nil {
		return err
	}
	for _, edge := range projection.direct.Edges {
		fromRef, fromOK := projection.directNodeObjectRefs[edge.CallerID]
		toRef, toOK := projection.directNodeObjectRefs[edge.CalleeID]
		if !fromOK || !toOK {
			return fmt.Errorf("Go program index adapter: direct edge %q has no projected endpoint", edge.ID)
		}
		location, err := projection.surfaceLocation(edge.RepresentativeCallsite)
		if err != nil {
			return err
		}
		callee, ok := projection.direct.Node(edge.CalleeID)
		if !ok {
			return fmt.Errorf("Go program index adapter: direct edge %q has no callee", edge.ID)
		}
		patterns, err := projection.callPatterns(edge.Patterns, callee.Symbol.Name, edge.ID)
		if err != nil {
			return err
		}
		witnesses := make([]programindex.Witness, 0, len(edge.Patterns))
		for _, pattern := range edge.Patterns {
			patternLocation, locationErr := projection.surfaceLocation(pattern.Callsite)
			if locationErr != nil {
				return locationErr
			}
			witnesses = append(witnesses, programindex.Witness{
				Kind: "go_direct_call", Detail: locationDetail(pattern.Callsite), Location: patternLocation,
			})
		}
		if len(witnesses) == 0 {
			witnesses = append(witnesses, programindex.Witness{
				Kind: "go_direct_call", Detail: locationDetail(edge.RepresentativeCallsite), Location: location,
			})
		}
		guard, err := projection.guard(edge.Guard)
		if err != nil {
			return err
		}
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef: edge.ID, Kind: programindex.RelationCalls,
			FromRef: fromRef, ToRefs: []string{toRef}, Resolution: programindex.ResolutionExact,
			Invocation: goInvocation(string(edge.Invocation)), Location: location, TargetsObserved: 1,
			Witnesses: witnesses, WitnessesObserved: len(witnesses),
			Patterns: patterns, PatternsObserved: edge.PatternsObserved, Guard: guard,
		})
	}
	if err := projection.projectFieldAccesses(); err != nil {
		return err
	}
	if err := projection.projectComparisons(); err != nil {
		return err
	}
	dynamicRepresented, err := projection.projectDynamicHandoffs()
	if err != nil {
		return err
	}

	directFrontiers := make(map[string]surfacediscovery.DirectCallNodeFrontier, len(projection.direct.Frontiers))
	for _, frontier := range projection.direct.Frontiers {
		fromRef, ok := projection.directNodeObjectRefs[frontier.CallerID]
		if !ok {
			return fmt.Errorf("Go program index adapter: direct frontier has no caller %q", frontier.CallerID)
		}
		directFrontiers[frontier.CallerID] = frontier
		represented := dynamicRepresented[frontier.CallerID]
		projection.addUnresolved(
			fromRef, programindex.RelationCalls, "dynamic", "go_dynamic_invoke",
			positiveDifference(frontier.DynamicInvokesExcluded, represented.interfaceInvokes),
		)
		projection.addUnresolved(
			fromRef, programindex.RelationCalls, "non_static", "go_non_static_call",
			positiveDifference(frontier.NonStaticCallsExcluded, represented.functionValueCalls),
		)
		projection.addUnresolved(fromRef, programindex.RelationCalls, "depth_bound", "go_depth_bound_call", frontier.DepthBoundRepositoryCallsExcluded)
	}

	for _, family := range projection.external.Families {
		fromRef, ok := projection.directNodeObjectRefs[family.CallerID]
		if !ok {
			return fmt.Errorf("Go program index adapter: external family %q has no caller", family.ID)
		}
		toRef, ok := projection.externalRefs[externalTargetKey(family.Target)]
		if !ok {
			return fmt.Errorf("Go program index adapter: external family %q has no target", family.ID)
		}
		invocation, dispatch := goInvocation(string(family.Invocation)), ""
		witnessKind := "go_external_static_call"
		resolution := programindex.ResolutionExact
		if family.Dispatch == surfacediscovery.ExternalCallInterfaceImplementation {
			// The observed external value is one possible receiver of a
			// repository interface, like a repository implementation would be.
			dispatch = programindex.DispatchInterface
			witnessKind = "go_interface_external_implementation"
			resolution = programindex.ResolutionAlternatives
		} else if generatedCgoTarget(family.Target) {
			witnessKind = "go_generated_cgo_wrapper_call"
		} else if family.Dispatch == surfacediscovery.ExternalCallInterfaceInvoke {
			// The exact target is the declared interface method, never an
			// inferred runtime implementation.
			dispatch = programindex.DispatchInterfaceMethod
			witnessKind = "go_declared_interface_dispatch"
		}
		witnesses := make([]programindex.Witness, 0, len(family.Callsites))
		var relationLocation *programindex.Location
		for _, callsite := range family.Callsites {
			location, err := projection.surfaceLocation(callsite)
			if err != nil {
				return err
			}
			if relationLocation == nil && location != nil {
				relationLocation = location
			}
			witnesses = append(witnesses, programindex.Witness{
				Kind: witnessKind, Detail: locationDetail(callsite), Location: location,
			})
		}
		patterns, err := projection.callPatterns(family.Patterns, family.Target.Name, family.ID)
		if err != nil {
			return err
		}
		targetsObserved := 1
		if family.Dispatch == surfacediscovery.ExternalCallInterfaceImplementation {
			// Other values of the interface stay an explicit omission here,
			// as they do on a repository implementation's alternatives.
			unknown := 0
			for _, callsite := range family.Callsites {
				unknown = max(unknown, projection.implementationFrontiers[declaredDispatchKey(family.CallerID, callsite.Path, callsite.Line, callsite.Column)])
			}
			targetsObserved += unknown
			if unknown == 0 {
				resolution = programindex.ResolutionExact
			}
		}
		guard, err := projection.sitesGuard(family.Callsites)
		if err != nil {
			return err
		}
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef: family.ID, Kind: programindex.RelationInvokesExternal,
			FromRef: fromRef, ToRefs: []string{toRef}, Resolution: resolution,
			Invocation: invocation, Dispatch: dispatch, Location: relationLocation, TargetsObserved: targetsObserved,
			Witnesses: witnesses, WitnessesObserved: len(witnesses),
			Patterns: patterns, PatternsObserved: family.PatternsObserved, Guard: guard,
		})
	}

	for _, frontier := range projection.external.Frontiers {
		fromRef, ok := projection.directNodeObjectRefs[frontier.CallerID]
		if !ok {
			return fmt.Errorf("Go program index adapter: external frontier has no caller %q", frontier.CallerID)
		}
		directFrontier := directFrontiers[frontier.CallerID]
		represented := dynamicRepresented[frontier.CallerID]
		projection.addUnresolved(
			fromRef, programindex.RelationCalls, "dynamic", "go_dynamic_invoke",
			positiveDifference(
				frontier.DynamicInvokesExcluded,
				max(directFrontier.DynamicInvokesExcluded, represented.interfaceInvokes),
			),
		)
		projection.addUnresolved(
			fromRef, programindex.RelationCalls, "non_static", "go_non_static_call",
			positiveDifference(
				frontier.NonStaticCallsExcluded,
				max(directFrontier.NonStaticCallsExcluded, represented.functionValueCalls),
			),
		)
		projection.addUnresolved(
			fromRef, programindex.RelationInvokesExternal, "unnamed_static", "go_unnamed_external_callee",
			frontier.UnnamedStaticCalleesExcluded,
		)
		projection.addUnresolved(
			fromRef, programindex.RelationInvokesExternal, "invalid_callsite", "go_external_invalid_callsite",
			frontier.InvalidCallsitesExcluded,
		)
	}

	for _, frontier := range projection.external.PackageFrontiers {
		fromRef, ok := projection.packageRefs[packageKey(frontier.ModuleID, frontier.PackagePath)]
		if !ok {
			return fmt.Errorf(
				"Go program index adapter: external package frontier has no package %q",
				frontier.PackagePath,
			)
		}
		projection.addUnresolved(
			fromRef, programindex.RelationInvokesExternal, "synthetic_caller", "go_synthetic_external_caller",
			frontier.SyntheticCallerWitnessesExcluded,
		)
		projection.addUnresolved(
			fromRef, programindex.RelationInvokesExternal, "invalid_caller", "go_invalid_external_caller",
			frontier.InvalidCallerWitnessesExcluded,
		)
	}
	return nil
}

// projectFieldAccesses turns each read and write of a repository struct's
// field a node's body names into a reads or writes relation, one per site,
// exact, whose target is the field object and whose field_path is the field
// as the code reaches it (GO, PROGRAM_INDEX).
func (projection *goProjection) projectFieldAccesses() error {
	for _, access := range projection.direct.FieldAccesses {
		fromRef, ok := projection.directNodeObjectRefs[access.CallerID]
		if !ok {
			return fmt.Errorf("Go program index adapter: field access has no projected caller %q", access.CallerID)
		}
		fieldRef, ok := projection.fieldRefs[fieldKey(access.Package, access.Type, access.Field)]
		if !ok {
			return fmt.Errorf("Go program index adapter: field access names %s.%s.%s, absent from core objects", access.Package, access.Type, access.Field)
		}
		location, err := projection.surfaceLocation(access.Site)
		if err != nil {
			return err
		}
		kind, witness, verb := programindex.RelationReads, "go_field_read", "read of "
		if access.Write {
			kind, witness, verb = programindex.RelationWrites, "go_field_write", "write of "
		}
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef: stableRef("go-field-access", access.CallerID, fieldRef, string(kind), locationDetail(access.Site)),
			Kind:      kind, FromRef: fromRef, ToRefs: []string{fieldRef}, Resolution: programindex.ResolutionExact,
			Location: location, TargetsObserved: 1,
			Witnesses:         []programindex.Witness{{Kind: witness, Detail: verb + access.Path, Location: location}},
			WitnessesObserved: 1, FieldPath: access.Path,
		})
	}
	return nil
}

// projectComparisons gives each node the values its body compares with two
// or more different words (GO, PROGRAM_INDEX Comparison).
func (projection *goProjection) projectComparisons() error {
	if len(projection.direct.Comparisons) == 0 {
		return nil
	}
	objects := make(map[string]int, len(projection.objects))
	for position, object := range projection.objects {
		objects[object.SourceRef] = position
	}
	for _, comparison := range projection.direct.Comparisons {
		ref, ok := projection.directNodeObjectRefs[comparison.CallerID]
		if !ok {
			return fmt.Errorf("Go program index adapter: comparison has no projected caller %q", comparison.CallerID)
		}
		position, ok := objects[ref]
		if !ok {
			return fmt.Errorf("Go program index adapter: comparison caller %q has no object", comparison.CallerID)
		}
		location, err := projection.surfaceLocation(comparison.Site)
		if err != nil {
			return err
		}
		value := programindex.Comparison{Value: comparison.Value, Origin: sourcevalue.Clone(comparison.Origin), Location: location}
		for _, item := range comparison.Cases {
			at, err := projection.surfaceLocation(item.Site)
			if err != nil {
				return err
			}
			written := programindex.ComparisonCase{Form: programindex.ComparisonForm(item.Form), Words: slices.Clone(item.Words), Location: at, Exclusive: item.Exclusive}
			if item.BranchLine > 0 {
				written.Branch = &programindex.LineRange{Line: item.BranchLine, EndLine: item.BranchEnd, Column: item.BranchColumn, EndColumn: item.BranchEndColumn}
			}
			value.Cases = append(value.Cases, written)
		}
		projection.objects[position].Comparisons = append(projection.objects[position].Comparisons, value)
	}
	return nil
}

func (projection *goProjection) projectInterfaceImplementations() error {
	for _, match := range projection.core.InterfaceImplementations {
		interfaceRef, interfaceOK := projection.typeRefs[typeKey(match.InterfacePackage, match.InterfaceName)]
		implementationRef, implementationOK := projection.typeRefs[typeKey(match.ImplementationPackage, match.ImplementationName)]
		if !interfaceOK || !implementationOK {
			return fmt.Errorf("Go program index adapter: interface implementation has no projected type endpoint")
		}
		location := projection.typeLocations[implementationRef]
		witnesses := make([]programindex.Witness, 0, 2)
		if match.ValueReceiver {
			witnesses = append(witnesses, programindex.Witness{Kind: "go_interface_implementation", Detail: "value method set", Location: location})
		}
		if match.PointerReceiver {
			witnesses = append(witnesses, programindex.Witness{Kind: "go_interface_implementation", Detail: "pointer method set", Location: location})
		}
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef: stableRef("go-interface-implementation", implementationRef, interfaceRef),
			Kind:      programindex.RelationImplements, FromRef: implementationRef, ToRefs: []string{interfaceRef},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: location,
			Witnesses: witnesses, WitnessesObserved: len(witnesses),
		})

		interfaceMethods := projection.methodRefs[typeKey(match.InterfacePackage, match.InterfaceName)]
		implementationMethods := projection.methodRefs[typeKey(match.ImplementationPackage, match.ImplementationName)]
		for name, interfaceMethodRef := range interfaceMethods {
			implementationMethodRef := implementationMethods[name]
			if implementationMethodRef == "" {
				continue
			}
			methodLocation := projection.methodLocations[implementationMethodRef]
			projection.relations = append(projection.relations, programindex.RelationInput{
				SourceRef: stableRef("go-interface-method-implementation", implementationMethodRef, interfaceMethodRef),
				Kind:      programindex.RelationImplements, FromRef: implementationMethodRef, ToRefs: []string{interfaceMethodRef},
				Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: methodLocation,
				Witnesses:         []programindex.Witness{{Kind: "go_interface_method_implementation", Detail: "method set match", Location: methodLocation}},
				WitnessesObserved: 1,
			})
		}
	}
	return nil
}

// guard is a call edge's folded guard as the ProgramIndex says it (GO).
func (projection *goProjection) guard(value *surfacediscovery.CallGuard) (*programindex.Guard, error) {
	if value == nil {
		return nil, nil
	}
	location, err := projection.surfaceLocation(value.Location)
	if err != nil {
		return nil, err
	}
	condition, when := programindex.GuardCondition(value.Condition, value.When)
	return &programindex.Guard{Kind: value.Kind, Location: location, Condition: condition, When: when}, nil
}

// sitesGuard folds the guards of a relation's call sites: one unguarded
// site, or one the walk did not see, leaves none.
func (projection *goProjection) sitesGuard(callsites []surfacediscovery.Location) (*programindex.Guard, error) {
	guards := make([]*programindex.Guard, 0, len(callsites))
	for _, callsite := range callsites {
		guard, ok := projection.direct.CallGuards[callsite]
		if !ok {
			return nil, nil
		}
		value, err := projection.guard(&guard)
		if err != nil {
			return nil, err
		}
		guards = append(guards, value)
	}
	return programindex.WeakestGuard(guards), nil
}

func (projection *goProjection) callPatterns(
	values []surfacediscovery.ExternalCallPattern,
	selector, relationRef string,
) ([]programindex.RelationPatternInput, error) {
	result := make([]programindex.RelationPatternInput, 0, len(values))
	kept := make(map[string]bool, len(values))
	for _, pattern := range values {
		kept[pattern.ID] = true
	}
	for _, pattern := range values {
		location, err := projection.surfaceLocation(pattern.Callsite)
		if err != nil {
			return nil, err
		}
		arguments := make([]programindex.PatternArgumentInput, 0, len(pattern.Arguments))
		for _, argument := range pattern.Arguments {
			arguments = append(arguments, projection.externalCallPatternArgument(argument))
		}
		resultRef := ""
		if ref, ok := projection.callResultObjectRefs[pattern.ResultID]; ok {
			resultRef = ref
		}
		receiverRef := ""
		receiverOriginRefs := make([]string, 0, len(pattern.ReceiverResultIDs))
		for _, resultID := range pattern.ReceiverResultIDs {
			if ref, ok := projection.callResultObjectRefs[resultID]; ok {
				receiverOriginRefs = append(receiverOriginRefs, ref)
			}
		}
		sort.Strings(receiverOriginRefs)
		receiverOriginRefs = slices.Compact(receiverOriginRefs)
		receiverResolution := programindex.Resolution("")
		receiverOriginsObserved := pattern.ReceiversObserved
		if len(receiverOriginRefs) == 1 && pattern.ReceiversObserved == 1 && pattern.ReceiversOmitted == 0 {
			receiverRef = receiverOriginRefs[0]
			receiverOriginRefs = []string{}
			receiverOriginsObserved = 0
		} else if pattern.ReceiversObserved > 0 {
			switch {
			case len(receiverOriginRefs) == 0:
				receiverResolution = programindex.ResolutionUnresolved
			default:
				receiverResolution = programindex.ResolutionAlternatives
			}
		}
		var control []programindex.Witness
		for _, context := range pattern.Context {
			location, err := projection.surfaceLocation(context.Location)
			if err != nil {
				return nil, err
			}
			control = append(control, programindex.Witness{Kind: "control_context", Detail: context.Kind, Location: location})
		}
		// The call it reads the same value as is a call of the same callee
		// from the same caller, so of this relation; a call it names that
		// the relation did not keep names nothing.
		var sameValueAs *programindex.PatternRefInput
		if pattern.SameValueAs != "" && kept[pattern.SameValueAs] {
			sameValueAs = &programindex.PatternRefInput{RelationSourceRef: relationRef, PatternSourceRef: pattern.SameValueAs}
		}
		result = append(result, programindex.RelationPatternInput{
			ReceiverValue: sourcevalue.Clone(pattern.ReceiverValue), ResultValue: sourcevalue.Clone(pattern.ResultValue),
			Context: control, SameValueAs: sameValueAs,
			SourceRef: pattern.ID, Form: programindex.PatternCall, Selector: selector,
			Location:  location,
			ResultRef: resultRef, ReceiverRef: receiverRef,
			ReceiverOriginRefs:       receiverOriginRefs,
			ReceiverOriginResolution: receiverResolution,
			ReceiverOriginsObserved:  receiverOriginsObserved,
			Arguments:                arguments, ArgumentsObserved: pattern.ArgumentsObserved,
		})
	}
	return result, nil
}

func (projection *goProjection) externalCallPatternArgument(
	argument surfacediscovery.ExternalCallPatternArgument,
) programindex.PatternArgumentInput {
	objectRefs := make([]string, 0, len(argument.ObjectIDs))
	for _, objectID := range argument.ObjectIDs {
		if ref, ok := projection.directNodeObjectRefs[objectID]; ok {
			objectRefs = append(objectRefs, ref)
		}
	}
	sort.Strings(objectRefs)
	objectRefs = slices.Compact(objectRefs)
	resolution := programindex.Resolution("")
	if argument.ObjectsObserved > 0 {
		switch {
		case len(objectRefs) == 0:
			resolution = programindex.ResolutionUnresolved
		case len(objectRefs) == 1 && argument.ObjectsObserved == 1:
			resolution = programindex.ResolutionExact
		default:
			resolution = programindex.ResolutionAlternatives
		}
	}
	return programindex.PatternArgumentInput{
		Origin:   sourcevalue.Clone(argument.Origin),
		Position: argument.Position, Kind: programindex.PatternValueKind(argument.Kind),
		Value: argument.Value, ObjectRefs: objectRefs, Resolution: resolution,
		ObjectsObserved: argument.ObjectsObserved,
	}
}

type dynamicHandoffRepresentation struct {
	interfaceInvokes   int
	functionValueCalls int
}

func (projection *goProjection) projectDynamicHandoffs() (
	map[string]dynamicHandoffRepresentation,
	error,
) {
	represented := make(map[string]dynamicHandoffRepresentation)
	functionNames := make(map[string]string, len(projection.dynamic.Functions))
	for _, function := range projection.dynamic.Functions {
		functionNames[function.ID] = function.Symbol
	}
	// A call of a method declared on an external interface is already one
	// invokes_external fact naming that method, and an external implementation
	// of a repository interface is one naming the implementation. An SSA view
	// of the same site that found nothing more adds nothing and is not projected.
	// The witnesses of a field a branch left open are something more.
	declaredDispatch := make(map[string]bool)
	externalImplementation := make(map[string]bool)
	for _, family := range projection.external.Families {
		for _, callsite := range family.Callsites {
			key := declaredDispatchKey(family.CallerID, callsite.Path, callsite.Line, callsite.Column)
			switch family.Dispatch {
			case surfacediscovery.ExternalCallInterfaceInvoke:
				declaredDispatch[key] = true
			case surfacediscovery.ExternalCallInterfaceImplementation:
				externalImplementation[key] = true
			}
		}
	}
	for _, handoff := range projection.dynamic.Handoffs {
		key := declaredDispatchKey(handoff.CallerID, handoff.Callsite.Path, handoff.Callsite.Line, handoff.Callsite.Column)
		if handoff.Kind == godynamichandoff.InterfaceInvoke && len(handoff.Candidates) == 0 && len(handoff.Witnesses) == 0 &&
			(declaredDispatch[key] || externalImplementation[key]) {
			if externalImplementation[key] {
				if projection.implementationFrontiers == nil {
					projection.implementationFrontiers = make(map[string]int)
				}
				projection.implementationFrontiers[key] += handoff.CandidatesConsidered
			}
			counts := represented[handoff.CallerID]
			counts.interfaceInvokes++
			represented[handoff.CallerID] = counts
			continue
		}
		fromRef, ok := projection.directNodeObjectRefs[handoff.CallerID]
		if !ok {
			return nil, fmt.Errorf(
				"Go program index adapter: dynamic handoff %q has no projected caller",
				handoff.ID,
			)
		}
		toRefs := make([]string, 0, len(handoff.Candidates))
		for _, candidate := range handoff.Candidates {
			toRef, exists := projection.directNodeObjectRefs[candidate.FunctionID]
			if !exists {
				return nil, fmt.Errorf(
					"Go program index adapter: dynamic handoff %q has no projected candidate",
					handoff.ID,
				)
			}
			toRefs = append(toRefs, toRef)
		}
		resolution, err := programResolution(handoff.Resolution)
		if err != nil {
			return nil, fmt.Errorf("Go program index adapter: dynamic handoff %q: %w", handoff.ID, err)
		}
		location, err := projection.dynamicLocation(handoff.Callsite)
		if err != nil {
			return nil, err
		}
		targetsObserved := handoff.CandidatesConsidered
		if resolution == programindex.ResolutionUnresolved && targetsObserved == 0 {
			// One runtime target position was observed at the exact joint; its
			// implementation/value is deliberately unresolved.
			targetsObserved = 1
		}
		kind := programindex.RelationCalls
		if handoff.Kind == godynamichandoff.CallbackTransfer ||
			handoff.Kind == godynamichandoff.CallableBinding {
			kind = programindex.RelationPassesCallback
		}
		invocation, dispatch := goInvocation(string(handoff.Invocation)), ""
		switch handoff.Kind {
		case godynamichandoff.InterfaceInvoke:
			dispatch = programindex.DispatchInterface
		case godynamichandoff.FunctionValueCall:
			dispatch = programindex.DispatchFunctionValue
		case godynamichandoff.CallbackTransfer:
			if handoff.Slot.Method != "" {
				// An interface value, not a callable, crosses the call boundary.
				kind = programindex.RelationBindsImplementation
			}
		}
		var sourceArgument *programindex.PatternArgumentRefInput
		if handoff.Kind == godynamichandoff.CallbackTransfer && handoff.Slot.Method == "" {
			// This provenance identifies a callable value passed as an
			// argument. An interface object's method is carried indirectly;
			// its typed slot and callsite remain the transfer evidence.
			sourceArgument, err = projection.callbackSourceArgument(handoff)
			if err != nil {
				return nil, err
			}
		}
		if handoff.Kind == godynamichandoff.CallableBinding {
			ref, err := projection.constructRegistration(handoff, fromRef, toRefs, resolution, location)
			if err != nil {
				return nil, err
			}
			if ref != nil {
				sourceArgument = ref
			}
		}
		witnesses := []programindex.Witness{{Kind: "go_ssa_dynamic_handoff", Detail: dynamicHandoffDetail(handoff, functionNames), Location: location}}
		for _, field := range handoff.ReceiverFields {
			at, err := projection.dynamicLocation(field.Location)
			if err != nil {
				return nil, err
			}
			witnesses = append(witnesses, programindex.Witness{Kind: "callable_receiver_field", Detail: field.Field + " = " + field.Literal, Location: at})
		}
		for _, candidate := range handoff.Candidates {
			for _, assignment := range candidate.Assignments {
				at, err := projection.dynamicLocation(assignment)
				if err != nil {
					return nil, err
				}
				witnesses = append(witnesses, programindex.Witness{Kind: "interface_field_assignment", Detail: "observed receiver assignment for " + functionNames[candidate.FunctionID], Location: at})
			}
		}
		// A field a branch left open keeps what its stores put there as
		// witnesses of the open call, in the C adapter's words.
		for _, witness := range handoff.Witnesses {
			at, err := projection.dynamicLocation(witness.Assignment)
			if err != nil {
				return nil, err
			}
			detail, kind := functionNames[witness.FunctionID]+" stored in "+witness.Field, "interface_field_assignment"
			if witness.PassedTo != "" {
				// A callable a call hands the parameter the open call calls,
				// in the Python adapter's words.
				detail, kind = functionNames[witness.FunctionID]+" passed to "+functionNames[witness.PassedTo], "function_value_store"
			}
			if witness.UnderBranch {
				detail += " under a condition"
			}
			// The witness names the implementation its store put there; the
			// call stays open and the implementation is never its target.
			witnesses = append(witnesses, programindex.Witness{Kind: kind, Detail: detail, Location: at, ObjectRef: projection.directNodeObjectRefs[witness.FunctionID]})
		}
		var guard *programindex.Guard
		if kind != programindex.RelationBindsImplementation {
			guard, err = projection.sitesGuard([]surfacediscovery.Location{{Path: handoff.Callsite.Path, Line: handoff.Callsite.Line, Column: handoff.Callsite.Column}})
			if err != nil {
				return nil, err
			}
		}
		// Targets known by the repository's implementations of the
		// interface, no observed flow giving the value, say so.
		basis := ""
		if handoff.Kind == godynamichandoff.InterfaceInvoke && len(handoff.Candidates) > 0 &&
			handoff.Candidates[0].Evidence == godynamichandoff.EvidenceInterfaceImplementation {
			basis = programindex.BasisImplements
		}
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef:         handoff.ID,
			Kind:              kind,
			FromRef:           fromRef,
			ToRefs:            toRefs,
			Resolution:        resolution,
			Invocation:        invocation,
			Dispatch:          dispatch,
			Location:          location,
			TargetsObserved:   targetsObserved,
			Witnesses:         witnesses,
			WitnessesObserved: len(witnesses),
			SourceArgument:    sourceArgument,
			Basis:             basis,
			Guard:             guard,
		})
		counts := represented[handoff.CallerID]
		switch handoff.Kind {
		case godynamichandoff.InterfaceInvoke:
			counts.interfaceInvokes++
		case godynamichandoff.FunctionValueCall:
			counts.functionValueCalls++
		}
		represented[handoff.CallerID] = counts
	}
	return represented, nil
}

// constructRegistration projects a callable bound into a field of a value
// whose type another package declares as the construction of that value:
// Go writes &cobra.Command{Use: "serve", RunE: run} where other languages
// call a constructor. The relation invokes the type with the construct
// invocation; the string literals stored beside the callable are its keyword
// arguments and the bound field is the argument the callback crosses by. A
// value of a repository type is no construction outside the repository.
//
// A callable the code assigns to the field of a value it already holds
// (fs.Usage = c.Usage, srv.Handler = mux) constructs nothing: the store
// hands it to that outside field, which the relation names with its declared
// type (flag.FlagSet.Usage, func()), as a C table row names its record's
// field. One store is one relation; the literals the same value received
// stay its keyword arguments.
func (projection *goProjection) constructRegistration(
	handoff godynamichandoff.Handoff, fromRef string, toRefs []string,
	resolution programindex.Resolution, location *programindex.Location,
) (*programindex.PatternArgumentRefInput, error) {
	packagePath, typeName, ok := splitQualifiedType(handoff.Slot.ContainerType)
	if !ok || projection.repositoryPackages[packagePath] {
		return nil, nil
	}
	target := surfacediscovery.ExternalCallTarget{PackagePath: packagePath, Name: typeName}
	selector, invocation, witness, signature := typeName, programindex.InvocationConstruct, "go_composite_literal", ""
	if handoff.Slot.Assigned {
		target = surfacediscovery.ExternalCallTarget{PackagePath: packagePath, Receiver: typeName, Name: handoff.Slot.Field}
		selector, invocation, witness, signature = handoff.Slot.Field, "", "go_field_store", shortSignature(handoff.Slot.DeclaredType)
	}
	typeRef, exists := projection.externalRefs[externalTargetKey(target)]
	if !exists {
		typeRef = stableRef("go-external-symbol", packagePath, target.Receiver, target.Name)
		projection.externalRefs[externalTargetKey(target)] = typeRef
		authority := projection.externalAuthorities[packagePath]
		if authority == "" {
			authority = programindex.ExternalAuthorityPackage
		}
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: typeRef, Kind: programindex.ObjectExternalSymbol, Name: externalTargetName(target),
			Visibility: visibility(token.IsExported(target.Name)), Signature: signature,
			External: &programindex.ExternalSymbol{AuthorityKind: authority, PackagePath: packagePath, Receiver: target.Receiver, Name: target.Name},
		}); err != nil {
			return nil, err
		}
	}
	fields := append([]godynamichandoff.ReceiverField(nil), handoff.ReceiverFields...)
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Location.Line != fields[j].Location.Line {
			return fields[i].Location.Line < fields[j].Location.Line
		}
		return fields[i].Location.Column < fields[j].Location.Column
	})
	keyParts := []string{handoff.CallerID, handoff.Slot.ContainerType}
	for _, field := range fields {
		keyParts = append(keyParts, field.Field, field.Literal, field.Location.Path, strconv.Itoa(field.Location.Line), strconv.Itoa(field.Location.Column))
	}
	if len(fields) == 0 || handoff.Slot.Assigned {
		keyParts = append(keyParts, handoff.Callsite.Path, strconv.Itoa(handoff.Callsite.Line), strconv.Itoa(handoff.Callsite.Column))
	}
	if handoff.Slot.Assigned {
		keyParts = append(keyParts, "assigned", handoff.Slot.Field)
	}
	key := strings.Join(keyParts, "\x00")
	bound := programindex.PatternArgumentInput{
		Keyword: handoff.Slot.Field, Kind: programindex.PatternDynamic, ObjectRefs: toRefs,
		Resolution: resolution, ObjectsObserved: max(handoff.CandidatesConsidered, len(toRefs)),
	}
	position, exists := projection.constructs[key]
	if !exists {
		relationRef := stableRef("go-construct", key)
		patternRef := stableRef("go-construct-pattern", key)
		var arguments []programindex.PatternArgumentInput
		for _, field := range fields {
			if value, err := strconv.Unquote(field.Literal); err == nil {
				arguments = append(arguments, programindex.PatternArgumentInput{Keyword: field.Field, Kind: programindex.PatternLiteralString, Value: value})
			}
		}
		position = len(projection.relations)
		projection.constructs[key] = position
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef: relationRef, Kind: programindex.RelationInvokesExternal, FromRef: fromRef, ToRefs: []string{typeRef},
			Resolution: programindex.ResolutionExact, Invocation: invocation, Location: location, TargetsObserved: 1,
			Witnesses:         []programindex.Witness{{Kind: witness, Detail: externalTargetName(target), Location: location}},
			WitnessesObserved: 1,
			Patterns: []programindex.RelationPatternInput{{
				SourceRef: patternRef, Form: programindex.PatternCall, Selector: selector, Location: location,
				Arguments: arguments, ArgumentsObserved: len(arguments),
			}},
			PatternsObserved: 1,
		})
	}
	relation := &projection.relations[position]
	relation.Patterns[0].Arguments = append(relation.Patterns[0].Arguments, bound)
	relation.Patterns[0].ArgumentsObserved = len(relation.Patterns[0].Arguments)
	return &programindex.PatternArgumentRefInput{RelationSourceRef: relation.SourceRef, PatternSourceRef: relation.Patterns[0].SourceRef, Keyword: handoff.Slot.Field}, nil
}

// typedNames hands a signature's values over with the repository type each
// carries; a type of another package or module keeps its text alone.
func (projection *goProjection) typedNames(values []gocoreobject.TypedName) []programindex.TypedNameInput {
	if len(values) == 0 {
		return nil
	}
	result := make([]programindex.TypedNameInput, 0, len(values))
	for _, value := range values {
		typed := programindex.TypedNameInput{Name: value.Name, Type: shortSignature(value.Type)}
		if value.Package != "" {
			typed.TypeRef = projection.typeRefs[typeKey(value.Package, value.TypeName)]
		}
		result = append(result, typed)
	}
	return result
}

// splitQualifiedType reads "path/to/pkg.Type" as its package path and name.
func splitQualifiedType(qualified string) (string, string, bool) {
	dot := strings.LastIndex(qualified, ".")
	if dot <= 0 || dot == len(qualified)-1 || strings.ContainsAny(qualified, "[]* ") {
		return "", "", false
	}
	return qualified[:dot], qualified[dot+1:], true
}

// goInvocation maps Go's call forms onto the shared invocation words. A
// callable binding is not a call and has none.
func goInvocation(value string) string {
	switch value {
	case "goroutine":
		return programindex.InvocationGoroutine
	case "deferred":
		return programindex.InvocationDeferred
	default:
		return ""
	}
}

// frontierDispatch names how an unresolved frontier's target would have been
// found; the frontier kind itself stays in its witness.
var frontierDispatch = map[string]string{
	"dynamic":    programindex.DispatchInterface,
	"non_static": programindex.DispatchFunctionValue,
}

func declaredDispatchKey(callerID, path string, line, column int) string {
	return callerID + "\x00" + path + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(column)
}

// callbackSourceArgument joins an exact callable transfer back to the neutral
// source argument that carried it when that owning relation is retained in the
// target's direct/external relation reservoirs. The callback transfer itself
// is independently exact SSA authority: an unreachable owning call, or more
// than one retained owning pattern, therefore leaves this optional provenance
// unset instead of discarding the transfer or failing the target.
func (projection *goProjection) callbackSourceArgument(
	handoff godynamichandoff.Handoff,
) (*programindex.PatternArgumentRefInput, error) {
	type match struct {
		relationSourceRef string
		patternSourceRef  string
	}
	matchesByKey := make(map[string]match)
	visit := func(relationSourceRef string, patterns []surfacediscovery.ExternalCallPattern) {
		for _, pattern := range patterns {
			if pattern.Callsite.Path != handoff.Callsite.Path ||
				pattern.Callsite.Line != handoff.Callsite.Line ||
				pattern.Callsite.Column != handoff.Callsite.Column {
				continue
			}
			for _, argument := range pattern.Arguments {
				if argument.Position == handoff.Slot.Parameter {
					candidate := match{
						relationSourceRef: relationSourceRef,
						patternSourceRef:  pattern.ID,
					}
					matchesByKey[relationSourceRef+"\x00"+pattern.ID] = candidate
				}
			}
		}
	}
	for _, edge := range projection.direct.Edges {
		visit(edge.ID, edge.Patterns)
	}
	for _, family := range projection.external.Families {
		visit(family.ID, family.Patterns)
	}
	if len(matchesByKey) != 1 {
		return nil, nil
	}
	var matched match
	for _, candidate := range matchesByKey {
		matched = candidate
	}
	return &programindex.PatternArgumentRefInput{
		RelationSourceRef: matched.relationSourceRef,
		PatternSourceRef:  matched.patternSourceRef,
		Position:          handoff.Slot.Parameter,
	}, nil
}

func (projection *goProjection) targetInput() (programindex.TargetInput, error) {
	name := projection.target.PackagePath
	kind := "executable"
	if projection.target.Kind == analysistarget.KindModuleLibrary {
		name = projection.target.ModulePath
		kind = "library"
	}
	input := programindex.TargetInput{
		Language: "go", Kind: kind, Name: name, Selector: name,
	}
	switch projection.target.Kind {
	case analysistarget.KindExecutablePackage:
		input.Executables = []string{executableName(projection.target.PackagePath)}
		exactRoots, err := analysistarget.BindExactRoots(projection.target, &projection.direct)
		if err != nil {
			return programindex.TargetInput{}, fmt.Errorf("Go program index adapter: bind exact roots: %w", err)
		}
		if exactRoots.OmittedRoots != 0 || len(exactRoots.Roots) != len(projection.target.Roots) {
			return programindex.TargetInput{}, fmt.Errorf("Go program index adapter: incomplete exact root binding")
		}
		for _, root := range exactRoots.Roots {
			fileRef, ok := projection.repository.ID(root.Path)
			if !ok {
				return programindex.TargetInput{}, fmt.Errorf(
					"Go program index adapter: target root %q is outside repository corpus", root.Path,
				)
			}
			if input.AnchorFileRef == "" {
				input.AnchorFileRef = string(fileRef)
			}
			input.Sources = append(input.Sources, programindex.TargetSource{FileRef: string(fileRef), Path: root.Path})
			seedRef, ok := projection.directNodeObjectRefs[root.NodeID]
			if !ok {
				return programindex.TargetInput{}, fmt.Errorf(
					"Go program index adapter: exact root %q has no projected object", root.NodeID,
				)
			}
			input.Seeds = append(input.Seeds, programindex.TargetSeedInput{
				ObjectRef: seedRef,
				Kind:      programindex.SeedCallable,
				Location:  &programindex.Location{Path: root.Path, Line: root.Line, Column: 1},
			})
		}
	case analysistarget.KindModuleLibrary:
		manifestPath := "go.mod"
		if projection.target.ModuleDir != "." {
			manifestPath = path.Join(projection.target.ModuleDir, "go.mod")
		}
		manifestRef, ok := projection.repository.ID(manifestPath)
		if !ok {
			return programindex.TargetInput{}, fmt.Errorf(
				"Go program index adapter: module manifest %q is outside repository corpus", manifestPath,
			)
		}
		input.AnchorFileRef = string(manifestRef)
		input.Sources = append(input.Sources, programindex.TargetSource{FileRef: string(manifestRef), Path: manifestPath})
		for _, rootPackage := range projection.target.LibraryPackages {
			fileRef, err := projection.libraryPackageSourceRef(rootPackage.PackagePath)
			if err != nil {
				return programindex.TargetInput{}, err
			}
			info, ok := projection.repository.Info(fileRef)
			if !ok {
				return programindex.TargetInput{}, fmt.Errorf(
					"Go program index adapter: library source ref %q is outside repository corpus", fileRef,
				)
			}
			input.Sources = append(input.Sources, programindex.TargetSource{FileRef: string(fileRef), Path: info.Entry.Path})
		}
		exports, err := projection.libraryExports()
		if err != nil {
			return programindex.TargetInput{}, err
		}
		if len(exports) > 0 {
			input.Exports, input.ExportBasis = exports, programindex.ExportsVisibility
		}
	default:
		return programindex.TargetInput{}, fmt.Errorf(
			"Go program index adapter: unsupported target kind %q", projection.target.Kind,
		)
	}
	return input, nil
}

// libraryExports are a module library's API (GO.md "A library's exports"):
// the exported functions, and the exported methods of exported types, that
// its public packages declare outside test files. A package under internal/
// is none of them: only its own module may import it.
func (projection *goProjection) libraryExports() ([]programindex.TargetExportInput, error) {
	public := make(map[string]bool, len(projection.target.LibraryPackages))
	for _, pkg := range projection.target.LibraryPackages {
		public[pkg.PackagePath] = goPackageVisibility(pkg.PackagePath) == programindex.VisibilityPublic
	}
	var exports []programindex.TargetExportInput
	for _, declaration := range projection.core.Callables {
		// A closure (Open$1) is named after its function, not exported.
		if !declaration.Exported || !token.IsIdentifier(declaration.Name) || !public[declaration.Package] || strings.HasSuffix(declaration.Location.Path, "_test.go") {
			continue
		}
		if declaration.Kind == gocoreobject.CallableMethod {
			typeName, ok := receiverTypeName(declaration.Receiver, declaration.Package)
			if !ok || !token.IsExported(typeName) {
				continue
			}
		}
		location, err := projection.coreLocation(declaration.Location)
		if err != nil {
			return nil, err
		}
		exports = append(exports, programindex.TargetExportInput{ObjectRef: declaration.ID, Location: location})
	}
	return exports, nil
}

// executableName is the name `go build` and `go install` give a main
// package's executable: the last element of its import path, or the one
// before a major version suffix (example.com/tool/v2 is tool).
func executableName(importPath string) string {
	name := path.Base(importPath)
	if version := strings.TrimPrefix(name, "v"); name != importPath && len(version) > 0 && version != name && strings.Trim(version, "0123456789") == "" &&
		version[0] != '0' && version != "1" {
		return path.Base(path.Dir(importPath))
	}
	return name
}

func (projection *goProjection) libraryPackageSourceRef(packagePath string) (corpus.FileID, error) {
	var sourcePath string
	for _, pkg := range projection.core.Packages {
		if pkg.Path == packagePath {
			sourcePath = pkg.RepresentativeSource
			break
		}
	}
	if sourcePath == "" {
		return "", fmt.Errorf(
			"Go program index adapter: library package %q has no exact package source",
			packagePath,
		)
	}
	fileRef, ok := projection.repository.ID(sourcePath)
	if !ok {
		return "", fmt.Errorf(
			"Go program index adapter: library package source %q is outside repository corpus",
			sourcePath,
		)
	}
	return fileRef, nil
}

func (projection *goProjection) addObject(value programindex.ObjectInput) error {
	if _, duplicate := projection.objectRefs[value.SourceRef]; duplicate {
		return fmt.Errorf("Go program index adapter: duplicate object source ref %q", value.SourceRef)
	}
	projection.objectRefs[value.SourceRef] = struct{}{}
	projection.objects = append(projection.objects, value)
	return nil
}

func (projection *goProjection) addUnresolved(
	fromRef string,
	kind programindex.RelationKind,
	invocation string,
	witnessKind string,
	count int,
) {
	if count <= 0 {
		return
	}
	sourceRef := stableRef("go-frontier", fromRef, string(kind), invocation)
	if position, exists := projection.unresolvedRelations[sourceRef]; exists {
		relation := &projection.relations[position]
		relation.TargetsObserved += count
		relation.WitnessesObserved += count
		relation.Witnesses[0].Detail = strconv.Itoa(relation.WitnessesObserved)
		return
	}
	projection.relations = append(projection.relations, programindex.RelationInput{
		SourceRef: sourceRef,
		Kind:      kind, FromRef: fromRef, ToRefs: []string{},
		Resolution: programindex.ResolutionUnresolved, Dispatch: frontierDispatch[invocation],
		TargetsObserved:   count,
		Witnesses:         []programindex.Witness{{Kind: witnessKind, Detail: strconv.Itoa(count)}},
		WitnessesObserved: count,
	})
	if projection.unresolvedRelations == nil {
		projection.unresolvedRelations = make(map[string]int)
	}
	projection.unresolvedRelations[sourceRef] = len(projection.relations) - 1
}

func (projection *goProjection) packageRef(packagePath string) (string, error) {
	for _, pkg := range projection.core.Packages {
		if pkg.Path == packagePath {
			ref, ok := projection.packageRefs[packageKey(pkg.ModuleID, pkg.Path)]
			if ok {
				return ref, nil
			}
		}
	}
	return "", fmt.Errorf("Go program index adapter: package %q has no projected object", packagePath)
}

func (projection *goProjection) coreLocation(value gocoreobject.Location) (*programindex.Location, error) {
	if _, ok := projection.repository.ID(value.Path); !ok {
		return nil, fmt.Errorf(
			"Go program index adapter: core location %q is outside repository corpus", value.Path,
		)
	}
	return &programindex.Location{Path: value.Path, Line: value.Line, Column: value.Column}, nil
}

func (projection *goProjection) surfaceLocation(value surfacediscovery.Location) (*programindex.Location, error) {
	if _, ok := projection.repository.ID(value.Path); !ok {
		return nil, fmt.Errorf(
			"Go program index adapter: call location %q is outside repository corpus", value.Path,
		)
	}
	if value.Column <= 0 {
		return nil, nil
	}
	return &programindex.Location{Path: value.Path, Line: value.Line, Column: value.Column}, nil
}

func (projection *goProjection) dynamicLocation(value godynamichandoff.Location) (*programindex.Location, error) {
	if _, ok := projection.repository.ID(value.Path); !ok {
		return nil, fmt.Errorf(
			"Go program index adapter: dynamic handoff location %q is outside repository corpus",
			value.Path,
		)
	}
	return &programindex.Location{Path: value.Path, Line: value.Line, Column: value.Column}, nil
}

func programResolution(value godynamichandoff.Resolution) (programindex.Resolution, error) {
	switch value {
	case godynamichandoff.ResolutionExact:
		return programindex.ResolutionExact, nil
	case godynamichandoff.ResolutionAlternatives:
		return programindex.ResolutionAlternatives, nil
	case godynamichandoff.ResolutionUnresolved:
		return programindex.ResolutionUnresolved, nil
	default:
		return "", fmt.Errorf("unsupported resolution %q", value)
	}
}

func dynamicHandoffDetail(value godynamichandoff.Handoff, functionNames map[string]string) string {
	switch value.Kind {
	case godynamichandoff.InterfaceInvoke:
		detail := value.Slot.DeclaredType + "." + value.Slot.Method + " " + value.Slot.Signature
		if value.Slot.Field != "" {
			detail += " via field " + value.Slot.ContainerType + "." + value.Slot.Field
		}
		return detail
	case godynamichandoff.FunctionValueCall:
		return value.Slot.Signature
	case godynamichandoff.CallbackTransfer:
		detail := "parameter " + strconv.Itoa(value.Slot.Parameter) + " -> " + dynamicStaticTargetName(value.StaticTarget, functionNames)
		if value.Slot.Method != "" {
			detail += "; interface " + value.Slot.DeclaredType + " method " + value.Slot.Method
		}
		return detail + " " + value.Slot.Signature
	case godynamichandoff.CallableBinding:
		detail := value.Slot.ContainerType + "." + value.Slot.Field + " <- " + value.Slot.DeclaredType
		if value.Slot.Signature != "" {
			detail += " " + value.Slot.Signature
		}
		return detail
	default:
		return value.Slot.Signature
	}
}

func dynamicStaticTargetName(value godynamichandoff.StaticTarget, functionNames map[string]string) string {
	if value.FunctionID != "" {
		return functionNames[value.FunctionID]
	}
	name := value.Package + "."
	if value.Receiver != "" {
		name += value.Receiver + "."
	}
	return name + value.Name
}

func validateAuthority(
	repository *corpus.Corpus,
	target analysistarget.Target,
	direct surfacediscovery.DirectCallIndex,
	external surfacediscovery.ExternalCallIndex,
	core gocoreobject.Index,
	dynamic godynamichandoff.Index,
) error {
	if repository == nil {
		return fmt.Errorf("Go program index adapter: repository corpus is required")
	}
	if _, err := repository.Snapshot().Owned(); err != nil {
		return fmt.Errorf("Go program index adapter: repository corpus: %w", err)
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("Go program index adapter: target: %w", err)
	}
	if target.Kind != analysistarget.KindExecutablePackage && target.Kind != analysistarget.KindModuleLibrary {
		return fmt.Errorf("Go program index adapter: unsupported target kind %q", target.Kind)
	}
	if err := direct.Validate(); err != nil {
		return fmt.Errorf("Go program index adapter: direct call index: %w", err)
	}
	if direct.State != surfacediscovery.DirectCallIndexReady {
		return fmt.Errorf("Go program index adapter: direct call index is unavailable: %s", direct.ClosedReason)
	}
	if err := external.Validate(); err != nil {
		return fmt.Errorf("Go program index adapter: external call index: %w", err)
	}
	if err := core.Validate(); err != nil {
		return fmt.Errorf("Go program index adapter: core object index: %w", err)
	}
	if err := dynamic.Validate(); err != nil {
		return fmt.Errorf("Go program index adapter: dynamic handoff index: %w", err)
	}
	if dynamic.SourceDirectCallSHA256 != direct.SHA256 {
		return fmt.Errorf("Go program index adapter: dynamic handoff source does not match direct calls")
	}

	targetPackages := make([]string, 0)
	for _, pkg := range target.RootPackages() {
		targetPackages = append(targetPackages, pkg.PackagePath)
	}
	sort.Strings(targetPackages)
	if !direct.Scope.TargetScoped() || direct.Scope.TargetRef != target.Ref ||
		direct.Scope.TargetKind != string(target.Kind) || direct.Scope.TargetModuleID != target.ModuleID ||
		direct.Scope.TargetModulePath != target.ModulePath || direct.Scope.TargetModuleDir != target.ModuleDir ||
		direct.Scope.TargetPackage != target.PackagePath || !reflect.DeepEqual(direct.Scope.TargetPackages, targetPackages) {
		return fmt.Errorf("Go program index adapter: direct call scope does not match target")
	}
	if core.Scope.TargetRef != target.Ref || core.Scope.TargetKind != string(target.Kind) ||
		core.Scope.TargetModuleID != target.ModuleID || core.Scope.TargetModulePath != target.ModulePath ||
		core.Scope.TargetModuleDir != target.ModuleDir || core.Scope.TargetPackage != target.PackagePath ||
		!reflect.DeepEqual(core.Scope.TargetPackages, targetPackages) {
		return fmt.Errorf("Go program index adapter: core object scope does not match target")
	}
	if !sameSurfaceScenario(direct.Scenario, external.Scenario) ||
		core.Scenario.ID != direct.Scenario.ID || core.Scenario.GOOS != direct.Scenario.GOOS ||
		core.Scenario.GOARCH != direct.Scenario.GOARCH || !slices.Equal(core.Scenario.Tags, direct.Scenario.Tags) {
		return fmt.Errorf("Go program index adapter: producer scenarios do not match")
	}
	if dynamic.Scenario.ID != direct.Scenario.ID || dynamic.Scenario.GOOS != direct.Scenario.GOOS ||
		dynamic.Scenario.GOARCH != direct.Scenario.GOARCH || !slices.Equal(dynamic.Scenario.Tags, direct.Scenario.Tags) {
		return fmt.Errorf("Go program index adapter: dynamic handoff scenario does not match direct calls")
	}

	corePackages := make(map[string]gocoreobject.Package, len(core.Packages))
	coreModules := make(map[string]surfacediscovery.DirectCallModule)
	for _, pkg := range core.Packages {
		corePackages[packageKey(pkg.ModuleID, pkg.Path)] = pkg
		module := surfacediscovery.DirectCallModule{ID: pkg.ModuleID, Path: pkg.Module, Directory: pkg.ModuleDir}
		if previous, exists := coreModules[pkg.ModuleID]; exists && previous != module {
			return fmt.Errorf("Go program index adapter: conflicting core module %q", pkg.ModuleID)
		}
		coreModules[pkg.ModuleID] = module
	}
	if len(external.Packages) != len(corePackages) || len(external.Modules) != len(coreModules) {
		return fmt.Errorf("Go program index adapter: external package inventory does not match core objects")
	}
	for _, pkg := range external.Packages {
		exact, ok := corePackages[packageKey(pkg.ModuleID, pkg.PackagePath)]
		if !ok || exact.ModuleID != pkg.ModuleID || exact.Path != pkg.PackagePath {
			return fmt.Errorf("Go program index adapter: external package %q is outside core objects", pkg.PackagePath)
		}
	}
	for _, module := range external.Modules {
		if exact, ok := coreModules[module.ID]; !ok || exact != module {
			return fmt.Errorf("Go program index adapter: external module %q does not match core objects", module.ID)
		}
	}
	for _, module := range direct.Modules {
		if exact, ok := coreModules[module.ID]; !ok || exact != module {
			return fmt.Errorf("Go program index adapter: direct module %q does not match core objects", module.ID)
		}
	}
	directNodes := make(map[string]surfacediscovery.DirectCallNode, len(direct.Nodes))
	for _, node := range direct.Nodes {
		if _, ok := corePackages[packageKey(node.ModuleID, node.Package)]; !ok {
			return fmt.Errorf("Go program index adapter: direct node %q is outside core packages", node.ID)
		}
		directNodes[node.ID] = node
	}
	for _, variable := range direct.Variables {
		if _, ok := corePackages[packageKey(variable.ModuleID, variable.Package)]; !ok {
			return fmt.Errorf("Go program index adapter: direct variable %q is outside core packages", variable.ID)
		}
	}
	if len(dynamic.Functions) != len(directNodes) {
		return fmt.Errorf("Go program index adapter: dynamic function inventory does not match direct nodes")
	}
	for _, function := range dynamic.Functions {
		node, ok := directNodes[function.ID]
		if !ok || function.Package != node.Package || function.Symbol != node.Symbol.ID ||
			function.Location.Path != node.Declaration.Path || function.Location.Line != node.Declaration.Line ||
			function.Location.Column != node.Declaration.Column {
			return fmt.Errorf(
				"Go program index adapter: dynamic function %q does not match direct node",
				function.ID,
			)
		}
	}
	for _, caller := range external.Callers {
		exact, ok := directNodes[caller.ID]
		if !ok {
			return fmt.Errorf("Go program index adapter: external caller %q is outside direct nodes", caller.ID)
		}
		if !reflect.DeepEqual(exact, caller) {
			return fmt.Errorf("Go program index adapter: external caller %q does not match direct node", caller.ID)
		}
	}
	for _, declaration := range core.Callables {
		if declaration.DirectCallNodeID == "" {
			continue
		}
		node, ok := directNodes[declaration.DirectCallNodeID]
		if !ok || node.Package != declaration.Package || !coreCallableNameMatchesDirectNode(declaration.Name, node.Symbol.Name) ||
			node.Exported != declaration.Exported || node.Declaration.Path != declaration.Location.Path ||
			node.Declaration.Line != declaration.Location.Line || node.Declaration.Column != declaration.Location.Column {
			return fmt.Errorf("Go program index adapter: callable %q does not match its direct node", declaration.ID)
		}
	}
	return nil
}

// go/ssa assigns source-level package init declarations unique names such as
// init#1 and init#2. go/types correctly keeps their declared name as init.
// DirectCallNodeID plus the exact package and source position still bind the
// two producer records without ambiguity, so accept only this closed compiler
// spelling difference. Every other callable name must match byte-for-byte.
func coreCallableNameMatchesDirectNode(declared, direct string) bool {
	if declared == direct {
		return true
	}
	if declared != "init" || !strings.HasPrefix(direct, "init#") {
		return false
	}
	ordinal := strings.TrimPrefix(direct, "init#")
	value, err := strconv.Atoi(ordinal)
	return err == nil && value > 0 && strconv.Itoa(value) == ordinal
}

func sameSurfaceScenario(left, right surfacediscovery.Scenario) bool {
	return left.ID == right.ID && left.GOOS == right.GOOS && left.GOARCH == right.GOARCH &&
		left.GoFlags == right.GoFlags && slices.Equal(left.Tags, right.Tags)
}

func receiverTypeName(receiver, packagePath string) (string, bool) {
	value := strings.TrimPrefix(receiver, "*")
	prefix := packagePath + "."
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(value, prefix)
	if index := strings.IndexByte(name, '['); index >= 0 {
		if !strings.HasSuffix(name, "]") {
			return "", false
		}
		name = name[:index]
	}
	return name, token.IsIdentifier(name)
}

func visibility(exported bool) programindex.Visibility {
	if exported {
		return programindex.VisibilityPublic
	}
	return programindex.VisibilityInternal
}

func goPackageVisibility(packagePath string) programindex.Visibility {
	for _, part := range strings.Split(packagePath, "/") {
		if part == "internal" {
			return programindex.VisibilityInternal
		}
	}
	return programindex.VisibilityPublic
}

func packageKey(moduleID, packagePath string) string {
	return moduleID + "\x00" + packagePath
}

func typeKey(packagePath, name string) string {
	return packagePath + "\x00" + name
}

func fieldKey(packagePath, typeName, field string) string {
	return typeKey(packagePath, typeName) + "\x00" + field
}

func externalTargetKey(target surfacediscovery.ExternalCallTarget) string {
	return strings.Join([]string{target.PackagePath, target.Receiver, target.Name}, "\x00")
}

func externalTargetName(target surfacediscovery.ExternalCallTarget) string {
	name := target.PackagePath + "."
	if target.Receiver != "" {
		name += target.Receiver + "."
	}
	return name + target.Name
}

func generatedCgoTarget(target surfacediscovery.ExternalCallTarget) bool {
	return target.PackagePath == surfacediscovery.ExternalCallCgoPackagePath && target.Receiver == ""
}

func locationDetail(location surfacediscovery.Location) string {
	return location.Path + ":" + strconv.Itoa(location.Line) + ":" + strconv.Itoa(location.Column)
}

func positiveDifference(total, represented int) int {
	if total <= represented {
		return 0
	}
	return total - represented
}

func stableRef(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		digest.Write([]byte(strconv.Itoa(len(field))))
		digest.Write([]byte{0})
		digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func canonicalSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
