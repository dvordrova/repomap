package clientrecipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

const (
	robustnessGoldenVersion = 2
	h1FreezeReceiptVersion  = 2

	historicalH1FreezeSourceSHA256 = "c81b2dafe112358068a05e4dd3c6ac595195713c226260ef8c2214511013b7c2"
	historicalRobustnessRawSHA256  = "6c0a9d763c4fcb8923a49bec16b3352213bc0b69bb58544ba77505eed608df47"
	historicalAuthorityRawSHA256   = "6a3b9fb6721674c7c8e6e6c26ed63e5c2c8ea84d0e96ecaacf0b6f355272031f"
	historicalH0RawSHA256          = "140d7f786af7c26c26a21ccfd040dc3beb8040231baec20cfecf2780dee09f66"
	historicalH1RawSHA256          = "c58f7b97475b5e37cacf64388a793defd9d1ab314a2d2ab09781b56e46459b06"
	historicalEvaluationRawSHA256  = "3ce0e102056b170f42eec1de5626b172071cf0f4f069981007b418003a7799b4"
	historicalOracleRawSHA256      = "5474c64528cded2087fa485045d60072818ca6c63c5e9d13756ddaf287a35b40"
)

type robustnessGolden struct {
	Version         int                 `json:"version"`
	ExtractorFreeze h1FreezeReceipt     `json:"extractor_freeze"`
	Scorecard       robustnessScorecard `json:"scorecard"`
	Mutants         []robustnessMutant  `json:"mutants"`
}

type h1FreezeReceipt struct {
	Version                  int              `json:"version"`
	Status                   string           `json:"status"`
	Scope                    string           `json:"scope"`
	AuthorityVersion         int              `json:"authority_version"`
	H0Version                int              `json:"h0_version"`
	H1Version                int              `json:"h1_version"`
	EvaluationVersion        int              `json:"evaluation_version"`
	BaselineAuthoritySHA256  string           `json:"baseline_authority_sha256"`
	BaselineH0SHA256         string           `json:"baseline_h0_sha256"`
	BaselineH1SHA256         string           `json:"baseline_h1_sha256"`
	BaselineOracleSHA256     string           `json:"baseline_oracle_sha256"`
	BaselineEvaluationSHA256 string           `json:"baseline_evaluation_sha256"`
	SourceSHA256             string           `json:"source_sha256"`
	Sources                  []h1FreezeSource `json:"sources"`
	Rules                    []h1FreezeRule   `json:"rules"`
	SHA256                   string           `json:"sha256"`
}

type h1FreezeSource struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type h1FreezeRule struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
}

type robustnessScorecard struct {
	Verdict              EvaluationVerdict `json:"verdict"`
	Claim                string            `json:"claim"`
	Scope                string            `json:"scope"`
	Passed               int               `json:"passed"`
	Total                int               `json:"total"`
	Dimensions           []string          `json:"dimensions"`
	FeasibilityStatus    string            `json:"feasibility_status"`
	GeneralizationStatus string            `json:"generalization_status"`
	UserUtilityStatus    string            `json:"user_utility_status"`
	ProductionReadiness  string            `json:"production_readiness"`
}

type robustnessMutant struct {
	Name                 string                `json:"name"`
	Dimension            string                `json:"dimension"`
	Invariant            string                `json:"invariant"`
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
	repositoryRoot := filepath.Clean(filepath.Join(experimentRoot(t), "..", "..", ".."))
	baseOracle, err := DecodeOracle(readExperimentFile(t, filepath.Join(experimentRoot(t), "oracle.json")))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		dimension   string
		invariant   string
		oracleDelta string
		mutate      func(*testing.T, string, *Oracle)
	}{
		{
			name:        "noise_false_shapes",
			dimension:   "noise",
			invariant:   "irrelevant lookalike code does not change admitted boundaries, roles, evidence, exclusions, or callback accounting",
			oracleDelta: "none; added shapes remain outside the oracle universe",
			mutate:      mutateFalseShapeNoise,
		},
		{
			name:        "renamed_symbols_and_boundary_directory",
			dimension:   "rename",
			invariant:   "constructor, configuration, wrapper, helper, and directory names are not semantic admission authority",
			oracleDelta: "source paths and display symbols renamed; task semantics unchanged",
			mutate:      mutateNamesAndBoundaryDirectory,
		},
		{
			name:        "verification_layout_integration_to_unit",
			dimension:   "layout",
			invariant:   "verification remains grounded after relocation from an integration package to a package-local unit test",
			oracleDelta: "clickhouse verification path and kind changed to unit_test",
			mutate:      mutateVerificationLayout,
		},
	}

	golden := robustnessGolden{
		Version:         robustnessGoldenVersion,
		ExtractorFreeze: buildH1FreezeReceipt(t),
		Mutants:         make([]robustnessMutant, 0, len(tests)),
	}
	assertH1FreezeRejectsDrift(t, golden.ExtractorFreeze, repositoryRoot)
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
				Name: test.name, Dimension: test.dimension, Invariant: test.invariant,
				OracleDelta: test.oracleDelta, Verdict: evaluation.Verdict,
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
	golden.Scorecard = buildRobustnessScorecard(golden.Mutants)
	if err := golden.Validate(repositoryRoot); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	var decoded robustnessGolden
	if err := decodeStrict(raw, &decoded, "robustness golden"); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(repositoryRoot); err != nil {
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

func buildH1FreezeReceipt(t *testing.T) h1FreezeReceipt {
	t.Helper()
	repositoryRoot := filepath.Clean(filepath.Join(experimentRoot(t), "..", "..", ".."))
	// This receipt records the Cycle 1 extractor that was actually evaluated.
	// It is a historical snapshot, not a validator for today's source tree.
	// Current-source compatibility is established independently by rebuilding
	// the controlled Authority/H0/H1 artifacts and comparing their exact bytes.
	raw := readExperimentFile(t, filepath.Join(experimentRoot(t), "golden", "05-robustness.json"))
	if blindBytesSHA256(raw) != historicalRobustnessRawSHA256 {
		t.Fatal("client recipe H1 freeze: historical robustness bytes changed")
	}
	var historical robustnessGolden
	if err := decodeStrict(raw, &historical, "historical robustness golden"); err != nil {
		t.Fatal(err)
	}
	receipt := historical.ExtractorFreeze
	if err := receipt.Validate(repositoryRoot); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func h1FreezeRules() []h1FreezeRule {
	return []h1FreezeRule{
		{ID: "admissibility", Statement: "start from each exact external dependency/importer H0 candidate and require one restorable local structural wrapper"},
		{ID: "clustering", Statement: "bind one boundary instance to one H0 candidate and its single exact wrapper, constructor flow, cross-package wiring flow, and live consumer flow"},
		{ID: "completeness", Statement: "complete requires configuration, construction, local wrapper, consumer boundary, application wiring, production operation, verification, and observability; failure policy is not mandatory"},
		{ID: "exclusions", Statement: "retain generated, test-only, not-production-reachable, and not-external-boundary candidates as explicit closed exclusions"},
		{ID: "ranking", Statement: "among complete instances, maximize learned required-and-common role coverage and retain every exact tie as equally eligible"},
		{ID: "role_extraction", Statement: "derive closed roles only from exact typed relations, structural source forms, source classifications, production reachability, and grounded source evidence"},
		{ID: "role_reduction", Statement: "reduce roles over complete instances: all is required, at least two and two-thirds is common, and the remainder is optional"},
	}
}

func assertH1FreezeRejectsDrift(t *testing.T, frozen h1FreezeReceipt, repositoryRoot string) {
	t.Helper()
	clone := func() h1FreezeReceipt {
		value := frozen
		value.Sources = append([]h1FreezeSource(nil), frozen.Sources...)
		value.Rules = append([]h1FreezeRule(nil), frozen.Rules...)
		return value
	}
	reseal := func(value h1FreezeReceipt) h1FreezeReceipt {
		value.SourceSHA256 = h1FreezeSourcesDigest(value.Sources)
		value.SHA256 = h1FreezeDigest(value)
		return value
	}
	mutations := []struct {
		name   string
		mutate func(h1FreezeReceipt) h1FreezeReceipt
	}{
		{name: "omitted source", mutate: func(value h1FreezeReceipt) h1FreezeReceipt {
			value.Sources = value.Sources[1:]
			return reseal(value)
		}},
		{name: "invented source", mutate: func(value h1FreezeReceipt) h1FreezeReceipt {
			value.Sources = append(value.Sources, h1FreezeSource{
				Path: "zz-freeze-probe.go", SHA256: strings.Repeat("0", 64),
			})
			return reseal(value)
		}},
		{name: "oracle identity", mutate: func(value h1FreezeReceipt) h1FreezeReceipt {
			value.BaselineOracleSHA256 = strings.Repeat("0", 64)
			return reseal(value)
		}},
		{name: "evaluation identity", mutate: func(value h1FreezeReceipt) h1FreezeReceipt {
			value.BaselineEvaluationSHA256 = strings.Repeat("0", 64)
			return reseal(value)
		}},
	}
	for _, mutation := range mutations {
		if err := mutation.mutate(clone()).Validate(repositoryRoot); err == nil {
			t.Fatalf("H1 freeze accepted %s drift", mutation.name)
		}
	}
}

type h1FreezeBaseline struct {
	AuthoritySHA256  string
	H0SHA256         string
	H1SHA256         string
	OracleSHA256     string
	EvaluationSHA256 string
}

func loadH1FreezeBaseline(repositoryRoot string) (h1FreezeBaseline, error) {
	experiment := filepath.Join(repositoryRoot, "testdata", "experiments", "clientrecipe")
	read := func(relative string) ([]byte, error) {
		raw, err := os.ReadFile(filepath.Join(experiment, filepath.FromSlash(relative)))
		if err != nil {
			return nil, fmt.Errorf("client recipe H1 freeze: read %s: %w", relative, err)
		}
		return raw, nil
	}
	authorityRaw, err := read("golden/01-input-authority.json")
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	if blindBytesSHA256(authorityRaw) != historicalAuthorityRawSHA256 {
		return h1FreezeBaseline{}, fmt.Errorf("client recipe H1 freeze: historical Authority raw bytes changed")
	}
	authority, err := DecodeAuthority(authorityRaw)
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	h0Raw, err := read("golden/02-h0-candidates.json")
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	if blindBytesSHA256(h0Raw) != historicalH0RawSHA256 {
		return h1FreezeBaseline{}, fmt.Errorf("client recipe H1 freeze: historical H0 raw bytes changed")
	}
	h0, err := DecodeH0(h0Raw)
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	h1Raw, err := read("golden/03-h1-structural.json")
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	if blindBytesSHA256(h1Raw) != historicalH1RawSHA256 {
		return h1FreezeBaseline{}, fmt.Errorf("client recipe H1 freeze: historical H1 raw bytes changed")
	}
	h1, err := DecodeH1(h1Raw)
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	oracleRaw, err := read("oracle.json")
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	if blindBytesSHA256(oracleRaw) != historicalOracleRawSHA256 {
		return h1FreezeBaseline{}, fmt.Errorf("client recipe H1 freeze: historical oracle raw bytes changed")
	}
	oracle, err := DecodeOracle(oracleRaw)
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	evaluationRaw, err := read("golden/04-evaluation.json")
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	if blindBytesSHA256(evaluationRaw) != historicalEvaluationRawSHA256 {
		return h1FreezeBaseline{}, fmt.Errorf("client recipe H1 freeze: historical evaluation raw bytes changed")
	}
	evaluation, err := DecodeEvaluation(evaluationRaw)
	if err != nil {
		return h1FreezeBaseline{}, err
	}
	if h0.AuthoritySHA256 != authority.SHA256 || h1.AuthoritySHA256 != authority.SHA256 ||
		h1.H0SHA256 != h0.SHA256 || evaluation.H0SHA256 != h0.SHA256 ||
		evaluation.H1SHA256 != h1.SHA256 || evaluation.OracleSHA256 != oracleEvaluationDigest(oracle) {
		return h1FreezeBaseline{}, fmt.Errorf("client recipe H1 freeze: baseline identity chain mismatch")
	}
	oracleDigest := sha256.Sum256(oracleRaw)
	return h1FreezeBaseline{
		AuthoritySHA256:  authority.SHA256,
		H0SHA256:         h0.SHA256,
		H1SHA256:         h1.SHA256,
		OracleSHA256:     hex.EncodeToString(oracleDigest[:]),
		EvaluationSHA256: evaluation.SHA256,
	}, nil
}

func discoverH1FreezeSources(repositoryRoot string) ([]h1FreezeSource, error) {
	packagePath := filepath.Join(repositoryRoot, "internal", "experiments", "clientrecipe")
	entries, err := os.ReadDir(packagePath)
	if err != nil {
		return nil, fmt.Errorf("client recipe H1 freeze: discover package sources: %w", err)
	}
	paths := make([]string, 0, len(entries)+6)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("client recipe H1 freeze: package source %s is not regular", entry.Name())
		}
		paths = append(paths, filepath.ToSlash(filepath.Join("internal", "experiments", "clientrecipe", entry.Name())))
	}
	paths = append(paths,
		"go.mod",
		"go.sum",
		"internal/dependencies/catalog.go",
		"internal/programindex/index.go",
		"testdata/experiments/clientrecipe/negative_invariants.json",
		"testdata/experiments/clientrecipe/task_contract.json",
	)
	sort.Strings(paths)
	sources := make([]h1FreezeSource, 0, len(paths))
	previous := ""
	for _, sourcePath := range paths {
		if previous == sourcePath {
			return nil, fmt.Errorf("client recipe H1 freeze: duplicate source %s", sourcePath)
		}
		previous = sourcePath
		filename := filepath.Join(repositoryRoot, filepath.FromSlash(sourcePath))
		info, err := os.Lstat(filename)
		if err != nil {
			return nil, fmt.Errorf("client recipe H1 freeze: inspect %s: %w", sourcePath, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("client recipe H1 freeze: source %s is not regular", sourcePath)
		}
		raw, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("client recipe H1 freeze: read %s: %w", sourcePath, err)
		}
		digest := sha256.Sum256(raw)
		sources = append(sources, h1FreezeSource{Path: sourcePath, SHA256: hex.EncodeToString(digest[:])})
	}
	return sources, nil
}

func buildRobustnessScorecard(mutants []robustnessMutant) robustnessScorecard {
	dimensionSet := make(map[string]struct{}, len(mutants))
	passed := 0
	for _, mutant := range mutants {
		dimensionSet[mutant.Dimension] = struct{}{}
		if mutant.Verdict == EvaluationPass {
			passed++
		}
	}
	dimensions := make([]string, 0, len(dimensionSet))
	for dimension := range dimensionSet {
		dimensions = append(dimensions, dimension)
	}
	sort.Strings(dimensions)
	verdict := EvaluationFail
	if passed == len(mutants) && len(mutants) > 0 {
		verdict = EvaluationPass
	}
	return robustnessScorecard{
		Verdict: verdict,
		Claim:   "the frozen H1 behavior preserved its oracle score under rename, layout, and irrelevant-noise mutations of the same controlled fixture",
		Scope:   "controlled_fixture_mutations", Passed: passed, Total: len(mutants), Dimensions: dimensions,
		FeasibilityStatus: "ESTABLISHED", GeneralizationStatus: "NOT_ESTABLISHED",
		UserUtilityStatus: "NOT_TESTED", ProductionReadiness: "NOT_READY",
	}
}

func (value robustnessGolden) Validate(repositoryRoot string) error {
	if value.Version != robustnessGoldenVersion || value.Mutants == nil {
		return fmt.Errorf("client recipe robustness: invalid identity")
	}
	if err := value.ExtractorFreeze.Validate(repositoryRoot); err != nil {
		return err
	}
	wantScorecard := buildRobustnessScorecard(value.Mutants)
	if !reflect.DeepEqual(value.Scorecard, wantScorecard) {
		return fmt.Errorf("client recipe robustness: scorecard does not match mutant outcomes")
	}
	previous := ""
	for _, mutant := range value.Mutants {
		if mutant.Name == "" || mutant.Dimension == "" || mutant.Invariant == "" || mutant.OracleDelta == "" ||
			!mutant.Verdict.Valid() || (previous != "" && previous >= mutant.Name) {
			return fmt.Errorf("client recipe robustness: invalid or non-canonical mutant %q", mutant.Name)
		}
		previous = mutant.Name
	}
	return nil
}

func (value h1FreezeReceipt) Validate(repositoryRoot string) error {
	if value.Version != h1FreezeReceiptVersion || value.Status != "FROZEN" ||
		value.Scope != "test_only_h1_generalization_gate" || value.AuthorityVersion != AuthorityVersion ||
		value.H0Version != H0Version || value.H1Version != H1Version || value.EvaluationVersion != EvaluationVersion ||
		!validSHA256(value.BaselineAuthoritySHA256) || !validSHA256(value.BaselineH0SHA256) ||
		!validSHA256(value.BaselineH1SHA256) || !validSHA256(value.BaselineOracleSHA256) ||
		!validSHA256(value.BaselineEvaluationSHA256) || !validSHA256(value.SourceSHA256) || !validSHA256(value.SHA256) ||
		len(value.Sources) == 0 || len(value.Rules) == 0 {
		return fmt.Errorf("client recipe H1 freeze: invalid identity")
	}
	previous := ""
	for _, source := range value.Sources {
		if !validSourcePath(source.Path) || !validSHA256(source.SHA256) || (previous != "" && previous >= source.Path) {
			return fmt.Errorf("client recipe H1 freeze: invalid or non-canonical source %q", source.Path)
		}
		previous = source.Path
	}
	previous = ""
	for _, rule := range value.Rules {
		if rule.ID == "" || rule.Statement == "" || (previous != "" && previous >= rule.ID) {
			return fmt.Errorf("client recipe H1 freeze: invalid or non-canonical rule %q", rule.ID)
		}
		previous = rule.ID
	}
	if !reflect.DeepEqual(value.Rules, h1FreezeRules()) {
		return fmt.Errorf("client recipe H1 freeze: rule surface mismatch")
	}
	if value.SourceSHA256 != historicalH1FreezeSourceSHA256 || value.SHA256 != frozenReceiptSHA256 {
		return fmt.Errorf("client recipe H1 freeze: historical receipt identity mismatch")
	}
	baseline, err := loadH1FreezeBaseline(repositoryRoot)
	if err != nil {
		return err
	}
	if value.BaselineAuthoritySHA256 != baseline.AuthoritySHA256 || value.BaselineH0SHA256 != baseline.H0SHA256 ||
		value.BaselineH1SHA256 != baseline.H1SHA256 || value.BaselineOracleSHA256 != baseline.OracleSHA256 ||
		value.BaselineEvaluationSHA256 != baseline.EvaluationSHA256 {
		return fmt.Errorf("client recipe H1 freeze: stored baseline identity does not match canonical artifacts")
	}
	if value.SourceSHA256 != h1FreezeSourcesDigest(value.Sources) || value.SHA256 != h1FreezeDigest(value) {
		return fmt.Errorf("client recipe H1 freeze: digest mismatch")
	}
	return nil
}

func h1FreezeSourcesDigest(sources []h1FreezeSource) string {
	raw, _ := json.Marshal(sources)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func h1FreezeDigest(value h1FreezeReceipt) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
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
