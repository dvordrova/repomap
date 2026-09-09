package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reporttranslation"
)

// translateReportDisplay runs only after the English repository report has
// been assembled. Its output is a display artifact; analysis stays untouched.
func translateReportDisplay(ctx context.Context, options repositoryTargetDispatchOptions, runDir string, data *report.ReportData) (report.RenderOptions, error) {
	language, err := report.NormalizeDisplayLanguage(string(options.DisplayLanguage))
	if err != nil {
		return report.RenderOptions{}, err
	}
	renderOptions := report.RenderOptions{Language: language, NoModel: options.NoModel}
	if language == report.English {
		return renderOptions, nil
	}
	prepared, err := report.PreparePage(data, renderOptions)
	if err != nil {
		return report.RenderOptions{}, err
	}
	catalog := prepared.TextCatalog()
	raw, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return report.RenderOptions{}, err
	}
	if err := os.WriteFile(filepath.Join(runDir, "report-display-texts.json"), append(raw, '\n'), 0o600); err != nil {
		return report.RenderOptions{}, err
	}
	translations := report.DisplayTranslations{Version: report.DisplayTextVersion, Language: language,
		CatalogSHA256: catalog.SHA256, Entries: []report.DisplayTranslationEntry{}}
	if options.NoModel || len(catalog.Entries) == 0 {
		if err := translations.Validate(catalog); err != nil {
			return report.RenderOptions{}, err
		}
		renderOptions.Translations = &translations
		return renderOptions, nil
	}
	factory := options.Deps.newDisplayProvider
	if factory == nil {
		factory = options.Deps.newCubeProvider
	}
	if factory == nil {
		return report.RenderOptions{}, fmt.Errorf("report translation: model provider is unavailable")
	}
	provider, err := factory()
	if err != nil {
		return report.RenderOptions{}, err
	}
	writer, err := debugdump.OpenWriter(runDir)
	if err != nil {
		return report.RenderOptions{}, err
	}
	defer writer.Close()
	translationRound := 0
	executor := debugdump.BindStage(llm.Executor{
		RootDir: options.DebugDir, Enabled: !options.NoCache,
		Observer:         timed(options.Output, debugdump.NewSemanticObserver(writer)),
		BatchConcurrency: options.Deps.llmBatchConcurrency, BatchController: options.Deps.llmBatchController,
		PlanNotice: func(windows int) {
			if translationRound > 0 {
				options.Output.Stage("", fmt.Sprintf("continuing automatically in %d smaller requests after the previous request timed out or its response was refused", windows))
			} else {
				options.Output.Stage("", fmt.Sprintf("request windows this round: %d; checking cache before provider calls", windows))
			}
			translationRound++
		},
	}, debugdump.SemanticStageReportTranslation)
	options.Output.Stage("Report translation", fmt.Sprintf("translating %d display texts into %s", len(catalog.Entries), language))
	options.Output.Stage("", fmt.Sprintf("up to %d parallel requests; a 4m attempt timeout splits complete texts into smaller requests", max(1, options.Deps.llmBatchConcurrency)))
	started := time.Now()
	translations, err = reporttranslation.Translate(ctx, executor, provider, catalog, language)
	if err != nil {
		options.Output.State("Report translation", "failed", formatRunOutputWallDuration(time.Since(started)), options.Output.modelCallSummary(reporttranslation.StageName))
		return report.RenderOptions{}, fmt.Errorf("report translation: %w", err)
	}
	options.Output.State("Report translation", "ready", formatRunOutputWallDuration(time.Since(started)), options.Output.modelCallSummary(reporttranslation.StageName))
	renderOptions.Translations = &translations
	return renderOptions, nil
}
