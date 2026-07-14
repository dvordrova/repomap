package symbol

import (
	"strings"
	"testing"
)

func TestValidateReport(t *testing.T) {
	t.Parallel()

	bundle, err := Build(testGraph(), Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	report := validReport()
	if err := ValidateReport(bundle, report); err != nil {
		t.Fatalf("ValidateReport() error = %v", err)
	}
}

func TestValidateReportRejectsUnsupportedClaims(t *testing.T) {
	t.Parallel()

	bundle, err := Build(testGraph(), Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	tests := []struct {
		name     string
		mutate   func(*Report)
		expected string
	}{
		{
			name: "unknown evidence",
			mutate: func(report *Report) {
				report.Summary.EvidenceIDs = []string{"invented-001"}
			},
			expected: "unknown evidence id",
		},
		{
			name: "path outside bundle",
			mutate: func(report *Report) {
				report.FilesToReadInOrder[0].Path = "invented/file.go"
			},
			expected: "outside allowed_paths",
		},
		{
			name: "possible evidence presented as static",
			mutate: func(report *Report) {
				report.Callers[0].EvidenceIDs = []string{"call-in-001", "candidate-002"}
			},
			expected: "non-static evidence",
		},
		{
			name: "summary presented as static fact",
			mutate: func(report *Report) {
				report.Summary.Basis = BasisStaticFact
			},
			expected: "summary basis must be inference",
		},
		{
			name: "summary exceeds static inference confidence cap",
			mutate: func(report *Report) {
				report.Summary.Confidence = 0.8
			},
			expected: "confidence exceeds static-only inference cap",
		},
		{
			name: "wrong target",
			mutate: func(report *Report) {
				report.Target.Name = "KVServer.Put"
			},
			expected: "target does not match",
		},
		{
			name: "caller identity does not match call evidence",
			mutate: func(report *Report) {
				report.Callers[0].Symbol = EntityRef{Name: "retryPut", Kind: "function", Path: "server/retry.go", Line: 20}
			},
			expected: "symbol does not match",
		},
		{
			name: "file path not supported by evidence",
			mutate: func(report *Report) {
				report.FilesToReadInOrder[0].Path = "server/txn.go"
			},
			expected: "path and structural_role are not supported",
		},
		{
			name: "file structural role not supported by evidence",
			mutate: func(report *Report) {
				report.FilesToReadInOrder[0].StructuralRole = "static_callee"
			},
			expected: "path and structural_role are not supported",
		},
		{
			name: "tests require explicit test file",
			mutate: func(report *Report) {
				report.TestsToRead = []FileRecommendation{{Path: "server/key.go", StructuralRole: "test_reference", EvidenceIDs: []string{"resolution-001"}}}
			},
			expected: "not an explicit go test file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := validReport()
			tt.mutate(&report)
			err := ValidateReport(bundle, report)
			if err == nil || !strings.Contains(err.Error(), tt.expected) {
				t.Fatalf("ValidateReport() error = %v, want %q", err, tt.expected)
			}
		})
	}
}

func validReport() Report {
	return Report{
		Target: EntityRef{Name: "kvServer.Put", Kind: "method", Path: "server/key.go", Line: 90, Column: 20},
		Summary: Claim{
			Statement:   "kvServer.Put is the uniquely resolved target.",
			Basis:       BasisInference,
			EvidenceIDs: []string{"resolution-001"},
			Confidence:  0.7,
		},
		Responsibility: Claim{
			Statement:   "The method likely delegates work to Txn.",
			Basis:       BasisInference,
			EvidenceIDs: []string{"call-out-001"},
			Confidence:  0.7,
		},
		Callers: []Relationship{{
			Symbol:       EntityRef{Name: "servePut", Kind: "function", Path: "server/handler.go", Line: 40, Column: 6},
			Relationship: "statically calls the target under the stated build scenario",
			Basis:        BasisStaticFact,
			EvidenceIDs:  []string{"call-in-001"},
			Confidence:   1,
		}},
		Callees: []Relationship{{
			Symbol:       EntityRef{Name: "Txn", Kind: "method", Path: "server/txn.go", Line: 30, Column: 18},
			Relationship: "is statically called by the target under the stated build scenario",
			Basis:        BasisStaticFact,
			EvidenceIDs:  []string{"call-out-001"},
			Confidence:   1,
		}},
		FilesToReadInOrder: []FileRecommendation{{
			Path:           "server/key.go",
			Line:           90,
			StructuralRole: "target",
			EvidenceIDs:    []string{"resolution-001"},
		}},
		Unknowns: []string{"Which test executes this path?"},
		NextQueries: []NextQuery{{
			Query:  "references to kvServer.Put from _test.go files",
			Reason: "find an executable scenario",
		}},
		Warnings: []string{"No runtime evidence was provided."},
	}
}

func TestParseReportWarnsAndIgnoresUnknownFields(t *testing.T) {
	t.Parallel()

	bundle, err := Build(testGraph(), Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	data := []byte(`{
		"target": {},
		"summary": {},
		"responsibility": {},
		"callers": [],
		"callees": [],
		"files_to_read_in_order": [],
		"tests_to_read": [],
		"unknowns": [],
		"next_queries": [],
		"warnings": [],
		"runtime_hypotheses": []
	}`)
	result, err := ParseReport(bundle, data)
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if !hasWarningCode(result.Warnings, "schema.unknown_field") {
		t.Fatalf("warnings = %#v, want unknown field warning", result.Warnings)
	}
	if err := ValidateReport(bundle, result.Report); err != nil {
		t.Fatalf("normalized report error = %v", err)
	}
}

func TestParseReportToleratesWeakJSONContract(t *testing.T) {
	t.Parallel()

	bundle, err := Build(testGraph(), Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	data := []byte("```json\n" + `{
		"summary": "Looks like a Put handler",
		"responsibility": {
			"text": "Probably delegates to Txn",
			"basis": "fact",
			"confidence": "95%",
			"evidence_ids": ["invented-001"]
		},
		"callers": "servePut",
		"files_to_read": ["server/key.go", "invented/file.go"],
		"unknowns": "Which test executes it?",
		"next_queries": ["find tests"],
		"extra": true,
	}` + "\n```")

	result, err := ParseReport(bundle, data)
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if result.Format != "json" {
		t.Fatalf("format = %q, want json", result.Format)
	}
	for _, code := range []string{"json.recovered", "json.trailing_comma_removed", "schema.unknown_field", "responsibility.confidence_capped", "evidence.unknown_dropped", "callers.rebuilt", "files_to_read_in_order.path_dropped"} {
		if !hasWarningCode(result.Warnings, code) {
			t.Errorf("warnings missing %q: %#v", code, result.Warnings)
		}
	}
	if result.Report.Target.Name != "kvServer.Put" {
		t.Fatalf("target = %q", result.Report.Target.Name)
	}
	if len(result.Report.Callers) != len(bundle.IncomingCalls) {
		t.Fatalf("callers = %d, want %d", len(result.Report.Callers), len(bundle.IncomingCalls))
	}
	if result.Report.Responsibility.Confidence != 0.75 {
		t.Fatalf("responsibility confidence = %v, want 0.75", result.Report.Responsibility.Confidence)
	}
}

func TestParseReportAcceptsTaggedFormat(t *testing.T) {
	t.Parallel()

	bundle, err := Build(testGraph(), Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	data := []byte(`SUMMARY: Put handler inferred from static calls
SUMMARY_EVIDENCE: resolution-001, call-out-001
SUMMARY_CONFIDENCE: 65%
RESPONSIBILITY: Probably delegates to Txn
RESPONSIBILITY_EVIDENCE: call-out-001
RESPONSIBILITY_CONFIDENCE: 0.6
READ: resolution-001
READ: call-out-001
UNKNOWN: Which test executes it?
NEXT_QUERY: references to kvServer.Put || find an executable test scenario
WARNING: static graph is not runtime truth`)

	result, err := ParseReport(bundle, data)
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if result.Format != "tagged" {
		t.Fatalf("format = %q, want tagged", result.Format)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want no repairs for valid tagged response", result.Warnings)
	}
	if len(result.Report.FilesToReadInOrder) != 2 {
		t.Fatalf("files = %#v, want target and callee", result.Report.FilesToReadInOrder)
	}
	if result.Report.Summary.Confidence != 0.65 {
		t.Fatalf("summary confidence = %v, want 0.65", result.Report.Summary.Confidence)
	}
	if score := Evaluate(result).Score; score != 100 {
		t.Fatalf("evaluation score = %d, want 100; warnings = %#v", score, result.Warnings)
	}
}

func hasWarningCode(warnings []ParseWarning, code string) bool {
	for _, item := range warnings {
		if item.Code == code {
			return true
		}
	}
	return false
}
