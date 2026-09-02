// Package orientation owns the model-authored first-day guidance of a
// repomap run: one repository summary, one role per target, a run recipe,
// and one main flow. Every row references facts, claims, or group-graph
// subjects by id. Rows whose references do not resolve are rejected and
// recorded, never repaired.
package orientation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	Version          = 1
	ArtifactFilename = "orientation.json"
	RejectedFilename = "rejected.jsonl"

	digestDomain = "repomap-orientation-v1\x00"

	// MaxSentenceRunes bounds every model sentence; the constitution allows
	// one line per purpose and one sentence per step, not essays.
	MaxSentenceRunes = 400
)

// Role is the model's one-line description of what one target is. SubjectIDs
// name GroupsIndex subjects (group members) the role points at.
type Role struct {
	TargetID   string   `json:"target_id"`
	Role       string   `json:"role"`
	Purpose    string   `json:"purpose"`
	FactIDs    []string `json:"fact_ids"`
	ClaimIDs   []string `json:"claim_ids,omitempty"`
	SubjectIDs []string `json:"subject_ids,omitempty"`
}

// RecipeStep is one command a newcomer runs, anchored to the manifest or
// entrypoint facts it was derived from.
type RecipeStep struct {
	TargetID string   `json:"target_id,omitempty"`
	Command  string   `json:"command"`
	Cwd      string   `json:"cwd,omitempty"`
	Note     string   `json:"note,omitempty"`
	FactIDs  []string `json:"fact_ids"`
}

// FlowStep is one step of the main flow. Exactly one of FactID or SubjectID
// is set; SubjectID names a GroupsIndex subject (object or pattern) of the
// target in TargetID.
type FlowStep struct {
	TargetID    string `json:"target_id"`
	FactID      string `json:"fact_id,omitempty"`
	SubjectID   string `json:"subject_id,omitempty"`
	Explanation string `json:"explanation"`
}

// MainFlow is the one end-to-end path the reader should follow first.
type MainFlow struct {
	Title string     `json:"title"`
	Steps []FlowStep `json:"steps"`
}

// Result is the sealed orientation artifact. FactsSHA256 and ClaimsSHA256
// bind it to the exact inputs; GroupsSHA256s binds the GroupsIndex set.
type Result struct {
	Version       int          `json:"version"`
	FactsSHA256   string       `json:"facts_sha256"`
	ClaimsSHA256  string       `json:"claims_sha256"`
	GroupsSHA256s []string     `json:"groups_sha256s"`
	Summary       string       `json:"summary"`
	SummaryRefs   []string     `json:"summary_refs"`
	Roles         []Role       `json:"roles"`
	RunRecipe     []RecipeStep `json:"run_recipe"`
	MainFlow      MainFlow     `json:"main_flow"`
	RejectedCount int          `json:"rejected_count"`
	SHA256        string       `json:"sha256"`
}

// RejectedRow is one model row that failed validation. It keeps the raw
// model output so a human can see what was proposed and why it was refused.
type RejectedRow struct {
	Stage   string          `json:"stage"`
	Section string          `json:"section"`
	Raw     json.RawMessage `json:"raw"`
	Reason  string          `json:"reason"`
}

// Seal checks the shape and computes the digest.
func Seal(result Result) (Result, error) {
	owned := clone(result)
	owned.Version = Version
	if owned.GroupsSHA256s == nil {
		owned.GroupsSHA256s = []string{}
	}
	sort.Strings(owned.GroupsSHA256s)
	if owned.SummaryRefs == nil {
		owned.SummaryRefs = []string{}
	}
	if owned.Roles == nil {
		owned.Roles = []Role{}
	}
	if owned.RunRecipe == nil {
		owned.RunRecipe = []RecipeStep{}
	}
	if owned.MainFlow.Steps == nil {
		owned.MainFlow.Steps = []FlowStep{}
	}
	for position := range owned.Roles {
		if owned.Roles[position].FactIDs == nil {
			owned.Roles[position].FactIDs = []string{}
		}
	}
	for position := range owned.RunRecipe {
		if owned.RunRecipe[position].FactIDs == nil {
			owned.RunRecipe[position].FactIDs = []string{}
		}
	}
	digest, err := resultDigest(owned)
	if err != nil {
		return Result{}, err
	}
	owned.SHA256 = digest
	if err := owned.Validate(); err != nil {
		return Result{}, err
	}
	return owned, nil
}

// Validate checks the closed shape and the seal. Reference resolution against
// facts and groups is the stage's job before sealing; Validate only checks
// the persisted shape.
func (result Result) Validate() error {
	if result.Version != Version {
		return fmt.Errorf("orientation: unsupported version %d", result.Version)
	}
	if !validSHA256(result.FactsSHA256) || !validSHA256(result.ClaimsSHA256) {
		return fmt.Errorf("orientation: input digests are invalid")
	}
	if result.GroupsSHA256s == nil || result.SummaryRefs == nil || result.Roles == nil ||
		result.RunRecipe == nil || result.MainFlow.Steps == nil {
		return fmt.Errorf("orientation: collections are missing")
	}
	for position, digest := range result.GroupsSHA256s {
		if !validSHA256(digest) {
			return fmt.Errorf("orientation: groups digest %d is invalid", position)
		}
		if position > 0 && result.GroupsSHA256s[position-1] >= digest {
			return fmt.Errorf("orientation: groups digests are not canonical")
		}
	}
	if result.Summary != "" && !validSentence(result.Summary) {
		return fmt.Errorf("orientation: summary is invalid")
	}
	if err := validRefs(result.SummaryRefs); err != nil {
		return fmt.Errorf("orientation: summary refs: %w", err)
	}
	for position, role := range result.Roles {
		if !validText(role.TargetID) || !validSentence(role.Role) || !validSentence(role.Purpose) {
			return fmt.Errorf("orientation: role %d is invalid", position)
		}
		if err := validRefs(role.FactIDs); err != nil {
			return fmt.Errorf("orientation: role %d: %w", position, err)
		}
		if err := validRefs(role.ClaimIDs); err != nil {
			return fmt.Errorf("orientation: role %d claims: %w", position, err)
		}
		if err := validRefs(role.SubjectIDs); err != nil {
			return fmt.Errorf("orientation: role %d subjects: %w", position, err)
		}
	}
	for position, step := range result.RunRecipe {
		if !validSentence(step.Command) {
			return fmt.Errorf("orientation: recipe step %d is invalid", position)
		}
		if step.Note != "" && !validSentence(step.Note) {
			return fmt.Errorf("orientation: recipe step %d note is invalid", position)
		}
		if step.Cwd != "" && !validText(step.Cwd) {
			return fmt.Errorf("orientation: recipe step %d cwd is invalid", position)
		}
		if err := validRefs(step.FactIDs); err != nil {
			return fmt.Errorf("orientation: recipe step %d: %w", position, err)
		}
	}
	if result.MainFlow.Title != "" && !validSentence(result.MainFlow.Title) {
		return fmt.Errorf("orientation: main flow title is invalid")
	}
	for position, step := range result.MainFlow.Steps {
		if !validText(step.TargetID) || !validSentence(step.Explanation) {
			return fmt.Errorf("orientation: flow step %d is invalid", position)
		}
		if (step.FactID == "") == (step.SubjectID == "") {
			return fmt.Errorf("orientation: flow step %d needs exactly one of fact_id or subject_id", position)
		}
		if step.FactID != "" && !validText(step.FactID) || step.SubjectID != "" && !validText(step.SubjectID) {
			return fmt.Errorf("orientation: flow step %d ref is invalid", position)
		}
	}
	if result.RejectedCount < 0 {
		return fmt.Errorf("orientation: negative rejected count")
	}
	digest, err := resultDigest(result)
	if err != nil {
		return err
	}
	if digest != result.SHA256 {
		return fmt.Errorf("orientation: digest mismatch")
	}
	return nil
}

// Snapshot validates and returns an independently owned copy.
func (result Result) Snapshot() (Result, error) {
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return clone(result), nil
}

// Empty returns a sealed result with no model output, bound to the inputs.
// It is the legitimate artifact when the stage produced nothing acceptable.
func Empty(factsSHA256, claimsSHA256 string, groupsSHA256s []string, rejected int) (Result, error) {
	return Seal(Result{
		FactsSHA256:   factsSHA256,
		ClaimsSHA256:  claimsSHA256,
		GroupsSHA256s: append([]string(nil), groupsSHA256s...),
		RejectedCount: rejected,
	})
}

func validRefs(refs []string) error {
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if !validText(ref) {
			return fmt.Errorf("invalid ref %q", ref)
		}
		if _, duplicate := seen[ref]; duplicate {
			return fmt.Errorf("duplicate ref %q", ref)
		}
		seen[ref] = struct{}{}
	}
	return nil
}

func resultDigest(result Result) (string, error) {
	unsealed := clone(result)
	unsealed.SHA256 = ""
	encoded, err := json.Marshal(unsealed)
	if err != nil {
		return "", fmt.Errorf("orientation: digest: %w", err)
	}
	hasher := sha256.New()
	hasher.Write([]byte(digestDomain))
	hasher.Write(encoded)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// clone copies every collection and keeps an empty one empty: a sealed result
// distinguishes "the model returned nothing" from "the stage did not run".
func clone(result Result) Result {
	owned := result
	owned.GroupsSHA256s = cloneSlice(result.GroupsSHA256s)
	owned.SummaryRefs = cloneSlice(result.SummaryRefs)
	owned.Roles = make([]Role, len(result.Roles))
	for position, role := range result.Roles {
		copied := role
		copied.FactIDs = cloneSlice(role.FactIDs)
		copied.ClaimIDs = cloneSlice(role.ClaimIDs)
		copied.SubjectIDs = append([]string(nil), role.SubjectIDs...)
		owned.Roles[position] = copied
	}
	owned.RunRecipe = make([]RecipeStep, len(result.RunRecipe))
	for position, step := range result.RunRecipe {
		copied := step
		copied.FactIDs = cloneSlice(step.FactIDs)
		owned.RunRecipe[position] = copied
	}
	owned.MainFlow.Steps = cloneSlice(result.MainFlow.Steps)
	return owned
}

// cloneSlice copies a slice and preserves the difference between an empty
// slice and a missing one.
func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append(make([]T, 0, len(values)), values...)
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || r == 0 {
			return false
		}
	}
	return true
}

func validSentence(value string) bool {
	return validText(value) && utf8.RuneCountInString(value) <= MaxSentenceRunes
}
