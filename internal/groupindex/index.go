// Package groupindex owns the deterministic, sealed group graph built from
// one semantically enriched ProgramIndex and restored model proposals.
package groupindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version          = 19
	ArtifactFilename = "groups-index.json"
)

// Lane is the closed, presentation-level column in which a group belongs.
// Inbound and background-activity subjects deliberately share Triggers.
type Lane string

const (
	LaneTriggers     Lane = "triggers"
	LaneCore         Lane = "core"
	LaneDependencies Lane = "dependencies"
)

func (lane Lane) Valid() bool {
	return lane == LaneTriggers || lane == LaneCore || lane == LaneDependencies
}

// Group is one model-proposed responsibility restored to canonical
// ProgramIndex subject identities. Membership is sparse and overlapping.
// An empty Summary is the explicit no-description state; nothing fills it.
type Group struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Lane    Lane   `json:"lane"`
	// Core is the model's: the program exists for this group. Lane only says
	// where the group stands between what enters and what leaves.
	Core               bool     `json:"core,omitempty"`
	MemberSubjectIDs   []string `json:"member_subject_ids"`
	EvidenceSubjectIDs []string `json:"evidence_subject_ids"`
}

// Endpoint identifies a group in one exact selected target. The same shape is
// used by local connections and by later cross-target matching.
type Endpoint struct {
	TargetID string `json:"target_id"`
	GroupID  string `json:"group_id"`
}

// Connection is a directed semantic relation between two groups. SemanticKind
// is open vocabulary, constrained only to a stable snake_case spelling.
type Connection struct {
	ID                string                              `json:"id"`
	From              Endpoint                            `json:"from"`
	To                Endpoint                            `json:"to"`
	SemanticKind      string                              `json:"semantic_kind"`
	Label             string                              `json:"label"`
	Summary           string                              `json:"summary"`
	SupportResolution programindex.PatternValueResolution `json:"support_resolution"`
	Evidence          []SubjectEndpoint                   `json:"evidence"`
	SourceKind        string                              `json:"source_kind,omitempty"`
	SourceID          string                              `json:"source_id,omitempty"`
	FromSubjectID     string                              `json:"from_subject_id,omitempty"`
	ToSubjectID       string                              `json:"to_subject_id,omitempty"`
	FromLocation      *programindex.Location              `json:"from_location,omitempty"`
	ToLocation        *programindex.Location              `json:"to_location,omitempty"`
	// Phase is the phase of the connection's source subject: init wiring
	// or runtime flow. Derived, never persisted.
	Phase string `json:"-"`
	// ToHelper says the connection's far end is a declaration of this
	// program the helper question decided is a helper (its saved
	// Interpretation.Helper). Derived with Phase, never persisted.
	ToHelper bool `json:"-"`
	// Quiet says the connection is drawn only while one of its ends is
	// looked at: initialization or a call into a helper, in a program that
	// serves something, unless every connection of the program would be
	// quiet (phase.go). Derived, never persisted.
	Quiet bool `json:"-"`
}

// SubjectEndpoint qualifies evidence by target so a cross-target connection
// can cite exact subjects from both participating GroupsIndexes.
type SubjectEndpoint struct {
	TargetID  string `json:"target_id"`
	SubjectID string `json:"subject_id"`
}

// SubjectKind distinguishes the two canonical ProgramIndex identities that a
// grouping proposal may select directly.
type SubjectKind string

const (
	SubjectObject  SubjectKind = "object"
	SubjectPattern SubjectKind = "pattern"
)

func (kind SubjectKind) Valid() bool {
	return kind == SubjectObject || kind == SubjectPattern
}

// ObjectFacts are the exact matching and source-detail facts retained for one
// ProgramIndex object.
type ObjectFacts struct {
	Name        string                  `json:"name"`
	Kind        programindex.ObjectKind `json:"kind"`
	Visibility  programindex.Visibility `json:"visibility"`
	Signature   string                  `json:"signature,omitempty"`
	OwnerID     string                  `json:"owner_id,omitempty"`
	ContainerID string                  `json:"container_id,omitempty"`
	// Parameters and Results are what a callable takes and returns, with the
	// repository type each names when the adapter resolved it.
	Parameters []programindex.TypedName     `json:"parameters,omitempty"`
	Results    []programindex.TypedName     `json:"results,omitempty"`
	External   *programindex.ExternalSymbol `json:"external,omitempty"`
	Location   *programindex.Location       `json:"location,omitempty"`
}

// PatternValueCandidate retains one adapter-proven value reconstruction. Its
// source identities stay inside GroupsIndex; provider projections replace
// them with request-local refs.
type PatternValueCandidate struct {
	ID                      string                              `json:"id"`
	Kind                    programindex.PatternValueKind       `json:"kind"`
	Value                   string                              `json:"value,omitempty"`
	Parts                   []programindex.PatternPart          `json:"parts"`
	Resolution              programindex.PatternValueResolution `json:"resolution"`
	SourceKind              programindex.PatternValueSourceKind `json:"source_kind"`
	SourceObjectIDs         []string                            `json:"source_object_ids"`
	SourceObjectsObserved   int                                 `json:"source_objects_observed"`
	SourceObjectsOmitted    int                                 `json:"source_objects_omitted"`
	SourceArgumentIDs       []string                            `json:"source_argument_ids"`
	SourceArgumentsObserved int                                 `json:"source_arguments_observed"`
	SourceArgumentsOmitted  int                                 `json:"source_arguments_omitted"`
}

// PatternArgument retains literal, template, dynamic, resolved-object, and
// reconstructed-value authority without asking the matching model to recover
// source syntax.
type PatternArgument struct {
	ID                      string                        `json:"id"`
	Position                int                           `json:"position,omitempty"`
	Keyword                 string                        `json:"keyword,omitempty"`
	Kind                    programindex.PatternValueKind `json:"kind"`
	Value                   string                        `json:"value,omitempty"`
	Parts                   []programindex.PatternPart    `json:"parts"`
	ObjectIDs               []string                      `json:"object_ids"`
	Resolution              programindex.Resolution       `json:"resolution,omitempty"`
	ObjectsObserved         int                           `json:"objects_observed"`
	ObjectsOmitted          int                           `json:"objects_omitted"`
	ValueCandidates         []PatternValueCandidate       `json:"value_candidates"`
	ValueCandidatesObserved int                           `json:"value_candidates_observed"`
	ValueCandidatesOmitted  int                           `json:"value_candidates_omitted"`
}

// PatternFacts retain the exact relation context and neutral syntax of one
// grouped RelationPattern.
type PatternFacts struct {
	Form                     programindex.PatternForm  `json:"form"`
	Selector                 string                    `json:"selector"`
	Location                 *programindex.Location    `json:"location,omitempty"`
	RelationID               string                    `json:"relation_id"`
	RelationKind             programindex.RelationKind `json:"relation_kind"`
	RelationResolution       programindex.Resolution   `json:"relation_resolution"`
	FromID                   string                    `json:"from_id"`
	ToIDs                    []string                  `json:"to_ids"`
	Invocation               string                    `json:"invocation,omitempty"`
	ResultID                 string                    `json:"result_id,omitempty"`
	ReceiverID               string                    `json:"receiver_id,omitempty"`
	ReceiverOriginIDs        []string                  `json:"receiver_origin_ids"`
	ReceiverOriginResolution programindex.Resolution   `json:"receiver_origin_resolution,omitempty"`
	Arguments                []PatternArgument         `json:"arguments"`
}

// Subject is a self-contained GroupsIndex node used by later matching and
// frontend projection. Every ProgramIndex object and relation pattern is
// retained even when it has no presentation-group membership. Exactly one fact
// block matches Kind.
type Subject struct {
	ID         string                  `json:"id"`
	Kind       SubjectKind             `json:"kind"`
	Categories []programindex.Category `json:"categories"`
	Object     *ObjectFacts            `json:"object,omitempty"`
	Pattern    *PatternFacts           `json:"pattern,omitempty"`
	// Interpretation stays on the exact entity; grouping never replaces it.
	Interpretation *Interpretation `json:"interpretation,omitempty"`
	// Phase is init, runtime or both: derived, never persisted.
	Phase string `json:"phase,omitempty"`
}

type Interpretation struct {
	Line             string `json:"line"`
	Alias            string `json:"alias,omitempty"`
	Key              bool   `json:"key"`
	Activation       string `json:"activation,omitempty"`
	Operation        string `json:"operation,omitempty"`
	OperationSummary string `json:"operation_summary,omitempty"`
	// Helper is the helper question's decision that the declaration serves
	// the work of other declarations (atlas Symbol.Helper).
	Helper bool `json:"helper,omitempty"`
}

// SubjectAnnotation is the only subject material owned by GroupsIndex. Native
// names, locations, signatures and structure stay in ProgramIndex.
type SubjectAnnotation struct {
	ID             string                  `json:"id"`
	Categories     []programindex.Category `json:"categories"`
	Interpretation *Interpretation         `json:"interpretation,omitempty"`
}

// Operation retains an action on the same subject and group graph. Source
// distinguishes model interpretation from a directly extracted boundary.
type Operation struct {
	ID        string `json:"id"`
	FactID    string `json:"fact_id,omitempty"`
	SubjectID string `json:"subject_id,omitempty"`
	GroupID   string `json:"group_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	// Address is where the operation is reachable, when a publishing call
	// on the same holder stated it.
	Address  string                `json:"address,omitempty"`
	Summary  string                `json:"summary"`
	Source   string                `json:"source"`
	Location programindex.Location `json:"location"`
}

// StructuralEdgeRole is a deterministic projection of exact ProgramIndex
// structure; it never claims a model-authored semantic relation.
type StructuralEdgeRole string

const (
	EdgeObjectOwner                StructuralEdgeRole = "object_owner"
	EdgeObjectContainer            StructuralEdgeRole = "object_container"
	EdgeRelationTarget             StructuralEdgeRole = "relation_target"
	EdgeRelationPattern            StructuralEdgeRole = "relation_pattern"
	EdgePatternTarget              StructuralEdgeRole = "pattern_target"
	EdgePatternResult              StructuralEdgeRole = "pattern_result"
	EdgePatternReceiver            StructuralEdgeRole = "pattern_receiver"
	EdgePatternReceiverOrigin      StructuralEdgeRole = "pattern_receiver_origin"
	EdgePatternArgumentObject      StructuralEdgeRole = "pattern_argument_object"
	EdgePatternValueSourceObject   StructuralEdgeRole = "pattern_value_source_object"
	EdgePatternValueSourceArgument StructuralEdgeRole = "pattern_value_source_argument"
)

func (role StructuralEdgeRole) Valid() bool {
	switch role {
	case EdgeObjectOwner, EdgeObjectContainer, EdgeRelationTarget, EdgeRelationPattern,
		EdgePatternTarget, EdgePatternResult, EdgePatternReceiver,
		EdgePatternReceiverOrigin, EdgePatternArgumentObject,
		EdgePatternValueSourceObject, EdgePatternValueSourceArgument:
		return true
	default:
		return false
	}
}

// StructuralEdge joins subjects using validated ProgramIndex relations and
// nested pattern provenance while retaining the source resolution unchanged.
type StructuralEdge struct {
	FromSubjectID    string                              `json:"from_subject_id"`
	ToSubjectID      string                              `json:"to_subject_id"`
	Role             StructuralEdgeRole                  `json:"role"`
	RelationID       string                              `json:"relation_id"`
	RelationKind     programindex.RelationKind           `json:"relation_kind"`
	Resolution       programindex.Resolution             `json:"resolution"`
	Location         *programindex.Location              `json:"location,omitempty"`
	ArgumentID       string                              `json:"argument_id,omitempty"`
	ValueCandidateID string                              `json:"value_candidate_id,omitempty"`
	SourceArgumentID string                              `json:"source_argument_id,omitempty"`
	ValueResolution  programindex.PatternValueResolution `json:"value_resolution,omitempty"`
	ValueSourceKind  programindex.PatternValueSourceKind `json:"value_source_kind,omitempty"`
}

// Index is the single sealed group-graph authority for one enriched
// ProgramIndex target.
type Index struct {
	Data               []DataRecord        `json:"data,omitempty"`
	Version            int                 `json:"version"`
	Role               string              `json:"role,omitempty"`
	SharedCode         []string            `json:"shared_code,omitempty"`
	Summary            string              `json:"summary,omitempty"`
	Target             programindex.Target `json:"target"`
	ProgramIndexSHA256 string              `json:"program_index_sha256"`
	Subjects           []Subject           `json:"subjects"`
	Groups             []Group             `json:"groups"`
	Operations         []Operation         `json:"operations,omitempty"`
	Outbound           []OutboundCall      `json:"outbound,omitempty"`
	// Containers are the level above the groups: a handful of named parts,
	// each holding several groups. chi's router package really does hold
	// thirty groups — one per middleware file — and thirty is the truth and
	// also unreadable. A container says which of them are one part.
	// ProjectAtlas keeps them in the order the model listed its areas.
	Containers      []Container      `json:"containers"`
	StructuralEdges []StructuralEdge `json:"structural_edges"`
	Connections     []Connection     `json:"connections"`
	// OffMap names the target's files the map of parts does not draw, and
	// MapFailure why the target has no map at all. Their subjects remain
	// subjects, with their interpretations, outside every group.
	OffMap     []OffMapFile `json:"off_map,omitempty"`
	MapFailure string       `json:"map_failure,omitempty"`
	SHA256     string       `json:"sha256"`
	// Reach is what each input's handler reaches, one per operation in
	// Operations order, and Dispatch every relation calling one of several
	// alternatives, in source order (reach.go). Derived by Derive, never
	// persisted, and shared read-only by Snapshot.
	Reach    []Reach        `json:"-"`
	Dispatch []DispatchSite `json:"-"`
	// Entries are the target's seeds, each with its part or its off-map
	// reason (reach.go). Derived, never persisted.
	Entries []Entry `json:"-"`
}

// OffMapTests is the off-map reason of a file of a part made only of test
// code; the atlas's own reasons name the other files off the map.
const OffMapTests = "tests"

// OffMapUnreachable is the off-map reason of the declarations, in one file,
// of a part its program never runs (atlas Box.Unreached).
const OffMapUnreachable = "unreachable"

// OffMapFile is one file the map of parts does not draw, and why. Part names
// the test-only part a file of reason tests belongs to, or the part a file's
// declarations of reason unreachable belong to. SubjectIDs, when present,
// are the declarations of a file a part still holds that are off the map,
// in the atlas's order: for reason undecided, those no box of a file whose
// code goes in several boxes took; for left_out and conflict, those of a
// box the parts answer and its follow-up left out or listed twice, or the
// methods of a type off the map; the file is on the map through the others.
// For reason unreachable they are the part's declarations in that file, in
// subject order. Without them the whole file is off the map.
type OffMapFile struct {
	Path       string   `json:"path"`
	Reason     string   `json:"reason"`
	Part       string   `json:"part,omitempty"`
	SubjectIDs []string `json:"subject_ids,omitempty"`
}

// OffMapUndecided is the reason of a split file's declarations no box took.
const OffMapUndecided = "undecided"

// GroupProposal is one already-restored grouping row. Key exists only to join
// request-local connection proposals; it is never persisted as authority.
type GroupProposal struct {
	Key                string
	Title              string
	Summary            string
	Lane               Lane
	MemberSubjectIDs   []string
	EvidenceSubjectIDs []string
}

// ConnectionProposal joins proposal-local group keys after those groups have
// been restored and accepted.
type ConnectionProposal struct {
	FromGroupKey       string
	ToGroupKey         string
	SemanticKind       string
	Label              string
	Summary            string
	EvidenceSubjectIDs []string
}

// ConnectionInput is one already-restored local or cross-target connection.
// It uses only canonical GroupsIndex endpoints; WithConnections owns
// validation, stable identity, merge, and resealing.
type ConnectionInput struct {
	From              Endpoint
	To                Endpoint
	SemanticKind      string
	Label             string
	Summary           string
	SupportResolution programindex.PatternValueResolution
	Evidence          []SubjectEndpoint
}

// Proposals is the complete already-restored model output for one target.
type Proposals struct {
	Groups      []GroupProposal
	Connections []ConnectionProposal
	Containers  []ContainerProposal
}

// ContainerProposal names one part of a target and which group keys are in it.
// It selects no subject: a container holds exactly the members of the groups
// it names, and cannot add or drop one.
type ContainerProposal struct {
	Key       string
	Title     string
	Summary   string
	Lane      Lane
	GroupKeys []string
}

// Container is one part of a target: a name over several groups.
type Container struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Lane    Lane   `json:"lane"`
	// Core is true when one of the container's groups is core.
	Core     bool     `json:"core,omitempty"`
	GroupIDs []string `json:"group_ids"`
}

// Diagnostic reports one proposal row that could not acquire local authority.
// Broken rows are discarded; Build never guesses a subject or group.
type Diagnostic struct {
	Kind        string `json:"kind"`
	ProposalKey string `json:"proposal_key"`
	Reason      string `json:"reason"`
}

const (
	diagnosticInvalidGroup           = "invalid_group"
	diagnosticUnknownSubject         = "unknown_subject"
	diagnosticLaneMismatch           = "lane_mismatch"
	diagnosticUnsupportedEvidence    = "unsupported_evidence"
	diagnosticConflictingGroupKey    = "conflicting_group_key"
	diagnosticContainerIncomplete    = "container_incomplete"
	diagnosticContainerUnknownGroup  = "container_unknown_group"
	diagnosticContainerRepeatedGroup = "container_repeated_group"
	diagnosticContainerTooSmall      = "container_too_small"
	diagnosticInvalidConnection      = "invalid_connection"
	diagnosticUnknownGroup           = "unknown_group"
	diagnosticConflictingConnection  = "conflicting_connection"
)

// Build filters restored proposal rows against one exact enriched
// ProgramIndex, assigns stable group identities, resolves proposal-local group
// keys, canonicalizes compatible rows, and seals the result.
func Build(program programindex.Index, accepted Proposals) (Index, []Diagnostic, error) {
	if err := program.Validate(); err != nil {
		return Index{}, nil, fmt.Errorf("group index: validate program index: %w", err)
	}
	if program.Categorization == nil {
		return Index{}, nil, fmt.Errorf("group index: program index is not categorized")
	}

	subjects := compileSubjects(program)
	groupCandidates := make(map[string]map[string]Group)
	diagnostics := make([]Diagnostic, 0)
	for _, proposal := range accepted.Groups {
		group, kind, reason := compileGroupProposal(program.Target.ID, subjects, proposal)
		if reason != "" {
			diagnostics = append(diagnostics, Diagnostic{Kind: kind, ProposalKey: proposal.Key, Reason: reason})
			continue
		}
		byValue := groupCandidates[proposal.Key]
		if byValue == nil {
			byValue = make(map[string]Group)
			groupCandidates[proposal.Key] = byValue
		}
		byValue[groupKey(group)] = group
	}

	groupsByValue := make(map[string]Group)
	groupValueByProposalKey := make(map[string]string)
	for key, candidates := range groupCandidates {
		if len(candidates) != 1 {
			diagnostics = append(diagnostics, Diagnostic{
				Kind: diagnosticConflictingGroupKey, ProposalKey: key,
				Reason: "proposal-local group key resolves to conflicting groups",
			})
			continue
		}
		for value, group := range candidates {
			groupValueByProposalKey[key] = value
			groupsByValue[value] = group
		}
	}

	groups := make([]Group, 0, len(groupsByValue))
	for _, group := range groupsByValue {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groupKey(groups[i]) < groupKey(groups[j]) })
	groupIDByValue := make(map[string]string, len(groups))
	for position := range groups {
		groups[position].ID = compactOrdinal("g", position)
		groupIDByValue[groupKey(groups[position])] = groups[position].ID
	}
	groupIDsByKey := make(map[string]string, len(groupValueByProposalKey))
	for key, value := range groupValueByProposalKey {
		groupIDsByKey[key] = groupIDByValue[value]
	}
	if groups == nil {
		groups = []Group{}
	}

	containers, containerDiagnostics := compileContainers(program.Target.ID, groupIDsByKey, accepted.Containers)
	diagnostics = append(diagnostics, containerDiagnostics...)

	connectionCandidates := make(map[string]map[string]Connection)
	for _, proposal := range accepted.Connections {
		connection, kind, reason := compileConnectionProposal(program.Target.ID, subjects, groupIDsByKey, proposal)
		proposalKey := connectionProposalKey(proposal)
		if reason != "" {
			diagnostics = append(diagnostics, Diagnostic{Kind: kind, ProposalKey: proposalKey, Reason: reason})
			continue
		}
		slot := connectionSlot(connection)
		byValue := connectionCandidates[slot]
		if byValue == nil {
			byValue = make(map[string]Connection)
			connectionCandidates[slot] = byValue
		}
		byValue[connectionKey(connection)] = connection
	}

	connections := make([]Connection, 0, len(connectionCandidates))
	for slot, candidates := range connectionCandidates {
		if len(candidates) != 1 {
			diagnostics = append(diagnostics, Diagnostic{
				Kind: diagnosticConflictingConnection, ProposalKey: slot,
				Reason: "connection slot has conflicting accepted rows",
			})
			continue
		}
		for _, connection := range candidates {
			connections = append(connections, connection)
		}
	}
	sort.Slice(connections, func(i, j int) bool {
		return connectionKey(connections[i]) < connectionKey(connections[j])
	})
	assignConnectionIDs(connections, 0)
	if connections == nil {
		connections = []Connection{}
	}
	allSubjectIDs := make(map[string]struct{}, len(subjects))
	for subjectID := range subjects {
		allSubjectIDs[subjectID] = struct{}{}
	}
	allSubjects := compileRetainedSubjects(program, allSubjectIDs)
	structuralEdges := compileStructuralEdges(program, allSubjectIDs)

	index := Index{
		Version:            Version,
		Target:             program.Target.Snapshot(),
		ProgramIndexSHA256: program.SHA256,
		Subjects:           allSubjects,
		Groups:             groups,
		Containers:         containers,
		StructuralEdges:    structuralEdges,
		Connections:        connections,
	}
	Derive(&index)
	seal, err := indexDigest(index)
	if err != nil {
		return Index{}, nil, err
	}
	index.SHA256 = seal
	if err := index.Validate(); err != nil {
		return Index{}, nil, err
	}
	return index, canonicalDiagnostics(diagnostics), nil
}

// WithConnections merges restored local or cross-target connections into a
// complete GroupsIndex set. Each accepted connection is stored exactly once,
// in its From.TargetID index. Broken rows are diagnosed and discarded.
func WithConnections(indexes []Index, accepted []ConnectionInput) ([]Index, []Diagnostic, error) {
	if err := ValidateSet(indexes); err != nil {
		return nil, nil, err
	}
	result := make([]Index, len(indexes))
	indexPositionByTarget := make(map[string]int, len(indexes))
	groupsByTarget := make(map[string]map[string]struct{}, len(indexes))
	subjectsByTarget := make(map[string]map[string]struct{}, len(indexes))
	for position, index := range indexes {
		result[position] = index.Snapshot()
		indexPositionByTarget[index.Target.ID] = position
		groups := make(map[string]struct{}, len(index.Groups))
		for _, group := range index.Groups {
			groups[group.ID] = struct{}{}
		}
		groupsByTarget[index.Target.ID] = groups
		subjects := make(map[string]struct{}, len(index.Subjects))
		for _, subject := range index.Subjects {
			subjects[subject.ID] = struct{}{}
		}
		subjectsByTarget[index.Target.ID] = subjects
	}

	type connectionSlotCandidates struct {
		existing *Connection
		values   map[string]Connection
	}
	candidatesByOwner := make(map[string]map[string]*connectionSlotCandidates, len(indexes))
	for _, index := range indexes {
		bySlot := make(map[string]*connectionSlotCandidates)
		for _, connection := range index.Connections {
			copyValue := connection
			bySlot[connectionSlot(connection)] = &connectionSlotCandidates{
				existing: &copyValue,
				values:   map[string]Connection{connectionKey(connection): connection},
			}
		}
		candidatesByOwner[index.Target.ID] = bySlot
	}

	diagnostics := make([]Diagnostic, 0)
	for _, input := range accepted {
		proposalKey := connectionInputKey(input)
		connection, kind, reason := compileConnectionInput(groupsByTarget, subjectsByTarget, input)
		if reason != "" {
			diagnostics = append(diagnostics, Diagnostic{Kind: kind, ProposalKey: proposalKey, Reason: reason})
			continue
		}
		bySlot := candidatesByOwner[connection.From.TargetID]
		slot := connectionSlot(connection)
		candidates := bySlot[slot]
		if candidates == nil {
			candidates = &connectionSlotCandidates{values: make(map[string]Connection)}
			bySlot[slot] = candidates
		}
		candidates.values[connectionKey(connection)] = connection
	}

	for targetID, bySlot := range candidatesByOwner {
		connections := make([]Connection, 0, len(bySlot))
		newConnections := make([]Connection, 0, len(bySlot))
		for slot, candidates := range bySlot {
			if candidates.existing != nil {
				connections = append(connections, *candidates.existing)
				if len(candidates.values) > 1 {
					diagnostics = append(diagnostics, Diagnostic{
						Kind: diagnosticConflictingConnection, ProposalKey: slot,
						Reason: "accepted connection conflicts with existing authority",
					})
				}
				continue
			}
			if len(candidates.values) != 1 {
				diagnostics = append(diagnostics, Diagnostic{
					Kind: diagnosticConflictingConnection, ProposalKey: slot,
					Reason: "connection slot has conflicting accepted rows",
				})
				continue
			}
			for _, connection := range candidates.values {
				newConnections = append(newConnections, connection)
			}
		}
		sort.Slice(connections, func(i, j int) bool { return compactIDLess(connections[i].ID, connections[j].ID, "x") })
		sort.Slice(newConnections, func(i, j int) bool { return connectionKey(newConnections[i]) < connectionKey(newConnections[j]) })
		assignConnectionIDs(newConnections, len(connections))
		connections = append(connections, newConnections...)
		position := indexPositionByTarget[targetID]
		if reflect.DeepEqual(result[position].Connections, connections) {
			continue
		}
		result[position].Connections = connections
		result[position].SHA256 = ""
		seal, err := indexDigest(result[position])
		if err != nil {
			return nil, nil, err
		}
		result[position].SHA256 = seal
	}
	if err := ValidateSet(result); err != nil {
		return nil, nil, err
	}
	return result, canonicalDiagnostics(diagnostics), nil
}

// WithOutbound replaces the accepted communication overlay and reseals the
// index. Native ProgramIndex facts remain untouched.
func WithOutbound(index Index, outbound []OutboundCall) (Index, error) {
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	result := index.Snapshot()
	result.Outbound = append([]OutboundCall(nil), outbound...)
	for position := range result.Outbound {
		result.Outbound[position].Values = cloneStrings(outbound[position].Values)
		result.Outbound[position].Uses = cloneDestinationUses(outbound[position].Uses)
	}
	result.SHA256 = ""
	seal, err := indexDigest(result)
	if err != nil {
		return Index{}, err
	}
	result.SHA256 = seal
	if err := result.Validate(); err != nil {
		return Index{}, err
	}
	return result, nil
}

// Snapshot returns a consumer-owned deep copy. The derived Reach and
// Dispatch are immutable after Derive and are shared, not copied.
func (index Index) Snapshot() Index {
	result := index
	result.Data = cloneData(index.Data)
	result.SharedCode = cloneStrings(index.SharedCode)
	result.Operations = append([]Operation(nil), index.Operations...)
	result.Outbound = append([]OutboundCall(nil), index.Outbound...)
	for i := range result.Outbound {
		result.Outbound[i].Values = cloneStrings(index.Outbound[i].Values)
		result.Outbound[i].Uses = cloneDestinationUses(index.Outbound[i].Uses)
	}
	result.Target = index.Target.Snapshot()
	result.Subjects = make([]Subject, len(index.Subjects))
	for position, subject := range index.Subjects {
		result.Subjects[position] = cloneSubject(subject)
	}
	result.Groups = make([]Group, len(index.Groups))
	for position, group := range index.Groups {
		result.Groups[position] = group
		result.Groups[position].MemberSubjectIDs = cloneStrings(group.MemberSubjectIDs)
		result.Groups[position].EvidenceSubjectIDs = cloneStrings(group.EvidenceSubjectIDs)
	}
	result.StructuralEdges = make([]StructuralEdge, len(index.StructuralEdges))
	copy(result.StructuralEdges, index.StructuralEdges)
	for i := range result.StructuralEdges {
		result.StructuralEdges[i].Location = cloneLocation(index.StructuralEdges[i].Location)
	}
	result.Connections = make([]Connection, len(index.Connections))
	for position, connection := range index.Connections {
		result.Connections[position] = connection
		result.Connections[position].Evidence = cloneSubjectEndpoints(connection.Evidence)
		if connection.FromLocation != nil {
			location := *connection.FromLocation
			result.Connections[position].FromLocation = &location
		}
		if connection.ToLocation != nil {
			location := *connection.ToLocation
			result.Connections[position].ToLocation = &location
		}
	}
	return result
}

// Validate checks the complete canonical schema, identities, local endpoint
// bindings, and artifact seal. Foreign endpoints remain valid for later
// cross-target matching; each connection is stored by its source target.
func (index Index) Validate() error {
	if index.Version != Version || !validSHA256(index.ProgramIndexSHA256) {
		return fmt.Errorf("group index: invalid producer identity")
	}
	if err := index.Target.Validate(); err != nil {
		return fmt.Errorf("group index: invalid target: %w", err)
	}
	if index.Subjects == nil || index.Groups == nil || index.Containers == nil || index.StructuralEdges == nil || index.Connections == nil {
		return fmt.Errorf("group index: missing collections")
	}

	subjectsByID := make(map[string]Subject, len(index.Subjects))
	for position, subject := range index.Subjects {
		if err := validateSubject(subject); err != nil {
			return err
		}
		if position > 0 && !subjectIDLess(index.Subjects[position-1].ID, subject.ID) {
			return fmt.Errorf("group index: subjects are not canonical: %s before %s", index.Subjects[position-1].ID, subject.ID)
		}
		subjectsByID[subject.ID] = subject
	}
	if err := validateSubjectReferences(subjectsByID); err != nil {
		return err
	}
	groupsByID := make(map[string]struct{}, len(index.Groups))
	for position, group := range index.Groups {
		if err := validateGroup(index.Target.ID, subjectsByID, group); err != nil {
			return err
		}
		if group.ID != compactOrdinal("g", position) {
			return fmt.Errorf("group index: groups do not use canonical compact IDs")
		}
		groupsByID[group.ID] = struct{}{}
	}
	if err := validateOffMap(index.OffMap, index.MapFailure, subjectsByID); err != nil {
		return err
	}
	seenContainerGroups := make(map[string]struct{})
	for position, container := range index.Containers {
		if container.ID != compactOrdinal("k", position) || !validContainerID(container.ID) ||
			!validText(container.Title) || !validOptionalText(container.Summary) || !container.Lane.Valid() || len(container.GroupIDs) < 2 {
			return fmt.Errorf("group index: invalid container")
		}
		for groupPosition, groupID := range container.GroupIDs {
			if _, ok := groupsByID[groupID]; !ok || groupPosition > 0 && !compactIDLess(container.GroupIDs[groupPosition-1], groupID, "g") {
				return fmt.Errorf("group index: invalid container group")
			}
			if _, repeated := seenContainerGroups[groupID]; repeated {
				return fmt.Errorf("group index: group belongs to multiple containers")
			}
			seenContainerGroups[groupID] = struct{}{}
		}
	}
	for i, operation := range index.Operations {
		if !validOperationKind(operation.Kind) {
			return fmt.Errorf("group index: invalid operation kind %q", operation.Kind)
		}
		// An operation in a file off the map belongs to no group.
		_, groupExists := groupsByID[operation.GroupID]
		_, subjectExists := subjectsByID[operation.SubjectID]
		if operation.GroupID != "" && !groupExists || operation.SubjectID != "" && !subjectExists || operation.ID != compactOrdinal("o", i) || !validText(operation.Name) || !validOptionalText(operation.Summary) ||
			(operation.Source != "model" && operation.Source != "fact") || operation.Location.Path == "" || operation.Location.Line < 1 || operation.Location.Column < 1 {
			return fmt.Errorf("group index: invalid operation %q", operation.ID)
		}
	}
	if err := index.validateData(subjectsByID); err != nil {
		return err
	}
	if err := index.validateOutbound(subjectsByID, groupsByID); err != nil {
		return err
	}
	for position, edge := range index.StructuralEdges {
		if err := validateStructuralEdge(subjectsByID, edge); err != nil {
			return err
		}
		if position > 0 && structuralEdgeKey(index.StructuralEdges[position-1]) >= structuralEdgeKey(edge) {
			return fmt.Errorf("group index: structural edges are not canonical")
		}
	}
	connectionSlots := make(map[string]struct{}, len(index.Connections))
	for position, connection := range index.Connections {
		if err := validateConnection(index.Target.ID, groupsByID, subjectsByID, connection); err != nil {
			return err
		}
		if connection.ID != compactOrdinal("x", position) {
			return fmt.Errorf("group index: connections do not use canonical compact IDs")
		}
		slot := connectionSlot(connection)
		if _, exists := connectionSlots[slot]; exists {
			return fmt.Errorf("group index: conflicting connection slot")
		}
		connectionSlots[slot] = struct{}{}
	}

	want, err := indexDigest(index)
	if err != nil {
		return err
	}
	if !validSHA256(index.SHA256) || index.SHA256 != want {
		return fmt.Errorf("group index: sha256 mismatch")
	}
	return nil
}

// Empty is the sealed index of a target that was not grouped: the atlas path
// reads the program index directly and needs no groups, while the run
// directory still carries one valid groups-index.json for the readers that
// expect it.
func Empty(program programindex.Index) (Index, error) {
	if err := program.Validate(); err != nil {
		return Index{}, fmt.Errorf("group index: validate program index: %w", err)
	}
	index := Index{
		Version:            Version,
		Target:             program.Target.Snapshot(),
		ProgramIndexSHA256: program.SHA256,
		Subjects:           []Subject{},
		Groups:             []Group{},
		Containers:         []Container{},
		StructuralEdges:    []StructuralEdge{},
		Connections:        []Connection{},
	}
	seal, err := indexDigest(index)
	if err != nil {
		return Index{}, err
	}
	index.SHA256 = seal
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

// Encode validates and returns canonical JSON artifact bytes.
func Encode(index Index) ([]byte, error) {
	if err := index.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(OverlayFromIndex(index))
	if err != nil {
		return nil, fmt.Errorf("group index: encode artifact: %w", err)
	}
	return encoded, nil
}

// Decode strictly decodes one artifact and validates its identities and seal.
func Decode(encoded []byte, program programindex.Index) (Index, error) {
	if err := program.Validate(); err != nil {
		return Index{}, fmt.Errorf("group index: invalid ProgramIndex: %w", err)
	}
	artifact, err := DecodeOverlay(encoded)
	if err != nil {
		return Index{}, err
	}
	return artifact.Hydrate(program)
}

func DecodeOverlay(encoded []byte) (Overlay, error) {
	if len(encoded) == 0 {
		return Overlay{}, fmt.Errorf("group index: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var artifact Overlay
	if err := decoder.Decode(&artifact); err != nil {
		return Overlay{}, fmt.Errorf("group index: decode artifact: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Overlay{}, fmt.Errorf("group index: trailing JSON value")
		}
		return Overlay{}, fmt.Errorf("group index: trailing data: %w", err)
	}
	if err := artifact.Validate(); err != nil {
		return Overlay{}, err
	}
	return artifact, nil
}

// ValidateSet proves every cross-target group endpoint and evidence subject
// against the complete set of GroupsIndexes passed to the matching boundary.
// Each target may occur exactly once.
func ValidateSet(indexes []Index) error {
	groupsByTarget := make(map[string]map[string]struct{}, len(indexes))
	subjectsByTarget := make(map[string]map[string]struct{}, len(indexes))
	roles := make(map[string]string, len(indexes))
	for _, index := range indexes {
		if err := index.Validate(); err != nil {
			return err
		}
		if _, exists := groupsByTarget[index.Target.ID]; exists {
			return fmt.Errorf("group index: duplicate target in set %q", index.Target.ID)
		}
		groups := make(map[string]struct{}, len(index.Groups))
		for _, group := range index.Groups {
			groups[group.ID] = struct{}{}
		}
		groupsByTarget[index.Target.ID] = groups
		subjects := make(map[string]struct{}, len(index.Subjects))
		for _, subject := range index.Subjects {
			subjects[subject.ID] = struct{}{}
		}
		subjectsByTarget[index.Target.ID] = subjects
		roles[index.Target.ID] = index.Role
	}
	for _, index := range indexes {
		seenShared := make(map[string]bool)
		for _, id := range index.SharedCode {
			if id == index.Target.ID || seenShared[id] || roles[id] != "shared_code" {
				return fmt.Errorf("group index: invalid shared code target %q", id)
			}
			seenShared[id] = true
		}
		for _, connection := range index.Connections {
			for _, endpoint := range []Endpoint{connection.From, connection.To} {
				groups, ok := groupsByTarget[endpoint.TargetID]
				if !ok {
					return fmt.Errorf("group index: connection cites target absent from set %q", endpoint.TargetID)
				}
				if _, ok := groups[endpoint.GroupID]; !ok {
					return fmt.Errorf("group index: connection cites group absent from set %q", endpoint.GroupID)
				}
			}
			for _, evidence := range connection.Evidence {
				subjects, ok := subjectsByTarget[evidence.TargetID]
				if !ok {
					return fmt.Errorf("group index: connection evidence cites target absent from set %q", evidence.TargetID)
				}
				if _, ok := subjects[evidence.SubjectID]; !ok {
					return fmt.Errorf("group index: connection evidence cites subject absent from set %q", evidence.SubjectID)
				}
			}
		}
	}
	return nil
}

type subjectAuthority struct {
	categories                  map[programindex.Category]struct{}
	dependencyEvidenceSupported bool
}

func compileSubjects(index programindex.Index) map[string]subjectAuthority {
	result := make(map[string]subjectAuthority, len(index.Objects))
	for _, object := range index.Objects {
		result[object.ID] = subjectAuthority{
			categories:                  make(map[programindex.Category]struct{}),
			dependencyEvidenceSupported: programindex.CategorySupported(index, object.ID, programindex.CategoryDependency),
		}
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			result[pattern.ID] = subjectAuthority{
				categories:                  make(map[programindex.Category]struct{}),
				dependencyEvidenceSupported: programindex.CategorySupported(index, pattern.ID, programindex.CategoryDependency),
			}
		}
	}
	for _, assignment := range index.Categorization.Assignments {
		authority := result[assignment.SubjectID]
		for _, category := range assignment.Categories {
			authority.categories[category] = struct{}{}
		}
		result[assignment.SubjectID] = authority
	}
	return result
}

func compileGroupProposal(targetID string, subjects map[string]subjectAuthority, proposal GroupProposal) (Group, string, string) {
	if !validText(proposal.Key) || !validText(proposal.Title) || !validText(proposal.Summary) || !proposal.Lane.Valid() {
		return Group{}, diagnosticInvalidGroup, "group has an invalid key, title, summary, or lane"
	}
	members, ok := canonicalSubjectIDs(proposal.MemberSubjectIDs)
	if !ok || len(members) == 0 {
		return Group{}, diagnosticInvalidGroup, "group must have canonicalizable direct members"
	}
	evidence, ok := canonicalSubjectIDs(proposal.EvidenceSubjectIDs)
	if !ok {
		return Group{}, diagnosticInvalidGroup, "group has invalid evidence subjects"
	}
	for _, subjectID := range append(cloneStrings(members), evidence...) {
		if _, exists := subjects[subjectID]; !exists {
			return Group{}, diagnosticUnknownSubject, "group cites an unknown ProgramIndex subject"
		}
	}
	if proposal.Lane == LaneDependencies {
		for _, subjectID := range evidence {
			if !subjects[subjectID].dependencyEvidenceSupported {
				return Group{}, diagnosticUnsupportedEvidence, "platform authority cannot evidence a dependencies-lane group"
			}
		}
	}
	for _, subjectID := range members {
		if !subjectSupportsLane(subjects[subjectID], proposal.Lane) {
			return Group{}, diagnosticLaneMismatch, "group member is not categorized for its lane"
		}
	}
	group := Group{
		Title: proposal.Title, Summary: proposal.Summary, Lane: proposal.Lane,
		MemberSubjectIDs: members, EvidenceSubjectIDs: evidence,
	}
	return group, "", ""
}

func compileConnectionProposal(
	targetID string,
	subjects map[string]subjectAuthority,
	groupIDsByKey map[string]string,
	proposal ConnectionProposal,
) (Connection, string, string) {
	if !validText(proposal.FromGroupKey) || !validText(proposal.ToGroupKey) ||
		!validSnakeCase(proposal.SemanticKind) || !validText(proposal.Label) || !validText(proposal.Summary) {
		return Connection{}, diagnosticInvalidConnection, "connection has an invalid endpoint, semantic kind, label, or summary"
	}
	fromID, fromOK := groupIDsByKey[proposal.FromGroupKey]
	toID, toOK := groupIDsByKey[proposal.ToGroupKey]
	if !fromOK || !toOK {
		return Connection{}, diagnosticUnknownGroup, "connection cites an unknown or rejected group key"
	}
	evidence, ok := canonicalSubjectIDs(proposal.EvidenceSubjectIDs)
	if !ok {
		return Connection{}, diagnosticInvalidConnection, "connection has invalid evidence subjects"
	}
	for _, subjectID := range evidence {
		if _, exists := subjects[subjectID]; !exists {
			return Connection{}, diagnosticUnknownSubject, "connection cites an unknown ProgramIndex subject"
		}
	}
	connection := Connection{
		From:         Endpoint{TargetID: targetID, GroupID: fromID},
		To:           Endpoint{TargetID: targetID, GroupID: toID},
		SemanticKind: proposal.SemanticKind, Label: proposal.Label, Summary: proposal.Summary,
		SupportResolution: programindex.PatternValueExact,
		Evidence:          qualifySubjects(targetID, evidence),
	}
	return connection, "", ""
}

func compileConnectionInput(
	groupsByTarget map[string]map[string]struct{},
	subjectsByTarget map[string]map[string]struct{},
	input ConnectionInput,
) (Connection, string, string) {
	if !validTargetID(input.From.TargetID) || !validGroupID(input.From.GroupID) ||
		!validTargetID(input.To.TargetID) || !validGroupID(input.To.GroupID) ||
		!validSnakeCase(input.SemanticKind) || !validText(input.Label) || !validText(input.Summary) ||
		!input.SupportResolution.Valid() {
		return Connection{}, diagnosticInvalidConnection,
			"connection has an invalid endpoint, semantic kind, label, summary, or support resolution"
	}
	fromGroups, fromTargetOK := groupsByTarget[input.From.TargetID]
	toGroups, toTargetOK := groupsByTarget[input.To.TargetID]
	if !fromTargetOK || !toTargetOK {
		return Connection{}, diagnosticUnknownGroup, "connection cites a target absent from the GroupsIndex set"
	}
	if _, ok := fromGroups[input.From.GroupID]; !ok {
		return Connection{}, diagnosticUnknownGroup, "connection cites an unknown source group"
	}
	if _, ok := toGroups[input.To.GroupID]; !ok {
		return Connection{}, diagnosticUnknownGroup, "connection cites an unknown target group"
	}
	evidence, ok := canonicalizeSubjectEndpoints(input.Evidence)
	if !ok {
		return Connection{}, diagnosticInvalidConnection, "connection has invalid evidence subjects"
	}
	for _, endpoint := range evidence {
		subjects, targetOK := subjectsByTarget[endpoint.TargetID]
		if !targetOK {
			return Connection{}, diagnosticUnknownSubject, "connection evidence cites a target absent from the GroupsIndex set"
		}
		if _, ok := subjects[endpoint.SubjectID]; !ok {
			return Connection{}, diagnosticUnknownSubject, "connection evidence cites an unknown subject"
		}
	}
	connection := Connection{
		From: input.From, To: input.To, SemanticKind: input.SemanticKind,
		Label: input.Label, Summary: input.Summary, SupportResolution: input.SupportResolution,
		Evidence: evidence,
	}
	return connection, "", ""
}

func subjectSupportsLane(subject subjectAuthority, lane Lane) bool {
	var categories []programindex.Category
	switch lane {
	case LaneTriggers:
		categories = []programindex.Category{programindex.CategoryInbound, programindex.CategoryBackgroundActivity}
	case LaneCore:
		categories = []programindex.Category{programindex.CategoryCore}
	case LaneDependencies:
		categories = []programindex.Category{programindex.CategoryDependency}
	default:
		return false
	}
	for _, category := range categories {
		if _, ok := subject.categories[category]; ok {
			return true
		}
	}
	return false
}

func compileRetainedSubjects(index programindex.Index, retained map[string]struct{}) []Subject {
	categoriesByID := make(map[string][]programindex.Category)
	if index.Categorization != nil {
		for _, assignment := range index.Categorization.Assignments {
			categoriesByID[assignment.SubjectID] = append([]programindex.Category(nil), assignment.Categories...)
		}
	}
	result := make([]Subject, 0, len(retained))
	for _, object := range index.Objects {
		if _, ok := retained[object.ID]; !ok {
			continue
		}
		categories := categoriesByID[object.ID]
		if categories == nil {
			categories = []programindex.Category{}
		}
		result = append(result, Subject{
			ID: object.ID, Kind: SubjectObject, Categories: categories,
			Object: &ObjectFacts{
				Name: object.Name, Kind: object.Kind, Visibility: object.Visibility,
				Signature: object.Signature, OwnerID: object.OwnerID, ContainerID: object.ContainerID,
				Parameters: append([]programindex.TypedName(nil), object.Parameters...),
				Results:    append([]programindex.TypedName(nil), object.Results...),
				External:   cloneExternal(object.External), Location: cloneLocation(object.Location),
			},
		})
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if _, ok := retained[pattern.ID]; !ok {
				continue
			}
			categories := categoriesByID[pattern.ID]
			if categories == nil {
				categories = []programindex.Category{}
			}
			result = append(result, Subject{
				ID: pattern.ID, Kind: SubjectPattern, Categories: categories,
				Pattern: &PatternFacts{
					Form: pattern.Form, Selector: pattern.Selector, Location: cloneLocation(pattern.Location),
					RelationID: relation.ID, RelationKind: relation.Kind, RelationResolution: relation.Resolution,
					FromID: relation.FromID, ToIDs: cloneStrings(relation.ToIDs), Invocation: relation.Invocation,
					ResultID: pattern.ResultID, ReceiverID: pattern.ReceiverID,
					ReceiverOriginIDs:        cloneStrings(pattern.ReceiverOriginIDs),
					ReceiverOriginResolution: pattern.ReceiverOriginResolution,
					Arguments:                clonePatternArgumentsFromProgram(pattern.Arguments),
				},
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return subjectIDLess(result[i].ID, result[j].ID) })
	if result == nil {
		result = []Subject{}
	}
	return result
}

func compileStructuralEdges(index programindex.Index, retained map[string]struct{}) []StructuralEdge {
	byKey := make(map[string]StructuralEdge)
	type argumentOwner struct {
		patternID string
	}
	argumentOwners := make(map[string]argumentOwner)
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				argumentOwners[argument.ID] = argumentOwner{patternID: pattern.ID}
			}
		}
	}
	appendEdge := func(edge StructuralEdge) {
		if _, fromOK := retained[edge.FromSubjectID]; !fromOK {
			return
		}
		if _, toOK := retained[edge.ToSubjectID]; !toOK {
			return
		}
		byKey[structuralEdgeKey(edge)] = edge
	}
	for _, object := range index.Objects {
		if object.OwnerID != "" {
			appendEdge(StructuralEdge{
				FromSubjectID: object.OwnerID, ToSubjectID: object.ID,
				Role: EdgeObjectOwner, Resolution: programindex.ResolutionExact,
			})
		}
		if object.ContainerID != "" {
			appendEdge(StructuralEdge{
				FromSubjectID: object.ContainerID, ToSubjectID: object.ID,
				Role: EdgeObjectContainer, Resolution: programindex.ResolutionExact,
			})
		}
	}
	for _, relation := range index.Relations {
		for _, targetID := range relation.ToIDs {
			appendEdge(StructuralEdge{
				FromSubjectID: relation.FromID, ToSubjectID: targetID,
				Role: EdgeRelationTarget, RelationID: relation.ID,
				RelationKind: relation.Kind, Resolution: relation.Resolution,
				Location: cloneLocation(relation.Location),
			})
		}
		for _, pattern := range relation.Patterns {
			appendEdge(StructuralEdge{
				FromSubjectID: relation.FromID, ToSubjectID: pattern.ID,
				Role: EdgeRelationPattern, RelationID: relation.ID,
				RelationKind: relation.Kind, Resolution: relation.Resolution,
			})
			for _, targetID := range relation.ToIDs {
				appendEdge(StructuralEdge{
					FromSubjectID: pattern.ID, ToSubjectID: targetID,
					Role: EdgePatternTarget, RelationID: relation.ID,
					RelationKind: relation.Kind, Resolution: relation.Resolution,
				})
			}
			appendEdge(StructuralEdge{
				FromSubjectID: pattern.ID, ToSubjectID: pattern.ResultID,
				Role: EdgePatternResult, RelationID: relation.ID,
				RelationKind: relation.Kind, Resolution: programindex.ResolutionExact,
			})
			appendEdge(StructuralEdge{
				FromSubjectID: pattern.ID, ToSubjectID: pattern.ReceiverID,
				Role: EdgePatternReceiver, RelationID: relation.ID,
				RelationKind: relation.Kind, Resolution: programindex.ResolutionExact,
			})
			for _, objectID := range pattern.ReceiverOriginIDs {
				appendEdge(StructuralEdge{
					FromSubjectID: pattern.ID, ToSubjectID: objectID,
					Role: EdgePatternReceiverOrigin, RelationID: relation.ID, RelationKind: relation.Kind,
					Resolution: pattern.ReceiverOriginResolution,
				})
			}
			for _, argument := range pattern.Arguments {
				for _, objectID := range argument.ObjectIDs {
					appendEdge(StructuralEdge{
						FromSubjectID: pattern.ID, ToSubjectID: objectID,
						Role: EdgePatternArgumentObject, RelationID: relation.ID, RelationKind: relation.Kind,
						Resolution: argument.Resolution,
					})
				}
				for _, candidate := range argument.ValueCandidates {
					for _, objectID := range candidate.SourceObjectIDs {
						appendEdge(StructuralEdge{
							FromSubjectID: objectID, ToSubjectID: pattern.ID,
							Role: EdgePatternValueSourceObject, RelationID: relation.ID, RelationKind: relation.Kind,
							Resolution: programindex.ResolutionUnresolved,
							ArgumentID: argument.ID, ValueCandidateID: candidate.ID,
							ValueResolution: candidate.Resolution, ValueSourceKind: candidate.SourceKind,
						})
					}
					for _, sourceArgumentID := range candidate.SourceArgumentIDs {
						owner, known := argumentOwners[sourceArgumentID]
						if !known {
							continue
						}
						appendEdge(StructuralEdge{
							FromSubjectID: owner.patternID, ToSubjectID: pattern.ID,
							Role: EdgePatternValueSourceArgument, RelationID: relation.ID, RelationKind: relation.Kind,
							Resolution: programindex.ResolutionUnresolved,
							ArgumentID: argument.ID, ValueCandidateID: candidate.ID,
							SourceArgumentID: sourceArgumentID,
							ValueResolution:  candidate.Resolution, ValueSourceKind: candidate.SourceKind,
						})
					}
				}
			}
		}
	}
	keys := slices.Sorted(maps.Keys(byKey))
	result := make([]StructuralEdge, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func validateSubject(subject Subject) error {
	if subject.Interpretation != nil && subject.Interpretation.Line != "" && !validText(subject.Interpretation.Line) {
		return fmt.Errorf("group index: invalid subject interpretation")
	}
	if !validDirectSubjectID(subject.ID) || !subject.Kind.Valid() || subject.Categories == nil ||
		!canonicalCategories(subject.Categories) {
		return fmt.Errorf("group index: invalid subject")
	}
	switch subject.Kind {
	case SubjectObject:
		if subject.Object == nil || subject.Pattern != nil || !validObjectFacts(*subject.Object) {
			return fmt.Errorf("group index: invalid object subject")
		}
	case SubjectPattern:
		if subject.Object != nil || subject.Pattern == nil || !validPatternFacts(subject.ID, *subject.Pattern) {
			return fmt.Errorf("group index: invalid pattern subject")
		}
	}
	return nil
}

func validateGroup(targetID string, subjects map[string]Subject, group Group) error {
	if !validGroupID(group.ID) || !validText(group.Title) || !validOptionalText(group.Summary) || !group.Lane.Valid() ||
		len(group.MemberSubjectIDs) == 0 || !canonicalDirectSubjectIDs(group.MemberSubjectIDs) ||
		group.EvidenceSubjectIDs == nil || !canonicalDirectSubjectIDs(group.EvidenceSubjectIDs) {
		return fmt.Errorf("group index: invalid group")
	}
	for _, subjectID := range group.MemberSubjectIDs {
		subject, ok := subjects[subjectID]
		if !ok {
			return fmt.Errorf("group index: group has invalid member subject")
		}
		if !categoriesSupportLane(subject.Categories, group.Lane) {
			return fmt.Errorf("group index: group member is not categorized for its lane")
		}
	}
	for _, subjectID := range group.EvidenceSubjectIDs {
		if _, ok := subjects[subjectID]; !ok {
			return fmt.Errorf("group index: group has unknown evidence subject")
		}
		if group.Lane == LaneDependencies && !subjectSupportsDependencyEvidence(subjectID, subjects) {
			return fmt.Errorf("group index: platform authority cannot evidence a dependencies-lane group")
		}
	}
	return nil
}

func subjectSupportsDependencyEvidence(subjectID string, subjects map[string]Subject) bool {
	subject, ok := subjects[subjectID]
	if !ok {
		return true
	}
	if subject.Object != nil {
		return subject.Object.Kind != programindex.ObjectExternalSymbol ||
			!programindex.IsExternalPlatformAuthority(subject.Object.External)
	}
	if subject.Pattern == nil || subject.Pattern.RelationKind != programindex.RelationInvokesExternal ||
		subject.Pattern.RelationResolution != programindex.ResolutionExact || len(subject.Pattern.ToIDs) == 0 {
		return true
	}
	sawPlatformTarget := false
	for _, targetID := range subject.Pattern.ToIDs {
		target, known := subjects[targetID]
		if !known || target.Object == nil || target.Object.Kind != programindex.ObjectExternalSymbol ||
			!programindex.IsExternalPlatformAuthority(target.Object.External) {
			return true
		}
		sawPlatformTarget = true
	}
	return !sawPlatformTarget
}

func validateConnection(
	localTargetID string,
	localGroups map[string]struct{},
	subjects map[string]Subject,
	connection Connection,
) error {
	if connection.FromSubjectID != "" {
		if _, ok := subjects[connection.FromSubjectID]; !ok {
			return fmt.Errorf("group index: unknown connection source subject")
		}
	}
	if connection.To.TargetID == localTargetID && connection.ToSubjectID != "" {
		if _, ok := subjects[connection.ToSubjectID]; !ok {
			return fmt.Errorf("group index: unknown connection target subject")
		}
	}
	for _, location := range []*programindex.Location{connection.FromLocation, connection.ToLocation} {
		if location != nil && (location.Path == "" || location.Line < 1 || location.Column < 1) {
			return fmt.Errorf("group index: invalid connection location")
		}
	}
	if !validConnectionID(connection.ID) || !validTargetID(connection.From.TargetID) || !validGroupID(connection.From.GroupID) ||
		!validTargetID(connection.To.TargetID) || !validGroupID(connection.To.GroupID) ||
		!validSnakeCase(connection.SemanticKind) || !validText(connection.Label) || !validText(connection.Summary) ||
		!connection.SupportResolution.Valid() || connection.Evidence == nil ||
		!canonicalSubjectEndpoints(connection.Evidence) {
		return fmt.Errorf("group index: invalid connection")
	}
	if connection.From.TargetID != localTargetID {
		return fmt.Errorf("group index: connection is not stored by its source target")
	}
	if _, ok := localGroups[connection.From.GroupID]; !ok {
		return fmt.Errorf("group index: connection has unknown local source group")
	}
	if connection.To.TargetID == localTargetID {
		if _, ok := localGroups[connection.To.GroupID]; !ok {
			return fmt.Errorf("group index: connection has unknown local target group")
		}
	}
	for _, evidence := range connection.Evidence {
		if evidence.TargetID != localTargetID {
			continue
		}
		if _, ok := subjects[evidence.SubjectID]; !ok {
			return fmt.Errorf("group index: connection has unknown evidence subject")
		}
	}
	return nil
}

func validateStructuralEdge(subjects map[string]Subject, edge StructuralEdge) error {
	from, ok := subjects[edge.FromSubjectID]
	if !ok {
		return fmt.Errorf("group index: structural edge has unknown source subject")
	}
	to, ok := subjects[edge.ToSubjectID]
	if !ok {
		return fmt.Errorf("group index: structural edge has unknown target subject")
	}
	if !edge.Role.Valid() || !edge.Resolution.Valid() {
		return fmt.Errorf("group index: invalid structural edge")
	}
	if !validOptionalLocation(edge.Location) || edge.Location != nil && edge.Role != EdgeRelationTarget {
		return fmt.Errorf("group index: invalid structural edge location")
	}
	switch edge.Role {
	case EdgeObjectOwner:
		if edge.hasValueSourceEvidence() || edge.RelationID != "" || edge.RelationKind != "" || edge.Resolution != programindex.ResolutionExact ||
			from.Kind != SubjectObject || to.Kind != SubjectObject || to.Object.OwnerID != from.ID {
			return fmt.Errorf("group index: object-owner edge authority mismatch")
		}
	case EdgeObjectContainer:
		if edge.hasValueSourceEvidence() || edge.RelationID != "" || edge.RelationKind != "" || edge.Resolution != programindex.ResolutionExact ||
			from.Kind != SubjectObject || to.Kind != SubjectObject || to.Object.ContainerID != from.ID {
			return fmt.Errorf("group index: object-container edge authority mismatch")
		}
	case EdgeRelationTarget:
		if !validRelationEdgeAuthority(edge) || from.Kind != SubjectObject || to.Kind != SubjectObject {
			return fmt.Errorf("group index: invalid relation-target edge endpoints")
		}
	case EdgeRelationPattern:
		if !validRelationEdgeAuthority(edge) || from.Kind != SubjectObject || to.Kind != SubjectPattern || to.Pattern.FromID != from.ID ||
			to.Pattern.RelationID != edge.RelationID || to.Pattern.RelationKind != edge.RelationKind ||
			to.Pattern.RelationResolution != edge.Resolution {
			return fmt.Errorf("group index: relation-pattern edge authority mismatch")
		}
	case EdgePatternTarget:
		if !validRelationEdgeAuthority(edge) || from.Kind != SubjectPattern || to.Kind != SubjectObject ||
			!containsString(from.Pattern.ToIDs, to.ID) || !patternEdgeMatches(from, to, edge, to.ID, from.Pattern.RelationResolution) {
			return fmt.Errorf("group index: pattern-target edge authority mismatch")
		}
	case EdgePatternResult:
		if !validRelationEdgeAuthority(edge) || from.Kind != SubjectPattern || from.Pattern == nil ||
			!patternEdgeMatches(from, to, edge, from.Pattern.ResultID, programindex.ResolutionExact) {
			return fmt.Errorf("group index: pattern-result edge authority mismatch")
		}
	case EdgePatternReceiver:
		if !validRelationEdgeAuthority(edge) || from.Kind != SubjectPattern || from.Pattern == nil ||
			!patternEdgeMatches(from, to, edge, from.Pattern.ReceiverID, programindex.ResolutionExact) {
			return fmt.Errorf("group index: pattern-receiver edge authority mismatch")
		}
	case EdgePatternReceiverOrigin:
		if !validRelationEdgeAuthority(edge) || from.Kind != SubjectPattern || to.Kind != SubjectObject ||
			!containsString(from.Pattern.ReceiverOriginIDs, to.ID) ||
			!patternEdgeMatches(from, to, edge, to.ID, from.Pattern.ReceiverOriginResolution) {
			return fmt.Errorf("group index: pattern-receiver-origin edge authority mismatch")
		}
	case EdgePatternArgumentObject:
		if !validRelationEdgeAuthority(edge) || from.Kind != SubjectPattern || to.Kind != SubjectObject || !patternArgumentEdgeMatches(from, to, edge) {
			return fmt.Errorf("group index: pattern-argument edge authority mismatch")
		}
	case EdgePatternValueSourceObject:
		if !validValueSourceEdgeAuthority(edge) || from.Kind != SubjectObject || to.Kind != SubjectPattern ||
			!patternValueSourceObjectEdgeMatches(from, to, edge) {
			return fmt.Errorf("group index: pattern-value source-object edge authority mismatch")
		}
	case EdgePatternValueSourceArgument:
		if !validValueSourceEdgeAuthority(edge) || from.Kind != SubjectPattern || to.Kind != SubjectPattern ||
			!patternValueSourceArgumentEdgeMatches(from, to, edge) {
			return fmt.Errorf("group index: pattern-value source-argument edge authority mismatch")
		}
	}
	return nil
}

func validRelationEdgeAuthority(edge StructuralEdge) bool {
	return validRelationID(edge.RelationID) && edge.RelationKind.Valid() && !edge.hasValueSourceEvidence()
}

func (edge StructuralEdge) hasValueSourceEvidence() bool {
	return edge.ArgumentID != "" || edge.ValueCandidateID != "" || edge.SourceArgumentID != "" ||
		edge.ValueResolution != "" || edge.ValueSourceKind != ""
}

func validValueSourceEdgeAuthority(edge StructuralEdge) bool {
	return validRelationID(edge.RelationID) && edge.RelationKind.Valid() &&
		edge.Resolution == programindex.ResolutionUnresolved &&
		validPatternArgumentID(edge.ArgumentID) &&
		validPatternValueID(edge.ValueCandidateID) &&
		edge.ValueResolution.Valid() && edge.ValueSourceKind.Valid()
}

func validateSubjectReferences(subjects map[string]Subject) error {
	for _, subject := range subjects {
		switch subject.Kind {
		case SubjectObject:
			for _, objectID := range []string{subject.Object.OwnerID, subject.Object.ContainerID} {
				if objectID != "" && !isObjectSubject(subjects[objectID]) {
					return fmt.Errorf("group index: object subject has unknown structural reference")
				}
			}
		case SubjectPattern:
			objectIDs := []string{subject.Pattern.FromID, subject.Pattern.ResultID, subject.Pattern.ReceiverID}
			objectIDs = append(objectIDs, subject.Pattern.ToIDs...)
			objectIDs = append(objectIDs, subject.Pattern.ReceiverOriginIDs...)
			for _, argument := range subject.Pattern.Arguments {
				objectIDs = append(objectIDs, argument.ObjectIDs...)
				for _, candidate := range argument.ValueCandidates {
					objectIDs = append(objectIDs, candidate.SourceObjectIDs...)
				}
			}
			for _, objectID := range objectIDs {
				if objectID != "" && !isObjectSubject(subjects[objectID]) {
					return fmt.Errorf("group index: pattern subject has unknown object reference")
				}
			}
		}
	}
	arguments := make(map[string]struct {
		owner Subject
		value PatternArgument
	})
	for _, subject := range subjects {
		if subject.Kind != SubjectPattern {
			continue
		}
		for _, argument := range subject.Pattern.Arguments {
			if _, duplicate := arguments[argument.ID]; duplicate {
				return fmt.Errorf("group index: duplicate pattern argument identity")
			}
			arguments[argument.ID] = struct {
				owner Subject
				value PatternArgument
			}{owner: subject, value: argument}
		}
	}
	for _, subject := range subjects {
		if subject.Kind != SubjectPattern {
			continue
		}
		for _, argument := range subject.Pattern.Arguments {
			for _, candidate := range argument.ValueCandidates {
				for _, sourceArgumentID := range candidate.SourceArgumentIDs {
					source, known := arguments[sourceArgumentID]
					if !known || sourceArgumentID == argument.ID ||
						source.owner.Pattern.RelationResolution != programindex.ResolutionExact ||
						len(source.owner.Pattern.ToIDs) != 1 || !samePatternValue(candidate, source.value) {
						return fmt.Errorf("group index: pattern value candidate has unknown or incompatible source argument")
					}
				}
			}
		}
	}
	return nil
}

func patternEdgeMatches(from, to Subject, edge StructuralEdge, targetID string, resolution programindex.Resolution) bool {
	return from.Kind == SubjectPattern && to.Kind == SubjectObject && targetID == to.ID &&
		from.Pattern.RelationID == edge.RelationID && from.Pattern.RelationKind == edge.RelationKind &&
		edge.Resolution == resolution
}

func patternArgumentEdgeMatches(from, to Subject, edge StructuralEdge) bool {
	for _, argument := range from.Pattern.Arguments {
		if containsString(argument.ObjectIDs, to.ID) && argument.Resolution == edge.Resolution &&
			from.Pattern.RelationID == edge.RelationID && from.Pattern.RelationKind == edge.RelationKind {
			return true
		}
	}
	return false
}

func patternValueSourceObjectEdgeMatches(from, to Subject, edge StructuralEdge) bool {
	argument, candidate, ok := patternValueCandidateByID(to, edge.ArgumentID, edge.ValueCandidateID)
	return ok && edge.SourceArgumentID == "" && candidate.SourceKind == programindex.PatternValueSourceInitializer &&
		candidate.SourceKind == edge.ValueSourceKind && candidate.Resolution == edge.ValueResolution &&
		containsString(candidate.SourceObjectIDs, from.ID) &&
		to.Pattern.RelationID == edge.RelationID && to.Pattern.RelationKind == edge.RelationKind && argument.ID == edge.ArgumentID
}

func patternValueSourceArgumentEdgeMatches(from, to Subject, edge StructuralEdge) bool {
	_, candidate, ok := patternValueCandidateByID(to, edge.ArgumentID, edge.ValueCandidateID)
	if !ok || candidate.SourceKind != programindex.PatternValueSourceActualArgument ||
		candidate.SourceKind != edge.ValueSourceKind || candidate.Resolution != edge.ValueResolution ||
		!containsString(candidate.SourceArgumentIDs, edge.SourceArgumentID) ||
		to.Pattern.RelationID != edge.RelationID || to.Pattern.RelationKind != edge.RelationKind {
		return false
	}
	for _, argument := range from.Pattern.Arguments {
		if argument.ID == edge.SourceArgumentID {
			return samePatternValue(candidate, argument)
		}
	}
	return false
}

func patternValueCandidateByID(subject Subject, argumentID, candidateID string) (PatternArgument, PatternValueCandidate, bool) {
	if subject.Kind != SubjectPattern || subject.Pattern == nil {
		return PatternArgument{}, PatternValueCandidate{}, false
	}
	for _, argument := range subject.Pattern.Arguments {
		if argument.ID != argumentID {
			continue
		}
		for _, candidate := range argument.ValueCandidates {
			if candidate.ID == candidateID {
				return argument, candidate, true
			}
		}
	}
	return PatternArgument{}, PatternValueCandidate{}, false
}

func isObjectSubject(subject Subject) bool {
	return subject.Kind == SubjectObject && subject.Object != nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validObjectFacts(facts ObjectFacts) bool {
	if !validText(facts.Name) || !facts.Kind.Valid() || !facts.Visibility.Valid() ||
		!validOptionalText(facts.Signature) || !validOptionalDirectObjectID(facts.OwnerID) ||
		!validOptionalDirectObjectID(facts.ContainerID) || !validOptionalLocation(facts.Location) {
		return false
	}
	if facts.External == nil {
		return true
	}
	return facts.Kind == programindex.ObjectExternalSymbol && facts.External.AuthorityKind.Valid() &&
		validText(facts.External.PackagePath) &&
		validOptionalText(facts.External.Receiver) && validText(facts.External.Name) &&
		(facts.External.RepositoryPath == "" || (facts.External.AuthorityKind == programindex.ExternalAuthorityPackage &&
			validText(facts.External.RepositoryPath) && !strings.Contains(facts.External.RepositoryPath, "\\") && fs.ValidPath(facts.External.RepositoryPath)))
}

func validPatternFacts(patternID string, facts PatternFacts) bool {
	if !facts.Form.Valid() || !validText(facts.Selector) || !validOptionalLocation(facts.Location) ||
		!validRelationID(facts.RelationID) || !facts.RelationKind.Valid() || !facts.RelationResolution.Valid() ||
		!validCompactID(facts.FromID, "n") || facts.ToIDs == nil ||
		!canonicalDirectObjectIDs(facts.ToIDs) || !validOptionalText(facts.Invocation) ||
		!validOptionalDirectObjectID(facts.ResultID) || !validOptionalDirectObjectID(facts.ReceiverID) ||
		facts.ReceiverOriginIDs == nil || !canonicalDirectObjectIDs(facts.ReceiverOriginIDs) || facts.Arguments == nil {
		return false
	}
	if len(facts.ReceiverOriginIDs) == 0 {
		if facts.ReceiverOriginResolution != "" && facts.ReceiverOriginResolution != programindex.ResolutionUnresolved {
			return false
		}
	} else if !facts.ReceiverOriginResolution.Valid() || facts.ReceiverOriginResolution == programindex.ResolutionUnresolved {
		return false
	}
	for position, argument := range facts.Arguments {
		if !validPatternArgument(patternID, argument) {
			return false
		}
		if position > 0 && comparePatternArguments(facts.Arguments[position-1], argument) >= 0 {
			return false
		}
	}
	return true
}

func validPatternArgument(patternID string, argument PatternArgument) bool {
	if !validPatternArgumentID(argument.ID) ||
		!validPatternArgumentSelector(argument.Position, argument.Keyword) || !argument.Kind.Valid() ||
		argument.Parts == nil || argument.ObjectIDs == nil || argument.ValueCandidates == nil ||
		!canonicalDirectObjectIDs(argument.ObjectIDs) ||
		!validPatternArgumentObjectAuthority(argument.ObjectIDs, argument.Resolution, argument.ObjectsObserved, argument.ObjectsOmitted) ||
		argument.ValueCandidatesObserved != len(argument.ValueCandidates) ||
		argument.ValueCandidatesOmitted != 0 {
		return false
	}
	if !strings.HasPrefix(argument.ID, patternID+"a") {
		return false
	}
	if argument.Kind != programindex.PatternDynamic && len(argument.ValueCandidates) != 0 {
		return false
	}
	for position, candidate := range argument.ValueCandidates {
		if !validPatternValueCandidate(argument.ID, candidate) ||
			position > 0 && argument.ValueCandidates[position-1].ID >= candidate.ID {
			return false
		}
	}
	switch argument.Kind {
	case programindex.PatternLiteralString:
		return utf8.ValidString(argument.Value) && len(argument.Parts) == 0
	case programindex.PatternStringTemplate:
		if argument.Value != "" || len(argument.Parts) == 0 {
			return false
		}
		hasHole := false
		previousLiteral := false
		for _, part := range argument.Parts {
			if !part.Kind.Valid() || part.Kind == programindex.PatternPartHole && part.Text != "" ||
				part.Kind == programindex.PatternPartLiteral && (part.Text == "" || !utf8.ValidString(part.Text) || previousLiteral) {
				return false
			}
			hasHole = hasHole || part.Kind == programindex.PatternPartHole
			previousLiteral = part.Kind == programindex.PatternPartLiteral
		}
		return hasHole
	case programindex.PatternDynamic:
		return argument.Value == "" && len(argument.Parts) == 0
	default:
		return false
	}
}

func validPatternArgumentObjectAuthority(ids []string, resolution programindex.Resolution, observed, omitted int) bool {
	if observed < 0 || observed < len(ids) || omitted != observed-len(ids) {
		return false
	}
	if resolution == "" {
		return observed == 0 && len(ids) == 0 && omitted == 0
	}
	if !resolution.Valid() || observed == 0 {
		return false
	}
	switch resolution {
	case programindex.ResolutionExact:
		return len(ids) == 1 && omitted == 0
	case programindex.ResolutionAlternatives:
		return len(ids) > 0
	case programindex.ResolutionUnresolved:
		return len(ids) == 0
	default:
		return false
	}
}

func validPatternValueCandidate(argumentID string, candidate PatternValueCandidate) bool {
	if !validPatternValueID(candidate.ID) ||
		!candidate.Kind.Valid() || candidate.Kind == programindex.PatternDynamic ||
		!candidate.Resolution.Valid() || !candidate.SourceKind.Valid() || candidate.Parts == nil ||
		candidate.SourceObjectIDs == nil || candidate.SourceArgumentIDs == nil ||
		!canonicalDirectObjectIDs(candidate.SourceObjectIDs) || !canonicalPatternArgumentIDs(candidate.SourceArgumentIDs) ||
		candidate.SourceObjectsObserved != len(candidate.SourceObjectIDs) || candidate.SourceObjectsOmitted != 0 ||
		candidate.SourceArgumentsObserved != len(candidate.SourceArgumentIDs) || candidate.SourceArgumentsOmitted != 0 {
		return false
	}
	switch candidate.SourceKind {
	case programindex.PatternValueSourceInitializer:
		if len(candidate.SourceObjectIDs) == 0 || len(candidate.SourceArgumentIDs) != 0 || candidate.SourceArgumentsObserved != 0 {
			return false
		}
	case programindex.PatternValueSourceActualArgument:
		if len(candidate.SourceObjectIDs) != 0 || candidate.SourceObjectsObserved != 0 ||
			len(candidate.SourceArgumentIDs) != 1 || candidate.SourceArgumentsObserved != 1 ||
			candidate.Resolution != programindex.PatternValuePossible {
			return false
		}
	}
	if !strings.HasPrefix(candidate.ID, argumentID+"v") {
		return false
	}
	switch candidate.Kind {
	case programindex.PatternLiteralString:
		return utf8.ValidString(candidate.Value) && len(candidate.Parts) == 0
	case programindex.PatternStringTemplate:
		if candidate.Value != "" || len(candidate.Parts) == 0 {
			return false
		}
		hasHole := false
		previousLiteral := false
		for _, part := range candidate.Parts {
			if !part.Kind.Valid() || part.Kind == programindex.PatternPartHole && part.Text != "" ||
				part.Kind == programindex.PatternPartLiteral && (part.Text == "" || !utf8.ValidString(part.Text) || previousLiteral) {
				return false
			}
			hasHole = hasHole || part.Kind == programindex.PatternPartHole
			previousLiteral = part.Kind == programindex.PatternPartLiteral
		}
		return hasHole
	default:
		return false
	}
}

func samePatternValue(candidate PatternValueCandidate, argument PatternArgument) bool {
	return candidate.Kind == argument.Kind && candidate.Value == argument.Value && reflect.DeepEqual(candidate.Parts, argument.Parts)
}

func canonicalCategories(values []programindex.Category) bool {
	for position, category := range values {
		if !category.Valid() || position > 0 && values[position-1] >= category {
			return false
		}
	}
	return true
}

func categoriesSupportLane(categories []programindex.Category, lane Lane) bool {
	authority := subjectAuthority{categories: make(map[programindex.Category]struct{}, len(categories))}
	for _, category := range categories {
		authority.categories[category] = struct{}{}
	}
	return subjectSupportsLane(authority, lane)
}

func canonicalDirectObjectIDs(values []string) bool {
	for position, value := range values {
		if !validCompactID(value, "n") || position > 0 && values[position-1] >= value {
			return false
		}
	}
	return true
}

func canonicalPatternArgumentIDs(values []string) bool {
	for position, value := range values {
		if !validPatternArgumentID(value) || position > 0 && values[position-1] >= value {
			return false
		}
	}
	return true
}

func validOptionalDirectObjectID(value string) bool {
	return value == "" || validCompactID(value, "n")
}

func validPatternArgumentSelector(position int, keyword string) bool {
	return position > 0 && keyword == "" || position == 0 && validText(keyword)
}

func patternArgumentKey(argument PatternArgument) string {
	if argument.Position > 0 {
		return "position:" + strconv.Itoa(argument.Position)
	}
	return "keyword:" + argument.Keyword
}

func comparePatternArguments(left, right PatternArgument) int {
	if left.Position > 0 && right.Position == 0 {
		return -1
	}
	if left.Position == 0 && right.Position > 0 {
		return 1
	}
	if left.Position > 0 {
		if left.Position < right.Position {
			return -1
		}
		if left.Position > right.Position {
			return 1
		}
		return 0
	}
	return strings.Compare(left.Keyword, right.Keyword)
}

func structuralEdgeKey(edge StructuralEdge) string {
	return strings.Join([]string{
		edge.FromSubjectID, edge.ToSubjectID, string(edge.Role), edge.RelationID,
		string(edge.RelationKind), string(edge.Resolution), edge.ArgumentID, edge.ValueCandidateID,
		edge.SourceArgumentID, string(edge.ValueResolution), string(edge.ValueSourceKind),
	}, "\x00")
}

func patternValueCandidateIdentity(argumentID string, value PatternValueCandidate) string {
	fields := []string{
		argumentID, string(value.Kind), value.Value, string(value.Resolution), string(value.SourceKind),
	}
	for _, part := range value.Parts {
		fields = append(fields, string(part.Kind), part.Text)
	}
	fields = append(fields, "source-objects")
	fields = append(fields, value.SourceObjectIDs...)
	fields = append(fields, "source-arguments")
	fields = append(fields, value.SourceArgumentIDs...)
	return stableID("program-pattern-value", fields...)
}

// compileContainers resolves the group keys a container names. A container
// that ends up holding fewer than two groups is not a level, it is the group
// itself under another name, and is dropped.
func compileContainers(
	targetID string,
	groupIDsByKey map[string]string,
	proposals []ContainerProposal,
) ([]Container, []Diagnostic) {
	diagnostics := make([]Diagnostic, 0)
	seen := make(map[string]struct{})
	result := make([]Container, 0, len(proposals))
	for _, proposal := range proposals {
		if strings.TrimSpace(proposal.Title) == "" || !proposal.Lane.Valid() {
			diagnostics = append(diagnostics, Diagnostic{
				Kind: diagnosticContainerIncomplete, ProposalKey: proposal.Key,
				Reason: "a container needs a title and a valid lane",
			})
			continue
		}
		ids := make([]string, 0, len(proposal.GroupKeys))
		for _, key := range proposal.GroupKeys {
			id, known := groupIDsByKey[key]
			if !known {
				diagnostics = append(diagnostics, Diagnostic{
					Kind: diagnosticContainerUnknownGroup, ProposalKey: proposal.Key, Reason: key,
				})
				continue
			}
			if _, repeated := seen[id]; repeated {
				diagnostics = append(diagnostics, Diagnostic{
					Kind: diagnosticContainerRepeatedGroup, ProposalKey: proposal.Key, Reason: id,
				})
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		if len(ids) < 2 {
			// A part holding one group is that group under a second name.
			for _, id := range ids {
				delete(seen, id)
			}
			diagnostics = append(diagnostics, Diagnostic{
				Kind: diagnosticContainerTooSmall, ProposalKey: proposal.Key,
				Reason: proposal.Title,
			})
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j], "g") })
		container := Container{
			Title: strings.TrimSpace(proposal.Title), Summary: strings.TrimSpace(proposal.Summary),
			Lane: proposal.Lane, GroupIDs: ids,
		}
		result = append(result, container)
	}
	sort.Slice(result, func(i, j int) bool { return containerKey(result[i]) < containerKey(result[j]) })
	for position := range result {
		result[position].ID = compactOrdinal("k", position)
	}
	return result, diagnostics
}

func groupKey(group Group) string {
	fields := []string{string(group.Lane), group.Title, group.Summary, "members", strconv.Itoa(len(group.MemberSubjectIDs))}
	fields = append(fields, group.MemberSubjectIDs...)
	fields = append(fields, "evidence", strconv.Itoa(len(group.EvidenceSubjectIDs)))
	fields = append(fields, group.EvidenceSubjectIDs...)
	return strings.Join(fields, "\x00")
}

func containerKey(container Container) string {
	fields := []string{string(container.Lane), container.Title, container.Summary, strconv.Itoa(len(container.GroupIDs))}
	fields = append(fields, container.GroupIDs...)
	return strings.Join(fields, "\x00")
}

func connectionSlot(connection Connection) string {
	values := []string{
		connection.From.TargetID, connection.From.GroupID,
		connection.To.TargetID, connection.To.GroupID,
		connection.SemanticKind, connection.SourceKind, connection.SourceID,
	}
	for _, location := range []*programindex.Location{connection.FromLocation, connection.ToLocation} {
		if location != nil {
			values = append(values, location.Path, strconv.Itoa(location.Line), strconv.Itoa(location.Column))
		} else {
			values = append(values, "")
		}
	}
	return strings.Join(values, "\x00")
}

func connectionKey(connection Connection) string {
	values := []string{
		connectionSlot(connection), string(connection.SupportResolution),
		connection.Label, connection.Summary, strconv.Itoa(len(connection.Evidence)),
		connection.SourceKind,
		connection.FromSubjectID, connection.ToSubjectID,
	}
	for _, location := range []*programindex.Location{connection.FromLocation, connection.ToLocation} {
		if location != nil {
			values = append(values, location.Path, strconv.Itoa(location.Line), strconv.Itoa(location.Column))
		} else {
			values = append(values, "")
		}
	}
	for _, evidence := range connection.Evidence {
		values = append(values, evidence.TargetID, evidence.SubjectID)
	}
	return strings.Join(values, "\x00")
}

func assignConnectionIDs(connections []Connection, offset int) {
	for position := range connections {
		connections[position].ID = compactOrdinal("x", offset+position)
	}
}

func connectionProposalKey(proposal ConnectionProposal) string {
	return proposal.FromGroupKey + "->" + proposal.ToGroupKey + ":" + proposal.SemanticKind
}

func connectionInputKey(input ConnectionInput) string {
	return strings.Join([]string{
		input.From.TargetID, input.From.GroupID, "->", input.To.TargetID, input.To.GroupID, input.SemanticKind,
		string(input.SupportResolution),
	}, ":")
}

func canonicalSubjectIDs(values []string) ([]string, bool) {
	result := cloneStrings(values)
	for _, value := range result {
		if !validDirectSubjectID(value) {
			return nil, false
		}
	}
	sort.Slice(result, func(i, j int) bool { return subjectIDLess(result[i], result[j]) })
	result = compactStrings(result)
	if result == nil {
		result = []string{}
	}
	return result, true
}

func canonicalDirectSubjectIDs(values []string) bool {
	for position, value := range values {
		if !validDirectSubjectID(value) || position > 0 && !subjectIDLess(values[position-1], value) {
			return false
		}
	}
	return true
}

func canonicalDiagnostics(values []Diagnostic) []Diagnostic {
	sort.Slice(values, func(i, j int) bool {
		return diagnosticKey(values[i]) < diagnosticKey(values[j])
	})
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || !reflect.DeepEqual(result[len(result)-1], value) {
			result = append(result, value)
		}
	}
	if result == nil {
		result = []Diagnostic{}
	}
	return result
}

func diagnosticKey(value Diagnostic) string {
	return value.Kind + "\x00" + value.ProposalKey + "\x00" + value.Reason
}

// Overlay is the persisted semantic layer. It binds to one complete
// ProgramIndex and never repeats that index's native facts or derived edges.
type Overlay struct {
	Version            int                 `json:"version"`
	TargetID           string              `json:"target_id"`
	ProgramIndexSHA256 string              `json:"program_index_sha256"`
	Role               string              `json:"role,omitempty"`
	SharedCode         []string            `json:"shared_code,omitempty"`
	Summary            string              `json:"summary,omitempty"`
	Data               []DataRecord        `json:"data,omitempty"`
	Subjects           []SubjectAnnotation `json:"subjects"`
	Groups             []Group             `json:"groups"`
	Operations         []Operation         `json:"operations,omitempty"`
	Outbound           []OutboundCall      `json:"outbound,omitempty"`
	Containers         []Container         `json:"containers"`
	Connections        []Connection        `json:"connections"`
	OffMap             []OffMapFile        `json:"off_map,omitempty"`
	MapFailure         string              `json:"map_failure,omitempty"`
	SHA256             string              `json:"sha256"`
}

// validateOffMap checks the off-map record: known reasons, repository paths,
// canonical order, and a map failure only with every file off for it.
func validateOffMap(files []OffMapFile, failure string, subjects map[string]Subject) error {
	if !validOptionalText(failure) {
		return fmt.Errorf("group index: invalid map failure")
	}
	for position, file := range files {
		switch file.Reason {
		case "left_out", "conflict", "no_units", "map_failure", OffMapTests, OffMapUndecided, OffMapUnreachable:
		default:
			return fmt.Errorf("group index: invalid off-map reason %q", file.Reason)
		}
		named := file.Reason == OffMapTests || file.Reason == OffMapUnreachable
		listed := file.Reason == OffMapUndecided || file.Reason == OffMapUnreachable
		either := file.Reason == "left_out" || file.Reason == "conflict"
		if !validText(file.Path) || strings.HasPrefix(file.Path, "/") || !validOptionalText(file.Part) || (file.Part != "") != named ||
			(len(file.SubjectIDs) > 0) != listed && !either {
			return fmt.Errorf("group index: invalid off-map file %q", file.Path)
		}
		for _, id := range file.SubjectIDs {
			if _, known := subjects[id]; !known {
				return fmt.Errorf("group index: off-map file %q names an unknown subject %q", file.Path, id)
			}
		}
		if failure != "" && file.Reason != "map_failure" && !named || failure == "" && file.Reason == "map_failure" {
			return fmt.Errorf("group index: off-map file %q disagrees with the map failure", file.Path)
		}
		if position > 0 && offMapKey(files[position-1]) >= offMapKey(file) {
			return fmt.Errorf("group index: off-map files are not canonical")
		}
	}
	return nil
}

func offMapKey(file OffMapFile) string {
	return file.Path + "\x00" + file.Reason + "\x00" + file.Part + "\x00" + strings.Join(file.SubjectIDs, "\x00")
}

func OverlayFromIndex(index Index) Overlay {
	subjects := make([]SubjectAnnotation, len(index.Subjects))
	for position, subject := range index.Subjects {
		categories := make([]programindex.Category, len(subject.Categories))
		copy(categories, subject.Categories)
		subjects[position] = SubjectAnnotation{
			ID: subject.ID, Categories: categories,
		}
		if subject.Interpretation != nil {
			interpretation := *subject.Interpretation
			subjects[position].Interpretation = &interpretation
		}
	}
	return Overlay{
		Version: index.Version, TargetID: index.Target.ID, ProgramIndexSHA256: index.ProgramIndexSHA256,
		Role: index.Role, SharedCode: index.SharedCode, Summary: index.Summary, Data: index.Data,
		Subjects: subjects, Groups: index.Groups, Operations: index.Operations, Outbound: index.Outbound,
		Containers: index.Containers, Connections: index.Connections, OffMap: index.OffMap, MapFailure: index.MapFailure, SHA256: index.SHA256,
	}
}

func (artifact Overlay) Validate() error {
	if artifact.Version != Version || !validTargetID(artifact.TargetID) || !validSHA256(artifact.ProgramIndexSHA256) ||
		artifact.Subjects == nil || artifact.Groups == nil || artifact.Containers == nil || artifact.Connections == nil {
		return fmt.Errorf("group index: invalid semantic overlay")
	}
	seen := make(map[string]struct{}, len(artifact.Subjects))
	for _, subject := range artifact.Subjects {
		if !validDirectSubjectID(subject.ID) || !canonicalCategories(subject.Categories) {
			return fmt.Errorf("group index: invalid subject annotation")
		}
		if _, exists := seen[subject.ID]; exists {
			return fmt.Errorf("group index: duplicate subject annotation")
		}
		seen[subject.ID] = struct{}{}
		if subject.Interpretation != nil && subject.Interpretation.Line != "" && !validText(subject.Interpretation.Line) {
			return fmt.Errorf("group index: invalid subject interpretation")
		}
	}
	payload := artifact
	payload.SHA256 = ""
	want, err := artifactDigest(payload)
	if err != nil {
		return err
	}
	if !validSHA256(artifact.SHA256) || artifact.SHA256 != want {
		return fmt.Errorf("group index: sha256 mismatch")
	}
	return nil
}

func (artifact Overlay) Hydrate(program programindex.Index) (Index, error) {
	if err := program.Validate(); err != nil {
		return Index{}, fmt.Errorf("group index: invalid ProgramIndex: %w", err)
	}
	if err := artifact.Validate(); err != nil {
		return Index{}, err
	}
	if artifact.TargetID != program.Target.ID || artifact.ProgramIndexSHA256 != program.SHA256 {
		return Index{}, fmt.Errorf("group index: ProgramIndex binding mismatch")
	}
	retained := make(map[string]struct{}, len(artifact.Subjects))
	annotations := make(map[string]SubjectAnnotation, len(artifact.Subjects))
	for _, annotation := range artifact.Subjects {
		retained[annotation.ID] = struct{}{}
		annotations[annotation.ID] = annotation
	}
	subjects := compileRetainedSubjects(program, retained)
	for position := range subjects {
		annotation := annotations[subjects[position].ID]
		subjects[position].Categories = make([]programindex.Category, len(annotation.Categories))
		copy(subjects[position].Categories, annotation.Categories)
		if annotation.Interpretation != nil {
			interpretation := *annotation.Interpretation
			subjects[position].Interpretation = &interpretation
		}
	}
	operations := append([]Operation(nil), artifact.Operations...)
	index := Index{
		Version: artifact.Version, Role: artifact.Role, SharedCode: artifact.SharedCode, Summary: artifact.Summary,
		Target: program.Target.Snapshot(), ProgramIndexSHA256: artifact.ProgramIndexSHA256,
		Data: artifact.Data, Subjects: subjects, Groups: artifact.Groups, Operations: operations,
		Outbound: artifact.Outbound, Containers: artifact.Containers,
		StructuralEdges: compileStructuralEdges(program, retained), Connections: slices.Clone(artifact.Connections),
		OffMap: artifact.OffMap, MapFailure: artifact.MapFailure, SHA256: artifact.SHA256,
	}
	// Derive writes the connections' derived fields: on a copy, so hydrating
	// never changes the overlay it reads.
	Derive(&index)
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

func indexDigest(index Index) (string, error) {
	payload := OverlayFromIndex(index)
	payload.SHA256 = ""
	return artifactDigest(payload)
}

func artifactDigest(payload Overlay) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("group index: encode digest material: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func stableID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		_, _ = digest.Write([]byte(strconv.Itoa(len(field))))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSnakeCase(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	previousUnderscore := false
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			previousUnderscore = false
		case character == '_' && !previousUnderscore:
			previousUnderscore = true
		default:
			return false
		}
	}
	return !previousUnderscore
}

func validDirectSubjectID(value string) bool {
	return validCompactID(value, "n") || validPatternID(value)
}

func validGroupID(value string) bool {
	return validCompactID(value, "g")
}

func validConnectionID(value string) bool {
	return validCompactID(value, "x")
}

func validContainerID(value string) bool { return validCompactID(value, "k") }

func compactOrdinal(prefix string, zeroBased int) string {
	return prefix + strconv.Itoa(zeroBased+1)
}

func compactIDLess(left, right, prefix string) bool {
	leftOrdinal, leftErr := strconv.Atoi(strings.TrimPrefix(left, prefix))
	rightOrdinal, rightErr := strconv.Atoi(strings.TrimPrefix(right, prefix))
	if leftErr != nil || rightErr != nil {
		return left < right
	}
	return leftOrdinal < rightOrdinal
}

func subjectIDLess(left, right string) bool {
	leftObject := strings.HasPrefix(left, "n")
	rightObject := strings.HasPrefix(right, "n")
	if leftObject != rightObject {
		return leftObject
	}
	if leftObject {
		return compactIDLess(left, right, "n")
	}
	parsePattern := func(value string) (int, int) {
		separator := strings.IndexByte(value, 'p')
		if separator < 0 {
			return 0, 0
		}
		relation, _ := strconv.Atoi(strings.TrimPrefix(value[:separator], "e"))
		pattern, _ := strconv.Atoi(value[separator+1:])
		return relation, pattern
	}
	leftRelation, leftPattern := parsePattern(left)
	rightRelation, rightPattern := parsePattern(right)
	if leftRelation != rightRelation {
		return leftRelation < rightRelation
	}
	return leftPattern < rightPattern
}

// SubjectIDLess orders the compact object and pattern identities used by the
// shared graph and by qualified provider references.
func SubjectIDLess(left, right string) bool {
	return subjectIDLess(left, right)
}

func validRelationID(value string) bool {
	return validCompactID(value, "e")
}

func validCompactID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return false
	}
	ordinal, err := strconv.Atoi(value[len(prefix):])
	return err == nil && ordinal > 0 && prefix+strconv.Itoa(ordinal) == value
}

func validPatternID(value string) bool {
	return validScopedCompactID(value, "e", "p")
}

func validPatternArgumentID(value string) bool {
	patternEnd := strings.LastIndex(value, "a")
	return patternEnd > 0 && validPatternID(value[:patternEnd]) && validCompactID(value[patternEnd:], "a")
}

func validPatternValueID(value string) bool {
	argumentEnd := strings.LastIndex(value, "v")
	return argumentEnd > 0 && validPatternArgumentID(value[:argumentEnd]) && validCompactID(value[argumentEnd:], "v")
}

func validScopedCompactID(value, outerPrefix, innerPrefix string) bool {
	inner := strings.LastIndex(value, innerPrefix)
	return inner > 0 && validCompactID(value[:inner], outerPrefix) && validCompactID(value[inner:], innerPrefix)
}

func validTargetID(value string) bool {
	return validCompactID(value, "t")
}

func validPrefixedSHA256(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) && validSHA256(strings.TrimPrefix(value, prefix))
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func qualifySubjects(targetID string, subjectIDs []string) []SubjectEndpoint {
	result := make([]SubjectEndpoint, len(subjectIDs))
	for position, subjectID := range subjectIDs {
		result[position] = SubjectEndpoint{TargetID: targetID, SubjectID: subjectID}
	}
	return result
}

func canonicalSubjectEndpoints(values []SubjectEndpoint) bool {
	for position, value := range values {
		if !validTargetID(value.TargetID) || !validDirectSubjectID(value.SubjectID) ||
			position > 0 && subjectEndpointKey(values[position-1]) >= subjectEndpointKey(value) {
			return false
		}
	}
	return true
}

func canonicalizeSubjectEndpoints(values []SubjectEndpoint) ([]SubjectEndpoint, bool) {
	result := cloneSubjectEndpoints(values)
	for _, value := range result {
		if !validTargetID(value.TargetID) || !validDirectSubjectID(value.SubjectID) {
			return nil, false
		}
	}
	sort.Slice(result, func(i, j int) bool { return subjectEndpointKey(result[i]) < subjectEndpointKey(result[j]) })
	compacted := result[:0]
	for _, value := range result {
		if len(compacted) == 0 || subjectEndpointKey(compacted[len(compacted)-1]) != subjectEndpointKey(value) {
			compacted = append(compacted, value)
		}
	}
	if compacted == nil {
		compacted = []SubjectEndpoint{}
	}
	return compacted, true
}

func subjectEndpointKey(value SubjectEndpoint) string {
	return value.TargetID + "\x00" + value.SubjectID
}

func cloneSubjectEndpoints(values []SubjectEndpoint) []SubjectEndpoint {
	if values == nil {
		return nil
	}
	result := make([]SubjectEndpoint, len(values))
	copy(result, values)
	return result
}

func cloneSubject(subject Subject) Subject {
	result := subject
	if subject.Interpretation != nil {
		interpretation := *subject.Interpretation
		result.Interpretation = &interpretation
	}
	result.Categories = make([]programindex.Category, len(subject.Categories))
	copy(result.Categories, subject.Categories)
	if subject.Object != nil {
		object := *subject.Object
		object.External = cloneExternal(subject.Object.External)
		object.Location = cloneLocation(subject.Object.Location)
		result.Object = &object
	}
	if subject.Pattern != nil {
		pattern := *subject.Pattern
		pattern.Location = cloneLocation(subject.Pattern.Location)
		pattern.ToIDs = cloneStrings(subject.Pattern.ToIDs)
		pattern.ReceiverOriginIDs = cloneStrings(subject.Pattern.ReceiverOriginIDs)
		pattern.Arguments = clonePatternArguments(subject.Pattern.Arguments)
		result.Pattern = &pattern
	}
	return result
}

func clonePatternArgumentsFromProgram(values []programindex.PatternArgument) []PatternArgument {
	result := make([]PatternArgument, len(values))
	for position, value := range values {
		result[position] = PatternArgument{
			ID: value.ID, Position: value.Position, Keyword: value.Keyword, Kind: value.Kind, Value: value.Value,
			Parts: clonePatternParts(value.Parts), ObjectIDs: cloneStrings(value.ObjectIDs), Resolution: value.Resolution,
			ObjectsObserved: value.ObjectsObserved, ObjectsOmitted: value.ObjectsOmitted,
			ValueCandidates:         clonePatternValueCandidatesFromProgram(value.ValueCandidates),
			ValueCandidatesObserved: value.ValueCandidatesObserved, ValueCandidatesOmitted: value.ValueCandidatesOmitted,
		}
	}
	return result
}

func clonePatternArguments(values []PatternArgument) []PatternArgument {
	if values == nil {
		return nil
	}
	result := make([]PatternArgument, len(values))
	for position, value := range values {
		result[position] = value
		result[position].Parts = clonePatternParts(value.Parts)
		result[position].ObjectIDs = cloneStrings(value.ObjectIDs)
		result[position].ValueCandidates = clonePatternValueCandidates(value.ValueCandidates)
	}
	return result
}

func clonePatternValueCandidatesFromProgram(values []programindex.PatternValueCandidate) []PatternValueCandidate {
	result := make([]PatternValueCandidate, len(values))
	for position, value := range values {
		result[position] = PatternValueCandidate{
			ID: value.ID, Kind: value.Kind, Value: value.Value, Parts: clonePatternParts(value.Parts),
			Resolution: value.Resolution, SourceKind: value.SourceKind,
			SourceObjectIDs:       cloneStrings(value.SourceObjectIDs),
			SourceObjectsObserved: value.SourceObjectsObserved, SourceObjectsOmitted: value.SourceObjectsOmitted,
			SourceArgumentIDs:       cloneStrings(value.SourceArgumentIDs),
			SourceArgumentsObserved: value.SourceArgumentsObserved, SourceArgumentsOmitted: value.SourceArgumentsOmitted,
		}
	}
	return result
}

func clonePatternValueCandidates(values []PatternValueCandidate) []PatternValueCandidate {
	if values == nil {
		return nil
	}
	result := make([]PatternValueCandidate, len(values))
	for position, value := range values {
		result[position] = value
		result[position].Parts = clonePatternParts(value.Parts)
		result[position].SourceObjectIDs = cloneStrings(value.SourceObjectIDs)
		result[position].SourceArgumentIDs = cloneStrings(value.SourceArgumentIDs)
	}
	return result
}

func clonePatternParts(values []programindex.PatternPart) []programindex.PatternPart {
	if values == nil {
		return nil
	}
	result := make([]programindex.PatternPart, len(values))
	copy(result, values)
	return result
}

func cloneExternal(value *programindex.ExternalSymbol) *programindex.ExternalSymbol {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func validOptionalText(value string) bool {
	return value == "" || validText(value)
}

func validOptionalLocation(value *programindex.Location) bool {
	return value == nil || validText(value.Path) && !strings.HasPrefix(value.Path, "/") &&
		value.Line > 0 && value.Column > 0
}
