package localization

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestRussianPromptIsDeterministicAndContainsOnlyProtectedInput(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	first, err := BuildRussianPrompt(canonical, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildRussianPrompt(canonical, input)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := MarshalPrompt(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalPrompt(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("identical localization prompts differ:\n%s\n%s", firstJSON, secondJSON)
	}
	digest := sha256.Sum256(firstJSON)
	const wantPromptSHA256 = "e399c596ef763d1407d12d108b6168be07503f0d398eab181eedcb67e35a59a7"
	if got := hex.EncodeToString(digest[:]); got != wantPromptSHA256 {
		t.Fatalf("prompt SHA-256 = %q, want %q", got, wantPromptSHA256)
	}

	if first.Version != PromptVersion ||
		!strings.Contains(strings.ToLower(first.System), "valid json only") ||
		!strings.Contains(first.User, `"locale":"ru"`) {
		t.Fatalf("prompt contract = %#v", first)
	}
	for _, field := range input.Fields {
		if strings.Count(first.User, field.ID) != 1 {
			t.Fatalf("field ID %q does not occur exactly once in prompt", field.ID)
		}
	}
	const marker = "localization_input:\n"
	_, inputJSON, found := strings.Cut(first.User, marker)
	if !found {
		t.Fatal("prompt does not contain its localization input marker")
	}
	var decoded Input
	if err := json.Unmarshal([]byte(inputJSON), &decoded); err != nil {
		t.Fatalf("decode prompt input: %v", err)
	}
	decodedJSON, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decodedJSON, wantJSON) {
		t.Fatalf("embedded input changed:\n%s\n%s", decodedJSON, wantJSON)
	}
	for _, protected := range []string{
		"cmd/сервер main.go:42",
		"StartReplication",
		"PostgreSQL",
		"API/v2",
	} {
		if bytes.Contains(firstJSON, []byte(protected)) {
			t.Fatalf("prompt exposed protected term %q", protected)
		}
	}
}

func TestRussianPromptRejectsWrongDirectionAndStaleInput(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	english, err := BuildInput(canonical, LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRussianPrompt(canonical, english); err == nil {
		t.Fatal("English identity input unexpectedly became a Russian prompt")
	}

	russian, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	russian.Fields[0].Text += " drift"
	if _, err := BuildRussianPrompt(canonical, russian); err == nil {
		t.Fatal("stale localization input unexpectedly became a prompt")
	}
}

func TestRussianPromptBoundsBeforeProviderAdapter(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "oversize",
		Name:      FieldSummary,
		Text:      strings.Repeat("x", maxPromptBytes),
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRussianPrompt(canonical, input); err == nil ||
		!strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversize prompt error = %v", err)
	}

	input.Fields = make([]InputField, maxPromptFields+1)
	if _, err := BuildRussianPrompt(canonical, input); err == nil ||
		!strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversize prompt field count error = %v", err)
	}
}

func TestMarshalPromptRejectsInvalidEnvelopeAndUTF8(t *testing.T) {
	t.Parallel()

	for _, prompt := range []Prompt{
		{Version: "future", System: "json", User: "{}"},
		{Version: PromptVersion, System: "", User: "{}"},
		{Version: PromptVersion, System: "json", User: string([]byte{0xff})},
		{Version: PromptVersion, System: "json", User: strings.Repeat("x", maxPromptBytes)},
	} {
		if _, err := MarshalPrompt(prompt); err == nil {
			t.Fatalf("invalid prompt unexpectedly encoded: %#v", prompt)
		}
	}

	_, err := MarshalPrompt(Prompt{
		Version: PromptVersion,
		System:  strings.Repeat(" ", maxPromptBytes+1),
		User:    "{}",
	})
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversize whitespace prompt error = %v", err)
	}
}
