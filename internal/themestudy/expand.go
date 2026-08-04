package themestudy

import "fmt"

const expansionVersion = "v1"

// ExpandFiles executes the local f* source expansion (contract D). It is
// backend-executed and never performed by a model. Every requested file is
// expanded in full-or-indexed form, never filtered by name — relevance is never
// inferred from a filename alone. Small files become a bounded whole-source
// object; large files become a bounded indexed window with explicit omitted
// ranges. The result is persisted so the Adjudication request is rebuildable
// and replayable provider-free.
func ExpandFiles(files []FileRef, reader SourceReader, totalLines TotalLines) (SourceExpansion, error) {
	if reader == nil {
		return SourceExpansion{}, fmt.Errorf("source expansion requires a source reader")
	}
	if totalLines == nil {
		return SourceExpansion{}, fmt.Errorf("source expansion requires a total-lines resolver")
	}
	expansion := SourceExpansion{Version: expansionVersion}
	accBytes := 0
	accLines := 0
	var budgetOmitted []string
	for _, file := range files {
		if len(expansion.Files) >= MaxExpansionFiles {
			budgetOmitted = append(budgetOmitted, file.Ref)
			continue
		}
		fileLen, err := totalLines(file.Path)
		if err != nil {
			return SourceExpansion{}, fmt.Errorf("expand %s: total lines: %w", file.Path, err)
		}
		entry := ExpansionFile{Ref: file.Ref, Path: file.Path, TotalLines: fileLen, Small: fileLen <= MaxExpansionFileLines}
		if entry.Small {
			lines, err := reader(file.Path, 1, fileLen)
			if err != nil {
				return SourceExpansion{}, fmt.Errorf("expand %s: %w", file.Path, err)
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
			lines, err := reader(file.Path, 1, limit)
			if err != nil {
				return SourceExpansion{}, fmt.Errorf("expand %s: %w", file.Path, err)
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
		if len(expansion.Files) > 0 && accBytes+fileBytes > MaxExpansionBytes {
			budgetOmitted = append(budgetOmitted, file.Ref)
			continue
		}
		expansion.Files = append(expansion.Files, entry)
		accBytes += fileBytes
		accLines += entry.ExpandedLines
	}
	expansion.Requested = requestedRefs(files)
	expansion.ExpandedLines = accLines
	expansion.ExpandedBytes = accBytes
	expansion.CandidateSHA256 = candidateDigest(pathsOf(files))
	return expansion, nil
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
