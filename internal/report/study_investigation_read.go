package report

import (
	"fmt"
	"os"
	"sort"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// loadStudyInvestigationArtifacts restores the complete, persisted mechanism
// family into a report-neutral transient input. The family is optional only as
// a whole: one through three files are a broken prefix and fail closed. No
// request-local refs, canonical theme IDs, frontier diagnostics, or status
// values are copied into ReportData's public JSON projection.
func loadStudyInvestigationArtifacts(
	runDir string,
	data *ReportData,
	expectedRevision string,
	expectedFreshnessSHA256 string,
) error {
	if data == nil {
		return fmt.Errorf("study investigation report: report data is required")
	}
	if data.studyInvestigationArtifactsChecked {
		if data.studyInvestigationInput != nil &&
			expectedRevision != "" && data.studyInvestigationRepositoryRevision != expectedRevision {
			return fmt.Errorf("study investigation report: repository revision binding mismatch")
		}
		if data.studyInvestigationInput != nil && expectedFreshnessSHA256 != "" &&
			data.studyInvestigationFreshnessSHA256 != expectedFreshnessSHA256 {
			return fmt.Errorf("study investigation report: repository freshness binding mismatch")
		}
		return nil
	}

	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("study investigation report: open run directory: %w", err)
	}
	defer root.Close()

	type artifactSpec struct {
		name  string
		limit int
		raw   []byte
		has   bool
	}
	artifacts := []artifactSpec{
		{name: mechanismstudy.FactsArtifactFilename, limit: mechanismstudy.MaxFactsArtifactBytes},
		{name: mechanismstudy.CandidatesArtifactFilename, limit: mechanismstudy.MaxCandidatesArtifactBytes},
		{name: mechanismstudy.ResultArtifactFilename, limit: mechanismstudy.MaxResultArtifactBytes},
		{name: mechanismstudy.StatusArtifactFilename, limit: mechanismstudy.MaxStatusArtifactBytes},
	}
	present := 0
	for index := range artifacts {
		artifacts[index].raw, artifacts[index].has, err = readOptionalAtlasStudyArtifact(
			root,
			artifacts[index].name,
			artifacts[index].limit,
		)
		if err != nil {
			return fmt.Errorf("study investigation report: %w", err)
		}
		if artifacts[index].has {
			present++
		}
	}
	themesRaw, hasThemes, err := readOptionalAtlasStudyArtifact(
		root,
		themestudy.StudyThemesArtifactFilename,
		themestudy.MaxStudyThemesArtifactBytes,
	)
	if err != nil {
		return fmt.Errorf("study investigation report: %w", err)
	}
	if present == 0 && hasThemes {
		return fmt.Errorf("study investigation report: accepted study_themes requires the complete artifact family")
	}
	if present == 0 {
		data.studyInvestigationInput = nil
		data.studyInvestigationSourceLocations = nil
		data.studyInvestigationArtifactsChecked = true
		return nil
	}
	if present != len(artifacts) {
		return fmt.Errorf(
			"study investigation report: artifact family is incomplete (%d of %d files)",
			present,
			len(artifacts),
		)
	}

	if !hasThemes {
		return fmt.Errorf("study investigation report: artifact family requires study_themes")
	}

	return hydrateStudyInvestigationArtifacts(
		data,
		themesRaw,
		artifacts[0].raw,
		artifacts[1].raw,
		artifacts[2].raw,
		artifacts[3].raw,
		expectedRevision,
		expectedFreshnessSHA256,
	)
}

// hydrateStudyInvestigationArtifacts decodes one already-read exact artifact
// snapshot. Manifest verification passes the same bytes it hashed, avoiding a
// second file read between identity verification and semantic re-derivation.
func hydrateStudyInvestigationArtifacts(
	data *ReportData,
	themesRaw, factsRaw, candidatesRaw, resultRaw, statusRaw []byte,
	expectedRevision string,
	expectedFreshnessSHA256 string,
) error {
	if data == nil || len(themesRaw) == 0 || len(factsRaw) == 0 ||
		len(candidatesRaw) == 0 || len(resultRaw) == 0 || len(statusRaw) == 0 {
		return fmt.Errorf("study investigation report: complete artifact bytes are required")
	}
	themes, err := themestudy.DecodeStudyThemes(themesRaw)
	if err != nil {
		return fmt.Errorf("study investigation report: study_themes: %w", err)
	}
	facts, err := mechanismstudy.DecodeFacts(factsRaw)
	if err != nil {
		return fmt.Errorf("study investigation report: facts: %w", err)
	}
	if _, err := mechanismstudy.DecodeCandidates(factsRaw, candidatesRaw); err != nil {
		return fmt.Errorf("study investigation report: candidates: %w", err)
	}
	result, err := mechanismstudy.DecodeResult(factsRaw, candidatesRaw, resultRaw)
	if err != nil {
		return fmt.Errorf("study investigation report: result: %w", err)
	}
	if _, err := mechanismstudy.DecodeStatus(factsRaw, candidatesRaw, resultRaw, statusRaw); err != nil {
		return fmt.Errorf("study investigation report: status: %w", err)
	}

	binding := facts.Compilation.Binding
	themesSHA256 := manifestSHA256(themesRaw)
	if binding.ContextKind != mechanismstudy.ContextStudy ||
		binding.ContextSHA256 != themesSHA256 ||
		binding.StudyThemesSHA256 != themesSHA256 ||
		binding.AtlasStudyCatalogSHA256 != themes.ScoutSHA256 ||
		binding.RepositoryRevision != themes.Revision {
		return fmt.Errorf("study investigation report: Study artifact binding mismatch")
	}
	if expectedRevision != "" && binding.RepositoryRevision != expectedRevision {
		return fmt.Errorf("study investigation report: repository revision binding mismatch")
	}
	if expectedFreshnessSHA256 != "" &&
		binding.RepositoryFreshnessSHA256 != expectedFreshnessSHA256 {
		return fmt.Errorf("study investigation report: repository freshness binding mismatch")
	}

	publication, err := mechanismstudy.PublicationCards(facts.Compilation, result.Cards)
	if err != nil {
		return fmt.Errorf("study investigation report: restore publication: %w", err)
	}
	if err := validateStudyInvestigationPublicationCards(themes, publication); err != nil {
		return err
	}
	input, err := StudyInvestigationInputFromPublicationCards(publication)
	if err != nil {
		return err
	}
	locations, err := CollectStudyInvestigationSourceLocations(input)
	if err != nil {
		return err
	}

	data.studyInvestigationInput = &input
	data.studyInvestigationSourceLocations = append([]evidence.Location(nil), locations...)
	data.studyInvestigationArtifactsChecked = true
	data.studyInvestigationRepositoryRevision = binding.RepositoryRevision
	data.studyInvestigationFreshnessSHA256 = binding.RepositoryFreshnessSHA256
	return nil
}

// StudyInvestigationInputFromPublicationCards is the one report-owned adapter
// from validated mechanism authority into the neutral projection seam. Full
// symbols survive only transiently for the exact Architecture join.
func StudyInvestigationInputFromPublicationCards(
	cards []mechanismstudy.PublicationCard,
) (StudyInvestigationInput, error) {
	input := StudyInvestigationInput{
		Cards: make([]StudyInvestigationCardInput, 0, len(cards)),
	}
	for _, card := range cards {
		outcome := StudyInvestigationOutcomePrepared
		switch card.Outcome {
		case mechanismstudy.OutcomePrepared:
		case mechanismstudy.OutcomeMechanism:
			outcome = StudyInvestigationOutcomeMechanism
		default:
			return StudyInvestigationInput{}, fmt.Errorf(
				"study investigation report: unsupported publication outcome %q",
				card.Outcome,
			)
		}
		projected := StudyInvestigationCardInput{
			ThemeOrdinal:    card.StudyOrdinal,
			Outcome:         outcome,
			ReadingOrdinals: append([]int(nil), card.ReadingOrdinals...),
			Mechanisms:      make([]StudyInvestigationMechanismInput, 0, len(card.Mechanisms)),
		}
		for _, mechanism := range card.Mechanisms {
			path := StudyInvestigationMechanismInput{
				ReadingOrdinals: append([]int(nil), mechanism.ReadingOrdinals...),
				Nodes:           make([]StudyInvestigationNodeInput, 0, len(mechanism.Nodes)),
				Edges:           make([]StudyInvestigationEdgeInput, 0, len(mechanism.Edges)),
			}
			for _, node := range mechanism.Nodes {
				path.Nodes = append(path.Nodes, StudyInvestigationNodeInput{
					Label:    node.Label,
					Symbol:   node.Symbol.ID,
					Location: studyInvestigationEvidenceLocation(node.Declaration),
				})
			}
			for _, edge := range mechanism.Edges {
				path.Edges = append(path.Edges, StudyInvestigationEdgeInput{
					FromNodeOrdinal: edge.From,
					ToNodeOrdinal:   edge.To,
					Invocation:      edge.Invocation,
					WitnessCount:    edge.WitnessCount,
					Callsite:        studyInvestigationEvidenceLocation(edge.Callsite),
				})
			}
			projected.Mechanisms = append(projected.Mechanisms, path)
		}
		input.Cards = append(input.Cards, projected)
	}
	if _, err := CollectStudyInvestigationSourceLocations(input); err != nil {
		return StudyInvestigationInput{}, err
	}
	return input, nil
}

func validateStudyInvestigationPublicationCards(
	themes themestudy.StudyThemes,
	cards []mechanismstudy.PublicationCard,
) error {
	expected := append([]themestudy.ThemeCard(nil), themes.Cards...)
	sort.Slice(expected, func(i, j int) bool {
		if expected[i].Ordinal != expected[j].Ordinal {
			return expected[i].Ordinal < expected[j].Ordinal
		}
		return expected[i].CanonicalID < expected[j].CanonicalID
	})
	if len(expected) > mechanismstudy.MaxCards {
		expected = expected[:mechanismstudy.MaxCards]
	}
	if len(cards) != len(expected) {
		return fmt.Errorf("study investigation report: publication does not cover every Study card")
	}
	for position, card := range cards {
		if card.StudyOrdinal != expected[position].Ordinal ||
			card.StudyCanonicalID != expected[position].CanonicalID {
			return fmt.Errorf("study investigation report: publication Study card binding mismatch")
		}
	}
	return nil
}

func studyInvestigationEvidenceLocation(location surfacediscovery.Location) evidence.Location {
	return evidence.Location{Path: location.Path, Line: location.Line, Column: location.Column}
}

func cloneStudyInvestigationInput(input *StudyInvestigationInput) *StudyInvestigationInput {
	if input == nil {
		return nil
	}
	clone := StudyInvestigationInput{Cards: make([]StudyInvestigationCardInput, 0, len(input.Cards))}
	for _, card := range input.Cards {
		copied := StudyInvestigationCardInput{
			ThemeOrdinal:    card.ThemeOrdinal,
			Outcome:         card.Outcome,
			ReadingOrdinals: append([]int(nil), card.ReadingOrdinals...),
			Mechanisms:      make([]StudyInvestigationMechanismInput, 0, len(card.Mechanisms)),
		}
		for _, mechanism := range card.Mechanisms {
			copied.Mechanisms = append(copied.Mechanisms, StudyInvestigationMechanismInput{
				ReadingOrdinals: append([]int(nil), mechanism.ReadingOrdinals...),
				Nodes:           append([]StudyInvestigationNodeInput(nil), mechanism.Nodes...),
				Edges:           append([]StudyInvestigationEdgeInput(nil), mechanism.Edges...),
			})
		}
		clone.Cards = append(clone.Cards, copied)
	}
	return &clone
}
