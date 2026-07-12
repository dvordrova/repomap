package orient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type explainedFlow struct {
	FlowSeed             flowexplain.FlowSeed `json:"flow_seed"`
	FlowBundleSummary    flowBundleSummary    `json:"flow_bundle_summary"`
	FlowReport           json.RawMessage      `json:"flow_report,omitempty"`
	Error                string               `json:"error,omitempty"`
	ProviderRequestBytes int                  `json:"-"`
	ArtifactError        string               `json:"-"`
}

const flowArtifactStatusVersion = 1

const (
	flowStatusLocalOnly          = "local_only"
	flowStatusExpansionRequested = "expansion_requested"
	flowStatusSucceeded          = "succeeded"
	flowStatusFailed             = "failed"
)

type flowArtifactStatus struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"`
}

type flowBundleSummary struct {
	SelectedFilesCount int      `json:"selected_files_count"`
	SelectedTestsCount int      `json:"selected_tests_count"`
	SelectedDocsCount  int      `json:"selected_docs_count"`
	UnverifiedSeeds    []string `json:"unverified_seed_paths"`
}

func buildFlowBundlesFromSnapshot(s snapshot.Snapshot, n int, dw *debugdump.Writer, opts Options) ([]explainedFlow, error) {
	if s.GoFacts == nil || n <= 0 {
		return nil, nil
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
		cf := offlineCandidateFlow(oc)
		ef := explainOneFlow(context.Background(), nil, cf, s.FilteredFiles, s.GoFacts, maxFiles, dw, opts, false)
		if ef.ArtifactError != "" {
			return nil, fmt.Errorf("persist local direction %q: %s", cf.Name, ef.ArtifactError)
		}
		flows = append(flows, ef)
	}
	return flows, nil
}

func offlineCandidateFlow(candidate gofacts.OrientationCandidate) flowexplain.CandidateFlow {
	flow := flowexplain.CandidateFlow{
		Name:             candidate.Name,
		FlowType:         flowexplain.FlowTypeRequest,
		LikelyEntrypoint: candidate.EntrypointPackage,
		LikelyFiles:      candidate.OpenFiles,
		WhyInteresting:   candidate.Why,
		Confidence:       float64(candidate.Priority) / 5.0,
	}
	if candidate.Kind == gofacts.OrientationKindSignalFlow {
		flow.Name += " (offline hint)"
		flow.FlowType = flowexplain.FlowTypeOperational
		flow.Confidence = min(flow.Confidence, 0.3)
	}
	return flow
}

// writeLocalFlowBundles persists a deterministic focused bundle for every
// orientation direction that is not already going through model expansion.
// The browser can therefore reveal useful local evidence after a direction is
// selected without another provider call or a long-lived local server.
func writeLocalFlowBundles(
	ctx context.Context,
	candidates []flowexplain.CandidateFlow,
	skippedIDs map[string]struct{},
	trackedFiles []string,
	facts *gofacts.Facts,
	dw *debugdump.Writer,
	opts Options,
) error {
	if dw == nil {
		return nil
	}
	maxFiles := opts.MaxLocalDirectionFiles
	if maxFiles <= 0 {
		maxFiles = 20
	}
	for _, candidate := range candidates {
		flowID := flowexplain.GenerateFlowID(candidate.Name)
		if _, skipped := skippedIDs[flowID]; skipped {
			continue
		}
		result := explainOneFlow(
			ctx,
			nil,
			candidate,
			trackedFiles,
			facts,
			maxFiles,
			dw,
			opts,
			false,
		)
		if result.Error != "" {
			return fmt.Errorf("write local evidence for direction %q: %s", candidate.Name, result.Error)
		}
	}
	return nil
}

func writeFlowArtifactStatus(dw *debugdump.Writer, flowID, mode string, required bool) error {
	if dw == nil {
		if required {
			return fmt.Errorf("flow artifact writer is required")
		}
		return nil
	}
	data, err := json.MarshalIndent(flowArtifactStatus{
		Version: flowArtifactStatusVersion,
		Mode:    mode,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := dw.WriteDirFile("flows/"+flowID, "flow_status.json", append(data, '\n')); err != nil {
		if required {
			return err
		}
	}
	return nil
}

func explainOneFlow(ctx context.Context, client *deepseek.Client, cf flowexplain.CandidateFlow, trackedFiles []string, facts interface{}, maxFiles int, dw *debugdump.Writer, opts Options, callModel bool) explainedFlow {
	identityName := strings.TrimSuffix(cf.Name, " (offline hint)")
	fid := flowexplain.GenerateFlowID(identityName)
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
		FlowType:         cf.FlowType,
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
	requireArtifacts := opts.DumpLLM || opts.RequireArtifacts
	if dw != nil {
		if err := dw.WriteDirFile("flows/"+fid, "flow_bundle.json", bundleJSON); err != nil {
			if requireArtifacts {
				ef.Error = fmt.Sprintf("write required flow bundle: %v", err)
				ef.ArtifactError = ef.Error
				return ef
			}
		}
	} else if requireArtifacts {
		ef.Error = "flow artifact writer is required"
		ef.ArtifactError = ef.Error
		return ef
	}

	if !callModel {
		if err := writeFlowArtifactStatus(dw, fid, flowStatusLocalOnly, requireArtifacts); err != nil {
			ef.Error = fmt.Sprintf("write local flow status: %v", err)
			ef.ArtifactError = ef.Error
		}
		return ef
	}
	if client == nil {
		ef.Error = "flow model client is required"
		return ef
	}
	if err := writeFlowArtifactStatus(dw, fid, flowStatusExpansionRequested, requireArtifacts); err != nil {
		ef.Error = fmt.Sprintf("write requested flow status: %v", err)
		ef.ArtifactError = ef.Error
		return ef
	}

	raw, requestBytes, artifactFailure, err := callModelForFlow(ctx, client, fb, dw, fid, opts.DumpLLM)
	ef.ProviderRequestBytes = requestBytes
	if err != nil {
		ef.Error = err.Error()
		if artifactFailure {
			ef.ArtifactError = ef.Error
		}
		if dw != nil {
			dw.WriteDirError("flows/"+fid, err)
		}
		if statusErr := writeFlowArtifactStatus(dw, fid, flowStatusFailed, requireArtifacts); statusErr != nil {
			ef.ArtifactError = fmt.Sprintf("write failed flow status: %v", statusErr)
		}
	} else if normalized, normalizeErr := normalizeFlowReport(raw, fb); normalizeErr != nil {
		ef.Error = normalizeErr.Error()
		if dw != nil {
			dw.WriteDirError("flows/"+fid, normalizeErr)
		}
		if statusErr := writeFlowArtifactStatus(dw, fid, flowStatusFailed, requireArtifacts); statusErr != nil {
			ef.ArtifactError = fmt.Sprintf("write failed flow status: %v", statusErr)
		}
	} else {
		ef.FlowReport = json.RawMessage(normalized)
		if dw != nil {
			if err := dw.WriteDirFile("flows/"+fid, "flow_report.json", normalized); err != nil && requireArtifacts {
				ef.FlowReport = nil
				ef.Error = fmt.Sprintf("write required normalized flow report: %v", err)
				ef.ArtifactError = ef.Error
				return ef
			}
		}
		if statusErr := writeFlowArtifactStatus(dw, fid, flowStatusSucceeded, requireArtifacts); statusErr != nil {
			ef.Error = fmt.Sprintf("write successful flow status: %v", statusErr)
			ef.ArtifactError = ef.Error
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

func callModelForFlow(ctx context.Context, client *deepseek.Client, fb flowexplain.FlowBundle, dw *debugdump.Writer, fid string, dumpLLM bool) (json.RawMessage, int, bool, error) {
	if err := validateFlowBundleForRemote(fb); err != nil {
		return nil, 0, false, err
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

	reqPayload, err := client.FlowExplainPromptJSON(userPrompt, systemPrompt)
	if err != nil {
		return nil, 0, false, fmt.Errorf("build flow request: %w", err)
	}
	if dumpLLM && dw != nil {
		if err := dw.WriteDirFile("flows/"+fid, "llm_request.redacted.json", reqPayload); err != nil {
			return nil, 0, true, fmt.Errorf("write required flow request before provider call: %w", err)
		}
	} else if dumpLLM {
		return nil, 0, true, fmt.Errorf("flow request dump requires a debug writer")
	}

	raw, err := client.FlowExplain(ctx, userPrompt, systemPrompt)
	if err != nil {
		return nil, len(reqPayload), false, err
	}
	if err := validateProviderOutputForStorage(fmt.Sprintf("flow %q", fb.FlowSeed.Name), raw); err != nil {
		return nil, len(reqPayload), false, err
	}

	if dumpLLM && dw != nil {
		if err := dw.WriteDirFile("flows/"+fid, "llm_response.raw.json", raw); err != nil {
			return nil, len(reqPayload), true, fmt.Errorf("write required flow response: %w", err)
		}
	}

	return raw, len(reqPayload), false, nil
}
