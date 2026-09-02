package jstsproject

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Build performs discovery once and projects its sealed authority into the
// shared ProgramIndex and dependency catalog.
func Build(ctx context.Context, repository *corpus.Corpus, root string) (Result, programindex.Index, dependencies.Catalog, error) {
	result, err := Discover(ctx, repository, root)
	if err != nil {
		return Result{}, programindex.Index{}, dependencies.Catalog{}, err
	}
	index, catalog, err := BuildFromResult(result)
	if err != nil {
		return Result{}, programindex.Index{}, dependencies.Catalog{}, err
	}
	return result, index, catalog, nil
}

// BuildFromResult projects a previously discovered sealed result without
// invoking Node or reading the repository again.
func BuildFromResult(result Result) (programindex.Index, dependencies.Catalog, error) {
	if err := result.Validate(); err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	input, err := BuildInputFromResult(result)
	if err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	index, err := programindex.New(input)
	if err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	if err := ValidateProgramIndex(result, index); err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	catalog, err := BuildDependenciesFromResult(result)
	if err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	return index, catalog, nil
}

// BuildInputFromResult projects one complete compiler-backed result into the
// shared ProgramIndex input contract without reading the repository, invoking
// Node, or sealing the index. The common repository adapter path owns sealing.
func BuildInputFromResult(result Result) (programindex.Input, error) {
	if err := result.Validate(); err != nil {
		return programindex.Input{}, err
	}
	return programInputFor(result, result.SHA256), nil
}

// BuildDependenciesFromResult projects the same compiler-backed result into
// the language-neutral dependency contract. It performs no repository reads
// and is paired with BuildInputFromResult by the common adapter executor.
func BuildDependenciesFromResult(result Result) (dependencies.Catalog, error) {
	if err := result.Validate(); err != nil {
		return dependencies.Catalog{}, err
	}
	return dependencyCatalog(result)
}

// ValidateProgramIndex proves that the language-neutral index is the exact
// projection of this sealed adapter result and selected package target.
func ValidateProgramIndex(result Result, index programindex.Index) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if err := index.Validate(); err != nil {
		return err
	}
	if index.ScenarioSHA256 != result.SHA256 || index.SourceSHA256 != result.SourceSHA256 || index.Target.ID != result.ProgramTargetID {
		return fmt.Errorf("jsts project: ProgramIndex identity binding mismatch")
	}
	if index.Target.Language != result.Project.Language || index.Target.Kind != TargetKind(result) || index.Target.Name != result.Project.Name || index.Target.Selector != result.Project.Selector || index.Target.AnchorFileRef != result.Project.ManifestFileRef {
		return fmt.Errorf("jsts project: ProgramIndex target binding mismatch")
	}
	expected := targetSources(result)
	if len(expected) != len(index.Target.Sources) {
		return fmt.Errorf("jsts project: ProgramIndex target source mismatch")
	}
	for position := range expected {
		if expected[position] != index.Target.Sources[position] {
			return fmt.Errorf("jsts project: ProgramIndex target source mismatch")
		}
	}
	base, err := programindex.Base(index)
	if err != nil {
		return fmt.Errorf("jsts project: restore structural ProgramIndex projection: %w", err)
	}
	rederived, err := programIndexFor(result, result.SHA256)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rederived, base) {
		return fmt.Errorf("jsts project: ProgramIndex is not the exact Result projection")
	}
	return nil
}

func deriveProgramTargetID(result Result) (string, error) {
	index, err := programIndexFor(result, result.CorpusSHA256)
	if err != nil {
		return "", err
	}
	return index.Target.ID, nil
}

func bindProgramTargetIdentity(result *Result) error {
	// ProgramIndex construction validates every display field. Normalize
	// optional persistence-sensitive metadata before the first projection, not
	// only later in Seal, so a disposable helper signature cannot block the
	// target's exact structural identity.
	omitPersistenceSensitiveOptionalMetadata(result)
	targetID, err := deriveProgramTargetID(*result)
	if err != nil {
		return err
	}
	result.ProgramTargetID = targetID
	return nil
}

func externalProgramObjectRef(packagePath, receiver, name string) string {
	if name == "" {
		name = packagePath
	}
	return "external:" + packagePath + ":" + receiver + ":" + name
}

func programIndexFor(result Result, scenarioSHA string) (programindex.Index, error) {
	return programindex.New(programInputFor(result, scenarioSHA))
}

func programInputFor(result Result, scenarioSHA string) programindex.Input {
	objects := make([]programindex.ObjectInput, 0, len(result.Declarations)+len(result.Imports)+len(result.Calls))
	declarationByRef := make(map[string]Declaration, len(result.Declarations))
	fileLanguage := make(map[string]string, len(result.Files))
	for _, file := range result.Files {
		fileLanguage[file.FileRef] = file.Language
	}
	for _, declaration := range result.Declarations {
		declarationByRef[declaration.Ref] = declaration
	}
	exportNamesByDeclaration := map[string][]string{}
	for _, value := range result.Exports {
		if value.DeclarationRef == "" || value.Name == "*" || value.Resolution != "exact" {
			continue
		}
		exportNamesByDeclaration[value.DeclarationRef] = append(
			exportNamesByDeclaration[value.DeclarationRef], value.Name,
		)
	}
	for _, declaration := range result.Declarations {
		kind := projectedObjectKind(declaration, declarationByRef)
		visibility := programindex.VisibilityInternal
		if declaration.Exported || len(exportNamesByDeclaration[declaration.Ref]) > 0 {
			visibility = programindex.VisibilityPublic
		}
		container := declaration.OwnerRef
		if container == "" && declaration.Kind != "module" {
			container = moduleRefForFile(declaration.Location.FileRef)
		}
		linkIdentities := make([]programindex.SymbolLinkIdentityInput, 0)
		for _, exportName := range canonicalStrings(exportNamesByDeclaration[declaration.Ref]) {
			linkIdentities = append(linkIdentities, programindex.SymbolLinkIdentityInput{
				Domain: "jsts_package_export_v1", Parts: []string{"export", result.Project.PackagePath, exportName},
				Display: result.Project.PackagePath + "#" + exportName,
			})
		}
		objects = append(objects, programindex.ObjectInput{
			SourceRef: declaration.Ref, Kind: kind, Name: declarationDisplayName(declaration, declarationByRef), Visibility: visibility,
			Signature: declaration.Signature, OwnerRef: declaration.OwnerRef, ContainerRef: container, Location: programLocation(declaration.Location),
			SymbolLinkIdentities: linkIdentities,
		})
	}
	for _, value := range result.Calls {
		if value.Pattern == nil || value.Pattern.ResultRef == "" {
			continue
		}
		objects = append(objects, programindex.ObjectInput{
			SourceRef:  value.Pattern.ResultRef,
			Kind:       programindex.ObjectVariable,
			Name:       "call result",
			Visibility: programindex.VisibilityInternal,
			Location:   programLocation(value.Location),
		})
	}
	externalObjects := map[string]programindex.ObjectInput{}
	externalRef := func(packagePath, exportName, receiver, name string) string {
		if name == "" {
			name = packagePath
		}
		ref := externalProgramObjectRef(packagePath, receiver, name)
		if _, exists := externalObjects[ref]; !exists {
			displayName := packagePath + "."
			if receiver != "" {
				displayName += receiver + "."
			}
			displayName += name
			linkIdentities := []programindex.SymbolLinkIdentityInput{}
			if packagePath != javascriptPlatform && exportName != "" {
				linkIdentities = append(linkIdentities, programindex.SymbolLinkIdentityInput{
					Domain: "jsts_package_export_v1", Parts: []string{"export", packagePath, exportName},
					Display: packagePath + "#" + exportName,
				})
			}
			externalObjects[ref] = programindex.ObjectInput{
				SourceRef: ref, Kind: programindex.ObjectExternalSymbol, Name: displayName,
				Visibility:           programindex.VisibilityPublic,
				SymbolLinkIdentities: linkIdentities,
				External: &programindex.ExternalSymbol{
					AuthorityKind: externalAuthorityKind(packagePath),
					PackagePath:   packagePath,
					Receiver:      receiver,
					Name:          name,
				},
			}
		}
		return ref
	}
	relations := []programindex.RelationInput{}
	addRelation := func(sourceRef string, kind programindex.RelationKind, fromRef string, toRefs []string, resolution programindex.Resolution, location Location, witnessKind, expression, invocation string) int {
		targetsObserved := len(toRefs)
		if targetsObserved == 0 {
			targetsObserved = 1
		}
		relations = append(relations, programindex.RelationInput{SourceRef: sourceRef, Kind: kind, FromRef: fromRef, ToRefs: toRefs, Resolution: resolution, Invocation: invocation, Location: programLocation(location), TargetsObserved: targetsObserved, Witnesses: []programindex.Witness{{Kind: witnessKind, SourceExpression: expression, Location: programLocation(location)}}, WitnessesObserved: 1})
		return len(relations) - 1
	}
	for _, declaration := range result.Declarations {
		if declaration.Kind == "module" {
			continue
		}
		container := declaration.OwnerRef
		if container == "" {
			container = moduleRefForFile(declaration.Location.FileRef)
		}
		addRelation("contains:"+declaration.Ref, programindex.RelationContains, container, []string{declaration.Ref}, programindex.ResolutionExact, declaration.Location, "typescript_declaration", "", "")
	}
	for _, value := range result.Imports {
		from := moduleRefForFile(value.ImporterFileRef)
		to := []string{}
		resolution := programResolution(value.Resolution)
		if value.ResolvedFileRef != "" {
			to = []string{moduleRefForFile(value.ResolvedFileRef)}
		} else if value.ExternalPackage != "" && resolution == programindex.ResolutionExact {
			to = []string{externalRef(value.ExternalPackage, "", "", value.ExternalPackage)}
		}
		if len(to) == 0 {
			resolution = programindex.ResolutionUnresolved
		}
		witnessKind := "typescript_module_resolution"
		if fileLanguage[value.Location.FileRef] == "javascript" {
			witnessKind = "javascript_module_resolution"
		}
		addRelation("program:"+value.Ref, programindex.RelationImports, from, to, resolution, value.Location, witnessKind, "", "")
	}
	for _, value := range result.Calls {
		to := append([]string(nil), value.CalleeRefs...)
		kind := programindex.RelationCalls
		resolution := programResolution(value.Resolution)
		if value.ExternalPackage != "" && resolution != programindex.ResolutionUnresolved {
			to = []string{externalRef(value.ExternalPackage, value.ExternalExport, value.ExternalReceiver, value.ExternalName)}
			kind = programindex.RelationInvokesExternal
		}
		if len(to) == 0 {
			resolution = programindex.ResolutionUnresolved
		}
		callLanguage := fileLanguage[value.Location.FileRef]
		if callLanguage == "javascript" && resolution == programindex.ResolutionExact && len(to) == 1 {
			resolution = programindex.ResolutionAlternatives
		}
		witnessKind := "typescript_call"
		if callLanguage == "javascript" {
			witnessKind = "javascript_call_candidate"
		}
		relationIndex := addRelation("program:"+value.Ref, kind, value.CallerRef, to, resolution, value.Location, witnessKind, value.Expression, value.Invocation)
		relations[relationIndex].Patterns = programCallPatterns(value)
		relations[relationIndex].PatternsObserved = value.PatternsObserved
		if value.Pattern == nil {
			continue
		}
		for _, argument := range value.Pattern.Arguments {
			if len(argument.ObjectRefs) == 0 {
				continue
			}
			callableRefs := make([]string, 0, len(argument.ObjectRefs))
			for _, ref := range argument.ObjectRefs {
				declaration, ok := declarationByRef[ref]
				if !ok || (declaration.Kind != "function" && declaration.Kind != "method" && declaration.Kind != "lambda") {
					continue
				}
				callableRefs = append(callableRefs, ref)
			}
			if len(callableRefs) == 0 {
				continue
			}
			callbackWitnessKind := "typescript_callback_argument"
			if callLanguage == "javascript" {
				callbackWitnessKind = "javascript_callback_candidate"
			}
			callbackResolution := programResolution(argument.Resolution)
			// An exact callback edge may have exactly one observed callable and no
			// omitted sibling. Known non-callable declarations and unmaterialized
			// object candidates remain measured omissions instead of erasing the
			// callable refs or promoting them to exact authority.
			if callbackResolution == programindex.ResolutionExact &&
				(argument.ObjectsObserved != 1 || len(callableRefs) != 1 || len(argument.ObjectRefs) != 1) {
				callbackResolution = programindex.ResolutionAlternatives
			}
			callbackRelation := addRelation(
				fmt.Sprintf("callback:%s:%d", value.Ref, argument.Position),
				programindex.RelationPassesCallback,
				value.CallerRef,
				callableRefs,
				callbackResolution,
				value.Location,
				callbackWitnessKind,
				value.Expression,
				"",
			)
			relations[callbackRelation].TargetsObserved = argument.ObjectsObserved
			relations[callbackRelation].SourceArgument = &programindex.PatternArgumentRefInput{
				RelationSourceRef: "program:" + value.Ref,
				PatternSourceRef:  "pattern:" + value.Ref,
				Position:          argument.Position,
			}
		}
	}
	for _, contract := range result.Contracts {
		if contract.DeclarationRef == "" {
			continue
		}
		for _, caller := range contract.UsedByRefs {
			if caller == contract.DeclarationRef {
				continue
			}
			addRelation("contract-use:"+contract.Ref+":"+caller, programindex.RelationReads, caller, []string{contract.DeclarationRef}, programindex.ResolutionExact, contract.Location, "typescript_symbol_reference", contract.Name, "")
		}
	}
	keys := make([]string, 0, len(externalObjects))
	for key := range externalObjects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		objects = append(objects, externalObjects[key])
	}
	seeds := []programindex.TargetSeedInput{}
	seedRefs := map[string]struct{}{}
	for _, surface := range result.Surfaces {
		if surface.Role != SurfaceProduct || (surface.Kind != SurfaceBrowser && surface.Kind != SurfaceServer) {
			continue
		}
		for _, ref := range surface.EntryRefs {
			if _, exists := seedRefs[ref]; exists {
				continue
			}
			declaration, ok := declarationByRef[ref]
			if !ok {
				continue
			}
			var kind programindex.SeedKind
			switch projectedObjectKind(declaration, declarationByRef) {
			case programindex.ObjectModule, programindex.ObjectPackage:
				kind = programindex.SeedModule
			case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda:
				kind = programindex.SeedCallable
			case programindex.ObjectVariable, programindex.ObjectType:
				kind = programindex.SeedBoundObject
			default:
				continue
			}
			seeds = append(seeds, programindex.TargetSeedInput{ObjectRef: ref, Kind: kind, Location: programLocation(declaration.Location)})
			seedRefs[ref] = struct{}{}
		}
	}
	if hasCLIProductSurface(result) {
		for _, script := range result.Project.Scripts {
			if (script.Name != "start" && script.Name != "dev") || script.Kind != "runtime" || len(script.EntryFileRefs) != 1 {
				continue
			}
			ref := moduleRefForFile(script.EntryFileRefs[0])
			if _, exists := seedRefs[ref]; exists {
				continue
			}
			declaration, ok := declarationByRef[ref]
			if !ok || projectedObjectKind(declaration, declarationByRef) != programindex.ObjectModule {
				continue
			}
			seeds = append(seeds, programindex.TargetSeedInput{
				ObjectRef: ref,
				Kind:      programindex.SeedScript,
				Location:  programLocation(declaration.Location),
			})
			seedRefs[ref] = struct{}{}
		}
	}
	return programindex.Input{
		ScenarioSHA256: scenarioSHA, SourceSHA256: result.SourceSHA256,
		Target:  programindex.TargetInput{Language: result.Project.Language, Kind: TargetKind(result), Name: result.Project.Name, Selector: result.Project.Selector, Sources: targetSources(result), AnchorFileRef: result.Project.ManifestFileRef, Seeds: seeds},
		Objects: objects, Relations: relations, Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	}
}

// TargetKind returns application only for an exact browser, server, or package
// binary CLI product surface. Libraries and tool-only packages are never
// promoted.
func TargetKind(result Result) string {
	for _, surface := range result.Surfaces {
		if surface.Role == SurfaceProduct &&
			(surface.Kind == SurfaceBrowser || surface.Kind == SurfaceServer || surface.Kind == SurfaceCLI) {
			return "application"
		}
	}
	return "library"
}

func hasCLIProductSurface(result Result) bool {
	for _, surface := range result.Surfaces {
		if surface.Role == SurfaceProduct && surface.Kind == SurfaceCLI {
			return true
		}
	}
	return false
}

func targetSources(result Result) []programindex.TargetSource {
	byPath := map[string]string{result.Project.ManifestPath: result.Project.ManifestFileRef}
	if result.Project.ConfigPath != "" {
		byPath[result.Project.ConfigPath] = result.Project.ConfigFileRef
	}
	if result.Project.LockfilePath != "" {
		byPath[result.Project.LockfilePath] = result.Project.LockfileFileRef
	}
	for _, file := range result.Files {
		byPath[file.Path] = file.FileRef
	}
	for _, file := range result.Project.ToolConfigs {
		byPath[file.Path] = file.FileRef
	}
	for _, binary := range result.Project.Binaries {
		byPath[binary.Path] = binary.FileRef
	}
	values := make([]programindex.TargetSource, 0, len(byPath))
	for filePath := range byPath {
		values = append(values, programindex.TargetSource{FileRef: byPath[filePath], Path: filePath})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].FileRef != values[j].FileRef {
			return values[i].FileRef < values[j].FileRef
		}
		return values[i].Path < values[j].Path
	})
	return values
}

func programLocation(value Location) *programindex.Location {
	return &programindex.Location{Path: value.Path, Line: value.Line, Column: value.Column}
}
func programResolution(value string) programindex.Resolution {
	switch value {
	case "exact":
		return programindex.ResolutionExact
	case "alternatives":
		return programindex.ResolutionAlternatives
	default:
		return programindex.ResolutionUnresolved
	}
}

func programOptionalResolution(value string) programindex.Resolution {
	if value == "" {
		return ""
	}
	return programResolution(value)
}

func programCallPatterns(value Call) []programindex.RelationPatternInput {
	if value.Pattern == nil {
		return nil
	}
	arguments := make([]programindex.PatternArgumentInput, 0, len(value.Pattern.Arguments))
	for _, argument := range value.Pattern.Arguments {
		parts := make([]programindex.PatternPartInput, 0, len(argument.Parts))
		for _, part := range argument.Parts {
			parts = append(parts, programindex.PatternPartInput{
				Kind: programindex.PatternPartKind(part.Kind),
				Text: part.Text,
			})
		}
		valueCandidates := make([]programindex.PatternValueCandidateInput, 0, len(argument.ValueCandidates))
		for _, candidate := range argument.ValueCandidates {
			candidateParts := make([]programindex.PatternPartInput, 0, len(candidate.Parts))
			for _, part := range candidate.Parts {
				candidateParts = append(candidateParts, programindex.PatternPartInput{
					Kind: programindex.PatternPartKind(part.Kind), Text: part.Text,
				})
			}
			valueCandidates = append(valueCandidates, programindex.PatternValueCandidateInput{
				Kind:       programindex.PatternValueKind(candidate.Kind),
				Value:      candidate.Value,
				Parts:      candidateParts,
				Resolution: programindex.PatternValueResolution(candidate.Resolution),
				SourceKind: programindex.PatternValueSourceKind(candidate.SourceKind),
				SourceArgumentRefs: []programindex.PatternArgumentRefInput{{
					RelationSourceRef: "program:" + candidate.SourceCallRef,
					PatternSourceRef:  "pattern:" + candidate.SourceCallRef,
					Position:          candidate.SourcePosition,
				}},
				SourceArgumentsObserved: 1,
			})
		}
		arguments = append(arguments, programindex.PatternArgumentInput{
			Position:        argument.Position,
			Kind:            programindex.PatternValueKind(argument.Kind),
			Value:           argument.Value,
			Parts:           parts,
			ObjectRefs:      append([]string(nil), argument.ObjectRefs...),
			Resolution:      programOptionalResolution(argument.Resolution),
			ObjectsObserved: argument.ObjectsObserved,
			ValueCandidates: valueCandidates, ValueCandidatesObserved: argument.ValueCandidatesObserved,
		})
	}
	return []programindex.RelationPatternInput{{
		SourceRef:                "pattern:" + value.Ref,
		Form:                     programindex.PatternCall,
		Selector:                 value.Pattern.Selector,
		Location:                 programLocation(value.Location),
		ResultRef:                value.Pattern.ResultRef,
		ReceiverRef:              value.Pattern.ReceiverRef,
		ReceiverOriginRefs:       append([]string(nil), value.Pattern.ReceiverOriginRefs...),
		ReceiverOriginResolution: programOptionalResolution(value.Pattern.ReceiverOriginResolution),
		ReceiverOriginsObserved:  value.Pattern.ReceiverOriginsObserved,
		Arguments:                arguments,
		ArgumentsObserved:        value.Pattern.ArgumentsObserved,
	}}
}

func programObjectKind(value string) programindex.ObjectKind {
	switch value {
	case "module":
		return programindex.ObjectModule
	case "type":
		return programindex.ObjectType
	case "function":
		return programindex.ObjectFunction
	case "method":
		return programindex.ObjectMethod
	case "lambda":
		return programindex.ObjectLambda
	default:
		return programindex.ObjectVariable
	}
}

func projectedObjectKind(declaration Declaration, declarations map[string]Declaration) programindex.ObjectKind {
	kind := programObjectKind(declaration.Kind)
	if kind != programindex.ObjectMethod {
		return kind
	}
	owner, ok := declarations[declaration.OwnerRef]
	if ok && owner.Kind == "type" {
		return kind
	}
	// TypeScript uses MethodDeclaration for both class methods and callable
	// properties in object literals. ProgramIndex method authority requires an
	// exact type receiver; retain every other callable property as a nested
	// function without inventing one.
	return programindex.ObjectFunction
}

// declarationDisplayName projects adapter-local declaration structure into a
// path-free presentation name. Repository paths remain solely in Location;
// SourceRef, the derived Object ID, and SymbolLinkIdentities retain identity.
// Module names are already logical module identities and may therefore keep
// their adapter-normalized path-like spelling.
func declarationDisplayName(declaration Declaration, declarations map[string]Declaration) string {
	if declaration.Kind == "module" {
		return declaration.Name
	}

	parts := []string{declaration.Name}
	seen := map[string]struct{}{declaration.Ref: {}}
	for ownerRef := declaration.OwnerRef; ownerRef != ""; {
		if _, duplicate := seen[ownerRef]; duplicate {
			break
		}
		seen[ownerRef] = struct{}{}
		owner, ok := declarations[ownerRef]
		if !ok || owner.Kind == "module" {
			break
		}
		parts = append(parts, owner.Name)
		ownerRef = owner.OwnerRef
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.Join(parts, ".")
}

func externalAuthorityKind(packagePath string) programindex.ExternalAuthorityKind {
	if packagePath == javascriptPlatform || nodeStandardLibrary(packagePath) {
		return programindex.ExternalAuthorityPlatform
	}
	return programindex.ExternalAuthorityPackage
}

func dependencyCatalog(result Result) (dependencies.Catalog, error) {
	importer, err := dependencies.SealImporter(dependencies.Importer{Language: result.Project.Language, Name: result.Project.Name, ModulePath: result.Project.PackagePath, PackagePath: result.Project.PackagePath, RepositoryPath: path.Dir(result.Project.ManifestPath)})
	if err != nil {
		return dependencies.Catalog{}, err
	}
	valuesByPackage := map[string]dependencies.Dependency{}
	for _, value := range result.Imports {
		packagePath := value.ExternalPackage
		if packagePath == "" {
			continue
		}
		kind := dependencies.KindExternal
		modulePath := packagePath
		if nodeStandardLibrary(packagePath) {
			kind = dependencies.KindStdlib
			modulePath = ""
		}
		valuesByPackage[packagePath] = dependencies.Dependency{Language: result.Project.Language, Kind: kind, Name: packagePath, ModulePath: modulePath, PackagePath: packagePath, ImporterRefs: []string{importer.Ref}}
	}
	values := make([]dependencies.Dependency, 0, len(valuesByPackage))
	for _, value := range valuesByPackage {
		values = append(values, value)
	}
	return dependencies.BuildWithOmissions([]dependencies.Importer{importer}, values, nil)
}

func nodeStandardLibrary(packagePath string) bool {
	if strings.HasPrefix(packagePath, "node:") {
		return true
	}
	switch packagePath {
	case "assert", "buffer", "child_process", "cluster", "crypto", "dns", "events", "fs", "http", "https", "module", "net", "os", "path", "perf_hooks", "process", "querystring", "readline", "stream", "string_decoder", "timers", "tls", "tty", "url", "util", "v8", "vm", "worker_threads", "zlib":
		return true
	}
	return false
}
