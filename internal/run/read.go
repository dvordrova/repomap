package run

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/terminology"
)

func runRead(args []string, stdout io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	output := newRunOutput(stdout)
	err := runReadConfiguredWithOutput(ctx, args, stdout, defaultTargetPortfolioProviderFactory, true, output)
	if err != nil {
		writeRunOutputErrorTo(output, os.Stderr, err)
	}
	return err
}

// Configured and command-line questions keep their order, with one result per text.
func appendQuestion(questions *[]string, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("question must not be empty")
	}
	for _, existing := range *questions {
		if existing == text {
			return nil
		}
	}
	*questions = append(*questions, text)
	return nil
}

func runReadWithProvider(ctx context.Context, args []string, stdout io.Writer, factory func() (llm.Provider, error)) error {
	return runReadConfigured(ctx, args, stdout, factory, false)
}

func runReadConfigured(ctx context.Context, args []string, stdout io.Writer, factory func() (llm.Provider, error), collectTerms bool) error {
	return runReadConfiguredWithOutput(ctx, args, stdout, factory, collectTerms, newRunOutput(stdout))
}

func runReadConfiguredWithOutput(ctx context.Context, args []string, stdout io.Writer, factory targetPortfolioProviderFactory, collectTerms bool, output *runOutput) error {
	fs := flag.NewFlagSet("repomap read", flag.ContinueOnError)
	fs.SetOutput(stdout)
	through := fs.String("through", "", "stop after directories, files, symbols, operations, boundaries, zones, arrows, targets, joints, learn, question or answer")
	var questions []string
	fs.Func("question", "answer from code and documentation with a reading route; repeat for several questions", func(value string) error { return appendQuestion(&questions, value) })
	outputDir := fs.String("output", "", "new directory for the reading; default: a new directory under debug-dir")
	promptFile := fs.String("prompt", "", "system prompt for --through's stage; predecessors use bundled prompts")
	windowRows := fs.Int("window-rows", 0, "row budget for --through's stage (all stages when omitted); 0 keeps defaults")
	inputBytes := fs.Int("input-bytes", 0, "system + user UTF-8 byte budget for that stage; 0 keeps defaults")
	cacheRoot := fs.String("debug-dir", defaultDebugDir(), "shared run and model cache directory")
	noCache := fs.Bool("no-cache", false, "bypass model response cache")
	fs.Usage = func() { fmt.Fprintln(stdout, "Usage: repomap read READING_INPUT.json [flags]"); fs.PrintDefaults() }
	if len(args) == 0 {
		fs.Usage()
		return fmt.Errorf("read: reading input is required")
	}
	if args[0] == "--help" || args[0] == "-h" {
		fs.Usage()
		return nil
	}
	filename := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("read: unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	stage, err := reading.StageName(*through)
	if err != nil {
		return err
	}
	if len(questions) > 0 {
		if stage != "" && stage != lines.StageQuestion && stage != lines.StageAnswer {
			return fmt.Errorf("read: --question cannot be combined with an earlier --through stage")
		}
		if stage == "" {
			stage = lines.StageAnswer
		}
	}
	if (stage == lines.StageQuestion || stage == lines.StageAnswer) && len(questions) == 0 {
		return fmt.Errorf("read: --through question or answer requires --question")
	}
	if *windowRows < 0 || *inputBytes < 0 {
		return fmt.Errorf("read: budgets cannot be negative")
	}
	if *promptFile != "" && stage == "" {
		return fmt.Errorf("read: --prompt requires --through")
	}
	input, err := reading.LoadInput(filename)
	if err != nil {
		return err
	}
	opts := input.Options()
	opts.Questions = questions
	opts.Through, opts.WindowRows, opts.InputBytes = stage, *windowRows, *inputBytes
	if *promptFile != "" {
		raw, err := os.ReadFile(*promptFile)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(raw)) == "" {
			return fmt.Errorf("read: prompt is empty")
		}
		opts.Prompt = string(raw)
	}
	provider, err := providerFactoryWithOutput(factory, output)()
	if err != nil {
		return err
	}
	var termCollector *terminology.Collector
	if collectTerms {
		termCollector = terminology.NewCollector(readingTerminologyPaths(opts.Graph))
		provider = termCollector.Wrap(provider)
	}
	if err := os.MkdirAll(*cacheRoot, 0o700); err != nil {
		return err
	}
	if *outputDir == "" {
		*outputDir, err = os.MkdirTemp(*cacheRoot, "reading-")
	} else {
		if err := os.MkdirAll(filepath.Dir(*outputDir), 0o700); err != nil {
			return err
		}
		err = os.Mkdir(*outputDir, 0o700)
	}
	if err != nil {
		return fmt.Errorf("read: create new output directory: %w", err)
	}
	absolute, err := filepath.Abs(*outputDir)
	if err != nil {
		return err
	}
	output.Stage("Reading", "input: "+filename, "output: "+absolute)
	writer, err := debugdump.OpenWriter(absolute)
	if err != nil {
		return err
	}
	defer writer.Close()
	opts.OwnerRunDir, opts.Provider = absolute, provider
	opts.Executor = llm.Executor{RootDir: *cacheRoot, Enabled: !*noCache, BatchConcurrency: llm.DefaultBatchConcurrency, BatchController: &llm.BatchController{}, Observer: timed(output, debugdump.NewSemanticObserver(writer))}
	opts.Stage, opts.State = output.Stage, output.State
	started := time.Now()
	result, err := reading.Read(ctx, opts)
	if err != nil {
		return err
	}
	if err := modeldiag.Append(absolute, result.Rejected); err != nil {
		return err
	}
	if termCollector != nil {
		if err := termCollector.Generate(ctx, debugdump.BindStage(opts.Executor, debugdump.SemanticStageGlossary), provider); err != nil {
			return fmt.Errorf("read: glossary: %w", err)
		}
		if err := writeGlossaryArtifact(absolute, "terminology.json", termCollector.Snapshot()); err != nil {
			return err
		}
	}
	if result.Complete {
		if err := atlas.Persist(absolute, result.Atlas); err != nil {
			return err
		}
	}
	summary := struct {
		Version         int              `json:"version"`
		Through         string           `json:"through"`
		Complete        bool             `json:"complete"`
		WallMillis      int64            `json:"wall_ms"`
		Stages          []atlas.StageUse `json:"stages"`
		RejectedWindows int              `json:"rejected_windows"`
	}{1, result.Through, result.Complete, time.Since(started).Milliseconds(), result.Uses, len(result.Rejected)}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(absolute, "reading-result.json"), data, 0o600); err != nil {
		return err
	}
	output.State("Reading", "ready", "through: "+result.Through, "tables: "+result.TablesPath, fmt.Sprintf("rejected windows: %d", len(result.Rejected)), formatRunOutputWallDuration(time.Since(started)))
	return nil
}

// LoadInput has already validated this graph. File, document and source-fact
// places, and anchored observations, carry source authority. An entity's path
// may instead name a logical or missing output and cannot grant that authority.
func readingTerminologyPaths(graph atlas.Graph) []string {
	var paths []string
	for _, place := range graph.Places {
		switch place.Kind {
		case atlas.PlaceFile, atlas.PlaceDocument, atlas.PlaceSourceFact:
			if place.Path != "" {
				paths = append(paths, place.Path)
			}
		}
	}
	for _, edge := range graph.Edges {
		if edge.Evidence != nil && edge.Evidence.Path != "" {
			paths = append(paths, edge.Evidence.Path)
		}
	}
	return paths
}
