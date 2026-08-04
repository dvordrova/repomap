package semanticdiscovery

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	MechanismVersion          = 1
	MechanismPayloadVersion   = 1
	MechanismInputVersion     = 1
	MechanismValidatorVersion = "semantic-mechanism-v1"
	MechanismFile             = "mechanism_v1.json"

	maxMechanismBytes = 512 << 10
)

type MechanismScopeKind string

const MechanismScopeGoPackage MechanismScopeKind = "go_package"

type MechanismScope struct {
	Kind  MechanismScopeKind `json:"kind"`
	Value string             `json:"value"`
}

// MechanismIdentity is owner-assigned local identity. Model wording,
// candidate IDs, repository revisions, and report component IDs do not
// participate in it.
type MechanismIdentity struct {
	RepositoryNamespace string         `json:"repository_namespace"`
	IntentKey           string         `json:"intent_key"`
	Scope               MechanismScope `json:"scope"`
}

type MechanismProbeInput struct {
	ContractVersion int    `json:"contract_version"`
	ID              string `json:"id"`
	SHA256          string `json:"sha256"`
}

type MechanismFactRole string

const (
	MechanismFactClaimSupport    MechanismFactRole = "claim_support"
	MechanismFactAvailableUnused MechanismFactRole = "available_unused"
	MechanismFactCandidateSeed   MechanismFactRole = "candidate_seed"
)

type MechanismFactInput struct {
	ID     string            `json:"id"`
	SHA256 string            `json:"sha256"`
	Role   MechanismFactRole `json:"role"`
}

// MechanismInputManifest binds only the candidate-scoped compatibility
// inputs. In particular it contains no whole-bundle hash, PlannerContext,
// repository revision, or report-derived component focus.
type MechanismInputManifest struct {
	Version          int                  `json:"version"`
	ValidatorVersion string               `json:"validator_version"`
	Probe            MechanismProbeInput  `json:"probe"`
	ContractSHA256   string               `json:"contract_sha256"`
	Facts            []MechanismFactInput `json:"facts"`
}

type MechanismOrderingBasis string

const MechanismOrderingEditorial MechanismOrderingBasis = "editorial"

// MechanismPayload preserves the smallest existing semantic authority needed
// to re-run the current validators: one candidate, one locally verified leaf,
// and one fan-in proposal with its claim-to-observation lineage.
type MechanismPayload struct {
	Version       int                    `json:"version"`
	OrderingBasis MechanismOrderingBasis `json:"ordering_basis"`
	Candidate     OpportunityCandidate   `json:"candidate"`
	Leaf          LeafArtifact           `json:"leaf"`
	Proposal      ArtifactProposal       `json:"proposal"`
}

// Mechanism is the first concrete durable semantic object. It deliberately is
// not a generic knowledge-object envelope or interface.
type Mechanism struct {
	Version       int                    `json:"version"`
	ID            string                 `json:"id"`
	ContentSHA256 string                 `json:"content_sha256"`
	Identity      MechanismIdentity      `json:"identity"`
	Input         MechanismInputManifest `json:"input"`
	Payload       MechanismPayload       `json:"payload"`
}

type mechanismContent struct {
	Version  int                    `json:"version"`
	ID       string                 `json:"id"`
	Identity MechanismIdentity      `json:"identity"`
	Input    MechanismInputManifest `json:"input"`
	Payload  MechanismPayload       `json:"payload"`
}

type mechanismSemanticContract struct {
	Kind               ArtifactKind        `json:"kind"`
	CapabilityContract *CapabilityContract `json:"capability_contract"`
	IntentContract     *IntentContract     `json:"intent_contract"`
}

// ExtractMechanism narrows one already accepted Record to a single candidate
// and proves that candidate-scoped replay materializes the exact same Artifact
// as the original full-bundle replay.
func ExtractMechanism(
	bundle Bundle,
	record Record,
	candidateID string,
	identity MechanismIdentity,
	probe MechanismProbeInput,
) (Mechanism, Artifact, error) {
	if _, err := validateRecord(bundle, record); err != nil {
		return Mechanism{}, Artifact{}, fmt.Errorf("semantic mechanism: source record is invalid: %w", err)
	}
	if err := validateMechanismIdentity(identity); err != nil {
		return Mechanism{}, Artifact{}, err
	}
	if err := validateMechanismProbe(probe); err != nil {
		return Mechanism{}, Artifact{}, err
	}
	if probe.ID != identity.IntentKey {
		return Mechanism{}, Artifact{}, fmt.Errorf("semantic mechanism: probe id does not match intent key")
	}

	candidate, leaf, proposal, err := mechanismRecordSlice(record, candidateID)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}
	if candidate.Kind != ArtifactMechanism {
		return Mechanism{}, Artifact{}, fmt.Errorf("semantic mechanism: candidate is not a mechanism")
	}
	scoped, err := mechanismScopedBundle(bundle, candidateFactIDs(candidate), identity.RepositoryNamespace)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}
	payload := canonicalMechanismPayload(MechanismPayload{
		Version:       MechanismPayloadVersion,
		OrderingBasis: MechanismOrderingEditorial,
		Candidate:     candidate,
		Leaf:          leaf.Artifact,
		Proposal:      proposal,
	})
	projected, err := projectMechanism(scoped, payload)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}

	fullArtifacts, err := MaterializePartialArtifacts(bundle, record.Leaves, record.FanIn)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}
	original, found := mechanismArtifactByCandidate(fullArtifacts, candidateID)
	if !found {
		return Mechanism{}, Artifact{}, fmt.Errorf("semantic mechanism: source artifact is unavailable")
	}
	if !reflect.DeepEqual(projected, original) {
		return Mechanism{}, Artifact{}, fmt.Errorf("semantic mechanism: scoped projection differs from source artifact")
	}

	contractSHA, err := mechanismContractSHA(candidate)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}
	factInputs, err := mechanismFactInputs(scoped.Facts, projected)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}
	mechanism := canonicalMechanism(Mechanism{
		Version:  MechanismVersion,
		Identity: identity,
		Input: MechanismInputManifest{
			Version:          MechanismInputVersion,
			ValidatorVersion: MechanismValidatorVersion,
			Probe:            probe,
			ContractSHA256:   contractSHA,
			Facts:            factInputs,
		},
		Payload: payload,
	})
	mechanism.ID, err = MechanismLogicalID(identity)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}
	mechanism.ContentSHA256, err = MechanismContentHash(mechanism)
	if err != nil {
		return Mechanism{}, Artifact{}, err
	}
	mechanism = canonicalMechanism(mechanism)
	if _, err := ReplayMechanism(bundle, probe, mechanism); err != nil {
		return Mechanism{}, Artifact{}, err
	}
	return mechanism, projected, nil
}

// ReplayMechanism ignores PlannerContext and every unbound fact. Only the
// manifest facts are hashed and passed back through the existing opportunity,
// leaf, fan-in, and Artifact materialization validators.
func ReplayMechanism(
	bundle Bundle,
	currentProbe MechanismProbeInput,
	mechanism Mechanism,
) (Artifact, error) {
	mechanism = canonicalMechanism(mechanism)
	if err := validateMechanism(mechanism); err != nil {
		return Artifact{}, err
	}
	if err := validateMechanismProbe(currentProbe); err != nil {
		return Artifact{}, err
	}
	if currentProbe != mechanism.Input.Probe {
		return Artifact{}, fmt.Errorf("semantic mechanism: bounded probe input changed")
	}
	manifestIDs := mechanismManifestFactIDs(mechanism.Input.Facts)
	scoped, err := mechanismScopedBundle(
		bundle,
		manifestIDs,
		mechanism.Identity.RepositoryNamespace,
	)
	if err != nil {
		return Artifact{}, err
	}
	bindings := make(map[string]MechanismFactInput, len(mechanism.Input.Facts))
	for _, input := range mechanism.Input.Facts {
		bindings[input.ID] = input
	}
	for _, fact := range scoped.Facts {
		sha, hashErr := mechanismFactSHA(fact)
		if hashErr != nil {
			return Artifact{}, hashErr
		}
		if sha != bindings[fact.ID].SHA256 {
			return Artifact{}, fmt.Errorf("semantic mechanism: bound fact %q changed", fact.ID)
		}
	}

	artifact, err := projectMechanism(scoped, mechanism.Payload)
	if err != nil {
		return Artifact{}, err
	}
	expectedInputs, err := mechanismFactInputs(scoped.Facts, artifact)
	if err != nil {
		return Artifact{}, err
	}
	if !reflect.DeepEqual(expectedInputs, mechanism.Input.Facts) {
		return Artifact{}, fmt.Errorf("semantic mechanism: fact usage partition changed")
	}
	return artifact, nil
}

func EncodeMechanism(mechanism Mechanism) ([]byte, error) {
	mechanism = canonicalMechanism(mechanism)
	if err := validateMechanism(mechanism); err != nil {
		return nil, err
	}
	_, encoded, err := hashJSON("semantic mechanism", mechanism)
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxMechanismBytes {
		return nil, fmt.Errorf("semantic mechanism: record is too large")
	}
	return encoded, nil
}

func DecodeMechanism(raw []byte) (Mechanism, error) {
	var mechanism Mechanism
	if err := decodeStrict(raw, &mechanism, maxMechanismBytes); err != nil {
		return Mechanism{}, fmt.Errorf("semantic mechanism: invalid record json: %w", err)
	}
	mechanism = canonicalMechanism(mechanism)
	if err := validateMechanism(mechanism); err != nil {
		return Mechanism{}, err
	}
	return mechanism, nil
}

func MechanismLogicalID(identity MechanismIdentity) (string, error) {
	if err := validateMechanismIdentity(identity); err != nil {
		return "", err
	}
	return stableID(
		"semantic-mechanism",
		identity.RepositoryNamespace,
		string(identity.Scope.Kind),
		identity.Scope.Value,
		identity.IntentKey,
	), nil
}

// MechanismContentHash covers the concrete payload and its exact manifest but
// intentionally leaves logical identity derivation independent from content.
func MechanismContentHash(mechanism Mechanism) (string, error) {
	mechanism = canonicalMechanism(mechanism)
	return mechanismContentSHA(mechanism)
}

func projectMechanism(bundle Bundle, payload MechanismPayload) (Artifact, error) {
	payload = canonicalMechanismPayload(payload)
	opportunity := OpportunityProposal{
		Version:    OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{payload.Candidate},
	}
	if err := ValidateOpportunityProposal(bundle, opportunity); err != nil {
		return Artifact{}, fmt.Errorf("semantic mechanism: scoped opportunity is invalid: %w", err)
	}
	selected, err := SelectOpportunities(bundle, opportunity, 1)
	if err != nil {
		return Artifact{}, err
	}
	tasks, err := PlanLeafTasks(bundle, selected)
	if err != nil {
		return Artifact{}, err
	}
	if len(tasks) != 1 || tasks[0].ID != payload.Leaf.TaskID {
		return Artifact{}, fmt.Errorf("semantic mechanism: scoped leaf identity changed")
	}
	leaf := LeafResult{Task: tasks[0], Artifact: payload.Leaf}
	if err := ValidateLeafArtifact(leaf.Task, leaf.Artifact); err != nil {
		return Artifact{}, fmt.Errorf("semantic mechanism: scoped leaf is invalid: %w", err)
	}
	fanIn := FanInArtifact{
		Version:   FanInArtifactVersion,
		Artifacts: []ArtifactProposal{payload.Proposal},
	}
	recordRaw, err := EncodeRecord(bundle, opportunity, selected, []LeafResult{leaf}, fanIn)
	if err != nil {
		return Artifact{}, fmt.Errorf("semantic mechanism: scoped fan-in is invalid: %w", err)
	}
	artifacts, err := ReplayRecord(bundle, recordRaw)
	if err != nil {
		return Artifact{}, err
	}
	if len(artifacts) != 1 || artifacts[0].Kind != ArtifactMechanism {
		return Artifact{}, fmt.Errorf("semantic mechanism: scoped replay did not produce one mechanism")
	}
	return artifacts[0], nil
}

func mechanismRecordSlice(
	record Record,
	candidateID string,
) (OpportunityCandidate, LeafResult, ArtifactProposal, error) {
	if err := validateOpaque("mechanism candidate id", candidateID); err != nil {
		return OpportunityCandidate{}, LeafResult{}, ArtifactProposal{}, err
	}
	selected := false
	for _, id := range record.SelectedCandidateIDs {
		if id == candidateID {
			selected = true
			break
		}
	}
	if !selected {
		return OpportunityCandidate{}, LeafResult{}, ArtifactProposal{}, fmt.Errorf(
			"semantic mechanism: candidate was not selected",
		)
	}
	var candidate OpportunityCandidate
	for _, item := range record.Opportunity.Candidates {
		if item.ID == candidateID {
			if candidate.ID != "" {
				return OpportunityCandidate{}, LeafResult{}, ArtifactProposal{}, fmt.Errorf(
					"semantic mechanism: candidate is duplicated",
				)
			}
			candidate = item
		}
	}
	var leaf LeafResult
	for _, item := range record.Leaves {
		if item.Task.Candidate.ID == candidateID {
			if leaf.Task.ID != "" {
				return OpportunityCandidate{}, LeafResult{}, ArtifactProposal{}, fmt.Errorf(
					"semantic mechanism: candidate leaf is duplicated",
				)
			}
			leaf = item
		}
	}
	var proposal ArtifactProposal
	for _, item := range record.FanIn.Artifacts {
		if item.CandidateID == candidateID {
			if proposal.CandidateID != "" {
				return OpportunityCandidate{}, LeafResult{}, ArtifactProposal{}, fmt.Errorf(
					"semantic mechanism: candidate proposal is duplicated",
				)
			}
			proposal = item
		}
	}
	if candidate.ID == "" || leaf.Task.ID == "" || proposal.CandidateID == "" {
		return OpportunityCandidate{}, LeafResult{}, ArtifactProposal{}, fmt.Errorf(
			"semantic mechanism: candidate record slice is incomplete",
		)
	}
	return candidate, leaf, proposal, nil
}

func mechanismScopedBundle(bundle Bundle, ids []string, repository string) (Bundle, error) {
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if err := validateOpaque("mechanism fact id", id); err != nil {
			return Bundle{}, err
		}
		if _, duplicate := wanted[id]; duplicate {
			return Bundle{}, fmt.Errorf("semantic mechanism: duplicate manifest fact %q", id)
		}
		wanted[id] = struct{}{}
	}
	if len(wanted) == 0 {
		return Bundle{}, fmt.Errorf("semantic mechanism: fact manifest is empty")
	}
	seen := make(map[string]struct{}, len(wanted))
	facts := make([]Fact, 0, len(wanted))
	for _, fact := range bundle.Facts {
		if _, needed := wanted[fact.ID]; !needed {
			continue
		}
		if _, duplicate := seen[fact.ID]; duplicate {
			return Bundle{}, fmt.Errorf("semantic mechanism: bound fact %q is duplicated", fact.ID)
		}
		seen[fact.ID] = struct{}{}
		facts = append(facts, fact)
	}
	if len(facts) != len(wanted) {
		for id := range wanted {
			if _, exists := seen[id]; !exists {
				return Bundle{}, fmt.Errorf("semantic mechanism: bound fact %q is missing", id)
			}
		}
	}
	scoped := Bundle{
		Version:  BundleVersion,
		RepoName: repository,
		Facts:    facts,
	}
	if err := scoped.Validate(); err != nil {
		return Bundle{}, err
	}
	return canonicalBundle(scoped), nil
}

func mechanismFactInputs(facts []Fact, artifact Artifact) ([]MechanismFactInput, error) {
	used := make(map[string]struct{}, len(artifact.UsedFactIDs))
	unused := make(map[string]struct{}, len(artifact.UnusedAvailableFactIDs))
	addIDs(used, artifact.UsedFactIDs)
	addIDs(unused, artifact.UnusedAvailableFactIDs)
	inputs := make([]MechanismFactInput, 0, len(facts))
	for _, fact := range facts {
		sha, err := mechanismFactSHA(fact)
		if err != nil {
			return nil, err
		}
		role := MechanismFactCandidateSeed
		if _, exists := used[fact.ID]; exists {
			role = MechanismFactClaimSupport
		} else if _, exists := unused[fact.ID]; exists {
			role = MechanismFactAvailableUnused
		}
		inputs = append(inputs, MechanismFactInput{ID: fact.ID, SHA256: sha, Role: role})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].ID < inputs[j].ID })
	return inputs, nil
}

func mechanismFactSHA(fact Fact) (string, error) {
	if err := validateFact(fact); err != nil {
		return "", err
	}
	fact = canonicalFact(fact)
	fact.Focus = Focus{}
	sha, _, err := hashJSON("semantic mechanism fact", fact)
	return sha, err
}

func mechanismContractSHA(candidate OpportunityCandidate) (string, error) {
	if candidate.Kind != ArtifactMechanism ||
		candidate.CapabilityContract == nil || candidate.IntentContract == nil {
		return "", fmt.Errorf("semantic mechanism: typed capability and intent contracts are required")
	}
	candidate = canonicalOpportunityCandidate(candidate)
	intent := *candidate.IntentContract
	// Search aliases are presentation copy. Their edits do not stale semantic
	// truth, although the accepted payload retains the original aliases.
	intent.LocalSearchAliases = nil
	contract := mechanismSemanticContract{
		Kind:               candidate.Kind,
		CapabilityContract: candidate.CapabilityContract,
		IntentContract:     &intent,
	}
	sha, _, err := hashJSON("semantic mechanism contract", contract)
	return sha, err
}

func validateMechanism(mechanism Mechanism) error {
	if mechanism.Version != MechanismVersion ||
		mechanism.Input.Version != MechanismInputVersion ||
		mechanism.Payload.Version != MechanismPayloadVersion {
		return fmt.Errorf("semantic mechanism: unsupported version")
	}
	if err := validateMechanismIdentity(mechanism.Identity); err != nil {
		return err
	}
	wantID, err := MechanismLogicalID(mechanism.Identity)
	if err != nil {
		return err
	}
	if mechanism.ID != wantID {
		return fmt.Errorf("semantic mechanism: logical id does not match identity")
	}
	if mechanism.Input.ValidatorVersion != MechanismValidatorVersion {
		return fmt.Errorf("semantic mechanism: validator version changed")
	}
	if err := validateMechanismProbe(mechanism.Input.Probe); err != nil {
		return err
	}
	if mechanism.Input.Probe.ID != mechanism.Identity.IntentKey {
		return fmt.Errorf("semantic mechanism: probe id does not match intent key")
	}
	if mechanism.Payload.OrderingBasis != MechanismOrderingEditorial {
		return fmt.Errorf("semantic mechanism: unsupported ordering basis")
	}
	candidate := mechanism.Payload.Candidate
	if candidate.Kind != ArtifactMechanism {
		return fmt.Errorf("semantic mechanism: payload candidate kind is invalid")
	}
	if err := validateOpaque("mechanism candidate id", candidate.ID); err != nil {
		return err
	}
	if mechanism.Payload.Leaf.CandidateID != candidate.ID ||
		mechanism.Payload.Proposal.CandidateID != candidate.ID {
		return fmt.Errorf("semantic mechanism: payload identities disagree")
	}
	if err := validateIntentContract(candidate.IntentContract); err != nil {
		return fmt.Errorf("semantic mechanism: invalid intent contract: %w", err)
	}
	if err := validateIntentCapabilityContract(
		candidate.IntentContract,
		candidate.CapabilityContract,
	); err != nil {
		return fmt.Errorf("semantic mechanism: invalid typed contract: %w", err)
	}
	contractSHA, err := mechanismContractSHA(candidate)
	if err != nil {
		return err
	}
	if mechanism.Input.ContractSHA256 != contractSHA {
		return fmt.Errorf("semantic mechanism: contract hash does not match payload")
	}
	if len(mechanism.Input.Facts) == 0 || len(mechanism.Input.Facts) > maxFacts {
		return fmt.Errorf("semantic mechanism: manifest fact count is invalid")
	}
	seen := make(map[string]struct{}, len(mechanism.Input.Facts))
	previous := ""
	for _, input := range mechanism.Input.Facts {
		if err := validateOpaque("mechanism fact id", input.ID); err != nil {
			return err
		}
		if _, duplicate := seen[input.ID]; duplicate || (previous != "" && input.ID < previous) {
			return fmt.Errorf("semantic mechanism: manifest facts are not canonical and unique")
		}
		seen[input.ID] = struct{}{}
		previous = input.ID
		if !validMechanismSHA256(input.SHA256) {
			return fmt.Errorf("semantic mechanism: invalid fact sha256")
		}
		switch input.Role {
		case MechanismFactClaimSupport,
			MechanismFactAvailableUnused,
			MechanismFactCandidateSeed:
		default:
			return fmt.Errorf("semantic mechanism: invalid fact role %q", input.Role)
		}
	}
	if !equalStringSets(mechanismManifestFactIDs(mechanism.Input.Facts), candidateFactIDs(candidate)) {
		return fmt.Errorf("semantic mechanism: manifest does not match candidate fact scope")
	}
	if !validMechanismSHA256(mechanism.Input.ContractSHA256) ||
		!validMechanismSHA256(mechanism.ContentSHA256) {
		return fmt.Errorf("semantic mechanism: invalid content or contract sha256")
	}
	wantContentSHA, err := mechanismContentSHA(mechanism)
	if err != nil {
		return err
	}
	if mechanism.ContentSHA256 != wantContentSHA {
		return fmt.Errorf("semantic mechanism: content hash does not match payload")
	}
	return nil
}

func validateMechanismIdentity(identity MechanismIdentity) error {
	if err := validateLocalText(
		"mechanism repository namespace",
		identity.RepositoryNamespace,
		256,
		true,
	); err != nil {
		return err
	}
	if strings.HasPrefix(identity.RepositoryNamespace, "/") ||
		strings.Contains(identity.RepositoryNamespace, "\\") {
		return fmt.Errorf("semantic mechanism: repository namespace is not portable")
	}
	if err := validateOpaque("mechanism intent key", identity.IntentKey); err != nil {
		return err
	}
	if identity.Scope.Kind != MechanismScopeGoPackage {
		return fmt.Errorf("semantic mechanism: unsupported scope kind %q", identity.Scope.Kind)
	}
	if err := validateLocalText("mechanism scope", identity.Scope.Value, 512, true); err != nil {
		return err
	}
	if strings.HasPrefix(identity.Scope.Value, "/") || strings.Contains(identity.Scope.Value, "\\") {
		return fmt.Errorf("semantic mechanism: scope is not portable")
	}
	return nil
}

func validateMechanismProbe(probe MechanismProbeInput) error {
	if probe.ContractVersion <= 0 {
		return fmt.Errorf("semantic mechanism: invalid probe contract version")
	}
	if err := validateOpaque("mechanism probe id", probe.ID); err != nil {
		return err
	}
	if !validMechanismSHA256(probe.SHA256) {
		return fmt.Errorf("semantic mechanism: invalid probe sha256")
	}
	return nil
}

func mechanismContentSHA(mechanism Mechanism) (string, error) {
	mechanism = canonicalMechanism(mechanism)
	content := mechanismContent{
		Version:  mechanism.Version,
		ID:       mechanism.ID,
		Identity: mechanism.Identity,
		Input:    mechanism.Input,
		Payload:  mechanism.Payload,
	}
	sha, _, err := hashJSON("semantic mechanism content", content)
	return sha, err
}

func canonicalMechanism(mechanism Mechanism) Mechanism {
	result := mechanism
	result.Input.Facts = append([]MechanismFactInput(nil), mechanism.Input.Facts...)
	sort.Slice(result.Input.Facts, func(i, j int) bool {
		return result.Input.Facts[i].ID < result.Input.Facts[j].ID
	})
	result.Payload = canonicalMechanismPayload(mechanism.Payload)
	return result
}

func canonicalMechanismPayload(payload MechanismPayload) MechanismPayload {
	result := payload
	result.Candidate = canonicalOpportunityCandidate(payload.Candidate)
	result.Leaf = NormalizeLeafArtifact(payload.Leaf)
	normalized := NormalizeFanInArtifact(FanInArtifact{
		Version:   FanInArtifactVersion,
		Artifacts: []ArtifactProposal{payload.Proposal},
	})
	if len(normalized.Artifacts) == 1 {
		result.Proposal = normalized.Artifacts[0]
	}
	return result
}

func mechanismManifestFactIDs(inputs []MechanismFactInput) []string {
	ids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		ids = append(ids, input.ID)
	}
	return ids
}

func mechanismArtifactByCandidate(
	artifacts []Artifact,
	candidateID string,
) (Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.CandidateID == candidateID {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func validMechanismSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
