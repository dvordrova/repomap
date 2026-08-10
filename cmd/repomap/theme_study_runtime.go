package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// themeCacheContracts are the Decision 213 accepted-only stage cache
// contracts. New semantics — new contract names, never reinterpreted.
const (
	themeScoutCacheContract        = themestudy.ScoutCacheContract
	themeAdjudicationCacheContract = themestudy.AdjudicationCacheContract
	themeScoutCacheStage           = "theme_scout"
	themeAdjudicationCacheStage    = "theme_adjudication"
)

// themeStudyRunOutcome is the run-wiring summary of the two-stage theme
// pipeline (Decision 213). The retired single-stage atlas-study call no longer
// contributes a DirectionCount; the theme shelf is summarized by card counts.
type themeStudyRunOutcome struct {
	State             atlasstudy.ProductState
	FailureCode       atlasstudy.FailureCode
	ProviderSkipped   bool
	Cached            bool
	SemanticCalls     int
	ScoutAccepted     int
	ScoutRejected     int
	AdjAccepted       int
	AdjRejected       int
	PublishedCards    int
	CoProjected       int
	Partial           bool
	RequestBytes      int
	ResponseBytes     int
	InputTokens       int
	OutputTokens      int
	TransportAttempts int
	LatencyMillis     int64
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
// Atlas, usable canonical visible Architecture Canvas and exact saved sources.
func runThemeStudyForRun(
	ctx context.Context,
	preparedData *report.ReportData,
	analysisTarget *analysistarget.Target,
	directCallIndex *surfacediscovery.DirectCallIndex,
	targetRoots *analysistarget.TargetRoots,
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
	var libraryTarget analysistarget.Target
	var libraryRoots analysistarget.TargetRoots
	hasLibraryRootAuthority := false
	if preparedData.AnalysisTarget != nil &&
		(preparedData.AnalysisTarget.Kind == analysistarget.KindLibraryPackage ||
			preparedData.AnalysisTarget.Kind == analysistarget.KindModuleLibrary) {
		if analysisTarget == nil || directCallIndex == nil || targetRoots == nil ||
			analysisTarget.Ref != preparedData.AnalysisTarget.Ref {
			return themeStudyRunOutcome{}, fmt.Errorf("theme study run: selected library root authority is unavailable")
		}
		libraryTarget = analysisTarget.Snapshot()
		libraryRoots = targetRoots.Snapshot()
		if err := analysistarget.ValidateExactRoots(libraryTarget, directCallIndex, libraryRoots); err != nil {
			return themeStudyRunOutcome{}, fmt.Errorf("theme study run: validate selected library roots: %w", err)
		}
		hasLibraryRootAuthority = true
	}
	input, err := report.BuildAtlasStudyInput(preparedData, languageFromReport(language))
	if err != nil {
		return themeStudyRunOutcome{}, fmt.Errorf("theme study run: build exact input: %w", err)
	}
	if hasLibraryRootAuthority {
		if err := validateThemeLibraryRootLocators(input, libraryTarget, libraryRoots); err != nil {
			return themeStudyRunOutcome{}, fmt.Errorf("theme study run: bind selected library roots: %w", err)
		}
	}
	compileInput, closure, err := shapeThemeStudyCompileInput(input)
	if err != nil {
		return themeStudyRunOutcome{}, themeTerminalResource(err, 0)
	}
	output.ThemeInputClosure(closure)
	// The local compile is the exact seed producer (Decision 213 §2.5); it is
	// never followed by the retired single-stage provider call. The Theme-only
	// Atlas closure removes unreferenced package scaffold; the authoritative
	// saved Atlas and every semantic surface/target/span remain unchanged.
	product, err := atlasstudy.Compile(compileInput)
	if err != nil {
		if outcome, handled, unavailableErr := themeStudyCandidateUnavailableOutcome(
			err, runDir, output,
		); handled {
			return outcome, unavailableErr
		}
		return themeStudyRunOutcome{}, themeTerminalResource(err, 0)
	}
	providerInput, err := atlasstudy.SelectAnalysisTargetRootFrontier(compileInput)
	if err != nil {
		return themeStudyRunOutcome{}, themeTerminalResource(err, 0)
	}
	return runThemeStudyProductForRun(
		ctx, runDir, runsDir, analysisRoot, repository, policy, noCache, providerEnabled,
		providerInput, product, language, preparedData, output, defaultThemeStudyClientFactory,
	)
}

func validateThemeLibraryRootLocators(
	input atlasstudy.Input,
	target analysistarget.Target,
	roots analysistarget.TargetRoots,
) error {
	if input.AnalysisTargetRoot == nil ||
		input.AnalysisTargetRoot.AnalysisTarget.Ref != target.Ref ||
		roots.TargetRef != target.Ref || roots.OmittedRoots != 0 {
		return fmt.Errorf("selected AnalysisTarget identity or root accounting mismatch")
	}
	rootTargets := make(map[string]struct{})
	for _, support := range input.ReadingSupports {
		if support.Role == atlasstudy.SupportAnalysisTargetRoot {
			rootTargets[support.TargetID] = struct{}{}
		}
	}
	advertised := make(map[string]struct{}, len(rootTargets))
	for _, reading := range input.ReadingTargets {
		if _, root := rootTargets[reading.ID]; !root {
			continue
		}
		key := fmt.Sprintf("%s\x00%d", reading.Location.Path, reading.Location.Line)
		if _, duplicate := advertised[key]; duplicate {
			return fmt.Errorf("public API reading locator is ambiguous")
		}
		advertised[key] = struct{}{}
	}
	if len(advertised) != len(rootTargets) {
		return fmt.Errorf("public API reading roots are incomplete")
	}
	live := make(map[string]struct{}, len(roots.Roots))
	rootPackages := make(map[string]struct{})
	for _, pkg := range target.RootPackages() {
		rootPackages[pkg.PackagePath] = struct{}{}
	}
	for _, root := range roots.Roots {
		if _, selected := rootPackages[root.Package]; !selected {
			return fmt.Errorf("live root package does not match selected target")
		}
		key := fmt.Sprintf("%s\x00%d", root.Path, root.Line)
		if _, duplicate := live[key]; duplicate {
			return fmt.Errorf("live public API root locator is ambiguous")
		}
		live[key] = struct{}{}
	}
	if len(live) != len(advertised) {
		return fmt.Errorf("live and advertised public API root counts differ")
	}
	for key := range advertised {
		if _, exact := live[key]; !exact {
			return fmt.Errorf("advertised public API locator has no exact live root")
		}
	}
	return nil
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
	combined := themeStudyRunOutcome{
		State: outcome.State, FailureCode: outcome.FailureCode,
		ProviderSkipped: false, Cached: outcome.Cached,
		ScoutAccepted: outcome.ScoutAccepted, ScoutRejected: outcome.ScoutRejected,
		SemanticCalls: outcome.SemanticCalls,
	}
	if outcome.SemanticCalls > 0 {
		combined.RequestBytes = outcome.RequestBytes
		combined.ResponseBytes = outcome.ResponseBytes
		combined.InputTokens = outcome.InputTokens
		combined.OutputTokens = outcome.OutputTokens
		combined.TransportAttempts = outcome.TransportAttempts
		combined.LatencyMillis = outcome.LatencyMillis
	}
	return combined
}

// themeOutcomeWithAdjudication adds only provider work performed by the
// current run. Cache records retain historical response metadata so they can
// be revalidated, but those bytes, tokens and latency are not current external
// traffic. A combined run is a cache hit only when both reached semantic
// stages were cache hits.
func themeOutcomeWithAdjudication(
	outcome themeStudyRunOutcome,
	adjudication themeAdjStageOutcome,
) themeStudyRunOutcome {
	outcome.Cached = outcome.Cached && adjudication.Cached
	outcome.SemanticCalls += adjudication.SemanticCalls
	outcome.AdjAccepted = adjudication.AdjAccepted
	outcome.AdjRejected = adjudication.AdjRejected
	if adjudication.SemanticCalls > 0 {
		outcome.RequestBytes += adjudication.RequestBytes
		outcome.ResponseBytes += adjudication.ResponseBytes
		outcome.InputTokens += adjudication.InputTokens
		outcome.OutputTokens += adjudication.OutputTokens
		outcome.TransportAttempts += adjudication.TransportAttempts
		outcome.LatencyMillis += adjudication.LatencyMillis
	}
	return outcome
}

// themeScoutContext builds the compact, bounded context block of the Scout
// wire from the compiled product: repository name plus the labels-only
// Architecture projection and the backend-owned span questions. It never
// carries source bytes, raw trees, or canonical identities. Phase 3
// validation audit: each span question carries the exact a* anchor ref it
// compiled to (spanAnchorRefs) so the model never has to guess which
// anchor a backend-owned question belongs to.
func themeScoutContext(product atlasstudy.Product, repoName string, spanAnchorRefs map[string]string) themestudy.ScoutContext {
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
			// Decision 235 (v11): span questions are backend-owned — read
			// the span's real question and kind, never the empty catalog
			// label. A span without a backend-owned question is omitted
			// entirely (no placeholder objects: corpus had 712/712 empty).
			if strings.TrimSpace(object.Question) == "" {
				continue
			}
			question := themestudy.ScoutSpanQuestion{
				Kind: string(object.SpanKind), Question: object.Question,
			}
			if ref, ok := spanAnchorRefs[object.CanonicalID]; ok {
				question.AnchorRef = ref
			}
			context.SpanQuestions = append(context.SpanQuestions, question)
		}
	}
	return context
}

// themeSpanAnchorRefsFromPacks compiles the span-ID → a* anchor-ref binding
// from the accepted seed packs (each pack's seed carries its canonical span
// identity). A span that owns exactly one reading target binds to that
// anchor; multi-target spans have no single anchor and are left unbounded
// (the model still sees the question, just without a forced anchor).
func themeSpanAnchorRefsFromPacks(packs themestudy.SeedPackResult) map[string]string {
	binding := make(map[string]string)
	for _, pack := range packs.Packs {
		if pack.Seed.CanonicalSpanID == "" {
			continue
		}
		if _, taken := binding[pack.Seed.CanonicalSpanID]; !taken {
			binding[pack.Seed.CanonicalSpanID] = pack.Seed.Ref
		}
	}
	return binding
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
	readingRolesByPath := themeReadingRolesByPath(input)
	vocabulary := themestudy.BuildFileVocabularyWithRoles(
		preparedData.OpenablePaths, 0, func(string) bool { return true },
		func(path string) themestudy.Role { return themestudy.Role(readingRolesByPath[path]) },
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
	outcome := themeOutcomeFromScout(scoutOutcome)

	// Contract D — local source expansion (backend executes, never the model).
	// Only the f* refs the Scout candidates requested are expanded: the
	// expansion artifact must contain exactly the files the Adjudication
	// request references (contract D binding), not the first-N vocabulary
	// entries. Requested refs absent from the vocabulary resolve to nothing
	// and the binding check fails closed downstream.
	requested := themestudy.RefsForExpansion(scoutResult.Candidates)
	expansion, err := prepareThemeSourceExpansionForRun(
		themestudy.ExpansionFilesForRefs(vocabulary, requested), requested,
		themeExpansionSourceReader(analysisRoot), themeTotalLines(analysisRoot), writer, output,
	)
	if err != nil {
		return outcome, err
	}

	// Stage 2 — Theme Adjudication (contract E).
	anchors := themeAnchorInfo(input)
	adjStage := runThemeAdjudicationStage(
		ctx, runDir, runsDir, repository, policy, noCache,
		scoutRequest, scoutResult, expansion, anchors, language, writer, output, clients,
	)
	outcome = themeOutcomeWithAdjudication(outcome, adjStage.outcome)
	if adjStage.err != nil {
		// Ordinary provider and response-validation failures return err=nil
		// only after persisting their typed terminal status. A remaining
		// error means that durable closure itself failed (or a mandatory
		// integrity/resource gate fired), so attempting report publication
		// would turn an accepted Scout prefix into a false complete product.
		return outcome, adjStage.err
	}
	adjOutcome, adjRequest, adjResult := adjStage.outcome, adjStage.request, adjStage.result
	if adjOutcome.State != atlasstudy.ProductStateAccepted &&
		adjOutcome.State != atlasstudy.ProductStateAcceptedPartial {
		// Adjudication failure after a successful Scout is honest: no shelf,
		// the neutral browse survives. Provider accounting reflects whether
		// each stage was live or served by its accepted-only cache.
		outcome.State = adjOutcome.State
		outcome.FailureCode = adjOutcome.FailureCode
		return outcome, nil
	}
	outcome.State = adjOutcome.State

	// Local reducer (contract F) — deterministic, no retry, no re-ranking.
	reduction, err := themestudy.Reduce(themestudy.ReducerInput{
		Themes: adjResult.Themes, Candidates: themeCandidatesByRef(scoutResult.Candidates),
		Anchors: anchors,
	})
	if err != nil {
		// A reducer failure must not leave accepted_partial scout/adjudication
		// status records behind without the themes artifact: the report read
		// path would demand the missing artifact and the run would fail late.
		if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
			return outcome, errors.Join(
				fmt.Errorf("theme study run: reducer: %w", err),
				cleanupErr,
			)
		}
		return outcome, fmt.Errorf("theme study run: reducer: %w", err)
	}
	if len(reduction.Cards) == 0 {
		// The published scout/adjudication status records already claim an
		// accepted_partial state; a reducer rejection must reset the theme
		// artifact set so the report read path sees a consistent neutral
		// browse instead of demanding the missing themes artifact.
		if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
			return outcome, errors.Join(
				fmt.Errorf("theme study run: reducer accepted no valid theme: %w", err),
				cleanupErr,
			)
		}
		outcome.State = atlasstudy.ProductStateFailed
		outcome.FailureCode = atlasstudy.FailureValidation
		output.Warn("Study themes unavailable", "the local reducer accepted no valid theme", "local Atlas and Architecture remain available")
		return outcome, nil
	}
	themes := themestudy.StudyThemes{
		// Decision 233: StudyThemesVersion 2 (alternate co-projection +
		// concentration diagnostic); Decision 235: version 3 (theme
		// equivalence accounting); Decision 246: version 5 binds the exact
		// repository revision. The literal must track the constant.
		Version: themestudy.StudyThemesVersion, Revision: repository.Revision,
		ScoutSHA256: scoutRequest.CatalogSHA256,
		AdjSHA256:   adjRequest.CatalogSHA256,
		Cards:       reduction.Cards, Omitted: reduction.Omitted,
		CoProjected: reduction.CoProjected, Partial: reduction.Partial,
		Diagnostics: reduction.Diagnostics,
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
	outcome.CoProjected = themes.CoProjected
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
	output.State("Study", string(outcome.State), themeStudyCompletionDetails(outcome)...)
	return outcome, nil
}

func prepareThemeSourceExpansionForRun(
	files []themestudy.FileRef,
	requested []string,
	reader themestudy.SourceReader,
	totalLines themestudy.TotalLines,
	writer *debugdump.Writer,
	output *runOutput,
) (themestudy.SourceExpansion, error) {
	if writer == nil {
		return themestudy.SourceExpansion{}, fmt.Errorf("theme study run: source expansion writer is required")
	}
	expansion, err := themestudy.ExpandFiles(files, reader, totalLines)
	if err != nil {
		return themestudy.SourceExpansion{}, fmt.Errorf("theme study run: local source expansion: %w", err)
	}
	expansion.Requested = append([]string(nil), requested...)
	// Decision 235 (v11) 1D maddy: the mandatory secret scan is
	// partitioned per file — an unsafe file is closed with a typed reason
	// (never echoing content), safe files and the accepted Scout result
	// survive. The whole-payload scan remains as the final net after
	// closure so nothing unsafe is ever persisted.
	for index := range expansion.Files {
		file := &expansion.Files[index]
		if len(file.Objects) == 0 {
			continue
		}
		payloadBytes, err := json.Marshal(file.Objects)
		if err != nil {
			return themestudy.SourceExpansion{}, fmt.Errorf("theme study run: encode expansion file: %w", err)
		}
		if kind, found := secretscan.DetectSourceMaterial(string(payloadBytes)); found {
			if file.ExpandedLines > expansion.ExpandedLines {
				return themestudy.SourceExpansion{}, fmt.Errorf("theme study run: source expansion line accounting is invalid")
			}
			expansion.ExpandedLines -= file.ExpandedLines
			for _, object := range file.Objects {
				objectBytes := themeSourceObjectBytes(object)
				if objectBytes > expansion.ExpandedBytes {
					return themestudy.SourceExpansion{}, fmt.Errorf("theme study run: source expansion byte accounting is invalid")
				}
				expansion.ExpandedBytes -= objectBytes
			}
			file.Objects = nil
			file.Omitted = nil
			file.ExpandedLines = 0
			file.Closed = true
			file.ClosedReason = "secret_scan:" + string(secretscan.ClosedKind(kind))
			expansion.OmittedRefs = append(expansion.OmittedRefs, file.Ref)
		}
	}
	expansionBytes, err := themestudy.EncodeExpansion(expansion)
	if err != nil {
		return themestudy.SourceExpansion{}, fmt.Errorf("theme study run: encode expansion: %w", err)
	}
	if unsafeErr := themeUnsafeSourcePayload("theme_source_expansion", expansionBytes); unsafeErr != nil {
		return themestudy.SourceExpansion{}, unsafeErr
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
		return themestudy.SourceExpansion{}, fmt.Errorf("theme study run: persist expansion: %w", err)
	}
	warnThemeSourceExpansionClosures(output, expansion)
	return expansion, nil
}

func themeSourceObjectBytes(object themestudy.SourceObject) int {
	total := 0
	for _, line := range object.Lines {
		total += len(line)
	}
	return total
}

func warnThemeSourceExpansionClosures(output *runOutput, expansion themestudy.SourceExpansion) {
	if output == nil {
		return
	}
	refs := make(map[string]struct{}, len(expansion.OmittedRefs))
	for _, ref := range expansion.OmittedRefs {
		if ref != "" {
			refs[ref] = struct{}{}
		}
	}
	if len(refs) == 0 {
		return
	}
	output.Warn(
		"Some requested Study source context was skipped",
		fmt.Sprintf("source files skipped: %d", len(refs)),
		"unreadable, invalid, unsafe, or over-budget source stays excluded",
		"remaining exact source context continues",
	)
}

func themeStudyCompletionDetails(outcome themeStudyRunOutcome) []string {
	details := []string{
		fmt.Sprintf("theme cards: %d", outcome.PublishedCards),
		fmt.Sprintf("scout %d/%d · adjudication %d/%d",
			outcome.ScoutAccepted, outcome.ScoutAccepted+outcome.ScoutRejected,
			outcome.AdjAccepted, outcome.AdjAccepted+outcome.AdjRejected),
		fmt.Sprintf("provider calls: %d · transport attempts: %d", outcome.SemanticCalls, outcome.TransportAttempts),
		fmt.Sprintf("request/response bytes: %d/%d", outcome.RequestBytes, outcome.ResponseBytes),
		formatRunOutputTokens(outcome.InputTokens, outcome.OutputTokens),
		formatRunOutputDuration(outcome.LatencyMillis),
	}
	if outcome.CoProjected > 0 {
		details = append(details, fmt.Sprintf(
			"equivalent themes merged into existing cards: %d · alternate readings retained",
			outcome.CoProjected,
		))
	}
	return details
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
	orderedTargets := themeOrderedReadingTargets(input)
	seeds := make([]themestudy.SeedSpec, 0, len(orderedTargets))
	for index, target := range orderedTargets {
		spec := themestudy.SeedSpec{
			Ref:        fmt.Sprintf("a%d", index+1),
			Path:       target.Location.Path,
			Line:       target.Location.Line,
			Symbol:     target.Symbol,
			Provenance: "d211_span_reading_target",
			Kind:       "focused",
			Role:       themestudy.Role(themeReadingTargetRole(input, target)),
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
	orderedTargets := themeOrderedReadingTargets(input)
	anchors := make(map[string]themestudy.AnchorInfo, len(orderedTargets))
	for index, target := range orderedTargets {
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

// themeReadingTargetRole projects only producer-owned generic evidence into
// the shared artifact-role vocabulary. Target names and repository words are
// deliberately irrelevant.
func themeReadingTargetRole(input atlasstudy.Input, target atlasstudy.ReadingTarget) artifactrole.Role {
	hints := artifactrole.Hints{
		PrimaryEntry:  target.Kind == atlasstudy.ReadingTargetEntrypoint,
		Documentation: target.Kind == atlasstudy.ReadingTargetDocument,
	}
	for _, support := range input.ReadingSupports {
		if support.TargetID != target.ID {
			continue
		}
		switch support.Role {
		case atlasstudy.SupportProcessEntry:
			hints.PrimaryEntry = true
		case atlasstudy.SupportAnalysisTargetRoot:
			hints.PublicAPI = true
		}
	}
	return artifactrole.Classify(target.Location.Path, hints)
}

// themeOrderedReadingTargets makes the seed budget production-aware before
// refs are allocated. Stable exact locator/identity ties preserve replayable
// ordering without a repository-specific vocabulary.
func themeOrderedReadingTargets(input atlasstudy.Input) []atlasstudy.ReadingTarget {
	ordered := append([]atlasstudy.ReadingTarget(nil), input.ReadingTargets...)
	if input.AnalysisTargetRoot != nil {
		result, err := atlasstudy.OrderAnalysisTargetRootReadingTargets(input)
		if err != nil {
			return nil
		}
		return result
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		leftRole := themeReadingTargetRole(input, ordered[i])
		rightRole := themeReadingTargetRole(input, ordered[j])
		leftPriority := artifactrole.SelectionPriority(leftRole)
		rightPriority := artifactrole.SelectionPriority(rightRole)
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		if ordered[i].Location.Path != ordered[j].Location.Path {
			return artifactrole.LessPath(ordered[i].Location.Path, ordered[j].Location.Path, leftRole)
		}
		if ordered[i].Location.Line != ordered[j].Location.Line {
			return ordered[i].Location.Line < ordered[j].Location.Line
		}
		if ordered[i].Symbol != ordered[j].Symbol {
			return ordered[i].Symbol < ordered[j].Symbol
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

func themeReadingRolesByPath(input atlasstudy.Input) map[string]artifactrole.Role {
	roles := make(map[string]artifactrole.Role)
	for _, target := range input.ReadingTargets {
		role := themeReadingTargetRole(input, target)
		current, found := roles[target.Location.Path]
		if !found || artifactrole.SelectionPriority(role) > artifactrole.SelectionPriority(current) {
			roles[target.Location.Path] = role
		}
	}
	return roles
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
		if startLine < 1 || endLine < startLine {
			return nil, fmt.Errorf("theme study run: invalid line window %d..%d", startLine, endLine)
		}
		resolved, err := resolveThemeSourcePath(analysisRoot, path)
		if err != nil {
			return nil, err
		}
		lines, err := readFileLines(resolved, startLine, endLine)
		if err != nil {
			return nil, err
		}
		return lines, nil
	}
}

// themeExpansionSourceReader applies the f* per-file byte bound while reading,
// so a generated multi-megabyte line is closed without first retaining the
// entire line. Seed-pack reads keep their separate existing object bounds.
func themeExpansionSourceReader(analysisRoot string) themestudy.SourceReader {
	return func(path string, startLine, endLine int) ([]string, error) {
		if startLine < 1 || endLine < startLine {
			return nil, fmt.Errorf("theme study run: invalid line window %d..%d", startLine, endLine)
		}
		resolved, err := resolveThemeSourcePath(analysisRoot, path)
		if err != nil {
			return nil, err
		}
		return readFileLinesBounded(
			resolved, startLine, endLine, themestudy.MaxExpansionFileBytes,
		)
	}
}

func resolveThemeSourcePath(analysisRoot, path string) (string, error) {
	if analysisRoot == "" || path == "" {
		return "", fmt.Errorf("theme study run: source reader without analysis root")
	}
	resolved, err := filepath.Abs(filepath.Join(analysisRoot, filepath.FromSlash(path)))
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(analysisRoot)
	if err != nil {
		return "", err
	}
	if resolved != rootAbs && !strings.HasPrefix(resolved, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("theme study run: source path escapes analysis root: %s", path)
	}
	return resolved, nil
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
	// bufio.Reader.ReadBytes has no token-size ceiling (unlike bufio.Scanner),
	// so generated single-line files (statik-style embedded payloads) read
	// correctly; the encoded-expansion budget downstream bounds what is kept.
	reader := bufio.NewReader(file)
	lines := make([]string, 0, endLine-startLine+1)
	lineNumber := 0
	for {
		raw, readErr := reader.ReadBytes('\n')
		if len(raw) > 0 {
			lineNumber++
			if lineNumber >= startLine && lineNumber <= endLine {
				lines = append(lines, strings.TrimSuffix(string(raw), "\n"))
			}
			if lineNumber > endLine {
				break
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
	}
	return lines, nil
}

func readFileLinesBounded(path string, startLine, endLine, byteLimit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if byteLimit <= 0 {
		return nil, themestudy.ErrExpansionSourceOversized
	}

	reader := bufio.NewReader(file)
	lines := make([]string, 0, endLine-startLine+1)
	lineNumber := 0
	selectedBytes := 0
	var selectedLine []byte
	for {
		fragment, readErr := reader.ReadSlice('\n')
		if len(fragment) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		selected := lineNumber+1 >= startLine && lineNumber+1 <= endLine
		if selected {
			payload := fragment
			if !errors.Is(readErr, bufio.ErrBufferFull) && len(payload) > 0 && payload[len(payload)-1] == '\n' {
				payload = payload[:len(payload)-1]
			}
			if selectedBytes+len(payload) > byteLimit {
				return nil, themestudy.ErrExpansionSourceOversized
			}
			selectedLine = append(selectedLine, payload...)
			selectedBytes += len(payload)
		}
		if errors.Is(readErr, bufio.ErrBufferFull) {
			continue
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, readErr
		}
		lineNumber++
		if selected {
			lines = append(lines, string(selectedLine))
			selectedLine = nil
		}
		if lineNumber >= endLine {
			break
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	return lines, nil
}

func countFileLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	// Byte counting is independent of line length — a generated file with
	// one multi-megabyte line is one line, never a Scanner token error.
	count := 0
	totalBytes := int64(0)
	chunk := make([]byte, 64<<10)
	lastWasNewline := false
	for {
		n, readErr := file.Read(chunk)
		if n > 0 {
			totalBytes += int64(n)
			count += bytes.Count(chunk[:n], []byte{'\n'})
			lastWasNewline = chunk[n-1] == '\n'
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return 0, readErr
		}
	}
	if totalBytes > 0 && !lastWasNewline {
		count++
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
			Stage: "theme_study", Kind: modelresearch.ResourceLimitCatalogItems,
			ConfiguredMaxTokens: maxTokens,
			Limit:               local.Limit, Observed: local.Actual, ObservedKnown: true,
		}
		switch {
		case local.Section == "wire_bytes":
			details.Kind = modelresearch.ResourceLimitRequestBytes
		case local.Section == "response_bytes":
			details.Kind = modelresearch.ResourceLimitResponseBytes
		case strings.HasSuffix(local.Section, "_artifact_bytes"):
			details.Kind = modelresearch.ResourceLimitRecordBytes
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
