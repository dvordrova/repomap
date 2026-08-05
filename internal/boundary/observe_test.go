package boundary

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

// writeFixtureRepo writes a tiny Go repository with known boundary call sites
// and returns the repository path plus the gofacts.Facts it produces.
func writeFixtureRepo(t *testing.T) (string, gofacts.Facts, []string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.21\n",
		"main.go": `package main

import (
	"database/sql"
	"net/http"
	"os"

	"example.com/fixture/store"
)

func main() {
	db, _ := sql.Open("sqlite3", "file.db")
	_ = db
	client := &http.Client{Timeout: 0}
	_ = client
	f, _ := os.OpenFile("data.bin", os.O_WRONLY|os.O_CREATE, 0o600)
	_ = f
	store.Start()
}
`,
		"store/store.go": `package store

import (
	"database/sql"
	"net/http"
	"google.golang.org/grpc"
)

func Start() {
	db, _ := sql.Open("postgres", "host=db")
	_ = db
	client := http.NewClient()
	_ = client
	conn, _ := grpc.Dial("localhost:50051")
	_ = conn
}
`,
	}
	paths := make([]string, 0, len(files))
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	sort.Strings(paths)
	facts, err := gofacts.Load(context.Background(), root, paths, 64, 64)
	if err != nil {
		t.Fatalf("gofacts.Load: %v", err)
	}
	return root, *facts, paths
}

func TestObserveFindsStorageAndOutboundCallSites(t *testing.T) {
	root, facts, filtered := writeFixtureRepo(t)
	result, err := Observe(context.Background(), root, filtered, facts)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	// database/sql.Open in main.go (persistent_storage), enclosing main.
	if !hasObservation(result, ClassPersistentStorage, "database/sql", "main.go", "main") {
		t.Errorf("missing sql.Open storage observation in main.go:\n%+v", result.Observations)
	}
	// http.Client composite literal in main.go (outbound_client).
	if !hasObservation(result, ClassOutboundClient, "net/http", "main.go", "main") {
		t.Errorf("missing http.Client composite-literal observation in main.go:\n%+v", result.Observations)
	}
	// os.OpenFile in main.go (persistent_storage, durable write).
	if !hasObservation(result, ClassPersistentStorage, "os", "main.go", "main") {
		t.Errorf("missing os.OpenFile storage observation in main.go:\n%+v", result.Observations)
	}
	// store.Start: sql.Open + http.NewClient + grpc.Dial, enclosing Start.
	if !hasObservation(result, ClassPersistentStorage, "database/sql", "store/store.go", "Start") {
		t.Errorf("missing sql.Open observation in store/store.go:\n%+v", result.Observations)
	}
	if !hasObservation(result, ClassOutboundClient, "net/http", "store/store.go", "Start") {
		t.Errorf("missing http.NewClient observation in store/store.go:\n%+v", result.Observations)
	}
	if !hasObservation(result, ClassOutboundClient, "google.golang.org/grpc", "store/store.go", "Start") {
		t.Errorf("missing grpc.Dial observation in store/store.go:\n%+v", result.Observations)
	}
}

func TestObserveRecordsExactSymbolAndLocation(t *testing.T) {
	root, facts, filtered := writeFixtureRepo(t)
	result, err := Observe(context.Background(), root, filtered, facts)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	for _, observation := range result.Observations {
		if observation.Location.Path == "" || observation.Location.Line <= 0 ||
			observation.Location.Column <= 0 || observation.Symbol == "" ||
			observation.PackagePath == "" {
			t.Errorf("observation missing exact evidence: %+v", observation)
		}
	}
}

func TestObserveIsDeterministic(t *testing.T) {
	root, facts, filtered := writeFixtureRepo(t)
	first, err := Observe(context.Background(), root, filtered, facts)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	second, err := Observe(context.Background(), root, filtered, facts)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if len(first.Observations) != len(second.Observations) {
		t.Fatalf("observation count changed between runs: %d vs %d", len(first.Observations), len(second.Observations))
	}
	for index := range first.Observations {
		if first.Observations[index] != second.Observations[index] {
			t.Fatalf("observation %d differs between runs: %+v vs %+v", index, first.Observations[index], second.Observations[index])
		}
	}
}

func hasObservation(
	result Result,
	class Class,
	importPath string,
	pathSuffix string,
	symbol string,
) bool {
	for _, observation := range result.Observations {
		if observation.Class != class || observation.ImportPath != importPath ||
			!stringsHasSuffix(observation.Location.Path, pathSuffix) {
			continue
		}
		if symbol != "" && observation.Symbol != symbol {
			continue
		}
		return true
	}
	return false
}

func stringsHasSuffix(value, suffix string) bool {
	return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
}

func TestIsExternalImport(t *testing.T) {
	modules := []string{"example.com/fixture"}
	cases := []struct {
		importPath string
		want       bool
	}{
		{"github.com/redis/go-redis/v9", true},
		{"database/sql", false},
		{"net/http", false},
		{"example.com/fixture/store", false},
		{"example.com/fixture", false},
		{"example.com/other/pkg", true},
		{"", false},
	}
	for _, tc := range cases {
		if got := isExternalImport(tc.importPath, modules); got != tc.want {
			t.Errorf("isExternalImport(%q) = %v, want %v", tc.importPath, got, tc.want)
		}
	}
}

func TestIsClientConstructor(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"NewClient", false},
		{"NewRedisClient", true},
		{"NewGrpcClient", true},
		{"NewHTTPClient", true},
		{"Open", false},
		{"NewSomethingElse", false},
	}
	for _, tc := range cases {
		if got := isClientConstructor(tc.name); got != tc.want {
			t.Errorf("isClientConstructor(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
