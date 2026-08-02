package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/localization"
)

func TestMaterializeArchitectureLocalizationIdentityWritesExactProviderFreeSet(t *testing.T) {
	t.Parallel()

	for _, normalized := range []bool{false, true} {
		normalized := normalized
		t.Run(map[bool]string{false: "accepted", true: "accepted_normalized"}[normalized], func(t *testing.T) {
			t.Parallel()

			runDir := architectureLocalizationSavedRun(t, normalized)
			before := architectureLocalizationUnrelatedArtifactBytes(t, runDir)

			artifactDir, err := MaterializeArchitectureLocalizationIdentity(runDir)
			if err != nil {
				t.Fatal(err)
			}
			if artifactDir != filepath.Join(runDir, architectureLocalizationArtifactDir) {
				t.Fatalf("artifact dir = %q", artifactDir)
			}

			info, err := os.Lstat(artifactDir)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o700 {
				t.Fatalf("localization dir mode = %o, want 700", info.Mode().Perm())
			}
			payloads := readArchitectureLocalizationPayloadSet(t, artifactDir)
			for _, payload := range payloads {
				info, err := os.Lstat(filepath.Join(artifactDir, payload.name))
				if err != nil {
					t.Fatal(err)
				}
				if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
					t.Fatalf("%s mode = %v, want regular 600", payload.name, info.Mode())
				}
			}

			data, err := ReadRunDir(runDir)
			if err != nil {
				t.Fatal(err)
			}
			canvasJSON, err := json.Marshal(data.ArchitectureCanvas)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateArchitectureLocalizationIdentityPayloads(canvasJSON, payloads); err != nil {
				t.Fatalf("persisted identity replay: %v", err)
			}

			var canonical localization.CanonicalArtifact
			var input localization.Input
			var projection localization.Projection
			if err := decodeArchitectureLocalizationJSON(payloads[0].data, &canonical); err != nil {
				t.Fatal(err)
			}
			if err := decodeArchitectureLocalizationJSON(payloads[1].data, &input); err != nil {
				t.Fatal(err)
			}
			if err := decodeArchitectureLocalizationJSON(payloads[2].data, &projection); err != nil {
				t.Fatal(err)
			}
			if canonical.Locale != localization.LocaleEnglish ||
				input.SourceLocale != localization.LocaleEnglish ||
				input.TargetLocale != localization.LocaleEnglish ||
				projection.Locale != localization.LocaleEnglish ||
				input.CanonicalSHA256 != canonical.SHA256 ||
				projection.CanonicalSHA256 != canonical.SHA256 {
				t.Fatalf(
					"identity envelopes = canonical %#v input %#v projection %#v",
					canonical,
					input,
					projection,
				)
			}

			first := append([]architectureLocalizationArtifactPayload(nil), payloads...)
			if _, err := MaterializeArchitectureLocalizationIdentity(runDir); err != nil {
				t.Fatalf("second materialization: %v", err)
			}
			second := readArchitectureLocalizationPayloadSet(t, artifactDir)
			for index := range first {
				if !bytes.Equal(first[index].data, second[index].data) {
					t.Fatalf("%s changed across identical materialization", first[index].name)
				}
			}
			after := architectureLocalizationUnrelatedArtifactBytes(t, runDir)
			if !equalArchitectureLocalizationByteMap(before, after) {
				t.Fatalf("unrelated run artifacts changed: before=%v after=%v", before, after)
			}

			entries, err := os.ReadDir(artifactDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 3 {
				t.Fatalf("localization entries = %v, want exact three files", entries)
			}
		})
	}
}

func TestBuildArchitectureLocalizationIdentityPayloadsRejectsIneligibleLanguageAndStatus(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	statusJSON, err := os.ReadFile(filepath.Join(runDir, ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	synthesisJSON, err := os.ReadFile(filepath.Join(runDir, ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		status    []byte
		synthesis []byte
		want      string
	}{
		{
			name:      "Russian",
			status:    statusJSON,
			synthesis: bytes.Replace(synthesisJSON, []byte(`"output_language":"en"`), []byte(`"output_language":"ru"`), 1),
			want:      "not explicit current English",
		},
		{
			name:      "legacy language unknown",
			status:    statusJSON,
			synthesis: bytes.Replace(synthesisJSON, []byte(`,"output_language":"en"`), nil, 1),
			want:      "not explicit current English",
		},
		{
			name: "failed status",
			status: []byte(`{
				"version":2,
				"state":"failed",
				"error_code":"invalid_response"
			}`),
			synthesis: synthesisJSON,
			want:      "not an accepted current v2 result",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildArchitectureLocalizationIdentityPayloads(
				data,
				test.status,
				test.synthesis,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildArchitectureLocalizationIdentityPayloadsRejectsMalformedOrStaleInputs(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	statusJSON, err := os.ReadFile(filepath.Join(runDir, ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatal(err)
	}
	synthesisJSON, err := os.ReadFile(filepath.Join(runDir, ArchitectureSynthesisFile))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("unknown status field", func(t *testing.T) {
		unknown := bytes.Replace(statusJSON, []byte(`{`), []byte(`{"unexpected":true,`), 1)
		_, err := buildArchitectureLocalizationIdentityPayloads(data, unknown, synthesisJSON)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("trailing synthesis value", func(t *testing.T) {
		trailing := append(append([]byte(nil), synthesisJSON...), []byte(` {}`)...)
		_, err := buildArchitectureLocalizationIdentityPayloads(data, statusJSON, trailing)
		if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("stale current facts", func(t *testing.T) {
		stale := *data
		graph := *data.RepositoryGraph
		graph.Modules = append(
			append([]ModuleInfo(nil), graph.Modules...),
			ModuleInfo{Path: "example.com/changed"},
		)
		graph.PackageEdges = append(
			append([]EdgeInfo(nil), graph.PackageEdges...),
			EdgeInfo{
				From: "example.com/changed/cmd",
				To:   "example.com/project/internal/repo",
			},
		)
		stale.RepositoryGraph = &graph
		_, err := buildArchitectureLocalizationIdentityPayloads(&stale, statusJSON, synthesisJSON)
		if err == nil || !strings.Contains(err.Error(), "replay current synthesis") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("fallback canvas", func(t *testing.T) {
		fallback := *data
		canvas := *data.ArchitectureCanvas
		canvas.Fallback = true
		canvas.FallbackReason = componentmap.FallbackProposalInvalid
		fallback.ArchitectureCanvas = &canvas
		_, err := buildArchitectureLocalizationIdentityPayloads(&fallback, statusJSON, synthesisJSON)
		if err == nil || !strings.Contains(err.Error(), "fallback Architecture Canvas") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("canvas metadata mismatch", func(t *testing.T) {
		mismatch := *data
		canvas := *data.ArchitectureCanvas
		canvas.ArchitectureLevel++
		mismatch.ArchitectureCanvas = &canvas
		_, err := buildArchitectureLocalizationIdentityPayloads(&mismatch, statusJSON, synthesisJSON)
		if err == nil || !strings.Contains(err.Error(), "does not match the current canvas") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestReadArchitectureLocalizationArtifactInputBoundsBeforeAllocation(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	root, err := os.OpenRoot(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	for _, test := range []struct {
		name  string
		limit int64
	}{
		{name: ArchitectureSynthesisStatusFile, limit: maxArchitectureLocalizationStatusBytes},
		{name: ArchitectureSynthesisFile, limit: maxArchitectureLocalizationSynthesisBytes},
	} {
		if err := os.WriteFile(
			filepath.Join(runDir, test.name),
			bytes.Repeat([]byte("x"), int(test.limit)+1),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := readArchitectureLocalizationArtifactInput(
			root,
			test.name,
			test.limit,
		); err == nil || !strings.Contains(err.Error(), "not a bounded regular file") {
			t.Fatalf("%s error = %v", test.name, err)
		}
	}
}

func TestMaterializeArchitectureLocalizationIdentityRejectsOversizeBeforeRunRead(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		status    []byte
		synthesis []byte
		want      string
	}{
		{
			name:      "status",
			status:    bytes.Repeat([]byte("x"), maxArchitectureLocalizationStatusBytes+1),
			synthesis: []byte(`{}`),
			want:      ArchitectureSynthesisStatusFile,
		},
		{
			name:      "synthesis",
			status:    []byte(`{}`),
			synthesis: bytes.Repeat([]byte("x"), maxArchitectureLocalizationSynthesisBytes+1),
			want:      ArchitectureSynthesisFile,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir := t.TempDir()
			writeArchitectureBuildFixture(
				t,
				runDir,
				ArchitectureSynthesisStatusFile,
				test.status,
			)
			writeArchitectureBuildFixture(
				t,
				runDir,
				ArchitectureSynthesisFile,
				test.synthesis,
			)
			_, err := MaterializeArchitectureLocalizationIdentity(runDir)
			if err == nil ||
				!strings.Contains(err.Error(), test.want) ||
				!strings.Contains(err.Error(), "bounded regular file") {
				t.Fatalf("error = %v", err)
			}
			if _, statErr := os.Lstat(
				filepath.Join(runDir, architectureLocalizationArtifactDir),
			); !os.IsNotExist(statErr) {
				t.Fatalf("localization dir exists after bounded rejection: %v", statErr)
			}
		})
	}
}

func TestWriteArchitectureLocalizationIdentityPayloadsRejectsSymlinksAndConflicts(t *testing.T) {
	t.Parallel()

	payloads := []architectureLocalizationArtifactPayload{
		{name: architectureLocalizationCanonicalFile, data: []byte(`{"one":1}`)},
		{name: architectureLocalizationInputFile, data: []byte(`{"two":2}`)},
		{name: architectureLocalizationProjectionFile, data: []byte(`{"three":3}`)},
	}

	t.Run("directory symlink", func(t *testing.T) {
		runDir := t.TempDir()
		victim := t.TempDir()
		if err := os.Symlink(victim, filepath.Join(runDir, architectureLocalizationArtifactDir)); err != nil {
			t.Fatal(err)
		}
		err := writeArchitectureLocalizationIdentityPayloads(runDir, payloads)
		if err == nil || !strings.Contains(err.Error(), "not a real directory") {
			t.Fatalf("error = %v", err)
		}
		entries, readErr := os.ReadDir(victim)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("symlink victim changed: %v", entries)
		}
	})

	t.Run("leaf symlink", func(t *testing.T) {
		runDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(runDir, architectureLocalizationArtifactDir), 0o700); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(t.TempDir(), "victim.json")
		const victimBytes = "keep me"
		if err := os.WriteFile(victim, []byte(victimBytes), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(
			victim,
			filepath.Join(
				runDir,
				architectureLocalizationArtifactDir,
				architectureLocalizationCanonicalFile,
			),
		); err != nil {
			t.Fatal(err)
		}
		err := writeArchitectureLocalizationIdentityPayloads(runDir, payloads)
		if err == nil || !strings.Contains(err.Error(), "not a bounded regular file") {
			t.Fatalf("error = %v", err)
		}
		got, readErr := os.ReadFile(victim)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != victimBytes {
			t.Fatalf("victim = %q", got)
		}
	})

	t.Run("conflicting regular file", func(t *testing.T) {
		runDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(runDir, architectureLocalizationArtifactDir), 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(
			runDir,
			architectureLocalizationArtifactDir,
			architectureLocalizationCanonicalFile,
		)
		if err := os.WriteFile(path, []byte("owner data"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := writeArchitectureLocalizationIdentityPayloads(runDir, payloads)
		if err == nil || !strings.Contains(err.Error(), "conflicts with current identity") {
			t.Fatalf("error = %v", err)
		}
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != "owner data" {
			t.Fatalf("conflicting file was overwritten: %q", got)
		}
	})

	t.Run("incomplete exact set", func(t *testing.T) {
		runDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(runDir, architectureLocalizationArtifactDir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(
				runDir,
				architectureLocalizationArtifactDir,
				architectureLocalizationCanonicalFile,
			),
			payloads[0].data,
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		err := writeArchitectureLocalizationIdentityPayloads(runDir, payloads)
		if err == nil || !strings.Contains(err.Error(), "existing identity set is incomplete") {
			t.Fatalf("error = %v", err)
		}
		entries, readErr := os.ReadDir(filepath.Join(runDir, architectureLocalizationArtifactDir))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 1 || entries[0].Name() != architectureLocalizationCanonicalFile {
			t.Fatalf("incomplete owner set changed: %v", entries)
		}
	})
}

func TestInstallArchitectureLocalizationIdentityPayloadsRollsBackPartialSet(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(runDir, architectureLocalizationArtifactDir), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	payloads := []architectureLocalizationArtifactPayload{
		{name: architectureLocalizationCanonicalFile, data: []byte(`{"one":1}`)},
		{name: architectureLocalizationInputFile, data: []byte(`{"two":2}`)},
		{name: architectureLocalizationProjectionFile, data: []byte(`{"three":3}`)},
	}
	renames := 0
	err = installArchitectureLocalizationIdentityPayloads(
		root,
		payloads,
		func(from, to string) error {
			renames++
			if renames == 2 {
				return errors.New("injected rename failure")
			}
			return root.Rename(from, to)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(runDir, architectureLocalizationArtifactDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial artifact set survived rollback: %v", entries)
	}
}

func TestValidateArchitectureLocalizationIdentityPayloadsRejectsSecretWithoutEcho(t *testing.T) {
	t.Parallel()

	const secret = "company-secret-value-12345"
	canonical, err := localization.NewCanonical([]localization.FieldSpec{{
		OwnerKind: localization.OwnerComponent,
		OwnerID:   "component-1",
		Name:      localization.FieldDescription,
		Text:      `api_key="` + secret + `"`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := localization.BuildInput(canonical, localization.LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := localization.IdentityProjection(canonical)
	if err != nil {
		t.Fatal(err)
	}
	canonicalJSON, err := localization.MarshalCanonical(canonical)
	if err != nil {
		t.Fatal(err)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	projectionJSON, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	payloads := []architectureLocalizationArtifactPayload{
		{name: architectureLocalizationCanonicalFile, data: canonicalJSON},
		{name: architectureLocalizationInputFile, data: inputJSON},
		{name: architectureLocalizationProjectionFile, data: projectionJSON},
	}
	err = validateArchitectureLocalizationIdentityPayloads([]byte(`{}`), payloads)
	if err == nil || !strings.Contains(err.Error(), "obvious credential") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("secret-like value was echoed")
	}
}

func TestValidateArchitectureLocalizationIdentityPayloadsRejectsOversizeBeforeDecode(t *testing.T) {
	t.Parallel()

	payloads := []architectureLocalizationArtifactPayload{
		{
			name: architectureLocalizationCanonicalFile,
			data: bytes.Repeat([]byte("x"), maxArchitectureLocalizationArtifactBytes+1),
		},
		{name: architectureLocalizationInputFile, data: []byte(`{}`)},
		{name: architectureLocalizationProjectionFile, data: []byte(`{}`)},
	}
	err := validateArchitectureLocalizationIdentityPayloads([]byte(`{}`), payloads)
	if err == nil || !strings.Contains(err.Error(), "exceeds its byte limit") {
		t.Fatalf("error = %v", err)
	}
}

func architectureLocalizationSavedRun(t *testing.T, normalized bool) string {
	t.Helper()

	runDir := t.TempDir()
	writeArchitectureBuildFixture(t, runDir, "snapshot.json", []byte(`{"repo_name":"fixture"}`))
	writeArchitectureBuildFixture(t, runDir, "llm_bundle.json", []byte(`{
		"go": {
			"module_summaries": [{"module_path":"example.com/project","module_dir":"."}],
			"important_edges": [{"from":"example.com/project/cmd","to":"example.com/project/internal/repo"}]
		}
	}`))
	writeArchitectureBuildFixture(t, runDir, "orientation_report.json", []byte(`{
		"project_guess":"saved fixture",
		"candidate_flows":[{
			"name":"Server startup",
			"trigger":"process starts",
			"likely_entrypoint":"cmd/main.go",
			"likely_files":["cmd/main.go"]
		}]
	}`))
	if normalized {
		architectureLocalizationWriteNormalizedGrounding(t, runDir)
	}

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := componentmap.BuildSynthesisRequest(input.CandidateBundle)
	if err != nil {
		t.Fatal(err)
	}
	components := make([]architectureTestWireComponent, 0, len(request.Candidates))
	componentByRef := make(map[string]int, len(request.Candidates))
	for index, candidate := range request.Candidates {
		if index >= len(input.CandidateBundle.Candidates) {
			t.Fatalf("request-local candidate %d has no exact fixture member", index)
		}
		candidateName := input.CandidateBundle.Candidates[index].Name
		componentByRef[string(candidate.Ref.Kind)+"\x00"+candidate.Ref.Ref] = len(components)
		components = append(components, architectureTestWireComponent{
			Name:        "Runtime member " + candidateName,
			Description: "Owns " + candidateName + " in the exact saved fixture.",
			MemberRefs:  []componentmap.SynthesisMemberRef{candidate.Ref},
			Hypothesis:  input.CandidateBundle.GroundingMode != componentmap.GroundingPackages,
		})
	}
	for _, anchor := range request.BehaviorAnchors {
		for _, memberRef := range anchor.MemberRefs {
			index, ok := componentByRef[string(memberRef.Kind)+"\x00"+memberRef.Ref]
			if !ok {
				t.Fatalf("behavior anchor member ref is outside the exact fixture catalog: %#v", memberRef)
			}
			components[index].AnchorRefs = append(components[index].AnchorRefs, anchor.Ref)
			components[index].Hypothesis = false
		}
	}
	description := "Groups the exact saved fixture runtime."
	if normalized {
		normalizedIndex := -1
		for index, component := range components {
			if len(component.AnchorRefs) == 0 && component.MemberRefs[0].Kind == componentmap.MemberPackage {
				normalizedIndex = index
				break
			}
		}
		if normalizedIndex < 0 {
			t.Fatal("normalized fixture has no unanchored package-only component")
		}
		components[normalizedIndex].Hypothesis = false
	}
	response := marshalArchitectureTestWireResponse(t, architectureTestWireResponse{
		Subsystems: []architectureTestWireSubsystem{{
			Name:        "Runtime",
			Description: description,
			Components:  components,
		}},
	})
	result, err := componentmap.RecordSynthesisResponseForLanguage(
		input.CandidateBundle,
		"revision-localization",
		"test",
		"test-model",
		localization.LocaleEnglish,
		time.Millisecond,
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback {
		t.Fatalf("fixture synthesis fell back: %#v", result.Landscape)
	}
	if normalized && result.Landscape.ValidationOutcome != componentmap.ValidationAcceptedNormalized {
		t.Fatalf("normalized fixture outcome = %q", result.Landscape.ValidationOutcome)
	}
	if !normalized && result.Landscape.ValidationOutcome != componentmap.ValidationAccepted {
		t.Fatalf("accepted fixture outcome = %q", result.Landscape.ValidationOutcome)
	}
	result.Record.Call.Metadata.UsageReported = true
	result.Record.Call.Metadata.InputTokens = 25
	result.Record.Call.Metadata.OutputTokens = 11
	result.Record.Call.Metadata.FinishReason = "stop"
	result.Record.Call.Metadata.TransportAttempts = 1
	result.Record.Call.Metadata.ResponseComplete = true
	recordJSON, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisFile, recordJSON)

	metadata := result.Record.Call.Metadata
	status := architectureSynthesisV4AcceptedFixture()
	status.RequestBytes = architectureTestExactProviderRequestBytes(
		t,
		input.CandidateBundle,
		localization.LocaleEnglish,
	)
	status.ResponseBytes = result.Record.Call.ResponseBytes
	status.ResponseContentBytes = len(result.Record.Call.Response)
	status.CandidateCount = len(input.CandidateBundle.Candidates)
	status.AnchorCount = len(input.CandidateBundle.BehaviorAnchors)
	status.MemberOccurrences = len(input.CandidateBundle.Candidates)
	status.DistinctMembers = len(input.CandidateBundle.Candidates)
	status.ProposalNormalized = metadata.ValidationOutcome == componentmap.ValidationAcceptedNormalized
	status.ArchitectureSource = string(metadata.ArchitectureSource)
	status.ArchitectureLevel = metadata.ArchitectureLevel
	status.NormalizationCount = len(metadata.Normalizations)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	statusJSON, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisStatusFile, statusJSON)
	return runDir
}

func architectureLocalizationWriteNormalizedGrounding(t *testing.T, runDir string) {
	t.Helper()
	location := evidence.Location{Path: "cmd/main.go", Line: 1, Column: 1}
	grounding := ArchitectureGrounding{
		Version: ArchitectureGroundingVersion,
		RepositoryArchetype: ArchitectureArchetype{
			Selected: componentmap.ArchetypeApplication,
			Evidence: []string{"Exact fixture process entry."},
		},
		GroundingMode: componentmap.GroundingMixed,
		BehaviorAnchors: []ArchitectureBehaviorAnchor{{
			ID: "fixture-process-entry", Kind: componentmap.AnchorProcessEntry,
			Label: "Fixture process entry", Location: location,
			Scenario: architectureGroundingScenario{
				ID: "go:test", GOOS: "darwin", GOARCH: "arm64",
			},
			Producer: evidence.Provenance{
				Provider: "go_syntax", Version: "fixture-v1", Operation: "process_entry",
				Location: &location,
			},
			Certainty: evidence.CertaintyStatic,
			AssociatedMembers: []ArchitectureAnchorMember{{
				ID: "example.com/project/cmd.main", Package: "example.com/project/cmd",
				Name: "main", Location: location,
			}},
			Limitations: []string{"Static fixture evidence; execution is not observed."},
		}},
	}
	encoded, err := json.Marshal(grounding)
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, ArchitectureGroundingFile, encoded)
}

func readArchitectureLocalizationPayloadSet(
	t *testing.T,
	artifactDir string,
) []architectureLocalizationArtifactPayload {
	t.Helper()

	names := []string{
		architectureLocalizationCanonicalFile,
		architectureLocalizationInputFile,
		architectureLocalizationProjectionFile,
	}
	payloads := make([]architectureLocalizationArtifactPayload, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(artifactDir, name))
		if err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, architectureLocalizationArtifactPayload{name: name, data: data})
	}
	return payloads
}

func architectureLocalizationUnrelatedArtifactBytes(
	t *testing.T,
	runDir string,
) map[string][]byte {
	t.Helper()

	result := make(map[string][]byte)
	for _, name := range []string{
		"report.json",
		"report.html",
		RunManifestFilename,
		ArchitectureSynthesisFile,
		ArchitectureSynthesisStatusFile,
	} {
		data, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				result[name] = nil
				continue
			}
			t.Fatal(err)
		}
		result[name] = data
	}
	return result
}

func equalArchitectureLocalizationByteMap(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if !bytes.Equal(value, right[key]) {
			return false
		}
	}
	return true
}
