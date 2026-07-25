// Package workspaceedgeselection defines a bounded immutable projection of
// already-selected exact edges from one authorized workspace package graph.
package workspaceedgeselection

import (
	"errors"

	"github.com/dvordrova/repomap/internal/workspacegraph"
)

const (
	// MaxRows bounds the ordered candidate and selected edge collections.
	MaxRows = 1000
	// MaxEndpointBytes bounds each exact package endpoint.
	MaxEndpointBytes = 4096
	// MaxAggregateEndpointBytes bounds all endpoints in one construction.
	MaxAggregateEndpointBytes = 4 * 1024 * 1024
)

var (
	errRawBounds       = errors.New("workspace edge selection: candidates exceed bounds")
	errEndpointBounds  = errors.New("workspace edge selection: endpoint exceeds bounds")
	errAggregateBounds = errors.New("workspace edge selection: aggregate endpoints exceed bounds")
	errUnauthorized    = errors.New("workspace edge selection: candidate is unavailable")
)

// Candidate is one caller-selected exact directed package pair.
type Candidate struct {
	From string
	To   string
}

// Edge is one graph-authorized exact directed package pair.
type Edge struct {
	From string
	To   string
}

// Input is the complete already-constructed authority and ordered selection.
// New performs no discovery, traversal, sorting, or source access.
type Input struct {
	Graph      workspacegraph.Graph
	Candidates []Candidate
}

// Selection is an immutable ordered projection. Edges returns defensive
// copies and preserves the input's nil versus non-nil empty shape.
type Selection struct {
	edges       []Edge
	initialized bool
}

// New validates the complete raw-input budget before graph lookup or result
// allocation, then authorizes every candidate as an exact directed graph edge.
// Any unavailable candidate rejects the complete selection.
func New(input Input) (Selection, error) {
	if err := preflight(input.Candidates); err != nil {
		return Selection{}, err
	}
	if input.Candidates == nil {
		return Selection{initialized: true}, nil
	}

	edges := make([]Edge, len(input.Candidates))
	for index, candidate := range input.Candidates {
		edge, ok := input.Graph.Edge(candidate.From, candidate.To)
		if !ok {
			return Selection{}, errUnauthorized
		}
		edges[index] = Edge{
			From: edge.FromPackage,
			To:   edge.ToPackage,
		}
	}
	return Selection{edges: edges, initialized: true}, nil
}

// Edges returns a defensive copy in exact candidate order.
func (selection Selection) Edges() []Edge {
	if !selection.initialized || selection.edges == nil {
		return nil
	}
	edges := make([]Edge, len(selection.edges))
	copy(edges, selection.edges)
	return edges
}

func preflight(candidates []Candidate) error {
	if len(candidates) > MaxRows {
		return errRawBounds
	}
	for _, candidate := range candidates {
		for _, endpoint := range [...]string{candidate.From, candidate.To} {
			if len(endpoint) > MaxEndpointBytes {
				return errEndpointBounds
			}
		}
	}
	remaining := MaxAggregateEndpointBytes
	for _, candidate := range candidates {
		for _, endpoint := range [...]string{candidate.From, candidate.To} {
			if len(endpoint) > remaining {
				return errAggregateBounds
			}
			remaining -= len(endpoint)
		}
	}
	return nil
}
