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
	index, err := programIndexFor(result, result.SHA256)
	if err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	if err := ValidateProgramIndex(result, index); err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	catalog, err := dependencyCatalog(result)
	if err != nil {
		return programindex.Index{}, dependencies.Catalog{}, err
	}
	return index, catalog, nil
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
	rederived, err := programIndexFor(result, result.SHA256)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(rederived, index) {
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

func programIndexFor(result Result, scenarioSHA string) (programindex.Index, error) {
	objects := make([]programindex.ObjectInput, 0, len(result.Declarations)+len(result.Imports)+len(result.Calls))
	declarationByRef := make(map[string]Declaration, len(result.Declarations))
	fileLanguage := make(map[string]string, len(result.Files))
	for _, file := range result.Files {
		fileLanguage[file.FileRef] = file.Language
	}
	for _, declaration := range result.Declarations {
		declarationByRef[declaration.Ref] = declaration
	}
	for _, declaration := range result.Declarations {
		kind := projectedObjectKind(declaration, declarationByRef)
		visibility := programindex.VisibilityInternal
		if declaration.Exported {
			visibility = programindex.VisibilityPublic
		}
		container := declaration.OwnerRef
		if container == "" && declaration.Kind != "module" {
			container = moduleRefForFile(declaration.Location.FileRef)
		}
		objects = append(objects, programindex.ObjectInput{
			SourceRef: declaration.Ref, Kind: kind, Name: declaration.QualifiedName, Visibility: visibility,
			Signature: declaration.Signature, OwnerRef: declaration.OwnerRef, ContainerRef: container, Location: programLocation(declaration.Location),
		})
	}
	externalObjects := map[string]programindex.ObjectInput{}
	externalRef := func(packagePath, receiver, name string) string {
		if name == "" {
			name = packagePath
		}
		ref := "external:" + packagePath + ":" + receiver + ":" + name
		if _, exists := externalObjects[ref]; !exists {
			displayName := packagePath + "."
			if receiver != "" {
				displayName += receiver + "."
			}
			displayName += name
			externalObjects[ref] = programindex.ObjectInput{SourceRef: ref, Kind: programindex.ObjectExternalSymbol, Name: displayName, Visibility: programindex.VisibilityPublic, External: &programindex.ExternalSymbol{PackagePath: packagePath, Receiver: receiver, Name: name}}
		}
		return ref
	}
	relations := []programindex.RelationInput{}
	addRelation := func(sourceRef string, kind programindex.RelationKind, fromRef string, toRefs []string, resolution programindex.Resolution, location Location, witnessKind, expression, invocation string) {
		targetsObserved := len(toRefs)
		if targetsObserved == 0 {
			targetsObserved = 1
		}
		relations = append(relations, programindex.RelationInput{SourceRef: sourceRef, Kind: kind, FromRef: fromRef, ToRefs: toRefs, Resolution: resolution, Invocation: invocation, Location: programLocation(location), TargetsObserved: targetsObserved, Witnesses: []programindex.Witness{{Kind: witnessKind, SourceExpression: expression, Location: programLocation(location)}}, WitnessesObserved: 1})
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
			to = []string{externalRef(value.ExternalPackage, "", value.ExternalPackage)}
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
			to = []string{externalRef(value.ExternalPackage, value.ExternalReceiver, value.ExternalName)}
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
		addRelation("program:"+value.Ref, kind, value.CallerRef, to, resolution, value.Location, witnessKind, value.Expression, value.Invocation)
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
	return programindex.New(programindex.Input{
		ScenarioSHA256: scenarioSHA, SourceSHA256: result.SourceSHA256,
		Target:  programindex.TargetInput{Language: result.Project.Language, Kind: TargetKind(result), Name: result.Project.Name, Selector: result.Project.Selector, Sources: targetSources(result), AnchorFileRef: result.Project.ManifestFileRef, Seeds: seeds},
		Objects: objects, Relations: relations, Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
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

func buildProductPaths(result Result) []ProductPath {
	declarationByRef := map[string]Declaration{}
	for _, value := range result.Declarations {
		declarationByRef[value.Ref] = value
	}
	ownerChainContains := func(ref, target string) bool {
		for ref != "" {
			if ref == target {
				return true
			}
			ref = declarationByRef[ref].OwnerRef
		}
		return false
	}
	callGraph := map[string][]string{}
	for _, call := range result.Calls {
		if call.Resolution == "unresolved" {
			continue
		}
		callGraph[call.CallerRef] = append(callGraph[call.CallerRef], call.CalleeRefs...)
	}
	productServerEntries := []string{}
	for _, surface := range result.Surfaces {
		if surface.Kind == SurfaceServer && surface.Role == SurfaceProduct {
			productServerEntries = append(productServerEntries, surface.EntryRefs...)
		}
	}
	serverReachable := map[string]bool{}
	queue := append([]string(nil), productServerEntries...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == "" || serverReachable[current] {
			continue
		}
		serverReachable[current] = true
		queue = append(queue, callGraph[current]...)
	}
	paths := []ProductPath{}
	surfaceRefs := func(kind SurfaceKind) []string {
		refs := []string{}
		for _, surface := range result.Surfaces {
			if surface.Role == SurfaceProduct && surface.Kind == kind {
				refs = append(refs, surface.Ref)
			}
		}
		return refs
	}
	sharedSurfaceRefs := func() []string {
		refs := []string{}
		for _, surface := range result.Surfaces {
			if surface.Role == SurfaceSupporting && surface.Kind == SurfaceShared {
				refs = append(refs, surface.Ref)
			}
		}
		return refs
	}()
	for _, browserRoute := range result.Routes {
		if browserRoute.Kind != RouteBrowser || browserRoute.ComponentRef == "" {
			continue
		}
		for _, httpUse := range result.HTTPUses {
			if !ownerChainContains(httpUse.CallerRef, browserRoute.ComponentRef) {
				continue
			}
			for _, serverRoute := range result.Routes {
				if serverRoute.Kind != RouteHTTP || !serverReachable[serverRoute.OwnerRef] || serverRoute.Method != httpUse.Method || serverRoute.Path != httpUse.Path {
					continue
				}
				steps := []PathStep{}
				appendStep := func(kind, label, source string, targets []string, resolution, authority string, location Location) {
					steps = append(steps, PathStep{Ordinal: len(steps) + 1, Kind: kind, Label: label, SourceRef: source, TargetRefs: targets, Resolution: resolution, Authority: authority, Location: location})
				}
				var wrapperCall *Call
				for index := range result.Calls {
					candidate := &result.Calls[index]
					if candidate.CallerRef == httpUse.CallerRef && sameLocation(candidate.Location, httpUse.Location) {
						wrapperCall = candidate
						break
					}
				}
				appendStep("page_route", browserRoute.Path, browserRoute.Ref, append(nonemptyRefs(browserRoute.ComponentRef), surfaceRefs(SurfaceBrowser)...), "exact", "exact_static", browserRoute.Location)
				appendStep("render_target", declarationByRef[browserRoute.ComponentRef].QualifiedName, browserRoute.ComponentRef, nonemptyRefs(httpUse.CallerRef), "exact", "resolved_indirect", declarationByRef[browserRoute.ComponentRef].Location)
				mutationTargets := []string{httpUse.Ref}
				if wrapperCall != nil {
					mutationTargets = []string{wrapperCall.Ref}
				}
				appendStep("mutation_site", declarationByRef[httpUse.CallerRef].QualifiedName, httpUse.CallerRef, mutationTargets, "exact", "resolved_indirect", httpUse.Location)
				if wrapperCall != nil {
					appendStep("program_call", wrapperCall.Expression, wrapperCall.Ref, wrapperCall.CalleeRefs, wrapperCall.Resolution, pathAuthority(wrapperCall.Resolution, false), wrapperCall.Location)
					for _, callee := range wrapperCall.CalleeRefs {
						for index := range result.Calls {
							nested := result.Calls[index]
							if ownerChainContains(nested.CallerRef, callee) && nested.Expression == "fetch" {
								appendStep("program_call", "fetch", nested.Ref, nested.CalleeRefs, nested.Resolution, pathAuthority(nested.Resolution, false), nested.Location)
							}
						}
					}
				}
				appendStep("client_http_use", httpUse.Method+" "+httpUse.Path, httpUse.Ref, []string{serverRoute.Ref}, "exact", "exact_static", httpUse.Location)
				appendStep("http_method_path_match", httpUse.Method+" "+httpUse.Path, httpUse.Ref, []string{serverRoute.Ref}, "exact", "exact_static", serverRoute.Location)
				appendStep("server_route", serverRoute.Method+" "+serverRoute.Path, serverRoute.Ref, surfaceRefs(SurfaceServer), "exact", "exact_static", serverRoute.Location)
				if len(serverRoute.MiddlewareRefs) > 0 {
					appendStep("middleware", "route middleware", serverRoute.Ref, serverRoute.MiddlewareRefs, "exact", "resolved_indirect", serverRoute.Location)
				}
				handlerRefs := serverRoute.HandlerRefs
				if len(handlerRefs) == 0 {
					appendStep("handler", "handler unresolved", serverRoute.Ref, nil, "unresolved", "unresolved_frontier", serverRoute.Location)
				} else {
					executionHandlers := []string{}
					for _, handler := range handlerRefs {
						appendStep("handler_factory", declarationByRef[handler].QualifiedName, serverRoute.Ref, []string{handler}, "exact", "resolved_indirect", declarationByRef[handler].Location)
						returned := []string{}
						for _, declaration := range result.Declarations {
							if declaration.OwnerRef == handler && declaration.Kind == "lambda" && declaration.Name == "returned_handler" {
								returned = append(returned, declaration.Ref)
							}
						}
						if len(returned) > 0 {
							appendStep("handler", "returned request handler", handler, returned, "exact", "resolved_indirect", declarationByRef[returned[0]].Location)
							executionHandlers = append(executionHandlers, returned...)
						} else {
							executionHandlers = append(executionHandlers, handler)
						}
					}
					for _, handler := range executionHandlers {
						for _, contract := range result.Contracts {
							usedWithinHandler := false
							for _, usedBy := range contract.UsedByRefs {
								if ownerChainContains(usedBy, handler) {
									usedWithinHandler = true
									break
								}
							}
							if usedWithinHandler && contract.Kind == "zod_schema" {
								appendStep("contract_validation", contract.Name, handler, append([]string{contract.Ref}, sharedSurfaceRefs...), "exact", "resolved_indirect", contract.Location)
							}
						}
						for _, call := range result.Calls {
							storageReceiver := strings.HasPrefix(call.Expression, "deps.") || strings.HasPrefix(call.Expression, "storage.")
							storageOperation := strings.Contains(call.Expression, "get") || strings.Contains(call.Expression, "update") || strings.Contains(call.Expression, "create") || strings.Contains(call.Expression, "insert") || strings.Contains(call.Expression, "delete")
							if !ownerChainContains(call.CallerRef, handler) || !storageReceiver || !storageOperation {
								continue
							}
							appendStep("storage_call", call.Expression, call.Ref, call.CalleeRefs, call.Resolution, pathAuthority(call.Resolution, false), call.Location)
						}
					}
				}
				storageRefs := []string{}
				storageCallRefs := []string{}
				for _, step := range steps {
					if step.Kind == "storage_call" {
						storageRefs = append(storageRefs, step.TargetRefs...)
						storageCallRefs = append(storageCallRefs, step.SourceRef)
					}
				}
				for _, resource := range result.Resources {
					if resource.Kind != "postgres_database" && resource.Kind != "google_calendar" {
						continue
					}
					matched := false
					for _, ref := range storageRefs {
						if containsString(resource.UsedByRefs, ref) {
							matched = true
						}
					}
					if matched || (resource.Kind == "postgres_database" && len(storageCallRefs) > 0) {
						appendStep("resource_boundary", resource.Name, storageCallRefs[len(storageCallRefs)-1], []string{resource.Ref}, "alternatives", "possible", resource.Location)
					}
				}
				frontier := ""
				for _, step := range steps {
					if step.Resolution == "unresolved" {
						frontier = step.Label
						break
					}
				}
				paths = append(paths, ProductPath{Ref: "product-path:" + httpUse.Ref + ":" + serverRoute.Ref, Name: browserRoute.Path + " → " + httpUse.Method + " " + httpUse.Path, Outcome: "Browser action reaches the matching server route with locally grounded program evidence.", Steps: steps, Frontier: frontier})
			}
		}
	}
	return paths
}

func pathAuthority(resolution string, static bool) string {
	if resolution == "exact" {
		if static {
			return "exact_static"
		}
		return "resolved_indirect"
	}
	if resolution == "alternatives" {
		return "possible"
	}
	return "unresolved_frontier"
}
func nonemptyRefs(values ...string) []string {
	result := []string{}
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameLocation(left, right Location) bool {
	return left.Path == right.Path && left.FileRef == right.FileRef && left.Line == right.Line && left.Column == right.Column
}
