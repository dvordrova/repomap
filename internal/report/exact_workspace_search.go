package report

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

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
	entries  []workspacesearch.Entry
	entities map[string]struct{}
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
	if canvas := data.ArchitectureCanvas; canvas != nil {
		if len(canvas.Components) > maxReportExactComponents {
			return nil, fmt.Errorf("semantic search: exact component input exceeds %d entries", maxReportExactComponents)
		}
		rawCandidates := 0
		for _, component := range canvas.Components {
			if len(component.Members) > maxReportExactSymbols-rawCandidates {
				return nil, fmt.Errorf("semantic search: exact candidate input exceeds %d entries", maxReportExactSymbols)
			}
			rawCandidates += len(component.Members)
			for _, member := range component.Members {
				entity, ok := exactMemberEntity(member)
				if !ok {
					continue
				}
				if len(symbols) == maxReportExactSymbols {
					return nil, fmt.Errorf("semantic search: exact symbol input exceeds %d entries", maxReportExactSymbols)
				}
				symbols = append(symbols, entity)
			}
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
		entries:  entries,
		entities: make(map[string]struct{}, min(len(symbols), maxReportExactSymbols)),
	}
	for _, entry := range entries {
		if entry.Kind == workspacesearch.KindSymbol && entry.Entity != nil {
			exact.entities[exactEntityKey(*entry.Entity)] = struct{}{}
		}
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
	for _, component := range builder.data.ArchitectureCanvas.Components {
		for _, member := range component.Members {
			if exact.acceptsMember(member) {
				builder.addMember(component, member)
			}
		}
	}
}

func (exact *reportExactSearch) acceptsMember(member componentmap.Candidate) bool {
	if exact == nil {
		return false
	}
	entity, ok := exactMemberEntity(member)
	if !ok {
		return false
	}
	_, ok = exact.entities[exactEntityKey(entity)]
	return ok
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

func exactEntityKey(entity evidence.Entity) string {
	if entity.Location == nil {
		return ""
	}
	location := entity.Location
	return strings.Join([]string{
		entity.ID,
		string(entity.Kind),
		entity.Name,
		location.Path,
		strconv.Itoa(location.Line),
		strconv.Itoa(location.Column),
		strconv.Itoa(location.EndLine),
		strconv.Itoa(location.EndColumn),
	}, "\x00")
}
