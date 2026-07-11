package sourceexplain

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/symbol"
)

func TestParseReportBuildsOnlyGroundedLocalClaims(t *testing.T) {
	t.Parallel()

	bundle := sourceBundleFixture(t)
	result, err := ParseReport(bundle, []byte(validSourceResponse))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	if len(result.Report.Claims) != 4 {
		t.Fatalf("claims = %#v", result.Report.Claims)
	}
	for _, claim := range result.Report.Claims {
		if claim.EvidenceLevel != EvidenceLevelSourceSupported || claim.Statement == "" {
			t.Fatalf("claim = %#v", claim)
		}
	}
	statements := []string{
		result.Report.Claims[0].Statement,
		result.Report.Claims[1].Statement,
		result.Report.Claims[2].Statement,
		result.Report.Claims[3].Statement,
	}
	for _, fragment := range []string{
		"uses the result of calling checkPutRequest in a conditional guard",
		"assigns value(s) returned by Put",
		"locally visible nil comparison",
		"what the callee changes remains unverified",
	} {
		if !containsFragment(statements, fragment) {
			t.Fatalf("claims = %#v, missing statement fragment %q", result.Report.Claims, fragment)
		}
	}
	if result.Report.NextAction.Operation != OperationFindTests || result.Report.NextAction.Origin != ActionOriginModel {
		t.Fatalf("next action = %#v", result.Report.NextAction)
	}
	if !hasUnknown(result.Report.Unknowns, UnknownTestCoverage, bundle.Target.EvidenceID) ||
		!hasUnknown(result.Report.Unknowns, UnknownRuntimeReachability, bundle.Target.EvidenceID) {
		t.Fatalf("mandatory unknowns = %#v", result.Report.Unknowns)
	}
}

func TestParseReportDowngradesIncompleteMultilineValidationEvidence(t *testing.T) {
	t.Parallel()

	structural, card := labelsIsValidFixture()
	bundle, err := Build(structural, card)
	if err != nil {
		t.Fatal(err)
	}
	response := `{
  "assessments": [{
    "question_id": "question-call-out-001",
    "verdict": "shown",
    "source_evidence_ids": ["source-119"]
  }],
  "unknowns": [
    {"kind":"test_coverage","anchor_evidence_id":"resolution-001"},
    {"kind":"runtime_reachability","anchor_evidence_id":"resolution-001"}
  ],
  "next_action_id": "action-find-tests"
}`
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	assessment := result.Report.Assessments[0]
	if assessment.Verdict != VerdictAmbiguous ||
		!equalStrings(assessment.SourceEvidenceIDs, []string{"source-119"}) ||
		len(result.Report.Claims) != 0 {
		t.Fatalf("report = %#v", result.Report)
	}
	if !hasParseWarning(result.Warnings, "assessment.shown_without_predicate_support") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	evaluation := Evaluate(result)
	if evaluation.Score != 75 || evaluation.MaxScore != 100 {
		t.Fatalf("evaluation = %#v", evaluation)
	}
}

func TestParseReportAcceptsExplicitMultilineValidationEvidenceCleanly(t *testing.T) {
	t.Parallel()

	structural, card := labelsIsValidFixture()
	bundle, err := Build(structural, card)
	if err != nil {
		t.Fatal(err)
	}
	response := `{
  "assessments": [{
    "question_id": "question-call-out-001",
    "verdict": "shown",
    "source_evidence_ids": ["source-119", "source-132"]
  }],
  "unknowns": [
    {"kind":"test_coverage","anchor_evidence_id":"resolution-001"},
    {"kind":"runtime_reachability","anchor_evidence_id":"resolution-001"}
  ],
  "next_action_id": "action-find-tests"
}`
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if len(result.Warnings) != 0 || len(result.Report.Claims) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if evaluation := Evaluate(result); evaluation.Score != 100 {
		t.Fatalf("evaluation = %#v", evaluation)
	}
}

func TestParseReportDropsValidButIrrelevantEvidence(t *testing.T) {
	t.Parallel()

	bundle := sourceBundleFixture(t)
	response := `{
  "assessments": [
    {"question_id":"question-call-out-002","verdict":"shown","source_evidence_ids":["source-95"]}
  ],
  "unknowns": [],
  "next_action_id": "action-find-tests"
}`
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if result.Report.Assessments[0].Verdict != VerdictAmbiguous {
		t.Fatalf("assessment = %#v", result.Report.Assessments[0])
	}
	if hasClaim(result.Report.Claims, PredicateValidatesInput) {
		t.Fatalf("irrelevant evidence produced claim: %#v", result.Report.Claims)
	}
	if !hasParseWarning(result.Warnings, "assessment.evidence_irrelevant_dropped") ||
		!hasParseWarning(result.Warnings, "assessment.shown_without_anchor") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestParseReportRecoversWeakJSONDrift(t *testing.T) {
	t.Parallel()

	bundle := sourceBundleFixture(t)
	response := "result follows\n```json\n{\n" +
		`"assessments":{"question_id":"question-call-out-002","verdict":"SHOWN","evidence_ids":"source-91"},` + "\n" +
		`"unknowns":"test_coverage",` + "\n" +
		`"next_action_id":"action-find-tests",` + "\n" +
		`"invented_path":"/tmp/escape"` + "\n}\n```"
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if result.Report.Assessments[0].Verdict != VerdictShown || len(result.Report.Claims) != 1 {
		t.Fatalf("report = %#v", result.Report)
	}
	wantWarnings := []string{
		"assessment.evidence_alias_accepted",
		"assessment.evidence_scalar_accepted",
		"assessment.verdict_case_normalized",
		"assessments.object_accepted",
		"response.object_recovered",
		"response.unknown_field",
		"unknowns.scalar_accepted",
	}
	for _, code := range wantWarnings {
		if !hasParseWarning(result.Warnings, code) {
			t.Fatalf("warnings = %#v, missing %q", result.Warnings, code)
		}
	}
}

func TestParseReportUsesLocalActionFallback(t *testing.T) {
	t.Parallel()

	bundle := sourceBundleFixture(t)
	response := `{
  "assessments": [],
  "unknowns": [{"kind":"runtime_observed","anchor_evidence_id":"invented"}],
  "next_action_id": "run-arbitrary-command"
}`
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if result.Report.NextAction.Origin != ActionOriginLocalPolicy || result.Report.NextAction.ID != "action-find-tests" {
		t.Fatalf("next action = %#v", result.Report.NextAction)
	}
	if len(result.Report.Claims) != 0 {
		t.Fatalf("claims = %#v", result.Report.Claims)
	}
	if !hasParseWarning(result.Warnings, "action.local_fallback") || !hasParseWarning(result.Warnings, "unknown.kind_ignored") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestParseReportMakesDuplicateAssessmentAmbiguous(t *testing.T) {
	t.Parallel()

	bundle := sourceBundleFixture(t)
	response := `{
  "assessments": [
    {"question_id":"question-call-out-002","verdict":"shown","source_evidence_ids":["source-91"]},
    {"question_id":"question-call-out-002","verdict":"not_shown","source_evidence_ids":[]}
  ],
  "unknowns": [],
  "next_action_id": "action-find-tests"
}`
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if result.Report.Assessments[0].Verdict != VerdictAmbiguous || hasClaim(result.Report.Claims, PredicateValidatesInput) {
		t.Fatalf("report = %#v", result.Report)
	}
	if !hasParseWarning(result.Warnings, "assessment.duplicate_ambiguous") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestParseReportNeverAcceptsNotShownFromLexicalWindow(t *testing.T) {
	t.Parallel()

	bundle := sourceBundleFixture(t)
	response := `{
  "assessments": [
    {"question_id":"question-call-out-002","verdict":"not_shown","source_evidence_ids":[]}
  ],
  "unknowns": [],
  "next_action_id": "action-find-tests"
}`
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if result.Report.Assessments[0].Verdict != VerdictAmbiguous || !hasParseWarning(result.Warnings, "assessment.not_shown_lexical_window") {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseReportWarnsWhenUnknownsFieldIsMissing(t *testing.T) {
	t.Parallel()

	response := `{"assessments":[],"next_action_id":"action-find-tests"}`
	result, err := ParseReport(sourceBundleFixture(t), []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if !hasParseWarning(result.Warnings, "unknowns.missing_defaulted") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	if !hasParseWarning(result.Warnings, "unknowns.mandatory_defaulted") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	if !hasUnknown(result.Report.Unknowns, UnknownTestCoverage, result.Report.Target.EvidenceID) ||
		!hasUnknown(result.Report.Unknowns, UnknownRuntimeReachability, result.Report.Target.EvidenceID) {
		t.Fatalf("mandatory unknowns = %#v", result.Report.Unknowns)
	}
}

func TestParseReportDoesNotPromoteMisleadingCalleeNames(t *testing.T) {
	t.Parallel()

	bundle := misleadingBundleFixture(t)
	response := `{
  "assessments": [
    {"question_id":"question-call-out-check","verdict":"shown","source_evidence_ids":["source-91"]},
    {"question_id":"question-call-out-error","verdict":"shown","source_evidence_ids":["source-92"]}
  ],
  "unknowns": [],
  "next_action_id": "action-find-tests"
}`
	result, err := ParseReport(bundle, []byte(response))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if len(result.Report.Claims) != 0 {
		t.Fatalf("misleading callees produced claims: %#v", result.Report.Claims)
	}
	for _, assessment := range result.Report.Assessments {
		if assessment.Verdict != VerdictAmbiguous {
			t.Fatalf("assessment = %#v", assessment)
		}
	}
	if countParseWarnings(result.Warnings, "assessment.shown_without_predicate_support") != 2 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestValidateReportRejectsForgedShownAssessmentWithoutLexicalSupport(t *testing.T) {
	t.Parallel()

	bundle := misleadingBundleFixture(t)
	result, err := ParseReport(bundle, []byte(`{
  "assessments": [],
  "unknowns": [],
  "next_action_id": "action-find-tests"
}`))
	if err != nil {
		t.Fatal(err)
	}
	result.Report.Assessments[0] = Assessment{
		QuestionID:        bundle.Questions[0].ID,
		Verdict:           VerdictShown,
		SourceEvidenceIDs: []string{bundle.Questions[0].AnchorSourceEvidenceID},
	}
	if err := ValidateReport(bundle, result.Report); err == nil || !strings.Contains(err.Error(), "predicate-specific lexical support") {
		t.Fatalf("ValidateReport() error = %v", err)
	}
}

func TestParseReportRejectsUnreadableResponse(t *testing.T) {
	t.Parallel()

	if _, err := ParseReport(sourceBundleFixture(t), []byte("not json")); err == nil {
		t.Fatal("ParseReport() error = nil")
	}
}

func TestBuildDoesNotPromoteMisleadingTargetName(t *testing.T) {
	t.Parallel()

	structural := structuralFixture()
	structural.Target.Entity.Name = "Validator.Validate"
	structural.Query = "Validator.Validate"
	structural.OutgoingCalls = nil
	card := sourceCardFixture()
	card.Target.Name = "Validator.Validate"
	card.Lines[0].Text = "func (v *Validator) Validate() bool {"
	if _, err := Build(structural, card); err == nil {
		t.Fatal("Build() created source questions from target name alone")
	}
}

func sourceBundleFixture(t *testing.T) Bundle {
	t.Helper()
	bundle, err := Build(structuralFixture(), sourceCardFixture())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return bundle
}

func misleadingBundleFixture(t *testing.T) Bundle {
	t.Helper()
	structural := structuralFixture()
	structural.OutgoingCalls = []symbol.CallFact{
		callFact("call-out-check", "checkInput", 91),
		callFact("call-out-error", "logError", 92),
	}
	card := sourceCardFixture()
	card.Lines[1].Text = "\tcheckInput()"
	card.Lines[2].Text = "\tlogError(err)"
	card.Window.IncludedBytes = includedBytes(card.Lines)
	bundle, err := Build(structural, card)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return bundle
}

func hasClaim(claims []Claim, predicate Predicate) bool {
	for _, claim := range claims {
		if claim.Predicate == predicate {
			return true
		}
	}
	return false
}

func hasUnknown(unknowns []Unknown, kind UnknownKind, anchor string) bool {
	for _, unknown := range unknowns {
		if unknown.Kind == kind && unknown.AnchorEvidenceID == anchor {
			return true
		}
	}
	return false
}

func hasParseWarning(warnings []ParseWarning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func countParseWarnings(warnings []ParseWarning, code string) int {
	count := 0
	for _, warning := range warnings {
		if warning.Code == code {
			count++
		}
	}
	return count
}

func containsFragment(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

const validSourceResponse = `{
  "assessments": [
    {
      "question_id": "question-call-out-002",
      "verdict": "shown",
      "source_evidence_ids": ["source-91"]
    },
    {
      "question_id": "question-call-out-004",
      "verdict": "shown",
      "source_evidence_ids": ["source-95"]
    },
    {
      "question_id": "question-call-out-003",
      "verdict": "shown",
      "source_evidence_ids": ["source-96", "source-97"]
    },
    {
      "question_id": "question-call-out-001",
      "verdict": "shown",
      "source_evidence_ids": ["source-100"]
    }
  ],
  "unknowns": [
    {"kind": "callee_behavior", "anchor_evidence_id": "call-out-004"},
    {"kind": "test_coverage", "anchor_evidence_id": "resolution-001"},
    {"kind": "runtime_reachability", "anchor_evidence_id": "resolution-001"}
  ],
  "next_action_id": "action-find-tests"
}`
