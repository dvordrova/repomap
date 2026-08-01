package reportserver

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

func writeOrientationSelectionFixture(t *testing.T, runDir string, canonicalBundle []byte) {
	t.Helper()
	typedWire := []byte(`{}`)
	caps := llmbundle.SelectionCaps{
		ReadmeBytes: 1024, Modules: 8, Entrypoints: 8, CandidateFiles: 8,
		Edges: 8, SourceSignalsTotal: 200, SourceSignalsPerFile: 5,
		KnownDocs: 30, CommandTraces: 8,
	}
	artifact, err := llmbundle.EncodeOrientationContextSelection(llmbundle.OrientationContextSelection{
		Version:               llmbundle.OrientationContextSelectionVersion,
		CanonicalBundleSHA256: fmt.Sprintf("%x", sha256.Sum256(canonicalBundle)),
		TypedWireSHA256:       fmt.Sprintf("%x", sha256.Sum256(typedWire)),
		CanonicalBundleBytes:  len(canonicalBundle),
		TypedWireBytes:        len(typedWire),
		ConfiguredCaps:        caps,
		EffectiveCaps:         caps,
		ByteFit: llmbundle.ByteFitTrace{
			Attempts: 1, Fit: true, InitialBytes: len(canonicalBundle), FittedBytes: len(canonicalBundle),
		},
		SelectedCandidates: []llmbundle.CandidateSelectionRow{},
		SourceSignalScan: sourcesignals.ScanTrace{
			MaxPerFile: 5,
			MaxTotal:   200,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, llmbundle.OrientationContextSelectionFilename),
		artifact,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}
