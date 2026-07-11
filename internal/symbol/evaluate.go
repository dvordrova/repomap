package symbol

import "strings"

const EvaluationVersion = 1

type Evaluation struct {
	Version      int               `json:"version"`
	Score        int               `json:"score"`
	MaxScore     int               `json:"max_score"`
	WarningCount int               `json:"warning_count"`
	WarningCodes []string          `json:"warning_codes,omitempty"`
	Checks       []EvaluationCheck `json:"checks"`
}

type EvaluationCheck struct {
	Name      string `json:"name"`
	Points    int    `json:"points"`
	MaxPoints int    `json:"max_points"`
	Passed    bool   `json:"passed"`
	Detail    string `json:"detail,omitempty"`
}

// Evaluate scores only observable contract quality. It does not claim that an
// interpretation is semantically correct; that requires source or runtime
// evidence which is intentionally absent from a symbol bundle.
func Evaluate(result ParseResult) Evaluation {
	evaluation := Evaluation{
		Version:      EvaluationVersion,
		MaxScore:     100,
		WarningCount: len(result.Warnings),
		WarningCodes: sortedWarningCodes(result.Warnings),
	}

	add := func(name string, maxPoints int, passed bool, detail string) {
		points := 0
		if passed {
			points = maxPoints
		}
		evaluation.Score += points
		evaluation.Checks = append(evaluation.Checks, EvaluationCheck{
			Name: name, Points: points, MaxPoints: maxPoints, Passed: passed, Detail: detail,
		})
	}
	addPoints := func(name string, points, maxPoints int, detail string) {
		evaluation.Score += points
		evaluation.Checks = append(evaluation.Checks, EvaluationCheck{
			Name: name, Points: points, MaxPoints: maxPoints, Passed: points == maxPoints, Detail: detail,
		})
	}

	add("response_decoded_cleanly", 10,
		!hasWarningPrefix(result.Warnings, "json.") &&
			!hasWarningPrefix(result.Warnings, "tagged.line_ignored") &&
			!hasWarningPrefix(result.Warnings, "tagged.key_ignored"),
		"JSON recovery and ignored tagged lines reduce format reliability")
	add("schema_followed", 10,
		!hasWarningPrefix(result.Warnings, "schema."),
		"Unknown fields indicate prompt-contract drift")
	add("evidence_grounded", 15,
		!hasWarningPrefix(result.Warnings, "evidence.") &&
			!hasWarningPrefix(result.Warnings, "target."),
		"Invented evidence or target drift is discarded locally")
	add("summary_contract", 15,
		claimLooksUseful(result.Report.Summary) && !hasWarningPrefix(result.Warnings, "summary."),
		"Summary needs a statement, valid evidence, and confidence without repair")
	add("responsibility_contract", 15,
		claimLooksUseful(result.Report.Responsibility) && !hasWarningPrefix(result.Warnings, "responsibility."),
		"Responsibility needs a statement, valid evidence, and confidence without repair")
	add("structural_facts_not_repaired", 5,
		!hasWarningPrefix(result.Warnings, "callers.") && !hasWarningPrefix(result.Warnings, "callees."),
		"Target and relationships are rebuilt locally; model drift is still measured")
	add("reading_order_grounded", 10,
		len(result.Report.FilesToReadInOrder) > 0 &&
			!hasAnyWarningFragment(result.Warnings, ".path_dropped", ".non_test_dropped", ".unsupported_dropped", ".item_dropped"),
		"Path and role inference from valid evidence IDs is expected and not penalized")
	add("uncertainty_and_next_steps", 10,
		len(result.Report.Unknowns) > 0 && len(result.Report.NextQueries) > 0,
		"A useful static report should expose an unknown and a concrete next query")
	cautionPoints := 0
	if usesInferenceLanguage(result.Report.Summary.Statement) {
		cautionPoints += 5
	}
	if usesInferenceLanguage(result.Report.Responsibility.Statement) {
		cautionPoints += 5
	}
	addPoints("epistemic_caution", cautionPoints, 10,
		"Both interpretations should explicitly say likely, suggests, inferred, or static")

	return evaluation
}

func usesInferenceLanguage(statement string) bool {
	statement = strings.ToLower(statement)
	for _, marker := range []string{"likely", "suggest", "infer", "appears", "probably", "may ", "could ", "static"} {
		if strings.Contains(statement, marker) {
			return true
		}
	}
	return false
}

func claimLooksUseful(claim Claim) bool {
	return strings.TrimSpace(claim.Statement) != "" && len(claim.EvidenceIDs) > 0 && claim.Confidence >= 0 && claim.Confidence <= 0.75
}

func hasWarningPrefix(warnings []ParseWarning, prefix string) bool {
	for _, item := range warnings {
		if strings.HasPrefix(item.Code, prefix) {
			return true
		}
	}
	return false
}

func hasAnyWarningFragment(warnings []ParseWarning, fragments ...string) bool {
	for _, item := range warnings {
		for _, fragment := range fragments {
			if strings.Contains(item.Code, fragment) {
				return true
			}
		}
	}
	return false
}
