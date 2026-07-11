package orient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type Options struct {
	RepoPath            string
	SnapshotOnly        bool
	LLMBundleOnly       bool
	OutputJSON          bool
	Offline             bool
	FlowCount           int
	FlowBundlesOnly     bool
	MaxReadmeBytes      int
	MaxReadmeLLMBytes   int
	MaxTreeLines        int
	MaxInterestingFiles int
	MaxGoPkgs           int
	MaxGoEdges          int
	MaxLLMEntrypoints   int
	MaxLLMModules       int
	MaxLLMFiles         int
	MaxLLMEdges         int
	DebugDir            string
	RunID               string
	DumpLLM             bool
	DumpRedacted        bool
	ExplainFlows        int
}

type combinedReport struct {
	RepoName       string           `json:"repo_name"`
	Orientation    *orientationPart `json:"orientation,omitempty"`
	ExplainedFlows []explainedFlow  `json:"explained_flows"`
	Warnings       []string         `json:"warnings,omitempty"`
}

type orientationPart struct {
	ProjectGuess   string                      `json:"project_guess"`
	CandidateFlows []flowexplain.CandidateFlow `json:"candidate_flows"`
}

type explainedFlow struct {
	FlowSeed          flowexplain.FlowSeed `json:"flow_seed"`
	FlowBundleSummary flowBundleSummary    `json:"flow_bundle_summary"`
	FlowReport        json.RawMessage      `json:"flow_report,omitempty"`
}

type flowBundleSummary struct {
	SelectedFilesCount int      `json:"selected_files_count"`
	SelectedTestsCount int      `json:"selected_tests_count"`
	SelectedDocsCount  int      `json:"selected_docs_count"`
	UnverifiedSeeds    []string `json:"unverified_seed_paths"`
}

type flowReportFields struct {
	FilesToReadInOrder []fileToOpen     `json:"files_to_read_in_order"`
	TestsToRead        []fileToOpen     `json:"tests_to_read"`
	EvidenceFiles      []string         `json:"evidence_files"`
	UnverifiedPaths    []unverifiedPath `json:"unverified_paths"`
	Unknowns           []string         `json:"unknowns"`
	NextQueries        []string         `json:"next_queries"`
	Warnings           []string         `json:"warnings"`
	Summary            string           `json:"summary"`
	Confidence         float64          `json:"confidence"`
}

type fileToOpen struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority,omitempty"`
}

type unverifiedPath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func Run(ctx context.Context, opts Options) ([]byte, error) {
	s, err := snapshot.Build(snapshot.Options{
		RepoPath:            opts.RepoPath,
		MaxReadmeBytes:      opts.MaxReadmeBytes,
		MaxTreeLines:        opts.MaxTreeLines,
		MaxInterestingFiles: opts.MaxInterestingFiles,
		MaxGoPkgs:           opts.MaxGoPkgs,
		MaxGoEdges:          opts.MaxGoEdges,
	})
	if err != nil {
		return nil, err
	}

	snapshotJSON, _ := s.JSON()

	if opts.SnapshotOnly {
		if opts.OutputJSON || opts.SnapshotOnly {
			return append(snapshotJSON, '\n'), nil
		}
		return snapshotJSON, nil
	}

	bundle := llmbundle.Build(s, s.FilteredFiles, llmbundle.Options{
		MaxReadmeBytes: opts.MaxReadmeLLMBytes,
		MaxModules:     opts.MaxLLMModules,
		MaxEntrypoints: opts.MaxLLMEntrypoints,
		MaxFiles:       opts.MaxLLMFiles,
		MaxEdges:       opts.MaxLLMEdges,
		RepoPath:       opts.RepoPath,
	})
	bundleJSON, _ := json.MarshalIndent(bundle, "", "  ")

	if opts.LLMBundleOnly {
		out := append(bundleJSON, '\n')
		return out, nil
	}

	runID := opts.RunID
	if runID == "" {
		runID = debugdump.GenerateRunID(s.RepoName)
	}

	var dw *debugdump.Writer
	if opts.DebugDir != "" {
		dw, _ = debugdump.NewWriter(opts.DebugDir, runID, opts.DumpRedacted)
		if dw != nil {
			dw.WriteMetadata(debugdump.RunMeta{
				RunID:         runID,
				CreatedAt:     time.Now().UTC().Format(time.RFC3339),
				RepoName:      s.RepoName,
				RepoPath:      opts.RepoPath,
				Command:       "orient",
				LLMBundleOnly: opts.LLMBundleOnly,
			})
			dw.WriteSnapshot(snapshotJSON)
			dw.WriteLLMBundle(bundleJSON)
		}
	}

	report := combinedReport{
		RepoName: s.RepoName,
	}

	flowCount := opts.FlowCount
	if opts.ExplainFlows > 0 {
		flowCount = opts.ExplainFlows
	}
	if flowCount <= 0 {
		flowCount = 20
	}

	if opts.Offline {
		report.Warnings = append(report.Warnings, "offline mode: skipping all DeepSeek calls")
		flows := buildFlowBundlesFromSnapshot(s, flowCount, dw)
		report.ExplainedFlows = flows
		report.Warnings = append(report.Warnings, fmt.Sprintf("run %s to get AI orientation", "repomap "+opts.RepoPath))
	} else {
		client, err := deepseek.NewFromEnv()
		if err != nil {
			if dw != nil {
				dw.WriteError(err)
			}
			return nil, fmt.Errorf("DEEPSEEK_API_KEY is not set.\nRun:\n  export DEEPSEEK_API_KEY=...\n  repomap %s\n\nOr run offline:\n  repomap %s --offline", opts.RepoPath, opts.RepoPath)
		}

		raw, err := client.Orient(ctx, bundleJSON)
		if err != nil {
			if dw != nil {
				dw.WriteError(err)
			}
			return nil, err
		}

		if opts.DumpLLM && dw != nil {
			dw.WriteLLMResponse(raw)
		}

		var or orientResponse
		if err := json.Unmarshal(raw, &or); err != nil {
			if dw != nil {
				dw.WriteError(fmt.Errorf("invalid orientation JSON: %s", string(raw)))
			}
			return nil, fmt.Errorf("DeepSeek returned invalid JSON for orientation")
		}

		report.Orientation = &orientationPart{
			ProjectGuess:   or.ProjectGuess,
			CandidateFlows: or.CandidateFlows,
		}

		out, _ := json.MarshalIndent(or, "", "  ")
		if dw != nil {
			dw.WriteOrientationReport(out)
		}

		cfs := selectTopFlows(or.CandidateFlows, flowCount)
		for _, cf := range cfs {
			ef := explainOneFlow(ctx, client, cf, s.FilteredFiles, s.GoFacts, opts.MaxLLMFiles, dw, opts, !opts.Offline && !opts.FlowBundlesOnly)
			report.ExplainedFlows = append(report.ExplainedFlows, ef)
		}
	}

	if opts.OutputJSON {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, err
		}
		out = append(out, '\n')
		return out, nil
	}

	text := formatHumanReadable(report, opts.DebugDir, runID)
	return []byte(text), nil
}

type orientResponse struct {
	ProjectGuess   string                      `json:"project_guess"`
	Confidence     float64                     `json:"confidence"`
	CandidateFlows []flowexplain.CandidateFlow `json:"candidate_flows"`
}

func buildFlowBundlesFromSnapshot(s snapshot.Snapshot, n int, dw *debugdump.Writer) []explainedFlow {
	if s.GoFacts == nil {
		return nil
	}

	var flows []explainedFlow
	for _, oc := range s.GoFacts.OrientationCandidates {
		if len(flows) >= n {
			break
		}
		if oc.Kind == "unknown" && len(flows) > 0 {
			continue
		}

		cf := flowexplain.CandidateFlow{
			Name:             oc.Name,
			LikelyEntrypoint: oc.EntrypointPackage,
			LikelyFiles:      oc.OpenFiles,
			Confidence:       float64(oc.Priority) / 5.0,
		}
		ef := explainOneFlow(context.Background(), nil, cf, s.FilteredFiles, s.GoFacts, 50, dw, Options{}, false)
		flows = append(flows, ef)
	}
	return flows
}

func explainOneFlow(ctx context.Context, client *deepseek.Client, cf flowexplain.CandidateFlow, trackedFiles []string, facts interface{}, maxFiles int, dw *debugdump.Writer, opts Options, callDeepSeek bool) explainedFlow {
	fid := flowexplain.GenerateFlowID(cf.Name)
	valid, unverified := flowexplain.ValidateSeedFiles(cf.LikelyFiles, trackedFiles)
	queryTerms, aliasTerms := flowexplain.ExtractTerms(cf.Name, cf.Trigger, cf.LikelyEntrypoint, cf.Evidence)

	var gofactsFacts *gofacts.Facts
	if gf, ok := facts.(*gofacts.Facts); ok {
		gofactsFacts = gf
	}
	files, tests, docs, selectedPkgs, relatedEdges := flowexplain.SelectFlowFiles(trackedFiles, aliasTerms, valid, gofactsFacts, maxFiles)

	seed := flowexplain.FlowSeed{
		ID:               fid,
		Name:             cf.Name,
		Trigger:          cf.Trigger,
		LikelyEntrypoint: cf.LikelyEntrypoint,
		ValidSeedFiles:   valid,
		UnverifiedSeeds:  unverified,
		Evidence:         cf.Evidence,
	}

	fb := flowexplain.FlowBundle{
		FlowSeed:         seed,
		QueryTerms:       queryTerms,
		AliasTerms:       aliasTerms,
		SelectedFiles:    files,
		SelectedTests:    tests,
		SelectedDocs:     docs,
		SelectedPackages: selectedPkgs,
		RelatedEdges:     relatedEdges,
	}

	// Add source signals for selected flow files
	if opts.RepoPath != "" {
		var flowFilePaths []string
		for _, f := range files {
			flowFilePaths = append(flowFilePaths, f.Path)
		}
		for _, f := range tests {
			flowFilePaths = append(flowFilePaths, f.Path)
		}
		if len(flowFilePaths) > 0 {
			flowSignals := sourcesignals.ScanSelectedFiles(flowFilePaths, opts.RepoPath, 30)
			if len(flowSignals) > 0 {
				fb.SourceSignals = flowSignals
			}
		}
	}

	bundleJSON, _ := json.MarshalIndent(fb, "", "  ")
	if dw != nil {
		dw.WriteDirFile("flows/"+fid, "flow_bundle.json", bundleJSON)
	}

	summary := flowBundleSummary{
		SelectedFilesCount: len(files),
		SelectedTestsCount: len(tests),
		SelectedDocsCount:  len(docs),
		UnverifiedSeeds:    unverified,
	}

	ef := explainedFlow{
		FlowSeed:          seed,
		FlowBundleSummary: summary,
	}

	if callDeepSeek && client != nil {
		raw, err := callDeepSeekForFlow(ctx, client, fb, dw, fid, opts.DumpLLM)
		if err == nil {
			var report json.RawMessage
			if json.Unmarshal(raw, &report) == nil {
				ef.FlowReport = report
			}
		}
	}

	return ef
}

func formatHumanReadable(report combinedReport, debugDir string, runID string) string {
	var b strings.Builder

	if report.Orientation != nil {
		b.WriteString(fmt.Sprintf("Project: %s\n", report.Orientation.ProjectGuess))
		b.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", report.Orientation.ProjectGuessConfidence()*100))
	}
	b.WriteString(fmt.Sprintf("\n%d candidate flow(s) explained:\n\n", len(report.ExplainedFlows)))

	for i, ef := range report.ExplainedFlows {
		b.WriteString(fmt.Sprintf("━ %s ━\n", ef.FlowSeed.Name))

		var summary string
		var confidence float64
		var chainLen int
		var readFiles []fileToOpen
		var testsToRead []fileToOpen
		var unknowns []string
		var warnings []string

		if ef.FlowReport != nil {
			var fr flowReportFields
			if json.Unmarshal(ef.FlowReport, &fr) == nil {
				summary = fr.Summary
				confidence = fr.Confidence
				chainLen = len(fr.FilesToReadInOrder) // approximate
				readFiles = fr.FilesToReadInOrder
				testsToRead = fr.TestsToRead
				unknowns = fr.Unknowns
				warnings = fr.Warnings
			}
		}

		if summary != "" {
			b.WriteString(fmt.Sprintf("  %s\n", summary))
		} else {
			b.WriteString(fmt.Sprintf("  (selected %d files, %d tests, %d docs)\n",
				ef.FlowBundleSummary.SelectedFilesCount,
				ef.FlowBundleSummary.SelectedTestsCount,
				ef.FlowBundleSummary.SelectedDocsCount))
		}

		if len(readFiles) > 0 {
			b.WriteString(fmt.Sprintf("  Files to read (%d):\n", len(readFiles)))
			for _, f := range readFiles {
				b.WriteString(fmt.Sprintf("    %s\n", f.Path))
				if f.Reason != "" {
					b.WriteString(fmt.Sprintf("      %s\n", f.Reason))
				}
			}
		}

		if len(testsToRead) > 0 {
			b.WriteString(fmt.Sprintf("  Tests (%d):\n", len(testsToRead)))
			for _, t := range testsToRead {
				b.WriteString(fmt.Sprintf("    %s\n", t.Path))
			}
		}

		if len(ef.FlowBundleSummary.UnverifiedSeeds) > 0 {
			b.WriteString(fmt.Sprintf("  Unverified seeds: %v\n", ef.FlowBundleSummary.UnverifiedSeeds))
		}

		if len(unknowns) > 0 {
			b.WriteString(fmt.Sprintf("  Unknowns: %v\n", unknowns))
		}

		if len(warnings) > 0 {
			b.WriteString(fmt.Sprintf("  Warnings: %v\n", warnings))
		}

		_ = confidence
		_ = chainLen

		if i < len(report.ExplainedFlows)-1 {
			b.WriteString("\n")
		}
	}

	if len(report.Warnings) > 0 {
		b.WriteString(fmt.Sprintf("\nWarnings: %v\n", report.Warnings))
	}

	if debugDir != "" {
		b.WriteString(fmt.Sprintf("\nArtifacts: %s/%s\n", debugDir, runID))
	}

	return b.String()
}

func (op *orientationPart) ProjectGuessConfidence() float64 {
	return 0.8
}

func selectTopFlows(flows []flowexplain.CandidateFlow, n int) []flowexplain.CandidateFlow {
	if n <= 0 || len(flows) == 0 {
		return nil
	}
	sorted := make([]flowexplain.CandidateFlow, len(flows))
	copy(sorted, flows)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Confidence < sorted[j].Confidence {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

func callDeepSeekForFlow(ctx context.Context, client *deepseek.Client, fb flowexplain.FlowBundle, dw *debugdump.Writer, fid string, dumpLLM bool) (json.RawMessage, error) {
	bundleJSON, _ := json.MarshalIndent(fb, "", "  ")

	systemPrompt := "You are a senior Go engineer explaining one runtime/event flow in a large unfamiliar Go repository. Use only the provided focused flow bundle. Distinguish evidence from guesses. Return valid json only."

	userPrompt := fmt.Sprintf(`Explain the flow "%s" using only the provided facts bundle. Return json with summary, files_to_read_in_order, tests_to_read, likely_chain, unknowns, and warnings.

Critical JSON shape rules:
- files_to_read_in_order MUST be an array of objects, never strings
- tests_to_read MUST be an array of objects, never strings
- unverified_paths MUST be an array of objects, never strings
- likely_chain[].evidence_files MUST be an array of repo-relative path strings

Each object in files_to_read_in_order/tests_to_read:
  {"path":"relative/file.go","reason":"why to read it","priority":1}

Each object in unverified_paths:
  {"path":"relative/file.go","reason":"why it might not exist"}

Bad example — NEVER do this:
  "files_to_read_in_order": ["a.go"]
  "tests_to_read": ["a_test.go"]

Good example — ALWAYS do this:
  "files_to_read_in_order": [{"path":"a.go","reason":"entrypoint","priority":1}]
  "tests_to_read": [{"path":"a_test.go","reason":"covers the handler"}]

Focused flow bundle:
%s`, fb.FlowSeed.Name, string(bundleJSON))

	if dumpLLM && dw != nil {
		reqPayload, _ := client.FlowExplainPromptJSON(userPrompt, systemPrompt)
		dw.WriteDirFile("flows/"+fid, "llm_request.redacted.json", reqPayload)
	}

	raw, err := client.FlowExplain(ctx, userPrompt, systemPrompt)
	if err != nil {
		return nil, err
	}

	if dumpLLM && dw != nil {
		dw.WriteDirFile("flows/"+fid, "llm_response.raw.json", raw)
	}

	var pretty json.RawMessage
	json.Unmarshal(raw, &pretty)

	if dw != nil {
		dw.WriteDirFile("flows/"+fid, "flow_report.json", raw)
	}

	return raw, nil
}
