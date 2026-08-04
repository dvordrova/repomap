package themestudy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

const vocabularyVersion = "v1"

// refName maps a 1-based index to a typed short ref: f1..fN, a1..aN, t1..tN.
func refName(prefix string, index int) string {
	return fmt.Sprintf("%s%d", prefix, index)
}

// languageForPath derives an extension-based language id.
func languageForPath(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".ts", ".tsx", ".jsx":
		return "javascript"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".sh", ".bash":
		return "shell"
	case ".sql":
		return "sql"
	case ".yml", ".yaml", ".toml":
		return "config"
	case ".json":
		return "json"
	default:
		return "text"
	}
}

// FileRole classifies a tracked path into the closed production/test/doc role.
func FileRole(filePath string) Role {
	lower := strings.ToLower(filePath)
	if strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, "_test.py") ||
		strings.HasSuffix(lower, "_test.js") || strings.Contains(lower, "/test/") {
		return RoleTest
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".markdown", ".rst", ".txt", ".adoc":
		return RoleDocumentation
	default:
		return RoleProductionSource
	}
}

// candidateDigest hashes the complete canonical candidate set (repo-relative
// paths joined in sorted order) so the full considered set stays bound by a
// stable digest even when only part of it is advertised.
func candidateDigest(paths []string) string {
	hash := sha256.New()
	for _, p := range paths {
		_, _ = hash.Write([]byte(p))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// candidatelist describes the complete canonical candidate set of a layer.
type candidatelist struct {
	Paths  []string `json:"-"`
	Digest string   `json:"digest"`
	Count  int      `json:"count"`
}

// dedupeSorted returns the deduplicated, canonical-ordered path list.
func dedupeSorted(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// fileRefItemSize returns the JSON size of one FileRef item.
func fileRefItemSize(item FileRef) int {
	data, err := json.Marshal(item)
	if err != nil {
		return 64
	}
	return len(data)
}

// BuildFileVocabulary produces the flat names-only f* layer (contract A).
// paths is the eligible tracked inventory; advertisable, when non-nil, marks
// which of those may be advertised (OpenablePaths ∩ eligible). maxBytes is the
// dedicated bounded request budget; 0 selects MaxFileVocabularyBytes.
//
// The layer is complete when the whole canonical list fits the budget;
// otherwise it records exact considered/advertised counts, a stable digest over
// the complete candidate set, and closed coverage-aware omission aggregates
// (eligible_not_advertisable, vocabulary_budget) that exactly partition
// considered - advertised. It never silently becomes first-N.
func BuildFileVocabulary(paths []string, maxBytes int, advertisable func(path string) bool) Vocabulary {
	if maxBytes <= 0 {
		maxBytes = MaxFileVocabularyBytes
	}
	notAdvertisable := make([]string, 0)
	eligible := make([]string, 0)
	for _, p := range paths {
		if p == "" {
			continue
		}
		if advertisable != nil && !advertisable(p) {
			notAdvertisable = append(notAdvertisable, p)
			continue
		}
		eligible = append(eligible, p)
	}
	eligible = dedupeSorted(eligible)
	cand := summarizeCandidates(eligible)
	refs := make([]FileRef, 0, len(eligible))
	acc := 0
	budgetOmitted := 0
	var budgetReps []string
	for index, p := range eligible {
		item := FileRef{Ref: refName("f", index+1), Path: p, Language: languageForPath(p), Role: FileRole(p)}
		size := fileRefItemSize(item)
		if len(refs) > 0 && acc+size > maxBytes {
			budgetOmitted++
			if len(budgetReps) < MaxOmissionRepresentatives {
				budgetReps = append(budgetReps, item.Ref)
			}
			continue
		}
		refs = append(refs, item)
		acc += size
	}
	vocab := Vocabulary{
		Version:         vocabularyVersion,
		Complete:        budgetOmitted == 0 && len(refs) == len(eligible),
		Considered:      len(paths),
		Advertised:      len(refs),
		CandidateSHA256: cand.Digest,
		Files:           refs,
	}
	if len(notAdvertisable) > 0 {
		vocab.Omissions = append(vocab.Omissions, Omission{Reason: "eligible_not_advertisable", Count: len(notAdvertisable), Representatives: representativeRefs(notAdvertisable)})
	}
	if budgetOmitted > 0 {
		vocab.Omissions = append(vocab.Omissions, Omission{Reason: "vocabulary_budget", Count: budgetOmitted, Representatives: budgetReps})
	}
	return vocab
}

func summarizeCandidates(paths []string) candidatelist {
	return candidatelist{Paths: append([]string(nil), paths...), Digest: candidateDigest(paths), Count: len(paths)}
}

func representativeRefs(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	if len(paths) <= MaxOmissionRepresentatives {
		out := make([]string, len(paths))
		for i, p := range paths {
			out[i] = p
		}
		return out
	}
	return append([]string(nil), paths[:MaxOmissionRepresentatives]...)
}
