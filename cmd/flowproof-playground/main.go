// flowproof-playground replays local proof construction from saved orientation
// and compact-bundle artifacts. It never calls a model provider.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dvordrova/repomap/internal/flowexplain"
	flowproofassemble "github.com/dvordrova/repomap/internal/flowproof/assemble"
	"github.com/dvordrova/repomap/internal/llmbundle"
)

type orientationArtifact struct {
	CandidateFlows []flowexplain.CandidateFlow `json:"candidate_flows"`
	Warnings       []string                    `json:"warnings,omitempty"`
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
	bundlePath := flag.String("bundle", "", "saved llm_bundle.json")
	flag.Parse()
	if *repoPath == "" || *orientationPath == "" || *bundlePath == "" {
		return fmt.Errorf("--repo, --orientation, and --bundle are required")
	}

	var orientation orientationArtifact
	if err := readJSON(*orientationPath, &orientation); err != nil {
		return err
	}
	var bundle llmbundle.Bundle
	if err := readJSON(*bundlePath, &bundle); err != nil {
		return err
	}
	orientation.Warnings = append(orientation.Warnings,
		flowproofassemble.Attach(context.Background(), *repoPath, orientation.CandidateFlows, bundle.Go.CommandTraces)...,
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
