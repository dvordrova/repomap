package surfacediscovery_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestArchitectureGroundingArtifactAcceptsFilteredDeclarationCoverage(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	writeArchitectureArtifactFixture(t, filepath.Join(repository, "go.mod"), `module example.com/certificate

go 1.24
`)
	writeArchitectureArtifactFixture(t, filepath.Join(repository, "certificate.go"), `package certificate

var initialized = initialize()

func initialize() bool { return true }

func Decode() {}
`)

	result, err := surfacediscovery.Analyze(surfacediscovery.DefaultOptions(repository))
	if err != nil {
		t.Fatal(err)
	}
	coverage := result.Grounding.Coverage
	if !coverage.Complete || coverage.DeclarationFamilyMembersConsidered == 0 ||
		coverage.DeclarationFamilyMembersConsidered != coverage.DeclarationFamilyMembersPublished {
		t.Fatalf("producer declaration-family coverage = %#v", coverage)
	}

	runDir := t.TempDir()
	if err := surfacediscovery.WriteArtifacts(runDir, result); err != nil {
		t.Fatal(err)
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureGrounding == nil ||
		data.ArchitectureGrounding.Coverage.DeclarationFamilyMembersConsidered != coverage.DeclarationFamilyMembersConsidered ||
		data.ArchitectureGrounding.Coverage.DeclarationFamilyMembersPublished != coverage.DeclarationFamilyMembersPublished {
		t.Fatalf("validated architecture grounding = %#v", data.ArchitectureGrounding)
	}
	for _, warning := range data.Warnings {
		if strings.Contains(warning, "architecture grounding:") {
			t.Fatalf("producer artifact failed report validation: %s", warning)
		}
	}
}

func writeArchitectureArtifactFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
