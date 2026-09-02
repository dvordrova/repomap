// Package claims owns the claims layer of a repomap run: statements quoted
// from human-written artifacts (README files, docstrings, comments, commit
// messages). A claim is never verified code behavior; it always carries its
// source path and, when known, its date and age so the reader can judge
// staleness.
package claims

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	Version          = 1
	ArtifactFilename = "claims.json"

	digestDomain = "repomap-claims-v1\x00"
	idDomain     = "repomap-claim-id-v1\x00"
	idHexWidth   = 16

	// MaxTextRunes bounds one quoted claim; longer sources are split by the
	// extractor, never truncated silently.
	MaxTextRunes = 600
)

// Source is the closed vocabulary of claim origins.
type Source string

const (
	SourceReadme    Source = "readme"
	SourceDocstring Source = "docstring"
	SourceComment   Source = "comment"
	SourceCommit    Source = "commit"
)

func (source Source) Valid() bool {
	switch source {
	case SourceReadme, SourceDocstring, SourceComment, SourceCommit:
		return true
	default:
		return false
	}
}

// Claim is one quoted statement. Path/Line locate it for file sources; Commit
// identifies it for commit messages. Date is an ISO-8601 date (YYYY-MM-DD)
// and AgeDays is measured from the captured revision's commit date.
type Claim struct {
	ID       string `json:"id"`
	Source   Source `json:"source"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Commit   string `json:"commit,omitempty"`
	Text     string `json:"text"`
	Date     string `json:"date,omitempty"`
	AgeDays  int    `json:"age_days,omitempty"`
	TargetID string `json:"target_id,omitempty"`
}

// Result is the sealed claims artifact. Dropped counts quotes the extractor
// withheld because their text matched a credential shape; the count is kept
// so a reader can tell "nothing quoted" from "quotes withheld".
type Result struct {
	Version  int     `json:"version"`
	Revision string  `json:"revision"`
	AsOf     string  `json:"as_of,omitempty"`
	Claims   []Claim `json:"claims"`
	Dropped  int     `json:"dropped,omitempty"`
	SHA256   string  `json:"sha256"`
}

// NewClaimID derives a stable id from the source, its location and the text.
func NewClaimID(source Source, location string, text string) string {
	hasher := sha256.New()
	hasher.Write([]byte(idDomain))
	for _, part := range []string{string(source), location, strings.TrimSpace(text)} {
		hasher.Write([]byte(part))
		hasher.Write([]byte{0})
	}
	return "c-" + hex.EncodeToString(hasher.Sum(nil))[:idHexWidth]
}

// Seal sorts the rows canonically, checks the shape, and computes the digest.
func Seal(result Result) (Result, error) {
	owned := clone(result)
	owned.Version = Version
	if owned.Claims == nil {
		owned.Claims = []Claim{}
	}
	sort.SliceStable(owned.Claims, func(i, j int) bool { return claimLess(owned.Claims[i], owned.Claims[j]) })
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

// Validate checks the closed shape, canonical order, and the seal.
func (result Result) Validate() error {
	if result.Version != Version {
		return fmt.Errorf("claims: unsupported version %d", result.Version)
	}
	if result.Claims == nil {
		return fmt.Errorf("claims: collection is missing")
	}
	if result.AsOf != "" && !validDate(result.AsOf) {
		return fmt.Errorf("claims: invalid as_of date %q", result.AsOf)
	}
	if result.Dropped < 0 {
		return fmt.Errorf("claims: negative dropped count")
	}
	ids := make(map[string]struct{}, len(result.Claims))
	for position, claim := range result.Claims {
		if err := claim.validate(); err != nil {
			return fmt.Errorf("claims: claim %d: %w", position, err)
		}
		if _, duplicate := ids[claim.ID]; duplicate {
			return fmt.Errorf("claims: duplicate id %q", claim.ID)
		}
		ids[claim.ID] = struct{}{}
		if position > 0 && !claimLess(result.Claims[position-1], claim) {
			return fmt.Errorf("claims: rows are not canonical at %d", position)
		}
	}
	digest, err := resultDigest(result)
	if err != nil {
		return err
	}
	if digest != result.SHA256 {
		return fmt.Errorf("claims: digest mismatch")
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

// ByID indexes claims by id.
func (result Result) ByID() map[string]Claim {
	index := make(map[string]Claim, len(result.Claims))
	for _, claim := range result.Claims {
		index[claim.ID] = claim
	}
	return index
}

func (claim Claim) validate() error {
	if !strings.HasPrefix(claim.ID, "c-") || len(claim.ID) < 2+idHexWidth {
		return fmt.Errorf("invalid id %q", claim.ID)
	}
	if !claim.Source.Valid() {
		return fmt.Errorf("invalid source %q", claim.Source)
	}
	if !validText(claim.Text) || utf8.RuneCountInString(claim.Text) > MaxTextRunes {
		return fmt.Errorf("invalid text")
	}
	switch claim.Source {
	case SourceCommit:
		if claim.Commit == "" || claim.Path != "" || claim.Line != 0 {
			return fmt.Errorf("commit claim requires a commit and no path")
		}
	default:
		if claim.Path == "" || claim.Commit != "" {
			return fmt.Errorf("%s claim requires a path and no commit", claim.Source)
		}
		if err := validateRepositoryPath(claim.Path); err != nil {
			return err
		}
	}
	if claim.Line < 0 || claim.AgeDays < 0 {
		return fmt.Errorf("negative position or age")
	}
	if claim.Date != "" && !validDate(claim.Date) {
		return fmt.Errorf("invalid date %q", claim.Date)
	}
	if claim.Date == "" && claim.AgeDays != 0 {
		return fmt.Errorf("age without date")
	}
	if claim.TargetID != "" && !validText(claim.TargetID) {
		return fmt.Errorf("invalid target id")
	}
	return nil
}

func claimLess(a, b Claim) bool {
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	if a.Date != b.Date {
		return a.Date > b.Date
	}
	return a.ID < b.ID
}

func resultDigest(result Result) (string, error) {
	unsealed := clone(result)
	unsealed.SHA256 = ""
	encoded, err := json.Marshal(unsealed)
	if err != nil {
		return "", fmt.Errorf("claims: digest: %w", err)
	}
	hasher := sha256.New()
	hasher.Write([]byte(digestDomain))
	hasher.Write(encoded)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// clone copies the rows and preserves the difference between an empty result
// and a missing one.
func clone(result Result) Result {
	owned := result
	if result.Claims != nil {
		owned.Claims = append(make([]Claim, 0, len(result.Claims)), result.Claims...)
	}
	return owned
}

func validDate(value string) bool {
	if len(value) != 10 || value[4] != '-' || value[7] != '-' {
		return false
	}
	for i, r := range value {
		if i == 4 || i == 7 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	return !strings.ContainsRune(value, 0)
}

func validateRepositoryPath(value string) error {
	if value == "" || !utf8.ValidString(value) {
		return fmt.Errorf("claims: empty path")
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value {
		return fmt.Errorf("claims: path %q is not canonical repository-relative", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("claims: path %q has an invalid segment", value)
		}
	}
	return nil
}
