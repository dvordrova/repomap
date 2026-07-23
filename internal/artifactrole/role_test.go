package artifactrole

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		hints Hints
		want  Role
	}{
		{name: "primary command", path: "cmd/litestream/restore.go", want: RolePrimaryProductionEntry},
		{name: "preview before command", path: "cmd/canvas-preview/main.go", want: RolePlayground},
		{name: "test binary before command", path: "cmd/litestream-test/main.go", want: RoleTest},
		{name: "fixture before test", path: "internal/testdata/app/main.go", want: RoleFixture},
		{name: "mock support", path: "mock/replica_client.go", want: RoleFixture},
		{name: "fake support", path: "internal/fakes/client.go", want: RoleFixture},
		{name: "test utility", path: "internal/testutil/database.go", want: RoleFixture},
		{name: "generated before api", path: "api/service.pb.go", want: RoleGenerated},
		{name: "public api", path: "client/client.go", want: RolePublicAPI},
		{name: "effect boundary", path: "internal/storage/writer.go", want: RoleEffectBoundary},
		{name: "production core", path: "internal/scheduler/run.go", want: RoleProductionCore},
		{name: "example", path: "_examples/library/main.go", want: RoleExample},
		{name: "experimental", path: "internal/experiment/probe.go", want: RoleExperimental},
		{name: "current docs", path: "docs/agent-room/CURRENT.md", want: RoleCurrentDocumentation},
		{name: "decision docs", path: "docs/agent-room/114-product.md", want: RoleHistoricalDocumentation},
		{name: "exact entry hint", path: "app/start.go", hints: Hints{PrimaryEntry: true}, want: RolePrimaryProductionEntry},
		{name: "fixture defeats entry hint", path: "testdata/app/main.go", hints: Hints{PrimaryEntry: true}, want: RoleFixture},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Classify(test.path, test.hints); got != test.want {
				t.Fatalf("Classify(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestSortPathsPrefersShallowRolePeersAndExactMain(t *testing.T) {
	input := []string{
		"abs/replica_client.go",
		"file/replica_client.go",
		"mock/replica_client.go",
		"replica_client.go",
		"internal/runtime/scheduler/run.go",
		"replica.go",
		"cmd/app/serve.go",
		"cmd/app/main.go",
	}

	got := SortPaths(input)
	wantPrefix := []string{
		"cmd/app/main.go",
		"replica_client.go",
		"replica.go",
		"cmd/app/serve.go",
	}
	for index := range wantPrefix {
		if got[index] != wantPrefix[index] {
			t.Fatalf("SortPaths() = %v, want prefix %v", got, wantPrefix)
		}
	}
	if got[len(got)-1] != "mock/replica_client.go" {
		t.Fatalf("test-support path was not deferred: %v", got)
	}
}

func TestSortPathsKeepsProductionAheadOfAuxiliaryArtifacts(t *testing.T) {
	input := []string{
		"testdata/tool/main.go",
		"cmd/app-preview/main.go",
		"cmd/app/second.go",
		"internal/core/run.go",
		"internal/storage/write.go",
		"cmd/app/main.go",
	}

	got := SortPaths(input)
	want := []string{
		"cmd/app/main.go",
		"internal/storage/write.go",
		"internal/core/run.go",
		"cmd/app/second.go",
		"cmd/app-preview/main.go",
		"testdata/tool/main.go",
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("SortPaths() = %v, want %v", got, want)
		}
	}
	if input[0] != "testdata/tool/main.go" {
		t.Fatalf("SortPaths mutated caller input: %v", input)
	}
}
