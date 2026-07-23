// Package goldenmechanism extracts one bounded, syntax-only Go mechanism
// artifact from exact repository-local seed functions. It does not load Go
// packages, build SSA or a call graph, or claim that syntax is executed at
// runtime.
package goldenmechanism

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const Version = 1

const (
	minSeeds = 1
	maxSeeds = 6

	hardMaxDepth         = 3
	hardMaxFiles         = 6
	hardMaxFunctions     = 24
	hardMaxParsedBytes   = 2 * 1024 * 1024
	hardMaxSourceBytes   = 1024 * 1024
	hardMaxFunctionLines = 300
	hardMaxFunctionBytes = 64 * 1024
	hardMaxTimeout       = 5 * time.Second
)

var mechanismIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,127}$`)

// Plan identifies one mechanism and the exact functions from which its
// bounded local probe may start. ExpansionAllowlist is optional. When set,
// local direct calls are still recorded, but only listed symbols may be added
// to the function frontier.
type Plan struct {
	MechanismID        string   `json:"mechanism_id"`
	Seeds              []Seed   `json:"seeds"`
	ExpansionAllowlist []string `json:"expansion_allowlist,omitempty"`
	Limits             Limits   `json:"limits"`
}

// Seed binds an exact repository symbol lookup to an existing source/evidence
// anchor in the same file. The anchor authorizes the bounded file; declaration
// resolution by the probe, not the old fact prose, establishes the symbol.
type Seed struct {
	OriginFactID     string `json:"origin_fact_id"`
	OriginEvidenceID string `json:"origin_evidence_id"`
	Path             string `json:"path"`
	Symbol           string `json:"symbol"`
	// Depth and ReachedFromEvidenceID are assigned only by a bounded local
	// named-frontier planner. Their zero values preserve the original seed
	// representation used by saved probes.
	Depth                 int    `json:"depth,omitempty"`
	ReachedFromEvidenceID string `json:"reached_from_evidence_id,omitempty"`
}

// Limits are deliberately small and have hard ceilings. MaxParsedSourceBytes
// bounds the exact seed files read by go/parser; MaxSourceBytes independently
// bounds retained function windows exposed by the probe artifact.
type Limits struct {
	MaxDepth             int           `json:"max_depth"`
	MaxFiles             int           `json:"max_files"`
	MaxFunctions         int           `json:"max_functions"`
	MaxParsedSourceBytes int           `json:"max_parsed_source_bytes"`
	MaxSourceBytes       int           `json:"max_source_bytes"`
	MaxFunctionLines     int           `json:"max_function_lines"`
	MaxFunctionBytes     int           `json:"max_function_bytes"`
	Timeout              time.Duration `json:"timeout"`
}

type StopReason string

const (
	StopFileLimit         StopReason = "file_limit"
	StopSourceByteLimit   StopReason = "source_byte_limit"
	StopFunctionLimit     StopReason = "function_limit"
	StopDepthLimit        StopReason = "depth_limit"
	StopFunctionLineLimit StopReason = "function_line_limit"
	StopFunctionByteLimit StopReason = "function_byte_limit"
	StopTimeout           StopReason = "timeout"
)

type SeedStatus string

const (
	SeedResolved             SeedStatus = "resolved"
	SeedSkippedFileLimit     SeedStatus = "skipped_file_limit"
	SeedSkippedByteLimit     SeedStatus = "skipped_source_byte_limit"
	SeedSkippedFunctionLimit SeedStatus = "skipped_function_limit"
	SeedSkippedTimeout       SeedStatus = "skipped_timeout"
)

type SyntaxBasis string

const (
	BasisDeclaration  SyntaxBasis = "go_ast_declaration"
	BasisDirectCall   SyntaxBasis = "go_ast_direct_call"
	BasisBranch       SyntaxBasis = "go_ast_branch"
	BasisRead         SyntaxBasis = "go_ast_read"
	BasisAssignment   SyntaxBasis = "go_ast_assignment"
	BasisTransform    SyntaxBasis = "go_ast_transformation"
	BasisOutput       SyntaxBasis = "go_ast_output"
	BasisReturn       SyntaxBasis = "go_ast_error_return"
	BasisErrorHandoff SyntaxBasis = "go_ast_local_error_handoff"
	BasisLexicalOrder SyntaxBasis = "go_ast_lexical_order"
)

// Result is a locally verifiable artifact. Statements describe syntax only;
// runtime execution and cross-file dynamic dispatch remain outside its scope.
type Result struct {
	Version      int              `json:"version"`
	MechanismID  string           `json:"mechanism_id"`
	Seeds        []SeedResolution `json:"seeds"`
	Files        []File           `json:"files"`
	Functions    []Function       `json:"functions"`
	Observations []Observation    `json:"observations"`
	Budget       BudgetStats      `json:"budget"`
	Partial      bool             `json:"partial"`
	StopReason   StopReason       `json:"stop_reason,omitempty"`
}

type SeedResolution struct {
	Seed       Seed       `json:"seed"`
	Status     SeedStatus `json:"status"`
	FunctionID string     `json:"function_id,omitempty"`
}

type File struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Bytes   int    `json:"bytes"`
	Package string `json:"package"`
}

type Function struct {
	ID                     string            `json:"id"`
	Symbol                 string            `json:"symbol"`
	Path                   string            `json:"path"`
	Location               evidence.Location `json:"location"`
	Seed                   bool              `json:"seed"`
	Depth                  int               `json:"depth"`
	OriginFactIDs          []string          `json:"origin_fact_ids,omitempty"`
	OriginEvidenceIDs      []string          `json:"origin_evidence_ids,omitempty"`
	ReachedFromIDs         []string          `json:"reached_from_ids,omitempty"`
	PlannedFromEvidenceIDs []string          `json:"planned_from_evidence_ids,omitempty"`
	Source                 []SourceLine      `json:"source"`
	SourceTruncated        bool              `json:"source_truncated"`
	SourceStopReason       string            `json:"source_stop_reason"`
}

type SourceLine struct {
	ID        string            `json:"id"`
	Location  evidence.Location `json:"location"`
	Text      string            `json:"text"`
	Truncated bool              `json:"truncated,omitempty"`
}

// Observation is one deterministic syntax fact. TargetFunctionID may name an
// indexed local declaration that was not expanded because of an allowlist or
// budget; RelatedObservationIDs are used only by lexical-order observations.
type Observation struct {
	ID                    string                       `json:"id"`
	Capability            semanticdiscovery.Capability `json:"capability"`
	FunctionID            string                       `json:"function_id"`
	Operation             string                       `json:"operation"`
	Statement             string                       `json:"statement"`
	Subject               string                       `json:"subject,omitempty"`
	Object                string                       `json:"object,omitempty"`
	TargetFunctionID      string                       `json:"target_function_id,omitempty"`
	TargetSymbol          string                       `json:"target_symbol,omitempty"`
	RelatedObservationIDs []string                     `json:"related_observation_ids,omitempty"`
	Basis                 SyntaxBasis                  `json:"basis"`
	Evidence              []EvidenceRef                `json:"evidence"`
}

type EvidenceRef struct {
	ID       string            `json:"id"`
	Location evidence.Location `json:"location"`
}

type BudgetStats struct {
	SeedCount           int   `json:"seed_count"`
	ResolvedSeedCount   int   `json:"resolved_seed_count"`
	FilesParsed         int   `json:"files_parsed"`
	ParsedSourceBytes   int   `json:"parsed_source_bytes"`
	FunctionsIncluded   int   `json:"functions_included"`
	IncludedSourceBytes int   `json:"included_source_bytes"`
	Observations        int   `json:"observations"`
	MaxDepthReached     int   `json:"max_depth_reached"`
	ElapsedMillis       int64 `json:"elapsed_millis"`
}

func (plan Plan) normalized() (Plan, error) {
	plan.MechanismID = strings.TrimSpace(plan.MechanismID)
	if !mechanismIDPattern.MatchString(plan.MechanismID) {
		return Plan{}, fmt.Errorf("golden mechanism: invalid mechanism id %q", plan.MechanismID)
	}
	if len(plan.Seeds) < minSeeds || len(plan.Seeds) > maxSeeds {
		return Plan{}, fmt.Errorf("golden mechanism: seeds must contain %d to %d entries", minSeeds, maxSeeds)
	}
	seenSeeds := make(map[string]struct{}, len(plan.Seeds))
	for index := range plan.Seeds {
		seed := &plan.Seeds[index]
		seed.Path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(seed.Path))))
		seed.Symbol = canonicalSymbol(seed.Symbol)
		if strings.TrimSpace(seed.OriginFactID) == "" || strings.TrimSpace(seed.OriginEvidenceID) == "" {
			return Plan{}, fmt.Errorf("golden mechanism: seed[%d] origin ids are required", index)
		}
		if !validGoPath(seed.Path) {
			return Plan{}, fmt.Errorf("golden mechanism: seed[%d] has invalid repository-relative Go path %q", index, seed.Path)
		}
		if seed.Symbol == "" {
			return Plan{}, fmt.Errorf("golden mechanism: seed[%d] symbol is required", index)
		}
		if seed.Depth < 0 || seed.Depth > hardMaxDepth {
			return Plan{}, fmt.Errorf(
				"golden mechanism: seed[%d] depth must be between 0 and %d",
				index,
				hardMaxDepth,
			)
		}
		if seed.Depth == 0 && seed.ReachedFromEvidenceID != "" {
			return Plan{}, fmt.Errorf(
				"golden mechanism: seed[%d] root cannot have reached-from evidence",
				index,
			)
		}
		if seed.Depth > 0 && strings.TrimSpace(seed.ReachedFromEvidenceID) == "" {
			return Plan{}, fmt.Errorf(
				"golden mechanism: seed[%d] frontier requires reached-from evidence",
				index,
			)
		}
		key := seed.Path + "\x00" + seed.Symbol
		if _, exists := seenSeeds[key]; exists {
			return Plan{}, fmt.Errorf("golden mechanism: duplicate seed %s in %s", seed.Symbol, seed.Path)
		}
		seenSeeds[key] = struct{}{}
	}

	seenAllowed := make(map[string]struct{}, len(plan.ExpansionAllowlist))
	allowed := make([]string, 0, len(plan.ExpansionAllowlist))
	for index, symbol := range plan.ExpansionAllowlist {
		symbol = canonicalSymbol(symbol)
		if symbol == "" {
			return Plan{}, fmt.Errorf("golden mechanism: expansion_allowlist[%d] is empty", index)
		}
		if _, exists := seenAllowed[symbol]; exists {
			continue
		}
		seenAllowed[symbol] = struct{}{}
		allowed = append(allowed, symbol)
	}
	sort.Strings(allowed)
	plan.ExpansionAllowlist = allowed

	limits, err := plan.Limits.normalized()
	if err != nil {
		return Plan{}, err
	}
	plan.Limits = limits
	for index, seed := range plan.Seeds {
		if seed.Depth > limits.MaxDepth {
			return Plan{}, fmt.Errorf(
				"golden mechanism: seed[%d] depth %d exceeds max_depth %d",
				index,
				seed.Depth,
				limits.MaxDepth,
			)
		}
	}
	return plan, nil
}

func (limits Limits) normalized() (Limits, error) {
	if limits.MaxDepth == 0 {
		limits.MaxDepth = 2
	}
	if limits.MaxFiles == 0 {
		limits.MaxFiles = 4
	}
	if limits.MaxFunctions == 0 {
		limits.MaxFunctions = 12
	}
	if limits.MaxParsedSourceBytes == 0 {
		limits.MaxParsedSourceBytes = 256 * 1024
	}
	if limits.MaxSourceBytes == 0 {
		limits.MaxSourceBytes = 256 * 1024
	}
	if limits.MaxFunctionLines == 0 {
		limits.MaxFunctionLines = 220
	}
	if limits.MaxFunctionBytes == 0 {
		limits.MaxFunctionBytes = 48 * 1024
	}
	if limits.Timeout == 0 {
		limits.Timeout = 2 * time.Second
	}
	checks := []struct {
		name  string
		value int
		max   int
	}{
		{"max_depth", limits.MaxDepth, hardMaxDepth},
		{"max_files", limits.MaxFiles, hardMaxFiles},
		{"max_functions", limits.MaxFunctions, hardMaxFunctions},
		{"max_parsed_source_bytes", limits.MaxParsedSourceBytes, hardMaxParsedBytes},
		{"max_source_bytes", limits.MaxSourceBytes, hardMaxSourceBytes},
		{"max_function_lines", limits.MaxFunctionLines, hardMaxFunctionLines},
		{"max_function_bytes", limits.MaxFunctionBytes, hardMaxFunctionBytes},
	}
	for _, check := range checks {
		if check.value <= 0 || check.value > check.max {
			return Limits{}, fmt.Errorf("golden mechanism: %s must be between 1 and %d", check.name, check.max)
		}
	}
	if limits.Timeout < 0 || limits.Timeout > hardMaxTimeout {
		return Limits{}, fmt.Errorf("golden mechanism: timeout must be positive and at most %s", hardMaxTimeout)
	}
	return limits, nil
}

func validGoPath(value string) bool {
	if value == "" || filepath.IsAbs(value) || !filepath.IsLocal(filepath.FromSlash(value)) {
		return false
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	return cleaned != "." && strings.EqualFold(filepath.Ext(cleaned), ".go")
}

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "-" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
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
