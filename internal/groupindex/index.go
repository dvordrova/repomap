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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version          = 4
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
type Group struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	Lane               Lane     `json:"lane"`
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
	Name                 string                            `json:"name"`
	Kind                 programindex.ObjectKind           `json:"kind"`
	Visibility           programindex.Visibility           `json:"visibility"`
	Signature            string                            `json:"signature,omitempty"`
	OwnerID              string                            `json:"owner_id,omitempty"`
	ContainerID          string                            `json:"container_id,omitempty"`
	SymbolLinkIdentities []programindex.SymbolLinkIdentity `json:"symbol_link_identities"`
	External             *programindex.ExternalSymbol      `json:"external,omitempty"`
	Location             *programindex.Location            `json:"location,omitempty"`
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
	ArgumentID       string                              `json:"argument_id,omitempty"`
	ValueCandidateID string                              `json:"value_candidate_id,omitempty"`
	SourceArgumentID string                              `json:"source_argument_id,omitempty"`
	ValueResolution  programindex.PatternValueResolution `json:"value_resolution,omitempty"`
	ValueSourceKind  programindex.PatternValueSourceKind `json:"value_source_kind,omitempty"`
}

// Index is the single sealed group-graph authority for one enriched
// ProgramIndex target.
type Index struct {
	Version            int                 `json:"version"`
	Target             programindex.Target `json:"target"`
	ProgramIndexSHA256 string              `json:"program_index_sha256"`
	Subjects           []Subject           `json:"subjects"`
	Groups             []Group             `json:"groups"`
	StructuralEdges    []StructuralEdge    `json:"structural_edges"`
	Connections        []Connection        `json:"connections"`
	SHA256             string              `json:"sha256"`
}

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
}

// Diagnostic reports one proposal row that could not acquire local authority.
// Broken rows are discarded; Build never guesses a subject or group.
type Diagnostic struct {
	Kind        string `json:"kind"`
	ProposalKey string `json:"proposal_key"`
	Reason      string `json:"reason"`
}

const (
	diagnosticInvalidGroup          = "invalid_group"
	diagnosticUnknownSubject        = "unknown_subject"
	diagnosticLaneMismatch          = "lane_mismatch"
	diagnosticUnsupportedEvidence   = "unsupported_evidence"
	diagnosticConflictingGroupKey   = "conflicting_group_key"
	diagnosticInvalidConnection     = "invalid_connection"
	diagnosticUnknownGroup          = "unknown_group"
	diagnosticConflictingConnection = "conflicting_connection"
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
		byID := groupCandidates[proposal.Key]
		if byID == nil {
			byID = make(map[string]Group)
			groupCandidates[proposal.Key] = byID
		}
		byID[group.ID] = group
	}

	groupsByID := make(map[string]Group)
	groupIDsByKey := make(map[string]string)
	for key, candidates := range groupCandidates {
		if len(candidates) != 1 {
			diagnostics = append(diagnostics, Diagnostic{
				Kind: diagnosticConflictingGroupKey, ProposalKey: key,
				Reason: "proposal-local group key resolves to conflicting groups",
			})
			continue
		}
		for id, group := range candidates {
			groupIDsByKey[key] = id
			groupsByID[id] = group
		}
	}

	groups := make([]Group, 0, len(groupsByID))
	for _, group := range groupsByID {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	if groups == nil {
		groups = []Group{}
	}

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
		StructuralEdges:    structuralEdges,
		Connections:        connections,
	}
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
				connections = append(connections, connection)
			}
		}
		sort.Slice(connections, func(i, j int) bool { return connectionKey(connections[i]) < connectionKey(connections[j]) })
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

// Snapshot returns a consumer-owned deep copy.
func (index Index) Snapshot() Index {
	result := index
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
	result.Connections = make([]Connection, len(index.Connections))
	for position, connection := range index.Connections {
		result.Connections[position] = connection
		result.Connections[position].Evidence = cloneSubjectEndpoints(connection.Evidence)
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
	if index.Subjects == nil || index.Groups == nil || index.StructuralEdges == nil || index.Connections == nil {
		return fmt.Errorf("group index: missing collections")
	}

	subjectsByID := make(map[string]Subject, len(index.Subjects))
	for position, subject := range index.Subjects {
		if err := validateSubject(subject); err != nil {
			return err
		}
		if position > 0 && index.Subjects[position-1].ID >= subject.ID {
			return fmt.Errorf("group index: subjects are not canonical")
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
		if position > 0 && index.Groups[position-1].ID >= group.ID {
			return fmt.Errorf("group index: groups are not canonical")
		}
		groupsByID[group.ID] = struct{}{}
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
		if position > 0 && connectionKey(index.Connections[position-1]) >= connectionKey(connection) {
			return fmt.Errorf("group index: connections are not canonical")
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

// Encode validates and returns canonical JSON artifact bytes.
func Encode(index Index) ([]byte, error) {
	if err := index.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("group index: encode artifact: %w", err)
	}
	return encoded, nil
}

// Decode strictly decodes one artifact and validates its identities and seal.
func Decode(encoded []byte) (Index, error) {
	if len(encoded) == 0 {
		return Index{}, fmt.Errorf("group index: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var index Index
	if err := decoder.Decode(&index); err != nil {
		return Index{}, fmt.Errorf("group index: decode artifact: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Index{}, fmt.Errorf("group index: trailing JSON value")
		}
		return Index{}, fmt.Errorf("group index: trailing data: %w", err)
	}
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

// ValidateSet proves every cross-target group endpoint and evidence subject
// against the complete set of GroupsIndexes passed to the matching boundary.
// Each target may occur exactly once.
func ValidateSet(indexes []Index) error {
	groupsByTarget := make(map[string]map[string]struct{}, len(indexes))
	subjectsByTarget := make(map[string]map[string]struct{}, len(indexes))
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
	}
	for _, index := range indexes {
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
	group.ID = groupIdentity(targetID, group)
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
	connection.ID = connectionIdentity(connection)
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
	connection.ID = connectionIdentity(connection)
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
	categoriesByID := make(map[string][]programindex.Category, len(index.Categorization.Assignments))
	for _, assignment := range index.Categorization.Assignments {
		categoriesByID[assignment.SubjectID] = append([]programindex.Category(nil), assignment.Categories...)
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
				SymbolLinkIdentities: cloneSymbolLinkIdentities(object.SymbolLinkIdentities),
				External:             cloneExternal(object.External),
				Location:             cloneLocation(object.Location),
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
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
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
	result := make([]StructuralEdge, 0, len(byKey))
	for _, edge := range byKey {
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool { return structuralEdgeKey(result[i]) < structuralEdgeKey(result[j]) })
	if result == nil {
		result = []StructuralEdge{}
	}
	return result
}

func validateSubject(subject Subject) error {
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
	if !validGroupID(group.ID) || !validText(group.Title) || !validText(group.Summary) || !group.Lane.Valid() ||
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
	if group.ID != groupIdentity(targetID, group) {
		return fmt.Errorf("group index: group identity mismatch")
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
	if !validConnectionID(connection.ID) || !validTargetID(connection.From.TargetID) || !validGroupID(connection.From.GroupID) ||
		!validTargetID(connection.To.TargetID) || !validGroupID(connection.To.GroupID) ||
		!validSnakeCase(connection.SemanticKind) || !validText(connection.Label) || !validText(connection.Summary) ||
		!connection.SupportResolution.Valid() || connection.Evidence == nil ||
		!canonicalSubjectEndpoints(connection.Evidence) {
		return fmt.Errorf("group index: invalid connection")
	}
	if connection.ID != connectionIdentity(connection) {
		return fmt.Errorf("group index: connection identity mismatch")
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
		validPrefixedSHA256(edge.ArgumentID, "program-pattern-argument-") &&
		validPrefixedSHA256(edge.ValueCandidateID, "program-pattern-value-") &&
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
	position := sort.SearchStrings(values, value)
	return position < len(values) && values[position] == value
}

func validObjectFacts(facts ObjectFacts) bool {
	if !validText(facts.Name) || !facts.Kind.Valid() || !facts.Visibility.Valid() ||
		!validOptionalText(facts.Signature) || !validOptionalDirectObjectID(facts.OwnerID) ||
		!validOptionalDirectObjectID(facts.ContainerID) || facts.SymbolLinkIdentities == nil ||
		!canonicalSymbolLinkIdentities(facts.SymbolLinkIdentities) || !validOptionalLocation(facts.Location) {
		return false
	}
	if facts.External == nil {
		return true
	}
	return facts.Kind == programindex.ObjectExternalSymbol && facts.External.AuthorityKind.Valid() &&
		validText(facts.External.PackagePath) &&
		validOptionalText(facts.External.Receiver) && validText(facts.External.Name)
}

func validPatternFacts(patternID string, facts PatternFacts) bool {
	if !facts.Form.Valid() || !validText(facts.Selector) || !validOptionalLocation(facts.Location) ||
		!validRelationID(facts.RelationID) || !facts.RelationKind.Valid() || !facts.RelationResolution.Valid() ||
		!validPrefixedSHA256(facts.FromID, "program-object-") || facts.ToIDs == nil ||
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
	if !validPrefixedSHA256(argument.ID, "program-pattern-argument-") ||
		!validPatternArgumentSelector(argument.Position, argument.Keyword) || !argument.Kind.Valid() ||
		argument.Parts == nil || argument.ObjectIDs == nil || argument.ValueCandidates == nil ||
		!canonicalDirectObjectIDs(argument.ObjectIDs) ||
		!validPatternArgumentObjectAuthority(argument.ObjectIDs, argument.Resolution, argument.ObjectsObserved, argument.ObjectsOmitted) ||
		argument.ValueCandidatesObserved != len(argument.ValueCandidates) ||
		argument.ValueCandidatesOmitted != 0 {
		return false
	}
	if argument.ID != stableID("program-pattern-argument", patternID, patternArgumentKey(argument)) {
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
	if !validPrefixedSHA256(candidate.ID, "program-pattern-value-") ||
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
	if candidate.ID != patternValueCandidateIdentity(argumentID, candidate) {
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
		if !validPrefixedSHA256(value, "program-object-") || position > 0 && values[position-1] >= value {
			return false
		}
	}
	return true
}

func canonicalPatternArgumentIDs(values []string) bool {
	for position, value := range values {
		if !validPrefixedSHA256(value, "program-pattern-argument-") || position > 0 && values[position-1] >= value {
			return false
		}
	}
	return true
}

func validOptionalDirectObjectID(value string) bool {
	return value == "" || validPrefixedSHA256(value, "program-object-")
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

func groupIdentity(targetID string, group Group) string {
	fields := []string{targetID, string(group.Lane), group.Title, group.Summary, "members", strconv.Itoa(len(group.MemberSubjectIDs))}
	fields = append(fields, group.MemberSubjectIDs...)
	fields = append(fields, "evidence", strconv.Itoa(len(group.EvidenceSubjectIDs)))
	fields = append(fields, group.EvidenceSubjectIDs...)
	return stableID("program-group", fields...)
}

func connectionSlot(connection Connection) string {
	return strings.Join([]string{
		connection.From.TargetID, connection.From.GroupID,
		connection.To.TargetID, connection.To.GroupID,
		connection.SemanticKind,
	}, "\x00")
}

func connectionKey(connection Connection) string {
	values := []string{
		connectionSlot(connection), string(connection.SupportResolution),
		connection.Label, connection.Summary, strconv.Itoa(len(connection.Evidence)),
	}
	for _, evidence := range connection.Evidence {
		values = append(values, evidence.TargetID, evidence.SubjectID)
	}
	return strings.Join(values, "\x00")
}

func connectionIdentity(connection Connection) string {
	copyValue := connection
	copyValue.ID = ""
	return stableID("program-group-connection", connectionKey(copyValue))
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
	sort.Strings(result)
	result = compactStrings(result)
	if result == nil {
		result = []string{}
	}
	return result, true
}

func canonicalDirectSubjectIDs(values []string) bool {
	for position, value := range values {
		if !validDirectSubjectID(value) || position > 0 && values[position-1] >= value {
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

func indexDigest(index Index) (string, error) {
	payload := index.Snapshot()
	payload.SHA256 = ""
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
	return validPrefixedSHA256(value, "program-object-") || validPrefixedSHA256(value, "program-pattern-")
}

func validGroupID(value string) bool {
	return validPrefixedSHA256(value, "program-group-")
}

func validConnectionID(value string) bool {
	return validPrefixedSHA256(value, "program-group-connection-")
}

func validRelationID(value string) bool {
	return validPrefixedSHA256(value, "program-relation-")
}

func validTargetID(value string) bool {
	return validPrefixedSHA256(value, "program-target-")
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
	result.Categories = make([]programindex.Category, len(subject.Categories))
	copy(result.Categories, subject.Categories)
	if subject.Object != nil {
		object := *subject.Object
		object.SymbolLinkIdentities = cloneSymbolLinkIdentities(subject.Object.SymbolLinkIdentities)
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

func cloneSymbolLinkIdentities(values []programindex.SymbolLinkIdentity) []programindex.SymbolLinkIdentity {
	if len(values) == 0 {
		return []programindex.SymbolLinkIdentity{}
	}
	result := make([]programindex.SymbolLinkIdentity, len(values))
	copy(result, values)
	return result
}

func canonicalSymbolLinkIdentities(values []programindex.SymbolLinkIdentity) bool {
	previous := ""
	for _, value := range values {
		if !validText(value.Domain) || !validSymbolLinkKey(value.Key) || !validOptionalText(value.Display) || value.PartCount <= 0 {
			return false
		}
		key := value.Domain + "\x00" + value.Key
		if previous != "" && previous >= key {
			return false
		}
		previous = key
	}
	return true
}

func validSymbolLinkKey(value string) bool {
	return validPrefixedSHA256(value, "symbol-link-")
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
