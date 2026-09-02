package groupmatching

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

type targetWire struct {
	Ref      string `json:"ref"`
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Selector string `json:"selector,omitempty"`
}

type groupWire struct {
	Ref              string          `json:"ref"`
	TargetRef        string          `json:"target_ref"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary"`
	Lane             groupindex.Lane `json:"lane"`
	MemberRefs       []string        `json:"member_refs"`
	EvidenceRefs     []string        `json:"evidence_refs"`
	BoundaryEdgeRefs []string        `json:"boundary_edge_refs"`
}

type symbolLinkWire struct {
	Domain    string `json:"domain"`
	Display   string `json:"display,omitempty"`
	PartCount int    `json:"part_count"`
}

type objectWire struct {
	Name         string                       `json:"name"`
	Kind         programindex.ObjectKind      `json:"kind"`
	Visibility   programindex.Visibility      `json:"visibility"`
	Signature    string                       `json:"signature,omitempty"`
	OwnerRef     string                       `json:"owner_ref,omitempty"`
	ContainerRef string                       `json:"container_ref,omitempty"`
	SymbolLinks  []symbolLinkWire             `json:"symbol_links"`
	External     *programindex.ExternalSymbol `json:"external,omitempty"`
	Location     *programindex.Location       `json:"location,omitempty"`
}

type argumentWire struct {
	Ref                     string                        `json:"ref"`
	Position                int                           `json:"position,omitempty"`
	Keyword                 string                        `json:"keyword,omitempty"`
	Kind                    programindex.PatternValueKind `json:"kind"`
	Value                   string                        `json:"value,omitempty"`
	Parts                   []programindex.PatternPart    `json:"parts"`
	ObjectRefs              []string                      `json:"object_refs"`
	Resolution              programindex.Resolution       `json:"resolution,omitempty"`
	ObjectsObserved         int                           `json:"objects_observed"`
	ObjectsOmitted          int                           `json:"objects_omitted"`
	ValueCandidates         []valueCandidateWire          `json:"value_candidates"`
	ValueCandidatesObserved int                           `json:"value_candidates_observed"`
	ValueCandidatesOmitted  int                           `json:"value_candidates_omitted"`
}

type valueCandidateWire struct {
	Ref                     string                              `json:"ref"`
	Kind                    programindex.PatternValueKind       `json:"kind"`
	Value                   string                              `json:"value,omitempty"`
	Parts                   []programindex.PatternPart          `json:"parts"`
	Resolution              programindex.PatternValueResolution `json:"resolution"`
	SourceKind              programindex.PatternValueSourceKind `json:"source_kind"`
	SourceObjectRefs        []string                            `json:"source_object_refs"`
	SourceObjectsObserved   int                                 `json:"source_objects_observed"`
	SourceObjectsOmitted    int                                 `json:"source_objects_omitted"`
	SourceArgumentRefs      []string                            `json:"source_argument_refs"`
	SourceArgumentsObserved int                                 `json:"source_arguments_observed"`
	SourceArgumentsOmitted  int                                 `json:"source_arguments_omitted"`
}

type patternWire struct {
	Form                     programindex.PatternForm  `json:"form"`
	Selector                 string                    `json:"selector"`
	Location                 *programindex.Location    `json:"location,omitempty"`
	RelationKind             programindex.RelationKind `json:"relation_kind"`
	RelationResolution       programindex.Resolution   `json:"relation_resolution"`
	FromRef                  string                    `json:"from_ref"`
	ToRefs                   []string                  `json:"to_refs"`
	Invocation               string                    `json:"invocation,omitempty"`
	ResultRef                string                    `json:"result_ref,omitempty"`
	ReceiverRef              string                    `json:"receiver_ref,omitempty"`
	ReceiverOriginRefs       []string                  `json:"receiver_origin_refs"`
	ReceiverOriginResolution programindex.Resolution   `json:"receiver_origin_resolution,omitempty"`
	Arguments                []argumentWire            `json:"arguments"`
}

type subjectWire struct {
	Ref        string                  `json:"ref"`
	TargetRef  string                  `json:"target_ref"`
	Kind       groupindex.SubjectKind  `json:"kind"`
	Categories []programindex.Category `json:"categories"`
	Object     *objectWire             `json:"object,omitempty"`
	Pattern    *patternWire            `json:"pattern,omitempty"`
}

type structuralEdgeWire struct {
	Ref               string                              `json:"ref"`
	TargetRef         string                              `json:"target_ref"`
	FromRef           string                              `json:"from_ref"`
	ToRef             string                              `json:"to_ref"`
	Role              groupindex.StructuralEdgeRole       `json:"role"`
	RelationKind      programindex.RelationKind           `json:"relation_kind"`
	Resolution        programindex.Resolution             `json:"resolution"`
	ArgumentRef       string                              `json:"argument_ref,omitempty"`
	ValueCandidateRef string                              `json:"value_candidate_ref,omitempty"`
	SourceArgumentRef string                              `json:"source_argument_ref,omitempty"`
	ValueResolution   programindex.PatternValueResolution `json:"value_resolution,omitempty"`
	ValueSourceKind   programindex.PatternValueSourceKind `json:"value_source_kind,omitempty"`
}

type localConnectionWire struct {
	Ref          string           `json:"ref"`
	FromGroup    groupContextWire `json:"from_group"`
	ToGroup      groupContextWire `json:"to_group"`
	SemanticKind string           `json:"semantic_kind"`
	Label        string           `json:"label"`
	Summary      string           `json:"summary"`
	EvidenceRefs []string         `json:"evidence_refs"`
}

type groupContextWire struct {
	Ref       string          `json:"ref"`
	TargetRef string          `json:"target_ref"`
	Title     string          `json:"title"`
	Summary   string          `json:"summary"`
	Lane      groupindex.Lane `json:"lane"`
}

type pairWire struct {
	Ref        string    `json:"ref"`
	LeftGroup  groupWire `json:"left_group"`
	RightGroup groupWire `json:"right_group"`
}

type witnessCandidateWire struct {
	Ref                  string                              `json:"ref"`
	Kind                 string                              `json:"kind"`
	LeftBoundaryEdgeRef  string                              `json:"left_boundary_edge_ref"`
	LeftPatternRef       string                              `json:"left_pattern_ref"`
	LeftArgumentRef      string                              `json:"left_argument_ref"`
	RightBoundaryEdgeRef string                              `json:"right_boundary_edge_ref"`
	RightPatternRef      string                              `json:"right_pattern_ref"`
	RightArgumentRef     string                              `json:"right_argument_ref"`
	SupportResolution    programindex.PatternValueResolution `json:"support_resolution"`
	RequiredFromGroupRef string                              `json:"required_from_group_ref,omitempty"`
	RequiredToGroupRef   string                              `json:"required_to_group_ref,omitempty"`
}

// Request carries only request-local refs and rich GroupsIndex facts. It has
// no canonical target/group/subject/relation identity or ProgramIndex digest.
type Request struct {
	Version           int                    `json:"version"`
	Pair              pairWire               `json:"pair"`
	Targets           []targetWire           `json:"targets"`
	Subjects          []subjectWire          `json:"subjects"`
	StructuralEdges   []structuralEdgeWire   `json:"structural_edges"`
	LocalConnections  []localConnectionWire  `json:"local_connections"`
	WitnessCandidates []witnessCandidateWire `json:"witness_candidates"`
}

type responseConnection struct {
	PairRef          string   `json:"pair_ref"`
	FromGroupRef     string   `json:"from_group_ref"`
	ToGroupRef       string   `json:"to_group_ref"`
	SemanticKind     string   `json:"semantic_kind"`
	Label            string   `json:"label"`
	Summary          string   `json:"summary"`
	WitnessJointRefs []string `json:"witness_joint_refs"`
}

type responseWitnessJoint struct {
	Kind                 string `json:"kind"`
	LeftBoundaryEdgeRef  string `json:"left_boundary_edge_ref"`
	LeftArgumentRef      string `json:"left_argument_ref"`
	RightBoundaryEdgeRef string `json:"right_boundary_edge_ref"`
	RightArgumentRef     string `json:"right_argument_ref"`
}

func (compilation Compilation) request(pairRef string) (Request, error) {
	pair, known := compilation.pairByRef[pairRef]
	if !known {
		return Request{}, fmt.Errorf("group matching: request cites unknown pair ref %q", pairRef)
	}
	left, leftKnown := compilation.groupByRef[pair.leftGroupRef]
	right, rightKnown := compilation.groupByRef[pair.rightGroupRef]
	if !leftKnown || !rightKnown {
		return Request{}, fmt.Errorf("group matching: request pair retained an unknown endpoint group")
	}
	request := Request{
		Version: requestVersion,
		Pair: pairWire{
			Ref: pair.ref, LeftGroup: compilation.groupWire(left), RightGroup: compilation.groupWire(right),
		},
		Targets: []targetWire{}, Subjects: []subjectWire{},
		StructuralEdges: []structuralEdgeWire{}, LocalConnections: []localConnectionWire{},
		WitnessCandidates: []witnessCandidateWire{},
	}
	for _, target := range compilation.targets {
		if _, include := pair.targetRefs[target.ref]; !include {
			continue
		}
		request.Targets = append(request.Targets, compilation.targetWire(target))
	}
	for _, subject := range compilation.subjects {
		if _, include := pair.subjectRefs[subject.ref]; !include {
			continue
		}
		request.Subjects = append(request.Subjects, compilation.subjectWire(subject))
	}
	for _, edge := range compilation.edges {
		if _, include := pair.edgeRefs[edge.ref]; !include {
			continue
		}
		request.StructuralEdges = append(request.StructuralEdges, compilation.structuralEdgeWire(edge))
	}
	for _, connection := range compilation.localConnections {
		if _, include := pair.connectionRefs[connection.ref]; !include {
			continue
		}
		request.LocalConnections = append(request.LocalConnections, compilation.localConnectionWire(connection))
	}
	for _, authority := range pair.witnessCandidates {
		request.WitnessCandidates = append(request.WitnessCandidates, compilation.witnessCandidateWire(authority))
	}
	return request, nil
}

func (compilation Compilation) witnessCandidateWire(authority witnessCandidateAuthority) witnessCandidateWire {
	joint := authority.joint
	return witnessCandidateWire{
		Ref: authority.ref, Kind: joint.kind,
		LeftBoundaryEdgeRef: joint.leftBoundaryEdgeRef, LeftPatternRef: joint.leftPatternRef,
		LeftArgumentRef:      joint.leftArgumentRef,
		RightBoundaryEdgeRef: joint.rightBoundaryEdgeRef, RightPatternRef: joint.rightPatternRef,
		RightArgumentRef: joint.rightArgumentRef, SupportResolution: joint.resolution,
		RequiredFromGroupRef: authority.requiredFromGroupRef,
		RequiredToGroupRef:   authority.requiredToGroupRef,
	}
}

func (compilation Compilation) targetWire(authority targetAuthority) targetWire {
	target := authority.index.Target
	return targetWire{
		Ref: authority.ref, Language: target.Language, Kind: target.Kind,
		Name: target.Name, Selector: target.Selector,
	}
}

func (compilation Compilation) groupWire(authority groupAuthority) groupWire {
	row := groupWire{
		Ref: authority.ref, TargetRef: authority.targetRef,
		Title: authority.group.Title, Summary: authority.group.Summary, Lane: authority.group.Lane,
		MemberRefs:       make([]string, 0, len(authority.group.MemberSubjectIDs)),
		EvidenceRefs:     make([]string, 0, len(authority.group.EvidenceSubjectIDs)),
		BoundaryEdgeRefs: []string{},
	}
	for _, id := range authority.group.MemberSubjectIDs {
		row.MemberRefs = append(row.MemberRefs, compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, id)])
	}
	for _, id := range authority.group.EvidenceSubjectIDs {
		row.EvidenceRefs = append(row.EvidenceRefs, compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, id)])
	}
	for ref := range compilation.boundaryEdgesByGroupRef[authority.ref] {
		row.BoundaryEdgeRefs = append(row.BoundaryEdgeRefs, ref)
	}
	row.BoundaryEdgeRefs = canonicalStrings(row.BoundaryEdgeRefs)
	return row
}

func (compilation Compilation) subjectWire(authority subjectAuthority) subjectWire {
	subject := authority.subject
	row := subjectWire{
		Ref: authority.ref, TargetRef: authority.targetRef, Kind: subject.Kind,
		Categories: append([]programindex.Category(nil), subject.Categories...),
	}
	if row.Categories == nil {
		row.Categories = []programindex.Category{}
	}
	if subject.Object != nil {
		facts := subject.Object
		row.Object = &objectWire{
			Name: facts.Name, Kind: facts.Kind, Visibility: facts.Visibility, Signature: facts.Signature,
			OwnerRef:     compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, facts.OwnerID)],
			ContainerRef: compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, facts.ContainerID)],
			SymbolLinks:  make([]symbolLinkWire, 0, len(facts.SymbolLinkIdentities)),
			External:     cloneExternal(facts.External), Location: cloneLocation(facts.Location),
		}
		for _, identity := range facts.SymbolLinkIdentities {
			row.Object.SymbolLinks = append(row.Object.SymbolLinks, symbolLinkWire{
				Domain: identity.Domain, Display: identity.Display, PartCount: identity.PartCount,
			})
		}
		return row
	}
	facts := subject.Pattern
	row.Pattern = &patternWire{
		Form: facts.Form, Selector: facts.Selector, Location: cloneLocation(facts.Location),
		RelationKind: facts.RelationKind, RelationResolution: facts.RelationResolution,
		FromRef: compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, facts.FromID)],
		ToRefs:  make([]string, 0, len(facts.ToIDs)), Invocation: facts.Invocation,
		ResultRef:                compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, facts.ResultID)],
		ReceiverRef:              compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, facts.ReceiverID)],
		ReceiverOriginRefs:       make([]string, 0, len(facts.ReceiverOriginIDs)),
		ReceiverOriginResolution: facts.ReceiverOriginResolution,
		Arguments:                make([]argumentWire, 0, len(facts.Arguments)),
	}
	for _, id := range facts.ToIDs {
		row.Pattern.ToRefs = append(row.Pattern.ToRefs, compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, id)])
	}
	for _, id := range facts.ReceiverOriginIDs {
		row.Pattern.ReceiverOriginRefs = append(
			row.Pattern.ReceiverOriginRefs,
			compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, id)],
		)
	}
	for _, argument := range facts.Arguments {
		argumentRow := argumentWire{
			Ref:      compilation.argumentRefByEndpoint[nestedEndpointKey(authority.targetID, argument.ID)],
			Position: argument.Position, Keyword: argument.Keyword, Kind: argument.Kind,
			Value: argument.Value, Parts: append([]programindex.PatternPart(nil), argument.Parts...),
			ObjectRefs: make([]string, 0, len(argument.ObjectIDs)), Resolution: argument.Resolution,
			ObjectsObserved: argument.ObjectsObserved, ObjectsOmitted: argument.ObjectsOmitted,
			ValueCandidates:         make([]valueCandidateWire, 0, len(argument.ValueCandidates)),
			ValueCandidatesObserved: argument.ValueCandidatesObserved,
			ValueCandidatesOmitted:  argument.ValueCandidatesOmitted,
		}
		if argumentRow.Parts == nil {
			argumentRow.Parts = []programindex.PatternPart{}
		}
		for _, id := range argument.ObjectIDs {
			argumentRow.ObjectRefs = append(
				argumentRow.ObjectRefs,
				compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, id)],
			)
		}
		for _, candidate := range argument.ValueCandidates {
			candidateRow := valueCandidateWire{
				Ref:  compilation.valueRefByEndpoint[nestedEndpointKey(authority.targetID, candidate.ID)],
				Kind: candidate.Kind, Value: candidate.Value,
				Parts:      append([]programindex.PatternPart(nil), candidate.Parts...),
				Resolution: candidate.Resolution, SourceKind: candidate.SourceKind,
				SourceObjectRefs:        make([]string, 0, len(candidate.SourceObjectIDs)),
				SourceObjectsObserved:   candidate.SourceObjectsObserved,
				SourceObjectsOmitted:    candidate.SourceObjectsOmitted,
				SourceArgumentRefs:      make([]string, 0, len(candidate.SourceArgumentIDs)),
				SourceArgumentsObserved: candidate.SourceArgumentsObserved,
				SourceArgumentsOmitted:  candidate.SourceArgumentsOmitted,
			}
			if candidateRow.Parts == nil {
				candidateRow.Parts = []programindex.PatternPart{}
			}
			for _, id := range candidate.SourceObjectIDs {
				candidateRow.SourceObjectRefs = append(candidateRow.SourceObjectRefs,
					compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, id)])
			}
			for _, id := range candidate.SourceArgumentIDs {
				candidateRow.SourceArgumentRefs = append(candidateRow.SourceArgumentRefs,
					compilation.argumentRefByEndpoint[nestedEndpointKey(authority.targetID, id)])
			}
			argumentRow.ValueCandidates = append(argumentRow.ValueCandidates, candidateRow)
		}
		row.Pattern.Arguments = append(row.Pattern.Arguments, argumentRow)
	}
	return row
}

func (compilation Compilation) structuralEdgeWire(authority edgeAuthority) structuralEdgeWire {
	return structuralEdgeWire{
		Ref: authority.ref, TargetRef: authority.targetRef,
		FromRef: compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, authority.edge.FromSubjectID)],
		ToRef:   compilation.subjectRefByEndpoint[subjectEndpointKey(authority.targetID, authority.edge.ToSubjectID)],
		Role:    authority.edge.Role, RelationKind: authority.edge.RelationKind, Resolution: authority.edge.Resolution,
		ArgumentRef:       compilation.argumentRefByEndpoint[nestedEndpointKey(authority.targetID, authority.edge.ArgumentID)],
		ValueCandidateRef: compilation.valueRefByEndpoint[nestedEndpointKey(authority.targetID, authority.edge.ValueCandidateID)],
		SourceArgumentRef: compilation.argumentRefByEndpoint[nestedEndpointKey(authority.targetID, authority.edge.SourceArgumentID)],
		ValueResolution:   authority.edge.ValueResolution, ValueSourceKind: authority.edge.ValueSourceKind,
	}
}

func (compilation Compilation) localConnectionWire(authority localConnectionAuthority) localConnectionWire {
	connection := authority.connection
	from := compilation.groupByRef[compilation.groupRefByEndpoint[groupEndpointKey(connection.From.TargetID, connection.From.GroupID)]]
	to := compilation.groupByRef[compilation.groupRefByEndpoint[groupEndpointKey(connection.To.TargetID, connection.To.GroupID)]]
	row := localConnectionWire{
		Ref:          authority.ref,
		FromGroup:    compilation.groupContextWire(from),
		ToGroup:      compilation.groupContextWire(to),
		SemanticKind: connection.SemanticKind, Label: connection.Label, Summary: connection.Summary,
		EvidenceRefs: make([]string, 0, len(connection.Evidence)),
	}
	for _, evidence := range connection.Evidence {
		row.EvidenceRefs = append(
			row.EvidenceRefs,
			compilation.subjectRefByEndpoint[subjectEndpointKey(evidence.TargetID, evidence.SubjectID)],
		)
	}
	return row
}

func (compilation Compilation) groupContextWire(authority groupAuthority) groupContextWire {
	return groupContextWire{
		Ref: authority.ref, TargetRef: authority.targetRef,
		Title: authority.group.Title, Summary: authority.group.Summary, Lane: authority.group.Lane,
	}
}

func unionRefSets(destination, source map[string]struct{}) {
	for ref := range source {
		destination[ref] = struct{}{}
	}
}

func cloneLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneExternal(value *programindex.ExternalSymbol) *programindex.ExternalSymbol {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
