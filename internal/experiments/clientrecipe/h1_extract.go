package clientrecipe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

type h1ParsedSource struct {
	fact    SourceFact
	raw     []byte
	file    *ast.File
	imports map[string]string
}

type h1ParsedFunction struct {
	source   *h1ParsedSource
	decl     *ast.FuncDecl
	object   *programindex.Object
	receiver string
}

type h1ParsedType struct {
	source *h1ParsedSource
	spec   *ast.TypeSpec
	object *programindex.Object
}

type h1Repository struct {
	root             string
	files            *token.FileSet
	sources          map[string]*h1ParsedSource
	functions        []*h1ParsedFunction
	types            []*h1ParsedType
	functionByObject map[string]*h1ParsedFunction
	typeByObject     map[string]*h1ParsedType
	objects          map[string]programindex.Object
	relations        map[string]programindex.Relation
	importerByDir    map[string]dependencies.Importer
}

type h1ConsumerBinding struct {
	interfaceType     *h1ParsedType
	interfaceMethod   *ast.Field
	operationRelation programindex.Relation
	wrapperMethod     *h1ParsedFunction
}

// ExtractH1 reads only source rows already admitted by Authority. It performs
// no package load and accepts no evaluator input.
func ExtractH1(repoRoot string, authority Authority) (H1Result, error) {
	if err := authority.Validate(); err != nil {
		return H1Result{}, err
	}
	repository, err := loadH1Repository(repoRoot, authority)
	if err != nil {
		return H1Result{}, err
	}
	closures, err := repository.callbackClosures(authority)
	if err != nil {
		return H1Result{}, err
	}
	reachability := repository.reachability(authority, closures)
	baseline, err := BuildH0(authority)
	if err != nil {
		return H1Result{}, err
	}
	instances, excluded, bindings, err := repository.structuralInstances(authority, baseline, reachability, closures)
	if err != nil {
		return H1Result{}, err
	}
	additional, err := repository.structuralExclusions(authority, reachability, instances, bindings)
	if err != nil {
		return H1Result{}, err
	}
	excluded = append(excluded, additional...)
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].ID < excluded[j].ID })
	roles, err := reduceH1Roles(instances)
	if err != nil {
		return H1Result{}, err
	}
	universe := h1ObservedUniverse(instances, excluded)
	result, err := sealH1(H1Result{
		Version: H1Version, AuthoritySHA256: authority.SHA256, H0SHA256: baseline.SHA256,
		Instances: instances, Excluded: excluded, Roles: roles,
		Callbacks: H1CallbackSummary{
			Observed: authority.Callbacks.UnresolvedInvocations, Closed: len(closures),
			Frontier: authority.Callbacks.UnresolvedInvocations - len(closures), Closures: closures,
		},
		Reachability: reachability, ObservedUniverse: universe,
		Ledger: H1Ledger{Observed: len(instances) + len(excluded), Admitted: len(instances), Excluded: len(excluded)},
	})
	if err != nil {
		return H1Result{}, err
	}
	if err := result.ValidateAgainst(authority); err != nil {
		return H1Result{}, err
	}
	return result, nil
}

func loadH1Repository(repoRoot string, authority Authority) (*h1Repository, error) {
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("client recipe H1: resolve repository root: %w", err)
	}
	repository := &h1Repository{
		root: absoluteRoot, files: token.NewFileSet(), sources: make(map[string]*h1ParsedSource, len(authority.Sources)),
		functionByObject: make(map[string]*h1ParsedFunction), typeByObject: make(map[string]*h1ParsedType),
		objects:       make(map[string]programindex.Object, len(authority.Program.Objects)),
		relations:     make(map[string]programindex.Relation, len(authority.Program.Relations)),
		importerByDir: make(map[string]dependencies.Importer, len(authority.Dependencies.Importers)),
	}
	for _, object := range authority.Program.Objects {
		repository.objects[object.ID] = object
	}
	for _, relation := range authority.Program.Relations {
		repository.relations[relation.ID] = relation
	}
	for _, importer := range authority.Dependencies.Importers {
		repository.importerByDir[importer.RepositoryPath] = importer
	}
	for _, fact := range authority.Sources {
		filename := filepath.Join(absoluteRoot, filepath.FromSlash(fact.Path))
		info, err := os.Lstat(filename)
		if err != nil {
			return nil, fmt.Errorf("client recipe H1: inspect %s: %w", fact.Path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("client recipe H1: source %s is not a regular file", fact.Path)
		}
		raw, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("client recipe H1: read %s: %w", fact.Path, err)
		}
		digest := sha256.Sum256(raw)
		if len(raw) != fact.Bytes || hex.EncodeToString(digest[:]) != fact.SHA256 {
			return nil, fmt.Errorf("client recipe H1: source %s changed after Authority preparation", fact.Path)
		}
		source := &h1ParsedSource{fact: fact, raw: raw, imports: map[string]string{}}
		if strings.HasSuffix(fact.Path, ".go") {
			parsed, err := parser.ParseFile(repository.files, fact.Path, raw, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("client recipe H1: parse %s: %w", fact.Path, err)
			}
			source.file = parsed
			for _, imported := range parsed.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return nil, fmt.Errorf("client recipe H1: import in %s: %w", fact.Path, err)
				}
				name := path.Base(importPath)
				if imported.Name != nil {
					name = imported.Name.Name
				}
				if name != "_" && name != "." {
					source.imports[name] = importPath
				}
			}
		}
		repository.sources[fact.Path] = source
	}
	repository.indexDeclarations(authority)
	return repository, nil
}

func (repository *h1Repository) indexDeclarations(authority Authority) {
	objectsByLocation := make(map[string][]programindex.Object)
	for _, object := range authority.Program.Objects {
		if object.Location == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d", object.Location.Path, object.Location.Line)
		objectsByLocation[key] = append(objectsByLocation[key], object)
	}
	paths := make([]string, 0, len(repository.sources))
	for sourcePath := range repository.sources {
		paths = append(paths, sourcePath)
	}
	sort.Strings(paths)
	for _, sourcePath := range paths {
		source := repository.sources[sourcePath]
		if source.file == nil {
			continue
		}
		for _, declaration := range source.file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				line := repository.files.Position(value.Name.Pos()).Line
				var matched *programindex.Object
				for index := range objectsByLocation[fmt.Sprintf("%s:%d", sourcePath, line)] {
					object := objectsByLocation[fmt.Sprintf("%s:%d", sourcePath, line)][index]
					if object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod {
						copy := object
						matched = &copy
						break
					}
				}
				function := &h1ParsedFunction{source: source, decl: value, object: matched, receiver: receiverTypeName(value)}
				repository.functions = append(repository.functions, function)
				if matched != nil {
					repository.functionByObject[matched.ID] = function
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					line := repository.files.Position(typeSpec.Name.Pos()).Line
					var matched *programindex.Object
					for index := range objectsByLocation[fmt.Sprintf("%s:%d", sourcePath, line)] {
						object := objectsByLocation[fmt.Sprintf("%s:%d", sourcePath, line)][index]
						if object.Kind == programindex.ObjectType && object.Name == typeSpec.Name.Name {
							copy := object
							matched = &copy
							break
						}
					}
					parsedType := &h1ParsedType{source: source, spec: typeSpec, object: matched}
					repository.types = append(repository.types, parsedType)
					if matched != nil {
						repository.typeByObject[matched.ID] = parsedType
					}
				}
			}
		}
	}
}

func (repository *h1Repository) callbackClosures(authority Authority) ([]H1CallbackClosure, error) {
	closures := make([]H1CallbackClosure, 0)
	for _, pass := range authority.Program.Relations {
		if pass.Kind != programindex.RelationPassesCallback || pass.Resolution != programindex.ResolutionExact ||
			pass.Location == nil || len(pass.ToIDs) != 1 {
			continue
		}
		caller := repository.functionByObject[pass.FromID]
		if caller == nil || caller.decl.Body == nil {
			continue
		}
		outer, argument, argumentIndex := repository.callbackArgument(caller, *pass.Location)
		if outer == nil || argument == nil {
			continue
		}
		calleeRelation, found := repository.exactCallAt(pass.FromID, caller.source.fact.Path, outer.Pos())
		if !found || len(calleeRelation.ToIDs) != 1 {
			continue
		}
		callee := repository.functionByObject[calleeRelation.ToIDs[0]]
		if callee == nil {
			continue
		}
		parameter := parameterNameAt(callee.decl.Type.Params, argumentIndex)
		if parameter == "" {
			continue
		}
		matches := repository.callbackInvocationMatches(callee, parameter)
		if len(matches) != 1 {
			continue
		}
		match := matches[0]
		passEvidence, err := repository.evidence(caller.source, argument, repository.nodeText(caller.source, argument), pass.ID)
		if err != nil {
			return nil, err
		}
		invokeSymbol := repository.nodeText(match.function.source, match.call.Fun)
		invokeEvidence, err := repository.evidence(match.function.source, match.call.Fun, invokeSymbol, match.relation.ID)
		if err != nil {
			return nil, err
		}
		closure := H1CallbackClosure{
			Kind: match.kind, PassRelationID: pass.ID, UnresolvedRelationID: match.relation.ID,
			TargetObjectID: pass.ToIDs[0], Evidence: canonicalizeH1Evidence([]H1Evidence{passEvidence, invokeEvidence}),
		}
		closure.ID = h1CallbackID(closure.PassRelationID, closure.UnresolvedRelationID, closure.TargetObjectID)
		closures = append(closures, closure)
	}
	sort.Slice(closures, func(i, j int) bool { return closures[i].ID < closures[j].ID })
	return closures, nil
}

type h1CallbackMatch struct {
	kind     string
	function *h1ParsedFunction
	call     *ast.CallExpr
	relation programindex.Relation
}

func (repository *h1Repository) callbackArgument(
	function *h1ParsedFunction,
	location programindex.Location,
) (*ast.CallExpr, ast.Expr, int) {
	var outer *ast.CallExpr
	var argument ast.Expr
	argumentIndex := -1
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || outer != nil {
			return true
		}
		for index, candidate := range call.Args {
			position := repository.files.Position(candidate.Pos())
			if position.Filename == location.Path && position.Line == location.Line && position.Column == location.Column {
				outer, argument, argumentIndex = call, candidate, index
				return false
			}
		}
		return true
	})
	return outer, argument, argumentIndex
}

func (repository *h1Repository) callbackInvocationMatches(
	callee *h1ParsedFunction,
	parameter string,
) []h1CallbackMatch {
	matches := make([]h1CallbackMatch, 0, 1)
	ast.Inspect(callee.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if !ok || identifier.Name != parameter {
			return true
		}
		if relation, found := repository.unresolvedCallAt(callee.object.ID, callee.source.fact.Path, call.Pos()); found {
			matches = append(matches, h1CallbackMatch{kind: "parameter_invoke", function: callee, call: call, relation: relation})
		}
		return true
	})
	stored := make(map[string]string)
	ast.Inspect(callee.decl.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		typeName := expressionTypeName(literal.Type)
		for _, element := range literal.Elts {
			keyValue, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, keyOK := keyValue.Key.(*ast.Ident)
			value, valueOK := keyValue.Value.(*ast.Ident)
			if keyOK && valueOK && value.Name == parameter && typeName != "" {
				stored[typeName] = key.Name
			}
		}
		return true
	})
	for typeName, fieldName := range stored {
		for _, method := range repository.functions {
			if method.receiver != typeName || method.decl.Body == nil || method.object == nil {
				continue
			}
			receiverName := receiverVariableName(method.decl)
			ast.Inspect(method.decl.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				base, baseOK := selector.X.(*ast.Ident)
				if !baseOK || base.Name != receiverName || selector.Sel.Name != fieldName {
					return true
				}
				if relation, found := repository.unresolvedCallAt(method.object.ID, method.source.fact.Path, call.Pos()); found {
					matches = append(matches, h1CallbackMatch{kind: "stored_field_invoke", function: method, call: call, relation: relation})
				}
				return true
			})
		}
	}
	return matches
}

func (repository *h1Repository) reachability(
	authority Authority,
	closures []H1CallbackClosure,
) H1Reachability {
	localObjects := make(map[string]struct{})
	for _, object := range authority.Program.Objects {
		if object.External == nil {
			localObjects[object.ID] = struct{}{}
		}
	}
	reached := make(map[string]struct{})
	for _, seed := range authority.Program.Target.Seeds {
		reached[seed.ObjectID] = struct{}{}
	}
	closureTargets := make(map[string][]string)
	for _, closure := range closures {
		if _, local := localObjects[closure.TargetObjectID]; local {
			from := repository.relations[closure.UnresolvedRelationID].FromID
			closureTargets[from] = append(closureTargets[from], closure.TargetObjectID)
		}
	}
	usedRelations := make(map[string]struct{})
	changed := true
	for changed {
		changed = false
		for _, relation := range authority.Program.Relations {
			if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
				continue
			}
			if _, found := reached[relation.FromID]; !found {
				continue
			}
			if _, local := localObjects[relation.ToIDs[0]]; !local {
				continue
			}
			usedRelations[relation.ID] = struct{}{}
			if _, found := reached[relation.ToIDs[0]]; !found {
				reached[relation.ToIDs[0]] = struct{}{}
				changed = true
			}
		}
		for from, targets := range closureTargets {
			if _, found := reached[from]; !found {
				continue
			}
			for _, target := range targets {
				if _, found := reached[target]; !found {
					reached[target] = struct{}{}
					changed = true
				}
			}
		}
	}
	result := H1Reachability{}
	for _, seed := range authority.Program.Target.Seeds {
		result.SeedObjectIDs = append(result.SeedObjectIDs, seed.ObjectID)
	}
	for id := range reached {
		result.ReachedObjectIDs = append(result.ReachedObjectIDs, id)
	}
	for id := range usedRelations {
		result.ExactRelationIDs = append(result.ExactRelationIDs, id)
	}
	sort.Strings(result.SeedObjectIDs)
	sort.Strings(result.ReachedObjectIDs)
	sort.Strings(result.ExactRelationIDs)
	return result
}

func (repository *h1Repository) exactCallAt(fromID, sourcePath string, position token.Pos) (programindex.Relation, bool) {
	location := repository.files.Position(position)
	for _, relation := range repository.relations {
		if relation.FromID == fromID && relation.Kind == programindex.RelationCalls &&
			relation.Resolution == programindex.ResolutionExact && relation.Location != nil &&
			relation.Location.Path == sourcePath && relation.Location.Line == location.Line && relation.Location.Column == location.Column {
			return relation, true
		}
	}
	return programindex.Relation{}, false
}

func (repository *h1Repository) unresolvedCallAt(fromID, sourcePath string, position token.Pos) (programindex.Relation, bool) {
	location := repository.files.Position(position)
	for _, relation := range repository.relations {
		if relation.FromID == fromID && relation.Kind == programindex.RelationCalls &&
			relation.Resolution == programindex.ResolutionUnresolved && relation.Location != nil &&
			relation.Location.Path == sourcePath && relation.Location.Line == location.Line && relation.Location.Column == location.Column {
			return relation, true
		}
	}
	return programindex.Relation{}, false
}

func (repository *h1Repository) evidence(
	source *h1ParsedSource,
	node ast.Node,
	symbol, authorityID string,
) (H1Evidence, error) {
	position := repository.files.Position(node.Pos())
	symbol = strings.TrimSpace(symbol)
	if position.Filename != source.fact.Path || symbol == "" || strings.ContainsAny(symbol, "\r\n") {
		return H1Evidence{}, fmt.Errorf("client recipe H1: invalid evidence locator in %s", source.fact.Path)
	}
	line := sourceLine(source.raw, position.Line)
	if !strings.Contains(line, symbol) {
		return H1Evidence{}, fmt.Errorf("client recipe H1: symbol %q is absent from %s:%d", symbol, source.fact.Path, position.Line)
	}
	return H1Evidence{
		Path: source.fact.Path, SourceSHA256: source.fact.SHA256, Line: position.Line,
		Column: strings.Index(line, symbol) + 1, Symbol: symbol, AuthorityID: authorityID,
	}, nil
}

func (repository *h1Repository) nodeText(source *h1ParsedSource, node ast.Node) string {
	start := repository.files.Position(node.Pos())
	end := repository.files.Position(node.End())
	if start.Filename != source.fact.Path || end.Filename != source.fact.Path ||
		start.Offset < 0 || end.Offset < start.Offset || end.Offset > len(source.raw) {
		return ""
	}
	return string(source.raw[start.Offset:end.Offset])
}

func sourceLine(raw []byte, line int) string {
	if line <= 0 {
		return ""
	}
	lines := strings.Split(string(raw), "\n")
	if line > len(lines) {
		return ""
	}
	return lines[line-1]
}

func receiverTypeName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	return expressionTypeName(function.Recv.List[0].Type)
}

func receiverVariableName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
		return ""
	}
	return function.Recv.List[0].Names[0].Name
}

func expressionTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return expressionTypeName(value.X)
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return expressionTypeName(value.X)
	case *ast.IndexListExpr:
		return expressionTypeName(value.X)
	default:
		return ""
	}
}

func parameterNameAt(fields *ast.FieldList, argumentIndex int) string {
	if fields == nil || argumentIndex < 0 {
		return ""
	}
	position := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			if position == argumentIndex {
				return ""
			}
			position++
			continue
		}
		for _, name := range field.Names {
			if position == argumentIndex {
				return name.Name
			}
			position++
		}
	}
	return ""
}

func reduceH1Roles(instances []H1Instance) ([]H1RoleFrequency, error) {
	complete := 0
	counts := make(map[H1Role]int, len(h1Roles))
	for _, instance := range instances {
		if !instance.Complete {
			continue
		}
		complete++
		for _, role := range instance.Roles {
			counts[role.Role]++
		}
	}
	if complete == 0 {
		return nil, fmt.Errorf("client recipe H1: no complete structural instance")
	}
	result := make([]H1RoleFrequency, 0, len(h1Roles))
	for _, role := range h1Roles {
		result = append(result, H1RoleFrequency{
			Role: role, CompleteInstances: counts[role], Necessity: h1Necessity(counts[role], complete),
		})
	}
	return result, nil
}

func (repository *h1Repository) structuralInstances(
	authority Authority,
	baseline H0Result,
	reachability H1Reachability,
	closures []H1CallbackClosure,
) ([]H1Instance, []H1Excluded, map[string]h1ConsumerBinding, error) {
	reached := stringSet(reachability.ReachedObjectIDs)
	instances := make([]H1Instance, 0)
	excluded := make([]H1Excluded, 0)
	bindings := make(map[string]h1ConsumerBinding)
	for _, candidate := range baseline.Candidates {
		wrappers := repository.externalWrapperTypes(candidate)
		if len(wrappers) != 1 || wrappers[0].object == nil {
			return nil, nil, nil, fmt.Errorf("client recipe H1: dependency/importer %s has %d exact wrapper types", candidate.ID, len(wrappers))
		}
		wrapper := wrappers[0]
		if wrapper.source.fact.Class == SourceGenerated {
			evidence, err := repository.generatedEvidence(wrapper)
			if err != nil {
				return nil, nil, nil, err
			}
			excluded = append(excluded, newH1Exclusion("external_dependency", candidate.ID, H1ExcludedGenerated, evidence))
			continue
		}
		constructors := repository.wrapperConstructors(candidate, wrapper.spec.Name.Name)
		if len(constructors) == 0 {
			return nil, nil, nil, fmt.Errorf("client recipe H1: wrapper %s has no structural constructor", wrapper.object.ID)
		}
		wiringRelations := repository.reachableConstructorCalls(authority, reached, constructors)
		if len(wiringRelations) == 0 {
			evidence := make([]H1Evidence, 0, 1+len(constructors))
			wrapperEvidence, err := repository.typeEvidence(wrapper)
			if err != nil {
				return nil, nil, nil, err
			}
			evidence = append(evidence, wrapperEvidence)
			for _, constructor := range constructors {
				constructorEvidence, err := repository.functionEvidence(constructor)
				if err != nil {
					return nil, nil, nil, err
				}
				evidence = append(evidence, constructorEvidence)
			}
			excluded = append(excluded, newH1Exclusion(
				"external_dependency", candidate.ID, H1ExcludedNotProductionReachable, evidence,
			))
			continue
		}
		wiringCandidates := make([]programindex.Relation, 0, len(wiringRelations))
		for _, relation := range wiringRelations {
			caller := repository.objects[relation.FromID]
			if caller.Location != nil && path.Dir(caller.Location.Path) != candidate.ImporterRepositoryPath {
				wiringCandidates = append(wiringCandidates, relation)
			}
		}
		if len(wiringCandidates) != 1 {
			return nil, nil, nil, fmt.Errorf("client recipe H1: wrapper %s has %d exact cross-package wiring calls",
				wrapper.object.ID, len(wiringCandidates))
		}
		wiring := wiringCandidates[0]
		wiringFunction := repository.functionByObject[wiring.FromID]
		constructor := repository.functionByObject[wiring.ToIDs[0]]
		if wiringFunction == nil || constructor == nil || wiring.Location == nil {
			return nil, nil, nil, fmt.Errorf("client recipe H1: wiring relation %s cannot be restored", wiring.ID)
		}
		wiringCall := repository.callAtLocation(wiringFunction, *wiring.Location)
		if wiringCall == nil {
			return nil, nil, nil, fmt.Errorf("client recipe H1: wiring call %s is absent", wiring.ID)
		}
		roleEvidence := make(map[H1Role][]H1Evidence, len(h1Roles))
		wrapperEvidence, err := repository.typeEvidence(wrapper)
		if err != nil {
			return nil, nil, nil, err
		}
		roleEvidence[H1RoleLocalWrapper] = append(roleEvidence[H1RoleLocalWrapper], wrapperEvidence)
		wiringEvidence, err := repository.callEvidence(wiringFunction, wiringCall, wiring.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		roleEvidence[H1RoleApplicationWiring] = append(roleEvidence[H1RoleApplicationWiring], wiringEvidence)
		constructionEvidence, err := repository.constructionEvidence(authority, candidate, constructor, wrapper.spec.Name.Name, closures)
		if err != nil {
			return nil, nil, nil, err
		}
		roleEvidence[H1RoleConstruction] = append(roleEvidence[H1RoleConstruction], constructionEvidence...)
		configurationEvidence, err := repository.configurationEvidence(candidate, wiringFunction, wiringCall, constructor)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(configurationEvidence) > 0 {
			roleEvidence[H1RoleConfiguration] = append(roleEvidence[H1RoleConfiguration], configurationEvidence...)
		}
		binding, err := repository.consumerBinding(candidate, wrapper, wiringFunction, wiringCall, wiring, reached)
		if err != nil {
			return nil, nil, nil, err
		}
		boundaryEvidence, err := repository.interfaceEvidence(binding.interfaceType)
		if err != nil {
			return nil, nil, nil, err
		}
		roleEvidence[H1RoleConsumerBoundary] = append(roleEvidence[H1RoleConsumerBoundary], boundaryEvidence)
		operationFunction := repository.functionByObject[binding.operationRelation.FromID]
		operationCall := repository.callAtLocation(operationFunction, *binding.operationRelation.Location)
		operationEvidence, err := repository.callEvidence(operationFunction, operationCall, binding.operationRelation.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		roleEvidence[H1RoleProductionOperation] = append(roleEvidence[H1RoleProductionOperation], operationEvidence)
		verificationKind, verification, err := repository.verificationEvidence(candidate, constructor, binding.wrapperMethod)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(verification) > 0 {
			roleEvidence[H1RoleVerification] = append(roleEvidence[H1RoleVerification], verification...)
		}
		observability, err := repository.observabilityEvidence(binding.wrapperMethod)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(observability) > 0 {
			roleEvidence[H1RoleObservability] = append(roleEvidence[H1RoleObservability], observability...)
		}
		failurePolicy, err := repository.failurePolicyEvidence(authority, candidate, binding.wrapperMethod)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(failurePolicy) > 0 {
			roleEvidence[H1RoleFailurePolicy] = append(roleEvidence[H1RoleFailurePolicy], failurePolicy...)
		}
		roles, present := buildH1RoleEvidence(roleEvidence)
		complete := h1HasMandatoryRoles(present)
		instance := H1Instance{
			H0CandidateID: candidate.ID, DependencyID: candidate.DependencyID, ImporterRef: candidate.ImporterRef,
			PackagePath: candidate.PackagePath, ImporterPackagePath: candidate.ImporterPackagePath,
			ImporterRepositoryPath: candidate.ImporterRepositoryPath, WrapperType: wrapper.spec.Name.Name,
			WrapperObjectID: wrapper.object.ID, VerificationKind: verificationKind, Complete: complete,
			Roles: roles, Missing: h1MissingRoles(present, complete),
		}
		instance.ID = h1InstanceID(instance.H0CandidateID, instance.WrapperObjectID)
		instances = append(instances, instance)
		bindings[instance.ID] = binding
	}
	return instances, excluded, bindings, nil
}

func (repository *h1Repository) externalWrapperTypes(candidate H0Candidate) []*h1ParsedType {
	result := make([]*h1ParsedType, 0, 1)
	for _, parsedType := range repository.types {
		if path.Dir(parsedType.source.fact.Path) != candidate.ImporterRepositoryPath {
			continue
		}
		structure, ok := parsedType.spec.Type.(*ast.StructType)
		if !ok {
			continue
		}
		found := false
		for _, field := range structure.Fields.List {
			if expressionReferencesPackage(parsedType.source, field.Type, candidate.PackagePath) {
				found = true
				break
			}
		}
		if found {
			result = append(result, parsedType)
		}
	}
	return result
}

func expressionReferencesPackage(source *h1ParsedSource, expression ast.Expr, packagePath string) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && source.imports[identifier.Name] == packagePath {
			found = true
			return false
		}
		return true
	})
	return found
}

func (repository *h1Repository) wrapperConstructors(candidate H0Candidate, wrapperType string) []*h1ParsedFunction {
	result := make([]*h1ParsedFunction, 0, 2)
	for _, function := range repository.functions {
		if function.object == nil || function.receiver != "" ||
			path.Dir(function.source.fact.Path) != candidate.ImporterRepositoryPath ||
			!functionReturnsType(function.decl, wrapperType) {
			continue
		}
		result = append(result, function)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].object.ID < result[j].object.ID })
	return result
}

func functionReturnsType(function *ast.FuncDecl, typeName string) bool {
	if function.Type.Results == nil {
		return false
	}
	for _, field := range function.Type.Results.List {
		if expressionTypeName(field.Type) == typeName {
			return true
		}
	}
	return false
}

func (repository *h1Repository) reachableConstructorCalls(
	authority Authority,
	reached map[string]struct{},
	constructors []*h1ParsedFunction,
) []programindex.Relation {
	targets := make(map[string]struct{}, len(constructors))
	for _, constructor := range constructors {
		targets[constructor.object.ID] = struct{}{}
	}
	result := make([]programindex.Relation, 0, 1)
	for _, relation := range authority.Program.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact ||
			len(relation.ToIDs) != 1 || relation.Location == nil {
			continue
		}
		if _, target := targets[relation.ToIDs[0]]; !target {
			continue
		}
		if _, live := reached[relation.FromID]; live {
			result = append(result, relation)
		}
	}
	sort.Slice(result, func(i, j int) bool { return relationLocationKey(result[i]) < relationLocationKey(result[j]) })
	return result
}

func (repository *h1Repository) constructionEvidence(
	authority Authority,
	candidate H0Candidate,
	constructor *h1ParsedFunction,
	wrapperType string,
	closures []H1CallbackClosure,
) ([]H1Evidence, error) {
	result := make([]H1Evidence, 0, 4)
	base, err := repository.functionEvidence(constructor)
	if err != nil {
		return nil, err
	}
	result = append(result, base)
	constructorFunctions := repository.localConstructorClosure(authority, constructor, wrapperType)
	for _, operation := range authority.ExternalOperations {
		if operation.DependencyID != candidate.DependencyID || operation.ImporterRef != candidate.ImporterRef {
			continue
		}
		if _, found := constructorFunctions[operation.CallerID]; !found {
			continue
		}
		function := repository.functionByObject[operation.CallerID]
		call := repository.callAtLocation(function, operation.Callsite)
		evidence, err := repository.callEvidence(function, call, operation.RelationID)
		if err != nil {
			return nil, err
		}
		result = append(result, evidence)
	}
	for _, closure := range closures {
		if _, found := constructorFunctions[repository.relations[closure.PassRelationID].FromID]; !found {
			continue
		}
		target := repository.objects[closure.TargetObjectID]
		if target.External == nil || target.External.PackagePath != candidate.PackagePath {
			continue
		}
		result = append(result, closure.Evidence...)
	}
	if len(result) < 2 {
		return nil, fmt.Errorf("client recipe H1: constructor %s (%s at %s) has no exact external construction flow",
			constructor.object.ID, constructor.object.Name, constructor.source.fact.Path)
	}
	return canonicalizeH1Evidence(result), nil
}

func (repository *h1Repository) localConstructorClosure(
	authority Authority,
	root *h1ParsedFunction,
	wrapperType string,
) map[string]struct{} {
	result := map[string]struct{}{root.object.ID: {}}
	changed := true
	for changed {
		changed = false
		for _, relation := range authority.Program.Relations {
			if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
				continue
			}
			if _, found := result[relation.FromID]; !found {
				continue
			}
			callee := repository.functionByObject[relation.ToIDs[0]]
			if callee == nil || !functionReturnsType(callee.decl, wrapperType) {
				continue
			}
			if _, found := result[callee.object.ID]; !found {
				result[callee.object.ID] = struct{}{}
				changed = true
			}
		}
	}
	return result
}

func (repository *h1Repository) configurationEvidence(
	candidate H0Candidate,
	wiringFunction *h1ParsedFunction,
	wiringCall *ast.CallExpr,
	constructor *h1ParsedFunction,
) ([]H1Evidence, error) {
	result := make([]H1Evidence, 0, 1)
	ast.Inspect(wiringCall, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || call == wiringCall {
			return true
		}
		relation, found := repository.exactCallAt(wiringFunction.object.ID, wiringFunction.source.fact.Path, call.Pos())
		if !found || len(relation.ToIDs) != 1 || relation.ToIDs[0] == constructor.object.ID {
			return true
		}
		callee := repository.functionByObject[relation.ToIDs[0]]
		if callee == nil || path.Dir(callee.source.fact.Path) != candidate.ImporterRepositoryPath ||
			callee.receiver != "" || callee.decl.Type.Results == nil {
			return true
		}
		evidence, err := repository.functionEvidence(callee)
		if err == nil {
			result = append(result, evidence)
		}
		return true
	})
	return canonicalizeH1Evidence(result), nil
}

func (repository *h1Repository) consumerBinding(
	candidate H0Candidate,
	wrapper *h1ParsedType,
	wiringFunction *h1ParsedFunction,
	wiringCall *ast.CallExpr,
	wiringRelation programindex.Relation,
	reached map[string]struct{},
) (h1ConsumerBinding, error) {
	bound := assignedIdentifier(wiringFunction.decl.Body, wiringCall)
	if bound == "" {
		return h1ConsumerBinding{}, fmt.Errorf("client recipe H1: wiring %s has no exact value binding", wiringRelation.ID)
	}
	type candidateFlow struct {
		callee        *h1ParsedFunction
		parameter     string
		field         string
		ownerType     string
		interfaceType *h1ParsedType
	}
	flows := make([]candidateFlow, 0, 1)
	ast.Inspect(wiringFunction.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || call.Pos() <= wiringCall.Pos() {
			return true
		}
		argumentIndex := directIdentifierArgument(call, bound)
		if argumentIndex < 0 {
			return true
		}
		relation, found := repository.exactCallAt(wiringFunction.object.ID, wiringFunction.source.fact.Path, call.Pos())
		if !found || len(relation.ToIDs) != 1 {
			return true
		}
		callee := repository.functionByObject[relation.ToIDs[0]]
		if callee == nil {
			return true
		}
		parameter := parameterNameAt(callee.decl.Type.Params, argumentIndex)
		parameterType := parameterTypeNameAt(callee.decl.Type.Params, argumentIndex)
		interfaceType := repository.typeInDirectory(path.Dir(callee.source.fact.Path), parameterType)
		if parameter == "" || interfaceType == nil {
			return true
		}
		if _, ok := interfaceType.spec.Type.(*ast.InterfaceType); !ok {
			return true
		}
		ownerType, field := storedFieldForParameter(callee.decl.Body, parameter)
		if ownerType == "" || field == "" {
			return true
		}
		flows = append(flows, candidateFlow{callee: callee, parameter: parameter, field: field, ownerType: ownerType, interfaceType: interfaceType})
		return true
	})
	if len(flows) != 1 {
		return h1ConsumerBinding{}, fmt.Errorf("client recipe H1: wrapper %s has %d exact consumer flows", wrapper.object.ID, len(flows))
	}
	flow := flows[0]
	interfaceNode := flow.interfaceType.spec.Type.(*ast.InterfaceType)
	methods := make(map[string]*ast.Field)
	for _, field := range interfaceNode.Methods.List {
		for _, name := range field.Names {
			methods[name.Name] = field
		}
	}
	matches := make([]h1ConsumerBinding, 0, 1)
	for _, method := range repository.functions {
		if method.receiver != flow.ownerType || method.object == nil || method.decl.Body == nil {
			continue
		}
		if _, live := reached[method.object.ID]; !live {
			continue
		}
		receiver := receiverVariableName(method.decl)
		ast.Inspect(method.decl.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fieldSelector, fieldOK := selector.X.(*ast.SelectorExpr)
			if !fieldOK {
				return true
			}
			base, baseOK := fieldSelector.X.(*ast.Ident)
			if !baseOK || base.Name != receiver || fieldSelector.Sel.Name != flow.field {
				return true
			}
			interfaceMethod, methodFound := methods[selector.Sel.Name]
			if !methodFound {
				return true
			}
			relation, relationFound := repository.exactCallAt(method.object.ID, method.source.fact.Path, call.Pos())
			if !relationFound || len(relation.ToIDs) != 1 {
				return true
			}
			target := repository.objects[relation.ToIDs[0]]
			if target.OwnerID != flow.interfaceType.object.ID {
				return true
			}
			wrapperMethods := repository.methodsForType(candidate.ImporterRepositoryPath, wrapper.spec.Name.Name, selector.Sel.Name)
			if len(wrapperMethods) != 1 {
				return true
			}
			matches = append(matches, h1ConsumerBinding{
				interfaceType: flow.interfaceType, interfaceMethod: interfaceMethod,
				operationRelation: relation, wrapperMethod: wrapperMethods[0],
			})
			return true
		})
	}
	if len(matches) != 1 {
		return h1ConsumerBinding{}, fmt.Errorf("client recipe H1: wrapper %s has %d exact live interface operations", wrapper.object.ID, len(matches))
	}
	return matches[0], nil
}

func (repository *h1Repository) verificationEvidence(
	candidate H0Candidate,
	constructor, operation *h1ParsedFunction,
) (string, []H1Evidence, error) {
	type match struct {
		kind      string
		function  *h1ParsedFunction
		assertion ast.Expr
	}
	matches := make([]match, 0, 1)
	for _, test := range repository.functions {
		if test.source.fact.Class != SourceTest || test.decl.Body == nil || !strings.HasPrefix(test.decl.Name.Name, "Test") {
			continue
		}
		constructorCall := findMatchingCall(test.decl.Body, func(call *ast.CallExpr) bool {
			return callMatchesPackageFunction(test.source, call.Fun, candidate.ImporterPackagePath, constructor.decl.Name.Name,
				path.Dir(test.source.fact.Path) == candidate.ImporterRepositoryPath)
		})
		if constructorCall == nil {
			continue
		}
		bound := assignedIdentifier(test.decl.Body, constructorCall)
		if bound == "" {
			continue
		}
		operationCall := findMatchingCall(test.decl.Body, func(call *ast.CallExpr) bool {
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return false
			}
			identifier, identifierOK := selector.X.(*ast.Ident)
			return identifierOK && identifier.Name == bound && selector.Sel.Name == operation.decl.Name.Name
		})
		if operationCall == nil {
			continue
		}
		resultName := assignedIdentifier(test.decl.Body, operationCall)
		if resultName == "" || resultName == "err" {
			continue
		}
		assertion := assertionForResult(test.decl.Body, operationCall.End(), resultName)
		if assertion == nil {
			continue
		}
		kind := "integration_test"
		if path.Dir(test.source.fact.Path) == candidate.ImporterRepositoryPath {
			kind = "unit_test"
		}
		matches = append(matches, match{kind: kind, function: test, assertion: assertion})
	}
	if len(matches) == 0 {
		return "none", nil, nil
	}
	if len(matches) != 1 {
		return "", nil, fmt.Errorf("client recipe H1: wrapper operation %s has %d exact verification layouts", operation.decl.Name.Name, len(matches))
	}
	testEvidence, err := repository.evidence(matches[0].function.source, matches[0].function.decl.Name, matches[0].function.decl.Name.Name, "")
	if err != nil {
		return "", nil, err
	}
	assertionText := repository.nodeText(matches[0].function.source, matches[0].assertion)
	assertionEvidence, err := repository.evidence(matches[0].function.source, matches[0].assertion, assertionText, "")
	if err != nil {
		return "", nil, err
	}
	return matches[0].kind, canonicalizeH1Evidence([]H1Evidence{testEvidence, assertionEvidence}), nil
}

func (repository *h1Repository) observabilityEvidence(operation *h1ParsedFunction) ([]H1Evidence, error) {
	result := make([]H1Evidence, 0)
	if operation == nil || operation.object == nil {
		return result, nil
	}
	wrapperDirectory := path.Dir(operation.source.fact.Path)
	for _, relation := range repository.relations {
		if relation.FromID != operation.object.ID || relation.Kind != programindex.RelationCalls ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil {
			continue
		}
		target := repository.objects[relation.ToIDs[0]]
		if target.Location == nil || path.Dir(target.Location.Path) == wrapperDirectory {
			continue
		}
		localTarget := repository.functionByObject[target.ID]
		if localTarget == nil || !repository.isObservabilityHelper(localTarget) {
			continue
		}
		call := repository.callAtLocation(operation, *relation.Location)
		evidence, err := repository.callEvidence(operation, call, relation.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, evidence)
	}
	return canonicalizeH1Evidence(result), nil
}

func (repository *h1Repository) isObservabilityHelper(function *h1ParsedFunction) bool {
	if function == nil || function.decl.Body == nil || function.object == nil {
		return false
	}
	receiverName := receiverIdentifier(function.decl)
	mutatesReceiverState := false
	if receiverName != "" {
		ast.Inspect(function.decl.Body, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					if expressionRootIdentifier(left) == receiverName {
						mutatesReceiverState = true
						return false
					}
				}
			case *ast.IncDecStmt:
				if expressionRootIdentifier(value.X) == receiverName {
					mutatesReceiverState = true
					return false
				}
			}
			return !mutatesReceiverState
		})
	}
	if mutatesReceiverState {
		return true
	}
	for _, relation := range repository.relations {
		if relation.FromID != function.object.ID || relation.Kind != programindex.RelationInvokesExternal ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
			continue
		}
		target := repository.objects[relation.ToIDs[0]]
		if target.External != nil && target.External.PackagePath == "log" {
			return true
		}
	}
	return false
}

func receiverIdentifier(function *ast.FuncDecl) string {
	if function == nil || function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
		return ""
	}
	return function.Recv.List[0].Names[0].Name
}

func expressionRootIdentifier(expression ast.Expr) string {
	for {
		switch value := expression.(type) {
		case *ast.Ident:
			return value.Name
		case *ast.SelectorExpr:
			expression = value.X
		case *ast.IndexExpr:
			expression = value.X
		case *ast.IndexListExpr:
			expression = value.X
		case *ast.StarExpr:
			expression = value.X
		case *ast.ParenExpr:
			expression = value.X
		default:
			return ""
		}
	}
}

func (repository *h1Repository) failurePolicyEvidence(
	authority Authority,
	candidate H0Candidate,
	operation *h1ParsedFunction,
) ([]H1Evidence, error) {
	result := make([]H1Evidence, 0, 2)
	if operation == nil || operation.object == nil || operation.decl.Body == nil {
		return result, nil
	}
	for _, relation := range authority.Program.Relations {
		if relation.FromID != operation.object.ID || relation.Kind != programindex.RelationInvokesExternal ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil {
			continue
		}
		target := repository.objects[relation.ToIDs[0]]
		if target.External != nil && target.External.PackagePath == "context" && target.External.Name == "WithTimeout" {
			call := repository.callAtLocation(operation, *relation.Location)
			evidence, err := repository.callEvidence(operation, call, relation.ID)
			if err != nil {
				return nil, err
			}
			result = append(result, evidence)
		}
	}
	ast.Inspect(operation.decl.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.ForStmt)
		if !ok || loop.Cond == nil {
			return true
		}
		containsExternal := false
		for _, external := range authority.ExternalOperations {
			if external.DependencyID != candidate.DependencyID || external.ImporterRef != candidate.ImporterRef ||
				external.CallerID != operation.object.ID {
				continue
			}
			if repository.locationWithinNode(operation.source, external.Callsite, loop.Body) {
				containsExternal = true
				break
			}
		}
		if !containsExternal {
			return true
		}
		condition := repository.nodeText(operation.source, loop.Cond)
		evidence, err := repository.evidence(operation.source, loop.Cond, condition, "")
		if err == nil {
			result = append(result, evidence)
		}
		return true
	})
	return canonicalizeH1Evidence(result), nil
}

func (repository *h1Repository) structuralExclusions(
	authority Authority,
	reachability H1Reachability,
	instances []H1Instance,
	bindings map[string]h1ConsumerBinding,
) ([]H1Excluded, error) {
	result := make([]H1Excluded, 0)
	interfaceMethods := make([]struct {
		source *h1ParsedSource
		field  *ast.Field
	}, 0, len(bindings))
	admittedDirectories := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		admittedDirectories[instance.ImporterRepositoryPath] = struct{}{}
		binding := bindings[instance.ID]
		if binding.interfaceType != nil && binding.interfaceMethod != nil {
			interfaceMethods = append(interfaceMethods, struct {
				source *h1ParsedSource
				field  *ast.Field
			}{source: binding.interfaceType.source, field: binding.interfaceMethod})
		}
	}
	for _, parsedType := range repository.types {
		if parsedType.source.fact.Class != SourceTest {
			continue
		}
		if _, ok := parsedType.spec.Type.(*ast.StructType); !ok {
			continue
		}
		matched := false
		for _, function := range repository.functions {
			if function.source.fact.Class != SourceTest || function.receiver != parsedType.spec.Name.Name {
				continue
			}
			if compatibleTestMethod(function, interfaceMethods) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		evidence, err := repository.typeEvidence(parsedType)
		if err != nil {
			return nil, err
		}
		origin := fmt.Sprintf("%s:%d:%s", parsedType.source.fact.Path,
			repository.files.Position(parsedType.spec.Name.Pos()).Line, parsedType.spec.Name.Name)
		result = append(result, newH1Exclusion("test_type", origin, H1ExcludedTestOnly, []H1Evidence{evidence}))
	}
	for _, source := range repository.sources {
		if source.fact.Class != SourceProse {
			continue
		}
		evidence, err := proseEvidence(source)
		if err != nil {
			return nil, err
		}
		result = append(result, newH1Exclusion(
			"prose", source.fact.Path, H1ExcludedNotProductionReachable, []H1Evidence{evidence},
		))
	}
	reached := stringSet(reachability.ReachedObjectIDs)
	for _, binding := range bindings {
		if binding.wrapperMethod != nil && binding.wrapperMethod.object != nil {
			reached[binding.wrapperMethod.object.ID] = struct{}{}
		}
	}
	changed := true
	for changed {
		changed = false
		for _, relation := range authority.Program.Relations {
			if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact ||
				len(relation.ToIDs) != 1 {
				continue
			}
			if _, live := reached[relation.FromID]; !live {
				continue
			}
			target := repository.objects[relation.ToIDs[0]]
			if target.External != nil {
				continue
			}
			if _, live := reached[target.ID]; !live {
				reached[target.ID] = struct{}{}
				changed = true
			}
		}
	}
	stdlibByImporter := make(map[string]map[string]struct{})
	for _, dependency := range authority.Dependencies.Dependencies {
		if dependency.Kind != dependencies.KindStdlib {
			continue
		}
		for _, importerRef := range dependency.ImporterRefs {
			if stdlibByImporter[importerRef] == nil {
				stdlibByImporter[importerRef] = make(map[string]struct{})
			}
			stdlibByImporter[importerRef][dependency.PackagePath] = struct{}{}
		}
	}
	for _, function := range repository.functions {
		if function.object == nil || function.source.fact.Class != SourceProduction ||
			function.object.Visibility != programindex.VisibilityPublic {
			continue
		}
		directory := path.Dir(function.source.fact.Path)
		if _, admitted := admittedDirectories[directory]; admitted {
			continue
		}
		if _, live := reached[function.object.ID]; live {
			continue
		}
		importer, found := repository.importerByDir[directory]
		if !found {
			continue
		}
		stdlibParameterCall := false
		for _, relation := range authority.Program.Relations {
			if relation.FromID != function.object.ID || relation.Kind != programindex.RelationInvokesExternal ||
				relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil {
				continue
			}
			target := repository.objects[relation.ToIDs[0]]
			if target.External == nil {
				continue
			}
			if _, found := stdlibByImporter[importer.Ref][target.External.PackagePath]; !found {
				continue
			}
			parameterNames := parameterNamesForImport(function, target.External.PackagePath)
			call := repository.callAtLocation(function, *relation.Location)
			if call == nil {
				continue
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			identifier, directParameter := selector.X.(*ast.Ident)
			if ok && directParameter {
				_, stdlibParameterCall = parameterNames[identifier.Name]
			}
			if stdlibParameterCall {
				break
			}
		}
		if !stdlibParameterCall {
			continue
		}
		evidence, err := repository.functionEvidence(function)
		if err != nil {
			return nil, err
		}
		result = append(result, newH1Exclusion(
			"stdlib_helper", function.object.ID, H1ExcludedNotExternalBoundary, []H1Evidence{evidence},
		))
	}
	return result, nil
}

func compatibleTestMethod(function *h1ParsedFunction, candidates []struct {
	source *h1ParsedSource
	field  *ast.Field
}) bool {
	for _, candidate := range candidates {
		methodType, ok := candidate.field.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		matchedName := false
		for _, name := range candidate.field.Names {
			if name.Name == function.decl.Name.Name {
				matchedName = true
				break
			}
		}
		if matchedName && reflect.DeepEqual(
			fieldListTypeShapes(function.source, function.decl.Type.Params),
			fieldListTypeShapes(candidate.source, methodType.Params),
		) && reflect.DeepEqual(
			fieldListTypeShapes(function.source, function.decl.Type.Results),
			fieldListTypeShapes(candidate.source, methodType.Results),
		) {
			return true
		}
	}
	return false
}

func fieldListTypeShapes(source *h1ParsedSource, fields *ast.FieldList) []string {
	if fields == nil {
		return []string{}
	}
	result := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		shape := expressionTypeShape(source, field.Type)
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			result = append(result, shape)
		}
	}
	return result
}

func expressionTypeShape(source *h1ParsedSource, expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		if qualifier, ok := value.X.(*ast.Ident); ok {
			if importPath := source.imports[qualifier.Name]; importPath != "" {
				return importPath + "." + value.Sel.Name
			}
		}
		return expressionTypeShape(source, value.X) + "." + value.Sel.Name
	case *ast.StarExpr:
		return "*" + expressionTypeShape(source, value.X)
	case *ast.ArrayType:
		prefix := "[]"
		if value.Len != nil {
			prefix = "[n]"
		}
		return prefix + expressionTypeShape(source, value.Elt)
	case *ast.MapType:
		return "map[" + expressionTypeShape(source, value.Key) + "]" + expressionTypeShape(source, value.Value)
	case *ast.Ellipsis:
		return "..." + expressionTypeShape(source, value.Elt)
	case *ast.ChanType:
		return "chan:" + strconv.Itoa(int(value.Dir)) + ":" + expressionTypeShape(source, value.Value)
	default:
		return fmt.Sprintf("%T", expression)
	}
}

func parameterNamesForImport(function *h1ParsedFunction, packagePath string) map[string]struct{} {
	result := make(map[string]struct{})
	if function == nil || function.decl.Type.Params == nil {
		return result
	}
	for _, field := range function.decl.Type.Params.List {
		if !typeUsesImport(function.source, field.Type, packagePath) {
			continue
		}
		for _, name := range field.Names {
			result[name.Name] = struct{}{}
		}
	}
	return result
}

func typeUsesImport(source *h1ParsedSource, expression ast.Expr, packagePath string) bool {
	for {
		switch value := expression.(type) {
		case *ast.StarExpr:
			expression = value.X
		case *ast.ArrayType:
			expression = value.Elt
		case *ast.Ellipsis:
			expression = value.Elt
		case *ast.SelectorExpr:
			qualifier, ok := value.X.(*ast.Ident)
			return ok && source.imports[qualifier.Name] == packagePath
		default:
			return false
		}
	}
}

func (repository *h1Repository) generatedEvidence(parsedType *h1ParsedType) ([]H1Evidence, error) {
	result := make([]H1Evidence, 0, 2)
	for _, group := range parsedType.source.file.Comments {
		for _, comment := range group.List {
			if strings.Contains(comment.Text, "Code generated") {
				evidence, err := repository.evidence(parsedType.source, comment, strings.TrimPrefix(comment.Text, "// "), "")
				if err != nil {
					return nil, err
				}
				result = append(result, evidence)
				break
			}
		}
	}
	typeEvidence, err := repository.typeEvidence(parsedType)
	if err != nil {
		return nil, err
	}
	result = append(result, typeEvidence)
	return canonicalizeH1Evidence(result), nil
}

func (repository *h1Repository) typeEvidence(parsedType *h1ParsedType) (H1Evidence, error) {
	kind := "type"
	switch parsedType.spec.Type.(type) {
	case *ast.StructType:
		kind = "struct"
	case *ast.InterfaceType:
		kind = "interface"
	}
	authorityID := ""
	if parsedType.object != nil {
		authorityID = parsedType.object.ID
	}
	return repository.evidence(parsedType.source, parsedType.spec.Name, parsedType.spec.Name.Name+" "+kind, authorityID)
}

func (repository *h1Repository) interfaceEvidence(parsedType *h1ParsedType) (H1Evidence, error) {
	return repository.evidence(parsedType.source, parsedType.spec.Name, parsedType.spec.Name.Name+" interface", parsedType.object.ID)
}

func (repository *h1Repository) functionEvidence(function *h1ParsedFunction) (H1Evidence, error) {
	authorityID := ""
	if function.object != nil {
		authorityID = function.object.ID
	}
	return repository.evidence(function.source, function.decl.Name, function.decl.Name.Name, authorityID)
}

func (repository *h1Repository) callEvidence(
	function *h1ParsedFunction,
	call *ast.CallExpr,
	authorityID string,
) (H1Evidence, error) {
	if function == nil || call == nil {
		return H1Evidence{}, fmt.Errorf("client recipe H1: call evidence cannot be restored")
	}
	return repository.evidence(function.source, call.Fun, repository.nodeText(function.source, call.Fun), authorityID)
}

func (repository *h1Repository) callAtLocation(function *h1ParsedFunction, location programindex.Location) *ast.CallExpr {
	if function == nil || function.decl.Body == nil {
		return nil
	}
	var result *ast.CallExpr
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || result != nil {
			return true
		}
		position := repository.files.Position(call.Pos())
		if position.Filename == location.Path && position.Line == location.Line && position.Column == location.Column {
			result = call
			return false
		}
		return true
	})
	return result
}

func (repository *h1Repository) locationWithinNode(
	source *h1ParsedSource,
	location programindex.Location,
	node ast.Node,
) bool {
	start := repository.files.Position(node.Pos())
	end := repository.files.Position(node.End())
	if start.Filename != source.fact.Path || end.Filename != source.fact.Path || location.Path != source.fact.Path {
		return false
	}
	return (location.Line > start.Line || location.Line == start.Line && location.Column >= start.Column) &&
		(location.Line < end.Line || location.Line == end.Line && location.Column <= end.Column)
}

func (repository *h1Repository) typeInDirectory(directory, name string) *h1ParsedType {
	for _, parsedType := range repository.types {
		if path.Dir(parsedType.source.fact.Path) == directory && parsedType.spec.Name.Name == name {
			return parsedType
		}
	}
	return nil
}

func (repository *h1Repository) methodsForType(directory, receiver, name string) []*h1ParsedFunction {
	result := make([]*h1ParsedFunction, 0, 1)
	for _, function := range repository.functions {
		if path.Dir(function.source.fact.Path) == directory && function.receiver == receiver && function.decl.Name.Name == name {
			result = append(result, function)
		}
	}
	return result
}

func assignedIdentifier(body *ast.BlockStmt, target *ast.CallExpr) string {
	if body == nil || target == nil {
		return ""
	}
	result := ""
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || result != "" {
			return true
		}
		for rightIndex, expression := range assignment.Rhs {
			if expression != target {
				continue
			}
			leftIndex := rightIndex
			if len(assignment.Rhs) == 1 {
				leftIndex = 0
			}
			if leftIndex >= len(assignment.Lhs) {
				return false
			}
			identifier, ok := assignment.Lhs[leftIndex].(*ast.Ident)
			if ok {
				result = identifier.Name
			}
			return false
		}
		return true
	})
	return result
}

func directIdentifierArgument(call *ast.CallExpr, name string) int {
	for index, argument := range call.Args {
		identifier, ok := argument.(*ast.Ident)
		if ok && identifier.Name == name {
			return index
		}
	}
	return -1
}

func parameterTypeNameAt(fields *ast.FieldList, argumentIndex int) string {
	if fields == nil || argumentIndex < 0 {
		return ""
	}
	position := 0
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		if argumentIndex >= position && argumentIndex < position+count {
			return expressionTypeName(field.Type)
		}
		position += count
	}
	return ""
}

func storedFieldForParameter(body *ast.BlockStmt, parameter string) (string, string) {
	ownerType, fieldName := "", ""
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || ownerType != "" {
			return true
		}
		for _, element := range literal.Elts {
			keyValue, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, keyOK := keyValue.Key.(*ast.Ident)
			value, valueOK := keyValue.Value.(*ast.Ident)
			if keyOK && valueOK && value.Name == parameter {
				ownerType, fieldName = expressionTypeName(literal.Type), key.Name
				return false
			}
		}
		return true
	})
	return ownerType, fieldName
}

func findMatchingCall(body *ast.BlockStmt, matches func(*ast.CallExpr) bool) *ast.CallExpr {
	var result *ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && result == nil && matches(call) {
			result = call
			return false
		}
		return result == nil
	})
	return result
}

func callMatchesPackageFunction(
	source *h1ParsedSource,
	callee ast.Expr,
	packagePath, functionName string,
	samePackage bool,
) bool {
	switch value := callee.(type) {
	case *ast.Ident:
		return samePackage && value.Name == functionName
	case *ast.SelectorExpr:
		identifier, ok := value.X.(*ast.Ident)
		return ok && source.imports[identifier.Name] == packagePath && value.Sel.Name == functionName
	default:
		return false
	}
}

func assertionForResult(body *ast.BlockStmt, after token.Pos, result string) ast.Expr {
	var match ast.Expr
	ast.Inspect(body, func(node ast.Node) bool {
		statement, ok := node.(*ast.IfStmt)
		if !ok || match != nil || statement.Pos() <= after || !expressionContainsIdentifier(statement.Cond, result) {
			return true
		}
		match = statement.Cond
		return false
	})
	return match
}

func expressionContainsIdentifier(expression ast.Expr, name string) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func proseEvidence(source *h1ParsedSource) (H1Evidence, error) {
	for index, line := range strings.Split(string(source.raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return H1Evidence{
			Path: source.fact.Path, SourceSHA256: source.fact.SHA256, Line: index + 1,
			Column: strings.Index(line, trimmed) + 1, Symbol: trimmed,
		}, nil
	}
	return H1Evidence{}, fmt.Errorf("client recipe H1: prose source %s has no positive locator", source.fact.Path)
}

func newH1Exclusion(kind, origin string, reason H1ExclusionReason, evidence []H1Evidence) H1Excluded {
	value := H1Excluded{Kind: kind, OriginID: origin, Reason: reason, Evidence: canonicalizeH1Evidence(evidence)}
	value.ID = h1ExcludedID(value.Kind, value.OriginID)
	return value
}

func buildH1RoleEvidence(values map[H1Role][]H1Evidence) ([]H1RoleEvidence, map[H1Role]struct{}) {
	result := make([]H1RoleEvidence, 0, len(values))
	present := make(map[H1Role]struct{}, len(values))
	for _, role := range h1Roles {
		evidence := canonicalizeH1Evidence(values[role])
		if len(evidence) == 0 {
			continue
		}
		result = append(result, H1RoleEvidence{Role: role, Evidence: evidence})
		present[role] = struct{}{}
	}
	return result, present
}

func relationLocationKey(value programindex.Relation) string {
	if value.Location == nil {
		return value.ID
	}
	return fmt.Sprintf("%s:%09d:%09d:%s", value.Location.Path, value.Location.Line, value.Location.Column, value.ID)
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func h1ObservedUniverse(instances []H1Instance, excluded []H1Excluded) H1ObservedUniverse {
	result := H1ObservedUniverse{H0Candidates: len(instances)}
	for _, row := range excluded {
		switch row.Kind {
		case "external_dependency":
			result.H0Candidates++
			if row.Reason == H1ExcludedGenerated {
				result.GeneratedH0Groups++
			}
		case "test_type":
			result.QualifyingTestFakes++
		case "prose":
			result.ProseCandidates++
		case "stdlib_helper":
			result.StdlibHelpers++
		}
	}
	return result
}
