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
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const goPublicCallableLinkDomain = "go_public_callable_v1"

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
) (programindex.Index, error) {
	input, err := BuildInput(repository, target, packageOrigins, direct, external, core, dynamic)
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
		typeRefs:             make(map[string]string),
		directNodeObjectRefs: make(map[string]string),
		externalRefs:         make(map[string]string),
		callResultObjectRefs: make(map[string]string),
		unresolvedRelations:  make(map[string]int),
	}
	if err := projection.projectObjects(); err != nil {
		return programindex.Input{}, err
	}
	if err := projection.projectRelations(); err != nil {
		return programindex.Input{}, err
	}
	targetInput, err := projection.targetInput()
	if err != nil {
		return programindex.Input{}, err
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
		CorpusSHA256         string `json:"corpus_sha256"`
		TargetRef            string `json:"target_ref"`
		PackageOriginsSHA256 string `json:"package_origins_sha256"`
		DirectSHA256         string `json:"direct_sha256"`
		ExternalSHA256       string `json:"external_sha256"`
		CoreSHA256           string `json:"core_sha256"`
		DynamicSHA256        string `json:"dynamic_sha256"`
	}{
		CorpusSHA256: repository.SHA256(), TargetRef: target.Ref,
		PackageOriginsSHA256: packageOriginsSHA256,
		DirectSHA256:         direct.SHA256, ExternalSHA256: external.SHA256, CoreSHA256: core.SHA256,
		DynamicSHA256: dynamic.SHA256,
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

	return programindex.Input{
		ScenarioSHA256: scenarioSHA256,
		SourceSHA256:   sourceSHA256,
		Target:         targetInput,
		Objects:        projection.objects,
		Relations:      projection.relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: objectsObserved, RelationsObserved: relationsObserved,
		},
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
	repository          *corpus.Corpus
	target              analysistarget.Target
	direct              surfacediscovery.DirectCallIndex
	external            surfacediscovery.ExternalCallIndex
	core                gocoreobject.Index
	dynamic             godynamichandoff.Index
	externalAuthorities map[string]programindex.ExternalAuthorityKind

	objects   []programindex.ObjectInput
	relations []programindex.RelationInput

	objectRefs           map[string]struct{}
	moduleRefs           map[string]string
	packageRefs          map[string]string
	typeRefs             map[string]string
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
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: packageRef, Kind: programindex.ObjectPackage, Name: pkg.Path,
			Visibility: goPackageVisibility(pkg.Path), OwnerRef: moduleRef, ContainerRef: moduleRef,
		}); err != nil {
			return err
		}
		projection.addContains(moduleRef, packageRef, pkg.Path, nil)
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
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: declaration.ID, Kind: programindex.ObjectType, Name: declaration.Name,
			Visibility: visibility(declaration.Exported), OwnerRef: packageRef, ContainerRef: packageRef,
			Location: location,
		}); err != nil {
			return err
		}
		projection.addContains(packageRef, declaration.ID, declaration.Name, location)
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
		var linkIdentities []programindex.SymbolLinkIdentityInput
		if declaration.Exported {
			linkIdentities = goPublicCallableLinkIdentities(
				declaration.Package, receiverName, declaration.Name,
			)
		}
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: declaration.ID, Kind: kind, Name: declaration.Name,
			Visibility: visibility(declaration.Exported), Signature: declaration.Signature,
			OwnerRef: ownerRef, ContainerRef: containerRef, Location: location,
			SymbolLinkIdentities: linkIdentities,
		}); err != nil {
			return err
		}
		projection.addContains(containerRef, declaration.ID, declaration.Name, location)
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
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: node.ID, Kind: programindex.ObjectFunction, Name: node.Symbol.Name,
			Visibility: visibility(node.Exported), OwnerRef: packageRef, ContainerRef: packageRef,
			Location: location,
		}); err != nil {
			return err
		}
		projection.addContains(packageRef, node.ID, node.Symbol.Name, location)
		projection.directNodeObjectRefs[node.ID] = node.ID
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
		linkIdentities := goPublicCallableLinkIdentities(
			family.Target.PackagePath, normalizeGoLinkReceiver(family.Target.Receiver), family.Target.Name,
		)
		if generatedCgoTarget(family.Target) {
			// The target is an exact compiler-generated wrapper boundary, not a
			// portable repository or dependency declaration.
			objectVisibility = programindex.VisibilityUnknown
			linkIdentities = nil
		}
		if err := projection.addObject(programindex.ObjectInput{
			SourceRef: ref, Kind: programindex.ObjectExternalSymbol,
			Name: externalTargetName(family.Target), Visibility: objectVisibility,
			SymbolLinkIdentities: linkIdentities,
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
			Visibility: programindex.VisibilityInternal, Signature: fact.signature,
			Location: location,
		}); err != nil {
			return err
		}
		projection.callResultObjectRefs[id] = id
	}
	return nil
}

func (projection *goProjection) projectRelations() error {
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
		patterns, err := projection.callPatterns(edge.Patterns, callee.Symbol.Name)
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
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef: edge.ID, Kind: programindex.RelationCalls,
			FromRef: fromRef, ToRefs: []string{toRef}, Resolution: programindex.ResolutionExact,
			Invocation: string(edge.Invocation), Location: location, TargetsObserved: 1,
			Witnesses: witnesses, WitnessesObserved: len(witnesses),
			Patterns: patterns, PatternsObserved: edge.PatternsObserved,
		})
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
		invocation := string(family.Invocation)
		witnessKind := "go_external_static_call"
		if generatedCgoTarget(family.Target) {
			invocation = "generated_cgo_wrapper:" + invocation
			witnessKind = "go_generated_cgo_wrapper_call"
		} else if family.Dispatch == surfacediscovery.ExternalCallInterfaceInvoke {
			// The exact target is the declared interface dispatch symbol carried by
			// the producer, never an inferred runtime implementation.
			invocation = "declared_interface_dispatch:" + invocation
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
		patterns, err := projection.callPatterns(family.Patterns, family.Target.Name)
		if err != nil {
			return err
		}
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef: family.ID, Kind: programindex.RelationInvokesExternal,
			FromRef: fromRef, ToRefs: []string{toRef}, Resolution: programindex.ResolutionExact,
			Invocation: invocation, Location: relationLocation, TargetsObserved: 1,
			Witnesses: witnesses, WitnessesObserved: len(witnesses),
			Patterns: patterns, PatternsObserved: family.PatternsObserved,
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

func (projection *goProjection) callPatterns(
	values []surfacediscovery.ExternalCallPattern,
	selector string,
) ([]programindex.RelationPatternInput, error) {
	result := make([]programindex.RelationPatternInput, 0, len(values))
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
		result = append(result, programindex.RelationPatternInput{
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
	for _, handoff := range projection.dynamic.Handoffs {
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
		invocation := string(handoff.Kind) + ":" + string(handoff.Invocation)
		if handoff.Kind == godynamichandoff.CallableBinding {
			// A field assignment identifies the callable value exactly, but it is
			// not proof of runtime execution at this source location.
			invocation = "callable_binding:field"
		}
		var sourceArgument *programindex.PatternArgumentRefInput
		if handoff.Kind == godynamichandoff.CallbackTransfer {
			sourceArgument, err = projection.callbackSourceArgument(handoff)
			if err != nil {
				return nil, err
			}
		}
		projection.relations = append(projection.relations, programindex.RelationInput{
			SourceRef:       handoff.ID,
			Kind:            kind,
			FromRef:         fromRef,
			ToRefs:          toRefs,
			Resolution:      resolution,
			Invocation:      invocation,
			Location:        location,
			TargetsObserved: targetsObserved,
			Witnesses: []programindex.Witness{{
				Kind: "go_ssa_dynamic_handoff", Detail: dynamicHandoffDetail(handoff), Location: location,
			}},
			WitnessesObserved: 1,
			SourceArgument:    sourceArgument,
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
	default:
		return programindex.TargetInput{}, fmt.Errorf(
			"Go program index adapter: unsupported target kind %q", projection.target.Kind,
		)
	}
	return input, nil
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

func (projection *goProjection) addContains(
	fromRef string,
	toRef string,
	detail string,
	location *programindex.Location,
) {
	projection.relations = append(projection.relations, programindex.RelationInput{
		SourceRef: stableRef("go-contains", fromRef, toRef),
		Kind:      programindex.RelationContains, FromRef: fromRef,
		ToRefs: []string{toRef}, Resolution: programindex.ResolutionExact,
		TargetsObserved: 1,
		Witnesses: []programindex.Witness{{
			Kind: "go_declaration_contains", Detail: detail, Location: location,
		}},
		WitnessesObserved: 1,
	})
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
		Resolution: programindex.ResolutionUnresolved, Invocation: invocation,
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

func dynamicHandoffDetail(value godynamichandoff.Handoff) string {
	switch value.Kind {
	case godynamichandoff.InterfaceInvoke:
		return value.Slot.DeclaredType + "." + value.Slot.Method + " " + value.Slot.Signature
	case godynamichandoff.FunctionValueCall:
		return value.Slot.Signature
	case godynamichandoff.CallbackTransfer:
		return "parameter " + strconv.Itoa(value.Slot.Parameter) + " -> " +
			dynamicStaticTargetName(value.StaticTarget) + " " + value.Slot.Signature
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

func dynamicStaticTargetName(value godynamichandoff.StaticTarget) string {
	if value.FunctionID != "" {
		return value.FunctionID
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

func goPublicCallableLinkIdentities(packagePath, receiver, name string) []programindex.SymbolLinkIdentityInput {
	if packagePath == "" || name == "" {
		return nil
	}
	display := name
	if receiver != "" {
		display = receiver + "." + name
	}
	parts := []string{"function", packagePath, name}
	if receiver != "" {
		parts = []string{"method", packagePath, receiver, name}
	}
	return []programindex.SymbolLinkIdentityInput{{
		Domain:  goPublicCallableLinkDomain,
		Parts:   parts,
		Display: display,
	}}
}

func normalizeGoLinkReceiver(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "*")
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
