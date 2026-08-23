package activitysurface

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/llm"
)

type testProvider struct {
	prompt        llm.Prompt
	limits        llm.Limits
	prepareCalls  int
	completeCalls int
	respond       func(Request) []byte
}

func (provider *testProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"fixture"}`)
}

func (provider *testProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	provider.prepareCalls++
	provider.prompt = prompt
	provider.limits = limits
	encoded, err := json.Marshal(struct {
		System string `json:"system"`
		User   string `json:"user"`
	}{System: prompt.System, User: prompt.User})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(encoded)
}

func (provider *testProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	provider.completeCalls++
	var request Request
	if err := json.Unmarshal([]byte(provider.prompt.User), &request); err != nil {
		return llm.Completion{}, err
	}
	response := []byte(`{"surface_proposals":[]}`)
	if provider.respond != nil {
		response = provider.respond(request)
	}
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{
			InputTokens: 20, OutputTokens: 10, ProviderResponseBytes: len(response),
			UsageReported: true, Latency: time.Millisecond, Attempts: 1,
		},
	}, nil
}

func TestRunSendsOnlyBoundedSurfaceCatalogAndRestoresExactActivities(t *testing.T) {
	substrate := fixtureSubstrate()
	var responseWire []byte
	provider := &testProvider{respond: func(request Request) []byte {
		t.Helper()
		if request.Version != RequestVersion || request.Catalog.Version != entrycall.SurfaceRequestVersion {
			t.Fatalf("request = %#v", request)
		}
		route := requestCandidateWithValue(t, request, "/v1/users")
		command := requestCandidateWithValue(t, request, "serve [address]")
		job := requestCandidateWithValue(t, request, "@every 5m")
		response := Response{SurfaceProposals: []entrycall.ResponseSurfaceProposal{
			{
				CandidateRef: job.Ref, KindRef: entrycall.SurfaceKindRefScheduledJob,
				Bindings: []entrycall.ResponseSurfaceBinding{
					{SlotRef: entrycall.SurfaceSlotRefIdentity, FactRef: requestFactRef(t, job, "@every 5m")},
					{SlotRef: entrycall.SurfaceSlotRefHandler, FactRef: requestFactRef(t, job, "runCleanup")},
				},
			},
			{
				CandidateRef: route.Ref, KindRef: entrycall.SurfaceKindRefHTTPRoute,
				Bindings: []entrycall.ResponseSurfaceBinding{
					{SlotRef: entrycall.SurfaceSlotRefMethod, FactRef: requestFactRef(t, route, "POST")},
					{SlotRef: entrycall.SurfaceSlotRefPath, FactRef: requestFactRef(t, route, "/v1/users")},
					{SlotRef: entrycall.SurfaceSlotRefHandler, FactRef: requestFactRef(t, route, "createUser")},
				},
			},
			{
				CandidateRef: command.Ref, KindRef: entrycall.SurfaceKindRefCLICommand,
				Bindings: []entrycall.ResponseSurfaceBinding{
					{SlotRef: entrycall.SurfaceSlotRefIdentity, FactRef: requestFactRef(t, command, "serve [address]")},
					{SlotRef: entrycall.SurfaceSlotRefHandler, FactRef: requestFactRef(t, command, "runServe")},
				},
			},
		}}
		responseWire, _ = json.Marshal(response)
		return responseWire
	}}

	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, substrate)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if provider.prepareCalls != 1 || provider.completeCalls != 1 || !provider.prompt.ResponseFormatJSON {
		t.Fatalf("provider calls/prompt = %d/%d %#v", provider.prepareCalls, provider.completeCalls, provider.prompt)
	}
	if provider.limits.MaxRequestBytes != entrycall.MaxProviderRequestBytes ||
		provider.limits.MaxResponseBytes != entrycall.MaxResponseBytes ||
		provider.limits.MaxOutputTokens != maxOutputTokens {
		t.Fatalf("limits = %#v", provider.limits)
	}
	for _, required := range []string{
		"Candidates are evidence, not framework or runtime proof",
		"Omission means uncertain",
		"Generic callbacks, event hooks, consumer registrations",
	} {
		if !strings.Contains(provider.prompt.System, required) {
			t.Fatalf("prompt omitted %q", required)
		}
	}
	for _, private := range []string{"root-node", "candidate-route", "routes.go", `"entries"`, `"families"`} {
		if strings.Contains(provider.prompt.User, private) {
			t.Fatalf("provider-visible request leaked private authority %q: %s", private, provider.prompt.User)
		}
	}
	for _, forbidden := range []string{"/v1/users", "serve [address]", "@every 5m", "createUser"} {
		if strings.Contains(string(responseWire), forbidden) {
			t.Fatalf("response was not refs-only; found %q in %s", forbidden, responseWire)
		}
	}
	if len(result.Surfaces) != 3 || result.Coverage.Selected != 3 ||
		result.Coverage.Rejected != 0 || !result.Coverage.ModelCalled {
		t.Fatalf("result = %#v", result)
	}
	byKind := make(map[string]Surface, len(result.Surfaces))
	for _, surface := range result.Surfaces {
		byKind[surface.Kind] = surface
		if surface.RootNodeID != "root-node" {
			t.Fatalf("surface lost exact root: %#v", surface)
		}
	}
	route := byKind[entrycall.SurfaceKindHTTPRoute]
	if route.Registration != location("routes.go", 10) || route.Method == nil || route.Method.Text != "POST" ||
		route.Path == nil || route.Path.Text != "/v1/users" || route.Handler == nil || route.Handler.Text != "createUser" ||
		route.Role != entrycall.SurfaceRoleEntrySurface {
		t.Fatalf("route = %#v", route)
	}
	command := byKind[entrycall.SurfaceKindCLICommand]
	if command.Registration != location("commands.go", 20) || command.Identity == nil ||
		command.Identity.Text != "serve [address]" || command.Handler == nil || command.Handler.Text != "runServe" ||
		command.Role != entrycall.SurfaceRoleDescriptor {
		t.Fatalf("command = %#v", command)
	}
	job := byKind[entrycall.SurfaceKindScheduledJob]
	if job.Registration != location("jobs.go", 30) || job.Identity == nil || job.Identity.Text != "@every 5m" ||
		job.Handler == nil || job.Handler.Text != "runCleanup" || job.Role != entrycall.SurfaceRoleEntrySurface {
		t.Fatalf("job = %#v", job)
	}
	if err := result.ValidateAgainst(substrate); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, requestRefField := range []string{"candidate_ref", "root_ref", "kind_ref", "slot_ref", "fact_ref"} {
		if strings.Contains(string(encoded), requestRefField) {
			t.Fatalf("local result retained request ref field %q: %s", requestRefField, encoded)
		}
	}

	tampered := result
	tampered.Surfaces = append([]Surface(nil), result.Surfaces...)
	for index := range tampered.Surfaces {
		if tampered.Surfaces[index].Kind != entrycall.SurfaceKindHTTPRoute {
			continue
		}
		path := *tampered.Surfaces[index].Path
		path.Text = "/v1/tampered"
		tampered.Surfaces[index].Path = &path
	}
	if err := tampered.ValidateAgainst(substrate); err == nil || !strings.Contains(err.Error(), "exact authority") {
		t.Fatalf("tampered exact result error = %v", err)
	}

	tamperedMethod := result
	tamperedMethod.Surfaces = append([]Surface(nil), result.Surfaces...)
	for index := range tamperedMethod.Surfaces {
		if tamperedMethod.Surfaces[index].Kind != entrycall.SurfaceKindHTTPRoute {
			continue
		}
		method := *tamperedMethod.Surfaces[index].Method
		method.Text = "BREW"
		tamperedMethod.Surfaces[index].Method = &method
	}
	if err := tamperedMethod.Validate(); err == nil || !strings.Contains(err.Error(), "HTTP route") {
		t.Fatalf("non-standard token method error = %v", err)
	}
}

func TestRunRejectsUnknownRefsAndUnknownResponseFields(t *testing.T) {
	for name, response := range map[string][]byte{
		"unknown candidate": []byte(`{"surface_proposals":[{"candidate_ref":"c999","kind_ref":"k2","bindings":[]}]}`),
		"unknown field":     []byte(`{"surface_proposals":[],"explanation":"looks good"}`),
	} {
		t.Run(name, func(t *testing.T) {
			provider := &testProvider{respond: func(Request) []byte { return response }}
			_, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, fixtureSubstrate())
			if err == nil {
				t.Fatal("Run accepted invalid refs-only response")
			}
			if name == "unknown candidate" && !strings.Contains(err.Error(), "unknown surface candidate ref") {
				t.Fatalf("unknown-ref error = %v", err)
			}
			if name == "unknown field" && !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("unknown-field error = %v", err)
			}
		})
	}
}

func TestRunFailsClosedOnAnyIncompatibleProposal(t *testing.T) {
	provider := &testProvider{respond: func(request Request) []byte {
		route := requestCandidateWithValue(t, request, "/v1/users")
		job := requestCandidateWithValue(t, request, "@every 5m")
		response := Response{SurfaceProposals: []entrycall.ResponseSurfaceProposal{
			{
				CandidateRef: route.Ref, KindRef: entrycall.SurfaceKindRefCLICommand,
				Bindings: []entrycall.ResponseSurfaceBinding{{
					SlotRef: entrycall.SurfaceSlotRefIdentity, FactRef: requestFactRef(t, route, "/v1/users"),
				}},
			},
			{
				CandidateRef: job.Ref, KindRef: entrycall.SurfaceKindRefScheduledJob,
				Bindings: []entrycall.ResponseSurfaceBinding{
					{SlotRef: entrycall.SurfaceSlotRefIdentity, FactRef: requestFactRef(t, job, "@every 5m")},
					{SlotRef: entrycall.SurfaceSlotRefHandler, FactRef: requestFactRef(t, job, "runCleanup")},
				},
			},
		}}
		wire, _ := json.Marshal(response)
		return wire
	}}
	_, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, fixtureSubstrate())
	if err == nil || !strings.Contains(err.Error(), "invalid surface proposal") ||
		!strings.Contains(err.Error(), string(entrycall.RejectedSurfaceIncompatibleForm)) {
		t.Fatalf("incompatible proposal error = %v", err)
	}
}

func TestRunSkipsProviderWhenNoCandidatesAreAdvertised(t *testing.T) {
	substrate := fixtureSubstrate()
	substrate.SurfaceCandidates = []entrycall.ExactSurfaceCandidate{}
	substrate.Coverage.SurfaceCandidatesConsidered = 0
	substrate.Coverage.SurfaceCandidatesIndexed = 0
	substrate.Coverage.SurfaceCandidateFactsConsidered = 0
	substrate.Coverage.SurfaceCandidateFactsIndexed = 0

	result, err := Run(t.Context(), llm.Executor{Enabled: true}, nil, substrate)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Surfaces) != 0 || len(result.Rejections) != 0 ||
		result.Coverage.ModelCalled || result.Coverage.Candidates.AdvertisedCandidates != 0 {
		t.Fatalf("empty result = %#v", result)
	}
	if err := result.ValidateAgainst(substrate); err != nil {
		t.Fatalf("ValidateAgainst empty result: %v", err)
	}
}

func TestRunReturnsClosedEmptyResultForUnavailableSubstrate(t *testing.T) {
	substrate := entrycall.Unavailable(entrycall.ClosedNoEntrypoints)
	result, err := Run(t.Context(), llm.Executor{Enabled: true}, nil, substrate)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != entrycall.StateUnavailable || result.ClosedReason != entrycall.ClosedNoEntrypoints ||
		len(result.Surfaces) != 0 || len(result.Rejections) != 0 || result.Coverage != (Coverage{}) {
		t.Fatalf("unavailable result = %#v", result)
	}
	if err := result.ValidateAgainst(substrate); err != nil {
		t.Fatalf("ValidateAgainst unavailable result: %v", err)
	}
	tampered := result
	tampered.ClosedReason = entrycall.ClosedSSAUnavailable
	if err := tampered.ValidateAgainst(substrate); err == nil || !strings.Contains(err.Error(), "binding mismatch") {
		t.Fatalf("tampered unavailable result error = %v", err)
	}
}

func TestRunRejectsCandidateOmittedFromProviderCatalogBeforeModelCall(t *testing.T) {
	substrate := fixtureSubstrate()
	hiddenSite := location("hidden.go", 70)
	hiddenIdentity := fact(
		"hidden-command-identity", entrycall.SurfaceFactString, 1,
		"Use", "hidden", hiddenSite,
	)
	substrate.SurfaceCandidates = append(substrate.SurfaceCandidates, entrycall.ExactSurfaceCandidate{
		ID: "hidden-command", RootNodeID: "root-node",
		Form: entrycall.SurfaceCandidateKeyedComposite, Sketch: "Descriptor", Site: hiddenSite,
		Facts: []entrycall.ExactSurfaceFact{hiddenIdentity},
	})
	substrate.Coverage.SurfaceCandidatesConsidered++
	substrate.Coverage.SurfaceCandidatesIndexed++
	substrate.Coverage.SurfaceCandidateFactsConsidered++
	substrate.Coverage.SurfaceCandidateFactsIndexed++

	_, err := Run(t.Context(), llm.Executor{Enabled: true}, nil, substrate)
	if err == nil ||
		!strings.Contains(err.Error(), "bounded candidate catalog is incomplete") ||
		!strings.Contains(err.Error(), "candidates considered=4 advertised=3 omitted=1") ||
		!strings.Contains(err.Error(), "facts considered=10 advertised=9 omitted=1") {
		t.Fatalf("incomplete candidate catalog error = %v", err)
	}
}

func fixtureSubstrate() entrycall.Substrate {
	routeSite := location("routes.go", 10)
	commandSite := location("commands.go", 20)
	jobSite := location("jobs.go", 30)
	candidates := []entrycall.ExactSurfaceCandidate{
		{
			ID: "candidate-route", RootNodeID: "root-node", Form: entrycall.SurfaceCandidateDirectCall,
			Sketch: "Handle", Site: routeSite,
			Facts: []entrycall.ExactSurfaceFact{
				fact("route-selector", entrycall.SurfaceFactToken, 0, "terminal selector", "Handle", routeSite),
				fact("route-method", entrycall.SurfaceFactToken, 1, "argument 1 method", "POST", routeSite),
				fact("route-path", entrycall.SurfaceFactString, 1, "argument 1 path", "/v1/users", routeSite),
				fact("route-handler", entrycall.SurfaceFactCallable, 2, "argument 2", "createUser", location("handlers.go", 40)),
			},
		},
		{
			ID: "candidate-command", RootNodeID: "root-node", Form: entrycall.SurfaceCandidateKeyedComposite,
			Sketch: "Descriptor", Site: commandSite,
			Facts: []entrycall.ExactSurfaceFact{
				fact("command-use", entrycall.SurfaceFactString, 1, "Use", "serve [address]", commandSite),
				fact("command-handler", entrycall.SurfaceFactCallable, 2, "RunE", "runServe", location("commands.go", 50)),
			},
		},
		{
			ID: "candidate-job", RootNodeID: "root-node", Form: entrycall.SurfaceCandidateDirectCall,
			Sketch: "AddFunc", Site: jobSite,
			Facts: []entrycall.ExactSurfaceFact{
				fact("job-selector", entrycall.SurfaceFactToken, 0, "terminal selector", "AddFunc", jobSite),
				fact("job-identity", entrycall.SurfaceFactString, 1, "argument 1", "@every 5m", jobSite),
				fact("job-handler", entrycall.SurfaceFactCallable, 2, "argument 2", "runCleanup", location("jobs.go", 60)),
			},
		},
	}
	facts := 0
	for _, candidate := range candidates {
		facts += len(candidate.Facts)
	}
	return entrycall.Substrate{
		Version: entrycall.SubstrateVersion, State: entrycall.StateReady,
		Roots:             []entrycall.ExactRoot{{NodeID: "root-node"}},
		SurfaceCandidates: candidates,
		Coverage: entrycall.Coverage{
			RootsConsidered:             1,
			SurfaceCandidatesConsidered: len(candidates), SurfaceCandidatesIndexed: len(candidates),
			SurfaceCandidateFactsConsidered: facts, SurfaceCandidateFactsIndexed: facts,
		},
	}
}

func fact(
	id string,
	kind entrycall.SurfaceFactKind,
	position int,
	label, value string,
	where entrycall.Location,
) entrycall.ExactSurfaceFact {
	return entrycall.ExactSurfaceFact{
		ID: id, Kind: kind, Position: position, Label: label, Value: value, Location: where,
	}
}

func location(path string, line int) entrycall.Location {
	return entrycall.Location{Path: path, Line: line, Column: 1}
}

func requestCandidateWithValue(t *testing.T, request Request, value string) entrycall.RequestSurfaceCandidate {
	t.Helper()
	for _, candidate := range request.Catalog.Candidates {
		for _, fact := range candidate.Facts {
			if fact.Value == value {
				return candidate
			}
		}
	}
	t.Fatalf("candidate with value %q not found in %#v", value, request.Catalog.Candidates)
	return entrycall.RequestSurfaceCandidate{}
}

func requestFactRef(t *testing.T, candidate entrycall.RequestSurfaceCandidate, value string) string {
	t.Helper()
	for _, fact := range candidate.Facts {
		if fact.Value == value {
			return fact.Ref
		}
	}
	t.Fatalf("fact %q not found in candidate %s: %#v", value, candidate.Ref, candidate)
	return fmt.Sprintf("missing-%s", value)
}
