package main

import (
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
)

func TestPresentationLocalizationOversizePayloadDegradesWithoutProviderCall(
	t *testing.T,
) {
	t.Parallel()

	data, prepared := presentationLocalizationFixture(t)
	prepared.Input.Fields[0].Text = strings.Repeat("x", 1<<20)
	provider := newFakePresentationLocalizationProvider(
		"https://translation.example.test/v1/chat/completions",
		"translation-model",
		nil,
	)

	outcome, err := executePresentationLocalization(
		context.Background(),
		t.TempDir(),
		t.TempDir(),
		false,
		data,
		prepared,
		provider,
	)
	if err != nil {
		t.Fatalf("executePresentationLocalization() error = %v", err)
	}
	if outcome.State != report.PresentationLocalizationFailed ||
		outcome.ReasonCode != report.LocalizationFailurePayloadTooLarge ||
		provider.executeCalls != 0 {
		t.Fatalf(
			"oversize outcome/provider calls = %#v/%d",
			outcome,
			provider.executeCalls,
		)
	}
}
