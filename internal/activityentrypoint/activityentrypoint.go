// Package activityentrypoint selects useful activity starts from one sealed,
// language-neutral ProgramIndex. Generic graph structure bounds the candidate
// catalog; only the model can grant activity-entrypoint semantics.
package activityentrypoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version                 = 3
	PreparationVersion      = 5
	ResponseSchemaVersion   = 3
	ArtifactFilename        = "activity-entrypoints.json"
	MaxCandidatesPerBatch   = 1_024
	MaxCandidateBatches     = 32
	MaxAdvertisedCandidates = MaxCandidatesPerBatch * MaxCandidateBatches
	// Selection is a subset of the complete advertised authority, not a
	// separately quota-limited semantic product.
	MaxSelectedEntrypoints     = MaxAdvertisedCandidates
	MaxRelationsPerCandidate   = 16
	MaxCounterpartsPerRelation = 8
	MaxWitnessesPerRelation    = 4
	MaxPayloadBytes            = 2 << 20
	MaxProviderRequestBytes    = 16 << 20
	MaxResponseBytes           = 256 << 10
	MaxOutputTokens            = 8_192
)

const executionContract = "program-index-activity-entrypoint-selection-v6"

// Coverage proves that every structurally eligible callable and exact seeded
// module/package launch anchor in the sealed ProgramIndex was advertised
// exactly once. Omitted is always zero for a successful result.
type Coverage struct {
	ObjectsIndexed               int  `json:"objects_indexed"`
	ProgramObjectsOmitted        int  `json:"program_objects_omitted"`
	ProgramRelationsOmitted      int  `json:"program_relations_omitted"`
	ProgramTargetsOmitted        int  `json:"program_targets_omitted"`
	ProgramWitnessesOmitted      int  `json:"program_witnesses_omitted"`
	CallablesIndexed             int  `json:"callables_indexed"`
	CallablesWithoutLocation     int  `json:"callables_without_location"`
	CallablesIneligible          int  `json:"callables_ineligible"`
	SeededModulesIndexed         int  `json:"seeded_modules_indexed"`
	SeededModulesWithoutLocation int  `json:"seeded_modules_without_location"`
	CandidatesObserved           int  `json:"candidates_observed"`
	CandidatesAdvertised         int  `json:"candidates_advertised"`
	CandidatesOmitted            int  `json:"candidates_omitted"`
	Selected                     int  `json:"selected"`
	Batches                      int  `json:"batches"`
	ModelCalled                  bool `json:"model_called"`
}

// Result restores model-selected refs to the exact ProgramIndex Object rows.
// ProgramIndexSHA256 binds both the selection and its complete coverage to the
// input authority; SHA256 seals the standalone canonical artifact.
type Result struct {
	Version            int                   `json:"version"`
	ProgramIndexSHA256 string                `json:"program_index_sha256"`
	Objects            []programindex.Object `json:"objects"`
	Coverage           Coverage              `json:"coverage"`
	SHA256             string                `json:"sha256"`
}

type targetSourceRequest struct {
	Ref  string `json:"ref"`
	Path string `json:"path"`
}

type targetRequest struct {
	Language        string                `json:"language"`
	Kind            string                `json:"kind"`
	Name            string                `json:"name"`
	Selector        string                `json:"selector,omitempty"`
	Sources         []targetSourceRequest `json:"sources"`
	AnchorSourceRef string                `json:"anchor_source_ref"`
}

type seedRequest struct {
	Ref          string                  `json:"ref"`
	CandidateRef string                  `json:"candidate_ref,omitempty"`
	Kind         programindex.SeedKind   `json:"kind"`
	ObjectKind   programindex.ObjectKind `json:"object_kind"`
	Name         string                  `json:"name"`
	Path         string                  `json:"path"`
	Line         int                     `json:"line"`
	Column       int                     `json:"column"`
}

type topologyRequest struct {
	IncomingCalls     int `json:"incoming_calls"`
	OutgoingCalls     int `json:"outgoing_calls"`
	IncomingExternal  int `json:"incoming_external"`
	OutgoingExternal  int `json:"outgoing_external"`
	DecoratorJoints   int `json:"decorator_joints"`
	IncomingCallbacks int `json:"incoming_callbacks"`
	OutgoingCallbacks int `json:"outgoing_callbacks"`
	UncertainIncoming int `json:"uncertain_incoming"`
	UncertainOutgoing int `json:"uncertain_outgoing"`
}

type relationWitnessRequest struct {
	Kind     string                 `json:"kind"`
	Detail   string                 `json:"detail,omitempty"`
	Location *programindex.Location `json:"location,omitempty"`
}

// relationEvidenceRequest is a bounded, exact adjacency excerpt. It exposes
// adapter-owned decorator, callback, route-like witness and external-call
// facts without asking the model to infer them from aggregate degree counts.
// Omitted counters make the excerpt non-authoritative for absence.
type relationEvidenceRequest struct {
	Direction            string                    `json:"direction"`
	Kind                 programindex.RelationKind `json:"kind"`
	Resolution           programindex.Resolution   `json:"resolution"`
	Invocation           string                    `json:"invocation,omitempty"`
	Location             *programindex.Location    `json:"location,omitempty"`
	CounterpartNames     []string                  `json:"counterpart_names"`
	CounterpartsObserved int                       `json:"counterparts_observed"`
	CounterpartsOmitted  int                       `json:"counterparts_omitted"`
	Witnesses            []relationWitnessRequest  `json:"witnesses"`
	WitnessesObserved    int                       `json:"witnesses_observed"`
	WitnessesOmitted     int                       `json:"witnesses_omitted"`
}

type candidateRequest struct {
	Ref               string                    `json:"ref"`
	Path              string                    `json:"path"`
	Line              int                       `json:"line"`
	Column            int                       `json:"column"`
	Kind              programindex.ObjectKind   `json:"kind"`
	Name              string                    `json:"name"`
	Signature         string                    `json:"signature,omitempty"`
	Visibility        programindex.Visibility   `json:"visibility"`
	OwnerName         string                    `json:"owner_name,omitempty"`
	ContainerName     string                    `json:"container_name,omitempty"`
	SeedKinds         []programindex.SeedKind   `json:"seed_kinds,omitempty"`
	Topology          topologyRequest           `json:"topology"`
	RelationsObserved int                       `json:"relations_observed"`
	RelationEvidence  []relationEvidenceRequest `json:"relation_evidence"`
	RelationsOmitted  int                       `json:"relations_omitted"`
}

type request struct {
	BatchIndex           int                `json:"batch_index"`
	BatchCount           int                `json:"batch_count"`
	CandidatesObserved   int                `json:"candidates_observed"`
	CandidatesAdvertised int                `json:"candidates_advertised"`
	CandidatesOmitted    int                `json:"candidates_omitted"`
	ProgramFrontier      frontierRequest    `json:"program_frontier"`
	Target               targetRequest      `json:"target"`
	Seeds                []seedRequest      `json:"seeds"`
	Candidates           []candidateRequest `json:"candidates"`
}

type response struct {
	ActivityRefs []string `json:"activity_refs"`
}

type candidate struct {
	ref    string
	object programindex.Object
	row    candidateRequest
}

type compilation struct {
	index      programindex.Index
	target     targetRequest
	seeds      []seedRequest
	candidates []candidate
	batches    [][]candidate
	coverage   Coverage
}

type frontierRequest struct {
	ObjectsOmitted               int `json:"objects_omitted"`
	RelationsOmitted             int `json:"relations_omitted"`
	TargetsOmitted               int `json:"targets_omitted"`
	WitnessesOmitted             int `json:"witnesses_omitted"`
	CallablesWithoutLocation     int `json:"callables_without_location"`
	CallablesIneligible          int `json:"callables_ineligible"`
	SeededModulesWithoutLocation int `json:"seeded_modules_without_location"`
}

// Run executes the bounded language-neutral activity classifier. A successful
// result has complete candidate coverage; provider, validation, request-size,
// or batch-count failures are terminal and never produce a partial selection.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	index programindex.Index,
) (Result, error) {
	compiled, err := compile(index)
	if err != nil {
		return Result{}, err
	}
	coverage := compiled.coverage
	coverage.Batches = len(compiled.batches)
	coverage.ModelCalled = len(compiled.candidates) > 0
	if len(compiled.candidates) == 0 {
		return newResult(compiled.index, nil, coverage)
	}

	calls := make([]llm.Call[response], 0, len(compiled.batches))
	for batchIndex, values := range compiled.batches {
		payload := requestForBatch(compiled, batchIndex, values)
		wire, err := json.Marshal(payload)
		if err != nil {
			return Result{}, fmt.Errorf("activity entrypoint: encode batch %d: %w", batchIndex+1, err)
		}
		if len(wire) > MaxPayloadBytes {
			return Result{}, fmt.Errorf(
				"activity entrypoint: complete batch %d payload is %d bytes, limit is %d",
				batchIndex+1, len(wire), MaxPayloadBytes,
			)
		}
		allowed := make(map[string]struct{}, len(values))
		for _, value := range values {
			allowed[value.ref] = struct{}{}
		}
		state, err := compileState(compiled.index.SHA256, batchIndex+1, len(compiled.batches), wire)
		if err != nil {
			return Result{}, fmt.Errorf("activity entrypoint: state batch %d: %w", batchIndex+1, err)
		}
		batchAllowed := allowed
		calls = append(calls, llm.Call[response]{
			State: state,
			Prompt: llm.Prompt{
				System: classifierPrompt, User: string(wire), ResponseFormatJSON: true,
			},
			Limits: llm.Limits{
				MaxRequestBytes: MaxProviderRequestBytes, MaxResponseBytes: MaxResponseBytes,
				MaxOutputTokens: MaxOutputTokens,
			},
			DecodeValidate: func(raw []byte) (response, error) {
				return decodeResponse(raw, batchAllowed)
			},
		})
	}

	outcomes, err := llm.ExecuteJSONBatch(ctx, executor, provider, calls)
	if err != nil {
		return Result{}, fmt.Errorf("activity entrypoint: model cube: %w", err)
	}
	selectedRefs := make(map[string]struct{})
	for _, outcome := range outcomes {
		for _, ref := range outcome.Value.ActivityRefs {
			selectedRefs[ref] = struct{}{}
		}
	}
	if len(selectedRefs) > MaxSelectedEntrypoints {
		return Result{}, fmt.Errorf(
			"activity entrypoint: model selected %d activity starts, limit is %d",
			len(selectedRefs), MaxSelectedEntrypoints,
		)
	}
	selected := make([]programindex.Object, 0, len(selectedRefs))
	for _, value := range compiled.candidates {
		if _, ok := selectedRefs[value.ref]; ok {
			selected = append(selected, cloneObject(value.object))
		}
	}
	coverage.Selected = len(selected)
	result, err := newResult(compiled.index, selected, coverage)
	if err != nil {
		return Result{}, err
	}
	if err := result.ValidateAgainst(compiled.index); err != nil {
		return Result{}, err
	}
	return result, nil
}

func compile(index programindex.Index) (compilation, error) {
	owned := index.Snapshot()
	if err := owned.Validate(); err != nil {
		return compilation{}, fmt.Errorf("activity entrypoint: ProgramIndex: %w", err)
	}
	target, err := compileTarget(owned.Target)
	if err != nil {
		return compilation{}, err
	}
	candidates, byObjectID, candidateCoverage, err := compileCandidates(owned)
	if err != nil {
		return compilation{}, err
	}
	seeds, err := compileSeeds(owned, byObjectID)
	if err != nil {
		return compilation{}, err
	}
	compiled := compilation{
		index: owned, target: target, seeds: seeds, candidates: candidates,
		coverage: Coverage{
			ObjectsIndexed: len(owned.Objects), ProgramObjectsOmitted: owned.Coverage.ObjectsOmitted,
			ProgramRelationsOmitted:      owned.Coverage.RelationsOmitted,
			ProgramTargetsOmitted:        owned.Coverage.TargetsOmitted,
			ProgramWitnessesOmitted:      owned.Coverage.WitnessesOmitted,
			CallablesIndexed:             candidateCoverage.callablesIndexed,
			CallablesWithoutLocation:     candidateCoverage.callablesWithoutLocation,
			CallablesIneligible:          candidateCoverage.callablesIneligible,
			SeededModulesIndexed:         candidateCoverage.seededModulesIndexed,
			SeededModulesWithoutLocation: candidateCoverage.seededModulesWithoutLocation,
			CandidatesObserved:           len(candidates), CandidatesAdvertised: len(candidates), CandidatesOmitted: 0,
		},
	}
	batches, err := partitionCandidates(compiled)
	if err != nil {
		return compilation{}, err
	}
	compiled.batches = batches
	return compiled, nil
}

func compileTarget(target programindex.Target) (targetRequest, error) {
	sources := make([]targetSourceRequest, len(target.Sources))
	anchorRef := ""
	for position, source := range target.Sources {
		ref := fmt.Sprintf("p%d", position+1)
		sources[position] = targetSourceRequest{Ref: ref, Path: source.Path}
		if source.FileRef == target.AnchorFileRef {
			anchorRef = ref
		}
	}
	if anchorRef == "" {
		return targetRequest{}, fmt.Errorf("activity entrypoint: target anchor is absent from target sources")
	}
	return targetRequest{
		Language: target.Language, Kind: target.Kind, Name: target.Name, Selector: target.Selector,
		Sources: sources, AnchorSourceRef: anchorRef,
	}, nil
}

type candidateCoverage struct {
	callablesIndexed             int
	callablesWithoutLocation     int
	callablesIneligible          int
	seededModulesIndexed         int
	seededModulesWithoutLocation int
}

type candidateRelationEvidence struct {
	observed int
	rows     []relationEvidenceRequest
}

func compileCandidates(index programindex.Index) ([]candidate, map[string]string, candidateCoverage, error) {
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	relationEvidence := compileRelationEvidence(index.Relations, objects)
	topology := make(map[string]topologyRequest)
	for _, relation := range index.Relations {
		category := relationCategory(relation.Kind)
		dynamic := dynamicRelation(relation)
		if category != "" {
			value := topology[relation.FromID]
			incrementOutgoing(&value, category)
			topology[relation.FromID] = value
			for _, targetID := range relation.ToIDs {
				value := topology[targetID]
				incrementIncoming(&value, category)
				topology[targetID] = value
			}
		}
		if dynamic {
			value := topology[relation.FromID]
			value.UncertainOutgoing++
			topology[relation.FromID] = value
			for _, targetID := range relation.ToIDs {
				value := topology[targetID]
				value.UncertainIncoming++
				topology[targetID] = value
			}
		}
	}
	seedKinds := make(map[string][]programindex.SeedKind)
	for _, seed := range index.Target.Seeds {
		seedKinds[seed.ObjectID] = append(seedKinds[seed.ObjectID], seed.Kind)
	}
	eligibleCallables := compileEligibleCallables(index, objects, seedKinds)

	values := make([]candidate, 0)
	var coverage candidateCoverage
	for _, object := range index.Objects {
		callableObject := callable(object.Kind)
		seededModule := seededModuleAnchor(object.Kind, seedKinds[object.ID])
		if !callableObject && !seededModule {
			continue
		}
		if callableObject {
			coverage.callablesIndexed++
		} else {
			coverage.seededModulesIndexed++
		}
		if object.Location == nil {
			if callableObject {
				coverage.callablesWithoutLocation++
			} else {
				coverage.seededModulesWithoutLocation++
			}
			continue
		}
		if callableObject {
			if _, eligible := eligibleCallables[object.ID]; !eligible {
				coverage.callablesIneligible++
				continue
			}
		}
		row := candidateRequest{
			Path: object.Location.Path, Line: object.Location.Line, Column: object.Location.Column,
			Kind: object.Kind, Name: object.Name, Signature: object.Signature,
			Visibility: object.Visibility, SeedKinds: append([]programindex.SeedKind(nil), seedKinds[object.ID]...),
			Topology: topology[object.ID],
		}
		if evidence := relationEvidence[object.ID]; evidence.observed > 0 {
			row.RelationsObserved = evidence.observed
			row.RelationEvidence = append([]relationEvidenceRequest(nil), evidence.rows...)
			row.RelationsOmitted = evidence.observed - len(evidence.rows)
		} else {
			row.RelationEvidence = []relationEvidenceRequest{}
		}
		if owner, ok := objects[object.OwnerID]; ok {
			row.OwnerName = owner.Name
		}
		if container, ok := objects[object.ContainerID]; ok {
			row.ContainerName = container.Name
		}
		values = append(values, candidate{object: cloneObject(object), row: row})
	}
	if len(values) > MaxAdvertisedCandidates {
		return nil, nil, candidateCoverage{}, fmt.Errorf(
			"activity entrypoint: complete activity-anchor catalog has %d candidates, limit is %d",
			len(values), MaxAdvertisedCandidates,
		)
	}
	sort.Slice(values, func(left, right int) bool {
		return candidateObjectLess(values[left].object, values[right].object)
	})
	refs := make(map[string]string, len(values))
	for position := range values {
		values[position].ref = fmt.Sprintf("a%d", position+1)
		values[position].row.Ref = values[position].ref
		refs[values[position].object.ID] = values[position].ref
	}
	return values, refs, coverage, nil
}

// compileEligibleCallables is a language- and framework-neutral advertisement
// boundary. It retains exact launch seeds, structural roots, direct seed
// handoffs, and every callback/decorator/implementation joint. Library targets
// additionally retain all public callables. These facts only make an object
// eligible for model classification; they do not establish activity semantics.
func compileEligibleCallables(
	index programindex.Index,
	objects map[string]programindex.Object,
	seedKinds map[string][]programindex.SeedKind,
) map[string]struct{} {
	eligible := make(map[string]struct{})
	exactIncoming := make(map[string]struct{})
	seedObjects := make(map[string]struct{}, len(seedKinds))
	for objectID := range seedKinds {
		seedObjects[objectID] = struct{}{}
		if object, ok := objects[objectID]; ok && callable(object.Kind) {
			eligible[objectID] = struct{}{}
		}
	}

	for _, relation := range index.Relations {
		callRelation := relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationExecutes
		if callRelation && relation.Resolution == programindex.ResolutionExact && relation.TargetsOmitted == 0 {
			for _, targetID := range relation.ToIDs {
				if targetID != relation.FromID {
					exactIncoming[targetID] = struct{}{}
				}
			}
		}
		if callRelation {
			if _, seeded := seedObjects[relation.FromID]; seeded {
				for _, targetID := range relation.ToIDs {
					if object, ok := objects[targetID]; ok && callable(object.Kind) {
						eligible[targetID] = struct{}{}
					}
				}
			}
		}
		if relation.Kind == programindex.RelationPassesCallback ||
			relation.Kind == programindex.RelationDecorates ||
			relation.Kind == programindex.RelationImplements {
			if object, ok := objects[relation.FromID]; ok && callable(object.Kind) {
				eligible[relation.FromID] = struct{}{}
			}
			for _, targetID := range relation.ToIDs {
				if object, ok := objects[targetID]; ok && callable(object.Kind) {
					eligible[targetID] = struct{}{}
				}
			}
		}
	}

	for _, object := range index.Objects {
		if !callable(object.Kind) {
			continue
		}
		if _, hasExactIncoming := exactIncoming[object.ID]; !hasExactIncoming {
			eligible[object.ID] = struct{}{}
		}
		if index.Target.Kind == "library" && object.Visibility == programindex.VisibilityPublic {
			eligible[object.ID] = struct{}{}
		}
	}
	return eligible
}

func compileRelationEvidence(
	relations []programindex.Relation,
	objects map[string]programindex.Object,
) map[string]candidateRelationEvidence {
	result := make(map[string]candidateRelationEvidence)
	appendRow := func(objectID string, row relationEvidenceRequest) {
		value := result[objectID]
		value.observed++
		if len(value.rows) < MaxRelationsPerCandidate {
			value.rows = append(value.rows, row)
		}
		result[objectID] = value
	}
	for _, relation := range relations {
		if !activityEvidenceRelation(relation.Kind) {
			continue
		}
		outgoing := relationEvidenceRow(relation, "outgoing", relation.ToIDs, objects)
		appendRow(relation.FromID, outgoing)
		seenTargets := make(map[string]struct{}, len(relation.ToIDs))
		for _, targetID := range relation.ToIDs {
			if targetID == relation.FromID {
				continue
			}
			if _, duplicate := seenTargets[targetID]; duplicate {
				continue
			}
			seenTargets[targetID] = struct{}{}
			incoming := relationEvidenceRow(
				relation, "incoming", []string{relation.FromID}, objects,
			)
			appendRow(targetID, incoming)
		}
	}
	return result
}

func relationEvidenceRow(
	relation programindex.Relation,
	direction string,
	counterpartIDs []string,
	objects map[string]programindex.Object,
) relationEvidenceRequest {
	observedCounterparts := len(counterpartIDs)
	if direction == "outgoing" {
		observedCounterparts = relation.TargetsObserved
	}
	row := relationEvidenceRequest{
		Direction: direction, Kind: relation.Kind, Resolution: relation.Resolution,
		Invocation: relation.Invocation, Location: cloneLocation(relation.Location),
		CounterpartNames: []string{}, CounterpartsObserved: observedCounterparts,
		Witnesses: []relationWitnessRequest{}, WitnessesObserved: relation.WitnessesObserved,
	}
	for _, objectID := range counterpartIDs {
		if len(row.CounterpartNames) == MaxCounterpartsPerRelation {
			break
		}
		if object, ok := objects[objectID]; ok {
			row.CounterpartNames = append(row.CounterpartNames, object.Name)
		}
	}
	row.CounterpartsOmitted = max(0, row.CounterpartsObserved-len(row.CounterpartNames))
	for _, witness := range relation.Witnesses {
		if len(row.Witnesses) == MaxWitnessesPerRelation {
			break
		}
		row.Witnesses = append(row.Witnesses, relationWitnessRequest{
			Kind: witness.Kind, Detail: witness.Detail, Location: cloneLocation(witness.Location),
		})
	}
	row.WitnessesOmitted = max(0, row.WitnessesObserved-len(row.Witnesses))
	return row
}

func activityEvidenceRelation(kind programindex.RelationKind) bool {
	switch kind {
	case programindex.RelationCalls, programindex.RelationExecutes,
		programindex.RelationDecorates, programindex.RelationPassesCallback,
		programindex.RelationImplements, programindex.RelationReads,
		programindex.RelationWrites, programindex.RelationInvokesExternal:
		return true
	default:
		return false
	}
}

func compileSeeds(index programindex.Index, candidateRefs map[string]string) ([]seedRequest, error) {
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	rows := make([]seedRequest, len(index.Target.Seeds))
	for position, seed := range index.Target.Seeds {
		object, ok := objects[seed.ObjectID]
		if !ok || seed.Location == nil {
			return nil, fmt.Errorf("activity entrypoint: target seed %q has no exact object and location", seed.ObjectID)
		}
		rows[position] = seedRequest{
			Ref: fmt.Sprintf("t%d", position+1), CandidateRef: candidateRefs[object.ID],
			Kind: seed.Kind, ObjectKind: object.Kind, Name: object.Name,
			Path: seed.Location.Path, Line: seed.Location.Line, Column: seed.Location.Column,
		}
	}
	return rows, nil
}

func partitionCandidates(compiled compilation) ([][]candidate, error) {
	if len(compiled.candidates) == 0 {
		return [][]candidate{}, nil
	}
	batches := make([][]candidate, 0)
	for start := 0; start < len(compiled.candidates); {
		maximum := min(MaxCandidatesPerBatch, len(compiled.candidates)-start)
		low, high, accepted := 1, maximum, 0
		for low <= high {
			count := low + (high-low)/2
			trial := compiled.candidates[start : start+count]
			payload := requestForPartition(compiled, trial)
			wire, err := json.Marshal(payload)
			if err != nil {
				return nil, fmt.Errorf("activity entrypoint: encode complete partition: %w", err)
			}
			if len(wire) > MaxPayloadBytes {
				high = count - 1
				continue
			}
			accepted = count
			low = count + 1
		}
		if accepted == 0 {
			return nil, fmt.Errorf(
				"activity entrypoint: candidate %q cannot fit the %d-byte request envelope",
				compiled.candidates[start].object.ID, MaxPayloadBytes,
			)
		}
		end := start + accepted
		batches = append(batches, append([]candidate(nil), compiled.candidates[start:end]...))
		if len(batches) > MaxCandidateBatches {
			return nil, fmt.Errorf(
				"activity entrypoint: complete activity-anchor catalog requires more than %d batches",
				MaxCandidateBatches,
			)
		}
		start = end
	}
	return batches, nil
}

func requestForPartition(compiled compilation, values []candidate) request {
	return request{
		BatchIndex: MaxCandidateBatches, BatchCount: MaxCandidateBatches,
		CandidatesObserved: len(compiled.candidates), CandidatesAdvertised: len(compiled.candidates),
		CandidatesOmitted: 0, ProgramFrontier: frontierForRequest(compiled.coverage),
		Target: compiled.target, Seeds: seedsForCandidates(compiled.seeds, values),
		Candidates: candidateRows(values),
	}
}

func requestForBatch(compiled compilation, batchIndex int, values []candidate) request {
	return request{
		BatchIndex: batchIndex + 1, BatchCount: len(compiled.batches),
		CandidatesObserved: len(compiled.candidates), CandidatesAdvertised: len(compiled.candidates),
		CandidatesOmitted: 0, ProgramFrontier: frontierForRequest(compiled.coverage),
		Target: compiled.target, Seeds: seedsForCandidates(compiled.seeds, values),
		Candidates: candidateRows(values),
	}
}

func seedsForCandidates(seeds []seedRequest, values []candidate) []seedRequest {
	batchRefs := make(map[string]struct{}, len(values))
	for _, value := range values {
		batchRefs[value.ref] = struct{}{}
	}
	rows := append([]seedRequest(nil), seeds...)
	for position := range rows {
		if _, selectable := batchRefs[rows[position].CandidateRef]; !selectable {
			rows[position].CandidateRef = ""
		}
	}
	return rows
}

func candidateRows(values []candidate) []candidateRequest {
	rows := make([]candidateRequest, len(values))
	for position := range values {
		rows[position] = values[position].row
		rows[position].SeedKinds = append([]programindex.SeedKind(nil), values[position].row.SeedKinds...)
		rows[position].RelationEvidence = cloneRelationEvidence(values[position].row.RelationEvidence)
	}
	return rows
}

func cloneRelationEvidence(values []relationEvidenceRequest) []relationEvidenceRequest {
	result := make([]relationEvidenceRequest, len(values))
	for position, value := range values {
		result[position] = value
		result[position].Location = cloneLocation(value.Location)
		result[position].CounterpartNames = append([]string(nil), value.CounterpartNames...)
		result[position].Witnesses = make([]relationWitnessRequest, len(value.Witnesses))
		for witnessPosition, witness := range value.Witnesses {
			result[position].Witnesses[witnessPosition] = witness
			result[position].Witnesses[witnessPosition].Location = cloneLocation(witness.Location)
		}
	}
	return result
}

func decodeResponse(raw []byte, allowed map[string]struct{}) (response, error) {
	value, err := llm.DecodeJSON[response](nil)(raw)
	if err != nil {
		return response{}, err
	}
	if value.ActivityRefs == nil {
		return response{}, fmt.Errorf("activity_refs must be an array")
	}
	known := make([]string, 0, len(value.ActivityRefs))
	seen := make(map[string]struct{}, len(value.ActivityRefs))
	for _, ref := range value.ActivityRefs {
		if _, ok := allowed[ref]; !ok {
			continue
		}
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		known = append(known, ref)
	}
	return response{ActivityRefs: known}, nil
}

func compileState(indexSHA string, batchIndex, batchCount int, wire []byte) ([]byte, error) {
	state := struct {
		Contract              string `json:"contract"`
		PreparationVersion    int    `json:"preparation_version"`
		ResponseSchemaVersion int    `json:"response_schema_version"`
		PromptVersion         string `json:"prompt_version"`
		ProgramIndexSHA256    string `json:"program_index_sha256"`
		BatchIndex            int    `json:"batch_index"`
		BatchCount            int    `json:"batch_count"`
		RequestSHA256         string `json:"request_sha256"`
	}{
		Contract: executionContract, PreparationVersion: PreparationVersion,
		ResponseSchemaVersion: ResponseSchemaVersion, PromptVersion: promptVersion,
		ProgramIndexSHA256: indexSHA, BatchIndex: batchIndex, BatchCount: batchCount,
		RequestSHA256: sha256Hex(wire),
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func relationCategory(kind programindex.RelationKind) string {
	switch kind {
	case programindex.RelationCalls, programindex.RelationExecutes:
		return "calls"
	case programindex.RelationDecorates:
		return "decorator"
	case programindex.RelationPassesCallback:
		return "callbacks"
	case programindex.RelationInvokesExternal:
		return "external"
	default:
		return ""
	}
}

func dynamicRelation(relation programindex.Relation) bool {
	if relation.Resolution == programindex.ResolutionExact && relation.TargetsOmitted == 0 {
		return false
	}
	switch relation.Kind {
	case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationDecorates,
		programindex.RelationPassesCallback, programindex.RelationImplements,
		programindex.RelationInvokesExternal:
		return true
	default:
		return false
	}
}

func incrementIncoming(value *topologyRequest, category string) {
	switch category {
	case "calls":
		value.IncomingCalls++
	case "decorator":
		value.DecoratorJoints++
	case "callbacks":
		value.IncomingCallbacks++
	case "external":
		value.IncomingExternal++
	}
}

func incrementOutgoing(value *topologyRequest, category string) {
	switch category {
	case "calls":
		value.OutgoingCalls++
	case "decorator":
		value.DecoratorJoints++
	case "callbacks":
		value.OutgoingCallbacks++
	case "external":
		value.OutgoingExternal++
	}
}

func callable(kind programindex.ObjectKind) bool {
	return kind == programindex.ObjectFunction || kind == programindex.ObjectMethod || kind == programindex.ObjectLambda
}

func seededModuleAnchor(kind programindex.ObjectKind, seeds []programindex.SeedKind) bool {
	if kind != programindex.ObjectModule && kind != programindex.ObjectPackage {
		return false
	}
	for _, seed := range seeds {
		switch seed {
		case programindex.SeedModule, programindex.SeedMainGuard, programindex.SeedScript:
			return true
		}
	}
	return false
}

func candidateObjectLess(left, right programindex.Object) bool {
	leftLocation, rightLocation := left.Location, right.Location
	if leftLocation.Path != rightLocation.Path {
		return leftLocation.Path < rightLocation.Path
	}
	if leftLocation.Line != rightLocation.Line {
		return leftLocation.Line < rightLocation.Line
	}
	if leftLocation.Column != rightLocation.Column {
		return leftLocation.Column < rightLocation.Column
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.ID < right.ID
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func cloneObject(value programindex.Object) programindex.Object {
	result := value
	if value.Location != nil {
		location := *value.Location
		result.Location = &location
	}
	return result
}

func cloneLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func frontierForRequest(coverage Coverage) frontierRequest {
	return frontierRequest{
		ObjectsOmitted:               coverage.ProgramObjectsOmitted,
		RelationsOmitted:             coverage.ProgramRelationsOmitted,
		TargetsOmitted:               coverage.ProgramTargetsOmitted,
		WitnessesOmitted:             coverage.ProgramWitnessesOmitted,
		CallablesWithoutLocation:     coverage.CallablesWithoutLocation,
		CallablesIneligible:          coverage.CallablesIneligible,
		SeededModulesWithoutLocation: coverage.SeededModulesWithoutLocation,
	}
}
