package clientrecipe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
	"golang.org/x/tools/go/packages"
)

type productionPackageLoader interface {
	Load(context.Context, string) ([]*packages.Package, error)
}

type defaultProductionPackageLoader struct{}

func (defaultProductionPackageLoader) Load(ctx context.Context, repoRoot string) ([]*packages.Package, error) {
	configuration := &packages.Config{
		Context: ctx,
		Dir:     repoRoot,
		Env:     append(os.Environ(), "GOPROXY=off", "GOSUMDB=off"),
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax | packages.NeedModule,
	}
	loaded, err := packages.Load(configuration, "./...")
	if err != nil {
		return nil, fmt.Errorf("client recipe authority: load production packages: %w", err)
	}
	return loaded, nil
}

// PrepareAuthority performs one production package/type load and a separate
// repo-root-only file classification. The Go loader may read declared modules
// for type metadata, but no outside source body or path becomes Authority
// evidence. The boundary has no evaluator input.
func PrepareAuthority(repoRoot string) (Authority, error) {
	return prepareAuthority(context.Background(), repoRoot, defaultProductionPackageLoader{})
}

func prepareAuthority(
	ctx context.Context,
	repoRoot string,
	loader productionPackageLoader,
) (Authority, error) {
	if loader == nil {
		return Authority{}, fmt.Errorf("client recipe authority: package loader is nil")
	}
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Authority{}, fmt.Errorf("client recipe authority: resolve root: %w", err)
	}
	sources, repositorySHA256, err := prepareSourceFacts(absoluteRoot)
	if err != nil {
		return Authority{}, err
	}
	loaded, err := loader.Load(ctx, absoluteRoot)
	if err != nil {
		return Authority{}, err
	}
	packagesByPath, modulePath, err := productionPackages(loaded)
	if err != nil {
		return Authority{}, err
	}
	catalog, err := buildDependencyCatalog(absoluteRoot, modulePath, packagesByPath)
	if err != nil {
		return Authority{}, err
	}
	index, err := buildProgramIndex(absoluteRoot, modulePath, repositorySHA256, packagesByPath, sources)
	if err != nil {
		return Authority{}, err
	}
	operations, err := externalOperationFacts(index, catalog, sources)
	if err != nil {
		return Authority{}, err
	}
	callbacks := callbackCoverage(index)
	coverage := AuthorityCoverage{FilesObserved: len(sources)}
	for _, source := range sources {
		switch source.Class {
		case SourceProduction:
			coverage.ProductionFiles++
		case SourceTest:
			coverage.TestFiles++
		case SourceGenerated:
			coverage.GeneratedFiles++
		case SourceProse:
			coverage.ProseFiles++
		case SourceManifest:
			coverage.ManifestFiles++
		case SourceOther:
			coverage.OtherFiles++
		}
	}
	coverage.DependencyUsesObserved = catalog.Coverage.ImportsObserved
	coverage.ExternalCallsObserved = len(operations)
	return sealAuthority(Authority{
		Version: AuthorityVersion, RepositorySHA256: repositorySHA256,
		Program: index, Dependencies: catalog, Sources: sources,
		ExternalOperations: operations, Callbacks: callbacks, Coverage: coverage,
	})
}

func prepareSourceFacts(repoRoot string) ([]SourceFact, string, error) {
	files, err := RepositoryFiles(repoRoot)
	if err != nil {
		return nil, "", err
	}
	facts := make([]SourceFact, 0, len(files))
	for _, relative := range files {
		raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relative)))
		if err != nil {
			return nil, "", fmt.Errorf("client recipe authority: read %s: %w", relative, err)
		}
		class := classifySource(relative, raw)
		digest := sha256.Sum256(raw)
		facts = append(facts, SourceFact{
			Path: relative, Class: class, SHA256: hex.EncodeToString(digest[:]), Bytes: len(raw),
			ProductionCode: class == SourceProduction || class == SourceGenerated,
		})
	}
	facts = canonicalSourceFacts(facts)
	return facts, sourceFactsDigest(facts), nil
}

func classifySource(relative string, raw []byte) SourceClass {
	if strings.HasSuffix(relative, "_test.go") {
		return SourceTest
	}
	if strings.HasSuffix(relative, ".go") {
		prefix := string(raw)
		if len(prefix) > 256 {
			prefix = prefix[:256]
		}
		if strings.Contains(prefix, "Code generated") || strings.HasSuffix(relative, ".gen.go") {
			return SourceGenerated
		}
		return SourceProduction
	}
	if strings.HasSuffix(relative, ".md") {
		return SourceProse
	}
	if path.Base(relative) == "go.mod" || path.Base(relative) == "go.sum" {
		return SourceManifest
	}
	return SourceOther
}

func productionPackages(loaded []*packages.Package) ([]*packages.Package, string, error) {
	if len(loaded) == 0 {
		return nil, "", fmt.Errorf("client recipe authority: production package load is empty")
	}
	result := make([]*packages.Package, 0, len(loaded))
	for _, pkg := range loaded {
		// packages.Load may return a root row for a directory containing only
		// external-package tests even when Tests is false. Such a row has no
		// build-selected production source and is not production authority.
		if pkg == nil || len(pkg.CompiledGoFiles) == 0 {
			continue
		}
		result = append(result, pkg)
	}
	if len(result) == 0 {
		return nil, "", fmt.Errorf("client recipe authority: production package load has no source-bearing packages")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PkgPath < result[j].PkgPath })
	modulePath := ""
	for _, pkg := range result {
		if len(pkg.Errors) > 0 {
			return nil, "", fmt.Errorf("client recipe authority: package %s: %s", pkg.PkgPath, pkg.Errors[0].Msg)
		}
		if pkg.Module == nil || pkg.Module.Path == "" {
			return nil, "", fmt.Errorf("client recipe authority: package %s has no module authority", pkg.PkgPath)
		}
		if modulePath == "" {
			modulePath = pkg.Module.Path
		}
		if pkg.Module.Path != modulePath {
			return nil, "", fmt.Errorf("client recipe authority: production packages span modules")
		}
		if pkg.Types == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
			return nil, "", fmt.Errorf("client recipe authority: package %s has incomplete typed syntax", pkg.PkgPath)
		}
	}
	return result, modulePath, nil
}

func buildDependencyCatalog(
	repoRoot, modulePath string,
	loaded []*packages.Package,
) (dependencies.Catalog, error) {
	importers := make([]dependencies.Importer, 0, len(loaded))
	refs := make(map[string]string, len(loaded))
	for _, pkg := range loaded {
		repositoryPath, err := packageRepositoryPath(repoRoot, pkg)
		if err != nil {
			return dependencies.Catalog{}, err
		}
		importer, err := dependencies.SealImporter(dependencies.Importer{
			Language: "go", Name: pkg.Name, ModulePath: modulePath,
			PackagePath: pkg.PkgPath, RepositoryPath: repositoryPath,
		})
		if err != nil {
			return dependencies.Catalog{}, err
		}
		importers = append(importers, importer)
		refs[pkg.PkgPath] = importer.Ref
	}
	values := make([]dependencies.Dependency, 0)
	for _, pkg := range loaded {
		paths := make([]string, 0, len(pkg.Imports))
		for importPath := range pkg.Imports {
			paths = append(paths, importPath)
		}
		sort.Strings(paths)
		for _, importPath := range paths {
			imported := pkg.Imports[importPath]
			value := dependencies.Dependency{
				Language: "go", Name: path.Base(importPath), PackagePath: importPath,
				ImporterRefs: []string{refs[pkg.PkgPath]},
			}
			switch {
			case importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/"):
				value.Kind = dependencies.KindWorkspace
				value.ModulePath = modulePath
				repositoryPath, err := packageRepositoryPath(repoRoot, imported)
				if err != nil {
					return dependencies.Catalog{}, err
				}
				value.RepositoryPath = repositoryPath
			case imported.Module == nil:
				value.Kind = dependencies.KindStdlib
			default:
				value.Kind = dependencies.KindExternal
				value.ModulePath = imported.Module.Path
				value.ModuleVersion = imported.Module.Version
				if imported.Module.Replace != nil && imported.Module.Replace.Dir != "" {
					value.Replacement = &dependencies.Replacement{Local: true}
				}
			}
			values = append(values, value)
		}
	}
	return dependencies.BuildWithOmissions(importers, values, nil)
}

func packageRepositoryPath(repoRoot string, pkg *packages.Package) (string, error) {
	if pkg == nil || len(pkg.CompiledGoFiles) == 0 {
		return "", fmt.Errorf("client recipe authority: package has no compiled source")
	}
	directory := filepath.Dir(pkg.CompiledGoFiles[0])
	relative, err := filepath.Rel(repoRoot, directory)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("client recipe authority: package %s is outside repository", pkg.PkgPath)
	}
	return filepath.ToSlash(relative), nil
}

func buildProgramIndex(
	repoRoot, modulePath, repositorySHA256 string,
	loaded []*packages.Package,
	sources []SourceFact,
) (programindex.Index, error) {
	return buildProgramIndexWithSealObserver(
		repoRoot, modulePath, repositorySHA256, loaded, sources, nil,
	)
}

// programIndexSealObserver exists so adapter tests can inspect the complete,
// evaluator-blind observation ledger immediately before and after collision-only
// disambiguation. It is deliberately request-local rather than a package-level
// hook, so concurrent authority builds cannot observe or mutate one another.
type programIndexSealObserver func(before, after []programindex.RelationInput) error

func buildProgramIndexWithSealObserver(
	repoRoot, modulePath, repositorySHA256 string,
	loaded []*packages.Package,
	sources []SourceFact,
	observer programIndexSealObserver,
) (programindex.Index, error) {
	fileRefs := make(map[string]string)
	targetSources := make([]programindex.TargetSource, 0)
	for _, source := range sources {
		if !source.ProductionCode || !strings.HasSuffix(source.Path, ".go") {
			continue
		}
		ref := fmt.Sprintf("f%04d", len(targetSources)+1)
		fileRefs[source.Path] = ref
		targetSources = append(targetSources, programindex.TargetSource{FileRef: ref, Path: source.Path})
	}
	objects := make([]programindex.ObjectInput, 0)
	objectRefs := make(map[types.Object]string)
	type mainCandidate struct {
		ref         string
		packagePath string
		location    programindex.Location
	}
	mainCandidates := make([]mainCandidate, 0, 1)
	for _, pkg := range loaded {
		for _, file := range pkg.Syntax {
			relative, err := relativePositionPath(repoRoot, pkg.Fset, file.Pos())
			if err != nil {
				return programindex.Index{}, err
			}
			for _, declaration := range file.Decls {
				switch value := declaration.(type) {
				case *ast.FuncDecl:
					object, _ := pkg.TypesInfo.Defs[value.Name].(*types.Func)
					if object == nil {
						continue
					}
					location := locationFor(pkg.Fset, value.Name.Pos(), relative)
					ref := localObjectRef(pkg.PkgPath, relative, location.Line, object)
					kind := programindex.ObjectFunction
					name := object.Name()
					if signature, ok := object.Type().(*types.Signature); ok && signature.Recv() != nil {
						kind = programindex.ObjectMethod
						name = receiverName(signature.Recv().Type()) + "." + object.Name()
					}
					objects = append(objects, programindex.ObjectInput{
						SourceRef: ref, Kind: kind, Name: name, Visibility: visibilityFor(object),
						Signature: types.TypeString(object.Type(), packageQualifier), Location: &location,
					})
					objectRefs[object] = ref
					if pkg.Name == "main" && object.Name() == "main" {
						signature, _ := object.Type().(*types.Signature)
						if signature != nil && signature.Recv() == nil && signature.Params().Len() == 0 && signature.Results().Len() == 0 {
							mainCandidates = append(mainCandidates, mainCandidate{
								ref: ref, packagePath: pkg.PkgPath, location: location,
							})
						}
					}
				case *ast.GenDecl:
					for _, spec := range value.Specs {
						typeSpec, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						object, _ := pkg.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
						if object == nil {
							continue
						}
						location := locationFor(pkg.Fset, typeSpec.Name.Pos(), relative)
						ref := localObjectRef(pkg.PkgPath, relative, location.Line, object)
						objects = append(objects, programindex.ObjectInput{
							SourceRef: ref, Kind: programindex.ObjectType, Name: object.Name(),
							Visibility: visibilityFor(object), Location: &location,
						})
						objectRefs[object] = ref
						interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
						if !ok {
							continue
						}
						for _, field := range interfaceType.Methods.List {
							for _, methodName := range field.Names {
								method, _ := pkg.TypesInfo.Defs[methodName].(*types.Func)
								if method == nil {
									continue
								}
								methodLocation := locationFor(pkg.Fset, methodName.Pos(), relative)
								methodRef := localObjectRef(pkg.PkgPath, relative, methodLocation.Line, method)
								objects = append(objects, programindex.ObjectInput{
									SourceRef: methodRef, Kind: programindex.ObjectMethod,
									Name: object.Name() + "." + method.Name(), Visibility: visibilityFor(method),
									Signature: types.TypeString(method.Type(), packageQualifier), OwnerRef: ref,
									Location: &methodLocation,
								})
								objectRefs[method] = methodRef
							}
						}
					}
				}
			}
		}
	}
	if len(mainCandidates) != 1 {
		return programindex.Index{}, fmt.Errorf("client recipe authority: found %d unambiguous main seeds, want 1", len(mainCandidates))
	}
	main := mainCandidates[0]
	relations := make([]programindex.RelationInput, 0)
	externalRefs := make(map[string]string)
	for _, pkg := range loaded {
		for _, file := range pkg.Syntax {
			relative, err := relativePositionPath(repoRoot, pkg.Fset, file.Pos())
			if err != nil {
				return programindex.Index{}, err
			}
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				owner, _ := pkg.TypesInfo.Defs[function.Name].(*types.Func)
				fromRef := objectRefs[owner]
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					location := locationFor(pkg.Fset, call.Pos(), relative)
					expression := nodeString(pkg.Fset, call)
					callee := calledObject(pkg.TypesInfo, call.Fun)
					switch object := callee.(type) {
					case *types.Func:
						toRef, external := relationObjectRef(object, modulePath, objectRefs, externalRefs, &objects)
						if toRef == "" {
							break
						}
						kind := programindex.RelationCalls
						if external {
							kind = programindex.RelationInvokesExternal
						}
						relations = append(relations, exactRelation(
							fromRef, toRef, kind, location, expression, "call",
						))
					case *types.Var:
						if _, ok := object.Type().Underlying().(*types.Signature); ok {
							relations = append(relations, unresolvedRelation(
								fromRef, location, expression,
							))
						}
					}
					for _, argument := range call.Args {
						callback, _ := calledObject(pkg.TypesInfo, argument).(*types.Func)
						if callback == nil {
							continue
						}
						toRef, _ := relationObjectRef(callback, modulePath, objectRefs, externalRefs, &objects)
						if toRef == "" {
							continue
						}
						callbackLocation := locationFor(pkg.Fset, argument.Pos(), relative)
						relations = append(relations, exactRelation(
							fromRef, toRef, programindex.RelationPassesCallback,
							callbackLocation, nodeString(pkg.Fset, argument), "callback_argument",
						))
					}
					return true
				})
			}
		}
	}
	mainPath := main.location.Path
	anchorRef := fileRefs[mainPath]
	if anchorRef == "" {
		return programindex.Index{}, fmt.Errorf("client recipe authority: main source is absent")
	}
	var legacyRelations []programindex.RelationInput
	if observer != nil {
		legacyRelations = cloneRelationInputs(relations)
	}
	relations = disambiguateRelationObservations(relations)
	if observer != nil {
		if err := observer(legacyRelations, cloneRelationInputs(relations)); err != nil {
			return programindex.Index{}, err
		}
	}
	scenario := sha256.Sum256([]byte("clientrecipe-authority-v1\x00" + repositorySHA256))
	input := programindex.Input{
		ScenarioSHA256: hex.EncodeToString(scenario[:]), SourceSHA256: repositorySHA256,
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: path.Base(main.packagePath), Selector: "go:" + path.Dir(mainPath),
			Sources: targetSources, AnchorFileRef: anchorRef,
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: main.ref, Kind: programindex.SeedCallable, Location: &main.location,
			}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations),
		},
	}
	return programindex.New(input)
}

func relationObjectRef(
	object *types.Func,
	modulePath string,
	local map[types.Object]string,
	external map[string]string,
	objects *[]programindex.ObjectInput,
) (string, bool) {
	if object == nil || object.Pkg() == nil {
		return "", false
	}
	if object.Pkg().Path() == modulePath || strings.HasPrefix(object.Pkg().Path(), modulePath+"/") {
		return local[object], false
	}
	signature, _ := object.Type().(*types.Signature)
	receiver := ""
	if signature != nil && signature.Recv() != nil {
		receiver = types.TypeString(signature.Recv().Type(), packageQualifier)
	}
	key := object.Pkg().Path() + "\x00" + receiver + "\x00" + object.Name() + "\x00" + types.TypeString(object.Type(), packageQualifier)
	if ref := external[key]; ref != "" {
		return ref, true
	}
	digest := sha256.Sum256([]byte(key))
	ref := "external:" + hex.EncodeToString(digest[:12])
	external[key] = ref
	*objects = append(*objects, programindex.ObjectInput{
		SourceRef: ref, Kind: programindex.ObjectExternalSymbol,
		Name: object.Pkg().Path() + "." + object.Name(), Visibility: programindex.VisibilityPublic,
		Signature: types.TypeString(object.Type(), packageQualifier),
		External: &programindex.ExternalSymbol{
			PackagePath: object.Pkg().Path(), Receiver: receiver, Name: object.Name(),
		},
	})
	return ref, true
}

func exactRelation(
	fromRef, toRef string,
	kind programindex.RelationKind,
	location programindex.Location,
	expression, witnessKind string,
) programindex.RelationInput {
	relation := programindex.RelationInput{
		Kind: kind, FromRef: fromRef, ToRefs: []string{toRef}, Resolution: programindex.ResolutionExact,
		Location: &location, TargetsObserved: 1,
		Witnesses: []programindex.Witness{{
			Kind: witnessKind, SourceExpression: expression, Location: &location,
		}},
		WitnessesObserved: 1,
	}
	relation.SourceRef = legacyRelationSourceRef(relation)
	return relation
}

func unresolvedRelation(
	fromRef string,
	location programindex.Location,
	expression string,
) programindex.RelationInput {
	relation := programindex.RelationInput{
		Kind: programindex.RelationCalls, FromRef: fromRef, ToRefs: []string{},
		Resolution: programindex.ResolutionUnresolved, Location: &location, TargetsObserved: 1,
		Witnesses: []programindex.Witness{{
			Kind: "function_value_call", SourceExpression: expression, Location: &location,
		}},
		WitnessesObserved: 1,
	}
	relation.SourceRef = legacyRelationSourceRef(relation)
	return relation
}

func legacyRelationSourceRef(relation programindex.RelationInput) string {
	suffix := string(relation.Kind)
	if relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionUnresolved {
		suffix = "unresolved_call"
	}
	return fmt.Sprintf(
		"relation:%s:%d:%d:%s",
		relation.Location.Path, relation.Location.Line, relation.Location.Column, suffix,
	)
}

// disambiguateRelationObservations changes only a legacy identity shared by
// structurally distinct observations. Singleton identities remain byte-for-byte
// stable. An identical duplicate traversal receives the same suffix and is
// therefore still rejected by ProgramIndex instead of being silently dropped.
func disambiguateRelationObservations(relations []programindex.RelationInput) []programindex.RelationInput {
	positions := make(map[string][]int, len(relations))
	for position, relation := range relations {
		key := strings.Join([]string{relation.SourceRef, string(relation.Kind), relation.FromRef}, "\x00")
		positions[key] = append(positions[key], position)
	}
	for _, group := range positions {
		if len(group) < 2 {
			continue
		}
		tuples := make(map[string]struct{}, len(group))
		for _, position := range group {
			tuples[relationObservationTuple(relations[position])] = struct{}{}
		}
		if len(tuples) < 2 {
			continue
		}
		for _, position := range group {
			digest := sha256.Sum256([]byte(relationObservationTuple(relations[position])))
			relations[position].SourceRef += ":" + hex.EncodeToString(digest[:12])
		}
	}
	return relations
}

func cloneRelationInputs(relations []programindex.RelationInput) []programindex.RelationInput {
	result := make([]programindex.RelationInput, len(relations))
	for position, relation := range relations {
		result[position] = relation
		result[position].ToRefs = append([]string(nil), relation.ToRefs...)
		result[position].Witnesses = append([]programindex.Witness(nil), relation.Witnesses...)
	}
	return result
}

// relationObservationTuple binds the complete adapter-observed semantics. The
// witness expression is necessary because same-target fluent calls may share
// both call.Pos and target identity while remaining distinct syntax events.
func relationObservationTuple(relation programindex.RelationInput) string {
	targets := append([]string(nil), relation.ToRefs...)
	sort.Strings(targets)
	witnesses := make([]string, 0, len(relation.Witnesses))
	for _, witness := range relation.Witnesses {
		witnessLocation := ""
		if witness.Location != nil {
			witnessLocation = fmt.Sprintf(
				"%s:%d:%d", witness.Location.Path, witness.Location.Line, witness.Location.Column,
			)
		}
		witnesses = append(witnesses, strings.Join([]string{
			witness.Kind, witness.Detail, witness.SourceExpression, witnessLocation,
		}, "\x1e"))
	}
	sort.Strings(witnesses)
	location := ""
	if relation.Location != nil {
		location = fmt.Sprintf("%s:%d:%d", relation.Location.Path, relation.Location.Line, relation.Location.Column)
	}
	tuple := strings.Join([]string{
		relation.FromRef,
		string(relation.Kind),
		string(relation.Resolution),
		location,
		strings.Join(targets, "\x1f"),
		fmt.Sprintf("%d", relation.TargetsObserved),
		strings.Join(witnesses, "\x1f"),
		fmt.Sprintf("%d", relation.WitnessesObserved),
		relation.Invocation,
	}, "\x00")
	return tuple
}

func calledObject(info *types.Info, expression ast.Expr) types.Object {
	switch value := expression.(type) {
	case *ast.Ident:
		return info.ObjectOf(value)
	case *ast.SelectorExpr:
		if selection := info.Selections[value]; selection != nil {
			return selection.Obj()
		}
		return info.ObjectOf(value.Sel)
	case *ast.IndexExpr:
		return calledObject(info, value.X)
	case *ast.IndexListExpr:
		return calledObject(info, value.X)
	default:
		return nil
	}
}

func localObjectRef(packagePath, relative string, line int, object types.Object) string {
	return fmt.Sprintf("local:%s:%s:%d:%s", packagePath, relative, line, object.Name())
}

func locationFor(files *token.FileSet, position token.Pos, relative string) programindex.Location {
	resolved := files.PositionFor(position, true)
	return programindex.Location{Path: relative, Line: resolved.Line, Column: resolved.Column}
}

func relativePositionPath(repoRoot string, files *token.FileSet, position token.Pos) (string, error) {
	filename := files.PositionFor(position, true).Filename
	relative, err := filepath.Rel(repoRoot, filename)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("client recipe authority: source escaped repository: %s", filename)
	}
	return filepath.ToSlash(relative), nil
}

func nodeString(files *token.FileSet, node ast.Node) string {
	var buffer strings.Builder
	if err := format.Node(&buffer, files, node); err != nil {
		return "unavailable_expression"
	}
	formatted := buffer.String()
	lexicalFiles := token.NewFileSet()
	lexicalFile := lexicalFiles.AddFile("expression.go", -1, len(formatted))
	valid := true
	var lexical scanner.Scanner
	lexical.Init(lexicalFile, []byte(formatted), func(token.Position, string) { valid = false }, 0)
	var result strings.Builder
	previous := token.ILLEGAL
	for {
		_, current, literal := lexical.Scan()
		if current == token.EOF {
			break
		}
		if current == token.SEMICOLON && literal == "\n" {
			continue
		}
		if literal == "" {
			literal = current.String()
		}
		if result.Len() > 0 && expressionTokensNeedSpace(previous, current) {
			result.WriteByte(' ')
		}
		result.WriteString(literal)
		if current == token.COMMA {
			result.WriteByte(' ')
		}
		previous = current
	}
	if !valid || result.Len() == 0 {
		return "unavailable_expression"
	}
	return strings.TrimSpace(result.String())
}

func expressionTokensNeedSpace(previous, current token.Token) bool {
	return expressionWordToken(previous) && expressionWordToken(current)
}

func expressionWordToken(value token.Token) bool {
	return value == token.IDENT || value.IsLiteral() || value.IsKeyword()
}

func visibilityFor(object types.Object) programindex.Visibility {
	if object.Exported() {
		return programindex.VisibilityPublic
	}
	return programindex.VisibilityInternal
}

func receiverName(value types.Type) string {
	text := types.TypeString(value, func(*types.Package) string { return "" })
	return strings.TrimPrefix(text, "*")
}

func packageQualifier(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	return pkg.Path()
}

func externalOperationFacts(
	index programindex.Index,
	catalog dependencies.Catalog,
	sources []SourceFact,
) ([]ExternalOperationFact, error) {
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	dependencyByPackage := make(map[string]dependencies.Dependency, len(catalog.Dependencies))
	for _, dependency := range catalog.Dependencies {
		dependencyByPackage[dependency.PackagePath] = dependency
	}
	importers := make(map[string]dependencies.Importer, len(catalog.Importers))
	for _, importer := range catalog.Importers {
		importers[importer.Ref] = importer
	}
	generated := make(map[string]struct{})
	for _, source := range sources {
		if source.Class == SourceGenerated {
			generated[source.Path] = struct{}{}
		}
	}
	result := make([]ExternalOperationFact, 0)
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || relation.Resolution != programindex.ResolutionExact ||
			len(relation.ToIDs) != 1 || relation.Location == nil || len(relation.Witnesses) != 1 {
			continue
		}
		external := objects[relation.ToIDs[0]]
		if external.External == nil {
			return nil, fmt.Errorf("client recipe authority: external relation has no external symbol")
		}
		dependency, found := dependencyByPackage[external.External.PackagePath]
		if !found {
			// A typed call can resolve to a package that the caller did not import
			// directly (for example a promoted standard-library method on an SDK
			// value). ProgramIndex keeps the call, but H0 has no direct importer
			// authority with which to admit it as a client operation.
			continue
		}
		if dependency.Kind != dependencies.KindExternal {
			continue
		}
		caller := objects[relation.FromID]
		if caller.Location == nil {
			return nil, fmt.Errorf("client recipe authority: external caller has no location")
		}
		importerRef := ""
		callerDirectory := path.Dir(caller.Location.Path)
		for _, candidateRef := range dependency.ImporterRefs {
			importer := importers[candidateRef]
			if importer.RepositoryPath == callerDirectory {
				importerRef = candidateRef
				break
			}
		}
		if importerRef == "" {
			continue
		}
		callee := external.External.PackagePath + "."
		if external.External.Receiver != "" {
			callee += external.External.Receiver + "."
		}
		callee += external.External.Name
		_, isGenerated := generated[relation.Location.Path]
		result = append(result, ExternalOperationFact{
			RelationID: relation.ID, DependencyID: dependency.ID, ImporterRef: importerRef, ExternalObjectID: external.ID,
			CallerID: relation.FromID, PackagePath: external.External.PackagePath,
			CanonicalCallee: callee, Callsite: *relation.Location,
			SourceExpression: relation.Witnesses[0].SourceExpression, Generated: isGenerated,
		})
	}
	sort.Slice(result, func(i, j int) bool { return externalOperationKey(result[i]) < externalOperationKey(result[j]) })
	return result, nil
}

func callbackCoverage(index programindex.Index) CallbackCoverage {
	result := CallbackCoverage{}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.Resolution == programindex.ResolutionExact {
			result.ExactPassRelations++
		}
		if relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionUnresolved {
			result.UnresolvedInvocations++
		}
	}
	switch {
	case result.UnresolvedInvocations > 0 || result.ExactPassRelations > 0:
		result.Status = "frontier"
	default:
		result.Status = "none"
	}
	return result
}
