package orient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type explainedFlow struct {
	FlowSeed          flowexplain.FlowSeed `json:"flow_seed"`
	FlowBundleSummary flowBundleSummary    `json:"flow_bundle_summary"`
	ArtifactError     string               `json:"-"`
}

const flowArtifactStatusVersion = 1

const (
	flowStatusLocalOnly = "local_only"
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
		ef := explainOneFlow(cf, s.FilteredFiles, s.GoFacts, maxFiles, dw, opts)
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
		CandidateBasis:   flowexplain.CandidateBasisLocalEntrypoint,
	}
	if candidate.Kind == gofacts.OrientationKindSignalFlow {
		flow.Name += " (offline hint)"
		flow.FlowType = flowexplain.FlowTypeOperational
		flow.Confidence = min(flow.Confidence, 0.3)
		flow.CandidateBasis = flowexplain.CandidateBasisSourceSignalAggregate
	}
	return flow
}

// writeLocalFlowBundles persists a deterministic focused bundle for every
// orientation direction that is not already going through model expansion.
// The browser can therefore reveal useful local evidence after a direction is
// selected without another provider call or a long-lived local server.
func writeLocalFlowBundles(
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
			candidate,
			trackedFiles,
			facts,
			maxFiles,
			dw,
			opts,
		)
		if result.ArtifactError != "" {
			return fmt.Errorf("write local evidence for direction %q: %s", candidate.Name, result.ArtifactError)
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

func explainOneFlow(
	cf flowexplain.CandidateFlow,
	trackedFiles []string,
	facts interface{},
	maxFiles int,
	dw *debugdump.Writer,
	opts Options,
) explainedFlow {
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
		CandidateBasis:   cf.CandidateBasis,
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
	requireArtifacts := opts.RequireArtifacts
	if dw != nil {
		if err := dw.WriteDirFile("flows/"+fid, "flow_bundle.json", bundleJSON); err != nil {
			if requireArtifacts {
				ef.ArtifactError = fmt.Sprintf("write required flow bundle: %v", err)
				return ef
			}
		}
	} else if requireArtifacts {
		ef.ArtifactError = "flow artifact writer is required"
		return ef
	}

	if err := writeFlowArtifactStatus(dw, fid, flowStatusLocalOnly, requireArtifacts); err != nil {
		ef.ArtifactError = fmt.Sprintf("write local flow status: %v", err)
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
