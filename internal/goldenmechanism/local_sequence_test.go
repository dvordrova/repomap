package goldenmechanism

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestProveSameBranchDirectCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		functions []Function
		branchFn  string
		callFn    string
		wantError bool
	}{
		{
			name: "same branch condition and direct return call",
			functions: []Function{localSequenceFunction("fn-handler", "handler", []string{
				"func handler() error {",
				"    if browseEnabled {",
				"        return serveBrowse()",
				"    }",
				"    return nil",
				"}",
			})},
			branchFn: "fn-handler",
			callFn:   "fn-handler",
		},
		{
			name: "unrelated statements in one function",
			functions: []Function{localSequenceFunction("fn-handler", "handler", []string{
				"func handler() error {",
				"    if browseEnabled {",
				"        recordBrowseIntent()",
				"    }",
				"    return serveBrowse()",
				"}",
			})},
			branchFn:  "fn-handler",
			callFn:    "fn-handler",
			wantError: true,
		},
		{
			name: "calls in sibling branches",
			functions: []Function{localSequenceFunction("fn-handler", "handler", []string{
				"func handler() error {",
				"    if browseEnabled {",
				"        return nil",
				"    }",
				"    if fallbackEnabled {",
				"        return serveBrowse()",
				"    }",
				"    return nil",
				"}",
			})},
			branchFn:  "fn-handler",
			callFn:    "fn-handler",
			wantError: true,
		},
		{
			name: "call in nested child branch",
			functions: []Function{localSequenceFunction("fn-handler", "handler", []string{
				"func handler() error {",
				"    if browseEnabled {",
				"        if requestAllowed {",
				"            return serveBrowse()",
				"        }",
				"    }",
				"    return nil",
				"}",
			})},
			branchFn:  "fn-handler",
			callFn:    "fn-handler",
			wantError: true,
		},
		{
			name: "call after an early return in the same body",
			functions: []Function{localSequenceFunction("fn-handler", "handler", []string{
				"func handler() error {",
				"    if browseEnabled {",
				"        return nil",
				"        return serveBrowse()",
				"    }",
				"    return nil",
				"}",
			})},
			branchFn:  "fn-handler",
			callFn:    "fn-handler",
			wantError: true,
		},
		{
			name: "condition and call in different functions",
			functions: []Function{
				localSequenceFunction("fn-handler", "handler", []string{
					"func handler() error {",
					"    if browseEnabled {",
					"        return nil",
					"    }",
					"    return nil",
					"}",
				}),
				localSequenceFunction("fn-browse", "browse", []string{
					"func browse() error {",
					"    return serveBrowse()",
					"}",
				}),
			},
			branchFn:  "fn-handler",
			callFn:    "fn-browse",
			wantError: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := Result{Functions: test.functions}
			branch := localSequenceObservation(
				"obs-branch", test.branchFn, semanticdiscovery.CapabilityBranch,
				"branch_predicate", BasisBranch, "browseEnabled", "", test.functions,
			)
			call := localSequenceObservation(
				"obs-call", test.callFn, semanticdiscovery.CapabilityDirectCall,
				"direct_local_call", BasisDirectCall, "serveBrowse", "serveBrowse", test.functions,
			)
			result.Observations = []Observation{branch, call}

			proof, err := ProveSameBranchDirectCall(result, SameBranchDirectCallRequest{
				FunctionSymbol:      "handler",
				BranchObservationID: branch.ID,
				CallObservationID:   call.ID,
			})
			if test.wantError {
				if err == nil {
					t.Fatalf("ProveSameBranchDirectCall() proof = %#v, want error", proof)
				}
				return
			}
			if err != nil {
				t.Fatalf("ProveSameBranchDirectCall() error = %v", err)
			}
			if proof.Scope != LocalSequenceScopeSameFunctionBranch ||
				proof.BranchCondition != "browseEnabled" || proof.CalledSymbol != "serveBrowse" ||
				proof.BranchObservation.ID != branch.ID || proof.CallObservation.ID != call.ID {
				t.Fatalf("ProveSameBranchDirectCall() = %#v", proof)
			}
		})
	}
}

func localSequenceFunction(id, symbol string, lines []string) Function {
	source := make([]SourceLine, 0, len(lines))
	for index, text := range lines {
		source = append(source, SourceLine{
			ID: fmt.Sprintf("src-%s-%d", id, index),
			Location: evidence.Location{
				Path: "handler.go", Line: index + 1, Column: 1,
				EndLine: index + 1, EndColumn: len(text) + 1,
			},
			Text: text,
		})
	}
	return Function{ID: id, Symbol: symbol, Path: "handler.go", Source: source}
}

func localSequenceObservation(
	id string,
	functionID string,
	capability semanticdiscovery.Capability,
	operation string,
	basis SyntaxBasis,
	object string,
	target string,
	functions []Function,
) Observation {
	line, column := 2, 8
	if operation == "direct_local_call" {
		line, column = 0, 0
		for _, function := range functions {
			if function.ID != functionID {
				continue
			}
			for _, sourceLine := range function.Source {
				if offset := strings.Index(sourceLine.Text, "serveBrowse("); offset >= 0 {
					line, column = sourceLine.Location.Line, offset+1
					break
				}
			}
		}
	}
	return Observation{
		ID: id, FunctionID: functionID, Capability: capability,
		Operation: operation, Basis: basis, Object: object, TargetSymbol: target,
		Evidence: []EvidenceRef{{
			ID: "ev-" + id,
			Location: evidence.Location{
				Path: "handler.go", Line: line, Column: column,
				EndLine: line, EndColumn: column + len(object),
			},
		}},
	}
}
