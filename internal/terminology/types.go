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
