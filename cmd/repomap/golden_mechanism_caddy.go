package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const goldenDirectoryListingCandidateID = "semantic-candidate-8d47b99879d5cfbc1413052a"

const goldenDirectoryListingEntryFactID = "gmf-0ec8ef0974e6bca4e91a0355"

type goldenProjection struct {
	Candidate semanticdiscovery.OpportunityCandidate `json:"candidate"`
	Facts     []semanticdiscovery.Fact               `json:"facts"`
	Leaf      semanticdiscovery.LeafResult           `json:"leaf"`
}

func addCaddyDirectoryListingLocalSequence(
	projection goldenProjection,
	probe goldenmechanism.Result,
) (goldenProjection, semanticdiscovery.Fact, goldenmechanism.LocalSequenceProof, error) {
	index, err := newGoldenObservationIndex(probe)
	if err != nil {
		return goldenProjection{}, semanticdiscovery.Fact{}, goldenmechanism.LocalSequenceProof{}, err
	}
	branch, err := index.requiredObservation(
		"directory browse branch",
		goldenObservationSelector{
			symbol: "FileServer.ServeHTTP", operation: "branch_predicate",
			contains: "fsrv.Browse != nil && !fileHidden(filename, filesToHide)",
		},
	)
	if err != nil {
		return goldenProjection{}, semanticdiscovery.Fact{}, goldenmechanism.LocalSequenceProof{}, err
	}
	call, err := index.requiredObservation(
		"directory browse call",
		goldenObservationSelector{
			symbol: "FileServer.ServeHTTP", operation: "direct_local_call",
			target: "FileServer.serveBrowse",
		},
	)
	if err != nil {
		return goldenProjection{}, semanticdiscovery.Fact{}, goldenmechanism.LocalSequenceProof{}, err
	}
	proof, err := goldenmechanism.ProveSameBranchDirectCall(
		probe,
		goldenmechanism.SameBranchDirectCallRequest{
			FunctionSymbol:      "FileServer.ServeHTTP",
			BranchObservationID: branch.ID,
			CallObservationID:   call.ID,
		},
	)
	if err != nil {
		return goldenProjection{}, semanticdiscovery.Fact{}, goldenmechanism.LocalSequenceProof{}, err
	}
	if proof.Scope != goldenmechanism.LocalSequenceScopeSameFunctionBranch ||
		proof.BranchCondition != branch.Object ||
		proof.CalledSymbol != "FileServer.serveBrowse" {
		return goldenProjection{}, semanticdiscovery.Fact{}, goldenmechanism.LocalSequenceProof{},
			fmt.Errorf("golden mechanism: local browse sequence has an unexpected scope")
	}
	entrySourceGroup := ""
	for _, fact := range projection.Facts {
		if fact.ID == goldenDirectoryListingEntryFactID {
			entrySourceGroup = fact.SourceGroup
			break
		}
	}
	if entrySourceGroup == "" {
		return goldenProjection{}, semanticdiscovery.Fact{}, goldenmechanism.LocalSequenceProof{},
			fmt.Errorf("golden mechanism: fixed entry fact is unavailable")
	}

	sequenceFact := newGoldenFact(
		"entry-local-sequence",
		"Within the saved request handler's directory-handling source block, if the browse-enabled and not-hidden predicate's true branch is entered, that same branch directly returns the browse call. This same-function and same-branch relation does not establish selection of the enclosing directory branch, call success, absence of other actions in broader handling, or wider runtime order.",
		nil,
		[]string{"browse branch", "conditional local sequence", "directory listing"},
		[]semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityBranch,
			semanticdiscovery.CapabilityDirectCall,
			semanticdiscovery.CapabilitySequence,
			semanticdiscovery.CapabilityLimitation,
		},
		[]goldenmechanism.Observation{proof.BranchObservation, proof.CallObservation},
		nil,
	)
	sequenceFact.SourceGroup = entrySourceGroup

	updated := projection
	updated.Facts = append(append([]semanticdiscovery.Fact(nil), projection.Facts...), sequenceFact)
	updated.Candidate = projection.Candidate
	updated.Candidate.EnrichmentSupportIDs = append(
		append([]string(nil), projection.Candidate.EnrichmentSupportIDs...),
		sequenceFact.ID,
	)
	sort.Strings(updated.Candidate.EnrichmentSupportIDs)
	updated.Leaf = semanticdiscovery.LeafResult{}
	return updated, sequenceFact, proof, nil
}

func caddyDirectoryListingPlan(
	bundle semanticdiscovery.Bundle,
) (goldenmechanism.Plan, error) {
	seeds := []goldenmechanism.Seed{
		{
			OriginFactID: "sf-5316f83d110fd4ae", OriginEvidenceID: "se-851494fa50a5b9f8",
			Path: "modules/caddyhttp/fileserver/staticfiles.go", Symbol: "FileServer.ServeHTTP",
		},
		{
			OriginFactID: "sf-a76968dd8981ba15", OriginEvidenceID: "se-4ac8086fec1232ba",
			Path: "modules/caddyhttp/fileserver/browse.go", Symbol: "FileServer.serveBrowse",
		},
		{
			OriginFactID: "sf-2fde70cb99f7f341", OriginEvidenceID: "se-02f890c69508920e",
			Path: "modules/caddyhttp/fileserver/browse.go", Symbol: "FileServer.browseApplyQueryParams",
		},
		{
			OriginFactID: "sf-06613468fcd3163c", OriginEvidenceID: "se-8897541cba14e0f1",
			Path: "modules/caddyhttp/fileserver/browsetplcontext.go", Symbol: "browseTemplateContext.applySortAndLimit",
		},
	}
	facts := make(map[string]semanticdiscovery.Fact, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		facts[fact.ID] = fact
	}
	for _, seed := range seeds {
		fact, exists := facts[seed.OriginFactID]
		if !exists {
			return goldenmechanism.Plan{}, fmt.Errorf(
				"golden mechanism: saved seed fact %q is unavailable",
				seed.OriginFactID,
			)
		}
		matched := false
		for _, reference := range fact.Evidence {
			if reference.ID == seed.OriginEvidenceID && reference.Path == seed.Path {
				matched = true
				break
			}
		}
		if !matched {
			return goldenmechanism.Plan{}, fmt.Errorf(
				"golden mechanism: seed %q is not bound to its saved source evidence",
				seed.Symbol,
			)
		}
	}
	return goldenmechanism.Plan{
		MechanismID: "caddy-directory-listing",
		Seeds:       seeds,
		ExpansionAllowlist: []string{
			"FileServer.ServeHTTP",
			"FileServer.serveBrowse",
			"FileServer.loadDirectoryContents",
			"FileServer.browseApplyQueryParams",
			"FileServer.directoryListing",
			"browseTemplateContext.applySortAndLimit",
		},
		Limits: goldenmechanism.Limits{
			MaxDepth: 2, MaxFiles: 4, MaxFunctions: 12,
			MaxParsedSourceBytes: 96 << 10,
			MaxSourceBytes:       48 << 10,
			MaxFunctionLines:     192,
			MaxFunctionBytes:     32 << 10,
			Timeout:              3 * time.Second,
		},
	}, nil
}

func projectCaddyDirectoryListing(
	bundle semanticdiscovery.Bundle,
	original semanticdiscovery.OpportunityCandidate,
	probe goldenmechanism.Result,
) (goldenProjection, error) {
	index, err := newGoldenObservationIndex(probe)
	if err != nil {
		return goldenProjection{}, err
	}

	entry, err := index.requiredObservations(
		"entry trigger",
		goldenObservationSelector{symbol: "FileServer.ServeHTTP", operation: "http_handler_entry_signature"},
		goldenObservationSelector{symbol: "FileServer.ServeHTTP", operation: "branch_predicate", contains: "info.IsDir()"},
		goldenObservationSelector{symbol: "FileServer.ServeHTTP", operation: "branch_predicate", contains: "fsrv.Browse != nil"},
		goldenObservationSelector{symbol: "FileServer.ServeHTTP", operation: "direct_local_call", target: "FileServer.serveBrowse"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	collection, err := index.requiredObservations(
		"item collection",
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "direct_local_call", target: "FileServer.loadDirectoryContents"},
		goldenObservationSelector{symbol: "FileServer.loadDirectoryContents", operation: "read_directory"},
		goldenObservationSelector{symbol: "FileServer.loadDirectoryContents", operation: "direct_local_call", target: "FileServer.directoryListing"},
		goldenObservationSelector{symbol: "FileServer.directoryListing", operation: "append_assignment", contains: "tplCtx.Items"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	requestOptions := make([]goldenmechanism.Observation, 0, 6)
	for _, key := range []string{"layout", "limit", "offset", "sort", "order"} {
		observation, findErr := index.requiredObservation(
			"request query "+key,
			goldenObservationSelector{
				symbol: "FileServer.browseApplyQueryParams", operation: "url_query_get", object: key,
			},
		)
		if findErr != nil {
			return goldenProjection{}, findErr
		}
		requestOptions = append(requestOptions, observation)
	}
	applyCall, err := index.requiredObservation(
		"request option transformation call",
		goldenObservationSelector{
			symbol: "FileServer.browseApplyQueryParams", operation: "direct_local_call",
			target: "browseTemplateContext.applySortAndLimit",
		},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	requestOptions = append(requestOptions, applyCall)
	sortDefault, err := index.requiredSourceLine(
		"sort default", "FileServer.browseApplyQueryParams", "sortParam = sortByNameDirFirst",
	)
	if err != nil {
		return goldenProjection{}, err
	}
	orderDefault, err := index.requiredSourceLine(
		"order default", "FileServer.browseApplyQueryParams", "orderParam = sortOrderAsc",
	)
	if err != nil {
		return goldenProjection{}, err
	}

	sortAndPage, err := index.requiredObservations(
		"sort and page",
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "branch_predicate", contains: `l.Order == "desc"`},
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "sort", contains: "byName(*l)"},
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "sort", contains: "byNameDirFirst(*l)"},
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "sort", contains: "bySize(*l)"},
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "sort", contains: "byTime(*l)"},
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "slice", contains: "l.Items[offset:]"},
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "slice", contains: "l.Items[:limit]"},
		goldenObservationSelector{symbol: "browseTemplateContext.applySortAndLimit", operation: "assignment", object: "l.Items"},
	)
	if err != nil {
		return goldenProjection{}, err
	}

	formatAndOutput, err := index.requiredObservations(
		"format selection and response output",
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "branch_predicate", contains: "acceptHeader"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "json_encode"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "plain_format", contains: "item.Name"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "template_execute"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "response_header", contains: "application/json"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "response_header", contains: "text/plain"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "response_header", contains: "text/html"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "write_to_response"},
	)
	if err != nil {
		return goldenProjection{}, err
	}

	branches, err := index.requiredObservations(
		"important branches",
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "branch_predicate", contains: "!strings.HasSuffix"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "error_return", contains: "redirect("},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "error_return", contains: "http.StatusForbidden"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "error_return", contains: "fsrv.notFound"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "error_return", contains: "http.StatusInternalServerError"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "write_header", contains: "http.StatusNotModified"},
		goldenObservationSelector{symbol: "FileServer.serveBrowse", operation: "error_return", contains: "parsing browse template"},
	)
	if err != nil {
		return goldenProjection{}, err
	}

	facts := []semanticdiscovery.Fact{
		newGoldenFact(
			"entry-trigger",
			"The request handler enters directory browsing when the resolved target remains a directory, browsing is configured, and the target is not hidden; the branch contains a direct local call into browsing.",
			[]string{"entry_trigger"},
			[]string{"directory listing", "browse entry"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityEntry,
				semanticdiscovery.CapabilityDirectCall, semanticdiscovery.CapabilityBranch,
			},
			entry,
			nil,
		),
		newGoldenFact(
			"item-collection",
			"Directory browsing calls a bounded collection step that reads directory entries, passes them to local listing construction, and appends structured item records to the listing.",
			[]string{"item_collection"},
			[]string{"directory entries", "listing items"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilitySequence, semanticdiscovery.CapabilityDataRead,
				semanticdiscovery.CapabilityDataWrite, semanticdiscovery.CapabilityDataTransformation,
			},
			collection,
			nil,
		),
		newGoldenFact(
			"request-options",
			"Query handling reads layout, limit, offset, sort, and order from request parameters. Empty sort and order values have named defaults before a direct local call passes the selected controls to listing transformation.",
			[]string{"request_options"},
			[]string{"query options", "sort order limit offset"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityDataRead,
				semanticdiscovery.CapabilityBranch, semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilitySequence,
			},
			requestOptions,
			[]goldenmechanism.SourceLine{sortDefault, orderDefault},
		),
		newGoldenFact(
			"sort-and-page",
			"Listing transformation branches between ascending and reverse ordering for name, directories-first name, size, and modification time, then slices the item list for valid offset and limit values.",
			[]string{"sort_and_page"},
			[]string{"listing sort", "offset limit"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilitySequence, semanticdiscovery.CapabilityDataWrite,
				semanticdiscovery.CapabilityDataTransformation,
			},
			sortAndPage,
			nil,
		),
		newGoldenFact(
			"format-and-output",
			"The representation stage branches on the normalized Accept value: JSON encoding writes listing items to a buffer, plain-text formatting writes rows, and the default executes an HTML template. Each branch sets its content type, and the final buffered write targets the HTTP response writer.",
			[]string{"format_selection", "response_output"},
			[]string{"accept format", "json plain html", "response output"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilitySequence, semanticdiscovery.CapabilityDataTransformation,
				semanticdiscovery.CapabilityOutputEffect,
			},
			formatAndOutput,
			nil,
		),
		newGoldenFact(
			"important-branches",
			"Material alternative paths include a trailing-slash redirect, forbidden and not-found outcomes for collection failures, an internal-error outcome, a not-modified response, and template parsing failure.",
			[]string{"important_branches"},
			[]string{"redirect", "not modified", "error paths"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityBranch, semanticdiscovery.CapabilityErrorPath,
				semanticdiscovery.CapabilityOutputEffect,
			},
			branches,
			nil,
		),
	}
	if len(facts) != 6 {
		return goldenProjection{}, fmt.Errorf("golden mechanism: invalid projected fact count")
	}

	candidate := original
	candidate.SupportIDs = append([]string(nil), original.SupportIDs...)
	candidate.EnrichmentSupportIDs = make([]string, 0, len(facts))
	for _, fact := range facts {
		candidate.EnrichmentSupportIDs = append(candidate.EnrichmentSupportIDs, fact.ID)
	}
	sort.Strings(candidate.EnrichmentSupportIDs)
	candidate.MissingInformation = []string{
		"Direct tests for sorting, paging, and response formats were not inspected by this bounded probe",
	}
	candidate.CapabilityContract = &semanticdiscovery.CapabilityContract{
		RequiredCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityEntry,
			semanticdiscovery.CapabilityDirectCall, semanticdiscovery.CapabilitySequence,
			semanticdiscovery.CapabilityBranch, semanticdiscovery.CapabilityDataRead,
			semanticdiscovery.CapabilityDataWrite, semanticdiscovery.CapabilityDataTransformation,
			semanticdiscovery.CapabilityOutputEffect, semanticdiscovery.CapabilityErrorPath,
			semanticdiscovery.CapabilityTestEvidence,
		},
		AvailableCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic, semanticdiscovery.CapabilityEntry,
			semanticdiscovery.CapabilityDirectCall, semanticdiscovery.CapabilitySequence,
			semanticdiscovery.CapabilityBranch, semanticdiscovery.CapabilityDataRead,
			semanticdiscovery.CapabilityDataWrite, semanticdiscovery.CapabilityDataTransformation,
			semanticdiscovery.CapabilityOutputEffect, semanticdiscovery.CapabilityErrorPath,
		},
		MissingCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityTestEvidence,
		},
		Resolution: semanticdiscovery.CapabilityResolutionPartial,
	}
	candidate.IntentContract = &semanticdiscovery.IntentContract{
		RequiredAnswerAspects: []semanticdiscovery.AnswerAspect{
			{ID: "entry_trigger", Label: "What triggers directory browsing?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityEntry, semanticdiscovery.CapabilityDirectCall, semanticdiscovery.CapabilityBranch}, Key: true},
			{ID: "item_collection", Label: "Where are directory entries collected into listing items?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDataRead, semanticdiscovery.CapabilityDataWrite, semanticdiscovery.CapabilityDirectCall}, Key: true},
			{ID: "request_options", Label: "Where do sort, order, limit, and offset come from?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDataRead, semanticdiscovery.CapabilityDirectCall}},
			{ID: "sort_and_page", Label: "How are ordering, offset, and limit applied?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDataTransformation, semanticdiscovery.CapabilityDataWrite, semanticdiscovery.CapabilityBranch}, Key: true},
			{ID: "format_selection", Label: "How is the response representation selected?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBranch, semanticdiscovery.CapabilityOutputEffect}},
			{ID: "response_output", Label: "Where is the representation written to the client?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityOutputEffect}, Key: true},
			{ID: "important_branches", Label: "Which redirects, cache responses, and error paths matter?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBranch, semanticdiscovery.CapabilityErrorPath}},
			{ID: "known_unknowns", Label: "Which direct behavior tests remain uninspected?", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityTestEvidence}},
		},
		MinCovered:    4,
		MinKeyCovered: 3,
		LocalSearchAliases: []string{
			"how Caddy builds a file listing",
			"how does Caddy build directory listings",
			"как Caddy строит список файлов",
		},
	}

	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(
		semanticdiscovery.Bundle{
			Version: bundle.Version, RepoName: bundle.RepoName,
			PlannerContext: bundle.PlannerContext,
			Facts:          append(append([]semanticdiscovery.Fact(nil), bundle.Facts...), facts...),
		},
		proposal,
	); err != nil {
		// The report layer will perform the authoritative bounded merge and
		// bundle hash. This early check exists only to fail before a model call.
		return goldenProjection{}, fmt.Errorf("golden mechanism: projected candidate contract: %w", err)
	}
	return goldenProjection{Candidate: candidate, Facts: facts}, nil
}

func buildGoldenMechanismLeaf(
	bundle semanticdiscovery.Bundle,
	candidate semanticdiscovery.OpportunityCandidate,
) (semanticdiscovery.LeafResult, error) {
	tasks, err := semanticdiscovery.PlanLeafTasks(
		bundle,
		[]semanticdiscovery.OpportunityCandidate{candidate},
	)
	if err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	facts := make(map[string]semanticdiscovery.Fact, len(tasks[0].Facts))
	for _, fact := range tasks[0].Facts {
		facts[fact.ID] = fact
	}
	artifact := semanticdiscovery.LeafArtifact{
		Version:     semanticdiscovery.LeafArtifactVersion,
		TaskID:      tasks[0].ID,
		CandidateID: candidate.ID,
		Status:      semanticdiscovery.LeafStatusUsable,
	}
	used := make([]string, 0, len(candidate.EnrichmentSupportIDs))
	for _, id := range candidate.EnrichmentSupportIDs {
		fact, exists := facts[id]
		if !exists {
			return semanticdiscovery.LeafResult{}, fmt.Errorf(
				"golden mechanism: enriched fact %q was lost before leaf planning",
				id,
			)
		}
		artifact.Observations = append(artifact.Observations, semanticdiscovery.LeafObservation{
			Text: fact.Statement, SupportIDs: []string{id},
		})
		used = append(used, id)
	}
	missingSupport := make([]string, 0, 2)
	for _, id := range candidate.EnrichmentSupportIDs {
		fact := facts[id]
		for _, keyword := range fact.Keywords {
			if keyword == "answer_aspect:sort_and_page" || keyword == "answer_aspect:format_selection" {
				missingSupport = append(missingSupport, id)
				break
			}
		}
	}
	if len(missingSupport) != 2 {
		return semanticdiscovery.LeafResult{}, fmt.Errorf(
			"golden mechanism: test gap lost its sort and response support",
		)
	}
	artifact.MissingEvidence = []semanticdiscovery.LeafMissingEvidence{{
		Explanation: "Direct behavior tests for sorting, paging, and response representations remain uninspected by the bounded probe.",
		SupportIDs:  missingSupport,
		MissingCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityTestEvidence,
		},
	}}
	used = append(used, missingSupport...)
	artifact.CandidateConnection = semanticdiscovery.LeafCandidateConnection{
		CandidateID: candidate.ID,
		Relation:    "needs_combination",
		Explanation: "The locally verified mechanism observations require one bounded editorial synthesis.",
		SupportIDs:  sortedGoldenStrings(used),
	}
	artifact = semanticdiscovery.NormalizeLeafArtifact(artifact)
	if err := semanticdiscovery.ValidateLeafArtifact(tasks[0], artifact); err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	return semanticdiscovery.LeafResult{Task: tasks[0], Artifact: artifact}, nil
}

type goldenObservationSelector struct {
	symbol    string
	operation string
	target    string
	object    string
	contains  string
}

type goldenObservationIndex struct {
	functions    map[string]goldenmechanism.Function
	bySymbol     map[string]goldenmechanism.Function
	observations map[string][]goldenmechanism.Observation
}

func newGoldenObservationIndex(result goldenmechanism.Result) (goldenObservationIndex, error) {
	if err := result.Validate(); err != nil {
		return goldenObservationIndex{}, err
	}
	index := goldenObservationIndex{
		functions:    make(map[string]goldenmechanism.Function, len(result.Functions)),
		bySymbol:     make(map[string]goldenmechanism.Function, len(result.Functions)),
		observations: make(map[string][]goldenmechanism.Observation),
	}
	for _, function := range result.Functions {
		index.functions[function.ID] = function
		index.bySymbol[function.Symbol] = function
	}
	for _, observation := range result.Observations {
		function, exists := index.functions[observation.FunctionID]
		if !exists {
			return goldenObservationIndex{}, fmt.Errorf("golden mechanism: observation lost its function")
		}
		index.observations[function.Symbol] = append(index.observations[function.Symbol], observation)
	}
	return index, nil
}

func (index goldenObservationIndex) requiredObservations(
	label string,
	selectors ...goldenObservationSelector,
) ([]goldenmechanism.Observation, error) {
	result := make([]goldenmechanism.Observation, 0, len(selectors))
	for _, selector := range selectors {
		observation, err := index.requiredObservation(label, selector)
		if err != nil {
			return nil, err
		}
		result = append(result, observation)
	}
	return result, nil
}

func (index goldenObservationIndex) requiredObservation(
	label string,
	selector goldenObservationSelector,
) (goldenmechanism.Observation, error) {
	for _, observation := range index.observations[selector.symbol] {
		if observation.Operation != selector.operation ||
			(selector.target != "" && observation.TargetSymbol != selector.target) ||
			(selector.object != "" && observation.Object != selector.object) ||
			(selector.contains != "" && !strings.Contains(observation.Object, selector.contains)) {
			continue
		}
		return observation, nil
	}
	return goldenmechanism.Observation{}, fmt.Errorf(
		"golden mechanism: probe did not establish %s (%s %s)",
		label,
		selector.symbol,
		selector.operation,
	)
}

func (index goldenObservationIndex) requiredSourceLine(
	label string,
	symbol string,
	contains string,
) (goldenmechanism.SourceLine, error) {
	function, exists := index.bySymbol[symbol]
	if !exists {
		return goldenmechanism.SourceLine{}, fmt.Errorf(
			"golden mechanism: probe did not retain %s function",
			label,
		)
	}
	for _, line := range function.Source {
		if strings.Contains(line.Text, contains) {
			return line, nil
		}
	}
	return goldenmechanism.SourceLine{}, fmt.Errorf(
		"golden mechanism: probe did not retain %s source line",
		label,
	)
}

func newGoldenFact(
	identity string,
	statement string,
	aspects []string,
	keywords []string,
	capabilities []semanticdiscovery.Capability,
	observations []goldenmechanism.Observation,
	sourceLines []goldenmechanism.SourceLine,
) semanticdiscovery.Fact {
	references := make(map[string]semanticdiscovery.EvidenceRef)
	identityParts := []string{identity, statement}
	for _, observation := range observations {
		identityParts = append(identityParts, observation.ID)
		for _, reference := range observation.Evidence {
			references[reference.ID] = semanticdiscovery.EvidenceRef{
				ID: reference.ID, Kind: "bounded_go_syntax",
				Path: reference.Location.Path, Line: reference.Location.Line,
				Column: reference.Location.Column,
			}
		}
	}
	for _, line := range sourceLines {
		identityParts = append(identityParts, line.ID)
		references[line.ID] = semanticdiscovery.EvidenceRef{
			ID: line.ID, Kind: "bounded_go_syntax",
			Path: line.Location.Path, Line: line.Location.Line,
			Column: line.Location.Column,
		}
	}
	evidence := make([]semanticdiscovery.EvidenceRef, 0, len(references))
	for _, reference := range references {
		evidence = append(evidence, reference)
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].Path != evidence[j].Path {
			return evidence[i].Path < evidence[j].Path
		}
		if evidence[i].Line != evidence[j].Line {
			return evidence[i].Line < evidence[j].Line
		}
		return evidence[i].ID < evidence[j].ID
	})
	factKeywords := append([]string(nil), keywords...)
	for _, aspect := range aspects {
		factKeywords = append(factKeywords, "answer_aspect:"+aspect)
	}
	factKeywords = sortedGoldenStrings(factKeywords)
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	return semanticdiscovery.Fact{
		ID:           goldenStableID("gmf", identityParts...),
		Kind:         semanticdiscovery.FactSourceSignal,
		Statement:    statement,
		Keywords:     factKeywords,
		SourceGroup:  goldenStableID("gmsg", identity),
		Capabilities: capabilities,
		Scope:        semanticdiscovery.FactScopeLocal,
		Evidence:     evidence,
	}
}

func goldenStableID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "-" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func sortedGoldenStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
