package report

import (
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/workspacesearch"
)

const (
	maxReportExactPaths           = 4096
	maxReportExactPathBytes       = 4096
	maxReportExactComponents      = 512
	maxReportExactSymbols         = 1024
	maxReportExactMemberships     = 8192
	maxReportExactFactsPerMember  = 16
	maxReportExactEntityIDBytes   = 512
	maxReportExactEntityNameBytes = 256
)

// BuildSemanticSearchIndexWithCatalog adapts neutral exact workspace facts
// into the existing versioned report projection. Editorial search ownership,
// IDs, targets, ranking inputs, and suggestions remain in this package.
func BuildSemanticSearchIndexWithCatalog(
	data *ReportData,
	catalog sourcecatalog.Catalog,
) (SemanticSearchIndex, error) {
	if data == nil {
		return SemanticSearchIndex{}, fmt.Errorf("semantic search: report data is required")
	}
	// Reject incompatible canonical input before the exact adapter applies any
	// path, component, member, or membership bounds. A historical Canvas must
	// never be reported as a current-shape capacity failure.
	if err := validateSemanticSearchCanvasVersion(data.ArchitectureCanvas); err != nil {
		return SemanticSearchIndex{}, err
	}
	exact, err := newReportExactSearch(data, catalog)
	if err != nil {
		return SemanticSearchIndex{}, err
	}
	return buildSemanticSearchIndex(data, exact)
}

// AttachExactWorkspaceSearch replaces only the derived search projection of a
// coherent report. It does not change report or manifest authority.
func AttachExactWorkspaceSearch(data *ReportData, catalog sourcecatalog.Catalog) error {
	if data == nil {
		return fmt.Errorf("semantic search: report data is required")
	}
	if data.SemanticSearchDisabled {
		data.SemanticSearch = nil
		return nil
	}
	index, err := BuildSemanticSearchIndexWithCatalog(data, catalog)
	if err != nil {
		return err
	}
	data.SemanticSearch = &index
	return nil
}

func authorizedExactSearchCatalog(
	data *ReportData,
	authority RunAuthority,
) (sourcecatalog.Catalog, bool, error) {
	if authority.inputs == nil {
		return sourcecatalog.Catalog{}, false, nil
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: authority.repository.Identity,
		AnalysisRoot:   authority.analysisRoot,
		AllowedPaths:   data.OpenablePaths,
		CapturedInputs: authority.inputs,
	})
	if err != nil {
		return sourcecatalog.Catalog{}, false, fmt.Errorf("semantic search: authorized source catalog: %w", err)
	}
	return catalog, true, nil
}

type reportExactSearch struct {
	entries []workspacesearch.Entry
	members []reportExactMember
}

type reportExactMember struct {
	member componentmap.Candidate
}

func newReportExactSearch(data *ReportData, catalog sourcecatalog.Catalog) (*reportExactSearch, error) {
	if data == nil {
		return nil, fmt.Errorf("semantic search: report data is required")
	}
	if len(data.OpenablePaths) > maxReportExactPaths {
		return nil, fmt.Errorf("semantic search: exact path input exceeds %d entries", maxReportExactPaths)
	}
	if catalog.Len() > maxReportExactPaths {
		return nil, fmt.Errorf("semantic search: source catalog exceeds %d entries", maxReportExactPaths)
	}
	if catalog.Len() != len(data.OpenablePaths) {
		return nil, fmt.Errorf("semantic search: source catalog does not match openable paths")
	}
	for index, sourcePath := range data.OpenablePaths {
		if len(sourcePath) > maxReportExactPathBytes {
			return nil, fmt.Errorf(
				"semantic search: exact path %d exceeds %d bytes",
				index,
				maxReportExactPathBytes,
			)
		}
	}
	catalogPaths := catalog.Paths()
	for index, sourcePath := range catalogPaths {
		if len(sourcePath) > maxReportExactPathBytes {
			return nil, fmt.Errorf(
				"semantic search: catalog path %d exceeds %d bytes",
				index,
				maxReportExactPathBytes,
			)
		}
	}
	if !slices.Equal(catalogPaths, data.OpenablePaths) {
		return nil, fmt.Errorf("semantic search: source catalog does not match openable paths")
	}

	symbols := make([]evidence.Entity, 0, maxReportExactSymbols)
	exactMembers := make([]reportExactMember, 0, maxReportExactSymbols)
	if canvas := data.ArchitectureCanvas; canvas != nil {
		if len(canvas.Components) > maxReportExactComponents {
			return nil, fmt.Errorf("semantic search: exact component input exceeds %d entries", maxReportExactComponents)
		}
		type memberAccumulator struct {
			member     componentmap.Candidate
			components map[componentmap.ComponentID]struct{}
		}
		members := make(map[componentmap.MemberID]*memberAccumulator)
		membershipCount := 0
		for _, component := range canvas.Components {
			for _, member := range component.Members {
				membershipCount++
				if membershipCount > maxReportExactMemberships {
					return nil, fmt.Errorf(
						"semantic search: exact membership input exceeds %d entries",
						maxReportExactMemberships,
					)
				}
				accumulator := members[member.ID]
				if accumulator == nil {
					if len(members) == maxReportExactSymbols {
						return nil, fmt.Errorf("semantic search: exact member input exceeds %d entries", maxReportExactSymbols)
					}
					accumulator = &memberAccumulator{
						member:     member,
						components: make(map[componentmap.ComponentID]struct{}),
					}
					members[member.ID] = accumulator
				} else if !reflect.DeepEqual(accumulator.member, member) {
					return nil, fmt.Errorf("semantic search: shared exact member %q changed across memberships", member.ID.Value)
				}
				if _, duplicate := accumulator.components[component.ID]; duplicate {
					return nil, fmt.Errorf(
						"semantic search: component %q repeats exact member %q",
						component.ID,
						member.ID.Value,
					)
				}
				accumulator.components[component.ID] = struct{}{}
			}
		}
		memberIDs := make([]componentmap.MemberID, 0, len(members))
		for memberID := range members {
			memberIDs = append(memberIDs, memberID)
		}
		sort.Slice(memberIDs, func(i, j int) bool {
			if memberIDs[i].Kind != memberIDs[j].Kind {
				return memberIDs[i].Kind < memberIDs[j].Kind
			}
			return memberIDs[i].Value < memberIDs[j].Value
		})
		for _, memberID := range memberIDs {
			accumulator := members[memberID]
			if accumulator == nil {
				return nil, fmt.Errorf("semantic search: exact member %q has no accumulator", memberID.Value)
			}
			entity, ok := exactMemberEntity(accumulator.member)
			if ok {
				symbols = append(symbols, entity)
			}
			exactMembers = append(exactMembers, reportExactMember{
				member: accumulator.member,
			})
		}
	}

	index, err := workspacesearch.New(workspacesearch.Input{
		Catalog: catalog,
		Symbols: symbols,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic search: exact workspace: %w", err)
	}
	entries := index.Entries()
	exact := &reportExactSearch{
		entries: entries,
		members: exactMembers,
	}
	return exact, nil
}

func (builder *semanticSearchBuilder) addExactWorkspace(exact *reportExactSearch, includeArchitecture bool) {
	if exact == nil {
		return
	}
	for _, entry := range exact.entries {
		if entry.Kind == workspacesearch.KindSymbol {
			continue
		}
		location := evidence.Location{Path: entry.Path}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindLocation, entry.Path),
			Kind:      SemanticSearchKindLocation,
			Title:     entry.Path,
			Stability: SemanticSearchStabilityExact,
			Target: SemanticSearchTarget{
				Kind:     SemanticSearchTargetLocation,
				Location: &location,
			},
		}, 100)
	}
	if !includeArchitecture || builder.data.ArchitectureCanvas == nil {
		return
	}
	for _, exactMember := range exact.members {
		builder.addMemberWithoutOwner(exactMember.member)
	}
}

func exactMemberEntity(member componentmap.Candidate) (evidence.Entity, bool) {
	if len(member.Facts) > maxReportExactFactsPerMember {
		return evidence.Entity{}, false
	}
	var (
		kind     evidence.EntityKind
		idPrefix string
	)
	switch member.ID.Kind {
	case componentmap.MemberSymbol:
		kind = evidence.EntityReference
		idPrefix = "symbol:"
	case componentmap.MemberEntrypoint:
		kind = evidence.EntityEntrypoint
		idPrefix = "entrypoint:"
	default:
		return evidence.Entity{}, false
	}
	if len(member.Name) > maxReportExactEntityNameBytes ||
		len(member.ID.Value) > maxReportExactEntityIDBytes-len(idPrefix) {
		return evidence.Entity{}, false
	}
	for _, fact := range member.Facts {
		if fact.Kind != componentmap.FactDeclaration || fact.Location == nil {
			continue
		}
		location := *fact.Location
		return evidence.Entity{
			ID:       idPrefix + member.ID.Value,
			Kind:     kind,
			Name:     member.Name,
			Scope:    evidence.SourceScopeRepository,
			Location: &location,
		}, true
	}
	return evidence.Entity{}, false
}

func exactMemberSearchLocation(member componentmap.Candidate) *evidence.Location {
	// Keep source targeting under the same bounded fact contract used by the
	// exact workspace entity projection. The member remains searchable when a
	// producer supplies an oversized fact list, but it must not gain a location
	// target by scanning input that the exact adapter declined to admit.
	if len(member.Facts) > maxReportExactFactsPerMember {
		return nil
	}
	locations := make(map[evidence.Location]struct{}, len(member.Facts))
	for _, fact := range member.Facts {
		if fact.Location != nil && fact.Location.Path != "" {
			locations[*fact.Location] = struct{}{}
		}
	}
	if len(locations) != 1 {
		return nil
	}
	for location := range locations {
		copy := location
		return &copy
	}
	return nil
}
