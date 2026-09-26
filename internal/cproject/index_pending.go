package cproject

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

// This file stands in for the ProgramIndex projection (build.go, resolve.go,
// deps.go) until that projection is merged beside the run wiring. It declares
// the same Result and Index, so the merge cannot keep both: delete this file
// when build.go arrives. Until then every C program fails closed here and is
// reported as not analyzed; nothing is projected in its place.

// Result is one C program as the ordinary pipeline reads it.
type Result struct {
	Program      Program
	Toolchain    Toolchain
	Outside      []string
	Input        programindex.Input
	Dependencies dependencies.Catalog
}

// Index projects one parsed program into ProgramIndex input.
func Index(repository *corpus.Corpus, parsed *Parsed) (*Result, error) {
	if repository == nil || parsed == nil {
		return nil, fmt.Errorf("C: repository and parsed program are required")
	}
	return nil, fmt.Errorf("C program %s: the ProgramIndex projection is not part of this build", parsed.Program.Selector)
}
