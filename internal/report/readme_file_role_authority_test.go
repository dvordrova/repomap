package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestDecodeReadmeFileRoleAuthorityRetainsArtifactBeyondFormerLimit(t *testing.T) {
	artifact := readmeFileRoleArtifact{
		Version: 1,
		Files: []readmeFileRoleArtifactRow{{
			FileRef: "f1", Path: "cmd/server/main.go",
			Classifications: []readmetargetscout.Classification{{
				Class:      readmetargetscout.ClassTargetEntry,
				Hypotheses: []string{strings.Repeat("x", readmetargetscout.AdvisoryArtifactBytes+1)},
			}},
		}},
	}
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= readmetargetscout.AdvisoryArtifactBytes {
		t.Fatalf("fixture = %d bytes", len(raw))
	}
	paths, err := decodeReadmeFileRoleAuthority(raw)
	if err != nil {
		t.Fatalf("decode complete artifact: %v", err)
	}
	if paths["f1"] != "cmd/server/main.go" {
		t.Fatalf("paths = %#v", paths)
	}
}
