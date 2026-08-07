// Package pavedpath defines bounded operational evidence and presentation-only
// operating guides. A Paved Path is not a runtime proof: it preserves exact
// repository-owned commands and references without claiming they succeed.
package pavedpath

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	BundleVersion   = 1
	ProposalVersion = 1
	RecordVersion   = 1

	PromptVersion = "repository-paved-paths-json-v1"
	BundleFile    = "operational_evidence.json"
	AttemptFile   = "paved_paths_attempt.json"
	RecordFile    = "paved_paths.json"
	StatusFile    = "paved_paths_status.json"

	MaxEvidence  = 120
	MaxPaths     = 8
	MaxActions   = 8
	MaxLandmarks = 24

	maxArtifactBytes          = 4 << 20
	maxPublicationResultBytes = 4 << 10

	PublicationIssueMissingPrerequisite = "missing_prerequisite"
	PublicationIssueMissingActions      = "missing_essential_action_sequence"
	PublicationIssueMissingResult       = "missing_observable_result"
)

var publicationOutputFlagRE = regexp.MustCompile(
	`(?:^|[ \t])(?:-o|--output|-coverprofile)(?:=|[ \t]+)(?:"([^"\r\n]+)"|'([^'\r\n]+)'|([^ \t\r\n]+))`,
)

var publicationAbsentPrerequisiteRE = regexp.MustCompile(
	`\b(?:prerequisites?|requirements?)\s*:\s*(?:none|no|n/?a|not\s+applicable|optional)\b`,
)

// EvidenceRole describes an operational affordance, independently of the
// production-code roles used by the Study Map selector.
type EvidenceRole string

const (
	RoleDocumentedProcedure EvidenceRole = "documented_procedure"
	RoleBuildTarget         EvidenceRole = "build_target"
	RolePackageScript       EvidenceRole = "package_script"
	RoleRepositoryScript    EvidenceRole = "repository_script"
	RoleComposeService      EvidenceRole = "compose_service"
	RoleEnvironment         EvidenceRole = "environment_declaration"
	RoleConfiguration       EvidenceRole = "configuration"
	RoleEndpoint            EvidenceRole = "endpoint"
	RoleLogLocation         EvidenceRole = "log_location"
	RoleVerification        EvidenceRole = "verification"
)

type CommandBasis string

const (
	CommandExact      CommandBasis = "exact_documented"
	CommandStructural CommandBasis = "structurally_derived"
)

type OrderingBasis string

const (
	OrderingDocumented OrderingBasis = "documented_procedure"
	OrderingScript     OrderingBasis = "script_sequence"
	OrderingEditorial  OrderingBasis = "editorial"
)

// Bundle is deterministic and contains only bounded, redacted repository
// evidence. It is the complete authority for a Paved Path editor.
type Bundle struct {
	Version      int        `json:"version"`
	RepoName     string     `json:"repo_name"`
	Evidence     []Evidence `json:"evidence"`
	AllowedPaths []string   `json:"allowed_paths"`
	Stats        Stats      `json:"stats"`
}

type Stats struct {
	ConsideredFiles int  `json:"considered_files"`
	ReadFiles       int  `json:"read_files"`
	ReadBytes       int  `json:"read_bytes"`
	Redactions      int  `json:"redactions"`
	Truncated       bool `json:"truncated,omitempty"`
}

type Evidence struct {
	ID         string       `json:"id"`
	Role       EvidenceRole `json:"role"`
	Path       string       `json:"path"`
	StartLine  int          `json:"start_line"`
	EndLine    int          `json:"end_line"`
	Label      string       `json:"label"`
	Excerpt    []string     `json:"excerpt"`
	Commands   []Command    `json:"commands,omitempty"`
	Endpoint   string       `json:"endpoint,omitempty"`
	Target     string       `json:"target,omitempty"`
	Redacted   bool         `json:"redacted,omitempty"`
	Executable bool         `json:"executable,omitempty"`
}

type Command struct {
	Value      string       `json:"value"`
	Basis      CommandBasis `json:"basis"`
	SafeToCopy bool         `json:"safe_to_copy"`
}

// Proposal is model-authored editorial grouping over supplied evidence IDs.
// Version is backend-owned contract identity (Phase 8 reviewer finding): the
// model is not asked to echo it; the decoder stamps ProposalVersion.
type Proposal struct {
	Version int            `json:"version,omitempty"`
	Paths   []ProposedPath `json:"paths"`
}

type ProposedPath struct {
	Title                      string           `json:"title"`
	Goal                       string           `json:"goal"`
	PrerequisiteEvidenceIDs    []string         `json:"prerequisite_evidence_ids,omitempty"`
	Actions                    []ProposedAction `json:"actions"`
	ExpectedEvidenceIDs        []string         `json:"expected_evidence_ids,omitempty"`
	TroubleshootingEvidenceIDs []string         `json:"troubleshooting_evidence_ids,omitempty"`
	RelatedStudyDirectionIDs   []string         `json:"related_study_direction_ids,omitempty"`
	OrderingBasis              OrderingBasis    `json:"ordering_basis"`
}

type ProposedAction struct {
	EvidenceID  string `json:"evidence_id"`
	Instruction string `json:"instruction"`
	Command     string `json:"command,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
}

type Action struct {
	EvidenceID  string `json:"evidence_id"`
	Instruction string `json:"instruction"`
	Command     string `json:"command,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	SafeToCopy  bool   `json:"safe_to_copy"`
}

type Path struct {
	ID                         string        `json:"id"`
	Title                      string        `json:"title"`
	Goal                       string        `json:"goal"`
	PrerequisiteEvidenceIDs    []string      `json:"prerequisite_evidence_ids,omitempty"`
	Actions                    []Action      `json:"actions"`
	ExpectedEvidenceIDs        []string      `json:"expected_evidence_ids,omitempty"`
	TroubleshootingEvidenceIDs []string      `json:"troubleshooting_evidence_ids,omitempty"`
	RelatedStudyDirectionIDs   []string      `json:"related_study_direction_ids,omitempty"`
	OrderingBasis              OrderingBasis `json:"ordering_basis"`
}

type Landmark struct {
	ID         string       `json:"id"`
	EvidenceID string       `json:"evidence_id"`
	Label      string       `json:"label"`
	Command    string       `json:"command,omitempty"`
	Endpoint   string       `json:"endpoint,omitempty"`
	SafeToCopy bool         `json:"safe_to_copy"`
	Role       EvidenceRole `json:"role"`
}

type Issue struct {
	PathIndex int    `json:"path_index"`
	Code      string `json:"code"`
	Detail    string `json:"detail,omitempty"`
}

type PublicationResultKind string

const (
	PublicationResultCommandOutput     PublicationResultKind = "command_output"
	PublicationResultGeneratedArtifact PublicationResultKind = "generated_artifact"
)

// PublicationResult is an internal, non-serialized classification of an
// exact result already present in selected operational evidence.
type PublicationResult struct {
	Kind        PublicationResultKind
	Value       string
	AfterAction int
	EvidenceID  string
	StartOffset int
	EndOffset   int
}

type PublicationAssessment struct {
	IssueCode string
	Results   []PublicationResult
}

type Record struct {
	Version      int        `json:"version"`
	BundleSHA256 string     `json:"bundle_sha256"`
	Bundle       Bundle     `json:"bundle"`
	Paths        []Path     `json:"paths,omitempty"`
	Landmarks    []Landmark `json:"landmarks,omitempty"`
	Issues       []Issue    `json:"issues,omitempty"`
}

func DecodeProposal(raw []byte) (Proposal, error) {
	if len(raw) == 0 || len(raw) > maxArtifactBytes {
		return Proposal{}, fmt.Errorf("paved paths: proposal is outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var proposal Proposal
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, fmt.Errorf("paved paths: decode proposal: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Proposal{}, err
	}
	// Phase 8 reviewer finding (backend-authority leakage): version is
	// backend-owned contract identity — a model response that omits it is
	// stamped with ProposalVersion; a legacy echo is validated as before.
	if proposal.Version != 0 && proposal.Version != ProposalVersion {
		return Proposal{}, fmt.Errorf("paved paths: unsupported proposal version")
	}
	proposal.Version = ProposalVersion
	if len(proposal.Paths) > MaxPaths {
		return Proposal{}, fmt.Errorf("paved paths: oversized proposal")
	}
	return proposal, nil
}

func DecodeRecord(raw []byte) (Record, error) {
	if len(raw) == 0 || len(raw) > maxArtifactBytes {
		return Record{}, fmt.Errorf("paved paths: record is outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("paved paths: decode record: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Record{}, err
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

func BundleHash(bundle Bundle) (string, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("paved paths: marshal bundle: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// BuildRecord rejects paths independently and always retains locally useful
// landmarks. allowedStudyIDs authorizes optional navigation links; it never
// affects operational evidence validity.
func BuildRecord(bundle Bundle, proposal Proposal, allowedStudyIDs []string) (Record, error) {
	return BuildRecordScoped(bundle, proposal, allowedStudyIDs, nil)
}

// BuildRecordScoped retains the full evidence bundle for landmarks while
// allowing model-authored paths to reference only the exact bounded subset
// that was sent to that editor call. A nil scope authorizes the full bundle
// for deterministic fallback construction and backward-compatible callers.
func BuildRecordScoped(
	bundle Bundle,
	proposal Proposal,
	allowedStudyIDs []string,
	allowedEvidenceIDs []string,
) (Record, error) {
	bundle = canonicalBundle(bundle)
	if err := bundle.Validate(); err != nil {
		return Record{}, err
	}
	if proposal.Version != ProposalVersion || len(proposal.Paths) > MaxPaths {
		return Record{}, fmt.Errorf("paved paths: unsupported or oversized proposal")
	}
	hash, err := BundleHash(bundle)
	if err != nil {
		return Record{}, err
	}
	evidence := make(map[string]Evidence, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		evidence[item.ID] = item
	}
	studies := make(map[string]struct{}, len(allowedStudyIDs))
	for _, id := range allowedStudyIDs {
		if validOpaque(id) {
			studies[id] = struct{}{}
		}
	}
	record := Record{Version: RecordVersion, BundleSHA256: hash, Bundle: bundle}
	var allowedEvidence map[string]struct{}
	if allowedEvidenceIDs != nil {
		allowedEvidence = make(map[string]struct{}, len(allowedEvidenceIDs))
		for _, id := range allowedEvidenceIDs {
			if _, exists := evidence[id]; !exists {
				return Record{}, fmt.Errorf("paved paths: editor scope references unknown evidence")
			}
			allowedEvidence[id] = struct{}{}
		}
	}
	rejectedActions := make(map[publicationActionIdentity]struct{})
	completeActions := make(map[publicationActionIdentity]struct{})
	for index, proposed := range proposal.Paths {
		if !proposedPathWithinEvidenceScope(proposed, allowedEvidence) {
			record.Issues = append(record.Issues, Issue{
				PathIndex: index,
				Code:      "evidence_outside_editor_bundle",
			})
			continue
		}
		built, code := validateProposedPath(proposed, evidence, studies)
		if code != "" {
			record.Issues = append(record.Issues, Issue{PathIndex: index, Code: code})
			continue
		}
		assessment := AssessPathPublication(built, evidence)
		if assessment.IssueCode != "" {
			record.Issues = append(record.Issues, Issue{
				PathIndex: index,
				Code:      assessment.IssueCode,
			})
			markPublicationActions(rejectedActions, built)
			continue
		}
		markPublicationActions(completeActions, built)
		record.Paths = append(record.Paths, built)
	}
	record.Paths = compressPaths(record.Paths)
	record.Landmarks = buildLandmarks(
		bundle.Evidence,
		record.Paths,
		rejectedActions,
		completeActions,
	)
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

func proposedPathWithinEvidenceScope(proposed ProposedPath, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	ids := append([]string(nil), proposed.PrerequisiteEvidenceIDs...)
	ids = append(ids, proposed.ExpectedEvidenceIDs...)
	ids = append(ids, proposed.TroubleshootingEvidenceIDs...)
	for _, action := range proposed.Actions {
		ids = append(ids, action.EvidenceID)
	}
	for _, id := range ids {
		if _, ok := allowed[id]; !ok {
			return false
		}
	}
	return true
}

func (bundle Bundle) Validate() error {
	if bundle.Version != BundleVersion || !validText(bundle.RepoName, 256, true) ||
		len(bundle.Evidence) > MaxEvidence {
		return fmt.Errorf("paved paths: invalid bundle header")
	}
	allowed := make(map[string]struct{}, len(bundle.AllowedPaths))
	for _, filePath := range bundle.AllowedPaths {
		if !validPath(filePath) {
			return fmt.Errorf("paved paths: invalid allowed path")
		}
		if _, duplicate := allowed[filePath]; duplicate {
			return fmt.Errorf("paved paths: duplicate allowed path")
		}
		allowed[filePath] = struct{}{}
	}
	seen := make(map[string]struct{}, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		if !validOpaque(item.ID) || !validEvidenceRole(item.Role) || !validPath(item.Path) ||
			item.StartLine <= 0 || item.EndLine < item.StartLine ||
			!validText(item.Label, 256, true) || len(item.Excerpt) == 0 || len(item.Excerpt) > 40 ||
			item.EndLine-item.StartLine+1 != len(item.Excerpt) {
			return fmt.Errorf("paved paths: invalid evidence %q", item.ID)
		}
		if _, ok := allowed[item.Path]; !ok {
			return fmt.Errorf("paved paths: evidence path is not allowed")
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("paved paths: duplicate evidence id")
		}
		seen[item.ID] = struct{}{}
		for index, line := range item.Excerpt {
			_, obviousSecret := secretscan.Detect(line)
			if !validText(line, 2048, false) || containsCredential(line) || obviousSecret {
				return fmt.Errorf(
					"paved paths: unsafe evidence excerpt in %q at line %d",
					item.Path,
					item.StartLine+index,
				)
			}
		}
		for _, command := range item.Commands {
			_, obviousSecret := secretscan.Detect(command.Value)
			if !validCommand(command) || containsCredential(command.Value) || obviousSecret {
				return fmt.Errorf("paved paths: invalid command evidence in %q", item.Path)
			}
			if item.Redacted && command.SafeToCopy {
				return fmt.Errorf("paved paths: redacted evidence cannot authorize copy")
			}
		}
		if item.Endpoint != "" && (!validText(item.Endpoint, 512, true) || containsCredential(item.Endpoint)) {
			return fmt.Errorf("paved paths: invalid endpoint evidence")
		}
	}
	return nil
}

func (record Record) Validate() error {
	if record.Version != RecordVersion || len(record.Paths) > MaxPaths || len(record.Landmarks) > MaxLandmarks {
		return fmt.Errorf("paved paths: invalid record header")
	}
	if err := record.Bundle.Validate(); err != nil {
		return err
	}
	hash, err := BundleHash(record.Bundle)
	if err != nil || hash != record.BundleSHA256 {
		return fmt.Errorf("paved paths: bundle hash mismatch")
	}
	evidence := make(map[string]Evidence, len(record.Bundle.Evidence))
	for _, item := range record.Bundle.Evidence {
		evidence[item.ID] = item
	}
	pathIDs := make(map[string]struct{}, len(record.Paths))
	for _, item := range record.Paths {
		if !validOpaque(item.ID) || !validText(item.Title, 256, true) || !validText(item.Goal, 1024, true) ||
			!validOrderingBasis(item.OrderingBasis) || len(item.Actions) == 0 || len(item.Actions) > MaxActions {
			return fmt.Errorf("paved paths: invalid saved path")
		}
		if _, duplicate := pathIDs[item.ID]; duplicate {
			return fmt.Errorf("paved paths: duplicate saved path")
		}
		pathIDs[item.ID] = struct{}{}
		validated, code := validateProposedPath(ProposedPath{
			Title: item.Title, Goal: item.Goal,
			PrerequisiteEvidenceIDs:    item.PrerequisiteEvidenceIDs,
			ExpectedEvidenceIDs:        item.ExpectedEvidenceIDs,
			TroubleshootingEvidenceIDs: item.TroubleshootingEvidenceIDs,
			RelatedStudyDirectionIDs:   item.RelatedStudyDirectionIDs,
			OrderingBasis:              item.OrderingBasis,
			Actions:                    proposedActions(item.Actions),
		}, evidence, stringSet(item.RelatedStudyDirectionIDs))
		if code != "" {
			return fmt.Errorf("paved paths: invalid saved path: %s", code)
		}
		if validated.OrderingBasis != item.OrderingBasis {
			return fmt.Errorf("paved paths: saved ordering basis is not locally supported")
		}
		if !slices.Equal(validated.Actions, item.Actions) ||
			!slices.Equal(validated.PrerequisiteEvidenceIDs, item.PrerequisiteEvidenceIDs) ||
			!slices.Equal(validated.ExpectedEvidenceIDs, item.ExpectedEvidenceIDs) ||
			!slices.Equal(validated.TroubleshootingEvidenceIDs, item.TroubleshootingEvidenceIDs) ||
			!slices.Equal(validated.RelatedStudyDirectionIDs, item.RelatedStudyDirectionIDs) {
			return fmt.Errorf("paved paths: saved path differs from local validation")
		}
		if item.ID != stableID("operate", item.Title, actionIdentity(item.Actions)) {
			return fmt.Errorf("paved paths: saved path id mismatch")
		}
	}
	landmarkIDs := make(map[string]struct{}, len(record.Landmarks))
	for _, item := range record.Landmarks {
		source, ok := evidence[item.EvidenceID]
		if !ok || !validOpaque(item.ID) || item.Label != source.Label || item.Role != source.Role ||
			item.ID != stableID("landmark", item.EvidenceID, item.Command, item.Endpoint) {
			return fmt.Errorf("paved paths: invalid saved landmark")
		}
		if _, duplicate := landmarkIDs[item.ID]; duplicate {
			return fmt.Errorf("paved paths: duplicate saved landmark")
		}
		landmarkIDs[item.ID] = struct{}{}
		if item.Command != "" && item.Endpoint != "" {
			return fmt.Errorf("paved paths: landmark cannot contain both command and endpoint")
		}
		if item.Command != "" {
			command, ok := commandByValue(source.Commands, item.Command)
			if !ok || item.SafeToCopy && !command.SafeToCopy {
				return fmt.Errorf("paved paths: landmark command mismatch")
			}
		} else if item.Endpoint != source.Endpoint {
			return fmt.Errorf("paved paths: landmark endpoint mismatch")
		}
	}
	return nil
}

func validateProposedPath(
	proposed ProposedPath,
	evidence map[string]Evidence,
	studies map[string]struct{},
) (Path, string) {
	if !validOperationalProse(proposed.Title, 256) || !validOperationalProse(proposed.Goal, 1024) ||
		!validOrderingBasis(proposed.OrderingBasis) || len(proposed.Actions) == 0 || len(proposed.Actions) > MaxActions {
		return Path{}, "path_shape_invalid"
	}
	prerequisites, ok := knownIDs(proposed.PrerequisiteEvidenceIDs, evidence)
	if !ok {
		return Path{}, "unknown_prerequisite_evidence"
	}
	expected, ok := knownIDs(proposed.ExpectedEvidenceIDs, evidence)
	if !ok {
		return Path{}, "unknown_expected_evidence"
	}
	troubleshooting, ok := knownIDs(proposed.TroubleshootingEvidenceIDs, evidence)
	if !ok {
		return Path{}, "unknown_troubleshooting_evidence"
	}
	related := uniqueStrings(proposed.RelatedStudyDirectionIDs)
	for _, id := range related {
		if _, exists := studies[id]; !exists {
			return Path{}, "unknown_study_direction"
		}
	}
	actions := make([]Action, 0, len(proposed.Actions))
	seen := make(map[string]struct{}, len(proposed.Actions))
	concrete := 0
	for _, proposedAction := range proposed.Actions {
		item, exists := evidence[proposedAction.EvidenceID]
		if !exists || !validOperationalInstruction(proposedAction) ||
			containsCredential(proposedAction.Instruction) {
			return Path{}, "action_evidence_invalid"
		}
		if item.Redacted && (proposedAction.Command != "" || proposedAction.Endpoint != "") {
			return Path{}, "redacted_action_not_concrete"
		}
		key := proposedAction.EvidenceID + "\x00" + proposedAction.Command + "\x00" + proposedAction.Endpoint
		if _, duplicate := seen[key]; duplicate {
			return Path{}, "duplicate_action"
		}
		seen[key] = struct{}{}
		action := Action{EvidenceID: item.ID, Instruction: strings.TrimSpace(proposedAction.Instruction)}
		// Phase 6 prompt cleanup: the model selects evidence IDs and writes
		// editorial instruction; the backend restores the exact command,
		// endpoint and safe-to-copy metadata from the evidence. A model that
		// still echoes an exact value is validated against the evidence as
		// before; an omitted value is restored deterministically.
		if proposedAction.Command != "" {
			command, exact := commandByValue(item.Commands, proposedAction.Command)
			if !exact {
				return Path{}, "command_not_in_evidence"
			}
			if !substantiveStructuralCommand(item, command) {
				return Path{}, "non_substantive_structural_command"
			}
			action.Command = command.Value
			action.SafeToCopy = command.SafeToCopy
			concrete++
		} else if command, restored := restorePrimaryCommand(item); restored {
			action.Command = command.Value
			action.SafeToCopy = command.SafeToCopy
			concrete++
		}
		if proposedAction.Endpoint != "" {
			if proposedAction.Endpoint != item.Endpoint || !publishableEndpoint(item.Endpoint) {
				return Path{}, "endpoint_not_in_evidence"
			}
			action.Endpoint = item.Endpoint
			concrete++
		} else if item.Endpoint != "" && publishableEndpoint(item.Endpoint) {
			// Backend-owned restoration: the model never needs to echo the
			// endpoint; the exact value is derived from the selected evidence.
			action.Endpoint = item.Endpoint
			concrete++
		}
		if action.Command == "" && action.Endpoint == "" && !concreteEvidenceRole(item.Role) {
			return Path{}, "generic_action"
		}
		if concreteEvidenceRole(item.Role) {
			concrete++
		}
		actions = append(actions, action)
	}
	if concrete == 0 {
		return Path{}, "concrete_operational_evidence_missing"
	}
	if len(actions) == 1 && actions[0].Command == "" && actions[0].Endpoint == "" {
		return Path{}, "single_action_not_executable"
	}
	built := Path{
		Title: strings.TrimSpace(proposed.Title), Goal: strings.TrimSpace(proposed.Goal),
		PrerequisiteEvidenceIDs: prerequisites, Actions: actions,
		ExpectedEvidenceIDs: expected, TroubleshootingEvidenceIDs: troubleshooting,
		RelatedStudyDirectionIDs: related,
		OrderingBasis:            normalizedOrderingBasis(proposed.OrderingBasis, actions, evidence),
	}
	built.ID = stableID("operate", built.Title, actionIdentity(built.Actions))
	return built, ""
}

func validOperationalInstruction(action ProposedAction) bool {
	instruction := action.Instruction
	for _, exact := range []string{action.Command, action.Endpoint} {
		if exact != "" {
			instruction = strings.ReplaceAll(instruction, exact, "the exact repository action")
		}
	}
	return validOperationalProse(instruction, 768)
}

type publicationActionIdentity struct {
	evidenceID string
	command    string
	endpoint   string
}

func markPublicationActions(seen map[publicationActionIdentity]struct{}, item Path) {
	for _, action := range item.Actions {
		seen[publicationActionIdentity{
			evidenceID: action.EvidenceID,
			command:    action.Command,
			endpoint:   action.Endpoint,
		}] = struct{}{}
	}
}

func publicationActionRejectedOnly(
	action publicationActionIdentity,
	rejected map[publicationActionIdentity]struct{},
	complete map[publicationActionIdentity]struct{},
) bool {
	if _, rejectedAction := rejected[action]; !rejectedAction {
		return false
	}
	_, completeAction := complete[action]
	return !completeAction
}

func buildLandmarks(
	evidence []Evidence,
	paths []Path,
	rejectedActions map[publicationActionIdentity]struct{},
	completeActions map[publicationActionIdentity]struct{},
) []Landmark {
	used := make(map[string]struct{})
	for _, item := range paths {
		for _, action := range item.Actions {
			used[action.EvidenceID] = struct{}{}
		}
	}
	result := make([]Landmark, 0, min(MaxLandmarks, len(evidence)))
	appendLandmark := func(item Evidence, command Command, endpoint string) {
		viewOnly := publicationActionRejectedOnly(
			publicationActionIdentity{
				evidenceID: item.ID,
				command:    command.Value,
				endpoint:   endpoint,
			},
			rejectedActions,
			completeActions,
		)
		landmark := Landmark{
			EvidenceID: item.ID, Label: item.Label, Command: command.Value,
			Endpoint: endpoint, SafeToCopy: command.SafeToCopy && !viewOnly, Role: item.Role,
		}
		landmark.ID = stableID("landmark", item.ID, landmark.Command, landmark.Endpoint)
		result = append(result, landmark)
	}
	for _, preferUnused := range []bool{true, false} {
		for _, item := range evidence {
			_, isUsed := used[item.ID]
			if preferUnused == isUsed {
				continue
			}
			if len(item.Commands) > 0 {
				if !substantiveStructuralCommand(item, item.Commands[0]) {
					continue
				}
				appendLandmark(item, item.Commands[0], "")
			} else if item.Endpoint != "" {
				if !publishableEndpoint(item.Endpoint) {
					continue
				}
				appendLandmark(item, Command{}, item.Endpoint)
			} else if concreteEvidenceRole(item.Role) {
				appendLandmark(item, Command{}, "")
			} else {
				continue
			}
			if len(result) == MaxLandmarks {
				return result
			}
		}
	}
	return result
}

// substantiveStructuralCommand keeps a syntactically valid Make target from
// becoming an operational affordance when its recipe only prints a banner.
// Targets with dependencies or a real recipe remain exact structural actions;
// help/info/version output targets are intentionally useful on their own.
func substantiveStructuralCommand(item Evidence, command Command) bool {
	if command.Basis != CommandStructural || item.Role != RoleBuildTarget ||
		!strings.HasPrefix(command.Value, "make ") {
		return true
	}
	target := strings.TrimSpace(strings.TrimPrefix(command.Value, "make "))
	switch target {
	case "help", "info", "version":
		return true
	}
	if len(item.Excerpt) == 0 {
		return false
	}
	if separator := strings.IndexByte(item.Excerpt[0], ':'); separator >= 0 {
		dependencies := strings.TrimSpace(strings.SplitN(item.Excerpt[0][separator+1:], "#", 2)[0])
		if dependencies != "" {
			return true
		}
	}
	for _, line := range item.Excerpt[1:] {
		if !strings.HasPrefix(line, "\t") {
			continue
		}
		recipe := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "@-+"))
		if recipe == "" || strings.HasPrefix(recipe, "#") {
			continue
		}
		fields := strings.Fields(recipe)
		if len(fields) == 0 {
			continue
		}
		if !slices.Contains([]string{"echo", "printf", "true", ":"}, path.Base(fields[0])) {
			return true
		}
	}
	return false
}

func publishableEndpoint(endpoint string) bool {
	return endpoint != "" && endpoint == strings.TrimSpace(endpoint) &&
		!strings.ContainsAny(endpoint, "`<>")
}

func compressPaths(paths []Path) []Path {
	result := make([]Path, 0, len(paths))
	for _, candidate := range paths {
		duplicate := false
		for _, selected := range result {
			if stringJaccard(pathEvidenceIDs(candidate), pathEvidenceIDs(selected)) >= 0.75 ||
				stringJaccard(words(candidate.Title+" "+candidate.Goal), words(selected.Title+" "+selected.Goal)) >= 0.72 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, candidate)
		}
	}
	return result
}

func canonicalBundle(bundle Bundle) Bundle {
	bundle.AllowedPaths = uniqueStrings(bundle.AllowedPaths)
	bundle.Evidence = append([]Evidence(nil), bundle.Evidence...)
	for index := range bundle.Evidence {
		bundle.Evidence[index].Commands = append([]Command(nil), bundle.Evidence[index].Commands...)
	}
	sort.Slice(bundle.Evidence, func(i, j int) bool {
		left, right := bundle.Evidence[i], bundle.Evidence[j]
		if evidenceRoleRank(left.Role) != evidenceRoleRank(right.Role) {
			return evidenceRoleRank(left.Role) < evidenceRoleRank(right.Role)
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.StartLine != right.StartLine {
			return left.StartLine < right.StartLine
		}
		return left.ID < right.ID
	})
	return bundle
}

func evidenceRoleRank(role EvidenceRole) int {
	switch role {
	case RoleBuildTarget:
		return 0
	case RoleRepositoryScript:
		return 1
	case RolePackageScript:
		return 2
	case RoleComposeService:
		return 3
	case RoleVerification:
		return 4
	case RoleEndpoint:
		return 5
	case RoleEnvironment:
		return 6
	case RoleConfiguration:
		return 7
	case RoleLogLocation:
		return 8
	case RoleDocumentedProcedure:
		return 9
	default:
		return 10
	}
}

// normalizedOrderingBasis keeps model-authored ordering explicitly
// editorial unless one repository-owned document or script proves the saved
// action sequence. It does not claim that the commands succeed.
func normalizedOrderingBasis(
	proposed OrderingBasis,
	actions []Action,
	evidence map[string]Evidence,
) OrderingBasis {
	if proposed == OrderingEditorial || len(actions) == 0 {
		return OrderingEditorial
	}
	first := evidence[actions[0].EvidenceID]
	for index, action := range actions {
		item := evidence[action.EvidenceID]
		if item.Path != first.Path {
			return OrderingEditorial
		}
		switch proposed {
		case OrderingDocumented:
			if item.Role != RoleDocumentedProcedure && item.Role != RoleVerification {
				return OrderingEditorial
			}
		case OrderingScript:
			if item.Role != RoleRepositoryScript && !item.Executable {
				return OrderingEditorial
			}
		default:
			return OrderingEditorial
		}
		if index > 0 && operationalActionPosition(actions[index-1], evidence) >
			operationalActionPosition(action, evidence) {
			return OrderingEditorial
		}
	}
	return proposed
}

// AssessPathPublication applies the fail-closed operating-path publication
// contract and derives the exact typed results consumed by report projection.
// Record decoding deliberately does not call this helper: historical records
// remain replayable, while new construction and public replay share one
// deterministic assessment.
func AssessPathPublication(saved Path, evidence map[string]Evidence) PublicationAssessment {
	results := derivePublicationResults(saved, evidence)
	if !publicationPrerequisitesClosed(saved, evidence) {
		return PublicationAssessment{
			IssueCode: PublicationIssueMissingPrerequisite,
			Results:   results,
		}
	}
	if !publicationActionSequenceClosed(saved, evidence) {
		return PublicationAssessment{
			IssueCode: PublicationIssueMissingActions,
			Results:   results,
		}
	}
	for _, result := range results {
		if result.AfterAction == len(saved.Actions) {
			return PublicationAssessment{Results: results}
		}
	}
	return PublicationAssessment{
		IssueCode: PublicationIssueMissingResult,
		Results:   results,
	}
}

func publicationPrerequisitesClosed(saved Path, evidence map[string]Evidence) bool {
	if len(saved.PrerequisiteEvidenceIDs) == 0 || len(saved.Actions) == 0 {
		return false
	}
	actionIDs := make(map[string]struct{}, len(saved.Actions))
	for _, action := range saved.Actions {
		if _, ok := evidence[action.EvidenceID]; !ok {
			return false
		}
		actionIDs[action.EvidenceID] = struct{}{}
	}
	for _, id := range saved.PrerequisiteEvidenceIDs {
		item, ok := evidence[id]
		if !ok || item.Redacted || !publicationPrerequisiteRole(item.Role) ||
			!explicitPrerequisiteEvidence(item) {
			return false
		}
		if _, reused := actionIDs[id]; reused {
			return false
		}
		sameProcedure := false
		for _, action := range saved.Actions {
			actionEvidence := evidence[action.EvidenceID]
			if publicationEvidenceDuplicatesAction(item, actionEvidence, action) {
				return false
			}
			if publicationPrerequisiteMatchesAction(item, actionEvidence, action) {
				sameProcedure = true
				break
			}
		}
		if !sameProcedure {
			return false
		}
	}
	return true
}

func publicationEvidenceDuplicatesAction(
	prerequisite Evidence,
	actionEvidence Evidence,
	action Action,
) bool {
	if action.Command != "" {
		for _, command := range prerequisite.Commands {
			if command.Value == action.Command {
				return true
			}
		}
	}
	return prerequisite.Path == actionEvidence.Path &&
		prerequisite.StartLine == actionEvidence.StartLine &&
		prerequisite.EndLine == actionEvidence.EndLine &&
		slices.Equal(prerequisite.Excerpt, actionEvidence.Excerpt)
}

func publicationPrerequisiteRole(role EvidenceRole) bool {
	switch role {
	case RoleDocumentedProcedure, RoleBuildTarget, RolePackageScript,
		RoleRepositoryScript, RoleComposeService, RoleEnvironment,
		RoleConfiguration:
		return true
	default:
		return false
	}
}

func explicitPrerequisiteEvidence(item Evidence) bool {
	text := " " + strings.ToLower(strings.Join(item.Excerpt, "\n")) + " "
	if publicationAbsentPrerequisiteRE.MatchString(text) {
		return false
	}
	for _, negation := range []string{
		" no prerequisite", " no additional prerequisite", " no requirements",
		" no additional requirements", " no setup ", " no installation ",
		" no configuration ", " not required", " none required",
		" prerequisites: none", " requirements: none", " without prerequisites",
		" without requirements", " requires no ", " does not require",
		" do not need", " don't need", " need not ", " nothing required",
	} {
		if strings.Contains(text, negation) {
			return false
		}
	}
	for _, marker := range []string{
		" prerequisite:", " prerequisites:", " requirement:", " requirements:",
		" requires ", " required ",
		" must ", " need ", " needs ", " before ", "first install",
		"first, install", "install first", "first set up", "first, set up",
		"first configure", "first, configure",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// restorePrimaryCommand restores the single substantive structural command of
// an evidence item deterministically (Phase 6 prompt cleanup: the model never
// needs to echo exact command bytes; the backend owns command identity).
// Returns false when the item carries no unique substantive command.
func restorePrimaryCommand(item Evidence) (Command, bool) {
	var primary *Command
	for index := range item.Commands {
		command := item.Commands[index]
		if !substantiveStructuralCommand(item, command) {
			continue
		}
		if primary != nil {
			// Ambiguous: more than one substantive command — leave
			// restoration to the caller's explicit echo path.
			return Command{}, false
		}
		commandCopy := command
		primary = &commandCopy
	}
	if primary == nil {
		return Command{}, false
	}
	return *primary, true
}

func publicationActionSequenceClosed(saved Path, evidence map[string]Evidence) bool {
	if len(saved.Actions) == 0 || saved.OrderingBasis == OrderingEditorial ||
		normalizedOrderingBasis(saved.OrderingBasis, saved.Actions, evidence) != saved.OrderingBasis {
		return false
	}
	first, ok := evidence[saved.Actions[0].EvidenceID]
	if !ok || first.Redacted {
		return false
	}
	previousPosition := -1
	for _, action := range saved.Actions {
		item, exists := evidence[action.EvidenceID]
		if !exists || item.Redacted || item.ID != first.ID ||
			(action.Command == "") == (action.Endpoint == "") {
			return false
		}
		position := operationalActionPosition(action, evidence)
		if previousPosition >= 0 && position != previousPosition+1 {
			return false
		}
		previousPosition = position
	}
	return true
}

func publicationPrerequisiteMatchesAction(
	prerequisite Evidence,
	actionEvidence Evidence,
	action Action,
) bool {
	if prerequisite.Path == "" || prerequisite.Path != actionEvidence.Path {
		return false
	}
	if prerequisite.StartLine > actionEvidence.EndLine ||
		actionEvidence.StartLine > prerequisite.EndLine {
		return false
	}
	prerequisiteWords := words(
		prerequisite.Label + "\n" + strings.Join(prerequisite.Excerpt, "\n"),
	)
	actionWords := words(action.Command + "\n" + action.Endpoint)
	for _, word := range prerequisiteWords {
		if slices.Contains(actionWords, word) {
			return true
		}
	}
	return false
}

type publicationResultCandidate struct {
	kind        PublicationResultKind
	value       string
	startOffset int
	endOffset   int
}

func derivePublicationResults(
	saved Path,
	evidence map[string]Evidence,
) []PublicationResult {
	results := []PublicationResult{}
	for actionIndex, action := range saved.Actions {
		if strings.TrimSpace(action.Command) == "" {
			continue
		}
		item, ok := evidence[action.EvidenceID]
		if !ok || item.Redacted {
			continue
		}
		candidates := publicationResultCandidates(action, item)
		seen := make(map[string]struct{}, len(candidates))
		for _, candidate := range candidates {
			key := string(candidate.kind) + "\x00" + candidate.value
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			results = append(results, PublicationResult{
				Kind: candidate.kind, Value: candidate.value, AfterAction: actionIndex + 1,
				EvidenceID: item.ID, StartOffset: candidate.startOffset,
				EndOffset: candidate.endOffset,
			})
		}
	}
	return results
}

func publicationResultCandidates(
	action Action,
	item Evidence,
) []publicationResultCandidate {
	result := []publicationResultCandidate{}
	if value, startOffset, endOffset, ok := publicationDocumentedCommandOutput(
		action.Command,
		item,
	); ok {
		result = append(result, publicationResultCandidate{
			kind: PublicationResultCommandOutput, value: value,
			startOffset: startOffset, endOffset: endOffset,
		})
	}
	for _, value := range publicationOutputFlagValues(action.Command, publicationOutputFlagRE) {
		offset, ok := publicationOutputFlagLine(item.Excerpt, value, publicationOutputFlagRE)
		if !ok {
			continue
		}
		result = append(result, publicationResultCandidate{
			kind: PublicationResultGeneratedArtifact, value: value,
			startOffset: offset, endOffset: offset,
		})
	}

	isMakeTarget := item.Role == RoleBuildTarget &&
		strings.HasPrefix(strings.TrimSpace(action.Command), "make ")
	if !isMakeTarget {
		return result
	}
	for offset, line := range item.Excerpt {
		if !strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, value := range publicationMakeCoverProfileValues(line) {
			result = append(result, publicationResultCandidate{
				kind: PublicationResultGeneratedArtifact, value: value,
				startOffset: offset, endOffset: offset,
			})
		}
	}
	return result
}

func publicationDocumentedCommandOutput(
	command string,
	item Evidence,
) (string, int, int, bool) {
	if item.Role != RoleDocumentedProcedure {
		return "", 0, 0, false
	}
	prompt := "$ " + strings.TrimSpace(command)
	for index, line := range item.Excerpt {
		if strings.TrimSpace(line) != prompt {
			continue
		}
		start := index + 1
		end := len(item.Excerpt) - 1
		for offset := start; offset < len(item.Excerpt); offset++ {
			trimmed := strings.TrimSpace(item.Excerpt[offset])
			isNextPrompt := strings.HasPrefix(trimmed, "$ ") ||
				strings.HasPrefix(trimmed, "% ") || strings.HasPrefix(trimmed, "> ")
			if isNextPrompt {
				end = offset - 1
				break
			}
		}
		for start <= end && strings.TrimSpace(item.Excerpt[start]) == "" {
			start++
		}
		for end >= start && strings.TrimSpace(item.Excerpt[end]) == "" {
			end--
		}
		if start > end {
			continue
		}
		value := dedentPublicationOutput(item.Excerpt[start : end+1])
		if validPublicationResultValue(value) {
			return value, start, end, true
		}
	}
	return "", 0, 0, false
}

func dedentPublicationOutput(lines []string) string {
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		current := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent < 0 || current < indent {
			indent = current
		}
	}
	if indent <= 0 {
		return strings.Join(lines, "\n")
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) >= indent {
			line = line[indent:]
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func publicationOutputFlagValues(command string, expression *regexp.Regexp) []string {
	values := []string{}
	for _, match := range expression.FindAllStringSubmatch(command, -1) {
		for index := 1; index < len(match); index++ {
			if validPublicationArtifactPath(match[index]) {
				values = append(values, match[index])
				break
			}
		}
	}
	return values
}

func publicationMakeCoverProfileValues(line string) []string {
	const marker = "-coverprofile="
	values := []string{}
	remaining := line
	for {
		index := strings.Index(remaining, marker)
		if index < 0 {
			return values
		}
		remaining = remaining[index+len(marker):]
		value := strings.Fields(remaining)
		if len(value) == 0 {
			return values
		}
		candidate := strings.Trim(value[0], `"'`)
		if validPublicationArtifactPath(candidate) {
			values = append(values, candidate)
		}
		remaining = remaining[len(value[0]):]
	}
}

func publicationOutputFlagLine(
	lines []string,
	value string,
	expression *regexp.Regexp,
) (int, bool) {
	for index, line := range lines {
		for _, candidate := range publicationOutputFlagValues(line, expression) {
			if candidate == value {
				return index, true
			}
		}
	}
	return 0, false
}

func validPublicationArtifactPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 || strings.HasPrefix(value, "-") ||
		strings.ContainsAny(value, "\r\n\x00`$|;&<>{}()") {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func validPublicationResultValue(value string) bool {
	if strings.TrimSpace(value) == "" || len(value) > maxPublicationResultBytes {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) && char != '\t' && char != '\n' {
			return false
		}
	}
	return true
}

func operationalActionPosition(action Action, evidence map[string]Evidence) int {
	item := evidence[action.EvidenceID]
	position := item.StartLine * (MaxActions + 1)
	if action.Command == "" {
		return position
	}
	for index, command := range item.Commands {
		if command.Value == action.Command {
			return position + index
		}
	}
	return position
}

func commandByValue(commands []Command, value string) (Command, bool) {
	for _, command := range commands {
		if command.Value == value {
			return command, true
		}
	}
	return Command{}, false
}

func proposedActions(actions []Action) []ProposedAction {
	result := make([]ProposedAction, 0, len(actions))
	for _, action := range actions {
		result = append(result, ProposedAction{
			EvidenceID: action.EvidenceID, Instruction: action.Instruction,
			Command: action.Command, Endpoint: action.Endpoint,
		})
	}
	return result
}

func knownIDs(values []string, known map[string]Evidence) ([]string, bool) {
	values = uniqueStrings(values)
	for _, value := range values {
		if _, ok := known[value]; !ok {
			return nil, false
		}
	}
	return values, true
}

func concreteEvidenceRole(role EvidenceRole) bool {
	switch role {
	case RoleBuildTarget, RolePackageScript, RoleRepositoryScript, RoleComposeService,
		RoleEnvironment, RoleConfiguration, RoleEndpoint, RoleLogLocation, RoleVerification:
		return true
	default:
		return false
	}
}

func validEvidenceRole(role EvidenceRole) bool {
	switch role {
	case RoleDocumentedProcedure, RoleBuildTarget, RolePackageScript, RoleRepositoryScript,
		RoleComposeService, RoleEnvironment, RoleConfiguration, RoleEndpoint,
		RoleLogLocation, RoleVerification:
		return true
	default:
		return false
	}
}

func validOrderingBasis(value OrderingBasis) bool {
	return value == OrderingDocumented || value == OrderingScript || value == OrderingEditorial
}

func validCommand(command Command) bool {
	if !validText(command.Value, 2048, true) || strings.ContainsAny(command.Value, "\x00\r") {
		return false
	}
	return command.Basis == CommandExact || command.Basis == CommandStructural
}

func containsCredential(value string) bool {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "begin private key") || strings.Contains(lower, "authorization: bearer ") {
		return true
	}
	for _, marker := range []string{"password=", "password:", "api_key=", "apikey=", "secret=", "token="} {
		if index := strings.Index(lower, marker); index >= 0 {
			remainder := strings.TrimSpace(value[index+len(marker):])
			if remainder != "" && !strings.HasPrefix(remainder, "<redacted>") &&
				remainder != "${" && !strings.HasPrefix(remainder, "$") {
				return true
			}
		}
	}
	return false
}

func validOpaque(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return false
	}
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune("-_.:/", char) {
			continue
		}
		return false
	}
	return true
}

func validPath(value string) bool {
	return value != "" && value == path.Clean(value) && !path.IsAbs(value) &&
		value != "." && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}

func validText(value string, limit int, required bool) bool {
	value = strings.TrimSpace(value)
	return (!required || value != "") && len(value) <= limit && !strings.ContainsAny(value, "\x00\r")
}

// validOperationalProse prevents the model from smuggling an unvalidated
// command or endpoint through presentation copy. Exact executable values have
// dedicated, locally checked fields.
func validOperationalProse(value string, limit int) bool {
	if !validText(value, limit, true) || strings.ContainsAny(value, "\n`;$|<>") ||
		strings.Contains(value, "$(") || strings.Contains(value, "&&") ||
		strings.Contains(value, "://") || containsCommandSequence(value) {
		return false
	}
	return true
}

func containsCommandSequence(value string) bool {
	fields := strings.Fields(strings.ToLower(value))
	for index := 0; index < len(fields); index++ {
		first := strings.Trim(fields[index], "()[]{}.,:!?\"'")
		if strings.HasPrefix(first, "-") {
			return true
		}
		if strings.HasPrefix(first, "./") || strings.HasPrefix(first, "../") {
			return true
		}
		if index+1 >= len(fields) {
			continue
		}
		next := strings.Trim(fields[index+1], "()[]{}.,:!?\"'")
		switch path.Base(first) {
		case "rm", "sudo", "kubectl", "terraform":
			return true
		case "go":
			if slices.Contains([]string{"build", "test", "run", "vet", "install", "generate", "env", "list"}, next) {
				return true
			}
		case "make", "task", "just", "curl", "wget":
			return true
		case "npm", "pnpm", "yarn", "bun":
			if slices.Contains([]string{"run", "test", "build", "install", "start"}, next) {
				return true
			}
		case "docker":
			if slices.Contains([]string{"compose", "build", "run", "exec"}, next) {
				return true
			}
		case "bash", "sh", "zsh", "python", "python3", "node":
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func pathEvidenceIDs(item Path) []string {
	result := append([]string(nil), item.PrerequisiteEvidenceIDs...)
	for _, action := range item.Actions {
		result = append(result, action.EvidenceID)
	}
	result = append(result, item.ExpectedEvidenceIDs...)
	return uniqueStrings(result)
}

func actionIdentity(actions []Action) string {
	parts := make([]string, 0, len(actions)*3)
	for _, action := range actions {
		parts = append(parts, action.EvidenceID, action.Command, action.Endpoint)
	}
	return strings.Join(parts, "\x00")
}

func words(value string) []string {
	set := make(map[string]struct{})
	for _, word := range strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char)
	}) {
		if len(word) >= 4 {
			set[word] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for word := range set {
		result = append(result, word)
	}
	sort.Strings(result)
	return result
}

func stringJaccard(left, right []string) float64 {
	left = uniqueStrings(left)
	right = uniqueStrings(right)
	intersection := 0
	for _, value := range left {
		if slices.Contains(right, value) {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func stableID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "-" + hex.EncodeToString(digest[:8])
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("paved paths: trailing JSON")
		}
		return fmt.Errorf("paved paths: invalid trailing JSON: %w", err)
	}
	return nil
}
