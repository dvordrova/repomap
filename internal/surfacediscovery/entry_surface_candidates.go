package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/token"
	"go/types"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/secretscan"
	"golang.org/x/tools/go/ssa"
)

const (
	maxEntrySurfaceTokenBytes = 128
	maxEntrySurfaceValueBytes = 128
)

type rawEntrySurfaceCandidate struct {
	id     string
	owner  *ssa.Function
	form   entrycall.SurfaceCandidateForm
	sketch string
	site   entrycall.Location
	facts  []entrycall.ExactSurfaceFact
}

// observeEntryCall retains only the repository-local static adjacency needed
// to bind syntax candidates to an exact process/init closure. It observes the
// existing instruction pass and never changes DirectCallIndex authority.
func (builder *directCallIndexBuilder) observeEntryCall(a *analyzer, call ssa.CallInstruction) {
	if builder == nil || builder.entryCalls == nil || a == nil ||
		call == nil || call.Parent() == nil || !a.isRepositoryFunction(call.Parent()) {
		return
	}
	common := call.Common()
	if common == nil || common.IsInvoke() {
		return
	}
	callee := common.StaticCallee()
	if callee == nil {
		return
	}
	if !a.isRepositoryFunction(callee) {
		return
	}
	caller := entrySurfaceCanonicalFunction(call.Parent())
	callee = entrySurfaceCanonicalFunction(callee)
	if caller == nil || callee == nil {
		return
	}
	if builder.entryCalls.repositoryCalls[caller] == nil {
		builder.entryCalls.repositoryCalls[caller] = make(map[*ssa.Function]struct{})
	}
	builder.entryCalls.repositoryCalls[caller][callee] = struct{}{}
}

func entrySurfaceCanonicalFunction(function *ssa.Function) *ssa.Function {
	if function == nil {
		return nil
	}
	if origin := function.Origin(); origin != nil {
		return origin
	}
	return function
}

// recordEntrySurfaceSyntaxCandidates walks only the already-loaded,
// build-selected syntax and type information. Direct calls and keyed
// composites share one structural admission path; no framework name or field
// allowlist participates.
func (builder *directCallIndexBuilder) recordEntrySurfaceSyntaxCandidates(
	a *analyzer,
	entrypoints []*ssa.Function,
) {
	if builder == nil || builder.entryCalls == nil || a == nil ||
		a.program == nil || builder.state != DirectCallIndexReady {
		return
	}
	reachable := make(map[*ssa.Function]bool)
	for _, root := range entrySurfaceProcessRoots(a, entrypoints) {
		for function := range builder.entryCalls.surfaceReachable(a, root) {
			reachable[function] = true
		}
	}
	owners := make(map[token.Pos]*ssa.Function)
	for _, function := range a.orderedFunctions() {
		if function == nil || !a.isRepositoryFunction(function) || function.Syntax() == nil {
			continue
		}
		function = entrySurfaceCanonicalFunction(function)
		owners[function.Syntax().Pos()] = function
	}
	packagesByPath := make(map[string]*ssa.Package)
	for _, pkg := range a.packages {
		if pkg != nil && pkg.Pkg != nil {
			packagesByPath[pkg.Pkg.Path()] = pkg
		}
	}
	packagePaths := make([]string, 0, len(a.packageFacts))
	for packagePath, facts := range a.packageFacts {
		if facts != nil && facts.TypesInfo != nil && entrySurfaceRepositoryPackage(a, packagePath) {
			packagePaths = append(packagePaths, packagePath)
		}
	}
	sort.Strings(packagePaths)
	for _, packagePath := range packagePaths {
		if a.ctx.Err() != nil {
			return
		}
		facts := a.packageFacts[packagePath]
		var packageInit *ssa.Function
		if pkg := packagesByPath[packagePath]; pkg != nil {
			packageInit = entrySurfaceCanonicalFunction(pkg.Func("init"))
		}
		files := append([]*ast.File(nil), facts.Syntax...)
		sort.Slice(files, func(i, j int) bool {
			return a.location(files[i].Pos()).Path < a.location(files[j].Pos()).Path
		})
		for _, file := range files {
			if file == nil {
				continue
			}
			ast.Walk(&entrySurfaceSyntaxVisitor{
				a: a, sidecar: builder.entryCalls, info: facts.TypesInfo,
				owners: owners, owner: packageInit, reachable: reachable,
			}, file)
		}
	}
}

type entrySurfaceSyntaxVisitor struct {
	a         *analyzer
	sidecar   *entryCallSidecar
	info      *types.Info
	owners    map[token.Pos]*ssa.Function
	owner     *ssa.Function
	reachable map[*ssa.Function]bool
}

func (visitor *entrySurfaceSyntaxVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil || visitor == nil || visitor.a == nil || visitor.a.ctx.Err() != nil {
		return nil
	}
	switch current := node.(type) {
	case *ast.FuncDecl:
		return visitor.withOwner(visitor.owners[current.Pos()])
	case *ast.FuncLit:
		return visitor.withOwner(visitor.owners[current.Pos()])
	case *ast.CallExpr:
		visitor.recordDirectCall(current)
	case *ast.CompositeLit:
		visitor.recordKeyedComposite(current)
	}
	return visitor
}

func (visitor *entrySurfaceSyntaxVisitor) withOwner(owner *ssa.Function) *entrySurfaceSyntaxVisitor {
	copy := *visitor
	copy.owner = entrySurfaceCanonicalFunction(owner)
	return &copy
}

func (visitor *entrySurfaceSyntaxVisitor) recordDirectCall(call *ast.CallExpr) {
	if visitor.owner == nil || !visitor.a.isRepositoryFunction(visitor.owner) || call == nil {
		return
	}
	called := entrySurfaceExactCalledFunction(call.Fun, visitor.info)
	if called == nil || called.Name() == "" || !entrySurfaceSafeToken(called.Name()) {
		return
	}
	site := entryCallLocation(visitor.a.location(call.Pos()))
	if !validRepositoryDirectCallLocation(Location(site)) {
		return
	}
	callableByArgument := make([][]entrycall.ExactSurfaceFact, len(call.Args))
	for index, argument := range call.Args {
		position := index + 1
		label := "argument " + strconv.Itoa(position)
		callableByArgument[index] = visitor.callableFacts(argument, position, label, 0)
	}
	facts := []entrycall.ExactSurfaceFact{}
	terminalLocation := entryCallLocation(visitor.a.location(call.Fun.Pos()))
	facts = append(facts, visitor.sidecar.surfaceFact(
		entrycall.SurfaceFactToken, 0, "terminal selector", called.Name(), terminalLocation,
	))
	stringsFound := 0
	for index, argument := range call.Args {
		position := index + 1
		label := "argument " + strconv.Itoa(position)
		if value, ok := typedStringValue(argument, visitor.info); ok {
			if stringFacts, safe := visitor.sidecar.safeDirectCallStringFacts(
				position, label, value, entryCallLocation(visitor.a.location(argument.Pos())),
			); safe {
				facts = append(facts, stringFacts...)
				stringsFound++
			}
		}
		facts = append(facts, callableByArgument[index]...)
	}
	if stringsFound == 0 {
		return
	}
	owner := entrySurfaceCanonicalFunction(visitor.owner)
	visitor.sidecar.recordSurfaceCandidate(rawEntrySurfaceCandidate{
		owner: owner,
		form:  entrycall.SurfaceCandidateDirectCall, sketch: called.Name(), site: site, facts: facts,
	}, visitor.reachable[owner])
}

func (visitor *entrySurfaceSyntaxVisitor) recordKeyedComposite(literal *ast.CompositeLit) {
	if visitor.owner == nil || !visitor.a.isRepositoryFunction(visitor.owner) || literal == nil {
		return
	}
	sketch := entrySurfaceCompositeSketch(literal, visitor.info)
	if sketch == "" || !entrySurfaceSafeToken(sketch) {
		return
	}
	site := entryCallLocation(visitor.a.location(literal.Pos()))
	if !validRepositoryDirectCallLocation(Location(site)) {
		return
	}
	callableByField := make(map[int][]entrycall.ExactSurfaceFact)
	callablesFound := 0
	for index, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := field.Key.(*ast.Ident)
		if !ok || !entrySurfaceSafeToken(key.Name) {
			continue
		}
		position := index + 1
		callableFacts := visitor.callableFacts(field.Value, position, key.Name, 0)
		for factIndex := range callableFacts {
			callableFacts[factIndex].Label = key.Name
			callableFacts[factIndex].ID = stableEntrySurfaceID(
				"entry-surface-fact", string(callableFacts[factIndex].Kind),
				strconv.Itoa(callableFacts[factIndex].Position), callableFacts[factIndex].Label,
				callableFacts[factIndex].Value, entryCallLocationKey(callableFacts[factIndex].Location),
			)
		}
		callableByField[index] = callableFacts
		callablesFound += len(callableFacts)
	}
	if callablesFound == 0 {
		return
	}
	facts := []entrycall.ExactSurfaceFact{}
	stringsFound := 0
	for index, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := field.Key.(*ast.Ident)
		if !ok || !entrySurfaceSafeToken(key.Name) {
			continue
		}
		position := index + 1
		if value, ok := typedStringValue(field.Value, visitor.info); ok {
			if fact, safe := visitor.sidecar.safeSurfaceStringFact(
				position, key.Name, value, entryCallLocation(visitor.a.location(field.Value.Pos())),
			); safe {
				facts = append(facts, fact)
				stringsFound++
			}
		}
		facts = append(facts, callableByField[index]...)
	}
	if stringsFound == 0 {
		return
	}
	owner := entrySurfaceCanonicalFunction(visitor.owner)
	visitor.sidecar.recordSurfaceCandidate(rawEntrySurfaceCandidate{
		owner: owner,
		form:  entrycall.SurfaceCandidateKeyedComposite, sketch: sketch, site: site, facts: facts,
	}, visitor.reachable[owner])
}

func (visitor *entrySurfaceSyntaxVisitor) callableFacts(
	expression ast.Expr,
	position int,
	label string,
	depth int,
) []entrycall.ExactSurfaceFact {
	if expression == nil || visitor.info == nil || depth > 3 {
		return nil
	}
	switch current := expression.(type) {
	case *ast.ParenExpr:
		return visitor.callableFacts(current.X, position, label, depth+1)
	case *ast.CallExpr:
		facts := []entrycall.ExactSurfaceFact{}
		for _, argument := range current.Args {
			facts = append(facts, visitor.callableFacts(argument, position, label, depth+1)...)
		}
		return compactEntrySurfaceFacts(facts)
	case *ast.FuncLit:
		location := entryCallLocation(visitor.a.location(current.Pos()))
		if !validRepositoryDirectCallLocation(Location(location)) {
			return nil
		}
		return []entrycall.ExactSurfaceFact{visitor.sidecar.surfaceFact(
			entrycall.SurfaceFactCallable, position, label, "func literal", location,
		)}
	}
	function := entrySurfaceExpressionFunction(expression, visitor.info)
	if function == nil || function.Pkg() == nil ||
		!entrySurfaceRepositoryPackage(visitor.a, function.Pkg().Path()) {
		return nil
	}
	location := entryCallLocation(visitor.a.location(function.Pos()))
	if !validRepositoryDirectCallLocation(Location(location)) {
		return nil
	}
	return []entrycall.ExactSurfaceFact{visitor.sidecar.surfaceFact(
		entrycall.SurfaceFactCallable, position, label, entrySurfaceCallableName(function), location,
	)}
}

func entrySurfaceCallableName(function *types.Func) string {
	if function == nil {
		return ""
	}
	signature, _ := function.Type().(*types.Signature)
	receiver := receiverName(signature)
	if receiver == "" {
		return function.Name()
	}
	return "(" + receiver + ")." + function.Name()
}

func entrySurfaceExactCalledFunction(expression ast.Expr, info *types.Info) *types.Func {
	if expression == nil || info == nil {
		return nil
	}
	for {
		switch current := expression.(type) {
		case *ast.ParenExpr:
			expression = current.X
		case *ast.IndexExpr:
			expression = current.X
		case *ast.IndexListExpr:
			expression = current.X
		default:
			if selector, ok := expression.(*ast.SelectorExpr); ok {
				if selection := info.Selections[selector]; selection != nil &&
					entrySurfaceDynamicReceiver(selection.Recv()) {
					return nil
				}
			}
			return entrySurfaceExpressionFunction(expression, info)
		}
	}
}

func entrySurfaceDynamicReceiver(receiver types.Type) bool {
	if receiver == nil {
		return false
	}
	if _, parameter := receiver.(*types.TypeParam); parameter {
		return true
	}
	_, dynamic := receiver.Underlying().(*types.Interface)
	return dynamic
}

func entrySurfaceExpressionFunction(expression ast.Expr, info *types.Info) *types.Func {
	if expression == nil || info == nil {
		return nil
	}
	var object types.Object
	switch current := expression.(type) {
	case *ast.Ident:
		object = info.ObjectOf(current)
	case *ast.SelectorExpr:
		if selection := info.Selections[current]; selection != nil {
			object = selection.Obj()
		} else {
			object = info.ObjectOf(current.Sel)
		}
	}
	function, _ := object.(*types.Func)
	return function
}

func entrySurfaceCompositeSketch(literal *ast.CompositeLit, info *types.Info) string {
	if literal == nil || info == nil {
		return ""
	}
	typeValue := info.TypeOf(literal)
	if typeValue == nil {
		typeValue = info.TypeOf(literal.Type)
	}
	if pointer, ok := typeValue.(*types.Pointer); ok {
		typeValue = pointer.Elem()
	}
	typeValue = types.Unalias(typeValue)
	if named, ok := typeValue.(*types.Named); ok && named.Obj() != nil {
		return named.Obj().Name()
	}
	return ""
}

func (sidecar *entryCallSidecar) safeSurfaceStringFact(
	position int,
	label string,
	value string,
	location entrycall.Location,
) (entrycall.ExactSurfaceFact, bool) {
	if sidecar == nil {
		return entrycall.ExactSurfaceFact{}, false
	}
	if !entrySurfaceSafeValue(label, value) || !validRepositoryDirectCallLocation(Location(location)) {
		sidecar.surfaceCoverage.UnsafeSurfaceCandidateFactsExcluded++
		return entrycall.ExactSurfaceFact{}, false
	}
	return exactEntrySurfaceFact(entrycall.SurfaceFactString, position, label, value, location), true
}

// safeDirectCallStringFacts keeps ordinary safe string literals unchanged.
// Go 1.22 ServeMux patterns may instead carry an exact method and path in one
// literal. Split only that closed standard-method shape so the refs-only model
// can bind independent backend-owned facts to the independent HTTP slots.
func (sidecar *entryCallSidecar) safeDirectCallStringFacts(
	position int,
	label string,
	value string,
	location entrycall.Location,
) ([]entrycall.ExactSurfaceFact, bool) {
	original, safe := sidecar.safeSurfaceStringFact(position, label, value, location)
	if !safe {
		return nil, false
	}
	method, path, combined := splitEntrySurfaceHTTPPattern(value)
	if !combined {
		return []entrycall.ExactSurfaceFact{original}, true
	}
	methodLabel := label + " method"
	pathLabel := label + " path"
	if !entrySurfaceSafeValue(methodLabel, method) || !entrySurfaceSafeValue(pathLabel, path) {
		sidecar.surfaceCoverage.UnsafeSurfaceCandidateFactsExcluded++
		return nil, false
	}
	return []entrycall.ExactSurfaceFact{
		exactEntrySurfaceFact(entrycall.SurfaceFactToken, position, methodLabel, method, location),
		exactEntrySurfaceFact(entrycall.SurfaceFactString, position, pathLabel, path, location),
	}, true
}

func splitEntrySurfaceHTTPPattern(value string) (string, string, bool) {
	separator := strings.IndexByte(value, ' ')
	if separator <= 0 || separator == len(value)-1 ||
		strings.IndexByte(value[separator+1:], ' ') >= 0 {
		return "", "", false
	}
	method, path := value[:separator], value[separator+1:]
	if !entrySurfaceStandardHTTPMethod(method) || !strings.HasPrefix(path, "/") ||
		strings.IndexFunc(path, unicode.IsSpace) >= 0 {
		return "", "", false
	}
	return method, path, true
}

func entrySurfaceStandardHTTPMethod(value string) bool {
	switch strings.ToUpper(value) {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead,
		http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
		return true
	default:
		return false
	}
}

func (sidecar *entryCallSidecar) surfaceFact(
	kind entrycall.SurfaceFactKind,
	position int,
	label string,
	value string,
	location entrycall.Location,
) entrycall.ExactSurfaceFact {
	return exactEntrySurfaceFact(kind, position, label, value, location)
}

func exactEntrySurfaceFact(
	kind entrycall.SurfaceFactKind,
	position int,
	label string,
	value string,
	location entrycall.Location,
) entrycall.ExactSurfaceFact {
	fact := entrycall.ExactSurfaceFact{
		Kind: kind, Position: position, Label: label, Value: value, Location: location,
	}
	fact.ID = stableEntrySurfaceID(
		"entry-surface-fact", string(kind), strconv.Itoa(position), label, value,
		entryCallLocationKey(location),
	)
	return fact
}

func (sidecar *entryCallSidecar) recordSurfaceCandidate(candidate rawEntrySurfaceCandidate, reachable bool) {
	if sidecar == nil || candidate.owner == nil || !candidate.form.Valid() ||
		candidate.sketch == "" || len(candidate.facts) == 0 {
		return
	}
	originalFactCount := len(compactEntrySurfaceFacts(candidate.facts))
	sidecar.surfaceCoverage.SurfaceCandidateFactsConsidered += originalFactCount
	candidate.facts = boundEntrySurfaceFacts(candidate.facts)
	if originalFactCount > len(candidate.facts) {
		sidecar.surfaceCoverage.SurfaceCandidateFactLimitExcluded += originalFactCount - len(candidate.facts)
	}
	if !entrySurfaceCandidateAdmissible(candidate.form, candidate.facts) {
		return
	}
	candidate.id = stableEntrySurfaceCandidateID(candidate)
	sidecar.surfaceCoverage.SurfaceCandidatesConsidered++
	if !reachable {
		sidecar.surfaceCoverage.UnreachableSurfaceCandidatesExcluded++
		return
	}
	if _, duplicate := sidecar.surfaceCandidates[candidate.id]; duplicate {
		return
	}
	if len(sidecar.surfaceCandidates) < entrycall.MaxRawSurfaceCandidates {
		sidecar.surfaceCandidates[candidate.id] = candidate
		return
	}
	worstID := ""
	var worst rawEntrySurfaceCandidate
	for id, retained := range sidecar.surfaceCandidates {
		if worstID == "" || entrySurfaceRawCandidateLess(worst, retained) {
			worstID, worst = id, retained
		}
	}
	sidecar.surfaceCoverage.SurfaceCandidateLimitExcluded++
	if entrySurfaceRawCandidateLess(candidate, worst) {
		delete(sidecar.surfaceCandidates, worstID)
		sidecar.surfaceCandidates[candidate.id] = candidate
	}
}

func boundEntrySurfaceFacts(facts []entrycall.ExactSurfaceFact) []entrycall.ExactSurfaceFact {
	facts = compactEntrySurfaceFacts(facts)
	sort.Slice(facts, func(i, j int) bool { return entrySurfaceFactLess(facts[i], facts[j]) })
	if len(facts) <= entrycall.MaxRawSurfaceFactsPerCandidate {
		return facts
	}
	selected := make(map[string]entrycall.ExactSurfaceFact)
	for _, fact := range facts {
		if entrySurfacePathLikeFact(fact) {
			selected[fact.ID] = fact
			break
		}
	}
	for _, required := range []entrycall.SurfaceFactKind{
		entrycall.SurfaceFactToken, entrycall.SurfaceFactString, entrycall.SurfaceFactCallable,
	} {
		for _, fact := range facts {
			if fact.Kind == required {
				selected[fact.ID] = fact
				break
			}
		}
	}
	for _, fact := range facts {
		if len(selected) >= entrycall.MaxRawSurfaceFactsPerCandidate {
			break
		}
		selected[fact.ID] = fact
	}
	result := make([]entrycall.ExactSurfaceFact, 0, len(selected))
	for _, fact := range selected {
		result = append(result, fact)
	}
	sort.Slice(result, func(i, j int) bool { return entrySurfaceFactLess(result[i], result[j]) })
	return result
}

func compactEntrySurfaceFacts(facts []entrycall.ExactSurfaceFact) []entrycall.ExactSurfaceFact {
	byID := make(map[string]entrycall.ExactSurfaceFact, len(facts))
	for _, fact := range facts {
		if fact.ID != "" {
			byID[fact.ID] = fact
		}
	}
	result := make([]entrycall.ExactSurfaceFact, 0, len(byID))
	for _, fact := range byID {
		result = append(result, fact)
	}
	sort.Slice(result, func(i, j int) bool { return entrySurfaceFactLess(result[i], result[j]) })
	return result
}

func entrySurfaceFactLess(left, right entrycall.ExactSurfaceFact) bool {
	if left.Position != right.Position {
		return left.Position < right.Position
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Label != right.Label {
		return left.Label < right.Label
	}
	if left.Value != right.Value {
		return left.Value < right.Value
	}
	return entryCallLocationKey(left.Location) < entryCallLocationKey(right.Location)
}

func entrySurfaceFactsHaveKinds(facts []entrycall.ExactSurfaceFact, kinds ...entrycall.SurfaceFactKind) bool {
	found := make(map[entrycall.SurfaceFactKind]bool, len(kinds))
	for _, fact := range facts {
		found[fact.Kind] = true
	}
	for _, kind := range kinds {
		if !found[kind] {
			return false
		}
	}
	return true
}

func entrySurfaceCandidateAdmissible(
	form entrycall.SurfaceCandidateForm,
	facts []entrycall.ExactSurfaceFact,
) bool {
	if entrySurfaceFactsHaveKinds(facts, entrycall.SurfaceFactString, entrycall.SurfaceFactCallable) {
		return true
	}
	if form != entrycall.SurfaceCandidateDirectCall {
		return false
	}
	for _, pathFact := range facts {
		if !entrySurfacePathLikeFact(pathFact) {
			continue
		}
		for _, companion := range facts {
			if companion.ID != pathFact.ID &&
				(companion.Kind == entrycall.SurfaceFactString || companion.Kind == entrycall.SurfaceFactToken) {
				return true
			}
		}
	}
	return false
}

func entrySurfacePathLikeFact(fact entrycall.ExactSurfaceFact) bool {
	return fact.Kind == entrycall.SurfaceFactString && strings.HasPrefix(fact.Value, "/")
}

func entrySurfaceCandidateRank(form entrycall.SurfaceCandidateForm, facts []entrycall.ExactSurfaceFact) int {
	if entrySurfaceFactsHaveKinds(facts, entrycall.SurfaceFactString, entrycall.SurfaceFactCallable) {
		return 0
	}
	if entrySurfaceCandidateAdmissible(form, facts) {
		return 1
	}
	return 2
}

func stableEntrySurfaceCandidateID(candidate rawEntrySurfaceCandidate) string {
	fields := []string{
		string(candidate.form), candidate.sketch, entryCallLocationKey(candidate.site),
	}
	for _, fact := range candidate.facts {
		fields = append(fields, fact.ID)
	}
	return stableEntrySurfaceID("entry-surface-candidate", fields...)
}

func entrySurfaceRawCandidateLess(left, right rawEntrySurfaceCandidate) bool {
	leftRank := entrySurfaceCandidateRank(left.form, left.facts)
	rightRank := entrySurfaceCandidateRank(right.form, right.facts)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	if left.site.Path != right.site.Path {
		return left.site.Path < right.site.Path
	}
	if left.site.Line != right.site.Line {
		return left.site.Line < right.site.Line
	}
	if left.site.Column != right.site.Column {
		return left.site.Column < right.site.Column
	}
	if left.form != right.form {
		return left.form < right.form
	}
	if left.sketch != right.sketch {
		return left.sketch < right.sketch
	}
	return left.id < right.id
}

func stableEntrySurfaceID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		digest.Write([]byte(strconv.Itoa(len(field))))
		digest.Write([]byte{0})
		digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func entryCallLocationKey(location entrycall.Location) string {
	return location.Path + ":" + strconv.Itoa(location.Line) + ":" + strconv.Itoa(location.Column)
}

func entrySurfaceSafeToken(value string) bool {
	if value == "" || len(value) > maxEntrySurfaceTokenBytes || !utf8.ValidString(value) {
		return false
	}
	for index, character := range value {
		if character == '_' || unicode.IsLetter(character) || index > 0 && unicode.IsDigit(character) {
			continue
		}
		return false
	}
	return true
}

func entrySurfaceSafeValue(label, value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxEntrySurfaceValueBytes ||
		!utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	if _, found := secretscan.Detect(value); found {
		return false
	}
	if _, found := secretscan.Detect(label + `: "` + value + `"`); found {
		return false
	}
	return !entrySurfaceHighEntropy(value)
}

func entrySurfaceHighEntropy(value string) bool {
	if len(value) < 24 || strings.ContainsAny(value, " \t{}[]<>:") {
		return false
	}
	counts := make(map[rune]int)
	total := 0
	for _, character := range value {
		counts[character]++
		total++
	}
	if total < 24 {
		return false
	}
	entropy := 0.0
	for _, count := range counts {
		probability := float64(count) / float64(total)
		entropy -= probability * math.Log2(probability)
	}
	return entropy >= 4.25
}

func entrySurfaceRepositoryPackage(a *analyzer, packagePath string) bool {
	return a != nil && packagePath != "" && a.admittedPackages[packagePath]
}

func (sidecar *entryCallSidecar) projectSurfaceCandidates(
	a *analyzer,
	builder *directCallIndexBuilder,
	entrypoints []*ssa.Function,
) ([]entrycall.ExactSurfaceCandidate, entrycall.Coverage) {
	coverage := sidecar.surfaceCoverage
	coverage.SurfaceCandidatesIndexed = 0
	coverage.SurfaceCandidateFactsIndexed = 0
	if sidecar == nil || a == nil || builder == nil || len(sidecar.surfaceCandidates) == 0 {
		return []entrycall.ExactSurfaceCandidate{}, coverage
	}
	roots := entrySurfaceProcessRoots(a, entrypoints)
	type rootClosure struct {
		function *ssa.Function
		nodeID   string
		reached  map[*ssa.Function]bool
	}
	closures := make([]rootClosure, 0, len(roots))
	for _, root := range roots {
		root = entrySurfaceCanonicalFunction(root)
		nodeID := builder.functionNode[root]
		if root == nil || nodeID == "" {
			continue
		}
		closures = append(closures, rootClosure{
			function: root, nodeID: nodeID, reached: sidecar.surfaceReachable(a, root),
		})
	}
	sort.Slice(closures, func(i, j int) bool { return closures[i].nodeID < closures[j].nodeID })
	raw := make([]rawEntrySurfaceCandidate, 0, len(sidecar.surfaceCandidates))
	for _, candidate := range sidecar.surfaceCandidates {
		raw = append(raw, candidate)
	}
	sort.Slice(raw, func(i, j int) bool { return entrySurfaceRawCandidateLess(raw[i], raw[j]) })
	expanded := make([]entrycall.ExactSurfaceCandidate, 0, len(raw))
	for _, candidate := range raw {
		matchedRoots := 0
		for _, closure := range closures {
			if !closure.reached[entrySurfaceCanonicalFunction(candidate.owner)] {
				continue
			}
			matchedRoots++
			facts := append([]entrycall.ExactSurfaceFact(nil), candidate.facts...)
			for factIndex := range facts {
				facts[factIndex].ID = stableEntrySurfaceID(
					"entry-surface-root-fact", closure.nodeID, candidate.id, facts[factIndex].ID,
				)
			}
			exact := entrycall.ExactSurfaceCandidate{
				RootNodeID: closure.nodeID, Form: candidate.form, Sketch: candidate.sketch,
				Site: candidate.site, Facts: facts,
			}
			exact.ID = stableEntrySurfaceID(
				"entry-surface-root-candidate", closure.nodeID, candidate.id,
			)
			expanded = append(expanded, exact)
		}
		if matchedRoots == 0 {
			coverage.UnreachableSurfaceCandidatesExcluded++
		} else if matchedRoots > 1 {
			coverage.SurfaceCandidatesConsidered += matchedRoots - 1
			coverage.SurfaceCandidateFactsConsidered += (matchedRoots - 1) * len(candidate.facts)
		}
	}
	sort.Slice(expanded, func(i, j int) bool {
		leftRank := entrySurfaceCandidateRank(expanded[i].Form, expanded[i].Facts)
		rightRank := entrySurfaceCandidateRank(expanded[j].Form, expanded[j].Facts)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if expanded[i].RootNodeID != expanded[j].RootNodeID {
			return expanded[i].RootNodeID < expanded[j].RootNodeID
		}
		left, right := entryCallLocationKey(expanded[i].Site), entryCallLocationKey(expanded[j].Site)
		if left != right {
			return left < right
		}
		return expanded[i].ID < expanded[j].ID
	})
	if len(expanded) > entrycall.MaxRawSurfaceCandidates {
		coverage.SurfaceCandidateLimitExcluded += len(expanded) - entrycall.MaxRawSurfaceCandidates
		expanded = expanded[:entrycall.MaxRawSurfaceCandidates]
	}
	coverage.SurfaceCandidatesIndexed = len(expanded)
	for _, candidate := range expanded {
		coverage.SurfaceCandidateFactsIndexed += len(candidate.Facts)
	}
	return expanded, coverage
}

func entrySurfaceProcessRoots(a *analyzer, entrypoints []*ssa.Function) []*ssa.Function {
	if a == nil {
		return nil
	}
	result := make([]*ssa.Function, 0, len(entrypoints))
	for _, entrypoint := range entrypoints {
		if entrypoint == nil {
			continue
		}
		if target := a.input.AnalysisTarget; target != nil {
			if target.Kind != AnalysisTargetExecutablePackage ||
				functionPackagePath(entrypoint) != target.PackagePath {
				continue
			}
			location := a.location(entrypoint.Pos())
			matched := false
			for _, root := range target.Roots {
				if root.Path == location.Path && root.Line == location.Line {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, entrySurfaceCanonicalFunction(entrypoint))
	}
	sort.Slice(result, func(i, j int) bool { return a.functionID(result[i]) < a.functionID(result[j]) })
	return result
}

func (sidecar *entryCallSidecar) surfaceReachable(a *analyzer, root *ssa.Function) map[*ssa.Function]bool {
	reached := make(map[*ssa.Function]bool)
	if sidecar == nil || a == nil || root == nil {
		return reached
	}
	packages := importedPackagePaths(root)
	queue := []*ssa.Function{entrySurfaceCanonicalFunction(root)}
	for _, pkg := range a.packages {
		if pkg == nil || pkg.Pkg == nil || !packages[pkg.Pkg.Path()] ||
			!entrySurfaceRepositoryPackage(a, pkg.Pkg.Path()) {
			continue
		}
		if initializer := entrySurfaceCanonicalFunction(pkg.Func("init")); initializer != nil {
			queue = append(queue, initializer)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil || reached[current] || !packages[functionPackagePath(current)] {
			continue
		}
		reached[current] = true
		callees := make([]*ssa.Function, 0, len(sidecar.repositoryCalls[current]))
		for callee := range sidecar.repositoryCalls[current] {
			callee = entrySurfaceCanonicalFunction(callee)
			if callee != nil && packages[functionPackagePath(callee)] {
				callees = append(callees, callee)
			}
		}
		sort.Slice(callees, func(i, j int) bool { return a.functionID(callees[i]) < a.functionID(callees[j]) })
		queue = append(queue, callees...)
	}
	return reached
}
