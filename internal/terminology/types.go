// Package terminology collects source-backed terminology alongside accepted
// analysis responses. It does not change the owning analysis cube's schema.
package terminology

import (
	"slices"
	"sort"
)

type Source struct {
	Path string `json:"path"`
	// Zero honestly denotes a whole-file source, not an invented line.
	Line int `json:"line"`
}

// Origin identifies the accepted response row that actually mentions a term.
// An empty Row denotes prose outside a keyed result row.
type Origin struct {
	RequestSHA256 string `json:"request_sha256"`
	Row           string `json:"row"`
}

type Candidate struct {
	Name        string   `json:"name"`
	Explanation string   `json:"explanation"`
	Sources     []Source `json:"sources"`
	Origins     []Origin `json:"origins"`
}

// TermKind names the concept kinds the prompt asks the model to choose. Every
// kind names a concept; a machine name is not a glossary kind. Self-runs spent
// most generated terms on an "identifier" kind that code then discarded. The
// kind is neither stored nor shown, so the decoder does not gate on it except
// to drop a term that declares itself the retired identifier kind.
type TermKind string

const (
	KindAcronym  TermKind = "acronym"
	KindDomain   TermKind = "domain"
	KindProtocol TermKind = "protocol"
	KindFormat   TermKind = "format"

	retiredIdentifierKind = "identifier"
)

// CodeNameKind says which existing code observation spells a name exactly.
// A generated term with that exact spelling is journaled, never published.
type CodeNameKind string

const (
	CodeDeclaration    CodeNameKind = "declaration"
	CodePackage        CodeNameKind = "package"
	CodeFile           CodeNameKind = "file"
	CodeEnvironmentKey CodeNameKind = "environment key"
)

// codeNameRank keeps one deterministic kind when several observations share
// a spelling; the drop itself does not depend on which kind is reported.
func codeNameRank(kind CodeNameKind) int {
	return slices.Index([]CodeNameKind{CodeDeclaration, CodePackage, CodeFile, CodeEnvironmentKey}, kind)
}

func normalizeSources(sources []Source) []Source {
	result := append([]Source(nil), sources...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path || result[i].Path == result[j].Path && result[i].Line < result[j].Line
	})
	return slices.Compact(result)
}

func normalizeOrigins(origins []Origin) []Origin {
	result := append([]Origin(nil), origins...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].RequestSHA256 != result[j].RequestSHA256 {
			return result[i].RequestSHA256 < result[j].RequestSHA256
		}
		return result[i].Row < result[j].Row
	})
	return slices.Compact(result)
}
