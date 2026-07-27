package etcdput_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const pinnedETCDRevision = "58f45a9ff1c083130830eb02b0cc7d9783609095"

var (
	stableIDPattern        = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	hexDigestPattern       = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	absolutePOSIXPattern   = regexp.MustCompile(`(?:^|[\s"'=(\[])\/[^/\s]`)
	absoluteWindowsPattern = regexp.MustCompile(`(?:^|[\s"'=(\[])[A-Za-z]:[\\/]`)
	readerClaimPattern     = regexp.MustCompile("^`([a-z][a-z0-9]*(?:-[a-z0-9]+)*)` — \\*\\*([A-Z]+)\\*\\*$")
	pinnedBlobLinkPattern  = regexp.MustCompile(`\]\((https://github\.com/etcd-io/etcd/blob/[0-9a-f]{40}/[^)\s]+)\)`)
)

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
	Boundaries      []boundary    `json:"boundaries"`
	Flow            flow          `json:"flow"`
	NonClaims       []nonClaim    `json:"non_claims"`
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
	URL        string `json:"url"`
	Needle     string `json:"needle"`
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

type boundary struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Label     string   `json:"label"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
	Limits    string   `json:"limits"`
}

type flow struct {
	ID                  string               `json:"id"`
	State               string               `json:"state"`
	Summary             string               `json:"summary"`
	Nodes               []flowNode           `json:"nodes"`
	Edges               []flowEdge           `json:"edges"`
	Frontiers           []frontier           `json:"frontiers"`
	NonOrderConstraints []nonOrderConstraint `json:"non_order_constraints"`
}

type flowNode struct {
	ID      string `json:"id"`
	ClaimID string `json:"claim_id"`
}

type flowEdge struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Relation       string   `json:"relation"`
	Ordering       string   `json:"ordering"`
	SupportFactIDs []string `json:"support_fact_ids"`
}

type frontier struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
}

type nonOrderConstraint struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
}

type nonClaim struct {
	ID        string   `json:"id"`
	Statement string   `json:"statement"`
	Reason    string   `json:"reason"`
	AnchorIDs []string `json:"anchor_ids"`
}

type uncertainty struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
	NextCheck string   `json:"next_check"`
}

type claimMetadata struct {
	ID             string
	State          string
	AnchorIDs      []string
	SupportFactIDs []string
}

type markdownClaimSection struct {
	ReaderID    string
	ReaderState string
	Metadata    claimMetadata
	MarkerLine  int
	SourceURLs  []string
}

func TestEpisodeIntegrity(t *testing.T) {
	rawJSON := readFixture(t, "episode.json")
	rawMarkdown := readFixture(t, "how-it-works.md")

	assertNoAbsolutePaths(t, "episode.json", rawJSON)
	assertNoAbsolutePaths(t, "how-it-works.md", rawMarkdown)

	var got episode
	decoder := json.NewDecoder(bytes.NewReader(rawJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode episode.json: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("episode.json contains trailing data: %v", err)
	}

	if got.ArtifactKind != "source-episode-microexperiment" {
		t.Fatalf("artifact_kind = %q, want %q", got.ArtifactKind, "source-episode-microexperiment")
	}
	if got.ArtifactVersion != "1" {
		t.Fatalf("artifact_version = %q, want %q", got.ArtifactVersion, "1")
	}
	if got.Repository.Name != "etcd-io/etcd" {
		t.Fatalf("repository.name = %q, want %q", got.Repository.Name, "etcd-io/etcd")
	}
	if got.Repository.Revision != pinnedETCDRevision {
		t.Fatalf("repository.revision = %q, want pinned revision %q", got.Repository.Revision, pinnedETCDRevision)
	}
	validateWebBase(t, got.Repository.WebBase)

	allIDs := make(map[string]string)
	registerID(t, allIDs, got.EpisodeID, "episode")
	if !bytes.Contains(rawMarkdown, []byte(got.EpisodeID)) {
		t.Errorf("how-it-works.md does not contain episode ID %q", got.EpisodeID)
	}

	trustStates := make(map[string]struct{}, len(got.TrustStates))
	for i, state := range got.TrustStates {
		if !stableIDPattern.MatchString(state) {
			t.Errorf("trust_states[%d] = %q is not a stable ID", i, state)
		}
		if _, exists := trustStates[state]; exists {
			t.Errorf("duplicate trust state %q", state)
		}
		trustStates[state] = struct{}{}
	}
	if len(trustStates) == 0 {
		t.Fatal("trust_states is empty")
	}

	anchors := make(map[string]anchor, len(got.Anchors))
	for i, item := range got.Anchors {
		where := fmt.Sprintf("anchors[%d]", i)
		registerID(t, allIDs, item.ID, where)
		if _, exists := anchors[item.ID]; exists {
			t.Errorf("%s duplicates anchor ID %q", where, item.ID)
		}
		anchors[item.ID] = item
		validateAnchor(t, where, got.Repository.Revision, item)
	}
	if len(anchors) == 0 {
		t.Fatal("anchors is empty")
	}

	for i, item := range got.Facts {
		where := fmt.Sprintf("facts[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, trustStates, where, item.State)
		validateAnchorReferences(t, anchors, where, item.AnchorIDs, true)
		if !bytes.Contains(rawMarkdown, []byte(item.ID)) {
			t.Errorf("how-it-works.md does not contain fact ID %q", item.ID)
		}
	}

	for i, item := range got.Claims {
		where := fmt.Sprintf("claims[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, trustStates, where, item.State)
		validateClaimStrength(t, where, item.Strength)
		if len(item.AnchorIDs) < 1 || len(item.AnchorIDs) > 3 {
			t.Errorf("%s has %d anchors, want 1–3", where, len(item.AnchorIDs))
		}
		validateAnchorReferences(t, anchors, where, item.AnchorIDs, true)
		if !bytes.Contains(rawMarkdown, []byte(item.ID)) {
			t.Errorf("how-it-works.md does not contain claim ID %q", item.ID)
		}
	}

	facts := make(map[string]struct{}, len(got.Facts))
	for _, item := range got.Facts {
		facts[item.ID] = struct{}{}
	}
	claims := make(map[string]struct{}, len(got.Claims))
	for i, item := range got.Claims {
		claims[item.ID] = struct{}{}
		validateFactReferences(t, facts, fmt.Sprintf("claims[%d]", i), item.SupportFactIDs, true)
	}

	t.Run("markdown_claim_contract", func(t *testing.T) {
		if err := validateMarkdownClaims(rawMarkdown, got.Claims, anchors); err != nil {
			t.Fatal(err)
		}

		t.Run("rejects_reader_state_promotion", func(t *testing.T) {
			promoted := replaceOnce(
				t,
				rawMarkdown,
				"`claim-wal-carries-replayable-request-bytes` — **INFERRED**",
				"`claim-wal-carries-replayable-request-bytes` — **CORROBORATED**",
			)
			err := validateMarkdownClaims(promoted, got.Claims, anchors)
			if err == nil || !strings.Contains(err.Error(), "reader state") {
				t.Fatalf("state promotion error = %v, want reader state mismatch", err)
			}
		})

		t.Run("rejects_metadata_anchor_drift", func(t *testing.T) {
			drifted := replaceOnce(
				t,
				rawMarkdown,
				`"anchor_ids":["anchor-rpc-put-return"`,
				`"anchor_ids":["anchor-put-submits-raft"`,
			)
			err := validateMarkdownClaims(drifted, got.Claims, anchors)
			if err == nil || !strings.Contains(err.Error(), "anchor_ids") {
				t.Fatalf("anchor drift error = %v, want anchor_ids mismatch", err)
			}
		})

		t.Run("rejects_source_url_drift", func(t *testing.T) {
			drifted := replaceOnce(
				t,
				rawMarkdown,
				anchors["anchor-rpc-put-return"].URL,
				anchors["anchor-put-submits-raft"].URL,
			)
			err := validateMarkdownClaims(drifted, got.Claims, anchors)
			if err == nil || !strings.Contains(err.Error(), "source URL set") {
				t.Fatalf("source URL drift error = %v, want source URL set mismatch", err)
			}
		})
	})

	for i, item := range got.Boundaries {
		where := fmt.Sprintf("boundaries[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, trustStates, where, item.State)
		validateAnchorReferences(t, anchors, where, item.AnchorIDs, true)
	}

	registerID(t, allIDs, got.Flow.ID, "flow")
	if got.Flow.State != "partial" {
		t.Errorf("flow.state = %q, want explicit %q", got.Flow.State, "partial")
	}
	nodes := make(map[string]struct{}, len(got.Flow.Nodes))
	for i, item := range got.Flow.Nodes {
		where := fmt.Sprintf("flow.nodes[%d]", i)
		registerID(t, allIDs, item.ID, where)
		nodes[item.ID] = struct{}{}
		if _, ok := claims[item.ClaimID]; !ok {
			t.Errorf("%s.claim_id references unknown claim %q", where, item.ClaimID)
		}
	}
	for i, item := range got.Flow.Edges {
		where := fmt.Sprintf("flow.edges[%d]", i)
		if _, ok := nodes[item.From]; !ok {
			t.Errorf("%s.from references unknown node %q", where, item.From)
		}
		if _, ok := nodes[item.To]; !ok {
			t.Errorf("%s.to references unknown node %q", where, item.To)
		}
		validateFactReferences(t, facts, where, item.SupportFactIDs, true)
	}
	for i, item := range got.Flow.Frontiers {
		where := fmt.Sprintf("flow.frontiers[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, trustStates, where, item.State)
		validateAnchorReferences(t, anchors, where, item.AnchorIDs, true)
	}
	for i, item := range got.Flow.NonOrderConstraints {
		where := fmt.Sprintf("flow.non_order_constraints[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, trustStates, where, item.State)
		validateAnchorReferences(t, anchors, where, item.AnchorIDs, true)
	}
	for i, item := range got.NonClaims {
		where := fmt.Sprintf("non_claims[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateAnchorReferences(t, anchors, where, item.AnchorIDs, true)
	}
	for i, item := range got.Uncertainties {
		where := fmt.Sprintf("uncertainties[%d]", i)
		registerID(t, allIDs, item.ID, where)
		validateState(t, trustStates, where, item.State)
		validateAnchorReferences(t, anchors, where, item.AnchorIDs, true)
	}

	t.Run("external_anchor_verification", func(t *testing.T) {
		verifyExternalAnchors(t, got.Repository.Revision, got.Anchors)
	})
}

func validateMarkdownClaims(markdown []byte, claims []claim, anchors map[string]anchor) error {
	sections, err := parseMarkdownClaimSections(markdown)
	if err != nil {
		return fmt.Errorf("parse Markdown claims: %w", err)
	}

	expected := make(map[string]claim, len(claims))
	for _, item := range claims {
		expected[item.ID] = item
	}
	seen := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		item, ok := expected[section.Metadata.ID]
		if !ok {
			return fmt.Errorf("Markdown claim metadata references unknown claim %q", section.Metadata.ID)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("Markdown contains duplicate metadata for claim %q", item.ID)
		}
		seen[item.ID] = struct{}{}

		if section.ReaderID != item.ID {
			return fmt.Errorf("claim %q metadata follows reader marker %q", item.ID, section.ReaderID)
		}
		if section.Metadata.State != item.State {
			return fmt.Errorf("claim %q metadata state = %q, want %q", item.ID, section.Metadata.State, item.State)
		}
		if section.ReaderState != strings.ToUpper(item.State) {
			return fmt.Errorf("claim %q reader state = %q, want %q", item.ID, section.ReaderState, strings.ToUpper(item.State))
		}
		if !slices.Equal(section.Metadata.AnchorIDs, item.AnchorIDs) {
			return fmt.Errorf("claim %q metadata anchor_ids = %q, want %q in the same order", item.ID, section.Metadata.AnchorIDs, item.AnchorIDs)
		}
		if !slices.Equal(section.Metadata.SupportFactIDs, item.SupportFactIDs) {
			return fmt.Errorf("claim %q metadata support_fact_ids = %q, want %q in the same order", item.ID, section.Metadata.SupportFactIDs, item.SupportFactIDs)
		}

		wantURLs := make([]string, 0, len(item.AnchorIDs))
		for _, anchorID := range item.AnchorIDs {
			anchorItem, ok := anchors[anchorID]
			if !ok {
				return fmt.Errorf("claim %q references unknown anchor %q", item.ID, anchorID)
			}
			wantURLs = append(wantURLs, anchorItem.URL)
		}
		if !equalStringSet(section.SourceURLs, wantURLs) {
			return fmt.Errorf("claim %q source URL set = %q, want exactly %q", item.ID, section.SourceURLs, wantURLs)
		}
	}
	for _, item := range claims {
		if _, ok := seen[item.ID]; !ok {
			return fmt.Errorf("Markdown is missing metadata for claim %q", item.ID)
		}
	}
	if len(sections) != len(claims) {
		return fmt.Errorf("Markdown contains %d claim metadata records, want %d", len(sections), len(claims))
	}
	return nil
}

func parseMarkdownClaimSections(markdown []byte) ([]markdownClaimSection, error) {
	lines := strings.Split(string(markdown), "\n")
	sections := make([]markdownClaimSection, 0)
	metadataLines := make(map[int]struct{})

	for lineIndex, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		match := readerClaimPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if lineIndex+1 >= len(lines) {
			return nil, fmt.Errorf("reader claim marker on line %d has no following metadata", lineIndex+1)
		}
		metadataLine := strings.TrimSuffix(lines[lineIndex+1], "\r")
		metadata, err := parseClaimMetadataComment(metadataLine)
		if err != nil {
			return nil, fmt.Errorf("claim marker %q on line %d: %w", match[1], lineIndex+1, err)
		}
		metadataLines[lineIndex+1] = struct{}{}
		sections = append(sections, markdownClaimSection{
			ReaderID:    match[1],
			ReaderState: match[2],
			Metadata:    metadata,
			MarkerLine:  lineIndex,
		})
	}

	for lineIndex, line := range lines {
		if !strings.Contains(line, "episode-claim") {
			continue
		}
		if _, paired := metadataLines[lineIndex]; !paired {
			return nil, fmt.Errorf("unpaired or malformed episode-claim metadata on line %d", lineIndex+1)
		}
	}

	for i := range sections {
		endLine := len(lines)
		if i+1 < len(sections) {
			endLine = sections[i+1].MarkerLine
		} else {
			for lineIndex := sections[i].MarkerLine + 2; lineIndex < len(lines); lineIndex++ {
				if strings.HasPrefix(strings.TrimSuffix(lines[lineIndex], "\r"), "## ") {
					endLine = lineIndex
					break
				}
			}
		}
		sectionText := strings.Join(lines[sections[i].MarkerLine:endLine], "\n")
		matches := pinnedBlobLinkPattern.FindAllStringSubmatch(sectionText, -1)
		sections[i].SourceURLs = make([]string, 0, len(matches))
		for _, match := range matches {
			sections[i].SourceURLs = append(sections[i].SourceURLs, match[1])
		}
	}
	return sections, nil
}

func parseClaimMetadataComment(line string) (claimMetadata, error) {
	const (
		prefix = "<!-- episode-claim "
		suffix = " -->"
	)
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return claimMetadata{}, fmt.Errorf("metadata must be one exact %q HTML comment", "episode-claim")
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix)
	return decodeClaimMetadata([]byte(payload))
}

func decodeClaimMetadata(payload []byte) (claimMetadata, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil {
		return claimMetadata{}, fmt.Errorf("decode metadata object: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return claimMetadata{}, fmt.Errorf("metadata must be a JSON object")
	}

	var result claimMetadata
	seen := make(map[string]struct{}, 4)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return claimMetadata{}, fmt.Errorf("decode metadata field: %w", err)
		}
		key, ok := token.(string)
		if !ok {
			return claimMetadata{}, fmt.Errorf("metadata field name has type %T", token)
		}
		if _, duplicate := seen[key]; duplicate {
			return claimMetadata{}, fmt.Errorf("metadata contains duplicate field %q", key)
		}
		seen[key] = struct{}{}

		switch key {
		case "id":
			err = decoder.Decode(&result.ID)
		case "state":
			err = decoder.Decode(&result.State)
		case "anchor_ids":
			err = decoder.Decode(&result.AnchorIDs)
		case "support_fact_ids":
			err = decoder.Decode(&result.SupportFactIDs)
		default:
			return claimMetadata{}, fmt.Errorf("metadata contains unknown field %q", key)
		}
		if err != nil {
			return claimMetadata{}, fmt.Errorf("decode metadata field %q: %w", key, err)
		}
	}
	token, err = decoder.Token()
	if err != nil {
		return claimMetadata{}, fmt.Errorf("close metadata object: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '}' {
		return claimMetadata{}, fmt.Errorf("metadata object has invalid closing token %v", token)
	}
	token, err = decoder.Token()
	if err == nil {
		return claimMetadata{}, fmt.Errorf("metadata contains trailing token %v", token)
	}
	if !errors.Is(err, io.EOF) {
		return claimMetadata{}, fmt.Errorf("decode trailing metadata: %w", err)
	}

	for _, field := range []string{"id", "state", "anchor_ids", "support_fact_ids"} {
		if _, ok := seen[field]; !ok {
			return claimMetadata{}, fmt.Errorf("metadata is missing required field %q", field)
		}
	}
	if result.ID == "" || result.State == "" || result.AnchorIDs == nil || result.SupportFactIDs == nil {
		return claimMetadata{}, fmt.Errorf("metadata contains an empty or null required field")
	}
	return result, nil
}

func equalStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	values := make(map[string]struct{}, len(want))
	for _, item := range want {
		values[item] = struct{}{}
	}
	if len(values) != len(want) {
		return false
	}
	for _, item := range got {
		if _, ok := values[item]; !ok {
			return false
		}
		delete(values, item)
	}
	return len(values) == 0
}

func replaceOnce(t *testing.T, content []byte, oldValue, newValue string) []byte {
	t.Helper()
	if count := bytes.Count(content, []byte(oldValue)); count != 1 {
		t.Fatalf("fixture contains %d copies of %q, want exactly one", count, oldValue)
	}
	return bytes.Replace(content, []byte(oldValue), []byte(newValue), 1)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return content
}

func registerID(t *testing.T, seen map[string]string, id, where string) {
	t.Helper()
	if !stableIDPattern.MatchString(id) {
		t.Errorf("%s ID %q is not stable kebab-case", where, id)
		return
	}
	if prior, exists := seen[id]; exists {
		t.Errorf("%s ID %q duplicates %s", where, id, prior)
		return
	}
	seen[id] = where
}

func validateState(t *testing.T, states map[string]struct{}, where, state string) {
	t.Helper()
	if _, ok := states[state]; !ok {
		t.Errorf("%s.state references unknown trust state %q", where, state)
	}
}

func validateClaimStrength(t *testing.T, where, strength string) {
	t.Helper()
	switch strength {
	case "strong", "moderate", "weak", "open":
	default:
		t.Errorf("%s.strength = %q, want strong, moderate, weak, or open", where, strength)
	}
}

func validateWebBase(t *testing.T, value string) {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("repository.web_base: %v", err)
	}
	wantPath := "/etcd-io/etcd/tree/" + pinnedETCDRevision
	if parsed.Scheme != "https" || parsed.Host != "github.com" || strings.TrimSuffix(parsed.Path, "/") != wantPath {
		t.Fatalf("repository.web_base = %q, want the pinned HTTPS etcd GitHub tree", value)
	}
}

func validateAnchor(t *testing.T, where, revision string, item anchor) {
	t.Helper()
	if err := safeRepositoryPath(item.Path); err != nil {
		t.Errorf("%s.path: %v", where, err)
	}
	if item.StartLine < 1 || item.EndLine < item.StartLine {
		t.Errorf("%s has invalid line range %d-%d", where, item.StartLine, item.EndLine)
	}
	if !hexDigestPattern.MatchString(item.BlobOID) {
		t.Errorf("%s.blob_oid = %q, want lowercase Git object ID", where, item.BlobOID)
	}
	if len(item.LineSHA256) != sha256.Size*2 {
		t.Errorf("%s.line_sha256 length = %d, want %d", where, len(item.LineSHA256), sha256.Size*2)
	} else if _, err := hex.DecodeString(item.LineSHA256); err != nil || item.LineSHA256 != strings.ToLower(item.LineSHA256) {
		t.Errorf("%s.line_sha256 = %q, want lowercase SHA-256", where, item.LineSHA256)
	}
	if strings.TrimSpace(item.Needle) == "" {
		t.Errorf("%s.needle is empty", where)
	}
	parsed, err := url.Parse(item.URL)
	if err != nil {
		t.Errorf("%s.url: %v", where, err)
		return
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		t.Errorf("%s.url = %q, want an HTTPS pinned-source URL", where, item.URL)
	}
	wantSuffix := "/blob/" + revision + "/" + item.Path
	if !strings.Contains(parsed.Path, wantSuffix) {
		t.Errorf("%s.url path = %q, want pinned suffix %q", where, parsed.Path, wantSuffix)
	}
}

func safeRepositoryPath(value string) error {
	switch {
	case value == "":
		return fmt.Errorf("path is empty")
	case strings.ContainsRune(value, '\x00'):
		return fmt.Errorf("path contains NUL")
	case strings.ContainsAny(value, `\:`):
		return fmt.Errorf("path contains a non-portable separator or colon")
	case path.IsAbs(value), filepath.IsAbs(value):
		return fmt.Errorf("path %q is absolute", value)
	case value == ".", value == "..", strings.HasPrefix(value, "../"):
		return fmt.Errorf("path %q escapes the repository", value)
	case path.Clean(value) != value:
		return fmt.Errorf("path %q is not canonical", value)
	default:
		return nil
	}
}

func validateAnchorReferences(t *testing.T, anchors map[string]anchor, where string, ids []string, require bool) {
	t.Helper()
	if require && len(ids) == 0 {
		t.Errorf("%s.anchor_ids is empty", where)
		return
	}
	seen := make(map[string]struct{}, len(ids))
	for i, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			t.Errorf("%s.anchor_ids[%d] duplicates %q", where, i, id)
			continue
		}
		seen[id] = struct{}{}
		if _, ok := anchors[id]; !ok {
			t.Errorf("%s.anchor_ids[%d] references unknown anchor %q", where, i, id)
		}
	}
}

func validateFactReferences(t *testing.T, facts map[string]struct{}, where string, ids []string, require bool) {
	t.Helper()
	if require && len(ids) == 0 {
		t.Errorf("%s.support_fact_ids is empty", where)
		return
	}
	seen := make(map[string]struct{}, len(ids))
	for i, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			t.Errorf("%s.support_fact_ids[%d] duplicates %q", where, i, id)
			continue
		}
		seen[id] = struct{}{}
		if _, ok := facts[id]; !ok {
			t.Errorf("%s.support_fact_ids[%d] references unknown fact %q", where, i, id)
		}
	}
}

func assertNoAbsolutePaths(t *testing.T, name string, content []byte) {
	t.Helper()
	text := string(content)
	if strings.Contains(text, "file://") ||
		strings.Contains(text, "~/") ||
		absolutePOSIXPattern.MatchString(text) ||
		absoluteWindowsPattern.MatchString(text) {
		t.Errorf("%s contains an absolute or home-relative filesystem path", name)
	}
}

func verifyExternalAnchors(t *testing.T, revision string, anchors []anchor) {
	t.Helper()
	repositoryPath := os.Getenv("ETCD_REPO")
	if repositoryPath == "" {
		repositoryPath = filepath.Clean("../../../../etcd")
	}
	if _, err := os.Stat(repositoryPath); err != nil {
		t.Skipf("external etcd checkout unavailable at %s: %v", repositoryPath, err)
	}

	head, err := gitOutput(repositoryPath, "rev-parse", "HEAD")
	if err != nil {
		t.Skipf("external etcd checkout cannot be read at %s: %v", repositoryPath, err)
	}
	if strings.TrimSpace(string(head)) != revision {
		t.Skipf("external etcd checkout HEAD is %s, pinned episode revision is %s", strings.TrimSpace(string(head)), revision)
	}

	for _, item := range anchors {
		t.Run(item.ID, func(t *testing.T) {
			oid, err := gitOutput(repositoryPath, "rev-parse", revision+":"+item.Path)
			if err != nil {
				t.Fatalf("resolve blob OID: %v", err)
			}
			if got := strings.TrimSpace(string(oid)); got != item.BlobOID {
				t.Errorf("blob OID = %q, want %q", got, item.BlobOID)
			}

			blob, err := gitOutput(repositoryPath, "show", revision+":"+item.Path)
			if err != nil {
				t.Fatalf("read pinned blob: %v", err)
			}
			selected, err := selectLines(blob, item.StartLine, item.EndLine)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(selected)
			if got := hex.EncodeToString(sum[:]); got != item.LineSHA256 {
				t.Errorf("line SHA-256 = %q, want %q", got, item.LineSHA256)
			}
			if !bytes.Contains(selected, []byte(item.Needle)) {
				t.Errorf("selected lines do not contain needle %q", item.Needle)
			}
		})
	}
}

func gitOutput(repositoryPath string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", repositoryPath}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	if err == nil {
		return output, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}

func selectLines(blob []byte, start, end int) ([]byte, error) {
	lines := bytes.Split(blob, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if start < 1 || end < start || end > len(lines) {
		return nil, fmt.Errorf("line range %d-%d is outside blob with %d lines", start, end, len(lines))
	}
	return bytes.Join(lines[start-1:end], []byte("\n")), nil
}
