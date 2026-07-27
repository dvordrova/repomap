package causalpipeline

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var localAbsolutePathPattern = regexp.MustCompile(`(?:^|[\s"'=(\[])(?:/Users/|/home/|[A-Za-z]:[\\/])`)

func TestComposeDjangoMatchesReviewedArtifacts(t *testing.T) {
	episodeJSON := readFixture(t, "../source-episode/django-atomic/episode.json")
	questionJSON := readFixture(t, "../source-episode/django-atomic/outbox-question.json")

	got, err := Compose(episodeJSON, questionJSON)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	again, err := Compose(episodeJSON, questionJSON)
	if err != nil {
		t.Fatalf("second Compose() error = %v", err)
	}
	if !bytes.Equal(got.CausalSliceJSON, again.CausalSliceJSON) ||
		!bytes.Equal(got.WhereToChangeMarkdown, again.WhereToChangeMarkdown) {
		t.Fatal("Compose() is not deterministic")
	}
	assertBytesEqual(t, "causal slice", got.CausalSliceJSON, readFixture(t, "../source-episode/django-atomic/outbox-causal-slice.json"))
	assertBytesEqual(t, "where-to-change", got.WhereToChangeMarkdown, readFixture(t, "../source-episode/django-atomic/where-to-change.md"))
}

func TestComposeDjangoKeepsSourceTrustSeparateFromDesignTrust(t *testing.T) {
	output := composeDjango(t)
	var got causalSlice
	if err := json.Unmarshal(output.CausalSliceJSON, &got); err != nil {
		t.Fatalf("decode causal slice: %v", err)
	}

	assertStates(t, "claims", []string{"corroborated", "corroborated", "corroborated", "inferred"}, claimStates(got.Evidence.Claims))
	assertStates(t, "facts", []string{"extracted", "extracted", "extracted", "extracted", "corroborated", "corroborated"}, factStates(got.Evidence.Facts))
	assertStates(t, "uncertainties", []string{"unknown"}, uncertaintyStates(got.Evidence.Uncertainties))
	for _, statement := range got.Statements {
		if statement.State != "inferred" && statement.State != "unknown" {
			t.Errorf("authored statement %q promoted to %q", statement.ID, statement.State)
		}
	}
	for _, relation := range got.Relations {
		if relation.State != "inferred" && relation.State != "unknown" {
			t.Errorf("authored relation %q promoted to %q", relation.ID, relation.State)
		}
	}

	for _, required := range []string{
		"application-service boundary that owns the business mutation",
		"no longer provides ordering against that commit",
		"Independently scan pending outbox rows",
		"Publish-to-record gap",
		"Exact application edit points were not inspected",
		"Source navigation — context, not automatic edit points",
	} {
		if !bytes.Contains(output.WhereToChangeMarkdown, []byte(required)) {
			t.Errorf("where-to-change.md is missing %q", required)
		}
	}
}

func TestComposeReplaysEtcdThroughTheSameCausalBoundary(t *testing.T) {
	episodeJSON := readFixture(t, "../source-episode/etcd-put/episode.json")
	output, err := Compose(episodeJSON, []byte(etcdQuestionSpec))
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}

	var got causalSlice
	if err := json.Unmarshal(output.CausalSliceJSON, &got); err != nil {
		t.Fatalf("decode causal slice: %v", err)
	}
	if got.Flow == nil || got.Flow.ID != "flow-successful-put-partial" || got.Flow.State != "partial" {
		t.Fatalf("flow = %#v, want accepted partial etcd flow", got.Flow)
	}
	wantClaimIDs := []string{
		"claim-success-waits-for-local-apply",
		"claim-applied-put-is-mvcc-visible",
		"claim-small-put-does-not-wait-for-backend-commit",
		"claim-backend-carries-materialized-keyvalue-bytes",
		"claim-consistent-index-bounds-replay",
		"claim-ack-versus-wal-save-order-is-unknown",
	}
	wantFactIDs := []string{
		"fact-apply-result-triggers-waiter",
		"fact-mvcc-end-publishes-revision",
		"fact-put-buffer-readable-before-commit",
		"fact-backend-commits-periodically",
		"fact-mvcc-encodes-revision-and-keyvalue",
		"fact-backend-commit-saves-consistent-index",
	}
	wantFlowNodeIDs := []string{
		"flow-node-client-ack",
		"flow-node-mvcc-visible",
		"flow-node-backend-committed",
		"flow-node-replay-guard",
	}
	wantAnchorIDs := []string{
		"anchor-apply-result-to-waiter",
		"anchor-mvcc-revision-publication",
		"anchor-put-read-buffer-before-commit",
		"anchor-backend-periodic-commit",
		"anchor-mvcc-keyvalue-bytes",
		"anchor-backend-saves-consistent-index",
		"anchor-backend-bbolt-commit",
	}
	if !reflect.DeepEqual(claimIDs(got.Evidence.Claims), wantClaimIDs) {
		t.Fatalf("claim IDs = %q, want %q", claimIDs(got.Evidence.Claims), wantClaimIDs)
	}
	if !reflect.DeepEqual(factIDs(got.Evidence.Facts), wantFactIDs) {
		t.Fatalf("fact IDs = %q, want %q", factIDs(got.Evidence.Facts), wantFactIDs)
	}
	if !reflect.DeepEqual(flowNodeIDs(got.Evidence.FlowNodes), wantFlowNodeIDs) {
		t.Fatalf("flow node IDs = %q, want %q", flowNodeIDs(got.Evidence.FlowNodes), wantFlowNodeIDs)
	}
	if !reflect.DeepEqual(anchorIDs(got.Evidence.Anchors), wantAnchorIDs) {
		t.Fatalf("anchor IDs = %q, want %q", anchorIDs(got.Evidence.Anchors), wantAnchorIDs)
	}
	assertStates(t, "etcd claims", []string{"corroborated", "corroborated", "corroborated", "corroborated", "corroborated", "unknown"}, claimStates(got.Evidence.Claims))
	assertStates(t, "etcd facts", []string{"extracted", "extracted", "extracted", "extracted", "extracted", "extracted"}, factStates(got.Evidence.Facts))
	if got.Evidence.Claims[len(got.Evidence.Claims)-1].Strength != "open" {
		t.Fatalf("unknown etcd claim strength = %q, want open", got.Evidence.Claims[len(got.Evidence.Claims)-1].Strength)
	}
	if !reflect.DeepEqual(uncertaintyIDs(got.Evidence.Uncertainties), []string{"uncertainty-local-wal-timing-at-ack"}) ||
		!reflect.DeepEqual(uncertaintyStates(got.Evidence.Uncertainties), []string{"unknown"}) {
		t.Fatalf("etcd uncertainties = %#v, want accepted WAL-timing unknown", got.Evidence.Uncertainties)
	}
	if len(got.Statements) != 2 ||
		got.Statements[0].ID != "design-etcd-two-ended-barrier" ||
		got.Statements[0].Role != "answer" ||
		got.Statements[0].State != "inferred" ||
		got.Statements[1].ID != "unknown-etcd-wal-order" ||
		got.Statements[1].Role != "limit" ||
		got.Statements[1].State != "unknown" ||
		!reflect.DeepEqual(got.Statements[1].ClaimIDs, []string{"claim-ack-versus-wal-save-order-is-unknown"}) ||
		!reflect.DeepEqual(got.Statements[1].UncertaintyIDs, []string{"uncertainty-local-wal-timing-at-ack"}) {
		t.Fatalf("etcd authored trust boundary drifted: %#v", got.Statements)
	}
	if len(got.Relations) != 1 ||
		got.Relations[0].ID != "relation-commit-watermark-gates-ack" ||
		got.Relations[0].State != "inferred" ||
		got.Relations[0].FromID != "flow-node-backend-committed" ||
		got.Relations[0].ToID != "flow-node-client-ack" {
		t.Fatalf("etcd proposed relation drifted: %#v", got.Relations)
	}
	for _, relation := range got.Relations {
		if relation.FromID == "flow-node-wal-stable" || relation.ToID == "flow-node-wal-stable" {
			t.Fatalf("authored relation invents ACK/WAL ordering: %#v", relation)
		}
	}

	wantPaths := map[string]string{
		"anchor-apply-result-to-waiter":         "server/etcdserver/server.go",
		"anchor-backend-bbolt-commit":           "server/storage/backend/batch_tx.go",
		"anchor-backend-saves-consistent-index": "server/storage/hooks.go",
	}
	for _, anchor := range got.Evidence.Anchors {
		if want, ok := wantPaths[anchor.ID]; ok && anchor.Path != want {
			t.Errorf("anchor %q path = %q, want %q", anchor.ID, anchor.Path, want)
		}
	}

	var reviewed struct {
		CurrentBehavior struct {
			ClaimIDs []string `json:"claim_ids"`
		} `json:"current_behavior"`
		BarrierDesign struct {
			State       string   `json:"state"`
			ClaimIDs    []string `json:"claim_ids"`
			FactIDs     []string `json:"fact_ids"`
			FlowNodeIDs []string `json:"flow_node_ids"`
			AnchorIDs   []string `json:"anchor_ids"`
		} `json:"barrier_design"`
		BehaviorChangeState string `json:"behavior_change_state"`
		RemainingLimits     struct {
			State    string   `json:"state"`
			ClaimIDs []string `json:"claim_ids"`
		} `json:"remaining_limits"`
	}
	if err := json.Unmarshal(readFixture(t, "../source-episode/etcd-put/where-to-change.projection.json"), &reviewed); err != nil {
		t.Fatalf("decode reviewed etcd projection: %v", err)
	}
	replayedDesignClaims := got.Evidence.Claims[:len(got.Evidence.Claims)-1]
	if !reflect.DeepEqual(claimIDs(got.Evidence.Claims[:3]), reviewed.CurrentBehavior.ClaimIDs) ||
		!reflect.DeepEqual(claimIDs(replayedDesignClaims), reviewed.BarrierDesign.ClaimIDs) ||
		!reflect.DeepEqual(factIDs(got.Evidence.Facts), reviewed.BarrierDesign.FactIDs) ||
		!reflect.DeepEqual(flowNodeIDs(got.Evidence.FlowNodes), reviewed.BarrierDesign.FlowNodeIDs) ||
		!reflect.DeepEqual(anchorIDs(got.Evidence.Anchors), reviewed.BarrierDesign.AnchorIDs) ||
		!reflect.DeepEqual([]string{got.Evidence.Claims[len(got.Evidence.Claims)-1].ID}, reviewed.RemainingLimits.ClaimIDs) {
		t.Fatal("neutral replay changed the reviewed etcd evidence selection")
	}
	if reviewed.BarrierDesign.State != "inferred" ||
		reviewed.BehaviorChangeState != "inferred" ||
		reviewed.RemainingLimits.State != "unknown" {
		t.Fatalf("reviewed etcd trust states drifted: barrier=%q behavior=%q limits=%q", reviewed.BarrierDesign.State, reviewed.BehaviorChangeState, reviewed.RemainingLimits.State)
	}
}

func TestComposeRejectsDriftAtTheQuestionBoundary(t *testing.T) {
	episodeJSON := readFixture(t, "../source-episode/django-atomic/episode.json")
	questionJSON := readFixture(t, "../source-episode/django-atomic/outbox-question.json")

	tests := []struct {
		name        string
		old         string
		replacement string
		wantError   string
	}{
		{
			name:        "unknown selected claim",
			old:         `"claim-inner-success-remains-rollbackable"`,
			replacement: `"claim-does-not-exist"`,
			wantError:   "unknown ID",
		},
		{
			name:        "unselected statement reference",
			old:         `"claim-oncommit-is-handoff-not-delivery"`,
			replacement: `"claim-outermost-owns-commit-boundary"`,
			wantError:   "unselected ID",
		},
		{
			name:        "promoted design",
			old:         `"state": "inferred"`,
			replacement: `"state": "corroborated"`,
			wantError:   "must be inferred",
		},
		{
			name:        "promoted unknown",
			old:         "\"id\": \"unknown-commit-to-callback-gap\",\n      \"role\": \"limit\",\n      \"state\": \"unknown\"",
			replacement: "\"id\": \"unknown-commit-to-callback-gap\",\n      \"role\": \"limit\",\n      \"state\": \"inferred\"",
			wantError:   "must be unknown",
		},
		{
			name:        "unknown field",
			old:         `"artifact_version": "1",`,
			replacement: `"artifact_version": "1",` + "\n  " + `"language": "python",`,
			wantError:   "unknown field",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := replaceOnce(t, questionJSON, test.old, test.replacement)
			_, err := Compose(episodeJSON, mutated)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Compose() error = %v, want substring %q", err, test.wantError)
			}
		})
	}

	_, err := Compose(episodeJSON, bytes.Repeat([]byte("x"), maxQuestionBytes+1))
	if err == nil || !strings.Contains(err.Error(), "question input exceeds") {
		t.Fatalf("oversized question error = %v", err)
	}
	_, err = Compose(bytes.Repeat([]byte("x"), maxEpisodeBytes+1), questionJSON)
	if err == nil || !strings.Contains(err.Error(), "episode input exceeds") {
		t.Fatalf("oversized episode error = %v", err)
	}
}

func TestComposeEmitsNavigationWithoutRuntimeProofFields(t *testing.T) {
	output := composeDjango(t)
	for name, raw := range map[string][]byte{
		"causal slice":    output.CausalSliceJSON,
		"where-to-change": output.WhereToChangeMarkdown,
	} {
		if localAbsolutePathPattern.Match(raw) {
			t.Errorf("%s contains a local absolute path: %s", name, localAbsolutePathPattern.Find(raw))
		}
		for _, forbidden := range []string{"line_sha256", "blob_oid", "needle", "file://"} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Errorf("%s contains runtime proof field %q", name, forbidden)
			}
		}
	}
	if !bytes.Contains(output.WhereToChangeMarkdown, []byte("django/db/transaction.py:242")) {
		t.Fatal("where-to-change.md lost ordinary source navigation")
	}
}

func composeDjango(t *testing.T) Output {
	t.Helper()
	output, err := Compose(
		readFixture(t, "../source-episode/django-atomic/episode.json"),
		readFixture(t, "../source-episode/django-atomic/outbox-question.json"),
	)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	return output
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return raw
}

func replaceOnce(t *testing.T, raw []byte, old, replacement string) []byte {
	t.Helper()
	if bytes.Count(raw, []byte(old)) == 0 {
		t.Fatalf("fixture does not contain %q", old)
	}
	return bytes.Replace(raw, []byte(old), []byte(replacement), 1)
}

func assertBytesEqual(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	limit := min(len(got), len(want))
	offset := limit
	for i := 0; i < limit; i++ {
		if got[i] != want[i] {
			offset = i
			break
		}
	}
	t.Fatalf("%s differs at byte %d\n--- got ---\n%s\n--- want ---\n%s", name, offset, got, want)
}

func assertStates(t *testing.T, name string, want, got []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s states = %q, want %q", name, got, want)
	}
}

func claimIDs(items []claimInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func factIDs(items []factInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func flowNodeIDs(items []flowNodeInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func anchorIDs(items []anchorInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func claimStates(items []claimInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.State)
	}
	return result
}

func factStates(items []factInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.State)
	}
	return result
}

func uncertaintyStates(items []uncertaintyInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.State)
	}
	return result
}

func uncertaintyIDs(items []uncertaintyInput) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

const etcdQuestionSpec = `{
  "artifact_kind": "source-episode-question-spec",
  "artifact_version": "1",
  "question_id": "etcd-put-backend-ack-change",
  "episode_id": "etcd-put-recoverability",
  "flow_id": "flow-successful-put-partial",
  "title": "Where to add a backend-commit acknowledgment barrier",
  "question": "Where would a backend-commit acknowledgment barrier be inserted, and what behavior would it change?",
  "evidence": {
    "claim_ids": [
      "claim-success-waits-for-local-apply",
      "claim-applied-put-is-mvcc-visible",
      "claim-small-put-does-not-wait-for-backend-commit",
      "claim-backend-carries-materialized-keyvalue-bytes",
      "claim-consistent-index-bounds-replay",
      "claim-ack-versus-wal-save-order-is-unknown"
    ],
    "fact_ids": [
      "fact-apply-result-triggers-waiter",
      "fact-mvcc-end-publishes-revision",
      "fact-put-buffer-readable-before-commit",
      "fact-backend-commits-periodically",
      "fact-mvcc-encodes-revision-and-keyvalue",
      "fact-backend-commit-saves-consistent-index"
    ],
    "flow_node_ids": [
      "flow-node-client-ack",
      "flow-node-mvcc-visible",
      "flow-node-backend-committed",
      "flow-node-replay-guard"
    ],
    "anchor_ids": [
      "anchor-apply-result-to-waiter",
      "anchor-mvcc-revision-publication",
      "anchor-put-read-buffer-before-commit",
      "anchor-backend-periodic-commit",
      "anchor-mvcc-keyvalue-bytes",
      "anchor-backend-saves-consistent-index",
      "anchor-backend-bbolt-commit"
    ],
    "uncertainty_ids": [
      "uncertainty-local-wal-timing-at-ack"
    ]
  },
  "design_statements": [
    {
      "id": "design-etcd-two-ended-barrier",
      "role": "answer",
      "state": "inferred",
      "title": "Gate response release on a committed watermark",
      "statement": "Keep the apply result pending at the waiter seam and release it only after a successful backend commit covers its applied index.",
      "claim_ids": [
        "claim-success-waits-for-local-apply",
        "claim-small-put-does-not-wait-for-backend-commit",
        "claim-consistent-index-bounds-replay"
      ],
      "fact_ids": [
        "fact-apply-result-triggers-waiter",
        "fact-backend-commit-saves-consistent-index"
      ],
      "flow_node_ids": [
        "flow-node-client-ack",
        "flow-node-backend-committed",
        "flow-node-replay-guard"
      ],
      "anchor_ids": [
        "anchor-apply-result-to-waiter",
        "anchor-backend-saves-consistent-index",
        "anchor-backend-bbolt-commit"
      ]
    },
    {
      "id": "unknown-etcd-wal-order",
      "role": "limit",
      "state": "unknown",
      "title": "WAL ordering remains outside this answer",
      "statement": "The selected source does not order client acknowledgment against the relevant WAL Save completion.",
      "claim_ids": [
        "claim-ack-versus-wal-save-order-is-unknown"
      ],
      "uncertainty_ids": [
        "uncertainty-local-wal-timing-at-ack"
      ]
    }
  ],
  "relations": [
    {
      "id": "relation-commit-watermark-gates-ack",
      "state": "inferred",
      "from_id": "flow-node-backend-committed",
      "to_id": "flow-node-client-ack",
      "statement": "The proposed design adds this ordering; the accepted episode does not claim it exists today."
    }
  ]
}`
