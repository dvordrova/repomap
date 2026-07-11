// Package deepseektest provides stable DeepSeek fixtures and a small in-memory
// explainer for tests outside the HTTP client package.
package deepseektest

import (
	"context"
	_ "embed"
	"sync"
)

//go:embed testdata/symbol_bundle.json
var SymbolBundleJSON []byte

//go:embed testdata/symbol_response.json
var SymbolResponseJSON []byte

//go:embed testdata/source_card.json
var SourceCardJSON []byte

//go:embed testdata/source_bundle.json
var SourceBundleJSON []byte

//go:embed testdata/source_response.json
var SourceResponseJSON []byte

//go:embed testdata/source_manifest.json
var SourceManifestJSON []byte

type Explainer struct {
	mu             sync.Mutex
	Response       []byte
	SourceResponse []byte
	Err            error
	requests       [][]byte
	sourceRequests [][]byte
}

func NewExplainer() *Explainer {
	return &Explainer{
		Response:       append([]byte(nil), SymbolResponseJSON...),
		SourceResponse: append([]byte(nil), SourceResponseJSON...),
	}
}

func (e *Explainer) AssessSource(_ context.Context, bundleJSON []byte) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sourceRequests = append(e.sourceRequests, append([]byte(nil), bundleJSON...))
	return append([]byte(nil), e.SourceResponse...), e.Err
}

func (e *Explainer) ExplainSymbol(_ context.Context, bundleJSON []byte) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, append([]byte(nil), bundleJSON...))
	return append([]byte(nil), e.Response...), e.Err
}

func (e *Explainer) SourceRequests() [][]byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([][]byte, len(e.sourceRequests))
	for index := range e.sourceRequests {
		result[index] = append([]byte(nil), e.sourceRequests[index]...)
	}
	return result
}

func (e *Explainer) Requests() [][]byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([][]byte, len(e.requests))
	for index := range e.requests {
		result[index] = append([]byte(nil), e.requests[index]...)
	}
	return result
}
