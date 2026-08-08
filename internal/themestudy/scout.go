package themestudy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
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
// runes. It keeps the largest complete-word prefix that fits with an explicit
// ellipsis when Unicode whitespace supplies a word boundary; text without
// whitespace keeps the largest rune-safe prefix. The caller records the
// normalization count, so the bounded loss remains explicit in status.
func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit == 1 {
		return "…"
	}
	prefix := runes[:limit-1]
	wordEnd := len(prefix)
	if !unicode.IsSpace(runes[limit-1]) {
		for index := len(prefix) - 1; index >= 0; index-- {
			if unicode.IsSpace(prefix[index]) {
				wordEnd = index
				break
			}
		}
	}
	result := strings.TrimSpace(string(prefix[:wordEnd]))
	if result == "" {
		result = string(prefix)
	}
	// A one- or two-rune token at the artificial boundary is usually a
	// dangling connector (for example Russian "и" or English "to"). Rolling
	// it back keeps the explicit ellipsis honest without publishing visibly
	// unfinished copy. Longer technical tokens remain untouched.
	if lastSpace := strings.LastIndexFunc(result, unicode.IsSpace); lastSpace >= 0 {
		lastToken := strings.TrimSpace(result[lastSpace+1:])
		if utf8.RuneCountInString(lastToken) <= 2 {
			result = strings.TrimSpace(result[:lastSpace])
		}
	}
	return result + "…"
}

// dedupeRefs keeps the first occurrence of each exact ref and returns the
// deduplicated slice plus the count of removed duplicates (Decision 232).
func dedupeRefs(refs []string) ([]string, int) {
	if len(refs) < 2 {
		return append([]string(nil), refs...), 0
	}
	seen := make(map[string]struct{}, len(refs))
	result := make([]string, 0, len(refs))
	duplicates := 0
	for _, ref := range refs {
		if _, ok := seen[ref]; ok {
			duplicates++
			continue
		}
		seen[ref] = struct{}{}
		result = append(result, ref)
	}
	return result, duplicates
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
		// Decision 232 (Archive 9): exact duplicate anchor/file refs are
		// normalized deterministically (keep first occurrence) and
		// counted — they never reject a valid candidate.
		dedupedAnchors, duplicateAnchors := dedupeRefs(candidate.AnchorRefs)
		dedupedFiles, duplicateFiles := dedupeRefs(candidate.ExpansionFileRefs)
		if duplicateAnchors > 0 {
			status.Normalized["duplicate_anchor_refs"] += duplicateAnchors
		}
		if duplicateFiles > 0 {
			status.Normalized["duplicate_file_refs"] += duplicateFiles
		}
		candidate.AnchorRefs = dedupedAnchors
		candidate.ExpansionFileRefs = dedupedFiles
		// Decision 239: validate every returned identity before normalization,
		// then keep a bounded first set instead of rejecting the complete useful
		// theme for an editorial cardinality excess. Unknown/wrong-kind refs in
		// the omitted suffix still fail closed; only known exact refs normalize.
		if code := scoutCandidateReferenceIssue(candidate, anchorRefs, fileRefs); code != "" {
			reject(&status, position, code)
			continue
		}
		if len(candidate.AnchorRefs) > MaxThemeAnchors {
			status.Normalized["anchor_refs_capped"] += len(candidate.AnchorRefs) - MaxThemeAnchors
			candidate.AnchorRefs = candidate.AnchorRefs[:MaxThemeAnchors]
		}
		// Phase 3 validation audit: relation_claim is backend-owned — the
		// design rule says a model may never create runtime facts, so its
		// value is ALWAYS editorial_only and we assign it ourselves. The
		// model is not asked to echo it and a stray value in the
		// unrequested field never rejects the theme.
		candidate.RelationClaim = RelationClaimEditorialOnly
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
	// and records typed counts. Theme kind is presentation metadata and is
	// normalized by the caller; schema/identity/ref/cardinality failures remain
	// hard rejections below.
	if len(candidate.AnchorRefs) < MinThemeAnchors || len(candidate.AnchorRefs) > MaxThemeAnchors {
		return ScoutIssueInvalidAnchorCount
	}
	if code := scoutCandidateReferenceIssue(candidate, anchorRefs, fileRefs); code != "" {
		return code
	}
	key := normalizeProse(candidate.Question) + "|" + normalizeProse(candidate.ExpectedLearning)
	if _, ok := seenNormalized[key]; ok {
		return ScoutIssueDuplicateCandidate
	}
	return ""
}

func scoutCandidateReferenceIssue(
	candidate ScoutCandidate,
	anchorRefs, fileRefs map[string]struct{},
) ScoutIssueCode {
	for _, ref := range candidate.AnchorRefs {
		if refKind(ref) != "a" {
			return ScoutIssueWrongKindRef
		}
		if _, ok := anchorRefs[ref]; !ok {
			return ScoutIssueUnknownRef
		}
	}
	for _, ref := range candidate.ExpansionFileRefs {
		if refKind(ref) != "f" {
			return ScoutIssueWrongKindRef
		}
		if _, ok := fileRefs[ref]; !ok {
			return ScoutIssueUnknownRef
		}
	}
	return ""
}

// normalizeScoutCandidate normalizes presentation-only metadata and bounds
// every editorial field to its closed limit deterministically (readable
// word-boundary truncation, Decision 224/D241). It returns typed
// per-field counts so the status records every normalization — never a silent
// rewrite.
func normalizeScoutCandidate(candidate ScoutCandidate) (ScoutCandidate, map[string]int) {
	normalized := candidate
	counts := map[string]int{}
	// Decision 239: theme_kind is presentation metadata, not evidence identity
	// or authority. Preserve an otherwise source-backed candidate by mapping a
	// missing or provider-invented kind to the neutral closed kind.
	if !normalized.ThemeKind.Valid() {
		normalized.ThemeKind = KindSharedDomainResponsibility
		counts["theme_kind"]++
	}
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
