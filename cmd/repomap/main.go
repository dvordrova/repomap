package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/dvordrova/repomap/internal/orient"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s orient --repo /path/to/local/git/repo [flags]\n", os.Args[0])
		os.Exit(2)
	}

	switch os.Args[1] {
	case "orient":
		if err := runOrient(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func runOrient(args []string) error {
	fs := flag.NewFlagSet("orient", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repo := fs.String("repo", "", "path to local git repository")
	snapshotOnly := fs.Bool("snapshot-only", false, "print local snapshot JSON only")
	out := fs.String("out", "", "write output JSON to file instead of stdout")
	maxReadmeBytes := fs.Int("max-readme-bytes", 20000, "maximum bytes read from README")
	maxTreeLines := fs.Int("max-tree-lines", 400, "maximum lines in file tree output")
	maxInterestingFiles := fs.Int("max-interesting-files", 200, "maximum interesting files")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" {
		return fmt.Errorf("--repo is required")
	}

	opts := orient.Options{
		RepoPath:            *repo,
		SnapshotOnly:        *snapshotOnly,
		MaxReadmeBytes:      *maxReadmeBytes,
		MaxTreeLines:        *maxTreeLines,
		MaxInterestingFiles: *maxInterestingFiles,
	}

	output, err := orient.Run(context.Background(), opts)
	if err != nil {
		return err
	}

	if *out != "" {
		if err := os.WriteFile(*out, output, 0o644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	}

	if _, err := os.Stdout.Write(output); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	if len(output) == 0 || output[len(output)-1] != '\n' {
		fmt.Println()
	}
	return nil
}
