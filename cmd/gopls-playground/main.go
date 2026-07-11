package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/evidence"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var (
		repoPath               = flag.String("repo", ".", "path to a Go repository")
		query                  = flag.String("query", "", "workspace symbol query")
		outPath                = flag.String("out", "", "write graph JSON to this file")
		summaryPath            = flag.String("summary-out", "", "write a human-readable Markdown summary")
		maxSymbols             = flag.Int("max-symbols", 40, "maximum fuzzy symbol matches")
		maxCallRoots           = flag.Int("max-call-roots", 3, "maximum matched functions to expand")
		maxImplementationRoots = flag.Int("max-implementation-roots", 2, "maximum interfaces/types to expand")
		commandTimeout         = flag.Duration("command-timeout", 30*time.Second, "timeout for each gopls command")
		includeExternal        = flag.Bool("include-external", false, "include symbols outside the repository")
		implementations        = flag.Bool("implementations", false, "query possible interface implementations")
	)
	flag.Parse()

	if *query == "" {
		return fmt.Errorf("--query is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	analyzer := goplsanalyzer.New(goplsanalyzer.Options{
		MaxSymbols:             *maxSymbols,
		MaxCallRoots:           *maxCallRoots,
		MaxImplementationRoots: *maxImplementationRoots,
		CommandTimeout:         *commandTimeout,
		IncludeExternal:        *includeExternal,
		IncludeImplementations: *implementations,
	})
	graph, err := analyzer.Analyze(ctx, analysis.Request{
		RepoPath: *repoPath,
		Query:    *query,
	})
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}
	data = append(data, '\n')

	if *outPath == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
		if err := os.WriteFile(*outPath, data, 0o644); err != nil {
			return fmt.Errorf("write graph: %w", err)
		}
	}

	summary := graph.Summary()
	fmt.Fprintf(os.Stderr, "evidence graph: %d entities, %d relations (possible=%d, static=%d)\n",
		summary.Entities,
		summary.Relations,
		summary.ByCertainty[evidence.CertaintyPossible],
		summary.ByCertainty[evidence.CertaintyStatic],
	)
	if *outPath != "" {
		fmt.Fprintf(os.Stderr, "wrote %s\n", *outPath)
	}
	if *summaryPath != "" {
		if err := writeMarkdownSummary(*summaryPath, graph); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *summaryPath)
	}
	return nil
}

func writeMarkdownSummary(path string, graph evidence.Graph) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create summary directory: %w", err)
	}
	entities := make(map[string]evidence.Entity, len(graph.Entities))
	for _, entity := range graph.Entities {
		entities[entity.ID] = entity
	}

	summary := graph.Summary()
	var output strings.Builder
	fmt.Fprintf(&output, "# gopls evidence: %s\n\n", markdownCell(graph.Query))
	fmt.Fprintf(&output, "- Repository: `%s`\n", graph.RepoPath)
	fmt.Fprintf(&output, "- Build: `%s/%s`\n", graph.Build.GOOS, graph.Build.GOARCH)
	fmt.Fprintf(&output, "- Entities: %d\n", summary.Entities)
	fmt.Fprintf(&output, "- Relations: %d (`possible`: %d, `static`: %d)\n\n",
		summary.Relations,
		summary.ByCertainty[evidence.CertaintyPossible],
		summary.ByCertainty[evidence.CertaintyStatic],
	)

	output.WriteString("## Possible symbol matches\n\n")
	output.WriteString("| Kind | Symbol | Location |\n| --- | --- | --- |\n")
	for _, relation := range graph.Relations {
		if relation.Kind != evidence.RelationMatchesQuery {
			continue
		}
		entity := entities[relation.To]
		fmt.Fprintf(&output, "| %s | `%s` | `%s` |\n",
			entity.Kind,
			markdownCell(entity.Name),
			locationText(entity.Location),
		)
	}

	output.WriteString("\n## Static direct calls\n\n")
	output.WriteString("| Caller | Callee | Callsite |\n| --- | --- | --- |\n")
	for _, relation := range graph.Relations {
		if relation.Kind != evidence.RelationCalls || relation.Certainty != evidence.CertaintyStatic {
			continue
		}
		caller := entities[relation.From]
		callee := entities[relation.To]
		var callsite *evidence.Location
		if len(relation.Provenance) > 0 {
			callsite = relation.Provenance[0].Location
		}
		fmt.Fprintf(&output, "| `%s` | `%s` | `%s` |\n",
			markdownCell(caller.Name),
			markdownCell(callee.Name),
			locationText(callsite),
		)
	}

	if len(graph.Warnings) > 0 {
		output.WriteString("\n## Warnings\n\n")
		for _, warning := range graph.Warnings {
			fmt.Fprintf(&output, "- %s\n", warning)
		}
	}

	if err := os.WriteFile(path, []byte(output.String()), 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

func locationText(location *evidence.Location) string {
	if location == nil {
		return ""
	}
	if location.Line == 0 {
		return markdownCell(location.Path)
	}
	return fmt.Sprintf("%s:%d:%d", markdownCell(location.Path), location.Line, location.Column)
}

func markdownCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
