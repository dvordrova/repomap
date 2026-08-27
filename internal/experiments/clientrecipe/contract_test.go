package clientrecipe

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestTaskContract(t *testing.T) {
	root := experimentRoot(t)
	raw := readExperimentFile(t, filepath.Join(root, "task_contract.json"))
	contract, err := DecodeTaskContract(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.UserNeeds) != 9 || len(contract.StateMachine) != 5 {
		t.Fatalf("unexpected task coverage: %d needs, %d states", len(contract.UserNeeds), len(contract.StateMachine))
	}
	assertStrictDecoder(t, raw, func(candidate []byte) error {
		_, err := DecodeTaskContract(candidate)
		return err
	})
}

func TestTraceability(t *testing.T) {
	root := experimentRoot(t)
	contract, err := DecodeTaskContract(readExperimentFile(t, filepath.Join(root, "task_contract.json")))
	if err != nil {
		t.Fatal(err)
	}
	traceabilityRaw := readExperimentFile(t, filepath.Join(root, "traceability.json"))
	traceability, err := DecodeTraceability(traceabilityRaw)
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := DecodeSignalMatrix(readExperimentFile(t, filepath.Join(root, "signal_matrix.json")))
	if err != nil {
		t.Fatal(err)
	}
	signals := make(map[string]struct{}, len(matrix.Rows))
	for _, row := range matrix.Rows {
		signals[row.Signal] = struct{}{}
	}
	mapped := make(map[string]int, len(contract.UserNeeds))
	for _, row := range traceability.Rows {
		mapped[row.UserNeed]++
		if _, ok := signals[row.RequiredSignal]; !ok {
			t.Errorf("traceability need %q cites unknown signal %q", row.UserNeed, row.RequiredSignal)
		}
	}
	for _, need := range contract.UserNeeds {
		if mapped[need.ID] == 0 {
			t.Errorf("user need %q has no UI-to-authority trace", need.ID)
		}
	}
	assertStrictDecoder(t, traceabilityRaw, func(candidate []byte) error {
		_, err := DecodeTraceability(candidate)
		return err
	})
}

func TestSignalMatrix(t *testing.T) {
	raw := readExperimentFile(t, filepath.Join(experimentRoot(t), "signal_matrix.json"))
	matrix, err := DecodeSignalMatrix(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"bootstrap", "config_value_flow", "constructor_callsite", "consumer_interface",
		"consumer_operation", "dead_code", "external_package", "formal_role_reduction",
		"generated", "instance_completeness", "integration_verification", "logging", "metrics",
		"metrics_logging", "production_reachability", "retry_policy", "source_locations",
		"test_assertions", "test_fake", "test_only", "timeout_policy", "unit_verification",
		"unreachable", "wiring", "wrapper",
	}
	if len(matrix.Rows) != len(want) {
		t.Fatalf("signal rows = %d, want %d", len(matrix.Rows), len(want))
	}
	for index, signal := range want {
		if matrix.Rows[index].Signal != signal {
			t.Fatalf("signal %d = %q, want %q", index, matrix.Rows[index].Signal, signal)
		}
	}
	assertStrictDecoder(t, raw, func(candidate []byte) error {
		_, err := DecodeSignalMatrix(candidate)
		return err
	})
}

func TestNegativeInvariants(t *testing.T) {
	raw := readExperimentFile(t, filepath.Join(experimentRoot(t), "negative_invariants.json"))
	value, err := DecodeNegativeInvariants(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"accounting_exact", "evidence_closed_initially", "incomplete_not_best", "no_default_network",
		"no_generated_promotion", "no_invented_edge", "no_invented_ref", "no_invented_source",
		"no_invented_step", "no_oracle_input", "no_stdlib_promotion", "no_test_only_promotion",
		"no_unreachable_promotion",
	}
	if len(value.Invariants) != len(want) {
		t.Fatalf("negative invariants = %d, want %d", len(value.Invariants), len(want))
	}
	for index, id := range want {
		if value.Invariants[index].ID != id {
			t.Fatalf("negative invariant %d = %q, want %q", index, value.Invariants[index].ID, id)
		}
	}
	assertStrictDecoder(t, raw, func(candidate []byte) error {
		_, err := DecodeNegativeInvariants(candidate)
		return err
	})
}

func TestOracleIsolation(t *testing.T) {
	experiment := experimentRoot(t)
	repoRoot := filepath.Join(experiment, "repo")
	oraclePath := filepath.Join(experiment, "oracle.json")
	relative, err := filepath.Rel(repoRoot, oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	if relative == "." || !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("oracle must be outside analyzed repository, relative path is %q", relative)
	}
	files, err := RepositoryFiles(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, filename := range files {
		if strings.Contains(filename, "oracle") || strings.HasPrefix(filename, "../") {
			t.Fatalf("repo-root source walk escaped into hidden authority: %q", filename)
		}
	}
	oracleRaw := readExperimentFile(t, oraclePath)
	if _, err := DecodeOracle(oracleRaw); err != nil {
		t.Fatal(err)
	}
	escaped := bytes.Replace(oracleRaw, []byte(`"cmd/service/main.go"`), []byte(`"../oracle.json"`), 1)
	if _, err := DecodeOracle(escaped); err == nil {
		t.Fatal("oracle decoder accepted a path traversal locator")
	}
	assertStrictDecoder(t, oracleRaw, func(candidate []byte) error {
		_, err := DecodeOracle(candidate)
		return err
	})
}

func TestFixtureCompiles(t *testing.T) {
	repoRoot := filepath.Join(experimentRoot(t), "repo")
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("offline fixture go test: %v\n%s", err, output)
	}
}

func TestOracleLocators(t *testing.T) {
	experiment := experimentRoot(t)
	oracle, err := DecodeOracle(readExperimentFile(t, filepath.Join(experiment, "oracle.json")))
	if err != nil {
		t.Fatal(err)
	}
	locators := append(append([]SourceLocator{}, oracle.Bootstrap...), oracle.Entrypoints...)
	for _, instance := range oracle.Instances {
		for _, slot := range instance.Slots {
			locators = append(locators, slot.Evidence...)
		}
	}
	for _, excluded := range oracle.Excluded {
		locators = append(locators, excluded.Evidence...)
	}
	for _, locator := range locators {
		assertLocator(t, filepath.Join(experiment, "repo"), locator)
	}
}

func TestFixtureShape(t *testing.T) {
	oracle, err := DecodeOracle(readExperimentFile(t, filepath.Join(experimentRoot(t), "oracle.json")))
	if err != nil {
		t.Fatal(err)
	}
	if len(oracle.Instances) != 4 {
		t.Fatalf("admitted instances = %d, want 4", len(oracle.Instances))
	}
	complete := 0
	wantVerification := map[string]string{
		"clickhouse": "integration_test", "kubernetes": "unit_test", "notifier": "none", "vault": "unit_test",
	}
	for _, instance := range oracle.Instances {
		if instance.Complete {
			complete++
		}
		if instance.VerificationKind != wantVerification[instance.ID] {
			t.Errorf("verification kind for %s = %q", instance.ID, instance.VerificationKind)
		}
	}
	if complete != 3 {
		t.Fatalf("complete instances = %d, want 3", complete)
	}
	wantReasons := map[string]int{
		"generated": 1, "test_only": 2, "not_production_reachable": 2, "not_external_boundary": 1,
	}
	for _, excluded := range oracle.Excluded {
		wantReasons[excluded.Reason]--
	}
	for reason, remaining := range wantReasons {
		if remaining != 0 {
			t.Errorf("exclusion reason %s remaining count = %d", reason, remaining)
		}
	}
	assertProductionReachabilityChain(t)

	mutated := oracle
	mutated.ExpectedRoles = append([]OracleRole(nil), oracle.ExpectedRoles...)
	mutated.ExpectedRoles[0].ObservedCompleteInstances--
	if err := mutated.Validate(); err == nil {
		t.Fatal("oracle accepted a stored formal-role count that contradicts complete instances")
	}
}

func assertProductionReachabilityChain(t *testing.T) {
	t.Helper()
	repoRoot := filepath.Join(experimentRoot(t), "repo")
	want := map[string][]string{
		"cmd/service/main.go":        {"service.Run(context.Background())"},
		"internal/launch/service.go": {"NewHandler(service.Resolve)", "handler.HandleStartup(ctx)"},
		"internal/launch/handler.go": {"handler.resolve(ctx", "HistoryLimit: 2"},
	}
	for relative, fragments := range want {
		source := string(readExperimentFile(t, filepath.Join(repoRoot, filepath.FromSlash(relative))))
		for _, fragment := range fragments {
			if !strings.Contains(source, fragment) {
				t.Errorf("production reachability chain %s lacks %q", relative, fragment)
			}
		}
	}
}

func TestRepositoryFilesDeterministic(t *testing.T) {
	repoRoot := filepath.Join(experimentRoot(t), "repo")
	first, err := RepositoryFiles(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RepositoryFiles(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !sort.StringsAreSorted(first) {
		t.Fatalf("repository source inventory is not byte/order deterministic")
	}
	if !bytes.Equal([]byte(strings.Join(first, "\n")), []byte(strings.Join(second, "\n"))) {
		t.Fatal("repository source inventory encoding changed between reads")
	}
}

func experimentRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	return filepath.Join(repositoryRoot, "testdata", "experiments", "clientrecipe")
}

func readExperimentFile(t *testing.T, filename string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	return raw
}

func assertStrictDecoder(t *testing.T, raw []byte, decode func([]byte) error) {
	t.Helper()
	if err := decode(append(append([]byte{}, raw...), []byte("\n{}")...)); err == nil {
		t.Fatal("strict decoder accepted trailing JSON")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '}' {
		t.Fatal("test input is not one JSON object")
	}
	withUnknown := append(append([]byte{}, trimmed[:len(trimmed)-1]...), []byte(",\"unexpected\":true}")...)
	if err := decode(withUnknown); err == nil {
		t.Fatal("strict decoder accepted an unknown field")
	}
}

func assertLocator(t *testing.T, repoRoot string, locator SourceLocator) {
	t.Helper()
	joined := filepath.Join(repoRoot, filepath.FromSlash(locator.Path))
	relative, err := filepath.Rel(repoRoot, joined)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Errorf("locator escaped repository root: %#v", locator)
		return
	}
	file, err := os.Open(joined)
	if err != nil {
		t.Errorf("open locator %s:%d: %v", locator.Path, locator.Line, err)
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		if line == locator.Line {
			if !strings.Contains(scanner.Text(), locator.Symbol) {
				t.Errorf("locator %s:%d does not contain %q: %s", locator.Path, locator.Line, locator.Symbol, scanner.Text())
			}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("scan locator %s:%d: %v", locator.Path, locator.Line, err)
		return
	}
	t.Errorf("locator %s:%d is past end of file", locator.Path, locator.Line)
}
