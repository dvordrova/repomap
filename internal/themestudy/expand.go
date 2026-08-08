package themestudy

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

const expansionVersion = "v1"

// ErrExpansionSourceOversized lets a bounded local SourceReader stop before it
// materializes an arbitrarily large line. ExpandFiles converts it into the
// same item-local closed reason as a post-read per-file byte overflow.
var ErrExpansionSourceOversized = errors.New("source expansion exceeds the per-file byte bound")

// ExpandFiles executes the local f* source expansion (contract D). It is
// backend-executed and never performed by a model. Every requested file is
// expanded in full-or-indexed form or retained as a typed item-local closure;
// relevance is never inferred from a filename. Small files become a bounded
// whole-source object; large files become a bounded indexed window with
// explicit omitted ranges. Unreadable, invalid, and oversized siblings do not
// terminate valid exact sources. The result is persisted so the Adjudication
// request is rebuildable and replayable provider-free.
func ExpandFiles(files []FileRef, reader SourceReader, totalLines TotalLines) (SourceExpansion, error) {
	if reader == nil {
		return SourceExpansion{}, fmt.Errorf("source expansion requires a source reader")
	}
	if totalLines == nil {
		return SourceExpansion{}, fmt.Errorf("source expansion requires a total-lines resolver")
	}
	expansion := SourceExpansion{Version: expansionVersion}
	accBytes := 0
	accEncoded := 0
	accLines := 0
	var omittedRefs []string
	for _, file := range files {
		if len(expansion.Files) >= MaxExpansionFiles {
			omittedRefs = append(omittedRefs, file.Ref)
			continue
		}
		fileLen, err := totalLines(file.Path)
		if err != nil {
			appendClosedExpansionFile(
				&expansion, &omittedRefs, file, 0, false,
				ExpansionClosedReasonUnreadable,
			)
			continue
		}
		if fileLen <= 0 {
			appendClosedExpansionFile(
				&expansion, &omittedRefs, file, fileLen, false,
				ExpansionClosedReasonInvalid,
			)
			continue
		}
		entry := ExpansionFile{Ref: file.Ref, Path: file.Path, TotalLines: fileLen, Small: fileLen <= MaxExpansionFileLines}
		expectedLines := fileLen
		if entry.Small {
			lines, err := reader(file.Path, 1, fileLen)
			if err != nil {
				appendClosedExpansionFile(
					&expansion, &omittedRefs, file, fileLen, entry.Small,
					expansionReadClosedReason(err),
				)
				continue
			}
			if !validExpansionLines(lines, expectedLines) {
				appendClosedExpansionFile(
					&expansion, &omittedRefs, file, fileLen, entry.Small,
					ExpansionClosedReasonInvalid,
				)
				continue
			}
			entry.Objects = []SourceObject{{
				Role:          SourceRoleDeclaration,
				Path:          file.Path,
				Line:          1,
				Symbol:        "",
				FullBody:      true,
				Lines:         lines,
				ContentSHA256: contentSHA(lines),
			}}
			entry.ExpandedLines = len(lines)
		} else {
			// Large file: bounded indexed window (first MaxSourceObjectLines
			// lines) with the remainder as an explicit omitted range.
			limit := MaxSourceObjectLines
			if fileLen < limit {
				limit = fileLen
			}
			expectedLines = limit
			lines, err := reader(file.Path, 1, limit)
			if err != nil {
				appendClosedExpansionFile(
					&expansion, &omittedRefs, file, fileLen, entry.Small,
					expansionReadClosedReason(err),
				)
				continue
			}
			if !validExpansionLines(lines, expectedLines) {
				appendClosedExpansionFile(
					&expansion, &omittedRefs, file, fileLen, entry.Small,
					ExpansionClosedReasonInvalid,
				)
				continue
			}
			omitted := []LineRange{{StartLine: limit + 1, EndLine: fileLen}}
			entry.Objects = []SourceObject{{
				Role:          SourceRoleDeclaration,
				Path:          file.Path,
				Line:          1,
				FullBody:      false,
				Partial:       true,
				Lines:         lines,
				Omitted:       omitted,
				ContentSHA256: contentSHA(lines),
			}}
			entry.ExpandedLines = len(lines)
			entry.Omitted = omitted
		}
		fileBytes := 0
		for _, object := range entry.Objects {
			fileBytes += linesBytes(object.Lines)
		}
		if fileBytes > MaxExpansionFileBytes {
			appendClosedExpansionFile(
				&expansion, &omittedRefs, file, fileLen, entry.Small,
				ExpansionClosedReasonOversized,
			)
			continue
		}
		// Decision 235 / long-horizon incident (casdoor 2026-08-07): the
		// budget MUST be measured on the ENCODED artifact bytes, not raw
		// source bytes. JSON escaping and the per-object envelope grow raw
		// source ~1.3-2x, so a raw budget at MaxExpansionBytes let the
		// encoded artifact exceed MaxExpansionArtifactBytes and terminated
		// the whole Study stage ("theme source expansion artifact exceeds
		// 393216 bytes") despite accepted Scout state. The expansion
		// artifact is a locally persisted provider-free file — the ONLY
		// hard provider limit is the Adjudication wire (1 MiB) checked at
		// request compile. Excess requested files are recorded under
		// OmittedRefs, never dropped silently and never terminal (D190/D195).
		entryBytes, encodeErr := json.Marshal(entry)
		if encodeErr != nil {
			return SourceExpansion{}, fmt.Errorf("expand %s: encode entry: %w", file.Path, encodeErr)
		}
		encodedFileBytes := len(entryBytes)
		if encodedFileBytes > MaxExpansionBytes {
			appendClosedExpansionFile(
				&expansion, &omittedRefs, file, fileLen, entry.Small,
				ExpansionClosedReasonOversized,
			)
			continue
		}
		if accEncoded+encodedFileBytes > MaxExpansionBytes {
			appendClosedExpansionFile(
				&expansion, &omittedRefs, file, fileLen, entry.Small,
				ExpansionClosedReasonBudget,
			)
			continue
		}
		expansion.Files = append(expansion.Files, entry)
		accBytes += fileBytes
		accEncoded += encodedFileBytes
		accLines += entry.ExpandedLines
	}
	expansion.Requested = requestedRefs(files)
	expansion.OmittedRefs = omittedRefs
	expansion.ExpandedLines = accLines
	expansion.ExpandedBytes = accBytes
	expansion.CandidateSHA256 = candidateDigest(pathsOf(files))
	return expansion, nil
}

func appendClosedExpansionFile(
	expansion *SourceExpansion,
	omittedRefs *[]string,
	file FileRef,
	totalLines int,
	small bool,
	reason string,
) {
	expansion.Files = append(expansion.Files, ExpansionFile{
		Ref: file.Ref, Path: file.Path, TotalLines: totalLines, Small: small,
		Closed: true, ClosedReason: reason,
	})
	*omittedRefs = append(*omittedRefs, file.Ref)
}

func validExpansionLines(lines []string, expected int) bool {
	if expected <= 0 || len(lines) != expected {
		return false
	}
	for _, line := range lines {
		if !utf8.ValidString(line) {
			return false
		}
	}
	return true
}

func expansionReadClosedReason(err error) string {
	if errors.Is(err, ErrExpansionSourceOversized) {
		return ExpansionClosedReasonOversized
	}
	return ExpansionClosedReasonUnreadable
}

func requestedRefs(files []FileRef) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Ref)
	}
	return out
}

func pathsOf(files []FileRef) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}
