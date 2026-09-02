package programgrouping

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

type targetWire struct {
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Selector string `json:"selector,omitempty"`
}

type locationWire struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type externalWire struct {
	AuthorityKind programindex.ExternalAuthorityKind `json:"authority_kind"`
	PackagePath   string                             `json:"package_path"`
	Receiver      string                             `json:"receiver,omitempty"`
	Name          string                             `json:"name"`
}

type symbolLinkWire struct {
	Domain    string `json:"domain"`
	Display   string `json:"display,omitempty"`
	PartCount int    `json:"part_count"`
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

type subjectWire struct {
	Ref        string                  `json:"ref"`
	Kind       subjectKind             `json:"kind"`
	Categories []programindex.Category `json:"categories"`

	ObjectKind   programindex.ObjectKind `json:"object_kind,omitempty"`
	Name         string                  `json:"name,omitempty"`
	Signature    string                  `json:"signature,omitempty"`
	Visibility   programindex.Visibility `json:"visibility,omitempty"`
	OwnerRef     string                  `json:"owner_ref,omitempty"`
	ContainerRef string                  `json:"container_ref,omitempty"`
	Location     *locationWire           `json:"location,omitempty"`
	External     *externalWire           `json:"external,omitempty"`
	SymbolLinks  []symbolLinkWire        `json:"symbol_links,omitempty"`
	SeedKinds    []programindex.SeedKind `json:"seed_kinds,omitempty"`

	PatternForm              programindex.PatternForm  `json:"pattern_form,omitempty"`
	Selector                 string                    `json:"selector,omitempty"`
	RelationKind             programindex.RelationKind `json:"relation_kind,omitempty"`
	RelationResolution       programindex.Resolution   `json:"relation_resolution,omitempty"`
	Invocation               string                    `json:"invocation,omitempty"`
	ResultRef                string                    `json:"result_ref,omitempty"`
	ReceiverRef              string                    `json:"receiver_ref,omitempty"`
	ReceiverOriginRefs       []string                  `json:"receiver_origin_refs,omitempty"`
	ReceiverOriginResolution programindex.Resolution   `json:"receiver_origin_resolution,omitempty"`
	ReceiverOriginsObserved  int                       `json:"receiver_origins_observed,omitempty"`
	ReceiverOriginsOmitted   int                       `json:"receiver_origins_omitted,omitempty"`
	Arguments                []argumentWire            `json:"arguments,omitempty"`
	ArgumentsObserved        int                       `json:"arguments_observed,omitempty"`
	ArgumentsOmitted         int                       `json:"arguments_omitted,omitempty"`
}

type witnessWire struct {
	Kind             string        `json:"kind"`
	Detail           string        `json:"detail,omitempty"`
	SourceExpression string        `json:"source_expression,omitempty"`
	Location         *locationWire `json:"location,omitempty"`
}

type sourceArgumentWire struct {
	PatternRef string `json:"pattern_ref"`
	Position   int    `json:"position,omitempty"`
	Keyword    string `json:"keyword,omitempty"`
}

type edgeWire struct {
	Ref               string                    `json:"ref"`
	Role              string                    `json:"role"`
	FromRef           string                    `json:"from_ref"`
	ToRef             string                    `json:"to_ref"`
	RelationKind      programindex.RelationKind `json:"relation_kind,omitempty"`
	Resolution        programindex.Resolution   `json:"resolution"`
	Invocation        string                    `json:"invocation,omitempty"`
	Location          *locationWire             `json:"location,omitempty"`
	TargetsObserved   int                       `json:"targets_observed,omitempty"`
	TargetsOmitted    int                       `json:"targets_omitted,omitempty"`
	Witnesses         []witnessWire             `json:"witnesses,omitempty"`
	WitnessesObserved int                       `json:"witnesses_observed,omitempty"`
	WitnessesOmitted  int                       `json:"witnesses_omitted,omitempty"`
	SourceArgument    *sourceArgumentWire       `json:"source_argument,omitempty"`
}

type candidateGroupWire struct {
	Ref          string          `json:"ref"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	Lane         groupindex.Lane `json:"lane"`
	MemberRefs   []string        `json:"member_refs"`
	EvidenceRefs []string        `json:"evidence_refs"`
}

type candidateConnectionWire struct {
	FromGroupRef string   `json:"from_group_ref"`
	ToGroupRef   string   `json:"to_group_ref"`
	SemanticKind string   `json:"semantic_kind"`
	Label        string   `json:"label"`
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs"`
}

// Request is one bounded provider-visible shard. GroupRefs is the only member
// selection authority. Other subjects are complete incident context and may
// be cited as evidence, but an unclassified row is never a selectable member.
type Request struct {
	Version              int                       `json:"version"`
	Phase                phase                     `json:"phase"`
	Target               targetWire                `json:"target"`
	GroupRefs            []string                  `json:"group_refs"`
	Subjects             []subjectWire             `json:"subjects"`
	Edges                []edgeWire                `json:"edges"`
	CandidateGroups      []candidateGroupWire      `json:"candidate_groups"`
	CandidateConnections []candidateConnectionWire `json:"candidate_connections"`
}

type responseGroup struct {
	Key          string          `json:"key"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	Lane         groupindex.Lane `json:"lane"`
	MemberRefs   []string        `json:"member_refs"`
	EvidenceRefs []string        `json:"evidence_refs"`
}

type responseConnection struct {
	FromGroupKey string   `json:"from_group_key"`
	ToGroupKey   string   `json:"to_group_key"`
	SemanticKind string   `json:"semantic_kind"`
	Label        string   `json:"label"`
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs"`
}

func (compilation Compilation) request(
	requestPhase phase,
	groupRefs []string,
	candidates proposalSet,
) (Request, error) {
	ownedIDs := make(map[string]struct{}, len(groupRefs))
	for _, ref := range groupRefs {
		subject, known := compilation.subjectByRef[ref]
		if !known {
			return Request{}, fmt.Errorf("program grouping: request cites unknown subject ref %q", ref)
		}
		if _, selectable := compilation.categorizedRefSet[ref]; !selectable {
			return Request{}, fmt.Errorf("program grouping: request cites unclassified member ref %q", ref)
		}
		if _, duplicate := ownedIDs[subject.id]; duplicate {
			return Request{}, fmt.Errorf("program grouping: request repeats subject ref %q", ref)
		}
		ownedIDs[subject.id] = struct{}{}
	}

	contextIDs := make(map[string]struct{}, len(ownedIDs))
	for id := range ownedIDs {
		contextIDs[id] = struct{}{}
	}
	incidentEdges := make([]graphEdge, 0)
	for _, edge := range compilation.edges {
		_, fromOwned := ownedIDs[edge.fromID]
		_, toOwned := ownedIDs[edge.toID]
		if !fromOwned && !toOwned {
			continue
		}
		incidentEdges = append(incidentEdges, edge)
		contextIDs[edge.fromID] = struct{}{}
		contextIDs[edge.toID] = struct{}{}
		if edge.sourceArgument != nil {
			contextIDs[edge.sourceArgument.patternID] = struct{}{}
		}
	}
	for _, group := range candidates.groups {
		for _, id := range append(cloneStrings(group.MemberSubjectIDs), group.EvidenceSubjectIDs...) {
			if compilation.refBySubjectID[id] == "" {
				return Request{}, fmt.Errorf("program grouping: merge candidate cites unknown subject")
			}
			contextIDs[id] = struct{}{}
		}
	}
	for _, connection := range candidates.connections {
		for _, id := range connection.EvidenceSubjectIDs {
			if compilation.refBySubjectID[id] == "" {
				return Request{}, fmt.Errorf("program grouping: merge connection cites unknown evidence subject")
			}
			contextIDs[id] = struct{}{}
		}
	}
	compilation.expandReferencedContext(contextIDs)

	request := Request{
		Version: requestVersion, Phase: requestPhase,
		Target: targetWire{
			Language: compilation.index.Target.Language, Kind: compilation.index.Target.Kind,
			Name: compilation.index.Target.Name, Selector: compilation.index.Target.Selector,
		},
		GroupRefs:            append([]string(nil), groupRefs...),
		Subjects:             make([]subjectWire, 0, len(contextIDs)),
		Edges:                make([]edgeWire, 0, len(incidentEdges)),
		CandidateGroups:      []candidateGroupWire{},
		CandidateConnections: []candidateConnectionWire{},
	}
	seedKinds := make(map[string][]programindex.SeedKind)
	for _, seed := range compilation.index.Target.Seeds {
		seedKinds[seed.ObjectID] = append(seedKinds[seed.ObjectID], seed.Kind)
	}
	for id := range seedKinds {
		sort.Slice(seedKinds[id], func(i, j int) bool { return seedKinds[id][i] < seedKinds[id][j] })
	}
	for _, subject := range compilation.subjects {
		if _, include := contextIDs[subject.id]; !include {
			continue
		}
		request.Subjects = append(request.Subjects, compilation.subjectWire(subject, seedKinds[subject.id]))
	}
	for _, edge := range incidentEdges {
		request.Edges = append(request.Edges, compilation.edgeWire(edge))
	}
	if requestPhase == phaseMerge {
		if err := compilation.attachCandidates(&request, candidates); err != nil {
			return Request{}, err
		}
	}
	return request, nil
}

func (compilation Compilation) expandReferencedContext(ids map[string]struct{}) {
	for {
		changed := false
		for _, subject := range compilation.subjects {
			if _, include := ids[subject.id]; !include {
				continue
			}
			var referenced []string
			if subject.object != nil {
				referenced = append(referenced, subject.object.OwnerID, subject.object.ContainerID)
			} else {
				referenced = append(referenced, subject.pattern.ResultID, subject.pattern.ReceiverID)
				referenced = append(referenced, subject.pattern.ReceiverOriginIDs...)
				for _, argument := range subject.pattern.Arguments {
					referenced = append(referenced, argument.ObjectIDs...)
					for _, candidate := range argument.ValueCandidates {
						referenced = append(referenced, candidate.SourceObjectIDs...)
						for _, sourceArgumentID := range candidate.SourceArgumentIDs {
							if authority, known := compilation.argumentByID[sourceArgumentID]; known {
								referenced = append(referenced, authority.patternID)
							}
						}
					}
				}
			}
			for _, id := range referenced {
				if id == "" || compilation.refBySubjectID[id] == "" {
					continue
				}
				if _, exists := ids[id]; exists {
					continue
				}
				ids[id] = struct{}{}
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func (compilation Compilation) subjectWire(subject subjectAuthority, seedKinds []programindex.SeedKind) subjectWire {
	row := subjectWire{
		Ref: subject.ref, Kind: subject.kind,
		Categories: append([]programindex.Category(nil), subject.categories...),
	}
	if row.Categories == nil {
		row.Categories = []programindex.Category{}
	}
	if subject.object != nil {
		object := subject.object
		row.ObjectKind = object.Kind
		row.Name = object.Name
		row.Signature = object.Signature
		row.Visibility = object.Visibility
		row.OwnerRef = compilation.refBySubjectID[object.OwnerID]
		row.ContainerRef = compilation.refBySubjectID[object.ContainerID]
		row.Location = toLocationWire(object.Location)
		row.SeedKinds = append([]programindex.SeedKind(nil), seedKinds...)
		if object.External != nil {
			row.External = &externalWire{
				AuthorityKind: object.External.AuthorityKind,
				PackagePath:   object.External.PackagePath,
				Receiver:      object.External.Receiver,
				Name:          object.External.Name,
			}
		}
		for _, identity := range object.SymbolLinkIdentities {
			row.SymbolLinks = append(row.SymbolLinks, symbolLinkWire{
				Domain: identity.Domain, Display: identity.Display, PartCount: identity.PartCount,
			})
		}
		return row
	}
	pattern := subject.pattern
	relation := subject.relation
	row.PatternForm = pattern.Form
	row.Selector = pattern.Selector
	row.RelationKind = relation.Kind
	row.RelationResolution = relation.Resolution
	row.Invocation = relation.Invocation
	location := pattern.Location
	if location == nil {
		location = relation.Location
	}
	row.Location = toLocationWire(location)
	row.ResultRef = compilation.refBySubjectID[pattern.ResultID]
	row.ReceiverRef = compilation.refBySubjectID[pattern.ReceiverID]
	row.ReceiverOriginResolution = pattern.ReceiverOriginResolution
	row.ReceiverOriginsObserved = pattern.ReceiverOriginsObserved
	row.ReceiverOriginsOmitted = pattern.ReceiverOriginsOmitted
	for _, id := range pattern.ReceiverOriginIDs {
		row.ReceiverOriginRefs = append(row.ReceiverOriginRefs, compilation.refBySubjectID[id])
	}
	row.ArgumentsObserved = pattern.ArgumentsObserved
	row.ArgumentsOmitted = pattern.ArgumentsOmitted
	row.Arguments = make([]argumentWire, 0, len(pattern.Arguments))
	for _, argument := range pattern.Arguments {
		argumentRow := argumentWire{
			Ref: compilation.refByArgumentID[argument.ID], Position: argument.Position, Keyword: argument.Keyword,
			Kind: argument.Kind, Value: argument.Value,
			Parts:           append([]programindex.PatternPart(nil), argument.Parts...),
			Resolution:      argument.Resolution,
			ObjectsObserved: argument.ObjectsObserved, ObjectsOmitted: argument.ObjectsOmitted,
			ValueCandidatesObserved: argument.ValueCandidatesObserved,
			ValueCandidatesOmitted:  argument.ValueCandidatesOmitted,
		}
		if argumentRow.Parts == nil {
			argumentRow.Parts = []programindex.PatternPart{}
		}
		for _, id := range argument.ObjectIDs {
			argumentRow.ObjectRefs = append(argumentRow.ObjectRefs, compilation.refBySubjectID[id])
		}
		if argumentRow.ObjectRefs == nil {
			argumentRow.ObjectRefs = []string{}
		}
		argumentRow.ValueCandidates = make([]valueCandidateWire, 0, len(argument.ValueCandidates))
		for _, candidate := range argument.ValueCandidates {
			candidateRow := valueCandidateWire{
				Kind: candidate.Kind, Value: candidate.Value,
				Parts:      append([]programindex.PatternPart(nil), candidate.Parts...),
				Resolution: candidate.Resolution, SourceKind: candidate.SourceKind,
				SourceObjectsObserved:   candidate.SourceObjectsObserved,
				SourceObjectsOmitted:    candidate.SourceObjectsOmitted,
				SourceArgumentRefs:      []string{},
				SourceArgumentsObserved: candidate.SourceArgumentsObserved,
				SourceArgumentsOmitted:  candidate.SourceArgumentsOmitted,
			}
			if candidateRow.Parts == nil {
				candidateRow.Parts = []programindex.PatternPart{}
			}
			for _, id := range candidate.SourceObjectIDs {
				candidateRow.SourceObjectRefs = append(
					candidateRow.SourceObjectRefs, compilation.refBySubjectID[id],
				)
			}
			for _, id := range candidate.SourceArgumentIDs {
				candidateRow.SourceArgumentRefs = append(
					candidateRow.SourceArgumentRefs, compilation.refByArgumentID[id],
				)
			}
			if candidateRow.SourceObjectRefs == nil {
				candidateRow.SourceObjectRefs = []string{}
			}
			argumentRow.ValueCandidates = append(argumentRow.ValueCandidates, candidateRow)
		}
		row.Arguments = append(row.Arguments, argumentRow)
	}
	return row
}

func (compilation Compilation) edgeWire(edge graphEdge) edgeWire {
	row := edgeWire{
		Ref: edge.ref, Role: edge.role,
		FromRef: compilation.refBySubjectID[edge.fromID], ToRef: compilation.refBySubjectID[edge.toID],
		RelationKind: edge.relationKind, Resolution: edge.resolution, Invocation: edge.invocation,
		Location:        toLocationWire(edge.location),
		TargetsObserved: edge.targetsObserved, TargetsOmitted: edge.targetsOmitted,
		WitnessesObserved: edge.witnessesObserved, WitnessesOmitted: edge.witnessesOmitted,
	}
	for _, witness := range edge.witnesses {
		row.Witnesses = append(row.Witnesses, witnessWire{
			Kind: witness.Kind, Detail: witness.Detail, SourceExpression: witness.SourceExpression,
			Location: toLocationWire(witness.Location),
		})
	}
	if edge.sourceArgument != nil {
		row.SourceArgument = &sourceArgumentWire{
			PatternRef: compilation.refBySubjectID[edge.sourceArgument.patternID],
			Position:   edge.sourceArgument.position, Keyword: edge.sourceArgument.keyword,
		}
	}
	return row
}

func (compilation Compilation) attachCandidates(request *Request, candidates proposalSet) error {
	groups := append([]groupProposal(nil), candidates.groups...)
	sort.Slice(groups, func(i, j int) bool { return groupProposalKey(groups[i]) < groupProposalKey(groups[j]) })
	refByKey := make(map[string]string, len(groups))
	for position, group := range groups {
		ref := fmt.Sprintf("g%d", position+1)
		refByKey[group.Key] = ref
		row := candidateGroupWire{
			Ref: ref, Title: group.Title, Summary: group.Summary, Lane: group.Lane,
			MemberRefs:   make([]string, 0, len(group.MemberSubjectIDs)),
			EvidenceRefs: make([]string, 0, len(group.EvidenceSubjectIDs)),
		}
		for _, id := range group.MemberSubjectIDs {
			ref := compilation.refBySubjectID[id]
			if ref == "" {
				return fmt.Errorf("program grouping: merge group has unknown member subject")
			}
			row.MemberRefs = append(row.MemberRefs, ref)
		}
		for _, id := range group.EvidenceSubjectIDs {
			ref := compilation.refBySubjectID[id]
			if ref == "" {
				return fmt.Errorf("program grouping: merge group has unknown evidence subject")
			}
			row.EvidenceRefs = append(row.EvidenceRefs, ref)
		}
		request.CandidateGroups = append(request.CandidateGroups, row)
	}
	for _, connection := range candidates.connections {
		from := refByKey[connection.FromGroupKey]
		to := refByKey[connection.ToGroupKey]
		if from == "" || to == "" {
			continue
		}
		row := candidateConnectionWire{
			FromGroupRef: from, ToGroupRef: to,
			SemanticKind: connection.SemanticKind, Label: connection.Label, Summary: connection.Summary,
		}
		for _, id := range connection.EvidenceSubjectIDs {
			ref := compilation.refBySubjectID[id]
			if ref == "" {
				return fmt.Errorf("program grouping: merge connection has unknown evidence subject")
			}
			row.EvidenceRefs = append(row.EvidenceRefs, ref)
		}
		if row.EvidenceRefs == nil {
			row.EvidenceRefs = []string{}
		}
		request.CandidateConnections = append(request.CandidateConnections, row)
	}
	return nil
}

func groupProposalKey(value groupProposal) string {
	return strings.Join([]string{
		string(value.Lane), value.Title, value.Summary,
		strings.Join(value.MemberSubjectIDs, "\x01"), strings.Join(value.EvidenceSubjectIDs, "\x01"), value.Key,
	}, "\x00")
}

func toLocationWire(value *programindex.Location) *locationWire {
	if value == nil {
		return nil
	}
	return &locationWire{Path: value.Path, Line: value.Line, Column: value.Column}
}
