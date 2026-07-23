package sourcewindowfacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strings"
)

type ObservationKind string

const (
	ObservationDeclaration ObservationKind = "declaration"
	ObservationDirectCall  ObservationKind = "direct_call"
	ObservationBranch      ObservationKind = "branch"
	ObservationAssignment  ObservationKind = "assignment"
	ObservationRead        ObservationKind = "read"
	ObservationReturn      ObservationKind = "return"
)

// Observation is a bounded syntax observation. It describes only an AST node
// visible in Function.Lines and does not claim runtime execution.
type Observation struct {
	ID        string          `json:"id"`
	Kind      ObservationKind `json:"kind"`
	Subject   string          `json:"subject"`
	Object    string          `json:"object,omitempty"`
	Target    string          `json:"target,omitempty"`
	Value     string          `json:"value,omitempty"`
	Operator  string          `json:"operator,omitempty"`
	Line      int             `json:"line"`
	Column    int             `json:"column"`
	EndLine   int             `json:"end_line"`
	EndColumn int             `json:"end_column"`
}

// Function is one function declaration whose start and body opening are
// visible in a source Window. Partial is true only when unmatched trailing
// braces were closed synthetically for parsing; Lines and hashes never include
// those synthetic braces.
type Function struct {
	Symbol        string        `json:"symbol"`
	Path          string        `json:"path"`
	StartLine     int           `json:"start_line"`
	EndLine       int           `json:"end_line"`
	Lines         []string      `json:"lines"`
	ContentSHA256 string        `json:"content_sha256"`
	Partial       bool          `json:"partial"`
	Observations  []Observation `json:"observations"`
}

// Validate checks that every observation remains inside the visible function
// range and that the visible content hash is intact.
func (function Function) Validate() error {
	if canonicalSymbol(function.Symbol) != function.Symbol || function.Symbol == "" {
		return fmt.Errorf("source window function: invalid canonical symbol %q", function.Symbol)
	}
	if err := validateGoPath(function.Path); err != nil {
		return err
	}
	if function.StartLine <= 0 || function.EndLine < function.StartLine ||
		len(function.Lines) != function.EndLine-function.StartLine+1 || len(function.Lines) == 0 {
		return fmt.Errorf("source window function: line count does not match bounds")
	}
	if function.ContentSHA256 != linesSHA256(function.Lines) {
		return fmt.Errorf("source window function: content sha256 does not match lines")
	}
	seen := make(map[string]struct{}, len(function.Observations))
	for index, observation := range function.Observations {
		if err := validateObservation(function, observation); err != nil {
			return fmt.Errorf("source window function: observation[%d]: %w", index, err)
		}
		if _, duplicate := seen[observation.ID]; duplicate {
			return fmt.Errorf("source window function: duplicate observation id %q", observation.ID)
		}
		seen[observation.ID] = struct{}{}
	}
	return nil
}

// ExtractGoFunction extracts one named Go function from a bounded source
// window. A declaration that starts in the window but ends after its last line
// is parsed with synthetic closing braces and returned with Partial=true.
func ExtractGoFunction(window Window, canonicalSymbol string) (Function, error) {
	if err := window.Validate(); err != nil {
		return Function{}, err
	}
	wanted := canonicalizeRequestedSymbol(canonicalSymbol)
	if wanted == "" {
		return Function{}, fmt.Errorf("source window function: invalid symbol %q", canonicalSymbol)
	}
	source := strings.Join(window.Lines, "\n")
	candidates, err := scanFunctionCandidates(source)
	if err != nil {
		return Function{}, err
	}
	var matches []parsedFunction
	for _, candidate := range candidates {
		parsed, parseErr := parseFunctionCandidate(source, candidate)
		if parseErr != nil || parsed.symbol != wanted {
			continue
		}
		matches = append(matches, parsed)
	}
	if len(matches) == 0 {
		return Function{}, fmt.Errorf("source window function: symbol %q is not fully anchored in the window", wanted)
	}
	if len(matches) > 1 {
		return Function{}, fmt.Errorf("source window function: symbol %q is ambiguous in the window", wanted)
	}
	return buildFunction(window, matches[0])
}

// ExtractGoFunctions extracts every uniquely named Go function whose
// declaration starts in one verified source window. Malformed speculative
// candidates and function literals are ignored; lexical scan errors and
// invalid windows fail closed. Results are deduplicated by canonical symbol
// and returned in source order.
func ExtractGoFunctions(window Window) ([]Function, error) {
	if err := window.Validate(); err != nil {
		return nil, err
	}
	source := strings.Join(window.Lines, "\n")
	candidates, err := scanFunctionCandidates(source)
	if err != nil {
		return nil, err
	}
	parsedBySymbol := make(map[string]parsedFunction)
	for _, candidate := range candidates {
		parsed, parseErr := parseFunctionCandidate(source, candidate)
		if parseErr != nil {
			continue
		}
		if previous, duplicate := parsedBySymbol[parsed.symbol]; duplicate &&
			previous.startLine <= parsed.startLine {
			continue
		}
		parsedBySymbol[parsed.symbol] = parsed
	}
	parsedFunctions := make([]parsedFunction, 0, len(parsedBySymbol))
	for _, parsed := range parsedBySymbol {
		parsedFunctions = append(parsedFunctions, parsed)
	}
	sort.Slice(parsedFunctions, func(i, j int) bool {
		if parsedFunctions[i].startLine != parsedFunctions[j].startLine {
			return parsedFunctions[i].startLine < parsedFunctions[j].startLine
		}
		return parsedFunctions[i].symbol < parsedFunctions[j].symbol
	})
	functions := make([]Function, 0, len(parsedFunctions))
	for _, parsed := range parsedFunctions {
		function, buildErr := buildFunction(window, parsed)
		if buildErr != nil {
			return nil, buildErr
		}
		functions = append(functions, function)
	}
	return functions, nil
}

func buildFunction(window Window, parsed parsedFunction) (Function, error) {
	startLine := window.StartLine + parsed.startLine - 1
	endLine := window.StartLine + parsed.endLine - 1
	if parsed.partial {
		endLine = window.EndLine
	}
	lineStart := startLine - window.StartLine
	lineEnd := endLine - window.StartLine + 1
	visibleLines := append([]string(nil), window.Lines[lineStart:lineEnd]...)
	function := Function{
		Symbol:        parsed.symbol,
		Path:          window.Path,
		StartLine:     startLine,
		EndLine:       endLine,
		Lines:         visibleLines,
		ContentSHA256: linesSHA256(visibleLines),
		Partial:       parsed.partial,
	}
	function.Observations = buildObservations(function, parsed)
	if err := function.Validate(); err != nil {
		return Function{}, err
	}
	return function, nil
}

type scannedToken struct {
	token  token.Token
	offset int
	line   int
}

type functionCandidate struct {
	startOffset     int
	endOffset       int
	startLine       int
	endLine         int
	partial         bool
	syntheticBraces int
}

type parsedFunction struct {
	decl          *ast.FuncDecl
	fset          *token.FileSet
	symbol        string
	startLine     int
	endLine       int
	partial       bool
	visibleBytes  int
	packagePrefix int
}

func scanFunctionCandidates(source string) ([]functionCandidate, error) {
	fset := token.NewFileSet()
	file := fset.AddFile("window.go", -1, len(source))
	var scan scanner.Scanner
	var scanErrors []string
	scan.Init(file, []byte(source), func(position token.Position, message string) {
		scanErrors = append(scanErrors, fmt.Sprintf("%d:%d: %s", position.Line, position.Column, message))
	}, 0)
	var tokens []scannedToken
	for {
		position, tok, _ := scan.Scan()
		if tok == token.EOF {
			break
		}
		resolved := file.Position(position)
		tokens = append(tokens, scannedToken{token: tok, offset: resolved.Offset, line: resolved.Line})
	}
	if len(scanErrors) > 0 {
		return nil, fmt.Errorf("source window function: scan bounded Go source: %s", strings.Join(scanErrors, "; "))
	}

	seen := make(map[string]struct{})
	var candidates []functionCandidate
	for index, item := range tokens {
		if item.token != token.FUNC {
			continue
		}
		for braceIndex := index + 1; braceIndex < len(tokens); braceIndex++ {
			if tokens[braceIndex].token != token.LBRACE {
				continue
			}
			depth := 0
			closing := -1
			for cursor := braceIndex; cursor < len(tokens); cursor++ {
				switch tokens[cursor].token {
				case token.LBRACE:
					depth++
				case token.RBRACE:
					depth--
					if depth == 0 {
						closing = cursor
					}
				}
				if closing >= 0 {
					break
				}
			}
			candidate := functionCandidate{startOffset: item.offset, startLine: item.line}
			if closing >= 0 {
				candidate.endOffset = tokens[closing].offset + 1
				candidate.endLine = tokens[closing].line
			} else {
				candidate.endOffset = len(source)
				candidate.endLine = 1 + strings.Count(source, "\n")
				candidate.partial = true
				candidate.syntheticBraces = depth
			}
			key := fmt.Sprintf("%d:%d:%t:%d", candidate.startOffset, candidate.endOffset, candidate.partial, candidate.syntheticBraces)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func parseFunctionCandidate(source string, candidate functionCandidate) (parsedFunction, error) {
	visible := source[candidate.startOffset:candidate.endOffset]
	parseSource := visible
	if candidate.partial {
		if candidate.syntheticBraces <= 0 {
			return parsedFunction{}, fmt.Errorf("partial function has no unmatched body brace")
		}
		parseSource += "\n" + strings.Repeat("}", candidate.syntheticBraces)
	}
	const prefix = "package sourcewindow\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "window.go", prefix+parseSource, parser.SkipObjectResolution)
	if err != nil || len(file.Decls) != 1 {
		return parsedFunction{}, fmt.Errorf("candidate is not one Go function declaration")
	}
	decl, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || decl.Body == nil {
		return parsedFunction{}, fmt.Errorf("candidate is not a Go function declaration")
	}
	symbol := symbolForDecl(decl)
	if symbol == "" {
		return parsedFunction{}, fmt.Errorf("candidate has an unsupported function symbol")
	}
	return parsedFunction{
		decl:          decl,
		fset:          fset,
		symbol:        symbol,
		startLine:     candidate.startLine,
		endLine:       candidate.endLine,
		partial:       candidate.partial,
		visibleBytes:  len(prefix) + len(visible),
		packagePrefix: len(prefix),
	}, nil
}

func buildObservations(function Function, parsed parsedFunction) []Observation {
	builder := observationBuilder{function: function, parsed: parsed, seen: make(map[string]struct{})}
	builder.add(ObservationDeclaration, parsed.decl.Name, function.Symbol, "", "", "")
	ast.Inspect(parsed.decl.Body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if !builder.visible(node) {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			target := builder.text(value.Fun)
			if target != "" {
				builder.add(ObservationDirectCall, value, builder.text(value), target, "", "call")
			}
		case *ast.IfStmt:
			builder.addBranch("if", value.Cond)
			builder.addReads(value.Cond)
		case *ast.ForStmt:
			if value.Cond != nil {
				builder.addBranch("for", value.Cond)
				builder.addReads(value.Cond)
			}
		case *ast.RangeStmt:
			builder.addBranch("range", value.X)
			builder.addReads(value.X)
		case *ast.SwitchStmt:
			if value.Tag != nil {
				builder.addBranch("switch", value.Tag)
				builder.addReads(value.Tag)
			} else {
				builder.add(ObservationBranch, value, "", "", "", "switch")
			}
		case *ast.TypeSwitchStmt:
			builder.add(ObservationBranch, value, builder.text(value.Assign), "", "", "type_switch")
		case *ast.SelectStmt:
			builder.add(ObservationBranch, value, "", "", "", "select")
		case *ast.AssignStmt:
			builder.addAssignment(value)
			for _, expression := range value.Rhs {
				builder.addReads(expression)
			}
		case *ast.IncDecStmt:
			builder.add(ObservationAssignment, value, builder.text(value.X), "", "", value.Tok.String())
		case *ast.DeclStmt:
			builder.addValueAssignments(value)
		case *ast.ReturnStmt:
			values := builder.textList(value.Results)
			builder.add(ObservationReturn, value, "", "", values, "return")
			for _, expression := range value.Results {
				builder.addReads(expression)
			}
		}
		return true
	})
	sort.Slice(builder.observations, func(i, j int) bool {
		left, right := builder.observations[i], builder.observations[j]
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Target != right.Target {
			return left.Target < right.Target
		}
		return left.Object < right.Object
	})
	return builder.observations
}

type observationBuilder struct {
	function     Function
	parsed       parsedFunction
	seen         map[string]struct{}
	observations []Observation
}

func (builder *observationBuilder) add(
	kind ObservationKind,
	node ast.Node,
	object string,
	target string,
	value string,
	operator string,
) {
	if node == nil || !builder.visible(node) {
		return
	}
	start := builder.parsed.fset.Position(node.Pos())
	end := builder.parsed.fset.Position(node.End())
	line := builder.function.StartLine + start.Line - 2
	endLine := builder.function.StartLine + end.Line - 2
	endColumn := end.Column
	if endLine > builder.function.EndLine {
		endLine = builder.function.EndLine
		endColumn = len(builder.function.Lines[len(builder.function.Lines)-1]) + 1
	}
	key := strings.Join([]string{
		string(kind), fmt.Sprint(line), fmt.Sprint(start.Column), object, target, value, operator,
	}, "\x00")
	if _, duplicate := builder.seen[key]; duplicate {
		return
	}
	builder.seen[key] = struct{}{}
	observation := Observation{
		Kind: kind, Subject: builder.function.Symbol, Object: object, Target: target,
		Value: value, Operator: operator, Line: line, Column: start.Column,
		EndLine: endLine, EndColumn: endColumn,
	}
	observation.ID = observationID(builder.function, observation)
	builder.observations = append(builder.observations, observation)
}

func (builder *observationBuilder) addBranch(operator string, expression ast.Expr) {
	builder.add(ObservationBranch, expression, builder.text(expression), "", "", operator)
}

func (builder *observationBuilder) addAssignment(statement *ast.AssignStmt) {
	allValues := builder.textList(statement.Rhs)
	for index, lhs := range statement.Lhs {
		value := allValues
		if len(statement.Rhs) == len(statement.Lhs) {
			value = builder.text(statement.Rhs[index])
		} else if len(statement.Rhs) == 1 {
			value = builder.text(statement.Rhs[0])
		}
		builder.add(ObservationAssignment, lhs, builder.text(lhs), "", value, statement.Tok.String())
	}
}

func (builder *observationBuilder) addValueAssignments(statement *ast.DeclStmt) {
	declaration, ok := statement.Decl.(*ast.GenDecl)
	if !ok || declaration.Tok != token.VAR {
		return
	}
	for _, spec := range declaration.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for index, name := range valueSpec.Names {
			value := builder.textList(valueSpec.Values)
			if len(valueSpec.Names) == len(valueSpec.Values) {
				value = builder.text(valueSpec.Values[index])
			}
			builder.add(ObservationAssignment, name, name.Name, "", value, "var")
		}
		for _, expression := range valueSpec.Values {
			builder.addReads(expression)
		}
	}
}

func (builder *observationBuilder) addReads(expression ast.Expr) {
	if expression == nil {
		return
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			for _, argument := range value.Args {
				builder.addReads(argument)
			}
			return false
		case *ast.SelectorExpr:
			builder.add(ObservationRead, value, builder.text(value), "", "", "read")
			return false
		case *ast.IndexExpr:
			builder.add(ObservationRead, value, builder.text(value), "", "", "read")
			return false
		case *ast.IndexListExpr:
			builder.add(ObservationRead, value, builder.text(value), "", "", "read")
			return false
		case *ast.Ident:
			if !ignoredReadIdentifier(value.Name) {
				builder.add(ObservationRead, value, value.Name, "", "", "read")
			}
			return false
		}
		return true
	})
}

func (builder *observationBuilder) visible(node ast.Node) bool {
	if node == nil || !node.Pos().IsValid() {
		return false
	}
	position := builder.parsed.fset.Position(node.Pos())
	return position.Offset >= builder.parsed.packagePrefix && position.Offset < builder.parsed.visibleBytes
}

func (builder *observationBuilder) text(node any) string {
	astNode, ok := node.(ast.Node)
	if !ok || astNode == nil {
		return ""
	}
	var buffer bytes.Buffer
	if err := format.Node(&buffer, builder.parsed.fset, astNode); err != nil {
		return ""
	}
	return strings.TrimSpace(buffer.String())
}

func (builder *observationBuilder) textList(expressions []ast.Expr) string {
	values := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		if value := builder.text(expression); value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, ", ")
}

func validateObservation(function Function, observation Observation) error {
	if observation.ID == "" || observation.Subject != function.Symbol || !validObservationKind(observation.Kind) {
		return fmt.Errorf("observation identity is invalid")
	}
	if observation.Line < function.StartLine || observation.Line > function.EndLine ||
		observation.EndLine < observation.Line || observation.EndLine > function.EndLine ||
		observation.Column <= 0 || observation.EndColumn <= 0 {
		return fmt.Errorf("observation location is outside visible function lines")
	}
	startLine := function.Lines[observation.Line-function.StartLine]
	endLine := function.Lines[observation.EndLine-function.StartLine]
	if observation.Column > len(startLine)+1 || observation.EndColumn > len(endLine)+1 ||
		(observation.Line == observation.EndLine && observation.EndColumn < observation.Column) {
		return fmt.Errorf("observation columns are outside visible function lines")
	}
	if observation.ID != observationID(function, observation) {
		return fmt.Errorf("observation id does not match its content")
	}
	switch observation.Kind {
	case ObservationDeclaration, ObservationBranch, ObservationAssignment, ObservationRead:
		if observation.Object == "" && observation.Kind != ObservationBranch {
			return fmt.Errorf("observation object is empty")
		}
	case ObservationDirectCall:
		if observation.Target == "" || observation.Object == "" {
			return fmt.Errorf("direct-call observation is incomplete")
		}
	case ObservationReturn:
		// Bare returns intentionally have an empty value.
	}
	return nil
}

func validObservationKind(kind ObservationKind) bool {
	switch kind {
	case ObservationDeclaration, ObservationDirectCall, ObservationBranch,
		ObservationAssignment, ObservationRead, ObservationReturn:
		return true
	default:
		return false
	}
}

func observationID(function Function, observation Observation) string {
	hash := sha256.New()
	for _, part := range []string{
		function.ContentSHA256, function.Path, function.Symbol, string(observation.Kind),
		fmt.Sprint(observation.Line), fmt.Sprint(observation.Column),
		fmt.Sprint(observation.EndLine), fmt.Sprint(observation.EndColumn),
		observation.Object, observation.Target,
		observation.Value, observation.Operator,
	} {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return "swf-observation-" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func symbolForDecl(declaration *ast.FuncDecl) string {
	if declaration == nil || declaration.Name == nil || !token.IsIdentifier(declaration.Name.Name) {
		return ""
	}
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return declaration.Name.Name
	}
	receiver := receiverBaseName(declaration.Recv.List[0].Type)
	if receiver == "" {
		return ""
	}
	return receiver + "." + declaration.Name.Name
}

func receiverBaseName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverBaseName(value.X)
	case *ast.ParenExpr:
		return receiverBaseName(value.X)
	case *ast.IndexExpr:
		return receiverBaseName(value.X)
	case *ast.IndexListExpr:
		return receiverBaseName(value.X)
	default:
		return ""
	}
}

func canonicalizeRequestedSymbol(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), " ", "")
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "(*") || strings.HasPrefix(value, "(") {
		opening := 1
		if strings.HasPrefix(value, "(*") {
			opening = 2
		}
		closing := strings.Index(value, ").")
		if closing <= opening {
			return ""
		}
		value = value[opening:closing] + "." + value[closing+2:]
	}
	return canonicalSymbol(value)
}

func canonicalSymbol(value string) string {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 1 && token.IsIdentifier(parts[0]) {
		return parts[0]
	}
	if len(parts) == 2 && token.IsIdentifier(parts[0]) && token.IsIdentifier(parts[1]) {
		return parts[0] + "." + parts[1]
	}
	return ""
}

func ignoredReadIdentifier(value string) bool {
	switch value {
	case "", "_", "nil", "true", "false", "iota":
		return true
	default:
		return false
	}
}
