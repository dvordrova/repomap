// Package runtimeportfolio synthesizes one repository-level inventory of
// runnable roles and reusable library products from validated target-local
// evidence. Models select only request-local refs; exact program target
// identities and source locations are restored locally.
package runtimeportfolio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Version          = 3
	ArtifactFilename = "runtime-portfolio.json"

	// MaxRequestBytes bounds the cube-owned user JSON for a legacy single call
	// and for every exhaustive map or reduce shard. Keeping this below the
	// provider's observed multi-megabyte context rejection point leaves room
	// for the OpenAI-compatible envelope and JSON escaping. The complete
	// canonical input has no independent entity-count ceiling and may span a
	// sequence of whole-target shards within the cube's total semantic-call
	// resource bound; no target or evidence row is truncated.
	MaxRequestBytes         = 1 * 1024 * 1024
	MaxProviderRequestBytes = 2*MaxRequestBytes + 64*1024
	MaxResponseBytes        = 2 * 1024 * 1024
	MaxOutputTokens         = 64_000
	MaxTextBytes            = 512
	MaxArtifactBytes        = 4 << 20
)

const executionContract = "repository-runtime-portfolio-v4"

type Prominence string

const (
	ProminencePrimary    Prominence = "primary"
	ProminenceSupporting Prominence = "supporting"
	ProminenceUnknown    Prominence = "unknown"
)

type RoleKind string

const (
	RoleKindService        RoleKind = "service"
	RoleKindDaemon         RoleKind = "daemon"
	RoleKindWorker         RoleKind = "worker"
	RoleKindCLI            RoleKind = "cli"
	RoleKindLibrary        RoleKind = "library"
	RoleKindExample        RoleKind = "example"
	RoleKindSupportingTool RoleKind = "supporting_tool"
	RoleKindUnknown        RoleKind = "unknown"
)

type Requiredness string

const (
	RequirednessRequired     Requiredness = "required"
	RequirednessOptional     Requiredness = "optional"
	RequirednessExperimental Requiredness = "experimental"
	RequirednessUnknown      Requiredness = "unknown"
)

type Confidence string

const (
	ConfidenceHigh    Confidence = "high"
	ConfidenceMedium  Confidence = "medium"
	ConfidenceLow     Confidence = "low"
	ConfidenceUnknown Confidence = "unknown"
)

type MappingStatus string

const (
	MappingMapped  MappingStatus = "mapped"
	MappingUnknown MappingStatus = "unknown"
)

type EvidenceKind string

const (
	EvidenceRepositoryGuidance EvidenceKind = "repository_guidance"
	EvidenceTargetEntrypoint   EvidenceKind = "target_entrypoint"
	EvidenceResponsibility     EvidenceKind = "responsibility"
	EvidenceActivityStart      EvidenceKind = "activity_start"
	EvidenceIntegration        EvidenceKind = "integration"
	EvidenceConfiguration      EvidenceKind = "configuration"
	EvidenceDeployment         EvidenceKind = "deployment"
	EvidenceProgramFact        EvidenceKind = "program_fact"
)

// Location is an exact repository-relative source location. Line and column
// are zero only when the producer has file-level evidence.
type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// EvidenceInput is validated upstream evidence used to compile the closed
// provider catalog. ProgramTargetID is local authority and never crosses the
// provider boundary.
type EvidenceInput struct {
	Kind            EvidenceKind
	Label           string
	Location        Location
	ProgramTargetID string
}

// TargetInput is one exact completed target page. Semantic summaries are
// prior validated cube results; counts keep large exact inventories visible
// without turning every operation into a new portfolio-role candidate.
type TargetInput struct {
	ProgramTargetID  string
	DisplayName      string
	Language         string
	Kind             string
	Selector         string
	Default          bool
	ProgramObjects   int
	ProgramRelations int
	ActivityStarts   int
	IntegrationUses  int
	Responsibilities []ResponsibilityInput
	Evidence         []EvidenceInput
}

type ResponsibilityInput struct {
	Name     string
	Purpose  string
	Evidence []EvidenceInput
}

// Input is the complete request-local authority for one repository synthesis.
type Input struct {
	RepositoryName            string
	CapturedRevision          string
	TargetPagePortfolioSHA256 string
	Targets                   []TargetInput
	RepositoryEvidence        []EvidenceInput
}

type Target struct {
	ProgramTargetID string `json:"program_target_id"`
	DisplayName     string `json:"display_name"`
	Language        string `json:"language"`
	Kind            string `json:"kind"`
	Selector        string `json:"selector"`
	Default         bool   `json:"default"`
}

type Evidence struct {
	ID              string       `json:"id"`
	Kind            EvidenceKind `json:"kind"`
	Label           string       `json:"label"`
	Location        Location     `json:"location"`
	ProgramTargetID string       `json:"program_target_id,omitempty"`
}

type Implementation struct {
	ProgramTargetID string `json:"program_target_id"`
	Mode            string `json:"mode,omitempty"`
}

type Role struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Purpose         string           `json:"purpose"`
	Prominence      Prominence       `json:"prominence"`
	Kind            RoleKind         `json:"role_kind"`
	Requiredness    Requiredness     `json:"requiredness"`
	Confidence      Confidence       `json:"confidence"`
	MappingStatus   MappingStatus    `json:"mapping_status"`
	Implementations []Implementation `json:"implementations"`
	Evidence        []Evidence       `json:"evidence"`
}

type Coverage struct {
	TargetsObserved     int `json:"targets_observed"`
	TargetsMapped       int `json:"targets_mapped"`
	TargetsUnclassified int `json:"targets_unclassified"`
	Roles               int `json:"roles"`
	EvidenceAdvertised  int `json:"evidence_advertised"`
	EvidenceSelected    int `json:"evidence_selected"`
}

// Result is the canonical semantic artifact. UnclassifiedTargetIDs is the
// exact local complement of target IDs selected by at least one mapped role.
type Result struct {
	Version                   int      `json:"version"`
	TargetPagePortfolioSHA256 string   `json:"target_page_portfolio_sha256"`
	Targets                   []Target `json:"targets"`
	Roles                     []Role   `json:"roles"`
	UnclassifiedTargetIDs     []string `json:"unclassified_target_ids"`
	Coverage                  Coverage `json:"coverage"`
}

func (result Result) Validate() error {
	if result.Version != Version || !validSHA256(result.TargetPagePortfolioSHA256) ||
		result.Targets == nil || len(result.Targets) == 0 || result.Roles == nil ||
		result.UnclassifiedTargetIDs == nil {
		return fmt.Errorf("runtime portfolio: invalid result identity")
	}
	targets := make(map[string]Target, len(result.Targets))
	defaultCount := 0
	for index, target := range result.Targets {
		if !validText(target.ProgramTargetID) || !validText(target.DisplayName) ||
			!validText(target.Language) || !validText(target.Kind) || !validText(target.Selector) {
			return fmt.Errorf("runtime portfolio: target %d is invalid", index)
		}
		if _, duplicate := targets[target.ProgramTargetID]; duplicate {
			return fmt.Errorf("runtime portfolio: duplicate target identity")
		}
		if index > 0 && result.Targets[index-1].ProgramTargetID >= target.ProgramTargetID {
			return fmt.Errorf("runtime portfolio: targets are not canonical")
		}
		if target.Default {
			defaultCount++
		}
		targets[target.ProgramTargetID] = target
	}
	if defaultCount != 1 {
		return fmt.Errorf("runtime portfolio: exactly one target must be default")
	}
	mapped := make(map[string]struct{})
	selectedEvidence := make(map[string]struct{})
	roleNames := make(map[string]struct{}, len(result.Roles))
	previousRoleKey := ""
	for index, role := range result.Roles {
		if err := validateRole(role, targets); err != nil {
			return fmt.Errorf("runtime portfolio: role %d: %w", index, err)
		}
		nameKey := strings.ToLower(role.Name)
		if _, duplicate := roleNames[nameKey]; duplicate {
			return fmt.Errorf("runtime portfolio: duplicate role name")
		}
		roleNames[nameKey] = struct{}{}
		key := roleSortKey(role)
		if previousRoleKey != "" && previousRoleKey >= key {
			return fmt.Errorf("runtime portfolio: roles are not canonical")
		}
		previousRoleKey = key
		for _, implementation := range role.Implementations {
			mapped[implementation.ProgramTargetID] = struct{}{}
		}
		for _, evidence := range role.Evidence {
			selectedEvidence[evidence.ID] = struct{}{}
		}
	}
	wantUnclassified := make([]string, 0, len(targets)-len(mapped))
	for targetID := range targets {
		if _, ok := mapped[targetID]; !ok {
			wantUnclassified = append(wantUnclassified, targetID)
		}
	}
	sort.Strings(wantUnclassified)
	if !equalStrings(result.UnclassifiedTargetIDs, wantUnclassified) {
		return fmt.Errorf("runtime portfolio: unclassified target complement mismatch")
	}
	wantCoverage := Coverage{
		TargetsObserved: len(targets), TargetsMapped: len(mapped),
		TargetsUnclassified: len(wantUnclassified), Roles: len(result.Roles),
		EvidenceAdvertised: result.Coverage.EvidenceAdvertised,
		EvidenceSelected:   len(selectedEvidence),
	}
	if result.Coverage != wantCoverage || result.Coverage.EvidenceAdvertised < result.Coverage.EvidenceSelected {
		return fmt.Errorf("runtime portfolio: coverage mismatch")
	}
	return nil
}

func validateRole(role Role, targets map[string]Target) error {
	if !validText(role.Name) || !validText(role.Purpose) || !validProminence(role.Prominence) ||
		!validRoleKind(role.Kind) || !validRequiredness(role.Requiredness) ||
		!validConfidence(role.Confidence) || !validMappingStatus(role.MappingStatus) ||
		role.Implementations == nil || role.Evidence == nil || len(role.Evidence) == 0 {
		return fmt.Errorf("invalid role contract")
	}
	if role.MappingStatus == MappingMapped && len(role.Implementations) == 0 {
		return fmt.Errorf("mapped role has no implementation")
	}
	if role.MappingStatus == MappingUnknown && len(role.Implementations) != 0 {
		return fmt.Errorf("unknown mapping cites an implementation")
	}
	if (role.Kind == RoleKindExample || role.Kind == RoleKindSupportingTool) &&
		role.Prominence != ProminenceSupporting {
		return fmt.Errorf("example or supporting-tool role is not supporting")
	}
	previousImplementation := ""
	for _, implementation := range role.Implementations {
		if _, known := targets[implementation.ProgramTargetID]; !known ||
			(implementation.Mode != "" && !validText(implementation.Mode)) {
			return fmt.Errorf("implementation is outside target authority")
		}
		key := implementation.ProgramTargetID + "\x00" + implementation.Mode
		if previousImplementation != "" && previousImplementation >= key {
			return fmt.Errorf("implementations are not canonical")
		}
		previousImplementation = key
	}
	previousEvidence := ""
	evidencedTargetIDs := make(map[string]struct{}, len(role.Evidence))
	libraryEvidenceTargetIDs := make(map[string]struct{}, len(role.Evidence))
	for _, evidence := range role.Evidence {
		if err := validateEvidence(evidence, targets); err != nil {
			return err
		}
		if previousEvidence != "" && previousEvidence >= evidence.ID {
			return fmt.Errorf("evidence is not canonical")
		}
		previousEvidence = evidence.ID
		if evidence.ProgramTargetID != "" {
			evidencedTargetIDs[evidence.ProgramTargetID] = struct{}{}
			if evidence.Kind == EvidenceResponsibility || evidence.Kind == EvidenceProgramFact {
				libraryEvidenceTargetIDs[evidence.ProgramTargetID] = struct{}{}
			}
		}
	}
	for _, implementation := range role.Implementations {
		if _, supported := evidencedTargetIDs[implementation.ProgramTargetID]; !supported {
			return fmt.Errorf("implementation has no target-bound role evidence")
		}
		if role.Kind == RoleKindLibrary {
			if implementation.Mode != "" {
				return fmt.Errorf("library implementation has an executable mode")
			}
			if _, supported := libraryEvidenceTargetIDs[implementation.ProgramTargetID]; !supported {
				return fmt.Errorf("library implementation has no exact responsibility or program-fact evidence")
			}
		}
	}
	wantID, err := roleID(role)
	if err != nil || role.ID != wantID {
		return fmt.Errorf("role identity mismatch")
	}
	return nil
}

func validateEvidence(value Evidence, targets map[string]Target) error {
	if !validEvidenceKind(value.Kind) || !validText(value.Label) || !validLocation(value.Location) {
		return fmt.Errorf("invalid role evidence")
	}
	if value.ProgramTargetID != "" {
		if _, known := targets[value.ProgramTargetID]; !known {
			return fmt.Errorf("role evidence target is unknown")
		}
	}
	want, err := evidenceID(value)
	if err != nil || value.ID != want {
		return fmt.Errorf("role evidence identity mismatch")
	}
	return nil
}

func (result Result) CanonicalJSON() ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (result Result) ArtifactSHA256() (string, error) {
	raw, err := Encode(result)
	if err != nil {
		return "", err
	}
	return sha256Hex(raw), nil
}

func Encode(result Result) ([]byte, error) {
	raw, err := result.CanonicalJSON()
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return nil, fmt.Errorf("runtime portfolio: indent artifact: %w", err)
	}
	if err := pretty.WriteByte('\n'); err != nil {
		return nil, fmt.Errorf("runtime portfolio: finish artifact: %w", err)
	}
	encoded := pretty.Bytes()
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf(
			"runtime portfolio: artifact is %d bytes, limit is %d", len(encoded), MaxArtifactBytes,
		)
	}
	return append([]byte(nil), encoded...), nil
}

func Decode(raw []byte) (Result, error) {
	if len(raw) == 0 || len(raw) > MaxArtifactBytes {
		return Result{}, fmt.Errorf("runtime portfolio: invalid artifact size")
	}
	var result Result
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("runtime portfolio: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("runtime portfolio: trailing artifact data")
		}
		return Result{}, fmt.Errorf("runtime portfolio: invalid trailing artifact data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return Result{}, fmt.Errorf("runtime portfolio: artifact is not canonical")
	}
	return result, nil
}

func roleID(role Role) (string, error) {
	copyRole := role
	copyRole.ID = ""
	raw, err := json.Marshal(copyRole)
	if err != nil {
		return "", err
	}
	return "runtime-role-" + shortSHA(raw), nil
}

func evidenceID(value Evidence) (string, error) {
	copyEvidence := value
	copyEvidence.ID = ""
	raw, err := json.Marshal(copyEvidence)
	if err != nil {
		return "", err
	}
	return "runtime-evidence-" + shortSHA(raw), nil
}

func roleSortKey(role Role) string {
	return prominenceOrder(role.Prominence) + "\x00" + strings.ToLower(role.Name) + "\x00" + role.ID
}

func prominenceOrder(value Prominence) string {
	switch value {
	case ProminencePrimary:
		return "1"
	case ProminenceSupporting:
		return "2"
	default:
		return "3"
	}
}

func validProminence(value Prominence) bool {
	return value == ProminencePrimary || value == ProminenceSupporting || value == ProminenceUnknown
}

func validRoleKind(value RoleKind) bool {
	switch value {
	case RoleKindService, RoleKindDaemon, RoleKindWorker, RoleKindCLI, RoleKindLibrary, RoleKindExample,
		RoleKindSupportingTool, RoleKindUnknown:
		return true
	default:
		return false
	}
}

func validRequiredness(value Requiredness) bool {
	switch value {
	case RequirednessRequired, RequirednessOptional, RequirednessExperimental, RequirednessUnknown:
		return true
	default:
		return false
	}
}

func validConfidence(value Confidence) bool {
	return value == ConfidenceHigh || value == ConfidenceMedium || value == ConfidenceLow || value == ConfidenceUnknown
}

func validMappingStatus(value MappingStatus) bool {
	return value == MappingMapped || value == MappingUnknown
}

func validEvidenceKind(value EvidenceKind) bool {
	switch value {
	case EvidenceRepositoryGuidance, EvidenceTargetEntrypoint, EvidenceResponsibility,
		EvidenceActivityStart, EvidenceIntegration, EvidenceConfiguration,
		EvidenceDeployment, EvidenceProgramFact:
		return true
	default:
		return false
	}
}

func validText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || len(value) > MaxTextBytes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validLocation(value Location) bool {
	if value.Line < 0 || value.Column < 0 || (value.Line == 0 && value.Column != 0) ||
		value.Path == "" || value.Path != path.Clean(value.Path) || path.IsAbs(value.Path) ||
		value.Path == "." || value.Path == ".." || strings.HasPrefix(value.Path, "../") ||
		strings.Contains(value.Path, "\\") {
		return false
	}
	return validText(value.Path)
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func validRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func shortSHA(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:12])
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
