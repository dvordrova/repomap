// Command pyright-playground explores one exact Python callable without
// changing repomap's default repository survey or making a model request.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	pyrightanalyzer "github.com/dvordrova/repomap/internal/analyzer/python/pyright"
	"github.com/dvordrova/repomap/internal/evidence"
)

type config struct {
	repoPath       string
	path           string
	line           int
	column         int
	binary         string
	maxIncoming    int
	maxOutgoing    int
	maxReferences  int
	requestTimeout time.Duration
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) (runErr error) {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	analyzer := pyrightanalyzer.New(pyrightanalyzer.Options{
		Binary:         cfg.binary,
		MaxIncoming:    cfg.maxIncoming,
		MaxOutgoing:    cfg.maxOutgoing,
		MaxReferences:  cfg.maxReferences,
		RequestTimeout: cfg.requestTimeout,
	})
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := analyzer.Close(closeCtx); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("pyright-playground: close language server: %w", closeErr))
		}
	}()

	resolution, err := analyzer.ResolveLocation(ctx, analysis.LocationRequest{
		RepoPath:      cfg.repoPath,
		Location:      evidence.Location{Path: cfg.path, Line: cfg.line, Column: cfg.column},
		MaxCandidates: 4,
	})
	if err != nil {
		return fmt.Errorf("pyright-playground: resolve location: %w", err)
	}
	if len(resolution.Candidates) == 0 {
		return fmt.Errorf("pyright-playground: no callable declaration at %s:%d", cfg.path, cfg.line)
	}
	if len(resolution.Candidates) > 1 {
		return fmt.Errorf("pyright-playground: location is ambiguous; pass --column to select one of %d declarations", len(resolution.Candidates))
	}
	selected := resolution.Candidates[0]
	if !selected.Investigable {
		return fmt.Errorf("pyright-playground: selected %s is not callable", selected.Entity.Kind)
	}
	graph, err := analyzer.AnalyzeExactSymbol(ctx, analysis.ExactSymbolRequest{
		RepoPath: cfg.repoPath,
		Symbol:   selected.Entity,
	})
	if err != nil {
		return fmt.Errorf("pyright-playground: analyze exact symbol: %w", err)
	}
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("pyright-playground: marshal evidence graph: %w", err)
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", data); err != nil {
		return fmt.Errorf("pyright-playground: write graph: %w", err)
	}
	summary := graph.Summary()
	if _, err := fmt.Fprintf(
		stderr,
		"pyright-playground: %s -> %d entities, %d relations, %d warnings\n",
		selected.Entity.Name,
		summary.Entities,
		summary.Relations,
		len(graph.Warnings),
	); err != nil {
		return fmt.Errorf("pyright-playground: write summary: %w", err)
	}
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("pyright-playground", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.repoPath, "repo", "", "path to a Python repository")
	flags.StringVar(&cfg.path, "path", "", "repository-relative Python file")
	flags.IntVar(&cfg.line, "line", 0, "one-based declaration line")
	flags.IntVar(&cfg.column, "column", 0, "optional one-based declaration-name column")
	flags.StringVar(&cfg.binary, "pyright-langserver", "", "explicit pyright-langserver executable")
	flags.IntVar(&cfg.maxIncoming, "max-incoming", 12, "maximum unique direct callers")
	flags.IntVar(&cfg.maxOutgoing, "max-outgoing", 12, "maximum unique direct callees")
	flags.IntVar(&cfg.maxReferences, "max-references", 40, "maximum unique repository references")
	flags.DurationVar(&cfg.requestTimeout, "request-timeout", 30*time.Second, "timeout for each LSP request")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("pyright-playground: unexpected positional arguments")
	}
	if cfg.repoPath == "" || cfg.path == "" || cfg.line <= 0 || cfg.column < 0 {
		return config{}, fmt.Errorf("pyright-playground: --repo, --path, and positive --line are required; --column must be non-negative")
	}
	if cfg.maxIncoming <= 0 || cfg.maxOutgoing <= 0 || cfg.maxReferences <= 0 || cfg.requestTimeout <= 0 {
		return config{}, fmt.Errorf("pyright-playground: limits and --request-timeout must be positive")
	}
	return cfg, nil
}
