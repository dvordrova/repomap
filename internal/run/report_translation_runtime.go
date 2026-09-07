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
	if options.Deps.newCubeProvider == nil {
		return report.RenderOptions{}, fmt.Errorf("report translation: model provider is unavailable")
	}
	provider, err := options.Deps.newCubeProvider()
	if err != nil {
		return report.RenderOptions{}, err
	}
	writer, err := debugdump.OpenWriter(runDir)
	if err != nil {
		return report.RenderOptions{}, err
	}
	defer writer.Close()
	executor := debugdump.BindStage(llm.Executor{
		RootDir: options.DebugDir, Enabled: !options.NoCache,
		Observer:         timed(options.Output, debugdump.NewSemanticObserver(writer)),
		BatchConcurrency: options.Deps.llmBatchConcurrency, BatchController: options.Deps.llmBatchController,
	}, debugdump.SemanticStageReportTranslation)
	options.Output.Stage("Report translation", fmt.Sprintf("translating %d display texts into %s", len(catalog.Entries), language))
	started := time.Now()
	translations, err = reporttranslation.Translate(ctx, executor, provider, catalog, language)
	if err != nil {
		return report.RenderOptions{}, fmt.Errorf("report translation: %w", err)
	}
	options.Output.State("Report translation", "ready", formatRunOutputWallDuration(time.Since(started)))
	renderOptions.Translations = &translations
	return renderOptions, nil
}
