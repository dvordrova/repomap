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
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version          = 4
	ArtifactFilename = "orientation.json"
	RejectedFilename = "rejected.jsonl"

	digestDomain = "repomap-orientation-v1\x00"
)

// Role is the model's one-line description of what one target is. SubjectIDs
// name GroupsIndex subjects (group members) the role points at, qualified by
// their target (t1.n3) because bare subject ids repeat across targets. Summary
// refs qualify subjects the same way. Purpose may be empty: the label alone is
// the model's decision.
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
// target in TargetID. A walked flow (path.go) writes no prose: Explanation
// is the atlas line accepted for the declaration, Via how the step before
// reaches it, as code says it ("called", "one of 3", "handed to
// quil.core.sketch.setup"), Site, for one of a dispatch site's alternatives,
// the declaration of the target holding that site (its function, never its
// line). On the last step of a flow, or of one of its paths, that ends at an
// undecided split: Paths, the ways followed from it, each a path of its own
// (owner, 2026-09-30: several main paths are allowed where the model is torn
// between them), and Branches, the candidates no way follows. At a split the
// categorizer decided, Passed are the candidates the path did not follow
// (version 2): the calls of the step it goes on beside (freqtrade's
// FreqtradeBot.process passes IStrategy). Registered and RunBy (version 3)
// say where a registered callable is registered and what runs it. Joins
// (version 4) are every way a path goes on as, each with its reach.
type FlowStep struct {
	TargetID    string `json:"target_id"`
	FactID      string `json:"fact_id,omitempty"`
	SubjectID   string `json:"subject_id,omitempty"`
	Explanation string `json:"explanation,omitempty"`
	Via         string `json:"via,omitempty"`
	Site        string `json:"site,omitempty"`
	// Through is the helper the step before's work passes on its way to
	// this one: a subject the helper question decided serves others' work,
	// never a step of its own (Lua's handle_script reaches lua_pcallk
	// through docall).
	Through []string `json:"through,omitempty"`
	// Basis is, for a step the step before calls as a method of a
	// repository type implementing the interface it calls, no observed flow
	// giving the value, "implements" (ProgramIndex Relation.Basis): known by
	// method set, never a traced call (etcd's gateway reaching
	// electionServer.Campaign).
	Basis string `json:"basis,omitempty"`
	// Guard is the construct the step before's call of it runs under, the
	// weakest of its ways (ProgramIndex Guard), Loop the loop statement it
	// runs in; Stop, on the last step of a path, why the path ends there.
	Guard *programindex.Guard    `json:"guard,omitempty"`
	Loop  *programindex.Location `json:"loop,omitempty"`
	Stop  string                 `json:"stop,omitempty"`
	// Joins are, on the last step of a path, the ways it goes on as: each
	// chosen candidate that is where another way of the splits around it
	// starts, reached as a branch is, never walked twice. They end a path
	// alone (StopJoins) or stand beside its own ways (StopTorn).
	Joins []FlowBranch `json:"joins,omitempty"`
	// OpenAt is, on a route's last step, where the step calls through a
	// value the index leaves open with no possible target.
	OpenAt   *programindex.Location `json:"open_at,omitempty"`
	Branches []FlowBranch           `json:"branches,omitempty"`
	Passed   []FlowBranch           `json:"passed,omitempty"`
	Paths    []FlowPath             `json:"paths,omitempty"`
	// Registered and RunBy are, for a step whose callable a registration
	// hands over, where it is registered and what runs it, as the walk
	// reaches them (readRegistrations, version 3).
	Registered []FlowRegistration `json:"registered,omitempty"`
	RunBy      []FlowRunner       `json:"run_by,omitempty"`
}

// FlowPath is one way a Main flow goes on from a split: its steps, the last
// of which may part again.
type FlowPath struct {
	Steps []FlowStep `json:"steps"`
}

// FlowBranch is one candidate of a named fork: the declaration and how the
// fork's step reaches it.
type FlowBranch struct {
	SubjectID string                 `json:"subject_id"`
	Via       string                 `json:"via,omitempty"`
	Site      string                 `json:"site,omitempty"`
	Through   []string               `json:"through,omitempty"`
	Basis     string                 `json:"basis,omitempty"`
	Guard     *programindex.Guard    `json:"guard,omitempty"`
	Loop      *programindex.Location `json:"loop,omitempty"`
}

// Why a walked path ends at its last step (FlowStep.Stop).
const (
	StopTorn        = "torn"
	StopUnanswered  = "unanswered"
	StopLeaf        = "leaf"
	StopFailureOnly = "failure_only"
	StopRevisits    = "revisits"
	StopJoins       = "joins"
)

// validStop checks why a path ends against the ways it joins: a join ends
// a path alone or stands beside the ways of a torn split, never elsewhere.
func validStop(stop string, joins int) bool {
	switch stop {
	case "", StopUnanswered, StopLeaf, StopFailureOnly, StopRevisits:
		return joins == 0
	case StopTorn:
		return true
	case StopJoins:
		return joins > 0
	}
	return false
}

func validGuard(guard *programindex.Guard) bool {
	return guard == nil || guard.Kind == programindex.GuardBranch || guard.Kind == programindex.GuardError || guard.Kind == programindex.GuardNoReturn
}

func cloneGuard(guard *programindex.Guard) *programindex.Guard {
	if guard == nil {
		return nil
	}
	copied := *guard
	copied.Location = cloneLocation(guard.Location)
	return &copied
}

func cloneLocation(location *programindex.Location) *programindex.Location {
	if location == nil {
		return nil
	}
	copied := *location
	return &copied
}

// MainFlow is the one end-to-end path the reader should follow first.
type MainFlow struct {
	// Title is written empty and read by no one: the page names the parts a
	// flow passes through instead of "From main to <where the walk
	// stopped>" (2026-10-03). It goes at the next version's change, so a
	// saved version 4 artifact still decodes.
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
		if !validText(role.TargetID) || !validSentence(role.Role) || role.Purpose != "" && !validSentence(role.Purpose) {
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
	if err := validFlowSteps(result.MainFlow.Steps); err != nil {
		return err
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

// validThrough checks the helpers a flow step or branch is reached through:
// each a subject ref.
func validThrough(through []string) bool {
	return !slices.ContainsFunc(through, func(id string) bool { return !validText(id) })
}

// validBasis checks how a flow step or branch is known: observed (empty)
// or by the implementations of the interface called.
func validBasis(basis string) bool {
	return basis == "" || basis == programindex.BasisImplements
}

// validFlowSteps checks a flow's steps and, on a step that parts, each of
// its paths the same way.
func validFlowSteps(steps []FlowStep) error {
	for position, step := range steps {
		if !validText(step.TargetID) || step.Explanation != "" && !validSentence(step.Explanation) || step.Via != "" && !validSentence(step.Via) || step.Site != "" && !validText(step.Site) ||
			!validThrough(step.Through) || !validBasis(step.Basis) || !validGuard(step.Guard) || !validStop(step.Stop, len(step.Joins)) {
			return fmt.Errorf("orientation: flow step %d is invalid", position)
		}
		for _, branch := range slices.Concat(step.Branches, step.Passed, step.Joins) {
			if !validText(branch.SubjectID) || branch.Via != "" && !validSentence(branch.Via) || branch.Site != "" && !validText(branch.Site) || !validThrough(branch.Through) ||
				!validBasis(branch.Basis) || !validGuard(branch.Guard) {
				return fmt.Errorf("orientation: flow step %d branch is invalid", position)
			}
		}
		for _, registration := range step.Registered {
			if !validText(registration.FactID) || !validChain(registration.Chain) {
				return fmt.Errorf("orientation: flow step %d registration is invalid", position)
			}
		}
		for _, runner := range step.RunBy {
			if !validChain(runner.Chain) {
				return fmt.Errorf("orientation: flow step %d runner is invalid", position)
			}
		}
		if (step.FactID == "") == (step.SubjectID == "") {
			return fmt.Errorf("orientation: flow step %d needs exactly one of fact_id or subject_id", position)
		}
		if step.FactID != "" && !validText(step.FactID) || step.SubjectID != "" && !validText(step.SubjectID) {
			return fmt.Errorf("orientation: flow step %d ref is invalid", position)
		}
		if len(step.Paths) > 0 && position != len(steps)-1 {
			return fmt.Errorf("orientation: flow step %d parts before the path ends", position)
		}
		for _, path := range step.Paths {
			if len(path.Steps) == 0 {
				return fmt.Errorf("orientation: flow step %d has an empty path", position)
			}
			if err := validFlowSteps(path.Steps); err != nil {
				return err
			}
		}
	}
	return nil
}

// validChain checks a run of hops: at least one, each naming a subject, the
// first reached by nothing.
func validChain(chain []FlowHop) bool {
	if len(chain) == 0 || chain[0].Possible || chain[0].Handed {
		return false
	}
	for _, hop := range chain {
		if !validText(hop.SubjectID) || hop.Possible && hop.Handed {
			return false
		}
	}
	return true
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
	owned.MainFlow.Steps = cloneFlowSteps(result.MainFlow.Steps)
	return owned
}

// cloneBranches copies a step's branches with the helpers each is reached
// through.
func cloneBranches(branches []FlowBranch) []FlowBranch {
	owned := cloneSlice(branches)
	for position := range owned {
		owned[position].Through = cloneSlice(branches[position].Through)
		owned[position].Guard, owned[position].Loop = cloneGuard(branches[position].Guard), cloneLocation(branches[position].Loop)
	}
	return owned
}

// cloneFlowSteps copies a flow's steps, their branches and paths.
func cloneFlowSteps(steps []FlowStep) []FlowStep {
	owned := cloneSlice(steps)
	for position := range owned {
		owned[position].Through = cloneSlice(steps[position].Through)
		owned[position].Guard, owned[position].Loop = cloneGuard(steps[position].Guard), cloneLocation(steps[position].Loop)
		owned[position].OpenAt = cloneLocation(steps[position].OpenAt)
		owned[position].Branches = cloneBranches(steps[position].Branches)
		owned[position].Passed = cloneBranches(steps[position].Passed)
		owned[position].Registered = cloneSlice(steps[position].Registered)
		for at := range owned[position].Registered {
			owned[position].Registered[at].Chain = cloneSlice(steps[position].Registered[at].Chain)
		}
		owned[position].RunBy = cloneSlice(steps[position].RunBy)
		for at := range owned[position].RunBy {
			owned[position].RunBy[at].Chain = cloneSlice(steps[position].RunBy[at].Chain)
		}
		if steps[position].Paths != nil {
			owned[position].Paths = make([]FlowPath, len(steps[position].Paths))
			for at, path := range steps[position].Paths {
				owned[position].Paths[at] = FlowPath{Steps: cloneFlowSteps(path.Steps)}
			}
		}
	}
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
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0)
}
