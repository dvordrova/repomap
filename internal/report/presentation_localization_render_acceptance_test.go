package report

import (
	"bytes"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
)

func TestPresentationLocalizationRichRussianRenderContainsNoEnglishProseSentinel(
	t *testing.T,
) {
	t.Parallel()

	canonical := broadPresentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("complete projection result = %#v", result)
	}
	html, err := RenderHTML(projected)
	if err != nil {
		t.Fatal(err)
	}

	for class, sentinel := range map[string]string{
		"repository guide":       "Guide the reader through replication.",
		"repository thesis":      "Explain the durable replication boundary.",
		"first-file reason":      "Begin with the request coordinator.",
		"orientation subsystem":  "It owns the durable replication boundary.",
		"orientation direction":  "A client requests a replication update.",
		"flow":                   "The request is validated before the durable write.",
		"architecture":           "This area connects validation to persistence.",
		"guided tour":            "Follow the accepted replication story.",
		"guided tour trigger":    "A user asks how replication becomes durable.",
		"guided tour gap label":  "Remote acknowledgement",
		"guided tour gap detail": "The receiver outcome remains unresolved.",
		"study":                  "How does StartReplication update storage?",
		"mechanism":              "It validates the request and writes durable state.",
		"topic":                  "The exact storage effect is not established yet.",
		"operation":              "Observe the validated durable result.",
		"presentation warning":   "Evidence remains bounded.",
		"domain explanation":     "The durable PostgreSQL log position.",
		"human-facing question":  "Which replication mode is deployed in production?",
		"unverified path reason": "The model mentioned this path without local confirmation.",
	} {
		if bytes.Contains(html, []byte(sentinel)) {
			t.Errorf("%s English prose sentinel survived Russian render: %q", class, sentinel)
		}
	}

	for class, sentinel := range map[string]string{
		"path":      "internal/storage/replicate.go",
		"symbol":    "StartReplication",
		"object ID": "direction-replication",
		"command":   "go test ./internal/storage",
	} {
		if !bytes.Contains(html, []byte(sentinel)) {
			t.Errorf("%s opaque sentinel did not remain byte-identical: %q", class, sentinel)
		}
	}
}
