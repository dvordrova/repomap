package djangoatomic_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	pinnedDjangoRevision  = "3e389b7ddaf08109900da5415ddaac5a355a170f"
	expectedEpisodeSHA256 = "9599553a777e8d8fd582bb1874dd4ab534c1f24d9d87e82cfce09cc775281665"
)

var (
	stableIDPattern        = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	hex40Pattern           = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hex64Pattern           = regexp.MustCompile(`^[0-9a-f]{64}$`)
	absolutePOSIXPattern   = regexp.MustCompile(`(?:^|[\s"'=(\[])\/[^/\s]`)
	absoluteWindowsPattern = regexp.MustCompile(`(?:^|[\s"'=(\[])[A-Za-z]:[\\/]`)
	claimSectionPattern    = regexp.MustCompile(`(?m)^### (.+) — \*\*([A-Z]+)\*\*\n<!-- episode-claim (\{[^\n]+\}) -->`)
	pinnedBlobLinkPattern  = regexp.MustCompile(`https://github\.com/django/django/blob/[0-9a-f]{40}/[^)\s]+`)
	htmlCommentPattern     = regexp.MustCompile(`(?s)<!--.*?-->`)
)

var expectedBlobs = map[string]string{
	"django/db/transaction.py":         "0c2eee8e736408af3838cdd55aed3c7ddff3bcde",
	"django/db/backends/base/base.py":  "aa9cdb5748793535b84e364b04cdd553e8ac379a",
	"tests/transaction_hooks/tests.py": "938e92575f0c796da36918a2b25a17d528db8a36",
}

type episode struct {
	ArtifactKind    string        `json:"artifact_kind"`
	ArtifactVersion string        `json:"artifact_version"`
	EpisodeID       string        `json:"episode_id"`
	Repository      repository    `json:"repository"`
	Question        string        `json:"question"`
	TrustStates     []string      `json:"trust_states"`
	Scope           scope         `json:"scope"`
	Anchors         []anchor      `json:"anchors"`
	Facts           []fact        `json:"facts"`
	Claims          []claim       `json:"claims"`
	BranchMap       []branch      `json:"branch_map"`
	Uncertainties   []uncertainty `json:"uncertainties"`
}

type repository struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	WebBase  string `json:"web_base"`
}

type scope struct {
	Included []string `json:"included"`
	Excluded []string `json:"excluded"`
}

type anchor struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	BlobOID    string `json:"blob_oid"`
	LineSHA256 string `json:"line_sha256"`
	Needle     string `json:"needle"`
	URL        string `json:"url"`
}

type fact struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
}

type claim struct {
	ID             string   `json:"id"`
	State          string   `json:"state"`
	Strength       string   `json:"strength"`
	Title          string   `json:"title"`
	Statement      string   `json:"statement"`
	SupportFactIDs []string `json:"support_fact_ids"`
	AnchorIDs      []string `json:"anchor_ids"`
	Limits         []string `json:"limits"`
}

type branch struct {
	ID                string   `json:"id"`
	State             string   `json:"state"`
	Trigger           string   `json:"trigger"`
	TransactionEffect string   `json:"transaction_effect"`
	CallbackEffect    string   `json:"callback_effect"`
	ClaimIDs          []string `json:"claim_ids"`
}

type uncertainty struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
	NextCheck string   `json:"next_check"`
}

type claimMarker struct {
	ID             string   `json:"id"`
	State          string   `json:"state"`
	AnchorIDs      []string `json:"anchor_ids"`
	SupportFactIDs []string `json:"support_fact_ids"`
}

func TestEpisodeIntegrity(t *testing.T) {
	rawJSON := readFixture(t, "episode.json")
	rawMarkdown := readFixture(t, "how-it-works.md")

	if got := sha256.Sum256(rawJSON); hex.EncodeToString(got[:]) != expectedEpisodeSHA256 {
		t.Fatalf("episode.json SHA-256 = %x, want pinned %s", got, expectedEpisodeSHA256)
	}
	assertNoAbsolutePaths(t, "episode.json", rawJSON)
	assertNoAbsolutePaths(t, "how-it-works.md", rawMarkdown)

	var got episode
	decodeStrict(t, rawJSON, &got)
	if got.ArtifactKind != "source-episode-microexperiment" || got.ArtifactVersion != "1" {
		t.Fatalf("artifact identity = %q/%q", got.ArtifactKind, got.ArtifactVersion)
	}
	if got.Repository.Name != "django/django" || got.Repository.Revision != pinnedDjangoRevision {
		t.Fatalf("repository identity = %q@%q", got.Repository.Name, got.Repository.Revision)
	}
	wantWebBase := "https://github.com/django/django/tree/" + pinnedDjangoRevision
	if got.Repository.WebBase != wantWebBase {
		t.Fatalf("repository.web_base = %q, want %q", got.Repository.WebBase, wantWebBase)
	}

	states := make(map[string]struct{}, len(got.TrustStates))
	for _, state := range got.TrustStates {
		if !stableIDPattern.MatchString(state) {
			t.Errorf("invalid trust state %q", state)
		}
		states[state] = struct{}{}
	}
	for _, required := range []string{"extracted", "corroborated", "inferred", "unknown"} {
		if _, ok := states[required]; !ok {
			t.Errorf("missing trust state %q", required)
		}
	}

	allIDs := map[string]string{}
	registerID(t, allIDs, got.EpisodeID, "episode")
	anchors := make(map[string]anchor, len(got.Anchors))
	for i, item := range got.Anchors {
		where := fmt.Sprintf("anchors[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateAnchor(t, where, item)
		anchors[item.ID] = item
	}

	facts := make(map[string]fact, len(got.Facts))
	for i, item := range got.Facts {
		where := fmt.Sprintf("facts[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, states, where, item.State)
		validateRefs(t, anchors, where+".anchor_ids", item.AnchorIDs, 1, 3)
		facts[item.ID] = item
	}

	claims := make(map[string]claim, len(got.Claims))
	for i, item := range got.Claims {
		where := fmt.Sprintf("claims[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, states, where, item.State)
		if item.Strength != "strong" && item.Strength != "weak" {
			t.Errorf("%s.strength = %q", where, item.Strength)
		}
		if item.State == "inferred" && item.Strength != "weak" {
			t.Errorf("%s promotes inferred claim to %q strength", where, item.Strength)
		}
		validateRefs(t, anchors, where+".anchor_ids", item.AnchorIDs, 1, 3)
		validateRefs(t, facts, where+".support_fact_ids", item.SupportFactIDs, 1, 4)
		claims[item.ID] = item
	}

	for i, item := range got.BranchMap {
		where := fmt.Sprintf("branch_map[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, states, where, item.State)
		validateRefs(t, claims, where+".claim_ids", item.ClaimIDs, 1, 3)
	}
	for i, item := range got.Uncertainties {
		where := fmt.Sprintf("uncertainties[%d]", i)
		registerID(t, allIDs, item.ID, where)
		if item.State != "unknown" && item.State != "inferred" {
			t.Errorf("%s.state = %q, want weak state", where, item.State)
		}
		validateRefs(t, anchors, where+".anchor_ids", item.AnchorIDs, 1, 2)
	}

	t.Run("reader_contract", func(t *testing.T) {
		if err := validateMarkdown(rawMarkdown, claims, anchors); err != nil {
			t.Fatal(err)
		}
		visible := htmlCommentPattern.ReplaceAll(rawMarkdown, nil)
		for _, internalPrefix := range []string{"anchor-", "fact-"} {
			if bytes.Contains(visible, []byte(internalPrefix)) {
				t.Errorf("reader text exposes internal %q identifiers", internalPrefix)
			}
		}
		if !bytes.Contains(visible, []byte("post-commit handoff, not a delivery guarantee — **INFERRED**")) {
			t.Error("reader text hides the useful inferred signal")
		}
		if bytes.Count(visible, []byte("**UNKNOWN")) < 2 {
			t.Error("reader text does not keep both unknowns visible")
		}
	})

	t.Run("rejects_reader_state_promotion", func(t *testing.T) {
		promoted := replaceOnce(
			t,
			rawMarkdown,
			"post-commit handoff, not a delivery guarantee — **INFERRED**",
			"post-commit handoff, not a delivery guarantee — **CORROBORATED**",
		)
		if err := validateMarkdown(promoted, claims, anchors); err == nil || !strings.Contains(err.Error(), "state") {
			t.Fatalf("promotion error = %v, want state mismatch", err)
		}
	})

	t.Run("rejects_anchor_drift", func(t *testing.T) {
		drifted := replaceOnce(
			t,
			rawMarkdown,
			`"anchor_ids":["anchor-atomic-enter","anchor-atomic-clean-exit"]`,
			`"anchor_ids":["anchor-atomic-clean-exit","anchor-atomic-enter"]`,
		)
		if err := validateMarkdown(drifted, claims, anchors); err == nil || !strings.Contains(err.Error(), "anchor_ids") {
			t.Fatalf("anchor drift error = %v, want anchor_ids mismatch", err)
		}
	})

	t.Run("rejects_source_link_drift", func(t *testing.T) {
		drifted := replaceOnce(
			t,
			rawMarkdown,
			"[`Atomic.__enter__`: start the outer boundary or create a nested savepoint]("+anchors["anchor-atomic-enter"].URL+")",
			"[`Atomic.__enter__`: start the outer boundary or create a nested savepoint]("+anchors["anchor-atomic-clean-exit"].URL+")",
		)
		if err := validateMarkdown(drifted, claims, anchors); err == nil || !strings.Contains(err.Error(), "source URLs") {
			t.Fatalf("source drift error = %v, want source URLs mismatch", err)
		}
	})
}

func validateAnchor(t *testing.T, where string, item anchor) {
	t.Helper()
	if item.Path == "" || path.Clean(item.Path) != item.Path || path.IsAbs(item.Path) || filepath.IsAbs(item.Path) {
		t.Errorf("%s.path = %q is not canonical repository-relative", where, item.Path)
	}
	wantBlob, ok := expectedBlobs[item.Path]
	if !ok {
		t.Errorf("%s.path = %q is outside the pinned source boundary", where, item.Path)
	} else if item.BlobOID != wantBlob || !hex40Pattern.MatchString(item.BlobOID) {
		t.Errorf("%s.blob_oid = %q, want %q", where, item.BlobOID, wantBlob)
	}
	if item.StartLine < 1 || item.EndLine < item.StartLine || item.EndLine-item.StartLine > 80 {
		t.Errorf("%s lines = %d-%d are invalid or unbounded", where, item.StartLine, item.EndLine)
	}
	if !hex64Pattern.MatchString(item.LineSHA256) {
		t.Errorf("%s.line_sha256 = %q", where, item.LineSHA256)
	}
	if strings.TrimSpace(item.Needle) == "" || len(item.Needle) > 160 {
		t.Errorf("%s.needle is empty or unbounded", where)
	}
	wantURL := fmt.Sprintf(
		"https://github.com/django/django/blob/%s/%s#L%d-L%d",
		pinnedDjangoRevision,
		item.Path,
		item.StartLine,
		item.EndLine,
	)
	if item.URL != wantURL {
		t.Errorf("%s.url = %q, want %q", where, item.URL, wantURL)
	}
}

func validateMarkdown(raw []byte, claims map[string]claim, anchors map[string]anchor) error {
	matches := claimSectionPattern.FindAllSubmatchIndex(raw, -1)
	if len(matches) != len(claims) {
		return fmt.Errorf("reader claim section count = %d, want %d", len(matches), len(claims))
	}
	seen := make(map[string]struct{}, len(matches))
	for i, match := range matches {
		title := string(raw[match[2]:match[3]])
		readerState := strings.ToLower(string(raw[match[4]:match[5]]))
		var marker claimMarker
		decoder := json.NewDecoder(bytes.NewReader(raw[match[6]:match[7]]))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&marker); err != nil {
			return fmt.Errorf("decode marker below %q: %w", title, err)
		}
		item, ok := claims[marker.ID]
		if !ok {
			return fmt.Errorf("unknown reader claim %q", marker.ID)
		}
		if _, duplicate := seen[marker.ID]; duplicate {
			return fmt.Errorf("duplicate reader claim %q", marker.ID)
		}
		seen[marker.ID] = struct{}{}
		if title != item.Title {
			return fmt.Errorf("%s title = %q, want %q", item.ID, title, item.Title)
		}
		if readerState != item.State || marker.State != item.State {
			return fmt.Errorf("%s state = heading %q metadata %q, want %q", item.ID, readerState, marker.State, item.State)
		}
		if !equalStrings(marker.AnchorIDs, item.AnchorIDs) {
			return fmt.Errorf("%s anchor_ids mismatch", item.ID)
		}
		if !equalStrings(marker.SupportFactIDs, item.SupportFactIDs) {
			return fmt.Errorf("%s support_fact_ids mismatch", item.ID)
		}

		sectionEnd := len(raw)
		if i+1 < len(matches) {
			sectionEnd = matches[i+1][0]
		}
		gotURLs := uniqueSorted(pinnedBlobLinkPattern.FindAllString(string(raw[match[0]:sectionEnd]), -1))
		wantURLs := make([]string, 0, len(item.AnchorIDs))
		for _, anchorID := range item.AnchorIDs {
			wantURLs = append(wantURLs, anchors[anchorID].URL)
		}
		sort.Strings(wantURLs)
		if !equalStrings(gotURLs, wantURLs) {
			return fmt.Errorf("%s source URLs = %v, want %v", item.ID, gotURLs, wantURLs)
		}
	}
	return nil
}

func decodeStrict(t *testing.T, raw []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode episode.json: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("episode.json contains trailing data: %v", err)
	}
}

func validateState(t *testing.T, states map[string]struct{}, where, state string) {
	t.Helper()
	if _, ok := states[state]; !ok {
		t.Errorf("%s.state = %q is not declared", where, state)
	}
}

func validateRefs[T any](t *testing.T, known map[string]T, where string, refs []string, min, max int) {
	t.Helper()
	if len(refs) < min || len(refs) > max {
		t.Errorf("%s has %d references, want %d-%d", where, len(refs), min, max)
	}
	seen := map[string]struct{}{}
	for _, ref := range refs {
		if _, ok := known[ref]; !ok {
			t.Errorf("%s references unknown ID %q", where, ref)
		}
		if _, duplicate := seen[ref]; duplicate {
			t.Errorf("%s duplicates ID %q", where, ref)
		}
		seen[ref] = struct{}{}
	}
}

func registerID(t *testing.T, all map[string]string, id, where string) {
	t.Helper()
	if !stableIDPattern.MatchString(id) {
		t.Errorf("%s ID %q is not stable", where, id)
	}
	if previous, exists := all[id]; exists {
		t.Errorf("%s ID %q duplicates %s", where, id, previous)
	}
	all[id] = where
}

func assertNoAbsolutePaths(t *testing.T, name string, raw []byte) {
	t.Helper()
	if absolutePOSIXPattern.Match(raw) || absoluteWindowsPattern.Match(raw) {
		t.Errorf("%s contains an absolute local path", name)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func replaceOnce(t *testing.T, raw []byte, old, replacement string) []byte {
	t.Helper()
	if bytes.Count(raw, []byte(old)) != 1 {
		t.Fatalf("fixture contains %q %d times, want exactly once", old, bytes.Count(raw, []byte(old)))
	}
	return bytes.Replace(raw, []byte(old), []byte(replacement), 1)
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
