package clientrecipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/programindex"
)

const H1Version = 1

type H1Role string

const (
	H1RoleConfiguration       H1Role = "configuration"
	H1RoleConstruction        H1Role = "construction"
	H1RoleLocalWrapper        H1Role = "local_wrapper"
	H1RoleConsumerBoundary    H1Role = "consumer_boundary"
	H1RoleApplicationWiring   H1Role = "application_wiring"
	H1RoleProductionOperation H1Role = "production_operation"
	H1RoleVerification        H1Role = "verification"
	H1RoleObservability       H1Role = "observability"
	H1RoleFailurePolicy       H1Role = "failure_policy"
)

var h1Roles = []H1Role{
	H1RoleConfiguration,
	H1RoleConstruction,
	H1RoleLocalWrapper,
	H1RoleConsumerBoundary,
	H1RoleApplicationWiring,
	H1RoleProductionOperation,
	H1RoleVerification,
	H1RoleObservability,
	H1RoleFailurePolicy,
}

var h1MandatoryRoles = map[H1Role]struct{}{
	H1RoleConfiguration: {}, H1RoleConstruction: {}, H1RoleLocalWrapper: {},
	H1RoleConsumerBoundary: {}, H1RoleApplicationWiring: {}, H1RoleProductionOperation: {},
	H1RoleVerification: {}, H1RoleObservability: {},
}

type H1Necessity string

const (
	H1Required H1Necessity = "required"
	H1Common   H1Necessity = "common"
	H1Optional H1Necessity = "optional"
)

type H1ExclusionReason string

const (
	H1ExcludedGenerated              H1ExclusionReason = "generated"
	H1ExcludedTestOnly               H1ExclusionReason = "test_only"
	H1ExcludedNotProductionReachable H1ExclusionReason = "not_production_reachable"
	H1ExcludedNotExternalBoundary    H1ExclusionReason = "not_external_boundary"
)

type H1Evidence struct {
	Path         string `json:"path"`
	SourceSHA256 string `json:"source_sha256"`
	Line         int    `json:"line"`
	Column       int    `json:"column"`
	Symbol       string `json:"symbol"`
	AuthorityID  string `json:"authority_id,omitempty"`
}

type H1RoleEvidence struct {
	Role     H1Role       `json:"role"`
	Evidence []H1Evidence `json:"evidence"`
}

type H1Instance struct {
	ID                     string           `json:"id"`
	H0CandidateID          string           `json:"h0_candidate_id"`
	DependencyID           string           `json:"dependency_id"`
	ImporterRef            string           `json:"importer_ref"`
	PackagePath            string           `json:"package_path"`
	ImporterPackagePath    string           `json:"importer_package_path"`
	ImporterRepositoryPath string           `json:"importer_repository_path"`
	WrapperType            string           `json:"wrapper_type"`
	WrapperObjectID        string           `json:"wrapper_object_id"`
	VerificationKind       string           `json:"verification_kind"`
	Complete               bool             `json:"complete"`
	Roles                  []H1RoleEvidence `json:"roles"`
	Missing                []H1Role         `json:"missing"`
}

type H1Excluded struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	OriginID string            `json:"origin_id"`
	Reason   H1ExclusionReason `json:"reason"`
	Evidence []H1Evidence      `json:"evidence"`
}

type H1CallbackClosure struct {
	ID                   string       `json:"id"`
	Kind                 string       `json:"kind"`
	PassRelationID       string       `json:"pass_relation_id"`
	UnresolvedRelationID string       `json:"unresolved_relation_id"`
	TargetObjectID       string       `json:"target_object_id"`
	Evidence             []H1Evidence `json:"evidence"`
}

type H1CallbackSummary struct {
	Observed int                 `json:"observed"`
	Closed   int                 `json:"closed"`
	Frontier int                 `json:"frontier"`
	Closures []H1CallbackClosure `json:"closures"`
}

type H1Reachability struct {
	SeedObjectIDs    []string `json:"seed_object_ids"`
	ReachedObjectIDs []string `json:"reached_object_ids"`
	ExactRelationIDs []string `json:"exact_relation_ids"`
}

type H1RoleFrequency struct {
	Role              H1Role      `json:"role"`
	CompleteInstances int         `json:"complete_instances"`
	Necessity         H1Necessity `json:"necessity"`
}

type H1Ledger struct {
	Observed int `json:"observed"`
	Admitted int `json:"admitted"`
	Excluded int `json:"excluded"`
}

type H1ObservedUniverse struct {
	H0Candidates        int `json:"h0_candidates"`
	GeneratedH0Groups   int `json:"generated_h0_groups"`
	QualifyingTestFakes int `json:"qualifying_test_fakes"`
	ProseCandidates     int `json:"prose_candidates"`
	StdlibHelpers       int `json:"stdlib_helpers"`
}

type H1Result struct {
	Version          int                `json:"version"`
	AuthoritySHA256  string             `json:"authority_sha256"`
	H0SHA256         string             `json:"h0_sha256"`
	Instances        []H1Instance       `json:"instances"`
	Excluded         []H1Excluded       `json:"excluded"`
	Roles            []H1RoleFrequency  `json:"roles"`
	Callbacks        H1CallbackSummary  `json:"callbacks"`
	Reachability     H1Reachability     `json:"reachability"`
	ObservedUniverse H1ObservedUniverse `json:"observed_universe"`
	Ledger           H1Ledger           `json:"ledger"`
	SHA256           string             `json:"sha256"`
}

func EncodeH1(value H1Result) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("client recipe H1: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodeH1(raw []byte) (H1Result, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value H1Result
	if err := decoder.Decode(&value); err != nil {
		return H1Result{}, fmt.Errorf("client recipe H1: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return H1Result{}, fmt.Errorf("client recipe H1: trailing data")
	}
	if err := value.Validate(); err != nil {
		return H1Result{}, err
	}
	canonical, err := EncodeH1(value)
	if err != nil {
		return H1Result{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return H1Result{}, fmt.Errorf("client recipe H1: non-canonical bytes")
	}
	return value, nil
}

func (value H1Result) Validate() error {
	if value.Version != H1Version || !validSHA256(value.AuthoritySHA256) || !validSHA256(value.H0SHA256) || !validSHA256(value.SHA256) ||
		value.Instances == nil || value.Excluded == nil || value.Roles == nil ||
		value.Callbacks.Closures == nil || value.Reachability.SeedObjectIDs == nil ||
		value.Reachability.ReachedObjectIDs == nil || value.Reachability.ExactRelationIDs == nil {
		return fmt.Errorf("client recipe H1: invalid identity")
	}
	if value.Ledger.Observed != len(value.Instances)+len(value.Excluded) ||
		value.Ledger.Admitted != len(value.Instances) || value.Ledger.Excluded != len(value.Excluded) {
		return fmt.Errorf("client recipe H1: ledger mismatch")
	}
	if value.ObservedUniverse.H0Candidates < 0 || value.ObservedUniverse.GeneratedH0Groups < 0 ||
		value.ObservedUniverse.QualifyingTestFakes < 0 || value.ObservedUniverse.ProseCandidates < 0 ||
		value.ObservedUniverse.StdlibHelpers < 0 ||
		value.ObservedUniverse.GeneratedH0Groups > value.ObservedUniverse.H0Candidates ||
		value.Ledger.Observed != value.ObservedUniverse.H0Candidates+value.ObservedUniverse.QualifyingTestFakes+
			value.ObservedUniverse.ProseCandidates+value.ObservedUniverse.StdlibHelpers {
		return fmt.Errorf("client recipe H1: observed universe mismatch")
	}
	previous := ""
	completeCount := 0
	roleCounts := make(map[H1Role]int, len(h1Roles))
	seenIDs := make(map[string]struct{}, value.Ledger.Observed)
	for _, instance := range value.Instances {
		if !validText(instance.ID) || instance.ID != h1InstanceID(instance.H0CandidateID, instance.WrapperObjectID) ||
			!validText(instance.H0CandidateID) || !validText(instance.DependencyID) || !validText(instance.ImporterRef) ||
			!validText(instance.PackagePath) || !validText(instance.ImporterPackagePath) ||
			!validSourcePath(instance.ImporterRepositoryPath) || !validText(instance.WrapperType) ||
			!validText(instance.WrapperObjectID) || !validH1VerificationKind(instance.VerificationKind) ||
			instance.Roles == nil || instance.Missing == nil ||
			(previous != "" && previous >= instance.ID) {
			return fmt.Errorf("client recipe H1: invalid or non-canonical instance")
		}
		previous = instance.ID
		seenIDs[instance.ID] = struct{}{}
		present := make(map[H1Role]struct{}, len(instance.Roles))
		lastRole := -1
		for _, role := range instance.Roles {
			index := h1RoleIndex(role.Role)
			if index < 0 || index <= lastRole || len(role.Evidence) == 0 || !canonicalH1Evidence(role.Evidence) {
				return fmt.Errorf("client recipe H1: invalid role evidence")
			}
			lastRole = index
			present[role.Role] = struct{}{}
		}
		wantComplete := h1HasMandatoryRoles(present)
		_, hasVerification := present[H1RoleVerification]
		if hasVerification != (instance.VerificationKind != "none") {
			return fmt.Errorf("client recipe H1: verification kind mismatch")
		}
		if instance.Complete != wantComplete {
			return fmt.Errorf("client recipe H1: instance completeness mismatch")
		}
		wantMissing := h1MissingRoles(present, wantComplete)
		if !reflect.DeepEqual(instance.Missing, wantMissing) {
			return fmt.Errorf("client recipe H1: instance missing-role mismatch")
		}
		if instance.Complete {
			completeCount++
			for role := range present {
				roleCounts[role]++
			}
		}
	}
	previous = ""
	wantUniverse := H1ObservedUniverse{H0Candidates: len(value.Instances)}
	for _, excluded := range value.Excluded {
		if !validText(excluded.ID) || excluded.ID != h1ExcludedID(excluded.Kind, excluded.OriginID) ||
			!validText(excluded.Kind) || !validText(excluded.OriginID) || !excluded.Reason.Valid() ||
			len(excluded.Evidence) == 0 || !canonicalH1Evidence(excluded.Evidence) ||
			(previous != "" && previous >= excluded.ID) {
			return fmt.Errorf("client recipe H1: invalid or non-canonical exclusion")
		}
		if _, duplicate := seenIDs[excluded.ID]; duplicate {
			return fmt.Errorf("client recipe H1: duplicate ledger identity")
		}
		seenIDs[excluded.ID] = struct{}{}
		previous = excluded.ID
		switch excluded.Kind {
		case "external_dependency":
			wantUniverse.H0Candidates++
			if excluded.Reason == H1ExcludedGenerated {
				wantUniverse.GeneratedH0Groups++
			}
		case "test_type":
			wantUniverse.QualifyingTestFakes++
		case "prose":
			wantUniverse.ProseCandidates++
		case "stdlib_helper":
			wantUniverse.StdlibHelpers++
		default:
			return fmt.Errorf("client recipe H1: unknown observed-universe kind")
		}
	}
	if value.ObservedUniverse != wantUniverse {
		return fmt.Errorf("client recipe H1: observed-universe projection mismatch")
	}
	if len(value.Roles) != len(h1Roles) || completeCount == 0 {
		return fmt.Errorf("client recipe H1: invalid role reduction")
	}
	for index, role := range value.Roles {
		if role.Role != h1Roles[index] || role.CompleteInstances != roleCounts[role.Role] ||
			role.Necessity != h1Necessity(role.CompleteInstances, completeCount) {
			return fmt.Errorf("client recipe H1: role reduction mismatch")
		}
	}
	if value.Callbacks.Observed < 0 || value.Callbacks.Closed != len(value.Callbacks.Closures) ||
		value.Callbacks.Frontier != value.Callbacks.Observed-value.Callbacks.Closed || value.Callbacks.Frontier < 0 {
		return fmt.Errorf("client recipe H1: callback accounting mismatch")
	}
	previous = ""
	for _, closure := range value.Callbacks.Closures {
		if !validText(closure.ID) || closure.ID != h1CallbackID(closure.PassRelationID, closure.UnresolvedRelationID, closure.TargetObjectID) ||
			(closure.Kind != "parameter_invoke" && closure.Kind != "stored_field_invoke") ||
			!validText(closure.PassRelationID) || !validText(closure.UnresolvedRelationID) ||
			!validText(closure.TargetObjectID) || len(closure.Evidence) < 2 ||
			!canonicalH1Evidence(closure.Evidence) || (previous != "" && previous >= closure.ID) {
			return fmt.Errorf("client recipe H1: invalid callback closure")
		}
		previous = closure.ID
	}
	if !sortedUnique(value.Reachability.SeedObjectIDs) || !sortedUnique(value.Reachability.ReachedObjectIDs) ||
		!sortedUnique(value.Reachability.ExactRelationIDs) {
		return fmt.Errorf("client recipe H1: reachability is not canonical")
	}
	if value.SHA256 != h1Digest(value) {
		return fmt.Errorf("client recipe H1: sha256 mismatch")
	}
	return nil
}

func (value H1Result) ValidateAgainst(authority Authority) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if err := authority.Validate(); err != nil {
		return err
	}
	if value.AuthoritySHA256 != authority.SHA256 {
		return fmt.Errorf("client recipe H1: authority binding mismatch")
	}
	if value.Callbacks.Observed != authority.Callbacks.UnresolvedInvocations {
		return fmt.Errorf("client recipe H1: callback observation binding mismatch")
	}
	baseline, err := BuildH0(authority)
	if err != nil {
		return err
	}
	if value.H0SHA256 != baseline.SHA256 {
		return fmt.Errorf("client recipe H1: H0 binding mismatch")
	}
	h0Candidates := make(map[string]H0Candidate, len(baseline.Candidates))
	for _, candidate := range baseline.Candidates {
		h0Candidates[candidate.ID] = candidate
	}
	h0Dispositions := make(map[string]struct{}, len(baseline.Candidates))
	sources := make(map[string]SourceFact, len(authority.Sources))
	for _, source := range authority.Sources {
		sources[source.Path] = source
	}
	objects := make(map[string]programindex.Object, len(authority.Program.Objects))
	for _, object := range authority.Program.Objects {
		objects[object.ID] = object
	}
	relations := make(map[string]programindex.Relation, len(authority.Program.Relations))
	for _, relation := range authority.Program.Relations {
		relations[relation.ID] = relation
	}
	checkEvidence := func(values []H1Evidence) error {
		for _, evidence := range values {
			source, found := sources[evidence.Path]
			if !found || source.SHA256 != evidence.SourceSHA256 {
				return fmt.Errorf("client recipe H1: evidence source binding mismatch")
			}
			if evidence.AuthorityID == "" {
				continue
			}
			if object, found := objects[evidence.AuthorityID]; found {
				if object.Location == nil || object.Location.Path != evidence.Path || object.Location.Line != evidence.Line {
					return fmt.Errorf("client recipe H1: evidence object locator mismatch")
				}
				continue
			}
			if relation, found := relations[evidence.AuthorityID]; found {
				if relation.Location == nil || relation.Location.Path != evidence.Path || relation.Location.Line != evidence.Line {
					return fmt.Errorf("client recipe H1: evidence relation locator mismatch")
				}
				continue
			}
			return fmt.Errorf("client recipe H1: evidence cites unknown authority")
		}
		return nil
	}
	for _, instance := range value.Instances {
		candidate, found := h0Candidates[instance.H0CandidateID]
		if !found || candidate.DependencyID != instance.DependencyID || candidate.ImporterRef != instance.ImporterRef ||
			candidate.PackagePath != instance.PackagePath || candidate.ImporterPackagePath != instance.ImporterPackagePath ||
			candidate.ImporterRepositoryPath != instance.ImporterRepositoryPath {
			return fmt.Errorf("client recipe H1: instance H0 origin mismatch")
		}
		if _, duplicate := h0Dispositions[candidate.ID]; duplicate {
			return fmt.Errorf("client recipe H1: duplicate H0 disposition")
		}
		h0Dispositions[candidate.ID] = struct{}{}
		wrapper, found := objects[instance.WrapperObjectID]
		if !found || wrapper.Kind != programindex.ObjectType || wrapper.Location == nil ||
			wrapper.Location.Path == "" || wrapper.Name != instance.WrapperType {
			return fmt.Errorf("client recipe H1: wrapper authority mismatch")
		}
		for _, role := range instance.Roles {
			if err := checkEvidence(role.Evidence); err != nil {
				return err
			}
		}
	}
	for _, excluded := range value.Excluded {
		if excluded.Kind == "external_dependency" {
			if _, found := h0Candidates[excluded.OriginID]; !found {
				return fmt.Errorf("client recipe H1: exclusion H0 origin mismatch")
			}
			if _, duplicate := h0Dispositions[excluded.OriginID]; duplicate {
				return fmt.Errorf("client recipe H1: duplicate H0 disposition")
			}
			h0Dispositions[excluded.OriginID] = struct{}{}
		}
		if err := checkEvidence(excluded.Evidence); err != nil {
			return err
		}
	}
	if len(h0Dispositions) != len(h0Candidates) {
		return fmt.Errorf("client recipe H1: H0 candidate accounting is incomplete")
	}
	for _, closure := range value.Callbacks.Closures {
		pass, passFound := relations[closure.PassRelationID]
		unresolved, unresolvedFound := relations[closure.UnresolvedRelationID]
		if !passFound || pass.Kind != programindex.RelationPassesCallback || pass.Resolution != programindex.ResolutionExact ||
			len(pass.ToIDs) != 1 || pass.ToIDs[0] != closure.TargetObjectID || !unresolvedFound ||
			unresolved.Kind != programindex.RelationCalls || unresolved.Resolution != programindex.ResolutionUnresolved {
			return fmt.Errorf("client recipe H1: callback closure authority mismatch")
		}
		if err := checkEvidence(closure.Evidence); err != nil {
			return err
		}
	}
	seedSet := make(map[string]struct{}, len(authority.Program.Target.Seeds))
	for _, seed := range authority.Program.Target.Seeds {
		seedSet[seed.ObjectID] = struct{}{}
	}
	if !sameStringSet(value.Reachability.SeedObjectIDs, seedSet) {
		return fmt.Errorf("client recipe H1: reachability seed mismatch")
	}
	for _, id := range value.Reachability.ReachedObjectIDs {
		if _, found := objects[id]; !found {
			return fmt.Errorf("client recipe H1: reachability cites unknown object")
		}
	}
	for _, id := range value.Reachability.ExactRelationIDs {
		relation, found := relations[id]
		if !found || relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact {
			return fmt.Errorf("client recipe H1: reachability cites unsupported relation")
		}
	}
	return nil
}

// ValidateAgainstRepository re-runs the source-bound projection and compares
// canonical bytes. This is the completeness check for the AST-observed rows
// that cannot be reconstructed from Authority alone.
func (value H1Result) ValidateAgainstRepository(repoRoot string, authority Authority) error {
	if err := value.ValidateAgainst(authority); err != nil {
		return err
	}
	want, err := ExtractH1(repoRoot, authority)
	if err != nil {
		return err
	}
	actualRaw, err := EncodeH1(value)
	if err != nil {
		return err
	}
	wantRaw, err := EncodeH1(want)
	if err != nil {
		return err
	}
	if !bytes.Equal(actualRaw, wantRaw) {
		return fmt.Errorf("client recipe H1: repository-bound projection mismatch")
	}
	return nil
}

func sealH1(value H1Result) (H1Result, error) {
	value.SHA256 = ""
	value.SHA256 = h1Digest(value)
	if err := value.Validate(); err != nil {
		return H1Result{}, err
	}
	return value, nil
}

func h1Digest(value H1Result) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func h1InstanceID(h0ID, wrapperID string) string {
	digest := sha256.Sum256([]byte("clientrecipe-h1-instance-v1\x00" + h0ID + "\x00" + wrapperID))
	return "h1-instance-" + hex.EncodeToString(digest[:12])
}

func h1ExcludedID(kind, originID string) string {
	digest := sha256.Sum256([]byte("clientrecipe-h1-excluded-v1\x00" + kind + "\x00" + originID))
	return "h1-excluded-" + hex.EncodeToString(digest[:12])
}

func h1CallbackID(passID, unresolvedID, targetID string) string {
	digest := sha256.Sum256([]byte("clientrecipe-h1-callback-v1\x00" + passID + "\x00" + unresolvedID + "\x00" + targetID))
	return "h1-callback-" + hex.EncodeToString(digest[:12])
}

func (reason H1ExclusionReason) Valid() bool {
	switch reason {
	case H1ExcludedGenerated, H1ExcludedTestOnly, H1ExcludedNotProductionReachable, H1ExcludedNotExternalBoundary:
		return true
	default:
		return false
	}
}

func h1RoleIndex(role H1Role) int {
	for index, candidate := range h1Roles {
		if role == candidate {
			return index
		}
	}
	return -1
}

func h1HasMandatoryRoles(present map[H1Role]struct{}) bool {
	for role := range h1MandatoryRoles {
		if _, found := present[role]; !found {
			return false
		}
	}
	return true
}

func h1MissingRoles(present map[H1Role]struct{}, complete bool) []H1Role {
	if complete {
		return []H1Role{}
	}
	result := make([]H1Role, 0)
	for _, role := range h1Roles {
		if _, found := present[role]; !found {
			result = append(result, role)
		}
	}
	return result
}

func h1Necessity(observed, complete int) H1Necessity {
	switch {
	case observed == complete:
		return H1Required
	case observed >= 2 && observed*3 >= complete*2:
		return H1Common
	default:
		return H1Optional
	}
}

func canonicalH1Evidence(values []H1Evidence) bool {
	previous := ""
	for _, evidence := range values {
		if !validSourcePath(evidence.Path) || !validSHA256(evidence.SourceSHA256) || evidence.Line <= 0 ||
			evidence.Column <= 0 || !validText(evidence.Symbol) || !validOptionalH1Text(evidence.AuthorityID) {
			return false
		}
		key := h1EvidenceKey(evidence)
		if previous != "" && previous >= key {
			return false
		}
		previous = key
	}
	return true
}

func canonicalizeH1Evidence(values []H1Evidence) []H1Evidence {
	result := append([]H1Evidence(nil), values...)
	sort.Slice(result, func(i, j int) bool { return h1EvidenceKey(result[i]) < h1EvidenceKey(result[j]) })
	if len(result) < 2 {
		return result
	}
	compacted := result[:1]
	for _, value := range result[1:] {
		if h1EvidenceKey(compacted[len(compacted)-1]) != h1EvidenceKey(value) {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

func h1EvidenceKey(value H1Evidence) string {
	return fmt.Sprintf("%s\x00%09d\x00%09d\x00%s\x00%s", value.Path, value.Line, value.Column, value.Symbol, value.AuthorityID)
}

func validOptionalH1Text(value string) bool { return value == "" || validText(value) }

func validH1VerificationKind(value string) bool {
	return value == "none" || value == "unit_test" || value == "integration_test"
}

func sameStringSet(values []string, want map[string]struct{}) bool {
	if len(values) != len(want) {
		return false
	}
	for _, value := range values {
		if _, found := want[value]; !found {
			return false
		}
	}
	return true
}
