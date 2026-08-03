package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

func TestReplayArchitectureLocalizationRussianFileIsExactAndReadOnly(t *testing.T) {
	t.Parallel()

	for _, normalized := range []bool{false, true} {
		normalized := normalized
		t.Run(map[bool]string{false: "accepted", true: "accepted_normalized"}[normalized], func(t *testing.T) {
			t.Parallel()

			runDir := architectureLocalizationSavedRun(t, normalized)
			context, canonical, input := architectureLocalizationRussianContext(t, runDir)
			projectionPath := filepath.Join(t.TempDir(), "architecture.ru.projection.v1.json")
			projectionJSON := architectureLocalizationRussianProjectionJSON(t, canonical, input)
			if err := os.WriteFile(projectionPath, projectionJSON, 0o600); err != nil {
				t.Fatal(err)
			}
			before := architectureLocalizationReplayRunSnapshot(t, runDir)

			first, err := ReplayArchitectureLocalizationRussianFile(runDir, projectionPath)
			if err != nil {
				t.Fatal(err)
			}
			second, err := ReplayArchitectureLocalizationRussianFile(runDir, projectionPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Fatal("identical Russian replay was not byte-stable")
			}
			after := architectureLocalizationReplayRunSnapshot(t, runDir)
			if !equalArchitectureLocalizationByteMap(before, after) {
				t.Fatalf("Russian replay changed its run directory: before=%v after=%v", before, after)
			}

			var replay ArchitectureLocalizationReplay
			if err := decodeArchitectureLocalizationJSON(first, &replay); err != nil {
				t.Fatal(err)
			}
			if replay.Version != ArchitectureLocalizationReplayVersion ||
				replay.CanonicalSHA256 != canonical.SHA256 ||
				replay.Locale != localization.LocaleRussian ||
				replay.Fallback ||
				len(replay.Diagnostics) != 0 {
				t.Fatalf("Russian replay envelope = %#v", replay)
			}
			assertArchitectureLocalizationRussianProjection(
				t,
				context.canvas,
				replay.Canvas,
				canonical,
			)
		})
	}
}

func TestReplayArchitectureLocalizationRussianPreservesOneFieldFallback(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	context, canonical, input := architectureLocalizationRussianContext(t, runDir)
	valid := architectureLocalizationRussianProjection(t, canonical, input)
	placeholderID := ""
	for _, field := range input.Fields {
		if len(field.Placeholders) > 0 {
			placeholderID = field.ID
			break
		}
	}
	if placeholderID == "" {
		t.Fatal("fixture has no protected placeholder field")
	}

	for _, test := range []struct {
		name       string
		fieldID    string
		code       string
		invalidate func(map[string]string)
	}{
		{
			name:    "missing translation",
			fieldID: input.Fields[0].ID,
			code:    "missing_translation",
			invalidate: func(translations map[string]string) {
				delete(translations, input.Fields[0].ID)
			},
		},
		{
			name:    "placeholder mismatch",
			fieldID: placeholderID,
			code:    "placeholder_mismatch",
			invalidate: func(translations map[string]string) {
				translations[placeholderID] = "Архитектурная область без обязательного термина."
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			projection := valid
			projection.Translations = cloneArchitectureLocalizationTranslations(valid.Translations)
			test.invalidate(projection.Translations)
			projectionJSON, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}

			encoded, err := ReplayArchitectureLocalizationRussian(runDir, projectionJSON)
			if err != nil {
				t.Fatal(err)
			}
			var replay ArchitectureLocalizationReplay
			if err := decodeArchitectureLocalizationJSON(encoded, &replay); err != nil {
				t.Fatal(err)
			}
			want := []localization.Diagnostic{{
				Code:    test.code,
				FieldID: test.fieldID,
			}}
			if !replay.Fallback ||
				!equalArchitectureLocalizationReplayDiagnostics(replay.Diagnostics, want) {
				t.Fatalf("fallback replay = %#v, want diagnostics %#v", replay, want)
			}
			originalByID := architectureLocalizationReplayTextByID(t, context.canvas)
			projectedByID := architectureLocalizationReplayTextByID(t, replay.Canvas)
			if projectedByID[test.fieldID] != originalByID[test.fieldID] {
				t.Fatalf("invalid field %q did not retain canonical English", test.fieldID)
			}
			translated := 0
			for id, original := range originalByID {
				if id != test.fieldID && projectedByID[id] != original {
					translated++
				}
			}
			if translated == 0 {
				t.Fatal("one invalid field discarded every valid Russian translation")
			}
		})
	}
}

func TestReplayArchitectureLocalizationRussianKeepsEnvelopeFailureAtomic(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	context, canonical, input := architectureLocalizationRussianContext(t, runDir)
	valid := architectureLocalizationRussianProjection(t, canonical, input)
	for _, test := range []struct {
		name       string
		mutate     func(*localization.Projection)
		diagnostic string
	}{
		{
			name: "version",
			mutate: func(projection *localization.Projection) {
				projection.Version++
			},
			diagnostic: "projection_version_mismatch",
		},
		{
			name: "canonical hash",
			mutate: func(projection *localization.Projection) {
				projection.CanonicalSHA256 = strings.Repeat("0", 64)
			},
			diagnostic: "canonical_hash_mismatch",
		},
		{
			name: "locale",
			mutate: func(projection *localization.Projection) {
				projection.Locale = localization.LocaleEnglish
			},
			diagnostic: "projection_locale_mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			projection := valid
			projection.Translations = cloneArchitectureLocalizationTranslations(valid.Translations)
			test.mutate(&projection)
			data, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := ReplayArchitectureLocalizationRussian(runDir, data)
			if err != nil {
				t.Fatal(err)
			}
			var replay ArchitectureLocalizationReplay
			if err := decodeArchitectureLocalizationJSON(encoded, &replay); err != nil {
				t.Fatal(err)
			}
			if !replay.Fallback ||
				!equalArchitectureLocalizationReplayDiagnostics(replay.Diagnostics, []localization.Diagnostic{{
					Code: test.diagnostic,
				}}) {
				t.Fatalf("envelope replay = %#v", replay)
			}
			before, err := json.Marshal(context.canvas)
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(replay.Canvas)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("%s envelope failure changed the canonical Canvas", test.name)
			}
		})
	}
}

func TestReplayArchitectureLocalizationRussianRejectsMalformedBeforeRunWork(t *testing.T) {
	t.Parallel()

	invalidUTF8 := []byte(`{"version":1,"canonical_sha256":"`)
	invalidUTF8 = append(invalidUTF8, 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","locale":"ru","translations":{}}`)...)
	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: "byte limit"},
		{name: "oversize", data: bytes.Repeat([]byte("x"), maxArchitectureLocalizationArtifactBytes+1), want: "byte limit"},
		{name: "invalid UTF-8", data: invalidUTF8, want: "valid UTF-8"},
		{name: "unknown field", data: []byte(`{"version":1,"canonical_sha256":"x","locale":"ru","translations":{},"extra":true}`), want: "strict JSON"},
		{name: "trailing JSON", data: []byte(`{"version":1,"canonical_sha256":"x","locale":"ru","translations":{}} {}`), want: "strict JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReplayArchitectureLocalizationRussian(
				filepath.Join(t.TempDir(), "missing-run"),
				test.data,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "inspect run dir") {
				t.Fatalf("projection preflight did not precede run work: %v", err)
			}
		})
	}
}

func TestReplayArchitectureLocalizationRussianDoesNotEchoEscapedUnknownField(t *testing.T) {
	t.Parallel()

	const secret = `company-secret-value-123456789`
	data := []byte(
		`{"version":1,"canonical_sha256":"x","locale":"ru","translations":{},` +
			`"\u0061pi_key=\u0022` + secret + `\u0022":true}`,
	)
	_, err := ReplayArchitectureLocalizationRussian(
		filepath.Join(t.TempDir(), "missing-run"),
		data,
	)
	if err == nil || !strings.Contains(err.Error(), "strict JSON") {
		t.Fatalf("escaped unknown field error = %v", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "api_key") {
		t.Fatalf("escaped unknown field leaked through decoder error: %v", err)
	}
}

func TestReplayArchitectureLocalizationRussianFileRejectsNonRegularAndSensitiveInput(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	projectionPath := filepath.Join(directory, "projection.json")
	if err := os.WriteFile(
		projectionPath,
		bytes.Repeat([]byte("x"), maxArchitectureLocalizationArtifactBytes+1),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplayArchitectureLocalizationRussianFile(
		t.TempDir(),
		projectionPath,
	); err == nil || !strings.Contains(err.Error(), "bounded regular file") {
		t.Fatalf("oversize file error = %v", err)
	}

	target := filepath.Join(directory, "target.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "symlink.json")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplayArchitectureLocalizationRussianFile(
		t.TempDir(),
		symlink,
	); err == nil || !strings.Contains(err.Error(), "bounded regular file") {
		t.Fatalf("symlink error = %v", err)
	}

	const secret = "sk-sensitive-replay-value"
	sensitive := []byte(`{
		"version":1,
		"canonical_sha256":"` + strings.Repeat("0", 64) + `",
		"locale":"ru",
		"translations":{"component:test.description":"api_key=\"` + secret + `\""}
	}`)
	_, err := ReplayArchitectureLocalizationRussian(t.TempDir(), sensitive)
	if err == nil || !strings.Contains(err.Error(), "obvious ") {
		t.Fatalf("sensitive response error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("sensitive response error echoed the credential-like value")
	}

	runDir := architectureLocalizationSavedRun(t, false)
	_, canonical, input := architectureLocalizationRussianContext(t, runDir)
	projection := architectureLocalizationRussianProjection(t, canonical, input)
	for id := range projection.Translations {
		projection.Translations[id] = `api_key="sk-sensitive-replay-value"`
		break
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	escaped := bytes.ReplaceAll(encoded, []byte("api_key"), []byte(`\u0061pi_key`))
	escaped = bytes.ReplaceAll(escaped, []byte("sk-sensitive"), []byte(`\u0073\u006b-sensitive`))
	if secretscanKind, found := secretscan.Detect(string(escaped)); found {
		t.Fatalf("escaped fixture unexpectedly matched raw secret scan as %s", secretscanKind)
	}
	_, err = ReplayArchitectureLocalizationRussian(runDir, escaped)
	if err == nil || !strings.Contains(err.Error(), "decoded projection contains an obvious ") {
		t.Fatalf("escaped sensitive response error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("escaped sensitive response error echoed the credential-like value")
	}
}

func TestArchitectureLocalizationReplayPreflightScansTypedCanvasAndBoundsEncoding(t *testing.T) {
	t.Parallel()

	replay := ArchitectureLocalizationReplay{
		Version: ArchitectureLocalizationReplayVersion,
		Locale:  localization.LocaleRussian,
		Canvas: ArchitectureCanvas{
			Title: `API_KEY="abcdefghijklmnop"`,
		},
	}
	encoded, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	if kind, found := secretscan.Detect(string(encoded)); found {
		t.Fatalf("escaped replay unexpectedly matched raw secret scan as %s", kind)
	}
	kind, found, err := preflightArchitectureLocalizationReplay(replay)
	if err != nil {
		t.Fatal(err)
	}
	if !found || kind != "credential assignment" {
		t.Fatalf("typed preflight = %q, %v", kind, found)
	}

	replay.Canvas.Title = strings.Repeat("x", maxArchitectureLocalizationArtifactBytes+1)
	if _, _, err := preflightArchitectureLocalizationReplay(replay); err == nil ||
		!strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversize replay preflight error = %v", err)
	}
}

func TestArchitectureLocalizationReplayPreflightUpperBoundsJSONEncoding(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"ordinary ASCII",
		"кавычки \" slash \\\\ CJK 路径",
		"<script>&\u2028\u2029",
		string([]byte{'a', 0, '\n', 0xff, 'z'}),
	} {
		state := architectureLocalizationReplayPreflight{
			remaining: maxArchitectureLocalizationArtifactBytes,
		}
		if err := state.consumeString(value, false); err != nil {
			t.Fatal(err)
		}
		upperBound := maxArchitectureLocalizationArtifactBytes - state.remaining
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if len(encoded) > upperBound {
			t.Fatalf(
				"encoded string length %d exceeds preflight bound %d for %q",
				len(encoded),
				upperBound,
				value,
			)
		}
	}
}

func architectureLocalizationRussianContext(
	t *testing.T,
	runDir string,
) (architectureLocalizationIdentityContext, localization.CanonicalArtifact, localization.Input) {
	t.Helper()

	context, err := prepareArchitectureLocalizationIdentity(runDir)
	if err != nil {
		t.Fatal(err)
	}
	canonical, input, err := buildArchitectureLocalization(
		context.canvas,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	return context, canonical, input
}

func architectureLocalizationRussianProjection(
	t *testing.T,
	canonical localization.CanonicalArtifact,
	input localization.Input,
) localization.Projection {
	t.Helper()

	translations := make(map[string]string, len(input.Fields))
	for _, field := range input.Fields {
		var translated string
		switch {
		case strings.HasSuffix(field.ID, "."+string(localization.FieldNameText)):
			translated = "Архитектурная область"
		case strings.HasSuffix(field.ID, "."+string(localization.FieldDescription)):
			translated = "Описывает ответственность этой части репозитория"
		default:
			t.Fatalf("unexpected Architecture localization field %q", field.ID)
		}
		for _, placeholder := range field.Placeholders {
			for count := 0; count < placeholder.Count; count++ {
				translated += " " + placeholder.Token
			}
		}
		translations[field.ID] = translated + "."
	}
	return localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    translations,
	}
}

func architectureLocalizationRussianProjectionJSON(
	t *testing.T,
	canonical localization.CanonicalArtifact,
	input localization.Input,
) []byte {
	t.Helper()

	data, err := json.Marshal(architectureLocalizationRussianProjection(t, canonical, input))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func architectureLocalizationProviderResponse(
	t *testing.T,
	canonical localization.CanonicalArtifact,
	input localization.Input,
	projection localization.Projection,
) []byte {
	t.Helper()

	translations := make([]localization.ProviderTranslation, len(input.Fields))
	for index, field := range input.Fields {
		text, found := projection.Translations[field.ID]
		if !found {
			t.Fatalf("provider response is missing translation %q", field.ID)
		}
		translations[index] = localization.NewProviderTranslation(index, text)
	}
	response, err := json.Marshal(localization.ProviderResponse{
		Version:         localization.ProviderResponseVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          input.TargetLocale,
		Translations:    translations,
	})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertArchitectureLocalizationRussianProjection(
	t *testing.T,
	original,
	projected ArchitectureCanvas,
	canonical localization.CanonicalArtifact,
) {
	t.Helper()

	projectedByID := architectureLocalizationReplayTextByID(t, projected)
	for _, field := range canonical.Fields {
		text := projectedByID[field.ID]
		if !strings.ContainsAny(text, "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯабвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
			t.Fatalf("field %q is not genuinely Russian: %q", field.ID, text)
		}
		if strings.Contains(text, "{{term_") {
			t.Fatalf("field %q retained a placeholder: %q", field.ID, text)
		}
		for _, term := range field.ProtectedTerms {
			if strings.Count(text, term.Value) != term.Count {
				t.Fatalf(
					"field %q protected term %q count = %d, want %d",
					field.ID,
					term.Value,
					strings.Count(text, term.Value),
					term.Count,
				)
			}
		}
	}

	reset := projected
	reset.Subsystems = append([]ArchitectureSubsystem(nil), projected.Subsystems...)
	reset.Components = append([]ArchitectureComponent(nil), projected.Components...)
	for index := range reset.Subsystems {
		reset.Subsystems[index].Name = original.Subsystems[index].Name
		reset.Subsystems[index].Description = original.Subsystems[index].Description
	}
	for index := range reset.Components {
		reset.Components[index].Name = original.Components[index].Name
		reset.Components[index].Description = original.Components[index].Description
	}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(reset)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("Russian projection changed structured Architecture data:\n%s\n%s", before, after)
	}
}

func architectureLocalizationReplayTextByID(
	t *testing.T,
	canvas ArchitectureCanvas,
) map[string]string {
	t.Helper()

	result := make(map[string]string, 2*(len(canvas.Subsystems)+len(canvas.Components)))
	add := func(kind localization.OwnerKind, ownerID string, name localization.FieldName, text string) {
		id, err := localization.FieldID(kind, ownerID, name)
		if err != nil {
			t.Fatal(err)
		}
		result[id] = text
	}
	for _, subsystem := range canvas.Subsystems {
		add(localization.OwnerSubsystem, string(subsystem.ID), localization.FieldNameText, subsystem.Name)
		if subsystem.Description != "" {
			add(localization.OwnerSubsystem, string(subsystem.ID), localization.FieldDescription, subsystem.Description)
		}
	}
	for _, component := range canvas.Components {
		add(localization.OwnerComponent, string(component.ID), localization.FieldNameText, component.Name)
		if component.Description != "" {
			add(localization.OwnerComponent, string(component.ID), localization.FieldDescription, component.Description)
		}
	}
	return result
}

func architectureLocalizationReplayRunSnapshot(
	t *testing.T,
	runDir string,
) map[string][]byte {
	t.Helper()

	result := make(map[string][]byte)
	err := filepath.WalkDir(runDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(runDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func cloneArchitectureLocalizationTranslations(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for id, text := range source {
		result[id] = text
	}
	return result
}

func equalArchitectureLocalizationReplayDiagnostics(
	left,
	right []localization.Diagnostic,
) bool {
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
