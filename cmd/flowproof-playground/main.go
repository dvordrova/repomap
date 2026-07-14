// flowproof-playground replays local proof construction from saved orientation
// and full local snapshot artifacts. It never calls a model provider.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	flowproofassemble "github.com/dvordrova/repomap/internal/flowproof/assemble"
	"github.com/dvordrova/repomap/internal/gofacts"
)

type orientationArtifact struct {
	CandidateFlows []flowexplain.CandidateFlow `json:"candidate_flows"`
	Warnings       []string                    `json:"warnings,omitempty"`
}

type snapshotArtifact struct {
	GoFacts *struct {
		CommandTraces []gofacts.CommandTrace `json:"command_traces"`
	} `json:"go_facts"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "flowproof-playground:", err)
		os.Exit(1)
	}
}

func run() error {
	repoPath := flag.String("repo", "", "repository root used by saved artifacts")
	orientationPath := flag.String("orientation", "", "saved orientation_report.json")
	snapshotPath := flag.String("snapshot", "", "saved snapshot.json")
	flag.Parse()
	if *repoPath == "" || *orientationPath == "" || *snapshotPath == "" {
		return fmt.Errorf("--repo, --orientation, and --snapshot are required")
	}

	var orientation orientationArtifact
	if err := readJSON(*orientationPath, &orientation); err != nil {
		return err
	}
	var snapshot snapshotArtifact
	if err := readJSON(*snapshotPath, &snapshot); err != nil {
		return err
	}
	var traces []gofacts.CommandTrace
	if snapshot.GoFacts != nil {
		traces = snapshot.GoFacts.CommandTraces
	}
	orientation.Warnings = append(orientation.Warnings,
		flowproofassemble.Attach(context.Background(), *repoPath, orientation.CandidateFlows, flowproofassemble.Input{
			CommandTraces: traces,
			ProofBudget:   flowproof.DefaultBudget(),
		})...,
	)
	out, err := json.MarshalIndent(orientation, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal proof replay: %w", err)
	}
	fmt.Println(string(out))
	return nil
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
