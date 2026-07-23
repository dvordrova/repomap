package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/pavedpath"
)

func TestDeriveOperationalResultsFuegoCoverageArtifact(t *testing.T) {
	t.Parallel()

	const evidenceID = "operational-evidence-fuego-cover"
	saved := pavedpath.Path{Actions: []pavedpath.Action{{
		EvidenceID: evidenceID,
		Command:    "make cover",
	}}}
	evidence := map[string]pavedpath.Evidence{
		evidenceID: {
			ID: evidenceID, Role: pavedpath.RoleBuildTarget, Path: "Makefile", StartLine: 10,
			Excerpt: []string{
				"cover:",
				"\tgo test -coverprofile=coverage.out ./...",
				"\tgo tool cover -func=coverage.out",
			},
		},
	}

	results, err := deriveOperationalResults(&ReportData{}, saved, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one generated artifact", results)
	}
	result := results[0]
	if result.Kind != OperationalResultGeneratedArtifact || result.Value != "coverage.out" ||
		result.AfterAction != 1 || len(result.ResultEvidenceIDs) != 1 ||
		result.ResultEvidenceIDs[0] != evidenceID {
		t.Fatalf("result = %#v", result)
	}
	if result.Reference.Location.Path != "Makefile" || result.Reference.Location.Line != 11 ||
		result.Reference.Source.StartLine != 11 || result.Reference.Source.EndLine != 11 {
		t.Fatalf("result reference = %#v", result.Reference)
	}
}

func TestDeriveOperationalResultsResticDocumentedTranscript(t *testing.T) {
	t.Parallel()

	const evidenceID = "operational-evidence-restic-update"
	saved := pavedpath.Path{Actions: []pavedpath.Action{
		{EvidenceID: evidenceID, Command: "restic version"},
		{EvidenceID: evidenceID, Command: "restic self-update"},
	}}
	evidence := map[string]pavedpath.Evidence{
		evidenceID: {
			ID: evidenceID, Role: pavedpath.RoleDocumentedProcedure,
			Path: "doc/020_installation.rst", StartLine: 231,
			Excerpt: []string{
				"    $ restic version",
				"    restic 0.9.3 compiled with go1.11.2 on linux/amd64",
				"",
				"    $ restic self-update",
				"    find latest release of restic at GitHub",
				"    latest version is 0.9.4",
				"    download file SHA256SUMS",
				"    GPG signature verification succeeded",
				"    downloaded restic_0.9.4_linux_amd64.bz2",
				"    saved 12115904 bytes in ./restic",
				"    successfully updated restic to version 0.9.4",
				"",
				"    $ restic version",
			},
		},
	}

	results, err := deriveOperationalResults(&ReportData{}, saved, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v, want two documented outputs", results)
	}
	if results[0].Kind != OperationalResultCommandOutput || results[0].AfterAction != 1 ||
		results[0].Value != "restic 0.9.3 compiled with go1.11.2 on linux/amd64" ||
		results[0].Reference.Location.Line != 232 {
		t.Fatalf("version result = %#v", results[0])
	}
	if results[1].Kind != OperationalResultCommandOutput || results[1].AfterAction != 2 ||
		!strings.HasPrefix(results[1].Value, "find latest release of restic at GitHub\n") ||
		!strings.HasSuffix(results[1].Value, "successfully updated restic to version 0.9.4") ||
		results[1].Reference.Location.Line != 235 {
		t.Fatalf("self-update result = %#v", results[1])
	}
}

func TestDeriveOperationalResultsRequiresExactCompletionEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		action   pavedpath.Action
		evidence pavedpath.Evidence
		want     string
	}{
		{
			name:   "repomap build has an exact output path",
			action: pavedpath.Action{EvidenceID: "build", Command: "go build -o ./repomap ./cmd/repomap"},
			evidence: pavedpath.Evidence{
				ID: "build", Role: pavedpath.RoleDocumentedProcedure, Path: "README.md", StartLine: 20,
				Excerpt: []string{"go build -o ./repomap ./cmd/repomap", "./repomap --help"},
			},
			want: "./repomap",
		},
		{
			name:   "repomap explore has no completion evidence",
			action: pavedpath.Action{EvidenceID: "explore", Command: "./repomap ../etcd"},
			evidence: pavedpath.Evidence{
				ID: "explore", Role: pavedpath.RoleDocumentedProcedure, Path: "README.md", StartLine: 40,
				Excerpt: []string{"./repomap ../etcd", "Open the generated report in your browser."},
			},
		},
		{
			name:   "unprompted prose is not command output",
			action: pavedpath.Action{EvidenceID: "prose", Command: "tool run"},
			evidence: pavedpath.Evidence{
				ID: "prose", Role: pavedpath.RoleDocumentedProcedure, Path: "README.md", StartLine: 60,
				Excerpt: []string{"tool run", "Everything is ready."},
			},
		},
		{
			name:   "redacted evidence cannot publish a result",
			action: pavedpath.Action{EvidenceID: "redacted", Command: "tool run -o result.txt"},
			evidence: pavedpath.Evidence{
				ID: "redacted", Role: pavedpath.RoleDocumentedProcedure, Path: "README.md", StartLine: 70,
				Excerpt: []string{"tool run -o result.txt"}, Redacted: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			results, err := deriveOperationalResults(
				&ReportData{},
				pavedpath.Path{Actions: []pavedpath.Action{test.action}},
				map[string]pavedpath.Evidence{test.evidence.ID: test.evidence},
			)
			if err != nil {
				t.Fatal(err)
			}
			if test.want == "" {
				if len(results) != 0 {
					t.Fatalf("results = %#v, want none", results)
				}
				return
			}
			if len(results) != 1 || results[0].Kind != OperationalResultGeneratedArtifact ||
				results[0].Value != test.want {
				t.Fatalf("results = %#v, want artifact %q", results, test.want)
			}
		})
	}
}
