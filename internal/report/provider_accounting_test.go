package report

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestAtlasFirstProviderAccountingDoesNotDoubleCountCompletedMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		metadataCalls int
		metadataBytes int
		complete      bool
		researchUsage *modelresearch.Usage
		architecture  *ArchitectureSynthesisStatus
		wantCalls     int
		wantBytes     int
	}{
		{
			name:          "atlas-first cold live totals stay three",
			metadataCalls: 3, metadataBytes: 300, complete: true,
			researchUsage: &modelresearch.Usage{SemanticCalls: 1, RequestBytes: 100},
			architecture: &ArchitectureSynthesisStatus{
				ProviderRequestCount: 1,
			},
			wantCalls: 3, wantBytes: 300,
		},
		{
			name:          "atlas-first cached totals stay zero",
			metadataCalls: 0, metadataBytes: 0, complete: true,
			researchUsage: &modelresearch.Usage{SemanticCalls: 3, RequestBytes: 300},
			architecture: &ArchitectureSynthesisStatus{
				State: ArchitectureSynthesisCached, ProviderRequestCount: 0,
			},
			wantCalls: 0, wantBytes: 0,
		},
		{
			name:          "historical metadata still adds architecture",
			metadataCalls: 2, metadataBytes: 200,
			architecture: &ArchitectureSynthesisStatus{
				ProviderRequestCount: 1,
			},
			wantCalls: 3, wantBytes: 200,
		},
		{
			name:          "historical model research still replaces metadata totals",
			metadataCalls: 7, metadataBytes: 700,
			researchUsage: &modelresearch.Usage{SemanticCalls: 2, RequestBytes: 220},
			architecture: &ArchitectureSynthesisStatus{
				ProviderRequestCount: 1,
			},
			wantCalls: 2, wantBytes: 220,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDir := t.TempDir()
			metadata := fmt.Sprintf(
				`{"provider_request_count":%d,"external_request_bytes":%d,"provider_accounting_complete":%t}`,
				test.metadataCalls,
				test.metadataBytes,
				test.complete,
			)
			writeTestFile(t, runDir, "metadata.json", metadata)
			data := &ReportData{}
			if warning := parseRunMetadata(filepath.Join(runDir, "metadata.json"), data); warning != "" {
				t.Fatalf("parseRunMetadata: %s", warning)
			}
			if test.researchUsage != nil {
				data.ModelResearch = &modelresearch.State{Usage: *test.researchUsage}
			}
			data.ArchitectureSynthesis = test.architecture
			reconcileLegacyProviderAccounting(data)

			if data.Run == nil || data.Run.ProviderRequestCount != test.wantCalls ||
				data.Run.ExternalRequestBytes != test.wantBytes ||
				data.Run.ProviderAccountingComplete != test.complete {
				t.Fatalf("accounting = %#v, want calls=%d bytes=%d complete=%t", data.Run, test.wantCalls, test.wantBytes, test.complete)
			}
		})
	}
}
