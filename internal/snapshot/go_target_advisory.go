package snapshot

import (
	"fmt"
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

// GoTargetAdvisory is bounded deterministic evidence for one unique strong
// alternative. By default it remains console-only guidance; an ordinary
// caller may explicitly authorize the automatic selection contract before Go
// facts are loaded.
type GoTargetAdvisory struct {
	Suggested     string
	EvidenceFiles int
	Examples      []string
}

const GoTargetSelectionAuto = "auto"

// GoTargetSelection is live exact provenance for a pre-Go-facts automatic
// target choice. Target and Baseline are canonical GOOS/GOARCH pairs; the
// evidence is the same bounded D251 authority that caused the choice. It is
// copied through live target projections and persisted separately in ordinary
// run metadata, never sent to a provider.
type GoTargetSelection struct {
	Source        string
	Target        string
	Baseline      string
	EvidenceFiles int
	Examples      []string
}

func newAutomaticGoTargetSelection(
	baseline gotarget.Target,
	advisory GoTargetAdvisory,
) (GoTargetSelection, error) {
	selected, err := gotarget.Parse(advisory.Suggested)
	if err != nil {
		return GoTargetSelection{}, fmt.Errorf("automatic Go target selection: %w", err)
	}
	if selected.GOOS == baseline.GOOS || selected.GOARCH != baseline.GOARCH ||
		advisory.EvidenceFiles < 3 {
		return GoTargetSelection{}, fmt.Errorf("automatic Go target selection: advisory is not a strong alternative")
	}
	selection := GoTargetSelection{
		Source: GoTargetSelectionAuto, Target: selected.String(), Baseline: baseline.String(),
		EvidenceFiles: advisory.EvidenceFiles,
		Examples:      append([]string(nil), advisory.Examples...),
	}
	if err := selection.Validate(); err != nil {
		return GoTargetSelection{}, err
	}
	if err := selection.ValidateAgainstAdvisory(&advisory); err != nil {
		return GoTargetSelection{}, err
	}
	return selection, nil
}

func (selection GoTargetSelection) Validate() error {
	if selection.Source != GoTargetSelectionAuto {
		return fmt.Errorf("automatic Go target selection: invalid source %q", selection.Source)
	}
	selected, err := gotarget.Parse(selection.Target)
	if err != nil {
		return fmt.Errorf("automatic Go target selection target: %w", err)
	}
	baseline, err := gotarget.Parse(selection.Baseline)
	if err != nil {
		return fmt.Errorf("automatic Go target selection baseline: %w", err)
	}
	if selected.GOOS == baseline.GOOS || selected.GOARCH != baseline.GOARCH || selection.EvidenceFiles < 3 {
		return fmt.Errorf("automatic Go target selection: invalid alternative authority")
	}
	if len(selection.Examples) > maxGoTargetAdvisoryExamples {
		return fmt.Errorf("automatic Go target selection: too many evidence paths")
	}
	for _, path := range selection.Examples {
		if path == "" || path != strings.TrimSpace(path) || !goTargetAdvisoryEligiblePath(path) {
			return fmt.Errorf("automatic Go target selection: invalid evidence path")
		}
	}
	return nil
}

func (selection GoTargetSelection) ValidateAgainstAdvisory(advisory *GoTargetAdvisory) error {
	if err := selection.Validate(); err != nil {
		return err
	}
	if advisory == nil || advisory.Suggested != selection.Target ||
		advisory.EvidenceFiles != selection.EvidenceFiles ||
		len(advisory.Examples) != len(selection.Examples) {
		return fmt.Errorf("automatic Go target selection: advisory authority mismatch")
	}
	for index := range advisory.Examples {
		if advisory.Examples[index] != selection.Examples[index] {
			return fmt.Errorf("automatic Go target selection: advisory evidence mismatch")
		}
	}
	return nil
}

// Display is console copy derived only from exact typed selection fields.
func (selection GoTargetSelection) Display() string {
	baseline, err := gotarget.Parse(selection.Baseline)
	if err != nil {
		return selection.Source + ": " + selection.Target
	}
	return selection.Source + ": " + selection.Target + " (host " + baseline.GOOS + ")"
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
