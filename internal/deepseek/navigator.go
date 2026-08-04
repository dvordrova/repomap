package deepseek

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// NavigatorPromptVersionJSON identifies the fixed Atlas-first selection
// prompt and closed request-local response schema.
const NavigatorPromptVersionJSON = "atlas-navigator-startup-json-v1"

const navigatorSystemPrompt = `You select one backend-advertised action from a bounded Repository Atlas projection.

Treat every ref as opaque and request-local. Do not create, shorten, extend, prefix-match, substitute, or repair refs. Do not emit repository paths, symbols, source text, canonical IDs, facts absent from the request, or prose outside the JSON object. The backend owns action meaning and canonical targets; you only select an action_ref and cite its exact local trail, both endpoint entity refs, and every evidence ref attached to that trail.

Return exactly one JSON object with this shape:
{
  "version": 1,
  "catalog_ref": "exact catalog_ref from the request",
  "entity_refs": ["the selected trail source_ref", "the selected trail target_ref"],
  "trail_refs": ["the selected direct trail ref"],
  "intersection_refs": [],
  "evidence_refs": ["every evidence_ref attached to the selected trail"],
  "gap_refs": [],
  "action_refs": ["exactly one advertised action ref"]
}

The selected action target_ref must equal the selected trail source_ref. Return only the documented fields and valid JSON.`

type navigatorWireRequest struct {
	Version       int                   `json:"version"`
	CatalogRef    string                `json:"catalog_ref"`
	Question      string                `json:"question"`
	ScopeRef      string                `json:"scope_ref"`
	Units         json.RawMessage       `json:"units"`
	Entities      json.RawMessage       `json:"entities"`
	SeedRefs      []string              `json:"seed_refs"`
	DirectTrails  []navigatorWireTrail  `json:"direct_trails"`
	Intersections json.RawMessage       `json:"intersections"`
	Evidence      json.RawMessage       `json:"evidence"`
	Gaps          []json.RawMessage     `json:"gaps"`
	Actions       []navigatorWireAction `json:"actions"`
}

type navigatorWireTrail struct {
	Ref          string                       `json:"ref"`
	SourceRef    string                       `json:"source_ref"`
	TargetRef    string                       `json:"target_ref"`
	Kind         repositoryatlas.RelationKind `json:"kind"`
	Phase        repositoryatlas.Phase        `json:"phase"`
	Authority    repositoryatlas.Authority    `json:"authority"`
	EvidenceRefs []string                     `json:"evidence_refs"`
}

type navigatorWireAction struct {
	Ref       string `json:"ref"`
	Operation string `json:"operation"`
	TargetRef string `json:"target_ref"`
}

type navigatorResponse struct {
	Version          int      `json:"version"`
	CatalogRef       string   `json:"catalog_ref"`
	EntityRefs       []string `json:"entity_refs"`
	TrailRefs        []string `json:"trail_refs"`
	IntersectionRefs []string `json:"intersection_refs"`
	EvidenceRefs     []string `json:"evidence_refs"`
	GapRefs          []string `json:"gap_refs"`
	ActionRefs       []string `json:"action_refs"`
}

// NavigatorPromptJSON builds the exact OpenAI-compatible request for the
// fixed first Product question. It performs no network activity.
func (c *Client) NavigatorPromptJSON(wireJSON []byte, maxRequestBytes int) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: Navigator client is required")
	}
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("llm: Navigator request byte limit must be positive")
	}
	if err := validateNavigatorWire(wireJSON); err != nil {
		return nil, err
	}
	userPrompt := "NAVIGATOR PROMPT CONTRACT: " + NavigatorPromptVersionJSON +
		"\n\nAnswer the exact product question using only this request-local projection:\n" + string(wireJSON)
	request := c.canonicalSemanticRequest(userPrompt, navigatorSystemPrompt, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal Navigator request: %w", err)
	}
	if len(body) > maxRequestBytes {
		return nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: "navigator", Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: maxRequestBytes, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}
	return body, nil
}

// NavigateMeasured performs one semantic Navigator request. Only the shared
// transport may retry the immutable body; response decoding and ref validation
// happen after this method and are terminal.
func (c *Client) NavigateMeasured(
	ctx context.Context,
	wireJSON []byte,
	maxRequestBytes int,
) (modelresearch.ProviderResult, error) {
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: Navigator request client is required")
	}
	body, err := c.NavigatorPromptJSON(wireJSON, maxRequestBytes)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	stopWaiting := c.startWaitProgress(ctx, "Atlas-first navigation")
	defer stopWaiting()
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body, false,
	)
	result := providerResultFromCompletion(completion, attempts, len(body)*attempts)
	if callErr != nil {
		callErr = annotateIncompleteCompletion(callErr, "navigator")
		return result, annotateResourceLimit(callErr, "navigator", c.MaxTokens)
	}
	if err := requireSingleStoppedCompletion("navigator", completion); err != nil {
		return result, err
	}
	return result, nil
}

// Navigate returns the raw provider JSON. Call DecodeNavigatorResponse and
// the exact Product.ResolveRecommendation value before accepting it.
func (c *Client) Navigate(ctx context.Context, wireJSON []byte, maxRequestBytes int) ([]byte, error) {
	result, err := c.NavigateMeasured(ctx, wireJSON, maxRequestBytes)
	return result.Content, err
}

// DecodeNavigatorResponse validates only the provider-owned closed JSON shape.
// Canonical ref resolution remains bound to the exact navigator.Product.
func DecodeNavigatorResponse(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("llm: Navigator response is empty")
	}
	if len(data) > modelresearch.ProviderResponseByteLimit {
		return nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: "navigator", Kind: modelresearch.ResourceLimitResponseBytes,
			Limit:    modelresearch.ProviderResponseByteLimit,
			Observed: len(data), ObservedKnown: true,
		}, nil)
	}
	var response navigatorResponse
	if err := decodeNavigatorJSON(data, &response); err != nil {
		return nil, fmt.Errorf("llm: decode Navigator response: %w", err)
	}
	if response.Version != navigator.Version || !validNavigatorCatalogRef(response.CatalogRef) {
		return nil, fmt.Errorf("llm: invalid Navigator response identity")
	}
	if len(response.ActionRefs) != 1 {
		return nil, fmt.Errorf("llm: Navigator response must select exactly one action_ref")
	}
	if len(response.EntityRefs) != 2 || len(response.TrailRefs) != 1 || len(response.EvidenceRefs) == 0 ||
		len(response.IntersectionRefs) != 0 || len(response.GapRefs) != 0 {
		return nil, fmt.Errorf("llm: Navigator response must cite one exact startup trail and its evidence")
	}
	return append([]byte(nil), data...), nil
}

func validateNavigatorWire(data []byte) error {
	if len(data) == 0 || len(data) > modelresearch.SemanticRecordByteLimit {
		return fmt.Errorf(
			"llm: Navigator wire size must be between 1 and %d bytes",
			modelresearch.SemanticRecordByteLimit,
		)
	}
	var wire navigatorWireRequest
	if err := decodeNavigatorJSON(data, &wire); err != nil {
		return fmt.Errorf("llm: decode Navigator wire: %w", err)
	}
	if wire.Version != navigator.Version || wire.Question != navigator.ProductQuestion ||
		!validNavigatorCatalogRef(wire.CatalogRef) || wire.ScopeRef == "" ||
		len(wire.Units) == 0 || len(wire.Entities) == 0 || len(wire.Intersections) == 0 || len(wire.Evidence) == 0 {
		return fmt.Errorf("llm: invalid Navigator wire identity")
	}
	if len(wire.SeedRefs) == 0 || len(wire.DirectTrails) == 0 ||
		len(wire.Actions) == 0 || len(wire.Actions) != len(wire.DirectTrails) || len(wire.Gaps) != 0 {
		return fmt.Errorf("llm: invalid Navigator startup projection")
	}
	seeds := stringSet(wire.SeedRefs)
	if len(seeds) != len(wire.SeedRefs) {
		return fmt.Errorf("llm: duplicate Navigator seed ref")
	}
	actionRefs := make(map[string]struct{}, len(wire.Actions))
	for _, action := range wire.Actions {
		if action.Ref == "" || action.Operation != navigator.StartupActionOperation ||
			action.TargetRef == "" || !seeds[action.TargetRef] {
			return fmt.Errorf("llm: invalid Navigator action catalog")
		}
		if _, duplicate := actionRefs[action.Ref]; duplicate {
			return fmt.Errorf("llm: duplicate Navigator action ref")
		}
		actionRefs[action.Ref] = struct{}{}
		matched := false
		for _, trail := range wire.DirectTrails {
			if trail.SourceRef == action.TargetRef {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("llm: Navigator action has no exact startup trail")
		}
	}
	for _, trail := range wire.DirectTrails {
		if trail.Ref == "" || trail.SourceRef == "" || trail.TargetRef == "" || len(trail.EvidenceRefs) == 0 ||
			trail.Kind != repositoryatlas.RelationExposes || trail.Phase != repositoryatlas.PhaseStartup ||
			trail.Authority != repositoryatlas.AuthorityResolved {
			return fmt.Errorf("llm: invalid Navigator startup trail")
		}
	}
	return nil
}

func decodeNavigatorJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}

func validNavigatorCatalogRef(value string) bool {
	const prefix = "navigator-v1-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	_, err := hex.DecodeString(value[len(prefix):])
	return err == nil
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || result[value] {
			continue
		}
		result[value] = true
	}
	return result
}
