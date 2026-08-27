package runtimeportfolio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const shardOrdinalSentinel = 99_999_999

type reduceDetailMode string

const (
	reduceDetailExactEvidence    reduceDetailMode = "exact_evidence"
	reduceDetailValidatedSummary reduceDetailMode = "validated_summary"
)

type compiledMapShard struct {
	request       mapRequest
	wire          []byte
	targetsByRef  map[string]Target
	evidenceByRef map[string]Evidence
}

type mapRequestGroup struct {
	targets  []wireTarget
	evidence []wireEvidence
}

type wireTargetSummary struct {
	Ref         string `json:"ref"`
	DisplayName string `json:"display_name"`
	Language    string `json:"language"`
	Kind        string `json:"kind"`
	Selector    string `json:"selector"`
	Default     bool   `json:"default"`
}

type wireCandidateRole struct {
	Ref             string                   `json:"ref"`
	Name            string                   `json:"name"`
	Purpose         string                   `json:"purpose"`
	Prominence      Prominence               `json:"prominence"`
	Kind            RoleKind                 `json:"role_kind"`
	Requiredness    Requiredness             `json:"requiredness"`
	Confidence      Confidence               `json:"confidence"`
	MappingStatus   MappingStatus            `json:"mapping_status"`
	Implementations []responseImplementation `json:"implementations"`
	EvidenceRefs    []string                 `json:"evidence_refs,omitempty"`
	EvidenceCount   int                      `json:"evidence_count,omitempty"`
	EvidenceKinds   []EvidenceKind           `json:"evidence_kinds,omitempty"`
}

type reduceRequest struct {
	Phase           string              `json:"phase"`
	DetailMode      reduceDetailMode    `json:"detail_mode"`
	RepositoryName  string              `json:"repository_name"`
	TargetCount     int                 `json:"target_count"`
	Level           int                 `json:"level"`
	Batch           shardRequest        `json:"batch"`
	TargetCatalog   []wireTargetSummary `json:"target_catalog"`
	Candidates      []wireCandidateRole `json:"candidates"`
	EvidenceCatalog []wireEvidence      `json:"evidence_catalog,omitempty"`
}

type reduceResponseRole struct {
	Name          string        `json:"name"`
	Purpose       string        `json:"purpose"`
	Prominence    Prominence    `json:"prominence"`
	Kind          RoleKind      `json:"role_kind"`
	Requiredness  Requiredness  `json:"requiredness"`
	Confidence    Confidence    `json:"confidence"`
	MappingStatus MappingStatus `json:"mapping_status"`
	CandidateRefs []string      `json:"candidate_refs"`
}

type reduceResponse struct {
	Roles []reduceResponseRole `json:"roles"`
}

type compiledReduceBatch struct {
	request         reduceRequest
	wire            []byte
	candidatesByRef map[string]Role
}

type reduceRequestGroup struct {
	candidates   []wireCandidateRole
	evidenceRefs map[string]struct{}
	bytes        int
}

type executionAggregate struct {
	count         int
	liveCalls     int
	allCached     bool
	requestBytes  int
	responseBytes int
	metrics       llm.Metrics
	issues        []llm.Issue
	cacheKeys     []string
	requestSHAs   []string
	responseSHAs  []string
	request       []byte
	response      []byte
}

func runCompilation(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
) (RunOutcome, error) {
	if err := compilation.validate(); err != nil {
		return RunOutcome{}, err
	}
	if len(compilation.mapShards) == 0 {
		outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[Result]{
			State: append([]byte(nil), compilation.state...),
			Prompt: llm.Prompt{
				System: strings.TrimSpace(systemPrompt), User: string(compilation.wire), ResponseFormatJSON: true,
			},
			Limits: runtimeCallLimits(),
			DecodeValidate: func(raw []byte) (Result, error) {
				return ResolveResponse(compilation, raw)
			},
		})
		if err != nil {
			return RunOutcome{}, fmt.Errorf("runtime portfolio: model cube: %w", err)
		}
		semanticCalls := 1
		if outcome.Cached {
			semanticCalls = 0
			// RunOutcome reports transport work performed by this run. Preserve
			// the accepted call's cached usage while clearing historical work.
			outcome.Metrics.Latency = 0
			outcome.Metrics.Attempts = 0
		}
		return RunOutcome{Outcome: outcome, SemanticCalls: semanticCalls}, nil
	}
	aggregate := executionAggregate{allCached: true}
	mapCalls := make([]llm.Call[[]Role], len(compilation.mapShards))
	completeTargets := cloneTargetAuthority(compilation.targetsByRef)
	// An empty map result is already a legitimate empty portfolio, so permit the
	// map phase to consume the exact call budget. A non-empty result is checked
	// again below before its mandatory global reduce call starts.
	if err := checkRuntimeCallBudget(0, len(mapCalls)); err != nil {
		return RunOutcome{}, err
	}
	for index := range compilation.mapShards {
		shard := compilation.mapShards[index]
		prompt := mapPrompt
		state, err := runtimeCallState(
			compilation, "map", 0, shard.request.Shard.Ordinal,
			shard.request.Shard.Count, shard.wire, prompt, "",
		)
		if err != nil {
			return RunOutcome{}, err
		}
		targets := cloneTargetAuthority(shard.targetsByRef)
		evidence := cloneEvidenceAuthority(shard.evidenceByRef)
		mapCalls[index] = llm.Call[[]Role]{
			State: state,
			Prompt: llm.Prompt{
				System: strings.TrimSpace(prompt), User: string(shard.wire), ResponseFormatJSON: true,
			},
			Limits: runtimeCallLimits(),
			DecodeValidate: func(raw []byte) ([]Role, error) {
				return resolveMapCandidateResponse(raw, targets, evidence, completeTargets)
			},
		}
	}
	mapOutcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, mapCalls)
	if err != nil {
		return RunOutcome{}, fmt.Errorf("runtime portfolio: map shards: %w", err)
	}
	candidates := make([]Role, 0)
	for index := range mapOutcomes {
		addExecutionOutcome(&aggregate, mapOutcomes[index])
		candidates = append(candidates, mapOutcomes[index].Value...)
	}
	candidates = canonicalCandidateRoles(candidates)
	if err := checkRuntimeReduceReservation(aggregate.count, len(candidates)); err != nil {
		return RunOutcome{}, err
	}
	if len(candidates) == 0 {
		result, err := resultFromRoles(compilation, candidates)
		if err != nil {
			return RunOutcome{}, err
		}
		return aggregate.finish(result), nil
	}

	for level := 1; level <= maxReduceLevels; level++ {
		batches, err := packReduceRequests(compilation, level, candidates, MaxRequestBytes)
		if err != nil {
			return RunOutcome{}, err
		}
		calls := make([]llm.Call[[]Role], len(batches))
		if err := checkRuntimeCallBudget(aggregate.count, len(calls)); err != nil {
			return RunOutcome{}, err
		}
		for index := range batches {
			batch := batches[index]
			authoritySHA256, authorityErr := candidateAuthoritySHA256(batch.candidatesByRef)
			if authorityErr != nil {
				return RunOutcome{}, authorityErr
			}
			state, stateErr := runtimeCallState(
				compilation, "reduce", level, batch.request.Batch.Ordinal,
				batch.request.Batch.Count, batch.wire, reducePrompt, authoritySHA256,
			)
			if stateErr != nil {
				return RunOutcome{}, stateErr
			}
			authority := cloneRoleAuthority(batch.candidatesByRef)
			targets := targetsByID(compilation.targetsByRef)
			calls[index] = llm.Call[[]Role]{
				State: state,
				Prompt: llm.Prompt{
					System: strings.TrimSpace(reducePrompt), User: string(batch.wire), ResponseFormatJSON: true,
				},
				Limits: runtimeCallLimits(),
				DecodeValidate: func(raw []byte) ([]Role, error) {
					return resolveReduceResponse(raw, authority, targets)
				},
			}
		}
		outcomes, executeErr := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
		if executeErr != nil {
			return RunOutcome{}, fmt.Errorf("runtime portfolio: reduce level %d: %w", level, executeErr)
		}
		next := make([]Role, 0)
		for index := range outcomes {
			addExecutionOutcome(&aggregate, outcomes[index])
			next = append(next, outcomes[index].Value...)
		}
		next = canonicalCandidateRoles(next)
		if len(batches) == 1 || len(next) == 0 {
			result, resultErr := resultFromRoles(compilation, next)
			if resultErr != nil {
				return RunOutcome{}, resultErr
			}
			return aggregate.finish(result), nil
		}
		if len(next) > len(candidates) {
			return RunOutcome{}, fmt.Errorf(
				"runtime portfolio: reduce level %d increased candidates from %d to %d",
				level, len(candidates), len(next),
			)
		}
		// Level one is the exhaustive evidence-bearing reducer. Even when it
		// legitimately preserves every distinct role, its validated outputs can
		// advance to the compact closed-ref representation used by higher levels.
		if level == 1 {
			candidates = next
			continue
		}
		nextBatches, packErr := packReduceRequests(
			compilation, level+1, next, MaxRequestBytes,
		)
		if packErr != nil {
			return RunOutcome{}, packErr
		}
		beforeBytes := reduceBatchWireFootprint(batches)
		afterBytes := reduceBatchWireFootprint(nextBatches)
		if len(next) == len(candidates) && len(nextBatches) >= len(batches) &&
			afterBytes >= beforeBytes {
			return RunOutcome{}, fmt.Errorf(
				"runtime portfolio: compact reduce level %d made no bounded progress (%d candidates, %d batches, %d bytes)",
				level, len(candidates), len(batches), beforeBytes,
			)
		}
		candidates = next
	}
	return RunOutcome{}, fmt.Errorf(
		"runtime portfolio: reduction exceeded %d levels without reaching one bounded request",
		maxReduceLevels,
	)
}

func runtimeCallLimits() llm.Limits {
	return llm.Limits{
		MaxRequestBytes: MaxProviderRequestBytes, MaxResponseBytes: MaxResponseBytes,
		MaxOutputTokens: MaxOutputTokens,
	}
}

func resolveMapResponse(
	raw []byte,
	targetAuthority map[string]Target,
	evidenceAuthority map[string]Evidence,
) ([]Role, error) {
	return resolveRoleResponse(raw, targetAuthority, evidenceAuthority, false, nil)
}

func resolveMapCandidateResponse(
	raw []byte,
	targetAuthority map[string]Target,
	evidenceAuthority map[string]Evidence,
	completeTargetAuthority map[string]Target,
) ([]Role, error) {
	return resolveRoleResponse(
		raw, targetAuthority, evidenceAuthority, true, completeTargetAuthority,
	)
}

func resolveRoleResponse(
	raw []byte,
	targetAuthority map[string]Target,
	evidenceAuthority map[string]Evidence,
	filterUnsupportedImplementations bool,
	completeTargetAuthority map[string]Target,
) ([]Role, error) {
	decoded, err := decodeMapResponse(raw)
	if err != nil {
		return nil, err
	}
	targets := targetsByID(targetAuthority)
	roles := make([]Role, 0, len(decoded.Roles))
	roleByID := make(map[string]Role)
	nameToID := make(map[string]string)
	for index, proposed := range decoded.Roles {
		if filterUnsupportedImplementations {
			filtered, supported, filterErr := filterMapImplementations(
				proposed, targetAuthority, evidenceAuthority, completeTargetAuthority,
			)
			if filterErr != nil {
				return nil, fmt.Errorf("runtime portfolio: role %d: %w", index, filterErr)
			}
			if !supported {
				continue
			}
			proposed = filtered
		}
		role, restoreErr := restoreRole(proposed, targetAuthority, evidenceAuthority)
		if restoreErr != nil {
			return nil, fmt.Errorf("runtime portfolio: role %d: %w", index, restoreErr)
		}
		if validateErr := validateRole(role, targets); validateErr != nil {
			return nil, fmt.Errorf("runtime portfolio: role %d: %w", index, validateErr)
		}
		nameKey := strings.ToLower(role.Name)
		if previousID, duplicate := nameToID[nameKey]; duplicate && previousID != role.ID {
			return nil, fmt.Errorf("runtime portfolio: conflicting duplicate role name %q", role.Name)
		}
		nameToID[nameKey] = role.ID
		roleByID[role.ID] = role
	}
	for _, role := range roleByID {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roleSortKey(roles[i]) < roleSortKey(roles[j]) })
	return roles, nil
}

// filterMapImplementations applies exact negative compatibility only to
// preliminary map candidates. Every known implementation needs selected
// target-bound evidence; a library implementation specifically needs
// responsibility or program-fact evidence. An unsupported implementation is a
// set member and cannot enter reducer authority. Losing every known member
// drops the candidate instead of changing its mapped scalar or inventing
// evidence. The strict single-call/final resolver does not use this filter.
func filterMapImplementations(
	proposed responseRole,
	targetAuthority map[string]Target,
	evidenceAuthority map[string]Evidence,
	completeTargetAuthority map[string]Target,
) (responseRole, bool, error) {
	if err := validateResponseRoleFields(proposed); err != nil {
		return responseRole{}, false, err
	}
	if err := validateResponseRoleCompatibility(proposed); err != nil {
		return responseRole{}, false, err
	}
	if proposed.MappingStatus != MappingMapped {
		return proposed, true, nil
	}

	evidencedTargets := make(map[string]struct{})
	exactEvidenceTargets := make(map[string]struct{})
	for _, ref := range proposed.EvidenceRefs {
		evidence, known := evidenceAuthority[ref]
		if !known || evidence.ProgramTargetID == "" {
			continue
		}
		evidencedTargets[evidence.ProgramTargetID] = struct{}{}
		if evidence.Kind == EvidenceResponsibility || evidence.Kind == EvidenceProgramFact {
			exactEvidenceTargets[evidence.ProgramTargetID] = struct{}{}
		}
	}

	advertisedImplementations := 0
	filtered := make([]responseImplementation, 0, len(proposed.Implementations))
	for _, implementation := range proposed.Implementations {
		target, known := targetAuthority[implementation.TargetRef]
		if !known {
			if _, advertised := completeTargetAuthority[implementation.TargetRef]; advertised {
				advertisedImplementations++
				if implementation.Mode != "" && !validText(implementation.Mode) {
					return responseRole{}, false, fmt.Errorf("invalid executable mode")
				}
				if implementation.Mode != "" && proposed.Kind == RoleKindLibrary {
					return responseRole{}, false, fmt.Errorf("library implementation has an executable mode")
				}
			}
			continue
		}
		advertisedImplementations++
		// An invalid executable mode and every executable library mode are
		// incompatible semantic assignments, not missing set evidence, and
		// remain terminal in restore/validation.
		if implementation.Mode != "" && !validText(implementation.Mode) {
			return responseRole{}, false, fmt.Errorf("invalid executable mode")
		}
		if implementation.Mode != "" && proposed.Kind == RoleKindLibrary {
			return responseRole{}, false, fmt.Errorf("library implementation has an executable mode")
		}
		if _, supported := evidencedTargets[target.ProgramTargetID]; !supported {
			continue
		}
		if proposed.Kind == RoleKindLibrary {
			if _, supported := exactEvidenceTargets[target.ProgramTargetID]; !supported {
				continue
			}
		}
		filtered = append(filtered, implementation)
	}
	if advertisedImplementations == 0 {
		// Preserve the mandatory-mapping error when only unknown refs were
		// selected; filtering must not turn an unresolved scalar into absence.
		return proposed, true, nil
	}
	if len(filtered) == 0 {
		return responseRole{}, false, nil
	}
	proposed.Implementations = filtered
	return proposed, true, nil
}

func decodeMapResponse(raw []byte) (response, error) {
	if err := validateResponseEnvelope(raw); err != nil {
		return response{}, err
	}
	var decoded response
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return response{}, fmt.Errorf("runtime portfolio: invalid JSON response: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return response{}, err
	}
	if decoded.Roles == nil {
		return response{}, fmt.Errorf("runtime portfolio: roles must be an array")
	}
	return decoded, nil
}

func resolveReduceResponse(
	raw []byte,
	candidatesByRef map[string]Role,
	targets map[string]Target,
) ([]Role, error) {
	if err := validateResponseEnvelope(raw); err != nil {
		return nil, err
	}
	var decoded reduceResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("runtime portfolio: invalid reduce JSON response: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}
	if decoded.Roles == nil {
		return nil, fmt.Errorf("runtime portfolio: reduce roles must be an array")
	}
	usedCandidates := make(map[string]struct{})
	seenRows := make(map[string]struct{})
	roleByID := make(map[string]Role)
	nameToID := make(map[string]string)
	for index, proposed := range decoded.Roles {
		if !validText(proposed.Name) || !validText(proposed.Purpose) ||
			!validProminence(proposed.Prominence) || !validRoleKind(proposed.Kind) ||
			!validRequiredness(proposed.Requiredness) || !validConfidence(proposed.Confidence) ||
			!validMappingStatus(proposed.MappingStatus) || proposed.CandidateRefs == nil {
			return nil, fmt.Errorf("runtime portfolio: reduce role %d has invalid semantic fields", index)
		}
		selected := make(map[string]Role)
		selectedRefs := make([]string, 0, len(proposed.CandidateRefs))
		for _, ref := range proposed.CandidateRefs {
			candidate, known := candidatesByRef[ref]
			if !known {
				continue
			}
			if _, duplicate := selected[ref]; duplicate {
				continue
			}
			selected[ref] = candidate
			selectedRefs = append(selectedRefs, ref)
		}
		if len(selected) == 0 {
			continue
		}
		sort.Strings(selectedRefs)
		rowKeyRaw, marshalErr := json.Marshal(struct {
			Name          string
			Purpose       string
			Prominence    Prominence
			Kind          RoleKind
			Requiredness  Requiredness
			Confidence    Confidence
			MappingStatus MappingStatus
			CandidateRefs []string
		}{
			Name: proposed.Name, Purpose: proposed.Purpose, Prominence: proposed.Prominence,
			Kind: proposed.Kind, Requiredness: proposed.Requiredness, Confidence: proposed.Confidence,
			MappingStatus: proposed.MappingStatus, CandidateRefs: selectedRefs,
		})
		if marshalErr != nil {
			return nil, fmt.Errorf("runtime portfolio: encode normalized reduce row: %w", marshalErr)
		}
		rowKey := string(rowKeyRaw)
		if _, duplicate := seenRows[rowKey]; duplicate {
			continue
		}
		seenRows[rowKey] = struct{}{}
		for _, ref := range selectedRefs {
			if _, duplicate := usedCandidates[ref]; duplicate {
				return nil, fmt.Errorf("runtime portfolio: reduce candidate %q is selected by several roles", ref)
			}
			usedCandidates[ref] = struct{}{}
		}
		role := Role{
			Name: proposed.Name, Purpose: proposed.Purpose, Prominence: proposed.Prominence,
			Kind: proposed.Kind, Requiredness: proposed.Requiredness, Confidence: proposed.Confidence,
			MappingStatus: proposed.MappingStatus, Implementations: []Implementation{}, Evidence: []Evidence{},
		}
		implementations := make(map[string]Implementation)
		evidence := make(map[string]Evidence)
		for _, candidate := range selected {
			for _, implementation := range candidate.Implementations {
				implementations[implementation.ProgramTargetID+"\x00"+implementation.Mode] = implementation
			}
			for _, item := range candidate.Evidence {
				evidence[item.ID] = item
			}
		}
		for _, implementation := range implementations {
			role.Implementations = append(role.Implementations, implementation)
		}
		sort.Slice(role.Implementations, func(i, j int) bool {
			left := role.Implementations[i].ProgramTargetID + "\x00" + role.Implementations[i].Mode
			right := role.Implementations[j].ProgramTargetID + "\x00" + role.Implementations[j].Mode
			return left < right
		})
		for _, item := range evidence {
			role.Evidence = append(role.Evidence, item)
		}
		sort.Slice(role.Evidence, func(i, j int) bool { return role.Evidence[i].ID < role.Evidence[j].ID })
		roleIDValue, roleIDErr := roleID(role)
		if roleIDErr != nil {
			return nil, roleIDErr
		}
		role.ID = roleIDValue
		if err := validateRole(role, targets); err != nil {
			return nil, fmt.Errorf("runtime portfolio: reduce role %d: %w", index, err)
		}
		nameKey := strings.ToLower(role.Name)
		if previousID, duplicate := nameToID[nameKey]; duplicate && previousID != role.ID {
			return nil, fmt.Errorf("runtime portfolio: conflicting duplicate reduce role name %q", role.Name)
		}
		nameToID[nameKey] = role.ID
		roleByID[role.ID] = role
	}
	roles := make([]Role, 0, len(roleByID))
	for _, role := range roleByID {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roleSortKey(roles[i]) < roleSortKey(roles[j]) })
	return roles, nil
}

func validateResponseEnvelope(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return fmt.Errorf("runtime portfolio: response exceeds bounded envelope")
	}
	if _, found := secretscan.Detect(string(raw)); found {
		return fmt.Errorf("runtime portfolio: response contains credential-shaped content")
	}
	return nil
}

func resultFromRoles(compilation Compilation, roles []Role) (Result, error) {
	roles = canonicalCandidateRoles(roles)
	nameToID := make(map[string]string, len(roles))
	targets := targetsByID(compilation.targetsByRef)
	for index, role := range roles {
		if err := validateRole(role, targets); err != nil {
			return Result{}, fmt.Errorf("runtime portfolio: final role %d: %w", index, err)
		}
		nameKey := strings.ToLower(role.Name)
		if previousID, duplicate := nameToID[nameKey]; duplicate && previousID != role.ID {
			return Result{}, fmt.Errorf("runtime portfolio: conflicting duplicate role name %q", role.Name)
		}
		nameToID[nameKey] = role.ID
	}
	result := Result{
		Version: Version, TargetPagePortfolioSHA256: compilation.input.TargetPagePortfolioSHA256,
		Targets: resultTargets(compilation.input.Targets), Roles: roles,
		UnclassifiedTargetIDs: []string{},
	}
	mapped := make(map[string]struct{})
	selectedEvidence := make(map[string]struct{})
	for _, role := range result.Roles {
		for _, implementation := range role.Implementations {
			mapped[implementation.ProgramTargetID] = struct{}{}
		}
		for _, evidence := range role.Evidence {
			selectedEvidence[evidence.ID] = struct{}{}
		}
	}
	for _, target := range result.Targets {
		if _, ok := mapped[target.ProgramTargetID]; !ok {
			result.UnclassifiedTargetIDs = append(result.UnclassifiedTargetIDs, target.ProgramTargetID)
		}
	}
	result.Coverage = Coverage{
		TargetsObserved: len(result.Targets), TargetsMapped: len(mapped),
		TargetsUnclassified: len(result.UnclassifiedTargetIDs), Roles: len(result.Roles),
		EvidenceAdvertised: len(compilation.evidenceByRef), EvidenceSelected: len(selectedEvidence),
	}
	if err := result.validateAgainstCompilation(compilation); err != nil {
		return Result{}, err
	}
	return result, nil
}

func packMapRequests(
	request Request,
	targetAuthority map[string]Target,
	evidenceAuthority map[string]Evidence,
	limit int,
) ([]compiledMapShard, error) {
	if limit <= 0 || len(request.Targets) == 0 {
		return nil, fmt.Errorf("runtime portfolio: invalid map shard bound or empty target catalog")
	}
	type mapUnit struct {
		target   *wireTarget
		evidence []wireEvidence
	}
	evidenceByTarget := make(map[string][]wireEvidence)
	repositoryEvidence := make([]wireEvidence, 0)
	for _, evidence := range request.EvidenceCatalog {
		if evidence.TargetRef == "" {
			repositoryEvidence = append(repositoryEvidence, evidence)
			continue
		}
		evidenceByTarget[evidence.TargetRef] = append(evidenceByTarget[evidence.TargetRef], evidence)
	}
	targetCatalog := make([]wireTargetSummary, 0, len(request.Targets))
	units := make([]mapUnit, 0, len(request.Targets))
	for index := range request.Targets {
		target := request.Targets[index]
		targetCatalog = append(targetCatalog, wireTargetSummary{
			Ref: target.Ref, DisplayName: target.DisplayName, Language: target.Language,
			Kind: target.Kind, Selector: target.Selector, Default: target.Default,
		})
		units = append(units, mapUnit{
			target: &target, evidence: append([]wireEvidence(nil), evidenceByTarget[target.Ref]...),
		})
	}
	groups := make([]mapRequestGroup, 0)
	current := mapRequestGroup{targets: []wireTarget{}, evidence: []wireEvidence{}}
	base, err := json.Marshal(mapRequest{
		Phase: "map", RepositoryName: request.RepositoryName, TargetCount: request.TargetCount,
		Shard:         shardRequest{Ordinal: shardOrdinalSentinel, Count: shardOrdinalSentinel},
		TargetCatalog: targetCatalog, RepositoryEvidence: repositoryEvidence,
		Targets: []wireTarget{}, EvidenceCatalog: []wireEvidence{},
	})
	if err != nil {
		return nil, fmt.Errorf("runtime portfolio: encode empty map shard: %w", err)
	}
	if len(base) > limit {
		return nil, fmt.Errorf(
			"runtime portfolio: compact target catalog and repository evidence need %d bytes, shard limit is %d; global context was not truncated",
			len(base), limit,
		)
	}
	currentBytes := len(base)
	for unitIndex, unit := range units {
		unitBytes, sizeErr := mapUnitBytes(unit.target, unit.evidence, len(current.targets), len(current.evidence))
		if sizeErr != nil {
			return nil, sizeErr
		}
		if currentBytes+unitBytes <= limit {
			appendMapUnit(&current, unit.target, unit.evidence)
			currentBytes += unitBytes
			continue
		}
		if len(current.targets)+len(current.evidence) == 0 {
			return nil, fmt.Errorf(
				"runtime portfolio: map unit %d/%d needs %d bytes, shard limit is %d; the whole target or evidence row was not truncated",
				unitIndex+1, len(units), currentBytes+unitBytes, limit,
			)
		}
		groups = append(groups, current)
		current = mapRequestGroup{targets: []wireTarget{}, evidence: []wireEvidence{}}
		currentBytes = len(base)
		unitBytes, sizeErr = mapUnitBytes(unit.target, unit.evidence, 0, 0)
		if sizeErr != nil {
			return nil, sizeErr
		}
		if currentBytes+unitBytes > limit {
			return nil, fmt.Errorf(
				"runtime portfolio: map unit %d/%d needs %d bytes, shard limit is %d; the whole target or evidence row was not truncated",
				unitIndex+1, len(units), currentBytes+unitBytes, limit,
			)
		}
		appendMapUnit(&current, unit.target, unit.evidence)
		currentBytes += unitBytes
	}
	if len(current.targets)+len(current.evidence) > 0 {
		groups = append(groups, current)
	}
	if len(groups) == 0 || len(groups) > shardOrdinalSentinel {
		return nil, fmt.Errorf("runtime portfolio: invalid map shard count %d", len(groups))
	}
	result := make([]compiledMapShard, len(groups))
	for index, group := range groups {
		payload := mapRequest{
			Phase: "map", RepositoryName: request.RepositoryName, TargetCount: request.TargetCount,
			Shard:         shardRequest{Ordinal: index + 1, Count: len(groups)},
			TargetCatalog: targetCatalog, RepositoryEvidence: repositoryEvidence,
			Targets: group.targets, EvidenceCatalog: group.evidence,
		}
		wire, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, fmt.Errorf("runtime portfolio: encode map shard %d: %w", index+1, marshalErr)
		}
		if len(wire) > limit {
			return nil, fmt.Errorf("runtime portfolio: finalized map shard %d exceeds its byte bound", index+1)
		}
		targets := make(map[string]Target, len(group.targets))
		for _, target := range group.targets {
			authority, known := targetAuthority[target.Ref]
			if !known {
				return nil, fmt.Errorf("runtime portfolio: map shard cites unknown target ref")
			}
			targets[target.Ref] = authority
		}
		evidence := make(map[string]Evidence, len(group.evidence)+len(repositoryEvidence))
		for _, item := range group.evidence {
			authority, known := evidenceAuthority[item.Ref]
			if !known {
				return nil, fmt.Errorf("runtime portfolio: map shard cites unknown evidence ref")
			}
			evidence[item.Ref] = authority
		}
		for _, item := range repositoryEvidence {
			authority, known := evidenceAuthority[item.Ref]
			if !known {
				return nil, fmt.Errorf("runtime portfolio: map shard cites unknown repository evidence ref")
			}
			evidence[item.Ref] = authority
		}
		result[index] = compiledMapShard{
			request: payload, wire: wire, targetsByRef: targets, evidenceByRef: evidence,
		}
	}
	return result, nil
}

func mapUnitBytes(
	target *wireTarget,
	evidence []wireEvidence,
	targetCount int,
	evidenceCount int,
) (int, error) {
	total := 0
	if target != nil {
		raw, err := json.Marshal(target)
		if err != nil {
			return 0, fmt.Errorf("runtime portfolio: encode map target unit: %w", err)
		}
		total += len(raw)
		if targetCount > 0 {
			total++
		}
	}
	for index, item := range evidence {
		raw, err := json.Marshal(item)
		if err != nil {
			return 0, fmt.Errorf("runtime portfolio: encode map evidence unit: %w", err)
		}
		total += len(raw)
		if evidenceCount+index > 0 {
			total++
		}
	}
	return total, nil
}

func appendMapUnit(group *mapRequestGroup, target *wireTarget, evidence []wireEvidence) {
	if target != nil {
		group.targets = append(group.targets, *target)
	}
	group.evidence = append(group.evidence, evidence...)
}

func reduceDetailModeForLevel(level int) reduceDetailMode {
	if level == 1 {
		return reduceDetailExactEvidence
	}
	return reduceDetailValidatedSummary
}

func packReduceRequests(
	compilation Compilation,
	level int,
	candidates []Role,
	limit int,
) ([]compiledReduceBatch, error) {
	if level <= 0 || limit <= 0 || len(candidates) == 0 {
		return nil, fmt.Errorf("runtime portfolio: invalid reduce preparation input")
	}
	candidates = canonicalCandidateRoles(candidates)
	detailMode := reduceDetailModeForLevel(level)
	targetRefByID := make(map[string]string, len(compilation.targetsByRef))
	for ref, target := range compilation.targetsByRef {
		targetRefByID[target.ProgramTargetID] = ref
	}
	evidenceRefByID := make(map[string]string, len(compilation.evidenceByRef))
	for ref, evidence := range compilation.evidenceByRef {
		evidenceRefByID[evidence.ID] = ref
	}
	targetCatalog := make([]wireTargetSummary, 0, len(compilation.Request.Targets))
	for _, target := range compilation.Request.Targets {
		targetCatalog = append(targetCatalog, wireTargetSummary{
			Ref: target.Ref, DisplayName: target.DisplayName, Language: target.Language,
			Kind: target.Kind, Selector: target.Selector, Default: target.Default,
		})
	}
	wireEvidenceByRef := make(map[string]wireEvidence, len(compilation.Request.EvidenceCatalog))
	for _, evidence := range compilation.Request.EvidenceCatalog {
		wireEvidenceByRef[evidence.Ref] = evidence
	}
	wireCandidates := make([]wireCandidateRole, len(candidates))
	roleByCandidateRef := make(map[string]Role, len(candidates))
	for index, candidate := range candidates {
		ref := fmt.Sprintf("c%d", index+1)
		wire := wireCandidateRole{
			Ref: ref, Name: candidate.Name, Purpose: candidate.Purpose,
			Prominence: candidate.Prominence, Kind: candidate.Kind,
			Requiredness: candidate.Requiredness, Confidence: candidate.Confidence,
			MappingStatus:   candidate.MappingStatus,
			Implementations: []responseImplementation{},
		}
		for _, implementation := range candidate.Implementations {
			targetRef := targetRefByID[implementation.ProgramTargetID]
			if targetRef == "" {
				return nil, fmt.Errorf("runtime portfolio: reduce candidate implementation is outside target authority")
			}
			wire.Implementations = append(wire.Implementations, responseImplementation{
				TargetRef: targetRef, Mode: implementation.Mode,
			})
		}
		if detailMode == reduceDetailExactEvidence {
			wire.EvidenceRefs = []string{}
			for _, evidence := range candidate.Evidence {
				evidenceRef := evidenceRefByID[evidence.ID]
				if evidenceRef == "" {
					return nil, fmt.Errorf("runtime portfolio: reduce candidate evidence is outside request authority")
				}
				wire.EvidenceRefs = append(wire.EvidenceRefs, evidenceRef)
			}
			wire.EvidenceRefs = canonicalStrings(wire.EvidenceRefs)
		} else {
			wire.EvidenceCount = len(candidate.Evidence)
			for _, evidence := range candidate.Evidence {
				if evidenceRefByID[evidence.ID] == "" {
					return nil, fmt.Errorf("runtime portfolio: reduce candidate evidence is outside request authority")
				}
			}
			wire.EvidenceKinds = candidateEvidenceKinds(candidate.Evidence)
		}
		wireCandidates[index] = wire
		roleByCandidateRef[ref] = candidate
	}

	emptyRequest := reduceRequest{
		Phase: "reduce", DetailMode: detailMode,
		RepositoryName: compilation.Request.RepositoryName,
		TargetCount:    compilation.Request.TargetCount, Level: level,
		Batch:         shardRequest{Ordinal: shardOrdinalSentinel, Count: shardOrdinalSentinel},
		TargetCatalog: targetCatalog, Candidates: []wireCandidateRole{},
		EvidenceCatalog: []wireEvidence{},
	}
	emptyWire, err := json.Marshal(emptyRequest)
	if err != nil {
		return nil, fmt.Errorf("runtime portfolio: encode empty reduce request: %w", err)
	}
	emptyBytes := len(emptyWire)
	if detailMode == reduceDetailExactEvidence {
		// evidence_catalog is absent from the empty omitempty baseline but every
		// validated candidate contributes evidence. Reserve the exact field and
		// empty-array syntax before adding its rows below.
		emptyBytes += len(`,"evidence_catalog":[]`)
	}
	if emptyBytes > limit {
		return nil, fmt.Errorf(
			"runtime portfolio: compact target catalog needs %d bytes, reduce limit is %d; global context was not truncated",
			emptyBytes, limit,
		)
	}
	newGroup := func() reduceRequestGroup {
		return reduceRequestGroup{
			candidates: []wireCandidateRole{}, evidenceRefs: map[string]struct{}{},
			bytes: emptyBytes,
		}
	}
	groups := make([]reduceRequestGroup, 0)
	current := newGroup()
	for index, candidate := range wireCandidates {
		delta, sizeErr := reduceCandidateDelta(
			candidate, current, wireEvidenceByRef,
		)
		if sizeErr != nil {
			return nil, sizeErr
		}
		if current.bytes+delta <= limit {
			appendReduceCandidate(&current, candidate)
			current.bytes += delta
			continue
		}
		if len(current.candidates) == 0 {
			return nil, fmt.Errorf(
				"runtime portfolio: reduce candidate %d/%d needs %d bytes, shard limit is %d; the candidate was not truncated",
				index+1, len(wireCandidates), current.bytes+delta, limit,
			)
		}
		groups = append(groups, current)
		current = newGroup()
		delta, sizeErr = reduceCandidateDelta(candidate, current, wireEvidenceByRef)
		if sizeErr != nil {
			return nil, sizeErr
		}
		if current.bytes+delta > limit {
			return nil, fmt.Errorf(
				"runtime portfolio: reduce candidate %d/%d needs %d bytes, shard limit is %d; the candidate was not truncated",
				index+1, len(wireCandidates), current.bytes+delta, limit,
			)
		}
		appendReduceCandidate(&current, candidate)
		current.bytes += delta
	}
	if len(current.candidates) > 0 {
		groups = append(groups, current)
	}
	if len(groups) == 0 || len(groups) > shardOrdinalSentinel {
		return nil, fmt.Errorf("runtime portfolio: invalid reduce batch count %d", len(groups))
	}
	result := make([]compiledReduceBatch, len(groups))
	for index, group := range groups {
		evidence := make([]wireEvidence, 0, len(group.evidenceRefs))
		for _, item := range compilation.Request.EvidenceCatalog {
			if _, included := group.evidenceRefs[item.Ref]; included {
				evidence = append(evidence, item)
			}
		}
		payload := reduceRequest{
			Phase: "reduce", DetailMode: detailMode,
			RepositoryName: compilation.Request.RepositoryName,
			TargetCount:    compilation.Request.TargetCount, Level: level,
			Batch: shardRequest{Ordinal: index + 1, Count: len(groups)}, TargetCatalog: targetCatalog,
			Candidates: group.candidates, EvidenceCatalog: evidence,
		}
		wire, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, fmt.Errorf("runtime portfolio: encode reduce batch %d: %w", index+1, marshalErr)
		}
		if len(wire) > limit {
			return nil, fmt.Errorf("runtime portfolio: finalized reduce batch %d exceeds its byte bound", index+1)
		}
		authority := make(map[string]Role, len(group.candidates))
		for _, candidate := range group.candidates {
			authority[candidate.Ref] = roleByCandidateRef[candidate.Ref]
		}
		result[index] = compiledReduceBatch{
			request: payload, wire: wire, candidatesByRef: authority,
		}
	}
	return result, nil
}

func reduceCandidateDelta(
	candidate wireCandidateRole,
	group reduceRequestGroup,
	evidence map[string]wireEvidence,
) (int, error) {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return 0, fmt.Errorf("runtime portfolio: encode reduce candidate: %w", err)
	}
	total := len(raw)
	if len(group.candidates) > 0 {
		total++
	}
	newEvidence := 0
	for _, ref := range candidate.EvidenceRefs {
		if _, known := group.evidenceRefs[ref]; known {
			continue
		}
		item, known := evidence[ref]
		if !known {
			return 0, fmt.Errorf("runtime portfolio: reduce candidate cites unknown evidence ref")
		}
		evidenceRaw, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			return 0, marshalErr
		}
		total += len(evidenceRaw)
		if len(group.evidenceRefs)+newEvidence > 0 {
			total++
		}
		newEvidence++
	}
	return total, nil
}

func appendReduceCandidate(group *reduceRequestGroup, candidate wireCandidateRole) {
	group.candidates = append(group.candidates, candidate)
	for _, ref := range candidate.EvidenceRefs {
		group.evidenceRefs[ref] = struct{}{}
	}
}

func checkRuntimeCallBudget(completed int, planned int) error {
	if completed < 0 || planned < 0 || completed > maxRuntimePortfolioCalls-planned {
		return llm.NewResourceLimitError(llm.ResourceLimitError{
			Stage: "runtime_portfolio", Kind: llm.ResourceLimitSemanticCalls,
			Limit: maxRuntimePortfolioCalls, Observed: completed + planned, ObservedKnown: true,
		})
	}
	return nil
}

func checkRuntimeReduceReservation(completed int, candidates int) error {
	if candidates < 0 {
		return fmt.Errorf("runtime portfolio: invalid reduce candidate count")
	}
	planned := 0
	if candidates > 0 {
		planned = 1
	}
	return checkRuntimeCallBudget(completed, planned)
}

func runtimeCallState(
	compilation Compilation,
	phase string,
	level int,
	ordinal int,
	count int,
	wire []byte,
	prompt string,
	candidateAuthoritySHA256 string,
) ([]byte, error) {
	if phase != "map" && phase != "reduce" {
		return nil, fmt.Errorf("runtime portfolio: invalid execution phase")
	}
	if phase == "map" && candidateAuthoritySHA256 != "" {
		return nil, fmt.Errorf("runtime portfolio: map state has reduce candidate authority")
	}
	if phase == "reduce" && !validSHA256(candidateAuthoritySHA256) {
		return nil, fmt.Errorf("runtime portfolio: reduce state has invalid candidate authority")
	}
	inputSHA256, err := semanticInputSHA256(compilation.input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Contract                 string `json:"contract"`
		PromptVersion            string `json:"prompt_version"`
		PreparationVersion       int    `json:"preparation_version"`
		ResponseSchemaVersion    int    `json:"response_schema_version"`
		InputSHA256              string `json:"input_sha256"`
		CanonicalRequestSHA256   string `json:"canonical_request_sha256"`
		PromptSHA256             string `json:"prompt_sha256"`
		Phase                    string `json:"phase"`
		Level                    int    `json:"level"`
		Ordinal                  int    `json:"ordinal"`
		Count                    int    `json:"count"`
		RequestSHA256            string `json:"request_sha256"`
		CandidateAuthoritySHA256 string `json:"candidate_authority_sha256,omitempty"`
	}{
		Contract: shardedExecutionContract, PromptVersion: runtimePhasePromptVersion(phase),
		PreparationVersion:    runtimePhasePreparationVersion(phase),
		ResponseSchemaVersion: runtimePhaseResponseSchemaVersion(phase), InputSHA256: inputSHA256,
		CanonicalRequestSHA256: compilation.RequestSHA256,
		PromptSHA256:           sha256Hex([]byte(prompt)), Phase: phase, Level: level,
		Ordinal: ordinal, Count: count, RequestSHA256: sha256Hex(wire),
		CandidateAuthoritySHA256: candidateAuthoritySHA256,
	})
}

func candidateAuthoritySHA256(values map[string]Role) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("runtime portfolio: empty reduce candidate authority")
	}
	refs := make([]string, 0, len(values))
	for ref := range values {
		if !validText(ref) {
			return "", fmt.Errorf("runtime portfolio: invalid reduce candidate authority ref")
		}
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	rows := make([]struct {
		Ref  string `json:"ref"`
		Role Role   `json:"role"`
	}, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, struct {
			Ref  string `json:"ref"`
			Role Role   `json:"role"`
		}{Ref: ref, Role: values[ref]})
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("runtime portfolio: encode reduce candidate authority: %w", err)
	}
	return sha256Hex(raw), nil
}

func runtimePhasePromptVersion(phase string) string {
	if phase == "map" {
		return MapPromptVersion
	}
	return ReducePromptVersion
}

func runtimePhasePreparationVersion(phase string) int {
	if phase == "map" {
		return mapPreparationVersion
	}
	return reducePreparationVersion
}

func runtimePhaseResponseSchemaVersion(phase string) int {
	if phase == "map" {
		return mapResponseSchemaVersion
	}
	return reduceResponseSchemaVersion
}

func targetsByID(values map[string]Target) map[string]Target {
	result := make(map[string]Target, len(values))
	for _, target := range values {
		result[target.ProgramTargetID] = target
	}
	return result
}

func canonicalCandidateRoles(values []Role) []Role {
	byID := make(map[string]Role, len(values))
	for _, value := range values {
		byID[value.ID] = value
	}
	result := make([]Role, 0, len(byID))
	for _, value := range byID {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return roleSortKey(result[i]) < roleSortKey(result[j]) })
	return result
}

func reduceBatchWireFootprint(values []compiledReduceBatch) int {
	total := 0
	for _, value := range values {
		total += len(value.wire)
	}
	return total
}

func candidateEvidenceKinds(values []Evidence) []EvidenceKind {
	set := make(map[EvidenceKind]struct{})
	for _, value := range values {
		set[value.Kind] = struct{}{}
	}
	result := make([]EvidenceKind, 0, len(set))
	for kind := range set {
		result = append(result, kind)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func cloneTargetAuthority(values map[string]Target) map[string]Target {
	result := make(map[string]Target, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneEvidenceAuthority(values map[string]Evidence) map[string]Evidence {
	result := make(map[string]Evidence, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneRoleAuthority(values map[string]Role) map[string]Role {
	result := make(map[string]Role, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func addExecutionOutcome[T any](aggregate *executionAggregate, outcome llm.Outcome[T]) {
	if aggregate == nil {
		return
	}
	aggregate.count++
	aggregate.requestBytes += outcome.RequestBytes
	aggregate.responseBytes += outcome.ResponseBytes
	aggregate.issues = append(aggregate.issues, outcome.Issues...)
	aggregate.cacheKeys = append(aggregate.cacheKeys, outcome.CacheKey)
	aggregate.requestSHAs = append(aggregate.requestSHAs, outcome.RequestSHA256)
	aggregate.responseSHAs = append(aggregate.responseSHAs, outcome.ResponseSHA256)
	aggregate.metrics.InputTokens += outcome.Metrics.InputTokens
	aggregate.metrics.OutputTokens += outcome.Metrics.OutputTokens
	aggregate.metrics.ReasoningTokens += outcome.Metrics.ReasoningTokens
	aggregate.metrics.PromptCacheHitTokens += outcome.Metrics.PromptCacheHitTokens
	aggregate.metrics.PromptCacheMissTokens += outcome.Metrics.PromptCacheMissTokens
	aggregate.metrics.ProviderResponseBytes += outcome.Metrics.ProviderResponseBytes
	aggregate.metrics.UsageReported = aggregate.metrics.UsageReported || outcome.Metrics.UsageReported
	if aggregate.count == 1 {
		aggregate.request = append([]byte(nil), outcome.Request...)
		aggregate.response = append([]byte(nil), outcome.Response...)
	}
	if outcome.Cached {
		return
	}
	aggregate.allCached = false
	aggregate.liveCalls++
	aggregate.metrics.Latency += outcome.Metrics.Latency
	aggregate.metrics.Attempts += outcome.Metrics.Attempts
}

func (aggregate executionAggregate) finish(result Result) RunOutcome {
	outcome := llm.Outcome[Result]{
		Value: result, Cached: aggregate.allCached, RequestBytes: aggregate.requestBytes,
		ResponseBytes: aggregate.responseBytes, Metrics: aggregate.metrics,
		Issues: append([]llm.Issue(nil), aggregate.issues...),
	}
	if aggregate.count == 1 {
		outcome.CacheKey = firstString(aggregate.cacheKeys)
		outcome.RequestSHA256 = firstString(aggregate.requestSHAs)
		outcome.ResponseSHA256 = firstString(aggregate.responseSHAs)
		outcome.Request = aggregate.request
		outcome.Response = aggregate.response
		outcome.RequestRedacted = len(outcome.Request) == 0
		outcome.ResponseRedacted = len(outcome.Response) == 0
		outcome.FinishReason = llm.FinishStop
		outcome.ChoiceCount = 1
	} else {
		// There is no aggregate accepted-cache record. Keep CacheKey empty rather
		// than presenting a synthetic digest as an addressable cache identity.
		outcome.RequestSHA256 = aggregateStringDigest(aggregate.requestSHAs)
		outcome.ResponseSHA256 = aggregateStringDigest(aggregate.responseSHAs)
		outcome.RequestRedacted = true
		outcome.ResponseRedacted = true
		outcome.FinishReason = llm.FinishStop
		outcome.ChoiceCount = aggregate.count
	}
	return RunOutcome{Outcome: outcome, SemanticCalls: aggregate.liveCalls}
}

func aggregateStringDigest(values []string) string {
	raw, _ := json.Marshal(values)
	return sha256Hex(raw)
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
