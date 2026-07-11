package sourceexplain

import "strings"

const EvaluationVersion = 2

type Evaluation struct {
	Version      int               `json:"version"`
	Score        int               `json:"score"`
	MaxScore     int               `json:"max_score"`
	Checks       []EvaluationCheck `json:"checks"`
	WarningCodes []string          `json:"warning_codes"`
}

type EvaluationCheck struct {
	Name    string `json:"name"`
	Weight  int    `json:"weight"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

func Evaluate(result ParseResult) Evaluation {
	checks := []EvaluationCheck{
		check("all_questions_assessed", 20, !hasWarningPrefix(result.Warnings, "assessment.missing_"), "the model explicitly assessed every seeded question"),
		check("verdicts_understood", 15, !hasWarningPrefix(result.Warnings, "assessment.verdict_"), "all model verdicts used the supported vocabulary"),
		check("question_scoped_evidence", 25, !hasWarningPrefix(result.Warnings, "assessment.evidence_") && !hasWarningCode(result.Warnings, "assessment.shown_without_anchor"), "cited source evidence stayed inside each question's candidate set"),
		check("no_conflicting_assessments", 10, !hasWarningCode(result.Warnings, "assessment.duplicate_ambiguous"), "the model did not emit conflicting duplicate assessments"),
		check("allowed_model_action", 15, result.Report.NextAction.Origin == ActionOriginModel, "the model selected one locally allowed executable action"),
		check("clean_response_shape", 10, !usedContractRecovery(result.Warnings), "the response used the documented JSON fields and shapes without local format repair"),
		check("mandatory_unknowns_explicit", 5, !hasWarningCode(result.Warnings, "unknowns.missing_defaulted") && !hasWarningCode(result.Warnings, "unknowns.mandatory_defaulted"), "the model explicitly retained test coverage and runtime reachability as unknowns"),
	}
	evaluation := Evaluation{
		Version:      EvaluationVersion,
		MaxScore:     100,
		Checks:       checks,
		WarningCodes: warningCodes(result.Warnings),
	}
	for _, item := range checks {
		if item.Passed {
			evaluation.Score += item.Weight
		}
	}
	return evaluation
}

func usedContractRecovery(warnings []ParseWarning) bool {
	for _, warning := range warnings {
		switch warning.Code {
		case "response.object_recovered",
			"response.trailing_comma_repaired",
			"response.unknown_field",
			"assessments.object_accepted",
			"assessments.invalid_ignored",
			"unknowns.scalar_accepted",
			"unknowns.missing_defaulted":
			return true
		}
	}
	return false
}

func check(name string, weight int, passed bool, message string) EvaluationCheck {
	return EvaluationCheck{Name: name, Weight: weight, Passed: passed, Message: message}
}

func hasWarningPrefix(warnings []ParseWarning, prefix string) bool {
	for _, warning := range warnings {
		if strings.HasPrefix(warning.Code, prefix) {
			return true
		}
	}
	return false
}

func hasWarningCode(warnings []ParseWarning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func hasMandatoryUnknowns(report Report) bool {
	hasTests := false
	hasRuntime := false
	for _, unknown := range report.Unknowns {
		switch unknown.Kind {
		case UnknownTestCoverage:
			hasTests = true
		case UnknownRuntimeReachability:
			hasRuntime = true
		}
	}
	return hasTests && hasRuntime
}

func warningCodes(warnings []ParseWarning) []string {
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, warning.Code)
	}
	return result
}
