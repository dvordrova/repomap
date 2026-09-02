package programcategorization

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

type targetWire struct {
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Selector string `json:"selector,omitempty"`
}

type subjectWire struct {
	Ref               string                             `json:"ref"`
	Kind              subjectKind                        `json:"kind"`
	ObjectKind        programindex.ObjectKind            `json:"object_kind,omitempty"`
	Name              string                             `json:"name,omitempty"`
	Signature         string                             `json:"signature,omitempty"`
	Visibility        programindex.Visibility            `json:"visibility,omitempty"`
	Path              string                             `json:"path,omitempty"`
	Line              int                                `json:"line,omitempty"`
	Column            int                                `json:"column,omitempty"`
	ExternalPackage   string                             `json:"external_package,omitempty"`
	ExternalAuthority programindex.ExternalAuthorityKind `json:"external_authority_kind,omitempty"`
	ExternalSymbol    string                             `json:"external_symbol,omitempty"`
	SeedKinds         []programindex.SeedKind            `json:"seed_kinds,omitempty"`
	PatternForm       programindex.PatternForm           `json:"pattern_form,omitempty"`
	Selector          string                             `json:"selector,omitempty"`
	RelationKind      programindex.RelationKind          `json:"relation_kind,omitempty"`
	Arguments         []argumentWire                     `json:"arguments,omitempty"`
	// AllowedCategories appears only when this subject cannot carry every
	// category. A standard-library symbol is not an outbound dependency, and
	// stating that here keeps the wrong answer out of the response instead of
	// discarding it afterwards.
	AllowedCategories []Category `json:"allowed_categories,omitempty"`
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

type edgeWire struct {
	Ref          string                    `json:"ref"`
	Kind         string                    `json:"kind"`
	FromRef      string                    `json:"from_ref"`
	ToRef        string                    `json:"to_ref"`
	RelationKind programindex.RelationKind `json:"relation_kind,omitempty"`
	Resolution   programindex.Resolution   `json:"resolution"`
	Path         string                    `json:"path,omitempty"`
	Line         int                       `json:"line,omitempty"`
	Column       int                       `json:"column,omitempty"`
}

type documentationWire struct {
	Ref        string            `json:"ref"`
	Path       string            `json:"path,omitempty"`
	SourceKind string            `json:"source_kind,omitempty"`
	Kind       documentationKind `json:"kind"`
	Text       string            `json:"text"`
}

// Request is deliberately request-local: no ProgramIndex identity or digest
// is provider-visible.
type Request struct {
	Version        int                 `json:"version"`
	Target         targetWire          `json:"target"`
	CategorizeRefs []string            `json:"categorize_refs"`
	Subjects       []subjectWire       `json:"subjects"`
	Edges          []edgeWire          `json:"edges"`
	Documentation  []documentationWire `json:"documentation"`
}

type responseAssignment struct {
	Ref        string   `json:"ref"`
	Categories []string `json:"categories"`
}

func (compilation Compilation) request(subjectRefs, documentationRefs []string) (Request, error) {
	ownedIDs := make(map[string]struct{}, len(subjectRefs))
	for _, ref := range subjectRefs {
		subject, known := compilation.subjectByRef[ref]
		if !known {
			return Request{}, fmt.Errorf("program categorization: request cites unknown subject ref %q", ref)
		}
		if _, duplicate := ownedIDs[subject.id]; duplicate {
			return Request{}, fmt.Errorf("program categorization: request repeats subject ref %q", ref)
		}
		ownedIDs[subject.id] = struct{}{}
	}

	contextIDs := make(map[string]struct{}, len(ownedIDs))
	for id := range ownedIDs {
		contextIDs[id] = struct{}{}
	}
	edges := make([]graphEdge, 0)
	for _, edge := range compilation.edges {
		_, fromOwned := ownedIDs[edge.fromID]
		_, toOwned := ownedIDs[edge.toID]
		if !fromOwned && !toOwned {
			continue
		}
		edges = append(edges, edge)
		contextIDs[edge.fromID] = struct{}{}
		contextIDs[edge.toID] = struct{}{}
	}
	compilation.expandReferencedContext(contextIDs)

	seedKinds := make(map[string][]programindex.SeedKind)
	for _, seed := range compilation.index.Target.Seeds {
		seedKinds[seed.ObjectID] = append(seedKinds[seed.ObjectID], seed.Kind)
	}
	for objectID := range seedKinds {
		sort.Slice(seedKinds[objectID], func(i, j int) bool {
			return seedKinds[objectID][i] < seedKinds[objectID][j]
		})
	}

	request := Request{
		Version: requestVersion,
		Target: targetWire{
			Language: compilation.index.Target.Language,
			Kind:     compilation.index.Target.Kind,
			Name:     compilation.index.Target.Name,
			Selector: compilation.index.Target.Selector,
		},
		CategorizeRefs: append([]string(nil), subjectRefs...),
		Subjects:       make([]subjectWire, 0, len(contextIDs)),
		Edges:          make([]edgeWire, 0, len(edges)),
		Documentation:  make([]documentationWire, 0, len(documentationRefs)),
	}
	for _, subject := range compilation.subjects {
		if _, include := contextIDs[subject.id]; !include {
			continue
		}
		row := subjectWire{Ref: subject.ref, Kind: subject.kind}
		row.AllowedCategories = restrictedCategories(compilation.index, subject.id)
		if subject.object != nil {
			row.ObjectKind = subject.object.Kind
			row.Name = subject.object.Name
			row.Signature = subject.object.Signature
			row.Visibility = subject.object.Visibility
			row.SeedKinds = append([]programindex.SeedKind(nil), seedKinds[subject.object.ID]...)
			if subject.object.Location != nil {
				row.Path = subject.object.Location.Path
				row.Line = subject.object.Location.Line
				row.Column = subject.object.Location.Column
			}
			if subject.object.External != nil {
				row.ExternalPackage = subject.object.External.PackagePath
				row.ExternalAuthority = subject.object.External.AuthorityKind
				row.ExternalSymbol = subject.object.External.Name
				if subject.object.External.Receiver != "" {
					row.ExternalSymbol = strings.TrimSpace(subject.object.External.Receiver + "." + row.ExternalSymbol)
				}
			}
		} else {
			row.PatternForm = subject.pattern.Form
			row.Selector = subject.pattern.Selector
			row.RelationKind = subject.relation.Kind
			location := subject.pattern.Location
			if location == nil {
				location = subject.relation.Location
			}
			if location != nil {
				row.Path = location.Path
				row.Line = location.Line
				row.Column = location.Column
			}
			row.Arguments = make([]argumentWire, 0, len(subject.pattern.Arguments))
			for _, argument := range subject.pattern.Arguments {
				argumentRef := compilation.refByArgumentID[argument.ID]
				if argumentRef == "" {
					return Request{}, fmt.Errorf("program categorization: pattern argument has no request-local ref")
				}
				argumentRow := argumentWire{
					Ref: argumentRef, Position: argument.Position, Keyword: argument.Keyword,
					Kind: argument.Kind, Value: argument.Value,
					Parts:           append([]programindex.PatternPart(nil), argument.Parts...),
					Resolution:      argument.Resolution,
					ObjectsObserved: argument.ObjectsObserved, ObjectsOmitted: argument.ObjectsOmitted,
					ValueCandidatesObserved: argument.ValueCandidatesObserved,
					ValueCandidatesOmitted:  argument.ValueCandidatesOmitted,
					ObjectRefs:              []string{}, ValueCandidates: []valueCandidateWire{},
				}
				if argumentRow.Parts == nil {
					argumentRow.Parts = []programindex.PatternPart{}
				}
				for _, objectID := range argument.ObjectIDs {
					ref := compilation.refBySubjectID[objectID]
					if ref == "" {
						return Request{}, fmt.Errorf("program categorization: argument source object has no request-local ref")
					}
					argumentRow.ObjectRefs = append(argumentRow.ObjectRefs, ref)
				}
				for _, candidate := range argument.ValueCandidates {
					candidateRow := valueCandidateWire{
						Kind: candidate.Kind, Value: candidate.Value,
						Parts:      append([]programindex.PatternPart(nil), candidate.Parts...),
						Resolution: candidate.Resolution, SourceKind: candidate.SourceKind,
						SourceObjectRefs: []string{}, SourceArgumentRefs: []string{},
						SourceObjectsObserved:   candidate.SourceObjectsObserved,
						SourceObjectsOmitted:    candidate.SourceObjectsOmitted,
						SourceArgumentsObserved: candidate.SourceArgumentsObserved,
						SourceArgumentsOmitted:  candidate.SourceArgumentsOmitted,
					}
					if candidateRow.Parts == nil {
						candidateRow.Parts = []programindex.PatternPart{}
					}
					for _, objectID := range candidate.SourceObjectIDs {
						ref := compilation.refBySubjectID[objectID]
						if ref == "" {
							return Request{}, fmt.Errorf("program categorization: candidate source object has no request-local ref")
						}
						candidateRow.SourceObjectRefs = append(candidateRow.SourceObjectRefs, ref)
					}
					for _, sourceArgumentID := range candidate.SourceArgumentIDs {
						ref := compilation.refByArgumentID[sourceArgumentID]
						if ref == "" {
							return Request{}, fmt.Errorf("program categorization: candidate source argument has no request-local ref")
						}
						candidateRow.SourceArgumentRefs = append(candidateRow.SourceArgumentRefs, ref)
					}
					argumentRow.ValueCandidates = append(argumentRow.ValueCandidates, candidateRow)
				}
				row.Arguments = append(row.Arguments, argumentRow)
			}
		}
		request.Subjects = append(request.Subjects, row)
	}
	for _, edge := range edges {
		row := edgeWire{
			Ref: edge.ref, Kind: edge.kind,
			FromRef:      compilation.refBySubjectID[edge.fromID],
			ToRef:        compilation.refBySubjectID[edge.toID],
			RelationKind: edge.relationKind, Resolution: edge.resolution,
		}
		if edge.location != nil {
			row.Path = edge.location.Path
			row.Line = edge.location.Line
			row.Column = edge.location.Column
		}
		request.Edges = append(request.Edges, row)
	}
	seenDocumentation := make(map[string]struct{}, len(documentationRefs))
	for _, ref := range documentationRefs {
		document, known := compilation.documentationByRef[ref]
		if !known {
			return Request{}, fmt.Errorf("program categorization: request cites unknown documentation ref %q", ref)
		}
		if _, duplicate := seenDocumentation[ref]; duplicate {
			return Request{}, fmt.Errorf("program categorization: request repeats documentation ref %q", ref)
		}
		seenDocumentation[ref] = struct{}{}
		request.Documentation = append(request.Documentation, documentationWire{
			Ref: ref, Path: document.path, SourceKind: document.sourceKind,
			Kind: document.kind, Text: document.text,
		})
	}
	return request, nil
}

// expandReferencedContext keeps every nested request-local ref resolvable.
// In particular an actual-argument candidate brings the owning source pattern
// into the same request without exposing either canonical argument identity.
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
							authority, known := compilation.argumentByID[sourceArgumentID]
							if known {
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

// restrictedCategories lists what a subject may carry, but only when the full
// closed set is not available to it. An empty result means every category is
// allowed and the request stays silent about it.
func restrictedCategories(index programindex.Index, subjectID string) []Category {
	allowed := make([]Category, 0, len(allCategories))
	for _, category := range allCategories {
		if programindex.CategorySupported(index, subjectID, category) {
			allowed = append(allowed, category)
		}
	}
	if len(allowed) == len(allCategories) {
		return nil
	}
	return allowed
}

// allCategories is the closed vocabulary in its canonical order.
var allCategories = []Category{
	programindex.CategoryInbound,
	programindex.CategoryBackgroundActivity,
	programindex.CategoryCore,
	programindex.CategoryDependency,
}
