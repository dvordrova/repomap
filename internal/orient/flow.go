package orient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type explainedFlow struct {
	FlowSeed          flowexplain.FlowSeed `json:"flow_seed"`
	FlowBundleSummary flowBundleSummary    `json:"flow_bundle_summary"`
	FlowReport        json.RawMessage      `json:"flow_report,omitempty"`
	Error             string               `json:"error,omitempty"`
}

type flowBundleSummary struct {
	SelectedFilesCount int      `json:"selected_files_count"`
	SelectedTestsCount int      `json:"selected_tests_count"`
	SelectedDocsCount  int      `json:"selected_docs_count"`
	UnverifiedSeeds    []string `json:"unverified_seed_paths"`
}

func buildFlowBundlesFromSnapshot(s snapshot.Snapshot, n int, dw *debugdump.Writer, opts Options) []explainedFlow {
	if s.GoFacts == nil || n <= 0 {
		return nil
	}
	maxFiles := opts.MaxLLMFiles
	if maxFiles <= 0 {
		maxFiles = 50
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
		ef := explainOneFlow(context.Background(), nil, cf, s.FilteredFiles, s.GoFacts, maxFiles, dw, opts, false)
		flows = append(flows, ef)
	}
	return flows
}

func explainOneFlow(ctx context.Context, client *deepseek.Client, cf flowexplain.CandidateFlow, trackedFiles []string, facts interface{}, maxFiles int, dw *debugdump.Writer, opts Options, callModel bool) explainedFlow {
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
	if dw != nil {
		if err := dw.WriteDirFile("flows/"+fid, "flow_bundle.json", bundleJSON); err != nil && opts.DumpLLM {
			ef.Error = fmt.Sprintf("write required flow bundle: %v", err)
			return ef
		}
	}

	if callModel && client != nil {
		raw, err := callModelForFlow(ctx, client, fb, dw, fid, opts.DumpLLM)
		if err != nil {
			ef.Error = err.Error()
			if dw != nil {
				dw.WriteDirError("flows/"+fid, err)
			}
		} else if normalized, err := normalizeFlowReport(raw, fb); err != nil {
			ef.Error = err.Error()
			if dw != nil {
				dw.WriteDirError("flows/"+fid, err)
			}
		} else {
			ef.FlowReport = json.RawMessage(normalized)
			if dw != nil {
				if err := dw.WriteDirFile("flows/"+fid, "flow_report.json", normalized); err != nil && opts.DumpLLM {
					ef.FlowReport = nil
					ef.Error = fmt.Sprintf("write required normalized flow report: %v", err)
				}
			}
		}
	}

	return ef
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

func callModelForFlow(ctx context.Context, client *deepseek.Client, fb flowexplain.FlowBundle, dw *debugdump.Writer, fid string, dumpLLM bool) (json.RawMessage, error) {
	if err := validateFlowBundleForRemote(fb); err != nil {
		return nil, err
	}
	bundleJSON, _ := json.MarshalIndent(fb, "", "  ")

	systemPrompt := "You are a senior Go engineer explaining one runtime/event flow in a large unfamiliar Go repository. Use only the provided focused flow bundle. Distinguish evidence from guesses. Return valid json only."

	userPrompt := fmt.Sprintf(`Explain the flow "%s" using only the provided facts bundle. Return json with summary, confidence, files_to_read_in_order, tests_to_read, likely_chain, unverified_paths, unknowns, and warnings.

Critical JSON shape rules:
- files_to_read_in_order MUST be an array of objects, never strings
- tests_to_read MUST be an array of objects, never strings
- unverified_paths MUST be an array of objects, never strings
- likely_chain[].evidence_files MUST be an array of repo-relative path strings

Each object in files_to_read_in_order/tests_to_read:
  {"path":"relative/file.go","reason":"why to read it","priority":1}

Each object in unverified_paths:
  {"path":"relative/file.go","reason":"why it might not exist"}

Each object in likely_chain:
  {"step":1,"name":"short step name","what_happens":"supported description","evidence_files":["relative/file.go"],"confidence":0.5}

All documented fields are required. Arrays may be empty. Every likely_chain
step must cite at least one file from the focused bundle. tests_to_read may only
contain *_test.go paths from the focused bundle.

Bad example — NEVER do this:
  "files_to_read_in_order": ["a.go"]
  "tests_to_read": ["a_test.go"]

Good example — ALWAYS do this:
  "files_to_read_in_order": [{"path":"a.go","reason":"entrypoint","priority":1}]
  "tests_to_read": [{"path":"a_test.go","reason":"covers the handler"}]

Focused flow bundle:
%s`, fb.FlowSeed.Name, string(bundleJSON))

	if dumpLLM && dw != nil {
		reqPayload, err := client.FlowExplainPromptJSON(userPrompt, systemPrompt)
		if err != nil {
			return nil, fmt.Errorf("build flow request artifact: %w", err)
		}
		if err := dw.WriteDirFile("flows/"+fid, "llm_request.redacted.json", reqPayload); err != nil {
			return nil, fmt.Errorf("write required flow request before provider call: %w", err)
		}
	} else if dumpLLM {
		return nil, fmt.Errorf("flow request dump requires a debug writer")
	}

	raw, err := client.FlowExplain(ctx, userPrompt, systemPrompt)
	if err != nil {
		return nil, err
	}
	if err := validateProviderOutputForStorage(fmt.Sprintf("flow %q", fb.FlowSeed.Name), raw); err != nil {
		return nil, err
	}

	if dumpLLM && dw != nil {
		if err := dw.WriteDirFile("flows/"+fid, "llm_response.raw.json", raw); err != nil {
			return nil, fmt.Errorf("write required flow response: %w", err)
		}
	}

	return raw, nil
}
