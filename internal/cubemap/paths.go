package cubemap

import (
	"sort"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func buildShortestPaths(
	index surfacediscovery.DirectCallIndex,
	entrypoints []Symbol,
	integrations []IntegrationSymbol,
) []Path {
	entrypointIDs := make(map[string]struct{}, len(entrypoints))
	for _, entrypoint := range entrypoints {
		entrypointIDs[entrypoint.NodeID] = struct{}{}
	}
	result := make([]Path, 0, len(integrations))
	for _, integration := range integrations {
		nodes, ok := shortestReversePath(index, entrypointIDs, integration.Symbol.NodeID)
		if !ok {
			continue
		}
		path := Path{
			EntrypointNodeID: nodes[0].NodeID, IntegrationNodeID: integration.Symbol.NodeID,
			Nodes: nodes,
		}
		result = append(result, path)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].IntegrationNodeID < result[j].IntegrationNodeID })
	return result
}

func shortestReversePath(
	index surfacediscovery.DirectCallIndex,
	entrypointIDs map[string]struct{},
	integrationNodeID string,
) ([]Symbol, bool) {
	if _, exists := index.Node(integrationNodeID); !exists || len(entrypointIDs) == 0 {
		return nil, false
	}
	queue := []string{integrationNodeID}
	visited := map[string]struct{}{integrationNodeID: {}}
	nextTowardIntegration := make(map[string]string)
	for len(queue) > 0 {
		sort.Slice(queue, func(i, j int) bool {
			left, _ := index.Node(queue[i])
			right, _ := index.Node(queue[j])
			return symbolKey(symbolFromNode(left)) < symbolKey(symbolFromNode(right))
		})
		for _, nodeID := range queue {
			if _, selected := entrypointIDs[nodeID]; selected {
				return restorePath(index, nodeID, integrationNodeID, nextTowardIntegration)
			}
		}
		nextQueue := make([]string, 0)
		for _, calleeID := range queue {
			incoming := index.Incoming(calleeID)
			sort.Slice(incoming, func(i, j int) bool {
				left, _ := index.Node(incoming[i].CallerID)
				right, _ := index.Node(incoming[j].CallerID)
				leftKey := symbolKey(symbolFromNode(left))
				rightKey := symbolKey(symbolFromNode(right))
				if leftKey != rightKey {
					return leftKey < rightKey
				}
				return incoming[i].ID < incoming[j].ID
			})
			for _, edge := range incoming {
				if _, exists := visited[edge.CallerID]; exists {
					continue
				}
				visited[edge.CallerID] = struct{}{}
				nextTowardIntegration[edge.CallerID] = calleeID
				nextQueue = append(nextQueue, edge.CallerID)
			}
		}
		queue = nextQueue
	}
	return nil, false
}

func restorePath(
	index surfacediscovery.DirectCallIndex,
	entrypointNodeID string,
	integrationNodeID string,
	nextTowardIntegration map[string]string,
) ([]Symbol, bool) {
	result := make([]Symbol, 0)
	current := entrypointNodeID
	for {
		node, exists := index.Node(current)
		if !exists {
			return nil, false
		}
		result = append(result, symbolFromNode(node))
		if current == integrationNodeID {
			return result, true
		}
		next, exists := nextTowardIntegration[current]
		if !exists {
			return nil, false
		}
		current = next
	}
}
