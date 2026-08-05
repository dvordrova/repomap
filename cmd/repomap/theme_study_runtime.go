package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// themeCacheContracts are the Decision 213 accepted-only stage cache
// contracts. New semantics — new contract names, never reinterpreted.
const (
	themeScoutCacheContract       = themestudy.ScoutCacheContract
	themeAdjudicationCacheContract = themestudy.AdjudicationCacheContract
	themeScoutCacheStage          = "theme_scout"
	themeAdjudicationCacheStage   = "theme_adjudication"
)

// themeStudyRunOutcome is the run-wiring summary of the two-stage theme
// pipeline (Decision 213). The retired single-stage atlas-study call no longer
// contributes a DirectionCount; the theme shelf is summarized by card counts.
type themeStudyRunOutcome struct {
	State           atlasstudy.ProductState
	FailureCode     atlasstudy.FailureCode
	ProviderSkipped bool
	Cached          bool
	SemanticCalls   int
	ScoutAccepted   int
	ScoutRejected   int
	AdjAccepted     int
	AdjRejected     int
	PublishedCards  int
	Partial         bool
	RequestBytes    int
	ResponseBytes   int
	InputTokens     int
	OutputTokens    int
	TransportAttempts int
	LatencyMillis   int64
}

// themeStudyClient mirrors the deepseek client seam for the two theme stages.
type themeStudyClient interface {
	ThemeScoutPromptJSON(themestudy.ScoutPrompt, int) ([]byte, error)
	ThemeScoutMeasured(context.Context, themestudy.ScoutPrompt, int) (modelresearch.ProviderResult, error)
	ThemeAdjudicationPromptJSON(themestudy.AdjudicationPrompt, int) ([]byte, error)
	ThemeAdjudicationMeasured(context.Context, themestudy.AdjudicationPrompt, int) (modelresearch.ProviderResult, error)
	EffectiveConfig() deepseek.EffectiveConfig
}

type themeStudyClientFactory func(requireCredentials bool) (themeStudyClient, error)

func defaultThemeStudyClientFactory(requireCredentials bool) (themeStudyClient, error) {
	if requireCredentials {
		return deepseek.NewFromEnv()
	}
	return deepseek.NewPromptFromEnv()
}

// runThemeStudyForRun owns the Decision 213 Study pipeline: exactly two
// semantic stages (Theme Scout → local source expansion → Theme
// Adjudication) followed by the local reducer, over the exact compiled
// substrate. The retired single-stage atlas-study provider call is gone: the
// run writes the eight theme artifacts, and the Study report section is
// derived from them by readAtlasStudyReportProduct.
//
// The caller supplies the same authority-confirmed, source-covered ReportData
// later used by final Generate. BuildAtlasStudyInput then reads only its
// Atlas, usable canonical visible Architecture Canvas and exact saved sources;
// Navigator is neither an input nor a prerequisite.
func runThemeStudyForRun(
	ctx context.Context,
	preparedData *report.ReportData,
	runDir string,
	runsDir string,
	analysisRoot string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	noCache bool,
	providerEnabled bool,
	language themestudy.Language,
	output *runOutput,
) (themeStudyRunOutcome, error) {
	if preparedData == nil {
		return themeStudyRunOutcome{}, fmt.Errorf("theme study run: authorized prepared report data is required")
	}
	input, err := report.BuildAtlasStudyInput(preparedData, languageFromReport(language))
	if err != nil {
		return themeStudyRunOutcome{}, fmt.Errorf("theme study run: build exact input: %w", err)
	}
	// The local compile is the exact seed producer (Decision 213 §2.5); it is
	// never followed by the retired single-stage provider call.
	product, err := atlasstudy.Compile(input)
	if err != nil {
		if outcome, handled, unavailableErr := themeStudyCandidateUnavailableOutcome(
			err, runDir, output,
		); handled {
			return outcome, unavailableErr
		}
		return themeStudyRunOutcome{}, themeTerminalResource(err, 0)
	}
	return runThemeStudyProductForRun(
		ctx, runDir, runsDir, analysisRoot, repository, policy, noCache, providerEnabled,
		input, product, language, preparedData, output, defaultThemeStudyClientFactory,
	)
}

func languageFromReport(language themestudy.Language) atlasstudy.Language {
	if language == themestudy.LanguageRussian {
		return atlasstudy.LanguageRussian
	}
	return atlasstudy.LanguageEnglish
}

// themeScoutSeedBudget derives the a* seed-pack source-byte budget from the
// Scout request artifact bound. The encoded wire shares MaxScoutRequestArtifactBytes
// with the names-only f* vocabulary, the compact context block and JSON
// structure; measured on the reference repository, ~26 KiB of base overhead
// plus ~2.8x JSON expansion of source lines. The budget leaves explicit
// headroom so a valid repository never overflows the artifact bound: excess
// seeds are reported under the closed seed_budget omission, never truncated
// silently and never a terminal failure (D190/D195).
func themeScoutSeedBudget(vocabularyFiles int) int {
	// ~200 bytes of names-only JSON per f* file, 1 KiB context, 16 KiB
	// structural/escape headroom.
	baseEstimate := 4<<10 + vocabularyFiles*200
	headroom := 16 << 10
	available := themestudy.MaxScoutRequestArtifactBytes - baseEstimate - headroom
	if available <= 0 {
		return 8 << 10
	}
	// Measured expansion: real source lines escape to ~2.8x in the request
	// wire. Use 3.0 so the encoded request always fits its artifact bound.
	return available / 3
}

func themeOutcomeFromScout(outcome themeScoutStageOutcome) themeStudyRunOutcome {
	return themeStudyRunOutcome{
		State: outcome.State, FailureCode: outcome.FailureCode,
		ProviderSkipped: false, Cached: outcome.Cached,
		ScoutAccepted: outcome.ScoutAccepted, ScoutRejected: outcome.ScoutRejected,
		RequestBytes: outcome.RequestBytes, ResponseBytes: outcome.ResponseBytes,
		InputTokens: outcome.InputTokens, OutputTokens: outcome.OutputTokens,
		TransportAttempts: outcome.TransportAttempts, LatencyMillis: outcome.LatencyMillis,
	}
}

// themeScoutContext builds the compact, bounded context block of the Scout
// wire from the compiled product: repository name plus the labels-only
// Architecture projection and the backend-owned span questions. It never
// carries source bytes, raw trees, or canonical identities.
func themeScoutContext(product atlasstudy.Product, repoName string) themestudy.ScoutContext {
	request, err := product.RequestRecord()
	if err != nil {
		return themestudy.ScoutContext{RepositoryName: repoName}
	}
	context := themestudy.ScoutContext{RepositoryName: repoName}
	for _, object := range request.Catalog {
		switch object.Kind {
		case atlasstudy.RefComponent:
			context.Architecture.ComponentNames = append(
				context.Architecture.ComponentNames, object.Label,
			)
		case atlasstudy.RefRouteSpan:
			context.SpanQuestions = append(context.SpanQuestions, themestudy.ScoutSpanQuestion{
				Kind: string(object.SupportRole), Question: object.Label,
			})
		}
	}
	return context
}

// themeStudyCandidateUnavailableOutcome handles the typed unavailable catalog.
func themeStudyCandidateUnavailableOutcome(
	err error,
	runDir string,
	output *runOutput,
) (themeStudyRunOutcome, bool, error) {
	var unavailable *atlasstudy.CandidateUnavailableError
	if !errors.As(err, &unavailable) {
		return themeStudyRunOutcome{}, false, nil
	}
	outcome := themeStudyRunOutcome{
		State: atlasstudy.ProductStateUnavailable, ProviderSkipped: true,
	}
	if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
		return outcome, true, cleanupErr
	}
	if output != nil {
		reason := "typed Study catalog is unavailable"
		if unavailable.Reason != "" {
			reason = unavailable.Reason
		}
		output.State("Study", "unavailable", "provider calls: 0", "reason: "+reason)
	}
	return outcome, true, nil
}

// runThemeStudyProductForRun executes the two semantic stages with the shared
// journaling/cache/secretscan discipline of the retired single-stage runtime:
// every accepted response is persisted and cache-written only after exact
// item-local validation, rejected responses never enter the cache, and
// resource-limit outcomes are terminal without cache/apply/fallback.
func runThemeStudyProductForRun(
	ctx context.Context,
	runDir string,
	runsDir string,
	analysisRoot string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	noCache bool,
	providerEnabled bool,
	input atlasstudy.Input,
	product atlasstudy.Product,
	language themestudy.Language,
	preparedData *report.ReportData,
	output *runOutput,
	clients themeStudyClientFactory,
) (themeStudyRunOutcome, error) {
	if output == nil {
		output = newRunOutput(io.Discard)
	}
	if clients == nil {
		return themeStudyRunOutcome{}, fmt.Errorf("theme study run: client factory is required")
	}
	if err := policy.Validate(); err != nil {
		return themeStudyRunOutcome{}, fmt.Errorf("theme study run: model policy: %w", err)
	}
	output.Stage("Study", "compiling Atlas-backed source-grounded theme seeds")
	if err := resetThemeStudyArtifacts(runDir); err != nil {
		return themeStudyRunOutcome{}, err
	}
	// The canonical theme artifacts carry bounded provider-evidence source
	// (seed packs, expansion bodies, wire JSON). They are gated by the
	// mandatory secret scan (themeUnsafePayload reject) before every persist,
	// so the writer must not redact-mangle them: the dump redaction fallback
	// would replace whole artifacts with "[redacted: ...]", corrupting their
	// JSON contract. The semantic journal redacts independently of this flag.
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return themeStudyRunOutcome{}, fmt.Errorf("theme study run: open confined artifact writer: %w", err)
	}
	defer writer.Close()
	writer.SetWarningWriter(runOutputWarningSink{
		output: output, summary: "Theme Study semantic exchange journal unavailable",
	})
	if !providerEnabled {
		outcome := themeStudyRunOutcome{
			State: atlasstudy.ProductStateUnavailable, ProviderSkipped: true,
		}
		output.State("Study", "unavailable", "provider calls: 0", "reason: offline requested")
		return outcome, nil
	}

	// Seed producer: flat names-only f* vocabulary over the eligible openable
	// paths, and bounded exact a* seed packs over the compiled substrate.
	vocabulary := themestudy.BuildFileVocabulary(
		preparedData.OpenablePaths, 0, func(string) bool { return true },
	)
	if len(vocabulary.Files) == 0 {
		outcome := themeStudyRunOutcome{
			State: atlasstudy.ProductStateUnavailable, ProviderSkipped: true,
		}
		output.State("Study", "unavailable", "provider calls: 0", "reason: no eligible source files")
		return outcome, nil
	}
	seedSpecs := themeSeedSpecsFromInput(input)
	reader := themeSourceReader(analysisRoot)
	// The seed-pack source-byte budget is derived from the Scout request
	// artifact bound: the encoded request shares the 384 KiB wire with the
	// names-only f* vocabulary, the compact context block and the JSON
	// structure, and real source lines expand ~3x once escaped and wrapped in
	// per-object JSON. A fixed fraction keeps the request always inside its
	// artifact bound on large repositories; excess seeds are reported under
	// the closed seed_budget omission (D190/D195: bounded, never silently
	// truncated, never a terminal failure for a valid repository).
	packs, err := themestudy.BuildSeedPacks(
		seedSpecs, 0, themeScoutSeedBudget(len(vocabulary.Files)), 0, 0, reader, themeTotalLines(analysisRoot),
	)
	if err != nil {
		return themeStudyRunOutcome{}, fmt.Errorf("theme study run: build seed packs: %w", err)
	}
	if len(packs.Packs) == 0 {
		outcome := themeStudyRunOutcome{
			State: atlasstudy.ProductStateUnavailable, ProviderSkipped: true,
		}
		output.State("Study", "unavailable", "provider calls: 0", "reason: no exact seed anchors")
		return outcome, nil
	}

	// Stage 1 — Theme Scout (contract C).
	scoutStage := runThemeScoutStage(
		ctx, runDir, runsDir, repository, policy, noCache,
		product, vocabulary, packs, language, preparedData.RepoName, writer, output, clients,
	)
	if scoutStage.err != nil {
		return themeOutcomeFromScout(scoutStage.outcome), scoutStage.err
	}
	scoutOutcome, scoutRequest, scoutResult := scoutStage.outcome, scoutStage.request, scoutStage.result
	if scoutOutcome.State != atlasstudy.ProductStateAccepted &&
		scoutOutcome.State != atlasstudy.ProductStateAcceptedPartial {
		return themeOutcomeFromScout(scoutOutcome), nil
	}
	outcome := themeStudyRunOutcome{
		State: scoutOutcome.State, Cached: scoutOutcome.Cached,
		SemanticCalls: 1, ScoutAccepted: scoutOutcome.ScoutAccepted,
		ScoutRejected: scoutOutcome.ScoutRejected,
		RequestBytes:  scoutOutcome.RequestBytes, ResponseBytes: scoutOutcome.ResponseBytes,
		InputTokens: scoutOutcome.InputTokens, OutputTokens: scoutOutcome.OutputTokens,
		TransportAttempts: scoutOutcome.TransportAttempts, LatencyMillis: scoutOutcome.LatencyMillis,
	}

	// Contract D — local source expansion (backend executes, never the model).
	// Only the f* refs the Scout candidates requested are expanded: the
	// expansion artifact must contain exactly the files the Adjudication
	// request references (contract D binding), not the first-N vocabulary
	// entries. Requested refs absent from the vocabulary resolve to nothing
	// and the binding check fails closed downstream.
	requested := themestudy.RefsForExpansion(scoutResult.Candidates)
	expansion, err := themestudy.ExpandFiles(
		themestudy.ExpansionFilesForRefs(vocabulary, requested), reader, themeTotalLines(analysisRoot),
	)
	if err != nil {
		return outcome, fmt.Errorf("theme study run: local source expansion: %w", err)
	}
	expansion.Requested = requested
	expansionBytes, err := themestudy.EncodeExpansion(expansion)
	if err != nil {
		return outcome, fmt.Errorf("theme study run: encode expansion: %w", err)
	}
	if unsafeErr := themeUnsafePayload("theme_source_expansion", expansionBytes); unsafeErr != nil {
		return outcome, unsafeErr
	}
	if err := writer.WriteValidatedFile(themestudy.ExpansionArtifactFilename, expansionBytes, func(saved []byte) error {
		decoded, decodeErr := themestudy.DecodeExpansion(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if !reflect.DeepEqual(decoded, expansion) {
			return fmt.Errorf("theme study expansion changed before publication")
		}
		return nil
	}); err != nil {
		return outcome, fmt.Errorf("theme study run: persist expansion: %w", err)
	}

	// Stage 2 — Theme Adjudication (contract E).
	anchors := themeAnchorInfo(input)
	adjStage := runThemeAdjudicationStage(
		ctx, runDir, runsDir, repository, policy, noCache,
		scoutRequest, scoutResult, expansion, anchors, language, writer, output, clients,
	)
	if adjStage.err != nil {
		outcome.SemanticCalls = 2
		return outcome, adjStage.err
	}
	adjOutcome, adjRequest, adjResult := adjStage.outcome, adjStage.request, adjStage.result
	if adjOutcome.State != atlasstudy.ProductStateAccepted &&
		adjOutcome.State != atlasstudy.ProductStateAcceptedPartial {
		// Adjudication failure after a successful Scout is honest: no shelf,
		// the neutral browse survives. Semantic calls stay at 2 (the second
		// stage ran and failed).
		outcome.SemanticCalls = 2
		outcome.State = adjOutcome.State
		outcome.FailureCode = adjOutcome.FailureCode
		outcome.AdjAccepted = adjOutcome.AdjAccepted
		outcome.AdjRejected = adjOutcome.AdjRejected
		return outcome, nil
	}
	outcome.SemanticCalls = 2
	outcome.State = adjOutcome.State
	outcome.AdjAccepted = adjOutcome.AdjAccepted
	outcome.AdjRejected = adjOutcome.AdjRejected
	if adjOutcome.Cached {
		outcome.Cached = true
	}

	// Local reducer (contract F) — deterministic, no retry, no re-ranking.
	reduction, err := themestudy.Reduce(themestudy.ReducerInput{
		Themes: adjResult.Themes, Candidates: themeCandidatesByRef(scoutResult.Candidates),
		Anchors: anchors,
	})
	if err != nil {
		return outcome, fmt.Errorf("theme study run: reducer: %w", err)
	}
	if len(reduction.Cards) == 0 {
		outcome.State = atlasstudy.ProductStateFailed
		outcome.FailureCode = atlasstudy.FailureValidation
		output.Warn("Study themes unavailable", "the local reducer accepted no valid theme", "local Atlas and Architecture remain available")
		return outcome, nil
	}
	themes := themestudy.StudyThemes{
		Version: "v1", ScoutSHA256: scoutRequest.CatalogSHA256,
		AdjSHA256: adjRequest.CatalogSHA256,
		Cards:     reduction.Cards, Omitted: reduction.Omitted,
		Partial: reduction.Partial, Diagnostics: reduction.Diagnostics,
	}
	themesBytes, err := themestudy.EncodeStudyThemes(themes)
	if err != nil {
		return outcome, fmt.Errorf("theme study run: encode study_themes: %w", err)
	}
	if unsafeErr := themeUnsafePayload("study_themes", themesBytes); unsafeErr != nil {
		return outcome, unsafeErr
	}
	if err := writer.WriteValidatedFile(themestudy.StudyThemesArtifactFilename, themesBytes, func(saved []byte) error {
		decoded, decodeErr := themestudy.DecodeStudyThemes(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if !reflect.DeepEqual(decoded, themes) {
			return fmt.Errorf("theme study_themes changed before publication")
		}
		return nil
	}); err != nil {
		return outcome, fmt.Errorf("theme study run: persist study_themes: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
			return outcome, errors.Join(ctxErr, cleanupErr)
		}
		return outcome, ctxErr
	}
	outcome.PublishedCards = len(themes.Cards)
	outcome.Partial = themes.Partial
	// The final state is partial when either semantic stage dropped an item:
	// a Scout-rejected sibling, an Adjudication-rejected theme, or a
	// reducer drop all mean the published shelf is a truthful subset of the
	// model's proposal. The report read path derives the same state from the
	// artifacts (scoutResult.State / adjResult.State / themes.Partial).
	if outcome.Partial || scoutOutcome.State == atlasstudy.ProductStateAcceptedPartial ||
		adjOutcome.State == atlasstudy.ProductStateAcceptedPartial {
		outcome.State = atlasstudy.ProductStateAcceptedPartial
	}
	output.State(
		"Study", string(outcome.State),
		fmt.Sprintf("theme cards: %d", outcome.PublishedCards),
		fmt.Sprintf("scout %d/%d · adjudication %d/%d",
			outcome.ScoutAccepted, outcome.ScoutAccepted+outcome.ScoutRejected,
			outcome.AdjAccepted, outcome.AdjAccepted+outcome.AdjRejected),
	)
	return outcome, nil
}

// themeSeedSpecsFromInput compiles the a* seed specs from the exact compiled
// substrate: one focused seed per reading target, bound to its canonical route
// span when the target is the sole allowed target of one span.
func themeSeedSpecsFromInput(input atlasstudy.Input) []themestudy.SeedSpec {
	spanByTarget := make(map[string]string)
	for _, span := range input.RouteSpans {
		if len(span.AllowedTargetIDs) != 1 {
			continue
		}
		spanByTarget[span.AllowedTargetIDs[0]] = span.ID
	}
	seeds := make([]themestudy.SeedSpec, 0, len(input.ReadingTargets))
	for index, target := range input.ReadingTargets {
		spec := themestudy.SeedSpec{
			Ref:        fmt.Sprintf("a%d", index+1),
			Path:       target.Location.Path,
			Line:       target.Location.Line,
			Symbol:     target.Symbol,
			Provenance: "d211_span_reading_target",
			Kind:       "focused",
		}
		if spanID, ok := spanByTarget[target.ID]; ok {
			spec.CanonicalSpanID = spanID
		}
		seeds = append(seeds, spec)
	}
	return seeds
}

// themeAnchorInfo compiles the exact a* anchor identities for the reducer
// from the compiled substrate (path/symbol/line + canonical span binding).
func themeAnchorInfo(input atlasstudy.Input) map[string]themestudy.AnchorInfo {
	spanByTarget := make(map[string]string)
	for _, span := range input.RouteSpans {
		if len(span.AllowedTargetIDs) != 1 {
			continue
		}
		spanByTarget[span.AllowedTargetIDs[0]] = span.ID
	}
	anchors := make(map[string]themestudy.AnchorInfo, len(input.ReadingTargets))
	for index, target := range input.ReadingTargets {
		info := themestudy.AnchorInfo{
			Path: target.Location.Path, Symbol: target.Symbol, Line: target.Location.Line,
		}
		if spanID, ok := spanByTarget[target.ID]; ok {
			info.CanonicalSpanID = spanID
		}
		anchors[fmt.Sprintf("a%d", index+1)] = info
	}
	return anchors
}

func themeCandidatesByRef(candidates []themestudy.ScoutCandidate) map[string]*themestudy.ScoutCandidate {
	byRef := make(map[string]*themestudy.ScoutCandidate, len(candidates))
	for index := range candidates {
		byRef[candidates[index].Ref] = &candidates[index]
	}
	return byRef
}

// themeSourceReader is a themestudy.SourceReader bound to the exact analysis
// root. It reads line windows from authorized repository files only; any
// path escape fails closed.
func themeSourceReader(analysisRoot string) themestudy.SourceReader {
	return func(path string, startLine, endLine int) ([]string, error) {
		if analysisRoot == "" || path == "" {
			return nil, fmt.Errorf("theme study run: source reader without analysis root")
		}
		if startLine < 1 || endLine < startLine {
			return nil, fmt.Errorf("theme study run: invalid line window %d..%d", startLine, endLine)
		}
		resolved, err := filepath.Abs(filepath.Join(analysisRoot, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		rootAbs, err := filepath.Abs(analysisRoot)
		if err != nil {
			return nil, err
		}
		if resolved != rootAbs && !strings.HasPrefix(resolved, rootAbs+string(filepath.Separator)) {
			return nil, fmt.Errorf("theme study run: source path escapes analysis root: %s", path)
		}
		lines, err := readFileLines(resolved, startLine, endLine)
		if err != nil {
			return nil, err
		}
		return lines, nil
	}
}

func themeTotalLines(analysisRoot string) themestudy.TotalLines {
	return func(path string) (int, error) {
		if analysisRoot == "" || path == "" {
			return 0, fmt.Errorf("theme study run: total lines without analysis root")
		}
		resolved := filepath.Join(analysisRoot, filepath.FromSlash(path))
		return countFileLines(resolved)
	}
}

func readFileLines(path string, startLine, endLine int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	lines := make([]string, 0, endLine-startLine+1)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if lineNumber < startLine {
			continue
		}
		if lineNumber > endLine {
			break
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func countFileLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func resetThemeStudyArtifacts(runDir string) error {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("theme study run: open run directory for reset: %w", err)
	}
	defer root.Close()
	for _, name := range themestudy.ThemeArtifactFilenames {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("theme study run: reset artifact %s: %w", name, err)
		}
	}
	return nil
}

func themeTerminalResource(err error, maxTokens int) error {
	var local *atlasstudy.ResourceLimitError
	if errors.As(err, &local) {
		details := modelresearch.ResourceLimitError{
			Stage: "theme_study", Kind: modelresearch.ResourceLimitRequestBytes,
			ConfiguredMaxTokens: maxTokens,
		}
		switch {
		case local.Section == "response_bytes":
			details.Kind = modelresearch.ResourceLimitResponseBytes
			details.Limit = local.Limit
			details.Observed = local.Actual
			details.ObservedKnown = true
		case strings.HasSuffix(local.Section, "_artifact_bytes"):
			details.Kind = modelresearch.ResourceLimitRecordBytes
			details.Limit = local.Limit
			details.Observed = local.Actual
			details.ObservedKnown = true
		}
		return modelresearch.NewResourceLimitError(details, nil)
	}
	// The themestudy artifact encoders report bounded-artifact overruns as
	// plain errors ("<name> artifact exceeds <limit> bytes"). Treat a
	// request-artifact overrun as the typed terminal resource outcome (D190:
	// bounded, never silently truncated; the run stops, it does not retry).
	if overrun := themeArtifactOverrun(err); overrun != nil {
		details := modelresearch.ResourceLimitError{
			Stage: "theme_study", Kind: modelresearch.ResourceLimitRecordBytes,
			ConfiguredMaxTokens: maxTokens,
			Limit:               overrun.limit, Observed: overrun.observed, ObservedKnown: true,
		}
		return modelresearch.NewResourceLimitError(details, nil)
	}
	return err
}

type themeArtifactOverrunInfo struct {
	limit    int
	observed int
}

func themeArtifactOverrun(err error) *themeArtifactOverrunInfo {
	if err == nil {
		return nil
	}
	message := err.Error()
	const marker = " artifact exceeds "
	index := strings.Index(message, marker)
	if index < 0 {
		return nil
	}
	var limit int
	if _, scanErr := fmt.Sscanf(message[index+len(marker):], "%d bytes", &limit); scanErr != nil {
		return nil
	}
	// The observed size is not in the message; report the limit with an
	// unknown observed size so the diagnostic stays truthful.
	return &themeArtifactOverrunInfo{limit: limit, observed: limit}
}

// themeStageCacheInput builds the accepted-only stage cache identity for one
// theme semantic stage. The fingerprint includes repository, stage,
// prompt version, cache contract, provider endpoint/model, the exact request
// body, the evidence bundle digest and the prose language (D154: language is
// part of the semantic request and cache key, never UI cosmetics).
func themeStageCacheInput(
	runsDir string,
	repository modelresearch.RepositoryContext,
	policy modelresearch.Policy,
	config deepseek.EffectiveConfig,
	endpointSHA string,
	stage string,
	cacheContract string,
	evidenceDigest string,
	language themestudy.Language,
	request []byte,
) modelresearch.StageCacheInput {
	return modelresearch.StageCacheInput{
		RunsDir: runsDir,
		Fingerprint: modelresearch.FingerprintInput{
			Repository: repository, Stage: stage,
			PromptVersion: cacheContract, CacheContract: cacheContract,
			Profile: "openai-compatible/" + config.AuthMode,
			Model:   config.Model, ProviderEndpointSHA256: endpointSHA,
			RequestSHA256:      modelresearch.SHA256(request),
			EvidenceBundleHash: evidenceDigest, PolicyVersion: policy.Version,
			OutputLanguage: string(language),
		},
		Request: request, EvidenceBundleHash: evidenceDigest,
	}
}
