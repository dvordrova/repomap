package clientrecipe

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreviewProjectionKeepsStructuralAuthorityLocal(t *testing.T) {
	h1 := goldenH1(t)
	h2, err := BuildH2(t.Context(), h1, &recordingH2Provider{})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := goldenEvaluation(t)
	model, err := BuildPreviewModel(h1, h2, evaluation)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Steps) != 6 || len(model.Examples) != 4 || len(model.Audit) != 6 {
		t.Fatalf("preview projection = %d steps / %d examples / %d exclusions", len(model.Steps), len(model.Examples), len(model.Audit))
	}
	if model.Summary.Observed != 10 || model.Summary.Boundaries != 4 || model.Summary.Complete != 3 || model.Summary.Excluded != 6 {
		t.Fatalf("preview accounting = %#v", model.Summary)
	}
	if model.Summary.ObservedUniversalRoles != 8 || model.Summary.ObservedCommonRoles != 1 {
		t.Fatalf("preview role reduction = %#v", model.Summary)
	}
	mostComplete := []string{}
	for _, example := range model.Examples {
		if example.MostComplete {
			mostComplete = append(mostComplete, example.Name)
		}
	}
	if !reflect.DeepEqual(mostComplete, []string{"Kubernetes", "Vault"}) {
		t.Fatalf("most complete examples = %v", mostComplete)
	}
	if !reflect.DeepEqual(model.Examples[:3], completePreviewExamples(model.Examples[:3])) {
		t.Fatal("the three initially visible examples are not all complete")
	}
	notifier := previewExampleByName(t, model, "Notifier")
	if notifier.Complete || notifier.MostComplete || !reflect.DeepEqual(notifier.Missing, []string{"Verification", "Observability", "Failure policy"}) {
		t.Fatalf("Notifier projection = %#v", notifier)
	}
	clickHouse := previewExampleByName(t, model, "ClickHouse")
	clickHouseFailure := previewSlot(clickHouse, "s6")
	if clickHouseFailure.Status != "partial" || len(clickHouseFailure.Evidence) == 0 ||
		!reflect.DeepEqual(clickHouseFailure.Missing, []string{"Failure policy"}) {
		t.Fatalf("ClickHouse failure slot = %#v", clickHouseFailure)
	}
	notifierFailure := previewSlot(notifier, "s6")
	if notifierFailure.Status != "missing" || len(notifierFailure.Evidence) != 0 ||
		!reflect.DeepEqual(notifierFailure.Missing, []string{"Observability", "Failure policy"}) {
		t.Fatalf("Notifier failure slot = %#v", notifierFailure)
	}
	if len(model.Tasks) != 1 || !model.Tasks[0].Available {
		t.Fatalf("preview tasks = %#v", model.Tasks)
	}
	if model.Scope.Evidence != "Controlled fixture only" || model.Scope.Generalization != "Generalization not established" {
		t.Fatalf("preview scope = %#v", model.Scope)
	}
	roleSet := make(map[string]int)
	for _, step := range model.Steps {
		for _, role := range step.Roles {
			roleSet[role.ID]++
		}
	}
	if len(roleSet) != 9 {
		t.Fatalf("preview role cover = %#v", roleSet)
	}
	for role, count := range roleSet {
		if count != 1 {
			t.Fatalf("role %s appears in %d preview groups", role, count)
		}
	}
	verification := previewRoleByID(t, model, string(H1RoleVerification))
	if !verification.TaskRequired || verification.ObservedComplete != 3 || verification.CompleteExamples != 3 ||
		verification.ObservedNecessity != "Observed in all" {
		t.Fatalf("verification role projection = %#v", verification)
	}
	failurePolicy := previewRoleByID(t, model, string(H1RoleFailurePolicy))
	if failurePolicy.TaskRequired || failurePolicy.ObservedComplete != 2 || failurePolicy.CompleteExamples != 3 ||
		failurePolicy.ObservedNecessity != "Common pattern" {
		t.Fatalf("failure policy role projection = %#v", failurePolicy)
	}
}

func TestPreviewIsOneDeterministicOfflineHTML(t *testing.T) {
	h1 := goldenH1(t)
	h2, err := BuildH2(t.Context(), h1, &recordingH2Provider{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := RenderClientRecipePreview(h1, h2, goldenEvaluation(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderClientRecipePreview(h1, h2, goldenEvaluation(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, second) {
		t.Fatal("preview bytes changed for identical sealed inputs")
	}
	for _, forbidden := range []string{
		`<link`, `src="http`, `src='http`, `href="http`, `href='http`, `fetch(`,
		`XMLHttpRequest`, `WebSocket`, `@import`, `program-object-`, `program-relation-`, `h1-instance-`,
		`Recommended to copy`, `Six evidence-backed steps`, `9 / 9`, `coverage-bar`,
	} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("standalone preview contains forbidden dependency or raw authority %q", forbidden)
		}
	}
	mainStart := bytes.Index(raw, []byte(`<main id="app" tabindex="-1">`))
	mainEnd := bytes.Index(raw, []byte(`</main>`))
	if mainStart < 0 || mainEnd < mainStart || len(bytes.TrimSpace(raw[mainStart+len(`<main id="app" tabindex="-1">`):mainEnd])) != 0 {
		t.Fatal("recipe or evidence was materialized in the initial HTML DOM")
	}
	if !bytes.Contains(raw, []byte(`../repo/internal/clients/kubernetes/config.go#L10`)) {
		t.Fatal("preview lost its exact validated source action")
	}
	assertPreviewGolden(t, raw)
}

func TestPreviewRejectsTamperedBindingsAndEscapesCopy(t *testing.T) {
	h1 := goldenH1(t)
	h2, err := BuildH2(t.Context(), h1, &recordingH2Provider{})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := goldenEvaluation(t)

	tamperedEvaluation := evaluation
	tamperedEvaluation.H1SHA256 = strings.Repeat("a", 64)
	tamperedEvaluation.SHA256 = evaluationDigest(tamperedEvaluation)
	if _, err := BuildPreviewModel(h1, h2, tamperedEvaluation); err == nil {
		t.Fatal("preview accepted an evaluation for another H1 result")
	}
	tamperedEvaluation = evaluation
	tamperedEvaluation.Verdict = EvaluationFail
	tamperedEvaluation.SHA256 = evaluationDigest(tamperedEvaluation)
	if _, err := BuildPreviewModel(h1, h2, tamperedEvaluation); err == nil {
		t.Fatal("preview accepted a failed evaluation")
	}

	copyOnly := h2
	copyOnly.Steps = append([]H2StepCopy(nil), h2.Steps...)
	copyOnly.Steps[0].Title = `Shape "client" configuration & defaults`
	copyOnly.SHA256 = h2Digest(copyOnly)
	raw, err := RenderClientRecipePreview(h1, copyOnly, evaluation)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`Shape "client" configuration & defaults`)) || !bytes.Contains(raw, []byte(`Shape \"client\" configuration \u0026 defaults`)) {
		t.Fatal("H2 visible copy was not safely encoded into the inline model")
	}
}

func goldenEvaluation(t *testing.T) EvaluationResult {
	t.Helper()
	value, err := DecodeEvaluation(readExperimentFile(t, filepath.Join(experimentRoot(t), "golden", "04-evaluation.json")))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func previewExampleByName(t *testing.T, model PreviewModel, name string) PreviewExample {
	t.Helper()
	for _, example := range model.Examples {
		if example.Name == name {
			return example
		}
	}
	t.Fatalf("preview has no %s example", name)
	return PreviewExample{}
}

func previewRoleByID(t *testing.T, model PreviewModel, id string) PreviewRole {
	t.Helper()
	for _, role := range model.Roles {
		if role.ID == id {
			return role
		}
	}
	t.Fatalf("preview has no %s role", id)
	return PreviewRole{}
}

func completePreviewExamples(values []PreviewExample) []PreviewExample {
	for _, value := range values {
		if !value.Complete {
			return nil
		}
	}
	return values
}

func assertPreviewGolden(t *testing.T, actual []byte) {
	t.Helper()
	filename := filepath.Join(experimentRoot(t), "preview", "report.html")
	if os.Getenv("REPOMAP_UPDATE_EXPERIMENT_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read preview golden: %v; update with REPOMAP_UPDATE_EXPERIMENT_GOLDEN=1", err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatal("preview golden changed; inspect before REPOMAP_UPDATE_EXPERIMENT_GOLDEN=1")
	}
}
