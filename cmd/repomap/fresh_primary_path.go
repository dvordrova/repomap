package main

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

const (
	freshPrimaryPlanVersion        = 2
	freshPrimaryMaxFiles           = 8
	freshPrimaryMaxRootFiles       = 4
	freshPrimaryMaxAdditionalFiles = freshPrimaryMaxFiles - freshPrimaryMaxRootFiles
	freshPrimaryMaxFunctions       = 10
	freshPrimaryMaxAdditionalFuncs = freshPrimaryMaxFunctions - freshRepoDemoMaxSeedFuncs
	// One exact call establishes the bridge start. From that exact target the
	// planner may follow at most two additional repository-local frontiers.
	freshPrimaryMaxFrontiers     = 2
	freshPrimaryMaxPathFrontiers = 1 + freshPrimaryMaxFrontiers
	freshPrimaryMaxDepth         = 3
	freshPrimaryMaxRetainedBytes = 128 << 10
	// Parsing is local-only and never enters a model request. A 128 KiB cap
	// admits large production files while retained source remains independently
	// capped at 128 KiB.
	freshPrimaryMaxParsedFileBytes = 128 << 10
	freshPrimaryMaxEnumeratedCalls = 32
	freshPrimaryTimeout            = 5 * time.Second

	freshPrimaryMinPurposeAlignment = 1
	freshPrimaryMinExplanation      = 4
	freshPrimaryMinNavigation       = 2
	freshPrimaryMinEvidence         = 3
)

const (
	freshPrimaryAspectInput  = "input-trigger"
	freshPrimaryAspectCore   = "core-work"
	freshPrimaryAspectEffect = "observable-effect"
)

type freshPrimaryPlanStatus string

const (
	freshPrimaryPlanReady                freshPrimaryPlanStatus = "ready"
	freshPrimaryPlanRequiresBetterAnchor freshPrimaryPlanStatus = "requires_better_anchor"
	freshPrimaryPlanInsufficient         freshPrimaryPlanStatus = "insufficient_primary_evidence"
)

type freshPrimaryAspectRole string

const (
	freshPrimaryRoleInput      freshPrimaryAspectRole = "input_trigger"
	freshPrimaryRoleCore       freshPrimaryAspectRole = "core_work"
	freshPrimaryRoleEffect     freshPrimaryAspectRole = "observable_effect"
	freshPrimaryRoleSupporting freshPrimaryAspectRole = "supporting"
)

type freshPrimaryBoundaryKind string

const (
	freshBoundaryFileWrite        freshPrimaryBoundaryKind = "file_write"
	freshBoundaryDatabaseWrite    freshPrimaryBoundaryKind = "database_write"
	freshBoundaryNetworkSend      freshPrimaryBoundaryKind = "network_send"
	freshBoundaryBackendInterface freshPrimaryBoundaryKind = "backend_interface"
	freshBoundaryPublicOutput     freshPrimaryBoundaryKind = "public_output"
)

type freshPrimaryIntentInput struct {
	RepositoryNamespace string
	Question            string
	Kind                semanticdiscovery.ArtifactKind
	Scope               semanticdiscovery.MechanismScope
	CentralAnchorIDs    []string
}

type freshPrimaryAspect struct {
	ID                   string                         `json:"id"`
	Role                 freshPrimaryAspectRole         `json:"role"`
	Label                string                         `json:"label"`
	RequiredCapabilities []semanticdiscovery.Capability `json:"required_capabilities"`
	Key                  bool                           `json:"key"`
}

type freshPrimaryAnchor struct {
	ID               string `json:"id"`
	OriginFactID     string `json:"origin_fact_id"`
	OriginEvidenceID string `json:"origin_evidence_id"`
	Path             string `json:"path"`
	Symbol           string `json:"symbol"`
	ContentSHA256    string `json:"content_sha256"`
	Depth            int    `json:"depth"`
}

type freshPrimaryFrontier struct {
	ID               string `json:"id"`
	FromAnchorID     string `json:"from_anchor_id"`
	CallEvidenceID   string `json:"call_evidence_id"`
	CallPath         string `json:"call_path"`
	CallLine         int    `json:"call_line"`
	CallColumn       int    `json:"call_column"`
	TargetPath       string `json:"target_path,omitempty"`
	TargetSymbol     string `json:"target_symbol,omitempty"`
	TargetContentSHA string `json:"target_content_sha256,omitempty"`
	ReceiverType     string `json:"receiver_type,omitempty"`
	Operation        string `json:"operation"`
	Depth            int    `json:"depth"`
	Score            int    `json:"score"`
	Selected         bool   `json:"selected"`
	Resolution       string `json:"resolution"`
	ResolutionReason string `json:"resolution_reason,omitempty"`
	DemotionReason   string `json:"demotion_reason,omitempty"`
}

type freshPrimaryEffectBoundary struct {
	ID             string                   `json:"id"`
	Kind           freshPrimaryBoundaryKind `json:"kind"`
	FunctionPath   string                   `json:"function_path"`
	FunctionSymbol string                   `json:"function_symbol"`
	Operation      string                   `json:"operation"`
	ReceiverType   string                   `json:"receiver_type,omitempty"`
	EvidenceID     string                   `json:"evidence_id"`
	Line           int                      `json:"line"`
	Column         int                      `json:"column"`
	ClaimBoundary  string                   `json:"claim_boundary"`
}

type freshPrimaryLimits struct {
	MaxFrontierExpansions int           `json:"max_frontier_expansions"`
	MaxFiles              int           `json:"max_files"`
	MaxFunctions          int           `json:"max_functions"`
	MaxAdditionalFiles    int           `json:"max_additional_files"`
	MaxAdditionalFuncs    int           `json:"max_additional_functions"`
	MaxRetainedBytes      int           `json:"max_retained_source_bytes"`
	MaxDepth              int           `json:"max_depth"`
	Timeout               time.Duration `json:"timeout"`
}

type freshPrimaryEligibility struct {
	Status                 freshPrimaryPlanStatus `json:"status"`
	Reasons                []string               `json:"reasons,omitempty"`
	InputFactIDs           []string               `json:"input_fact_ids,omitempty"`
	CoreFactIDs            []string               `json:"core_fact_ids,omitempty"`
	EffectFactIDs          []string               `json:"effect_fact_ids,omitempty"`
	DistinctSymbols        []string               `json:"distinct_symbols,omitempty"`
	DistinctFiles          []string               `json:"distinct_files,omitempty"`
	IntentCollision        bool                   `json:"intent_collision"`
	AllLoggingOrErrorFacts bool                   `json:"all_logging_or_error_facts"`
}

type freshPrimaryProbePlan struct {
	Version              int                         `json:"version"`
	CandidateID          string                      `json:"candidate_id"`
	Question             string                      `json:"question"`
	IntentKey            string                      `json:"intent_key"`
	Status               freshPrimaryPlanStatus      `json:"status"`
	StatusReasons        []string                    `json:"status_reasons,omitempty"`
	RootAnchors          []freshPrimaryAnchor        `json:"root_anchors"`
	Aspects              []freshPrimaryAspect        `json:"aspects"`
	DesiredBoundaryKinds []freshPrimaryBoundaryKind  `json:"desired_boundary_kinds"`
	EnumeratedFrontiers  []freshPrimaryFrontier      `json:"enumerated_frontiers,omitempty"`
	SelectedFrontiers    []freshPrimaryFrontier      `json:"selected_frontiers,omitempty"`
	EffectBoundary       *freshPrimaryEffectBoundary `json:"effect_boundary,omitempty"`
	EffectResolution     string                      `json:"effect_resolution"`
	EffectFailureClass   string                      `json:"effect_failure_class,omitempty"`
	Limits               freshPrimaryLimits          `json:"limits"`
	StopConditions       []string                    `json:"stop_conditions"`
	AdditionalFilesRead  []string                    `json:"additional_files_read,omitempty"`
	RetainedSourceBytes  int                         `json:"retained_source_bytes"`
	ElapsedMillis        int64                       `json:"elapsed_millis"`
	Eligibility          freshPrimaryEligibility     `json:"eligibility"`
	AnchorFacts          []semanticdiscovery.Fact    `json:"anchor_facts,omitempty"`
	ProjectedFacts       []semanticdiscovery.Fact    `json:"projected_facts,omitempty"`
}

type freshPrimaryCandidateWork struct {
	ProbeFacts []semanticdiscovery.Fact
}

type freshPrimaryParsedFile struct {
	Path      string
	Data      []byte
	File      *ast.File
	FSet      *token.FileSet
	Functions map[string]freshPrimaryFunction
	Fields    map[string]map[string]string
	Imports   map[string]string
}

type freshPrimaryFunction struct {
	Function    sourcewindowfacts.Function
	Declaration *ast.FuncDecl
	PackageName string
	Receiver    string
	Calls       []freshPrimaryCall
}

type freshPrimaryCall struct {
	ID                 string
	Path               string
	Function           string
	Target             string
	Terminal           string
	ReceiverType       string
	ReceiverChain      string
	ImportPath         string
	ReceiverImportPath string
	Line               int
	Column             int
	EndLine            int
	EndColumn          int
}

func freshPrimaryIntentKey(input freshPrimaryIntentInput) string {
	anchors := sortedGoldenStrings(input.CentralAnchorIDs)
	parts := []string{
		"primary-path-v1",
		strings.TrimSpace(input.RepositoryNamespace),
		string(input.Kind),
		string(input.Scope.Kind),
		strings.TrimSpace(input.Scope.Value),
		freshNormalizeQuestion(input.Question),
	}
	parts = append(parts, anchors...)
	return goldenStableID("fresh-primary", parts...)
}

func freshNormalizeQuestion(question string) string {
	var normalized strings.Builder
	space := true
	for _, r := range strings.ToLower(strings.TrimSpace(question)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
			space = false
			continue
		}
		if !space {
			normalized.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(normalized.String())
}

func freshPrimaryAspects(question string) []freshPrimaryAspect {
	question = strings.TrimSpace(question)
	return []freshPrimaryAspect{
		{
			ID: freshPrimaryAspectInput, Role: freshPrimaryRoleInput,
			Label:                "Input or trigger for the selected question",
			RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityEntry},
			Key:                  true,
		},
		{
			ID: freshPrimaryAspectCore, Role: freshPrimaryRoleCore,
			Label: "Repository work that answers: " + question,
			RequiredCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDirectCall,
			},
			Key: true,
		},
		{
			ID: freshPrimaryAspectEffect, Role: freshPrimaryRoleEffect,
			Label:                "Observable effect or typed boundary of that work",
			RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityOutputEffect},
			Key:                  true,
		},
	}
}

func freshPrimaryAnswerAspects(aspects []freshPrimaryAspect) []semanticdiscovery.AnswerAspect {
	result := make([]semanticdiscovery.AnswerAspect, 0, len(aspects))
	for _, aspect := range aspects {
		result = append(result, semanticdiscovery.AnswerAspect{
			ID: aspect.ID, Label: aspect.Label,
			RequiredCapabilities: append([]semanticdiscovery.Capability(nil), aspect.RequiredCapabilities...),
			Key:                  aspect.Key,
		})
	}
	return result
}

func selectFreshPrimaryCandidates(
	repoRoot string,
	data *report.ReportData,
	proposal semanticdiscovery.OpportunityProposal,
	sources []freshSourceFunction,
) []freshCandidateWork {
	candidates := proposal.Candidates
	if len(candidates) > freshRepoDemoMaxCandidates {
		candidates = candidates[:freshRepoDemoMaxCandidates]
	}
	works := make([]freshCandidateWork, 0, len(candidates))
	for _, candidate := range candidates {
		work := planFreshPrimaryCandidate(repoRoot, data, candidate, sources)
		works = append(works, work)
	}
	freshMarkPrimaryIntentCollisions(works)
	sort.SliceStable(works, func(i, j int) bool {
		leftCentral := freshCandidateIsEligibleFirstContact(works[i])
		rightCentral := freshCandidateIsEligibleFirstContact(works[j])
		if leftCentral != rightCentral {
			return leftCentral
		}
		leftReady := works[i].Plan.Primary != nil && works[i].Plan.Primary.Status == freshPrimaryPlanReady
		rightReady := works[j].Plan.Primary != nil && works[j].Plan.Primary.Status == freshPrimaryPlanReady
		if leftReady != rightReady {
			return leftReady
		}
		if comparison := compareFreshCandidateCentrality(works[i].Plan.Centrality, works[j].Plan.Centrality); comparison != 0 {
			return comparison > 0
		}
		return works[i].Candidate.ID < works[j].Candidate.ID
	})
	if len(works) > freshRepoDemoMaxCandidates {
		works = works[:freshRepoDemoMaxCandidates]
	}
	return works
}

func planFreshPrimaryCandidate(
	repoRoot string,
	data *report.ReportData,
	candidate semanticdiscovery.OpportunityCandidate,
	sources []freshSourceFunction,
) freshCandidateWork {
	started := time.Now()
	primary := &freshPrimaryProbePlan{
		Version:     freshPrimaryPlanVersion,
		CandidateID: candidate.ID,
		Question:    candidate.QuestionAnswered,
		Status:      freshPrimaryPlanRequiresBetterAnchor,
		Aspects:     freshPrimaryAspects(candidate.QuestionAnswered),
		DesiredBoundaryKinds: []freshPrimaryBoundaryKind{
			freshBoundaryFileWrite, freshBoundaryDatabaseWrite, freshBoundaryNetworkSend,
			freshBoundaryBackendInterface, freshBoundaryPublicOutput,
		},
		Limits: freshPrimaryLimits{
			MaxFrontierExpansions: freshPrimaryMaxFrontiers,
			MaxFiles:              freshPrimaryMaxFiles,
			MaxFunctions:          freshPrimaryMaxFunctions,
			MaxAdditionalFiles:    freshPrimaryMaxAdditionalFiles,
			MaxAdditionalFuncs:    freshPrimaryMaxAdditionalFuncs,
			MaxRetainedBytes:      freshPrimaryMaxRetainedBytes,
			MaxDepth:              freshPrimaryMaxDepth,
			Timeout:               freshPrimaryTimeout,
		},
		StopConditions: []string{
			"validated_effect_boundary", "frontier_expansion_limit", "additional_file_limit",
			"additional_function_limit", "retained_source_byte_limit", "timeout",
		},
	}
	work := freshCandidateWork{Candidate: candidate, InitialSources: sources, Primary: &freshPrimaryCandidateWork{}}
	defer func() { primary.ElapsedMillis = time.Since(started).Milliseconds() }()

	reader, err := reporead.New(repoRoot)
	if err != nil {
		primary.StatusReasons = []string{"repository_reader_unavailable"}
		work.Plan = freshPrimaryCandidatePlan(data, candidate, sources, nil, primary)
		return work
	}
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), freshPrimaryTimeout)
	defer cancel()
	state := freshPrimaryPlannerState{
		ctx: ctx, reader: reader, data: data, candidate: candidate, sources: sources,
		files:        make(map[string]*freshPrimaryParsedFile),
		functions:    make(map[string]freshPrimaryFunction),
		limitReasons: make(map[string]struct{}),
	}
	roots, rootSources, err := state.resolveRoots()
	if err != nil || len(roots) == 0 {
		primary.StatusReasons = []string{"candidate_anchor_did_not_resolve_to_local_function"}
		work.Plan = freshPrimaryCandidatePlan(data, candidate, sources, rootSources, primary)
		work.Seeds = rootSources
		return work
	}
	primary.RootAnchors = roots
	work.Seeds = rootSources
	work.InitialSources = mergeFreshSourceFunctions(rootSources, sources, freshRepoDemoMaxSourceFacts)

	pkg, ok := freshOwningPackage(data, rootSources)
	if !ok {
		primary.StatusReasons = []string{"candidate_anchor_has_no_repository_package"}
		work.Plan = freshPrimaryCandidatePlan(data, candidate, sources, rootSources, primary)
		return work
	}
	scope := semanticdiscovery.MechanismScope{Kind: semanticdiscovery.MechanismScopeGoPackage, Value: pkg.CanonicalPath}
	anchorIDs := make([]string, 0, len(roots))
	for _, root := range roots {
		anchorIDs = append(anchorIDs, root.ID)
	}
	primary.IntentKey = freshPrimaryIntentKey(freshPrimaryIntentInput{
		RepositoryNamespace: pkg.ModulePath,
		Question:            candidate.QuestionAnswered,
		Kind:                candidate.Kind,
		Scope:               scope,
		CentralAnchorIDs:    anchorIDs,
	})

	selected, boundary := state.expand(roots)
	primary.EnumeratedFrontiers = state.frontiers
	primary.SelectedFrontiers = selected
	primary.EffectBoundary = boundary
	primary.EffectResolution, primary.EffectFailureClass = state.effectResolution(boundary)
	primary.AdditionalFilesRead = sortedGoldenStrings(state.additionalFiles)
	primary.RetainedSourceBytes = state.retainedSourceBytes(roots, selected)
	state.retainedBytes = primary.RetainedSourceBytes

	probeFacts := state.projectFacts(primary, roots, selected, boundary)
	work.Primary.ProbeFacts = probeFacts
	primary.AnchorFacts = freshFacts(rootSources)
	primary.ProjectedFacts = append([]semanticdiscovery.Fact(nil), probeFacts...)
	primary.Eligibility = deriveFreshPrimaryEligibility(primary, probeFacts, false)
	primary.Status = primary.Eligibility.Status
	primary.StatusReasons = append([]string(nil), primary.Eligibility.Reasons...)
	if primary.EffectBoundary == nil && primary.EffectFailureClass != "" {
		primary.StatusReasons = appendUniqueFreshString(
			primary.StatusReasons,
			primary.EffectFailureClass,
		)
		primary.Eligibility.Reasons = appendUniqueFreshString(
			primary.Eligibility.Reasons,
			primary.EffectFailureClass,
		)
	}

	work.Plan = freshPrimaryCandidatePlan(data, candidate, sources, rootSources, primary)
	work.Plan.Identity = semanticdiscovery.MechanismIdentity{
		RepositoryNamespace: pkg.ModulePath,
		IntentKey:           primary.IntentKey,
		Scope:               scope,
	}
	work.Plan.Aspects = freshPrimaryAnswerAspects(primary.Aspects)
	remaining := freshPrimaryTimeout - time.Since(started)
	if remaining <= 0 {
		primary.Status = freshPrimaryPlanInsufficient
		primary.StatusReasons = append(primary.StatusReasons, "local_planning_timeout")
		primary.Eligibility.Status = freshPrimaryPlanInsufficient
		primary.Eligibility.Reasons = append(primary.Eligibility.Reasons, "local_planning_timeout")
		return work
	}
	work.Plan.Probe = freshPrimaryGoldenPlan(primary, roots, selected, probeFacts, remaining)
	work.Plan.ExpectedFrontier = freshPrimarySelectedSymbols(
		freshPrimaryAdditionalFrontiers(selected),
	)
	freshApplyPrimaryProductEligibility(&work)
	return work
}

func freshPrimaryCandidatePlan(
	data *report.ReportData,
	candidate semanticdiscovery.OpportunityCandidate,
	sources []freshSourceFunction,
	seeds []freshSourceFunction,
	primary *freshPrimaryProbePlan,
) freshRepoCandidatePlan {
	anchorFacts := make([]string, 0, len(seeds))
	anchorEvidence := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		anchorFacts = append(anchorFacts, seed.Fact.ID)
		if len(seed.Fact.Evidence) > 0 {
			anchorEvidence = append(anchorEvidence, seed.Fact.Evidence[0].ID)
		}
	}
	anchorFacts = sortedGoldenStrings(anchorFacts)
	anchorEvidence = sortedGoldenStrings(anchorEvidence)
	purposeOverlap := freshPurposeOverlap(data, candidate)
	centrality := deriveFreshCandidateCentrality(data, candidate, sources, seeds, purposeOverlap)
	return freshRepoCandidatePlan{
		CandidateID:       candidate.ID,
		Question:          candidate.QuestionAnswered,
		Kind:              candidate.Kind,
		AnchorFactIDs:     anchorFacts,
		AnchorEvidenceIDs: anchorEvidence,
		CentralityReason:  freshCentralityReason(candidate, seeds, centrality),
		RequiredCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityEntry,
			semanticdiscovery.CapabilityBehavior,
			semanticdiscovery.CapabilityOutputEffect,
		},
		AvailableCapabilities:   freshSeedCapabilities(seeds),
		ExpectedOnboardingValue: candidate.ExpectedValue,
		LocalScore:              centrality.localScore(),
		Centrality:              centrality,
		ProductIntent:           candidate.ProductIntent,
		Primary:                 primary,
	}
}

func freshPrimaryGoldenPlan(
	primary *freshPrimaryProbePlan,
	roots []freshPrimaryAnchor,
	selected []freshPrimaryFrontier,
	facts []semanticdiscovery.Fact,
	timeout time.Duration,
) goldenmechanism.Plan {
	factByEvidence := make(map[string]semanticdiscovery.Fact)
	factByScope := make(map[string]semanticdiscovery.Fact)
	for _, fact := range facts {
		for _, reference := range fact.Evidence {
			factByEvidence[reference.ID] = fact
		}
		if fact.Source != nil {
			factByScope[fact.Source.Path+"\x00"+fact.Source.EnclosingSymbol] = fact
		}
	}
	rootLimit := len(roots)
	if len(selected) > freshPrimaryMaxFrontiers && rootLimit >= freshRepoDemoMaxSeedFuncs {
		// The first selected target is the exact bridge start, not an
		// additional expansion. Reserve one seed slot for it while keeping the
		// two downstream frontier slots unchanged.
		rootLimit = freshRepoDemoMaxSeedFuncs - 1
	}
	seeds := make([]goldenmechanism.Seed, 0, rootLimit+len(selected))
	for _, root := range roots[:rootLimit] {
		originFactID := root.OriginFactID
		originEvidenceID := root.OriginEvidenceID
		if fact, exists := factByScope[root.Path+"\x00"+root.Symbol]; exists {
			originFactID = fact.ID
			if len(fact.Evidence) > 0 {
				originEvidenceID = fact.Evidence[0].ID
			}
		}
		seeds = append(seeds, goldenmechanism.Seed{
			OriginFactID:     originFactID,
			OriginEvidenceID: originEvidenceID,
			Path:             root.Path,
			Symbol:           root.Symbol,
		})
	}
	for _, frontier := range selected {
		originFactID := roots[0].OriginFactID
		if fact, exists := factByEvidence[frontier.CallEvidenceID]; exists {
			originFactID = fact.ID
		}
		seeds = append(seeds, goldenmechanism.Seed{
			OriginFactID:          originFactID,
			OriginEvidenceID:      frontier.CallEvidenceID,
			Path:                  frontier.TargetPath,
			Symbol:                frontier.TargetSymbol,
			Depth:                 frontier.Depth,
			ReachedFromEvidenceID: frontier.CallEvidenceID,
		})
	}
	return goldenmechanism.Plan{
		MechanismID: primary.IntentKey,
		Seeds:       seeds,
		ExpansionAllowlist: freshPrimarySelectedSymbols(
			freshPrimaryAdditionalFrontiers(selected),
		),
		Limits: goldenmechanism.Limits{
			MaxDepth: freshPrimaryMaxDepth,
			MaxFiles: max(
				1,
				min(freshRepoDemoMaxProbeFiles, len(freshPrimaryUniqueSeedPaths(seeds))),
			),
			MaxFunctions:         freshPrimaryMaxFunctions,
			MaxParsedSourceBytes: freshRepoDemoMaxParsedBytes,
			MaxSourceBytes:       freshPrimaryMaxRetainedBytes,
			MaxFunctionLines:     freshRepoOnboardingMaxFunctionLines,
			MaxFunctionBytes:     48 << 10,
			Timeout:              timeout,
		},
	}
}

func freshPrimaryAdditionalFrontiers(
	selected []freshPrimaryFrontier,
) []freshPrimaryFrontier {
	if len(selected) <= freshPrimaryMaxFrontiers {
		return selected
	}
	return selected[len(selected)-freshPrimaryMaxFrontiers:]
}

func freshPrimaryUniqueSeedPaths(seeds []goldenmechanism.Seed) []string {
	var paths []string
	for _, seed := range seeds {
		paths = append(paths, seed.Path)
	}
	return sortedGoldenStrings(paths)
}

func freshPrimarySelectedSymbols(frontiers []freshPrimaryFrontier) []string {
	values := make([]string, 0, len(frontiers))
	for _, frontier := range frontiers {
		if frontier.TargetSymbol != "" {
			values = append(values, frontier.TargetSymbol)
		}
	}
	return sortedGoldenStrings(values)
}

func freshCandidateCentralAnchorIDs(
	candidate semanticdiscovery.OpportunityCandidate,
) []string {
	if candidate.ProductIntent == nil {
		return candidate.SupportIDs
	}
	return candidate.ProductIntent.CentralAnchorIDs
}

func freshCandidatePlanningAnchorIDs(
	candidate semanticdiscovery.OpportunityCandidate,
) []string {
	if candidate.ProductIntent == nil {
		return append([]string(nil), candidate.SupportIDs...)
	}
	intent := candidate.ProductIntent
	result := make([]string, 0, len(candidate.SupportIDs))
	seen := make(map[string]struct{}, len(candidate.SupportIDs))
	appendFirst := func(ids []string) {
		if len(ids) == 0 {
			return
		}
		id := ids[0]
		if _, duplicate := seen[id]; duplicate {
			return
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	appendAll := func(ids []string) {
		for _, id := range ids {
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}

	// Reserve one root slot for each planned answer aspect before filling
	// the remaining bounded root budget with central anchors. Every ID is
	// still an exact candidate support ID checked by local validation.
	appendFirst(intent.ExpectedPath.InputTrigger.SupportIDs)
	appendFirst(intent.ExpectedPath.CoreWork.SupportIDs)
	appendFirst(intent.ExpectedPath.ObservableEffect.SupportIDs)
	appendAll(intent.CentralAnchorIDs)
	appendAll(intent.ExpectedPath.InputTrigger.SupportIDs)
	appendAll(intent.ExpectedPath.CoreWork.SupportIDs)
	appendAll(intent.ExpectedPath.ObservableEffect.SupportIDs)
	return result
}

func freshCandidateIsEligibleFirstContact(work freshCandidateWork) bool {
	intent := work.Candidate.ProductIntent
	return intent != nil &&
		intent.OpportunityKind == semanticdiscovery.OpportunityKindCentralBehavior &&
		intent.TargetUserJob == semanticdiscovery.OpportunityUserJobFirstContact &&
		work.Plan.Primary != nil &&
		work.Plan.Primary.Status == freshPrimaryPlanReady
}

func freshApplyPrimaryProductEligibility(work *freshCandidateWork) {
	if work == nil || work.Plan.Primary == nil || work.Plan.Primary.Status != freshPrimaryPlanReady {
		return
	}
	intent := work.Candidate.ProductIntent
	if intent == nil ||
		intent.OpportunityKind != semanticdiscovery.OpportunityKindCentralBehavior ||
		intent.TargetUserJob != semanticdiscovery.OpportunityUserJobFirstContact {
		return
	}
	reasons := freshPrimaryProductThresholdReasons(work.Plan.Centrality)
	if len(reasons) == 0 {
		return
	}
	primary := work.Plan.Primary
	primary.Status = freshPrimaryPlanInsufficient
	primary.Eligibility.Status = freshPrimaryPlanInsufficient
	primary.StatusReasons = appendUniqueFreshReasons(primary.StatusReasons, reasons...)
	primary.Eligibility.Reasons = appendUniqueFreshReasons(primary.Eligibility.Reasons, reasons...)
}

func freshPrimaryProductThresholdReasons(
	centrality freshCandidateCentrality,
) []string {
	reasons := []string{}
	if centrality.PurposeAlignment < freshPrimaryMinPurposeAlignment {
		reasons = append(reasons, "purpose_alignment_below_primary_threshold")
	}
	if centrality.ExplanatoryValue < freshPrimaryMinExplanation {
		reasons = append(reasons, "explanatory_value_below_primary_threshold")
	}
	if centrality.NavigationValue < freshPrimaryMinNavigation {
		reasons = append(reasons, "navigation_value_below_primary_threshold")
	}
	if centrality.EvidenceReadiness < freshPrimaryMinEvidence {
		reasons = append(reasons, "evidence_readiness_below_primary_threshold")
	}
	return reasons
}

func appendUniqueFreshReasons(existing []string, additions ...string) []string {
	known := make(map[string]struct{}, len(existing)+len(additions))
	result := make([]string, 0, len(existing)+len(additions))
	for _, reason := range append(append([]string(nil), existing...), additions...) {
		if _, duplicate := known[reason]; duplicate {
			continue
		}
		known[reason] = struct{}{}
		result = append(result, reason)
	}
	return result
}

func freshMarkPrimaryIntentCollisions(works []freshCandidateWork) {
	byIntent := make(map[string][]int)
	for index := range works {
		if works[index].Plan.Primary == nil || works[index].Plan.Primary.IntentKey == "" {
			continue
		}
		byIntent[works[index].Plan.Primary.IntentKey] = append(byIntent[works[index].Plan.Primary.IntentKey], index)
	}
	for _, indexes := range byIntent {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			primary := works[index].Plan.Primary
			primary.Eligibility = deriveFreshPrimaryEligibility(primary, works[index].Primary.ProbeFacts, true)
			primary.Status = primary.Eligibility.Status
			primary.StatusReasons = append([]string(nil), primary.Eligibility.Reasons...)
		}
	}
	for index := range works {
		freshApplyPrimaryProductEligibility(&works[index])
	}
}

type freshPrimaryPlannerState struct {
	ctx             context.Context
	reader          *reporead.Reader
	data            *report.ReportData
	candidate       semanticdiscovery.OpportunityCandidate
	sources         []freshSourceFunction
	files           map[string]*freshPrimaryParsedFile
	functions       map[string]freshPrimaryFunction
	rootPaths       map[string]struct{}
	additionalFiles []string
	retainedBytes   int
	frontiers       []freshPrimaryFrontier
	limitReasons    map[string]struct{}
}

func (state *freshPrimaryPlannerState) noteLimit(reason string) {
	if state == nil || reason == "" {
		return
	}
	if state.limitReasons == nil {
		state.limitReasons = make(map[string]struct{})
	}
	state.limitReasons[reason] = struct{}{}
}

func (state *freshPrimaryPlannerState) effectResolution(
	boundary *freshPrimaryEffectBoundary,
) (string, string) {
	if boundary != nil {
		return "resolved_typed_boundary", ""
	}
	if len(state.limitReasons) > 0 {
		return "unresolved", "bounded_static_analysis_limit"
	}
	hasResolvedLocal := false
	hasMissingType := false
	hasMissingTarget := false
	for _, frontier := range state.frontiers {
		if frontier.Resolution == "resolved_local" {
			hasResolvedLocal = true
		}
		switch frontier.ResolutionReason {
		case "receiver_type_unresolved":
			hasMissingType = true
		case "repository_package_unresolved", "exact_local_target_not_found":
			hasMissingTarget = true
		case "additional_file_limit", "additional_function_limit", "timeout":
			return "unresolved", "bounded_static_analysis_limit"
		}
	}
	if hasMissingType {
		return "unresolved", "unresolved_dynamic_dispatch"
	}
	if hasMissingTarget {
		return "unresolved", "insufficient_cross_package_connectivity"
	}
	if hasResolvedLocal {
		selected := 0
		for _, frontier := range state.frontiers {
			if frontier.Selected {
				selected++
			}
		}
		if selected >= freshPrimaryMaxPathFrontiers {
			return "unresolved", "bounded_static_analysis_limit"
		}
		return "unresolved", "missing_effect_classification"
	}
	return "unresolved", "missing_effect_classification"
}

func appendUniqueFreshString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (state *freshPrimaryPlannerState) resolveRoots() (
	[]freshPrimaryAnchor,
	[]freshSourceFunction,
	error,
) {
	byFact := make(map[string]freshSourceFunction, len(state.sources))
	for _, source := range state.sources {
		byFact[source.Fact.ID] = source
	}
	exact := make([]freshSourceFunction, 0, freshRepoDemoMaxSeedFuncs)
	seenFunctions := make(map[string]struct{})
	rootPaths := make(map[string]struct{})
	for _, id := range freshCandidatePlanningAnchorIDs(state.candidate) {
		source, exists := byFact[id]
		if !exists || source.Function.Path == "" || source.Function.Symbol == "" {
			continue
		}
		// Saved research can contain bounded functions from tracked examples or
		// other files that are intentionally outside the repository package
		// graph. Such a fact remains usable editorial context, but it cannot be
		// a planner root. One unresolvable anchor must not discard independent
		// exact roots that are inside the locally authorized graph.
		if !freshPrimaryPathAllowed(state.data, source.Function.Path) {
			continue
		}
		key := source.Function.Path + "\x00" + source.Function.Symbol
		if _, duplicate := seenFunctions[key]; duplicate {
			continue
		}
		if _, seenPath := rootPaths[source.Function.Path]; !seenPath && len(rootPaths) >= freshPrimaryMaxRootFiles {
			continue
		}
		seenFunctions[key] = struct{}{}
		rootPaths[source.Function.Path] = struct{}{}
		exact = append(exact, source)
		if len(exact) == freshRepoDemoMaxSeedFuncs {
			break
		}
	}
	if len(exact) == 0 {
		return nil, nil, nil
	}
	state.rootPaths = rootPaths
	preferredByPath := make(map[string][]string, len(rootPaths))
	for _, source := range exact {
		preferredByPath[source.Function.Path] = append(
			preferredByPath[source.Function.Path],
			source.Function.Symbol,
		)
	}
	paths := make([]string, 0, len(preferredByPath))
	for path := range preferredByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if _, err := state.readFile(path, false, preferredByPath[path]...); err != nil {
			return nil, exact, err
		}
	}

	// A source-supported helper on a receiver may omit the public entry method
	// from its compressed fact. Admit at most one same-receiver entry companion
	// from the already authorized root file; this is still an exact local anchor.
	entryNames := map[string]struct{}{
		"Run": {}, "ServeHTTP": {}, "Handle": {}, "Start": {}, "Open": {}, "Execute": {},
	}
	known := make(map[string]freshSourceFunction)
	hasExactEntry := false
	for _, source := range exact {
		known[source.Function.Path+"\x00"+source.Function.Symbol] = source
		hasExactEntry = hasExactEntry || freshPrimaryFunctionIsEntry(source.Function.Symbol)
	}
	if !hasExactEntry {
		for _, source := range append([]freshSourceFunction(nil), exact...) {
			receiver := freshPrimaryReceiver(source.Function.Symbol)
			if receiver == "" {
				continue
			}
			file := state.files[source.Function.Path]
			if file == nil || file.File == nil {
				return nil, exact, fmt.Errorf(
					"fresh primary planner: loaded root file %q is unavailable",
					source.Function.Path,
				)
			}
			companionDeclarations := []*ast.FuncDecl{}
			for _, declaration := range file.File.Decls {
				functionDecl, ok := declaration.(*ast.FuncDecl)
				if !ok || functionDecl.Body == nil {
					continue
				}
				symbol := freshASTSymbol(functionDecl)
				if freshPrimaryReceiver(symbol) != receiver {
					continue
				}
				if _, entry := entryNames[freshPrimaryTerminal(symbol)]; !entry {
					continue
				}
				companionDeclarations = append(companionDeclarations, functionDecl)
			}
			sort.Slice(companionDeclarations, func(i, j int) bool {
				return companionDeclarations[i].Pos() < companionDeclarations[j].Pos()
			})
			if len(companionDeclarations) == 0 {
				continue
			}
			companionFunction, loadErr := state.indexFunction(file, companionDeclarations[0])
			if loadErr != nil {
				continue
			}
			companion := companionFunction.Function
			key := companion.Path + "\x00" + companion.Symbol
			if _, exists := known[key]; exists {
				continue
			}
			fact, err := freshWindowFunctionFact(companion)
			if err != nil {
				return nil, exact, err
			}
			item := freshSourceFunction{Function: companion, Fact: fact}
			exact = append(exact, item)
			known[key] = item
			break
		}
	}

	sort.Slice(exact, func(i, j int) bool {
		leftEntry := freshPrimaryFunctionIsEntry(exact[i].Function.Symbol)
		rightEntry := freshPrimaryFunctionIsEntry(exact[j].Function.Symbol)
		if leftEntry != rightEntry {
			return leftEntry
		}
		if exact[i].Function.Path != exact[j].Function.Path {
			return exact[i].Function.Path < exact[j].Function.Path
		}
		return exact[i].Function.StartLine < exact[j].Function.StartLine
	})
	if len(exact) > freshRepoDemoMaxSeedFuncs {
		exact = exact[:freshRepoDemoMaxSeedFuncs]
	}

	anchors := make([]freshPrimaryAnchor, 0, len(exact))
	for _, source := range exact {
		if len(source.Fact.Evidence) == 0 {
			continue
		}
		anchor := freshPrimaryAnchor{
			ID:               goldenStableID("fpa", source.Function.Path, source.Function.Symbol, source.Function.ContentSHA256),
			OriginFactID:     source.Fact.ID,
			OriginEvidenceID: source.Fact.Evidence[0].ID,
			Path:             source.Function.Path,
			Symbol:           source.Function.Symbol,
			ContentSHA256:    source.Function.ContentSHA256,
			Depth:            0,
		}
		anchors = append(anchors, anchor)
	}
	return anchors, exact, nil
}

func (state *freshPrimaryPlannerState) readFile(
	path string,
	additional bool,
	preferredSymbols ...string,
) (*freshPrimaryParsedFile, error) {
	if file := state.files[path]; file != nil {
		if err := state.indexPreferredFunctions(file, preferredSymbols); err != nil {
			return nil, err
		}
		return file, nil
	}
	if err := state.ctx.Err(); err != nil {
		return nil, err
	}
	if len(state.files) >= freshPrimaryMaxFiles {
		state.noteLimit("file_limit")
		return nil, fmt.Errorf("fresh primary planner: file limit reached")
	}
	if additional && len(state.additionalFiles) >= freshPrimaryMaxAdditionalFiles {
		state.noteLimit("additional_file_limit")
		return nil, fmt.Errorf("fresh primary planner: additional file limit reached")
	}
	if !freshPrimaryPathAllowed(state.data, path) {
		return nil, fmt.Errorf("fresh primary planner: path %q is not in the saved repository graph", path)
	}
	content, err := state.reader.ReadFile(path, freshPrimaryMaxParsedFileBytes)
	if err != nil {
		return nil, err
	}
	if content.Truncated {
		return nil, fmt.Errorf("fresh primary planner: %s exceeds bounded read budget", path)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, content.Bytes, 0)
	if err != nil {
		return nil, fmt.Errorf("fresh primary planner: parse %s: %w", path, err)
	}
	file := &freshPrimaryParsedFile{
		Path: path, Data: content.Bytes, File: parsed, FSet: fset,
		Functions: make(map[string]freshPrimaryFunction),
		Fields:    freshPrimaryStructFields(parsed),
		Imports:   freshPrimaryImports(parsed),
	}
	state.files[path] = file
	if additional {
		state.additionalFiles = append(state.additionalFiles, path)
	}
	if err := state.indexPreferredFunctions(file, preferredSymbols); err != nil {
		return nil, err
	}
	return file, nil
}

func (state *freshPrimaryPlannerState) indexPreferredFunctions(
	file *freshPrimaryParsedFile,
	symbols []string,
) error {
	wanted := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		if symbol != "" {
			wanted[symbol] = struct{}{}
		}
	}
	for _, declaration := range file.File.Decls {
		functionDecl, ok := declaration.(*ast.FuncDecl)
		if !ok || functionDecl.Body == nil {
			continue
		}
		if _, preferred := wanted[freshASTSymbol(functionDecl)]; !preferred {
			continue
		}
		if _, err := state.indexFunction(file, functionDecl); err != nil {
			return err
		}
	}
	return nil
}

func (state *freshPrimaryPlannerState) indexFunction(
	file *freshPrimaryParsedFile,
	declaration *ast.FuncDecl,
) (freshPrimaryFunction, error) {
	symbol := freshASTSymbol(declaration)
	if function, exists := file.Functions[symbol]; exists {
		return function, nil
	}
	if len(state.functions) >= freshPrimaryMaxFunctions {
		state.noteLimit("additional_function_limit")
		return freshPrimaryFunction{}, fmt.Errorf("fresh primary planner: function limit reached")
	}
	wrapped := freshParsedAnchorFile{
		Path: file.Path,
		Data: file.Data,
		File: file.File,
		FSet: file.FSet,
	}
	function, err := freshFunctionFromDeclaration(wrapped, declaration)
	if err != nil {
		return freshPrimaryFunction{}, err
	}
	item := freshPrimaryFunction{
		Function:    function,
		Declaration: declaration,
		PackageName: file.File.Name.Name,
		Receiver:    freshPrimaryReceiver(function.Symbol),
	}
	item.Calls = freshPrimaryCalls(file, item)
	file.Functions[function.Symbol] = item
	state.functions[file.Path+"\x00"+function.Symbol] = item
	return item, nil
}

func (state *freshPrimaryPlannerState) retainedSourceBytes(
	roots []freshPrimaryAnchor,
	selected []freshPrimaryFrontier,
) int {
	keys := make(map[string]struct{}, len(roots)+len(selected))
	for _, root := range roots {
		keys[root.Path+"\x00"+root.Symbol] = struct{}{}
	}
	for _, frontier := range selected {
		keys[frontier.TargetPath+"\x00"+frontier.TargetSymbol] = struct{}{}
	}
	total := 0
	for key := range keys {
		if function, exists := state.functions[key]; exists {
			total += len(strings.Join(function.Function.Lines, "\n"))
		}
	}
	return total
}

func freshPrimaryPathAllowed(data *report.ReportData, path string) bool {
	if data == nil || data.RepositoryGraph == nil || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
		return false
	}
	for _, pkg := range data.RepositoryGraph.Packages {
		if pkg.Locality != "" && pkg.Locality != "local" {
			continue
		}
		for _, file := range pkg.Files {
			if file == path {
				return true
			}
		}
	}
	return false
}

func freshPrimaryStructFields(file *ast.File) map[string]map[string]string {
	result := make(map[string]map[string]string)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			fields := make(map[string]string)
			for _, field := range structure.Fields.List {
				typeName := freshPrimaryTypeName(field.Type)
				for _, name := range field.Names {
					fields[name.Name] = typeName
				}
			}
			result[typeSpec.Name.Name] = fields
		}
	}
	return result
}

func freshPrimaryImports(file *ast.File) map[string]string {
	result := make(map[string]string)
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil || path == "" {
			continue
		}
		name := filepath.Base(path)
		if item.Name != nil {
			name = item.Name.Name
		}
		if name == "" || name == "_" || name == "." {
			continue
		}
		result[name] = path
	}
	return result
}

func freshPrimaryCalls(file *freshPrimaryParsedFile, function freshPrimaryFunction) []freshPrimaryCall {
	env := freshPrimaryLocalTypes(file, function.Declaration)

	var calls []freshPrimaryCall
	ast.Inspect(function.Declaration.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		target := freshPrimaryExpressionName(call.Fun)
		if target == "" {
			return true
		}
		start := file.FSet.Position(call.Pos())
		end := file.FSet.Position(call.End())
		receiverType := ""
		receiverChain := ""
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			receiverType = freshPrimaryInferredType(selector.X, env, file.Fields)
			receiverChain = freshPrimaryExpressionName(selector.X)
		}
		qualifier := strings.Split(target, ".")[0]
		receiverQualifier, _ := freshPrimaryQualifiedType(receiverType)
		calls = append(calls, freshPrimaryCall{
			ID:   goldenStableID("fpc", file.Path, function.Function.Symbol, fmt.Sprintf("%d:%d", start.Line, start.Column), target),
			Path: file.Path, Function: function.Function.Symbol, Target: target,
			Terminal: freshPrimaryTerminal(target), ReceiverType: receiverType,
			ReceiverChain:      receiverChain,
			ImportPath:         file.Imports[qualifier],
			ReceiverImportPath: file.Imports[receiverQualifier],
			Line:               start.Line, Column: start.Column, EndLine: end.Line, EndColumn: end.Column,
		})
		return true
	})
	sort.Slice(calls, func(i, j int) bool {
		if calls[i].Line != calls[j].Line {
			return calls[i].Line < calls[j].Line
		}
		if calls[i].Column != calls[j].Column {
			return calls[i].Column < calls[j].Column
		}
		return calls[i].Target < calls[j].Target
	})
	return calls
}

func freshPrimaryLocalTypes(
	file *freshPrimaryParsedFile,
	declaration *ast.FuncDecl,
) map[string]string {
	env := make(map[string]string)
	if declaration.Recv != nil {
		for _, field := range declaration.Recv.List {
			typeName := freshPrimaryTypeName(field.Type)
			for _, name := range field.Names {
				env[name.Name] = typeName
			}
		}
	}
	if declaration.Type.Params != nil {
		for _, field := range declaration.Type.Params.List {
			typeName := freshPrimaryTypeName(field.Type)
			for _, name := range field.Names {
				env[name.Name] = typeName
			}
		}
	}
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.DeclStmt:
			general, ok := value.Decl.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				return true
			}
			for _, spec := range general.Specs {
				decl, ok := spec.(*ast.ValueSpec)
				if !ok || decl.Type == nil {
					continue
				}
				typeName := freshPrimaryTypeName(decl.Type)
				for _, name := range decl.Names {
					env[name.Name] = typeName
				}
			}
		case *ast.AssignStmt:
			for index, lhs := range value.Lhs {
				identifier, ok := lhs.(*ast.Ident)
				if !ok || index >= len(value.Rhs) {
					continue
				}
				if typeName := freshPrimaryInferredTypeInFile(
					value.Rhs[index],
					env,
					file,
				); typeName != "" {
					env[identifier.Name] = typeName
				}
			}
		}
		return true
	})

	return env
}

func freshPrimaryInferredTypeInFile(
	expression ast.Expr,
	env map[string]string,
	file *freshPrimaryParsedFile,
) string {
	if typeName := freshPrimaryInferredType(expression, env, file.Fields); typeName != "" {
		return typeName
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return ""
	}
	target := freshPrimaryExpressionName(call.Fun)
	parts := strings.Split(target, ".")
	if len(parts) != 2 || file.Imports[parts[0]] != "os" {
		return ""
	}
	switch parts[1] {
	case "Create", "CreateTemp", "NewFile", "Open", "OpenFile":
		return parts[0] + ".File"
	default:
		return ""
	}
}

func freshPrimaryTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return freshPrimaryTypeName(value.X)
	case *ast.SelectorExpr:
		prefix := freshPrimaryExpressionName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	case *ast.IndexExpr:
		return freshPrimaryTypeName(value.X)
	case *ast.IndexListExpr:
		return freshPrimaryTypeName(value.X)
	case *ast.ArrayType:
		return freshPrimaryTypeName(value.Elt)
	case *ast.MapType:
		return freshPrimaryTypeName(value.Value)
	case *ast.ChanType:
		return freshPrimaryTypeName(value.Value)
	default:
		return ""
	}
}

func freshPrimaryInferredType(
	expression ast.Expr,
	env map[string]string,
	fields map[string]map[string]string,
) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return env[value.Name]
	case *ast.ParenExpr:
		return freshPrimaryInferredType(value.X, env, fields)
	case *ast.UnaryExpr:
		return freshPrimaryInferredType(value.X, env, fields)
	case *ast.CompositeLit:
		return freshPrimaryTypeName(value.Type)
	case *ast.SelectorExpr:
		base := freshPrimaryInferredType(value.X, env, fields)
		baseTerminal := freshPrimaryTerminal(base)
		if structFields := fields[baseTerminal]; structFields != nil {
			return structFields[value.Sel.Name]
		}
	case *ast.CallExpr:
		if identifier, ok := value.Fun.(*ast.Ident); ok && unicode.IsUpper([]rune(identifier.Name)[0]) {
			return identifier.Name
		}
	}
	return ""
}

func freshPrimaryExpressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := freshPrimaryExpressionName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	case *ast.ParenExpr:
		return freshPrimaryExpressionName(value.X)
	default:
		return ""
	}
}

func freshPrimaryReceiver(symbol string) string {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func freshPrimaryTerminal(value string) string {
	parts := strings.Split(value, ".")
	return parts[len(parts)-1]
}

func freshPrimaryFunctionIsEntry(symbol string) bool {
	switch freshPrimaryTerminal(symbol) {
	case "Run", "ServeHTTP", "Handle", "Start", "Open", "Execute":
		return true
	default:
		return false
	}
}

// inferCallReceiverType resolves only lexical range variables whose element
// type can be recovered from one exact repository-local function result. It
// intentionally does not guess interface implementations or propagate values
// through arbitrary assignments.
func (state *freshPrimaryPlannerState) inferCallReceiverType(
	from freshPrimaryAnchor,
	call freshPrimaryCall,
) freshPrimaryCall {
	if call.ReceiverType != "" || call.ReceiverChain == "" {
		return call
	}
	function, exists := state.functions[from.Path+"\x00"+from.Symbol]
	file := state.files[from.Path]
	if !exists || file == nil || function.Declaration == nil {
		return call
	}
	base := strings.Split(call.ReceiverChain, ".")[0]
	if base == "" {
		return call
	}
	type rangeBinding struct {
		statement *ast.RangeStmt
		value     bool
		size      int
	}
	var binding *rangeBinding
	ast.Inspect(function.Declaration.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.RangeStmt)
		if !ok || statement.Body == nil {
			return true
		}
		start := file.FSet.Position(statement.Body.Lbrace).Line
		end := file.FSet.Position(statement.Body.Rbrace).Line
		if call.Line < start || call.Line > end {
			return true
		}
		matches := func(expression ast.Expr) bool {
			identifier, ok := expression.(*ast.Ident)
			return ok && identifier.Name == base
		}
		isValue := matches(statement.Value)
		if !isValue && !matches(statement.Key) {
			return true
		}
		candidate := &rangeBinding{statement: statement, value: isValue, size: end - start}
		if binding == nil || candidate.size < binding.size {
			binding = candidate
		}
		return true
	})
	if binding == nil {
		return call
	}
	resolved := state.resolveRangeBindingType(
		file,
		function,
		binding.statement.X,
		binding.value,
	)
	if resolved == "" {
		return call
	}
	parts := strings.Split(call.ReceiverChain, ".")
	for _, field := range parts[1:] {
		resolved = state.resolveFieldType(file.Path, resolved, field)
		if resolved == "" {
			return call
		}
	}
	call.ReceiverType = resolved
	return call
}

func (state *freshPrimaryPlannerState) resolveRangeBindingType(
	file *freshPrimaryParsedFile,
	function freshPrimaryFunction,
	expression ast.Expr,
	valueBinding bool,
) string {
	callExpression, ok := expression.(*ast.CallExpr)
	if !ok {
		return ""
	}
	targetName := freshPrimaryExpressionName(callExpression.Fun)
	if targetName == "" {
		return ""
	}
	env := freshPrimaryLocalTypes(file, function.Declaration)
	supportCall := freshPrimaryCall{
		Path:     file.Path,
		Function: function.Function.Symbol,
		Target:   targetName,
		Terminal: freshPrimaryTerminal(targetName),
	}
	if selector, ok := callExpression.Fun.(*ast.SelectorExpr); ok {
		supportCall.ReceiverType = freshPrimaryInferredType(selector.X, env, file.Fields)
		supportCall.ReceiverChain = freshPrimaryExpressionName(selector.X)
	}
	target, resolved := state.resolveLoadedCall(supportCall)
	if !resolved {
		var reason string
		target, reason, resolved = state.resolveAdditionalCall(supportCall)
		_ = reason
	}
	if !resolved || target.Declaration == nil || target.Declaration.Type.Results == nil ||
		len(target.Declaration.Type.Results.List) == 0 {
		return ""
	}
	resultType := target.Declaration.Type.Results.List[0].Type
	elementType := freshPrimaryRangeElementType(resultType, valueBinding)
	if elementType == "" {
		return ""
	}
	if strings.Contains(elementType, ".") {
		return elementType
	}
	qualifier, _ := freshPrimaryQualifiedType(supportCall.ReceiverType)
	if qualifier != "" {
		return qualifier + "." + elementType
	}
	return state.qualifyTypeForCaller(elementType, target.Function.Path, file.Path)
}

func freshPrimaryRangeElementType(expression ast.Expr, valueBinding bool) string {
	switch value := expression.(type) {
	case *ast.ArrayType:
		if !valueBinding {
			return "int"
		}
		return freshPrimaryTypeName(value.Elt)
	case *ast.MapType:
		if valueBinding {
			return freshPrimaryTypeName(value.Value)
		}
		return freshPrimaryTypeName(value.Key)
	case *ast.ChanType:
		return freshPrimaryTypeName(value.Value)
	default:
		return ""
	}
}

func (state *freshPrimaryPlannerState) qualifyTypeForCaller(
	typeName string,
	targetPath string,
	callerPath string,
) string {
	targetPackage := state.packagePathForFile(targetPath)
	callerPackage := state.packagePathForFile(callerPath)
	if targetPackage == "" || targetPackage == callerPackage {
		return typeName
	}
	caller := state.files[callerPath]
	if caller == nil {
		return ""
	}
	for qualifier, path := range caller.Imports {
		if path == targetPackage {
			return qualifier + "." + typeName
		}
	}
	return ""
}

func (state *freshPrimaryPlannerState) resolveFieldType(
	callerPath string,
	receiverType string,
	fieldName string,
) string {
	qualifier, receiver := freshPrimaryQualifiedType(receiverType)
	if receiver == "" || fieldName == "" {
		return ""
	}
	paths, exactPackage := state.rankAdditionalPaths(callerPath, qualifier, receiver, "")
	if !exactPackage {
		return ""
	}
	for _, path := range paths {
		if state.ctx.Err() != nil {
			return ""
		}
		file := state.files[path]
		if file == nil {
			var err error
			file, err = state.readFile(path, true)
			if err != nil {
				continue
			}
		}
		fields := file.Fields[receiver]
		fieldType := fields[fieldName]
		if fieldType == "" {
			continue
		}
		if strings.Contains(fieldType, ".") {
			return fieldType
		}
		if qualifier != "" {
			return qualifier + "." + fieldType
		}
		return state.qualifyTypeForCaller(fieldType, path, callerPath)
	}
	return ""
}

func (state *freshPrimaryPlannerState) packagePathForFile(path string) string {
	if state.data == nil || state.data.RepositoryGraph == nil {
		return ""
	}
	for _, pkg := range state.data.RepositoryGraph.Packages {
		if pkg.Locality != "" && pkg.Locality != "local" {
			continue
		}
		for _, candidate := range pkg.Files {
			if candidate == path {
				return pkg.CanonicalPath
			}
		}
	}
	return ""
}

func (state *freshPrimaryPlannerState) expand(
	roots []freshPrimaryAnchor,
) ([]freshPrimaryFrontier, *freshPrimaryEffectBoundary) {
	rootIDs := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		rootIDs[root.Path+"\x00"+root.Symbol] = struct{}{}
	}
	for _, root := range roots {
		if function, exists := state.functions[root.Path+"\x00"+root.Symbol]; exists {
			if boundary := state.bestBoundary(function); boundary != nil {
				state.trimFrontiers()
				return nil, boundary
			}
		}
	}

	type rootCall struct {
		anchor freshPrimaryAnchor
		call   freshPrimaryCall
	}
	rootCalls := make([]rootCall, 0)
	for _, root := range roots {
		function, exists := state.functions[root.Path+"\x00"+root.Symbol]
		if !exists {
			continue
		}
		for _, call := range freshPrimaryRankedCalls(state.candidate, function.Calls) {
			rootCalls = append(rootCalls, rootCall{anchor: root, call: call})
		}
	}
	sort.SliceStable(rootCalls, func(i, j int) bool {
		left := freshPrimaryCallPotentialScore(state.candidate, rootCalls[i].call)
		right := freshPrimaryCallPotentialScore(state.candidate, rootCalls[j].call)
		if left != right {
			return left > right
		}
		if rootCalls[i].call.Path != rootCalls[j].call.Path {
			return rootCalls[i].call.Path < rootCalls[j].call.Path
		}
		if rootCalls[i].call.Line != rootCalls[j].call.Line {
			return rootCalls[i].call.Line < rootCalls[j].call.Line
		}
		return rootCalls[i].call.ID < rootCalls[j].call.ID
	})

	var fallback []freshPrimaryFrontier
	for _, item := range rootCalls {
		if state.ctx.Err() != nil {
			break
		}
		frontier, boundary := state.resolveFrontier(item.anchor, item.call, 1)
		state.frontiers = append(state.frontiers, frontier)
		if boundary != nil {
			state.trimFrontiers()
			return nil, boundary
		}
		if frontier.Resolution != "resolved_local" || frontier.DemotionReason != "" {
			continue
		}
		key := frontier.TargetPath + "\x00" + frontier.TargetSymbol
		if _, root := rootIDs[key]; root {
			continue
		}
		if state.retainedSourceBytes(roots, []freshPrimaryFrontier{frontier}) > freshPrimaryMaxRetainedBytes {
			state.noteLimit("retained_source_byte_limit")
			continue
		}
		if fallback == nil {
			fallback = []freshPrimaryFrontier{frontier}
		}
		visited := make(map[string]struct{}, len(rootIDs)+1)
		for rootKey := range rootIDs {
			visited[rootKey] = struct{}{}
		}
		visited[key] = struct{}{}
		path, effect := state.followEffectBridge(
			roots,
			[]freshPrimaryFrontier{frontier},
			freshPrimaryMaxFrontiers,
			visited,
		)
		if effect != nil {
			state.markSelectedFrontiers(path)
			state.trimFrontiers()
			return path, effect
		}
	}
	if len(fallback) > 0 {
		state.markSelectedFrontiers(fallback)
	}
	state.trimFrontiers()
	return fallback, nil
}

func (state *freshPrimaryPlannerState) followEffectBridge(
	roots []freshPrimaryAnchor,
	path []freshPrimaryFrontier,
	remaining int,
	visited map[string]struct{},
) ([]freshPrimaryFrontier, *freshPrimaryEffectBoundary) {
	current := path[len(path)-1]
	function, exists := state.functions[current.TargetPath+"\x00"+current.TargetSymbol]
	if !exists {
		return nil, nil
	}
	if boundary := state.bestBoundary(function); boundary != nil {
		return path, boundary
	}
	if remaining == 0 || state.ctx.Err() != nil {
		return nil, nil
	}
	anchor := freshPrimaryAnchor{
		ID:               goldenStableID("fpa", current.TargetPath, current.TargetSymbol, current.TargetContentSHA),
		OriginFactID:     roots[0].OriginFactID,
		OriginEvidenceID: current.CallEvidenceID,
		Path:             current.TargetPath,
		Symbol:           current.TargetSymbol,
		ContentSHA256:    current.TargetContentSHA,
		Depth:            current.Depth,
	}
	for _, call := range freshPrimaryRankedCalls(state.candidate, function.Calls) {
		frontier, boundary := state.resolveFrontier(anchor, call, current.Depth+1)
		state.frontiers = append(state.frontiers, frontier)
		if boundary != nil {
			return path, boundary
		}
		if frontier.Resolution != "resolved_local" || frontier.DemotionReason != "" {
			continue
		}
		key := frontier.TargetPath + "\x00" + frontier.TargetSymbol
		if _, duplicate := visited[key]; duplicate {
			continue
		}
		nextPath := append(append([]freshPrimaryFrontier(nil), path...), frontier)
		if len(nextPath) > freshPrimaryMaxPathFrontiers ||
			state.retainedSourceBytes(roots, nextPath) > freshPrimaryMaxRetainedBytes {
			state.noteLimit("retained_source_byte_limit")
			continue
		}
		visited[key] = struct{}{}
		resolvedPath, effect := state.followEffectBridge(roots, nextPath, remaining-1, visited)
		delete(visited, key)
		if effect != nil {
			return resolvedPath, effect
		}
	}
	return nil, nil
}

func freshPrimaryRankedCalls(
	candidate semanticdiscovery.OpportunityCandidate,
	calls []freshPrimaryCall,
) []freshPrimaryCall {
	result := append([]freshPrimaryCall(nil), calls...)
	sort.SliceStable(result, func(i, j int) bool {
		left := freshPrimaryCallPotentialScore(candidate, result[i])
		right := freshPrimaryCallPotentialScore(candidate, result[j])
		if left != right {
			return left > right
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func freshPrimaryCallPotentialScore(
	candidate semanticdiscovery.OpportunityCandidate,
	call freshPrimaryCall,
) int {
	if freshPrimaryCallDemotion(call) != "" {
		return -100
	}
	score := 0
	if freshPrimaryEffectLikeName(call.Terminal) {
		score += 40
	}
	for term := range freshTerms(call.Terminal) {
		if _, match := freshPrimarySemanticTerms(candidate.QuestionAnswered)[term]; match {
			score += 20
		}
	}
	if strings.Count(call.ReceiverChain, ".") > 0 {
		score += 5
	}
	return score
}

func (state *freshPrimaryPlannerState) markSelectedFrontiers(selected []freshPrimaryFrontier) {
	known := make(map[string]struct{}, len(selected))
	for index := range selected {
		selected[index].Selected = true
		known[selected[index].ID] = struct{}{}
	}
	for index := range state.frontiers {
		if _, exists := known[state.frontiers[index].ID]; exists {
			state.frontiers[index].Selected = true
		}
	}
}

func (state *freshPrimaryPlannerState) trimFrontiers() {
	if len(state.frontiers) <= freshPrimaryMaxEnumeratedCalls {
		return
	}
	sort.SliceStable(state.frontiers, func(i, j int) bool {
		if state.frontiers[i].Selected != state.frontiers[j].Selected {
			return state.frontiers[i].Selected
		}
		if state.frontiers[i].Score != state.frontiers[j].Score {
			return state.frontiers[i].Score > state.frontiers[j].Score
		}
		return state.frontiers[i].ID < state.frontiers[j].ID
	})
	state.frontiers = state.frontiers[:freshPrimaryMaxEnumeratedCalls]
}

func (state *freshPrimaryPlannerState) resolveFrontier(
	from freshPrimaryAnchor,
	call freshPrimaryCall,
	depth int,
) (freshPrimaryFrontier, *freshPrimaryEffectBoundary) {
	call = state.inferCallReceiverType(from, call)
	frontier := freshPrimaryFrontier{
		ID:             goldenStableID("fpf", state.candidate.ID, from.ID, call.ID),
		FromAnchorID:   from.ID,
		CallEvidenceID: call.ID,
		CallPath:       call.Path,
		CallLine:       call.Line,
		CallColumn:     call.Column,
		ReceiverType:   call.ReceiverType,
		Operation:      call.Target,
		Depth:          depth,
		Resolution:     "unresolved",
	}
	if reason := freshPrimaryCallDemotion(call); reason != "" {
		frontier.DemotionReason = reason
	}
	if target, ok := state.resolveLoadedCall(call); ok {
		frontier.TargetPath = target.Function.Path
		frontier.TargetSymbol = target.Function.Symbol
		frontier.TargetContentSHA = target.Function.ContentSHA256
		frontier.Resolution = "resolved_local"
		frontier.Score = state.frontierScore(from, call, target)
		return frontier, nil
	}
	if target, reason, ok := state.resolveAdditionalCall(call); ok {
		frontier.TargetPath = target.Function.Path
		frontier.TargetSymbol = target.Function.Symbol
		frontier.TargetContentSHA = target.Function.ContentSHA256
		frontier.Resolution = "resolved_local"
		frontier.Score = state.frontierScore(from, call, target)
		return frontier, nil
	} else {
		frontier.ResolutionReason = reason
	}
	if kind := freshPrimaryBoundaryForCall(call); kind != "" {
		frontier.Resolution = "typed_boundary"
		frontier.Score = 30
		caller := functionForAnchor(state, from)
		return frontier, freshPrimaryBoundaryFromCall(caller, call, kind)
	}
	return frontier, nil
}

func (state *freshPrimaryPlannerState) resolveLoadedCall(
	call freshPrimaryCall,
) (freshPrimaryFunction, bool) {
	wantedReceiver := freshPrimaryTerminal(call.ReceiverType)
	wantedSymbol := call.Terminal
	if wantedReceiver != "" {
		wantedSymbol = wantedReceiver + "." + call.Terminal
	}
	var matches []freshPrimaryFunction
	for _, function := range state.functions {
		if function.Function.Symbol != wantedSymbol {
			continue
		}
		matches = append(matches, function)
	}
	if len(matches) != 1 && wantedReceiver == "" {
		matches = nil
		for _, function := range state.functions {
			if freshPrimaryTerminal(function.Function.Symbol) == call.Terminal {
				matches = append(matches, function)
			}
		}
	}
	if len(matches) == 0 {
		matches = state.indexLoadedCallTargets(wantedSymbol, wantedReceiver == "")
	}
	return freshPrimaryUniqueFunction(matches)
}

func (state *freshPrimaryPlannerState) indexLoadedCallTargets(
	wantedSymbol string,
	matchTerminal bool,
) []freshPrimaryFunction {
	type target struct {
		file        *freshPrimaryParsedFile
		declaration *ast.FuncDecl
	}
	paths := make([]string, 0, len(state.files))
	for path := range state.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	targets := []target{}
	for _, path := range paths {
		file := state.files[path]
		if file == nil || file.File == nil {
			continue
		}
		for _, declaration := range file.File.Decls {
			functionDecl, ok := declaration.(*ast.FuncDecl)
			if !ok || functionDecl.Body == nil {
				continue
			}
			symbol := freshASTSymbol(functionDecl)
			matched := symbol == wantedSymbol
			if matchTerminal {
				matched = freshPrimaryTerminal(symbol) == freshPrimaryTerminal(wantedSymbol)
			}
			if matched {
				targets = append(targets, target{file: file, declaration: functionDecl})
			}
		}
	}
	if len(targets) != 1 {
		return nil
	}
	function, err := state.indexFunction(targets[0].file, targets[0].declaration)
	if err != nil {
		return nil
	}
	return []freshPrimaryFunction{function}
}

func (state *freshPrimaryPlannerState) resolveAdditionalCall(
	call freshPrimaryCall,
) (freshPrimaryFunction, string, bool) {
	if call.ReceiverType == "" {
		return freshPrimaryFunction{}, "receiver_type_unresolved", false
	}
	if len(state.additionalFiles) >= freshPrimaryMaxAdditionalFiles {
		state.noteLimit("additional_file_limit")
		return freshPrimaryFunction{}, "additional_file_limit", false
	}
	qualifier, receiver := freshPrimaryQualifiedType(call.ReceiverType)
	if receiver == "" {
		return freshPrimaryFunction{}, "receiver_type_unresolved", false
	}
	paths, exactPackage := state.rankAdditionalPaths(call.Path, qualifier, receiver, call.Terminal)
	if !exactPackage {
		return freshPrimaryFunction{}, "repository_package_unresolved", false
	}
	for _, path := range paths {
		if state.ctx.Err() != nil || len(state.additionalFiles) >= freshPrimaryMaxAdditionalFiles {
			break
		}
		wantedSymbol := receiver + "." + call.Terminal
		file, err := state.readFile(path, true, wantedSymbol)
		if err != nil {
			continue
		}
		if function, exists := file.Functions[receiver+"."+call.Terminal]; exists {
			return function, "", true
		}
	}
	if state.ctx.Err() != nil {
		return freshPrimaryFunction{}, "timeout", false
	}
	if len(state.additionalFiles) >= freshPrimaryMaxAdditionalFiles {
		state.noteLimit("additional_file_limit")
		return freshPrimaryFunction{}, "additional_file_limit", false
	}
	return freshPrimaryFunction{}, "exact_local_target_not_found", false
}

func freshPrimaryUniqueFunction(matches []freshPrimaryFunction) (freshPrimaryFunction, bool) {
	if len(matches) != 1 {
		return freshPrimaryFunction{}, false
	}
	return matches[0], true
}

func freshPrimaryQualifiedType(value string) (string, string) {
	parts := strings.Split(value, ".")
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

func (state *freshPrimaryPlannerState) rankAdditionalPaths(
	callPath string,
	qualifier string,
	receiver string,
	method string,
) ([]string, bool) {
	if state.data == nil || state.data.RepositoryGraph == nil {
		return nil, false
	}
	packagePath := state.resolveCallPackagePath(callPath, qualifier)
	if packagePath == "" {
		return nil, false
	}
	type rankedPath struct {
		path  string
		score int
	}
	var ranked []rankedPath
	receiverFile := strings.ToLower(freshPrimarySnake(receiver)) + ".go"
	methodTerms := freshTerms(method)
	questionTerms := freshTerms(state.candidate.QuestionAnswered)
	savedPaths := make(map[string]struct{}, len(state.sources))
	for _, source := range state.sources {
		savedPaths[source.Function.Path] = struct{}{}
	}
	for _, pkg := range state.data.RepositoryGraph.Packages {
		if pkg.Locality != "" && pkg.Locality != "local" {
			continue
		}
		if pkg.CanonicalPath != packagePath {
			continue
		}
		for _, path := range pkg.Files {
			if !freshPrimaryPathAllowed(state.data, path) {
				continue
			}
			score := 0
			base := strings.ToLower(filepath.Base(path))
			if base == receiverFile {
				score += 100
			}
			for term := range freshTerms(base) {
				if _, match := methodTerms[term]; match {
					score += 20
				}
				if _, match := questionTerms[term]; match {
					score += 4
				}
			}
			if _, saved := savedPaths[path]; saved {
				score += 8
			}
			ranked = append(ranked, rankedPath{path: path, score: score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].path < ranked[j].path
	})
	paths := make([]string, 0, len(ranked))
	for _, item := range ranked {
		paths = append(paths, item.path)
	}
	return paths, true
}

func (state *freshPrimaryPlannerState) resolveCallPackagePath(
	callPath string,
	qualifier string,
) string {
	if state.data == nil || state.data.RepositoryGraph == nil {
		return ""
	}
	if qualifier != "" {
		file := state.files[callPath]
		if file == nil {
			return ""
		}
		importPath := file.Imports[qualifier]
		if importPath == "" {
			return ""
		}
		for _, pkg := range state.data.RepositoryGraph.Packages {
			if pkg.Locality != "" && pkg.Locality != "local" {
				continue
			}
			if pkg.CanonicalPath == importPath {
				return importPath
			}
		}
		return ""
	}
	for _, pkg := range state.data.RepositoryGraph.Packages {
		if pkg.Locality != "" && pkg.Locality != "local" {
			continue
		}
		for _, path := range pkg.Files {
			if path == callPath {
				return pkg.CanonicalPath
			}
		}
	}
	return ""
}

func freshPrimarySnake(value string) string {
	var result strings.Builder
	for index, r := range value {
		if unicode.IsUpper(r) && index > 0 {
			result.WriteByte('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

func (state *freshPrimaryPlannerState) frontierScore(
	from freshPrimaryAnchor,
	call freshPrimaryCall,
	target freshPrimaryFunction,
) int {
	score := freshPrimaryAnchorAspectScore(state.candidate, from.OriginFactID)
	question := freshPrimarySemanticTerms(state.candidate.QuestionAnswered)
	for term := range freshTerms(call.Terminal) {
		if _, match := question[term]; match {
			score += 20
		}
	}
	if call.Path != target.Function.Path {
		score += 10
	}
	if freshPrimaryEffectLikeName(call.Terminal) {
		score += 12
	}
	if freshPrimaryCallDemotion(call) != "" {
		score -= 100
	}
	return score
}

func freshPrimaryAnchorAspectScore(
	candidate semanticdiscovery.OpportunityCandidate,
	factID string,
) int {
	if candidate.ProductIntent == nil || factID == "" {
		return 0
	}
	contains := func(ids []string) bool {
		for _, id := range ids {
			if id == factID {
				return true
			}
		}
		return false
	}
	intent := candidate.ProductIntent
	switch {
	case contains(intent.ExpectedPath.ObservableEffect.SupportIDs):
		return 30
	case contains(intent.ExpectedPath.CoreWork.SupportIDs):
		return 20
	case contains(intent.ExpectedPath.InputTrigger.SupportIDs):
		return 10
	default:
		return 0
	}
}

func freshPrimarySemanticTerms(value string) map[string]struct{} {
	terms := freshTerms(value)
	for _, stop := range []string{
		"a", "an", "and", "as", "by", "command", "does", "for", "from", "how",
		"into", "is", "it", "of", "process", "repository", "the", "this", "to",
		"what", "when", "where", "which", "with",
	} {
		delete(terms, stop)
	}
	return terms
}

func freshPrimaryCallDemotion(call freshPrimaryCall) string {
	if freshAuxiliaryWindowCall(call.Target) {
		return "auxiliary_call"
	}
	name := strings.ToLower(call.Terminal)
	for _, prefix := range []string{"can", "has", "is", "should"} {
		if strings.HasPrefix(name, prefix) {
			return "helper_predicate"
		}
	}
	for _, part := range []string{
		"close", "debug", "error", "info", "lock", "logger", "pos", "setpos",
		"stop", "string", "unlock", "usage", "warn",
	} {
		if strings.Contains(name, part) {
			return "setup_or_diagnostic_call"
		}
	}
	return ""
}

func freshPrimaryEffectLikeName(name string) bool {
	name = strings.ToLower(name)
	for _, part := range []string{
		"write", "send", "publish", "upload", "store", "save", "persist", "restore",
		"commit", "apply", "sync", "snapshot", "compact", "dispatch", "respond",
	} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

func (state *freshPrimaryPlannerState) bestBoundary(
	function freshPrimaryFunction,
) *freshPrimaryEffectBoundary {
	type rankedBoundary struct {
		boundary freshPrimaryEffectBoundary
		score    int
	}
	var matches []rankedBoundary
	for _, call := range function.Calls {
		kind := freshPrimaryBoundaryForCall(call)
		if kind == "" && freshPrimaryBackendRoleType(call.ReceiverType) &&
			freshPrimaryStrongBoundaryVerb(strings.ToLower(call.Terminal)) &&
			state.receiverIsRepositoryInterface(call) {
			kind = freshBoundaryBackendInterface
		}
		if kind == "" {
			continue
		}
		score := freshPrimaryBoundaryScore(call, kind)
		boundary := freshPrimaryBoundaryFromCall(function, call, kind)
		if boundary == nil {
			continue
		}
		matches = append(matches, rankedBoundary{
			boundary: *boundary,
			score:    score,
		})
	}
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].boundary.Line != matches[j].boundary.Line {
			return matches[i].boundary.Line < matches[j].boundary.Line
		}
		return matches[i].boundary.ID < matches[j].boundary.ID
	})
	result := matches[0].boundary
	return &result
}

func freshPrimaryBackendRoleType(typeName string) bool {
	typeName = strings.ToLower(freshPrimaryTerminal(typeName))
	for _, role := range []string{
		"backend", "client", "destination", "publisher", "remote", "sink", "store",
	} {
		if strings.Contains(typeName, role) {
			return true
		}
	}
	return false
}

func (state *freshPrimaryPlannerState) receiverIsRepositoryInterface(
	call freshPrimaryCall,
) bool {
	qualifier, typeName := freshPrimaryQualifiedType(call.ReceiverType)
	if typeName == "" {
		return false
	}
	packagePath := state.resolveCallPackagePath(call.Path, qualifier)
	if packagePath == "" {
		return false
	}
	paths, exactPackage := state.rankAdditionalPaths(call.Path, qualifier, typeName, "")
	if !exactPackage {
		return false
	}
	for _, path := range paths {
		if state.ctx.Err() != nil {
			return false
		}
		file := state.files[path]
		if file == nil {
			var err error
			file, err = state.readFile(path, true)
			if err != nil {
				continue
			}
		}
		if state.packagePathForFile(path) != packagePath {
			continue
		}
		if freshPrimaryFileDeclaresInterface(file.File, typeName) {
			return true
		}
	}
	return false
}

func freshPrimaryFileDeclaresInterface(file *ast.File, typeName string) bool {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}
			_, isInterface := typeSpec.Type.(*ast.InterfaceType)
			return isInterface
		}
	}
	return false
}

func functionForAnchor(
	state *freshPrimaryPlannerState,
	anchor freshPrimaryAnchor,
) freshPrimaryFunction {
	if state == nil {
		return freshPrimaryFunction{}
	}
	return state.functions[anchor.Path+"\x00"+anchor.Symbol]
}

func freshPrimaryBoundaryFromCall(
	function freshPrimaryFunction,
	call freshPrimaryCall,
	kind freshPrimaryBoundaryKind,
) *freshPrimaryEffectBoundary {
	if function.Function.Path == "" || function.Function.Symbol == "" || call.ID == "" || kind == "" {
		return nil
	}
	claim := "The bounded source reaches an observable operation boundary."
	switch kind {
	case freshBoundaryFileWrite:
		claim = "The bounded source reaches a direct file-output boundary."
	case freshBoundaryDatabaseWrite:
		claim = "The bounded source reaches a database-mutation boundary."
	case freshBoundaryNetworkSend:
		claim = "The bounded source reaches a network-send boundary."
	case freshBoundaryBackendInterface:
		claim = "The bounded source hands data to a typed backend/interface boundary; the concrete implementation is not resolved."
	case freshBoundaryPublicOutput:
		claim = "The bounded source reaches a public output boundary."
	}
	return &freshPrimaryEffectBoundary{
		ID:   goldenStableID("fpb", function.Function.Path, function.Function.Symbol, call.ID, string(kind)),
		Kind: kind, FunctionPath: function.Function.Path,
		FunctionSymbol: function.Function.Symbol, Operation: call.Target,
		ReceiverType: call.ReceiverType, EvidenceID: call.ID,
		Line: call.Line, Column: call.Column, ClaimBoundary: claim,
	}
}

func freshPrimaryBoundaryForCall(call freshPrimaryCall) freshPrimaryBoundaryKind {
	terminal := strings.ToLower(call.Terminal)
	target := strings.ToLower(call.Target)
	receiverType := strings.ToLower(strings.TrimPrefix(call.ReceiverType, "*"))
	receiverChain := strings.ToLower(call.ReceiverChain)
	if call.ImportPath == "os" {
		switch terminal {
		case "create", "rename", "writefile", "mkdir", "mkdirall", "truncate":
			return freshBoundaryFileWrite
		}
	}
	if call.ReceiverImportPath == "os" && receiverType == "os.file" {
		switch terminal {
		case "write", "writeat", "writestring":
			return freshBoundaryFileWrite
		}
	}
	if call.ImportPath == "fmt" && strings.HasPrefix(target, "fmt.print") {
		return freshBoundaryPublicOutput
	}
	if call.ImportPath == "os" &&
		(receiverChain == "os.stdout" || receiverChain == "os.stderr") && terminal == "write" {
		return freshBoundaryPublicOutput
	}
	if call.ReceiverImportPath == "net/http" && receiverType == "http.responsewriter" {
		switch terminal {
		case "write", "writeheader":
			return freshBoundaryPublicOutput
		}
	}
	receiverTerminal := strings.ToLower(freshPrimaryTerminal(receiverType))
	if call.ReceiverImportPath == "database/sql" &&
		(receiverTerminal == "db" || receiverTerminal == "tx") {
		switch terminal {
		case "exec", "execcontext", "commit", "insert", "update", "delete", "put", "apply":
			return freshBoundaryDatabaseWrite
		}
	}
	if (call.ReceiverImportPath == "net/http" && receiverType == "http.client") ||
		(call.ReceiverImportPath == "net" && receiverType == "net.conn") {
		if terminal == "do" || freshPrimaryStrongBoundaryVerb(terminal) {
			return freshBoundaryNetworkSend
		}
	}
	return ""
}

func freshPrimaryStrongBoundaryVerb(name string) bool {
	for _, prefix := range []string{"write", "send", "publish", "upload", "put", "store", "save", "commit"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func freshPrimaryBoundaryScore(call freshPrimaryCall, kind freshPrimaryBoundaryKind) int {
	score := 50
	switch kind {
	case freshBoundaryFileWrite:
		score += 20
		if strings.EqualFold(call.Terminal, "Rename") || strings.EqualFold(call.Terminal, "WriteFile") {
			score += 20
		}
	case freshBoundaryBackendInterface:
		score += 15
	case freshBoundaryDatabaseWrite, freshBoundaryNetworkSend:
		score += 18
	}
	return score
}

func (state *freshPrimaryPlannerState) projectFacts(
	plan *freshPrimaryProbePlan,
	roots []freshPrimaryAnchor,
	selected []freshPrimaryFrontier,
	boundary *freshPrimaryEffectBoundary,
) []semanticdiscovery.Fact {
	if plan.IntentKey == "" || len(roots) == 0 {
		return nil
	}
	var facts []semanticdiscovery.Fact
	entry := roots[0]
	for _, root := range roots {
		if freshPrimaryFunctionIsEntry(root.Symbol) {
			entry = root
			break
		}
	}
	if function, exists := state.functions[entry.Path+"\x00"+entry.Symbol]; exists {
		line := function.Function.StartLine
		column := 1
		evidenceID := goldenStableID("fpe", entry.ID, "declaration")
		statement := fmt.Sprintf(
			"The %s function is an exact bounded entry anchor for the selected repository question.",
			freshHumanLabel(function.Function.Symbol),
		)
		keyword := "entry declaration"
		capabilities := []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityEntry,
			semanticdiscovery.CapabilityBehavior,
		}
		if call, target, ok := state.entryToRootCall(entry, roots); ok {
			evidenceID = call.ID
			line = call.Line
			column = call.Column
			statement = fmt.Sprintf(
				"The %s entry function directly calls the %s repository operation.",
				freshHumanLabel(function.Function.Symbol),
				freshHumanLabel(target.Function.Symbol),
			)
			keyword = "entry call " + freshHumanLabel(call.Target)
			capabilities = append(capabilities, semanticdiscovery.CapabilityDirectCall)
		}
		facts = append(facts, freshPrimaryFact(
			plan, function, freshPrimaryAspectInput,
			statement,
			capabilities,
			evidenceID, line, column,
			keyword,
		))
	}

	coreCalls := state.primaryCoreCalls(roots, selected, boundary)
	for _, call := range coreCalls {
		function, exists := state.functions[call.Path+"\x00"+call.Function]
		if !exists {
			continue
		}
		facts = append(facts, freshPrimaryFact(
			plan, function, freshPrimaryAspectCore,
			fmt.Sprintf(
				"The %s function contains a direct source call to the %s operation.",
				freshHumanLabel(call.Function),
				freshHumanLabel(call.Target),
			),
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDirectCall,
			},
			call.ID, call.Line, call.Column,
			"direct call "+freshHumanLabel(call.Target),
		))
	}

	if boundary != nil {
		if function, exists := state.functions[boundary.FunctionPath+"\x00"+boundary.FunctionSymbol]; exists {
			statement := fmt.Sprintf(
				"The %s function contains the %s operation at a %s boundary.",
				freshHumanLabel(boundary.FunctionSymbol),
				freshHumanLabel(boundary.Operation),
				freshHumanLabel(string(boundary.Kind)),
			)
			if boundary.Kind == freshBoundaryBackendInterface {
				statement = fmt.Sprintf(
					"The %s function contains a call to the %s typed backend boundary; the concrete implementation is not resolved.",
					freshHumanLabel(boundary.FunctionSymbol),
					freshHumanLabel(boundary.Operation),
				)
			}
			facts = append(facts, freshPrimaryFact(
				plan, function, freshPrimaryAspectEffect, statement,
				[]semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityBehavior,
					semanticdiscovery.CapabilityDirectCall,
					semanticdiscovery.CapabilityOutputEffect,
				},
				boundary.EvidenceID, boundary.Line, boundary.Column,
				string(boundary.Kind)+" "+freshHumanLabel(boundary.Operation),
			))
		}
	}

	return facts
}

func (state *freshPrimaryPlannerState) entryToRootCall(
	entry freshPrimaryAnchor,
	roots []freshPrimaryAnchor,
) (freshPrimaryCall, freshPrimaryFunction, bool) {
	function, exists := state.functions[entry.Path+"\x00"+entry.Symbol]
	if !exists {
		return freshPrimaryCall{}, freshPrimaryFunction{}, false
	}
	rootKeys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root.Path == entry.Path && root.Symbol == entry.Symbol {
			continue
		}
		rootKeys[root.Path+"\x00"+root.Symbol] = struct{}{}
	}
	for _, call := range freshPrimaryRankedCalls(state.candidate, function.Calls) {
		target, ok := state.resolveLoadedCall(call)
		if !ok {
			continue
		}
		if _, root := rootKeys[target.Function.Path+"\x00"+target.Function.Symbol]; root {
			return call, target, true
		}
	}
	return freshPrimaryCall{}, freshPrimaryFunction{}, false
}

func freshPrimaryFact(
	plan *freshPrimaryProbePlan,
	function freshPrimaryFunction,
	aspectID string,
	statement string,
	capabilities []semanticdiscovery.Capability,
	evidenceID string,
	line int,
	column int,
	keyword string,
) semanticdiscovery.Fact {
	if column <= 0 {
		column = 1
	}
	factID := goldenStableID("fpp", plan.IntentKey, aspectID, evidenceID)
	return semanticdiscovery.Fact{
		ID:        factID,
		Kind:      semanticdiscovery.FactSourceSignal,
		Statement: statement,
		Keywords: sortedGoldenStrings([]string{
			"answer_aspect:" + aspectID,
			"candidate:" + plan.CandidateID,
			keyword,
		}),
		SourceGroup: goldenStableID(
			"fpg", plan.CandidateID, function.Function.Path,
			function.Function.Symbol, function.Function.ContentSHA256,
		),
		Capabilities: freshCapabilities(capabilities...),
		Scope:        semanticdiscovery.FactScopeLocal,
		Source: &semanticdiscovery.FactSource{
			Path:            function.Function.Path,
			StartLine:       function.Function.StartLine,
			EndLine:         function.Function.EndLine,
			EnclosingSymbol: function.Function.Symbol,
			ContentSHA256:   function.Function.ContentSHA256,
		},
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID:     evidenceID,
			Kind:   "bounded_primary_path_syntax",
			Label:  keyword,
			Path:   function.Function.Path,
			Line:   line,
			Column: column,
		}},
	}
}

func (state *freshPrimaryPlannerState) primaryCoreCalls(
	roots []freshPrimaryAnchor,
	selected []freshPrimaryFrontier,
	boundary *freshPrimaryEffectBoundary,
) []freshPrimaryCall {
	rootKeys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		rootKeys[root.Path+"\x00"+root.Symbol] = struct{}{}
	}
	callByID := make(map[string]freshPrimaryCall)
	for _, function := range state.functions {
		for _, call := range function.Calls {
			callByID[call.ID] = call
		}
	}
	var ranked []freshPrimaryCall
	seen := make(map[string]struct{})
	appendCall := func(call freshPrimaryCall) {
		if _, duplicate := seen[call.ID]; duplicate || freshPrimaryCallDemotion(call) != "" {
			return
		}
		seen[call.ID] = struct{}{}
		ranked = append(ranked, call)
	}
	for _, frontier := range selected {
		appendCall(callByID[frontier.CallEvidenceID])
	}
	for _, root := range roots {
		function := state.functions[root.Path+"\x00"+root.Symbol]
		for _, call := range function.Calls {
			target, ok := state.resolveLoadedCall(call)
			if !ok {
				continue
			}
			if _, candidateRoot := rootKeys[target.Function.Path+"\x00"+target.Function.Symbol]; candidateRoot {
				appendCall(call)
			}
		}
	}
	if boundary != nil {
		function := state.functions[boundary.FunctionPath+"\x00"+boundary.FunctionSymbol]
		var candidates []freshPrimaryCall
		questionTerms := freshTerms(state.candidate.QuestionAnswered)
		for _, call := range function.Calls {
			if call.ID == boundary.EvidenceID || freshPrimaryCallDemotion(call) != "" {
				continue
			}
			score := 0
			for term := range freshTerms(call.Target) {
				if _, match := questionTerms[term]; match {
					score += 10
				}
			}
			if strings.Contains(strings.ToLower(call.Terminal), "decode") ||
				strings.Contains(strings.ToLower(call.Terminal), "encode") ||
				strings.Contains(strings.ToLower(call.Terminal), "compact") ||
				strings.Contains(strings.ToLower(call.Terminal), "transform") {
				score += 12
			}
			if score > 0 {
				candidates = append(candidates, call)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			left := freshPrimaryCoreCallScore(candidates[i], questionTerms)
			right := freshPrimaryCoreCallScore(candidates[j], questionTerms)
			if left != right {
				return left > right
			}
			return candidates[i].ID < candidates[j].ID
		})
		if len(candidates) > 0 {
			appendCall(candidates[0])
		}
	}
	if len(ranked) > 3 {
		ranked = ranked[:3]
	}
	return ranked
}

func freshPrimaryCoreCallScore(call freshPrimaryCall, questionTerms map[string]struct{}) int {
	score := 0
	for term := range freshTerms(call.Target) {
		if _, match := questionTerms[term]; match {
			score += 10
		}
	}
	name := strings.ToLower(call.Terminal)
	for _, word := range []string{"decode", "encode", "compact", "transform", "apply", "restore"} {
		if strings.Contains(name, word) {
			score += 12
		}
	}
	return score
}

func deriveFreshPrimaryEligibility(
	plan *freshPrimaryProbePlan,
	facts []semanticdiscovery.Fact,
	intentCollision bool,
) freshPrimaryEligibility {
	result := freshPrimaryEligibility{IntentCollision: intentCollision}
	symbols := make(map[string]struct{})
	files := make(map[string]struct{})
	useful := 0
	for _, fact := range facts {
		aspect := freshPrimaryFactAspect(fact)
		switch aspect {
		case freshPrimaryAspectInput:
			if freshPrimaryFactHasCapabilities(fact, semanticdiscovery.CapabilityEntry) {
				result.InputFactIDs = append(result.InputFactIDs, fact.ID)
			}
		case freshPrimaryAspectCore:
			if freshPrimaryFactHasCapabilities(
				fact,
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDirectCall,
			) && !freshPrimaryFactIsAuxiliary(fact) {
				result.CoreFactIDs = append(result.CoreFactIDs, fact.ID)
			}
		case freshPrimaryAspectEffect:
			if plan != nil && plan.EffectBoundary != nil &&
				freshPrimaryFactHasCapabilities(fact, semanticdiscovery.CapabilityOutputEffect) &&
				freshFactHasEvidence(fact, plan.EffectBoundary.EvidenceID) {
				result.EffectFactIDs = append(result.EffectFactIDs, fact.ID)
			}
		}
		if fact.Source != nil {
			symbols[fact.Source.Path+"\x00"+fact.Source.EnclosingSymbol] = struct{}{}
			files[fact.Source.Path] = struct{}{}
		}
		visible := strings.ToLower(fact.Statement + " " + strings.Join(fact.Keywords, " "))
		if !strings.Contains(visible, "logger") && !strings.Contains(visible, "logging") &&
			!strings.Contains(visible, "error return") && !strings.Contains(visible, "error path") {
			useful++
		}
	}
	for symbol := range symbols {
		result.DistinctSymbols = append(result.DistinctSymbols, symbol)
	}
	for path := range files {
		result.DistinctFiles = append(result.DistinctFiles, path)
	}
	sort.Strings(result.DistinctSymbols)
	sort.Strings(result.DistinctFiles)
	result.InputFactIDs = sortedGoldenStrings(result.InputFactIDs)
	result.CoreFactIDs = sortedGoldenStrings(result.CoreFactIDs)
	result.EffectFactIDs = sortedGoldenStrings(result.EffectFactIDs)
	result.AllLoggingOrErrorFacts = len(facts) > 0 && useful == 0

	if plan == nil || len(plan.RootAnchors) == 0 {
		result.Reasons = append(result.Reasons, "candidate_specific_anchor_missing")
	}
	if len(result.InputFactIDs) == 0 {
		result.Reasons = append(result.Reasons, "input_trigger_fact_missing")
	}
	if len(result.CoreFactIDs) == 0 {
		result.Reasons = append(result.Reasons, "core_work_fact_missing")
	}
	if len(result.EffectFactIDs) == 0 || plan == nil || plan.EffectBoundary == nil {
		result.Reasons = append(result.Reasons, "observable_effect_fact_missing")
	}
	if len(result.DistinctSymbols) < 2 {
		result.Reasons = append(result.Reasons, "fewer_than_two_exact_symbols")
	}
	if result.IntentCollision {
		result.Reasons = append(result.Reasons, "intent_key_collision")
	}
	if result.AllLoggingOrErrorFacts {
		result.Reasons = append(result.Reasons, "all_logging_or_error_fact_set")
	}
	if plan != nil && plan.RetainedSourceBytes > freshPrimaryMaxRetainedBytes {
		result.Reasons = append(result.Reasons, "retained_source_byte_limit")
	}
	if plan != nil && len(plan.SelectedFrontiers) > freshPrimaryMaxPathFrontiers {
		result.Reasons = append(result.Reasons, "frontier_expansion_limit")
	}
	if len(result.Reasons) > 0 {
		result.Status = freshPrimaryPlanInsufficient
		return result
	}
	result.Status = freshPrimaryPlanReady
	return result
}

func freshPrimaryFactHasCapabilities(
	fact semanticdiscovery.Fact,
	wanted ...semanticdiscovery.Capability,
) bool {
	known := make(map[semanticdiscovery.Capability]struct{}, len(fact.Capabilities))
	for _, capability := range fact.Capabilities {
		known[capability] = struct{}{}
	}
	for _, capability := range wanted {
		if _, exists := known[capability]; !exists {
			return false
		}
	}
	return true
}

func freshPrimaryFactIsAuxiliary(fact semanticdiscovery.Fact) bool {
	visible := strings.ToLower(fact.Statement + " " + strings.Join(fact.Keywords, " "))
	for _, marker := range []string{
		"debug", "error return", "format", "helper predicate", "lock", "logger", "logging",
	} {
		if strings.Contains(visible, marker) {
			return true
		}
	}
	return false
}

func freshPrimaryFactAspect(fact semanticdiscovery.Fact) string {
	for _, keyword := range fact.Keywords {
		if strings.HasPrefix(keyword, "answer_aspect:") {
			return strings.TrimPrefix(keyword, "answer_aspect:")
		}
	}
	return ""
}

func validateFreshPrimaryArtifact(
	artifact semanticdiscovery.Artifact,
	plan *freshPrimaryProbePlan,
	facts []semanticdiscovery.Fact,
) error {
	if plan == nil || plan.Status != freshPrimaryPlanReady {
		return fmt.Errorf("fresh primary mechanism: planner eligibility is not ready")
	}
	if artifact.Question != plan.Question {
		return fmt.Errorf("fresh primary mechanism: synthesis changed the fixed question")
	}
	covered := make(map[string]struct{}, len(artifact.CoveredAspectIDs))
	for _, id := range artifact.CoveredAspectIDs {
		covered[id] = struct{}{}
	}
	for _, id := range []string{
		freshPrimaryAspectInput,
		freshPrimaryAspectCore,
		freshPrimaryAspectEffect,
	} {
		if _, ok := covered[id]; !ok {
			return fmt.Errorf("fresh primary mechanism: required aspect %q is not covered", id)
		}
	}
	if len(artifact.UncoveredAspectIDs) != 0 {
		return fmt.Errorf("fresh primary mechanism: required aspects remain uncovered")
	}
	if len(artifact.Steps) < 3 {
		return fmt.Errorf("fresh primary mechanism: retained %d phases, need at least three", len(artifact.Steps))
	}
	knownFacts := make(map[string]semanticdiscovery.Fact, len(facts))
	for _, fact := range facts {
		knownFacts[fact.ID] = fact
	}
	meaningful := 0
	errorOrLogging := 0
	usedPlannerFacts := make(map[string]struct{})
	for _, statement := range artifact.Statements {
		if statement.Basis == semanticdiscovery.ClaimUnresolved {
			return fmt.Errorf("fresh primary mechanism: unresolved claim retained")
		}
		if len(statement.SupportIDs) == 0 {
			return fmt.Errorf("fresh primary mechanism: statement has no deterministic support")
		}
		for _, id := range statement.SupportIDs {
			fact, ok := knownFacts[id]
			if !ok {
				// Existing candidate support facts are already checked by the core
				// validator; only planner facts are required to be in this slice.
				continue
			}
			if !freshPrimaryFactIsAuxiliary(fact) {
				usedPlannerFacts[id] = struct{}{}
			}
		}
		visible := strings.ToLower(statement.Text)
		if strings.Contains(visible, "logger") || strings.Contains(visible, "logging") ||
			strings.Contains(visible, "error return") || strings.Contains(visible, "error path") {
			errorOrLogging++
			continue
		}
		meaningful++
	}
	if meaningful < 3 {
		return fmt.Errorf("fresh primary mechanism: fewer than three meaningful supported claims")
	}
	if len(usedPlannerFacts) < 2 {
		return fmt.Errorf("fresh primary mechanism: fewer than two non-trivial supported operations")
	}
	if !freshPrimaryArtifactAnswersQuestion(plan.Question, artifact) {
		return fmt.Errorf("fresh primary mechanism: supported answer is not aligned with the fixed question")
	}
	if errorOrLogging > 0 && errorOrLogging*2 >= len(artifact.Statements) {
		return fmt.Errorf("fresh primary mechanism: logging or error claims are not subordinate")
	}
	for _, step := range artifact.Steps {
		if len(step.Evidence) == 0 {
			return fmt.Errorf("fresh primary mechanism: phase %q has no concrete source", step.Title)
		}
	}
	return nil
}

func freshPrimaryArtifactAnswersQuestion(
	question string,
	artifact semanticdiscovery.Artifact,
) bool {
	questionTerms := freshTerms(question)
	for _, generic := range []string{
		"how", "what", "when", "where", "which", "does", "do", "did", "the", "this",
		"that", "code", "work", "works", "repository", "project", "mechanism", "path",
	} {
		delete(questionTerms, generic)
	}
	if len(questionTerms) == 0 {
		return false
	}
	answerText := artifact.Summary
	for _, statement := range artifact.Statements {
		if statement.Basis == semanticdiscovery.ClaimDirect ||
			statement.Basis == semanticdiscovery.ClaimCompositional {
			answerText += " " + statement.Text
		}
	}
	answerTerms := freshTerms(answerText)
	for term := range questionTerms {
		if _, matched := answerTerms[term]; matched {
			return true
		}
	}
	return false
}

func editFreshPrimaryPathsForSavedRun(
	ctx context.Context,
	runDir string,
	repoRoot string,
	stderr io.Writer,
) (freshRepoDemoResult, error) {
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return freshRepoDemoResult{}, fmt.Errorf("fresh primary replan: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	return replanSavedFreshRepoMechanisms(ctx, runDir, repoRoot, client)
}

// replanSavedFreshRepoMechanisms reuses a saved opportunity proposal and
// saved/local source artifacts. It performs no opportunity call and no
// repository-wide analysis; only eligible candidates may reach one synthesis
// call each.
func replanSavedFreshRepoMechanisms(
	ctx context.Context,
	runDir string,
	repoRoot string,
	provider semanticDiscoveryEditor,
) (result freshRepoDemoResult, returnErr error) {
	started := time.Now()
	status := freshRepoDemoStatus{Version: 1, State: "started"}
	defer func() {
		status.WallMillis = time.Since(started).Milliseconds()
		if returnErr != nil && status.State != "published" {
			status.State = "failed"
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		result.Status = status
		if err := writeGoldenJSON(filepath.Join(runDir, "primary_path_replan_status.json"), status); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	if ctx == nil || provider == nil {
		return result, fmt.Errorf("fresh primary replan: context and provider are required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return result, fmt.Errorf("fresh primary replan: resolve run directory: %w", err)
	}
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return result, fmt.Errorf("fresh primary replan: resolve repository root: %w", err)
	}
	data, err := report.ReadRunDir(absRunDir)
	if err != nil {
		return result, fmt.Errorf("fresh primary replan: read saved run: %w", err)
	}
	if err := preserveLegacyFreshMechanism(absRunDir); err != nil {
		return result, err
	}
	savedSources, windowCount, _, savedSourceErr := freshSourceFunctions(absRunDir, absRepoRoot)
	centralSources, parsedBytes, err := freshCentralSourceFunctions(absRunDir, absRepoRoot, data)
	if err != nil {
		return result, err
	}
	if savedSourceErr != nil && len(centralSources) == 0 {
		return result, savedSourceErr
	}
	sources := mergeFreshSourceFunctions(savedSources, centralSources, freshRepoOnboardingMaxPlanningFacts)
	status.SourceWindows = windowCount
	status.SourceFunctions = len(sources)
	status.CentralFunctions = len(centralSources)
	status.CentralParsedBytes = parsedBytes
	status.SourceFacts = len(sources)
	data.SemanticSupplementalFacts = freshFacts(sources)
	savedCandidates, err := loadFreshReplanCandidates(absRunDir, data)
	if err != nil {
		return result, err
	}
	status.QuestionsProposed = len(savedCandidates.Proposal.Candidates)

	works := selectFreshPrimaryCandidates(absRepoRoot, data, savedCandidates.Proposal, sources)
	status.CandidatesSelected = len(works)
	plans := make([]freshRepoCandidatePlan, 0, len(works))
	for _, work := range works {
		plans = append(plans, work.Plan)
	}
	if err := writeGoldenJSON(filepath.Join(absRunDir, freshRepoDemoCandidatesFile), freshRepoCandidatesArtifact{
		Version:       savedCandidates.Version,
		Proposal:      savedCandidates.Proposal,
		Normalization: savedCandidates.Normalization,
		Selected:      plans,
	}); err != nil {
		return result, err
	}

	for _, work := range works {
		attempt, mechanism, summary, attemptErr := attemptFreshCandidate(
			ctx, absRunDir, absRepoRoot, data, work, provider,
		)
		if attempt.Synthesis != nil {
			status.addMetrics(*attempt.Synthesis)
		}
		if attemptErr != nil {
			status.Attempts = append(status.Attempts, attempt)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
			continue
		}
		role, err := freshPublishedOnboardingRole(absRunDir, summary.ID)
		if err != nil {
			status.Attempts = append(status.Attempts, attempt)
			return result, err
		}
		attempt.OnboardingRole = role
		status.Attempts = append(status.Attempts, attempt)
		status.PublishedMechanisms = append(status.PublishedMechanisms, freshPublishedMechanism{
			CandidateID: work.Candidate.ID,
			MechanismID: mechanism.ID,
			ArtifactID:  summary.ID,
			Role:        role,
		})
		status.PublishedCandidateID = work.Candidate.ID
		status.PublishedMechanismID = mechanism.ID
		status.PublishedArtifact = &summary
		status.State = "published"
		if role == report.OnboardingRolePrimaryBehavior {
			return result, nil
		}
	}
	status.State = "no_publishable_candidate"
	status.FailureReason = "bounded automatic evidence planning insufficient"
	return result, nil
}

func loadFreshReplanCandidates(
	runDir string,
	data *report.ReportData,
) (freshRepoCandidatesArtifact, error) {
	var saved freshRepoCandidatesArtifact
	candidatesPath := filepath.Join(runDir, freshRepoDemoCandidatesFile)
	if err := readFreshReplayJSON(candidatesPath, &saved); err == nil {
		if err := validateFreshReplanProposal(data, saved.Proposal); err != nil {
			return freshRepoCandidatesArtifact{}, err
		}
		return saved, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return freshRepoCandidatesArtifact{}, fmt.Errorf(
			"fresh primary replan: read saved candidates: %w",
			err,
		)
	}

	var attempt freshRepoOpportunityAttemptArtifact
	if err := readFreshReplayJSON(
		filepath.Join(runDir, freshRepoOpportunityFile),
		&attempt,
	); err != nil {
		return freshRepoCandidatesArtifact{}, fmt.Errorf(
			"fresh primary replan: read saved opportunity attempt: %w",
			err,
		)
	}
	bundle, err := report.BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return freshRepoCandidatesArtifact{}, fmt.Errorf(
			"fresh primary replan: rebuild opportunity bundle: %w",
			err,
		)
	}
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		bundle,
		attempt.ModelProposal,
	)
	proposal = capFreshProposal(proposal)
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		return freshRepoCandidatesArtifact{}, fmt.Errorf(
			"fresh primary replan: saved opportunity response remains invalid: %w",
			err,
		)
	}
	replay := freshRepoOpportunityAttemptArtifact{
		Version:            1,
		PromptVersion:      attempt.PromptVersion,
		ValidationState:    "accepted_local_replay",
		ModelProposal:      attempt.ModelProposal,
		NormalizedProposal: proposal,
		Normalization:      normalization,
	}
	if err := writeGoldenJSON(filepath.Join(runDir, freshRepoOpportunityReplay), replay); err != nil {
		return freshRepoCandidatesArtifact{}, err
	}
	return freshRepoCandidatesArtifact{
		Version:       1,
		Proposal:      proposal,
		Normalization: normalization,
	}, nil
}

func validateFreshReplanProposal(
	data *report.ReportData,
	proposal semanticdiscovery.OpportunityProposal,
) error {
	bundle, err := report.BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return fmt.Errorf("fresh primary replan: rebuild opportunity bundle: %w", err)
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		return fmt.Errorf("fresh primary replan: validate saved opportunity proposal: %w", err)
	}
	return nil
}
