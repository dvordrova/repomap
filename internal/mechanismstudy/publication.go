package mechanismstudy

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// PublicationCard is a validated neutral seam, not a report DTO. Every field
// is excluded from direct JSON marshaling so private canonical identities can
// only reach a report through an explicit backend-owned projection.
type PublicationCard struct {
	StudyOrdinal     int                    `json:"-"`
	StudyCanonicalID string                 `json:"-"`
	Outcome          OutcomeState           `json:"-"`
	ReadingOrdinals  []int                  `json:"-"`
	Mechanisms       []PublicationMechanism `json:"-"`
}

type PublicationMechanism struct {
	ReadingOrdinals []int             `json:"-"`
	Nodes           []PublicationNode `json:"-"`
	Edges           []PublicationEdge `json:"-"`
}

type PublicationNode struct {
	Label       string                    `json:"-"`
	Symbol      surfacediscovery.Symbol   `json:"-"`
	Declaration surfacediscovery.Location `json:"-"`
}

type PublicationEdge struct {
	// From and To are 1-based positions in the enclosing mechanism's Nodes.
	From         int                                   `json:"-"`
	To           int                                   `json:"-"`
	Invocation   surfacediscovery.DirectCallInvocation `json:"-"`
	WitnessCount int                                   `json:"-"`
	Callsite     surfacediscovery.Location             `json:"-"`
}

// PublicationCards converts canonical aggregate cards into exact source and
// call authority. It revalidates every path and never exposes t/r/n/e refs.
func PublicationCards(compilation *Compilation, results []CardResult) ([]PublicationCard, error) {
	if err := compilation.Validate(); err != nil {
		return nil, err
	}
	if err := validateAggregateCards(compilation, results); err != nil {
		return nil, err
	}
	publication := make([]PublicationCard, 0, len(results))
	for position, result := range results {
		card := compilation.Cards[position]
		authority := compilation.authority[card.Ref]
		published := PublicationCard{
			StudyOrdinal:     authority.sourceOrdinal,
			StudyCanonicalID: authority.sourceCanonical,
			Outcome:          result.State,
			ReadingOrdinals:  []int{},
			Mechanisms:       []PublicationMechanism{},
		}
		cardNodes := make(map[string]Node, len(card.Nodes))
		for _, node := range card.Nodes {
			cardNodes[node.Ref] = node
		}
		readingUnion := make(map[int]struct{})
		for _, mechanism := range result.Mechanisms {
			item := PublicationMechanism{
				ReadingOrdinals: make([]int, 0, len(mechanism.ReadingRefs)),
				Nodes:           make([]PublicationNode, 0, len(mechanism.NodeRefs)),
				Edges:           make([]PublicationEdge, 0, len(mechanism.EdgeRefs)),
			}
			for _, ref := range mechanism.ReadingRefs {
				ordinal := authority.readingOrdinalByRef[ref]
				if ordinal <= 0 {
					return nil, fmt.Errorf("mechanism study publication: missing exact reading ordinal")
				}
				item.ReadingOrdinals = append(item.ReadingOrdinals, ordinal)
				readingUnion[ordinal] = struct{}{}
			}
			sort.Ints(item.ReadingOrdinals)
			if !uniquePositiveInts(item.ReadingOrdinals) {
				return nil, fmt.Errorf("mechanism study publication: invalid exact reading ordinals")
			}
			nodePositionByID := make(map[string]int, len(mechanism.NodeRefs))
			for nodePosition, ref := range mechanism.NodeRefs {
				exact, ok := authority.nodeByRef[ref]
				public, publicOK := cardNodes[ref]
				if !ok || !publicOK || !validPublicationNode(exact, compilation.Scenario.ID) {
					return nil, fmt.Errorf("mechanism study publication: invalid exact node authority")
				}
				if _, duplicate := nodePositionByID[exact.ID]; duplicate {
					return nil, fmt.Errorf("mechanism study publication: duplicate exact node")
				}
				nodePositionByID[exact.ID] = nodePosition + 1
				item.Nodes = append(item.Nodes, PublicationNode{
					Label:       public.Label,
					Symbol:      copyExactNode(exact).Symbol,
					Declaration: exact.Declaration,
				})
			}
			for edgePosition, ref := range mechanism.EdgeRefs {
				exact, ok := authority.edgeByRef[ref]
				if !ok || !validPublicationEdge(exact, compilation.Scenario.ID) {
					return nil, fmt.Errorf("mechanism study publication: invalid exact edge authority")
				}
				from, fromOK := nodePositionByID[exact.CallerID]
				to, toOK := nodePositionByID[exact.CalleeID]
				if !fromOK || !toOK || from != edgePosition+1 || to != edgePosition+2 {
					return nil, fmt.Errorf("mechanism study publication: exact edge does not follow ordered path")
				}
				item.Edges = append(item.Edges, PublicationEdge{
					From: from, To: to, Invocation: exact.Invocation,
					WitnessCount: exact.WitnessCount,
					Callsite:     exact.RepresentativeCallsite,
				})
			}
			published.Mechanisms = append(published.Mechanisms, item)
		}
		for ordinal := range readingUnion {
			published.ReadingOrdinals = append(published.ReadingOrdinals, ordinal)
		}
		sort.Ints(published.ReadingOrdinals)
		publication = append(publication, published)
	}
	return publication, nil
}

func validPublicationNode(node surfacediscovery.DirectCallNode, scenarioID string) bool {
	return node.ID != "" && node.ScenarioID == scenarioID && node.Package != "" &&
		node.Symbol.ID != "" && node.Symbol.Package == node.Package && node.Symbol.Name != "" &&
		node.Symbol.Location == node.Declaration && validPublicationLocation(node.Declaration)
}

func validPublicationEdge(edge surfacediscovery.DirectCallEdge, scenarioID string) bool {
	return edge.ID != "" && edge.CallerID != "" && edge.CalleeID != "" &&
		edge.CallerID != edge.CalleeID && edge.ScenarioID == scenarioID && edge.Invocation.Valid() &&
		edge.WitnessCount > 0 && validPublicationLocation(edge.RepresentativeCallsite)
}

func validPublicationLocation(location surfacediscovery.Location) bool {
	if location.Line <= 0 || location.Column < 0 || location.Path == "" ||
		strings.Contains(location.Path, "\\") || strings.HasPrefix(location.Path, "/") ||
		path.Clean(location.Path) != location.Path || location.Path == "." || location.Path == ".." ||
		strings.HasPrefix(location.Path, "../") {
		return false
	}
	return true
}

func uniquePositiveInts(values []int) bool {
	previous := 0
	for _, value := range values {
		if value <= previous {
			return false
		}
		previous = value
	}
	return true
}
