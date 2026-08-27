package runtimeportfolio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/secretscan"
)

func TestSimpleSingleTargetTopology(t *testing.T) {
	input := singleTargetInput()
	compilation := mustCompile(t, input)
	wire := compilation.wire
	for _, localOnly := range []string{
		input.CapturedRevision,
		input.TargetPagePortfolioSHA256,
		input.Targets[0].ProgramTargetID,
	} {
		if bytes.Contains(wire, []byte(localOnly)) {
			t.Fatalf("provider request leaked local authority %q: %s", localOnly, wire)
		}
	}
	if compilation.Request.TargetCount != 1 || len(compilation.Request.Targets) != 1 ||
		compilation.Request.Targets[0].Ref != "t1" || len(compilation.Request.EvidenceCatalog) != 3 {
		t.Fatalf("request = %#v", compilation.Request)
	}
	if !strings.Contains(systemPrompt, "Prefer a simple topology") ||
		!strings.Contains(systemPrompt, "Do not promote every package") ||
		!strings.Contains(systemPrompt, "Use only advertised `t*` and `e*` refs") {
		t.Fatalf("system prompt lost topology or closed-ref contract: %s", systemPrompt)
	}

	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})
	if len(result.Roles) != 1 || len(result.Roles[0].Implementations) != 1 ||
		result.Roles[0].Implementations[0].ProgramTargetID != input.Targets[0].ProgramTargetID ||
		len(result.UnclassifiedTargetIDs) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.Coverage != (Coverage{
		TargetsObserved: 1, TargetsMapped: 1, TargetsUnclassified: 0, Roles: 1,
		EvidenceAdvertised: 3, EvidenceSelected: 1,
	}) {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
	if err := result.ValidateAgainst(input); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
}

func TestExampleRoleIsExplicitAndAlwaysSupporting(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	evidenceRef := evidenceRefForLabel(t, compilation, "Starts the API server")
	role := responseRole{
		Name: "Tutorial application", Purpose: "Demonstrates the supported API in a runnable sample.",
		Prominence: ProminencePrimary, Kind: RoleKindExample,
		Requiredness: RequirednessOptional, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRef},
	}
	if _, err := ResolveResponse(compilation, mustMarshalResponse(t, role)); err == nil ||
		!strings.Contains(err.Error(), "not supporting") {
		t.Fatalf("ResolveResponse accepted a primary example role: %v", err)
	}
	role.Prominence = ProminenceSupporting
	result := mustResolve(t, compilation, role)
	if len(result.Roles) != 1 || result.Roles[0].Kind != RoleKindExample ||
		result.Roles[0].Prominence != ProminenceSupporting {
		t.Fatalf("example role = %#v", result.Roles)
	}
}

func TestResolvePreservesSeveralModesOnOneTarget(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Runs the API and migration modes.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus: MappingMapped,
		Implementations: []responseImplementation{
			{TargetRef: "t1", Mode: "serve"},
			{TargetRef: "t1", Mode: "migrate"},
		},
		EvidenceRefs: []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})
	got := result.Roles[0].Implementations
	want := []Implementation{
		{ProgramTargetID: "program-target-server", Mode: "migrate"},
		{ProgramTargetID: "program-target-server", Mode: "serve"},
	}
	if !reflect.DeepEqual(got, want) || result.Coverage.TargetsMapped != 1 {
		t.Fatalf("implementations = %#v, coverage = %#v", got, result.Coverage)
	}
}

func TestResolvePreservesOneRoleAcrossSeveralTargets(t *testing.T) {
	input := twoTargetInput()
	compilation := mustCompile(t, input)
	serverEvidence := evidenceRefForLabel(t, compilation, "Starts the API server")
	workerEvidence := evidenceRefForLabel(t, compilation, "Starts the tuple worker")
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization runtime", Purpose: "Serves requests and processes tuple work.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t2"}, {TargetRef: "t1"}},
		EvidenceRefs:    []string{workerEvidence, serverEvidence},
	})
	if got := result.Roles[0].Implementations; !reflect.DeepEqual(got, []Implementation{
		{ProgramTargetID: "program-target-server"},
		{ProgramTargetID: "program-target-worker"},
	}) {
		t.Fatalf("implementations = %#v", got)
	}
	if len(result.UnclassifiedTargetIDs) != 0 || result.Coverage.TargetsMapped != 2 {
		t.Fatalf("coverage = %#v, unclassified = %v", result.Coverage, result.UnclassifiedTargetIDs)
	}
}

func TestResolveFiltersUnknownRefsAndDeduplicatesSets(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	evidenceRef := evidenceRefForLabel(t, compilation, "Starts the API server")
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus: MappingMapped,
		Implementations: []responseImplementation{
			{TargetRef: "unknown-target"},
			{TargetRef: "t1", Mode: "serve"},
			{TargetRef: "t1", Mode: "serve"},
		},
		EvidenceRefs: []string{"unknown-evidence", evidenceRef, evidenceRef},
	})
	if got := result.Roles[0]; len(got.Implementations) != 1 || len(got.Evidence) != 1 ||
		got.Implementations[0].ProgramTargetID != "program-target-server" ||
		got.Implementations[0].Mode != "serve" {
		t.Fatalf("filtered role = %#v", got)
	}
}

func TestResolveRejectsUnresolvedOrIncompatibleMandatoryMappings(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	evidenceRef := evidenceRefForLabel(t, compilation, "Starts the API server")
	tests := map[string]responseRole{
		"mapped role loses every target": {
			Name: "Service", Purpose: "Serves requests.", Prominence: ProminencePrimary,
			Kind: RoleKindService, Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
			MappingStatus:   MappingMapped,
			Implementations: []responseImplementation{{TargetRef: "unknown-target"}},
			EvidenceRefs:    []string{evidenceRef},
		},
		"unknown mapping selects known target": {
			Name: "Unresolved role", Purpose: "May serve requests.", Prominence: ProminenceUnknown,
			Kind: RoleKindUnknown, Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
			MappingStatus:   MappingUnknown,
			Implementations: []responseImplementation{{TargetRef: "t1"}},
			EvidenceRefs:    []string{evidenceRef},
		},
		"role loses every evidence ref": {
			Name: "Service", Purpose: "Serves requests.", Prominence: ProminencePrimary,
			Kind: RoleKindService, Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
			MappingStatus:   MappingMapped,
			Implementations: []responseImplementation{{TargetRef: "t1"}},
			EvidenceRefs:    []string{"unknown-evidence"},
		},
	}
	for name, role := range tests {
		t.Run(name, func(t *testing.T) {
			raw := mustMarshalResponse(t, role)
			if _, err := ResolveResponse(compilation, raw); err == nil {
				t.Fatal("ResolveResponse accepted unresolved mandatory semantics")
			}
		})
	}
}

func TestResolveAllowsUnknownMappingAndEmptyRoles(t *testing.T) {
	input := twoTargetInput()
	compilation := mustCompile(t, input)
	evidenceRef := evidenceRefForLabel(t, compilation, "Repository runs one server and one worker")
	unknown := mustResolve(t, compilation, responseRole{
		Name: "Unresolved operator", Purpose: "The repository mentions an operator role.",
		Prominence: ProminenceUnknown, Kind: RoleKindUnknown,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
		MappingStatus: MappingUnknown, Implementations: []responseImplementation{},
		EvidenceRefs: []string{evidenceRef},
	})
	if len(unknown.Roles) != 1 || unknown.Roles[0].Implementations == nil ||
		len(unknown.Roles[0].Implementations) != 0 ||
		!slices.Equal(unknown.UnclassifiedTargetIDs, []string{"program-target-server", "program-target-worker"}) {
		t.Fatalf("unknown mapping = %#v", unknown)
	}

	empty, err := ResolveResponse(compilation, []byte(`{"roles":[]}`))
	if err != nil {
		t.Fatalf("empty roles: %v", err)
	}
	if empty.Roles == nil || len(empty.Roles) != 0 ||
		!slices.Equal(empty.UnclassifiedTargetIDs, []string{"program-target-server", "program-target-worker"}) ||
		empty.Coverage.Roles != 0 || empty.Coverage.TargetsMapped != 0 ||
		empty.Coverage.TargetsUnclassified != 2 {
		t.Fatalf("empty result = %#v", empty)
	}
}

func TestLibraryOnlyTargetSupportsEvidenceBackedLibraryRoleAndAllowsEmptyPortfolio(t *testing.T) {
	input := singleTargetInput()
	input.Targets[0].DisplayName = "client API"
	input.Targets[0].Kind = "module_library"
	input.Targets[0].Selector = "example.com/authorization/client::module_library"
	input.Targets[0].ActivityStarts = 0
	input.Targets[0].Evidence[0] = EvidenceInput{
		Kind: EvidenceProgramFact, Label: "Exports the supported client API",
		Location:        Location{Path: "client/client.go", Line: 17, Column: 1},
		ProgramTargetID: input.Targets[0].ProgramTargetID,
	}
	input.Targets[0].Evidence = append(input.Targets[0].Evidence, EvidenceInput{
		Kind: EvidenceTargetEntrypoint, Label: "Selects the client package target",
		Location:        Location{Path: "client/client.go", Line: 1, Column: 1},
		ProgramTargetID: input.Targets[0].ProgramTargetID,
	})
	input.Targets[0].Responsibilities[0] = ResponsibilityInput{
		Name: "Client API", Purpose: "Provides reusable authorization client operations.",
		Evidence: []EvidenceInput{{
			Kind: EvidenceResponsibility, Label: "Provides reusable authorization client operations",
			Location:        Location{Path: "client/check.go", Line: 21, Column: 1},
			ProgramTargetID: input.Targets[0].ProgramTargetID,
		}},
	}
	input.RepositoryEvidence[0].Label = "Repository publishes a supported authorization client library"
	compilation := mustCompile(t, input)

	empty, err := ResolveResponse(compilation, []byte(`{"roles":[]}`))
	if err != nil {
		t.Fatalf("library-only empty portfolio: %v", err)
	}
	if len(empty.Roles) != 0 ||
		!slices.Equal(empty.UnclassifiedTargetIDs, []string{"program-target-server"}) ||
		empty.Coverage.TargetsMapped != 0 || empty.Coverage.TargetsUnclassified != 1 {
		t.Fatalf("library-only empty result = %#v", empty)
	}

	evidenceRef := evidenceRefForLabel(t, compilation, "Exports the supported client API")
	repositoryEvidenceRef := evidenceRefForLabel(
		t, compilation, "Repository publishes a supported authorization client library",
	)
	result := mustResolve(t, compilation, responseRole{
		Name: "Client library", Purpose: "Provides the client API.",
		Prominence: ProminencePrimary, Kind: RoleKindLibrary,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRef, repositoryEvidenceRef},
	})
	if len(result.Roles) != 1 || result.Roles[0].Kind != RoleKindLibrary ||
		result.Coverage.TargetsMapped != 1 || len(result.UnclassifiedTargetIDs) != 0 {
		t.Fatalf("library result = %#v", result)
	}

	unsupported := responseRole{
		Name: "Client library", Purpose: "Provides the client API.",
		Prominence: ProminencePrimary, Kind: RoleKindLibrary,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{repositoryEvidenceRef},
	}
	if _, err := ResolveResponse(compilation, mustMarshalResponse(t, unsupported)); err == nil || !strings.Contains(err.Error(), "target-bound role evidence") {
		t.Fatalf("runtime portfolio accepted a library implementation without target-bound evidence: %v", err)
	}

	t.Run("target binding without library semantics", func(t *testing.T) {
		unsupported.EvidenceRefs = []string{
			evidenceRefForLabel(t, compilation, "Selects the client package target"),
		}
		if _, err := ResolveResponse(compilation, mustMarshalResponse(t, unsupported)); err == nil || !strings.Contains(err.Error(), "responsibility or program-fact evidence") {
			t.Fatalf("runtime portfolio accepted a library without exact semantic evidence: %v", err)
		}
	})

	t.Run("executable mode", func(t *testing.T) {
		unsupported.Implementations[0].Mode = "serve"
		unsupported.EvidenceRefs = []string{evidenceRef}
		if _, err := ResolveResponse(compilation, mustMarshalResponse(t, unsupported)); err == nil || !strings.Contains(err.Error(), "library implementation has an executable mode") {
			t.Fatalf("runtime portfolio accepted an executable library mode: %v", err)
		}
	})
}

func TestMapCandidateFiltersUnsupportedImplementationSetMembers(t *testing.T) {
	compilation := mustCompile(t, twoTargetInput())
	serverResponsibility := evidenceRefForLabel(t, compilation, "Evaluates authorization requests")
	serverEntrypoint := evidenceRefForLabel(t, compilation, "Starts the API server")
	workerEntrypoint := evidenceRefForLabel(t, compilation, "Starts the tuple worker")
	repositoryEvidence := evidenceRefForLabel(t, compilation, "Repository runs one server and one worker")

	partiallySupportedLibrary := responseRole{
		Name: "Reusable server API", Purpose: "Provides a reusable server API.",
		Prominence: ProminenceUnknown, Kind: RoleKindLibrary,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceHigh,
		MappingStatus: MappingMapped,
		Implementations: []responseImplementation{
			{TargetRef: "t1"},
			{TargetRef: "t2"},
		},
		EvidenceRefs: []string{serverResponsibility, workerEntrypoint},
	}
	fullyUnsupportedLibrary := responseRole{
		Name: "Unproven worker library", Purpose: "Claims a reusable worker API.",
		Prominence: ProminenceUnknown, Kind: RoleKindLibrary,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t2"}},
		EvidenceRefs:    []string{workerEntrypoint},
	}
	partiallySupportedRuntime := responseRole{
		Name: "Runtime modes", Purpose: "Runs the server and worker modes.",
		Prominence: ProminenceUnknown, Kind: RoleKindService,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceHigh,
		MappingStatus: MappingMapped,
		Implementations: []responseImplementation{
			{TargetRef: "t1", Mode: "serve"},
			{TargetRef: "t2", Mode: "consume"},
		},
		EvidenceRefs: []string{serverEntrypoint},
	}
	fullyUnsupportedRuntime := responseRole{
		Name: "Unbound administration CLI", Purpose: "Claims an administration mode.",
		Prominence: ProminenceUnknown, Kind: RoleKindCLI,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t2", Mode: "administer"}},
		EvidenceRefs:    []string{repositoryEvidence},
	}
	validSibling := responseRole{
		Name: "Tuple worker", Purpose: "Processes tuple work.",
		Prominence: ProminenceSupporting, Kind: RoleKindWorker,
		Requiredness: RequirednessOptional, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t2"}},
		EvidenceRefs:    []string{workerEntrypoint},
	}

	roles, err := resolveMapCandidateResponse(
		mustMarshalResponse(
			t, partiallySupportedLibrary, fullyUnsupportedLibrary,
			partiallySupportedRuntime, fullyUnsupportedRuntime, validSibling,
		),
		compilation.targetsByRef,
		compilation.evidenceByRef,
		compilation.targetsByRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 3 {
		t.Fatalf("filtered map candidates = %#v, want three supported roles", roles)
	}
	byName := make(map[string]Role, len(roles))
	for _, role := range roles {
		byName[role.Name] = role
	}
	library, found := byName[partiallySupportedLibrary.Name]
	if !found || !reflect.DeepEqual(library.Implementations, []Implementation{{
		ProgramTargetID: "program-target-server",
	}}) {
		t.Fatalf("partially supported library = %#v", library)
	}
	if _, found := byName[fullyUnsupportedLibrary.Name]; found {
		t.Fatalf("unsupported library candidate survived = %#v", byName[fullyUnsupportedLibrary.Name])
	}
	runtime, found := byName[partiallySupportedRuntime.Name]
	if !found || !reflect.DeepEqual(runtime.Implementations, []Implementation{{
		ProgramTargetID: "program-target-server", Mode: "serve",
	}}) {
		t.Fatalf("partially supported runtime = %#v", runtime)
	}
	if _, found := byName[fullyUnsupportedRuntime.Name]; found {
		t.Fatalf("unsupported runtime candidate survived = %#v", byName[fullyUnsupportedRuntime.Name])
	}
	if _, found := byName[validSibling.Name]; !found {
		t.Fatalf("independently valid sibling was discarded: %#v", roles)
	}

	t.Run("advertised target outside detailed shard is unsupported", func(t *testing.T) {
		detailedTargets := map[string]Target{"t1": compilation.targetsByRef["t1"]}
		detailedEvidence := make(map[string]Evidence)
		for ref, evidence := range compilation.evidenceByRef {
			if evidence.ProgramTargetID == "" ||
				evidence.ProgramTargetID == detailedTargets["t1"].ProgramTargetID {
				detailedEvidence[ref] = evidence
			}
		}
		roles, err := resolveMapCandidateResponse(
			mustMarshalResponse(t, fullyUnsupportedRuntime, partiallySupportedRuntime),
			detailedTargets,
			detailedEvidence,
			compilation.targetsByRef,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(roles) != 1 || roles[0].Name != partiallySupportedRuntime.Name ||
			!reflect.DeepEqual(roles[0].Implementations, []Implementation{{
				ProgramTargetID: "program-target-server", Mode: "serve",
			}}) {
			t.Fatalf("outside-shard filtering = %#v", roles)
		}
	})

	t.Run("does not repair incompatible executable mode", func(t *testing.T) {
		incompatible := partiallySupportedLibrary
		incompatible.Implementations = []responseImplementation{{TargetRef: "t1", Mode: "serve"}}
		if _, err := resolveMapCandidateResponse(
			mustMarshalResponse(t, incompatible),
			compilation.targetsByRef,
			compilation.evidenceByRef,
			compilation.targetsByRef,
		); err == nil || !strings.Contains(err.Error(), "library implementation has an executable mode") {
			t.Fatalf("map candidate repaired executable library mode: %v", err)
		}
	})

	t.Run("does not hide invalid executable mode", func(t *testing.T) {
		invalid := fullyUnsupportedRuntime
		invalid.Implementations[0].Mode = " invalid "
		if _, err := resolveMapCandidateResponse(
			mustMarshalResponse(t, invalid),
			compilation.targetsByRef,
			compilation.evidenceByRef,
			compilation.targetsByRef,
		); err == nil || !strings.Contains(err.Error(), "invalid executable mode") {
			t.Fatalf("map candidate hid invalid executable mode: %v", err)
		}
	})

	for _, kind := range []RoleKind{RoleKindExample, RoleKindSupportingTool} {
		t.Run("does not hide primary "+string(kind), func(t *testing.T) {
			invalid := fullyUnsupportedRuntime
			invalid.Name = "Invalid " + string(kind)
			invalid.Kind = kind
			invalid.Prominence = ProminencePrimary
			if _, err := resolveMapCandidateResponse(
				mustMarshalResponse(t, invalid),
				compilation.targetsByRef,
				compilation.evidenceByRef,
				compilation.targetsByRef,
			); err == nil || !strings.Contains(err.Error(), "not supporting") {
				t.Fatalf("map candidate hid primary %s: %v", kind, err)
			}
		})
	}

	t.Run("does not repair unresolved mandatory mapping", func(t *testing.T) {
		unresolved := partiallySupportedLibrary
		unresolved.Implementations = []responseImplementation{{TargetRef: "unknown-target"}}
		if _, err := resolveMapCandidateResponse(
			mustMarshalResponse(t, unresolved),
			compilation.targetsByRef,
			compilation.evidenceByRef,
			compilation.targetsByRef,
		); err == nil || !strings.Contains(err.Error(), "mapped role unresolved") {
			t.Fatalf("map candidate repaired unresolved mandatory mapping: %v", err)
		}
	})
}

func TestArtifactEncodeDecodeAndAuthorityTamper(t *testing.T) {
	input := singleTargetInput()
	compilation := mustCompile(t, input)
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, result) {
		t.Fatalf("Decode = %#v, err = %v", decoded, err)
	}
	if digest, err := result.ArtifactSHA256(); err != nil || !validSHA256(digest) {
		t.Fatalf("ArtifactSHA256 = %q, %v", digest, err)
	}
	for name, suffix := range map[string][]byte{
		"second value":     []byte(`{"extra":true}`),
		"malformed tail":   []byte(`{`),
		"extra whitespace": []byte(` `),
	} {
		t.Run(name, func(t *testing.T) {
			tampered := append(append([]byte(nil), encoded...), suffix...)
			if _, err := Decode(tampered); err == nil {
				t.Fatal("Decode accepted trailing artifact data")
			}
		})
	}
	minified, err := result.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(minified); err == nil {
		t.Fatal("Decode accepted valid but noncanonical artifact bytes")
	}
	if _, err := Decode(make([]byte, MaxArtifactBytes+1)); err == nil {
		t.Fatal("Decode accepted an artifact outside the domain byte envelope")
	}

	tests := map[string]func(Result) Result{
		"target metadata": func(value Result) Result {
			value.Targets[0].DisplayName = "substituted server"
			return value
		},
		"portfolio identity": func(value Result) Result {
			value.TargetPagePortfolioSHA256 = strings.Repeat("c", 64)
			return value
		},
		"advertised evidence count": func(value Result) Result {
			value.Coverage.EvidenceAdvertised++
			return value
		},
		"invented evidence": func(value Result) Result {
			value.Roles[0].Evidence[0].Label = "Invented source claim"
			value.Roles[0].Evidence[0].ID, _ = evidenceID(value.Roles[0].Evidence[0])
			value.Roles[0].ID, _ = roleID(value.Roles[0])
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			tampered := mutate(cloneResult(result))
			if err := tampered.Validate(); err != nil {
				t.Fatalf("tamper should remain standalone-valid to exercise authority check: %v", err)
			}
			if err := tampered.ValidateAgainst(input); err == nil {
				t.Fatal("ValidateAgainst accepted substituted authority")
			}
		})
	}
}

func TestResolveRejectsLocalRestorationBeyondArtifactEnvelope(t *testing.T) {
	input := singleTargetInput()
	input.Targets[0].Evidence[0].Label = strings.Repeat("e", 500)
	input.Targets[0].Evidence[0].Location.Path = "evidence/" + strings.Repeat("a", 490)
	compilation := mustCompile(t, input)
	evidenceRef := evidenceRefForLabel(t, compilation, strings.Repeat("e", 500))
	roles := make([]responseRole, 0, 4000)
	for index := range 4000 {
		roles = append(roles, responseRole{
			Name: fmt.Sprintf("Runtime role %04d", index), Purpose: "Runs one evidenced mode.",
			Prominence: ProminenceSupporting, Kind: RoleKindSupportingTool,
			Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
			MappingStatus:   MappingMapped,
			Implementations: []responseImplementation{{TargetRef: "t1"}},
			EvidenceRefs:    []string{evidenceRef},
		})
	}
	raw := mustMarshalResponse(t, roles...)
	if len(raw) >= MaxResponseBytes {
		t.Fatalf("test response no longer exercises local expansion: %d bytes", len(raw))
	}
	if _, err := ResolveResponse(compilation, raw); err == nil || !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("ResolveResponse accepted an unpersistable restored result: %v", err)
	}
}

func TestCompileCanonicalizesInputAndBindsCacheIdentity(t *testing.T) {
	input := twoTargetInput()
	input.RepositoryEvidence = append(input.RepositoryEvidence, EvidenceInput{
		Kind: EvidenceConfiguration, Label: "Worker mode is configured explicitly",
		Location: Location{Path: "config/defaults.yaml", Line: 7},
	})
	canonical := mustCompile(t, input)
	if len(canonical.wire) > MaxRequestBytes || len(canonical.mapShards) != 0 {
		t.Fatalf("bounded legacy request unexpectedly compiled a sharded plan")
	}

	permuted := cloneInput(input)
	slices.Reverse(permuted.Targets)
	slices.Reverse(permuted.RepositoryEvidence)
	permuted.RepositoryEvidence = append(permuted.RepositoryEvidence, permuted.RepositoryEvidence[0])
	permuted.Targets[0].Responsibilities = append(
		permuted.Targets[0].Responsibilities,
		permuted.Targets[0].Responsibilities[0],
	)
	permuted.Targets[0].Evidence = append(permuted.Targets[0].Evidence, permuted.Targets[0].Evidence[0])
	reordered := mustCompile(t, permuted)
	if canonical.RequestSHA256 != reordered.RequestSHA256 ||
		!bytes.Equal(canonical.wire, reordered.wire) || !bytes.Equal(canonical.state, reordered.state) ||
		canonical.seal != reordered.seal {
		t.Fatalf("equivalent input changed identity:\ncanonical=%s\nreordered=%s", canonical.wire, reordered.wire)
	}

	revisionChanged := cloneInput(input)
	revisionChanged.CapturedRevision = strings.Repeat("d", 40)
	revision := mustCompile(t, revisionChanged)
	if revision.RequestSHA256 != canonical.RequestSHA256 || bytes.Equal(revision.state, canonical.state) ||
		revision.seal == canonical.seal {
		t.Fatal("captured revision should remain provider-local and invalidate cube cache state")
	}

	portfolioChanged := cloneInput(input)
	portfolioChanged.TargetPagePortfolioSHA256 = strings.Repeat("e", 64)
	portfolio := mustCompile(t, portfolioChanged)
	if portfolio.RequestSHA256 != canonical.RequestSHA256 || !bytes.Equal(portfolio.wire, canonical.wire) ||
		!bytes.Equal(portfolio.state, canonical.state) || portfolio.seal == canonical.seal {
		t.Fatal("publication-only portfolio identity changed semantic cache identity or lost its local compilation binding")
	}

	factsChanged := cloneInput(input)
	factsChanged.Targets[0].ProgramObjects++
	facts := mustCompile(t, factsChanged)
	if facts.RequestSHA256 == canonical.RequestSHA256 || bytes.Equal(facts.state, canonical.state) ||
		facts.seal == canonical.seal {
		t.Fatal("provider-visible fact change did not invalidate request and cache state")
	}

	evidenceChanged := cloneInput(input)
	evidenceChanged.RepositoryEvidence[0].Label = "Repository runs a server and an independently configured worker"
	evidence := mustCompile(t, evidenceChanged)
	if evidence.RequestSHA256 == canonical.RequestSHA256 || bytes.Equal(evidence.state, canonical.state) ||
		evidence.seal == canonical.seal {
		t.Fatal("advertised evidence change did not invalidate request and cache state")
	}
}

func TestExecutionIdentityChangesInvalidateSemanticState(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	current := currentExecutionIdentity()
	baseline, err := executionStateWithIdentity(compilation.input, compilation.wire, current)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]executionIdentity{
		"execution contract":       current,
		"prompt identity":          current,
		"preparation identity":     current,
		"response schema identity": current,
	}
	changed := tests["execution contract"]
	changed.Contract += "-changed"
	tests["execution contract"] = changed
	changed = tests["prompt identity"]
	changed.PromptVersion += "-changed"
	tests["prompt identity"] = changed
	changed = tests["preparation identity"]
	changed.PreparationVersion++
	tests["preparation identity"] = changed
	changed = tests["response schema identity"]
	changed.ResponseSchemaVersion++
	tests["response schema identity"] = changed
	for name, identity := range tests {
		t.Run(name, func(t *testing.T) {
			state, stateErr := executionStateWithIdentity(compilation.input, compilation.wire, identity)
			if stateErr != nil {
				t.Fatal(stateErr)
			}
			if bytes.Equal(state, baseline) {
				t.Fatal("identity change did not invalidate semantic state")
			}
		})
	}
}

func TestRunUsesSharedExecutorCacheWithCubeIdentity(t *testing.T) {
	input := singleTargetInput()
	compilation := mustCompile(t, input)
	provider := &runtimeTestProvider{response: mustMarshalResponse(t, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})}
	executor := llm.Executor{RootDir: t.TempDir(), Enabled: true}
	first, err := Run(t.Context(), executor, provider, input)
	if err != nil || first.Cached || provider.completeCalls != 1 {
		t.Fatalf("first = %#v, calls = %d, err = %v", first, provider.completeCalls, err)
	}
	warmInput := cloneInput(input)
	warmInput.TargetPagePortfolioSHA256 = strings.Repeat("c", 64)
	warm, err := Run(t.Context(), executor, provider, warmInput)
	if err != nil || !warm.Cached || warm.CacheKey != first.CacheKey || provider.completeCalls != 1 {
		t.Fatalf("warm = %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
	}
	if warm.SemanticCalls != 0 || warm.Metrics.Attempts != 0 || warm.Metrics.Latency != 0 ||
		warm.Metrics.InputTokens != first.Metrics.InputTokens ||
		warm.Metrics.OutputTokens != first.Metrics.OutputTokens ||
		warm.Metrics.ProviderResponseBytes != first.Metrics.ProviderResponseBytes ||
		warm.Metrics.UsageReported != first.Metrics.UsageReported {
		t.Fatalf("warm cache usage/current transport accounting = %#v, first = %#v", warm.Metrics, first.Metrics)
	}
	if !bytes.Equal(first.Request, warm.Request) || first.RequestSHA256 != warm.RequestSHA256 ||
		first.ResponseSHA256 != warm.ResponseSHA256 || !reflect.DeepEqual(first.Value.Roles, warm.Value.Roles) ||
		!reflect.DeepEqual(first.Value.Targets, warm.Value.Targets) || first.Value.Coverage != warm.Value.Coverage {
		t.Fatal("publication-only warm execution changed provider bytes or canonical semantic role payload")
	}
	if first.Value.TargetPagePortfolioSHA256 != input.TargetPagePortfolioSHA256 ||
		warm.Value.TargetPagePortfolioSHA256 != warmInput.TargetPagePortfolioSHA256 ||
		first.Value.ValidateAgainst(input) != nil || warm.Value.ValidateAgainst(warmInput) != nil ||
		warm.Value.ValidateAgainst(input) == nil || first.Value.ValidateAgainst(warmInput) == nil {
		t.Fatal("live or cached result lost its current publication binding")
	}
	changed := cloneInput(input)
	changed.CapturedRevision = strings.Repeat("d", 40)
	invalidated, err := Run(t.Context(), executor, provider, changed)
	if err != nil || invalidated.Cached || invalidated.CacheKey == first.CacheKey || provider.completeCalls != 2 {
		t.Fatalf("invalidated = %#v, calls = %d, err = %v", invalidated, provider.completeCalls, err)
	}
	changedFacts := cloneInput(input)
	changedFacts.Targets[0].ProgramObjects++
	factsInvalidated, err := Run(t.Context(), executor, provider, changedFacts)
	if err != nil || factsInvalidated.Cached || factsInvalidated.CacheKey == first.CacheKey || provider.completeCalls != 3 {
		t.Fatalf("facts invalidated = %#v, calls = %d, err = %v", factsInvalidated, provider.completeCalls, err)
	}
}

func TestRunShardsOversizedPortfolioAndRestoresGlobalAuthority(t *testing.T) {
	const (
		targetCount               = 25
		responsibilitiesPerTarget = 150
	)
	input := oversizedRuntimePortfolioInput(targetCount, responsibilitiesPerTarget)
	compilation, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.wire) <= MaxRequestBytes || len(compilation.mapShards) < 2 {
		t.Fatalf(
			"oversized compilation = %d bytes / %d map shards, want > %d bytes / multiple shards",
			len(compilation.wire), len(compilation.mapShards), MaxRequestBytes,
		)
	}
	for index, shard := range compilation.mapShards {
		if len(shard.wire) == 0 || len(shard.wire) > MaxRequestBytes {
			t.Fatalf("map shard %d bytes = %d", index+1, len(shard.wire))
		}
		if len(shard.request.TargetCatalog) != targetCount ||
			len(shard.request.RepositoryEvidence) != len(input.RepositoryEvidence) {
			t.Fatalf(
				"map shard %d global context = %d targets / %d repository evidence",
				index+1, len(shard.request.TargetCatalog), len(shard.request.RepositoryEvidence),
			)
		}
	}
	permuted := cloneInput(input)
	slices.Reverse(permuted.Targets)
	slices.Reverse(permuted.RepositoryEvidence)
	recompiled := mustCompile(t, permuted)
	if !bytes.Equal(compilation.wire, recompiled.wire) ||
		len(compilation.mapShards) != len(recompiled.mapShards) {
		t.Fatal("equivalent oversized input changed its canonical request or shard count")
	}
	for index := range compilation.mapShards {
		if !bytes.Equal(compilation.mapShards[index].wire, recompiled.mapShards[index].wire) {
			t.Fatalf("equivalent oversized input changed map shard %d", index+1)
		}
	}
	t.Run("compilation plan is sealed", func(t *testing.T) {
		dropped := compilation
		dropped.mapShards = append([]compiledMapShard(nil), compilation.mapShards[:len(compilation.mapShards)-1]...)
		if err := dropped.validate(); err == nil || !strings.Contains(err.Error(), "map plan binding") {
			t.Fatalf("dropped map shard validation = %v", err)
		}

		wireTampered := compilation
		wireTampered.mapShards = append([]compiledMapShard(nil), compilation.mapShards...)
		wireTampered.mapShards[0].wire = append([]byte(nil), compilation.mapShards[0].wire...)
		wireTampered.mapShards[0].wire[0] ^= 1
		if err := wireTampered.validate(); err == nil || !strings.Contains(err.Error(), "map plan binding") {
			t.Fatalf("tampered map wire validation = %v", err)
		}

		authorityTampered := compilation
		authorityTampered.mapShards = append([]compiledMapShard(nil), compilation.mapShards...)
		authorityTampered.mapShards[0].targetsByRef = cloneTargetAuthority(compilation.mapShards[0].targetsByRef)
		for ref := range authorityTampered.mapShards[0].targetsByRef {
			delete(authorityTampered.mapShards[0].targetsByRef, ref)
			break
		}
		if err := authorityTampered.validate(); err == nil || !strings.Contains(err.Error(), "map plan binding") {
			t.Fatalf("tampered map authority validation = %v", err)
		}
	})

	provider := newRuntimeShardTestProvider()
	outcome, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4,
	}, provider, input)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.CacheKey != "" {
		t.Fatalf("no-cache sharded run exposed a synthetic cache key %q", outcome.CacheKey)
	}
	observed := provider.snapshot()
	if observed.mapCalls != len(compilation.mapShards) || observed.reduceCalls < 1 ||
		outcome.SemanticCalls != observed.mapCalls+observed.reduceCalls {
		t.Fatalf(
			"runtime calls = map %d/%d, reduce %d, semantic %d",
			observed.mapCalls, len(compilation.mapShards), observed.reduceCalls,
			outcome.SemanticCalls,
		)
	}
	for index, size := range observed.mapPayloadBytes {
		if size == 0 || size > MaxRequestBytes {
			t.Fatalf("provider map payload %d bytes = %d", index+1, size)
		}
	}
	for index, size := range observed.reducePayloadBytes {
		if size == 0 || size > MaxRequestBytes {
			t.Fatalf("provider reduce payload %d bytes = %d", index+1, size)
		}
	}
	for index, count := range observed.mapTargetCatalogCounts {
		if count != targetCount {
			t.Fatalf("provider map target catalog %d = %d, want %d", index+1, count, targetCount)
		}
	}
	for index, count := range observed.reduceTargetCatalogCounts {
		if count != targetCount {
			t.Fatalf("provider reduce target catalog %d = %d, want %d", index+1, count, targetCount)
		}
	}
	for ref := range compilation.targetsByRef {
		if observed.targetRefs[ref] != 1 {
			t.Fatalf("global target ref %q advertised %d times, want exactly once", ref, observed.targetRefs[ref])
		}
	}
	for ref, evidence := range compilation.evidenceByRef {
		want := 1
		if evidence.ProgramTargetID == "" {
			want = len(compilation.mapShards)
		}
		if observed.evidenceRefs[ref] != want {
			t.Fatalf("global evidence ref %q advertised %d times, want %d", ref, observed.evidenceRefs[ref], want)
		}
	}
	if len(observed.targetRefs) != len(compilation.targetsByRef) ||
		len(observed.evidenceRefs) != len(compilation.evidenceByRef) {
		t.Fatalf(
			"provider authority coverage = %d/%d targets, %d/%d evidence",
			len(observed.targetRefs), len(compilation.targetsByRef),
			len(observed.evidenceRefs), len(compilation.evidenceByRef),
		)
	}

	result := outcome.Value
	if len(result.Targets) != targetCount || len(result.Roles) != 1 ||
		len(result.Roles[0].Implementations) != targetCount ||
		len(result.UnclassifiedTargetIDs) != 0 ||
		result.Coverage.TargetsObserved != targetCount ||
		result.Coverage.TargetsMapped != targetCount ||
		result.Coverage.EvidenceAdvertised != len(compilation.evidenceByRef) {
		t.Fatalf("sharded runtime result = %#v", result)
	}
	for index, target := range result.Targets {
		want := fmt.Sprintf("program-target-%02d", index)
		if target.ProgramTargetID != want || result.Roles[0].Implementations[index].ProgramTargetID != want {
			t.Fatalf(
				"restored target/implementation %d = %q/%q, want %q",
				index, target.ProgramTargetID,
				result.Roles[0].Implementations[index].ProgramTargetID, want,
			)
		}
	}
	if err := result.ValidateAgainst(input); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
}

func TestCompileShardsBeforeProviderContextRejectsMultiMegabyteRequest(t *testing.T) {
	input := oversizedRuntimePortfolioInput(8, 100)
	compilation := mustCompile(t, input)
	if len(compilation.wire) <= MaxRequestBytes || len(compilation.wire) >= 4*1024*1024 ||
		len(compilation.mapShards) < 2 {
		t.Fatalf(
			"provider-context regression input = %d bytes / %d shards",
			len(compilation.wire), len(compilation.mapShards),
		)
	}
	provider := &deepseek.Client{
		HTTPClient: http.DefaultClient,
		APIKey:     "test-placeholder",
		Model:      "deepseek-v4-flash",
		MaxTokens:  MaxOutputTokens,
		Endpoint:   "https://api.deepseek.com/chat/completions",
	}
	for index, shard := range compilation.mapShards {
		if len(shard.wire) == 0 || len(shard.wire) > MaxRequestBytes {
			t.Fatalf("map shard %d raw bytes = %d", index+1, len(shard.wire))
		}
		prepared, err := provider.Prepare(llm.Prompt{
			System: strings.TrimSpace(mapPrompt), User: string(shard.wire), ResponseFormatJSON: true,
		}, runtimeCallLimits())
		if err != nil {
			t.Fatalf("prepare map shard %d: %v", index+1, err)
		}
		if prepared.Len() > MaxProviderRequestBytes {
			t.Fatalf(
				"map shard %d prepared bytes = %d, limit = %d",
				index+1, prepared.Len(), MaxProviderRequestBytes,
			)
		}
	}
}

func TestReduceUsesClosedCandidateSetsAndRestoresExactUnion(t *testing.T) {
	compilation := mustCompile(t, twoTargetInput())
	server, err := restoreRole(responseRole{
		Name: "Server candidate", Purpose: "Serves repository requests.",
		Prominence: ProminenceUnknown, Kind: RoleKindService,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	}, compilation.targetsByRef, compilation.evidenceByRef)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := restoreRole(responseRole{
		Name: "Worker candidate", Purpose: "Processes repository work.",
		Prominence: ProminenceUnknown, Kind: RoleKindWorker,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t2"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the tuple worker")},
	}, compilation.targetsByRef, compilation.evidenceByRef)
	if err != nil {
		t.Fatal(err)
	}
	authority := map[string]Role{"c1": server, "c2": worker}
	merged := reduceResponseRole{
		Name: "Repository runtime", Purpose: "Serves requests and processes queued work.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus: MappingMapped,
		CandidateRefs: []string{"c2", "c1", "c1", "unknown"},
	}
	raw, err := json.Marshal(reduceResponse{Roles: []reduceResponseRole{merged, merged}})
	if err != nil {
		t.Fatal(err)
	}
	roles, err := resolveReduceResponse(raw, authority, targetsByID(compilation.targetsByRef))
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 || len(roles[0].Implementations) != 2 || len(roles[0].Evidence) != 2 {
		t.Fatalf("reduced exact union = %#v", roles)
	}

	assignedTwice, err := json.Marshal(reduceResponse{Roles: []reduceResponseRole{
		merged,
		{
			Name: "Second role", Purpose: "Conflicts with the first assignment.",
			Prominence: ProminenceSupporting, Kind: RoleKindService,
			Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
			MappingStatus: MappingMapped, CandidateRefs: []string{"c1"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveReduceResponse(assignedTwice, authority, targetsByID(compilation.targetsByRef)); err == nil || !strings.Contains(err.Error(), "selected by several roles") {
		t.Fatalf("duplicate candidate assignment = %v", err)
	}

	conflictingName := merged
	conflictingName.CandidateRefs = []string{"c1"}
	otherNameConflict := merged
	otherNameConflict.Purpose = "Uses a conflicting vocabulary assignment."
	otherNameConflict.CandidateRefs = []string{"c2"}
	conflictRaw, err := json.Marshal(reduceResponse{Roles: []reduceResponseRole{conflictingName, otherNameConflict}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveReduceResponse(conflictRaw, authority, targetsByID(compilation.targetsByRef)); err == nil || !strings.Contains(err.Error(), "conflicting duplicate reduce role name") {
		t.Fatalf("same-name conflict = %v", err)
	}
}

func TestExecutionAggregatePreservesAcceptedUsageAndCountsOnlyLiveTransport(t *testing.T) {
	live := llm.Outcome[struct{}]{
		CacheKey: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64),
		ResponseSHA256: strings.Repeat("c", 64), RequestBytes: 11, ResponseBytes: 13,
		Metrics: llm.Metrics{
			InputTokens: 2, OutputTokens: 3, ReasoningTokens: 5,
			PromptCacheHitTokens: 7, PromptCacheMissTokens: 11,
			ProviderResponseBytes: 17, UsageReported: true,
			Latency: 19 * time.Millisecond, Attempts: 2,
		},
	}
	cached := llm.Outcome[struct{}]{
		CacheKey: strings.Repeat("d", 64), RequestSHA256: strings.Repeat("e", 64),
		ResponseSHA256: strings.Repeat("f", 64), RequestBytes: 23, ResponseBytes: 29,
		Cached: true,
		Metrics: llm.Metrics{
			InputTokens: 31, OutputTokens: 37, ReasoningTokens: 41,
			PromptCacheHitTokens: 43, PromptCacheMissTokens: 47,
			ProviderResponseBytes: 53, UsageReported: true,
			Latency: 59 * time.Millisecond, Attempts: 3,
		},
	}

	aggregate := executionAggregate{allCached: true}
	addExecutionOutcome(&aggregate, live)
	addExecutionOutcome(&aggregate, cached)
	outcome := aggregate.finish(Result{})
	wantMetrics := llm.Metrics{
		InputTokens: 33, OutputTokens: 40, ReasoningTokens: 46,
		PromptCacheHitTokens: 50, PromptCacheMissTokens: 58,
		ProviderResponseBytes: 70, UsageReported: true,
		Latency: 19 * time.Millisecond, Attempts: 2,
	}
	if outcome.Cached || outcome.SemanticCalls != 1 || outcome.CacheKey != "" ||
		outcome.RequestBytes != 34 || outcome.ResponseBytes != 42 ||
		!reflect.DeepEqual(outcome.Metrics, wantMetrics) {
		t.Fatalf("mixed execution aggregate = %#v", outcome)
	}

	allCached := executionAggregate{allCached: true}
	addExecutionOutcome(&allCached, cached)
	addExecutionOutcome(&allCached, cached)
	cachedOutcome := allCached.finish(Result{})
	wantCachedMetrics := cached.Metrics
	wantCachedMetrics.InputTokens *= 2
	wantCachedMetrics.OutputTokens *= 2
	wantCachedMetrics.ReasoningTokens *= 2
	wantCachedMetrics.PromptCacheHitTokens *= 2
	wantCachedMetrics.PromptCacheMissTokens *= 2
	wantCachedMetrics.ProviderResponseBytes *= 2
	wantCachedMetrics.Latency = 0
	wantCachedMetrics.Attempts = 0
	if !cachedOutcome.Cached || cachedOutcome.SemanticCalls != 0 || cachedOutcome.CacheKey != "" ||
		!reflect.DeepEqual(cachedOutcome.Metrics, wantCachedMetrics) {
		t.Fatalf("cached execution aggregate = %#v", cachedOutcome)
	}
}

func TestCompileRejectsOneOversizedTargetWithoutTruncation(t *testing.T) {
	input := oversizedRuntimePortfolioInput(1, 3_000)
	_, err := Compile(input)
	if err == nil || !strings.Contains(err.Error(), "whole target") ||
		!strings.Contains(err.Error(), "was not truncated") {
		t.Fatalf("oversized single-target compilation error = %v", err)
	}
}

func TestCompileHasNoEntityCountLimit(t *testing.T) {
	const targetCount = 5000
	input := Input{
		RepositoryName: "large-runtime-repository", CapturedRevision: strings.Repeat("a", 40),
		TargetPagePortfolioSHA256: strings.Repeat("b", 64),
		Targets:                   make([]TargetInput, 0, targetCount), RepositoryEvidence: []EvidenceInput{},
	}
	for index := range targetCount {
		input.Targets = append(input.Targets, TargetInput{
			ProgramTargetID: fmt.Sprintf("program-target-%05d", index),
			DisplayName:     fmt.Sprintf("target-%05d", index), Language: "Go", Kind: "command",
			Selector: fmt.Sprintf("./cmd/target-%05d", index), Default: index == 0,
			Responsibilities: []ResponsibilityInput{}, Evidence: []EvidenceInput{},
		})
	}
	compilation := mustCompile(t, input)
	if compilation.Request.TargetCount != targetCount || len(compilation.Request.Targets) != targetCount ||
		len(compilation.wire) <= MaxRequestBytes || len(compilation.mapShards) < 2 {
		t.Fatalf("large compilation = targets %d/%d, bytes %d", compilation.Request.TargetCount, targetCount, len(compilation.wire))
	}
	observed := make(map[string]int, targetCount)
	for _, shard := range compilation.mapShards {
		if len(shard.wire) > MaxRequestBytes {
			t.Fatalf("entity-count map shard bytes = %d", len(shard.wire))
		}
		for _, target := range shard.request.Targets {
			observed[target.Ref]++
		}
	}
	if len(observed) != targetCount {
		t.Fatalf("entity-count shard target coverage = %d/%d", len(observed), targetCount)
	}
	for ref, count := range observed {
		if count != 1 {
			t.Fatalf("entity-count target %s appears in %d shards", ref, count)
		}
	}
}

func TestCompileRequiresExactTargetEvidenceBinding(t *testing.T) {
	input := twoTargetInput()
	input.Targets[0].Evidence[0].ProgramTargetID = input.Targets[1].ProgramTargetID
	if _, err := Compile(input); err == nil {
		t.Fatal("Compile accepted target evidence bound to a different target")
	}
	input = twoTargetInput()
	input.Targets[0].Responsibilities[0].Evidence[0].ProgramTargetID = ""
	if _, err := Compile(input); err == nil {
		t.Fatal("Compile accepted responsibility evidence without exact target binding")
	}
}

func TestCompilationAndSecretEnvelopesFailClosed(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	tampered := compilation
	tampered.Request.Targets = append([]wireTarget(nil), compilation.Request.Targets...)
	tampered.Request.Targets[0].DisplayName = "substituted"
	if _, err := ResolveResponse(tampered, []byte(`{"roles":[]}`)); err == nil {
		t.Fatal("ResolveResponse accepted a tampered compilation")
	}
	publicationTampered := compilation
	publicationTampered.input.TargetPagePortfolioSHA256 = strings.Repeat("c", 64)
	if _, err := ResolveResponse(publicationTampered, []byte(`{"roles":[]}`)); err == nil {
		t.Fatal("ResolveResponse accepted a publication-substituted compilation with a stale local seal")
	}
	persistenceUnsafe := singleTargetInput()
	persistenceUnsafe.RepositoryEvidence[0].Label = "Authorization: Bearer opaque-provider-value"
	if _, err := Compile(persistenceUnsafe); err == nil ||
		!strings.Contains(err.Error(), "persistence-sensitive") ||
		strings.Contains(err.Error(), "opaque-provider-value") {
		t.Fatalf("always-on persistence request error = %v", err)
	}

	restore := secretscan.SetEnabled(true)
	defer restore()
	secret := "sk-ABCDEFGHIJKLMNOPQRSTUVWX"
	input := singleTargetInput()
	input.RepositoryEvidence[0].Label = "configured with " + secret
	if _, err := Compile(input); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("credential-shaped request error = %v", err)
	}
	role := responseRole{
		Name: "Authorization service", Purpose: "Configured with " + secret,
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	}
	if _, err := ResolveResponse(compilation, mustMarshalResponse(t, role)); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("credential-shaped response error = %v", err)
	}
}

type runtimeTestProvider struct {
	response      []byte
	completeCalls int
}

type runtimeShardTestProvider struct {
	mu                        sync.Mutex
	mapCalls                  int
	reduceCalls               int
	mapPayloadBytes           []int
	reducePayloadBytes        []int
	mapTargetCatalogCounts    []int
	reduceTargetCatalogCounts []int
	targetRefs                map[string]int
	evidenceRefs              map[string]int
}

type runtimeShardTestProviderSnapshot struct {
	mapCalls                  int
	reduceCalls               int
	mapPayloadBytes           []int
	reducePayloadBytes        []int
	mapTargetCatalogCounts    []int
	reduceTargetCatalogCounts []int
	targetRefs                map[string]int
	evidenceRefs              map[string]int
}

type runtimeShardPreparedEnvelope struct {
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
}

func newRuntimeShardTestProvider() *runtimeShardTestProvider {
	return &runtimeShardTestProvider{
		targetRefs: make(map[string]int), evidenceRefs: make(map[string]int),
	}
}

func (provider *runtimeShardTestProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"runtime-shard-test"}`)
}

func (provider *runtimeShardTestProvider) Prepare(
	prompt llm.Prompt,
	_ llm.Limits,
) (llm.Prepared, error) {
	user := []byte(prompt.User)
	var phase struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(user, &phase); err != nil {
		return llm.Prepared{}, err
	}
	var responseRaw []byte
	switch phase.Phase {
	case "map":
		var request mapRequest
		if err := json.Unmarshal(user, &request); err != nil {
			return llm.Prepared{}, err
		}
		role := responseRole{
			Name: "Repository runtime", Purpose: "Runs the repository product.",
			Prominence: ProminencePrimary, Kind: RoleKindService,
			Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
			MappingStatus: MappingMapped, Implementations: []responseImplementation{},
			EvidenceRefs: []string{},
		}
		for _, target := range request.Targets {
			if len(target.EvidenceRefs) == 0 {
				return llm.Prepared{}, fmt.Errorf("map target %q has no direct evidence", target.Ref)
			}
			role.Implementations = append(role.Implementations, responseImplementation{TargetRef: target.Ref})
			role.EvidenceRefs = append(role.EvidenceRefs, target.EvidenceRefs[0])
		}
		encoded, err := json.Marshal(response{Roles: []responseRole{role}})
		if err != nil {
			return llm.Prepared{}, err
		}
		responseRaw = encoded
		provider.mu.Lock()
		provider.mapCalls++
		provider.mapPayloadBytes = append(provider.mapPayloadBytes, len(user))
		provider.mapTargetCatalogCounts = append(provider.mapTargetCatalogCounts, len(request.TargetCatalog))
		for _, target := range request.Targets {
			provider.targetRefs[target.Ref]++
		}
		for _, evidence := range request.EvidenceCatalog {
			provider.evidenceRefs[evidence.Ref]++
		}
		for _, evidence := range request.RepositoryEvidence {
			provider.evidenceRefs[evidence.Ref]++
		}
		provider.mu.Unlock()
	case "reduce":
		var request reduceRequest
		if err := json.Unmarshal(user, &request); err != nil {
			return llm.Prepared{}, err
		}
		refs := make([]string, 0, len(request.Candidates))
		for _, candidate := range request.Candidates {
			refs = append(refs, candidate.Ref)
		}
		encoded, err := json.Marshal(reduceResponse{Roles: []reduceResponseRole{{
			Name: "Repository runtime", Purpose: "Runs the repository product.",
			Prominence: ProminencePrimary, Kind: RoleKindService,
			Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
			MappingStatus: MappingMapped, CandidateRefs: refs,
		}}})
		if err != nil {
			return llm.Prepared{}, err
		}
		responseRaw = encoded
		provider.mu.Lock()
		provider.reduceCalls++
		provider.reducePayloadBytes = append(provider.reducePayloadBytes, len(user))
		provider.reduceTargetCatalogCounts = append(
			provider.reduceTargetCatalogCounts, len(request.TargetCatalog),
		)
		provider.mu.Unlock()
	default:
		return llm.Prepared{}, fmt.Errorf("unexpected runtime phase %q", phase.Phase)
	}
	envelope, err := json.Marshal(runtimeShardPreparedEnvelope{Request: user, Response: responseRaw})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(envelope)
}

func (provider *runtimeShardTestProvider) Complete(
	_ context.Context,
	prepared llm.Prepared,
) (llm.Completion, error) {
	var envelope runtimeShardPreparedEnvelope
	if err := json.Unmarshal(prepared.Bytes(), &envelope); err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{
		Response: append([]byte(nil), envelope.Response...), FinishReason: llm.FinishStop,
		ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

func (provider *runtimeShardTestProvider) snapshot() runtimeShardTestProviderSnapshot {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	result := runtimeShardTestProviderSnapshot{
		mapCalls: provider.mapCalls, reduceCalls: provider.reduceCalls,
		mapPayloadBytes:           append([]int(nil), provider.mapPayloadBytes...),
		reducePayloadBytes:        append([]int(nil), provider.reducePayloadBytes...),
		mapTargetCatalogCounts:    append([]int(nil), provider.mapTargetCatalogCounts...),
		reduceTargetCatalogCounts: append([]int(nil), provider.reduceTargetCatalogCounts...),
		targetRefs:                make(map[string]int, len(provider.targetRefs)),
		evidenceRefs:              make(map[string]int, len(provider.evidenceRefs)),
	}
	for ref, count := range provider.targetRefs {
		result.targetRefs[ref] = count
	}
	for ref, count := range provider.evidenceRefs {
		result.evidenceRefs[ref] = count
	}
	return result
}

func (provider *runtimeTestProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"runtime-test"}`)
}

func (provider *runtimeTestProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	raw, err := json.Marshal(struct {
		System string `json:"system"`
		User   string `json:"user"`
		JSON   bool   `json:"json"`
		Tokens int    `json:"max_tokens"`
	}{prompt.System, prompt.User, prompt.ResponseFormatJSON, limits.MaxOutputTokens})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(raw)
}

func (provider *runtimeTestProvider) Complete(_ context.Context, _ llm.Prepared) (llm.Completion, error) {
	provider.completeCalls++
	return llm.Completion{
		Response: append([]byte(nil), provider.response...), FinishReason: llm.FinishStop,
		ChoiceCount: 1, Metrics: llm.Metrics{
			InputTokens: 2, OutputTokens: 3, ProviderResponseBytes: 5,
			UsageReported: true, Latency: 7 * time.Millisecond, Attempts: 1,
		},
	}, nil
}

func singleTargetInput() Input {
	const targetID = "program-target-server"
	return Input{
		RepositoryName: "authorization-server", CapturedRevision: strings.Repeat("a", 40),
		TargetPagePortfolioSHA256: strings.Repeat("b", 64),
		Targets: []TargetInput{{
			ProgramTargetID: targetID, DisplayName: "server", Language: "Go", Kind: "command",
			Selector: "./cmd/server", Default: true, ProgramObjects: 41, ProgramRelations: 73,
			ActivityStarts: 3, IntegrationUses: 12,
			Responsibilities: []ResponsibilityInput{{
				Name: "Authorization decisions", Purpose: "Evaluates authorization requests.",
				Evidence: []EvidenceInput{{
					Kind: EvidenceResponsibility, Label: "Evaluates authorization requests",
					Location:        Location{Path: "internal/server/check.go", Line: 21, Column: 2},
					ProgramTargetID: targetID,
				}},
			}},
			Evidence: []EvidenceInput{{
				Kind: EvidenceTargetEntrypoint, Label: "Starts the API server",
				Location:        Location{Path: "cmd/server/main.go", Line: 17, Column: 1},
				ProgramTargetID: targetID,
			}},
		}},
		RepositoryEvidence: []EvidenceInput{{
			Kind: EvidenceRepositoryGuidance, Label: "Repository runs one authorization server",
			Location: Location{Path: "README.md", Line: 9},
		}},
	}
}

func twoTargetInput() Input {
	input := singleTargetInput()
	input.RepositoryEvidence[0].Label = "Repository runs one server and one worker"
	const targetID = "program-target-worker"
	input.Targets = append(input.Targets, TargetInput{
		ProgramTargetID: targetID, DisplayName: "worker", Language: "Go", Kind: "command",
		Selector: "./cmd/worker", ProgramObjects: 19, ProgramRelations: 31,
		ActivityStarts: 1, IntegrationUses: 4,
		Responsibilities: []ResponsibilityInput{{
			Name: "Tuple processing", Purpose: "Processes queued tuple updates.",
			Evidence: []EvidenceInput{{
				Kind: EvidenceResponsibility, Label: "Processes queued tuple updates",
				Location:        Location{Path: "internal/worker/tuples.go", Line: 12, Column: 1},
				ProgramTargetID: targetID,
			}},
		}},
		Evidence: []EvidenceInput{{
			Kind: EvidenceTargetEntrypoint, Label: "Starts the tuple worker",
			Location:        Location{Path: "cmd/worker/main.go", Line: 14, Column: 1},
			ProgramTargetID: targetID,
		}},
	})
	return input
}

func oversizedRuntimePortfolioInput(targetCount, responsibilitiesPerTarget int) Input {
	input := Input{
		RepositoryName: "large-runtime-repository", CapturedRevision: strings.Repeat("a", 40),
		TargetPagePortfolioSHA256: strings.Repeat("b", 64),
		Targets:                   make([]TargetInput, 0, targetCount),
		RepositoryEvidence: []EvidenceInput{{
			Kind: EvidenceRepositoryGuidance, Label: "Repository publishes the complete runtime topology",
			Location: Location{Path: "README.md", Line: 7},
		}},
	}
	for targetIndex := range targetCount {
		targetID := fmt.Sprintf("program-target-%02d", targetIndex)
		target := TargetInput{
			ProgramTargetID: targetID, DisplayName: fmt.Sprintf("runtime-%02d", targetIndex),
			Language: "Go", Kind: "command", Selector: fmt.Sprintf("./cmd/runtime-%02d", targetIndex),
			Default: targetIndex == 0, ProgramObjects: 100, ProgramRelations: 200,
			ActivityStarts: 1, IntegrationUses: 1,
			Responsibilities: make([]ResponsibilityInput, 0, responsibilitiesPerTarget),
			Evidence: []EvidenceInput{{
				Kind: EvidenceTargetEntrypoint, Label: fmt.Sprintf("Starts runtime %02d", targetIndex),
				Location:        Location{Path: fmt.Sprintf("cmd/runtime-%02d/main.go", targetIndex), Line: 1},
				ProgramTargetID: targetID,
			}},
		}
		for responsibilityIndex := range responsibilitiesPerTarget {
			evidence := EvidenceInput{
				Kind: EvidenceResponsibility,
				Label: fmt.Sprintf(
					"Evidence %02d %04d %s", targetIndex, responsibilityIndex,
					strings.Repeat("e", 440),
				),
				Location: Location{
					Path: fmt.Sprintf("internal/runtime-%02d/responsibility-%04d.go", targetIndex, responsibilityIndex),
					Line: responsibilityIndex + 1,
				},
				ProgramTargetID: targetID,
			}
			target.Responsibilities = append(target.Responsibilities, ResponsibilityInput{
				Name: fmt.Sprintf(
					"Responsibility %02d %04d %s", targetIndex, responsibilityIndex,
					strings.Repeat("n", 430),
				),
				Purpose: fmt.Sprintf(
					"Describes runtime behavior %02d %04d %s", targetIndex, responsibilityIndex,
					strings.Repeat("p", 420),
				),
				Evidence: []EvidenceInput{evidence},
			})
		}
		input.Targets = append(input.Targets, target)
	}
	return input
}

func mustCompile(t *testing.T, input Input) Compilation {
	t.Helper()
	compilation, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return compilation
}

func evidenceRefForLabel(t *testing.T, compilation Compilation, label string) string {
	t.Helper()
	for _, evidence := range compilation.Request.EvidenceCatalog {
		if evidence.Label == label {
			return evidence.Ref
		}
	}
	t.Fatalf("evidence label %q is not advertised", label)
	return ""
}

func mustResolve(t *testing.T, compilation Compilation, roles ...responseRole) Result {
	t.Helper()
	result, err := ResolveResponse(compilation, mustMarshalResponse(t, roles...))
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	return result
}

func mustMarshalResponse(t *testing.T, roles ...responseRole) []byte {
	t.Helper()
	raw, err := json.Marshal(response{Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneInput(input Input) Input {
	result := input
	result.RepositoryEvidence = append([]EvidenceInput(nil), input.RepositoryEvidence...)
	result.Targets = append([]TargetInput(nil), input.Targets...)
	for targetIndex := range result.Targets {
		result.Targets[targetIndex].Evidence = append([]EvidenceInput(nil), input.Targets[targetIndex].Evidence...)
		result.Targets[targetIndex].Responsibilities = append(
			[]ResponsibilityInput(nil), input.Targets[targetIndex].Responsibilities...,
		)
		for responsibilityIndex := range result.Targets[targetIndex].Responsibilities {
			result.Targets[targetIndex].Responsibilities[responsibilityIndex].Evidence = append(
				[]EvidenceInput(nil),
				input.Targets[targetIndex].Responsibilities[responsibilityIndex].Evidence...,
			)
		}
	}
	return result
}

func cloneResult(result Result) Result {
	cloned := result
	cloned.Targets = append([]Target(nil), result.Targets...)
	cloned.UnclassifiedTargetIDs = append([]string{}, result.UnclassifiedTargetIDs...)
	cloned.Roles = append([]Role{}, result.Roles...)
	for roleIndex := range cloned.Roles {
		cloned.Roles[roleIndex].Implementations = append(
			[]Implementation(nil), result.Roles[roleIndex].Implementations...,
		)
		cloned.Roles[roleIndex].Evidence = append([]Evidence(nil), result.Roles[roleIndex].Evidence...)
	}
	return cloned
}
