package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("surface-discovery-playground", flag.ContinueOnError)
	repository := flags.String("repo", ".", "Go repository or module to analyze")
	output := flags.String("out", "tmp/surface-discovery", "artifact output directory")
	groupingResponse := flags.String("grouping-response", "", "saved grouping response to validate and replay")
	maxDepth := flags.Int("max-depth", 16, "maximum wrapper propagation depth")
	maxTasks := flags.Int("max-tasks", 1000, "maximum functions to inspect")
	maxTargets := flags.Int("max-targets", 8, "maximum targets retained per callsite")
	if err := flags.Parse(args); err != nil {
		return err
	}
	options := surfacediscovery.DefaultOptions(*repository)
	options.MaxDepth = *maxDepth
	options.MaxTasks = *maxTasks
	options.MaxTargets = *maxTargets
	result, err := surfacediscovery.Analyze(options)
	if err != nil {
		return err
	}
	if err := surfacediscovery.WriteArtifacts(*output, result); err != nil {
		return err
	}
	if *groupingResponse != "" {
		raw, err := os.ReadFile(*groupingResponse)
		if err != nil {
			return fmt.Errorf("read grouping response: %w", err)
		}
		if err := surfacediscovery.WriteGroupingReplayArtifacts(
			*output+"/deepseek",
			surfacediscovery.BuildGroupingBundle(result),
			raw,
		); err != nil {
			return err
		}
	}
	fmt.Printf(
		"wrote %d trigger(s), coverage, summaries, and Markdown to %s\n",
		len(result.Catalog.Triggers),
		*output,
	)
	return nil
}
