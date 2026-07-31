package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/report"
)

func TestReadCanonicalReportHydratesProducerOwnedWarningSidecar(t *testing.T) {
	t.Parallel()

	runDir, rawWarning := writePresentationWarningSidecarRun(t)
	data, err := readCanonicalReportForLocalization(runDir)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := report.PreparePresentationLocalization(
		data,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range prepared.Canonical.Fields {
		if field.Text == rawWarning {
			t.Fatal("CLI localization inventory retained a producer-owned warning")
		}
	}

	if err := os.WriteFile(
		filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
		[]byte(`{"version":99,"orientation_report_sha256":"","diagnostics":[]}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := readCanonicalReportForLocalization(runDir); err == nil {
		t.Fatal("CLI localization hydration accepted a corrupt warning sidecar")
	}
}

func TestPresentationLocalizationCacheMissThenHit(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	response := presentationLocalizationProjectionJSON(t, prepared)
	cacheRoot := filepath.Join(t.TempDir(), "cache")

	firstProvider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	first, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		cacheRoot,
		false,
		data,
		prepared,
		firstProvider,
	)
	if err != nil {
		t.Fatalf("first executePresentationLocalization() error = %v", err)
	}
	if first.State != report.PresentationLocalizationSucceeded ||
		first.CacheHit ||
		firstProvider.executeCalls != 1 {
		t.Fatalf("first outcome/provider calls = %#v/%d", first, firstProvider.executeCalls)
	}

	secondRun := t.TempDir()
	secondProvider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		nil,
	)
	secondProvider.executeErr = errors.New("cache hit must not call provider")
	second, err := executePresentationLocalization(
		context.Background(),
		secondRun,
		cacheRoot,
		false,
		data,
		prepared,
		secondProvider,
	)
	if err != nil {
		t.Fatalf("second executePresentationLocalization() error = %v", err)
	}
	if second.State != report.PresentationLocalizationSucceeded ||
		!second.CacheHit ||
		secondProvider.executeCalls != 0 {
		t.Fatalf("second outcome/provider calls = %#v/%d", second, secondProvider.executeCalls)
	}
	projected, status := report.LoadPresentationLocalization(
		secondRun,
		data,
		localization.LocaleRussian,
	)
	if status.State != report.PresentationLocalizationSucceeded ||
		!status.CacheHit ||
		projected.ReportLanguage != localization.LocaleRussian ||
		projected.ProjectGuess == data.ProjectGuess {
		t.Fatalf("saved cache-hit projection/status = %#v/%#v", projected, status)
	}
}

func TestPresentationLocalizationNoCacheBypassesReadAndWrite(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	response := presentationLocalizationProjectionJSON(t, prepared)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	seedProvider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	if _, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		cacheRoot,
		false,
		data,
		prepared,
		seedProvider,
	); err != nil {
		t.Fatalf("seed executePresentationLocalization() error = %v", err)
	}
	cacheDir := filepath.Join(cacheRoot, presentationLocalizationCacheVersionDir)
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("seed cache entries = %d, want 1", len(entries))
	}
	cachePath := filepath.Join(cacheDir, entries[0].Name())
	cacheBefore, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)

	for index := 0; index < 2; index++ {
		outcome, err := executePresentationLocalization(
			context.Background(),
			t.TempDir(),
			cacheRoot,
			true,
			data,
			prepared,
			provider,
		)
		if err != nil {
			t.Fatalf("executePresentationLocalization(%d) error = %v", index, err)
		}
		if outcome.State != report.PresentationLocalizationSucceeded ||
			outcome.CacheHit {
			t.Fatalf("no-cache outcome %d = %#v", index, outcome)
		}
	}
	if provider.executeCalls != 2 {
		t.Fatalf("provider execute calls = %d, want 2", provider.executeCalls)
	}
	cacheAfter, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(cacheAfter) != string(cacheBefore) {
		t.Fatal("--no-cache rewrote the persistent cache")
	}
}

func TestPresentationLocalizationCacheIdentityChanges(t *testing.T) {
	t.Parallel()

	_, prepared := presentationLocalizationFixture(t)
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		nil,
	)
	base := presentationLocalizationFirstBatchPlan(t, prepared, provider).Identity
	baseKey, _, err := presentationLocalizationCacheKey(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*presentationLocalizationCacheIdentity)
	}{
		{
			name: "endpoint",
			mutate: func(identity *presentationLocalizationCacheIdentity) {
				identity.Request.Endpoint = "https://other.example.test/v1/chat/completions"
			},
		},
		{
			name: "model",
			mutate: func(identity *presentationLocalizationCacheIdentity) {
				identity.Request.Model = "other-model"
			},
		},
		{
			name: "contract version",
			mutate: func(identity *presentationLocalizationCacheIdentity) {
				identity.ContractVersion = "future-contract"
			},
		},
		{
			name: "translation contract version",
			mutate: func(identity *presentationLocalizationCacheIdentity) {
				identity.TranslationContractVersion = "future-translation-contract"
			},
		},
		{
			name: "full request",
			mutate: func(identity *presentationLocalizationCacheIdentity) {
				identity.Request.Body = append(
					append([]byte(nil), identity.Request.Body...),
					' ',
				)
			},
		},
		{
			name: "locale",
			mutate: func(identity *presentationLocalizationCacheIdentity) {
				identity.TargetLocale = "uk"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity := base
			identity.Request.Body = append([]byte(nil), base.Request.Body...)
			test.mutate(&identity)
			key, _, err := presentationLocalizationCacheKey(identity)
			if err != nil {
				t.Fatalf("presentationLocalizationCacheKey() error = %v", err)
			}
			if key == baseKey {
				t.Fatalf("cache key did not change for %s", test.name)
			}
		})
	}
}

func TestPresentationLocalizationSelectionReasonDoesNotEnterPromptOrCacheKey(t *testing.T) {
	t.Parallel()

	prepare := func(reason string) report.PreparedPresentationLocalization {
		data := &report.ReportData{
			FormatVersion: report.CurrentFormatVersion,
			RepoName:      "fixture",
			ProjectGuess:  "An English repository guide",
			ModelResearch: &modelresearch.State{
				Rounds: []modelresearch.ResearchRound{{
					ID:              "round-cache-identity",
					SelectionReason: reason,
				}},
			},
		}
		prepared, err := report.PreparePresentationLocalization(
			data,
			localization.LocaleRussian,
		)
		if err != nil {
			t.Fatal(err)
		}
		return prepared
	}
	first := prepare("new_exact_evidence_and_high_value_frontier")
	second := prepare("runtime_only_frontier")
	if first.Canonical.SHA256 != second.Canonical.SHA256 {
		t.Fatal("opaque selection_reason changed the presentation inventory hash")
	}

	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		nil,
	)
	requestAndKey := func(
		prepared report.PreparedPresentationLocalization,
	) (deepseek.LocalizationRequestEvidence, string) {
		plan := presentationLocalizationFirstBatchPlan(t, prepared, provider)
		return plan.Request, plan.Key
	}
	firstRequest, firstKey := requestAndKey(first)
	secondRequest, secondKey := requestAndKey(second)
	if !bytes.Equal(firstRequest.Body, secondRequest.Body) {
		t.Fatal("opaque selection_reason changed the exact localization prompt request")
	}
	if firstKey != secondKey {
		t.Fatal("opaque selection_reason changed the localization cache key")
	}
}

func TestPresentationLocalizationCorruptCacheRecomputes(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	response := presentationLocalizationProjectionJSON(t, prepared)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	firstProvider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	if _, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		cacheRoot,
		false,
		data,
		prepared,
		firstProvider,
	); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(cacheRoot, presentationLocalizationCacheVersionDir)
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(entries))
	}
	if err := os.WriteFile(
		filepath.Join(cacheDir, entries[0].Name()),
		[]byte("{corrupt"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	secondProvider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	outcome, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		cacheRoot,
		false,
		data,
		prepared,
		secondProvider,
	)
	if err != nil {
		t.Fatalf("executePresentationLocalization() error = %v", err)
	}
	if outcome.State != report.PresentationLocalizationSucceeded ||
		outcome.CacheHit ||
		!outcome.CacheCorrupt ||
		secondProvider.executeCalls != 1 {
		t.Fatalf("corrupt-cache outcome/provider calls = %#v/%d", outcome, secondProvider.executeCalls)
	}
}

func TestPresentationLocalizationCacheReadErrorRetainsValidRunProjection(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	response := presentationLocalizationProjectionJSON(t, prepared)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	cachePath := presentationLocalizationCachePath(
		t,
		cacheRoot,
		prepared,
		provider,
	)
	if err := os.MkdirAll(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	outcome, err := executePresentationLocalization(
		context.Background(),
		runDir,
		cacheRoot,
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatalf("executePresentationLocalization() error = %v", err)
	}
	if outcome.State != report.PresentationLocalizationSucceeded ||
		outcome.ReasonCode != "" ||
		!outcome.CacheCorrupt ||
		!outcome.CacheWriteErr ||
		provider.executeCalls != 1 {
		t.Fatalf("cache-read failure outcome/provider calls = %#v/%d", outcome, provider.executeCalls)
	}
	presentation, status := report.LoadPresentationLocalization(
		runDir,
		data,
		localization.LocaleRussian,
	)
	if status.State != report.PresentationLocalizationSucceeded ||
		presentation.ReportLanguage != localization.LocaleRussian ||
		presentation.ProjectGuess == data.ProjectGuess {
		t.Fatalf("cache-read failure projection/status = %#v/%#v", presentation, status)
	}
}

func TestPresentationLocalizationCacheWriteErrorRetainsValidRunProjection(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	response := presentationLocalizationProjectionJSON(t, prepared)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		t.TempDir(),
		filepath.Join(cacheRoot, presentationLocalizationCacheVersionDir),
	); err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	outcome, err := executePresentationLocalization(
		context.Background(),
		runDir,
		cacheRoot,
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatalf("executePresentationLocalization() error = %v", err)
	}
	if outcome.State != report.PresentationLocalizationSucceeded ||
		outcome.ReasonCode != "" ||
		outcome.CacheCorrupt ||
		!outcome.CacheWriteErr ||
		provider.executeCalls != 1 {
		t.Fatalf("cache-write failure outcome/provider calls = %#v/%d", outcome, provider.executeCalls)
	}
	presentation, status := report.LoadPresentationLocalization(
		runDir,
		data,
		localization.LocaleRussian,
	)
	if status.State != report.PresentationLocalizationSucceeded ||
		presentation.ReportLanguage != localization.LocaleRussian ||
		presentation.ProjectGuess == data.ProjectGuess {
		t.Fatalf("cache-write failure projection/status = %#v/%#v", presentation, status)
	}
}

func TestPresentationLocalizationCacheHitDoesNotRequireAPIKey(t *testing.T) {
	data, prepared := presentationLocalizationFixture(t)
	response := presentationLocalizationProjectionJSON(t, prepared)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	seedProvider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	seedProvider.client.Auth = "bearer"
	if _, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		cacheRoot,
		false,
		data,
		prepared,
		seedProvider,
	); err != nil {
		t.Fatalf("seed executePresentationLocalization() error = %v", err)
	}

	runDir := t.TempDir()
	if err := report.WriteReportJSON(
		data,
		filepath.Join(runDir, "report.json"),
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REPOMAP_LLM_ENDPOINT", seedProvider.client.Endpoint)
	t.Setenv("REPOMAP_LLM_MODEL", seedProvider.client.Model)
	t.Setenv("REPOMAP_LLM_AUTH", "bearer")
	t.Setenv("REPOMAP_LLM_API_KEY", "")
	t.Setenv("REPOMAP_LLM_MAX_TOKENS", "2048")

	outcome, err := localizePresentationForRun(
		context.Background(),
		runDir,
		cacheRoot,
		false,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("localizePresentationForRun() error = %v", err)
	}
	if outcome.State != report.PresentationLocalizationSucceeded ||
		!outcome.CacheHit {
		t.Fatalf("cache-hit outcome without API key = %#v", outcome)
	}
}

func TestPresentationLocalizationRejectsSecretBearingProviderOutput(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	projection := presentationLocalizationProjection(t, prepared, "Перевод: ")
	for fieldID := range projection.Translations {
		projection.Translations[fieldID] = "api_key=secret-value-123456"
		break
	}
	response := localizationProviderResponseJSON(
		t,
		prepared.Input,
		projection,
	)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	runDir := t.TempDir()
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	outcome, err := executePresentationLocalization(
		context.Background(),
		runDir,
		cacheRoot,
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatalf("executePresentationLocalization() error = %v", err)
	}
	if outcome.State != report.PresentationLocalizationFailed ||
		outcome.ReasonCode != report.LocalizationFailureInvalidProjection ||
		provider.executeCalls != 1 {
		t.Fatalf("secret-bearing provider outcome/calls = %#v/%d", outcome, provider.executeCalls)
	}
	projectionMatches, err := filepath.Glob(filepath.Join(
		runDir,
		"presentation_localization_projection.v1.*.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(projectionMatches) != 0 {
		t.Fatalf("secret-bearing projection artifacts = %v, want none", projectionMatches)
	}
	if _, err := os.Stat(
		filepath.Join(cacheRoot, presentationLocalizationCacheVersionDir),
	); !os.IsNotExist(err) {
		t.Fatalf("secret-bearing cache stat error = %v, want not exist", err)
	}
}

func TestPresentationLocalizationCacheRecordIsImmutable(t *testing.T) {
	t.Parallel()

	_, prepared := presentationLocalizationFixture(t)
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		nil,
	)
	plan := presentationLocalizationFirstBatchPlan(t, prepared, provider)
	identity := plan.Identity
	key := plan.Key
	firstProjection := localizationProjectionFor(
		plan.Batch.Canonical,
		plan.Batch.Input,
		"Первый перевод: ",
	)
	secondProjection := localizationProjectionFor(
		plan.Batch.Canonical,
		plan.Batch.Input,
		"Второй перевод: ",
	)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	first := presentationLocalizationCacheRecord{
		Version:    presentationLocalizationCacheVersion,
		Key:        key,
		Identity:   identity,
		Projection: firstProjection,
	}
	if err := writePresentationLocalizationCache(cacheRoot, key, first); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Projection = secondProjection
	if err := writePresentationLocalizationCache(cacheRoot, key, second); err == nil {
		t.Fatal("immutable cache entry was overwritten by a different projection")
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("immutable cache bytes changed after conflicting publication")
	}
}

func TestPresentationLocalizationCorruptCacheRemovalPreservesConcurrentReplacement(
	t *testing.T,
) {
	t.Parallel()

	_, prepared := presentationLocalizationFixture(t)
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		nil,
	)
	plan := presentationLocalizationFirstBatchPlan(t, prepared, provider)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	cacheDir := filepath.Join(cacheRoot, presentationLocalizationCacheVersionDir)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheDir, plan.Key+".json")
	if err := os.WriteFile(cachePath, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, found, corrupt, observed := loadPresentationLocalizationCacheObserved(
		cacheRoot,
		plan.Key,
		plan.IdentityJSON,
	)
	if found || !corrupt || observed.info == nil || len(observed.data) == 0 {
		t.Fatalf(
			"corrupt observation = found %t corrupt %t observation %#v",
			found,
			corrupt,
			observed,
		)
	}

	winner := presentationLocalizationCacheRecord{
		Version:  presentationLocalizationCacheVersion,
		Key:      plan.Key,
		Identity: plan.Identity,
		Projection: localizationProjectionFor(
			plan.Batch.Canonical,
			plan.Batch.Input,
			"Победивший перевод: ",
		),
	}
	winnerJSON, err := json.Marshal(winner)
	if err != nil {
		t.Fatal(err)
	}
	winnerJSON = append(winnerJSON, '\n')
	replacement, err := os.CreateTemp(cacheDir, ".winner-")
	if err != nil {
		t.Fatal(err)
	}
	replacementPath := replacement.Name()
	if _, err := replacement.Write(winnerJSON); err != nil {
		_ = replacement.Close()
		t.Fatal(err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacementPath, cachePath); err != nil {
		t.Fatal(err)
	}

	removed, err := removeCorruptPresentationLocalizationCache(
		cacheRoot,
		plan.Key,
		observed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("corrupt-cache cleanup removed a concurrent replacement")
	}
	got, gotFound, gotCorrupt := loadPresentationLocalizationCache(
		cacheRoot,
		plan.Key,
		plan.IdentityJSON,
	)
	if !gotFound || gotCorrupt ||
		!presentationLocalizationBatchCacheProjectionValid(plan.Batch, got) {
		t.Fatalf(
			"concurrent valid replacement = found %t corrupt %t record %#v",
			gotFound,
			gotCorrupt,
			got,
		)
	}
}

func TestPresentationLocalizationProviderFailurePreservesCanonicalRun(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	runDir := t.TempDir()
	reportPath := filepath.Join(runDir, "report.json")
	if err := report.WriteReportJSON(data, reportPath); err != nil {
		t.Fatal(err)
	}
	canonicalBefore, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		nil,
	)
	provider.executeErr = errors.New("synthetic network failure")
	outcome, err := executePresentationLocalization(
		context.Background(),
		runDir,
		filepath.Join(t.TempDir(), "cache"),
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatalf("executePresentationLocalization() error = %v", err)
	}
	if outcome.State != report.PresentationLocalizationFailed ||
		outcome.ReasonCode != report.LocalizationFailureProviderRequest ||
		provider.executeCalls != 1 {
		t.Fatalf("failure outcome/provider calls = %#v/%d", outcome, provider.executeCalls)
	}
	canonicalAfter, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonicalAfter) != string(canonicalBefore) {
		t.Fatalf("canonical report.json changed after provider failure")
	}
	presentation, status := report.LoadPresentationLocalization(
		runDir,
		data,
		localization.LocaleRussian,
	)
	if status.State != report.PresentationLocalizationFailed ||
		status.ReasonCode != report.LocalizationFailureProviderRequest ||
		presentation.ReportLanguage != localization.LocaleRussian ||
		presentation.ProjectGuess != data.ProjectGuess {
		t.Fatalf("failure presentation/status = %#v/%#v", presentation, status)
	}
}

func TestPresentationLocalizationRejectsStrictlyInvalidBatchAfterOneProviderRequest(
	t *testing.T,
) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	invalidProjection := localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: prepared.Canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    make(map[string]string, len(prepared.Input.Fields)),
	}
	for _, field := range prepared.Input.Fields {
		invalidProjection.Translations[field.ID] = field.Text
	}
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		localizationProviderResponseJSON(t, prepared.Input, invalidProjection),
	)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	outcome, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		cacheRoot,
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != report.PresentationLocalizationFailed ||
		outcome.ReasonCode != report.LocalizationFailureInvalidProjection ||
		provider.buildCalls != 1 || provider.executeCalls != 1 ||
		outcome.ProviderCalls != 1 {
		t.Fatalf(
			"strict-invalid outcome/provider calls = %#v/%d",
			outcome,
			provider.executeCalls,
		)
	}
	if _, err := os.Stat(filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
	)); !os.IsNotExist(err) {
		t.Fatalf("strict-invalid batch populated cache: %v", err)
	}
}

func TestPresentationLocalizationAdoptsConcurrentValidCacheWinner(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	liveProjection := presentationLocalizationProjection(t, prepared, "Живой ответ: ")
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		localizationProviderResponseJSON(t, prepared.Input, liveProjection),
	)
	plan := presentationLocalizationFirstBatchPlan(t, prepared, provider)
	winnerProjection := localizationProjectionFor(
		plan.Batch.Canonical,
		plan.Batch.Input,
		"Победивший кэш: ",
	)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	provider.executeHook = func() {
		record := presentationLocalizationCacheRecord{
			Version: presentationLocalizationCacheVersion,
			Key:     plan.Key, Identity: plan.Identity,
			Projection: winnerProjection,
		}
		if err := writePresentationLocalizationCache(
			cacheRoot,
			plan.Key,
			record,
		); err != nil {
			t.Errorf("publish concurrent winner: %v", err)
		}
	}
	runDir := t.TempDir()
	outcome, err := executePresentationLocalization(
		context.Background(),
		runDir,
		cacheRoot,
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != report.PresentationLocalizationSucceeded {
		t.Fatalf("winner outcome = %#v", outcome)
	}
	expected, _, err := report.ApplyPresentationLocalization(
		data,
		prepared,
		winnerProjection,
	)
	if err != nil {
		t.Fatal(err)
	}
	actual, status := report.LoadPresentationLocalization(
		runDir,
		data,
		localization.LocaleRussian,
	)
	if status.State != report.PresentationLocalizationSucceeded ||
		actual.ProjectGuess != expected.ProjectGuess {
		t.Fatalf(
			"published projection/status = %q/%#v, want concurrent winner %q",
			actual.ProjectGuess,
			status,
			expected.ProjectGuess,
		)
	}
}

func TestPresentationLocalizationRejectsSecretBearingCacheHit(t *testing.T) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	response := presentationLocalizationProjectionJSON(t, prepared)
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		response,
	)
	plan := presentationLocalizationFirstBatchPlan(t, prepared, provider)
	identity := plan.Identity
	key := plan.Key
	identityJSON := plan.IdentityJSON
	unsafeProjection := localizationProjectionFor(
		plan.Batch.Canonical,
		plan.Batch.Input,
		"Перевод: ",
	)
	for fieldID := range unsafeProjection.Translations {
		unsafeProjection.Translations[fieldID] =
			"api_key=secret-value-123456"
		break
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	if err := writePresentationLocalizationCache(
		cacheRoot,
		key,
		presentationLocalizationCacheRecord{
			Version:    presentationLocalizationCacheVersion,
			Key:        key,
			Identity:   identity,
			Projection: unsafeProjection,
		},
	); err != nil {
		t.Fatal(err)
	}
	loaded, found, corrupt := loadPresentationLocalizationCache(
		cacheRoot,
		key,
		identityJSON,
	)
	if !found || corrupt ||
		presentationLocalizationBatchCacheProjectionValid(plan.Batch, loaded) {
		t.Fatalf(
			"unsafe cache setup/validation = found %t corrupt %t record %#v",
			found,
			corrupt,
			loaded,
		)
	}

	outcome, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		cacheRoot,
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != report.PresentationLocalizationSucceeded ||
		outcome.CacheHit ||
		!outcome.CacheCorrupt ||
		provider.executeCalls != 1 {
		t.Fatalf(
			"unsafe cache outcome/provider calls = %#v/%d",
			outcome,
			provider.executeCalls,
		)
	}
}

type fakePresentationLocalizationProvider struct {
	client       *deepseek.Client
	response     []byte
	responses    [][]byte
	executeErr   error
	buildCalls   int
	executeCalls int
	prompts      []localization.Prompt
	executeHook  func()
}

func newFakePresentationLocalizationProvider(
	endpoint,
	model string,
	response []byte,
) *fakePresentationLocalizationProvider {
	return &fakePresentationLocalizationProvider{
		client: &deepseek.Client{
			Endpoint:  endpoint,
			Auth:      "none",
			Model:     model,
			MaxTokens: 2048,
		},
		response: append([]byte(nil), response...),
	}
}

func (provider *fakePresentationLocalizationProvider) BuildLocalizationRequest(
	prompt localization.Prompt,
) (deepseek.LocalizationRequestEvidence, error) {
	provider.buildCalls++
	return provider.client.BuildLocalizationRequest(prompt)
}

func (provider *fakePresentationLocalizationProvider) ExecuteLocalizationRequest(
	_ context.Context,
	prompt localization.Prompt,
	_ deepseek.LocalizationRequestEvidence,
) (modelresearch.ProviderResult, error) {
	provider.executeCalls++
	provider.prompts = append(provider.prompts, prompt)
	if provider.executeHook != nil {
		provider.executeHook()
	}
	if provider.executeErr != nil {
		return modelresearch.ProviderResult{Attempts: 1}, provider.executeErr
	}
	response := provider.response
	if len(provider.responses) != 0 {
		responseIndex := provider.executeCalls - 1
		if responseIndex >= len(provider.responses) {
			responseIndex = len(provider.responses) - 1
		}
		response = provider.responses[responseIndex]
	}
	return modelresearch.ProviderResult{
		Content:      append([]byte(nil), response...),
		Attempts:     1,
		InputTokens:  100,
		OutputTokens: 20,
	}, nil
}

func presentationLocalizationFixture(
	t *testing.T,
) (*report.ReportData, report.PreparedPresentationLocalization) {
	t.Helper()
	data := &report.ReportData{
		FormatVersion: report.CurrentFormatVersion,
		RepoName:      "fixture",
		ProjectGuess:  "An English repository guide",
	}
	prepared, err := report.PreparePresentationLocalization(
		data,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatalf("PreparePresentationLocalization() error = %v", err)
	}
	if len(prepared.Input.Fields) == 0 {
		t.Fatal("fixture produced no presentation fields")
	}
	return data, prepared
}

func presentationLocalizationProjectionJSON(
	t *testing.T,
	prepared report.PreparedPresentationLocalization,
) []byte {
	t.Helper()
	projection := presentationLocalizationProjection(t, prepared, "Перевод: ")
	return localizationProviderResponseJSON(t, prepared.Input, projection)
}

func presentationLocalizationProjection(
	t *testing.T,
	prepared report.PreparedPresentationLocalization,
	prefix string,
) localization.Projection {
	t.Helper()
	return localizationProjectionFor(
		prepared.Canonical,
		prepared.Input,
		prefix,
	)
}

func localizationProjectionFor(
	canonical localization.CanonicalArtifact,
	input localization.Input,
	prefix string,
) localization.Projection {
	translations := make(map[string]string, len(input.Fields))
	for _, field := range input.Fields {
		translated := prefix + "Русское описание"
		for _, placeholder := range field.Placeholders {
			for count := 0; count < placeholder.Count; count++ {
				translated += " " + placeholder.Token
			}
		}
		translations[field.ID] = translated
	}
	return localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    translations,
	}
}

func localizationProviderResponseJSON(
	t *testing.T,
	input localization.Input,
	projection localization.Projection,
) []byte {
	t.Helper()
	translations := make([]localization.ProviderTranslation, len(input.Fields))
	for index, field := range input.Fields {
		translations[index] = localization.NewProviderTranslation(
			index,
			projection.Translations[field.ID],
		)
	}
	encoded, err := json.Marshal(localization.ProviderResponse{
		Version:         localization.ProviderResponseVersion,
		CanonicalSHA256: projection.CanonicalSHA256,
		Locale:          projection.Locale,
		Translations:    translations,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func presentationLocalizationFirstBatchPlan(
	t *testing.T,
	prepared report.PreparedPresentationLocalization,
	provider presentationLocalizationProvider,
) presentationLocalizationBatchPlan {
	t.Helper()
	plans, err := buildPresentationLocalizationBatchPlans(prepared, provider)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) == 0 {
		t.Fatal("localization fixture produced no batch plans")
	}
	return plans[0]
}

func presentationLocalizationCachePath(
	t *testing.T,
	cacheRoot string,
	prepared report.PreparedPresentationLocalization,
	provider presentationLocalizationProvider,
) string {
	t.Helper()
	key := presentationLocalizationFirstBatchPlan(t, prepared, provider).Key
	return filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
}

func writePresentationWarningSidecarRun(t *testing.T) (string, string) {
	t.Helper()
	const rawWarning = "local confidence gate capped candidate_flows[0] from 0.90 to 0.30"
	runDir := t.TempDir()
	orientationJSON := []byte(`{
  "project_guess":"Fixture repository",
  "confidence":0.3,
  "candidate_flows":[{
    "name":"Fixture direction",
    "confidence":0.3,
    "local_verification":{"status":"partial","confidence_cap":0.3}
  }],
  "warnings":["` + rawWarning + `"]
}`)
	if err := os.WriteFile(
		filepath.Join(runDir, "orientation_report.json"),
		orientationJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, "snapshot.json"),
		[]byte("{}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	sidecarJSON, err := orient.EncodeConfidenceWarningDiagnostics(
		orientationJSON,
		[]orient.ConfidenceWarningDiagnostic{{
			WarningIndex:   0,
			Code:           orient.ConfidenceWarningCandidateCapped,
			CandidateIndex: 0,
			Proposed:       0.9,
			Capped:         0.3,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
		sidecarJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	data.FormatVersion = report.CurrentFormatVersion
	if err := report.WriteReportJSON(data, filepath.Join(runDir, "report.json")); err != nil {
		t.Fatal(err)
	}
	return runDir, rawWarning
}
