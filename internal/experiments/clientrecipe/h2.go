package clientrecipe

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	H2Version          = 1
	maxH2ResponseBytes = 64 << 10
)

//go:embed h2_prompt.md
var h2Prompt string

// SynthesisProvider is the only model-facing boundary in the test-only H2
// experiment. The request contains presentation-safe short refs and bounded
// display facts; it never contains source, canonical IDs, or graph authority.
type SynthesisProvider interface {
	Synthesize(context.Context, string, []byte) ([]byte, error)
}

type SynthesisProviderFunc func(context.Context, string, []byte) ([]byte, error)

func (function SynthesisProviderFunc) Synthesize(ctx context.Context, prompt string, request []byte) ([]byte, error) {
	return function(ctx, prompt, request)
}

type H2StepRequest struct {
	Ref              string   `json:"ref"`
	Roles            []string `json:"roles"`
	Necessity        string   `json:"necessity"`
	CoveredExamples  int      `json:"covered_examples"`
	CompleteExamples int      `json:"complete_examples"`
}

type H2ExampleRequest struct {
	Ref              string   `json:"ref"`
	Name             string   `json:"name"`
	Wrapper          string   `json:"wrapper"`
	Complete         bool     `json:"complete"`
	VerificationKind string   `json:"verification_kind"`
	Missing          []string `json:"missing"`
}

type H2ProviderRequest struct {
	Version       int                `json:"version"`
	RequestDigest string             `json:"request_digest"`
	Steps         []H2StepRequest    `json:"steps"`
	Examples      []H2ExampleRequest `json:"examples"`
}

type H2StepCopy struct {
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	Purpose string `json:"purpose"`
}

type H2ExampleCopy struct {
	Ref     string `json:"ref"`
	Summary string `json:"summary"`
}

type H2Result struct {
	Version       int             `json:"version"`
	H1SHA256      string          `json:"h1_sha256"`
	RequestDigest string          `json:"request_digest"`
	Steps         []H2StepCopy    `json:"steps"`
	Examples      []H2ExampleCopy `json:"examples"`
	SHA256        string          `json:"sha256"`
}

type h2ProviderResponse struct {
	Version       int             `json:"version"`
	RequestDigest string          `json:"request_digest"`
	Steps         []H2StepCopy    `json:"steps"`
	Examples      []H2ExampleCopy `json:"examples"`
}

type h2StepDefinition struct {
	Ref   string
	Roles []H1Role
}

type h2ExampleDefinition struct {
	Ref      string
	Name     string
	Instance H1Instance
}

var h2StepDefinitions = []h2StepDefinition{
	{Ref: "s1", Roles: []H1Role{H1RoleConfiguration}},
	{Ref: "s2", Roles: []H1Role{H1RoleConstruction, H1RoleLocalWrapper}},
	{Ref: "s3", Roles: []H1Role{H1RoleConsumerBoundary, H1RoleProductionOperation}},
	{Ref: "s4", Roles: []H1Role{H1RoleApplicationWiring}},
	{Ref: "s5", Roles: []H1Role{H1RoleVerification}},
	{Ref: "s6", Roles: []H1Role{H1RoleObservability, H1RoleFailurePolicy}},
}

// BuildH2 asks an injected provider for presentation copy and restores the
// response into a result bound to the validated H1 authority. It deliberately
// has no provider fallback or partial-result path.
func BuildH2(ctx context.Context, h1 H1Result, provider SynthesisProvider) (H2Result, error) {
	if err := h1.Validate(); err != nil {
		return H2Result{}, err
	}
	if provider == nil {
		return H2Result{}, fmt.Errorf("client recipe H2: synthesis provider is required")
	}
	if err := ctx.Err(); err != nil {
		return H2Result{}, fmt.Errorf("client recipe H2: context: %w", err)
	}
	request, rawRequest, err := buildH2Request(h1)
	if err != nil {
		return H2Result{}, err
	}
	rawResponse, err := provider.Synthesize(ctx, h2Prompt, rawRequest)
	if err != nil {
		return H2Result{}, fmt.Errorf("client recipe H2: synthesize: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return H2Result{}, fmt.Errorf("client recipe H2: context: %w", err)
	}
	response, err := decodeH2ProviderResponse(rawResponse)
	if err != nil {
		return H2Result{}, err
	}
	if response.Version != H2Version || response.RequestDigest != request.RequestDigest {
		return H2Result{}, fmt.Errorf("client recipe H2: response/request binding mismatch")
	}
	steps, err := validateAndCanonicalizeH2Steps(response.Steps)
	if err != nil {
		return H2Result{}, err
	}
	examples, err := validateAndCanonicalizeH2Examples(response.Examples)
	if err != nil {
		return H2Result{}, err
	}
	result := H2Result{
		Version: H2Version, H1SHA256: h1.SHA256, RequestDigest: request.RequestDigest,
		Steps: steps, Examples: examples,
	}
	result.SHA256 = h2Digest(result)
	if err := result.ValidateAgainst(h1); err != nil {
		return H2Result{}, err
	}
	return result, nil
}

func EncodeH2(value H2Result) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("client recipe H2: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodeH2(raw []byte) (H2Result, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value H2Result
	if err := decoder.Decode(&value); err != nil {
		return H2Result{}, fmt.Errorf("client recipe H2: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return H2Result{}, fmt.Errorf("client recipe H2: trailing data")
	}
	if err := value.Validate(); err != nil {
		return H2Result{}, err
	}
	canonical, err := EncodeH2(value)
	if err != nil {
		return H2Result{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return H2Result{}, fmt.Errorf("client recipe H2: non-canonical bytes")
	}
	return value, nil
}

func (value H2Result) Validate() error {
	if value.Version != H2Version || !validSHA256(value.H1SHA256) ||
		!validSHA256(value.RequestDigest) || !validSHA256(value.SHA256) {
		return fmt.Errorf("client recipe H2: invalid identity")
	}
	steps, err := validateAndCanonicalizeH2Steps(value.Steps)
	if err != nil || !equalH2Steps(steps, value.Steps) {
		return fmt.Errorf("client recipe H2: non-canonical steps")
	}
	examples, err := validateAndCanonicalizeH2Examples(value.Examples)
	if err != nil || !equalH2Examples(examples, value.Examples) {
		return fmt.Errorf("client recipe H2: non-canonical examples")
	}
	if value.SHA256 != h2Digest(value) {
		return fmt.Errorf("client recipe H2: digest mismatch")
	}
	return nil
}

func (value H2Result) ValidateAgainst(h1 H1Result) error {
	if err := h1.Validate(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	request, _, err := buildH2Request(h1)
	if err != nil {
		return err
	}
	if value.H1SHA256 != h1.SHA256 || value.RequestDigest != request.RequestDigest {
		return fmt.Errorf("client recipe H2: H1/request binding mismatch")
	}
	return nil
}

func buildH2Request(h1 H1Result) (H2ProviderRequest, []byte, error) {
	examples := h2Examples(h1)
	if len(examples) != 4 {
		return H2ProviderRequest{}, nil, fmt.Errorf("client recipe H2: expected four validated examples")
	}
	necessities := make(map[H1Role]H1Necessity, len(h1.Roles))
	for _, role := range h1.Roles {
		necessities[role.Role] = role.Necessity
	}
	steps := make([]H2StepRequest, 0, len(h2StepDefinitions))
	for _, definition := range h2StepDefinitions {
		row := H2StepRequest{Ref: definition.Ref, Roles: make([]string, 0, len(definition.Roles))}
		seenNecessity := make(map[H1Necessity]struct{})
		for _, role := range definition.Roles {
			row.Roles = append(row.Roles, string(role))
			seenNecessity[necessities[role]] = struct{}{}
		}
		row.Necessity = h2NecessityLabel(seenNecessity)
		for _, example := range examples {
			if h1InstanceHasAnyRole(example.Instance, definition.Roles) {
				row.CoveredExamples++
				if example.Instance.Complete {
					row.CompleteExamples++
				}
			}
		}
		steps = append(steps, row)
	}
	exampleRows := make([]H2ExampleRequest, 0, len(examples))
	for _, example := range examples {
		missing := make([]string, len(example.Instance.Missing))
		for index, role := range example.Instance.Missing {
			missing[index] = string(role)
		}
		exampleRows = append(exampleRows, H2ExampleRequest{
			Ref: example.Ref, Name: example.Name, Wrapper: example.Instance.WrapperType,
			Complete: example.Instance.Complete, VerificationKind: example.Instance.VerificationKind,
			Missing: missing,
		})
	}
	payload := struct {
		Version  int                `json:"version"`
		Steps    []H2StepRequest    `json:"steps"`
		Examples []H2ExampleRequest `json:"examples"`
	}{Version: H2Version, Steps: steps, Examples: exampleRows}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return H2ProviderRequest{}, nil, fmt.Errorf("client recipe H2: encode request payload: %w", err)
	}
	digestBytes := sha256.Sum256(canonical)
	request := H2ProviderRequest{
		Version: H2Version, RequestDigest: hex.EncodeToString(digestBytes[:]),
		Steps: steps, Examples: exampleRows,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return H2ProviderRequest{}, nil, fmt.Errorf("client recipe H2: encode request: %w", err)
	}
	return request, append(raw, '\n'), nil
}

func decodeH2ProviderResponse(raw []byte) (h2ProviderResponse, error) {
	if len(raw) == 0 || len(raw) > maxH2ResponseBytes {
		return h2ProviderResponse{}, fmt.Errorf("client recipe H2: invalid response size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response h2ProviderResponse
	if err := decoder.Decode(&response); err != nil {
		return h2ProviderResponse{}, fmt.Errorf("client recipe H2: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return h2ProviderResponse{}, fmt.Errorf("client recipe H2: trailing response data")
	}
	return response, nil
}

func validateAndCanonicalizeH2Steps(rows []H2StepCopy) ([]H2StepCopy, error) {
	if len(rows) != len(h2StepDefinitions) {
		return nil, fmt.Errorf("client recipe H2: incomplete step copy")
	}
	seen := make(map[string]struct{}, len(rows))
	result := append([]H2StepCopy(nil), rows...)
	for _, row := range result {
		if !h2StepRef(row.Ref) || !h2PlainText(row.Title, 64) || !h2PlainText(row.Purpose, 240) {
			return nil, fmt.Errorf("client recipe H2: invalid step copy")
		}
		if _, duplicate := seen[row.Ref]; duplicate {
			return nil, fmt.Errorf("client recipe H2: duplicate step ref %q", row.Ref)
		}
		seen[row.Ref] = struct{}{}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Ref < result[j].Ref })
	for index, definition := range h2StepDefinitions {
		if result[index].Ref != definition.Ref {
			return nil, fmt.Errorf("client recipe H2: unknown or missing step ref")
		}
	}
	return result, nil
}

func validateAndCanonicalizeH2Examples(rows []H2ExampleCopy) ([]H2ExampleCopy, error) {
	if len(rows) != 4 {
		return nil, fmt.Errorf("client recipe H2: incomplete example copy")
	}
	seen := make(map[string]struct{}, len(rows))
	result := append([]H2ExampleCopy(nil), rows...)
	for _, row := range result {
		if !h2ExampleRef(row.Ref) || !h2PlainText(row.Summary, 200) {
			return nil, fmt.Errorf("client recipe H2: invalid example copy")
		}
		if _, duplicate := seen[row.Ref]; duplicate {
			return nil, fmt.Errorf("client recipe H2: duplicate example ref %q", row.Ref)
		}
		seen[row.Ref] = struct{}{}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Ref < result[j].Ref })
	for index, row := range result {
		if row.Ref != fmt.Sprintf("e%d", index+1) {
			return nil, fmt.Errorf("client recipe H2: unknown or missing example ref")
		}
	}
	return result, nil
}

func h2Examples(h1 H1Result) []h2ExampleDefinition {
	result := make([]h2ExampleDefinition, 0, len(h1.Instances))
	for _, instance := range h1.Instances {
		result = append(result, h2ExampleDefinition{Name: h2ExampleName(instance), Instance: instance})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Instance.ID < result[j].Instance.ID
	})
	for index := range result {
		result[index].Ref = fmt.Sprintf("e%d", index+1)
	}
	return result
}

func h2ExampleName(instance H1Instance) string {
	name := path.Base(instance.ImporterRepositoryPath)
	switch strings.ToLower(name) {
	case "clickhouse":
		return "ClickHouse"
	case "kubernetes":
		return "Kubernetes"
	case "notifier":
		return "Notifier"
	case "vault":
		return "Vault"
	default:
		runes := []rune(strings.ReplaceAll(name, "_", " "))
		if len(runes) != 0 {
			runes[0] = unicode.ToUpper(runes[0])
		}
		return string(runes)
	}
}

func h2NecessityLabel(values map[H1Necessity]struct{}) string {
	labels := make([]string, 0, len(values))
	for _, necessity := range []H1Necessity{H1Required, H1Common, H1Optional} {
		if _, present := values[necessity]; present {
			labels = append(labels, string(necessity))
		}
	}
	return strings.Join(labels, " + ")
}

func h1InstanceHasAnyRole(instance H1Instance, roles []H1Role) bool {
	for _, row := range instance.Roles {
		for _, role := range roles {
			if row.Role == role {
				return true
			}
		}
	}
	return false
}

func h2PlainText(value string, maximum int) bool {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		strings.ContainsAny(value, "<>\\/\r\n`[]{}#*") || strings.Contains(value, "://") ||
		strings.Contains(strings.ToLower(value), ".go") || strings.Contains(value, "#L") {
		return false
	}
	for index, r := range value {
		if unicode.IsControl(r) {
			return false
		}
		if r == ':' && index+1 < len(value) && value[index+1] >= '0' && value[index+1] <= '9' {
			return false
		}
	}
	return true
}

func h2StepRef(value string) bool {
	return len(value) == 2 && value[0] == 's' && value[1] >= '1' && value[1] <= '6'
}

func h2ExampleRef(value string) bool {
	return len(value) == 2 && value[0] == 'e' && value[1] >= '1' && value[1] <= '4'
}

func h2Digest(value H2Result) string {
	clone := value
	clone.SHA256 = ""
	raw, _ := json.Marshal(clone)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func equalH2Steps(left, right []H2StepCopy) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalH2Examples(left, right []H2ExampleCopy) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
