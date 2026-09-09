package report

import (
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
)

// The operation is the interpretation; FactID identifies its exact observed
// route. A route outside the map is retained, never assigned to a nearby group.
func (builder *pageBuilder) fillSectionOperations(section *pageSection) {
	var routes []pageHTTPRow
	byFact := make(map[string]pageHTTPRow)
	var routeFacts []facts.Fact
	if section.FactsAvailable {
		routeFacts = builder.targetFacts(section.factsTargetID, facts.KindHTTPRoute)
		rows := builder.httpRows(facts.KindHTTPRoute, section.factsTargetID)
		for i, fact := range routeFacts {
			byFact[fact.ID] = rows[i]
		}
	}
	seen := make(map[string]bool)
	if index := builder.graphIndex(section.programTargetID); index != nil {
		for _, operation := range index.Operations {
			if row, ok := byFact[operation.FactID]; ok && operation.Kind == "request" {
				if !seen[operation.FactID] {
					routes = append(routes, row)
					seen[operation.FactID] = true
				}
				continue
			}
			row := pageGroupOperation{
				Name: operation.Name, Kind: operation.Kind, Summary: operation.Summary, Source: operation.Source,
				Href:   "#" + operationNodeID(section.ID, operation.ID),
				Anchor: builder.links.anchor(operation.Location.Path, operation.Location.Line, operation.Location.Column),
			}
			if operation.Kind == "request" {
				section.Requests = append(section.Requests, row)
			} else {
				section.Activities = append(section.Activities, row)
			}
		}
	}
	for _, fact := range routeFacts {
		if !seen[fact.ID] {
			routes = append(routes, byFact[fact.ID])
		}
	}
	section.RouteGroups = groupRouteRows(routes)
}

// Empty observations belong in one coverage disclosure, not in a succession
// of empty catalogues. Missing evidence never becomes proof of absence.
func sectionCoverage(section *pageSection) []string {
	if !section.FactsAvailable {
		return []string{"Repository facts were not available."}
	}
	var missing []string
	for _, item := range []struct {
		empty bool
		label string
	}{
		{section.InboundCount == 0, "Incoming requests"},
		{len(section.Activities) == 0, "Other operations"},
		{len(section.Entrypoints) == 0, "Entrypoints"},
		{len(section.Core) == 0, "Core"},
		{len(section.Calls)+len(section.DependencyGroups)+len(section.Dependencies) == 0, "External calls & dependencies"},
		{len(section.Dynamic) == 0, "Runs code it is given"},
		{len(section.Config) == 0, "Configuration"},
		{len(section.Todos) == 0, "TODOs"},
	} {
		if item.empty {
			missing = append(missing, item.label)
		}
	}
	return missing
}

func shortEntrypointName(name string) string {
	// Drop only the package prefix. The exact identity remains on the fact and
	// the source link; receiver and declaration spellings remain unchanged.
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	return name
}

func recipeBasis(refs []string, byID map[string]facts.Fact) string {
	for _, ref := range refs {
		if byID[ref].Kind == facts.KindEntrypoint {
			return "Inferred from an entrypoint"
		}
	}
	return "Inferred from manifest settings"
}
