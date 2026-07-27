// Package composer projects the accepted etcd Put episode into one
// experiment-only where-to-change note. It cannot inspect source, Git, a
// provider, or a target repository.
package composer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxEpisodeBytes    = 256 << 10
	maxProjectionBytes = 16 << 10
	maxOutputBytes     = 64 << 10
	maxReferences      = 16

	episodeKind           = "source-episode-microexperiment"
	projectionKind        = "etcd-put-backend-ack-projection"
	artifactVersion       = "1"
	acceptedEpisodeSHA256 = "1f41085eea5fc0c59ddbb7ae66b7e3a67c82b8b588babd97edfe71ec873aa21a"
	projectionQuestion    = "Where would a backend-commit acknowledgment barrier be inserted, and what behavior would it change?"

	requiredCorrelation      = "committed_consistent_index >= applied_entry_index"
	requiredReleaseAnchor    = "anchor-apply-result-to-waiter"
	requiredPostCommitAnchor = "anchor-backend-bbolt-commit"
	requiredCoverageAnchor   = "anchor-backend-saves-consistent-index"
	requiredUnknownClaim     = "claim-ack-versus-wal-save-order-is-unknown"
)

type episodeInput struct {
	ArtifactKind    string          `json:"artifact_kind"`
	ArtifactVersion string          `json:"artifact_version"`
	EpisodeID       string          `json:"episode_id"`
	Repository      repositoryInput `json:"repository"`
	Anchors         []anchorInput   `json:"anchors"`
	Facts           []factInput     `json:"facts"`
	Claims          []claimInput    `json:"claims"`
	Flow            flowInput       `json:"flow"`
}

type repositoryInput struct {
	Revision string `json:"revision"`
	WebBase  string `json:"web_base"`
}

type anchorInput struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	URL       string `json:"url"`
}

type factInput struct {
	ID string `json:"id"`
}

type claimInput struct {
	ID             string   `json:"id"`
	State          string   `json:"state"`
	Title          string   `json:"title"`
	Statement      string   `json:"statement"`
	SupportFactIDs []string `json:"support_fact_ids"`
	AnchorIDs      []string `json:"anchor_ids"`
}

type flowInput struct {
	ID    string          `json:"id"`
	Nodes []flowNodeInput `json:"nodes"`
}

type flowNodeInput struct {
	ID      string `json:"id"`
	ClaimID string `json:"claim_id"`
}

type projectionInput struct {
	ArtifactKind        string                 `json:"artifact_kind"`
	ArtifactVersion     string                 `json:"artifact_version"`
	EpisodeID           string                 `json:"episode_id"`
	FlowID              string                 `json:"flow_id"`
	Question            string                 `json:"question"`
	CurrentBehavior     claimSelection         `json:"current_behavior"`
	BarrierDesign       barrierDesignSelection `json:"barrier_design"`
	BehaviorChangeState string                 `json:"behavior_change_state"`
	RemainingLimits     statedClaimSelection   `json:"remaining_limits"`
}

type claimSelection struct {
	ClaimIDs []string `json:"claim_ids"`
}

type statedClaimSelection struct {
	State    string   `json:"state"`
	ClaimIDs []string `json:"claim_ids"`
}

type barrierDesignSelection struct {
	State                string   `json:"state"`
	CorrelationPredicate string   `json:"correlation_predicate"`
	ReleaseAnchorID      string   `json:"release_anchor_id"`
	PostCommitAnchorID   string   `json:"post_commit_anchor_id"`
	CoverageAnchorID     string   `json:"coverage_anchor_id"`
	ClaimIDs             []string `json:"claim_ids"`
	FactIDs              []string `json:"fact_ids"`
	FlowNodeIDs          []string `json:"flow_node_ids"`
	AnchorIDs            []string `json:"anchor_ids"`
}

type evidenceIndex struct {
	anchors   map[string]anchorInput
	facts     map[string]factInput
	claims    map[string]claimInput
	flowNodes map[string]flowNodeInput
}

type evidenceMetadata struct {
	EpisodeID   string   `json:"episode_id"`
	FlowID      string   `json:"flow_id"`
	State       string   `json:"state"`
	ClaimIDs    []string `json:"claim_ids"`
	FactIDs     []string `json:"fact_ids"`
	FlowNodeIDs []string `json:"flow_node_ids"`
	AnchorIDs   []string `json:"anchor_ids"`
}

// Compose renders the approved question from supplied bytes. The episode is
// rejected unless it is byte-identical to the accepted source episode.
func Compose(episodeJSON, projectionJSON []byte) ([]byte, error) {
	if len(episodeJSON) > maxEpisodeBytes {
		return nil, fmt.Errorf("composer: episode input exceeds %d bytes", maxEpisodeBytes)
	}
	if len(projectionJSON) > maxProjectionBytes {
		return nil, fmt.Errorf("composer: projection input exceeds %d bytes", maxProjectionBytes)
	}
	digest := sha256.Sum256(episodeJSON)
	if got := hex.EncodeToString(digest[:]); got != acceptedEpisodeSHA256 {
		return nil, fmt.Errorf("composer: episode SHA-256 = %q, want accepted digest", got)
	}

	var episode episodeInput
	if err := json.Unmarshal(episodeJSON, &episode); err != nil {
		return nil, fmt.Errorf("composer: decode accepted episode: %w", err)
	}
	var projection projectionInput
	if err := decodeStrict(projectionJSON, &projection); err != nil {
		return nil, fmt.Errorf("composer: decode projection: %w", err)
	}
	index, err := indexEpisode(episode)
	if err != nil {
		return nil, err
	}
	if err := validateProjection(projection, episode, index); err != nil {
		return nil, err
	}

	rendered, err := render(episode, projection, index)
	if err != nil {
		return nil, err
	}
	if len(rendered) > maxOutputBytes {
		return nil, fmt.Errorf("composer: rendered output exceeds %d bytes", maxOutputBytes)
	}
	return rendered, nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}

func indexEpisode(episode episodeInput) (evidenceIndex, error) {
	if episode.ArtifactKind != episodeKind || episode.ArtifactVersion != artifactVersion {
		return evidenceIndex{}, errors.New("composer: unsupported accepted episode")
	}
	index := evidenceIndex{
		anchors:   make(map[string]anchorInput),
		facts:     make(map[string]factInput),
		claims:    make(map[string]claimInput),
		flowNodes: make(map[string]flowNodeInput),
	}
	for _, item := range episode.Anchors {
		index.anchors[item.ID] = item
	}
	for _, item := range episode.Facts {
		index.facts[item.ID] = item
	}
	for _, item := range episode.Claims {
		index.claims[item.ID] = item
	}
	for _, item := range episode.Flow.Nodes {
		index.flowNodes[item.ID] = item
	}
	return index, nil
}

func validateProjection(projection projectionInput, episode episodeInput, index evidenceIndex) error {
	if projection.ArtifactKind != projectionKind || projection.ArtifactVersion != artifactVersion {
		return errors.New("composer: unsupported projection artifact")
	}
	if projection.EpisodeID != episode.EpisodeID || projection.FlowID != episode.Flow.ID {
		return errors.New("composer: projection does not reference the accepted episode flow")
	}
	if projection.Question != projectionQuestion {
		return errors.New("composer: projection question differs from the approved question")
	}
	if projection.BarrierDesign.State != "inferred" || projection.BehaviorChangeState != "inferred" {
		return errors.New("composer: proposed design and behavior change must remain inferred")
	}
	if projection.RemainingLimits.State != "unknown" {
		return errors.New("composer: remaining limits must remain unknown")
	}
	if projection.BarrierDesign.CorrelationPredicate != requiredCorrelation {
		return fmt.Errorf("composer: correlation predicate = %q, want %q", projection.BarrierDesign.CorrelationPredicate, requiredCorrelation)
	}
	if projection.BarrierDesign.ReleaseAnchorID != requiredReleaseAnchor ||
		projection.BarrierDesign.PostCommitAnchorID != requiredPostCommitAnchor ||
		projection.BarrierDesign.CoverageAnchorID != requiredCoverageAnchor {
		return errors.New("composer: barrier seams differ from the accepted two-ended design")
	}

	if err := validateClaimIDs("current_behavior", projection.CurrentBehavior.ClaimIDs, index); err != nil {
		return err
	}
	if err := validateBarrierEvidence(projection.BarrierDesign, index); err != nil {
		return err
	}
	if err := validateClaimIDs("remaining_limits", projection.RemainingLimits.ClaimIDs, index); err != nil {
		return err
	}
	if err := requireIDs("remaining_limits.claim_ids", projection.RemainingLimits.ClaimIDs, requiredUnknownClaim); err != nil {
		return err
	}
	for _, id := range projection.RemainingLimits.ClaimIDs {
		if index.claims[id].State != "unknown" {
			return fmt.Errorf("composer: remaining limit %q is not unknown", id)
		}
	}
	return nil
}

func validateClaimIDs(name string, ids []string, index evidenceIndex) error {
	if len(ids) == 0 || len(ids) > maxReferences {
		return fmt.Errorf("composer: projection %s.claim_ids must contain 1–%d IDs", name, maxReferences)
	}
	seen := make(map[string]struct{})
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return fmt.Errorf("composer: projection %s.claim_ids contains duplicate %q", name, id)
		}
		seen[id] = struct{}{}
		if _, ok := index.claims[id]; !ok {
			return fmt.Errorf("composer: projection %s.claim_ids references unknown ID %q", name, id)
		}
	}
	return nil
}

func validateBarrierEvidence(barrier barrierDesignSelection, index evidenceIndex) error {
	if err := validateKnownIDs("barrier_design.claim_ids", barrier.ClaimIDs, func(id string) bool {
		_, ok := index.claims[id]
		return ok
	}); err != nil {
		return err
	}
	if err := validateKnownIDs("barrier_design.fact_ids", barrier.FactIDs, func(id string) bool {
		_, ok := index.facts[id]
		return ok
	}); err != nil {
		return err
	}
	if err := validateKnownIDs("barrier_design.flow_node_ids", barrier.FlowNodeIDs, func(id string) bool {
		_, ok := index.flowNodes[id]
		return ok
	}); err != nil {
		return err
	}
	if err := validateKnownIDs("barrier_design.anchor_ids", barrier.AnchorIDs, func(id string) bool {
		_, ok := index.anchors[id]
		return ok
	}); err != nil {
		return err
	}

	selectedClaims := make(map[string]struct{})
	supportedFacts := make(map[string]struct{})
	supportedAnchors := make(map[string]struct{})
	for _, id := range barrier.ClaimIDs {
		selectedClaims[id] = struct{}{}
		for _, factID := range index.claims[id].SupportFactIDs {
			supportedFacts[factID] = struct{}{}
		}
		for _, anchorID := range index.claims[id].AnchorIDs {
			supportedAnchors[anchorID] = struct{}{}
		}
	}
	for _, id := range barrier.FactIDs {
		if _, ok := supportedFacts[id]; !ok {
			return fmt.Errorf("composer: barrier fact %q is outside selected claim support", id)
		}
	}
	for _, id := range barrier.AnchorIDs {
		if _, ok := supportedAnchors[id]; !ok {
			return fmt.Errorf("composer: barrier anchor %q is outside selected claim support", id)
		}
	}
	for _, id := range barrier.FlowNodeIDs {
		claimID := index.flowNodes[id].ClaimID
		if _, ok := selectedClaims[claimID]; !ok {
			return fmt.Errorf("composer: barrier flow node %q lacks selected claim %q", id, claimID)
		}
	}
	if err := requireIDs(
		"barrier_design.claim_ids",
		barrier.ClaimIDs,
		"claim-success-waits-for-local-apply",
		"claim-applied-put-is-mvcc-visible",
		"claim-small-put-does-not-wait-for-backend-commit",
		"claim-backend-carries-materialized-keyvalue-bytes",
		"claim-consistent-index-bounds-replay",
	); err != nil {
		return err
	}
	return requireIDs(
		"barrier_design.anchor_ids",
		barrier.AnchorIDs,
		requiredReleaseAnchor,
		requiredPostCommitAnchor,
		requiredCoverageAnchor,
	)
}

func validateKnownIDs(name string, ids []string, known func(string) bool) error {
	if len(ids) == 0 || len(ids) > maxReferences {
		return fmt.Errorf("composer: projection %s must contain 1–%d IDs", name, maxReferences)
	}
	seen := make(map[string]struct{})
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return fmt.Errorf("composer: projection %s contains duplicate %q", name, id)
		}
		seen[id] = struct{}{}
		if !known(id) {
			return fmt.Errorf("composer: projection %s references unknown ID %q", name, id)
		}
	}
	return nil
}

func requireIDs(name string, got []string, required ...string) error {
	present := make(map[string]struct{})
	for _, id := range got {
		present[id] = struct{}{}
	}
	for _, id := range required {
		if _, ok := present[id]; !ok {
			return fmt.Errorf("composer: projection %s must include %q", name, id)
		}
	}
	return nil
}

func render(episode episodeInput, projection projectionInput, index evidenceIndex) ([]byte, error) {
	var output bytes.Buffer
	fmt.Fprintln(&output, "# Where to add a backend-commit acknowledgment barrier")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "Question: **%s**\n\n", projection.Question)
	fmt.Fprintf(&output, "Accepted input: `%s` at [`%s`](%s).\n\n", episode.EpisodeID, episode.Repository.Revision, episode.Repository.WebBase)
	if err := writeMetadata(&output, evidenceMetadata{
		EpisodeID:   episode.EpisodeID,
		FlowID:      projection.FlowID,
		State:       projection.BarrierDesign.State,
		ClaimIDs:    projection.BarrierDesign.ClaimIDs,
		FactIDs:     projection.BarrierDesign.FactIDs,
		FlowNodeIDs: projection.BarrierDesign.FlowNodeIDs,
		AnchorIDs:   projection.BarrierDesign.AnchorIDs,
	}); err != nil {
		return nil, err
	}

	fmt.Fprintln(&output, "## Short answer — INFERRED")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "This is a two-ended barrier, not one safe line move:")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "1. Keep the already-computed apply result pending at the current request-waiter release seam.")
	fmt.Fprintln(&output, "2. Publish a separate completion notification only after the backend transaction commits successfully.")
	fmt.Fprintf(&output, "3. Release the waiter only when `%s`.\n\n", projection.BarrierDesign.CorrelationPredicate)
	fmt.Fprintln(&output, "That would change successful handler completion from “the Put is locally applied and MVCC-visible” to “the Put is included in a successfully committed bbolt transaction.” The accepted episode does not expose an existing request-to-batch correlation primitive, so this is a design seam, not a ready-made edit.")

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## What happens today")
	fmt.Fprintln(&output)
	writeClaims(&output, projection.CurrentBehavior.ClaimIDs, index.claims)

	release := index.anchors[projection.BarrierDesign.ReleaseAnchorID]
	postCommit := index.anchors[projection.BarrierDesign.PostCommitAnchorID]
	coverage := index.anchors[projection.BarrierDesign.CoverageAnchorID]
	fmt.Fprintln(&output, "## Where to change — INFERRED")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "### 1. Gate response release at [`%s:%d`](%s)\n\n", release.Path, release.StartLine, release.URL)
	fmt.Fprintln(&output, "Today `s.w.Trigger(id, ar)` releases the request-ID waiter after local apply. Keep the apply result associated with its applied Raft index instead of triggering the waiter immediately.")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "### 2. Emit completion after [`%s:%d`](%s)\n\n", postCommit.Path, postCommit.StartLine, postCommit.URL)
	fmt.Fprintln(&output, "The notification must follow a successful `t.tx.Commit()`. A batch commit is shared work, not a per-Put callback, so the commit path should publish a committed watermark rather than wake a request waiter directly.")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "### 3. Correlate through the consistent index from [`%s:%d`](%s)\n\n", coverage.Path, coverage.StartLine, coverage.URL)
	fmt.Fprintf(&output, "The pre-commit hook records the consistent index in the transaction, but it runs before commit and cannot acknowledge success itself. A post-commit signal must carry the committed watermark; pending apply results become releasable only when `%s`.\n", projection.BarrierDesign.CorrelationPredicate)

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## What behavior changes — INFERRED")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "- Local MVCC visibility stays where it is; the response boundary moves later.")
	fmt.Fprintln(&output, "- A below-limit Put can wait for the next successful shared batch commit instead of returning as soon as buffered MVCC state is visible.")
	fmt.Fprintln(&output, "- Preserving batching requires holding multiple pending results and releasing every result covered by the committed consistent-index watermark.")
	fmt.Fprintln(&output, "- Forcing a commit for every Put is a separate policy choice, not a consequence of the episode.")
	fmt.Fprintln(&output, "- Commit failure, cancellation, shutdown, and backpressure need explicit behavior before this can become production code.")

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## What remains unknown")
	fmt.Fprintln(&output)
	writeClaims(&output, projection.RemainingLimits.ClaimIDs, index.claims)
	fmt.Fprintln(&output, "- **UNKNOWN** — The episode does not identify an existing request ID → applied index → committed batch notification path.")
	fmt.Fprintln(&output, "- **UNKNOWN** — Waiting for bbolt commit would not establish filesystem/device persistence or order the response against the relevant WAL save.")
	fmt.Fprintln(&output, "- **UNKNOWN** — The latency and batching cost require a focused runtime experiment; they are not derivable from this source slice.")

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Smallest useful proof before production")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "These are proposed checks, not commands claimed to exist:")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "1. Hold a below-limit Put after MVCC visibility and prove its handler still waits before backend commit.")
	fmt.Fprintln(&output, "2. Commit one batch containing several applied indices and release exactly the pending results covered by its persisted watermark.")
	fmt.Fprintln(&output, "3. Make backend commit fail and prove no covered Put reports success.")
	fmt.Fprintln(&output, "4. Measure the latency and batching delta without changing WAL-order claims.")

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Pinned source seams")
	fmt.Fprintln(&output)
	for _, id := range orderedUnion(
		projection.BarrierDesign.AnchorIDs,
		claimAnchorIDs(projection.RemainingLimits.ClaimIDs, index.claims),
	) {
		anchor := index.anchors[id]
		fmt.Fprintf(&output, "- [`%s` lines %d–%d](%s)\n", anchor.Path, anchor.StartLine, anchor.EndLine, anchor.URL)
	}
	return output.Bytes(), nil
}

func claimAnchorIDs(ids []string, claims map[string]claimInput) []string {
	var result []string
	for _, id := range ids {
		result = append(result, claims[id].AnchorIDs...)
	}
	return result
}

func writeClaims(output *bytes.Buffer, ids []string, claims map[string]claimInput) {
	for _, id := range ids {
		claim := claims[id]
		fmt.Fprintf(output, "- **%s** — %s. %s\n", strings.ToUpper(claim.State), claim.Title, claim.Statement)
	}
	fmt.Fprintln(output)
}

func writeMetadata(output *bytes.Buffer, metadata evidenceMetadata) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("composer: encode evidence metadata: %w", err)
	}
	fmt.Fprintf(output, "<!-- where-to-change %s -->\n\n", raw)
	return nil
}

func orderedUnion(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, group := range groups {
		for _, value := range group {
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}
