package themestudy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// MockScoutResponse builds one bounded, provider-free Theme Scout response for
// an exact request record. It groups advertised a* seed anchors into
// meaningfully distinct candidate themes (by role and seed provenance),
// emits desired 8-12 candidates with valid a*/f* refs, and passes item-local
// validation so the replay status is accepted. It never references provider
// prose, canonical identities or raw paths: it is a deterministic fixture for
// offline report rendering and replay tests, and it never invokes a provider.
func MockScoutResponse(request ScoutRequest) ([]byte, error) {
	anchorRefs := request.AnchorRefs()
	fileRefs := request.FileRefs()
	packs := request.SeedPacks.Packs
	if len(packs) == 0 {
		return nil, fmt.Errorf("theme scout mock response: no seed packs")
	}
	// Group seeds by their backend-owned provenance so the fixture proposes
	// thematic groupings instead of one-anchor direct-call cards.
	byProvenance := make(map[string][]string)
	var provenanceOrder []string
	seedRole := make(map[string]Role, len(packs))
	for _, pack := range packs {
		seed := pack.Seed
		seedRole[seed.Ref] = seed.Role
		provenance := seed.Provenance
		if provenance == "" {
			provenance = "exact_source_anchor"
		}
		if _, ok := byProvenance[provenance]; !ok {
			provenanceOrder = append(provenanceOrder, provenance)
		}
		byProvenance[provenance] = append(byProvenance[provenance], seed.Ref)
	}
	sort.Strings(provenanceOrder)
	for _, refs := range byProvenance {
		sort.Strings(refs)
	}

	// Advertised f* refs in canonical order for expansion requests.
	fileRefList := make([]string, 0, len(fileRefs))
	for ref := range fileRefs {
		fileRefList = append(fileRefList, ref)
	}
	sort.Strings(fileRefList)

	var candidates []ScoutCandidate
	usedQuestions := make(map[string]struct{})
	usedKinds := make(map[ThemeKind]struct{})
	kinds := []ThemeKind{
		KindUserJourney, KindCrossCuttingPolicy, KindSiblingImplementationFamily,
		KindIntegrationFamily, KindLifecycleConcern, KindSharedDomainResponsibility,
	}
	kindIndex := 0
	nextKind := func() ThemeKind {
		for attempt := 0; attempt < len(kinds); attempt++ {
			kind := kinds[kindIndex%len(kinds)]
			kindIndex++
			if _, dup := usedKinds[kind]; !dup {
				usedKinds[kind] = struct{}{}
				return kind
			}
		}
		return kinds[(kindIndex)%len(kinds)]
	}
	addCandidate := func(kind ThemeKind, group []string, focused bool) {
		question := fmt.Sprintf("How do the exact anchors %s work together in this repository?", strings.Join(group, ", "))
		if _, dup := usedQuestions[normalizeProse(question)]; dup {
			question = fmt.Sprintf("What responsibilities do anchors %s share?", strings.Join(group, ", "))
		}
		usedQuestions[normalizeProse(question)] = struct{}{}
		candidate := ScoutCandidate{
			Title:            fmt.Sprintf("%s over %s", themeKindTitle(kind), provenanceLabel("exact source anchors")),
			Question:         question,
			ThemeKind:        kind,
			AnchorRefs:       append([]string(nil), group...),
			WhyItMatters:     "The accepted anchors participate in one bounded editorial responsibility that a reader can inspect together.",
			ExpectedLearning: "The reader can locate the exact anchors and read their bounded source to answer the question.",
			RelationClaim:    RelationClaimEditorialOnly,
			Focused:          focused,
		}
		if len(fileRefList) > 0 {
			candidate.ExpansionFileRefs = []string{fileRefList[len(candidates)%len(fileRefList)]}
		}
		candidates = append(candidates, candidate)
	}
	for _, provenance := range provenanceOrder {
		refs := byProvenance[provenance]
		// Desired 2-5 anchors per theme; one-anchor focused candidates are
		// permitted but must not dominate.
		for start := 0; start < len(refs); start += 3 {
			end := start + 3
			if end > len(refs) {
				end = len(refs)
			}
			group := refs[start:end]
			if len(group) == 1 {
				addCandidate(nextKind(), group, true)
			} else {
				addCandidate(nextKind(), group, false)
			}
		}
	}
	// Reach the desired 8-12 range on small fixtures by composing distinct
	// cross-provenance groupings (themes may share anchors; the reducer
	// dedupes). Deterministic sliding pairs over the canonical anchor order.
	if len(candidates) < DesiredScoutMin {
		ordered := make([]string, 0, len(anchorRefs))
		for ref := range anchorRefs {
			ordered = append(ordered, ref)
		}
		sort.Strings(ordered)
		for start := 0; len(candidates) < DesiredScoutMin && start < len(ordered); start++ {
			group := []string{ordered[start], ordered[(start+1)%len(ordered)]}
			if group[0] == group[1] {
				continue
			}
			addCandidate(nextKind(), group, false)
		}
	}
	if len(candidates) > MaxScoutCandidates {
		candidates = candidates[:MaxScoutCandidates]
	}
	if len(candidates) < MinScoutCandidates {
		return nil, fmt.Errorf("theme scout mock response: no buildable candidates")
	}
	raw, err := json.Marshal(map[string][]ScoutCandidate{"themes": candidates})
	if err != nil {
		return nil, fmt.Errorf("theme scout mock response: encode: %w", err)
	}
	// Confirm the fixture is canonical and accepted by the validator.
	if _, _, err := ValidateScout(raw, anchorRefs, fileRefs, request.CatalogSHA256); err != nil {
		return nil, fmt.Errorf("theme scout mock response: not valid: %w", err)
	}
	return raw, nil
}

func themeKindTitle(kind ThemeKind) string {
	return strings.ReplaceAll(string(kind), "_", " ")
}

func provenanceLabel(provenance string) string {
	switch provenance {
	case "d211_span_reading_target":
		return "span reading targets"
	case "architecture_behavior_anchor":
		return "architecture behavior anchors"
	case "surface":
		return "exact surfaces"
	case "accepted_document":
		return "accepted documents"
	default:
		return "exact source anchors"
	}
}

// MockAdjudicationResponse builds one bounded, provider-free Theme
// Adjudication response for an exact request. Every candidate receives an
// assessment for each of its own anchors (all direct), a reading order equal
// to the candidate's anchor order, and bounded unknowns, so the response
// passes item-local validation and the replay status is accepted. It never
// references provider prose or canonical identities.
func MockAdjudicationResponse(request AdjudicationRequest) ([]byte, error) {
	var themes []AdjudicatedTheme
	for index, candidate := range request.Candidates {
		if index >= DesiredFinalMax {
			break
		}
		assessments := make([]AnchorAssessment, 0, len(candidate.AnchorRefs))
		for _, ref := range candidate.AnchorRefs {
			assessments = append(assessments, AnchorAssessment{
				AnchorRef:            ref,
				Fit:                  FitDirect,
				Role:                 "exact_anchor",
				SupportedObservation: "The exact source-backed anchor contributes to the final question.",
			})
		}
		readingOrder := append([]string(nil), candidate.AnchorRefs...)
		themes = append(themes, AdjudicatedTheme{
			CandidateRef:      candidate.Ref,
			FinalTitle:        candidate.Title,
			FinalQuestion:     candidate.Question,
			AnchorAssessments: assessments,
			ReadingOrder:      readingOrder,
			Unknowns:          []string{"The supplied source proves bounded local structure; complete runtime order remains unproven."},
		})
	}
	if len(themes) == 0 {
		return nil, fmt.Errorf("theme adjudication mock response: no buildable themes")
	}
	raw, err := json.Marshal(map[string][]AdjudicatedTheme{"themes": themes})
	if err != nil {
		return nil, fmt.Errorf("theme adjudication mock response: encode: %w", err)
	}
	if _, _, err := ValidateAdjudication(raw, candidateByRef(request.Candidates)); err != nil {
		return nil, fmt.Errorf("theme adjudication mock response: not valid: %w", err)
	}
	return raw, nil
}
