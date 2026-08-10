// Package targetportfolio builds and reduces one compact refs-only model
// choice over an exact AnalysisTarget catalog. It does not call a provider,
// choose an explicit --target override, or implement --all-targets policy.
package targetportfolio

import "github.com/dvordrova/repomap/internal/analysistarget"

const (
	CompilationVersion = 4
	RequestVersion     = 4
	ResultVersion      = 3

	// The selector carries the complete exact ordinary candidate surface plus
	// declaration labels, so its request has a dedicated 256 KiB envelope. The
	// complete private catalog may be broader. The refs-only response remains
	// bounded to 64 KiB. There is no target-count or symbol-count cap and no
	// prefix truncation within the advertised surface.
	MaxRequestBytes = 256 << 10
	// The semantic JSON is embedded as a JSON message string in the final
	// OpenAI-compatible body. Reserve the exact two-times escaping bound plus
	// 64 KiB for the fixed prompt and provider envelope.
	MaxProviderRequestBytes = 2*MaxRequestBytes + 64<<10
	MaxResponseBytes        = 64 << 10
)

type TargetKind string

const (
	TargetExecutable TargetKind = "executable"
	TargetLibrary    TargetKind = "library"
)

// Target is one provider-visible request-local option. DisplayPath is an
// exact flat repository-relative display label, never canonical identity.
type Target struct {
	Ref         string           `json:"ref"`
	DisplayPath string           `json:"display_path"`
	Kind        TargetKind       `json:"kind"`
	Packages    []PackageSymbols `json:"packages"`
}

// PackageSymbols keeps one exact package's declaration labels together. Its
// DisplayPath is repository-relative presentation evidence, not canonical Go
// import identity. Declaration locations and source never cross this wire.
type PackageSymbols struct {
	DisplayPath string        `json:"display_path"`
	Symbols     []SymbolGroup `json:"symbols"`
}

// SymbolGroup keeps exact declaration labels readable and compact. Methods
// use receiver-qualified names such as Server.Start.
type SymbolGroup struct {
	Kind  string   `json:"kind"`
	Names []string `json:"names"`
}

// Request is the complete provider-visible facts bundle. RequestRef binds the
// exact bundle and private catalog so a response cannot be replayed against a
// different t* mapping.
type Request struct {
	Version    int      `json:"version"`
	RequestRef string   `json:"request_ref"`
	RepoName   string   `json:"repo_name"`
	Targets    []Target `json:"targets"`
}

// Compilation owns the exact private restoration table. Only Request is
// permitted to cross the provider boundary.
type Compilation struct {
	Version       int     `json:"version"`
	CatalogRef    string  `json:"catalog_ref"`
	Request       Request `json:"request"`
	RequestSHA256 string  `json:"request_sha256"`

	wire      string
	catalog   analysistarget.TargetCatalog
	authority map[string]analysistarget.TargetCatalogEntry
	sealed    string
}

type Prompt struct {
	Version string
	System  string
	User    string
}

// Response is deliberately refs-only. TargetRefs is an unordered selected
// set; DefaultRef carries the one intentional preference.
type Response struct {
	Version    int      `json:"version"`
	RequestRef string   `json:"request_ref"`
	DefaultRef string   `json:"default_ref"`
	TargetRefs []string `json:"target_refs"`
}

// Selection restores exact backend-owned catalog entries. Targets are in the
// canonical request order, never provider response order.
type Selection struct {
	Version       int
	CatalogRef    string
	RequestRef    string
	RequestSHA256 string
	Default       analysistarget.TargetCatalogEntry
	Targets       []analysistarget.TargetCatalogEntry
}
