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
	if index := builder.graphIndex(section.programTargetID); index != nil {
		for _, operation := range index.Operations {
			if row, ok := byFact[operation.FactID]; ok && operation.Kind == "request" {
				row.OperationHrefs = append(row.OperationHrefs, "#"+operationNodeID(section.ID, operation.ID))
				byFact[operation.FactID] = row
				continue
			}
			row := pageGroupOperation{
				Name: builder.operationDisplayName(operation), Kind: operation.Kind, Summary: operation.Summary, Source: operation.Source,
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
		if fact.Anchor != nil && builder.testPaths[fact.Anchor.Path] {
			continue
		}
		routes = append(routes, byFact[fact.ID])
	}
	section.RouteGroups = groupRouteRows(routes)
}

type pageActivityGroup struct {
	Title string
	Rows  []pageGroupOperation
}

// NativeRouteCount counts the original HTTP registrations in this catalogue.
// Several operation links may describe one registration; unmatched model
// handlers remain separate request records, without inventing an identity join.
func (section *pageSection) NativeRouteCount() int {
	count := 0
	for _, group := range section.RouteGroups {
		count += group.Paths
	}
	return count
}

// Group existing, already localized operations for reading. This makes no new
// classification and adds no entries to the saved translation catalogue.
func (section *pageSection) ActivityGroups() []pageActivityGroup {
	groups := []pageActivityGroup{{Title: "Commands"}, {Title: "Background work"}, {Title: "User interactions"}, {Title: "Other operations"}}
	for _, row := range section.Activities {
		i := 3
		switch row.Kind {
		case "command":
			i = 0
		case "scheduled", "continuous":
			i = 1
		case "interaction":
			i = 2
		}
		groups[i].Rows = append(groups[i].Rows, row)
	}
	return groups
}

// Parts are the component's own leaf groups on the map, in every lane.
func (section *pageSection) PartsCount() int {
	count := 0
	if section.Map != nil {
		for _, node := range section.Map.Nodes {
			if !node.Remote && node.Branch == "" && node.Activation == "" {
				count++
			}
		}
	}
	return count
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
		{section.PartsCount() == 0, "Parts"},
		{len(section.Outbound) == 0, "External communication"},
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
