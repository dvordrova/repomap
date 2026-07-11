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

type Explainer struct {
	mu       sync.Mutex
	Response []byte
	Err      error
	requests [][]byte
}

func NewExplainer() *Explainer {
	return &Explainer{Response: append([]byte(nil), SymbolResponseJSON...)}
}

func (e *Explainer) ExplainSymbol(_ context.Context, bundleJSON []byte) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, append([]byte(nil), bundleJSON...))
	return append([]byte(nil), e.Response...), e.Err
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
