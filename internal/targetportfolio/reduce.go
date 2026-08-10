package targetportfolio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/secretscan"
)

// ResolveResponse validates the complete refs-only decision atomically and
// restores exact catalog entries from private authority. There is no repair,
// retry, fuzzy ref matching, or response-order ranking.
func ResolveResponse(compilation Compilation, raw []byte) (Selection, error) {
	if err := validateCompilation(compilation); err != nil {
		return Selection{}, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Selection{}, fmt.Errorf("target portfolio: response exceeds bounded envelope")
	}
	if _, found := secretscan.DetectAlways(string(raw)); found {
		return Selection{}, fmt.Errorf("target portfolio: response contains credential-shaped content")
	}
	var response Response
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return Selection{}, fmt.Errorf("target portfolio: invalid JSON response")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Selection{}, err
	}
	if response.Version != ResultVersion || response.RequestRef != compilation.Request.RequestRef {
		return Selection{}, fmt.Errorf("target portfolio: response identity mismatch")
	}
	if len(response.TargetRefs) == 0 {
		return Selection{}, fmt.Errorf("target portfolio: response selected no targets")
	}
	defaultTarget, known := compilation.authority[response.DefaultRef]
	if !known {
		return Selection{}, fmt.Errorf("target portfolio: response cites unknown default ref")
	}
	if !EligibleForSelection(defaultTarget) {
		return Selection{}, fmt.Errorf("target portfolio: response selects a library target with no advertised public API")
	}

	selected := make(map[string]struct{}, len(response.TargetRefs))
	for _, ref := range response.TargetRefs {
		entry, known := compilation.authority[ref]
		if !known {
			return Selection{}, fmt.Errorf("target portfolio: response cites unknown target ref")
		}
		if !EligibleForSelection(entry) {
			return Selection{}, fmt.Errorf("target portfolio: response selects a library target with no advertised public API")
		}
		if _, duplicate := selected[ref]; duplicate {
			return Selection{}, fmt.Errorf("target portfolio: response contains duplicate target ref")
		}
		selected[ref] = struct{}{}
	}
	if _, included := selected[response.DefaultRef]; !included {
		return Selection{}, fmt.Errorf("target portfolio: default ref is absent from target refs")
	}

	result := Selection{
		Version: ResultVersion, CatalogRef: compilation.CatalogRef,
		RequestRef: compilation.Request.RequestRef, RequestSHA256: compilation.RequestSHA256,
		Default: snapshotEntry(compilation.authority[response.DefaultRef]),
		Targets: make([]analysistarget.TargetCatalogEntry, 0, len(selected)),
	}
	for _, target := range compilation.Request.Targets {
		if _, keep := selected[target.Ref]; keep {
			result.Targets = append(result.Targets, snapshotEntry(compilation.authority[target.Ref]))
		}
	}
	return result, nil
}

// AdvertisedForSelection is the exact ordinary portfolio candidate-surface
// rule. Every executable is visible. A library is visible only when its exact
// package directory equals its exact module directory. The complete catalog is
// deliberately broader for explicit --target and --all-targets inclusion.
func AdvertisedForSelection(entry analysistarget.TargetCatalogEntry) bool {
	target := entry.Candidate.Target
	return target.Kind == analysistarget.KindExecutablePackage ||
		(target.Kind == analysistarget.KindLibraryPackage && target.PackageDir == target.ModuleDir)
}

// EligibleForSelection applies the private ordinary-selection rule used by
// both model-result restoration and the local fallback. Empty-API libraries
// and non-root library packages stay in the complete catalog but cannot become
// ordinary product report scopes.
func EligibleForSelection(entry analysistarget.TargetCatalogEntry) bool {
	return AdvertisedForSelection(entry) &&
		(entry.Candidate.Target.Kind != analysistarget.KindLibraryPackage || len(entry.Symbols) > 0)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("target portfolio: trailing JSON value")
		}
		return fmt.Errorf("target portfolio: invalid trailing response data")
	}
	return nil
}
