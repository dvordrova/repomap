package themestudy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	Cards   []ThemeCard `json:"cards"`
	Omitted int         `json:"omitted"`
	// CoProjected (Decision 233 / Archive 12 P0): how many accepted themes
	// folded into an earlier card as alternates (identical reading set or
	// identical normalized prose) instead of publishing a duplicate card.
	CoProjected int            `json:"co_projected,omitempty"`
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
	// Decision 233: semantic-equivalent co-projected themes retain their
	// title/question/readings as alternates on the representative card.
	alternateTitles    []string
	alternateQuestions []string
	alternateReadings  []Reading
}

// coProjectTheme folds a semantic-equivalent theme into the representative
// workTheme (Decision 233): the alternate title/question are retained as
// provenance and every distinct published reading appends (deduplicated by
// exact public identity, bounded by the published-set limits).
func coProjectTheme(target *workTheme, theme AdjudicatedTheme, entries []publishedEntry, anchors map[string]AnchorInfo) {
	target.alternateTitles = append(target.alternateTitles, theme.FinalTitle)
	target.alternateQuestions = append(target.alternateQuestions, theme.FinalQuestion)
	seenIdentity := make(map[string]struct{}, len(target.alternateReadings))
	for _, existing := range target.alternateReadings {
		seenIdentity[readingPublicIdentity(existing)] = struct{}{}
	}
	for _, existing := range target.entries {
		seenIdentity[readingPublicIdentity(existing.reading)] = struct{}{}
	}
	for _, entry := range entries {
		identity := readingPublicIdentity(entry.reading)
		if _, dup := seenIdentity[identity]; dup {
			continue
		}
		seenIdentity[identity] = struct{}{}
		target.alternateReadings = append(target.alternateReadings, entry.reading)
	}
}

func readingPublicIdentity(reading Reading) string {
	return fmt.Sprintf("%s\x00%d\x00%s", reading.Path, reading.Line, reading.Symbol)
}

// anchorFamily derives a repository-independent source family from an
// anchor's exact path: the top-level directory ("" for a root path). This
// is a generic lexical rule — no hard-coded TLS/logging/config keyword
// anywhere — so any overrepresented family (TLS/certificates, logging,
// metrics, config, serialization, tests, release tooling, ...) triggers the
// same concentration control.
func anchorFamily(info AnchorInfo) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(info.Path, "/"), "./")
	first := trimmed
	if index := strings.IndexByte(trimmed, '/'); index > 0 {
		first = trimmed[:index]
	}
	// Long-horizon program Phase 3 (Miniflux live run): Go's `internal/`
	// is a structural wrapper, not a source family — a shelf whose first
	// reading always sits under internal/ (every Go repository) would
	// hide real family concentration (config, tls, storage) behind a
	// trivial 100% `internal` bucket. Skip the structural prefix and use
	// the first domain-bearing segment. Generic rule: only `internal`
	// (and `pkg` — another Go structural convention) is skipped; any
	// other first segment is the family as before.
	if first == "internal" || first == "pkg" {
		rest := trimmed[len(first)+1:]
		if index := strings.IndexByte(rest, '/'); index > 0 {
			return rest[:index]
		}
		if rest != "" {
			return rest
		}
	}
	return first
}

// portfolioConcentration counts accepted themes by source family and
// returns the dominating family when one family holds more than half of the
// principal shelf, at least two themes share it, AND other families hold
// exact evidence (F7, fresh review: a homogeneous single-family repository
// is not "dominated" — it has no alternative evidence to rank behind). It
// never deletes or re-ranks; the Study status publishes the exact
// before/after counts.
func portfolioConcentration(all []workTheme) (family string, count, total int) {
	total = len(all)
	if total < 2 {
		return "", 0, total
	}
	counts := make(map[string]int, total)
	for _, theme := range all {
		// The family of a theme is the family of its first published
		// reading (stable: entries are ordered by the adjudicator's
		// reading order, then canonical order).
		if len(theme.entries) == 0 {
			continue
		}
		counts[anchorFamily(AnchorInfo{
			Path:   theme.entries[0].reading.Path,
			Line:   theme.entries[0].reading.Line,
			Symbol: theme.entries[0].reading.Symbol,
		})]++
	}
	if len(counts) < 2 {
		return "", 0, total
	}
	for candidate, n := range counts {
		if n*2 > total && n >= 2 {
			return candidate, n, total
		}
	}
	return "", 0, total
}

// applyConcentration marks every card whose first-reading family matches the
// dominating family with an exact-count marker and records the diagnostic
// counts. It never deletes or re-ranks (Decision 233).
func applyConcentration(cards []ThemeCard, all []workTheme, diagnostics map[string]int) {
	family, count, total := portfolioConcentration(all)
	if family == "" {
		return
	}
	for index, w := range all {
		if len(w.entries) == 0 {
			continue
		}
		themeFamily := anchorFamily(AnchorInfo{
			Path:   w.entries[0].reading.Path,
			Line:   w.entries[0].reading.Line,
			Symbol: w.entries[0].reading.Symbol,
		})
		if themeFamily != family {
			continue
		}
		cards[index].ConcentrationDiagnostic = fmt.Sprintf(
			"%s:%d/%d", family, count, total)
	}
	diagnostics["concentrated_family"] = count
	diagnostics["portfolio_total"] = total
	diagnostics["concentration_other"] = total - count
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
	seenCanonical := make(map[string]int) // canonicalID -> index into all
	seenNormal := make(map[string]int)    // normalKey -> index into all
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
		canonicalID := themeIdentity(entries, candidate.ThemeKind)
		if earlier, dup := seenCanonical[canonicalID]; dup {
			// Decision 233 + Archive 12 P0: a theme whose published reading
			// set is EXACTLY identical to an earlier card (even through
			// different anchor refs) is the same study card — it CO-PROJECTS
			// as alternate provenance (title/question/readings) instead of
			// being dropped or publishing a duplicate card.
			coProjectTheme(&all[earlier], theme, entries, input.Anchors)
			reduction.CoProjected++
			continue
		}
		normalKey := normalizeProse(theme.FinalQuestion) + "|" + normalizeProse(theme.FinalTitle)
		if earlier, dup := seenNormal[normalKey]; dup {
			// Decision 233 (Archive 9): a semantic-equivalent theme
			// CO-PROJECTS into the earlier card instead of being dropped.
			// Its title/question become alternate provenance and its
			// distinct readings append (bounded, deduplicated by exact
			// public identity).
			coProjectTheme(&all[earlier], theme, entries, input.Anchors)
			reduction.CoProjected++
			continue
		}
		seenCanonical[canonicalID] = len(all)
		seenNormal[normalKey] = len(all)
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

	// Ordinal order: the model's comparative editorial ranking survives
	// (Phase 2 prompt cleanup: Scout returns themes ordered by decreasing
	// usefulness, and the backend preserves that order). theme_kind is
	// presentation metadata only — it is unstable model classification,
	// not evidence identity, and never decides default prominence. The
	// slice is already in model order (co-projection folds into the
	// earlier card in place; balance cap preserves order); no re-rank.
	// Display order only; canonical identity never changes.

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
		// Decision 233 (F5, fresh review): co-projected alternate readings
		// are part of the visible card — a supporting/weak alternate makes
		// the badge partial (the badge must match the final visible
		// promise, never overstate).
		alternateHasSupportingOrUnknown := false
		for _, alternate := range w.alternateReadings {
			if alternate.Fit != FitDirect {
				alternateHasSupportingOrUnknown = true
				break
			}
		}
		if supporting > 0 || len(w.theme.Unknowns) > 0 || direct < len(w.entries) || alternateHasSupportingOrUnknown {
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
			WhyItMatters:     w.theme.WhyItMatters,
			ExpectedLearning: w.theme.ExpectedLearning,
			Readings:         readings,
			Badge:            badge,
			DirectCount:      direct,
			SupportingCount:  supporting,
			// Phase 8 reviewer finding: unknowns are the honest caveats of
			// the retained readings — they must reach the report card, not
			// only drive the partial badge. ReviewedAnchorCount is the
			// exact denominator for the per-card limitation line.
			Unknowns:            append([]string(nil), w.theme.Unknowns...),
			ReviewedAnchorCount: len(w.theme.AnchorAssessments),
			// Decision 233: co-projected semantic equivalents are retained
			// as alternate provenance (titles/questions/readings) — nothing
			// silently vanishes.
			AlternateTitles:    w.alternateTitles,
			AlternateQuestions: w.alternateQuestions,
			AlternateReadings:  w.alternateReadings,
		})
	}
	reduction.Diagnostics["published"] = len(reduction.Cards)
	applyConcentration(reduction.Cards, all, reduction.Diagnostics)
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

// themeIdentity derives the canonical identity of a published theme from the
// EXACT public reading identities (path,line,symbol) plus kind — never from
// anchor refs. Two themes whose readings resolve to the same exact source set
// (even through different anchor refs) are the SAME study card: Archive 12 P0
// (Telebot 3 pairs, Casdoor 2 cards with identical exact reading sets) showed
// ref-based identity let duplicate reading sets publish as separate cards.
func themeIdentity(entries []publishedEntry, kind ThemeKind) string {
	identities := make([]string, 0, len(entries))
	for _, entry := range entries {
		identities = append(identities, readingPublicIdentity(entry.reading))
	}
	sort.Strings(identities)
	// Long-horizon program Phase 3 (syn live run): identity is the EXACT
	// public reading set only — kind is model classification, never
	// evidence identity. Four accepted themes over the same exact reading
	// (different kind labels from the model) are one study card with the
	// extras co-projected as alternate provenance.
	payload, _ := json.Marshal(map[string]any{"readings": identities})
	hash := sha256.Sum256(payload)
	return "theme-" + hex.EncodeToString(hash[:])[:24]
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
