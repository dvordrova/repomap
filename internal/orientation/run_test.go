package orientation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestRunRestoresAcceptedRowsToExactIDs(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{
			"summary":      "Alpha serves an items list that Beta fetches over HTTP.",
			"summary_refs": []string{refs.fact("route"), refs.claim("readme")},
			"roles": []any{
				map[string]any{"target": refs.target("alpha"), "role": "Backend API service",
					"purpose": "Serves the items list over HTTP.", "refs": []string{refs.fact("entrypoint"), refs.fact("route")}},
				map[string]any{"target": refs.target("beta"), "role": "HTTP client",
					"purpose": "Fetches items from Alpha.", "refs": []string{refs.fact("call"), refs.subject("beta", "core")}},
			},
			"run_recipe": []any{
				map[string]any{"target": refs.target("alpha"), "command": "go run ./alpha", "cwd": "alpha",
					"note": "Listens on PORT.", "refs": []string{refs.fact("manifest"), refs.fact("config")}},
			},
			"main_flow": map[string]any{
				"title": "From the items request to the response",
				"steps": []any{
					map[string]any{"target": refs.target("beta"), "ref": refs.fact("call"), "explanation": "Beta calls GET /api/items."},
					map[string]any{"target": refs.target("beta"), "ref": refs.fact("portal"), "explanation": "The call reaches Alpha's route."},
					map[string]any{"target": refs.target("alpha"), "ref": refs.fact("route"), "explanation": "Alpha handles the route in Serve."},
					map[string]any{"target": refs.target("alpha"), "ref": refs.subject("alpha", "core"), "explanation": "Apply computes the items."},
				},
			},
		})
	}}

	result, rejected, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, fixture.input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rejected) != 0 || result.RejectedCount != 0 {
		t.Fatalf("rejected = %#v", rejected)
	}
	if result.FactsSHA256 != fixture.input.Facts.SHA256 || result.ClaimsSHA256 != fixture.input.Claims.SHA256 {
		t.Fatalf("input digests not bound: %#v", result)
	}
	if !reflect.DeepEqual(result.GroupsSHA256s, sortedDigests(groupDigests(fixture.input.Groups))) {
		t.Fatalf("groups digests = %v", result.GroupsSHA256s)
	}
	if !reflect.DeepEqual(result.SummaryRefs, []string{fixture.factID("route"), fixture.claimID("readme")}) {
		t.Fatalf("summary refs = %v", result.SummaryRefs)
	}
	if len(result.Roles) != 2 ||
		!reflect.DeepEqual(result.Roles[0].FactIDs, []string{fixture.factID("entrypoint"), fixture.factID("route")}) ||
		result.Roles[0].TargetID != fixture.targetID("alpha") ||
		!reflect.DeepEqual(result.Roles[1].FactIDs, []string{fixture.factID("call")}) ||
		!reflect.DeepEqual(result.Roles[1].SubjectIDs, []string{fixture.subjectID("beta", "core")}) {
		t.Fatalf("roles = %#v", result.Roles)
	}
	if len(result.RunRecipe) != 1 || result.RunRecipe[0].Cwd != "alpha" ||
		!reflect.DeepEqual(result.RunRecipe[0].FactIDs, []string{fixture.factID("manifest"), fixture.factID("config")}) {
		t.Fatalf("recipe = %#v", result.RunRecipe)
	}
	if len(result.MainFlow.Steps) != 4 || result.MainFlow.Title == "" ||
		result.MainFlow.Steps[1].FactID != fixture.factID("portal") ||
		result.MainFlow.Steps[3].SubjectID != fixture.subjectID("alpha", "core") ||
		result.MainFlow.Steps[3].TargetID != fixture.targetID("alpha") {
		t.Fatalf("flow = %#v", result.MainFlow)
	}
	if provider.completions != 1 {
		t.Fatalf("provider completions = %d, want one call", provider.completions)
	}
	provider.assertRequestShape(t, fixture)
}

func TestRunRejectsRowsWithUnknownRefsAndKeepsSiblings(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{
			"summary": "Alpha and Beta.", "summary_refs": []string{"g1"},
			"roles": []any{
				map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves items.", "refs": []string{"f999"}},
				map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves items.", "refs": []string{refs.fact("route"), refs.fact("route")}},
				map[string]any{"target": refs.target("beta"), "role": "Client", "purpose": strings.Repeat("x", MaxSentenceRunes+1), "refs": []string{refs.fact("call")}},
				map[string]any{"target": refs.target("beta"), "role": "Client", "purpose": "Fetches items.", "refs": []string{refs.fact("call")}},
				map[string]any{"target": "t9", "role": "Ghost", "purpose": "Does not exist.", "refs": []string{refs.fact("call")}},
			},
		})
	}}

	result, rejected, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, fixture.input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Roles) != 1 || result.Roles[0].TargetID != fixture.targetID("beta") {
		t.Fatalf("accepted roles = %#v", result.Roles)
	}
	if result.Summary != "" || len(result.SummaryRefs) != 0 {
		t.Fatalf("summary citing a group ref was accepted: %q %v", result.Summary, result.SummaryRefs)
	}
	if len(rejected) != 5 || result.RejectedCount != 5 {
		t.Fatalf("rejected = %d rows: %#v", len(rejected), rejected)
	}
	expectReasons := map[string]string{
		"summary": "not allowed here", "f999": "unknown ref", "duplicate": "duplicate ref",
		"t9": "unknown target ref", "long": "at most",
	}
	seen := map[string]bool{}
	for _, row := range rejected {
		if row.Stage != StageName || len(row.Raw) == 0 || row.Reason == "" {
			t.Fatalf("rejected row is incomplete: %#v", row)
		}
		raw := string(row.Raw)
		switch {
		case row.Section == "summary":
			seen["summary"] = strings.Contains(row.Reason, expectReasons["summary"])
		case strings.Contains(raw, "f999"):
			seen["f999"] = strings.Contains(row.Reason, expectReasons["f999"])
		case strings.Contains(raw, `"t9"`):
			seen["t9"] = strings.Contains(row.Reason, expectReasons["t9"])
		case strings.Contains(raw, "xxxx"):
			seen["long"] = strings.Contains(row.Reason, expectReasons["long"])
		default:
			seen["duplicate"] = strings.Contains(row.Reason, expectReasons["duplicate"])
		}
	}
	for key := range expectReasons {
		if !seen[key] {
			t.Fatalf("rejection %q missing or has wrong reason: %#v", key, rejected)
		}
	}
}

func TestRunRejectsRecipeWithoutManifestOrEntrypointFact(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{
			"run_recipe": []any{
				map[string]any{"command": "go run ./alpha", "refs": []string{refs.fact("route")}},
				map[string]any{"command": "go run ./alpha", "refs": []string{refs.claim("readme")}},
				map[string]any{"command": "go run ./alpha", "cwd": "alpha", "refs": []string{refs.fact("entrypoint")}},
			},
		})
	}}
	result, rejected, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, fixture.input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.RunRecipe) != 1 || result.RunRecipe[0].FactIDs[0] != fixture.factID("entrypoint") {
		t.Fatalf("recipe = %#v", result.RunRecipe)
	}
	if len(rejected) != 2 || rejected[0].Section != "run_recipe" ||
		!strings.Contains(rejected[0].Reason, "manifest or entrypoint") ||
		!strings.Contains(rejected[1].Reason, "not allowed here") {
		t.Fatalf("rejected = %#v", rejected)
	}
}

func TestRunRejectsFlowStepCitingMemberOfAnotherTarget(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{
			"main_flow": map[string]any{
				"title": "Items flow",
				"steps": []any{
					map[string]any{"target": refs.target("beta"), "ref": refs.subject("alpha", "core"), "explanation": "Wrong target."},
					map[string]any{"target": refs.target("alpha"), "ref": refs.claim("readme"), "explanation": "Claims are not steps."},
					map[string]any{"target": refs.target("alpha"), "ref": refs.subject("alpha", "core"), "explanation": "Apply computes the items."},
				},
			},
		})
	}}
	result, rejected, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, fixture.input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.MainFlow.Steps) != 1 || result.MainFlow.Steps[0].SubjectID != fixture.subjectID("alpha", "core") ||
		result.MainFlow.Title != "Items flow" {
		t.Fatalf("flow = %#v", result.MainFlow)
	}
	if len(rejected) != 2 || !strings.Contains(rejected[0].Reason, "does not belong to target") ||
		!strings.Contains(rejected[1].Reason, "not allowed here") {
		t.Fatalf("rejected = %#v", rejected)
	}
}

func TestRunMalformedResponseIsAnError(t *testing.T) {
	fixture := newFixture(t)
	for name, raw := range map[string]string{
		"not json":      `{"summary": `,
		"unknown field": `{"summary":"x","summary_refs":[],"extra":1}`,
		"wrong type":    `{"roles":"none"}`,
	} {
		provider := &presetProvider{respond: func([]byte) []byte { return []byte(raw) }}
		if _, _, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, fixture.input); err == nil {
			t.Fatalf("%s: Run accepted a malformed response", name)
		}
	}
}

func TestRunAllRejectedYieldsEmptySealedResult(t *testing.T) {
	fixture := newFixture(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{
			"summary": "Unsupported.", "summary_refs": []string{"f404"},
			"roles":      []any{map[string]any{"target": "t1", "role": "X", "purpose": "Y", "refs": []string{}}},
			"run_recipe": []any{map[string]any{"command": "make", "refs": []string{"c404"}}},
			"main_flow": map[string]any{"title": "Nothing", "steps": []any{
				map[string]any{"target": "t1", "ref": "s404", "explanation": "Missing."},
			}},
		})
	}}
	result, rejected, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, fixture.input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rejected) != 5 {
		t.Fatalf("rejected = %#v", rejected)
	}
	want, err := Empty(fixture.input.Facts.SHA256, fixture.input.Claims.SHA256, groupDigests(fixture.input.Groups), len(rejected))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %#v, want Empty", result)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestRequestBytesAreDeterministicAndCloseOverRefs(t *testing.T) {
	fixture := newFixture(t)
	first, _, err := encodeRequest(fixture.input, requestShapes[0])
	if err != nil {
		t.Fatal(err)
	}
	reordered := fixture.input
	reordered.Groups = []groupindex.Index{fixture.input.Groups[1], fixture.input.Groups[0]}
	second, _, err := encodeRequest(reordered, requestShapes[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("request bytes depend on groups order:\n%s\n%s", first, second)
	}
	for _, id := range fixture.canonicalIDs() {
		if bytes.Contains(first, []byte(id)) {
			t.Fatalf("request leaks exact id %q", id)
		}
	}
	if !bytes.Contains(first, []byte(`"content_trust":"`+contentTrust+`"`)) {
		t.Fatal("request lacks the content trust sentence")
	}
}

func TestRunShrinksRequestBeforeGivingUp(t *testing.T) {
	fixture := newFixture(t)
	withoutClaims, _, err := encodeRequest(fixture.input, requestShapes[1])
	if err != nil {
		t.Fatal(err)
	}
	provider := &presetProvider{
		maximumUserBytes: len(withoutClaims),
		respond:          func([]byte) []byte { return encodeResponse(t, map[string]any{}) },
	}
	result, rejected, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, fixture.input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rejected) != 1 || rejected[0].Section != "request" || !strings.Contains(string(rejected[0].Raw), `"claims"`) {
		t.Fatalf("dropped rows = %#v", rejected)
	}
	if result.RejectedCount != 1 || !bytes.Equal(provider.users[len(provider.users)-1], withoutClaims) {
		t.Fatalf("model did not receive the claim-free request")
	}

	tiny := &presetProvider{maximumUserBytes: 16, respond: func([]byte) []byte { return nil }}
	_, _, err = Run(context.Background(), llm.Executor{Enabled: false}, tiny, fixture.input)
	var resourceErr *llm.ResourceLimitError
	if err == nil || !strings.Contains(err.Error(), "no fact was truncated") || !errors.As(err, &resourceErr) {
		t.Fatalf("oversized request error = %v", err)
	}
	if tiny.completions != 0 {
		t.Fatal("provider was called with an oversized request")
	}
}

func TestPersistRejectedRoundTrip(t *testing.T) {
	rows := []RejectedRow{
		{Stage: StageName, Section: "roles", Raw: json.RawMessage(`{"target":"t9","refs":["f1"]}`), Reason: `unknown target ref "t9"`},
		{Stage: StageName, Section: "request", Raw: json.RawMessage(`{"dropped":"claims","count":2}`), Reason: "too large"},
	}
	runDir := t.TempDir()
	if err := PersistRejected(runDir, rows); err != nil {
		t.Fatalf("PersistRejected: %v", err)
	}
	restored, err := ReadRejected(runDir)
	if err != nil {
		t.Fatalf("ReadRejected: %v", err)
	}
	if !reflect.DeepEqual(restored, rows) {
		t.Fatalf("restored = %#v, want %#v", restored, rows)
	}
	if err := PersistRejected(runDir, nil); err != nil {
		t.Fatalf("PersistRejected empty: %v", err)
	}
	if restored, err = ReadRejected(runDir); err != nil || len(restored) != 0 {
		t.Fatalf("empty round trip = %#v, %v", restored, err)
	}
}

func TestPromptKeepsRepositoryTextUntrustedAndAvoidsInternalVocabulary(t *testing.T) {
	lower := strings.ToLower(promptText)
	for _, fragment := range []string{"untrusted", "one sentence", "manifest", "entrypoint", `"main_flow"`, "never invent a ref"} {
		if !strings.Contains(lower, fragment) {
			t.Fatalf("prompt lacks %q", fragment)
		}
	}
	for _, banned := range []string{"retained", "source-bound", "authority", "projection", "selector", "outcome", "target contract"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("prompt contains banned word %q", banned)
		}
	}
}

// presetProvider answers with canned JSON and records every request it saw.
type presetProvider struct {
	respond          func(user []byte) []byte
	maximumUserBytes int

	mu          sync.Mutex
	users       [][]byte
	completions int
}

type presetPrepared struct {
	System string `json:"system"`
	User   string `json:"user"`
}

func (provider *presetProvider) State() []byte {
	return []byte(`{"provider":"orientation-preset-v1"}`)
}

func (provider *presetProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	if !prompt.ResponseFormatJSON || prompt.System != strings.TrimSpace(promptText) || prompt.User == "" ||
		limits.MaxRequestBytes != llm.SemanticRecordByteLimit ||
		limits.MaxResponseBytes != llm.ProviderResponseByteLimit || limits.MaxOutputTokens != maxOutputTokens {
		return llm.Prepared{}, fmt.Errorf("preset received invalid request contract")
	}
	if provider.maximumUserBytes > 0 && len(prompt.User) > provider.maximumUserBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Stage: "preset_prepare", Kind: llm.ResourceLimitRequestBytes,
			Limit: provider.maximumUserBytes, Observed: len(prompt.User), ObservedKnown: true,
		})
	}
	wire, err := json.Marshal(presetPrepared{System: prompt.System, User: prompt.User})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(wire)
}

func (provider *presetProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var prompt presetPrepared
	if err := json.Unmarshal(prepared.Bytes(), &prompt); err != nil {
		return llm.Completion{}, err
	}
	provider.mu.Lock()
	provider.completions++
	provider.users = append(provider.users, []byte(prompt.User))
	provider.mu.Unlock()
	return llm.Completion{
		Response: provider.respond([]byte(prompt.User)), FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

func (provider *presetProvider) assertRequestShape(t *testing.T, fixture *fixture) {
	t.Helper()
	var seen request
	if err := json.Unmarshal(provider.users[0], &seen); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if len(seen.Targets) != 2 || seen.Targets[0].Root != "alpha" || seen.Targets[1].Manifest != "beta/go.mod" {
		t.Fatalf("targets = %#v", seen.Targets)
	}
	if seen.OmittedFactCounts["import"] != 1 || seen.OmittedFactCounts["todo"] != 1 || len(seen.Facts) != 10 {
		t.Fatalf("facts = %d rows, omitted %v", len(seen.Facts), seen.OmittedFactCounts)
	}
	if len(seen.Claims) != 2 || seen.Claims[1].Source != "readme" ||
		seen.Claims[1].Text != strings.Repeat("a", MaxRequestClaimRunes-1)+"…" {
		t.Fatalf("claims = %#v", seen.Claims)
	}
	if len(seen.Groups) != 6 || len(seen.Connections) != 3 || seen.Groups[0].Target != "t1" || seen.Groups[3].Target != "t2" {
		t.Fatalf("groups = %d, connections = %d", len(seen.Groups), len(seen.Connections))
	}
	for _, group := range seen.Groups {
		if group.MemberCount != len(group.Members) || len(group.Members) == 0 || group.Members[0].Anchor == "" {
			t.Fatalf("group members = %#v", group)
		}
	}
	var portal factWire
	for _, fact := range seen.Facts {
		if fact.Kind == string(facts.KindPortal) {
			portal = fact
		}
	}
	if portal.Peer != "t1" || len(portal.Links) != 2 || portal.Anchor != "beta/main.go:10" {
		t.Fatalf("portal wire = %#v", portal)
	}
	if _, aliased := fixture.subjectIDs["alpha"]["core"]; !aliased {
		t.Fatal("fixture lost subject ids")
	}
}

func encodeResponse(t *testing.T, value map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// fixture is a two-target repository: alpha serves GET /api/items, beta calls it.
type fixture struct {
	input      Input
	targetIDs  map[string]string            // alpha/beta -> facts target id
	factIDs    map[string]string            // label -> fact id
	claimIDs   map[string]string            // label -> claim id
	subjectIDs map[string]map[string]string // target label -> source ref -> subject id
}

func (fixture *fixture) targetID(label string) string { return fixture.targetIDs[label] }
func (fixture *fixture) factID(label string) string   { return fixture.factIDs[label] }
func (fixture *fixture) claimID(label string) string  { return fixture.claimIDs[label] }
func (fixture *fixture) subjectID(target, label string) string {
	return fixture.subjectIDs[target][label]
}

func (fixture *fixture) canonicalIDs() []string {
	ids := []string{}
	for _, id := range fixture.targetIDs {
		ids = append(ids, id)
	}
	for _, id := range fixture.factIDs {
		ids = append(ids, id)
	}
	for _, id := range fixture.claimIDs {
		ids = append(ids, id)
	}
	for _, byLabel := range fixture.subjectIDs {
		for _, id := range byLabel {
			ids = append(ids, id)
		}
	}
	return ids
}

// refLookup inverts the request catalog so tests can speak in request refs.
type refLookup struct {
	fixture *fixture
	cat     catalog
}

func (fixture *fixture) refs(t *testing.T) refLookup {
	t.Helper()
	_, cat, err := buildRequest(fixture.input, requestShapes[0])
	if err != nil {
		t.Fatal(err)
	}
	return refLookup{fixture: fixture, cat: cat}
}

func (lookup refLookup) target(label string) string {
	for ref, id := range lookup.cat.targets {
		if id == lookup.fixture.targetID(label) {
			return ref
		}
	}
	return ""
}

func (lookup refLookup) fact(label string) string {
	for ref, entry := range lookup.cat.facts {
		if entry.id == lookup.fixture.factID(label) {
			return ref
		}
	}
	return ""
}

func (lookup refLookup) claim(label string) string {
	for ref, id := range lookup.cat.claims {
		if id == lookup.fixture.claimID(label) {
			return ref
		}
	}
	return ""
}

func (lookup refLookup) subject(target, label string) string {
	targetRef := lookup.target(target)
	for ref, entry := range lookup.cat.subjects {
		if entry.id == lookup.fixture.subjectID(target, label) && entry.targetRef == targetRef {
			return ref
		}
	}
	return ""
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	alphaProgram, alphaIDs := testProgramIndex(t, "alpha")
	betaProgram, betaIDs := testProgramIndex(t, "beta")
	alpha, _, err := groupindex.Build(alphaProgram, testProposals(alphaIDs))
	if err != nil {
		t.Fatalf("Build alpha: %v", err)
	}
	beta, _, err := groupindex.Build(betaProgram, testProposals(betaIDs))
	if err != nil {
		t.Fatalf("Build beta: %v", err)
	}
	groups, diagnostics, err := groupindex.WithConnections([]groupindex.Index{alpha, beta}, []groupindex.ConnectionInput{{
		From:         groupindex.Endpoint{TargetID: beta.Target.ID, GroupID: groupByTitle(t, beta.Groups, "Core flow").ID},
		To:           groupindex.Endpoint{TargetID: alpha.Target.ID, GroupID: groupByTitle(t, alpha.Groups, "Execution triggers").ID},
		SemanticKind: "uses_http_api_of", Label: "uses HTTP API", Summary: "Beta fetches items from Alpha.",
		SupportResolution: programindex.PatternValuePossible,
		Evidence: []groupindex.SubjectEndpoint{
			{TargetID: beta.Target.ID, SubjectID: betaIDs["pattern"]},
			{TargetID: alpha.Target.ID, SubjectID: alphaIDs["inbound"]},
		},
	}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("WithConnections: %v %#v", err, diagnostics)
	}

	result := &fixture{
		targetIDs: map[string]string{
			"alpha": facts.NewTargetID("go", "alpha", "alpha/go.mod"),
			"beta":  facts.NewTargetID("go", "beta", "beta/go.mod"),
		},
		factIDs:    map[string]string{},
		claimIDs:   map[string]string{},
		subjectIDs: map[string]map[string]string{"alpha": alphaIDs, "beta": betaIDs},
	}
	result.input = Input{
		RepositoryName: "example",
		Facts:          result.facts(t, alphaProgram.Target.ID, betaProgram.Target.ID),
		Claims:         result.claims(t),
		Groups:         groups,
	}
	return result
}

func (fixture *fixture) facts(t *testing.T, alphaProgramID, betaProgramID string) facts.Result {
	t.Helper()
	alpha, beta := fixture.targetID("alpha"), fixture.targetID("beta")
	anchor := func(path string, line int) *facts.Anchor { return &facts.Anchor{Path: path, Line: line} }
	fact := func(label string, kind facts.Kind, row facts.Fact, principal ...string) facts.Fact {
		row.Kind = kind
		path := ""
		if row.Anchor != nil {
			path = row.Anchor.Path
		}
		row.ID = facts.NewFactID(row.TargetID, kind, path, label, principal...)
		fixture.factIDs[label] = row.ID
		return row
	}
	rows := []facts.Fact{
		fact("entrypoint", facts.KindEntrypoint, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/main.go", 1), Symbol: "Serve", Key: "callable"}),
		fact("route", facts.KindHTTPRoute, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/main.go", 10), Method: "GET", Path: "/api/items", Symbol: "Serve"}),
		fact("call", facts.KindHTTPCall, facts.Fact{TargetID: beta, Anchor: anchor("beta/main.go", 10), Method: "GET", Path: "/api/items", Symbol: "Serve"}),
		fact("manifest", facts.KindManifest, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/go.mod", 1), Key: "module", Value: "example.com/alpha"}),
		fact("config", facts.KindConfigRead, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/main.go", 3), Key: "PORT", Value: "8080"}),
		fact("risk", facts.KindRisk, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/main.go", 5), Key: "exec", Symbol: "Apply"}),
		fact("negative", facts.KindNegative, facts.Fact{Key: facts.NegativeNoTests, Text: "no test files"}),
		fact("dependency", facts.KindDependency, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/main.go", 4), Key: "example.com/queue"}),
		fact("dead", facts.KindDeadModule, facts.Fact{TargetID: alpha, Path: "alpha/dead.go"}),
		fact("import", facts.KindImport, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/main.go", 2), Path: "alpha/util.go"}),
		fact("todo", facts.KindTODO, facts.Fact{TargetID: alpha, Anchor: anchor("alpha/main.go", 6), Text: "TODO: tidy"}),
	}
	rows = append(rows, fact("portal", facts.KindPortal, facts.Fact{
		TargetID: beta, PeerTargetID: alpha, Anchor: anchor("beta/main.go", 10), Method: "GET", Path: "/api/items",
		Refs: []string{fixture.factID("call"), fixture.factID("route")}, Evidence: []facts.Anchor{{Path: "alpha/main.go", Line: 10}},
		Resolution: facts.ResolutionExact,
	}))
	sealed, err := facts.Seal(facts.Result{
		Revision: strings.Repeat("1", 40),
		Targets: []facts.Target{
			{ID: alpha, ProgramTargetID: alphaProgramID, Language: "go", Name: "alpha", Kind: "package", Root: "alpha", Manifest: "alpha/go.mod", Anchor: facts.Anchor{Path: "alpha/main.go", Line: 1}},
			{ID: beta, ProgramTargetID: betaProgramID, Language: "go", Name: "beta", Kind: "package", Root: "beta", Manifest: "beta/go.mod", Anchor: facts.Anchor{Path: "beta/main.go", Line: 1}},
		},
		Facts: rows,
	})
	if err != nil {
		t.Fatalf("facts.Seal: %v", err)
	}
	return sealed
}

func (fixture *fixture) claims(t *testing.T) claims.Result {
	t.Helper()
	readme := claims.Claim{
		Source: claims.SourceReadme, Path: "README.md", Line: 1, Date: "2024-02-26", AgeDays: 10,
		Text: strings.Repeat("a", MaxRequestClaimRunes+50), TargetID: fixture.targetID("alpha"),
	}
	readme.ID = claims.NewClaimID(readme.Source, "README.md:1", readme.Text)
	commit := claims.Claim{Source: claims.SourceCommit, Commit: "abc1234", Text: "Add items route", Date: "2024-03-01"}
	commit.ID = claims.NewClaimID(commit.Source, commit.Commit, commit.Text)
	fixture.claimIDs["readme"], fixture.claimIDs["commit"] = readme.ID, commit.ID
	sealed, err := claims.Seal(claims.Result{Revision: strings.Repeat("1", 40), Claims: []claims.Claim{readme, commit}})
	if err != nil {
		t.Fatalf("claims.Seal: %v", err)
	}
	return sealed
}

// testProgramIndex mirrors the smallest enriched ProgramIndex used by the
// groupindex tests: one inbound Serve, a core Apply, and a queue dependency.
func testProgramIndex(t *testing.T, selector string) (programindex.Index, map[string]string) {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: selector + "/main.go", Line: line, Column: 1}
	}
	objects := []programindex.ObjectInput{
		{SourceRef: "inbound", Kind: programindex.ObjectFunction, Name: "Serve", Visibility: programindex.VisibilityPublic, Signature: "func Serve()", Location: location(1)},
		{SourceRef: "background", Kind: programindex.ObjectFunction, Name: "Consume", Visibility: programindex.VisibilityInternal, ContainerRef: "inbound", Location: location(2)},
		{SourceRef: "core", Kind: programindex.ObjectFunction, Name: "Apply", Visibility: programindex.VisibilityInternal, OwnerRef: "inbound", Location: location(3)},
		{
			SourceRef: "dependency", Kind: programindex.ObjectExternalSymbol, Name: "queue.Publish", Visibility: programindex.VisibilityPublic,
			External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "example.com/queue", Name: "Publish"}, Location: location(4),
			SymbolLinkIdentities: []programindex.SymbolLinkIdentityInput{{Domain: "go", Parts: []string{"example.com/queue", "Publish"}, Display: "queue.Publish"}},
		},
		{SourceRef: "ungrouped", Kind: programindex.ObjectFunction, Name: "DebugOnly", Visibility: programindex.VisibilityInternal, Location: location(6)},
	}
	relations := []programindex.RelationInput{{
		SourceRef: "dispatch", Kind: programindex.RelationCalls, FromRef: "inbound", ToRefs: []string{"core"},
		Resolution: programindex.ResolutionExact, Invocation: "sync", Location: location(10), TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "direct_call", Location: location(10)}}, WitnessesObserved: 1,
		PatternsObserved: 1,
		Patterns: []programindex.RelationPatternInput{{
			SourceRef: "dispatch-pattern", Form: programindex.PatternCall, Selector: "get", Location: location(10),
			ResultRef: "core", ReceiverRef: "ungrouped", ReceiverOriginRefs: []string{"dependency"},
			ReceiverOriginResolution: programindex.ResolutionExact, ReceiverOriginsObserved: 1,
			ArgumentsObserved: 1,
			Arguments: []programindex.PatternArgumentInput{
				{Position: 1, Kind: programindex.PatternLiteralString, Value: "/api/items"},
			},
		}},
	}}
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "package", Name: selector, Selector: "go:" + selector,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: selector + "/main.go"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "inbound", Kind: programindex.SeedCallable, Location: location(1)}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	ids := make(map[string]string)
	for _, object := range base.Objects {
		ids[object.SourceRef] = object.ID
	}
	for _, relation := range base.Relations {
		for _, pattern := range relation.Patterns {
			ids["pattern"] = pattern.ID
		}
	}
	enriched, err := programindex.Enrich(base, strings.Repeat("d", 64), []programindex.CategoryAssignment{
		{SubjectID: ids["inbound"], Categories: []programindex.Category{programindex.CategoryInbound}},
		{SubjectID: ids["background"], Categories: []programindex.Category{programindex.CategoryBackgroundActivity}},
		{SubjectID: ids["core"], Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: ids["dependency"], Categories: []programindex.Category{programindex.CategoryDependency}},
		{SubjectID: ids["pattern"], Categories: []programindex.Category{programindex.CategoryInbound, programindex.CategoryCore}},
	})
	if err != nil {
		t.Fatalf("programindex.Enrich: %v", err)
	}
	return enriched, ids
}

func testProposals(ids map[string]string) groupindex.Proposals {
	return groupindex.Proposals{
		Groups: []groupindex.GroupProposal{
			{
				Key: "triggers", Title: "Execution triggers", Summary: "Inbound and background execution starts here.", Lane: groupindex.LaneTriggers,
				MemberSubjectIDs: []string{ids["background"], ids["inbound"], ids["pattern"]}, EvidenceSubjectIDs: []string{ids["core"]},
			},
			{
				Key: "core", Title: "Core flow", Summary: "The main product work.", Lane: groupindex.LaneCore,
				MemberSubjectIDs: []string{ids["core"], ids["pattern"]}, EvidenceSubjectIDs: []string{ids["inbound"]},
			},
			{
				Key: "dependencies", Title: "Queue dependency", Summary: "External queue boundary and its local caller.", Lane: groupindex.LaneDependencies,
				MemberSubjectIDs: []string{ids["dependency"]}, EvidenceSubjectIDs: []string{ids["core"], ids["pattern"]},
			},
		},
		Connections: []groupindex.ConnectionProposal{{
			FromGroupKey: "triggers", ToGroupKey: "core", SemanticKind: "awakens_domain_flow",
			Label: "Awakens", Summary: "Execution triggers enter the core flow.", EvidenceSubjectIDs: []string{ids["pattern"]},
		}},
	}
}

func groupByTitle(t *testing.T, groups []groupindex.Group, title string) groupindex.Group {
	t.Helper()
	for _, group := range groups {
		if group.Title == title {
			return group
		}
	}
	t.Fatalf("group %q not found", title)
	return groupindex.Group{}
}
