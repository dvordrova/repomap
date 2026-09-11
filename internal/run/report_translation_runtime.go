package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
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
				options.Output.Stage("", fmt.Sprintf("retrying only unfinished translations in %d smaller requests; completed translations are kept", windows))
			} else {
				options.Output.Stage("", fmt.Sprintf("request windows this round: %d; checking cache before provider calls", windows))
			}
			translationRound++
		},
	}, debugdump.SemanticStageReportTranslation)
	options.Output.Stage("Report translation", fmt.Sprintf("translating %d display texts into %s", len(catalog.Entries), language))
	options.Output.Stage("", fmt.Sprintf("up to %d parallel requests; HTTP 500 or a 4m attempt timeout splits complete texts into smaller requests", max(1, options.Deps.llmBatchConcurrency)))
	started := time.Now()
	translations, untranslated, err := reporttranslation.Translate(ctx, executor, provider, catalog, language)
	if err != nil {
		options.Output.State("Report translation", "failed", formatRunOutputWallDuration(time.Since(started)), options.Output.modelCallSummary(reporttranslation.StageName))
		return report.RenderOptions{}, fmt.Errorf("report translation: %w", err)
	}
	details := []string{formatRunOutputWallDuration(time.Since(started)), options.Output.modelCallSummary(reporttranslation.StageName)}
	if len(untranslated) > 0 {
		details = append(details, recordUntranslatedTexts(options.Output, runDir, untranslated))
	}
	options.Output.State("Report translation", "ready", details...)
	renderOptions.Translations = &translations
	return renderOptions, nil
}

// recordUntranslatedTexts journals every text the translation kept in its
// source language beside the stage's other rejections, one row per text with
// its final refusal, and returns the one console line that names them.
func recordUntranslatedTexts(output *runOutput, runDir string, untranslated []reporttranslation.Untranslated) string {
	rows := make([]modeldiag.Row, 0, len(untranslated))
	refs := make([]string, 0, len(untranslated))
	for _, entry := range untranslated {
		rows = append(rows, modeldiag.Row{
			Stage: reporttranslation.StageName, Kind: "entry_untranslated", Count: 1,
			Samples: []string{entry.Ref}, Reason: entry.Reason,
		})
		refs = append(refs, entry.Ref)
	}
	if err := modeldiag.Append(runDir, rows); err != nil {
		output.Warn("could not record report translation diagnostics", err.Error())
	}
	noun := "texts"
	if len(refs) == 1 {
		noun = "text"
	}
	const shown = 10
	listed := strings.Join(refs, ", ")
	if len(refs) > shown {
		listed = fmt.Sprintf("%s … and %d more in %s", strings.Join(refs[:shown], ", "), len(refs)-shown, modeldiag.Filename)
	}
	return fmt.Sprintf("%d %s kept in the source language: %s", len(refs), noun, listed)
}
