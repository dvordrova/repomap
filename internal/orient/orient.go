package orient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/snapshot"
)

type Options struct {
	RepoPath               string
	SnapshotOnly           bool
	LLMBundleOnly          bool
	LLMRequestOnly         bool
	OutputJSON             bool
	Offline                bool
	FlowCount              int
	FlowBundlesOnly        bool
	MaxReadmeBytes         int
	MaxReadmeLLMBytes      int
	MaxTreeLines           int
	MaxInterestingFiles    int
	MaxGoPkgs              int
	MaxGoEdges             int
	MaxLLMEntrypoints      int
	MaxLLMModules          int
	MaxLLMFiles            int
	MaxLocalDirectionFiles int
	MaxLLMEdges            int
	MaxLLMSignals          int
	MaxLLMSignalsPerFile   int
	DebugDir               string
	RunID                  string
	DumpLLM                bool
	DumpRedacted           bool
	RequireArtifacts       bool
	ExplainFlows           int
	Progress               func(ProgressEvent)
}

type combinedReport struct {
	RepoName       string           `json:"repo_name"`
	Orientation    *orientationPart `json:"orientation,omitempty"`
	ExplainedFlows []explainedFlow  `json:"explained_flows"`
	Warnings       []string         `json:"warnings,omitempty"`
}

func Run(ctx context.Context, opts Options) ([]byte, error) {
	requireArtifacts := opts.DumpLLM || opts.RequireArtifacts
	if opts.DumpLLM && opts.Offline {
		return nil, fmt.Errorf("--dump-llm cannot be used with offline mode; use request preview instead")
	}
	if opts.DumpLLM && !opts.SnapshotOnly && !opts.LLMBundleOnly && !opts.LLMRequestOnly && opts.DebugDir == "" {
		return nil, fmt.Errorf("--dump-llm requires a debug directory")
	}
	if opts.RequireArtifacts && !opts.SnapshotOnly && !opts.LLMBundleOnly && !opts.LLMRequestOnly && opts.DebugDir == "" {
		return nil, fmt.Errorf("required browser artifacts need a debug directory")
	}

	emitProgress(opts, ProgressEvent{
		Stage:    ProgressSnapshotStarted,
		RepoPath: opts.RepoPath,
	})

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
		MaxReadmeBytes:   opts.MaxReadmeLLMBytes,
		MaxModules:       opts.MaxLLMModules,
		MaxEntrypoints:   opts.MaxLLMEntrypoints,
		MaxFiles:         opts.MaxLLMFiles,
		MaxEdges:         opts.MaxLLMEdges,
		MaxSignalTotal:   opts.MaxLLMSignals,
		MaxSignalPerFile: opts.MaxLLMSignalsPerFile,
		RepoPath:         opts.RepoPath,
	})
	bundleJSON, _ := json.MarshalIndent(bundle, "", "  ")
	modelBundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal compact model bundle: %w", err)
	}
	emitProgress(opts, ProgressEvent{
		Stage:       ProgressBundleReady,
		RepoName:    s.RepoName,
		BundleBytes: len(modelBundleJSON),
	})

	if opts.LLMBundleOnly {
		out := append(bundleJSON, '\n')
		return out, nil
	}
	if opts.LLMRequestOnly || !opts.Offline {
		if err := validateOrientationBundleForRemote(bundle); err != nil {
			return nil, err
		}
	}
	if opts.LLMRequestOnly {
		client, err := deepseek.NewPromptFromEnv()
		if err != nil {
			return nil, err
		}
		requestJSON, err := client.OrientPromptJSON(modelBundleJSON)
		if err != nil {
			return nil, err
		}
		return requestJSON, nil
	}

	runID := opts.RunID
	if runID == "" {
		runID = debugdump.GenerateRunID(s.RepoName)
	}

	var dw *debugdump.Writer
	runMeta := debugdump.RunMeta{
		RunID:               runID,
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
		RepoName:            s.RepoName,
		RepoPath:            opts.RepoPath,
		Command:             "orient",
		CompactContextBytes: len(modelBundleJSON),
		LLMBundleOnly:       opts.LLMBundleOnly,
	}
	if opts.DebugDir != "" {
		dw, err = debugdump.NewWriter(opts.DebugDir, runID, opts.DumpRedacted)
		if err != nil {
			if requireArtifacts {
				return nil, fmt.Errorf("create required debug writer: %w", err)
			}
			dw = nil
		}
		if dw != nil {
			if err := dw.WriteMetadata(runMeta); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required debug metadata: %w", err)
			}
			if err := dw.WriteSnapshot(snapshotJSON); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required debug snapshot: %w", err)
			}
			if err := dw.WriteLLMBundle(append(modelBundleJSON, '\n')); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required model bundle: %w", err)
			}
		}
	}

	report := combinedReport{
		RepoName: s.RepoName,
	}

	flowCount := opts.FlowCount
	if opts.ExplainFlows > 0 {
		flowCount = opts.ExplainFlows
	}
	if flowCount < 0 {
		flowCount = 0
	}

	if opts.Offline {
		report.Warnings = append(report.Warnings, "offline mode: skipping all LLM calls")
		flows, err := buildFlowBundlesFromSnapshot(s, flowCount, dw, opts)
		if err != nil {
			return nil, err
		}
		report.ExplainedFlows = flows
		report.Warnings = append(report.Warnings, fmt.Sprintf("run %s to get LLM orientation", "repomap "+opts.RepoPath))
	} else {
		client, err := deepseek.NewFromEnv()
		if err != nil {
			if dw != nil {
				dw.WriteError(err)
			}
			return nil, fmt.Errorf(
				"configure REPOMAP_LLM_* for an OpenAI-compatible provider: %w\nOr run local facts only:\n  repomap %s --offline",
				err,
				opts.RepoPath,
			)
		}
		if dw != nil {
			runMeta.Model = client.Model
			runMeta.Endpoint = client.Endpoint
			runMeta.PromptVersion = deepseek.OrientationPromptVersionJSON
			if err := dw.WriteMetadata(runMeta); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required provider metadata: %w", err)
			}
		}

		requestJSON, err := client.OrientPromptJSON(modelBundleJSON)
		if err != nil {
			return nil, err
		}
		runMeta.ExternalRequestBytes = len(requestJSON)
		runMeta.ProviderRequestCount = 1
		if opts.DumpLLM && dw != nil {
			if err := dw.WriteLLMRequest(requestJSON); err != nil {
				return nil, fmt.Errorf("write required llm request before provider call: %w", err)
			}
		}
		emitProgress(opts, ProgressEvent{
			Stage:        ProgressModelRequest,
			RepoName:     s.RepoName,
			Model:        client.Model,
			BundleBytes:  len(modelBundleJSON),
			RequestBytes: len(requestJSON),
		})

		requestStarted := time.Now()
		raw, err := client.Orient(ctx, modelBundleJSON)
		providerLatency := time.Since(requestStarted).Milliseconds()
		runMeta.ProviderLatencyMillis = &providerLatency
		if dw != nil {
			if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write provider latency metadata: %w", metadataErr)
			}
		}
		if err != nil {
			if dw != nil {
				dw.WriteError(err)
			}
			return nil, err
		}
		if err := validateProviderOutputForStorage("orientation", raw); err != nil {
			if dw != nil {
				dw.WriteError(err)
			}
			return nil, err
		}

		if opts.DumpLLM && dw != nil {
			if err := dw.WriteLLMResponse(raw); err != nil {
				return nil, fmt.Errorf("write required llm response: %w", err)
			}
		}

		or, err := parseOrientation(raw)
		if err != nil {
			if dw != nil {
				dw.WriteError(fmt.Errorf("invalid orientation JSON: %s", string(raw)))
			}
			return nil, fmt.Errorf("llm provider returned invalid JSON for orientation")
		}

		allowedEntrypoints := orientationEntrypoints(bundle)
		normalizeOrientationGrounding(&or, bundle.AllowedPaths, allowedEntrypoints)
		if err := validateOrientation(or, bundle.AllowedPaths, allowedEntrypoints); err != nil {
			if dw != nil {
				dw.WriteError(err)
			}
			return nil, err
		}
		report.Orientation = &or
		runMeta.CandidateDirectionCount = len(or.CandidateFlows)
		if dw != nil {
			if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
				return nil, fmt.Errorf("write orientation metadata: %w", metadataErr)
			}
		}
		emitProgress(opts, ProgressEvent{
			Stage:          ProgressOrientationDone,
			RepoName:       s.RepoName,
			Model:          client.Model,
			CandidateCount: len(or.CandidateFlows),
			LatencyMillis:  providerLatency,
		})

		out, _ := json.MarshalIndent(or, "", "  ")
		if dw != nil {
			if err := dw.WriteOrientationReport(out); err != nil && requireArtifacts {
				return nil, fmt.Errorf("write required orientation report: %w", err)
			}
		}

		cfs := selectTopFlows(or.CandidateFlows, flowCount)
		expandedIDs := make(map[string]struct{}, len(cfs))
		for _, candidate := range cfs {
			expandedIDs[flowexplain.GenerateFlowID(candidate.Name)] = struct{}{}
		}
		if err := writeLocalFlowBundles(
			ctx,
			or.CandidateFlows,
			expandedIDs,
			s.FilteredFiles,
			s.GoFacts,
			dw,
			opts,
		); err != nil {
			return nil, err
		}
		for _, cf := range cfs {
			ef := explainOneFlow(ctx, client, cf, s.FilteredFiles, s.GoFacts, opts.MaxLLMFiles, dw, opts, !opts.Offline && !opts.FlowBundlesOnly)
			if ef.ProviderRequestBytes > 0 {
				runMeta.ExternalRequestBytes += ef.ProviderRequestBytes
				runMeta.ProviderRequestCount++
				if dw != nil {
					if metadataErr := dw.WriteMetadata(runMeta); metadataErr != nil && requireArtifacts {
						return nil, fmt.Errorf("write flow metadata: %w", metadataErr)
					}
				}
			}
			if ef.ArtifactError != "" {
				return nil, fmt.Errorf("persist flow %q: %s", cf.Name, ef.ArtifactError)
			}
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
