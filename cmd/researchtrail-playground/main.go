// Command researchtrail-playground composes saved component research artifacts
// into a presentation-neutral trail without repository, gopls, or model calls.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/componentprobe"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/componentteach"
	"github.com/dvordrova/repomap/internal/researchtrail"
	researchcomponent "github.com/dvordrova/repomap/internal/researchtrail/component"
)

type config struct {
	casePath             string
	studyBundlePath      string
	planPath             string
	planDiagnosticsPath  string
	round1Path           string
	round2Path           string
	teachBundlePath      string
	teachIndexPath       string
	teachReportPath      string
	teachDiagnosticsPath string
	outDir               string
}

type caseBinding struct {
	RepositoryStateSHA256 string `json:"repository_state_sha256"`
	ReportSHA256          string `json:"report_sha256"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}

	var binding caseBinding
	var study componentstudy.Bundle
	var plan componentstudy.Plan
	var round1 componentprobe.Bundle
	var teachBundle componentteach.Bundle
	var teachIndex componentteach.Index
	var teachReport componentteach.Report
	for _, input := range []struct {
		path  string
		value any
	}{
		{cfg.casePath, &binding},
		{cfg.studyBundlePath, &study},
		{cfg.planPath, &plan},
		{cfg.round1Path, &round1},
		{cfg.teachBundlePath, &teachBundle},
		{cfg.teachIndexPath, &teachIndex},
		{cfg.teachReportPath, &teachReport},
	} {
		if err := readJSON(input.path, input.value); err != nil {
			return err
		}
	}

	var round2 *componentprobe.Bundle
	if cfg.round2Path != "" {
		round2 = new(componentprobe.Bundle)
		if err := readJSON(cfg.round2Path, round2); err != nil {
			return err
		}
	}
	var planDiagnostics []componentstudy.Diagnostic
	if cfg.planDiagnosticsPath != "" {
		if err := readJSON(cfg.planDiagnosticsPath, &planDiagnostics); err != nil {
			return err
		}
	}
	var teachDiagnostics []componentteach.Diagnostic
	if cfg.teachDiagnosticsPath != "" {
		if err := readJSON(cfg.teachDiagnosticsPath, &teachDiagnostics); err != nil {
			return err
		}
	}

	trail, index, err := researchcomponent.Build(researchcomponent.Input{
		Binding: researchtrail.Binding{
			RepositoryStateSHA256: binding.RepositoryStateSHA256,
			ReportSHA256:          binding.ReportSHA256,
		},
		StudyBundle: study,
		StudyResult: componentstudy.Result{
			Plan:        plan,
			Diagnostics: planDiagnostics,
		},
		Round1:      round1,
		Round2:      round2,
		TeachBundle: teachBundle,
		TeachIndex:  teachIndex,
		TeachResult: componentteach.ParseResult{
			Report:      teachReport,
			Diagnostics: teachDiagnostics,
		},
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.outDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	trailPath := filepath.Join(cfg.outDir, "trail.json")
	indexPath := filepath.Join(cfg.outDir, "trail_index.json")
	if err := writeJSON(trailPath, trail); err != nil {
		return err
	}
	if err := writeJSON(indexPath, index); err != nil {
		return err
	}
	fmt.Fprintln(stdout, trailPath)
	fmt.Fprintln(stdout, indexPath)
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("researchtrail-playground", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.casePath, "case", "", "case.json containing report and repository-state hashes")
	flags.StringVar(&cfg.studyBundlePath, "study-bundle", "", "component planner bundle.json")
	flags.StringVar(&cfg.planPath, "plan", "", "normalized component planner plan.json")
	flags.StringVar(&cfg.planDiagnosticsPath, "plan-diagnostics", "", "optional planner parse_warnings.json")
	flags.StringVar(&cfg.round1Path, "probe-round1", "", "component probe round-1 bundle.json")
	flags.StringVar(&cfg.round2Path, "probe-round2", "", "optional component probe round-2 bundle.json")
	flags.StringVar(&cfg.teachBundlePath, "teacher-bundle", "", "focused teacher bundle.json")
	flags.StringVar(&cfg.teachIndexPath, "teacher-index", "", "focused teacher index.json")
	flags.StringVar(&cfg.teachReportPath, "teacher-report", "", "normalized focused teacher report.json")
	flags.StringVar(&cfg.teachDiagnosticsPath, "teacher-diagnostics", "", "optional teacher parse_warnings.json")
	flags.StringVar(&cfg.outDir, "out-dir", "", "output directory")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{"case", cfg.casePath},
		{"study-bundle", cfg.studyBundlePath},
		{"plan", cfg.planPath},
		{"probe-round1", cfg.round1Path},
		{"teacher-bundle", cfg.teachBundlePath},
		{"teacher-index", cfg.teachIndexPath},
		{"teacher-report", cfg.teachReportPath},
		{"out-dir", cfg.outDir},
	} {
		if strings.TrimSpace(required.value) == "" {
			return config{}, fmt.Errorf("--%s is required", required.name)
		}
	}
	return cfg, nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
