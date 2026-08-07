package themestudy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// SourceReader reads a bounded inclusive line range [startLine,endLine] of the
// authorized file at path. It is the only file-access boundary this package
// uses, so tests can inject synthetic source without a real repository.
type SourceReader func(path string, startLine, endLine int) ([]string, error)

// TotalLines reports the total line count of an authorized file, enabling the
// builder to mark explicit omitted ranges and Partial flags. When nil, bounded
// windows are treated as full body without omitted ranges.
type TotalLines func(path string) (int, error)

// SeedPackResult is the compiled a* seed-pack catalog.
type SeedPackResult struct {
	Packs      []SeedPack `json:"packs"`
	Omissions  []Omission `json:"omissions,omitempty"`
	TotalBytes int        `json:"total_bytes"`
}

func linesBytes(lines []string) int {
	n := 0
	for _, line := range lines {
		n += len(line)
	}
	return n
}

func contentSHA(lines []string) string {
	hash := sha256.New()
	for _, line := range lines {
		_, _ = hash.Write([]byte(line))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// shortSHA256 returns the first 12 hex chars of the SHA-256 of the exact
// text — the contract identity for prompts whose content is the contract
// (owner directive 2026-08-07: short prompt SHA instead of a hand-bumped
// version).
func shortSHA256(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])[:12]
}

// buildObject reads one bounded source object (full-body-or-partial) starting
// at line start.
func buildObject(reader SourceReader, totalLines TotalLines, role SourceRole, seed SeedSpec, startLine, maxLines, maxBytes int) (SourceObject, error) {
	if startLine <= 0 {
		return SourceObject{}, fmt.Errorf("seed %s: non-positive source line %d", seed.Ref, startLine)
	}
	end := startLine + maxLines - 1
	lines, err := reader(seed.Path, startLine, end)
	if err != nil {
		return SourceObject{}, fmt.Errorf("seed %s: read %s:%d: %w", seed.Ref, seed.Path, startLine, err)
	}
	if len(lines) == 0 {
		return SourceObject{}, fmt.Errorf("seed %s: empty source at %s:%d", seed.Ref, seed.Path, startLine)
	}
	var omitted []LineRange
	if totalLines != nil {
		fileLen, err := totalLines(seed.Path)
		if err == nil && end < fileLen {
			omitted = []LineRange{{StartLine: end + 1, EndLine: fileLen}}
		}
	}
	symbol := seed.Symbol
	if role == SourceRoleCaller {
		symbol = seed.CallerSymbol
	} else if role == SourceRoleCallee {
		symbol = seed.CalleeSymbol
	}
	fullBody := len(omitted) == 0 && linesBytes(lines) <= maxBytes && len(lines) <= maxLines
	return SourceObject{
		Role:          role,
		Path:          seed.Path,
		Line:          startLine,
		Symbol:        symbol,
		Provenance:    seed.Provenance,
		FullBody:      fullBody,
		Partial:       len(omitted) > 0,
		Lines:         lines,
		Omitted:       omitted,
		ContentSHA256: contentSHA(lines),
	}, nil
}

// callsiteObject reads exactly the representative callsite line, kept as a
// separate SourceObject from the caller and callee declarations (directive B,
// M5).
func callsiteObject(reader SourceReader, seed SeedSpec) (SourceObject, error) {
	if seed.CallLine <= 0 {
		return SourceObject{}, fmt.Errorf("seed %s: system_path seed missing call_line", seed.Ref)
	}
	lines, err := reader(seed.Path, seed.CallLine, seed.CallLine)
	if err != nil {
		return SourceObject{}, fmt.Errorf("seed %s: read callsite: %w", seed.Ref, err)
	}
	if len(lines) != 1 {
		return SourceObject{}, fmt.Errorf("seed %s: callsite is not a single line", seed.Ref)
	}
	return SourceObject{
		Role:          SourceRoleCallsite,
		Path:          seed.Path,
		Line:          seed.CallLine,
		Symbol:        seed.CalleeSymbol,
		Provenance:    seed.Provenance,
		FullBody:      true,
		Lines:         lines,
		ContentSHA256: contentSHA(lines),
	}, nil
}

// BuildSeedPacks compiles the bounded a* source packs (contract B). Seeds come
// from the compiled local substrate only. Every system-path seed emits exactly
// three role-tagged SourceObjects (caller declaration, representative callsite,
// callee declaration); a focused seed emits exactly one declaration. Source
// bytes are provider evidence only. maxAnchors/maxBytes are producer-owned
// bounds; seeds beyond the source-byte budget are reported under the closed
// seed_budget omission, never silently dropped.
func BuildSeedPacks(seeds []SeedSpec, maxAnchors, maxBytes, maxObjectLines, maxObjectBytes int, reader SourceReader, totalLines TotalLines) (SeedPackResult, error) {
	if maxAnchors <= 0 {
		maxAnchors = MaxSeedAnchors
	}
	if maxBytes <= 0 {
		maxBytes = MaxSeedSourceBytes
	}
	if maxObjectLines <= 0 {
		maxObjectLines = MaxSourceObjectLines
	}
	if maxObjectBytes <= 0 {
		maxObjectBytes = MaxSourceObjectBytes
	}
	if reader == nil {
		return SeedPackResult{}, fmt.Errorf("seed pack builder requires a source reader")
	}
	result := SeedPackResult{}
	acc := 0
	seen := make(map[string]struct{}, len(seeds))
	var budgetOmitted []string
	for _, seed := range seeds {
		if _, ok := seen[seed.Ref]; ok {
			continue
		}
		seen[seed.Ref] = struct{}{}
		if len(result.Packs) >= maxAnchors {
			budgetOmitted = append(budgetOmitted, seed.Ref)
			continue
		}
		pack := SeedPack{Seed: seed}
		var err error
		if seed.Kind == "system_path" {
			caller, err1 := buildObject(reader, totalLines, SourceRoleCaller, seed, seed.CallerLine, maxObjectLines, maxObjectBytes)
			if err1 != nil {
				return SeedPackResult{}, err1
			}
			callsite, err2 := callsiteObject(reader, seed)
			if err2 != nil {
				return SeedPackResult{}, err2
			}
			callee, err3 := buildObject(reader, totalLines, SourceRoleCallee, seed, seed.CalleeLine, maxObjectLines, maxObjectBytes)
			if err3 != nil {
				return SeedPackResult{}, err3
			}
			pack.Objects = []SourceObject{caller, callsite, callee}
		} else {
			var decl SourceObject
			decl, err = buildObject(reader, totalLines, SourceRoleDeclaration, seed, seed.Line, maxObjectLines, maxObjectBytes)
			if err != nil {
				return SeedPackResult{}, err
			}
			pack.Objects = []SourceObject{decl}
		}
		pack.TotalBytes = 0
		for _, object := range pack.Objects {
			pack.TotalBytes += linesBytes(object.Lines)
		}
		if len(result.Packs) > 0 && acc+pack.TotalBytes > maxBytes {
			budgetOmitted = append(budgetOmitted, seed.Ref)
			continue
		}
		result.Packs = append(result.Packs, pack)
		acc += pack.TotalBytes
	}
	if len(budgetOmitted) > 0 {
		result.Omissions = append(result.Omissions, Omission{Reason: "seed_budget", Count: len(budgetOmitted), Representatives: representativeRefs(budgetOmitted)})
	}
	result.TotalBytes = acc
	return result, nil
}
