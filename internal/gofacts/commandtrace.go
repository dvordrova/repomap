package gofacts

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	CommandTraceVersion       = 2
	maxCommandTraces          = 40
	maxHandlerCallsPerCommand = 16
	maxConditionBytes         = 160
)

// CommandTrace is bounded syntax evidence for one registered CLI command.
// Steps are exact package-local declarations. HandlerCalls are exact call
// sites, but unresolved selector calls remain syntax evidence rather than a
// claim about the concrete runtime target.
type CommandTrace struct {
	Version           int                `json:"version"`
	Framework         string             `json:"framework"`
	EntrypointPackage string             `json:"entrypoint_package"`
	Command           string             `json:"command"`
	Steps             []CommandTraceStep `json:"steps"`
	HandlerCalls      []CommandTraceCall `json:"handler_calls,omitempty"`
	Concurrency       ConcurrencyScope   `json:"concurrency"`
	Complete          bool               `json:"complete"`
	Missing           []string           `json:"missing,omitempty"`
}

// ConcurrencyScope is a bounded statement about the selected handler body,
// not the transitive behavior of every callee.
type ConcurrencyScope string

const (
	ConcurrencyUnknown                ConcurrencyScope = "unknown"
	ConcurrencyPresentInHandler       ConcurrencyScope = "present_in_handler"
	ConcurrencyAbsentFromHandlerScope ConcurrencyScope = "absent_from_handler_scope"
)

type CommandTraceStep struct {
	Symbol           string             `json:"symbol"`
	Relation         string             `json:"relation"`
	CallsiteLocation *evidence.Location `json:"callsite_location,omitempty"`
	TargetLocation   evidence.Location  `json:"target_location"`
}

type CommandTraceCall struct {
	Symbol     string              `json:"symbol"`
	Path       string              `json:"path"`
	Line       int                 `json:"line"`
	Relation   string              `json:"relation"`
	Condition  *evidence.Condition `json:"condition,omitempty"`
	Resolved   bool                `json:"resolved"`
	TargetPath string              `json:"target_path,omitempty"`
	TargetLine int                 `json:"target_line,omitempty"`
}

type commandSyntaxFunction struct {
	name string
	path string
	line int
	decl *ast.FuncDecl
	fset *token.FileSet
}

type commandSyntaxCall struct {
	name     string
	location evidence.Location
}

type rankedCommandCall struct {
	call  CommandTraceCall
	score int
}

// commandFrameworkReader is deliberately package-local. It keeps framework
// recognition replaceable without committing repomap to a public plugin API
// before a second real command framework needs the seam.
type commandFrameworkReader interface {
	Read(Entrypoint, map[string]commandSyntaxFunction) []CommandTrace
}

var commandFrameworkReaders = []commandFrameworkReader{
	cobraCommandReader{},
}

type cobraCommandReader struct{}

func buildCommandTraces(reader *reporead.Reader, entrypoints []Entrypoint) ([]CommandTrace, []string) {
	var traces []CommandTrace
	var warnings []string
	for _, entrypoint := range entrypoints {
		packageTraces, packageWarnings := buildEntrypointCommandTraces(reader, entrypoint)
		warnings = append(warnings, packageWarnings...)
		traces = append(traces, packageTraces...)
		if len(traces) >= maxCommandTraces {
			traces = traces[:maxCommandTraces]
			warnings = append(warnings, fmt.Sprintf("command traces reached %d-trace limit", maxCommandTraces))
			break
		}
	}
	return traces, warnings
}

func buildEntrypointCommandTraces(reader *reporead.Reader, entrypoint Entrypoint) ([]CommandTrace, []string) {
	functions, warnings := parseEntrypointFunctions(reader, entrypoint)
	var traces []CommandTrace
	for _, frameworkReader := range commandFrameworkReaders {
		traces = append(traces, frameworkReader.Read(entrypoint, functions)...)
		if len(traces) >= maxCommandTraces {
			traces = traces[:maxCommandTraces]
			break
		}
	}
	return traces, warnings
}

func (cobraCommandReader) Read(entrypoint Entrypoint, functions map[string]commandSyntaxFunction) []CommandTrace {
	mainFunction, ok := functions["main"]
	if !ok {
		return nil
	}

	rootCall := executedCommandRoot(mainFunction, functions)
	rootFunction, ok := functions[rootCall.name]
	if !ok {
		return nil
	}

	constructors := registeredCommandConstructors(rootFunction, functions)
	traces := make([]CommandTrace, 0, len(constructors))
	for _, constructorCall := range constructors {
		constructor := functions[constructorCall.name]
		command, handlerCall := commandHandler(constructor, functions)
		trace := CommandTrace{
			Version:           CommandTraceVersion,
			Framework:         "cobra",
			EntrypointPackage: entrypoint.ImportPath,
			Command:           command,
			Steps: []CommandTraceStep{
				commandTraceStep(mainFunction, "entrypoint"),
				commandTraceCallStep(rootFunction, rootCall.location, "calls"),
				commandTraceCallStep(constructor, constructorCall.location, "registers_command"),
			},
		}
		if command == "" {
			trace.Command = constructorCall.name
			trace.Missing = append(trace.Missing, "command name")
		}
		if handler, found := functions[handlerCall.name]; found {
			trace.Steps = append(trace.Steps, commandTraceCallStep(handler, handlerCall.location, "callback"))
			trace.HandlerCalls = meaningfulHandlerCalls(handler, functions)
			trace.Concurrency = handlerConcurrencyScope(handler)
		} else {
			trace.Concurrency = ConcurrencyUnknown
			trace.Missing = append(trace.Missing, "Run/RunE handler")
		}
		trace.Complete = len(trace.Steps) == 4 && len(trace.Missing) == 0
		traces = append(traces, trace)
		if len(traces) == maxCommandTraces {
			break
		}
	}
	return traces
}

func handlerConcurrencyScope(handler commandSyntaxFunction) ConcurrencyScope {
	present := false
	ast.Inspect(handler.decl.Body, func(node ast.Node) bool {
		if present {
			return false
		}
		if _, nestedCallback := node.(*ast.FuncLit); nestedCallback {
			return false
		}
		switch node := node.(type) {
		case *ast.GoStmt:
			present = true
			return false
		case *ast.CallExpr:
			if selectorName(node.Fun) != "Go" {
				return true
			}
			for _, argument := range node.Args {
				if _, callback := argument.(*ast.FuncLit); callback {
					present = true
					return false
				}
			}
		}
		return true
	})
	if present {
		return ConcurrencyPresentInHandler
	}
	return ConcurrencyAbsentFromHandlerScope
}

func parseEntrypointFunctions(reader *reporead.Reader, entrypoint Entrypoint) (map[string]commandSyntaxFunction, []string) {
	functions := make(map[string]commandSyntaxFunction)
	var warnings []string
	for _, goFile := range entrypoint.GoFiles {
		repoPath, err := entrypointSourcePath(entrypoint.PackageDir, goFile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("command trace %s: skip %q: %v", entrypoint.ImportPath, goFile, err))
			continue
		}
		content, err := reader.ReadFile(repoPath, maxEntrypointFileBytes)
		if err != nil || content.Truncated {
			warnings = append(warnings, fmt.Sprintf("command trace %s: cannot read bounded source %s", entrypoint.ImportPath, repoPath))
			continue
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, repoPath, content.Bytes, parser.SkipObjectResolution)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("command trace %s: cannot parse %s: %v", entrypoint.ImportPath, repoPath, err))
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Name == nil || function.Body == nil {
				continue
			}
			position := fileSet.PositionFor(function.Name.Pos(), false)
			functions[function.Name.Name] = commandSyntaxFunction{
				name: function.Name.Name,
				path: repoPath,
				line: position.Line,
				decl: function,
				fset: fileSet,
			}
		}
	}
	return functions, warnings
}

func executedCommandRoot(function commandSyntaxFunction, functions map[string]commandSyntaxFunction) commandSyntaxCall {
	var root commandSyntaxCall
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		if root.name != "" {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil || (selector.Sel.Name != "Execute" && selector.Sel.Name != "ExecuteContext") {
			return true
		}
		constructor, ok := selector.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := simpleCallName(constructor)
		if _, exists := functions[name]; exists {
			position := function.fset.PositionFor(constructor.Lparen, false)
			root = commandSyntaxCall{
				name:     name,
				location: evidence.Location{Path: function.path, Line: position.Line, Column: position.Column},
			}
			return false
		}
		return true
	})
	return root
}

func registeredCommandConstructors(function commandSyntaxFunction, functions map[string]commandSyntaxFunction) []commandSyntaxCall {
	seen := make(map[string]struct{})
	var constructors []commandSyntaxCall
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || selectorName(call.Fun) != "AddCommand" {
			return true
		}
		for _, argument := range call.Args {
			constructor, ok := argument.(*ast.CallExpr)
			if !ok {
				continue
			}
			name := simpleCallName(constructor)
			if _, exists := functions[name]; !exists {
				continue
			}
			if _, duplicate := seen[name]; duplicate {
				continue
			}
			seen[name] = struct{}{}
			position := function.fset.PositionFor(constructor.Lparen, false)
			constructors = append(constructors, commandSyntaxCall{
				name:     name,
				location: evidence.Location{Path: function.path, Line: position.Line, Column: position.Column},
			})
		}
		return true
	})
	return constructors
}

func commandHandler(function commandSyntaxFunction, functions map[string]commandSyntaxFunction) (string, commandSyntaxCall) {
	var command string
	var handler commandSyntaxCall
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		pair, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok {
			return true
		}
		switch key.Name {
		case "Use":
			value := leadingStaticString(pair.Value)
			if fields := strings.Fields(value); len(fields) > 0 {
				command = fields[0]
			}
		case "Run", "RunE":
			callback, ok := pair.Value.(*ast.FuncLit)
			if !ok {
				return true
			}
			handler = callbackHandlerCall(callback.Body, functions, function.path, function.fset)
		}
		return true
	})
	return command, handler
}

func leadingStaticString(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		if expression.Kind != token.STRING {
			return ""
		}
		value, err := strconv.Unquote(expression.Value)
		if err != nil {
			return ""
		}
		return value
	case *ast.BinaryExpr:
		if expression.Op != token.ADD {
			return ""
		}
		return leadingStaticString(expression.X)
	case *ast.ParenExpr:
		return leadingStaticString(expression.X)
	default:
		return ""
	}
}

func callbackHandlerCall(
	body *ast.BlockStmt,
	functions map[string]commandSyntaxFunction,
	path string,
	fileSet *token.FileSet,
) commandSyntaxCall {
	returnedNames := make(map[string]struct{})
	var directReturn commandSyntaxCall
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range statement.Results {
			if identifier, ok := result.(*ast.Ident); ok {
				returnedNames[identifier.Name] = struct{}{}
			}
			call, ok := result.(*ast.CallExpr)
			if !ok || directReturn.name != "" {
				continue
			}
			directReturn = localSyntaxCall(call, functions, path, fileSet)
		}
		return false
	})
	if directReturn.name != "" {
		return directReturn
	}

	var returnedResultCall commandSyntaxCall
	ast.Inspect(body, func(node ast.Node) bool {
		if returnedResultCall.name != "" {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || !assignmentReturnsName(assignment.Lhs, returnedNames) {
			return true
		}
		for _, expression := range assignment.Rhs {
			call, ok := expression.(*ast.CallExpr)
			if !ok {
				continue
			}
			returnedResultCall = localSyntaxCall(call, functions, path, fileSet)
			if returnedResultCall.name != "" {
				return false
			}
		}
		return true
	})
	if returnedResultCall.name != "" {
		return returnedResultCall
	}

	var result commandSyntaxCall
	ast.Inspect(body, func(node ast.Node) bool {
		if result.name != "" {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		result = localSyntaxCall(call, functions, path, fileSet)
		return result.name == ""
	})
	return result
}

func assignmentReturnsName(expressions []ast.Expr, returnedNames map[string]struct{}) bool {
	for _, expression := range expressions {
		identifier, ok := expression.(*ast.Ident)
		if !ok {
			continue
		}
		if _, returned := returnedNames[identifier.Name]; returned {
			return true
		}
	}
	return false
}

func localSyntaxCall(
	call *ast.CallExpr,
	functions map[string]commandSyntaxFunction,
	path string,
	fileSet *token.FileSet,
) commandSyntaxCall {
	candidate := simpleCallName(call)
	if _, exists := functions[candidate]; !exists {
		return commandSyntaxCall{}
	}
	position := fileSet.PositionFor(call.Lparen, false)
	return commandSyntaxCall{
		name:     candidate,
		location: evidence.Location{Path: path, Line: position.Line, Column: position.Column},
	}
}

func meaningfulHandlerCalls(handler commandSyntaxFunction, functions map[string]commandSyntaxFunction) []CommandTraceCall {
	seen := make(map[string]struct{})
	goroutineCalls := make(map[token.Pos]struct{})
	ast.Inspect(handler.decl.Body, func(node ast.Node) bool {
		if _, nestedCallback := node.(*ast.FuncLit); nestedCallback {
			return false
		}
		statement, ok := node.(*ast.GoStmt)
		if ok && statement.Call != nil {
			goroutineCalls[statement.Call.Pos()] = struct{}{}
		}
		return true
	})
	var ranked []rankedCommandCall
	ast.Inspect(handler.decl.Body, func(node ast.Node) bool {
		if _, nestedCallback := node.(*ast.FuncLit); nestedCallback {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		symbol := callExpressionName(call.Fun)
		if symbol == "" {
			return true
		}
		position := handler.fset.PositionFor(call.Lparen, false)
		key := fmt.Sprintf("%s:%d", symbol, position.Line)
		if _, duplicate := seen[key]; duplicate {
			return true
		}
		seen[key] = struct{}{}

		identifier, directIdentifier := call.Fun.(*ast.Ident)
		resolved := false
		var target commandSyntaxFunction
		if directIdentifier {
			target, resolved = functions[identifier.Name]
		}
		score, relation := commandCallScore(symbol, resolved)
		if _, started := goroutineCalls[call.Pos()]; started {
			relation = "starts_goroutine"
			score += 50
		}
		if score == 0 {
			return true
		}
		ranked = append(ranked, rankedCommandCall{
			call: CommandTraceCall{
				Symbol:     symbol,
				Path:       handler.path,
				Line:       position.Line,
				Relation:   relation,
				Condition:  enclosingIfCondition(handler.fset, handler.decl.Body, call, handler.path),
				Resolved:   resolved,
				TargetPath: target.path,
				TargetLine: target.line,
			},
			score: score,
		})
		return true
	})

	sortRankedCalls(ranked)
	if len(ranked) > maxHandlerCallsPerCommand {
		ranked = ranked[:maxHandlerCallsPerCommand]
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].call.Line != ranked[j].call.Line {
			return ranked[i].call.Line < ranked[j].call.Line
		}
		return ranked[i].call.Symbol < ranked[j].call.Symbol
	})
	calls := make([]CommandTraceCall, len(ranked))
	for index := range ranked {
		calls[index] = ranked[index].call
	}
	return calls
}

func enclosingIfCondition(fileSet *token.FileSet, root ast.Node, target ast.Node, fallbackPath string) *evidence.Condition {
	var expressions []string
	var location evidence.Location
	expressionComplete := true
	ast.Inspect(root, func(node ast.Node) bool {
		statement, ok := node.(*ast.IfStmt)
		if !ok || statement.Cond == nil {
			return true
		}

		negated := false
		switch {
		case containsASTNode(statement.Body, target):
		case statement.Else != nil && containsASTNode(statement.Else, target):
			negated = true
		default:
			return true
		}

		expression, ok := boundedConditionExpression(statement.Cond)
		if !ok {
			expressionComplete = false
		} else {
			if negated {
				expression = "!(" + expression + ")"
			}
			expressions = append(expressions, expression)
		}
		if location.Path == "" {
			position := fileSet.PositionFor(statement.Cond.Pos(), false)
			location = evidence.Location{
				Path: fallbackPath, Line: position.Line, Column: position.Column,
			}
		}
		return true
	})
	if location.Path == "" {
		return nil
	}
	expression := strings.Join(expressions, " && ")
	expressionOmitted := !expressionComplete || len(expression) > maxConditionBytes
	if expressionOmitted {
		expression = ""
	}
	return &evidence.Condition{
		Expression:        expression,
		ExpressionOmitted: expressionOmitted,
		Location:          location,
	}
}

func boundedConditionExpression(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name, value.Name != ""
	case *ast.SelectorExpr:
		left, ok := boundedConditionExpression(value.X)
		if !ok || value.Sel == nil || value.Sel.Name == "" {
			return "", false
		}
		return left + "." + value.Sel.Name, true
	case *ast.UnaryExpr:
		if value.Op != token.NOT {
			return "", false
		}
		inner, ok := boundedConditionExpression(value.X)
		if !ok {
			return "", false
		}
		return "!" + inner, true
	case *ast.ParenExpr:
		inner, ok := boundedConditionExpression(value.X)
		if !ok {
			return "", false
		}
		return "(" + inner + ")", true
	case *ast.BinaryExpr:
		if value.Op != token.LAND && value.Op != token.LOR {
			return "", false
		}
		left, leftOK := boundedConditionExpression(value.X)
		right, rightOK := boundedConditionExpression(value.Y)
		if !leftOK || !rightOK {
			return "", false
		}
		return left + " " + value.Op.String() + " " + right, true
	default:
		return "", false
	}
}

func containsASTNode(container, target ast.Node) bool {
	return container != nil && target != nil &&
		container.Pos() <= target.Pos() && target.End() <= container.End()
}

func sortRankedCalls(calls []rankedCommandCall) {
	sort.SliceStable(calls, func(i, j int) bool {
		if calls[i].score != calls[j].score {
			return calls[i].score > calls[j].score
		}
		if calls[i].call.Line != calls[j].call.Line {
			return calls[i].call.Line < calls[j].call.Line
		}
		return calls[i].call.Symbol < calls[j].call.Symbol
	})
}

func commandCallScore(symbol string, resolved bool) (int, string) {
	lower := strings.ToLower(symbol)
	base := lower
	if dot := strings.LastIndex(base, "."); dot >= 0 {
		base = base[dot+1:]
	}
	score := 0
	if base == "go" {
		score = 45
	}
	for token, weight := range map[string]int{
		"snapshot": 50, "restore": 50, "archive": 45, "scanner": 40,
		"scan": 35, "load": 35, "open": 35, "save": 35, "create": 35,
		"register": 30, "execute": 30, "start": 25, "new": 20,
	} {
		if strings.Contains(base, token) {
			score += weight
		}
	}
	if resolved {
		score += 10
	}
	relation := "calls"
	switch {
	case strings.HasPrefix(base, "new"):
		relation = "constructs"
	case strings.Contains(base, "register"):
		relation = "registers"
	case base == "go":
		relation = "starts_goroutine"
	}
	return score, relation
}

func commandTraceStep(function commandSyntaxFunction, relation string) CommandTraceStep {
	return CommandTraceStep{
		Symbol:         function.name,
		Relation:       relation,
		TargetLocation: evidence.Location{Path: function.path, Line: function.line},
	}
}

func commandTraceCallStep(function commandSyntaxFunction, callsite evidence.Location, relation string) CommandTraceStep {
	step := commandTraceStep(function, relation)
	step.CallsiteLocation = &callsite
	return step
}

func simpleCallName(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}
	identifier, _ := call.Fun.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func selectorName(expression ast.Expr) string {
	selector, _ := expression.(*ast.SelectorExpr)
	if selector == nil || selector.Sel == nil {
		return ""
	}
	return selector.Sel.Name
}

func callExpressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		left := callExpressionName(value.X)
		if left == "" {
			return value.Sel.Name
		}
		return left + "." + value.Sel.Name
	case *ast.CallExpr:
		return callExpressionName(value.Fun)
	default:
		return ""
	}
}
