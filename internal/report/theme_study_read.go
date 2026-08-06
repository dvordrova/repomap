package report

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// readAtlasStudyReportProduct derives the Decision 213 Study report section
// from the eight theme artifacts bound by the run manifest. The retired
// single-stage atlas-study provider call no longer contributes a Brief or
// per-span directions: the Study section carries the editorial theme shelf
// (Themes) plus the re-based four-stage browse
// (considered / seed-advertised / scout-anchored / published), and the
// RepositoryStudyMap is nil in new runs.
//
// Failure states stop the artifact prefix early: Scout failure leaves request +
// status only; Adjudication failure leaves the expansion through the
// Adjudication request/status. Each accepted stage is re-validated against its
// exact request identity (catalog/wire digests) and the rebuilt input, so the
// manifest DeepEqual round-trip re-derives byte-identical report bytes.
func readAtlasStudyReportProduct(
	runDir string,
	data *ReportData,
) (*AtlasStudyReportStatus, *RepositoryStudyMap, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: open run directory: %w", err)
	}
	defer root.Close()

	scoutRequestRaw, hasScoutRequest, err := readOptionalAtlasStudyArtifact(
		root, themestudy.ScoutRequestArtifactFilename, themestudy.MaxScoutRequestArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	scoutResultRaw, hasScoutResult, err := readOptionalAtlasStudyArtifact(
		root, themestudy.ScoutResultArtifactFilename, themestudy.MaxScoutResultArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	scoutStatusRaw, hasScoutStatus, err := readOptionalAtlasStudyArtifact(
		root, themestudy.ScoutStatusArtifactFilename, themestudy.MaxScoutStatusArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	expansionRaw, hasExpansion, err := readOptionalAtlasStudyArtifact(
		root, themestudy.ExpansionArtifactFilename, themestudy.MaxExpansionArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	adjRequestRaw, hasAdjRequest, err := readOptionalAtlasStudyArtifact(
		root, themestudy.AdjudicationRequestArtifactFilename, themestudy.MaxAdjRequestArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	adjResultRaw, hasAdjResult, err := readOptionalAtlasStudyArtifact(
		root, themestudy.AdjudicationResultArtifactFilename, themestudy.MaxAdjResultArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	adjStatusRaw, hasAdjStatus, err := readOptionalAtlasStudyArtifact(
		root, themestudy.AdjudicationStatusArtifactFilename, themestudy.MaxAdjStatusArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	themesRaw, hasThemes, err := readOptionalAtlasStudyArtifact(
		root, themestudy.StudyThemesArtifactFilename, themestudy.MaxStudyThemesArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	hasAnyArtifact := hasScoutRequest || hasScoutResult || hasScoutStatus ||
		hasExpansion || hasAdjRequest || hasAdjResult || hasAdjStatus || hasThemes
	if !hasAnyArtifact {
		return uncalledAtlasStudyReportStatus(data), nil, nil
	}
	if !hasScoutRequest || !hasScoutStatus {
		return nil, nil, fmt.Errorf("atlas study report: theme artifact set requires Scout request and status")
	}

	scoutRequest, err := themestudy.DecodeScoutRequest(scoutRequestRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Scout request: %w", err)
	}
	scoutStatus, err := themestudy.DecodeScoutStatus(scoutStatusRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Scout status: %w", err)
	}
	input, err := BuildAtlasStudyInput(data, languageFromTheme(scoutRequest.Language))
	if err != nil {
		return nil, nil, err
	}
	if err := validateThemeScoutRequestAgainstInput(scoutRequest, input); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Scout request binding: %w", err)
	}

	reportStatus := &AtlasStudyReportStatus{
		Version:           scoutStatus.Version,
		ProjectionVersion: AtlasStudyReportProjectionVersion,
		State:             atlasstudy.ProductState(scoutStatus.State),
	}
	if reportStatus.State == atlasstudy.ProductStateUnavailable {
		reportStatus.UnavailableCode = AtlasStudyUnavailableCode(scoutStatus.UnavailableCode)
	}
	if reportStatus.State == atlasstudy.ProductStateFailed {
		reportStatus.FailureCode = atlasstudy.FailureCode(scoutStatus.FailureCode)
	}

	switch scoutStatus.State {
	case string(atlasstudy.ProductStateAccepted), string(atlasstudy.ProductStateAcceptedPartial):
		return readThemeStudyAccepted(
			root, data, input, scoutRequest, scoutStatus,
			scoutResultRaw, hasScoutResult, expansionRaw, hasExpansion,
			adjRequestRaw, hasAdjRequest, adjResultRaw, hasAdjResult,
			adjStatusRaw, hasAdjStatus, themesRaw, hasThemes,
		)
	case string(atlasstudy.ProductStateFailed):
		if hasScoutResult {
			return nil, nil, fmt.Errorf("atlas study report: failed Scout status cannot contain a Scout result")
		}
		// An Adjudication failure after a successful Scout leaves the
		// expansion and the Adjudication request/status; a pure Scout failure
		// leaves none. Either way the browse is the neutral local-question
		// surface and the theme shelf is absent.
		if hasExpansion || hasAdjRequest || hasAdjResult || hasAdjStatus || hasThemes {
			if !hasAdjRequest || !hasAdjStatus || hasAdjResult || hasThemes {
				return nil, nil, fmt.Errorf("atlas study report: Adjudication failure has inconsistent stage artifacts")
			}
			reportStatus.State = atlasstudy.ProductStateFailed
		}
		browse, browseErr := deriveAtlasStudyFailedBrowse(input, data)
		if browseErr != nil {
			return nil, nil, browseErr
		}
		reportStatus.FrontierBrowse = browse
		return reportStatus, nil, nil
	case string(atlasstudy.ProductStateUnavailable):
		if hasScoutResult || hasExpansion || hasAdjRequest || hasAdjResult || hasAdjStatus || hasThemes {
			return nil, nil, fmt.Errorf("atlas study report: unavailable Scout status cannot contain later stage artifacts")
		}
		return reportStatus, nil, nil
	default:
		return nil, nil, fmt.Errorf("atlas study report: unsupported Scout state %q", scoutStatus.State)
	}
}

// readThemeStudyAccepted derives the accepted/accepted_partial Study report
// section: the full eight-artifact prefix is present and each stage is
// re-validated against its exact request identity, then the theme shelf and
// the re-based four-stage browse are derived.
func readThemeStudyAccepted(
	root *os.Root,
	data *ReportData,
	input atlasstudy.Input,
	scoutRequest themestudy.ScoutRequest,
	scoutStatus themestudy.ScoutStatusRecord,
	scoutResultRaw []byte, hasScoutResult bool,
	expansionRaw []byte, hasExpansion bool,
	adjRequestRaw []byte, hasAdjRequest bool,
	adjResultRaw []byte, hasAdjResult bool,
	adjStatusRaw []byte, hasAdjStatus bool,
	themesRaw []byte, hasThemes bool,
) (*AtlasStudyReportStatus, *RepositoryStudyMap, error) {
	// Decision 232 (Archive 9): zero accepted themes is an honest
	// semantic-empty outcome. It is detected BEFORE the all-artifacts
	// requirement: an Adjudication failure after a successful Scout keeps
	// the scout/expansion/adj artifacts but writes no study_themes, and the
	// report renders the complete local question browse (never fabricated
	// cards, never hidden information).
	if hasAdjStatus {
		adjStatusEarly, decodeErr := themestudy.DecodeAdjudicationStatus(adjStatusRaw)
		if decodeErr == nil && adjStatusEarly.Status.State == string(atlasstudy.ProductStateFailed) &&
			hasAdjResult && !hasThemes {
			adjResultEarly, decodeErr := themestudy.DecodeAdjudicationResult(adjResultRaw)
			if decodeErr == nil && len(adjResultEarly.Themes) == 0 {
				browse, browseErr := deriveAtlasStudyFailedBrowse(input, data)
				if browseErr != nil {
					return nil, nil, browseErr
				}
				return &AtlasStudyReportStatus{
					Version:           scoutStatus.Version,
					ProjectionVersion: AtlasStudyReportProjectionVersion,
					State:             atlasstudy.ProductStateFailed,
					FailureCode:       atlasstudy.FailureValidation,
					FrontierBrowse:    browse,
				}, nil, nil
			}
		}
	}
	if !hasScoutResult || !hasExpansion || !hasAdjRequest || !hasAdjResult || !hasAdjStatus || !hasThemes {
		return nil, nil, fmt.Errorf("atlas study report: accepted Scout status requires all theme stage artifacts")
	}
	scoutResult, err := themestudy.DecodeScoutResult(scoutResultRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Scout result: %w", err)
	}
	if err := validateThemeScoutResultAgainstRequest(scoutResult, scoutRequest); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Scout result binding: %w", err)
	}
	expansion, err := themestudy.DecodeExpansion(expansionRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: source expansion: %w", err)
	}
	if err := validateThemeExpansionAgainstResult(expansion, scoutResult, scoutRequest); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: source expansion binding: %w", err)
	}
	adjRequest, err := themestudy.DecodeAdjudicationRequest(adjRequestRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Adjudication request: %w", err)
	}
	if err := validateThemeAdjRequest(adjRequest, scoutResult, expansion, scoutRequest); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Adjudication request binding: %w", err)
	}
	adjResult, err := themestudy.DecodeAdjudicationResult(adjResultRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Adjudication result: %w", err)
	}
	adjStatus, err := themestudy.DecodeAdjudicationStatus(adjStatusRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Adjudication status: %w", err)
	}
	if err := validateThemeAdjResult(adjResult, adjStatus, adjRequest); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: Adjudication result binding: %w", err)
	}
	// Decision 232 (Archive 9): zero accepted themes is an honest
	// semantic-empty outcome. The report renders the complete local
	// question browse with a failed state — never fabricated cards, never
	// hidden information.
	if adjResult.State == string(atlasstudy.ProductStateFailed) {
		if len(adjResult.Themes) != 0 {
			return nil, nil, fmt.Errorf("atlas study report: failed Adjudication result contains themes")
		}
		if hasThemes {
			return nil, nil, fmt.Errorf("atlas study report: semantic-empty Adjudication cannot carry study_themes")
		}
		browse, browseErr := deriveAtlasStudyFailedBrowse(input, data)
		if browseErr != nil {
			return nil, nil, browseErr
		}
		return &AtlasStudyReportStatus{
			Version:           scoutResult.Version,
			ProjectionVersion: AtlasStudyReportProjectionVersion,
			State:             atlasstudy.ProductStateFailed,
			FailureCode:       atlasstudy.FailureValidation,
			FrontierBrowse:    browse,
		}, nil, nil
	}
	themes, err := themestudy.DecodeStudyThemes(themesRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: study_themes: %w", err)
	}
	if err := validateThemeStudyThemes(themes, adjResult, scoutRequest); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: study_themes binding: %w", err)
	}

	reportStatus := &AtlasStudyReportStatus{
		Version:           scoutResult.Version,
		ProjectionVersion: AtlasStudyReportProjectionVersion,
		State:             atlasstudy.ProductState(scoutResult.State),
	}
	if adjResult.State == string(atlasstudy.ProductStateAcceptedPartial) ||
		themes.Partial {
		reportStatus.State = atlasstudy.ProductStateAcceptedPartial
	}

	// Re-based four-stage browse: considered (rebuilt input), seed-advertised
	// (a* seeds in the Scout request catalog, bound to spans), scout-anchored
	// (spans in a Scout-accepted candidate's anchor_refs), published (spans in
	// final theme readings). The chain published ⊆ scout-anchored ⊆
	// seed-advertised ⊆ considered is re-verified with exact tally equality.
	browse, counts, err := deriveThemeStudyFrontierBrowse(input, scoutRequest, scoutResult, themes, data)
	if err != nil {
		return nil, nil, err
	}
	reportStatus.FrontierBrowse = browse
	reportStatus.ConsideredSpanCount = counts.considered
	reportStatus.AdvertisedSpanCount = counts.seedAdvertised
	reportStatus.ModelSelectedSpanCount = counts.scoutAnchored
	reportStatus.AcceptedSpanCount = counts.published
	// Decision 232: adjudication anchor coverage is projected as exact
	// counts (never published for unreviewed anchors).
	reportStatus.ReviewedAnchors = adjStatus.Status.ReviewedAnchors
	reportStatus.UnreviewedAnchors = adjStatus.Status.UnreviewedAnchors
	reportStatus.FrontierComplete = counts.seedAdvertised == counts.considered
	reportStatus.SelectedItemsComplete = reportStatus.State == atlasstudy.ProductStateAccepted
	reportStatus.SupportCoverageComplete = true
	reportStatus.PortfolioTargetMet = len(themes.Cards) >= 4 && len(themes.Cards) <= themestudy.MaxFinalThemes

	projected, err := projectThemeShelf(themes, data)
	if err != nil {
		return nil, nil, err
	}
	reportStatus.Themes = projected
	return reportStatus, nil, nil
}

// themeStageCounts is the exact per-stage span tally of the re-based browse.
type themeStageCounts struct {
	considered     int
	seedAdvertised int
	scoutAnchored  int
	published      int
}

// deriveThemeStudyFrontierBrowse computes the provider-free re-based per-span
// browse for accepted/accepted_partial theme runs. It runs ONLY inside
// readAtlasStudyReportProduct after every theme artifact was decoded and
// validated, so the manifest DeepEqual round-trip re-derives byte-identical
// browse bytes. Fail-closed invariants: the chain published ⊆ scout-anchored
// ⊆ seed-advertised ⊆ considered is re-verified, per-stage tallies over the
// FULL pre-truncation row set are returned, every published row's ThemeRefs
// list every matching published theme ordinal in canonical theme order, and
// only then the 256 ceiling is applied with truthful Total/Shown.
func deriveThemeStudyFrontierBrowse(
	input atlasstudy.Input,
	scoutRequest themestudy.ScoutRequest,
	scoutResult themestudy.ScoutResult,
	themes themestudy.StudyThemes,
	data *ReportData,
) (*FrontierBrowse, themeStageCounts, error) {
	considered := make(map[string]struct{}, len(input.RouteSpans))
	for _, span := range input.RouteSpans {
		considered[span.ID] = struct{}{}
	}
	// seed-advertised: spans whose canonical identity is an advertised a*
	// seed in the Scout request catalog.
	seedByRef := make(map[string]themestudy.SeedSpec)
	seedAdvertised := make(map[string]struct{})
	for _, pack := range scoutRequest.SeedPacks.Packs {
		seed := pack.Seed
		seedByRef[seed.Ref] = seed
		if seed.CanonicalSpanID == "" {
			continue
		}
		if _, ok := considered[seed.CanonicalSpanID]; !ok {
			return nil, themeStageCounts{}, fmt.Errorf(
				"atlas study report: Scout seed %q binds unavailable span %q (fail closed)", seed.Ref, seed.CanonicalSpanID)
		}
		seedAdvertised[seed.CanonicalSpanID] = struct{}{}
	}
	// scout-anchored: spans appearing in a Scout-accepted candidate's
	// anchor_refs. An advertised a* seed without a canonical span binding is a
	// legitimate candidate anchor (the seed producer emits one seed per
	// reading target; only sole-allowed targets bind a span) — it contributes
	// the theme card but no span row, exactly like an empty-span reading.
	scoutAnchored := make(map[string]struct{})
	for _, candidate := range scoutResult.Candidates {
		for _, ref := range candidate.AnchorRefs {
			seed, ok := seedByRef[ref]
			if !ok {
				return nil, themeStageCounts{}, fmt.Errorf(
					"atlas study report: Scout candidate anchor %q is not an advertised seed (fail closed)", ref)
			}
			if seed.CanonicalSpanID == "" {
				continue
			}
			if _, ok := considered[seed.CanonicalSpanID]; !ok {
				return nil, themeStageCounts{}, fmt.Errorf(
					"atlas study report: Scout candidate anchor %q binds unavailable span %q (fail closed)",
					ref, seed.CanonicalSpanID)
			}
			scoutAnchored[seed.CanonicalSpanID] = struct{}{}
		}
	}
	// published: spans in final theme readings.
	published := make(map[string]struct{})
	for _, card := range themes.Cards {
		for _, reading := range card.Readings {
			if reading.CanonicalSpanID == "" {
				continue
			}
			if _, ok := considered[reading.CanonicalSpanID]; !ok {
				return nil, themeStageCounts{}, fmt.Errorf(
					"atlas study report: theme reading binds unavailable span %q (fail closed)", reading.CanonicalSpanID)
			}
			published[reading.CanonicalSpanID] = struct{}{}
		}
	}
	// Re-verify published ⊆ scout-anchored ⊆ seed-advertised ⊆ considered.
	for spanID := range published {
		if _, ok := scoutAnchored[spanID]; !ok {
			return nil, themeStageCounts{}, fmt.Errorf("atlas study report: published span is outside scout-anchored")
		}
	}
	for spanID := range scoutAnchored {
		if _, ok := seedAdvertised[spanID]; !ok {
			return nil, themeStageCounts{}, fmt.Errorf("atlas study report: scout-anchored span is outside seed-advertised")
		}
	}
	for spanID := range seedAdvertised {
		if _, ok := considered[spanID]; !ok {
			return nil, themeStageCounts{}, fmt.Errorf("atlas study report: seed-advertised span is outside considered")
		}
	}
	// ThemeRefs: published rows list every matching published theme ordinal in
	// canonical theme order (D213 B1/N5).
	themeOrdinalsBySpan := make(map[string][]int)
	for _, card := range themes.Cards {
		for _, reading := range card.Readings {
			if reading.CanonicalSpanID == "" {
				continue
			}
			themeOrdinalsBySpan[reading.CanonicalSpanID] = append(
				themeOrdinalsBySpan[reading.CanonicalSpanID], card.Ordinal)
		}
	}
	for spanID := range themeOrdinalsBySpan {
		sort.Ints(themeOrdinalsBySpan[spanID])
	}

	rows := make([]atlasStudyBrowseRow, 0, len(input.RouteSpans))
	for _, span := range input.RouteSpans {
		stage := AtlasStudySpanStageConsidered
		if _, ok := published[span.ID]; ok {
			stage = AtlasStudySpanStagePublished
		} else if _, ok := scoutAnchored[span.ID]; ok {
			stage = AtlasStudySpanStageScoutAnchored
		} else if _, ok := seedAdvertised[span.ID]; ok {
			stage = AtlasStudySpanStageSeedAdvertised
		}
		title, question, source, endpoint, err := atlasStudyBrowseRowContent(input, span, data)
		if err != nil {
			return nil, themeStageCounts{}, err
		}
		row := atlasStudyBrowseRow{
			spanID: span.ID, learningStage: span.LearningStage, stage: stage,
			title: title, question: question, source: source, endpoint: endpoint,
			themeRefs: themeOrdinalsBySpan[span.ID],
		}
		rows = append(rows, row)
	}
	counts := themeStageCounts{
		considered:     len(considered),
		seedAdvertised: len(seedAdvertised),
		scoutAnchored:  len(scoutAnchored),
		published:      len(published),
	}
	return finishAtlasStudyBrowse(rows, nil), counts, nil
}

// projectThemeShelf projects the reduced theme portfolio into the bounded
// public-safe report shelf. Card DTOs carry editorial prose, exact readings
// and a badge — zero source bytes, zero canonical identities. Path/line
// publish only for paths in OpenablePaths; otherwise the neutral unavailable
// state renders (no dead buttons).
func projectThemeShelf(themes themestudy.StudyThemes, data *ReportData) (*AtlasStudyThemesProjection, error) {
	cards := make([]StudyThemeCard, 0, len(themes.Cards))
	for _, card := range themes.Cards {
		readings := make([]StudyThemeReading, 0, len(card.Readings))
		for _, reading := range card.Readings {
			projected := StudyThemeReading{
				Label: reading.Label, Symbol: reading.Symbol,
				SupportedObservation: reading.SupportedObservation,
			}
			switch reading.Fit {
			case themestudy.FitDirect:
				projected.Role = "direct"
			case themestudy.FitSupporting:
				projected.Role = "supporting"
			}
			if openableAtlasStudyBrowsePath(data, reading.Path) {
				projected.Path = reading.Path
				projected.Line = reading.Line
			}
			readings = append(readings, projected)
		}
		cards = append(cards, StudyThemeCard{
			Ordinal:          card.Ordinal,
			FinalTitle:       card.FinalTitle,
			FinalQuestion:    card.FinalQuestion,
			WhyItMatters:     card.WhyItMatters,
			ExpectedLearning: card.ExpectedLearning,
			ThemeKind:        string(card.ThemeKind),
			Readings:         readings,
			Badge:            card.Badge,
			Limitation:       themeLimitation(themes, card),
		})
	}
	total := len(cards)
	// No truncation: every published card renders. The portfolio is already
	// bounded by the study_themes artifact encode (MaxStudyThemesArtifactBytes).
	return &AtlasStudyThemesProjection{Total: total, Shown: total, Cards: cards}, nil
}

// themeLimitation is the honest per-card limitation line: a partial badge is
// always accompanied by the exact partial count statement.
func themeLimitation(themes themestudy.StudyThemes, card themestudy.ThemeCard) string {
	if !themes.Partial {
		return ""
	}
	if card.DirectCount+card.SupportingCount > 0 {
		return fmt.Sprintf("partial — %d of %d anchors passed source review", card.DirectCount+card.SupportingCount, card.DirectCount+card.SupportingCount)
	}
	return "partial"
}

// languageFromTheme maps the theme-stage prose language onto the atlasstudy
// input language used to rebuild the exact substrate.
func languageFromTheme(language themestudy.Language) atlasstudy.Language {
	if language == themestudy.LanguageRussian {
		return atlasstudy.LanguageRussian
	}
	return atlasstudy.LanguageEnglish
}

// readOptionalAtlasStudyArtifact is shared with atlas_study.go: it reads one
// bounded run artifact, returning has=false when it is absent (fs.ErrNotExist)
// and failing closed on any other read error, including obvious credentials.

// deriveAtlasStudyFailedBrowse builds the neutral local-question browse for a
// failed or unavailable Study run. Every row carries the considered stage and
// the surface is exempt from accepted-stage tally checks (no theme artifacts,
// no ThemeRefs). Total comes from the rebuilt input count.
func deriveAtlasStudyFailedBrowse(input atlasstudy.Input, data *ReportData) (*FrontierBrowse, error) {
	rows := make([]atlasStudyBrowseRow, 0, len(input.RouteSpans))
	for _, span := range input.RouteSpans {
		title, question, source, endpoint, err := atlasStudyBrowseRowContent(input, span, data)
		if err != nil {
			return nil, err
		}
		rows = append(rows, atlasStudyBrowseRow{
			spanID: span.ID, learningStage: span.LearningStage,
			stage: AtlasStudySpanStageConsidered,
			title: title, question: question, source: source, endpoint: endpoint,
		})
	}
	return finishAtlasStudyBrowse(rows, nil), nil
}

// validateThemeScoutRequestAgainstInput re-verifies that every advertised a*
// seed and f* file in the Scout request resolves against the rebuilt local
// substrate: seed paths/lines/symbols match a reading target and seed span
// bindings resolve to considered spans.
func validateThemeScoutRequestAgainstInput(request themestudy.ScoutRequest, input atlasstudy.Input) error {
	targets := make(map[string]atlasstudy.ReadingTarget, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		key := fmt.Sprintf("%s\x00%d\x00%s", target.Location.Path, target.Location.Line, target.Symbol)
		targets[key] = target
	}
	for _, pack := range request.SeedPacks.Packs {
		seed := pack.Seed
		key := fmt.Sprintf("%s\x00%d\x00%s", seed.Path, seed.Line, seed.Symbol)
		if _, ok := targets[key]; !ok {
			return fmt.Errorf("Scout seed %q (%s:%d %s) does not resolve to a reading target", seed.Ref, seed.Path, seed.Line, seed.Symbol)
		}
	}
	return nil
}

// validateThemeScoutResultAgainstRequest re-verifies the Scout result against
// its exact request identity: every accepted candidate's refs resolve to the
// request catalog, no cross-request refs, item-local rejection already
// recorded by the runtime.
func validateThemeScoutResultAgainstRequest(result themestudy.ScoutResult, request themestudy.ScoutRequest) error {
	if result.CatalogSHA256 != request.CatalogSHA256 {
		return fmt.Errorf("Scout result catalog digest does not match request")
	}
	if result.WireSHA256 != request.WireSHA256 {
		return fmt.Errorf("Scout result wire digest does not match request")
	}
	anchorRefs := request.AnchorRefs()
	fileRefs := request.FileRefs()
	for _, candidate := range result.Candidates {
		if candidate.Ref == "" {
			return fmt.Errorf("Scout candidate has no t* ref")
		}
		for _, ref := range candidate.AnchorRefs {
			if _, ok := anchorRefs[ref]; !ok {
				return fmt.Errorf("Scout candidate %q references unknown anchor %q", candidate.Ref, ref)
			}
		}
		for _, ref := range candidate.ExpansionFileRefs {
			if _, ok := fileRefs[ref]; !ok {
				return fmt.Errorf("Scout candidate %q references unknown file %q", candidate.Ref, ref)
			}
		}
	}
	return nil
}

// validateThemeExpansionAgainstResult re-verifies the local source expansion
// (contract D): it expands exactly the f* refs the accepted Scout candidates
// requested, in canonical order, with no unrequested files.
func validateThemeExpansionAgainstResult(
	expansion themestudy.SourceExpansion,
	result themestudy.ScoutResult,
	request themestudy.ScoutRequest,
) error {
	requested := make(map[string]struct{})
	for _, candidate := range result.Candidates {
		for _, ref := range candidate.ExpansionFileRefs {
			requested[ref] = struct{}{}
		}
	}
	fileRefs := request.FileRefs()
	for _, ref := range expansion.Requested {
		if _, ok := requested[ref]; !ok {
			return fmt.Errorf("source expansion contains unrequested file ref %q", ref)
		}
	}
	if len(expansion.Requested) != len(requested) {
		return fmt.Errorf("source expansion requested set does not match Scout candidates")
	}
	for _, file := range expansion.Files {
		if _, ok := fileRefs[file.Ref]; !ok {
			return fmt.Errorf("source expansion references unknown file ref %q", file.Ref)
		}
	}
	return nil
}

// validateThemeAdjRequest re-verifies the Adjudication request (contract E)
// against the accepted Scout candidates and the local expansion: the t*
// catalog matches, anchors are the candidate's own, and every expanded file
// came from the local expansion. The Adjudication catalog digest is the
// request's own self-identity (candidates + expansion + anchors), so the
// binding is by exact t* catalog equality, not digest equality.
func validateThemeAdjRequest(
	request themestudy.AdjudicationRequest,
	result themestudy.ScoutResult,
	expansion themestudy.SourceExpansion,
	scoutRequest themestudy.ScoutRequest,
) error {
	candidateByRef := make(map[string]struct{}, len(request.Candidates))
	for _, candidate := range request.Candidates {
		if candidate.Ref == "" {
			return fmt.Errorf("Adjudication request candidate has no t* ref")
		}
		candidateByRef[candidate.Ref] = struct{}{}
	}
	if len(candidateByRef) != len(result.Candidates) {
		return fmt.Errorf("Adjudication request candidate set does not match Scout result")
	}
	for _, candidate := range result.Candidates {
		if _, ok := candidateByRef[candidate.Ref]; !ok {
			return fmt.Errorf("Adjudication request candidate set does not match Scout result")
		}
	}
	anchorRefs := scoutRequest.AnchorRefs()
	for _, candidate := range request.Candidates {
		for _, ref := range candidate.AnchorRefs {
			if _, ok := anchorRefs[ref]; !ok {
				return fmt.Errorf("Adjudication request candidate %q references unknown anchor %q", candidate.Ref, ref)
			}
		}
	}
	expanded := make(map[string]struct{}, len(expansion.Files))
	for _, file := range expansion.Files {
		expanded[file.Ref] = struct{}{}
	}
	for _, candidate := range request.Candidates {
		for _, ref := range candidate.ExpansionFileRefs {
			if _, ok := expanded[ref]; !ok {
				return fmt.Errorf("Adjudication request candidate %q references unexpanded file %q", candidate.Ref, ref)
			}
		}
	}
	return nil
}

// validateThemeAdjResult re-verifies the Adjudication result and status
// against the Adjudication request identity.
func validateThemeAdjResult(
	result themestudy.AdjudicationResult,
	status themestudy.AdjudicationStatusRecord,
	request themestudy.AdjudicationRequest,
) error {
	if result.CatalogSHA256 != request.CatalogSHA256 {
		return fmt.Errorf("Adjudication result catalog digest does not match request")
	}
	if result.WireSHA256 != request.WireSHA256 {
		return fmt.Errorf("Adjudication result wire digest does not match request")
	}
	if status.CatalogSHA256 != request.CatalogSHA256 {
		return fmt.Errorf("Adjudication status catalog digest does not match request")
	}
	return nil
}

// validateThemeStudyThemes re-verifies the reduced portfolio artifact against
// the Adjudication result and the Scout request identity: its digests bind the
// exact stage chain and its cards carry zero source bytes.
func validateThemeStudyThemes(
	themes themestudy.StudyThemes,
	result themestudy.AdjudicationResult,
	scoutRequest themestudy.ScoutRequest,
) error {
	if themes.ScoutSHA256 != scoutRequest.CatalogSHA256 {
		return fmt.Errorf("study_themes Scout digest does not match Scout request")
	}
	if themes.AdjSHA256 != result.CatalogSHA256 {
		return fmt.Errorf("study_themes Adjudication digest does not match Adjudication result")
	}
	// Zero source bytes: cards carry no lines, no content hashes, no pack
	// bytes. Re-encoding the cards must not contain source-bearing fields.
	encoded, err := json.Marshal(themes.Cards)
	if err != nil {
		return fmt.Errorf("study_themes cards: %w", err)
	}
	var roundTrip []json.RawMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		return fmt.Errorf("study_themes cards: %w", err)
	}
	return nil
}
