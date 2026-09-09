package report

import (
	"reflect"
	"strings"
	"testing"
)

func TestTranslationMayRepeatAnExactSourcePlaceholder(t *testing.T) {
	// The etcd t3122 response repeated URLs to make its Russian sentence clear.
	entry := DisplayTextEntry{
		Ref: "t3122", Role: "explanation",
		Text:      "Confirms peer __REPOMAP_P1__ match those in the cluster peer list.",
		Protected: []DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "URLs"}},
		Terms:     []DisplayTextTerm{{ID: "url-definition", Spelling: "URLs", Explanation: "Resource addresses."}},
	}
	translated := "Подтверждает, что __REPOMAP_P1__ пира совпадают с __REPOMAP_P1__ пиров в списке кластера."
	if err := entry.ValidateTranslation(translated); err != nil {
		t.Fatal(err)
	}
	plain, spans, err := entry.finishDisplayText(translated)
	want := "Подтверждает, что URLs пира совпадают с URLs пиров в списке кластера."
	if err != nil || plain != want {
		t.Fatalf("restored text = %q, %v; want %q", plain, err, want)
	}
	wantSpans := []displayTermSpan{
		{Start: 18, End: 22, IDs: []string{"url-definition"}},
		{Start: 40, End: 44, IDs: []string{"url-definition"}},
	}
	if !reflect.DeepEqual(spans, wantSpans) {
		t.Fatalf("restored glossary spans = %+v, want %+v", spans, wantSpans)
	}
}

func TestRepeatedPlaceholderCannotReplaceAnotherRequiredSource(t *testing.T) {
	entry := DisplayTextEntry{
		Ref: "t1", Role: "explanation",
		Text: "Use __REPOMAP_P1__ with __REPOMAP_P2__.",
		Protected: []DisplayProtectedText{
			{Ref: "__REPOMAP_P1__", Text: "source.go"},
			{Ref: "__REPOMAP_P2__", Text: "destination.go"},
		},
	}
	if err := entry.ValidateTranslation("Использует __REPOMAP_P1__ с __REPOMAP_P1__."); err == nil {
		t.Fatal("repeating one source compensated for dropping another")
	}
}

func TestRepeatedPlaceholderDoesNotAuthorizeAnUnknownSource(t *testing.T) {
	entry := DisplayTextEntry{
		Ref: "t1", Role: "explanation", Text: "Use __REPOMAP_P1__.",
		Protected: []DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "source.go"}},
	}
	err := entry.ValidateTranslation("__REPOMAP_P1__ и __REPOMAP_P1__ используют __REPOMAP_P999__.")
	if err == nil || !strings.Contains(err.Error(), "introduced a source placeholder") {
		t.Fatalf("unknown source was not rejected independently of repetition: %v", err)
	}
}
