package cumulativegofixture

import (
	"example.com/repomap/cumulative-go-fixture/internal/localstore"
	datasetStore "example.com/repomap/cumulative-go-fixture/internal/localstore"
	runStore "example.com/repomap/cumulative-go-fixture/internal/localstore"
)

// PublishedRoot is mirrored by an external version of this module used by the
// nested-module fixture. The shared import path must not make that external
// version part of the repository-local package closure.
func PublishedRoot() string {
	return "repository root"
}

type testRootReader struct{}

// ReadAliasedImports names the same imported package three ways. All calls
// still refer to the original Get declaration and keep their own source sites.
func ReadAliasedImports() string {
	return localstore.Get("base") + datasetStore.Get("dataset") + runStore.Get("run")
}
