package goldenmechanism

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

type resolvedCallee struct {
	decl          *functionDecl
	observationID string
}

type positionedObservation struct {
	observation Observation
	position    token.Pos
}

type functionAnalyzer struct {
	ctx      context.Context
	fset     *token.FileSet
	index    map[string]*functionDecl
	decl     *functionDecl
	function Function
	window   functionWindow
	allowed  map[string]struct{}
	env      map[string]typeInfo

	positioned []positionedObservation
	seen       map[string]struct{}
	callees    map[string]resolvedCallee
}

func analyzeFunction(
	ctx context.Context,
	fset *token.FileSet,
	index map[string]*functionDecl,
	decl *functionDecl,
	function Function,
	window functionWindow,
	allowed map[string]struct{},
) ([]Observation, []resolvedCallee) {
	analyzer := &functionAnalyzer{
		ctx: ctx, fset: fset, index: index, decl: decl, function: function,
		window: window, allowed: allowed, env: make(map[string]typeInfo),
		seen: make(map[string]struct{}), callees: make(map[string]resolvedCallee),
	}
	for name, info := range decl.params {
		analyzer.env[name] = info
	}
	if decl.receiverVar != "" {
		analyzer.env[decl.receiverVar] = typeInfo{base: decl.receiver, text: decl.receiver}
	}

	analyzer.add(
		semanticdiscovery.CapabilityStatic,
		"function_declaration",
		fmt.Sprintf("Bounded Go syntax declares %s.", decl.symbol),
		decl.symbol,
		"",
		BasisDeclaration,
		decl.decl.Name,
		"",
		nil,
	)
	if function.Seed && httpHandlerSignature(decl) {
		analyzer.add(
			semanticdiscovery.CapabilityEntry,
			"http_handler_entry_signature",
			fmt.Sprintf("Bounded Go syntax declares %s with HTTP response-writer and request parameters plus an error result; this identifies a handler-shaped local entry, not proof of runtime registration.", decl.symbol),
			decl.symbol,
			"HTTP handler signature",
			BasisDeclaration,
			decl.decl.Name,
			"",
			nil,
		)
	}

	ast.Inspect(decl.decl.Body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if analyzer.ctx.Err() != nil || !analyzer.withinWindow(node.Pos()) {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			analyzer.observeAssignment(value)
			analyzer.updateAssignmentTypes(value)
		case *ast.DeclStmt:
			analyzer.updateDeclarationTypes(value)
		case *ast.CallExpr:
			analyzer.observeCall(value)
		case *ast.IfStmt:
			analyzer.observeIf(value)
		case *ast.SwitchStmt:
			analyzer.observeSwitch(value)
		case *ast.TypeSwitchStmt:
			analyzer.observeTypeSwitch(value)
		case *ast.ReturnStmt:
			analyzer.observeReturn(value)
		}
		return true
	})
	analyzer.observeLocalErrorHandoffs()
	analyzer.addLexicalSequences()

	observations := make([]Observation, 0, len(analyzer.positioned))
	for _, positioned := range analyzer.positioned {
		observations = append(observations, positioned.observation)
	}
	callees := make([]resolvedCallee, 0, len(analyzer.callees))
	for _, callee := range analyzer.callees {
		callees = append(callees, callee)
	}
	sort.Slice(callees, func(i, j int) bool {
		if callees[i].decl.file.path != callees[j].decl.file.path {
			return callees[i].decl.file.path < callees[j].decl.file.path
		}
		return callees[i].decl.decl.Pos() < callees[j].decl.decl.Pos()
	})
	return observations, callees
}

// observeLocalErrorHandoffs records only two exact, same-function syntax
// shapes. A direct return may pass through all results of a type-resolved
// local call. A guarded handoff may bind that call's declared error result,
// check the same identifier for non-nil, and return that identifier from the
// caller's error result position in the immediately following if statement.
// This deliberately does not follow aliases, fields, closures, interfaces, or
// calls across packages.
func (analyzer *functionAnalyzer) observeLocalErrorHandoffs() {
	if analyzer.decl.decl.Body == nil || len(analyzer.decl.errorResult) == 0 {
		return
	}
	analyzer.observeLocalErrorHandoffsInBlock(analyzer.decl.decl.Body)
}

func (analyzer *functionAnalyzer) observeLocalErrorHandoffsInBlock(block *ast.BlockStmt) {
	if block == nil || analyzer.ctx.Err() != nil || !analyzer.withinWindow(block.Pos()) {
		return
	}
	for index, statement := range block.List {
		if analyzer.ctx.Err() != nil || !analyzer.withinWindow(statement.Pos()) {
			return
		}
		if returned, ok := statement.(*ast.ReturnStmt); ok {
			analyzer.observeDirectLocalErrorReturn(returned)
		}
		if assignment, ok := statement.(*ast.AssignStmt); ok && index+1 < len(block.List) {
			if guarded, ok := block.List[index+1].(*ast.IfStmt); ok {
				analyzer.observeGuardedLocalErrorReturn(assignment, guarded)
			}
		}
		ast.Inspect(statement, func(node ast.Node) bool {
			if node == nil {
				return true
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			nested, ok := node.(*ast.BlockStmt)
			if !ok {
				return true
			}
			analyzer.observeLocalErrorHandoffsInBlock(nested)
			return false
		})
	}
}

func (analyzer *functionAnalyzer) observeDirectLocalErrorReturn(statement *ast.ReturnStmt) {
	if len(statement.Results) != 1 {
		return
	}
	call, ok := statement.Results[0].(*ast.CallExpr)
	if !ok {
		return
	}
	callee := analyzer.resolveCall(call)
	if callee == nil || !matchingErrorResults(analyzer.decl, callee) {
		return
	}
	analyzer.addLocalErrorHandoff(
		call,
		callee,
		"direct_return",
		fmt.Sprintf(
			"%s directly returns all results of the type-resolved local call to %s, whose declared error result aligns with the caller's error result; this is same-function syntax, not proof that the call executes.",
			analyzer.decl.symbol,
			callee.symbol,
		),
		call,
		statement,
	)
}

func (analyzer *functionAnalyzer) observeGuardedLocalErrorReturn(
	assignment *ast.AssignStmt,
	guarded *ast.IfStmt,
) {
	if len(assignment.Rhs) != 1 || len(guarded.Body.List) == 0 || guarded.Init != nil {
		return
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	callee := analyzer.resolveCall(call)
	if callee == nil || len(assignment.Lhs) != len(callee.returns) || len(callee.errorResult) != 1 {
		return
	}
	errorIndex := callee.errorResult[0]
	if errorIndex >= len(assignment.Lhs) {
		return
	}
	binding, ok := assignment.Lhs[errorIndex].(*ast.Ident)
	if !ok || binding.Name == "_" || !isNonNilGuard(guarded.Cond, binding.Name) {
		return
	}
	var returned *ast.ReturnStmt
	for _, statement := range guarded.Body.List {
		candidate, ok := statement.(*ast.ReturnStmt)
		if !ok || !returnsErrorBinding(analyzer.decl, candidate, binding.Name) {
			continue
		}
		if returned != nil {
			return
		}
		returned = candidate
	}
	if returned == nil {
		return
	}
	returnedBinding := returned.Results[analyzer.decl.errorResult[0]]
	analyzer.addLocalErrorHandoff(
		call,
		callee,
		binding.Name,
		fmt.Sprintf(
			"%s assigns the declared error result of a type-resolved local call to %s, checks the same local binding for non-nil, and returns that binding from an error result position; this is a bounded same-function handoff.",
			analyzer.decl.symbol,
			callee.symbol,
		),
		call,
		guarded.Cond,
		returnedBinding,
	)
}

func (analyzer *functionAnalyzer) addLocalErrorHandoff(
	call *ast.CallExpr,
	callee *functionDecl,
	object string,
	statement string,
	evidenceNodes ...ast.Node,
) {
	evidenceRefs := make([]EvidenceRef, 0, len(evidenceNodes))
	for index, node := range evidenceNodes {
		if node == nil || !analyzer.withinWindow(node.Pos()) {
			return
		}
		location := analyzer.location(node)
		evidenceRefs = append(evidenceRefs, EvidenceRef{
			ID: stableID(
				"gm-ev",
				analyzer.function.ID,
				fmt.Sprintf("%d", location.Line),
				fmt.Sprintf("%d", location.Column),
				fmt.Sprintf("local_error_handoff_%d", index),
			),
			Location: location,
		})
	}
	analyzer.add(
		semanticdiscovery.CapabilityErrorPath,
		"local_error_handoff",
		statement,
		analyzer.decl.symbol,
		object,
		BasisErrorHandoff,
		call,
		callee.id,
		nil,
		evidenceRefs...,
	)
}

func matchingErrorResults(caller, callee *functionDecl) bool {
	if len(caller.returns) != len(callee.returns) || len(caller.errorResult) == 0 ||
		len(caller.errorResult) != len(callee.errorResult) {
		return false
	}
	for index := range caller.errorResult {
		if caller.errorResult[index] != callee.errorResult[index] {
			return false
		}
	}
	return true
}

func isNonNilGuard(expression ast.Expr, binding string) bool {
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return false
	}
	left, leftOK := binary.X.(*ast.Ident)
	right, rightOK := binary.Y.(*ast.Ident)
	return leftOK && rightOK &&
		((left.Name == binding && right.Name == "nil") ||
			(left.Name == "nil" && right.Name == binding))
}

func returnsErrorBinding(decl *functionDecl, statement *ast.ReturnStmt, binding string) bool {
	if len(decl.errorResult) != 1 || len(statement.Results) != len(decl.returns) {
		return false
	}
	identifier, ok := statement.Results[decl.errorResult[0]].(*ast.Ident)
	return ok && identifier.Name == binding
}

func httpHandlerSignature(decl *functionDecl) bool {
	hasResponseWriter := false
	hasRequest := false
	for _, info := range decl.params {
		hasResponseWriter = hasResponseWriter || info.responseWriter
		hasRequest = hasRequest || info.base == "Request"
	}
	return hasResponseWriter && hasRequest && len(decl.errorResult) > 0
}

func (analyzer *functionAnalyzer) observeAssignment(assignment *ast.AssignStmt) {
	for _, lhs := range assignment.Lhs {
		switch lhs.(type) {
		case *ast.SelectorExpr, *ast.IndexExpr:
			object := analyzer.snippet(lhs)
			analyzer.add(
				semanticdiscovery.CapabilityDataWrite,
				"assignment",
				fmt.Sprintf("%s contains a syntactic assignment to %s.", analyzer.decl.symbol, object),
				analyzer.decl.symbol,
				object,
				BasisAssignment,
				lhs,
				"",
				nil,
			)
		}
	}
	for index, rhs := range assignment.Rhs {
		if slice, ok := rhs.(*ast.SliceExpr); ok {
			object := analyzer.snippet(slice)
			analyzer.add(
				semanticdiscovery.CapabilityDataTransformation,
				"slice",
				fmt.Sprintf("%s contains a slice transformation %s.", analyzer.decl.symbol, object),
				analyzer.decl.symbol,
				object,
				BasisTransform,
				slice,
				"",
				nil,
			)
		}
		call, ok := rhs.(*ast.CallExpr)
		if !ok || calledName(call.Fun) != "append" {
			continue
		}
		object := ""
		if index < len(assignment.Lhs) {
			object = analyzer.snippet(assignment.Lhs[index])
		} else if len(assignment.Lhs) == 1 {
			object = analyzer.snippet(assignment.Lhs[0])
		}
		analyzer.add(
			semanticdiscovery.CapabilityDataWrite,
			"append_assignment",
			fmt.Sprintf("%s assigns an append result to %s.", analyzer.decl.symbol, object),
			analyzer.decl.symbol,
			object,
			BasisAssignment,
			assignment,
			"",
			nil,
		)
	}
}

func (analyzer *functionAnalyzer) observeCall(call *ast.CallExpr) {
	if callee := analyzer.resolveCall(call); callee != nil {
		observation := analyzer.add(
			semanticdiscovery.CapabilityDirectCall,
			"direct_local_call",
			fmt.Sprintf("%s contains a type-resolved direct local call to %s; this is syntax evidence, not proof of runtime execution.", analyzer.decl.symbol, callee.symbol),
			analyzer.decl.symbol,
			callee.symbol,
			BasisDirectCall,
			call,
			callee.id,
			nil,
		)
		if observation.ID != "" {
			analyzer.callees[callee.id] = resolvedCallee{decl: callee, observationID: observation.ID}
		}
	}

	name := calledName(call.Fun)
	switch {
	case isURLQueryGet(call):
		key := stringLiteral(call.Args[0])
		analyzer.add(
			semanticdiscovery.CapabilityDataRead,
			"url_query_get",
			fmt.Sprintf("%s reads URL query parameter %q through Query().Get syntax.", analyzer.decl.symbol, key),
			analyzer.decl.symbol,
			key,
			BasisRead,
			call,
			"",
			nil,
		)
	case name == "ReadDir":
		analyzer.add(
			semanticdiscovery.CapabilityDataRead,
			"read_directory",
			fmt.Sprintf("%s contains a ReadDir call that reads directory entries.", analyzer.decl.symbol),
			analyzer.decl.symbol,
			analyzer.snippet(call),
			BasisRead,
			call,
			"",
			nil,
		)
	case name == "append":
		analyzer.add(
			semanticdiscovery.CapabilityDataTransformation,
			"append",
			fmt.Sprintf("%s contains an append transformation.", analyzer.decl.symbol),
			analyzer.decl.symbol,
			analyzer.snippet(call),
			BasisTransform,
			call,
			"",
			nil,
		)
	case isSortCall(call):
		analyzer.add(
			semanticdiscovery.CapabilityDataTransformation,
			"sort",
			fmt.Sprintf("%s contains a sort transformation.", analyzer.decl.symbol),
			analyzer.decl.symbol,
			analyzer.snippet(call),
			BasisTransform,
			call,
			"",
			nil,
		)
	case isParseCall(call):
		analyzer.add(
			semanticdiscovery.CapabilityDataTransformation,
			"parse_parameter",
			fmt.Sprintf("%s contains a parameter parsing transformation.", analyzer.decl.symbol),
			analyzer.decl.symbol,
			analyzer.snippet(call),
			BasisTransform,
			call,
			"",
			nil,
		)
	case name == "Encode" && analyzer.isJSONEncoder(call):
		analyzer.addOutput(call, "json_encode", "encodes JSON output")
	case name == "Execute" && analyzer.isTemplateExecute(call):
		analyzer.addOutput(call, "template_execute", "executes a template into an output buffer or writer")
	case isPlainFormatCall(call):
		analyzer.addOutput(call, "plain_format", "formats plain-text output into a writer")
	case name == "WriteHeader" && analyzer.callReceiverIsResponseWriter(call):
		analyzer.addOutput(call, "write_header", "writes an HTTP response status")
	case name == "Write" && analyzer.callReceiverIsResponseWriter(call):
		analyzer.addOutput(call, "write_response", "writes bytes to an HTTP response writer")
	case name == "WriteTo" && len(call.Args) > 0 && analyzer.isResponseWriter(call.Args[0]):
		analyzer.addOutput(call, "write_to_response", "writes buffered output to an HTTP response writer")
	case (name == "Set" || name == "Add") && analyzer.isResponseHeaderMutation(call):
		analyzer.addOutput(call, "response_header", "mutates an HTTP response header")
	}
}

func (analyzer *functionAnalyzer) addOutput(call *ast.CallExpr, operation, action string) {
	analyzer.add(
		semanticdiscovery.CapabilityOutputEffect,
		operation,
		fmt.Sprintf("%s %s.", analyzer.decl.symbol, action),
		analyzer.decl.symbol,
		analyzer.snippet(call),
		BasisOutput,
		call,
		"",
		nil,
	)
}

func (analyzer *functionAnalyzer) observeIf(statement *ast.IfStmt) {
	analyzer.addBranch(statement.Cond)
}

func (analyzer *functionAnalyzer) observeSwitch(statement *ast.SwitchStmt) {
	node := ast.Node(statement)
	if statement.Tag != nil {
		node = statement.Tag
	}
	analyzer.addBranch(node)
}

func (analyzer *functionAnalyzer) observeTypeSwitch(statement *ast.TypeSwitchStmt) {
	node := ast.Node(statement)
	if statement.Assign != nil {
		node = statement.Assign
	}
	analyzer.addBranch(node)
}

func (analyzer *functionAnalyzer) addBranch(node ast.Node) {
	predicate := analyzer.snippet(node)
	analyzer.add(
		semanticdiscovery.CapabilityBranch,
		"branch_predicate",
		fmt.Sprintf("%s contains the branch predicate %s; the probe does not claim which branch runs.", analyzer.decl.symbol, predicate),
		analyzer.decl.symbol,
		predicate,
		BasisBranch,
		node,
		"",
		nil,
	)
}

func (analyzer *functionAnalyzer) observeReturn(statement *ast.ReturnStmt) {
	if len(statement.Results) == 0 || len(analyzer.decl.errorResult) == 0 {
		return
	}
	for _, errorIndex := range analyzer.decl.errorResult {
		if errorIndex >= len(statement.Results) || isNilExpression(statement.Results[errorIndex]) {
			continue
		}
		expression := statement.Results[errorIndex]
		analyzer.add(
			semanticdiscovery.CapabilityErrorPath,
			"error_return",
			fmt.Sprintf("%s contains a non-nil expression in an error result position: %s.", analyzer.decl.symbol, analyzer.snippet(expression)),
			analyzer.decl.symbol,
			analyzer.snippet(expression),
			BasisReturn,
			expression,
			"",
			nil,
		)
	}
}

func (analyzer *functionAnalyzer) updateAssignmentTypes(assignment *ast.AssignStmt) {
	if len(assignment.Rhs) == 1 {
		types := analyzer.inferExpressionTypes(assignment.Rhs[0])
		if len(types) == len(assignment.Lhs) {
			for index, lhs := range assignment.Lhs {
				if identifier, ok := lhs.(*ast.Ident); ok && identifier.Name != "_" && types[index].base != "" {
					analyzer.env[identifier.Name] = types[index]
				}
			}
			return
		}
	}
	for index, rhs := range assignment.Rhs {
		if index >= len(assignment.Lhs) {
			break
		}
		identifier, ok := assignment.Lhs[index].(*ast.Ident)
		if !ok || identifier.Name == "_" {
			continue
		}
		types := analyzer.inferExpressionTypes(rhs)
		if len(types) == 1 && types[0].base != "" {
			analyzer.env[identifier.Name] = types[0]
		}
	}
}

func (analyzer *functionAnalyzer) updateDeclarationTypes(statement *ast.DeclStmt) {
	declaration, ok := statement.Decl.(*ast.GenDecl)
	if !ok || declaration.Tok != token.VAR {
		return
	}
	for _, spec := range declaration.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if value.Type != nil {
			info := typeFromExpression(value.Type)
			for _, name := range value.Names {
				if name.Name != "_" && info.base != "" {
					analyzer.env[name.Name] = info
				}
			}
			continue
		}
		for index, expression := range value.Values {
			if index >= len(value.Names) {
				break
			}
			types := analyzer.inferExpressionTypes(expression)
			if len(types) == 1 && types[0].base != "" {
				analyzer.env[value.Names[index].Name] = types[0]
			}
		}
	}
}

func (analyzer *functionAnalyzer) inferExpressionTypes(expression ast.Expr) []typeInfo {
	switch value := expression.(type) {
	case *ast.CompositeLit:
		return []typeInfo{typeFromExpression(value.Type)}
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return analyzer.inferExpressionTypes(value.X)
		}
	case *ast.ParenExpr:
		return analyzer.inferExpressionTypes(value.X)
	case *ast.Ident:
		if info, exists := analyzer.env[value.Name]; exists {
			return []typeInfo{info}
		}
	case *ast.CallExpr:
		if callee := analyzer.resolveCall(value); callee != nil {
			return callee.returns
		}
		if identifier, ok := value.Fun.(*ast.Ident); ok && token.IsIdentifier(identifier.Name) {
			return []typeInfo{{base: identifier.Name, text: identifier.Name}}
		}
	}
	return nil
}

func (analyzer *functionAnalyzer) resolveCall(call *ast.CallExpr) *functionDecl {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return analyzer.index[analyzer.decl.packageName+"\x00"+function.Name]
	case *ast.SelectorExpr:
		types := analyzer.inferExpressionTypes(function.X)
		if len(types) != 1 || types[0].base == "" {
			return nil
		}
		return analyzer.index[analyzer.decl.packageName+"\x00"+types[0].base+"."+function.Sel.Name]
	default:
		return nil
	}
}

func (analyzer *functionAnalyzer) addLexicalSequences() {
	events := make([]positionedObservation, 0, len(analyzer.positioned))
	for _, positioned := range analyzer.positioned {
		observation := positioned.observation
		include := false
		switch observation.Capability {
		case semanticdiscovery.CapabilityDataRead,
			semanticdiscovery.CapabilityDataTransformation,
			semanticdiscovery.CapabilityOutputEffect:
			include = true
		case semanticdiscovery.CapabilityDirectCall:
			if len(analyzer.allowed) == 0 {
				include = true
			} else {
				_, include = analyzer.allowed[observation.TargetSymbol]
			}
		}
		if include {
			events = append(events, positioned)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].position < events[j].position })
	const maxSequences = 16
	for index := 1; index < len(events) && index <= maxSequences; index++ {
		before, after := events[index-1], events[index]
		if before.position == after.position || before.observation.ID == after.observation.ID {
			continue
		}
		statement := fmt.Sprintf(
			"Within %s's lexical source, %s appears before %s; this does not prove runtime path execution.",
			analyzer.decl.symbol,
			before.observation.Operation,
			after.observation.Operation,
		)
		evidenceRefs := append([]EvidenceRef{}, before.observation.Evidence...)
		evidenceRefs = append(evidenceRefs, after.observation.Evidence...)
		analyzer.add(
			semanticdiscovery.CapabilitySequence,
			"lexical_order",
			statement,
			before.observation.Operation,
			after.observation.Operation,
			BasisLexicalOrder,
			astNodeAt(before.position, after.position),
			"",
			[]string{before.observation.ID, after.observation.ID},
			evidenceRefs...,
		)
	}
}

// syntheticNode carries only a source range for a locally constructed lexical
// order observation.
type syntheticNode struct{ start, end token.Pos }

func (node syntheticNode) Pos() token.Pos { return node.start }
func (node syntheticNode) End() token.Pos { return node.end }

func astNodeAt(start, end token.Pos) ast.Node {
	return syntheticNode{start: start, end: end}
}

func (analyzer *functionAnalyzer) add(
	capability semanticdiscovery.Capability,
	operation string,
	statement string,
	subject string,
	object string,
	basis SyntaxBasis,
	node ast.Node,
	targetFunctionID string,
	related []string,
	providedEvidence ...EvidenceRef,
) Observation {
	if node == nil || !analyzer.withinWindow(node.Pos()) {
		return Observation{}
	}
	location := analyzer.location(node)
	key := strings.Join([]string{
		string(capability), operation, fmt.Sprintf("%d:%d", location.Line, location.Column),
		object, targetFunctionID, strings.Join(related, ","),
	}, "\x00")
	if _, exists := analyzer.seen[key]; exists {
		return Observation{}
	}
	analyzer.seen[key] = struct{}{}
	evidenceRefs := providedEvidence
	if len(evidenceRefs) == 0 {
		evidenceRefs = []EvidenceRef{{
			ID:       stableID("gm-ev", analyzer.function.ID, fmt.Sprintf("%d", location.Line), fmt.Sprintf("%d", location.Column), operation),
			Location: location,
		}}
	}
	targetSymbol := ""
	if targetFunctionID != "" {
		for _, callee := range analyzer.index {
			if callee.id == targetFunctionID {
				targetSymbol = callee.symbol
				break
			}
		}
	}
	idParts := []string{analyzer.function.ID, string(capability), operation, fmt.Sprintf("%d", location.Line), fmt.Sprintf("%d", location.Column), object, targetFunctionID}
	idParts = append(idParts, related...)
	observation := Observation{
		ID: stableID("gm-obs", idParts...), Capability: capability, FunctionID: analyzer.function.ID,
		Operation: operation, Statement: statement, Subject: subject, Object: object,
		TargetFunctionID: targetFunctionID, TargetSymbol: targetSymbol,
		RelatedObservationIDs: append([]string(nil), related...), Basis: basis, Evidence: evidenceRefs,
	}
	analyzer.positioned = append(analyzer.positioned, positionedObservation{observation: observation, position: node.Pos()})
	return observation
}

func (analyzer *functionAnalyzer) withinWindow(position token.Pos) bool {
	if !position.IsValid() {
		return false
	}
	resolved := analyzer.fset.Position(position)
	if resolved.Filename != analyzer.decl.file.path || resolved.Line > analyzer.window.lastLine {
		return false
	}
	return resolved.Line < analyzer.window.lastLine || resolved.Column <= analyzer.window.lastColumn
}

func (analyzer *functionAnalyzer) location(node ast.Node) evidence.Location {
	start := analyzer.fset.Position(node.Pos())
	end := analyzer.fset.Position(node.End())
	if end.Filename != analyzer.decl.file.path || end.Line > analyzer.window.lastLine ||
		(end.Line == analyzer.window.lastLine && end.Column > analyzer.window.lastColumn) {
		end.Line = analyzer.window.lastLine
		end.Column = analyzer.window.lastColumn
	}
	return evidence.Location{
		Path: analyzer.decl.file.path, Line: start.Line, Column: start.Column,
		EndLine: end.Line, EndColumn: end.Column,
	}
}

func (analyzer *functionAnalyzer) snippet(node ast.Node) string {
	if node == nil {
		return ""
	}
	start := analyzer.fset.Position(node.Pos())
	end := analyzer.fset.Position(node.End())
	if start.Offset < 0 || end.Offset < start.Offset || start.Offset >= len(analyzer.decl.file.data) {
		return ""
	}
	if end.Offset > len(analyzer.decl.file.data) {
		end.Offset = len(analyzer.decl.file.data)
	}
	value := strings.Join(strings.Fields(string(analyzer.decl.file.data[start.Offset:end.Offset])), " ")
	return truncateText(value, 240)
}

func (analyzer *functionAnalyzer) isJSONEncoder(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if strings.HasPrefix(analyzer.snippet(selector.X), "json.NewEncoder(") {
		return true
	}
	types := analyzer.inferExpressionTypes(selector.X)
	return len(types) == 1 && types[0].base == "Encoder"
}

func (analyzer *functionAnalyzer) isTemplateExecute(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	types := analyzer.inferExpressionTypes(selector.X)
	return len(types) == 1 && types[0].base == "Template"
}

func (analyzer *functionAnalyzer) callReceiverIsResponseWriter(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && analyzer.isResponseWriter(selector.X)
}

func (analyzer *functionAnalyzer) isResponseWriter(expression ast.Expr) bool {
	types := analyzer.inferExpressionTypes(expression)
	return len(types) == 1 && types[0].responseWriter
}

func (analyzer *functionAnalyzer) isResponseHeaderMutation(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	headerCall, ok := selector.X.(*ast.CallExpr)
	if !ok || calledName(headerCall.Fun) != "Header" {
		return false
	}
	headerSelector, ok := headerCall.Fun.(*ast.SelectorExpr)
	return ok && analyzer.isResponseWriter(headerSelector.X)
}

func isURLQueryGet(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Get" || len(call.Args) != 1 || stringLiteral(call.Args[0]) == "" {
		return false
	}
	queryCall, ok := selector.X.(*ast.CallExpr)
	if !ok || calledName(queryCall.Fun) != "Query" {
		return false
	}
	querySelector, ok := queryCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	urlSelector, ok := querySelector.X.(*ast.SelectorExpr)
	return ok && urlSelector.Sel.Name == "URL"
}

func stringLiteral(expression ast.Expr) string {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return ""
	}
	return value
}

func calledName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func isSortCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok || (packageName.Name != "sort" && packageName.Name != "slices") {
		return false
	}
	switch selector.Sel.Name {
	case "Sort", "Stable", "Slice", "SliceStable", "SortFunc", "SortStableFunc",
		"Strings", "Ints", "Float64s":
		return true
	default:
		return false
	}
}

func isParseCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok || packageName.Name != "strconv" {
		return false
	}
	switch selector.Sel.Name {
	case "Atoi", "ParseInt", "ParseUint":
		return true
	default:
		return false
	}
}

func isPlainFormatCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok || packageName.Name != "fmt" {
		return false
	}
	switch selector.Sel.Name {
	case "Fprint", "Fprintf", "Fprintln":
		return true
	default:
		return false
	}
}

func isNilExpression(expression ast.Expr) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == "nil"
}

func receiverIdentity(function *ast.FuncDecl) (string, string) {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return "", ""
	}
	receiver := typeFromExpression(function.Recv.List[0].Type).base
	receiverVar := ""
	if len(function.Recv.List[0].Names) == 1 {
		receiverVar = function.Recv.List[0].Names[0].Name
	}
	return receiver, receiverVar
}

func parameterTypes(fields *ast.FieldList) map[string]typeInfo {
	result := make(map[string]typeInfo)
	if fields == nil {
		return result
	}
	for _, field := range fields.List {
		info := typeFromExpression(field.Type)
		for _, name := range field.Names {
			result[name.Name] = info
		}
	}
	return result
}

func resultTypes(fields *ast.FieldList) []typeInfo {
	if fields == nil {
		return nil
	}
	var result []typeInfo
	for _, field := range fields.List {
		count := max(1, len(field.Names))
		info := typeFromExpression(field.Type)
		for range count {
			result = append(result, info)
		}
	}
	return result
}

func errorResultIndexes(fields *ast.FieldList) []int {
	if fields == nil {
		return nil
	}
	var result []int
	index := 0
	for _, field := range fields.List {
		count := max(1, len(field.Names))
		info := typeFromExpression(field.Type)
		if info.base == "error" {
			for offset := range count {
				result = append(result, index+offset)
			}
		}
		index += count
	}
	return result
}

func typeFromExpression(expression ast.Expr) typeInfo {
	switch value := expression.(type) {
	case *ast.Ident:
		return typeInfo{base: value.Name, text: value.Name, responseWriter: value.Name == "ResponseWriter"}
	case *ast.SelectorExpr:
		prefix := typeFromExpression(value.X)
		text := value.Sel.Name
		if prefix.text != "" {
			text = prefix.text + "." + text
		}
		return typeInfo{base: value.Sel.Name, text: text, responseWriter: value.Sel.Name == "ResponseWriter"}
	case *ast.StarExpr:
		info := typeFromExpression(value.X)
		info.text = "*" + info.text
		return info
	case *ast.IndexExpr:
		return typeFromExpression(value.X)
	case *ast.IndexListExpr:
		return typeFromExpression(value.X)
	case *ast.ParenExpr:
		return typeFromExpression(value.X)
	case *ast.Ellipsis:
		return typeFromExpression(value.Elt)
	default:
		return typeInfo{}
	}
}

func canonicalSymbol(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), " ", "")
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "(*") {
		closing := strings.Index(value, ").")
		if closing <= 2 {
			return ""
		}
		receiver := value[2:closing]
		method := value[closing+2:]
		if token.IsIdentifier(receiver) && token.IsIdentifier(method) {
			return receiver + "." + method
		}
		return ""
	}
	parts := strings.Split(value, ".")
	if len(parts) == 1 {
		if token.IsIdentifier(parts[0]) {
			return parts[0]
		}
		return ""
	}
	if len(parts) == 2 && token.IsIdentifier(parts[0]) && token.IsIdentifier(parts[1]) {
		return parts[0] + "." + parts[1]
	}
	return ""
}

func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + "…"
}
