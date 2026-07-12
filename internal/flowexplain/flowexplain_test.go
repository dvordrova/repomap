package flowexplain

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestGenerateFlowID(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Watch stream flow", "watch-stream-flow"},
		{"gRPC Put request", "grpc-put-request"},
		{"Raft write path", "raft-write-path"},
	}
	for _, tc := range cases {
		got := GenerateFlowID(tc.name)
		if got != tc.want {
			t.Errorf("GenerateFlowID(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestExtractTermsRemovesIgnoredWords(t *testing.T) {
	query, alias := ExtractTerms("Raft write path", "", "", nil)

	for _, term := range query {
		if term == "path" || term == "flow" || term == "lifecycle" {
			t.Errorf("should not contain generic term %q in query_terms: %v", term, query)
		}
	}
	for _, term := range alias {
		if term == "path" || term == "flow" {
			t.Errorf("should not contain generic term %q in alias_terms: %v", term, alias)
		}
	}

	hasRaft := false
	for _, t := range alias {
		if t == "raft" {
			hasRaft = true
		}
	}
	for _, t := range query {
		if t == "raft" {
			hasRaft = true
		}
	}
	if !hasRaft {
		t.Fatal("should contain raft")
	}
}

func TestExtractTermsExpandsAliases(t *testing.T) {
	_, alias := ExtractTerms("Watch stream", "", "", nil)

	hasWatcher := false
	for _, term := range alias {
		if term == "watcher" || term == "watchable" {
			hasWatcher = true
		}
	}
	if !hasWatcher {
		t.Fatalf("alias should expand watch to watcher/watchable, got: %v", alias)
	}
}

func TestExtractTermsStripsPunctuation(t *testing.T) {
	query, alias := ExtractTerms("gRPC Put request (propose, apply)", "", "", nil)

	for _, term := range query {
		if strings.Contains(term, "(") || strings.Contains(term, ")") || strings.Contains(term, ",") {
			t.Errorf("should strip punctuation, got: %q", term)
		}
	}
	if !contains(alias, "put") || !contains(alias, "kv") {
		t.Fatalf("alias should contain put/kv, got: %v", alias)
	}
	_ = query
}

func TestExtractTermsDoesNotPromotePathStructure(t *testing.T) {
	query, alias := ExtractTerms(
		"Lease lifecycle (grant, renew, expire)",
		"client requests a lease",
		"server/lease/lease.go",
		[]string{"etcdctl/ctlv3/command/lease_command.go is a CLI seed"},
	)
	for _, forbidden := range []string{"command", "ctl", "ctlv3", "etcdctl", "cobra"} {
		if contains(query, forbidden) || contains(alias, forbidden) {
			t.Fatalf("path structure term %q leaked into query=%v alias=%v", forbidden, query, alias)
		}
	}
	if !contains(query, "lease") || !contains(alias, "lessor") {
		t.Fatalf("semantic lease terms missing: query=%v alias=%v", query, alias)
	}
}

func TestExtractTermsDoesNotPromoteGenericEntrypointBasename(t *testing.T) {
	query, alias := ExtractTerms(
		"Raft leader election flow",
		"a cluster member starts an election",
		"server/main.go",
		nil,
	)

	if contains(query, "main") || contains(alias, "main") {
		t.Fatalf("generic entrypoint basename leaked into query=%v alias=%v", query, alias)
	}
	if !contains(query, "raft") || !contains(alias, "rafthttp") {
		t.Fatalf("flow-specific raft terms missing: query=%v alias=%v", query, alias)
	}
}

func TestSelectFlowFilesIncludesPythonSourcesAndTests(t *testing.T) {
	t.Parallel()

	files, tests, _, _, _ := SelectFlowFiles(
		[]string{"src/tool/__main__.py", "src/tool/service.py", "tests/test_service.py", "pyproject.toml"},
		[]string{"service"},
		[]string{"src/tool/__main__.py"},
		nil,
		10,
	)
	if len(files) != 2 || files[0].Path != "src/tool/__main__.py" || files[1].Path != "src/tool/service.py" {
		t.Fatalf("selected Python sources = %#v", files)
	}
	if len(tests) != 1 || tests[0].Path != "tests/test_service.py" {
		t.Fatalf("selected Python tests = %#v", tests)
	}
}

func TestScoreFileLayeredPrefersV3RPCOverV2(t *testing.T) {
	v3rpcScore, _, _ := scoreFileLayered("server/etcdserver/api/v3rpc/watch.go", "source", []string{"watch", "v3rpc"}, false)
	v2storeScore, _, _ := scoreFileLayered("server/storage/mvcc/v2store_watcher.go", "source", []string{"watch", "v2store"}, false)

	if v3rpcScore <= v2storeScore {
		t.Errorf("v3rpc watch should outrank v2store watcher: v3rpc=%d, v2store=%d", v3rpcScore, v2storeScore)
	}
}

func TestScoreFileLayeredPrefersRaft(t *testing.T) {
	raftScore, _, _ := scoreFileLayered("server/etcdserver/raft.go", "source", []string{"raft", "propose", "apply"}, false)
	pathutilScore, _, _ := scoreFileLayered("client/pkg/pathutil/path.go", "source", []string{"raft", "propose", "apply"}, false)

	if raftScore <= pathutilScore {
		t.Errorf("raft.go should outrank pathutil/path.go: raft=%d, pathutil=%d", raftScore, pathutilScore)
	}
}

func TestScoreFileLayeredPrefersKV(t *testing.T) {
	kvScore, _, _ := scoreFileLayered("client/v3/kv.go", "source", []string{"put", "kv", "txn"}, false)
	clientScore, _, _ := scoreFileLayered("client/v3/client.go", "source", []string{"put", "kv", "txn"}, false)

	if kvScore <= 0 {
		t.Fatalf("kv.go should score > 0: got %d", kvScore)
	}
	if kvScore < clientScore {
		t.Errorf("kv.go should not lose to client.go: kv=%d, client=%d", kvScore, clientScore)
	}
}

func TestSelectFlowFilesProtoIsNotDoc(t *testing.T) {
	tracked := []string{"api/etcdserverpb/rpc.proto", "server/etcdserver/api/v3rpc/watch.go", "server/storage/mvcc/watchable_store.go"}
	terms := []string{"watch", "v3rpc", "proto"}
	files, _, docs, _, _ := SelectFlowFiles(tracked, terms, nil, nil, 10)

	for _, f := range files {
		if f.Path == "api/etcdserverpb/rpc.proto" {
			if f.Kind != "proto" {
				t.Errorf("proto should be kind=proto, got %s", f.Kind)
			}
			return
		}
	}
	for _, d := range docs {
		if d.Path == "api/etcdserverpb/rpc.proto" {
			t.Fatal("proto file should not be in selected_docs")
		}
	}
}

func TestSelectFlowFilesDropsLexicalMatchesFromNonGoArtifacts(t *testing.T) {
	tracked := []string{
		"batch.go",
		"commit.go",
		"docs/js/write-throughput.js",
		"internal/mkbench/testdata/write-throughput/run-summary.json",
	}
	files, tests, docs, _, _ := SelectFlowFiles(tracked, []string{"batch", "commit", "put", "write"}, []string{"batch.go"}, nil, 20)

	for _, selected := range append(append(files, tests...), docs...) {
		if strings.HasSuffix(selected.Path, ".js") || strings.HasSuffix(selected.Path, ".json") {
			t.Fatalf("non-Go lexical match was selected: %#v", selected)
		}
	}
	if !hasScoredPath(files, "batch.go") || !hasScoredPath(files, "commit.go") {
		t.Fatalf("expected Go flow files were not retained: %#v", files)
	}
}

func TestSelectFlowFilesKeepsExplicitNonGoSeed(t *testing.T) {
	const config = "config/write-path.yaml"
	files, _, _, _, _ := SelectFlowFiles([]string{config}, []string{"write"}, []string{config}, nil, 20)
	if !hasScoredPath(files, config) {
		t.Fatalf("explicit non-Go seed was discarded: %#v", files)
	}
}

func hasScoredPath(files []scoredFile, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func TestSelectFlowFilesV2StorePenalized(t *testing.T) {
	tracked := []string{"server/storage/mvcc/watchable_store.go", "server/storage/mvcc/v2store_watcher.go"}
	terms := []string{"watch", "mvcc"}
	files, _, _, _, _ := SelectFlowFiles(tracked, terms, nil, nil, 10)

	var v2score, v3score int
	for _, f := range files {
		if strings.Contains(f.Path, "v2store") {
			v2score = f.Score
		}
		if strings.Contains(f.Path, "watchable_store") {
			v3score = f.Score
		}
	}

	if v2score >= v3score {
		t.Errorf("v2store (%d) should score below watchable_store (%d)", v2score, v3score)
	}
}

func TestSelectFlowFilesIncludesPackages(t *testing.T) {
	facts := &gofacts.Facts{
		EntrypointPackages: []gofacts.Entrypoint{
			{ImportPath: "example.com/server/etcdserver", PackageDir: "server/etcdserver", GoFiles: []string{"server.go"}},
			{ImportPath: "example.com/server/etcdserver/api/v3rpc", PackageDir: "server/etcdserver/api/v3rpc", GoFiles: []string{"watch.go"}},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/server/etcdserver", To: "example.com/server/etcdserver/api/v3rpc"},
		},
	}
	tracked := []string{
		"server/etcdserver/server.go",
		"server/etcdserver/api/v3rpc/watch.go",
	}
	terms := []string{"server", "watch"}

	_, _, _, pkgs, edges := SelectFlowFiles(tracked, terms, nil, facts, 10)

	if len(pkgs) == 0 {
		t.Fatal("selected_packages should not be empty")
	}
	if len(edges) == 0 {
		t.Fatal("related_edges should not be empty when packages have internal edges")
	}
}

func TestSelectFlowFilesBuildsPackageContextFromRetainedFilesOnly(t *testing.T) {
	facts := &gofacts.Facts{
		EntrypointPackages: []gofacts.Entrypoint{
			{
				ImportPath: "example.com/server",
				PackageDir: "server",
				GoFiles:    []string{"main.go"},
			},
			{
				ImportPath: "example.com/unrelated-tool",
				PackageDir: "tools/unrelated",
				GoFiles:    []string{"main.go"},
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/unrelated-tool", To: "example.com/unrelated-support"},
		},
	}

	files, _, _, packages, edges := SelectFlowFiles(
		[]string{"server/main.go", "tools/unrelated/main.go"},
		[]string{"main"},
		[]string{"server/main.go"},
		facts,
		1,
	)

	if len(files) != 1 || files[0].Path != "server/main.go" {
		t.Fatalf("selected files = %#v, want only the seed", files)
	}
	if len(packages) != 1 || packages[0] != "example.com/server" {
		t.Fatalf("selected packages = %v, want only retained file package", packages)
	}
	if len(edges) != 0 {
		t.Fatalf("related edges = %v, want no edges from discarded candidates", edges)
	}
}

func TestSelectFlowFilesLeasePrefersLessor(t *testing.T) {
	tracked := []string{
		"server/lease/lessor.go",
		"server/lease/lease.go",
		"server/lease/lease_queue.go",
		"client/v3/lease.go",
		"server/etcdserver/api/v3rpc/lease.go",
	}
	terms := []string{"lease", "lessor", "keepalive", "ttl", "grant"}
	files, _, _, _, _ := SelectFlowFiles(tracked, terms, nil, nil, 10)

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}

	if !contains(paths, "server/lease/lessor.go") {
		t.Fatal("should include server/lease/lessor.go")
	}
	if !contains(paths, "client/v3/lease.go") {
		t.Fatal("should include client/v3/lease.go")
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
