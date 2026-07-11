// Package symbol builds compact, bounded evidence bundles for investigating one
// resolved repository symbol. Bundles contain facts, not raw analyzer graphs.
package symbol

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/dvordrova/repomap/internal/evidence"
)

const BundleVersion = 1

type Options struct {
	MaxCandidates        int
	MaxIncomingCalls     int
	MaxOutgoingCalls     int
	MaxProvenancePerFact int
}

type Bundle struct {
	Version       int            `json:"version"`
	RepoName      string         `json:"repo_name"`
	Query         string         `json:"query"`
	Target        Fact           `json:"target"`
	Candidates    []Fact         `json:"candidates"`
	IncomingCalls []CallFact     `json:"incoming_calls"`
	OutgoingCalls []CallFact     `json:"outgoing_calls"`
	Scenarios     []Scenario     `json:"scenarios"`
	AllowedPaths  []string       `json:"allowed_paths"`
	Warnings      []string       `json:"warnings"`
	Truncated     map[string]int `json:"truncated,omitempty"`
}

type Fact struct {
	EvidenceID string                `json:"evidence_id"`
	Entity     evidence.Entity       `json:"entity"`
	Certainty  evidence.Certainty    `json:"certainty"`
	Provenance []evidence.Provenance `json:"provenance"`
	Scenarios  []string              `json:"scenarios,omitempty"`
}

type CallFact struct {
	EvidenceID string                `json:"evidence_id"`
	Caller     evidence.Entity       `json:"caller"`
	Callee     evidence.Entity       `json:"callee"`
	Callsite   *evidence.Location    `json:"callsite,omitempty"`
	Certainty  evidence.Certainty    `json:"certainty"`
	Provenance []evidence.Provenance `json:"provenance"`
	Scenarios  []string              `json:"scenarios,omitempty"`
}

// Scenario intentionally omits environment variables, commands, and absolute
// working directories because they are unnecessary for model interpretation
// and may contain local or sensitive information.
type Scenario struct {
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	Build evidence.BuildContext `json:"build"`
}

func Build(graph evidence.Graph, opts Options) (Bundle, error) {
	if err := graph.Validate(); err != nil {
		return Bundle{}, fmt.Errorf("symbol: invalid evidence graph: %w", err)
	}
	opts = defaultOptions(opts)
	entities := make(map[string]evidence.Entity, len(graph.Entities))
	for _, entity := range graph.Entities {
		entities[entity.ID] = entity
	}

	resolutionRelations := relationsByKind(graph.Relations, evidence.RelationResolvesTo)
	if len(resolutionRelations) == 0 {
		return Bundle{}, fmt.Errorf("symbol: query %q has no unique exact resolution", graph.Query)
	}
	if len(resolutionRelations) > 1 {
		return Bundle{}, fmt.Errorf("symbol: query %q has %d exact resolutions", graph.Query, len(resolutionRelations))
	}
	resolution := resolutionRelations[0]
	target, ok := entities[resolution.To]
	if !ok {
		return Bundle{}, fmt.Errorf("symbol: resolved entity %q is missing", resolution.To)
	}
	if target.Location == nil || target.Location.Path == "" || target.Location.Line <= 0 {
		return Bundle{}, fmt.Errorf("symbol: resolved entity %q has no usable location", target.Name)
	}

	bundle := Bundle{
		Version:   BundleVersion,
		RepoName:  filepath.Base(filepath.Clean(graph.RepoPath)),
		Query:     graph.Query,
		Target:    factFromRelation("resolution-001", target, resolution, opts.MaxProvenancePerFact),
		Warnings:  append([]string{}, graph.Warnings...),
		Truncated: make(map[string]int),
	}
	bundle.Warnings = append(bundle.Warnings,
		"the bundle contains static analysis only; it does not prove which calls execute at runtime",
		"fuzzy candidates are possible matches; only target is the unique exact resolution",
	)

	for _, scenario := range graph.Scenarios {
		bundle.Scenarios = append(bundle.Scenarios, Scenario{
			ID:    scenario.ID,
			Name:  scenario.Name,
			Build: scenario.Build,
		})
	}

	matchRelations := relationsByKind(graph.Relations, evidence.RelationMatchesQuery)
	matchRelations = prioritizeTarget(matchRelations, target.ID)
	bundle.Candidates, bundle.Truncated["candidates"] = buildCandidates(matchRelations, entities, opts)

	var incoming []evidence.Relation
	var outgoing []evidence.Relation
	for _, relation := range graph.Relations {
		if relation.Kind != evidence.RelationCalls {
			continue
		}
		if relation.To == target.ID {
			incoming = append(incoming, relation)
		}
		if relation.From == target.ID {
			outgoing = append(outgoing, relation)
		}
	}
	sortRelations(incoming)
	sortRelations(outgoing)
	bundle.IncomingCalls, bundle.Truncated["incoming_calls"] = buildCalls("call-in", incoming, entities, opts.MaxIncomingCalls, opts.MaxProvenancePerFact)
	bundle.OutgoingCalls, bundle.Truncated["outgoing_calls"] = buildCalls("call-out", outgoing, entities, opts.MaxOutgoingCalls, opts.MaxProvenancePerFact)

	for key, count := range bundle.Truncated {
		if count == 0 {
			delete(bundle.Truncated, key)
		}
	}
	bundle.AllowedPaths = collectAllowedPaths(bundle)
	return bundle, nil
}

func defaultOptions(opts Options) Options {
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = 12
	}
	if opts.MaxIncomingCalls <= 0 {
		opts.MaxIncomingCalls = 30
	}
	if opts.MaxOutgoingCalls <= 0 {
		opts.MaxOutgoingCalls = 30
	}
	if opts.MaxProvenancePerFact <= 0 {
		opts.MaxProvenancePerFact = 3
	}
	return opts
}

func relationsByKind(relations []evidence.Relation, kind evidence.RelationKind) []evidence.Relation {
	var selected []evidence.Relation
	for _, relation := range relations {
		if relation.Kind == kind {
			selected = append(selected, relation)
		}
	}
	sortRelations(selected)
	return selected
}

func prioritizeTarget(relations []evidence.Relation, targetID string) []evidence.Relation {
	prioritized := make([]evidence.Relation, 0, len(relations))
	for _, relation := range relations {
		if relation.To == targetID {
			prioritized = append(prioritized, relation)
		}
	}
	for _, relation := range relations {
		if relation.To != targetID {
			prioritized = append(prioritized, relation)
		}
	}
	return prioritized
}

func buildCandidates(relations []evidence.Relation, entities map[string]evidence.Entity, opts Options) ([]Fact, int) {
	limit := min(len(relations), opts.MaxCandidates)
	facts := make([]Fact, 0, limit)
	for i, relation := range relations[:limit] {
		entity, ok := entities[relation.To]
		if !ok {
			continue
		}
		facts = append(facts, factFromRelation(fmt.Sprintf("candidate-%03d", i+1), entity, relation, opts.MaxProvenancePerFact))
	}
	return facts, len(relations) - limit
}

func factFromRelation(id string, entity evidence.Entity, relation evidence.Relation, maxProvenance int) Fact {
	return Fact{
		EvidenceID: id,
		Entity:     entity,
		Certainty:  relation.Certainty,
		Provenance: limitProvenance(relation.Provenance, maxProvenance),
		Scenarios:  append([]string{}, relation.Scenarios...),
	}
}

func buildCalls(prefix string, relations []evidence.Relation, entities map[string]evidence.Entity, maxCalls, maxProvenance int) ([]CallFact, int) {
	limit := min(len(relations), maxCalls)
	facts := make([]CallFact, 0, limit)
	for i, relation := range relations[:limit] {
		caller, callerOK := entities[relation.From]
		callee, calleeOK := entities[relation.To]
		if !callerOK || !calleeOK {
			continue
		}
		provenance := limitProvenance(relation.Provenance, maxProvenance)
		var callsite *evidence.Location
		for _, source := range provenance {
			if source.Location != nil {
				location := *source.Location
				callsite = &location
				break
			}
		}
		facts = append(facts, CallFact{
			EvidenceID: fmt.Sprintf("%s-%03d", prefix, i+1),
			Caller:     caller,
			Callee:     callee,
			Callsite:   callsite,
			Certainty:  relation.Certainty,
			Provenance: provenance,
			Scenarios:  append([]string{}, relation.Scenarios...),
		})
	}
	return facts, len(relations) - limit
}

func limitProvenance(provenance []evidence.Provenance, limit int) []evidence.Provenance {
	if len(provenance) > limit {
		provenance = provenance[:limit]
	}
	return append([]evidence.Provenance{}, provenance...)
}

func sortRelations(relations []evidence.Relation) {
	sort.Slice(relations, func(i, j int) bool {
		if relations[i].From != relations[j].From {
			return relations[i].From < relations[j].From
		}
		if relations[i].To != relations[j].To {
			return relations[i].To < relations[j].To
		}
		return relations[i].Kind < relations[j].Kind
	})
}

func collectAllowedPaths(bundle Bundle) []string {
	paths := make(map[string]struct{})
	addEntityPath(paths, bundle.Target.Entity)
	for _, candidate := range bundle.Candidates {
		addEntityPath(paths, candidate.Entity)
	}
	for _, call := range append(append([]CallFact{}, bundle.IncomingCalls...), bundle.OutgoingCalls...) {
		addEntityPath(paths, call.Caller)
		addEntityPath(paths, call.Callee)
		if call.Callsite != nil && call.Callsite.Path != "" {
			paths[call.Callsite.Path] = struct{}{}
		}
	}

	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func addEntityPath(paths map[string]struct{}, entity evidence.Entity) {
	if entity.Location != nil && entity.Location.Path != "" {
		paths[entity.Location.Path] = struct{}{}
	}
}
