package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/envfile"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/report"
)

func main() {
	_ = envfile.Load(".env")

	// Handle --help and --version at top level
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			printUsage()
			return
		case "--version", "-v":
			fmt.Println("repomap (dev)")
			return
		}
	}

	// repomap <repo> [flags]
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") && os.Args[1] != "orient" && os.Args[1] != "dev" {
		if err := runDefault(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "orient":
		if err := runOrient(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "dev":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: repomap dev render-report <run-dir>\n")
			os.Exit(2)
		}
		switch os.Args[2] {
		case "render-report":
			if len(os.Args) < 4 {
				fmt.Fprintf(os.Stderr, "Usage: repomap dev render-report <.repomap-runs/<run-id>>\n")
				os.Exit(2)
			}
			if err := runRenderReport(os.Args[3]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown dev command: %s\n", os.Args[2])
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func linkLatest(debugDir, runDir string) {
	latest := filepath.Join(debugDir, "latest")
	os.Remove(latest)
	if err := os.Symlink(filepath.Base(runDir), latest); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create latest symlink: %v\n", err)
	}
}

func runDefault(repo string, extraArgs []string) error {
	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	jsonOut := fs.Bool("json", false, "print combined JSON report instead of text")
	offline := fs.Bool("offline", false, "skip DeepSeek calls, build local bundles only")
	flows := fs.Int("flows", 4, "number of candidate flows to explain")
	noDebug := fs.Bool("no-debug", false, "disable debug artifact writing")
	debugDir := fs.String("debug-dir", ".repomap-runs", "directory for debug artifacts")
	dumpLLM := fs.Bool("dump-llm", false, "dump LLM request/response to debug dir")
	out := fs.String("out", "", "write output to file instead of stdout")

	if err := fs.Parse(extraArgs); err != nil {
		return err
	}

	dDir := *debugDir
	if *noDebug {
		dDir = ""
	}

	var runID string
	if dDir != "" {
		runID = debugdump.GenerateRunID(filepath.Base(filepath.Clean(repo)))
	}

	opts := orient.Options{
		RepoPath:            repo,
		OutputJSON:          *jsonOut,
		Offline:             *offline,
		FlowCount:           *flows,
		RunID:               runID,
		DebugDir:            dDir,
		DumpLLM:             *dumpLLM,
		DumpRedacted:        true,
		MaxLLMFiles:         300,
		MaxLLMEdges:         300,
		MaxLLMModules:       40,
		MaxLLMEntrypoints:   40,
		MaxReadmeBytes:      40000,
		MaxReadmeLLMBytes:   12000,
		MaxTreeLines:        800,
		MaxInterestingFiles: 400,
		MaxGoPkgs:           600,
		MaxGoEdges:          1000,
	}

	output, err := orient.Run(context.Background(), opts)
	if err != nil {
		return err
	}

	if dDir != "" {
		runDir := filepath.Join(dDir, runID)
		if err := report.Generate(runDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: report generation failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Report: %s/report.html\n", runDir)
			linkLatest(dDir, runDir)
		}
	}

	if *out != "" {
		return os.WriteFile(*out, output, 0o644)
	}

	if _, err := os.Stdout.Write(output); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	if len(output) == 0 || output[len(output)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func runOrient(args []string) error {
	fs := flag.NewFlagSet("orient", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repo := fs.String("repo", "", "path to local git repository")
	snapshotOnly := fs.Bool("snapshot-only", false, "print local snapshot JSON only")
	llmBundleOnly := fs.Bool("llm-bundle-only", false, "print compact LLM bundle (no API call)")
	out := fs.String("out", "", "write output to file")
	debugDir := fs.String("debug-dir", "", "directory for debug artifacts")
	dumpLLM := fs.Bool("dump-llm", false, "dump LLM request/response")
	explainFlows := fs.Int("explain-flows", 0, "explain top N candidate flows")
	flowBundlesOnly := fs.Bool("flow-bundles-only", false, "build flow bundles only")
	maxLLMFiles := fs.Int("max-llm-files", 150, "max files in LLM bundle")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" {
		return fmt.Errorf("--repo is required")
	}

	dDir := *debugDir
	var runID string
	if dDir != "" {
		runID = debugdump.GenerateRunID(filepath.Base(filepath.Clean(*repo)))
	}

	opts := orient.Options{
		RepoPath:            *repo,
		SnapshotOnly:        *snapshotOnly,
		LLMBundleOnly:       *llmBundleOnly,
		OutputJSON:          true,
		FlowCount:           *explainFlows,
		FlowBundlesOnly:     *flowBundlesOnly,
		RunID:               runID,
		DebugDir:            dDir,
		DumpLLM:             *dumpLLM,
		DumpRedacted:        true,
		MaxLLMFiles:         *maxLLMFiles,
		MaxLLMEdges:         500,
		MaxLLMModules:       40,
		MaxLLMEntrypoints:   40,
		MaxReadmeBytes:      40000,
		MaxReadmeLLMBytes:   12000,
		MaxTreeLines:        800,
		MaxInterestingFiles: 400,
		MaxGoPkgs:           600,
		MaxGoEdges:          1000,
	}

	output, err := orient.Run(context.Background(), opts)
	if err != nil {
		return err
	}

	if dDir != "" {
		runDir := filepath.Join(dDir, runID)
		if err := report.Generate(runDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: report generation failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Report: %s/report.html\n", runDir)
			linkLatest(dDir, runDir)
		}
	}

	if *out != "" {
		return os.WriteFile(*out, output, 0o644)
	}

	if _, err := os.Stdout.Write(output); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	if len(output) == 0 || output[len(output)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func runRenderReport(runDir string) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if err := report.Generate(absDir); err != nil {
		return err
	}
	fmt.Printf("Report: %s/report.html\n", absDir)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: repomap <repo> [flags]\n")
	fmt.Fprintf(os.Stderr, "       repomap orient --repo <repo> [flags]\n")
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	fmt.Fprintf(os.Stderr, "  --json          output JSON instead of text\n")
	fmt.Fprintf(os.Stderr, "  --offline       skip DeepSeek, local facts only\n")
	fmt.Fprintf(os.Stderr, "  --flows N       number of flows to explain (default 4)\n")
	fmt.Fprintf(os.Stderr, "  --no-debug      disable debug artifact writing\n")
	fmt.Fprintf(os.Stderr, "  --debug-dir DIR debug artifact directory (default .repomap-runs)\n")
	fmt.Fprintf(os.Stderr, "  --dump-llm      dump LLM request/response in debug dir\n")
	fmt.Fprintf(os.Stderr, "  --help, -h      show this help\n")
	fmt.Fprintf(os.Stderr, "  --version       show version\n")
	fmt.Fprintf(os.Stderr, "\nEnvironment:\n")
	fmt.Fprintf(os.Stderr, "  DEEPSEEK_API_KEY  (or create .env file)\n")
	fmt.Fprintf(os.Stderr, "  DEEPSEEK_MODEL    (default deepseek-v4-flash)\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  repomap ../etcd\n")
	fmt.Fprintf(os.Stderr, "  repomap ../etcd --offline\n")
	fmt.Fprintf(os.Stderr, "  repomap ../etcd --flows 2 --json | jq .\n")
	fmt.Fprintf(os.Stderr, "  repomap --help\n")
}
