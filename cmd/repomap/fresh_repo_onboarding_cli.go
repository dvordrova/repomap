package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/dvordrova/repomap/internal/report"
)

type freshRepoOnboardingCLIOptions struct {
	RunDir      string
	RepoRoot    string
	ReplaySaved bool
	ReplanSaved bool
}

func runFreshRepoOnboardingCLI(args []string, stdout io.Writer, stderr io.Writer) error {
	options, err := parseFreshRepoOnboardingCLIOptions(args, stderr)
	if err != nil {
		return err
	}
	absRunDir, err := filepath.Abs(options.RunDir)
	if err != nil {
		return fmt.Errorf("fresh-repo-onboarding: resolve run directory: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var result freshRepoDemoResult
	if options.ReplaySaved {
		result, err = replaySavedFreshRepoMechanisms(ctx, absRunDir)
	} else if options.ReplanSaved {
		absRepoRoot, resolveErr := filepath.Abs(options.RepoRoot)
		if resolveErr != nil {
			return fmt.Errorf("fresh-repo-onboarding: resolve repository root: %w", resolveErr)
		}
		result, err = editFreshPrimaryPathsForSavedRun(
			ctx,
			absRunDir,
			absRepoRoot,
			stderr,
		)
	} else {
		absRepoRoot, resolveErr := filepath.Abs(options.RepoRoot)
		if resolveErr != nil {
			return fmt.Errorf("fresh-repo-onboarding: resolve repository root: %w", resolveErr)
		}
		result, err = editFreshRepoMechanismForRun(ctx, absRunDir, absRepoRoot, stderr)
	}
	if err != nil {
		return err
	}
	if err := report.Generate(absRunDir); err != nil {
		return fmt.Errorf("fresh-repo-onboarding: generate report: %w", err)
	}
	output := struct {
		Status     freshRepoDemoStatus `json:"status"`
		ReportPath string              `json:"report_path"`
	}{
		Status:     result.Status,
		ReportPath: filepath.Join(absRunDir, "report.html"),
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func parseFreshRepoOnboardingCLIOptions(
	args []string,
	stderr io.Writer,
) (freshRepoOnboardingCLIOptions, error) {
	flags := flag.NewFlagSet("fresh-repo-onboarding", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runDir := flags.String("run-dir", "", "existing saved report run")
	repoRoot := flags.String("repo", "", "matching repository checkout")
	replaySaved := flags.Bool(
		"replay-saved",
		false,
		"revalidate and publish saved bounded candidate responses without a model or repository analysis",
	)
	replanSaved := flags.Bool(
		"replan-saved",
		false,
		"reuse the saved opportunity proposal, run bounded primary-path planning, and synthesize eligible candidates",
	)
	if err := flags.Parse(args); err != nil {
		return freshRepoOnboardingCLIOptions{}, err
	}
	if flags.NArg() != 0 {
		return freshRepoOnboardingCLIOptions{}, fmt.Errorf(
			"fresh-repo-onboarding: unexpected arguments: %v",
			flags.Args(),
		)
	}
	if *runDir == "" {
		return freshRepoOnboardingCLIOptions{}, fmt.Errorf(
			"fresh-repo-onboarding: --run-dir is required",
		)
	}
	if *replaySaved && *replanSaved {
		return freshRepoOnboardingCLIOptions{}, fmt.Errorf(
			"fresh-repo-onboarding: --replay-saved and --replan-saved are mutually exclusive",
		)
	}
	if !*replaySaved && *repoRoot == "" {
		return freshRepoOnboardingCLIOptions{}, fmt.Errorf(
			"fresh-repo-onboarding: --repo is required unless --replay-saved is set",
		)
	}
	return freshRepoOnboardingCLIOptions{
		RunDir:      *runDir,
		RepoRoot:    *repoRoot,
		ReplaySaved: *replaySaved,
		ReplanSaved: *replanSaved,
	}, nil
}
