package themestudy

import (
	"encoding/json"
	"fmt"
)

// ResolveScoutResponse validates one raw provider response for an exact Scout
// request and produces the accepted result record plus the persisted status.
// It performs no I/O and never calls a provider. Zero valid candidates is a
// semantic failure (returned as an error), never a locally fabricated shelf.
func ResolveScoutResponse(request ScoutRequest, raw []byte) (ScoutResult, ScoutStatusRecord, error) {
	anchorRefs := request.AnchorRefs()
	fileRefs := request.FileRefs()
	candidates, status, err := ValidateScout(raw, anchorRefs, fileRefs, request.CatalogSHA256)
	if err != nil {
		return ScoutResult{}, ScoutStatusRecord{}, err
	}
	if status.State == "failed" {
		return ScoutResult{}, ScoutStatusRecord{}, fmt.Errorf("theme scout: zero valid candidates (semantic failure)")
	}
	AssignCandidateRefs(candidates)
	result := ScoutResult{
		Version:       ScoutResultVersion,
		State:         status.State,
		PromptVersion: ScoutPromptVersion,
		Language:      request.Language,
		CatalogSHA256: request.CatalogSHA256,
		WireSHA256:    request.WireSHA256,
		Candidates:    candidates,
		Status:        status,
	}
	statusRecord := ScoutStatusRecord{
		Version: ScoutResultVersion, State: status.State,
		PromptVersion: ScoutPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, Status: status,
	}
	return result, statusRecord, nil
}

// ResolveAdjudicationResponse validates one raw provider response for an exact
// Adjudication request and produces the accepted result record plus the
// persisted status. It performs no I/O and never calls a provider. Zero
// accepted themes is a semantic failure, never a fabricated shelf.
func ResolveAdjudicationResponse(request AdjudicationRequest, raw []byte) (AdjudicationResult, AdjudicationStatusRecord, error) {
	byRef := candidateByRef(request.Candidates)
	themes, status, err := ValidateAdjudication(raw, byRef)
	if err != nil {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, err
	}
	// Decision 232 (Archive 9): zero accepted themes is an honest
	// semantic-empty result, NOT a transport failure. The result and the
	// failed status publish so the report renders the complete local
	// question browse (never fabricated cards, never hidden information).
	result := AdjudicationResult{
		Version:       AdjudicationResultVersion,
		State:         status.State,
		PromptVersion: AdjudicationPromptVersion,
		Language:      request.Language,
		CatalogSHA256: request.CatalogSHA256,
		WireSHA256:    request.WireSHA256,
		Themes:        themes,
		Status:        status,
	}
	statusRecord := AdjudicationStatusRecord{
		Version: AdjudicationResultVersion, State: status.State,
		PromptVersion: AdjudicationPromptVersion, Language: request.Language,
		CatalogSHA256: request.CatalogSHA256, Status: status,
	}
	return result, statusRecord, nil
}

// ReduceAccepted reduces the accepted adjudication output into the published
// study_themes portfolio (contract F). It is deterministic and never re-ranks
// model output. A reduction with zero published cards is an honest partial
// (never fabricated); the caller decides the state from the returned
// Reduction.
func ReduceAccepted(
	candidates []ScoutCandidate,
	themes []AdjudicatedTheme,
	anchors map[string]AnchorInfo,
) (Reduction, error) {
	byRef := candidateByRef(candidates)
	return Reduce(ReducerInput{
		Themes:     themes,
		Candidates: byRef,
		Anchors:    anchors,
	})
}

// anchorInfoFromPacks builds the exact anchor identity map from the compiled
// seed packs. Only advertised a* seeds appear; identities are backend-owned
// and never model prose.
func anchorInfoFromPacks(packs SeedPackResult) map[string]AnchorInfo {
	anchors := make(map[string]AnchorInfo, len(packs.Packs))
	for _, pack := range packs.Packs {
		seed := pack.Seed
		anchors[seed.Ref] = AnchorInfo{
			Path: seed.Path, Symbol: seed.Symbol, Line: seed.Line,
			CanonicalSpanID: seed.CanonicalSpanID,
		}
	}
	return anchors
}

// MarshalIndentJSON renders one value as indented JSON for the semantic
// exchange journal and debug artifacts. It never receives secrets.
func MarshalIndentJSON(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}
