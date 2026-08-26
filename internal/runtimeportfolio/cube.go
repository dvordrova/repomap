package runtimeportfolio

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	preparationVersion    = 1
	responseSchemaVersion = 3
)

//go:embed prompts/system.md
var systemPrompt string

var PromptVersion = "runtime-portfolio-prompt-" + shortSHA([]byte(systemPrompt))

type wireEvidence struct {
	Ref       string       `json:"ref"`
	Kind      EvidenceKind `json:"kind"`
	Label     string       `json:"label"`
	Location  Location     `json:"location"`
	TargetRef string       `json:"target_ref,omitempty"`
}

type wireResponsibility struct {
	Name         string   `json:"name"`
	Purpose      string   `json:"purpose"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type wireTarget struct {
	Ref              string               `json:"ref"`
	DisplayName      string               `json:"display_name"`
	Language         string               `json:"language"`
	Kind             string               `json:"kind"`
	Selector         string               `json:"selector"`
	Default          bool                 `json:"default"`
	ProgramObjects   int                  `json:"program_objects"`
	ProgramRelations int                  `json:"program_relations"`
	ActivityStarts   int                  `json:"activity_starts"`
	IntegrationUses  int                  `json:"integration_uses"`
	Responsibilities []wireResponsibility `json:"responsibilities"`
	EvidenceRefs     []string             `json:"evidence_refs"`
}

type Request struct {
	RepositoryName  string         `json:"repository_name"`
	TargetCount     int            `json:"target_count"`
	Targets         []wireTarget   `json:"targets"`
	EvidenceCatalog []wireEvidence `json:"evidence_catalog"`
}

type responseImplementation struct {
	TargetRef string `json:"target_ref"`
	Mode      string `json:"mode,omitempty"`
}

type responseRole struct {
	Name            string                   `json:"name"`
	Purpose         string                   `json:"purpose"`
	Prominence      Prominence               `json:"prominence"`
	Kind            RoleKind                 `json:"role_kind"`
	Requiredness    Requiredness             `json:"requiredness"`
	Confidence      Confidence               `json:"confidence"`
	MappingStatus   MappingStatus            `json:"mapping_status"`
	Implementations []responseImplementation `json:"implementations"`
	EvidenceRefs    []string                 `json:"evidence_refs"`
}

type response struct {
	Roles []responseRole `json:"roles"`
}

type Compilation struct {
	Request       Request
	RequestSHA256 string

	input         Input
	wire          []byte
	state         []byte
	targetsByRef  map[string]Target
	evidenceByRef map[string]Evidence
	seal          string
}

func Compile(input Input) (Compilation, error) {
	canonical, err := canonicalInput(input)
	if err != nil {
		return Compilation{}, err
	}
	request, targetsByRef, evidenceByRef, err := compileRequest(canonical)
	if err != nil {
		return Compilation{}, err
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("runtime portfolio: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Compilation{}, fmt.Errorf(
			"runtime portfolio: complete request is %d bytes, limit is %d", len(wire), MaxRequestBytes,
		)
	}
	if _, found := secretscan.Detect(string(wire)); found {
		return Compilation{}, fmt.Errorf("runtime portfolio: provider request contains credential-shaped content")
	}
	state, err := executionState(canonical, wire)
	if err != nil {
		return Compilation{}, err
	}
	compilation := Compilation{
		Request: request, RequestSHA256: sha256Hex(wire), input: canonical,
		wire: append([]byte(nil), wire...), state: state,
		targetsByRef: targetsByRef, evidenceByRef: evidenceByRef,
	}
	compilation.seal, err = compilationSeal(canonical, state, wire)
	if err != nil {
		return Compilation{}, err
	}
	if err := compilation.validate(); err != nil {
		return Compilation{}, err
	}
	return compilation, nil
}

func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	input Input,
) (llm.Outcome[Result], error) {
	compilation, err := Compile(input)
	if err != nil {
		return llm.Outcome[Result]{}, err
	}
	if provider == nil {
		return llm.Outcome[Result]{}, fmt.Errorf("runtime portfolio: provider is unavailable")
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[Result]{
		State: append([]byte(nil), compilation.state...),
		Prompt: llm.Prompt{
			System: strings.TrimSpace(systemPrompt), User: string(compilation.wire), ResponseFormatJSON: true,
		},
		Limits: llm.Limits{
			MaxRequestBytes: MaxProviderRequestBytes, MaxResponseBytes: MaxResponseBytes,
			MaxOutputTokens: MaxOutputTokens,
		},
		DecodeValidate: func(raw []byte) (Result, error) {
			return ResolveResponse(compilation, raw)
		},
	})
	if err != nil {
		return llm.Outcome[Result]{}, fmt.Errorf("runtime portfolio: model cube: %w", err)
	}
	return outcome, nil
}

func ResolveResponse(compilation Compilation, raw []byte) (Result, error) {
	if err := compilation.validate(); err != nil {
		return Result{}, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Result{}, fmt.Errorf("runtime portfolio: response exceeds bounded envelope")
	}
	if _, found := secretscan.Detect(string(raw)); found {
		return Result{}, fmt.Errorf("runtime portfolio: response contains credential-shaped content")
	}
	var decoded response
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return Result{}, fmt.Errorf("runtime portfolio: invalid JSON response: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Result{}, err
	}
	if decoded.Roles == nil {
		return Result{}, fmt.Errorf("runtime portfolio: roles must be an array")
	}
	result := Result{
		Version: Version, TargetPagePortfolioSHA256: compilation.input.TargetPagePortfolioSHA256,
		Targets: resultTargets(compilation.input.Targets), Roles: []Role{},
		UnclassifiedTargetIDs: []string{},
	}
	roleByID := make(map[string]Role)
	nameToID := make(map[string]string)
	selectedEvidence := make(map[string]struct{})
	for index, proposed := range decoded.Roles {
		role, err := restoreRole(proposed, compilation.targetsByRef, compilation.evidenceByRef)
		if err != nil {
			return Result{}, fmt.Errorf("runtime portfolio: role %d: %w", index, err)
		}
		if previousID, duplicateName := nameToID[strings.ToLower(role.Name)]; duplicateName && previousID != role.ID {
			return Result{}, fmt.Errorf("runtime portfolio: conflicting duplicate role name %q", role.Name)
		}
		nameToID[strings.ToLower(role.Name)] = role.ID
		roleByID[role.ID] = role
	}
	for _, role := range roleByID {
		result.Roles = append(result.Roles, role)
		for _, evidence := range role.Evidence {
			selectedEvidence[evidence.ID] = struct{}{}
		}
	}
	sort.Slice(result.Roles, func(i, j int) bool { return roleSortKey(result.Roles[i]) < roleSortKey(result.Roles[j]) })
	mapped := make(map[string]struct{})
	for _, role := range result.Roles {
		for _, implementation := range role.Implementations {
			mapped[implementation.ProgramTargetID] = struct{}{}
		}
	}
	for _, target := range result.Targets {
		if _, ok := mapped[target.ProgramTargetID]; !ok {
			result.UnclassifiedTargetIDs = append(result.UnclassifiedTargetIDs, target.ProgramTargetID)
		}
	}
	result.Coverage = Coverage{
		TargetsObserved: len(result.Targets), TargetsMapped: len(mapped),
		TargetsUnclassified: len(result.UnclassifiedTargetIDs), Roles: len(result.Roles),
		EvidenceAdvertised: len(compilation.evidenceByRef), EvidenceSelected: len(selectedEvidence),
	}
	if err := result.validateAgainstCompilation(compilation); err != nil {
		return Result{}, err
	}
	return result, nil
}

// ValidateAgainst proves that a standalone artifact is bound to the current
// complete target/evidence authority. Validate checks canonical shape only;
// this check additionally rejects substituted target metadata, portfolio
// identity, evidence, or advertised-evidence accounting.
func (result Result) ValidateAgainst(input Input) error {
	compilation, err := Compile(input)
	if err != nil {
		return fmt.Errorf("runtime portfolio: compile validation authority: %w", err)
	}
	return result.validateAgainstCompilation(compilation)
}

func (result Result) validateAgainstCompilation(compilation Compilation) error {
	if err := compilation.validate(); err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if result.TargetPagePortfolioSHA256 != compilation.input.TargetPagePortfolioSHA256 ||
		!reflect.DeepEqual(result.Targets, resultTargets(compilation.input.Targets)) {
		return fmt.Errorf("runtime portfolio: target authority mismatch")
	}
	if result.Coverage.EvidenceAdvertised != len(compilation.evidenceByRef) {
		return fmt.Errorf("runtime portfolio: evidence coverage authority mismatch")
	}
	advertisedByID := make(map[string]Evidence, len(compilation.evidenceByRef))
	for _, evidence := range compilation.evidenceByRef {
		advertisedByID[evidence.ID] = evidence
	}
	for _, role := range result.Roles {
		for _, evidence := range role.Evidence {
			advertised, known := advertisedByID[evidence.ID]
			if !known || !reflect.DeepEqual(advertised, evidence) {
				return fmt.Errorf("runtime portfolio: role evidence is outside current authority")
			}
		}
	}
	if _, err := Encode(result); err != nil {
		return err
	}
	return nil
}

func restoreRole(
	proposed responseRole,
	targetsByRef map[string]Target,
	evidenceByRef map[string]Evidence,
) (Role, error) {
	if !validText(proposed.Name) || !validText(proposed.Purpose) ||
		!validProminence(proposed.Prominence) || !validRoleKind(proposed.Kind) ||
		!validRequiredness(proposed.Requiredness) || !validConfidence(proposed.Confidence) ||
		!validMappingStatus(proposed.MappingStatus) || proposed.Implementations == nil ||
		proposed.EvidenceRefs == nil {
		return Role{}, fmt.Errorf("invalid semantic fields")
	}
	implementations := make(map[string]Implementation)
	for _, candidate := range proposed.Implementations {
		target, known := targetsByRef[candidate.TargetRef]
		if !known {
			continue
		}
		if candidate.Mode != "" && !validText(candidate.Mode) {
			return Role{}, fmt.Errorf("invalid executable mode")
		}
		implementation := Implementation{ProgramTargetID: target.ProgramTargetID, Mode: candidate.Mode}
		implementations[implementation.ProgramTargetID+"\x00"+implementation.Mode] = implementation
	}
	if proposed.MappingStatus == MappingMapped && len(implementations) == 0 {
		return Role{}, fmt.Errorf("known target filtering left mapped role unresolved")
	}
	if proposed.MappingStatus == MappingUnknown && len(implementations) != 0 {
		return Role{}, fmt.Errorf("unknown mapping selected a known target")
	}
	if (proposed.Kind == RoleKindExample || proposed.Kind == RoleKindSupportingTool) &&
		proposed.Prominence != ProminenceSupporting {
		return Role{}, fmt.Errorf("example or supporting-tool role is not supporting")
	}
	evidenceSet := make(map[string]Evidence)
	for _, ref := range proposed.EvidenceRefs {
		if evidence, known := evidenceByRef[ref]; known {
			evidenceSet[evidence.ID] = evidence
		}
	}
	if len(evidenceSet) == 0 {
		return Role{}, fmt.Errorf("known evidence filtering left role unsupported")
	}
	role := Role{
		Name: proposed.Name, Purpose: proposed.Purpose, Prominence: proposed.Prominence,
		Kind: proposed.Kind, Requiredness: proposed.Requiredness, Confidence: proposed.Confidence,
		MappingStatus: proposed.MappingStatus, Implementations: []Implementation{}, Evidence: []Evidence{},
	}
	for _, value := range implementations {
		role.Implementations = append(role.Implementations, value)
	}
	sort.Slice(role.Implementations, func(i, j int) bool {
		left := role.Implementations[i].ProgramTargetID + "\x00" + role.Implementations[i].Mode
		right := role.Implementations[j].ProgramTargetID + "\x00" + role.Implementations[j].Mode
		return left < right
	})
	for _, value := range evidenceSet {
		role.Evidence = append(role.Evidence, value)
	}
	sort.Slice(role.Evidence, func(i, j int) bool { return role.Evidence[i].ID < role.Evidence[j].ID })
	var err error
	role.ID, err = roleID(role)
	if err != nil {
		return Role{}, err
	}
	return role, nil
}

func compileRequest(input Input) (Request, map[string]Target, map[string]Evidence, error) {
	request := Request{
		RepositoryName: input.RepositoryName, TargetCount: len(input.Targets),
		Targets: []wireTarget{}, EvidenceCatalog: []wireEvidence{},
	}
	targetsByRef := make(map[string]Target, len(input.Targets))
	targetRefByID := make(map[string]string, len(input.Targets))
	for index, target := range input.Targets {
		ref := fmt.Sprintf("t%d", index+1)
		targetsByRef[ref] = Target{
			ProgramTargetID: target.ProgramTargetID, DisplayName: target.DisplayName,
			Language: target.Language, Kind: target.Kind, Selector: target.Selector, Default: target.Default,
		}
		targetRefByID[target.ProgramTargetID] = ref
	}
	allEvidence := make([]EvidenceInput, 0)
	for _, target := range input.Targets {
		for _, evidence := range target.Evidence {
			allEvidence = append(allEvidence, evidence)
		}
		for _, responsibility := range target.Responsibilities {
			for _, evidence := range responsibility.Evidence {
				allEvidence = append(allEvidence, evidence)
			}
		}
	}
	for _, evidence := range input.RepositoryEvidence {
		allEvidence = append(allEvidence, evidence)
	}
	evidenceByKey := make(map[string]EvidenceInput)
	for _, evidence := range allEvidence {
		key := evidenceInputKey(evidence)
		evidenceByKey[key] = evidence
	}
	keys := make([]string, 0, len(evidenceByKey))
	for key := range evidenceByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	evidenceByRef := make(map[string]Evidence, len(keys))
	evidenceRefByKey := make(map[string]string, len(keys))
	wireEvidenceByRef := make(map[string]wireEvidence, len(keys))
	for index, key := range keys {
		value := evidenceByKey[key]
		ref := fmt.Sprintf("e%d", index+1)
		evidence := Evidence{
			Kind: value.Kind, Label: value.Label, Location: value.Location,
			ProgramTargetID: value.ProgramTargetID,
		}
		var err error
		evidence.ID, err = evidenceID(evidence)
		if err != nil {
			return Request{}, nil, nil, err
		}
		evidenceByRef[ref] = evidence
		evidenceRefByKey[key] = ref
		wireEvidenceByRef[ref] = wireEvidence{
			Ref: ref, Kind: value.Kind, Label: value.Label, Location: value.Location,
			TargetRef: targetRefByID[value.ProgramTargetID],
		}
	}
	for index, target := range input.Targets {
		wireTarget := wireTarget{
			Ref: fmt.Sprintf("t%d", index+1), DisplayName: target.DisplayName,
			Language: target.Language, Kind: target.Kind, Selector: target.Selector, Default: target.Default,
			ProgramObjects: target.ProgramObjects, ProgramRelations: target.ProgramRelations,
			ActivityStarts: target.ActivityStarts, IntegrationUses: target.IntegrationUses,
			Responsibilities: []wireResponsibility{}, EvidenceRefs: []string{},
		}
		for _, evidence := range target.Evidence {
			wireTarget.EvidenceRefs = append(wireTarget.EvidenceRefs, evidenceRefByKey[evidenceInputKey(evidence)])
		}
		wireTarget.EvidenceRefs = canonicalStrings(wireTarget.EvidenceRefs)
		for _, responsibility := range target.Responsibilities {
			row := wireResponsibility{Name: responsibility.Name, Purpose: responsibility.Purpose, EvidenceRefs: []string{}}
			for _, evidence := range responsibility.Evidence {
				row.EvidenceRefs = append(row.EvidenceRefs, evidenceRefByKey[evidenceInputKey(evidence)])
			}
			row.EvidenceRefs = canonicalStrings(row.EvidenceRefs)
			wireTarget.Responsibilities = append(wireTarget.Responsibilities, row)
		}
		request.Targets = append(request.Targets, wireTarget)
	}
	for _, key := range keys {
		ref := evidenceRefByKey[key]
		request.EvidenceCatalog = append(request.EvidenceCatalog, wireEvidenceByRef[ref])
	}
	return request, targetsByRef, evidenceByRef, nil
}

func canonicalInput(input Input) (Input, error) {
	if !validText(input.RepositoryName) || !validRevision(input.CapturedRevision) ||
		!validSHA256(input.TargetPagePortfolioSHA256) || len(input.Targets) == 0 || input.Targets == nil ||
		input.RepositoryEvidence == nil {
		return Input{}, fmt.Errorf("runtime portfolio: invalid input identity")
	}
	result := input
	result.Targets = append([]TargetInput(nil), input.Targets...)
	sort.Slice(result.Targets, func(i, j int) bool { return result.Targets[i].ProgramTargetID < result.Targets[j].ProgramTargetID })
	targetIDs := make(map[string]struct{}, len(result.Targets))
	defaultCount := 0
	for index := range result.Targets {
		target := &result.Targets[index]
		if !validText(target.ProgramTargetID) || !validText(target.DisplayName) ||
			!validText(target.Language) || !validText(target.Kind) || !validText(target.Selector) ||
			target.ProgramObjects < 0 || target.ProgramRelations < 0 || target.ActivityStarts < 0 ||
			target.IntegrationUses < 0 || target.Responsibilities == nil || target.Evidence == nil {
			return Input{}, fmt.Errorf("runtime portfolio: invalid target input")
		}
		if _, duplicate := targetIDs[target.ProgramTargetID]; duplicate {
			return Input{}, fmt.Errorf("runtime portfolio: duplicate target input")
		}
		targetIDs[target.ProgramTargetID] = struct{}{}
		if target.Default {
			defaultCount++
		}
		target.Responsibilities = append([]ResponsibilityInput{}, target.Responsibilities...)
		for responsibilityIndex := range target.Responsibilities {
			responsibility := &target.Responsibilities[responsibilityIndex]
			if !validText(responsibility.Name) || !validText(responsibility.Purpose) || responsibility.Evidence == nil {
				return Input{}, fmt.Errorf("runtime portfolio: invalid responsibility input")
			}
			responsibility.Evidence = canonicalEvidenceInputs(responsibility.Evidence)
		}
		sort.Slice(target.Responsibilities, func(i, j int) bool {
			return responsibilityInputKey(target.Responsibilities[i]) < responsibilityInputKey(target.Responsibilities[j])
		})
		canonicalResponsibilities := target.Responsibilities[:0]
		for _, responsibility := range target.Responsibilities {
			if len(canonicalResponsibilities) == 0 ||
				responsibilityInputKey(canonicalResponsibilities[len(canonicalResponsibilities)-1]) != responsibilityInputKey(responsibility) {
				canonicalResponsibilities = append(canonicalResponsibilities, responsibility)
			}
		}
		target.Responsibilities = canonicalResponsibilities
		target.Evidence = canonicalEvidenceInputs(target.Evidence)
	}
	if defaultCount != 1 {
		return Input{}, fmt.Errorf("runtime portfolio: exactly one input target must be default")
	}
	result.RepositoryEvidence = canonicalEvidenceInputs(input.RepositoryEvidence)
	for _, target := range result.Targets {
		for _, evidence := range target.Evidence {
			if err := validateEvidenceInput(evidence, targetIDs); err != nil {
				return Input{}, err
			}
			if evidence.ProgramTargetID != target.ProgramTargetID {
				return Input{}, fmt.Errorf("runtime portfolio: target evidence is not bound to its target")
			}
		}
		for _, responsibility := range target.Responsibilities {
			for _, evidence := range responsibility.Evidence {
				if err := validateEvidenceInput(evidence, targetIDs); err != nil {
					return Input{}, err
				}
				if evidence.ProgramTargetID != target.ProgramTargetID {
					return Input{}, fmt.Errorf("runtime portfolio: responsibility evidence is not bound to its target")
				}
			}
		}
	}
	for _, evidence := range result.RepositoryEvidence {
		if err := validateEvidenceInput(evidence, targetIDs); err != nil {
			return Input{}, err
		}
	}
	return result, nil
}

func validateEvidenceInput(value EvidenceInput, targetIDs map[string]struct{}) error {
	if !validEvidenceKind(value.Kind) || !validText(value.Label) || !validLocation(value.Location) {
		return fmt.Errorf("runtime portfolio: invalid evidence input")
	}
	if value.ProgramTargetID != "" {
		if _, known := targetIDs[value.ProgramTargetID]; !known {
			return fmt.Errorf("runtime portfolio: evidence input cites unknown target")
		}
	}
	return nil
}

func canonicalEvidenceInputs(values []EvidenceInput) []EvidenceInput {
	result := append([]EvidenceInput{}, values...)
	sort.Slice(result, func(i, j int) bool { return evidenceInputKey(result[i]) < evidenceInputKey(result[j]) })
	out := result[:0]
	for _, value := range result {
		if len(out) == 0 || evidenceInputKey(out[len(out)-1]) != evidenceInputKey(value) {
			out = append(out, value)
		}
	}
	return out
}

func evidenceInputKey(value EvidenceInput) string {
	return value.ProgramTargetID + "\x00" + string(value.Kind) + "\x00" + value.Location.Path +
		fmt.Sprintf("\x00%010d\x00%010d\x00", value.Location.Line, value.Location.Column) + value.Label
}

func responsibilityInputKey(value ResponsibilityInput) string {
	var evidenceKeys strings.Builder
	for _, evidence := range value.Evidence {
		evidenceKeys.WriteString("\x00")
		evidenceKeys.WriteString(evidenceInputKey(evidence))
	}
	return strings.ToLower(value.Name) + "\x00" + value.Name + "\x00" + value.Purpose + evidenceKeys.String()
}

func resultTargets(values []TargetInput) []Target {
	result := make([]Target, len(values))
	for index, value := range values {
		result[index] = Target{
			ProgramTargetID: value.ProgramTargetID, DisplayName: value.DisplayName,
			Language: value.Language, Kind: value.Kind, Selector: value.Selector, Default: value.Default,
		}
	}
	return result
}

type executionIdentity struct {
	Contract              string
	PromptVersion         string
	PreparationVersion    int
	ResponseSchemaVersion int
}

func currentExecutionIdentity() executionIdentity {
	return executionIdentity{
		Contract: executionContract, PromptVersion: PromptVersion,
		PreparationVersion: preparationVersion, ResponseSchemaVersion: responseSchemaVersion,
	}
}

func executionState(input Input, request []byte) ([]byte, error) {
	return executionStateWithIdentity(input, request, currentExecutionIdentity())
}

func executionStateWithIdentity(input Input, request []byte, identity executionIdentity) ([]byte, error) {
	// TargetPagePortfolioSHA256 is a strict publication binding, but it changes
	// when current target-page run IDs change. Keep every repository-semantic
	// fact in the cache identity while excluding that one publication-local seal.
	semanticInputRaw, err := json.Marshal(struct {
		RepositoryName     string
		CapturedRevision   string
		Targets            []TargetInput
		RepositoryEvidence []EvidenceInput
	}{
		RepositoryName: input.RepositoryName, CapturedRevision: input.CapturedRevision,
		Targets: input.Targets, RepositoryEvidence: input.RepositoryEvidence,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime portfolio: encode semantic input state: %w", err)
	}
	return json.Marshal(struct {
		Contract              string `json:"contract"`
		PromptVersion         string `json:"prompt_version"`
		PreparationVersion    int    `json:"preparation_version"`
		ResponseSchemaVersion int    `json:"response_schema_version"`
		InputSHA256           string `json:"input_sha256"`
		RequestSHA256         string `json:"request_sha256"`
	}{
		Contract: identity.Contract, PromptVersion: identity.PromptVersion,
		PreparationVersion: identity.PreparationVersion, ResponseSchemaVersion: identity.ResponseSchemaVersion,
		InputSHA256: sha256Hex(semanticInputRaw), RequestSHA256: sha256Hex(request),
	})
}

func compilationSeal(input Input, state, request []byte) (string, error) {
	inputRaw, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("runtime portfolio: encode compilation input: %w", err)
	}
	binding, err := json.Marshal(struct {
		Contract      string `json:"contract"`
		InputSHA256   string `json:"input_sha256"`
		StateSHA256   string `json:"state_sha256"`
		RequestSHA256 string `json:"request_sha256"`
	}{
		Contract: "runtime-portfolio-compilation-v2", InputSHA256: sha256Hex(inputRaw),
		StateSHA256: sha256Hex(state), RequestSHA256: sha256Hex(request),
	})
	if err != nil {
		return "", fmt.Errorf("runtime portfolio: encode compilation binding: %w", err)
	}
	return sha256Hex(binding), nil
}

func (compilation Compilation) validate() error {
	canonical, err := canonicalInput(compilation.input)
	if err != nil || !reflect.DeepEqual(canonical, compilation.input) {
		return fmt.Errorf("runtime portfolio: compilation input authority mismatch")
	}
	request, targets, evidence, err := compileRequest(canonical)
	if err != nil || !reflect.DeepEqual(request, compilation.Request) ||
		!reflect.DeepEqual(targets, compilation.targetsByRef) || !reflect.DeepEqual(evidence, compilation.evidenceByRef) {
		return fmt.Errorf("runtime portfolio: compilation catalog authority mismatch")
	}
	wire, err := json.Marshal(request)
	if err != nil || !bytes.Equal(wire, compilation.wire) || compilation.RequestSHA256 != sha256Hex(wire) {
		return fmt.Errorf("runtime portfolio: compilation request binding mismatch")
	}
	state, err := executionState(canonical, wire)
	seal, sealErr := compilationSeal(canonical, state, wire)
	if err != nil || sealErr != nil || !bytes.Equal(state, compilation.state) || compilation.seal != seal {
		return fmt.Errorf("runtime portfolio: compilation state binding mismatch")
	}
	return nil
}

func canonicalStrings(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	out := result[:0]
	for _, value := range result {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func ensureEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("runtime portfolio: trailing JSON value")
		}
		return fmt.Errorf("runtime portfolio: invalid trailing JSON: %w", err)
	}
	return nil
}
