package lexicalhints

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestScanBuildsSparseCountsAndLocalCoverage(t *testing.T) {
	t.Parallel()

	files := map[string][]byte{
		"a.go":              []byte("CONFIG client Client"),
		"b.go":              []byte("// route request response sql dao kafka worker"),
		"empty.txt":         {},
		"saturated.txt":     []byte(strings.Repeat("client ", 300)),
		"binary.dat":        []byte("client\x00worker"),
		"invalid.txt":       {0xff, 0xfe},
		"vendor/v.go":       []byte("worker"),
		"node_modules/n.js": []byte("request"),
		"third_party/t.go":  []byte("response"),
		"package-lock.json": []byte("client"),
		"web/app.min.js":    []byte("client"),
		"assets/logo.png":   []byte("client"),
		"large.txt":         []byte(strings.Repeat("x", int(MaxFileBytes)+1)),
		"gone.go":           []byte("worker"),
	}
	repository, root := testCorpus(t, files)
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}

	result, err := Scan(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if result.CorpusRef == "" || result.CorpusRef != repository.Ref() {
		t.Fatalf("CorpusRef = %q, want %q", result.CorpusRef, repository.Ref())
	}
	wantCoverage := Coverage{
		TrackedFiles:         14,
		ScannedFiles:         4,
		PathExcludedFiles:    6,
		OversizeOmissions:    1,
		BinaryOmissions:      1,
		InvalidUTF8Omissions: 1,
		ReadOmissions:        1,
	}
	if !reflect.DeepEqual(result.Coverage, wantCoverage) {
		t.Fatalf("Coverage = %#v, want %#v", result.Coverage, wantCoverage)
	}
	if got := result.Coverage.TrackedFiles - result.Coverage.ScannedFiles; got != 10 {
		t.Fatalf("omitted file count = %d, want 10", got)
	}

	aRef, _ := repository.ID("a.go")
	bRef, _ := repository.ID("b.go")
	saturatedRef, _ := repository.ID("saturated.txt")
	want := map[corpus.FileID]map[string]uint8{
		aRef: {"config": 1, "client": 2},
		bRef: {
			"route": 1, "request": 1, "response": 1, "sql": 1,
			"dao": 1, "kafka": 1, "worker": 1,
		},
		saturatedRef: {"client": CountCap},
	}
	if !reflect.DeepEqual(result.Model.ByFile, want) {
		t.Fatalf("ByFile = %#v, want %#v", result.Model.ByFile, want)
	}

	wire, err := result.Model.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"a.go", root, "CONFIG client", "tracked_files"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("provider model leaked %q: %s", forbidden, wire)
		}
	}
}

func TestCanonicalJSONIsCompactDeterministicAndStrict(t *testing.T) {
	t.Parallel()

	model := Model{Version: Version, ByFile: map[corpus.FileID]map[string]uint8{
		"f2": {"worker": 255, "config": 2},
		"f1": {"client": 1},
	}}
	wire, err := model.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"by_file":{"f1":{"client":1},"f2":{"config":2,"worker":255}}}`
	if string(wire) != want {
		t.Fatalf("CanonicalJSON() = %s, want %s", wire, want)
	}
	if len(wire) != 74 {
		t.Fatalf("canonical fixture size = %d bytes, want 74", len(wire))
	}
	for name, invalid := range map[string]Model{
		"zero":      {Version: Version, ByFile: map[corpus.FileID]map[string]uint8{"f1": {"client": 0}}},
		"term":      {Version: Version, ByFile: map[corpus.FileID]map[string]uint8{"f1": {"main": 1}}},
		"file ref":  {Version: Version, ByFile: map[corpus.FileID]map[string]uint8{"x1": {"client": 1}}},
		"ref range": {Version: Version, ByFile: map[corpus.FileID]map[string]uint8{"f1000001": {"client": 1}}},
		"empty row": {Version: Version, ByFile: map[corpus.FileID]map[string]uint8{"f1": {}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := invalid.CanonicalJSON(); err == nil {
				t.Fatal("CanonicalJSON() accepted invalid model")
			}
		})
	}
}

func TestCanonicalModelHasSmallLinearEnvelope(t *testing.T) {
	t.Parallel()

	const files = 1_000
	byFile := make(map[corpus.FileID]map[string]uint8, files)
	for index := 1; index <= files; index++ {
		counts := make(map[string]uint8, len(closedTerms))
		for _, term := range closedTerms {
			counts[term] = CountCap
		}
		byFile[corpus.FileID("f"+decimal(index))] = counts
	}
	wire, err := (Model{Version: Version, ByFile: byFile}).CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	// Even the dense worst row stays below 128 bytes per hit file. Normal
	// repositories are sparse, so their actual payload is considerably smaller.
	if got, limit := len(wire), 128*files; got > limit {
		t.Fatalf("dense model = %d bytes, limit %d", got, limit)
	}
}

func TestScanReadsCurrentWorkingTreeAndHonorsCancellation(t *testing.T) {
	t.Parallel()

	repository, root := testCorpus(t, map[string][]byte{
		"changed.py": []byte("nothing yet"),
		"removed.py": []byte("worker"),
	})
	if err := os.WriteFile(filepath.Join(root, "changed.py"), []byte("request REQUEST"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "removed.py")); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	changedRef, _ := repository.ID("changed.py")
	if !reflect.DeepEqual(result.Model.ByFile, map[corpus.FileID]map[string]uint8{
		changedRef: {"request": 2},
	}) {
		t.Fatalf("ByFile = %#v", result.Model.ByFile)
	}
	if result.Coverage.ScannedFiles != 1 || result.Coverage.ReadOmissions != 1 {
		t.Fatalf("Coverage = %#v", result.Coverage)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, repository); err != context.Canceled {
		t.Fatalf("Scan() error = %v, want context.Canceled", err)
	}
}

func TestStateIsOwnedAndExact(t *testing.T) {
	t.Parallel()

	state := State()
	wantTerms := []string{"config", "dao", "sql", "request", "response", "client", "route", "worker", "kafka"}
	if state.Version != Version || state.MaxWorkers != 4 || state.CountCap != 255 ||
		!reflect.DeepEqual(state.Terms, wantTerms) {
		t.Fatalf("State() = %#v", state)
	}
	state.Terms[0] = "mutated"
	if State().Terms[0] != "config" {
		t.Fatal("State() returned shared term storage")
	}
}

func BenchmarkScanFiveHundredFiles(b *testing.B) {
	files := make(map[string][]byte, 500)
	content := []byte(strings.Repeat("config DAO sql request response client route worker kafka\n", 50))
	for index := 1; index <= 500; index++ {
		files["src/file"+decimal(index)+".go"] = content
	}
	repository, _ := testCorpus(b, files)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := Scan(context.Background(), repository); err != nil {
			b.Fatal(err)
		}
	}
}

func decimal(value int) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[position:])
}

func testCorpus(t testing.TB, files map[string][]byte) (*corpus.Corpus, string) {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, 0, len(files))
	for filePath, content := range files {
		absolute := filepath.Join(root, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, content, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filePath)
	}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{
		Paths:        append([]string(nil), paths...),
		RegularPaths: append([]string(nil), paths...),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close corpus: %v", err)
		}
	})
	return repository, root
}
