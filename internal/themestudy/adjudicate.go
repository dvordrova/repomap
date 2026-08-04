package themestudy

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidateAdjudication validates a Theme Adjudication response (contract E)
// item-locally against the Scout-accepted candidates (keyed by t* ref). Every
// theme's assessments must stay within its own candidate's anchor set; weak and
// irrelevant anchors never publish (the reducer clamps them deterministically,
// preserving relative order). Zero accepted themes is a semantic failure.
func ValidateAdjudication(data []byte, candidateByRef map[string]*ScoutCandidate) ([]AdjudicatedTheme, AdjudicationStatus, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: invalid JSON: %w", err)
	}
	if len(document) != 1 {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: unrequested top-level output %v", topLevelKeys(document))
	}
	raw, ok := document["themes"]
	if !ok {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: missing themes array")
	}
	var rawThemes []json.RawMessage
	if err := json.Unmarshal(raw, &rawThemes); err != nil {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: themes not an array: %w", err)
	}
	status := AdjudicationStatus{State: "accepted"}
	status.Received = len(rawThemes)
	accepted := make([]AdjudicatedTheme, 0, len(rawThemes))
	seen := make(map[string]struct{}, len(rawThemes))
	for position, rawTheme := range rawThemes {
		var theme AdjudicatedTheme
		if err := json.Unmarshal(rawTheme, &theme); err != nil {
			rejectAdj(&status, position, AdjIssueDecodeCandidate)
			continue
		}
		if code := adjThemeIssue(theme, candidateByRef, seen); code != "" {
			rejectAdj(&status, position, code)
			continue
		}
		seen[theme.CandidateRef] = struct{}{}
		if !adjHasDirect(theme) {
			rejectAdj(&status, position, AdjIssueNoDirect)
			continue
		}
		accepted = append(accepted, theme)
		status.Accepted++
	}
	status.Rejected = status.Received - status.Accepted
	switch {
	case status.Accepted == 0:
		status.State = "failed"
	case status.Rejected > 0:
		status.State = "accepted_partial"
	default:
		status.State = "accepted"
	}
	return accepted, status, nil
}

func adjThemeIssue(theme AdjudicatedTheme, candidateByRef map[string]*ScoutCandidate, seen map[string]struct{}) AdjudicationIssueCode {
	_, dup := seen[theme.CandidateRef]
	if dup {
		return AdjIssueDuplicateCandidateRef
	}
	candidate, ok := candidateByRef[theme.CandidateRef]
	if !ok {
		return AdjIssueUnknownCandidateRef
	}
	if strings.TrimSpace(theme.FinalTitle) == "" || strings.TrimSpace(theme.FinalQuestion) == "" {
		return AdjIssueEmptyFinalProse
	}
	if len(theme.AnchorAssessments) == 0 {
		return AdjIssueNoDirect
	}
	candidateAnchors := make(map[string]struct{}, len(candidate.AnchorRefs))
	for _, ref := range candidate.AnchorRefs {
		candidateAnchors[ref] = struct{}{}
	}
	seenAssess := make(map[string]struct{}, len(theme.AnchorAssessments))
	for _, assessment := range theme.AnchorAssessments {
		if !assessment.Fit.Valid() {
			return AdjIssueInvalidFit
		}
		if _, ok := candidateAnchors[assessment.AnchorRef]; !ok {
			return AdjIssueAnchorOutsideCandidate
		}
		if _, seen := seenAssess[assessment.AnchorRef]; seen {
			return AdjIssueDuplicateAssessment
		}
		seenAssess[assessment.AnchorRef] = struct{}{}
		if strings.TrimSpace(assessment.SupportedObservation) == "" {
			return AdjIssueEmptyObservation
		}
		if utf8.RuneCountInString(assessment.SupportedObservation) > MaxEditorialRunes {
			return AdjIssueEmptyObservation
		}
	}
	for _, ref := range theme.ReadingOrder {
		if _, ok := candidateAnchors[ref]; !ok {
			return AdjIssueUnknownReadingRef
		}
	}
	for _, unknown := range theme.Unknowns {
		if utf8.RuneCountInString(unknown) > MaxUnknownRunes {
			return AdjIssueEmptyObservation
		}
	}
	if len(theme.Unknowns) > MaxUnknownsPerTheme {
		return AdjIssueEmptyObservation
	}
	return ""
}

func adjHasDirect(theme AdjudicatedTheme) bool {
	for _, assessment := range theme.AnchorAssessments {
		if assessment.Fit == FitDirect {
			return true
		}
	}
	return false
}

func rejectAdj(status *AdjudicationStatus, position int, code AdjudicationIssueCode) {
	status.Rejected++
	status.Issues = append(status.Issues, AdjudicationIssue{Position: position, Code: code})
}
