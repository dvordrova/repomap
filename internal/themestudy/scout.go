package themestudy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// refKind classifies a typed short ref prefix (a*, f*, t*).
func refKind(ref string) string {
	for _, prefix := range []string{"a", "f", "t"} {
		if strings.HasPrefix(ref, prefix) {
			rest := strings.TrimPrefix(ref, prefix)
			if rest == "" {
				return "unknown"
			}
			for _, r := range rest {
				if r < '0' || r > '9' {
					return "unknown"
				}
			}
			if strings.Contains(ref, ".") {
				return "unknown"
			}
			return ref[:1]
		}
	}
	return "unknown"
}

func normalizeProse(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

// truncateRunes deterministically shortens value to at most limit whole
// runes, preserving Unicode boundaries. It never splits a rune and never
// appends markers: the caller records the normalization count so the
// truncation is visible in status, never silent.
func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

// ValidateScout validates a Theme Scout response (contract C) item-locally
// against the request-local catalog. anchorRefs and fileRefs are the advertised
// a*/f* ref sets; catalogDigest binds the request catalog (cross-request refs
// reject). One bad candidate never poisons the rest; only the offending
// candidate is rejected. A response with zero valid candidates is a semantic
// failure (failed state), never a locally fabricated shelf.
func ValidateScout(data []byte, anchorRefs map[string]struct{}, fileRefs map[string]struct{}, catalogDigest string) ([]ScoutCandidate, ScoutStatus, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, ScoutStatus{}, fmt.Errorf("theme scout: invalid JSON: %w", err)
	}
	if len(document) != 1 {
		return nil, ScoutStatus{}, fmt.Errorf("theme scout: unrequested top-level output %v", topLevelKeys(document))
	}
	raw, ok := document["themes"]
	if !ok {
		return nil, ScoutStatus{}, fmt.Errorf("theme scout: missing themes array")
	}
	var rawThemes []json.RawMessage
	if err := json.Unmarshal(raw, &rawThemes); err != nil {
		return nil, ScoutStatus{}, fmt.Errorf("theme scout: themes not an array: %w", err)
	}
	status := ScoutStatus{State: "accepted", Normalized: map[string]int{}}
	status.Received = len(rawThemes)
	accepted := make([]ScoutCandidate, 0, len(rawThemes))
	seenNormalized := make(map[string]struct{}, len(rawThemes))
	for position, rawTheme := range rawThemes {
		var candidate ScoutCandidate
		if err := json.Unmarshal(rawTheme, &candidate); err != nil {
			reject(&status, position, ScoutIssueDecodeCandidate)
			continue
		}
		if code := scoutCandidateIssue(candidate, anchorRefs, fileRefs, catalogDigest, seenNormalized); code != "" {
			reject(&status, position, code)
			continue
		}
		// Decision 224: bound overlong provisional prose deterministically
		// and record the truncation counts — never a silent editorial loss.
		normalized, counts := normalizeScoutCandidate(candidate)
		for field, count := range counts {
			status.Normalized[field] += count
		}
		seenNormalized[normalizeProse(normalized.Question)+"|"+normalizeProse(normalized.ExpectedLearning)] = struct{}{}
		accepted = append(accepted, normalized)
		status.Accepted++
	}
	status.Rejected = status.Received - status.Accepted
	if len(status.Normalized) == 0 {
		// Keep the artifact canonical: an empty Normalized map with omitempty
		// would decode as nil and break encode/decode DeepEqual.
		status.Normalized = nil
	}
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

func scoutCandidateIssue(candidate ScoutCandidate, anchorRefs, fileRefs map[string]struct{}, catalogDigest string, seenNormalized map[string]struct{}) ScoutIssueCode {
	if strings.TrimSpace(candidate.Title) == "" || strings.TrimSpace(candidate.Question) == "" ||
		strings.TrimSpace(candidate.WhyItMatters) == "" || strings.TrimSpace(candidate.ExpectedLearning) == "" {
		return ScoutIssueEmptyProse
	}
	// Decision 224 (resuming D219): overlong provisional prose is NOT a
	// rejection — Adjudication may rewrite title/question, and a long
	// provisional why/expected must not erase an otherwise valid
	// source-referenced candidate. The caller normalizes deterministically
	// and records typed counts. Only schema/identity/ref/kind/cardinality
	// failures remain hard rejections below.
	if !candidate.ThemeKind.Valid() {
		return ScoutIssueInvalidThemeKind
	}
	if !candidate.RelationClaim.Valid() {
		return ScoutIssueInvalidRelationClaim
	}
	if len(candidate.AnchorRefs) < MinThemeAnchors || len(candidate.AnchorRefs) > MaxThemeAnchors {
		return ScoutIssueInvalidAnchorCount
	}
	seenAnchor := make(map[string]struct{}, len(candidate.AnchorRefs))
	for _, ref := range candidate.AnchorRefs {
		if refKind(ref) != "a" {
			return ScoutIssueWrongKindRef
		}
		if _, ok := anchorRefs[ref]; !ok {
			return ScoutIssueUnknownRef
		}
		if _, dup := seenAnchor[ref]; dup {
			return ScoutIssueDuplicateRef
		}
		seenAnchor[ref] = struct{}{}
	}
	for _, ref := range candidate.ExpansionFileRefs {
		if refKind(ref) != "f" {
			return ScoutIssueWrongKindRef
		}
		if _, ok := fileRefs[ref]; !ok {
			return ScoutIssueUnknownRef
		}
	}
	key := normalizeProse(candidate.Question) + "|" + normalizeProse(candidate.ExpectedLearning)
	if _, ok := seenNormalized[key]; ok {
		return ScoutIssueDuplicateCandidate
	}
	return ""
}

// normalizeScoutCandidate bounds every editorial field to its closed limit
// deterministically (whole-rune truncation, Decision 224). It returns the
// normalized candidate plus typed counts of truncated fields so the status
// records every normalization — never a silent truncation.
func normalizeScoutCandidate(candidate ScoutCandidate) (ScoutCandidate, map[string]int) {
	normalized := candidate
	counts := map[string]int{}
	if utf8.RuneCountInString(normalized.Title) > MaxTitleRunes {
		normalized.Title = truncateRunes(normalized.Title, MaxTitleRunes)
		counts["title"]++
	}
	if utf8.RuneCountInString(normalized.Question) > MaxQuestionRunes {
		normalized.Question = truncateRunes(normalized.Question, MaxQuestionRunes)
		counts["question"]++
	}
	if utf8.RuneCountInString(normalized.WhyItMatters) > MaxEditorialRunes {
		normalized.WhyItMatters = truncateRunes(normalized.WhyItMatters, MaxEditorialRunes)
		counts["why_it_matters"]++
	}
	if utf8.RuneCountInString(normalized.ExpectedLearning) > MaxEditorialRunes {
		normalized.ExpectedLearning = truncateRunes(normalized.ExpectedLearning, MaxEditorialRunes)
		counts["expected_learning"]++
	}
	return normalized, counts
}

func reject(status *ScoutStatus, position int, code ScoutIssueCode) {
	status.Rejected++
	status.Issues = append(status.Issues, ScoutIssue{Position: position, Code: code})
}

func topLevelKeys(document map[string]json.RawMessage) []string {
	out := make([]string, 0, len(document))
	for key := range document {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
