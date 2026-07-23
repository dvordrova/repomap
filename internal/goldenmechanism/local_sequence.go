package goldenmechanism

import (
	"fmt"
	"go/ast"
	"go/parser"
	goscanner "go/scanner"
	"go/token"
	"strings"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const LocalSequenceScopeSameFunctionBranch = "same_function_branch"

// SameBranchDirectCallRequest names two already saved syntax observations.
// Proving the relation never reads repository source or runs the probe again.
type SameBranchDirectCallRequest struct {
	FunctionSymbol      string
	BranchObservationID string
	CallObservationID   string
}

// LocalSequenceProof is deliberately narrower than runtime ordering. It says
// only that one exact direct call is a top-level statement in one exact if body
// retained by the same bounded function snapshot.
type LocalSequenceProof struct {
	Scope             string
	FunctionID        string
	BranchCondition   string
	CalledSymbol      string
	BranchObservation Observation
	CallObservation   Observation
}

// ProveSameBranchDirectCall establishes a syntax-only conditional relation
// from a saved probe result. The call must be a direct expression, return
// expression, or assignment expression in the selected if body's top level.
// Calls outside that body, in nested/sibling branches, or in another function
// are rejected.
func ProveSameBranchDirectCall(
	result Result,
	request SameBranchDirectCallRequest,
) (LocalSequenceProof, error) {
	function, err := requiredSequenceFunction(result.Functions, request.FunctionSymbol)
	if err != nil {
		return LocalSequenceProof{}, err
	}
	branch, err := requiredSequenceObservation(result.Observations, request.BranchObservationID)
	if err != nil {
		return LocalSequenceProof{}, err
	}
	call, err := requiredSequenceObservation(result.Observations, request.CallObservationID)
	if err != nil {
		return LocalSequenceProof{}, err
	}
	if branch.FunctionID != function.ID || call.FunctionID != function.ID {
		return LocalSequenceProof{}, fmt.Errorf(
			"golden mechanism: local sequence observations do not belong to the same selected function",
		)
	}
	if branch.Capability != semanticdiscovery.CapabilityBranch ||
		branch.Operation != "branch_predicate" || branch.Basis != BasisBranch {
		return LocalSequenceProof{}, fmt.Errorf(
			"golden mechanism: local sequence branch observation is not an exact branch predicate",
		)
	}
	if call.Capability != semanticdiscovery.CapabilityDirectCall ||
		call.Operation != "direct_local_call" || call.Basis != BasisDirectCall ||
		strings.TrimSpace(call.TargetSymbol) == "" {
		return LocalSequenceProof{}, fmt.Errorf(
			"golden mechanism: local sequence call observation is not an exact direct call",
		)
	}
	if len(branch.Evidence) != 1 || len(call.Evidence) != 1 {
		return LocalSequenceProof{}, fmt.Errorf(
			"golden mechanism: local sequence observations need singular exact locations",
		)
	}
	if branch.Evidence[0].Location.Path != function.Path ||
		call.Evidence[0].Location.Path != function.Path {
		return LocalSequenceProof{}, fmt.Errorf(
			"golden mechanism: local sequence evidence is outside the selected function source",
		)
	}

	source, lineOffsets, err := retainedSequenceSource(function)
	if err != nil {
		return LocalSequenceProof{}, err
	}
	branchOffset, err := retainedSequenceOffset(
		source,
		lineOffsets,
		branch.Evidence[0].Location.Line,
		branch.Evidence[0].Location.Column,
	)
	if err != nil {
		return LocalSequenceProof{}, err
	}
	callOffset, err := retainedSequenceOffset(
		source,
		lineOffsets,
		call.Evidence[0].Location.Line,
		call.Evidence[0].Location.Column,
	)
	if err != nil {
		return LocalSequenceProof{}, err
	}
	ifStart, bodyEnd, err := retainedIfBody(
		function.Path,
		source,
		branchOffset,
		branch.Object,
	)
	if err != nil {
		return LocalSequenceProof{}, err
	}
	if callOffset <= ifStart || callOffset >= bodyEnd {
		return LocalSequenceProof{}, fmt.Errorf(
			"golden mechanism: direct call is not inside the selected branch body",
		)
	}
	if err := requireTopLevelBranchCall(
		source[ifStart:bodyEnd+1],
		callOffset-ifStart,
	); err != nil {
		return LocalSequenceProof{}, err
	}
	return LocalSequenceProof{
		Scope:             LocalSequenceScopeSameFunctionBranch,
		FunctionID:        function.ID,
		BranchCondition:   branch.Object,
		CalledSymbol:      call.TargetSymbol,
		BranchObservation: branch,
		CallObservation:   call,
	}, nil
}

func requiredSequenceFunction(functions []Function, symbol string) (Function, error) {
	var matched Function
	for _, function := range functions {
		if function.Symbol != symbol {
			continue
		}
		if matched.ID != "" {
			return Function{}, fmt.Errorf(
				"golden mechanism: local sequence function %q is ambiguous",
				symbol,
			)
		}
		matched = function
	}
	if matched.ID == "" || len(matched.Source) == 0 {
		return Function{}, fmt.Errorf(
			"golden mechanism: local sequence function %q is unavailable",
			symbol,
		)
	}
	return matched, nil
}

func requiredSequenceObservation(
	observations []Observation,
	id string,
) (Observation, error) {
	var matched Observation
	for _, observation := range observations {
		if observation.ID != id {
			continue
		}
		if matched.ID != "" {
			return Observation{}, fmt.Errorf(
				"golden mechanism: local sequence observation %q is duplicated",
				id,
			)
		}
		matched = observation
	}
	if matched.ID == "" {
		return Observation{}, fmt.Errorf(
			"golden mechanism: local sequence observation %q is unavailable",
			id,
		)
	}
	return matched, nil
}

func retainedSequenceSource(
	function Function,
) ([]byte, map[int]int, error) {
	var source strings.Builder
	lineOffsets := make(map[int]int, len(function.Source))
	expectedLine := function.Source[0].Location.Line
	for _, line := range function.Source {
		if line.Location.Path != function.Path || line.Location.Line != expectedLine ||
			line.Location.Column != 1 || strings.ContainsAny(line.Text, "\r\n") {
			return nil, nil, fmt.Errorf(
				"golden mechanism: retained local sequence source is not contiguous",
			)
		}
		lineOffsets[expectedLine] = source.Len()
		source.WriteString(line.Text)
		source.WriteByte('\n')
		expectedLine++
	}
	return []byte(source.String()), lineOffsets, nil
}

func retainedSequenceOffset(
	source []byte,
	lineOffsets map[int]int,
	line int,
	column int,
) (int, error) {
	start, exists := lineOffsets[line]
	if !exists || column <= 0 {
		return 0, fmt.Errorf(
			"golden mechanism: local sequence evidence is outside retained source",
		)
	}
	offset := start + column - 1
	if offset < start || offset >= len(source) {
		return 0, fmt.Errorf(
			"golden mechanism: local sequence evidence column is outside retained source",
		)
	}
	return offset, nil
}

type retainedToken struct {
	token  token.Token
	offset int
}

func retainedIfBody(
	filename string,
	source []byte,
	conditionOffset int,
	wantCondition string,
) (int, int, error) {
	fset := token.NewFileSet()
	file := fset.AddFile(filename, -1, len(source))
	var scanner goscanner.Scanner
	scanner.Init(file, source, nil, 0)
	tokens := make([]retainedToken, 0, len(source)/4)
	for {
		position, current, _ := scanner.Scan()
		if current == token.EOF {
			break
		}
		tokens = append(tokens, retainedToken{
			token: current, offset: file.Offset(position),
		})
	}
	for index, current := range tokens {
		if current.token != token.IF || current.offset >= conditionOffset {
			continue
		}
		open := -1
		for cursor := index + 1; cursor < len(tokens); cursor++ {
			if tokens[cursor].token == token.LBRACE {
				open = cursor
				break
			}
			if tokens[cursor].token == token.SEMICOLON {
				break
			}
		}
		if open < 0 || conditionOffset >= tokens[open].offset {
			continue
		}
		condition := strings.TrimSpace(
			string(source[current.offset+len("if") : tokens[open].offset]),
		)
		if strings.Join(strings.Fields(condition), " ") !=
			strings.Join(strings.Fields(wantCondition), " ") {
			continue
		}
		depth := 0
		for cursor := open; cursor < len(tokens); cursor++ {
			switch tokens[cursor].token {
			case token.LBRACE:
				depth++
			case token.RBRACE:
				depth--
				if depth == 0 {
					return current.offset, tokens[cursor].offset, nil
				}
			}
		}
		return 0, 0, fmt.Errorf(
			"golden mechanism: selected branch is truncated before its closing brace",
		)
	}
	return 0, 0, fmt.Errorf(
		"golden mechanism: saved syntax does not contain the selected branch predicate",
	)
}

func requireTopLevelBranchCall(branch []byte, relativeCallOffset int) error {
	const prefix = "package retained\nfunc retained() {\n"
	wrapped := append([]byte(prefix), branch...)
	wrapped = append(wrapped, []byte("\n}\n")...)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "retained.go", wrapped, 0)
	if err != nil {
		return fmt.Errorf("golden mechanism: parse retained local branch: %w", err)
	}
	if len(file.Decls) != 1 {
		return fmt.Errorf("golden mechanism: retained local branch has an unexpected declaration shape")
	}
	declaration, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || declaration.Body == nil || len(declaration.Body.List) != 1 {
		return fmt.Errorf("golden mechanism: retained local branch has an unexpected function shape")
	}
	branchNode, ok := declaration.Body.List[0].(*ast.IfStmt)
	if !ok {
		return fmt.Errorf("golden mechanism: retained local source is not an if branch")
	}
	if len(branchNode.Body.List) != 1 {
		return fmt.Errorf(
			"golden mechanism: selected branch does not have one unconditional direct-call statement",
		)
	}
	wantOffset := len(prefix) + relativeCallOffset
	calls := directStatementCalls(branchNode.Body.List[0])
	if len(calls) == 1 && fset.Position(calls[0].Pos()).Offset == wantOffset {
		return nil
	}
	return fmt.Errorf(
		"golden mechanism: direct call is not the selected branch's unconditional statement",
	)
}

func directStatementCalls(statement ast.Stmt) []*ast.CallExpr {
	var expressions []ast.Expr
	switch value := statement.(type) {
	case *ast.ExprStmt:
		if call, ok := value.X.(*ast.CallExpr); ok {
			return []*ast.CallExpr{call}
		}
	case *ast.ReturnStmt:
		if len(value.Results) == 1 {
			expressions = value.Results
		}
	case *ast.AssignStmt:
		if len(value.Rhs) == 1 {
			expressions = value.Rhs
		}
	default:
		return nil
	}
	calls := make([]*ast.CallExpr, 0, len(expressions))
	for _, expression := range expressions {
		if call, ok := expression.(*ast.CallExpr); ok {
			calls = append(calls, call)
		}
	}
	return calls
}
