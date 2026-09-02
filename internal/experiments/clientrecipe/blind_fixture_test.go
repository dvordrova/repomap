package clientrecipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	blindOracleVersion   = 1
	blindReceiptVersion  = 1
	blindScoreVersion    = 1
	cycle4FreezeVersion  = 1
	cycle4ReceiptVersion = 1

	cycle4CandidateID = "authority-collision-fix-v2"

	historicalBlindTerminalRawSHA256       = "e4501c066ce94febe0d2d98992743f3e95da54a28efb8fd2538501b0ee317da7"
	historicalBlindTerminalInternalSHA256  = "8fc36a875591be820092790fe0335ceb13d652cfa83865a5cb35e7c6d18bb659"
	historicalBlindFixtureReceiptRawSHA256 = "ab69d7842b7509abddbc302b7e2266d47a70f2530e24fcebfcedcb21208b4f89"
)

type cycle4ArtifactBinding struct {
	Stage          string `json:"stage"`
	Path           string `json:"path"`
	RawSHA256      string `json:"raw_sha256"`
	InternalSHA256 string `json:"internal_sha256"`
}

type cycle4BlindSnapshot struct {
	AuthorBriefRawSHA256     string `json:"author_brief_raw_sha256"`
	PreregistrationRawSHA256 string `json:"preregistration_raw_sha256"`
	OracleSchemaRawSHA256    string `json:"oracle_schema_raw_sha256"`
	OracleRawSHA256          string `json:"oracle_raw_sha256"`
	FixtureReceiptRawSHA256  string `json:"fixture_receipt_raw_sha256"`
	FixtureReceiptSHA256     string `json:"fixture_receipt_sha256"`
	RepositoryTreeSHA256     string `json:"repository_tree_sha256"`
	DependenciesTreeSHA256   string `json:"dependencies_tree_sha256"`
}

type cycle4HistoricalTerminal struct {
	Path           string `json:"path"`
	RawSHA256      string `json:"raw_sha256"`
	InternalSHA256 string `json:"internal_sha256"`
	Stage          string `json:"stage"`
	Reason         string `json:"reason"`
}

type cycle4CollisionContract struct {
	Classification string         `json:"classification"`
	ExpectedGroups int            `json:"expected_groups"`
	Sites          []blindLocator `json:"sites"`
	IdentityRule   string         `json:"identity_rule"`
}

type cycle4EvaluatorContract struct {
	InputsSHA256 string                 `json:"inputs_sha256"`
	Inputs       []h1FreezeSource       `json:"inputs"`
	Thresholds   blindThresholdContract `json:"thresholds"`
	OverallRule  string                 `json:"overall_rule"`
}

type cycle4ExtractorFreeze struct {
	Version                int                      `json:"version"`
	Status                 string                   `json:"status"`
	CandidateID            string                   `json:"candidate_id"`
	CandidateSourcesSHA256 string                   `json:"candidate_sources_sha256"`
	CandidateSources       []h1FreezeSource         `json:"candidate_sources"`
	Rules                  []h1FreezeRule           `json:"rules"`
	ControlledArtifacts    []cycle4ArtifactBinding  `json:"controlled_artifacts"`
	BlindSnapshot          cycle4BlindSnapshot      `json:"blind_snapshot"`
	HistoricalTerminal     cycle4HistoricalTerminal `json:"historical_terminal"`
	Collision              cycle4CollisionContract  `json:"collision"`
	Evaluator              cycle4EvaluatorContract  `json:"evaluator"`
	SHA256                 string                   `json:"sha256"`
}

type cycle4CollisionObservation struct {
	Classification  string         `json:"classification"`
	Groups          int            `json:"groups"`
	Sites           []blindLocator `json:"sites"`
	RelationsBefore int            `json:"relations_before"`
	RelationsAfter  int            `json:"relations_after"`
}

type cycle4RunArtifact struct {
	Stage          string `json:"stage"`
	RawSHA256      string `json:"raw_sha256"`
	InternalSHA256 string `json:"internal_sha256"`
}

type cycle4OracleIsolation struct {
	Status                  string `json:"status"`
	CandidateBytesUnchanged bool   `json:"candidate_bytes_unchanged"`
	EvaluationBytesChanged  bool   `json:"evaluation_bytes_changed"`
}

type cycle4TerminalState struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

type cycle4RunReceipt struct {
	Version                 int                        `json:"version"`
	Status                  string                     `json:"status"`
	CandidateID             string                     `json:"candidate_id"`
	ExtractorFreezeSHA256   string                     `json:"extractor_freeze_sha256"`
	Artifacts               []cycle4RunArtifact        `json:"artifacts"`
	UnavailableArtifacts    []string                   `json:"unavailable_artifacts"`
	Builds                  int                        `json:"builds"`
	ProductionLoadsPerBuild []int                      `json:"production_loads_per_build"`
	Collision               cycle4CollisionObservation `json:"collision"`
	CandidateDeterministic  bool                       `json:"candidate_deterministic"`
	EvaluationDeterministic bool                       `json:"evaluation_deterministic"`
	OracleIsolation         cycle4OracleIsolation      `json:"oracle_isolation"`
	Terminal                *cycle4TerminalState       `json:"terminal,omitempty"`
	Verdict                 EvaluationVerdict          `json:"verdict"`
	SHA256                  string                     `json:"sha256"`
}

type cycle4CandidateBuild struct {
	run          blindCandidateRun
	authorityRaw []byte
	h0Raw        []byte
	h1Raw        []byte
	loads        int
	collision    cycle4CollisionObservation
}

type blindPreregistration struct {
	Version                  int                    `json:"version"`
	Status                   string                 `json:"status"`
	AuthorBriefSHA256        string                 `json:"author_brief_sha256"`
	BlindOracleSchemaVersion int                    `json:"blind_oracle_schema_version"`
	FrozenExtractor          blindFrozenExtractor   `json:"frozen_extractor"`
	CandidateInputs          []string               `json:"candidate_inputs"`
	CandidateForbiddenInputs []string               `json:"candidate_forbidden_inputs"`
	InstanceMatch            blindInstanceMatchRule `json:"instance_match"`
	Thresholds               blindThresholdContract `json:"thresholds"`
	OverallRule              string                 `json:"overall_rule"`
	PassInterpretation       string                 `json:"pass_interpretation"`
	FailureInterpretation    string                 `json:"failure_interpretation"`
	UserUtility              string                 `json:"user_utility"`
	ProductionReadiness      string                 `json:"production_readiness"`
	FrequencyReducer         string                 `json:"frequency_reducer"`
}

type blindFrozenExtractor struct {
	H1SHA256      string `json:"h1_sha256"`
	ReceiptSHA256 string `json:"receipt_sha256"`
}

type blindInstanceMatchRule struct {
	Key                string   `json:"key"`
	ForbiddenFallbacks []string `json:"forbidden_fallbacks"`
}

type blindThresholdContract struct {
	TruthInstances                int    `json:"truth_instances"`
	InstancePrecision             string `json:"instance_precision"`
	InstanceRecall                string `json:"instance_recall"`
	CriticalFalsePositives        int    `json:"critical_false_positives"`
	RolePrecision                 string `json:"role_precision"`
	RoleRecall                    string `json:"role_recall"`
	TaskBehaviourCoverage         string `json:"task_behaviour_coverage"`
	MatchedCompletenessAndMissing string `json:"matched_completeness_and_missing_sets"`
	EvidenceGroundingPrecision    string `json:"evidence_grounding_precision"`
	OracleAnchorRecall            string `json:"oracle_anchor_recall"`
	DecoyAdmission                int    `json:"decoy_admission"`
	ExclusionReasonPrecision      string `json:"exclusion_reason_precision"`
	ExclusionRecall               string `json:"exclusion_recall"`
	CallbackAccounting            string `json:"callback_accounting"`
	CandidateAndEvaluation        string `json:"candidate_and_evaluation"`
	LedgerAndSourceAccounting     string `json:"ledger_and_source_accounting"`
}

type blindFixtureReceipt struct {
	Version                int                    `json:"version"`
	Status                 string                 `json:"status"`
	AuthorBriefSHA256      string                 `json:"author_brief_sha256"`
	PreregistrationSHA256  string                 `json:"preregistration_sha256"`
	OracleSchemaVersion    int                    `json:"oracle_schema_version"`
	OracleSchemaSHA256     string                 `json:"oracle_schema_sha256"`
	RepositoryFiles        []blindReceiptFile     `json:"repository_files"`
	DependenciesFiles      []blindReceiptFile     `json:"dependencies_files"`
	RepositoryTreeSHA256   string                 `json:"repository_tree_sha256"`
	DependenciesTreeSHA256 string                 `json:"dependencies_tree_sha256"`
	OracleSHA256           string                 `json:"oracle_sha256"`
	Chronology             blindReceiptChronology `json:"chronology"`
	SHA256                 string                 `json:"sha256"`
}

type blindReceiptFile struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type blindReceiptChronology struct {
	FixtureValidatedBeforeSeal  bool `json:"fixture_validated_before_seal"`
	OracleSealedBeforeCandidate bool `json:"oracle_sealed_before_candidate"`
}

type blindOracle struct {
	Version             int                    `json:"version"`
	AuthorBriefSHA256   string                 `json:"author_brief_sha256"`
	SealedFixtureCommit string                 `json:"sealed_fixture_commit"`
	Instances           []blindOracleInstance  `json:"boundaries"`
	Excluded            []blindOracleExclusion `json:"exclusions"`
	Callbacks           []blindOracleCallback  `json:"callbacks"`
}

type blindOracleInstance struct {
	ID             string            `json:"id"`
	Label          string            `json:"label"`
	ExternalSystem string            `json:"external_system"`
	WrapperAnchor  blindLocator      `json:"local_boundary_anchor"`
	TaskBehaviours []blindLocator    `json:"production_entry_anchors"`
	Complete       bool              `json:"complete"`
	Roles          []blindOracleRole `json:"roles"`
	Missing        []H1Role          `json:"missing_task_roles"`
}

type blindOracleRole struct {
	Role     H1Role         `json:"role"`
	Evidence []blindLocator `json:"anchors"`
}

type blindOracleExclusion struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"`
	Reason H1ExclusionReason `json:"reason"`
	Anchor blindLocator      `json:"anchor"`
}

type blindOracleCallback struct {
	ID           string       `json:"id"`
	PassAnchor   blindLocator `json:"pass_anchor"`
	TargetAnchor blindLocator `json:"target_anchor"`
}

type blindOracleCallbackCounts struct {
	Observed int `json:"observed"`
	Closed   int `json:"closed"`
	Frontier int `json:"frontier"`
}

type blindLocator struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

type blindCandidateView struct {
	Instances        []blindCandidateInstance
	Excluded         []blindCandidateExclusion
	Callbacks        H1CallbackSummary
	ReachableAnchors []blindLocator
	Ledger           H1Ledger
}

type blindCandidateInstance struct {
	ID               string
	Complete         bool
	VerificationKind string
	Roles            []blindCandidateRole
	Missing          []H1Role
}

type blindCandidateRole struct {
	Role     H1Role
	Evidence []blindLocator
}

type blindCandidateExclusion struct {
	ID       string
	Reason   H1ExclusionReason
	Evidence []blindLocator
}

type blindMatch struct {
	OracleID    string `json:"oracle_id"`
	CandidateID string `json:"candidate_id"`
}

type blindSetMetric struct {
	Truth         int      `json:"truth"`
	Predicted     int      `json:"predicted"`
	Matched       int      `json:"matched"`
	TruePositive  []string `json:"true_positive"`
	FalsePositive []string `json:"false_positive"`
	FalseNegative []string `json:"false_negative"`
}

type blindExactMetric struct {
	Correct    int      `json:"correct"`
	Total      int      `json:"total"`
	Mismatches []string `json:"mismatches"`
}

type blindGroundingMetric struct {
	Emitted    int      `json:"emitted"`
	Grounded   int      `json:"grounded"`
	Ungrounded []string `json:"ungrounded"`
}

type blindCallbackMetric struct {
	Expected blindOracleCallbackCounts `json:"expected"`
	Actual   blindOracleCallbackCounts `json:"actual"`
	Exact    bool                      `json:"exact"`
}

type blindLedgerMetric struct {
	Expected H1Ledger `json:"expected"`
	Actual   H1Ledger `json:"actual"`
	Exact    bool     `json:"exact"`
}

type blindGateResult struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
}

type blindScorecard struct {
	Version                 int                  `json:"version"`
	FrozenH1SHA256          string               `json:"frozen_h1_sha256"`
	FrozenReceiptSHA256     string               `json:"frozen_receipt_sha256"`
	AuthoritySHA256         string               `json:"authority_sha256"`
	H0SHA256                string               `json:"h0_sha256"`
	CandidateH1SHA256       string               `json:"candidate_h1_sha256"`
	InstanceMatches         []blindMatch         `json:"instance_matches"`
	ExclusionMatches        []blindMatch         `json:"exclusion_matches"`
	Instances               blindSetMetric       `json:"instances"`
	CriticalFalsePositives  int                  `json:"critical_false_positives"`
	Roles                   blindSetMetric       `json:"roles"`
	TaskBehaviours          blindSetMetric       `json:"task_behaviours"`
	Completeness            blindExactMetric     `json:"completeness"`
	MissingSets             blindExactMetric     `json:"missing_sets"`
	EvidenceGrounding       blindGroundingMetric `json:"evidence_grounding"`
	OracleAnchors           blindSetMetric       `json:"oracle_anchors"`
	DecoyAdmissions         []string             `json:"decoy_admissions"`
	Exclusions              blindSetMetric       `json:"exclusions"`
	ExclusionReasons        blindExactMetric     `json:"exclusion_reasons"`
	Callbacks               blindCallbackMetric  `json:"callbacks"`
	Ledger                  blindLedgerMetric    `json:"ledger"`
	SourceAccountingExact   bool                 `json:"source_accounting_exact"`
	CandidateDeterministic  bool                 `json:"candidate_deterministic"`
	EvaluationDeterministic bool                 `json:"evaluation_deterministic"`
	FrequencyReducer        string               `json:"frequency_reducer"`
	Gates                   []blindGateResult    `json:"gates"`
	Verdict                 EvaluationVerdict    `json:"verdict"`
}

type blindCandidateRun struct {
	authority Authority
	h0        H0Result
	h1        H1Result
	h1Raw     []byte
}

type blindCandidateTerminal struct {
	Stage  string
	Reason string
	Err    error
}

func (value *blindCandidateTerminal) Error() string { return value.Err.Error() }

type blindTerminalScorecard struct {
	Version                  int               `json:"version"`
	Status                   string            `json:"status"`
	Stage                    string            `json:"stage"`
	Reason                   string            `json:"reason"`
	FrozenH1SHA256           string            `json:"frozen_h1_sha256"`
	FrozenReceiptSHA256      string            `json:"frozen_receipt_sha256"`
	FixtureReceiptSHA256     string            `json:"fixture_receipt_sha256"`
	CandidateOracleIsolation bool              `json:"candidate_oracle_isolation"`
	AuthorityArtifact        bool              `json:"authority_artifact"`
	H0Artifact               bool              `json:"h0_artifact"`
	H1Artifact               bool              `json:"h1_artifact"`
	MetricsStatus            string            `json:"metrics_status"`
	GeneralizationStatus     string            `json:"generalization_status"`
	UserUtilityStatus        string            `json:"user_utility_status"`
	ProductionReadiness      string            `json:"production_readiness"`
	FrequencyReducer         string            `json:"frequency_reducer"`
	Verdict                  EvaluationVerdict `json:"verdict"`
	SHA256                   string            `json:"sha256"`
}

func TestBlindRepositoryExtractionHistoricalSnapshot(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join(experimentRoot(t), "..", "..", ".."))
	root := filepath.Join(repositoryRoot, "testdata", "experiments", "clientrecipe-blind")
	repoRoot := filepath.Join(root, "repo")
	oraclePath := filepath.Join(root, "oracle.json")
	receiptPath := filepath.Join(root, "fixture_receipt.json")

	missing, err := blindFixtureMissing(repoRoot, oraclePath, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Skipf("blind fixture is incomplete; waiting for sealed authority: %s", strings.Join(missing, ", "))
	}

	briefRaw := readExperimentFile(t, filepath.Join(root, "author_brief.md"))
	preregRaw := readExperimentFile(t, filepath.Join(root, "preregistration.json"))
	schemaRaw := readExperimentFile(t, filepath.Join(root, "oracle.schema.json"))
	prereg, err := decodeBlindPreregistration(preregRaw, briefRaw)
	if err != nil {
		t.Fatal(err)
	}
	oracleRaw := readExperimentFile(t, oraclePath)
	receiptRaw := readExperimentFile(t, receiptPath)
	receipt, err := decodeBlindFixtureReceipt(receiptRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Validate(root, briefRaw, preregRaw, schemaRaw, oracleRaw); err != nil {
		t.Fatal(err)
	}

	freeze := loadReducerFreeze(t, repositoryRoot)
	if prereg.FrozenExtractor != (blindFrozenExtractor{
		H1SHA256: freeze.BaselineH1SHA256, ReceiptSHA256: freeze.SHA256,
	}) {
		t.Fatal("blind gate: preregistered extractor no longer matches the validated freeze receipt")
	}

	oracle, err := decodeBlindOracle(oracleRaw)
	if err != nil {
		t.Fatal(err)
	}
	if oracle.AuthorBriefSHA256 != prereg.AuthorBriefSHA256 {
		t.Fatal("blind oracle: author brief binding mismatch")
	}
	if err := validateBlindOracleLocators(repoRoot, oracle); err != nil {
		t.Fatal(err)
	}

	terminalRaw := readExperimentFile(t, filepath.Join(root, "golden", "04-evaluation.json"))
	if blindBytesSHA256(terminalRaw) != historicalBlindTerminalRawSHA256 {
		t.Fatal("blind historical gate: terminal raw bytes changed")
	}
	var terminal blindTerminalScorecard
	if err := decodeStrict(terminalRaw, &terminal, "blind historical terminal scorecard"); err != nil {
		t.Fatal(err)
	}
	canonical, err := encodeBlindTerminalScorecard(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, terminalRaw) || terminal.SHA256 != historicalBlindTerminalInternalSHA256 ||
		terminal.FrozenH1SHA256 != freeze.BaselineH1SHA256 || terminal.FrozenReceiptSHA256 != freeze.SHA256 ||
		terminal.FixtureReceiptSHA256 != receipt.SHA256 || terminal.Stage != "authority_preparation" ||
		terminal.Reason != "duplicate_program_relation_identity" || terminal.Verdict != EvaluationFail {
		t.Fatal("blind historical gate: terminal snapshot identity chain changed")
	}
	t.Log("blind Cycle 3 terminal remains an immutable historical FAIL snapshot")
}

func TestBlindRepositoryExtractionGateCycle4(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join(experimentRoot(t), "..", "..", ".."))
	root := filepath.Join(repositoryRoot, "testdata", "experiments", "clientrecipe-blind")
	repoRoot := filepath.Join(root, "repo")
	briefRaw := readExperimentFile(t, filepath.Join(root, "author_brief.md"))
	preregRaw := readExperimentFile(t, filepath.Join(root, "preregistration.json"))
	schemaRaw := readExperimentFile(t, filepath.Join(root, "oracle.schema.json"))
	oracleRaw := readExperimentFile(t, filepath.Join(root, "oracle.json"))
	fixtureReceiptRaw := readExperimentFile(t, filepath.Join(root, "fixture_receipt.json"))
	prereg, err := decodeBlindPreregistration(preregRaw, briefRaw)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := decodeBlindFixtureReceipt(fixtureReceiptRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Validate(root, briefRaw, preregRaw, schemaRaw, oracleRaw); err != nil {
		t.Fatal(err)
	}
	oracle, err := decodeBlindOracle(oracleRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateBlindOracleLocators(repoRoot, oracle); err != nil {
		t.Fatal(err)
	}

	assertCycle4ControlledCompatibility(t, repositoryRoot)
	freeze := buildCycle4ExtractorFreeze(t, repositoryRoot, root, prereg, receipt,
		briefRaw, preregRaw, schemaRaw, oracleRaw, fixtureReceiptRaw)
	freezeRaw, err := encodeCycle4ExtractorFreeze(freeze)
	if err != nil {
		t.Fatal(err)
	}
	if err := freeze.Validate(repositoryRoot, root, prereg, receipt); err != nil {
		t.Fatal(err)
	}
	// The candidate identity is sealed before either blind candidate build.
	assertBlindCycle4Golden(t, root, "cycle4/00-extractor-freeze.json", freezeRaw)

	first, firstErr := runBlindCandidateCycle4(t, repoRoot)
	second, secondErr := runBlindCandidateCycle4(t, repoRoot)
	if firstErr != nil || secondErr != nil {
		freezeCycle4TerminalResult(t, root, freeze, prereg, receipt, first, second, firstErr, secondErr)
		return
	}
	candidateDeterministic := bytes.Equal(first.authorityRaw, second.authorityRaw) &&
		bytes.Equal(first.h0Raw, second.h0Raw) && bytes.Equal(first.h1Raw, second.h1Raw)
	if first.loads != 1 || second.loads != 1 {
		t.Fatalf("Cycle 4 production loads per build = [%d,%d], want [1,1]", first.loads, second.loads)
	}
	if !reflect.DeepEqual(first.collision, second.collision) {
		t.Fatalf("Cycle 4 collision ledger changed between builds: %#v / %#v", first.collision, second.collision)
	}
	if err := validateCycle4CollisionObservation(first.collision, freeze.Collision); err != nil {
		t.Fatal(err)
	}
	if err := receipt.Validate(root, briefRaw, preregRaw, schemaRaw, oracleRaw); err != nil {
		t.Fatalf("Cycle 4 sealed fixture changed during candidate builds: %v", err)
	}

	firstCandidate := blindCandidateFromH1(first.run.h1, first.run.authority)
	secondCandidate := blindCandidateFromH1(second.run.h1, second.run.authority)
	firstSourceExact := blindAuthorityMatchesReceipt(first.run.authority, receipt.RepositoryFiles)
	secondSourceExact := blindAuthorityMatchesReceipt(second.run.authority, receipt.RepositoryFiles)
	firstEvaluation := evaluateBlindCandidate(prereg, oracle, firstCandidate, first.run,
		firstSourceExact, candidateDeterministic, true)
	secondEvaluation := evaluateBlindCandidate(prereg, oracle, secondCandidate, second.run,
		secondSourceExact, candidateDeterministic, true)
	firstEvaluationRaw, err := encodeBlindScorecard(firstEvaluation)
	if err != nil {
		t.Fatal(err)
	}
	secondEvaluationRaw, err := encodeBlindScorecard(secondEvaluation)
	if err != nil {
		t.Fatal(err)
	}
	evaluationDeterministic := bytes.Equal(firstEvaluationRaw, secondEvaluationRaw)
	if !evaluationDeterministic {
		firstEvaluation = evaluateBlindCandidate(prereg, oracle, firstCandidate, first.run,
			firstSourceExact, candidateDeterministic, false)
		secondEvaluation = evaluateBlindCandidate(prereg, oracle, secondCandidate, second.run,
			secondSourceExact, candidateDeterministic, false)
		firstEvaluationRaw, err = encodeBlindScorecard(firstEvaluation)
		if err != nil {
			t.Fatal(err)
		}
		secondEvaluationRaw, err = encodeBlindScorecard(secondEvaluation)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := firstEvaluation.Validate(prereg); err != nil {
		t.Fatal(err)
	}
	if err := secondEvaluation.Validate(prereg); err != nil {
		t.Fatal(err)
	}

	candidateBundleBeforeOracleMutation := blindBytesSHA256(bytes.Join(
		[][]byte{first.authorityRaw, first.h0Raw, first.h1Raw}, []byte{0},
	))
	mutatedOracle := mutateBlindOracleForIsolation(t, oracle)
	mutatedEvaluation := evaluateBlindCandidate(prereg, mutatedOracle, firstCandidate, first.run,
		firstSourceExact, candidateDeterministic, evaluationDeterministic)
	mutatedEvaluationRaw, err := encodeBlindScorecard(mutatedEvaluation)
	if err != nil {
		t.Fatal(err)
	}
	candidateBundleAfterOracleMutation := blindBytesSHA256(bytes.Join(
		[][]byte{first.authorityRaw, first.h0Raw, first.h1Raw}, []byte{0},
	))
	oracleIsolation := cycle4OracleIsolation{
		Status:                  "PASS",
		CandidateBytesUnchanged: candidateBundleBeforeOracleMutation == candidateBundleAfterOracleMutation,
		EvaluationBytesChanged:  !bytes.Equal(firstEvaluationRaw, mutatedEvaluationRaw),
	}
	if !oracleIsolation.CandidateBytesUnchanged || !oracleIsolation.EvaluationBytesChanged {
		t.Fatalf("Cycle 4 oracle isolation failed: %#v", oracleIsolation)
	}

	assertBlindCycle4Golden(t, root, "cycle4/01-authority.json", first.authorityRaw)
	assertBlindCycle4Golden(t, root, "cycle4/02-h0.json", first.h0Raw)
	assertBlindCycle4Golden(t, root, "cycle4/03-h1.json", first.h1Raw)
	assertBlindCycle4Golden(t, root, "cycle4/04-evaluation.json", firstEvaluationRaw)

	runReceipt := buildCycle4RunReceipt(freeze, first, firstEvaluation, firstEvaluationRaw,
		candidateDeterministic, evaluationDeterministic, oracleIsolation)
	runReceiptRaw, err := encodeCycle4RunReceipt(runReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := runReceipt.Validate(freeze, firstEvaluation, firstEvaluationRaw); err != nil {
		t.Fatal(err)
	}
	assertBlindCycle4Golden(t, root, "cycle4/05-receipt.json", runReceiptRaw)
	t.Logf("Cycle 4 blind verdict: %s; instances %d/%d, roles %d/%d, exclusions %d/%d",
		firstEvaluation.Verdict, firstEvaluation.Instances.Matched, firstEvaluation.Instances.Truth,
		firstEvaluation.Roles.Matched, firstEvaluation.Roles.Truth,
		firstEvaluation.Exclusions.Matched, firstEvaluation.Exclusions.Truth)
}

func freezeCycle4TerminalResult(
	t *testing.T,
	root string,
	freeze cycle4ExtractorFreeze,
	prereg blindPreregistration,
	fixtureReceipt blindFixtureReceipt,
	first, second cycle4CandidateBuild,
	firstErr, secondErr error,
) {
	t.Helper()
	firstTerminal, firstOK := firstErr.(*blindCandidateTerminal)
	secondTerminal, secondOK := secondErr.(*blindCandidateTerminal)
	if !firstOK || !secondOK || firstTerminal.Stage != secondTerminal.Stage || firstTerminal.Reason != secondTerminal.Reason {
		t.Fatalf("Cycle 4 terminal outcome is not deterministic: %v / %v", firstErr, secondErr)
	}
	if first.loads != 1 || second.loads != 1 {
		t.Fatalf("Cycle 4 terminal production loads per build = [%d,%d], want [1,1]", first.loads, second.loads)
	}
	if !reflect.DeepEqual(first.collision, second.collision) {
		t.Fatalf("Cycle 4 terminal collision observations differ: %#v / %#v", first.collision, second.collision)
	}
	if err := validateCycle4CollisionObservation(first.collision, freeze.Collision); err != nil {
		t.Fatal(err)
	}
	candidateDeterministic := bytes.Equal(first.authorityRaw, second.authorityRaw) &&
		bytes.Equal(first.h0Raw, second.h0Raw) && bytes.Equal(first.h1Raw, second.h1Raw)
	if !candidateDeterministic {
		t.Fatal("Cycle 4 partial candidate bytes differ across terminal builds")
	}
	if len(first.authorityRaw) == 0 || len(first.h0Raw) == 0 || len(first.h1Raw) != 0 {
		t.Fatalf("Cycle 4 terminal artifact boundary = authority:%t h0:%t h1:%t, want true/true/false",
			len(first.authorityRaw) != 0, len(first.h0Raw) != 0, len(first.h1Raw) != 0)
	}
	assertBlindCycle4Golden(t, root, "cycle4/01-authority.json", first.authorityRaw)
	assertBlindCycle4Golden(t, root, "cycle4/02-h0.json", first.h0Raw)
	assertCycle4GoldenAbsent(t, root, "03-h1.json")

	firstFailure := buildBlindTerminalScorecard(prereg, fixtureReceipt, first.run, firstTerminal)
	secondFailure := buildBlindTerminalScorecard(prereg, fixtureReceipt, second.run, secondTerminal)
	firstFailureRaw, err := encodeBlindTerminalScorecard(firstFailure)
	if err != nil {
		t.Fatal(err)
	}
	secondFailureRaw, err := encodeBlindTerminalScorecard(secondFailure)
	if err != nil {
		t.Fatal(err)
	}
	evaluationDeterministic := bytes.Equal(firstFailureRaw, secondFailureRaw)
	if !evaluationDeterministic {
		t.Fatal("Cycle 4 terminal evaluation bytes differ across identical builds")
	}
	assertBlindCycle4Golden(t, root, "cycle4/04-evaluation.json", firstFailureRaw)

	runReceipt := buildCycle4TerminalRunReceipt(
		freeze, first, firstFailure, firstFailureRaw, candidateDeterministic, evaluationDeterministic,
	)
	runReceiptRaw, err := encodeCycle4RunReceipt(runReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := runReceipt.ValidateTerminal(freeze, first, firstFailure, firstFailureRaw); err != nil {
		t.Fatal(err)
	}
	assertBlindCycle4Golden(t, root, "cycle4/05-receipt.json", runReceiptRaw)
	t.Logf("Cycle 4 blind verdict: FAIL (terminal %s/%s); Authority and H0 retained, H1 unavailable",
		firstTerminal.Stage, firstTerminal.Reason)
}

func assertCycle4GoldenAbsent(t *testing.T, root, name string) {
	t.Helper()
	filename := filepath.Join(root, "golden", "cycle4", name)
	if _, err := os.Stat(filename); err == nil {
		t.Fatalf("Cycle 4 terminal must not invent %s", name)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Cycle 4 inspect absent %s: %v", name, err)
	}
}

func buildCycle4ExtractorFreeze(
	t *testing.T,
	repositoryRoot, root string,
	prereg blindPreregistration,
	receipt blindFixtureReceipt,
	briefRaw, preregRaw, schemaRaw, oracleRaw, fixtureReceiptRaw []byte,
) cycle4ExtractorFreeze {
	t.Helper()
	candidateSources, err := discoverH1FreezeSources(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	evaluatorInputs, err := discoverCycle4EvaluatorInputs(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	controlled, err := cycle4ControlledArtifactBindings(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	terminalRaw := readExperimentFile(t, filepath.Join(root, "golden", "04-evaluation.json"))
	var terminal blindTerminalScorecard
	if err := decodeStrict(terminalRaw, &terminal, "Cycle 3 terminal scorecard"); err != nil {
		t.Fatal(err)
	}
	result := cycle4ExtractorFreeze{
		Version: cycle4FreezeVersion, Status: "SEALED_BEFORE_CANDIDATE", CandidateID: cycle4CandidateID,
		CandidateSourcesSHA256: h1FreezeSourcesDigest(candidateSources), CandidateSources: candidateSources,
		Rules: h1FreezeRules(), ControlledArtifacts: controlled,
		BlindSnapshot: cycle4BlindSnapshot{
			AuthorBriefRawSHA256: blindBytesSHA256(briefRaw), PreregistrationRawSHA256: blindBytesSHA256(preregRaw),
			OracleSchemaRawSHA256: blindBytesSHA256(schemaRaw), OracleRawSHA256: blindBytesSHA256(oracleRaw),
			FixtureReceiptRawSHA256: blindBytesSHA256(fixtureReceiptRaw), FixtureReceiptSHA256: receipt.SHA256,
			RepositoryTreeSHA256: receipt.RepositoryTreeSHA256, DependenciesTreeSHA256: receipt.DependenciesTreeSHA256,
		},
		HistoricalTerminal: cycle4HistoricalTerminal{
			Path:      "testdata/experiments/clientrecipe-blind/golden/04-evaluation.json",
			RawSHA256: blindBytesSHA256(terminalRaw), InternalSHA256: terminal.SHA256,
			Stage: terminal.Stage, Reason: terminal.Reason,
		},
		Collision: cycle4CollisionContract{
			Classification: "distinct_semantic_observations_collapsed_by_source_ref", ExpectedGroups: 3,
			Sites: []blindLocator{
				{Path: "internal/httpapi/server.go", Line: 15},
				{Path: "internal/httpapi/server.go", Line: 24},
				{Path: "internal/httpapi/server.go", Line: 25},
			},
			IdentityRule: "legacy_singletons_unchanged_collision_groups_disambiguated_by_complete_structural_tuple",
		},
		Evaluator: cycle4EvaluatorContract{
			InputsSHA256: h1FreezeSourcesDigest(evaluatorInputs), Inputs: evaluatorInputs,
			Thresholds: prereg.Thresholds, OverallRule: prereg.OverallRule,
		},
	}
	result.SHA256 = cycle4ExtractorFreezeDigest(result)
	return result
}

func discoverCycle4EvaluatorInputs(repositoryRoot string) ([]h1FreezeSource, error) {
	paths := []string{
		"internal/experiments/clientrecipe/authority_test.go",
		"internal/experiments/clientrecipe/blind_fixture_test.go",
		"internal/experiments/clientrecipe/contract_test.go",
		"internal/experiments/clientrecipe/leave_one_out_test.go",
		"internal/experiments/clientrecipe/mutants_test.go",
		"testdata/experiments/clientrecipe-blind/author_brief.md",
		"testdata/experiments/clientrecipe-blind/fixture_receipt.json",
		"testdata/experiments/clientrecipe-blind/oracle.json",
		"testdata/experiments/clientrecipe-blind/oracle.schema.json",
		"testdata/experiments/clientrecipe-blind/preregistration.json",
	}
	sort.Strings(paths)
	result := make([]h1FreezeSource, 0, len(paths))
	for _, relative := range paths {
		raw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relative)))
		if err != nil {
			return nil, fmt.Errorf("Cycle 4 freeze: read evaluator input %s: %w", relative, err)
		}
		result = append(result, h1FreezeSource{Path: relative, SHA256: blindBytesSHA256(raw)})
	}
	return result, nil
}

func cycle4ControlledArtifactBindings(repositoryRoot string) ([]cycle4ArtifactBinding, error) {
	type artifact struct {
		stage, relative string
		decode          func([]byte) (string, error)
	}
	artifacts := []artifact{
		{stage: "authority", relative: "testdata/experiments/clientrecipe/golden/01-input-authority.json", decode: func(raw []byte) (string, error) {
			value, err := DecodeAuthority(raw)
			return value.SHA256, err
		}},
		{stage: "h0", relative: "testdata/experiments/clientrecipe/golden/02-h0-candidates.json", decode: func(raw []byte) (string, error) {
			value, err := DecodeH0(raw)
			return value.SHA256, err
		}},
		{stage: "h1", relative: "testdata/experiments/clientrecipe/golden/03-h1-structural.json", decode: func(raw []byte) (string, error) {
			value, err := DecodeH1(raw)
			return value.SHA256, err
		}},
	}
	result := make([]cycle4ArtifactBinding, 0, len(artifacts))
	for _, artifact := range artifacts {
		raw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(artifact.relative)))
		if err != nil {
			return nil, fmt.Errorf("Cycle 4 freeze: read controlled %s: %w", artifact.stage, err)
		}
		internal, err := artifact.decode(raw)
		if err != nil {
			return nil, fmt.Errorf("Cycle 4 freeze: decode controlled %s: %w", artifact.stage, err)
		}
		result = append(result, cycle4ArtifactBinding{
			Stage: artifact.stage, Path: artifact.relative,
			RawSHA256: blindBytesSHA256(raw), InternalSHA256: internal,
		})
	}
	return result, nil
}

func encodeCycle4ExtractorFreeze(value cycle4ExtractorFreeze) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("Cycle 4 freeze: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func cycle4ExtractorFreezeDigest(value cycle4ExtractorFreeze) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	return blindBytesSHA256(raw)
}

func (value cycle4ExtractorFreeze) Validate(
	repositoryRoot, root string,
	prereg blindPreregistration,
	receipt blindFixtureReceipt,
) error {
	if value.Version != cycle4FreezeVersion || value.Status != "SEALED_BEFORE_CANDIDATE" ||
		value.CandidateID != cycle4CandidateID || len(value.CandidateSources) == 0 || len(value.Rules) == 0 ||
		len(value.ControlledArtifacts) != 3 || !validSHA256(value.CandidateSourcesSHA256) || !validSHA256(value.SHA256) {
		return fmt.Errorf("Cycle 4 freeze: invalid identity")
	}
	wantSources, err := discoverH1FreezeSources(repositoryRoot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(value.CandidateSources, wantSources) ||
		value.CandidateSourcesSHA256 != h1FreezeSourcesDigest(value.CandidateSources) {
		return fmt.Errorf("Cycle 4 freeze: current candidate source surface changed")
	}
	if !reflect.DeepEqual(value.Rules, h1FreezeRules()) {
		return fmt.Errorf("Cycle 4 freeze: extractor rules changed")
	}
	wantControlled, err := cycle4ControlledArtifactBindings(repositoryRoot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(value.ControlledArtifacts, wantControlled) {
		return fmt.Errorf("Cycle 4 freeze: controlled artifact bindings changed")
	}
	wantBlind := cycle4BlindSnapshot{
		AuthorBriefRawSHA256:     "2ac6286b63b3ab2ed8017de9052597bfeb87a771fc221b9823f1fd8322ff1e81",
		PreregistrationRawSHA256: "1d7e51ffde62140742d7f04aa35dbfec3c5ac974aa75dc2088194c2850195b70",
		OracleSchemaRawSHA256:    "409de2cae576e9c2e759fa3a6fea811adbb3ecefba0c0bf761c0e59414ce24ea",
		OracleRawSHA256:          "ad1ec48672b31ccd7419fa352836ea2f73eba6141e2e54cf5a1dd8041f569023",
		FixtureReceiptRawSHA256:  historicalBlindFixtureReceiptRawSHA256,
		FixtureReceiptSHA256:     receipt.SHA256, RepositoryTreeSHA256: receipt.RepositoryTreeSHA256,
		DependenciesTreeSHA256: receipt.DependenciesTreeSHA256,
	}
	if !reflect.DeepEqual(value.BlindSnapshot, wantBlind) {
		return fmt.Errorf("Cycle 4 freeze: sealed blind snapshot changed")
	}
	wantTerminalRaw, err := os.ReadFile(filepath.Join(root, "golden", "04-evaluation.json"))
	if err != nil {
		return fmt.Errorf("Cycle 4 freeze: read historical terminal: %w", err)
	}
	wantTerminal := cycle4HistoricalTerminal{
		Path:      "testdata/experiments/clientrecipe-blind/golden/04-evaluation.json",
		RawSHA256: historicalBlindTerminalRawSHA256, InternalSHA256: historicalBlindTerminalInternalSHA256,
		Stage: "authority_preparation", Reason: "duplicate_program_relation_identity",
	}
	if blindBytesSHA256(wantTerminalRaw) != historicalBlindTerminalRawSHA256 ||
		!reflect.DeepEqual(value.HistoricalTerminal, wantTerminal) {
		return fmt.Errorf("Cycle 4 freeze: historical terminal changed")
	}
	wantCollision := cycle4CollisionContract{
		Classification: "distinct_semantic_observations_collapsed_by_source_ref", ExpectedGroups: 3,
		Sites: []blindLocator{
			{Path: "internal/httpapi/server.go", Line: 15},
			{Path: "internal/httpapi/server.go", Line: 24},
			{Path: "internal/httpapi/server.go", Line: 25},
		},
		IdentityRule: "legacy_singletons_unchanged_collision_groups_disambiguated_by_complete_structural_tuple",
	}
	if !reflect.DeepEqual(value.Collision, wantCollision) {
		return fmt.Errorf("Cycle 4 freeze: collision contract changed")
	}
	wantEvaluatorInputs, err := discoverCycle4EvaluatorInputs(repositoryRoot)
	if err != nil {
		return err
	}
	if value.Evaluator.InputsSHA256 != h1FreezeSourcesDigest(value.Evaluator.Inputs) ||
		!reflect.DeepEqual(value.Evaluator.Inputs, wantEvaluatorInputs) ||
		!reflect.DeepEqual(value.Evaluator.Thresholds, prereg.Thresholds) || value.Evaluator.OverallRule != prereg.OverallRule {
		return fmt.Errorf("Cycle 4 freeze: evaluator source or preregistered thresholds changed")
	}
	if value.SHA256 != cycle4ExtractorFreezeDigest(value) {
		return fmt.Errorf("Cycle 4 freeze: digest mismatch")
	}
	return nil
}

func assertCycle4ControlledCompatibility(t *testing.T, repositoryRoot string) {
	t.Helper()
	repoRoot := filepath.Join(repositoryRoot, "testdata", "experiments", "clientrecipe", "repo")
	loader := &countingProductionLoader{delegate: defaultProductionPackageLoader{}}
	authority, err := prepareAuthority(t.Context(), repoRoot, loader)
	if err != nil {
		t.Fatal(err)
	}
	h0, err := BuildH0(authority)
	if err != nil {
		t.Fatal(err)
	}
	h1, err := ExtractH1(repoRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	if loader.count != 1 {
		t.Fatalf("Cycle 4 controlled compatibility package loads = %d, want 1", loader.count)
	}
	actual := make([][]byte, 3)
	actual[0], err = EncodeAuthority(authority)
	if err != nil {
		t.Fatal(err)
	}
	actual[1], err = EncodeH0(h0)
	if err != nil {
		t.Fatal(err)
	}
	actual[2], err = EncodeH1(h1)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"golden/01-input-authority.json", "golden/02-h0-candidates.json", "golden/03-h1-structural.json",
	}
	for index, relative := range paths {
		want := readExperimentFile(t, filepath.Join(repositoryRoot, "testdata", "experiments", "clientrecipe", relative))
		if !bytes.Equal(actual[index], want) {
			t.Fatalf("Cycle 4 changed controlled %s bytes", relative)
		}
	}
}

func runBlindCandidateCycle4(t *testing.T, repoRoot string) (cycle4CandidateBuild, error) {
	t.Helper()
	var result cycle4CandidateBuild
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	sources, repositorySHA256, err := prepareSourceFacts(absoluteRoot)
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	loader := &countingProductionLoader{delegate: defaultProductionPackageLoader{}}
	loaded, err := loader.Load(t.Context(), absoluteRoot)
	result.loads = loader.count
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	packagesByPath, modulePath, err := productionPackages(loaded)
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	catalog, err := buildDependencyCatalog(absoluteRoot, modulePath, packagesByPath)
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	var collision cycle4CollisionObservation
	index, err := buildProgramIndexWithSealObserver(
		absoluteRoot, modulePath, repositorySHA256, packagesByPath, sources,
		func(before, after []programindex.RelationInput) error {
			var observerErr error
			collision, observerErr = observeCycle4RelationCollisions(before, after)
			return observerErr
		},
	)
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	result.collision = collision
	operations, err := externalOperationFacts(index, catalog, sources)
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	callbacks := callbackCoverage(index)
	coverage := AuthorityCoverage{FilesObserved: len(sources)}
	for _, source := range sources {
		switch source.Class {
		case SourceProduction:
			coverage.ProductionFiles++
		case SourceTest:
			coverage.TestFiles++
		case SourceGenerated:
			coverage.GeneratedFiles++
		case SourceProse:
			coverage.ProseFiles++
		case SourceManifest:
			coverage.ManifestFiles++
		case SourceOther:
			coverage.OtherFiles++
		}
	}
	coverage.DependencyUsesObserved = catalog.Coverage.ImportsObserved
	coverage.ExternalCallsObserved = len(operations)
	authority, err := sealAuthority(Authority{
		Version: AuthorityVersion, RepositorySHA256: repositorySHA256, Program: index,
		Dependencies: catalog, Sources: sources, ExternalOperations: operations, Callbacks: callbacks, Coverage: coverage,
	})
	if err != nil {
		return result, blindTerminal("authority_preparation", err)
	}
	result.run.authority = authority
	result.authorityRaw, err = EncodeAuthority(authority)
	if err != nil {
		return result, blindTerminal("authority_encoding", err)
	}
	h0, err := BuildH0(authority)
	if err != nil {
		return result, blindTerminal("h0_build", err)
	}
	result.run.h0 = h0
	result.h0Raw, err = EncodeH0(h0)
	if err != nil {
		return result, blindTerminal("h0_encoding", err)
	}
	h1, err := ExtractH1(absoluteRoot, authority)
	if err != nil {
		return result, blindTerminal("h1_extraction", err)
	}
	h1Raw, err := EncodeH1(h1)
	if err != nil {
		return result, blindTerminal("h1_encoding", err)
	}
	result.run.h1 = h1
	result.run.h1Raw = h1Raw
	result.h1Raw = h1Raw
	return result, nil
}

func observeCycle4RelationCollisions(before, after []programindex.RelationInput) (cycle4CollisionObservation, error) {
	type group struct {
		location blindLocator
		tuples   []string
	}
	groups := make(map[string]*group)
	for _, relation := range before {
		key := strings.Join([]string{relation.SourceRef, string(relation.Kind), relation.FromRef}, "\x00")
		entry := groups[key]
		if entry == nil {
			entry = &group{}
			if relation.Location != nil {
				entry.location = blindLocator{Path: relation.Location.Path, Line: relation.Location.Line}
			}
			groups[key] = entry
		}
		entry.tuples = append(entry.tuples, relationObservationTuple(relation))
	}
	sites := []blindLocator{}
	classification := "distinct_semantic_observations_collapsed_by_source_ref"
	collisions := 0
	for _, entry := range groups {
		if len(entry.tuples) < 2 {
			continue
		}
		collisions++
		distinct := make(map[string]struct{}, len(entry.tuples))
		for _, tuple := range entry.tuples {
			distinct[tuple] = struct{}{}
		}
		if len(distinct) != len(entry.tuples) {
			classification = "duplicate_or_mixed_relation_observations"
		}
		sites = append(sites, entry.location)
	}
	sites = canonicalizeBlindLocators(sites)
	if len(before) != len(after) {
		return cycle4CollisionObservation{}, fmt.Errorf("Cycle 4 collision fix dropped relation observations: %d -> %d", len(before), len(after))
	}
	post := make(map[string]int, len(after))
	for _, relation := range after {
		key := strings.Join([]string{relation.SourceRef, string(relation.Kind), relation.FromRef}, "\x00")
		post[key]++
		if post[key] > 1 {
			return cycle4CollisionObservation{}, fmt.Errorf("Cycle 4 collision fix retained duplicate relation identity")
		}
	}
	return cycle4CollisionObservation{
		Classification: classification, Groups: collisions, Sites: sites,
		RelationsBefore: len(before), RelationsAfter: len(after),
	}, nil
}

func validateCycle4CollisionObservation(value cycle4CollisionObservation, contract cycle4CollisionContract) error {
	if value.Classification != contract.Classification || value.Groups != contract.ExpectedGroups ||
		!reflect.DeepEqual(value.Sites, contract.Sites) || value.RelationsBefore <= 0 ||
		value.RelationsAfter != value.RelationsBefore {
		return fmt.Errorf("Cycle 4 collision observation does not satisfy the frozen contract: %#v", value)
	}
	return nil
}

func mutateBlindOracleForIsolation(t *testing.T, oracle blindOracle) blindOracle {
	t.Helper()
	raw, err := json.Marshal(oracle)
	if err != nil {
		t.Fatal(err)
	}
	var result blindOracle
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Excluded) == 0 {
		t.Fatal("Cycle 4 oracle isolation: no exclusion to mutate")
	}
	if result.Excluded[0].Reason == H1ExcludedTestOnly {
		result.Excluded[0].Reason = H1ExcludedNotProductionReachable
	} else {
		result.Excluded[0].Reason = H1ExcludedTestOnly
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func buildCycle4RunReceipt(
	freeze cycle4ExtractorFreeze,
	build cycle4CandidateBuild,
	evaluation blindScorecard,
	evaluationRaw []byte,
	candidateDeterministic, evaluationDeterministic bool,
	oracleIsolation cycle4OracleIsolation,
) cycle4RunReceipt {
	status := "FAIL"
	if evaluation.Verdict == EvaluationPass {
		status = "PASS"
	}
	result := cycle4RunReceipt{
		Version: cycle4ReceiptVersion, Status: status, CandidateID: cycle4CandidateID,
		ExtractorFreezeSHA256: freeze.SHA256,
		Artifacts: []cycle4RunArtifact{
			{Stage: "authority", RawSHA256: blindBytesSHA256(build.authorityRaw), InternalSHA256: build.run.authority.SHA256},
			{Stage: "h0", RawSHA256: blindBytesSHA256(build.h0Raw), InternalSHA256: build.run.h0.SHA256},
			{Stage: "h1", RawSHA256: blindBytesSHA256(build.h1Raw), InternalSHA256: build.run.h1.SHA256},
			{Stage: "evaluation", RawSHA256: blindBytesSHA256(evaluationRaw), InternalSHA256: blindScorecardSemanticSHA256(evaluation)},
		},
		UnavailableArtifacts: []string{},
		Builds:               2, ProductionLoadsPerBuild: []int{build.loads, build.loads}, Collision: build.collision,
		CandidateDeterministic: candidateDeterministic, EvaluationDeterministic: evaluationDeterministic,
		OracleIsolation: oracleIsolation, Verdict: evaluation.Verdict,
	}
	result.SHA256 = cycle4RunReceiptDigest(result)
	return result
}

func buildCycle4TerminalRunReceipt(
	freeze cycle4ExtractorFreeze,
	build cycle4CandidateBuild,
	failure blindTerminalScorecard,
	failureRaw []byte,
	candidateDeterministic, evaluationDeterministic bool,
) cycle4RunReceipt {
	result := cycle4RunReceipt{
		Version: cycle4ReceiptVersion, Status: "TERMINAL_ERROR", CandidateID: cycle4CandidateID,
		ExtractorFreezeSHA256: freeze.SHA256,
		Artifacts: []cycle4RunArtifact{
			{Stage: "authority", RawSHA256: blindBytesSHA256(build.authorityRaw), InternalSHA256: build.run.authority.SHA256},
			{Stage: "h0", RawSHA256: blindBytesSHA256(build.h0Raw), InternalSHA256: build.run.h0.SHA256},
			{Stage: "evaluation", RawSHA256: blindBytesSHA256(failureRaw), InternalSHA256: failure.SHA256},
		},
		UnavailableArtifacts: []string{"h1"},
		Builds:               2, ProductionLoadsPerBuild: []int{build.loads, build.loads}, Collision: build.collision,
		CandidateDeterministic: candidateDeterministic, EvaluationDeterministic: evaluationDeterministic,
		OracleIsolation: cycle4OracleIsolation{Status: "NOT_TESTED_TERMINAL_BEFORE_EVALUATION"},
		Terminal:        &cycle4TerminalState{Stage: failure.Stage, Reason: failure.Reason},
		Verdict:         EvaluationFail,
	}
	result.SHA256 = cycle4RunReceiptDigest(result)
	return result
}

func blindScorecardSemanticSHA256(value blindScorecard) string {
	raw, _ := json.Marshal(value)
	return blindBytesSHA256(raw)
}

func cycle4RunReceiptDigest(value cycle4RunReceipt) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	return blindBytesSHA256(raw)
}

func encodeCycle4RunReceipt(value cycle4RunReceipt) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("Cycle 4 receipt: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func (value cycle4RunReceipt) Validate(
	freeze cycle4ExtractorFreeze,
	evaluation blindScorecard,
	evaluationRaw []byte,
) error {
	wantStatus := "FAIL"
	if evaluation.Verdict == EvaluationPass {
		wantStatus = "PASS"
	}
	if value.Version != cycle4ReceiptVersion || value.Status != wantStatus || value.CandidateID != cycle4CandidateID ||
		value.ExtractorFreezeSHA256 != freeze.SHA256 || value.Builds != 2 ||
		!reflect.DeepEqual(value.ProductionLoadsPerBuild, []int{1, 1}) || len(value.Artifacts) != 4 ||
		len(value.UnavailableArtifacts) != 0 || value.Terminal != nil || value.Verdict != evaluation.Verdict ||
		!validSHA256(value.SHA256) {
		return fmt.Errorf("Cycle 4 receipt: invalid identity or run accounting")
	}
	wantInternal := map[string]string{
		"authority":  evaluation.AuthoritySHA256,
		"h0":         evaluation.H0SHA256,
		"h1":         evaluation.CandidateH1SHA256,
		"evaluation": blindScorecardSemanticSHA256(evaluation),
	}
	wantStages := []string{"authority", "h0", "h1", "evaluation"}
	for index, artifact := range value.Artifacts {
		if artifact.Stage != wantStages[index] ||
			!validSHA256(artifact.RawSHA256) || !validSHA256(artifact.InternalSHA256) ||
			artifact.InternalSHA256 != wantInternal[artifact.Stage] {
			return fmt.Errorf("Cycle 4 receipt: invalid artifact binding %q", artifact.Stage)
		}
	}
	if value.Artifacts[3].Stage != "evaluation" || value.Artifacts[3].RawSHA256 != blindBytesSHA256(evaluationRaw) {
		return fmt.Errorf("Cycle 4 receipt: evaluation raw binding mismatch")
	}
	if err := validateCycle4CollisionObservation(value.Collision, freeze.Collision); err != nil {
		return err
	}
	if !value.CandidateDeterministic || !value.EvaluationDeterministic || value.OracleIsolation.Status != "PASS" ||
		!value.OracleIsolation.CandidateBytesUnchanged || !value.OracleIsolation.EvaluationBytesChanged {
		return fmt.Errorf("Cycle 4 receipt: determinism or oracle isolation failed")
	}
	if value.SHA256 != cycle4RunReceiptDigest(value) {
		return fmt.Errorf("Cycle 4 receipt: digest mismatch")
	}
	return nil
}

func (value cycle4RunReceipt) ValidateTerminal(
	freeze cycle4ExtractorFreeze,
	build cycle4CandidateBuild,
	failure blindTerminalScorecard,
	failureRaw []byte,
) error {
	if value.Version != cycle4ReceiptVersion || value.Status != "TERMINAL_ERROR" ||
		value.CandidateID != cycle4CandidateID || value.ExtractorFreezeSHA256 != freeze.SHA256 ||
		value.Builds != 2 || !reflect.DeepEqual(value.ProductionLoadsPerBuild, []int{1, 1}) ||
		!reflect.DeepEqual(value.UnavailableArtifacts, []string{"h1"}) || len(value.Artifacts) != 3 ||
		value.Terminal == nil || value.Terminal.Stage != failure.Stage || value.Terminal.Reason != failure.Reason ||
		value.Verdict != EvaluationFail || !validSHA256(value.SHA256) {
		return fmt.Errorf("Cycle 4 terminal receipt: invalid identity or run accounting")
	}
	want := []cycle4RunArtifact{
		{Stage: "authority", RawSHA256: blindBytesSHA256(build.authorityRaw), InternalSHA256: build.run.authority.SHA256},
		{Stage: "h0", RawSHA256: blindBytesSHA256(build.h0Raw), InternalSHA256: build.run.h0.SHA256},
		{Stage: "evaluation", RawSHA256: blindBytesSHA256(failureRaw), InternalSHA256: failure.SHA256},
	}
	if !reflect.DeepEqual(value.Artifacts, want) {
		return fmt.Errorf("Cycle 4 terminal receipt: artifact bindings changed")
	}
	if err := validateCycle4CollisionObservation(value.Collision, freeze.Collision); err != nil {
		return err
	}
	if !value.CandidateDeterministic || !value.EvaluationDeterministic ||
		value.OracleIsolation != (cycle4OracleIsolation{Status: "NOT_TESTED_TERMINAL_BEFORE_EVALUATION"}) {
		return fmt.Errorf("Cycle 4 terminal receipt: determinism or oracle-isolation status changed")
	}
	if value.SHA256 != cycle4RunReceiptDigest(value) {
		return fmt.Errorf("Cycle 4 terminal receipt: digest mismatch")
	}
	return nil
}

func TestBlindFixtureReceiptContract(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join(experimentRoot(t), "..", "..", ".."))
	root := filepath.Join(repositoryRoot, "testdata", "experiments", "clientrecipe-blind")
	receiptRaw := readExperimentFile(t, filepath.Join(root, "fixture_receipt.json"))
	receipt, err := decodeBlindFixtureReceipt(receiptRaw)
	if err != nil {
		t.Fatal(err)
	}
	briefRaw := readExperimentFile(t, filepath.Join(root, "author_brief.md"))
	preregRaw := readExperimentFile(t, filepath.Join(root, "preregistration.json"))
	schemaRaw := readExperimentFile(t, filepath.Join(root, "oracle.schema.json"))
	oracleRaw := readExperimentFile(t, filepath.Join(root, "oracle.json"))
	if err := receipt.Validate(root, briefRaw, preregRaw, schemaRaw, oracleRaw); err != nil {
		t.Fatal(err)
	}
	canonical, err := encodeBlindFixtureReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(receiptRaw, canonical) {
		t.Fatal("blind fixture receipt: non-canonical bytes")
	}
}

func TestBlindEvaluatorRepresentsPassAndFail(t *testing.T) {
	prereg := syntheticBlindPreregistration()
	oracle := syntheticBlindOracle()
	if err := oracle.Validate(); err != nil {
		t.Fatal(err)
	}
	candidate := syntheticBlindCandidate(oracle)
	run := blindCandidateRun{
		authority: Authority{SHA256: strings.Repeat("1", 64)},
		h0:        H0Result{SHA256: strings.Repeat("2", 64)},
		h1:        H1Result{SHA256: strings.Repeat("3", 64)},
	}
	passing := evaluateBlindCandidate(prereg, oracle, candidate, run, true, true, true)
	if err := passing.Validate(prereg); err != nil {
		t.Fatal(err)
	}
	if passing.Verdict != EvaluationPass {
		t.Fatalf("representable passing scorecard = %s, gates %#v", passing.Verdict, passing.Gates)
	}
	wantExclusionIDs := make([]string, 0, len(oracle.Excluded))
	for _, excluded := range oracle.Excluded {
		wantExclusionIDs = append(wantExclusionIDs, excluded.ID)
	}
	sort.Strings(wantExclusionIDs)
	wantExclusions := blindSetMetric{
		Truth: len(oracle.Excluded), Predicted: len(oracle.Excluded), Matched: len(oracle.Excluded),
		TruePositive: wantExclusionIDs, FalsePositive: []string{}, FalseNegative: []string{},
	}
	wantReasons := blindExactMetric{
		Correct: len(oracle.Excluded), Total: len(oracle.Excluded), Mismatches: []string{},
	}
	if !reflect.DeepEqual(passing.Exclusions, wantExclusions) ||
		!reflect.DeepEqual(passing.ExclusionReasons, wantReasons) {
		t.Fatalf("representable PASS does not exhaustively account for exclusions/reasons: %#v / %#v",
			passing.Exclusions, passing.ExclusionReasons)
	}

	failingCandidate := candidate
	failingCandidate.Instances = append([]blindCandidateInstance(nil), candidate.Instances...)
	failingCandidate.Instances = append(failingCandidate.Instances, blindCandidateInstance{
		ID: "candidate-decoy", Roles: []blindCandidateRole{{
			Role: H1RoleLocalWrapper, Evidence: []blindLocator{oracle.Excluded[0].Anchor},
		}}, Missing: []H1Role{},
	})
	failingCandidate.Ledger.Admitted++
	failingCandidate.Ledger.Observed++
	failing := evaluateBlindCandidate(prereg, oracle, failingCandidate, run, true, true, true)
	if err := failing.Validate(prereg); err != nil {
		t.Fatal(err)
	}
	if failing.Verdict != EvaluationFail || failing.CriticalFalsePositives != 1 ||
		!reflect.DeepEqual(failing.DecoyAdmissions, []string{"candidate-decoy"}) {
		t.Fatalf("representable failing scorecard = %#v", failing)
	}
}

func blindFixtureMissing(repoRoot, oraclePath, receiptPath string) ([]string, error) {
	wanted := []struct {
		label string
		path  string
	}{
		{label: "repo/go.mod", path: filepath.Join(repoRoot, "go.mod")},
		{label: "oracle.json", path: oraclePath},
		{label: "fixture_receipt.json", path: receiptPath},
	}
	missing := make([]string, 0, len(wanted))
	for _, item := range wanted {
		info, err := os.Lstat(item.path)
		if os.IsNotExist(err) {
			missing = append(missing, item.label)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("blind fixture readiness: inspect %s: %w", item.label, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("blind fixture readiness: %s is not a regular file", item.label)
		}
	}
	return missing, nil
}

func decodeBlindPreregistration(raw, briefRaw []byte) (blindPreregistration, error) {
	var value blindPreregistration
	if err := decodeStrict(raw, &value, "blind preregistration"); err != nil {
		return blindPreregistration{}, err
	}
	briefSHA := blindBytesSHA256(briefRaw)
	want := syntheticBlindPreregistration()
	want.AuthorBriefSHA256 = briefSHA
	if !reflect.DeepEqual(value, want) {
		return blindPreregistration{}, fmt.Errorf("blind preregistration: contract differs from the frozen gate")
	}
	return value, nil
}

func syntheticBlindPreregistration() blindPreregistration {
	return blindPreregistration{
		Version: 1, Status: "PREREGISTERED_BEFORE_AUTHORSHIP",
		BlindOracleSchemaVersion: blindOracleVersion,
		FrozenExtractor: blindFrozenExtractor{
			H1SHA256: frozenH1SHA256, ReceiptSHA256: frozenReceiptSHA256,
		},
		CandidateInputs:          []string{"sealed_repository_root", "frozen_extractor_identity"},
		CandidateForbiddenInputs: []string{"blind_oracle", "existing_fixture", "existing_oracle", "evaluator", "model_provider", "fixture_specific_literals"},
		InstanceMatch: blindInstanceMatchRule{
			Key:                "unique_exact_local_boundary_anchor_path_and_line",
			ForbiddenFallbacks: []string{"basename", "directory", "vendor", "symbol_name", "opaque_candidate_id", "fuzzy_match"},
		},
		Thresholds: blindThresholdContract{
			TruthInstances: 5, InstancePrecision: "TP == predicted", InstanceRecall: "TP * 4 >= truth * 3",
			CriticalFalsePositives: 0, RolePrecision: "TP == predicted", RoleRecall: "TP * 5 >= truth * 4",
			TaskBehaviourCoverage: "matched * 5 >= truth * 4", MatchedCompletenessAndMissing: "exact",
			EvidenceGroundingPrecision: "grounded == emitted", OracleAnchorRecall: "matched * 5 >= truth * 4",
			DecoyAdmission: 0, ExclusionReasonPrecision: "correct == predicted",
			ExclusionRecall: "matched * 4 >= truth * 3", CallbackAccounting: "exact_when_present_else_0_0_0",
			CandidateAndEvaluation: "deterministic", LedgerAndSourceAccounting: "exact",
		},
		OverallRule: "PASS_IFF_EVERY_THRESHOLD_PASSES", PassInterpretation: "LIMITED_SECOND_FIXTURE_EVIDENCE",
		FailureInterpretation: "FROZEN_H1_DOES_NOT_GENERALIZE_ON_PREREGISTERED_SCENE",
		UserUtility:           "NOT_TESTED", ProductionReadiness: "NOT_READY", FrequencyReducer: "REJECTED_BY_CYCLE2",
	}
}

func decodeBlindFixtureReceipt(raw []byte) (blindFixtureReceipt, error) {
	var value blindFixtureReceipt
	if err := decodeStrict(raw, &value, "blind fixture receipt"); err != nil {
		return blindFixtureReceipt{}, err
	}
	return value, nil
}

func encodeBlindFixtureReceipt(value blindFixtureReceipt) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("blind fixture receipt: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func assertBlindCycle4Golden(t *testing.T, root, name string, actual []byte) {
	t.Helper()
	if !strings.HasPrefix(name, "cycle4/") || filepath.Clean(name) != filepath.FromSlash(name) {
		t.Fatalf("Cycle 4 artifact path escaped its namespace: %q", name)
	}
	filename := filepath.Join(root, "golden", name)
	if os.Getenv("REPOMAP_UPDATE_EXPERIMENT_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read blind golden %s: %v; update with REPOMAP_UPDATE_EXPERIMENT_GOLDEN=1", name, err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatalf("blind golden %s changed; inspect before REPOMAP_UPDATE_EXPERIMENT_GOLDEN=1", name)
	}
}

func (value blindFixtureReceipt) Validate(root string, briefRaw, preregRaw, schemaRaw, oracleRaw []byte) error {
	if value.Version != blindReceiptVersion || value.Status != "SEALED" ||
		value.OracleSchemaVersion != blindOracleVersion || !validSHA256(value.AuthorBriefSHA256) ||
		!validSHA256(value.PreregistrationSHA256) || !validSHA256(value.OracleSchemaSHA256) || !validSHA256(value.RepositoryTreeSHA256) ||
		!validSHA256(value.DependenciesTreeSHA256) || !validSHA256(value.OracleSHA256) || !validSHA256(value.SHA256) ||
		len(value.RepositoryFiles) == 0 || len(value.DependenciesFiles) == 0 ||
		!value.Chronology.FixtureValidatedBeforeSeal || !value.Chronology.OracleSealedBeforeCandidate {
		return fmt.Errorf("blind fixture receipt: invalid seal identity")
	}
	if value.AuthorBriefSHA256 != blindBytesSHA256(briefRaw) ||
		value.PreregistrationSHA256 != blindBytesSHA256(preregRaw) || value.OracleSchemaSHA256 != blindBytesSHA256(schemaRaw) ||
		value.OracleSHA256 != blindBytesSHA256(oracleRaw) {
		return fmt.Errorf("blind fixture receipt: authority digest mismatch")
	}
	repositoryFiles, err := blindFileInventory(filepath.Join(root, "repo"))
	if err != nil {
		return err
	}
	dependencyFiles, err := blindFileInventory(filepath.Join(root, "deps"))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(value.RepositoryFiles, repositoryFiles) ||
		!reflect.DeepEqual(value.DependenciesFiles, dependencyFiles) {
		return fmt.Errorf("blind fixture receipt: sealed file inventory drift")
	}
	if value.RepositoryTreeSHA256 != blindFileTreeSHA256(value.RepositoryFiles) ||
		value.DependenciesTreeSHA256 != blindFileTreeSHA256(value.DependenciesFiles) ||
		value.SHA256 != blindFixtureReceiptSHA256(value) {
		return fmt.Errorf("blind fixture receipt: tree or receipt digest mismatch")
	}
	return nil
}

func blindFileInventory(root string) ([]blindReceiptFile, error) {
	files, err := RepositoryFiles(root)
	if err != nil {
		return nil, fmt.Errorf("blind fixture receipt: inventory %s: %w", filepath.Base(root), err)
	}
	result := make([]blindReceiptFile, 0, len(files))
	for _, relative := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, fmt.Errorf("blind fixture receipt: read %s: %w", relative, err)
		}
		result = append(result, blindReceiptFile{Path: relative, Bytes: len(raw), SHA256: blindBytesSHA256(raw)})
	}
	return result, nil
}

func blindFileTreeSHA256(files []blindReceiptFile) string {
	raw, _ := json.Marshal(files)
	return blindBytesSHA256(raw)
}

func blindFixtureReceiptSHA256(value blindFixtureReceipt) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	return blindBytesSHA256(raw)
}

func blindTerminal(stage string, err error) error {
	reason := stage + "_failed"
	if stage == "authority_preparation" && strings.Contains(err.Error(), "duplicate relation identity") {
		reason = "duplicate_program_relation_identity"
	}
	return &blindCandidateTerminal{Stage: stage, Reason: reason, Err: err}
}

func buildBlindTerminalScorecard(
	prereg blindPreregistration,
	receipt blindFixtureReceipt,
	run blindCandidateRun,
	terminal *blindCandidateTerminal,
) blindTerminalScorecard {
	result := blindTerminalScorecard{
		Version: blindScoreVersion, Status: "TERMINAL_ERROR", Stage: terminal.Stage, Reason: terminal.Reason,
		FrozenH1SHA256: prereg.FrozenExtractor.H1SHA256, FrozenReceiptSHA256: prereg.FrozenExtractor.ReceiptSHA256,
		FixtureReceiptSHA256: receipt.SHA256, CandidateOracleIsolation: true,
		AuthorityArtifact: run.authority.SHA256 != "", H0Artifact: run.h0.SHA256 != "", H1Artifact: len(run.h1Raw) != 0,
		MetricsStatus: "NOT_AVAILABLE_TERMINAL_BEFORE_EVALUATION", GeneralizationStatus: "NOT_ESTABLISHED",
		UserUtilityStatus: "NOT_TESTED", ProductionReadiness: "NOT_READY", FrequencyReducer: "IGNORED_REJECTED",
		Verdict: EvaluationFail,
	}
	result.SHA256 = blindTerminalScorecardSHA256(result)
	return result
}

func encodeBlindTerminalScorecard(value blindTerminalScorecard) ([]byte, error) {
	if value.Version != blindScoreVersion || value.Status != "TERMINAL_ERROR" || value.Stage == "" || value.Reason == "" ||
		!validSHA256(value.FrozenH1SHA256) || !validSHA256(value.FrozenReceiptSHA256) ||
		!validSHA256(value.FixtureReceiptSHA256) || !value.CandidateOracleIsolation ||
		value.MetricsStatus != "NOT_AVAILABLE_TERMINAL_BEFORE_EVALUATION" || value.GeneralizationStatus != "NOT_ESTABLISHED" ||
		value.UserUtilityStatus != "NOT_TESTED" || value.ProductionReadiness != "NOT_READY" ||
		value.FrequencyReducer != "IGNORED_REJECTED" || value.Verdict != EvaluationFail ||
		value.SHA256 != blindTerminalScorecardSHA256(value) {
		return nil, fmt.Errorf("blind terminal scorecard: invalid closed failure")
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("blind terminal scorecard: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func blindTerminalScorecardSHA256(value blindTerminalScorecard) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	return blindBytesSHA256(raw)
}

func blindAuthorityMatchesReceipt(authority Authority, files []blindReceiptFile) bool {
	if authority.Coverage.FilesObserved != len(files) || len(authority.Sources) != len(files) {
		return false
	}
	for index, source := range authority.Sources {
		if source.Path != files[index].Path || source.Bytes != files[index].Bytes || source.SHA256 != files[index].SHA256 {
			return false
		}
	}
	return true
}

func decodeBlindOracle(raw []byte) (blindOracle, error) {
	var value blindOracle
	if err := decodeStrict(raw, &value, "blind oracle"); err != nil {
		return blindOracle{}, err
	}
	if err := value.Validate(); err != nil {
		return blindOracle{}, err
	}
	return value, nil
}

func (value blindOracle) Validate() error {
	if value.Version != blindOracleVersion || !validSHA256(value.AuthorBriefSHA256) ||
		len(value.SealedFixtureCommit) < 7 || len(value.SealedFixtureCommit) > 40 ||
		strings.Trim(value.SealedFixtureCommit, "0123456789abcdef") != "" ||
		len(value.Instances) != 5 || len(value.Excluded) < 6 || value.Callbacks == nil {
		return fmt.Errorf("blind oracle: invalid identity or required scene cardinality")
	}
	anchors := make(map[string]string, len(value.Instances)+len(value.Excluded))
	previous := ""
	for _, instance := range value.Instances {
		if !validBlindID(instance.ID) || (previous != "" && previous >= instance.ID) || !validText(instance.Label) ||
			!validText(instance.ExternalSystem) || instance.Roles == nil || instance.Missing == nil ||
			instance.WrapperAnchor.Validate() != nil || len(instance.TaskBehaviours) == 0 ||
			!canonicalBlindLocators(instance.TaskBehaviours) {
			return fmt.Errorf("blind oracle: invalid instance %q", instance.ID)
		}
		previous = instance.ID
		anchorKey := instance.WrapperAnchor.Key()
		if owner := anchors[anchorKey]; owner != "" {
			return fmt.Errorf("blind oracle: anchor %s is shared by %s and %s", anchorKey, owner, instance.ID)
		}
		anchors[anchorKey] = instance.ID
		present := make(map[H1Role]struct{}, len(instance.Roles))
		previousRole := ""
		for _, role := range instance.Roles {
			if h1RoleIndex(role.Role) < 0 || (previousRole != "" && previousRole >= string(role.Role)) ||
				len(role.Evidence) == 0 || !canonicalBlindLocators(role.Evidence) {
				return fmt.Errorf("blind oracle: invalid role evidence in %q", instance.ID)
			}
			previousRole = string(role.Role)
			present[role.Role] = struct{}{}
		}
		if instance.Complete != h1HasMandatoryRoles(present) ||
			!reflect.DeepEqual(instance.Missing, blindMissingTaskRoles(present)) {
			return fmt.Errorf("blind oracle: inconsistent wrapper, task, completeness, or missing set in %q", instance.ID)
		}
	}
	previous = ""
	for _, excluded := range value.Excluded {
		if !validBlindID(excluded.ID) || (previous != "" && previous >= excluded.ID) || !validText(excluded.Kind) ||
			!excluded.Reason.Valid() || excluded.Anchor.Validate() != nil {
			return fmt.Errorf("blind oracle: invalid exclusion %q", excluded.ID)
		}
		previous = excluded.ID
		anchorKey := excluded.Anchor.Key()
		if owner := anchors[anchorKey]; owner != "" {
			return fmt.Errorf("blind oracle: anchor %s is shared by %s and %s", anchorKey, owner, excluded.ID)
		}
		anchors[anchorKey] = excluded.ID
	}
	previous = ""
	for _, callback := range value.Callbacks {
		if !validBlindID(callback.ID) || (previous != "" && previous >= callback.ID) ||
			callback.PassAnchor.Validate() != nil || callback.TargetAnchor.Validate() != nil {
			return fmt.Errorf("blind oracle: invalid callback %q", callback.ID)
		}
		previous = callback.ID
	}
	return nil
}

func validBlindID(value string) bool {
	if value == "" || len(value) > 120 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func blindMissingTaskRoles(present map[H1Role]struct{}) []H1Role {
	result := make([]H1Role, 0)
	for _, role := range h1Roles {
		if role == H1RoleFailurePolicy {
			continue
		}
		if _, found := present[role]; !found {
			result = append(result, role)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func (value blindLocator) Validate() error {
	if !validSourcePath(value.Path) || value.Line <= 0 {
		return fmt.Errorf("invalid locator")
	}
	return nil
}

func (value blindLocator) Key() string     { return fmt.Sprintf("%s:%d", value.Path, value.Line) }
func (value blindLocator) sortKey() string { return fmt.Sprintf("%s\x00%09d", value.Path, value.Line) }

func canonicalBlindLocators(values []blindLocator) bool {
	previous := ""
	for _, value := range values {
		if value.Validate() != nil || (previous != "" && previous >= value.sortKey()) {
			return false
		}
		previous = value.sortKey()
	}
	return true
}

func canonicalizeBlindLocators(values []blindLocator) []blindLocator {
	result := append([]blindLocator{}, values...)
	sort.Slice(result, func(i, j int) bool { return result[i].sortKey() < result[j].sortKey() })
	if len(result) < 2 {
		return result
	}
	compacted := result[:1]
	for _, value := range result[1:] {
		if compacted[len(compacted)-1] != value {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

func blindLocatorsContain(values []blindLocator, want blindLocator) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validateBlindOracleLocators(repoRoot string, oracle blindOracle) error {
	locators := make([]blindLocator, 0)
	for _, instance := range oracle.Instances {
		locators = append(locators, instance.WrapperAnchor)
		locators = append(locators, instance.TaskBehaviours...)
		for _, role := range instance.Roles {
			locators = append(locators, role.Evidence...)
		}
	}
	for _, excluded := range oracle.Excluded {
		locators = append(locators, excluded.Anchor)
	}
	for _, callback := range oracle.Callbacks {
		locators = append(locators, callback.PassAnchor, callback.TargetAnchor)
	}
	cache := make(map[string][][]byte)
	for _, locator := range locators {
		lines, found := cache[locator.Path]
		if !found {
			raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(locator.Path)))
			if err != nil {
				return fmt.Errorf("blind oracle: read locator %s: %w", locator.Key(), err)
			}
			lines = bytes.Split(raw, []byte("\n"))
			cache[locator.Path] = lines
		}
		if locator.Line > len(lines) || len(bytes.TrimSpace(lines[locator.Line-1])) == 0 {
			return fmt.Errorf("blind oracle: locator %s does not name a non-empty source line", locator.Key())
		}
	}
	return nil
}

func blindCandidateFromH1(value H1Result, authority Authority) blindCandidateView {
	result := blindCandidateView{
		Instances: make([]blindCandidateInstance, 0, len(value.Instances)),
		Excluded:  make([]blindCandidateExclusion, 0, len(value.Excluded)),
		Callbacks: value.Callbacks, Ledger: value.Ledger,
	}
	objectLocations := make(map[string]blindLocator, len(authority.Program.Objects))
	for _, object := range authority.Program.Objects {
		if object.Location != nil {
			objectLocations[object.ID] = blindLocator{Path: object.Location.Path, Line: object.Location.Line}
		}
	}
	relationLocations := make(map[string]blindLocator, len(authority.Program.Relations))
	for _, relation := range authority.Program.Relations {
		if relation.Location != nil {
			relationLocations[relation.ID] = blindLocator{Path: relation.Location.Path, Line: relation.Location.Line}
		}
	}
	for _, id := range value.Reachability.ReachedObjectIDs {
		if locator, found := objectLocations[id]; found {
			result.ReachableAnchors = append(result.ReachableAnchors, locator)
		}
	}
	for _, id := range value.Reachability.ExactRelationIDs {
		if locator, found := relationLocations[id]; found {
			result.ReachableAnchors = append(result.ReachableAnchors, locator)
		}
	}
	result.ReachableAnchors = canonicalizeBlindLocators(result.ReachableAnchors)
	for _, instance := range value.Instances {
		candidate := blindCandidateInstance{
			ID: instance.ID, Complete: instance.Complete, VerificationKind: instance.VerificationKind,
			Roles: make([]blindCandidateRole, 0, len(instance.Roles)), Missing: append([]H1Role{}, instance.Missing...),
		}
		for _, role := range instance.Roles {
			candidate.Roles = append(candidate.Roles, blindCandidateRole{
				Role: role.Role, Evidence: blindLocatorsFromEvidence(role.Evidence),
			})
		}
		result.Instances = append(result.Instances, candidate)
	}
	for _, excluded := range value.Excluded {
		result.Excluded = append(result.Excluded, blindCandidateExclusion{
			ID: excluded.ID, Reason: excluded.Reason, Evidence: blindLocatorsFromEvidence(excluded.Evidence),
		})
	}
	return result
}

func blindLocatorsFromEvidence(values []H1Evidence) []blindLocator {
	result := make([]blindLocator, 0, len(values))
	for _, value := range values {
		result = append(result, blindLocator{Path: value.Path, Line: value.Line})
	}
	return result
}

func evaluateBlindCandidate(
	prereg blindPreregistration,
	oracle blindOracle,
	candidate blindCandidateView,
	run blindCandidateRun,
	sourceExact, candidateDeterministic, evaluationDeterministic bool,
) blindScorecard {
	instanceMatches := matchBlindInstances(candidate.Instances, oracle.Instances)
	exclusionMatches := matchBlindExclusions(candidate.Excluded, oracle.Excluded)
	instances := blindInstanceMetric(candidate.Instances, oracle.Instances, instanceMatches)
	roles := blindRoleMetric(candidate.Instances, oracle.Instances, instanceMatches)
	taskBehaviours := blindTaskBehaviourMetric(candidate, oracle.Instances, instanceMatches)
	complete, missing := blindClassificationMetrics(candidate.Instances, oracle.Instances, instanceMatches)
	grounding := blindGrounding(candidate, oracle, instanceMatches, exclusionMatches)
	exclusions, reasons := blindExclusionMetrics(candidate.Excluded, oracle.Excluded, exclusionMatches)
	expectedCallbacks := blindOracleCallbackCounts{Observed: len(oracle.Callbacks), Closed: len(oracle.Callbacks)}
	callbacks := blindCallbackMetric{
		Expected: expectedCallbacks,
		Actual: blindOracleCallbackCounts{
			Observed: candidate.Callbacks.Observed, Closed: candidate.Callbacks.Closed, Frontier: candidate.Callbacks.Frontier,
		},
	}
	callbacks.Exact = callbacks.Expected == callbacks.Actual &&
		blindCallbackAnchorsExact(candidate.Callbacks, run.authority, oracle.Callbacks)
	expectedLedger := H1Ledger{
		Observed: len(oracle.Instances) + len(oracle.Excluded),
		Admitted: len(oracle.Instances), Excluded: len(oracle.Excluded),
	}
	ledger := blindLedgerMetric{Expected: expectedLedger, Actual: candidate.Ledger, Exact: expectedLedger == candidate.Ledger}
	result := blindScorecard{
		Version: blindScoreVersion, FrozenH1SHA256: prereg.FrozenExtractor.H1SHA256,
		FrozenReceiptSHA256: prereg.FrozenExtractor.ReceiptSHA256,
		AuthoritySHA256:     run.authority.SHA256, H0SHA256: run.h0.SHA256, CandidateH1SHA256: run.h1.SHA256,
		InstanceMatches: instanceMatches, ExclusionMatches: exclusionMatches,
		Instances: instances, CriticalFalsePositives: len(instances.FalsePositive), Roles: roles,
		TaskBehaviours: taskBehaviours, Completeness: complete, MissingSets: missing,
		EvidenceGrounding: grounding, OracleAnchors: instances,
		DecoyAdmissions: blindDecoyAdmissions(candidate.Instances, oracle.Excluded),
		Exclusions:      exclusions, ExclusionReasons: reasons, Callbacks: callbacks, Ledger: ledger,
		SourceAccountingExact: sourceExact, CandidateDeterministic: candidateDeterministic,
		EvaluationDeterministic: evaluationDeterministic, FrequencyReducer: "IGNORED_REJECTED",
	}
	return finalizeBlindScorecard(result)
}

func matchBlindInstances(predictions []blindCandidateInstance, truth []blindOracleInstance) []blindMatch {
	truthByAnchor := make(map[string]string, len(truth))
	for _, instance := range truth {
		truthByAnchor[instance.WrapperAnchor.Key()] = instance.ID
	}
	proposals := make([]blindMatch, 0, len(predictions))
	counts := make(map[string]int)
	for _, prediction := range predictions {
		anchor, ok := blindCandidateRoleAnchor(prediction, H1RoleLocalWrapper)
		if !ok {
			continue
		}
		if oracleID := truthByAnchor[anchor.Key()]; oracleID != "" {
			proposals = append(proposals, blindMatch{OracleID: oracleID, CandidateID: prediction.ID})
			counts[oracleID]++
		}
	}
	result := make([]blindMatch, 0, len(proposals))
	for _, proposal := range proposals {
		if counts[proposal.OracleID] == 1 {
			result = append(result, proposal)
		}
	}
	sortBlindMatches(result)
	return result
}

func matchBlindExclusions(predictions []blindCandidateExclusion, truth []blindOracleExclusion) []blindMatch {
	anchors := make(map[string]string, len(truth))
	for _, excluded := range truth {
		anchors[excluded.Anchor.Key()] = excluded.ID
	}
	proposals := make([]blindMatch, 0, len(predictions))
	counts := make(map[string]int)
	for _, prediction := range predictions {
		matches := make(map[string]struct{})
		for _, evidence := range prediction.Evidence {
			if oracleID := anchors[evidence.Key()]; oracleID != "" {
				matches[oracleID] = struct{}{}
			}
		}
		if len(matches) != 1 {
			continue
		}
		for oracleID := range matches {
			proposals = append(proposals, blindMatch{OracleID: oracleID, CandidateID: prediction.ID})
			counts[oracleID]++
		}
	}
	result := make([]blindMatch, 0, len(proposals))
	for _, proposal := range proposals {
		if counts[proposal.OracleID] == 1 {
			result = append(result, proposal)
		}
	}
	sortBlindMatches(result)
	return result
}

func sortBlindMatches(values []blindMatch) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].OracleID != values[j].OracleID {
			return values[i].OracleID < values[j].OracleID
		}
		return values[i].CandidateID < values[j].CandidateID
	})
}

func blindCandidateRoleAnchor(instance blindCandidateInstance, role H1Role) (blindLocator, bool) {
	for _, candidate := range instance.Roles {
		if candidate.Role == role && len(candidate.Evidence) == 1 {
			return candidate.Evidence[0], true
		}
	}
	return blindLocator{}, false
}

func blindInstanceMetric(predictions []blindCandidateInstance, truth []blindOracleInstance, matches []blindMatch) blindSetMetric {
	metric := blindSetMetric{Truth: len(truth), Predicted: len(predictions), TruePositive: []string{}, FalsePositive: []string{}, FalseNegative: []string{}}
	matchedCandidate, matchedOracle := blindMatchedIDs(matches)
	for _, match := range matches {
		metric.TruePositive = append(metric.TruePositive, match.OracleID)
	}
	for _, prediction := range predictions {
		if _, found := matchedCandidate[prediction.ID]; !found {
			metric.FalsePositive = append(metric.FalsePositive, prediction.ID)
		}
	}
	for _, instance := range truth {
		if _, found := matchedOracle[instance.ID]; !found {
			metric.FalseNegative = append(metric.FalseNegative, instance.ID)
		}
	}
	blindCanonicalMetric(&metric)
	return metric
}

func blindRoleMetric(predictions []blindCandidateInstance, truth []blindOracleInstance, matches []blindMatch) blindSetMetric {
	metric := blindSetMetric{TruePositive: []string{}, FalsePositive: []string{}, FalseNegative: []string{}}
	predictedByID := blindCandidateInstancesByID(predictions)
	truthByID := blindOracleInstancesByID(truth)
	matchedCandidate := make(map[string]string, len(matches))
	for _, match := range matches {
		matchedCandidate[match.CandidateID] = match.OracleID
	}
	for _, prediction := range predictions {
		oracleID := matchedCandidate[prediction.ID]
		truthRoles := map[H1Role]struct{}{}
		if oracleID != "" {
			for _, role := range truthByID[oracleID].Roles {
				truthRoles[role.Role] = struct{}{}
			}
		}
		for _, role := range prediction.Roles {
			key := prediction.ID + "/" + string(role.Role)
			if _, found := truthRoles[role.Role]; found {
				metric.TruePositive = append(metric.TruePositive, oracleID+"/"+string(role.Role))
			} else {
				metric.FalsePositive = append(metric.FalsePositive, key)
			}
		}
	}
	for _, instance := range truth {
		var prediction blindCandidateInstance
		found := false
		for _, match := range matches {
			if match.OracleID == instance.ID {
				prediction, found = predictedByID[match.CandidateID], true
				break
			}
		}
		predictedRoles := map[H1Role]struct{}{}
		if found {
			for _, role := range prediction.Roles {
				predictedRoles[role.Role] = struct{}{}
			}
		}
		for _, role := range instance.Roles {
			if _, present := predictedRoles[role.Role]; !present {
				metric.FalseNegative = append(metric.FalseNegative, instance.ID+"/"+string(role.Role))
			}
		}
	}
	for _, instance := range predictions {
		metric.Predicted += len(instance.Roles)
	}
	for _, instance := range truth {
		metric.Truth += len(instance.Roles)
	}
	blindCanonicalMetric(&metric)
	return metric
}

func blindTaskBehaviourMetric(candidate blindCandidateView, truth []blindOracleInstance, matches []blindMatch) blindSetMetric {
	metric := blindSetMetric{TruePositive: []string{}, FalsePositive: []string{}, FalseNegative: []string{}}
	for _, instance := range truth {
		metric.Truth += len(instance.TaskBehaviours)
	}
	matchedOracle := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		matchedOracle[match.OracleID] = struct{}{}
	}
	for _, instance := range truth {
		if _, found := matchedOracle[instance.ID]; !found {
			continue
		}
		for _, anchor := range instance.TaskBehaviours {
			if blindLocatorsContain(candidate.ReachableAnchors, anchor) {
				metric.TruePositive = append(metric.TruePositive, instance.ID+"/"+anchor.Key())
			}
		}
	}
	metric.Predicted = len(metric.TruePositive)
	seen := make(map[string]struct{}, len(metric.TruePositive))
	for _, key := range metric.TruePositive {
		seen[key] = struct{}{}
	}
	for _, instance := range truth {
		for _, anchor := range instance.TaskBehaviours {
			key := instance.ID + "/" + anchor.Key()
			if _, found := seen[key]; !found {
				metric.FalseNegative = append(metric.FalseNegative, key)
			}
		}
	}
	blindCanonicalMetric(&metric)
	return metric
}

func blindClassificationMetrics(predictions []blindCandidateInstance, truth []blindOracleInstance, matches []blindMatch) (blindExactMetric, blindExactMetric) {
	predictedByID := blindCandidateInstancesByID(predictions)
	truthByID := blindOracleInstancesByID(truth)
	complete := blindExactMetric{Total: len(matches), Mismatches: []string{}}
	missing := blindExactMetric{Total: len(matches), Mismatches: []string{}}
	for _, match := range matches {
		prediction, expected := predictedByID[match.CandidateID], truthByID[match.OracleID]
		if prediction.Complete == expected.Complete {
			complete.Correct++
		} else {
			complete.Mismatches = append(complete.Mismatches, match.OracleID)
		}
		present := make(map[H1Role]struct{}, len(prediction.Roles))
		for _, role := range prediction.Roles {
			present[role.Role] = struct{}{}
		}
		if reflect.DeepEqual(blindMissingTaskRoles(present), expected.Missing) {
			missing.Correct++
		} else {
			missing.Mismatches = append(missing.Mismatches, match.OracleID)
		}
	}
	sort.Strings(complete.Mismatches)
	sort.Strings(missing.Mismatches)
	return complete, missing
}

func blindGrounding(candidate blindCandidateView, oracle blindOracle, instanceMatches, exclusionMatches []blindMatch) blindGroundingMetric {
	metric := blindGroundingMetric{Ungrounded: []string{}}
	truthInstances := blindOracleInstancesByID(oracle.Instances)
	instanceMatch := make(map[string]string, len(instanceMatches))
	for _, match := range instanceMatches {
		instanceMatch[match.CandidateID] = match.OracleID
	}
	for _, instance := range candidate.Instances {
		oracleID := instanceMatch[instance.ID]
		truthRoles := map[H1Role][]blindLocator{}
		if oracleID != "" {
			for _, role := range truthInstances[oracleID].Roles {
				truthRoles[role.Role] = role.Evidence
			}
		}
		for _, role := range instance.Roles {
			for _, evidence := range role.Evidence {
				metric.Emitted++
				if blindLocatorsContain(truthRoles[role.Role], evidence) {
					metric.Grounded++
				} else {
					metric.Ungrounded = append(metric.Ungrounded, instance.ID+"/"+string(role.Role)+"/"+evidence.Key())
				}
			}
		}
	}
	_ = exclusionMatches
	sort.Strings(metric.Ungrounded)
	return metric
}

func blindExclusionMetrics(predictions []blindCandidateExclusion, truth []blindOracleExclusion, matches []blindMatch) (blindSetMetric, blindExactMetric) {
	metric := blindSetMetric{Truth: len(truth), Predicted: len(predictions), TruePositive: []string{}, FalsePositive: []string{}, FalseNegative: []string{}}
	reasons := blindExactMetric{Total: len(predictions), Mismatches: []string{}}
	predictedByID := make(map[string]blindCandidateExclusion, len(predictions))
	truthByID := make(map[string]blindOracleExclusion, len(truth))
	for _, row := range predictions {
		predictedByID[row.ID] = row
	}
	for _, row := range truth {
		truthByID[row.ID] = row
	}
	matchedCandidate, matchedOracle := blindMatchedIDs(matches)
	for _, match := range matches {
		metric.TruePositive = append(metric.TruePositive, match.OracleID)
		if predictedByID[match.CandidateID].Reason == truthByID[match.OracleID].Reason {
			reasons.Correct++
		} else {
			reasons.Mismatches = append(reasons.Mismatches, match.CandidateID)
		}
	}
	for _, row := range predictions {
		if _, found := matchedCandidate[row.ID]; !found {
			metric.FalsePositive = append(metric.FalsePositive, row.ID)
			reasons.Mismatches = append(reasons.Mismatches, row.ID)
		}
	}
	for _, row := range truth {
		if _, found := matchedOracle[row.ID]; !found {
			metric.FalseNegative = append(metric.FalseNegative, row.ID)
		}
	}
	blindCanonicalMetric(&metric)
	sort.Strings(reasons.Mismatches)
	return metric, reasons
}

func blindDecoyAdmissions(predictions []blindCandidateInstance, exclusions []blindOracleExclusion) []string {
	anchors := make(map[string]struct{}, len(exclusions))
	for _, exclusion := range exclusions {
		anchors[exclusion.Anchor.Key()] = struct{}{}
	}
	result := []string{}
	for _, prediction := range predictions {
		anchor, found := blindCandidateRoleAnchor(prediction, H1RoleLocalWrapper)
		if !found {
			continue
		}
		if _, decoy := anchors[anchor.Key()]; decoy {
			result = append(result, prediction.ID)
		}
	}
	sort.Strings(result)
	return result
}

func blindCallbackAnchorsExact(actual H1CallbackSummary, authority Authority, expected []blindOracleCallback) bool {
	if len(actual.Closures) != len(expected) {
		return false
	}
	relations := make(map[string]blindLocator, len(authority.Program.Relations))
	for _, relation := range authority.Program.Relations {
		if relation.Location != nil {
			relations[relation.ID] = blindLocator{Path: relation.Location.Path, Line: relation.Location.Line}
		}
	}
	objects := make(map[string]blindLocator, len(authority.Program.Objects))
	for _, object := range authority.Program.Objects {
		if object.Location != nil {
			objects[object.ID] = blindLocator{Path: object.Location.Path, Line: object.Location.Line}
		}
	}
	matched := make(map[string]int, len(expected))
	for _, closure := range actual.Closures {
		candidates := make([]string, 0, 1)
		passAnchor, passFound := relations[closure.PassRelationID]
		targetAnchor, targetFound := objects[closure.TargetObjectID]
		if !passFound || !targetFound {
			return false
		}
		for _, callback := range expected {
			if passAnchor == callback.PassAnchor && targetAnchor == callback.TargetAnchor {
				candidates = append(candidates, callback.ID)
			}
		}
		if len(candidates) != 1 {
			return false
		}
		matched[candidates[0]]++
	}
	for _, callback := range expected {
		if matched[callback.ID] != 1 {
			return false
		}
	}
	return true
}

func blindMatchedIDs(matches []blindMatch) (map[string]struct{}, map[string]struct{}) {
	candidate := make(map[string]struct{}, len(matches))
	oracle := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		candidate[match.CandidateID] = struct{}{}
		oracle[match.OracleID] = struct{}{}
	}
	return candidate, oracle
}

func blindCandidateInstancesByID(values []blindCandidateInstance) map[string]blindCandidateInstance {
	result := make(map[string]blindCandidateInstance, len(values))
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}

func blindOracleInstancesByID(values []blindOracleInstance) map[string]blindOracleInstance {
	result := make(map[string]blindOracleInstance, len(values))
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}

func blindCanonicalMetric(value *blindSetMetric) {
	sort.Strings(value.TruePositive)
	sort.Strings(value.FalsePositive)
	sort.Strings(value.FalseNegative)
	value.Matched = len(value.TruePositive)
}

func finalizeBlindScorecard(value blindScorecard) blindScorecard {
	value.Gates = blindScorecardGates(value)
	value.Verdict = EvaluationPass
	for _, gate := range value.Gates {
		if !gate.Passed {
			value.Verdict = EvaluationFail
			break
		}
	}
	return value
}

func blindScorecardGates(value blindScorecard) []blindGateResult {
	return []blindGateResult{
		{ID: "callback_accounting", Passed: value.Callbacks.Exact},
		{ID: "candidate_determinism", Passed: value.CandidateDeterministic},
		{ID: "completeness", Passed: value.Completeness.Correct == value.Completeness.Total},
		{ID: "critical_false_positives", Passed: value.CriticalFalsePositives == 0},
		{ID: "decoy_admission", Passed: len(value.DecoyAdmissions) == 0},
		{ID: "evaluation_determinism", Passed: value.EvaluationDeterministic},
		{ID: "evidence_grounding_precision", Passed: value.EvidenceGrounding.Grounded == value.EvidenceGrounding.Emitted},
		{ID: "exclusion_reason_precision", Passed: value.ExclusionReasons.Correct == value.Exclusions.Predicted},
		{ID: "exclusion_recall", Passed: value.Exclusions.Matched*4 >= value.Exclusions.Truth*3},
		{ID: "frequency_reducer_excluded", Passed: value.FrequencyReducer == "IGNORED_REJECTED"},
		{ID: "instance_precision", Passed: value.Instances.Matched == value.Instances.Predicted},
		{ID: "instance_recall", Passed: value.Instances.Matched*4 >= value.Instances.Truth*3},
		{ID: "ledger_accounting", Passed: value.Ledger.Exact},
		{ID: "missing_sets", Passed: value.MissingSets.Correct == value.MissingSets.Total},
		{ID: "oracle_anchor_recall", Passed: value.OracleAnchors.Matched*5 >= value.OracleAnchors.Truth*4},
		{ID: "role_precision", Passed: value.Roles.Matched == value.Roles.Predicted},
		{ID: "role_recall", Passed: value.Roles.Matched*5 >= value.Roles.Truth*4},
		{ID: "source_accounting", Passed: value.SourceAccountingExact},
		{ID: "task_behaviour_coverage", Passed: value.TaskBehaviours.Matched*5 >= value.TaskBehaviours.Truth*4},
	}
}

func (value blindScorecard) Validate(prereg blindPreregistration) error {
	if value.Version != blindScoreVersion || !validSHA256(value.FrozenH1SHA256) ||
		!validSHA256(value.FrozenReceiptSHA256) || !validSHA256(value.AuthoritySHA256) ||
		!validSHA256(value.H0SHA256) || !validSHA256(value.CandidateH1SHA256) ||
		value.FrozenH1SHA256 != prereg.FrozenExtractor.H1SHA256 ||
		value.FrozenReceiptSHA256 != prereg.FrozenExtractor.ReceiptSHA256 ||
		value.InstanceMatches == nil || value.ExclusionMatches == nil || value.DecoyAdmissions == nil || value.Gates == nil {
		return fmt.Errorf("blind scorecard: invalid identity")
	}
	for label, metric := range map[string]blindSetMetric{
		"instances": value.Instances, "roles": value.Roles, "task behaviours": value.TaskBehaviours,
		"oracle anchors": value.OracleAnchors, "exclusions": value.Exclusions,
	} {
		if err := metric.Validate(); err != nil {
			return fmt.Errorf("blind scorecard: %s: %w", label, err)
		}
	}
	for label, metric := range map[string]blindExactMetric{
		"completeness": value.Completeness, "missing sets": value.MissingSets,
		"exclusion reasons": value.ExclusionReasons,
	} {
		if err := metric.Validate(); err != nil {
			return fmt.Errorf("blind scorecard: %s: %w", label, err)
		}
	}
	if value.EvidenceGrounding.Emitted < 0 || value.EvidenceGrounding.Grounded < 0 ||
		value.EvidenceGrounding.Grounded > value.EvidenceGrounding.Emitted || value.EvidenceGrounding.Ungrounded == nil ||
		value.EvidenceGrounding.Grounded+len(value.EvidenceGrounding.Ungrounded) != value.EvidenceGrounding.Emitted ||
		!sort.StringsAreSorted(value.EvidenceGrounding.Ungrounded) {
		return fmt.Errorf("blind scorecard: invalid grounding accounting")
	}
	if !canonicalBlindMatches(value.InstanceMatches) || !canonicalBlindMatches(value.ExclusionMatches) ||
		!sort.StringsAreSorted(value.DecoyAdmissions) || value.CriticalFalsePositives != len(value.Instances.FalsePositive) ||
		value.Callbacks.Expected.Closed+value.Callbacks.Expected.Frontier != value.Callbacks.Expected.Observed ||
		value.Callbacks.Actual.Closed+value.Callbacks.Actual.Frontier != value.Callbacks.Actual.Observed ||
		value.Ledger.Expected.Observed != value.Ledger.Expected.Admitted+value.Ledger.Expected.Excluded ||
		value.Ledger.Actual.Observed != value.Ledger.Actual.Admitted+value.Ledger.Actual.Excluded {
		return fmt.Errorf("blind scorecard: invalid matches, callbacks, or ledger accounting")
	}
	copy := value
	copy.Gates = nil
	copy.Verdict = ""
	want := finalizeBlindScorecard(copy)
	if !reflect.DeepEqual(value.Gates, want.Gates) || value.Verdict != want.Verdict {
		return fmt.Errorf("blind scorecard: verdict is not derived from every preregistered gate")
	}
	if !sort.SliceIsSorted(value.Gates, func(i, j int) bool { return value.Gates[i].ID < value.Gates[j].ID }) {
		return fmt.Errorf("blind scorecard: gates are not canonical")
	}
	return nil
}

func (value blindSetMetric) Validate() error {
	if value.Truth < 0 || value.Predicted < 0 || value.Matched < 0 || value.TruePositive == nil ||
		value.FalsePositive == nil || value.FalseNegative == nil || value.Matched != len(value.TruePositive) ||
		value.Truth != value.Matched+len(value.FalseNegative) || value.Predicted != value.Matched+len(value.FalsePositive) ||
		!sort.StringsAreSorted(value.TruePositive) || !sort.StringsAreSorted(value.FalsePositive) ||
		!sort.StringsAreSorted(value.FalseNegative) {
		return fmt.Errorf("invalid exhaustive set metric")
	}
	return nil
}

func (value blindExactMetric) Validate() error {
	if value.Correct < 0 || value.Total < 0 || value.Correct > value.Total || value.Mismatches == nil ||
		value.Correct+len(value.Mismatches) != value.Total || !sort.StringsAreSorted(value.Mismatches) {
		return fmt.Errorf("invalid exhaustive exact metric")
	}
	return nil
}

func canonicalBlindMatches(values []blindMatch) bool {
	seenOracle := make(map[string]struct{}, len(values))
	seenCandidate := make(map[string]struct{}, len(values))
	previous := ""
	for _, value := range values {
		key := value.OracleID + "\x00" + value.CandidateID
		if value.OracleID == "" || value.CandidateID == "" || (previous != "" && previous >= key) {
			return false
		}
		if _, duplicate := seenOracle[value.OracleID]; duplicate {
			return false
		}
		if _, duplicate := seenCandidate[value.CandidateID]; duplicate {
			return false
		}
		seenOracle[value.OracleID] = struct{}{}
		seenCandidate[value.CandidateID] = struct{}{}
		previous = key
	}
	return true
}

func encodeBlindScorecard(value blindScorecard) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("blind scorecard: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func syntheticBlindOracle() blindOracle {
	oracle := blindOracle{
		Version: blindOracleVersion, AuthorBriefSHA256: strings.Repeat("a", 64),
		SealedFixtureCommit: strings.Repeat("b", 40), Instances: []blindOracleInstance{},
		Excluded: []blindOracleExclusion{}, Callbacks: []blindOracleCallback{},
	}
	for index := 1; index <= 5; index++ {
		roles := make([]blindOracleRole, 0, len(h1Roles))
		for roleIndex, role := range h1Roles {
			roles = append(roles, blindOracleRole{Role: role, Evidence: []blindLocator{{Path: fmt.Sprintf("p%d/source.go", index), Line: roleIndex + 1}}})
		}
		sort.Slice(roles, func(i, j int) bool { return roles[i].Role < roles[j].Role })
		var wrapperAnchor, taskBehaviour blindLocator
		for _, role := range roles {
			switch role.Role {
			case H1RoleLocalWrapper:
				wrapperAnchor = role.Evidence[0]
			case H1RoleProductionOperation:
				taskBehaviour = role.Evidence[0]
			}
		}
		oracle.Instances = append(oracle.Instances, blindOracleInstance{
			ID: fmt.Sprintf("i%d", index), Label: fmt.Sprintf("Boundary %d", index),
			ExternalSystem: fmt.Sprintf("System %d", index), Complete: true,
			WrapperAnchor:  wrapperAnchor,
			TaskBehaviours: []blindLocator{taskBehaviour},
			Roles:          roles, Missing: []H1Role{},
		})
	}
	for index := 1; index <= 6; index++ {
		locator := blindLocator{Path: fmt.Sprintf("q%d/decoy.go", index), Line: 1}
		oracle.Excluded = append(oracle.Excluded, blindOracleExclusion{
			ID: fmt.Sprintf("x%d", index), Kind: "lookalike", Reason: H1ExcludedNotProductionReachable,
			Anchor: locator,
		})
	}
	return oracle
}

func syntheticBlindCandidate(oracle blindOracle) blindCandidateView {
	result := blindCandidateView{Instances: []blindCandidateInstance{}, Excluded: []blindCandidateExclusion{}}
	for _, instance := range oracle.Instances {
		roles := make([]blindCandidateRole, 0, len(instance.Roles))
		for _, role := range instance.Roles {
			roles = append(roles, blindCandidateRole{Role: role.Role, Evidence: append([]blindLocator{}, role.Evidence...)})
		}
		result.Instances = append(result.Instances, blindCandidateInstance{
			ID: "candidate-" + instance.ID, Complete: instance.Complete, VerificationKind: "unit_test",
			Roles: roles, Missing: append([]H1Role{}, instance.Missing...),
		})
		result.ReachableAnchors = append(result.ReachableAnchors, instance.TaskBehaviours...)
	}
	result.ReachableAnchors = canonicalizeBlindLocators(result.ReachableAnchors)
	for _, excluded := range oracle.Excluded {
		result.Excluded = append(result.Excluded, blindCandidateExclusion{
			ID: "candidate-" + excluded.ID, Reason: excluded.Reason, Evidence: []blindLocator{excluded.Anchor},
		})
	}
	result.Ledger = H1Ledger{Observed: len(result.Instances) + len(result.Excluded), Admitted: len(result.Instances), Excluded: len(result.Excluded)}
	return result
}

func blindBytesSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
