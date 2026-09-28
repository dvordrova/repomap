package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/terminology"
)

// The shared run collector contains accepted live/cache adjuncts from every
// analysis cube. Reduction happens once, before localization and publication.
// indexes are the sealed ProgramIndexes of every published target.
func reduceReportGlossary(ctx context.Context, options repositoryTargetDispatchOptions, runDir string, data *report.ReportData, indexes []programindex.Index) error {
	collector := options.Deps.terminology
	if collector == nil || options.NoModel {
		return nil
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
	options.Output.Stage("Glossary", "explaining unfamiliar names from accepted prose")
	progress := func(state, detail string) { options.Output.State("Glossary", state, detail) }
	collector.Progress = progress
	for kind, names := range glossaryCodeNames(indexes, data.Facts) {
		collector.ExcludeCodeNames(kind, names...)
	}
	// The report's own summary of the program is the shared context of every
	// name's closed decision.
	program := ""
	if data.Orientation != nil {
		program = data.Orientation.Summary
	}
	if err := collector.Generate(ctx, executor, provider, options.Categorizer, program); err != nil {
		return fmt.Errorf("glossary: %w", err)
	}
	candidates := collector.Snapshot()
	if err := writeGlossaryArtifact(runDir, "terminology.json", candidates); err != nil {
		return err
	}
	if err := writeGlossaryArtifact(runDir, "glossary_names.json", collector.Decisions()); err != nil {
		return err
	}
	options.Output.Stage("Glossary", fmt.Sprintf("combining %d source-backed term explanations", len(candidates)))
	started := time.Now()
	catalog, err := terminology.Reduce(ctx, executor, provider, candidates, progress)
	if err != nil {
		return fmt.Errorf("glossary: %w", err)
	}
	data.Glossary = &catalog
	if err := writeGlossaryArtifact(runDir, "glossary.json", catalog); err != nil {
		return err
	}
	// Where each term is written in the repository's own files: a fact the
	// glossary lists, found by the same lookup that underlines the term.
	// Only text is searched: a binary file is not where a reader looks.
	paths := collector.Paths()
	if options.Corpus != nil {
		paths = options.Corpus.VisiblePaths()
	}
	data.GlossaryOccurrences = terminology.Occurrences(catalog, paths, func(path string) (string, bool) {
		var text []byte
		if options.Corpus != nil {
			id, known := options.Corpus.ID(path)
			if !known {
				return "", false
			}
			content, err := options.Corpus.ReadFileAll(id)
			if err != nil {
				return "", false
			}
			text = content.Bytes
		} else {
			read, err := os.ReadFile(filepath.Join(options.Repo, filepath.FromSlash(path)))
			if err != nil {
				return "", false
			}
			text = read
		}
		return string(text), utf8.Valid(text)
	})
	state := "ready"
	if catalog.PartialComparison {
		state = "partial comparison; original explanations preserved"
	}
	options.Output.State("Glossary", state, fmt.Sprintf("%d entries", len(catalog.Entries)), formatRunOutputWallDuration(time.Since(started)))
	return nil
}

// The glossary explains concepts. These are every name declared in a sealed
// ProgramIndex, including locals, parameters, fields, methods and enum
// members, and every environment key the facts layer saw the code read;
// corpus paths and file names are already known to the collector. The
// collector keeps only names in code spelling, so a generated term that
// exactly spells one is dropped and journaled while a domain word the code
// also declares (candle, JSON, ROI) remains a concept. No existing artifact
// records command-line flag names, so flags rely on the prompt alone.
func glossaryCodeNames(indexes []programindex.Index, repository *facts.Result) map[terminology.CodeNameKind][]string {
	names := make(map[terminology.CodeNameKind][]string)
	for _, index := range indexes {
		for _, object := range index.Objects {
			switch object.Kind {
			case programindex.ObjectPackage, programindex.ObjectModule:
				names[terminology.CodePackage] = append(names[terminology.CodePackage], object.Name)
			case programindex.ObjectType, programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectVariable:
				names[terminology.CodeDeclaration] = append(names[terminology.CodeDeclaration], object.Name)
			}
			// A lambda declares no name, and an external symbol is declared
			// outside the repository: neither is a name this code owns.
		}
	}
	if repository != nil {
		for _, fact := range repository.Facts {
			if fact.Kind == facts.KindConfigRead {
				names[terminology.CodeEnvironmentKey] = append(names[terminology.CodeEnvironmentKey], fact.Key)
			}
		}
	}
	return names
}

func writeGlossaryArtifact(runDir, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, name), append(raw, '\n'), 0o600)
}
