package themestudy

// Decision 224 provider-free acceptance probe (D219): replay the saved
// Archive 5 raw Scout and Adjudication responses through the rebuilt
// validators and assert no valid core theme is lost to prose length and no
// populated observation is mislabeled empty. Skipped when the saved run
// directory is absent (CI without the owner's cache).
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const savedCasdoorRun = "20260805-124752-casdoor-08791e639777"
const savedEtcdRun = "20260805-123011-etcd-b16ef92438b9"

func savedRunDir() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "Caches", "repomap", "runs", savedCasdoorRun)
}

func TestD224SavedCasdoorRawResponsesKeepValidThemes(t *testing.T) {
	runDir := savedRunDir()
	if info, err := os.Stat(runDir); err != nil || !info.IsDir() {
		t.Skipf("saved run %s unavailable (no owner cache): %v", savedCasdoorRun, err)
	}
	exchanges := filepath.Join(runDir, "semantic_exchanges")
	entries, err := os.ReadDir(exchanges)
	if err != nil {
		t.Fatalf("read exchanges: %v", err)
	}
	var scoutRaw, adjRaw []byte
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		exchangeRaw, err := os.ReadFile(filepath.Join(exchanges, entry.Name(), "exchange.v1.json"))
		if err != nil {
			continue
		}
		var exchange struct {
			Stage string `json:"stage"`
		}
		if err := json.Unmarshal(exchangeRaw, &exchange); err != nil {
			continue
		}
		if exchange.Stage != "atlas_study" {
			continue
		}
		response, err := os.ReadFile(filepath.Join(exchanges, entry.Name(), "response.json"))
		if err != nil {
			continue
		}
		var themes struct {
			Themes []map[string]json.RawMessage `json:"themes"`
		}
		if err := json.Unmarshal(response, &themes); err != nil || len(themes.Themes) == 0 {
			continue
		}
		if _, isScout := themes.Themes[0]["anchor_refs"]; isScout {
			scoutRaw = response
		} else if _, isAdj := themes.Themes[0]["anchor_assessments"]; isAdj {
			adjRaw = response
		}
	}
	if scoutRaw == nil || adjRaw == nil {
		t.Fatalf("saved raw scout/adj responses not found in %s", exchanges)
	}

	// Advertised ref sets come from the saved Scout request artifact.
	scoutRequest, err := readSavedScoutRequest(t, runDir)
	if err != nil {
		t.Fatalf("read saved scout request: %v", err)
	}
	candidates, scoutStatus, err := ValidateScout(
		scoutRaw, scoutRequest.AnchorRefs(), scoutRequest.FileRefs(), scoutRequest.CatalogSHA256,
	)
	if err != nil {
		t.Fatalf("ValidateScout on saved casdoor raw: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("saved casdoor scout lost ALL candidates")
	}
	// The saved run rejected 1 candidate (invalid_anchor_count); prose length
	// must no longer reject anything. 12 raw -> >= 11 accepted.
	if scoutStatus.Accepted < 11 {
		t.Fatalf("saved casdoor scout accepted = %d, want >= 11 (no valid theme lost to prose)", scoutStatus.Accepted)
	}
	t.Logf("saved casdoor scout: accepted %d/%d, normalized %v", scoutStatus.Accepted, scoutStatus.Received, scoutStatus.Normalized)

	// Adjudication: every assessment observed in the raw response is
	// populated (250-496 runes) and must survive normalization, not be
	// rejected as empty. Candidate refs are assigned exactly as the
	// production scout stage does before the Adjudication request compiles.
	AssignCandidateRefs(candidates)
	candidateByRef := map[string]*ScoutCandidate{}
	for index := range candidates {
		candidateByRef[candidates[index].Ref] = &candidates[index]
	}
	adjThemes, adjStatus, err := ValidateAdjudication(adjRaw, candidateByRef)
	if err != nil {
		t.Fatalf("ValidateAdjudication on saved casdoor raw: %v", err)
	}
	if len(adjThemes) == 0 {
		t.Fatalf("saved casdoor adjudication lost ALL themes")
	}
	if adjStatus.Accepted < 9 {
		t.Fatalf("saved casdoor adj accepted = %d, want >= 9 (populated observations must not be dropped)", adjStatus.Accepted)
	}
	for index, theme := range adjThemes {
		for _, assessment := range theme.AnchorAssessments {
			if strings.TrimSpace(assessment.SupportedObservation) == "" {
				t.Fatalf("theme %d published an empty observation", index)
			}
		}
	}
	t.Logf("saved casdoor adj: accepted %d/%d, normalized %v", adjStatus.Accepted, adjStatus.Received, adjStatus.Normalized)
}

func readSavedScoutRequest(t *testing.T, runDir string) (ScoutRequest, error) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runDir, "theme_scout_request.v1.json"))
	if err != nil {
		return ScoutRequest{}, err
	}
	return DecodeScoutRequest(raw)
}

// TestD224SavedEtcdRawAdjudicationKeepsPopulatedObservations covers the
// D219 #2 defect class on the saved etcd run: observations of 250-496 runes
// were rejected as empty_observation (3 themes in Archive 6). The rebuilt
// validator must accept them with normalization.
func TestD224SavedEtcdRawAdjudicationKeepsPopulatedObservations(t *testing.T) {
	runDir := filepath.Join(os.Getenv("HOME"), "Library", "Caches", "repomap", "runs", savedEtcdRun)
	if info, err := os.Stat(runDir); err != nil || !info.IsDir() {
		t.Skipf("saved etcd run unavailable (no owner cache): %v", err)
	}
	exchanges := filepath.Join(runDir, "semantic_exchanges")
	entries, err := os.ReadDir(exchanges)
	if err != nil {
		t.Fatalf("read exchanges: %v", err)
	}
	var adjRaw []byte
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		exchangeRaw, err := os.ReadFile(filepath.Join(exchanges, entry.Name(), "exchange.v1.json"))
		if err != nil {
			continue
		}
		var exchange struct {
			Stage string `json:"stage"`
		}
		if err := json.Unmarshal(exchangeRaw, &exchange); err != nil {
			continue
		}
		if exchange.Stage != "atlas_study" {
			continue
		}
		response, err := os.ReadFile(filepath.Join(exchanges, entry.Name(), "response.json"))
		if err != nil {
			continue
		}
		var themes struct {
			Themes []map[string]json.RawMessage `json:"themes"`
		}
		if err := json.Unmarshal(response, &themes); err != nil || len(themes.Themes) == 0 {
			continue
		}
		if _, isAdj := themes.Themes[0]["anchor_assessments"]; isAdj {
			adjRaw = response
		}
	}
	if adjRaw == nil {
		t.Fatalf("saved etcd raw adj response not found in %s", exchanges)
	}
	// Candidate refs: the saved adj themes reference t*; rebuild the
	// candidate set from the saved Adjudication request artifact (the
	// saved scout result predates the v2 decoder and fails closed).
	adjRequestRaw, err := os.ReadFile(filepath.Join(runDir, "theme_adjudication_request.v1.json"))
	if err != nil {
		t.Fatalf("read saved adjudication request: %v", err)
	}
	adjRequest, err := DecodeAdjudicationRequest(adjRequestRaw)
	if err != nil {
		t.Fatalf("decode saved adjudication request: %v", err)
	}
	candidateByRef := map[string]*ScoutCandidate{}
	for index := range adjRequest.Candidates {
		candidateByRef[adjRequest.Candidates[index].Ref] = &adjRequest.Candidates[index]
	}
	adjThemes, adjStatus, err := ValidateAdjudication(adjRaw, candidateByRef)
	if err != nil {
		t.Fatalf("ValidateAdjudication on saved etcd raw: %v", err)
	}
	if len(adjThemes) == 0 {
		t.Fatalf("saved etcd adjudication lost ALL themes")
	}
	// Archive 6 rejected 3 as empty_observation; the rebuilt validator must
	// keep them: 12 raw -> >= 11 accepted.
	if adjStatus.Accepted < 11 {
		t.Fatalf("saved etcd adj accepted = %d, want >= 11 (populated observations dropped)", adjStatus.Accepted)
	}
	for _, issue := range adjStatus.Issues {
		if issue.Code == AdjIssueEmptyObservation {
			t.Fatalf("saved etcd still rejects populated observations as empty: %v", adjStatus.Issues)
		}
	}
	t.Logf("saved etcd adj: accepted %d/%d, normalized %v", adjStatus.Accepted, adjStatus.Received, adjStatus.Normalized)
}
