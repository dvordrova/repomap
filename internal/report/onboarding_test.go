package report

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestDeriveMechanismOnboardingRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      ReportData
		mechanism UserMechanism
		expected  OnboardingRole
	}{
		{
			name: "web response behavior is primary",
			data: ReportData{
				DocumentedPurpose: "A web server handles HTTP requests.",
				Components: []Component{{
					Name: "HTTP", Role: componentmap.RoleBoundary,
					AnchorGroups: []AnchorGroup{{Path: "modules/http/server.go"}},
				}},
			},
			mechanism: onboardingMechanism(
				"listing",
				"How does the file server build a directory listing from a request?",
				"The handler reads query options, sorts entries, and writes the HTTP response.",
				[]string{"modules/http/files/list.go", "modules/http/files/sort.go"},
				"Receive the request", "Sort the listing", "Write the response",
			),
			expected: OnboardingRolePrimaryBehavior,
		},
		{
			name: "router dispatch behavior is primary",
			data: ReportData{
				ProjectGuess: "A lightweight HTTP router for composable services",
				Components: []Component{{
					Name: "Router", Role: componentmap.RoleDomain,
					AnchorGroups: []AnchorGroup{{Path: "mux.go"}, {Path: "tree.go"}},
				}},
			},
			mechanism: onboardingMechanism(
				"dispatch",
				"How does the router dispatch an HTTP request?",
				"The handler prepares context, finds a route, and invokes the endpoint.",
				[]string{"mux.go", "tree.go"},
				"Prepare request context", "Find the route", "Invoke the endpoint",
			),
			expected: OnboardingRolePrimaryBehavior,
		},
		{
			name: "factory registry is extension",
			data: ReportData{DocumentedPurpose: "A database replication and recovery tool."},
			mechanism: onboardingMechanism(
				"registry",
				"How are replica clients registered and created from a URL scheme?",
				"Initialization registers a factory and URL creation looks it up.",
				[]string{"replica_client.go", "replica_url.go"},
				"Register factory", "Parse URL", "Look up factory", "Create client",
			),
			expected: OnboardingRoleExtensionPoint,
		},
		{
			name: "off-purpose core behavior remains secondary",
			data: ReportData{
				DocumentedPurpose: "A database replication and recovery tool.",
				Components: []Component{{
					Name: "Core", Role: componentmap.RoleDomain,
					AnchorGroups: []AnchorGroup{{Path: "internal/checksum.go"}},
				}},
			},
			mechanism: onboardingMechanism(
				"checksum",
				"How does a request compute and return a checksum?",
				"The handler reads input, computes a checksum, and returns a result.",
				[]string{"internal/checksum.go"},
				"Read input", "Compute checksum", "Return result",
			),
			expected: OnboardingRoleSecondaryBehavior,
		},
		{
			name: "purpose and validated core topic can bridge a sparse anchor list",
			data: ReportData{
				DocumentedPurpose: "A database replication and recovery tool.",
				Components: []Component{{
					Name: "Replication engine", Role: componentmap.RoleDomain,
					ModelPurpose: "Coordinates incremental replication.",
					AnchorGroups: []AnchorGroup{{Path: "server.go"}},
				}},
			},
			mechanism: onboardingMechanism(
				"replication",
				"How does a command start database replication?",
				"The command reads configuration, starts replication, and writes changes to storage.",
				[]string{"replica.go", "monitor.go"},
				"Read configuration", "Start replication", "Write changes",
			),
			expected: OnboardingRolePrimaryBehavior,
		},
		{
			name: "one unanchored file cannot claim core coverage from topic alone",
			data: ReportData{
				DocumentedPurpose: "A database replication and recovery tool.",
				Components: []Component{{
					Name: "Replication engine", Role: componentmap.RoleDomain,
					ModelPurpose: "Coordinates incremental replication.",
					AnchorGroups: []AnchorGroup{{Path: "server.go"}},
				}},
			},
			mechanism: onboardingMechanism(
				"local-replication",
				"How does a command start database replication?",
				"The command reads configuration, starts replication, and writes changes to storage.",
				[]string{"replica.go"},
				"Read configuration", "Start replication", "Write changes",
			),
			expected: OnboardingRoleSecondaryBehavior,
		},
		{
			name: "error checks are detail",
			mechanism: onboardingMechanism(
				"errors", "How are invalid values rejected?", "Errors are returned.",
				[]string{"validate.go"},
				"Validation error", "Error propagation", "Error return",
			),
			expected: OnboardingRoleErrorDetail,
		},
		{
			name: "logging is operational support",
			mechanism: onboardingMechanism(
				"logging", "How is structured logging configured?", "A logger records metrics.",
				[]string{"log.go"}, "Create logger", "Record metrics",
			),
			expected: OnboardingRoleOperationalSupport,
		},
		{
			name: "local helper is secondary",
			mechanism: onboardingMechanism(
				"helper", "How is a checksum computed?", "A helper computes a checksum.",
				[]string{"checksum.go"}, "Read bytes", "Compute checksum",
			),
			expected: OnboardingRoleSecondaryBehavior,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := DeriveMechanismOnboardingRole(&test.data, test.mechanism); got != test.expected {
				t.Fatalf("DeriveMechanismOnboardingRole() = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestDeriveMechanismOnboardingRolePrimaryPathContractWithoutCanvasFocus(t *testing.T) {
	t.Parallel()

	data := ReportData{
		DocumentedPurpose: "A database replication and recovery tool restores durable database copies.",
	}
	mechanism := onboardingMechanism(
		"restore",
		"How does the restore command recover a database?",
		"The command prepares the destination, restores the database, and replaces the output file.",
		[]string{"cmd/litestream/restore.go", "replica.go"},
		"Receive restore command", "Restore database", "Replace output file",
	)
	mechanism.canonicalCoveredAspectIDs = []string{
		"input-trigger",
		"core-work",
		"observable-effect",
	}

	if got := DeriveMechanismOnboardingRole(&data, mechanism); got != OnboardingRolePrimaryBehavior {
		t.Fatalf("DeriveMechanismOnboardingRole() = %q, want %q", got, OnboardingRolePrimaryBehavior)
	}

	mechanism.canonicalCoveredAspectIDs = []string{
		"input-trigger",
		"core-work",
		"observable-effect-detail",
	}
	if got := DeriveMechanismOnboardingRole(&data, mechanism); got != OnboardingRoleSecondaryBehavior {
		t.Fatalf("lookalike aspect ID classified as primary: got %q, want %q", got, OnboardingRoleSecondaryBehavior)
	}
}

func TestDeriveRepositoryThesisPrefersDocumentedPurposeAndExactAreas(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName:          "example",
		ProjectGuess:      "A model-written fallback that should not win",
		DocumentedPurpose: "# Example\n\nExample accepts database changes. It writes durable copies. Restore reads them back.",
		OpenablePaths:     []string{"cmd/main.go", "engine.go", "store/client.go"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Components: []ArchitectureComponent{{
				ID: "canvas-core",
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: "engine.go"},
				}},
			}},
			Subsystems: []ArchitectureSubsystem{{
				ID: "subsystem-core", Name: "Core", ComponentIDs: []componentmap.ComponentID{"canvas-core"},
			}},
		},
		Components: []Component{
			{
				Name: "Command entry", Role: componentmap.RoleEntry,
				ModelPurpose: "Accepts user commands.",
				AnchorGroups: []AnchorGroup{{Path: "cmd/main.go"}},
			},
			{
				Name: "Replication engine", Role: componentmap.RoleDomain,
				ModelPurpose: "Coordinates durable work.",
				AnchorGroups: []AnchorGroup{{Path: "engine.go"}},
			},
			{
				Name: "External store", Role: componentmap.RoleBoundary,
				ModelPurpose: "Writes repository data to an external store.",
				AnchorGroups: []AnchorGroup{{Path: "store/client.go"}},
			},
			{
				Name: "Unresolved area", Role: componentmap.RoleSupport,
				ModelPurpose: "Should not be visible without a target.",
				AnchorGroups: []AnchorGroup{{Path: "outside.go"}},
			},
		},
	}

	thesis := DeriveRepositoryThesis(data)
	if thesis == nil {
		t.Fatal("expected repository thesis")
	}
	if thesis.Purpose != "Example accepts database changes." {
		t.Fatalf("purpose = %q", thesis.Purpose)
	}
	if len(thesis.SystemStory) < 2 || thesis.SystemStory[0] != "It writes durable copies." ||
		thesis.SystemStory[1] != "Restore reads them back." {
		t.Fatalf("system story = %#v", thesis.SystemStory)
	}
	if len(thesis.Areas) != 3 {
		t.Fatalf("areas = %d, want 3: %#v", len(thesis.Areas), thesis.Areas)
	}
	allowed := map[string]bool{"cmd/main.go": true, "engine.go": true, "store/client.go": true}
	for _, area := range thesis.Areas {
		if area.CodeLocation == nil || !allowed[area.CodeLocation.Path] {
			t.Fatalf("area has non-openable target: %#v", area)
		}
	}
}

func TestSkipUnsafePurposeSentencesFiltersWarningsQuotesAndProtocolLists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "unstable warning is removed",
			input: []string{"**Note**: The `main` branch may be in an *unstable or even broken state* during development.", "Example stores durable records."},
			want:  []string{"Example stores durable records."},
		},
		{
			name:  "quoted marketing quote is removed",
			input: []string{">\"I never knew creating bots could be so _sexy_!\"", "Example builds bots."},
			want:  []string{"Example builds bots."},
		},
		{
			name:  "protocol capability list is removed",
			input: []string{"Supporting MCP, A2A, OAuth 2.0, OIDC (OAuth 2.x), SAML, CAS, LDAP, SCIM.", "Example is an identity server."},
			want:  []string{"Example is an identity server."},
		},
		{
			name:  "all unsafe sentences leave an empty result",
			input: []string{"**Note**: Work in progress.", "\"Marketing quote!\""},
			want:  nil,
		},
		{
			name:  "clean purpose is preserved",
			input: []string{"restic is a backup program that is fast, efficient and secure."},
			want:  []string{"restic is a backup program that is fast, efficient and secure."},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := skipUnsafePurposeSentences(test.input)
			if len(got) != len(test.want) {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("sentence %d = %q, want %q", index, got[index], test.want[index])
				}
			}
		})
	}
}

func TestFinalizeRepositoryOnboardingSelectsPrimaryAndKeepsExtension(t *testing.T) {
	t.Parallel()

	primary := onboardingMechanism(
		"primary", "How does the router dispatch a request?",
		"The request enters the router, which finds a route and invokes an endpoint.",
		[]string{"mux.go", "tree.go"},
		"Receive request", "Find route", "Invoke endpoint",
	)
	extension := onboardingMechanism(
		"extension", "How are router plugins registered in a factory registry?",
		"Plugins register factories for later lookup.",
		[]string{"registry.go"},
		"Register plugin", "Store factory", "Look up factory",
	)
	data := &ReportData{
		RepoName:          "router",
		DocumentedPurpose: "A router dispatches HTTP requests to endpoints.",
		OpenablePaths:     []string{"mux.go", "tree.go", "registry.go"},
		Components: []Component{{
			Name: "Router", Role: componentmap.RoleDomain,
			ModelPurpose: "Dispatches requests.",
			AnchorGroups: []AnchorGroup{{Path: "mux.go"}, {Path: "tree.go"}},
		}},
		UserMechanisms: []UserMechanism{extension, primary},
	}

	FinalizeRepositoryOnboarding(data, nil)
	if data.StartHereArtifactID != "primary" {
		t.Fatalf("Start Here = %q, want primary", data.StartHereArtifactID)
	}
	if len(data.UserMechanisms) != 2 || data.UserMechanisms[0].ArtifactID != "primary" ||
		data.UserMechanisms[0].Role != OnboardingRolePrimaryBehavior ||
		data.UserMechanisms[1].Role != OnboardingRoleExtensionPoint {
		t.Fatalf("role-aware order = %#v", data.UserMechanisms)
	}
	if data.RepositoryThesis == nil || data.RepositoryThesis.RecommendedArtifactID != "primary" {
		t.Fatalf("repository thesis = %#v", data.RepositoryThesis)
	}
	if data.RepositoryGuide == nil || data.RepositoryGuide.StartHereArtifactID != "primary" ||
		!reflect.DeepEqual(data.RepositoryGuide.ExtensionArtifactIDs, []string{"extension"}) ||
		len(data.RepositoryGuide.ReadNext) != 2 {
		t.Fatalf("repository guide = %#v", data.RepositoryGuide)
	}

	data.UserMechanisms = []UserMechanism{extension}
	FinalizeRepositoryOnboarding(data, nil)
	if data.StartHereArtifactID != "" {
		t.Fatalf("extension selected as Start Here: %q", data.StartHereArtifactID)
	}
	if data.RepositoryThesis == nil || data.RepositoryThesis.RecommendedArtifactID != "" {
		t.Fatalf("no-primary thesis recommends an artifact: %#v", data.RepositoryThesis)
	}
}

func TestRepositoryArchitectureRequiresDistinctResolvedAreas(t *testing.T) {
	t.Parallel()

	data := &ReportData{ArchitectureCanvas: &ArchitectureCanvas{}}
	one := []RepositoryThesisArea{{
		Label: "Core",
		MapTarget: &UserMapTarget{
			Kind: SemanticSearchTargetComponent, ComponentID: "component-core",
		},
	}}
	if repositoryArchitectureUseful(data, one) {
		t.Fatal("one architecture target was treated as a useful distinct-area map")
	}
	two := append(one, RepositoryThesisArea{
		Label: "Storage",
		MapTarget: &UserMapTarget{
			Kind: SemanticSearchTargetComponent, ComponentID: "component-storage",
		},
	})
	if !repositoryArchitectureUseful(data, two) {
		t.Fatal("two distinct architecture targets were hidden")
	}
}

func TestNarrativeCompressionIsLocalAndRetainsCanonicalSteps(t *testing.T) {
	t.Parallel()

	mechanism := onboardingMechanism(
		"registry", "How are implementations registered and created?",
		"Factories are registered and used during creation.",
		[]string{"register.go", "create.go"},
		"Registration initiation", "Registration implementation", "Creation initiation",
		"Local function call", "Error propagation", "Error return", "Conditional error check",
	)
	for index := range mechanism.Steps {
		mechanism.Steps[index].canonicalStatementIDs = []string{"statement-" + string(rune('a'+index))}
		mechanism.Steps[index].Locations = []UserCodeLocation{{
			Path: []string{"register.go", "create.go"}[min(index/2, 1)],
			Line: 10 + index,
		}}
		mechanism.Steps[index].Sources = []SourceSnippet{{
			Path:               mechanism.Steps[index].Locations[0].Path,
			StartLine:          10 + index,
			EndLine:            10 + index,
			PresentationSHA256: "source-" + string(rune('a'+index)),
		}}
	}
	artifact := semanticdiscovery.Artifact{
		ID: "canonical", Statements: []semanticdiscovery.Statement{{
			ID: "statement", Text: "Canonical statement", Basis: semanticdiscovery.ClaimDirect,
		}},
	}
	data := &ReportData{
		ProjectGuess:      "A replication system",
		OpenablePaths:     []string{"register.go", "create.go"},
		SemanticArtifacts: []semanticdiscovery.Artifact{artifact},
		UserMechanisms:    []UserMechanism{mechanism},
	}
	wantSteps := append([]UserMechanismStep(nil), mechanism.Steps...)
	wantArtifacts := append([]semanticdiscovery.Artifact(nil), data.SemanticArtifacts...)

	FinalizeRepositoryOnboarding(data, nil)
	got := data.UserMechanisms[0]
	if !reflect.DeepEqual(got.Steps, wantSteps) {
		t.Fatalf("canonical steps changed:\ngot:  %#v\nwant: %#v", got.Steps, wantSteps)
	}
	if !reflect.DeepEqual(data.SemanticArtifacts, wantArtifacts) {
		t.Fatalf("semantic artifacts changed: %#v", data.SemanticArtifacts)
	}
	if len(got.Phases) != 4 {
		t.Fatalf("deterministic phases = %d, want 4: %#v", len(got.Phases), got.Phases)
	}
	last := got.Phases[len(got.Phases)-1]
	if !reflect.DeepEqual(last.ImplementationStepIndexes, []int{3, 4, 5, 6}) {
		t.Fatalf("nested error details = %#v", last.ImplementationStepIndexes)
	}
	if len(last.Locations) != 4 || len(last.Sources) != 4 {
		t.Fatalf("phase source union lost exact members: %#v", last)
	}

	proposal := NarrativeCompression{
		ArtifactID: "registry", OrderingBasis: NarrativeOrderingEditorial,
		Phases: []NarrativeCompressionPhase{
			{Title: "Perform registration", MemberStatementIDs: []string{"statement-a", "statement-b"}},
			{Title: "Creation initiation", MemberStatementIDs: []string{"statement-c"}},
			{Title: "Local function call", MemberStatementIDs: []string{"statement-d", "statement-e", "statement-f", "statement-g"}},
		},
	}
	phases, ok := ProjectNarrativeCompression(mechanism, proposal)
	if !ok || len(phases) != 3 {
		t.Fatalf("validated proposal = %#v, ok=%v", phases, ok)
	}
	if phases[0].Title != "Perform registration" {
		t.Fatalf("editorial action title = %q", phases[0].Title)
	}
	if !reflect.DeepEqual(phases[2].ImplementationStepIndexes, []int{3, 4, 5, 6}) {
		t.Fatalf("proposal implementation details = %#v", phases[2].ImplementationStepIndexes)
	}
}

func TestProjectNarrativeCompressionRejectsUnknownOrDuplicateMembership(t *testing.T) {
	t.Parallel()

	mechanism := onboardingMechanism(
		"behavior", "How does input become output?", "Input becomes output.",
		[]string{"behavior.go"}, "Read input", "Apply work", "Write output",
	)
	for index := range mechanism.Steps {
		mechanism.Steps[index].canonicalStatementIDs = []string{"statement-" + string(rune('a'+index))}
	}

	tests := []struct {
		name   string
		phases []NarrativeCompressionPhase
	}{
		{
			name: "unknown statement",
			phases: []NarrativeCompressionPhase{
				{Title: "Read input", MemberStatementIDs: []string{"statement-a"}},
				{Title: "Apply work", MemberStatementIDs: []string{"statement-b"}},
				{Title: "Write output", MemberStatementIDs: []string{"unknown"}},
			},
		},
		{
			name: "duplicate statement",
			phases: []NarrativeCompressionPhase{
				{Title: "Read input", MemberStatementIDs: []string{"statement-a"}},
				{Title: "Apply work", MemberStatementIDs: []string{"statement-a", "statement-b"}},
				{Title: "Write output", MemberStatementIDs: []string{"statement-c"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, ok := ProjectNarrativeCompression(mechanism, NarrativeCompression{
				ArtifactID: "behavior", OrderingBasis: NarrativeOrderingEditorial, Phases: test.phases,
			})
			if ok {
				t.Fatal("invalid compression was accepted")
			}
		})
	}
}

func TestProjectNarrativeCompressionRequiresCoveredAspectProjection(t *testing.T) {
	t.Parallel()

	mechanism := onboardingMechanism(
		"behavior", "How does input become output?", "Input becomes output.",
		[]string{"behavior.go"}, "Read input", "Apply work", "Write output",
	)
	mechanism.canonicalCoveredAspectIDs = []string{"input", "output"}
	mechanism.Steps[0].canonicalAspectIDs = []string{"input"}
	compression := NarrativeCompression{
		ArtifactID: "behavior", OrderingBasis: NarrativeOrderingEditorial,
		Phases: []NarrativeCompressionPhase{
			{Title: "Read input", MemberStatementIDs: []string{"statement-a"}},
			{Title: "Apply work", MemberStatementIDs: []string{"statement-b"}},
			{Title: "Write output", MemberStatementIDs: []string{"statement-c"}},
		},
	}
	if _, ok := ProjectNarrativeCompression(mechanism, compression); ok {
		t.Fatal("compression accepted without a projected covered output aspect")
	}
	mechanism.Steps[2].canonicalAspectIDs = []string{"output"}
	if _, ok := ProjectNarrativeCompression(mechanism, compression); !ok {
		t.Fatal("compression rejected after every covered aspect was projected")
	}
}

func TestDocumentedPurposeSentencesPreserveTechnicalPunctuation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{
			name:  "inline code",
			value: "Run `go run ./cmd/server --version v1.2.3`. Then inspect logs.",
			want: []string{
				"Run `go run ./cmd/server --version v1.2.3`.",
				"Then inspect logs.",
			},
		},
		{
			name:  "URL with version and decimal query",
			value: "Read https://example.com/docs/v1.2.3?ratio=1.5 before starting. Then connect.",
			want: []string{
				"Read https://example.com/docs/v1.2.3?ratio=1.5 before starting.",
				"Then connect.",
			},
		},
		{
			name:  "semantic version",
			value: "Version v1.2.3 exposes the API. The protocol remains compatible.",
			want: []string{
				"Version v1.2.3 exposes the API.",
				"The protocol remains compatible.",
			},
		},
		{
			name:  "decimal",
			value: "The timeout is 1.25 seconds. Retry once.",
			want: []string{
				"The timeout is 1.25 seconds.",
				"Retry once.",
			},
		},
		{
			name:  "repository path command",
			value: "Use go run ./cmd/server to start the service. Then open the client.",
			want: []string{
				"Use go run ./cmd/server to start the service.",
				"Then open the client.",
			},
		},
		{
			name:  "UTF-8 byte offsets",
			value: "Запустите `go run ./cmd/server`. Затем откройте клиент.",
			want: []string{
				"Запустите `go run ./cmd/server`.",
				"Затем откройте клиент.",
			},
		},
		{
			name:  "closing quotes",
			value: `The service reports "ready." Then clients may connect.`,
			want: []string{
				`The service reports "ready."`,
				"Then clients may connect.",
			},
		},
		{
			name:  "closing emphasis",
			value: "The command is **ready.** Then clients may connect.",
			want: []string{
				"The command is **ready.**",
				"Then clients may connect.",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := documentedPurposeSentences(test.value, ""); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("sentences = %#v, want %#v", got, test.want)
			}
			end := firstSentenceEnd(test.value)
			if end < 0 || test.value[:end+1] != test.want[0] {
				got := ""
				if end >= 0 {
					got = test.value[:end+1]
				}
				t.Fatalf("first sentence = %q, want %q", got, test.want[0])
			}
		})
	}
}

func onboardingMechanism(
	artifactID string,
	question string,
	answer string,
	paths []string,
	stepTitles ...string,
) UserMechanism {
	files := make([]UserCodeLocation, 0, len(paths))
	for _, filePath := range paths {
		files = append(files, UserCodeLocation{Path: filePath, Line: 1})
	}
	steps := make([]UserMechanismStep, 0, len(stepTitles))
	for index, title := range stepTitles {
		filePath := paths[min(index, len(paths)-1)]
		steps = append(steps, UserMechanismStep{
			Title: title, Explanation: title + ".",
			Locations: []UserCodeLocation{{Path: filePath, Line: index + 1}},
			Sources: []SourceSnippet{{
				Path: filePath, StartLine: index + 1, EndLine: index + 1,
				Lines:           []SourceSnippetLine{{Line: index + 1, Text: title}},
				HighlightRanges: []SourceHighlight{{StartLine: index + 1, EndLine: index + 1}},
			}},
			canonicalStatementIDs: []string{"statement-" + string(rune('a'+index))},
			canonicalStepIndex:    index,
		})
	}
	return UserMechanism{
		ArtifactID: artifactID,
		Title:      question,
		Question:   question,
		Answer:     answer,
		Steps:      steps,
		Files:      files,
	}
}
