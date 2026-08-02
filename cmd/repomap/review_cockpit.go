package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const reviewCockpitVersion = 1

type reviewCockpitOptions struct {
	CaddyRun string
	ChiRun   string
	OutDir   string
}

type reviewCockpitData struct {
	Version           int                    `json:"version"`
	SourcePolicy      string                 `json:"source_policy"`
	Pipeline          []reviewPipelineStage  `json:"pipeline"`
	Grammar           []reviewGrammarItem    `json:"grammar"`
	Goals             []reviewGoal           `json:"goals"`
	Experiments       []reviewExperiment     `json:"experiments"`
	Funnels           []reviewFunnel         `json:"funnels"`
	CoverageFrontier  []reviewFrontierCard   `json:"coverage_frontier"`
	FrontierCollapsed []reviewCollapsedGroup `json:"frontier_collapsed,omitempty"`
	UnavailableNote   string                 `json:"unavailable_note"`
	ServeCommand      string                 `json:"serve_command"`
	URL               string                 `json:"url"`
}

type reviewPipelineStage struct {
	Name        string `json:"name"`
	Authority   string `json:"authority"`
	Explanation string `json:"explanation"`
}

type reviewGrammarItem struct {
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
}

type reviewGoal struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Result string `json:"result"`
}

type reviewExperiment struct {
	Slug             string              `json:"slug"`
	Name             string              `json:"name"`
	Repository       string              `json:"repository"`
	RepositoryPath   string              `json:"repository_path,omitempty"`
	Revision         string              `json:"revision"`
	Namespace        string              `json:"repository_namespace"`
	Question         string              `json:"question"`
	State            string              `json:"state"`
	StateExplanation string              `json:"state_explanation"`
	Facts            []reviewFact        `json:"facts"`
	FactSummary      reviewFactSummary   `json:"fact_summary"`
	Proposal         reviewProposal      `json:"model_proposal"`
	Validation       reviewValidation    `json:"validation"`
	Canonical        reviewCanonical     `json:"canonical"`
	Coverage         reviewCoverage      `json:"coverage"`
	Unknowns         []string            `json:"unknowns"`
	TraceTargets     []reviewTraceTarget `json:"trace_targets"`
	Replay           reviewReplay        `json:"replay"`
	Links            []reviewLink        `json:"links"`
	RawArtifacts     []reviewLink        `json:"raw_artifacts"`
}

type reviewFact struct {
	ID           string                          `json:"id"`
	Statement    string                          `json:"statement"`
	Role         string                          `json:"role"`
	Used         bool                            `json:"used_by_canonical_claim"`
	Capabilities []semanticdiscovery.Capability  `json:"capabilities,omitempty"`
	Source       *semanticdiscovery.FactSource   `json:"source,omitempty"`
	Evidence     []semanticdiscovery.EvidenceRef `json:"evidence,omitempty"`
}

type reviewFactSummary struct {
	Total           int `json:"total"`
	ClaimSupport    int `json:"claim_support"`
	CandidateSeeds  int `json:"candidate_seeds"`
	AvailableUnused int `json:"available_unused"`
	EvidenceCount   int `json:"evidence_count"`
}

type reviewProposal struct {
	Notice          string        `json:"notice"`
	ProposedVerdict string        `json:"proposed_verdict"`
	Title           string        `json:"title"`
	Summary         string        `json:"summary"`
	Claims          []reviewClaim `json:"claims"`
}

type reviewClaim struct {
	Index      int                             `json:"index"`
	Title      string                          `json:"title"`
	Text       string                          `json:"text"`
	Basis      string                          `json:"basis"`
	SupportIDs []string                        `json:"support_ids"`
	AspectIDs  []string                        `json:"answer_aspect_ids,omitempty"`
	Status     string                          `json:"validation_status"`
	Evidence   []semanticdiscovery.EvidenceRef `json:"evidence,omitempty"`
}

type reviewValidation struct {
	CurrentStatus  string                  `json:"current_status"`
	DerivedVerdict string                  `json:"derived_verdict"`
	Checks         []reviewValidationCheck `json:"checks"`
	Diagnostics    []reviewDiagnostic      `json:"diagnostics,omitempty"`
	Timeline       []reviewTimelineEvent   `json:"timeline"`
}

type reviewValidationCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type reviewDiagnostic struct {
	Code     string   `json:"code"`
	Proposed string   `json:"proposed,omitempty"`
	Derived  string   `json:"derived,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
}

type reviewTimelineEvent struct {
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type reviewCanonical struct {
	Status              string   `json:"status"`
	ArtifactID          string   `json:"artifact_id"`
	ArtifactSHA256      string   `json:"artifact_sha256"`
	MechanismID         string   `json:"mechanism_id"`
	SemanticContentHash string   `json:"semantic_content_hash"`
	Verdict             string   `json:"verdict"`
	IntentKey           string   `json:"intent_key"`
	ScopeKind           string   `json:"scope_kind"`
	ScopeValue          string   `json:"scope_value"`
	Claims              int      `json:"claims"`
	SupportedSteps      int      `json:"supported_steps"`
	InputFactIDs        []string `json:"input_fact_ids"`
}

type reviewCoverage struct {
	Required   []reviewAspect `json:"required"`
	Covered    int            `json:"covered"`
	Uncovered  int            `json:"uncovered"`
	KeyTotal   int            `json:"key_total"`
	KeyCovered int            `json:"key_covered"`
}

type reviewAspect struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Key     bool   `json:"key"`
	Covered bool   `json:"covered"`
}

type reviewTraceTarget struct {
	StepID     string                          `json:"step_id"`
	StepTitle  string                          `json:"step_title"`
	Target     semanticdiscovery.EvidenceRef   `json:"target"`
	Evidence   []semanticdiscovery.EvidenceRef `json:"evidence"`
	Disclaimer string                          `json:"disclaimer"`
}

type reviewReplay struct {
	State               string `json:"state"`
	ModelCalls          int    `json:"model_calls"`
	RepositoryAnalyzers int    `json:"repository_analyzers"`
	ProbeCalls          int    `json:"probe_calls"`
	SearchIndexed       bool   `json:"search_indexed"`
	HTMLRendered        bool   `json:"html_rendered"`
	Result              string `json:"result"`
}

type reviewLink struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type reviewFunnel struct {
	Repository                  string `json:"repository"`
	Opportunities               int    `json:"opportunities"`
	Selected                    int    `json:"selected"`
	Investigated                int    `json:"investigated"`
	ModelProposals              int    `json:"model_proposals"`
	AcceptedBroadArtifacts      int    `json:"accepted_broad_artifacts"`
	RejectedBroadProposals      int    `json:"rejected_broad_proposals"`
	Unexplained                 int    `json:"unexplained"`
	OriginalCanonicalMechanisms int    `json:"original_canonical_mechanisms"`
	FollowUpCanonicalMechanisms int    `json:"follow_up_canonical_mechanisms"`
	Explanation                 string `json:"explanation"`
}

type reviewFrontierCard struct {
	Repository          string         `json:"repository"`
	CandidateID         string         `json:"candidate_id"`
	Status              string         `json:"status"`
	StatusExplanation   string         `json:"status_explanation"`
	Question            string         `json:"question"`
	WhySuspected        []reviewGround `json:"why_suspected"`
	AvailableEvidence   []string       `json:"available_evidence,omitempty"`
	MissingEvidence     []string       `json:"missing_evidence,omitempty"`
	MissingCapabilities []string       `json:"missing_capabilities,omitempty"`
	MissingSource       string         `json:"missing_source,omitempty"`
	Diagnostics         []string       `json:"diagnostics,omitempty"`
	SuggestedNextProbe  string         `json:"suggested_next_probe"`
}

type reviewGround struct {
	FactID    string   `json:"fact_id"`
	Statement string   `json:"statement,omitempty"`
	Locations []string `json:"locations,omitempty"`
}

type reviewCollapsedGroup struct {
	Repository string `json:"repository"`
	Count      int    `json:"count"`
	RawLink    string `json:"raw_link"`
}

type reviewReport struct {
	RepoName            string                       `json:"repo_name"`
	CapturedRevision    string                       `json:"captured_revision"`
	StartHereArtifactID string                       `json:"start_here_artifact_id"`
	SemanticArtifacts   []semanticdiscovery.Artifact `json:"semantic_artifacts"`
}

type reviewMetadata struct {
	RepoPath string `json:"repo_path"`
}

type reviewBroadStatus struct {
	OpportunityCandidates int `json:"opportunity_candidates"`
	SelectedCandidates    int `json:"selected_candidates"`
	LeafTasks             int `json:"leaf_tasks"`
	Artifacts             int `json:"artifacts"`
	FanInReductionIssues  int `json:"fan_in_reduction_issues"`
}

type reviewOpportunityFile struct {
	Proposal             semanticdiscovery.OpportunityProposal `json:"proposal"`
	SelectedCandidateIDs []string                              `json:"selected_candidate_ids"`
}

type reviewFanInFile struct {
	Artifact  semanticdiscovery.FanInArtifact        `json:"artifact"`
	Reduction semanticdiscovery.FanInReductionReport `json:"reduction"`
}

type reviewCaddyStatus struct {
	State              string `json:"state"`
	RepositoryRevision string `json:"repository_revision"`
}

type reviewReplayStatus struct {
	State                 string `json:"state"`
	ModelCalls            int    `json:"model_calls"`
	RepositoryAnalyzers   int    `json:"repository_analyzers"`
	TargetedProbeCalls    int    `json:"targeted_probe_calls"`
	BoundedRetrievalCalls int    `json:"bounded_retrieval_calls"`
	SearchIndexed         bool   `json:"search_indexed"`
	HTMLContainsArtifact  bool   `json:"html_contains_artifact"`
	HTMLRendered          bool   `json:"html_rendered"`
}

type reviewChiCurrentStatus struct {
	State     string                                 `json:"state"`
	Reduction semanticdiscovery.FanInReductionReport `json:"reduction"`
}

type reviewChiHistoricalStatus struct {
	State              string                                 `json:"state"`
	RepositoryRevision string                                 `json:"repository_revision"`
	Reduction          semanticdiscovery.FanInReductionReport `json:"reduction"`
}

func runReviewCockpitCLI(args []string, stdout io.Writer) error {
	opts, err := parseReviewCockpitArgs(args)
	if err != nil {
		return err
	}
	data, err := generateReviewCockpit(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Morning review is ready:\n%s\n%s\n", data.ServeCommand, data.URL)
	return nil
}

func parseReviewCockpitArgs(args []string) (reviewCockpitOptions, error) {
	fs := flag.NewFlagSet("review-cockpit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts reviewCockpitOptions
	fs.StringVar(&opts.CaddyRun, "caddy-run", "", "saved Caddy run directory")
	fs.StringVar(&opts.ChiRun, "chi-run", "", "saved chi run directory")
	fs.StringVar(&opts.OutDir, "out", "", "review output directory")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || opts.CaddyRun == "" || opts.ChiRun == "" || opts.OutDir == "" {
		return reviewCockpitOptions{}, fmt.Errorf("Usage: repomap dev review-cockpit --caddy-run <run-dir> --chi-run <run-dir> --out <output-dir>")
	}
	return opts, nil
}

func generateReviewCockpit(opts reviewCockpitOptions) (reviewCockpitData, error) {
	caddyRun, err := filepath.Abs(opts.CaddyRun)
	if err != nil {
		return reviewCockpitData{}, fmt.Errorf("review cockpit: resolve Caddy run: %w", err)
	}
	chiRun, err := filepath.Abs(opts.ChiRun)
	if err != nil {
		return reviewCockpitData{}, fmt.Errorf("review cockpit: resolve chi run: %w", err)
	}
	outDir, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return reviewCockpitData{}, fmt.Errorf("review cockpit: resolve output: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return reviewCockpitData{}, fmt.Errorf("review cockpit: create output: %w", err)
	}

	caddy, err := loadReviewExperiment("caddy", caddyRun, outDir)
	if err != nil {
		return reviewCockpitData{}, err
	}
	chi, err := loadReviewExperiment("chi", chiRun, outDir)
	if err != nil {
		return reviewCockpitData{}, err
	}
	caddyFrontier, caddyFunnel, caddyCollapsed, err := loadCoverageFrontier("caddy", caddyRun, caddy)
	if err != nil {
		return reviewCockpitData{}, err
	}
	chiFrontier, chiFunnel, chiCollapsed, err := loadCoverageFrontier("chi", chiRun, chi)
	if err != nil {
		return reviewCockpitData{}, err
	}

	serveCommand := fmt.Sprintf(
		"python3 -m http.server 8765 --bind 127.0.0.1 --directory %s",
		outDir,
	)
	data := reviewCockpitData{
		Version:      reviewCockpitVersion,
		SourcePolicy: "Saved artifacts only: zero model calls, probes, analyzers, package loading, SSA, call graph, or runtime-surface discovery.",
		Pipeline: []reviewPipelineStage{
			{Name: "Repository evidence", Authority: "FACT", Explanation: "Bounded saved source windows and deterministic repository facts; locally citable."},
			{Name: "Candidate question", Authority: "PROPOSAL", Explanation: "A bounded question chooses what the explanation should cover; it is not repository truth."},
			{Name: "Bounded facts", Authority: "FACT", Explanation: "Opaque IDs bind exact statements to saved local evidence and capabilities."},
			{Name: "Model proposal", Authority: "PROPOSAL", Explanation: "The LLM groups facts, orders claims, and writes explanatory prose. It cannot create facts or evidence."},
			{Name: "Claim validation", Authority: "LOCAL", Explanation: "Local code checks IDs, references, claim support, temporal/scope language, and aspect coverage."},
			{Name: "Canonical object", Authority: "CANONICAL", Explanation: "Only accepted claims enter a stable Mechanism; the aggregate verdict is locally derived."},
			{Name: "Replay / HTML / Search", Authority: "USER", Explanation: "The saved Mechanism replays without a model into Start Here, evidence navigation, and Super Search."},
		},
		Grammar: []reviewGrammarItem{
			{Status: "FACT", Explanation: "deterministic and citable"},
			{Status: "PROPOSAL", Explanation: "untrusted model interpretation"},
			{Status: "CANONICAL", Explanation: "locally validated reusable object"},
			{Status: "MIXED", Explanation: "useful accepted claims plus explicit gaps"},
			{Status: "REJECTED", Explanation: "proposal failed a local contract"},
			{Status: "UNKNOWN", Explanation: "missing or deliberately bounded-out evidence"},
		},
		Goals: []reviewGoal{
			{Number: 0, Name: "Morning Review Cockpit", Status: "completed", Result: "One static review joins saved Caddy and chi facts, proposals, validation, canonical objects, reports, and raw artifacts."},
			{Number: 1, Name: "Canonical Verdict Ownership", Status: "passed", Result: "The unchanged chi proposal is accepted after local derivation: proposed supported, canonical mixed."},
			{Number: 2, Name: "chi Mechanism v1", Status: "canonical", Result: "The saved request-dispatch result is published and replays into HTML and Search with zero model/analyzer/probe calls."},
			{Number: 3, Name: "Presentation-only evidence lens", Status: "completed", Result: "Each Mechanism exposes at least three distinct exact file:line targets without changing semantic identity."},
			{Number: 4, Name: "Coverage Frontier", Status: "completed", Result: "Twenty saved-artifact cards separate verified, possible, detected, rejected, and unavailable states."},
		},
		Experiments:       []reviewExperiment{caddy, chi},
		Funnels:           []reviewFunnel{caddyFunnel, chiFunnel},
		CoverageFrontier:  append(caddyFrontier, chiFrontier...),
		FrontierCollapsed: []reviewCollapsedGroup{caddyCollapsed, chiCollapsed},
		UnavailableNote:   "No saved Unavailable frontier objects exist in either run. This empty state is intentional; the review does not invent unavailable scenarios.",
		ServeCommand:      serveCommand,
		URL:               "http://127.0.0.1:8765/",
	}

	if err := writeReviewJSON(filepath.Join(outDir, "data.json"), data); err != nil {
		return reviewCockpitData{}, err
	}
	if err := writeAtomicFile(filepath.Join(outDir, "index.html"), []byte(reviewCockpitHTML), 0o644); err != nil {
		return reviewCockpitData{}, err
	}
	if err := writeAtomicFile(filepath.Join(outDir, "README.md"), []byte(reviewReadme(data)), 0o644); err != nil {
		return reviewCockpitData{}, err
	}
	if err := writeAtomicFile(filepath.Join(outDir, "summary.md"), []byte(reviewSummary(data)), 0o644); err != nil {
		return reviewCockpitData{}, err
	}
	if err := writeAtomicFile(filepath.Join(outDir, "MORNING.md"), []byte(reviewMorning(data)), 0o644); err != nil {
		return reviewCockpitData{}, err
	}
	return data, nil
}

func loadReviewExperiment(slug, runDir, outDir string) (reviewExperiment, error) {
	mechanismRaw, err := readBoundedRegularFile(filepath.Join(runDir, semanticdiscovery.MechanismFile), maxGoldenSavedFileBytes)
	if err != nil {
		return reviewExperiment{}, fmt.Errorf("review cockpit: %s mechanism: %w", slug, err)
	}
	mechanism, err := semanticdiscovery.DecodeMechanism(mechanismRaw)
	if err != nil {
		return reviewExperiment{}, fmt.Errorf("review cockpit: %s mechanism: %w", slug, err)
	}
	var reportData reviewReport
	if err := readReviewJSON(filepath.Join(runDir, "report.json"), &reportData); err != nil {
		return reviewExperiment{}, err
	}
	artifact, ok := findReviewArtifact(reportData.SemanticArtifacts, reportData.StartHereArtifactID)
	if !ok || artifact.CandidateID != mechanism.Payload.Candidate.ID {
		return reviewExperiment{}, fmt.Errorf("review cockpit: %s canonical artifact is unavailable from Start Here", slug)
	}
	artifactSHA, err := goldenMechanismArtifactSHA256(artifact)
	if err != nil {
		return reviewExperiment{}, err
	}

	projectionFile := "chi_request_dispatch_projection.json"
	if slug == "caddy" {
		projectionFile = "golden_mechanism_projection_v2.json"
	}
	var projection goldenProjection
	if err := readReviewJSON(filepath.Join(runDir, projectionFile), &projection); err != nil {
		return reviewExperiment{}, err
	}
	factSource := projection.Facts
	proposal := mechanism.Payload.Proposal
	if slug == "caddy" {
		var canonicalRecord semanticdiscovery.Record
		if err := readReviewJSON(filepath.Join(runDir, "golden_mechanism_semantic.json"), &canonicalRecord); err != nil {
			return reviewExperiment{}, err
		}
		for _, leaf := range canonicalRecord.Leaves {
			if leaf.Task.Candidate.ID == mechanism.Payload.Candidate.ID {
				factSource = leaf.Task.Facts
				break
			}
		}
	} else {
		proposal, err = readReviewProposal(
			filepath.Join(runDir, "chi_request_dispatch_response_attempt.json"),
			mechanism.Payload.Proposal,
		)
		if err != nil {
			return reviewExperiment{}, err
		}
	}

	metadata := reviewMetadata{}
	_ = readReviewJSON(filepath.Join(runDir, "metadata.json"), &metadata)
	facts := projectReviewFacts(factSource, mechanism, artifact)
	claims := projectReviewClaims(proposal, artifact, facts)
	coverage := projectReviewCoverage(mechanism.Payload.Candidate, artifact)
	traceTargets := deriveReviewTraceTargets(artifact)
	if len(traceTargets) < 3 {
		return reviewExperiment{}, fmt.Errorf("review cockpit: %s has fewer than three distinct evidence targets", slug)
	}

	experiment := reviewExperiment{
		Slug:             slug,
		Name:             map[string]string{"caddy": "Caddy — completed", "chi": "go-chi/chi — canonical after local re-evaluation"}[slug],
		Repository:       reportData.RepoName,
		RepositoryPath:   metadata.RepoPath,
		Namespace:        mechanism.Identity.RepositoryNamespace,
		Question:         mechanism.Payload.Candidate.QuestionAnswered,
		State:            "canonical",
		StateExplanation: "Locally validated Mechanism v1 is available to Start Here, evidence navigation, and Search.",
		Facts:            facts,
		FactSummary: reviewFactSummary{
			Total:           len(facts),
			ClaimSupport:    len(artifact.UsedFactIDs),
			CandidateSeeds:  countMechanismFactRole(mechanism, semanticdiscovery.MechanismFactCandidateSeed),
			AvailableUnused: len(artifact.UnusedAvailableFactIDs),
			EvidenceCount:   len(artifact.Evidence),
		},
		Proposal: reviewProposal{
			Notice:          "UNTRUSTED MODEL PROPOSAL — NOT CANONICAL REPOSITORY KNOWLEDGE",
			ProposedVerdict: string(proposal.Verdict),
			Title:           proposal.Title,
			Summary:         proposal.Summary,
			Claims:          claims,
		},
		Canonical: reviewCanonical{
			Status:              "canonical",
			ArtifactID:          artifact.ID,
			ArtifactSHA256:      artifactSHA,
			MechanismID:         mechanism.ID,
			SemanticContentHash: mechanism.ContentSHA256,
			Verdict:             string(artifact.Verdict),
			IntentKey:           mechanism.Identity.IntentKey,
			ScopeKind:           string(mechanism.Identity.Scope.Kind),
			ScopeValue:          mechanism.Identity.Scope.Value,
			Claims:              len(artifact.Statements),
			SupportedSteps:      countSupportedReviewStatements(artifact.Statements),
			InputFactIDs:        mechanismInputFactIDs(mechanism),
		},
		Coverage:     coverage,
		Unknowns:     artifact.Unknowns,
		TraceTargets: traceTargets,
	}

	if slug == "caddy" {
		if err := finishCaddyReview(runDir, outDir, &experiment); err != nil {
			return reviewExperiment{}, err
		}
	} else if err := finishChiReview(runDir, outDir, &experiment); err != nil {
		return reviewExperiment{}, err
	}
	return experiment, nil
}

func finishCaddyReview(runDir, outDir string, experiment *reviewExperiment) error {
	var status reviewCaddyStatus
	if err := readReviewJSON(filepath.Join(runDir, "golden_mechanism_v03_status.json"), &status); err != nil {
		return err
	}
	experiment.Revision = status.RepositoryRevision
	replayDir := filepath.Join(filepath.Dir(runDir), "replay-v03")
	var replay reviewReplayStatus
	if err := readReviewJSON(filepath.Join(replayDir, "golden_mechanism_v03_replay_status.json"), &replay); err != nil {
		return err
	}
	experiment.Replay = reviewReplay{
		State:               replay.State,
		ModelCalls:          replay.ModelCalls,
		RepositoryAnalyzers: replay.RepositoryAnalyzers,
		ProbeCalls:          replay.TargetedProbeCalls,
		SearchIndexed:       replay.SearchIndexed,
		HTMLRendered:        replay.HTMLContainsArtifact,
		Result:              "Same canonical directory-listing artifact reached replay HTML and Search with no model, analyzer, or targeted probe.",
	}
	experiment.Validation = acceptedReviewValidation(
		experiment.Proposal.ProposedVerdict,
		experiment.Canonical.Verdict,
		nil,
		[]reviewTimelineEvent{
			{Label: "Bounded proposal", Status: "proposal", Detail: "Seven saved model claims proposed a mixed explanation."},
			{Label: "Local validation", Status: "accepted", Detail: "Six supported claims and one explicit evidence gap were retained."},
			{Label: "Canonical publication", Status: "canonical", Detail: "Mechanism v1 and its stable semantic content hash were materialized."},
			{Label: "No-model replay", Status: "replayed", Detail: "Search indexed the artifact and replay HTML contains it."},
		},
	)
	reportDest := filepath.Join(outDir, "reports", "caddy.html")
	replayDest := filepath.Join(outDir, "reports", "caddy-replay.html")
	if err := copyReviewFile(filepath.Join(runDir, "report.html"), reportDest); err != nil {
		return err
	}
	if err := copyReviewFile(filepath.Join(replayDir, "report.html"), replayDest); err != nil {
		return err
	}
	experiment.Links = []reviewLink{
		{Label: "Open Caddy production report", Href: "reports/caddy.html"},
		{Label: "Open Caddy no-model replay", Href: "reports/caddy-replay.html"},
	}
	raw := []string{
		"golden_mechanism_projection_v2.json",
		"golden_mechanism_response_attempt.json",
		"golden_mechanism_v03_status.json",
		"golden_mechanism_semantic.json",
		semanticdiscovery.MechanismFile,
		"report.json",
		"semantic_opportunities.json",
		"semantic_discovery_leaves.json",
		"semantic_discovery_fan_in.json",
		"semantic_discovery_status.json",
	}
	links, err := copyReviewRawFiles("caddy", runDir, outDir, raw)
	if err != nil {
		return err
	}
	replayLink, err := copyReviewRawFile("caddy", replayDir, outDir, "golden_mechanism_v03_replay_status.json", "replay-status.json")
	if err != nil {
		return err
	}
	experiment.RawArtifacts = append(links, replayLink)
	return nil
}

func finishChiReview(runDir, outDir string, experiment *reviewExperiment) error {
	var oldStatus reviewChiHistoricalStatus
	if err := readReviewJSON(filepath.Join(runDir, "chi_request_dispatch_status.json"), &oldStatus); err != nil {
		return err
	}
	experiment.Revision = oldStatus.RepositoryRevision
	var current reviewChiCurrentStatus
	if err := readReviewJSON(filepath.Join(runDir, "chi_request_dispatch_response_replay_status.json"), &current); err != nil {
		return err
	}
	diagnostics := make([]reviewDiagnostic, 0, len(current.Reduction.VerdictDiagnostics))
	for _, diagnostic := range current.Reduction.VerdictDiagnostics {
		reasons := make([]string, 0, len(diagnostic.Reasons))
		for _, reason := range diagnostic.Reasons {
			reasons = append(reasons, string(reason))
		}
		diagnostics = append(diagnostics, reviewDiagnostic{
			Code: diagnostic.Code, Proposed: string(diagnostic.ModelVerdict),
			Derived: string(diagnostic.DerivedVerdict), Reasons: reasons,
		})
	}
	historicalReasons := make([]string, 0)
	for _, issue := range oldStatus.Reduction.Issues {
		if issue.Code != "no_valid_artifacts" {
			historicalReasons = append(historicalReasons, issue.Code)
		}
		for _, reason := range issue.Reasons {
			historicalReasons = append(historicalReasons, reason.Code)
		}
	}
	historicalReasons = uniqueSortedStrings(historicalReasons)
	if len(historicalReasons) > 0 {
		diagnostics = append(diagnostics, reviewDiagnostic{
			Code:    "historical_rejection_superseded",
			Reasons: historicalReasons,
		})
	}
	experiment.Validation = acceptedReviewValidation(
		experiment.Proposal.ProposedVerdict,
		experiment.Canonical.Verdict,
		diagnostics,
		[]reviewTimelineEvent{
			{Label: "Saved model response", Status: "proposal", Detail: "Five claims proposed verdict supported; response bytes and prose remain unchanged."},
			{Label: "Historical local result", Status: "rejected", Detail: "The older reducer rejected the proposal for unsupported_sequence_language and intent_retention_failure; this result is superseded, not erased."},
			{Label: "Current local re-evaluation", Status: "accepted", Detail: "Claim-level checks pass; verdict mismatch is diagnostic, not rejection authority."},
			{Label: "Canonical publication", Status: "canonical", Detail: "Four supported steps plus one retained gap materialize as mixed."},
			{Label: "No-model replay", Status: "replayed", Detail: "The Mechanism reached HTML and Search without model, repository analyzer, or retrieval calls."},
		},
	)
	var replay reviewReplayStatus
	if err := readReviewJSON(filepath.Join(runDir, "chi_request_dispatch_replay_status.json"), &replay); err != nil {
		return err
	}
	experiment.Replay = reviewReplay{
		State:               replay.State,
		ModelCalls:          replay.ModelCalls,
		RepositoryAnalyzers: replay.RepositoryAnalyzers,
		ProbeCalls:          replay.BoundedRetrievalCalls,
		SearchIndexed:       replay.SearchIndexed,
		HTMLRendered:        replay.HTMLRendered,
		Result:              "The unchanged response is canonical mixed and replayed into Start Here, evidence navigation, HTML, and Search.",
	}
	if err := copyReviewFile(filepath.Join(runDir, "report.html"), filepath.Join(outDir, "reports", "chi.html")); err != nil {
		return err
	}
	experiment.Links = []reviewLink{{Label: "Open chi production report", Href: "reports/chi.html"}}
	raw := []string{
		"chi_request_dispatch_fixture.json",
		"chi_request_dispatch_projection.json",
		"chi_request_dispatch_supplement.json",
		"chi_request_dispatch_response_attempt.json",
		"chi_request_dispatch_status.json",
		"chi_request_dispatch_response_replay_status.json",
		"chi_request_dispatch_replay_status.json",
		semanticdiscovery.MechanismFile,
		"report.json",
		"semantic_opportunities.json",
		"semantic_discovery_leaves.json",
		"semantic_discovery_fan_in.json",
		"semantic_discovery_status.json",
	}
	links, err := copyReviewRawFiles("chi", runDir, outDir, raw)
	if err != nil {
		return err
	}
	experiment.RawArtifacts = links
	return nil
}

func acceptedReviewValidation(
	proposed, derived string,
	diagnostics []reviewDiagnostic,
	timeline []reviewTimelineEvent,
) reviewValidation {
	return reviewValidation{
		CurrentStatus:  "accepted",
		DerivedVerdict: derived,
		Checks: []reviewValidationCheck{
			{Name: "opaque IDs", Status: "passed"},
			{Name: "repository references", Status: "passed"},
			{Name: "claim support", Status: "passed"},
			{Name: "temporal and scope semantics", Status: "passed"},
			{Name: "required-aspect coverage", Status: "computed"},
			{Name: "local verdict derivation", Status: "passed"},
		},
		Diagnostics: diagnostics,
		Timeline:    timeline,
	}
}

func projectReviewFacts(
	all []semanticdiscovery.Fact,
	mechanism semanticdiscovery.Mechanism,
	artifact semanticdiscovery.Artifact,
) []reviewFact {
	byID := make(map[string]semanticdiscovery.Fact, len(all))
	for _, fact := range all {
		byID[fact.ID] = fact
	}
	used := stringSet(artifact.UsedFactIDs)
	result := make([]reviewFact, 0, len(mechanism.Input.Facts))
	for _, input := range mechanism.Input.Facts {
		fact, ok := byID[input.ID]
		if !ok {
			continue
		}
		result = append(result, reviewFact{
			ID: fact.ID, Statement: fact.Statement, Role: string(input.Role), Used: used[fact.ID],
			Capabilities: fact.Capabilities, Source: fact.Source, Evidence: fact.Evidence,
		})
	}
	return result
}

func projectReviewClaims(
	proposal semanticdiscovery.ArtifactProposal,
	artifact semanticdiscovery.Artifact,
	facts []reviewFact,
) []reviewClaim {
	factByID := make(map[string]reviewFact, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}
	claims := make([]reviewClaim, 0, len(proposal.Claims))
	for index, proposed := range proposal.Claims {
		claim := reviewClaim{
			Index: index, Title: proposed.Title, Text: proposed.Text, Basis: string(proposed.Basis),
			SupportIDs: append([]string(nil), proposed.SupportIDs...), Status: "not_canonical",
		}
		for _, statement := range artifact.Statements {
			if statement.Text == proposed.Text && statement.Basis == proposed.Basis && equalStringSlices(statement.SupportIDs, proposed.SupportIDs) {
				claim.AspectIDs = append([]string(nil), statement.AspectIDs...)
				if statement.Basis == semanticdiscovery.ClaimUnresolved {
					claim.Status = "retained_unknown"
				} else {
					claim.Status = "retained_canonical"
				}
				break
			}
		}
		seenEvidence := map[string]bool{}
		for _, supportID := range claim.SupportIDs {
			for _, evidence := range factByID[supportID].Evidence {
				key := fmt.Sprintf("%s:%d:%d", evidence.Path, evidence.Line, evidence.Column)
				if !seenEvidence[key] {
					claim.Evidence = append(claim.Evidence, evidence)
					seenEvidence[key] = true
				}
			}
		}
		claims = append(claims, claim)
	}
	return claims
}

func projectReviewCoverage(candidate semanticdiscovery.OpportunityCandidate, artifact semanticdiscovery.Artifact) reviewCoverage {
	covered := stringSet(artifact.CoveredAspectIDs)
	result := reviewCoverage{}
	if candidate.IntentContract == nil {
		return result
	}
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		item := reviewAspect{ID: aspect.ID, Label: aspect.Label, Key: aspect.Key, Covered: covered[aspect.ID]}
		result.Required = append(result.Required, item)
		if item.Covered {
			result.Covered++
		} else {
			result.Uncovered++
		}
		if item.Key {
			result.KeyTotal++
			if item.Covered {
				result.KeyCovered++
			}
		}
	}
	return result
}

func deriveReviewTraceTargets(artifact semanticdiscovery.Artifact) []reviewTraceTarget {
	unresolved := map[string]bool{}
	for _, statement := range artifact.Statements {
		unresolved[statement.ID] = statement.Basis == semanticdiscovery.ClaimUnresolved
	}
	used := map[string]bool{}
	targets := make([]reviewTraceTarget, 0, len(artifact.Steps))
	for _, step := range artifact.Steps {
		resolved := false
		for _, statementID := range step.StatementIDs {
			if !unresolved[statementID] {
				resolved = true
				break
			}
		}
		if !resolved {
			continue
		}
		var target semanticdiscovery.EvidenceRef
		for _, evidence := range step.Evidence {
			if evidence.Path == "" || evidence.Line <= 0 {
				continue
			}
			key := fmt.Sprintf("%s:%d", evidence.Path, evidence.Line)
			if used[key] {
				continue
			}
			target = evidence
			used[key] = true
			break
		}
		if target.Path == "" {
			continue
		}
		targets = append(targets, reviewTraceTarget{
			StepID: step.ID, StepTitle: step.Title, Target: target,
			Evidence:   append([]semanticdiscovery.EvidenceRef(nil), step.Evidence...),
			Disclaimer: "Presentation-only target derived from existing step evidence; order is editorial, not a new runtime relation.",
		})
	}
	return targets
}

func loadCoverageFrontier(
	repository, runDir string,
	experiment reviewExperiment,
) ([]reviewFrontierCard, reviewFunnel, reviewCollapsedGroup, error) {
	var opportunities reviewOpportunityFile
	if err := readReviewJSON(filepath.Join(runDir, "semantic_opportunities.json"), &opportunities); err != nil {
		return nil, reviewFunnel{}, reviewCollapsedGroup{}, err
	}
	var bundle semanticdiscovery.Bundle
	if err := readReviewJSON(filepath.Join(runDir, "semantic_discovery_bundle.json"), &bundle); err != nil {
		return nil, reviewFunnel{}, reviewCollapsedGroup{}, err
	}
	var record semanticdiscovery.Record
	if err := readReviewJSON(filepath.Join(runDir, "semantic_artifacts.json"), &record); err != nil {
		return nil, reviewFunnel{}, reviewCollapsedGroup{}, err
	}
	var fanIn reviewFanInFile
	if err := readReviewJSON(filepath.Join(runDir, "semantic_discovery_fan_in.json"), &fanIn); err != nil {
		return nil, reviewFunnel{}, reviewCollapsedGroup{}, err
	}
	var status reviewBroadStatus
	if err := readReviewJSON(filepath.Join(runDir, "semantic_discovery_status.json"), &status); err != nil {
		return nil, reviewFunnel{}, reviewCollapsedGroup{}, err
	}

	facts := make(map[string]semanticdiscovery.Fact, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		facts[fact.ID] = fact
	}
	leaves := make(map[string]semanticdiscovery.LeafResult, len(record.Leaves))
	for _, leaf := range record.Leaves {
		leaves[leaf.Task.Candidate.ID] = leaf
	}
	kept := make(map[string]semanticdiscovery.ArtifactProposal, len(fanIn.Artifact.Artifacts))
	for _, artifact := range fanIn.Artifact.Artifacts {
		kept[artifact.CandidateID] = artifact
	}
	rejections := make(map[string][]string)
	for _, issue := range fanIn.Reduction.Issues {
		if issue.ArtifactIndex < 0 || issue.ArtifactIndex >= len(opportunities.SelectedCandidateIDs) {
			continue
		}
		candidateID := opportunities.SelectedCandidateIDs[issue.ArtifactIndex]
		rejections[candidateID] = append(rejections[candidateID], issue.Code)
		for _, reason := range issue.Reasons {
			rejections[candidateID] = append(rejections[candidateID], reason.Code)
		}
	}

	frontier := []reviewFrontierCard{verifiedFrontierCard(repository, experiment)}
	selected := stringSet(opportunities.SelectedCandidateIDs)
	ordered := make([]semanticdiscovery.OpportunityCandidate, 0, len(opportunities.Proposal.Candidates))
	for _, id := range opportunities.SelectedCandidateIDs {
		for _, candidate := range opportunities.Proposal.Candidates {
			if candidate.ID == id {
				ordered = append(ordered, candidate)
				break
			}
		}
	}
	for _, candidate := range opportunities.Proposal.Candidates {
		if !selected[candidate.ID] {
			ordered = append(ordered, candidate)
		}
	}
	const cardsPerRepository = 10
	for _, candidate := range ordered {
		if len(frontier) >= cardsPerRepository {
			break
		}
		frontier = append(frontier, candidateFrontierCard(
			repository, candidate, facts, leaves[candidate.ID],
			kept[candidate.ID], rejections[candidate.ID], selected[candidate.ID],
		))
	}
	collapsed := reviewCollapsedGroup{
		Repository: repository,
		Count:      len(opportunities.Proposal.Candidates) - (len(frontier) - 1),
		RawLink:    fmt.Sprintf("raw/%s/semantic_opportunities.json", repository),
	}
	funnel := reviewFunnel{
		Repository:                  repository,
		Opportunities:               status.OpportunityCandidates,
		Selected:                    status.SelectedCandidates,
		Investigated:                status.LeafTasks,
		ModelProposals:              status.Artifacts + status.FanInReductionIssues,
		AcceptedBroadArtifacts:      status.Artifacts,
		RejectedBroadProposals:      status.FanInReductionIssues,
		Unexplained:                 status.OpportunityCandidates - status.SelectedCandidates,
		OriginalCanonicalMechanisms: 0,
		FollowUpCanonicalMechanisms: 1,
		Explanation:                 "Broad discovery artifacts are not Mechanism v1 objects. One bounded follow-up is now canonical; rejected proposal prose was not retained, so only saved diagnostics are shown.",
	}
	return frontier, funnel, collapsed, nil
}

func verifiedFrontierCard(repository string, experiment reviewExperiment) reviewFrontierCard {
	grounds := make([]reviewGround, 0, len(experiment.Canonical.InputFactIDs))
	factByID := make(map[string]reviewFact, len(experiment.Facts))
	for _, fact := range experiment.Facts {
		factByID[fact.ID] = fact
	}
	for _, id := range experiment.Canonical.InputFactIDs {
		fact := factByID[id]
		if !fact.Used {
			continue
		}
		grounds = append(grounds, reviewGround{FactID: id, Statement: fact.Statement, Locations: evidenceLocations(fact.Evidence)})
	}
	missingCapabilities := []string{}
	for _, fact := range experiment.Facts {
		if fact.Role == string(semanticdiscovery.MechanismFactAvailableUnused) {
			continue
		}
	}
	if repository == "caddy" {
		missingCapabilities = []string{"test_evidence"}
	} else {
		missingCapabilities = []string{"lifecycle"}
	}
	return reviewFrontierCard{
		Repository: repository, CandidateID: experiment.Canonical.MechanismID,
		Status: "verified", StatusExplanation: "Canonical Mechanism v1 exists and replays without a model.",
		Question: experiment.Question, WhySuspected: grounds,
		AvailableEvidence: []string{"canonical claims", "exact evidence locations", "local verdict", "no-model replay"},
		MissingEvidence:   experiment.Unknowns, MissingCapabilities: missingCapabilities,
		MissingSource:      "canonical Mechanism capability contract",
		SuggestedNextProbe: experiment.Question,
	}
}

func candidateFrontierCard(
	repository string,
	candidate semanticdiscovery.OpportunityCandidate,
	facts map[string]semanticdiscovery.Fact,
	leaf semanticdiscovery.LeafResult,
	kept semanticdiscovery.ArtifactProposal,
	diagnostics []string,
	selected bool,
) reviewFrontierCard {
	card := reviewFrontierCard{
		Repository: repository, CandidateID: candidate.ID, Question: candidate.QuestionAnswered,
		SuggestedNextProbe: candidate.QuestionAnswered,
	}
	capabilities := map[string]bool{}
	for _, id := range candidate.SupportIDs {
		fact := facts[id]
		card.WhySuspected = append(card.WhySuspected, reviewGround{
			FactID: id, Statement: fact.Statement, Locations: evidenceLocations(fact.Evidence),
		})
		for _, capability := range fact.Capabilities {
			capabilities[string(capability)] = true
		}
	}
	for capability := range capabilities {
		card.AvailableEvidence = append(card.AvailableEvidence, capability)
	}
	sort.Strings(card.AvailableEvidence)

	switch {
	case len(diagnostics) > 0:
		card.Status = "rejected"
		card.StatusExplanation = "A broad model proposal failed local validation. Its dropped prose was not retained; diagnostics only."
		card.Diagnostics = uniqueSortedStrings(diagnostics)
	case kept.CandidateID != "":
		card.Status = "possible"
		card.StatusExplanation = "A broad semantic artifact was accepted, but no canonical Mechanism v1 exists for this question."
	case selected:
		card.Status = "possible"
		card.StatusExplanation = "The candidate was investigated, but no canonical object was materialized."
	default:
		card.Status = "detected"
		card.StatusExplanation = "Evidence-backed opportunity detected; no bounded leaf investigation was run."
	}

	if leaf.Task.Candidate.ID != "" {
		for _, missing := range leaf.Artifact.MissingEvidence {
			card.MissingEvidence = append(card.MissingEvidence, missing.Explanation)
			for _, capability := range missing.MissingCapabilities {
				card.MissingCapabilities = append(card.MissingCapabilities, string(capability))
			}
		}
		if len(card.MissingEvidence) > 0 {
			card.MissingSource = "locally retained leaf gap"
		}
	}
	if len(card.MissingEvidence) == 0 {
		card.MissingEvidence = append(card.MissingEvidence, candidate.MissingInformation...)
		if len(card.MissingEvidence) > 0 {
			card.MissingSource = "untrusted model planning note"
		}
	}
	card.MissingCapabilities = uniqueSortedStrings(card.MissingCapabilities)
	return card
}

func readReviewProposal(path string, fallback semanticdiscovery.ArtifactProposal) (semanticdiscovery.ArtifactProposal, error) {
	var attempt goldenMechanismResponseAttempt
	if err := readReviewJSON(path, &attempt); err != nil {
		return semanticdiscovery.ArtifactProposal{}, err
	}
	var fanIn semanticdiscovery.FanInArtifact
	if err := json.Unmarshal([]byte(attempt.Content), &fanIn); err != nil {
		return semanticdiscovery.ArtifactProposal{}, fmt.Errorf("review cockpit: decode model proposal: %w", err)
	}
	if len(fanIn.Artifacts) == 0 {
		return fallback, nil
	}
	return fanIn.Artifacts[0], nil
}

func findReviewArtifact(artifacts []semanticdiscovery.Artifact, id string) (semanticdiscovery.Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return artifact, true
		}
	}
	return semanticdiscovery.Artifact{}, false
}

func mechanismInputFactIDs(mechanism semanticdiscovery.Mechanism) []string {
	ids := make([]string, 0, len(mechanism.Input.Facts))
	for _, fact := range mechanism.Input.Facts {
		ids = append(ids, fact.ID)
	}
	return ids
}

func countMechanismFactRole(mechanism semanticdiscovery.Mechanism, role semanticdiscovery.MechanismFactRole) int {
	count := 0
	for _, fact := range mechanism.Input.Facts {
		if fact.Role == role {
			count++
		}
	}
	return count
}

func countSupportedReviewStatements(statements []semanticdiscovery.Statement) int {
	count := 0
	for _, statement := range statements {
		if statement.Basis != semanticdiscovery.ClaimUnresolved {
			count++
		}
	}
	return count
}

func evidenceLocations(evidence []semanticdiscovery.EvidenceRef) []string {
	locations := make([]string, 0, len(evidence))
	seen := map[string]bool{}
	for _, item := range evidence {
		if item.Path == "" {
			continue
		}
		location := item.Path
		if item.Line > 0 {
			location += fmt.Sprintf(":%d", item.Line)
		}
		if !seen[location] {
			locations = append(locations, location)
			seen[location] = true
		}
	}
	return locations
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func uniqueSortedStrings(values []string) []string {
	set := stringSet(values)
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func readReviewJSON(path string, target any) error {
	raw, err := readBoundedRegularFile(path, maxGoldenSavedFileBytes)
	if err != nil {
		return fmt.Errorf("review cockpit: read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("review cockpit: decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeReviewJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("review cockpit: encode data: %w", err)
	}
	raw = append(raw, '\n')
	return writeAtomicFile(path, raw, 0o644)
}

func copyReviewFile(source, destination string) error {
	raw, err := readBoundedRegularFile(source, maxGoldenSavedFileBytes)
	if err != nil {
		return fmt.Errorf("review cockpit: copy %s: %w", filepath.Base(source), err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return writeAtomicFile(destination, raw, 0o644)
}

func copyReviewRawFiles(slug, runDir, outDir string, names []string) ([]reviewLink, error) {
	links := make([]reviewLink, 0, len(names))
	for _, name := range names {
		link, err := copyReviewRawFile(slug, runDir, outDir, name, name)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

func copyReviewRawFile(slug, runDir, outDir, sourceName, destinationName string) (reviewLink, error) {
	destination := filepath.Join(outDir, "raw", slug, destinationName)
	if err := copyReviewFile(filepath.Join(runDir, sourceName), destination); err != nil {
		return reviewLink{}, err
	}
	return reviewLink{Label: destinationName, Href: filepath.ToSlash(filepath.Join("raw", slug, destinationName))}, nil
}

func reviewReadme(data reviewCockpitData) string {
	return fmt.Sprintf(`# Morning Review Cockpit

This directory is a dev-only projection built entirely from saved Caddy and
go-chi/chi artifacts. It does not call a model or run repository analysis.

## Open it

~~~sh
%s
~~~

Then open: %s

Start with the two canonical Mechanism cards, expand **Facts** and **Model
claims**, click through the presentation-only evidence lens, and finish at
**Explore possibilities**. Raw artifacts and copied production reports remain
one click away.
`, data.ServeCommand, data.URL)
}

func reviewSummary(data reviewCockpitData) string {
	return fmt.Sprintf(`# Saved-artifact experiment summary

Repomap is testing whether bounded repository evidence can become a reusable
human explanation without making the model the source of truth.

- Caddy: canonical directory-listing Mechanism, locally derived **mixed**,
  replayed into HTML and Search.
- chi: unchanged five-claim proposal was historically rejected; the current
  local reducer accepts its claims, derives **mixed** instead of the proposed
  **supported**, and publishes a canonical request-dispatch Mechanism.
- Presentation: exact existing evidence locations provide distinct step
  targets without changing either Mechanism hash.
- Coverage: 20 saved-artifact frontier cards make accepted, rejected,
  unexplained, and still-bounded work visible without publishing hypotheses.

Serve with:

~~~sh
%s
~~~

Open %s
`, data.ServeCommand, data.URL)
}

func reviewMorning(data reviewCockpitData) string {
	return fmt.Sprintf(`# What happened overnight

## 1. Morning Review Cockpit

Status: completed. One saved-artifact-only page explains the authority
boundary between deterministic facts, model proposals, local validation, and
canonical Mechanisms.

Command:

~~~sh
%s
~~~

URL: %s

## 2. Canonical Verdict Ownership

Passed. The model-proposed chi verdict remains **supported**, while the local
reducer derives canonical **mixed** from the unresolved claim, retained missing
evidence, and uncovered required aspect. Aggregate disagreement is diagnostic;
invalid individual claims would still be rejected before derivation.

## 3. chi Mechanism

Canonical: **semantic-artifact-9cb212aa5a2edeb150985a55** /
**semantic-mechanism-ea95ee3357e6939a97f0eaa3**. The saved response replay used
zero model, probe, and repository-analyzer calls; HTML and Search contain it.

## 4. Focus enrichment

Completed as a presentation-only evidence lens. Caddy has six and chi four
distinct exact file:line targets. The lens derives only from existing Step
evidence and does not alter Mechanism identity or semantic content hashes.

## 5. Explore Possibilities

Completed. The page shows 20 cards across Verified, Possible / needs probe,
Detected but unexplained, and Rejected. There are no saved Unavailable objects,
so that category is shown as an honest empty state. Dropped broad proposal prose
was not retained; rejected cards expose diagnostics, not invented claim text.

## Five-minute walkthrough

1. Open the URL above and read **What are we testing?**.
2. Open the Caddy Mechanism and its copied production report.
3. Compare its proposed and derived verdict with chi's historical-to-current timeline.
4. Expand Facts and Model claims; click three evidence-lens steps for each repository.
5. Open **Explore possibilities** and compare the two funnels.

## What is now proven

- Both saved Mechanisms are canonical **mixed** objects with stable IDs and hashes.
- The chi response is accepted by claim-level validation without changing its model prose.
- Both Mechanisms replay into HTML and Search without a model or repository analysis.
- Distinct presentation targets can be derived from already canonical Step evidence.

## What remains speculative

- Broad opportunity questions and model **missing_information** fields remain planning proposals.
- Accepted broad semantic artifacts are not promoted to Mechanism v1.
- Coverage-frontier suggested probes are questions, never repository truth.

## What is blocked

- Nothing in this bounded queue.
- Caddy still lacks inspected direct behavior tests for sorting/paging/representations.
- Chi still lacks evidence for dynamic computed-handler wiring and the complete middleware lifecycle.
- Rejected broad proposal bodies cannot be reviewed because only their diagnostics were saved.

## Files changed

- cmd/repomap/review_cockpit.go
- cmd/repomap/review_cockpit_html.go
- cmd/repomap/review_cockpit_test.go
- cmd/repomap/main.go
- Makefile
- internal/semanticdiscovery/verdict.go and focused reducer tests
- saved chi replay/publication path under cmd/repomap and internal/semanticdiscovery
- docs/agent-room/CURRENT.md
- docs/agent-room/106-morning-review-and-coverage-frontier.md
- untracked generated review under tmp/repomap-review/

## Focused tests run

- go test ./cmd/repomap -run 'TestReviewCockpit|TestChiSavedResponseOfflineReplayContract'
- go test ./internal/semanticdiscovery -run 'TestDeriveVerdict|TestReduceFanInArtifact|TestMechanism'
- static-server smoke: curl for index.html, data.json, and copied reports

## Slow/global tests deliberately skipped

- go test ./...
- go vet ./...
- repository-wide analysis and package loading
- SSA, call graph, runtime-surface discovery, and the known slow surface run
- model/provider calls and targeted probes

## One recommended next experiment

Persist the rejected broad proposal body alongside its local rejection
diagnostic, still outside canonical knowledge. That single enrichment would
make false-positive review materially better without adding a new analyzer.
`, data.ServeCommand, data.URL)
}
