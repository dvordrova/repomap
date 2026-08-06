package themestudy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// AnchorInfo is the exact backend-owned identity for one a* anchor used to
// build a Reading (never prose). CanonicalSpanID, when non-empty, binds the
// anchor to the canonical route span it was compiled from so the re-based
// browse can derive the published stage from study_themes.v1.json readings.
type AnchorInfo struct {
	Path            string `json:"path"`
	Symbol          string `json:"symbol"`
	Line            int    `json:"line"`
	CanonicalSpanID string `json:"canonical_span_id,omitempty"`
}

// ReducerInput is the exact input to the deterministic reducer: the accepted
// adjudicated themes, the Scout candidates (for theme_kind), and the exact
// anchor identities from the compiled local substrate.
type ReducerInput struct {
	Themes     []AdjudicatedTheme
	Candidates map[string]*ScoutCandidate
	Anchors    map[string]AnchorInfo
}

// Reduction is the reduced, published Study-card portfolio.
type Reduction struct {
	Cards       []ThemeCard    `json:"cards"`
	Omitted     int            `json:"omitted"`
	Partial     bool           `json:"partial"`
	Diagnostics map[string]int `json:"diagnostics,omitempty"`
}

// published entry keeps the exact anchor ref with its Reading and fit so the
// balance cap can remove anchors while preserving ≥ 1 direct.
type publishedEntry struct {
	ref     string
	reading Reading
	fit     FitClass
}

// workTheme carries one adjudicated theme through the publish pipeline.
type workTheme struct {
	theme       AdjudicatedTheme
	kind        ThemeKind
	entries     []publishedEntry
	canonicalID string
	normalKey   string
}

// themeKindPortfolioRank orders theme kinds deterministically for the
// default shelf: production/user-facing concerns first, peripheral
// integration families last (Decision 224 / D219 F). It is a closed local
// contract — no repository-specific keyword table — and display-only;
// canonical identity never changes.
func themeKindPortfolioRank(kind ThemeKind) int {
	switch kind {
	case KindUserJourney, KindLifecycleConcern:
		return 0
	case KindSharedDomainResponsibility, KindCrossCuttingPolicy:
		return 1
	case KindSiblingImplementationFamily:
		return 2
	case KindIntegrationFamily:
		return 3
	default:
		return 4
	}
}

// Reduce publishes the final Study cards deterministically (contract F). It
// never re-ranks model output, never retries, and never lets model prose become
// identity, relation, or acceptance. Published cards carry editorial prose +
// exact readings + a badge and zero source bytes.
func Reduce(input ReducerInput) (Reduction, error) {
	reduction := Reduction{Diagnostics: map[string]int{}}
	if len(input.Candidates) == 0 || len(input.Anchors) == 0 {
		return reduction, fmt.Errorf("reducer requires candidates and anchor identities")
	}

	var all []workTheme
	seenCanonical := make(map[string]struct{})
	seenNormal := make(map[string]struct{})
	for _, theme := range input.Themes {
		candidate, ok := input.Candidates[theme.CandidateRef]
		if !ok {
			reduction.Omitted++
			continue
		}
		entries := publishEntries(theme, input.Anchors)
		if len(entries) == 0 || directIn(entries) == 0 {
			reduction.Omitted++
			continue
		}
		canonicalID := themeIdentity(publishedRefs(entries), candidate.ThemeKind)
		if _, dup := seenCanonical[canonicalID]; dup {
			reduction.Omitted++
			continue
		}
		normalKey := normalizeProse(theme.FinalQuestion) + "|" + normalizeProse(theme.FinalTitle)
		if _, dup := seenNormal[normalKey]; dup {
			reduction.Omitted++
			continue
		}
		seenCanonical[canonicalID] = struct{}{}
		seenNormal[normalKey] = struct{}{}
		all = append(all, workTheme{theme: theme, kind: candidate.ThemeKind, entries: entries, canonicalID: canonicalID, normalKey: normalKey})
	}

	if len(all) == 0 {
		reduction.Partial = true
		return reduction, nil
	}

	// Balance cap (catalog-relative): applies only when the accepted catalog has
	// enough distinct alternatives. Anchor-first removal keeps ≥ 1 direct +
	// answerable, else the theme is dropped with the honest count.
	if len(input.Anchors) >= MaxThemeAnchors {
		all = applyBalanceCap(all, &reduction)
	}

	// Ordinal order: deterministic portfolio rank (Decision 224 / D219 F) —
	// production/user-facing kinds before peripheral integration families —
	// with canonical identity as the stable locale-independent tiebreak.
	// Display order only; canonical identity never changes.
	sort.SliceStable(all, func(i, j int) bool {
		leftRank := themeKindPortfolioRank(all[i].kind)
		rightRank := themeKindPortfolioRank(all[j].kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return all[i].canonicalID < all[j].canonicalID
	})

	for ordinal, w := range all {
		direct, supporting := 0, 0
		for _, e := range w.entries {
			if e.fit == FitDirect {
				direct++
			} else {
				supporting++
			}
		}
		// Decision 224 (D219 D): the badge must match the final visible
		// promise. Full support only when every published reading is direct
		// AND the theme carries no unresolved unknowns that materially
		// qualify the question. Any supporting-only facet, any unknown, or
		// a narrowed reading set makes the badge partial.
		badge := "editorial_source_backed"
		if supporting > 0 || len(w.theme.Unknowns) > 0 || direct < len(w.entries) {
			badge = "partial"
		}
		readings := make([]Reading, 0, len(w.entries))
		for _, e := range w.entries {
			readings = append(readings, e.reading)
		}
		reduction.Cards = append(reduction.Cards, ThemeCard{
			Ordinal:          ordinal + 1,
			CanonicalID:      w.canonicalID,
			ThemeKind:        w.kind,
			FinalTitle:       w.theme.FinalTitle,
			FinalQuestion:    w.theme.FinalQuestion,
			WhyItMatters:     candidateProse(w.theme.CandidateRef, input.Candidates, 0),
			ExpectedLearning: candidateProse(w.theme.CandidateRef, input.Candidates, 1),
			Readings:         readings,
			Badge:            badge,
			DirectCount:      direct,
			SupportingCount:  supporting,
		})
	}
	reduction.Diagnostics["published"] = len(reduction.Cards)
	if reduction.Omitted > 0 {
		reduction.Partial = true
	}
	return reduction, nil
}

// publishEntries applies the publish filter (direct + supporting only) and
// orders readings by the adjudicator's reading order, then any remaining
// published anchors in canonical order. Weak and irrelevant anchors never
// appear as readings. Every reading ref must resolve to an exact anchor.
// Decision 224: readings deduplicate by exact public identity
// (path,line,symbol), so two anchors resolving to the same exact source
// never produce a repeated reading.
func publishEntries(theme AdjudicatedTheme, anchors map[string]AnchorInfo) []publishedEntry {
	fitByRef := make(map[string]FitClass, len(theme.AnchorAssessments))
	observationByRef := make(map[string]string, len(theme.AnchorAssessments))
	for _, assessment := range theme.AnchorAssessments {
		fitByRef[assessment.AnchorRef] = assessment.Fit
		observationByRef[assessment.AnchorRef] = assessment.SupportedObservation
	}
	var ordered []string
	seen := make(map[string]struct{}, len(theme.AnchorAssessments))
	seenIdentity := make(map[string]string, len(theme.AnchorAssessments)) // identity -> winning ref
	add := func(ref string) {
		if _, ok := seen[ref]; ok {
			return
		}
		fit, ok := fitByRef[ref]
		if !ok || (fit != FitDirect && fit != FitSupporting) {
			return
		}
		info, ok := anchors[ref]
		if !ok {
			return
		}
		identity := fmt.Sprintf("%s\x00%d\x00%s", info.Path, info.Line, info.Symbol)
		if winner, dup := seenIdentity[identity]; dup {
			// Two anchors share the exact public identity but differ in
			// fit. A direct reading must never be dropped in favor of a
			// supporting one (D219 E / D224 C: preserve at least one
			// direct reading): swap the winner when the newcomer is
			// direct and the incumbent is supporting. Both anchors keep
			// their exact references; the identity publishes once.
			winnerFit := fitByRef[winner]
			if fit == FitDirect && winnerFit == FitSupporting {
				delete(seen, winner)
				seen[ref] = struct{}{}
				seenIdentity[identity] = ref
				for index, existing := range ordered {
					if existing == winner {
						ordered[index] = ref
						break
					}
				}
			}
			return
		}
		seen[ref] = struct{}{}
		seenIdentity[identity] = ref
		ordered = append(ordered, ref)
	}
	for _, ref := range theme.ReadingOrder {
		add(ref)
	}
	var rest []string
	for ref, fit := range fitByRef {
		if fit == FitDirect || fit == FitSupporting {
			rest = append(rest, ref)
		}
	}
	sort.Strings(rest)
	for _, ref := range rest {
		add(ref)
	}
	entries := make([]publishedEntry, 0, len(ordered))
	for _, ref := range ordered {
		info := anchors[ref]
		entries = append(entries, publishedEntry{
			ref: ref,
			reading: Reading{
				Label: info.Symbol, Symbol: info.Symbol, Path: info.Path, Line: info.Line,
				SupportedObservation: observationByRef[ref], Fit: fitByRef[ref],
				CanonicalSpanID: info.CanonicalSpanID,
			},
			fit: fitByRef[ref],
		})
	}
	return entries
}

func directIn(entries []publishedEntry) int {
	n := 0
	for _, e := range entries {
		if e.fit == FitDirect {
			n++
		}
	}
	return n
}

func publishedRefs(entries []publishedEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ref)
	}
	return out
}

// themeIdentity derives the canonical id from accepted exact refs + local
// contract data (theme_kind), never model prose.
func themeIdentity(anchorRefs []string, kind ThemeKind) string {
	sorted := append([]string(nil), anchorRefs...)
	sort.Strings(sorted)
	payload, _ := json.Marshal(map[string]any{"anchors": sorted, "kind": string(kind)})
	hash := sha256.Sum256(payload)
	return "theme-" + hex.EncodeToString(hash[:])[:24]
}

func candidateProse(candidateRef string, candidates map[string]*ScoutCandidate, which int) string {
	c, ok := candidates[candidateRef]
	if !ok {
		return ""
	}
	if which == 0 {
		return c.WhyItMatters
	}
	return c.ExpectedLearning
}

// applyBalanceCap enforces the catalog-relative guard that no one anchor
// appears in more than half the final themes. It removes an offending anchor
// from a theme (anchor-first) while keeping ≥ 1 direct + answerable, else drops
// the theme with the honest count. Deterministic and never a hidden re-rank.
func applyBalanceCap(all []workTheme, reduction *Reduction) []workTheme {
	for consensus := true; consensus; {
		consensus = false
		capFloor := len(all) / 2
		counts := make(map[string]int, 8)
		for _, w := range all {
			for _, e := range w.entries {
				counts[e.ref]++
			}
		}
		for ref, count := range counts {
			if count <= capFloor {
				continue
			}
			var surviving []workTheme
			for _, w := range all {
				if !containsRef(w.entries, ref) {
					surviving = append(surviving, w)
					continue
				}
				kept := removeRef(w.entries, ref)
				if len(kept) == 0 || directIn(kept) == 0 {
					reduction.Omitted++
					consensus = true
					continue
				}
				w.entries = kept
				surviving = append(surviving, w)
				consensus = true
			}
			all = surviving
		}
	}
	return all
}

func containsRef(entries []publishedEntry, ref string) bool {
	for _, e := range entries {
		if e.ref == ref {
			return true
		}
	}
	return false
}

func removeRef(entries []publishedEntry, ref string) []publishedEntry {
	out := make([]publishedEntry, 0, len(entries))
	for _, e := range entries {
		if e.ref == ref {
			continue
		}
		out = append(out, e)
	}
	return out
}
