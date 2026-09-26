package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// Redis's socket connect was shown as "HTTP": the label came from the kind's
// protocol. No outgoing kind names a protocol now, and no label claims one.
func TestOutboundKindLabelsClaimNoProtocol(t *testing.T) {
	for _, kind := range atlas.OutgoingBoundaryKinds() {
		if label := outboundKindLabel(kind); strings.Contains(strings.ToLower(label), "http") {
			t.Fatalf("outgoing kind %q is labelled %q", kind, label)
		}
	}
	if label := outboundKindLabel(atlas.BoundaryClientRequest); label != "Request" {
		t.Fatalf("a request sent to another service is labelled %q, want Request", label)
	}
}
