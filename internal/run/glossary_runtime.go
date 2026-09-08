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
	"github.com/dvordrova/repomap/internal/terminology"
)

// The shared run collector contains accepted live/cache adjuncts from every
// analysis cube. Reduction happens once, before localization and publication.
func reduceReportGlossary(ctx context.Context, options repositoryTargetDispatchOptions, runDir string, data *report.ReportData) error {
	collector := options.Deps.terminology
	if collector == nil || options.NoModel {
		return nil
	}
	candidates := collector.Snapshot()
	if err := writeGlossaryArtifact(runDir, "terminology.json", candidates); err != nil {
		return err
	}
	factory := options.Deps.newDisplayProvider
	if factory == nil {
		return fmt.Errorf("glossary: base provider factory is missing")
	}
	provider, err := factory()
	if err != nil {
		return err
	}
	writer, err := debugdump.OpenWriter(runDir)
	if err != nil {
		return err
	}
	defer writer.Close()
	executor := debugdump.BindStage(llm.Executor{RootDir: options.DebugDir, Enabled: !options.NoCache,
		Observer:         timed(options.Output, debugdump.NewSemanticObserver(writer)),
		BatchConcurrency: options.Deps.llmBatchConcurrency, BatchController: options.Deps.llmBatchController,
	}, debugdump.SemanticStageGlossary)
	options.Output.Stage("Glossary", fmt.Sprintf("combining %d source-backed term explanations", len(candidates)))
	started := time.Now()
	catalog, err := terminology.Reduce(ctx, executor, provider, candidates)
	if err != nil {
		return fmt.Errorf("glossary: %w", err)
	}
	data.Glossary = &catalog
	if err := writeGlossaryArtifact(runDir, "glossary.json", catalog); err != nil {
		return err
	}
	options.Output.State("Glossary", "ready", fmt.Sprintf("%d entries", len(catalog.Entries)), formatRunOutputWallDuration(time.Since(started)))
	return nil
}

func writeGlossaryArtifact(runDir, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, name), append(raw, '\n'), 0o600)
}
