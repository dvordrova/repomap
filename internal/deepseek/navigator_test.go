package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestNavigatorPromptJSONPreservesClosedProductContract(t *testing.T) {
	t.Parallel()
	product, compiled := navigatorFixture(t)
	client := &Client{
		Endpoint: "https://api.deepseek.com/chat/completions",
		Model:    "deepseek-v4-flash", MaxTokens: 4096,
	}
	body, err := client.NavigatorPromptJSON(compiled.WireJSON(), compiled.MaxWireBytes())
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.Model != client.Model || request.MaxTokens != client.MaxTokens ||
		request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" ||
		request.Thinking == nil || request.Thinking.Type != "disabled" || len(request.Messages) != 2 {
		t.Fatalf("Navigator chat request = %#v", request)
	}
	if !strings.Contains(request.Messages[0].Content, NavigatorPromptVersionJSON) &&
		!strings.Contains(request.Messages[1].Content, NavigatorPromptVersionJSON) {
		t.Fatal("Navigator prompt version is absent")
	}
	user := request.Messages[1].Content
	for _, want := range []string{
		navigator.ProductQuestion, string(compiled.WireJSON()),
		canonicalEnglishUserContract,
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("Navigator user prompt missing %q: %s", want, user)
		}
	}
	for _, want := range []string{`"trail_refs"`, `"entity_refs"`, `"evidence_refs"`, "request-local"} {
		if !strings.Contains(request.Messages[0].Content, want) {
			t.Fatalf("Navigator system prompt missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"surface-secret-canonical", "operation-secret-canonical", "relation-secret-canonical",
		"evidence-secret-canonical", "cmd/private/main.go", "source_signals", "file_tree", "internal_edges",
	} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("Navigator prompt leaked %q: %s", forbidden, body)
		}
	}
	if len(product.Actions()) != 1 {
		t.Fatalf("fixture Product actions = %#v", product.Actions())
	}
}

func TestNavigatorPromptJSONRejectsNonProductWire(t *testing.T) {
	t.Parallel()
	_, compiled := navigatorFixture(t)
	client := &Client{Model: "fixture-model", MaxTokens: 1024}
	wire := compiled.WireJSON()
	unknown := append(append([]byte(nil), wire[:len(wire)-1]...), []byte(`,"unexpected":true}`)...)
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "malformed", data: []byte(`{"version":`), want: "decode Navigator wire"},
		{name: "wrong question", data: bytes.ReplaceAll(wire, []byte(navigator.ProductQuestion), []byte("another question")), want: "identity"},
		{name: "non-resolved trail", data: bytes.ReplaceAll(wire, []byte(`"authority":"resolved"`), []byte(`"authority":"inferred"`)), want: "startup trail"},
		{name: "unknown field", data: unknown, want: "unknown field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.NavigatorPromptJSON(test.data, compiled.MaxWireBytes())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NavigatorPromptJSON error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNavigatorPromptJSONEnforcesFinalProviderBodyBudget(t *testing.T) {
	t.Parallel()
	_, compiled := navigatorFixture(t)
	client := &Client{Model: "fixture-model", MaxTokens: 1024}
	body, err := client.NavigatorPromptJSON(compiled.WireJSON(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= len(compiled.WireJSON()) {
		t.Fatalf("provider body bytes = %d, wire bytes = %d", len(body), len(compiled.WireJSON()))
	}
	_, err = client.NavigatorPromptJSON(compiled.WireJSON(), len(body)-1)
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Stage != "navigator" ||
		limitErr.Kind != ResourceLimitRequestBytes || limitErr.Limit != len(body)-1 ||
		limitErr.Observed != len(body) || !limitErr.ObservedKnown {
		t.Fatalf("final request resource error = %#v / %v", limitErr, err)
	}
}

func TestDecodeNavigatorResponseRequiresOneExactStartupExplanation(t *testing.T) {
	t.Parallel()
	_, compiled := navigatorFixture(t)
	valid := navigatorProviderResponse(t, compiled)
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeNavigatorResponse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, encoded) {
		t.Fatalf("decoded response changed bytes: %s / %s", decoded, encoded)
	}

	tests := []struct {
		name   string
		mutate func(*navigatorResponse)
		want   string
	}{
		{name: "wrong version", mutate: func(value *navigatorResponse) { value.Version++ }, want: "identity"},
		{name: "zero action", mutate: func(value *navigatorResponse) { value.ActionRefs = nil }, want: "exactly one"},
		{name: "multiple actions", mutate: func(value *navigatorResponse) { value.ActionRefs = append(value.ActionRefs, "a9999") }, want: "exactly one"},
		{name: "no trail", mutate: func(value *navigatorResponse) { value.TrailRefs = nil }, want: "one exact startup trail"},
		{name: "no evidence", mutate: func(value *navigatorResponse) { value.EvidenceRefs = nil }, want: "one exact startup trail"},
		{name: "gap", mutate: func(value *navigatorResponse) { value.GapRefs = []string{"g0001"} }, want: "one exact startup trail"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneNavigatorResponse(valid)
			test.mutate(&candidate)
			data, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			_, err = DecodeNavigatorResponse(data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeNavigatorResponse error = %v, want %q", err, test.want)
			}
		})
	}
	unknown := append(append([]byte(nil), encoded[:len(encoded)-1]...), []byte(`,"unexpected":true}`)...)
	if _, err := DecodeNavigatorResponse(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown response field error = %v", err)
	}
	trailing := append(append([]byte(nil), encoded...), []byte(` {}`)...)
	if _, err := DecodeNavigatorResponse(trailing); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("trailing response error = %v", err)
	}
	oversized := bytes.Repeat([]byte{'x'}, maxProviderResponseBytes+1)
	_, err = DecodeNavigatorResponse(oversized)
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Stage != "navigator" ||
		limitErr.Kind != ResourceLimitResponseBytes || limitErr.Limit != maxProviderResponseBytes ||
		limitErr.Observed != len(oversized) || !limitErr.ObservedKnown {
		t.Fatalf("oversized Navigator response error = %#v / %v", limitErr, err)
	}
}

func TestNavigateMeasuredRetriesOnlyImmutableTransportAndResolvesLocally(t *testing.T) {
	product, compiled := navigatorFixture(t)
	response := navigatorProviderResponse(t, compiled)
	responseJSON, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		Endpoint: "https://provider.example.test/v1/chat/completions", Auth: authNone,
		Model: "fixture-model", MaxTokens: 2048,
	}
	wantBody, err := client.NavigatorPromptJSON(compiled.WireJSON(), compiled.MaxWireBytes())
	if err != nil {
		t.Fatal(err)
	}
	var bodies [][]byte
	client.HTTPClient = &http.Client{Transport: localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, append([]byte(nil), body...))
		if len(bodies) == 1 {
			return localizationHTTPResponse(request, http.StatusServiceUnavailable, `{"error":"busy"}`), nil
		}
		return localizationHTTPResponse(request, http.StatusOK, localizationProviderEnvelope(string(responseJSON))), nil
	})}
	result, err := client.NavigateMeasured(t.Context(), compiled.WireJSON(), compiled.MaxWireBytes())
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempts != 2 || len(bodies) != 2 || string(result.Content) != string(responseJSON) {
		t.Fatalf("NavigateMeasured calls/result = %d/%#v", len(bodies), result)
	}
	for index, body := range bodies {
		if !bytes.Equal(body, wantBody) {
			t.Fatalf("transport retry mutated body %d\ngot: %s\nwant: %s", index, body, wantBody)
		}
	}
	decoded, err := DecodeNavigatorResponse(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	record, err := product.ResolveRecommendation(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if record.Selected == nil || record.Selected.Operation != navigator.StartupActionOperation {
		t.Fatalf("resolved recommendation = %#v", record)
	}
}

func TestNavigateSemanticDecodeFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	_, compiled := navigatorFixture(t)
	invalid := navigatorProviderResponse(t, compiled)
	invalid.ActionRefs = nil
	invalidJSON, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	client := &Client{
		Endpoint: "https://provider.example.test/v1/chat/completions", Auth: authNone,
		Model: "fixture-model", MaxTokens: 2048,
		HTTPClient: &http.Client{Transport: localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			return localizationHTTPResponse(request, http.StatusOK, localizationProviderEnvelope(string(invalidJSON))), nil
		})},
	}
	result, err := client.NavigateMeasured(t.Context(), compiled.WireJSON(), compiled.MaxWireBytes())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Attempts != 1 {
		t.Fatalf("semantic invalid response calls/result = %d/%#v", calls, result)
	}
	if _, err := DecodeNavigatorResponse(result.Content); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("terminal decode error = %v", err)
	}
}

func TestNavigateOutputResourceFailureIsTerminalAndAnnotated(t *testing.T) {
	t.Parallel()
	_, compiled := navigatorFixture(t)
	responseJSON, err := json.Marshal(navigatorProviderResponse(t, compiled))
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	client := &Client{
		Endpoint: "https://provider.example.test/v1/chat/completions", Auth: authNone,
		Model: "fixture-model", MaxTokens: 2048,
		HTTPClient: &http.Client{Transport: localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			content, _ := json.Marshal(string(responseJSON))
			body := `{"choices":[{"finish_reason":"length","message":{"content":` + string(content) +
				`}}],"usage":{"prompt_tokens":100,"completion_tokens":2048}}`
			return localizationHTTPResponse(request, http.StatusOK, body), nil
		})},
	}
	result, err := client.NavigateMeasured(t.Context(), compiled.WireJSON(), compiled.MaxWireBytes())
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("NavigateMeasured error = %v, want ResourceLimitError", err)
	}
	if calls != 1 || result.Attempts != 1 || limitErr.Stage != "navigator" ||
		limitErr.Kind != ResourceLimitOutputTokens || limitErr.Limit != client.MaxTokens ||
		limitErr.FinishReason != "length" {
		t.Fatalf("resource calls/result/error = %d/%#v/%#v", calls, result, limitErr)
	}
}

func navigatorFixture(t *testing.T) (navigator.Product, navigator.Compiled) {
	t.Helper()
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "repository-secret-canonical", Kind: repositoryatlas.UnitRepository, Name: "fixture"},
			{ID: "module-secret-canonical", Kind: repositoryatlas.UnitModule, ParentID: "repository-secret-canonical", Name: "example.com/fixture"},
			{ID: "app-secret-canonical", Kind: repositoryatlas.UnitApp, ParentID: "module-secret-canonical", Name: "example.com/fixture/cmd/app"},
		},
		Entities: []repositoryatlas.Entity{
			{ID: "surface-secret-canonical", Kind: repositoryatlas.EntitySurface, UnitID: "app-secret-canonical"},
			{ID: "operation-secret-canonical", Kind: repositoryatlas.EntityOperation, UnitID: "app-secret-canonical"},
		},
		Observations: []repositoryatlas.Observation{
			{ID: "observation-surface-secret", UnitID: "app-secret-canonical", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-secret-canonical"}, EvidenceRefs: []string{"evidence-secret-canonical"}},
			{ID: "observation-operation-secret", UnitID: "app-secret-canonical", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation-secret-canonical"}, EvidenceRefs: []string{"evidence-secret-canonical"}},
		},
		Evidence: []repositoryatlas.Evidence{{
			ID: "evidence-secret-canonical", UnitID: "app-secret-canonical",
			Location: evidence.Location{Path: "cmd/private/main.go", Line: 7}, Symbol: "example.com/fixture/cmd/app.main",
			Provenance: evidence.Provenance{Provider: "fixture", Operation: "build_selected_main_declaration"},
		}},
		Relations: []repositoryatlas.Relation{{
			ID: "relation-secret-canonical", UnitID: "app-secret-canonical", Kind: repositoryatlas.RelationExposes,
			Source: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-secret-canonical"},
			Target: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation-secret-canonical"},
			Phase:  repositoryatlas.PhaseStartup, Authority: repositoryatlas.AuthorityResolved,
			EvidenceRefs: []string{"evidence-secret-canonical"},
		}},
	}
	product, err := navigator.CompileProduct(navigator.ProductInput{
		Atlas: atlas,
		Limits: navigator.Limits{
			MaxWireBytes: 128 << 10, MaxResponseBytes: 128 << 10, MaxUnitLabelBytes: 512,
			MaxSeeds: 8, MaxDirectTrails: 8, MaxIntersections: 8, MaxEvidence: 8, MaxGaps: 0, MaxActions: 8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled, ok := product.CompiledRequest()
	if !ok {
		t.Fatal("fixture did not produce a compiled Navigator request")
	}
	return product, compiled
}

func navigatorProviderResponse(t *testing.T, compiled navigator.Compiled) navigatorResponse {
	t.Helper()
	var wire navigatorWireRequest
	if err := json.Unmarshal(compiled.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Actions) != 1 || len(wire.DirectTrails) != 1 {
		t.Fatalf("fixture wire = %#v", wire)
	}
	trail := wire.DirectTrails[0]
	return navigatorResponse{
		Version: navigator.Version, CatalogRef: compiled.CatalogRef(),
		EntityRefs: []string{trail.SourceRef, trail.TargetRef}, TrailRefs: []string{trail.Ref},
		EvidenceRefs: append([]string(nil), trail.EvidenceRefs...), ActionRefs: []string{wire.Actions[0].Ref},
	}
}

func cloneNavigatorResponse(value navigatorResponse) navigatorResponse {
	cloned := value
	cloned.EntityRefs = append([]string(nil), value.EntityRefs...)
	cloned.TrailRefs = append([]string(nil), value.TrailRefs...)
	cloned.IntersectionRefs = append([]string(nil), value.IntersectionRefs...)
	cloned.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
	cloned.GapRefs = append([]string(nil), value.GapRefs...)
	cloned.ActionRefs = append([]string(nil), value.ActionRefs...)
	return cloned
}
