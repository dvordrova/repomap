package clientrecipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type robustnessGolden struct {
	Version int                `json:"version"`
	Mutants []robustnessMutant `json:"mutants"`
}

type robustnessMutant struct {
	Name                 string                `json:"name"`
	OracleDelta          string                `json:"oracle_delta"`
	Verdict              EvaluationVerdict     `json:"verdict"`
	Ledger               H1Ledger              `json:"ledger"`
	InstanceDiscovery    EvaluationSetMetric   `json:"instance_discovery"`
	RoleCoverage         EvaluationSetMetric   `json:"role_coverage"`
	EvidenceGrounding    EvaluationExactMetric `json:"evidence_grounding"`
	ExclusionGrounding   EvaluationExactMetric `json:"exclusion_grounding"`
	CallbackObserved     int                   `json:"callback_observed"`
	CallbackClosed       int                   `json:"callback_closed"`
	CallbackFrontier     int                   `json:"callback_frontier"`
	ProductionLoadCount  int                   `json:"production_load_count"`
	NotifierMissingRoles []H1Role              `json:"notifier_missing_roles"`
}

func TestH1RobustnessMutants(t *testing.T) {
	baseOracle, err := DecodeOracle(readExperimentFile(t, filepath.Join(experimentRoot(t), "oracle.json")))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		oracleDelta string
		mutate      func(*testing.T, string, *Oracle)
	}{
		{
			name:        "noise_false_shapes",
			oracleDelta: "none; added shapes remain outside the oracle universe",
			mutate:      mutateFalseShapeNoise,
		},
		{
			name:        "renamed_symbols_and_boundary_directory",
			oracleDelta: "source paths and display symbols renamed; task semantics unchanged",
			mutate:      mutateNamesAndBoundaryDirectory,
		},
		{
			name:        "verification_layout_integration_to_unit",
			oracleDelta: "clickhouse verification path and kind changed to unit_test",
			mutate:      mutateVerificationLayout,
		},
	}

	golden := robustnessGolden{Version: 1, Mutants: make([]robustnessMutant, 0, len(tests))}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			experiment := copyExperimentFixture(t)
			oracle := cloneOracle(t, baseOracle)
			test.mutate(t, experiment, &oracle)
			if err := oracle.Validate(); err != nil {
				t.Fatal(err)
			}
			repoRoot := filepath.Join(experiment, "repo")
			assertOracleLocatorsAt(t, repoRoot, oracle)
			loader := &countingProductionLoader{delegate: defaultProductionPackageLoader{}}
			authority, err := prepareAuthority(t.Context(), repoRoot, loader)
			if err != nil {
				t.Fatal(err)
			}
			h0, err := BuildH0(authority)
			if err != nil {
				t.Fatal(err)
			}
			first, err := ExtractH1(repoRoot, authority)
			if err != nil {
				t.Fatal(err)
			}
			second, err := ExtractH1(repoRoot, authority)
			if err != nil {
				t.Fatal(err)
			}
			if loader.count != 1 {
				t.Fatalf("production package loads = %d, want 1", loader.count)
			}
			evaluation, err := EvaluateClientRecipe(h0, first, oracle, second)
			if err != nil {
				t.Fatal(err)
			}
			if evaluation.Verdict != EvaluationPass || evaluation.H1.Verdict != EvaluationPass {
				t.Fatalf("mutant evaluation = overall %s / H1 %s: %#v", evaluation.Verdict, evaluation.H1.Verdict, evaluation.H1)
			}
			if first.Ledger != (H1Ledger{Observed: 10, Admitted: 4, Excluded: 6}) ||
				first.Callbacks.Observed != 4 || first.Callbacks.Closed != 2 || first.Callbacks.Frontier != 2 {
				t.Fatalf("mutant structural accounting = ledger %#v / callbacks %#v", first.Ledger, first.Callbacks)
			}
			missing := incompleteMissingRoles(t, first)
			wantMissing := []H1Role{H1RoleVerification, H1RoleObservability, H1RoleFailurePolicy}
			if !reflect.DeepEqual(missing, wantMissing) {
				t.Fatalf("incomplete instance missing roles = %v, want %v", missing, wantMissing)
			}
			golden.Mutants = append(golden.Mutants, robustnessMutant{
				Name: test.name, OracleDelta: test.oracleDelta, Verdict: evaluation.Verdict,
				Ledger: first.Ledger, InstanceDiscovery: evaluation.H1.InstanceDiscovery,
				RoleCoverage: evaluation.H1.RoleCoverage, EvidenceGrounding: evaluation.H1.EvidenceGrounding,
				ExclusionGrounding: evaluation.H1.ExclusionGrounding,
				CallbackObserved:   first.Callbacks.Observed, CallbackClosed: first.Callbacks.Closed,
				CallbackFrontier: first.Callbacks.Frontier, ProductionLoadCount: loader.count,
				NotifierMissingRoles: missing,
			})
		})
	}
	sort.Slice(golden.Mutants, func(i, j int) bool { return golden.Mutants[i].Name < golden.Mutants[j].Name })
	raw, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	var decoded robustnessGolden
	if err := decodeStrict(raw, &decoded, "robustness golden"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, golden) {
		t.Fatal("robustness strict decoder changed the scorecard")
	}
	assertStrictDecoder(t, raw, func(candidate []byte) error {
		var value robustnessGolden
		return decodeStrict(candidate, &value, "robustness golden")
	})
	assertExperimentGolden(t, "05-robustness.json", raw)
}

func mutateNamesAndBoundaryDirectory(t *testing.T, experiment string, oracle *Oracle) {
	t.Helper()
	repoRoot := filepath.Join(experiment, "repo")
	replacements := [][2]string{
		{"NewClient", "AssembleBoundary"},
		{"ConfigFromApplication", "ProjectSettings"},
		{"type Client struct", "type Boundary struct"},
		{"(*Client", "(*Boundary"},
		{"*Client)", "*Boundary)"},
		{" *Client {", " *Boundary {"},
		{"&Client{", "&Boundary{"},
		{"factory", "maker"},
		{"internal/clients/", "internal/adapters/"},
	}
	rewriteTree(t, repoRoot, func(relative string, raw []byte) []byte {
		if !strings.HasSuffix(relative, ".go") {
			return raw
		}
		for _, replacement := range replacements {
			raw = bytes.ReplaceAll(raw, []byte(replacement[0]), []byte(replacement[1]))
		}
		return raw
	})
	oldDirectory := filepath.Join(repoRoot, "internal", "clients")
	newDirectory := filepath.Join(repoRoot, "internal", "adapters")
	if err := os.Rename(oldDirectory, newDirectory); err != nil {
		t.Fatal(err)
	}
	for _, source := range mustRepositoryFiles(t, repoRoot) {
		if !strings.HasSuffix(source, ".go") {
			continue
		}
		raw := readExperimentFile(t, filepath.Join(repoRoot, filepath.FromSlash(source)))
		for _, forbidden := range []string{"NewClient", "ConfigFromApplication", "type Client struct", "factory", "internal/clients/"} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("rename mutant retained forbidden implementation form %q in %s", forbidden, source)
			}
		}
	}
	mutateOracleLocators(oracle, func(locator SourceLocator) SourceLocator {
		locator.Path = strings.Replace(locator.Path, "internal/clients/", "internal/adapters/", 1)
		for _, replacement := range replacements[:7] {
			locator.Symbol = strings.ReplaceAll(locator.Symbol, replacement[0], replacement[1])
		}
		locator.Symbol = strings.ReplaceAll(locator.Symbol, "Client struct", "Boundary struct")
		locator.Symbol = strings.ReplaceAll(locator.Symbol, "factory", "maker")
		return locator
	})
}

func mutateVerificationLayout(t *testing.T, experiment string, oracle *Oracle) {
	t.Helper()
	repoRoot := filepath.Join(experiment, "repo")
	oldPath := filepath.Join(repoRoot, "test", "integration", "clickhouse_test.go")
	newPath := filepath.Join(repoRoot, "internal", "clients", "clickhouse", "client_test.go")
	raw := readExperimentFile(t, oldPath)
	raw = bytes.Replace(raw, []byte("package integration"), []byte("package clickhouse"), 1)
	raw = bytes.Replace(raw,
		[]byte("\t\"example.com/launchservice/internal/clients/clickhouse\""),
		[]byte("\t// package-local client"), 1)
	raw = bytes.ReplaceAll(raw, []byte("clickhouse.NewClient"), []byte("NewClient"))
	raw = bytes.ReplaceAll(raw, []byte("clickhouse.Config"), []byte("Config"))
	if err := os.WriteFile(newPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	for index := range oracle.Instances {
		if oracle.Instances[index].ID != "clickhouse" {
			continue
		}
		oracle.Instances[index].VerificationKind = "unit_test"
		for slotIndex := range oracle.Instances[index].Slots {
			if oracle.Instances[index].Slots[slotIndex].Role != string(H1RoleVerification) {
				continue
			}
			for evidenceIndex := range oracle.Instances[index].Slots[slotIndex].Evidence {
				oracle.Instances[index].Slots[slotIndex].Evidence[evidenceIndex].Path = "internal/clients/clickhouse/client_test.go"
			}
		}
	}
}

func mutateFalseShapeNoise(t *testing.T, experiment string, _ *Oracle) {
	t.Helper()
	repoRoot := filepath.Join(experiment, "repo")
	writeMutantFile(t, filepath.Join(repoRoot, "internal", "noise", "format.go"), `package noise

import "fmt"

func FormatValue(value string) string { return fmt.Sprintf("%s", value) }
`)
	writeMutantFile(t, filepath.Join(repoRoot, "internal", "clients", "kubernetes", "wrong_signature_test.go"), `package kubernetes

type wrongSignatureNamespaceLister struct{}

func (wrongSignatureNamespaceLister) ListNamespaces() {}
`)
	writeMutantFile(t, filepath.Join(repoRoot, "internal", "audit", "normalize.go"), `package audit

import "strings"

func Normalize(value string) string { return strings.TrimSpace(value) }
`)
	notifierPath := filepath.Join(repoRoot, "internal", "clients", "notifier", "client.go")
	raw := readExperimentFile(t, notifierPath)
	raw = bytes.Replace(raw, []byte("\t\"context\"\n\n"),
		[]byte("\t\"context\"\n\t\"example.com/launchservice/internal/audit\"\n"), 1)
	raw = bytes.Replace(raw, []byte("func (client *Client) SendLaunch(ctx context.Context, message string) error {\n"),
		[]byte("func (client *Client) SendLaunch(ctx context.Context, message string) error {\n\t_ = audit.Normalize(message)\n"), 1)
	if err := os.WriteFile(notifierPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyExperimentFixture(t *testing.T) string {
	t.Helper()
	source := experimentRoot(t)
	destination := filepath.Join(t.TempDir(), "clientrecipe")
	err := filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular experiment fixture entry %s", relative)
		}
		raw, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return destination
}

func rewriteTree(t *testing.T, root string, rewrite func(string, []byte) []byte) {
	t.Helper()
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		updated := rewrite(filepath.ToSlash(relative), raw)
		if bytes.Equal(raw, updated) {
			return nil
		}
		return os.WriteFile(filename, updated, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mutateOracleLocators(oracle *Oracle, mutate func(SourceLocator) SourceLocator) {
	for index := range oracle.Bootstrap {
		oracle.Bootstrap[index] = mutate(oracle.Bootstrap[index])
	}
	for index := range oracle.Entrypoints {
		oracle.Entrypoints[index] = mutate(oracle.Entrypoints[index])
	}
	for instanceIndex := range oracle.Instances {
		for slotIndex := range oracle.Instances[instanceIndex].Slots {
			for evidenceIndex := range oracle.Instances[instanceIndex].Slots[slotIndex].Evidence {
				oracle.Instances[instanceIndex].Slots[slotIndex].Evidence[evidenceIndex] =
					mutate(oracle.Instances[instanceIndex].Slots[slotIndex].Evidence[evidenceIndex])
			}
		}
	}
	for excludedIndex := range oracle.Excluded {
		for evidenceIndex := range oracle.Excluded[excludedIndex].Evidence {
			oracle.Excluded[excludedIndex].Evidence[evidenceIndex] =
				mutate(oracle.Excluded[excludedIndex].Evidence[evidenceIndex])
		}
	}
}

func cloneOracle(t *testing.T, value Oracle) Oracle {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone Oracle
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func assertOracleLocatorsAt(t *testing.T, repoRoot string, oracle Oracle) {
	t.Helper()
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
		assertLocator(t, repoRoot, locator)
	}
}

func incompleteMissingRoles(t *testing.T, result H1Result) []H1Role {
	t.Helper()
	var missing []H1Role
	for _, instance := range result.Instances {
		if instance.Complete {
			continue
		}
		if missing != nil {
			t.Fatal("more than one incomplete client instance")
		}
		missing = append([]H1Role(nil), instance.Missing...)
	}
	if missing == nil {
		t.Fatal("missing incomplete client instance")
	}
	return missing
}

func writeMutantFile(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRepositoryFiles(t *testing.T, repoRoot string) []string {
	t.Helper()
	files, err := RepositoryFiles(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return files
}
