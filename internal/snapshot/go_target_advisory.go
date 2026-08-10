package snapshot

import (
	"go/build/constraint"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	maxGoTargetAdvisoryFiles       = 8192
	maxGoTargetAdvisoryPrefixBytes = 4096
	maxGoTargetAdvisoryExamples    = 3
)

// GoTargetAdvisory is console-only guidance and never changes the target.
type GoTargetAdvisory struct {
	Suggested     string
	EvidenceFiles int
	Examples      []string
}

// Keep the first heuristic deliberately small and useful for onboarding.
var advisoryGOOS = map[string]bool{
	"darwin": true, "freebsd": true, "linux": true, "windows": true,
}

func detectGoTargetAdvisory(repoPath string, files []string, current gotarget.Target) *GoTargetAdvisory {
	eligible := make([]string, 0)
	for _, path := range files {
		if goTargetAdvisoryEligiblePath(path) {
			eligible = append(eligible, path)
		}
	}
	if len(eligible) == 0 || len(eligible) > maxGoTargetAdvisoryFiles {
		return nil
	}
	reader, err := reporead.New(repoPath)
	if err != nil {
		return nil
	}
	defer reader.Close()

	counts := make(map[string]int)
	examples := make(map[string][]string)
	for _, path := range eligible {
		content, err := reader.ReadFileNoSymlinks(path, maxGoTargetAdvisoryPrefixBytes)
		if err != nil {
			return nil
		}
		for goos := range goTargetEvidenceForFile(path, content.Bytes, current.GOARCH) {
			counts[goos]++
			if len(examples[goos]) < maxGoTargetAdvisoryExamples {
				examples[goos] = append(examples[goos], path)
			}
		}
	}
	best, bestCount, tied := "", 0, false
	for goos, count := range counts {
		if goos == current.GOOS {
			continue
		}
		if count > bestCount {
			best, bestCount, tied = goos, count, false
		} else if count == bestCount {
			tied = true
		}
	}
	if best == "" || tied || bestCount < 3 || bestCount < 2*counts[current.GOOS] {
		return nil
	}
	return &GoTargetAdvisory{
		Suggested: best + "/" + current.GOARCH, EvidenceFiles: bestCount,
		Examples: append([]string(nil), examples[best]...),
	}
}

func goTargetAdvisoryEligiblePath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	wrapped := "/" + path + "/"
	for _, segment := range []string{"vendor", "testdata", "example", "examples", "tool", "tools"} {
		if strings.Contains(wrapped, "/"+segment+"/") {
			return false
		}
	}
	return true
}

func goTargetEvidenceForFile(path string, prefix []byte, arch string) map[string]struct{} {
	result := make(map[string]struct{})
	parts := strings.Split(strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".go"), "_")
	if len(parts) >= 2 && advisoryGOOS[parts[len(parts)-1]] {
		result[parts[len(parts)-1]] = struct{}{}
	} else if len(parts) >= 3 && advisoryGOOS[parts[len(parts)-2]] && parts[len(parts)-1] == arch {
		result[parts[len(parts)-2]] = struct{}{}
	}

	var buildLine string
	for _, line := range strings.Split(string(prefix), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			break
		}
		if strings.HasPrefix(line, "//go:build ") {
			buildLine = line
			break
		}
	}
	if buildLine == "" {
		return result
	}
	expr, err := constraint.Parse(buildLine)
	if err != nil {
		return result
	}
	positive := make(map[string]struct{})
	if !collectAdvisoryGOOS(expr, arch, true, positive) {
		return result
	}
	for goos := range positive {
		if expr.Eval(func(tag string) bool { return tag == goos || tag == arch }) {
			result[goos] = struct{}{}
		}
	}
	return result
}

func collectAdvisoryGOOS(expr constraint.Expr, arch string, positive bool, found map[string]struct{}) bool {
	switch expr := expr.(type) {
	case *constraint.TagExpr:
		if advisoryGOOS[expr.Tag] {
			if positive {
				found[expr.Tag] = struct{}{}
			}
			return true
		}
		return expr.Tag == arch
	case *constraint.NotExpr:
		return collectAdvisoryGOOS(expr.X, arch, !positive, found)
	case *constraint.AndExpr:
		return collectAdvisoryGOOS(expr.X, arch, positive, found) &&
			collectAdvisoryGOOS(expr.Y, arch, positive, found)
	case *constraint.OrExpr:
		return collectAdvisoryGOOS(expr.X, arch, positive, found) &&
			collectAdvisoryGOOS(expr.Y, arch, positive, found)
	default:
		return false
	}
}
