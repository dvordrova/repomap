package main

import (
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestFreshRoleAwareAnchorPathsReservesProductionRoles(t *testing.T) {
	anchors := []freshCentralAnchor{
		{Path: "cmd/app-preview/main.go", Line: 1, Score: 500},
		{Path: "testdata/fixture/main.go", Line: 1, Score: 500},
		{Path: "cmd/app/main.go", Line: 2, Score: 10},
		{Path: "cmd/app/serve.go", Line: 3, Score: 9},
		{Path: "internal/storage/write.go", Line: 4, Score: 8},
		{Path: "client/client.go", Line: 5, Score: 7},
		{Path: "internal/scheduler/run.go", Line: 6, Score: 6},
	}

	selected := freshRoleAwareAnchorPaths(anchors, 4)
	want := []string{
		"cmd/app/main.go",
		"internal/storage/write.go",
		"client/client.go",
		"internal/scheduler/run.go",
	}
	for index := range want {
		if selected[index] != want[index] {
			t.Fatalf("freshRoleAwareAnchorPaths() = %v, want %v", selected, want)
		}
	}
}

func TestMergeFreshSourceFunctionsInterleavesProductionRoles(t *testing.T) {
	central := []freshSourceFunction{
		{Function: sourceFunction("cmd/app/a.go", "a")},
		{Function: sourceFunction("cmd/app/b.go", "b")},
		{Function: sourceFunction("cmd/app/c.go", "c")},
		{Function: sourceFunction("cmd/app/d.go", "d")},
		{Function: sourceFunction("cmd/app/e.go", "e")},
	}
	saved := []freshSourceFunction{
		{Function: sourceFunction("internal/storage/write.go", "write")},
		{Function: sourceFunction("client/client.go", "send")},
		{Function: sourceFunction("internal/scheduler/run.go", "run")},
		{Function: sourceFunction("cmd/app-preview/main.go", "preview")},
	}

	selected := mergeFreshSourceFunctions(saved, central, 4)
	wantPaths := []string{
		"cmd/app/a.go",
		"internal/storage/write.go",
		"client/client.go",
		"internal/scheduler/run.go",
	}
	for index := range wantPaths {
		if selected[index].Function.Path != wantPaths[index] {
			t.Fatalf("mergeFreshSourceFunctions() paths = %v, want %v", sourcePaths(selected), wantPaths)
		}
	}
}

func TestSelectFreshRankedFunctionsInterleavesBeforeTwelveFunctionLimit(t *testing.T) {
	functions := []freshRankedFunction{
		{Function: sourceFunction("internal/storage/write.go", "write"), Score: 10},
		{Function: sourceFunction("client/client.go", "send"), Score: 9},
		{Function: sourceFunction("internal/scheduler/run.go", "run"), Score: 8},
		{Function: sourceFunction("cmd/app-preview/main.go", "preview"), Score: 1_000},
	}
	for index := 0; index < 20; index++ {
		functions = append(functions, freshRankedFunction{
			Function: sourceFunction(
				fmt.Sprintf("cmd/app/%02d.go", index),
				fmt.Sprintf("command%d", index),
			),
			Score: 500 - index,
		})
	}

	selected := selectFreshRankedFunctionsByRole(functions, freshRepoOnboardingMaxAnchorFuncs)
	if len(selected) != freshRepoOnboardingMaxAnchorFuncs {
		t.Fatalf("selected functions = %d, want %d", len(selected), freshRepoOnboardingMaxAnchorFuncs)
	}
	paths := sourcePathsFromRanked(selected)
	for _, required := range []string{
		"internal/storage/write.go",
		"client/client.go",
		"internal/scheduler/run.go",
	} {
		if !stringSliceHas(paths, required) {
			t.Fatalf("selected ranked functions = %v, missing production role %q", paths, required)
		}
	}
	if stringSliceHas(paths, "cmd/app-preview/main.go") {
		t.Fatalf("preview consumed a twelve-function production slot: %v", paths)
	}
}

func sourceFunction(path, symbol string) sourcewindowfacts.Function {
	return sourcewindowfacts.Function{Path: path, Symbol: symbol}
}

func sourcePaths(sources []freshSourceFunction) []string {
	paths := make([]string, len(sources))
	for index := range sources {
		paths[index] = sources[index].Function.Path
	}
	return paths
}

func sourcePathsFromRanked(functions []freshRankedFunction) []string {
	paths := make([]string, len(functions))
	for index := range functions {
		paths[index] = functions[index].Function.Path
	}
	return paths
}

func stringSliceHas(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
