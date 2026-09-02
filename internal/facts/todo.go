package facts

import (
	"regexp"
	"strings"
)

// todoScanMaxBytes skips generated or vendored blobs; a real TODO lives in a
// file a person edits.
const todoScanMaxBytes = 1 << 20

var todoMarker = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`)

// addTODOs scans every text file of the corpus. Rows under a target root are
// attributed to that target; the rest stay repository-level.
func (b *builder) addTODOs() {
	for _, filePath := range b.source.paths() {
		file, ok := b.source.file(filePath)
		if !ok || file.binary || file.size > todoScanMaxBytes {
			continue
		}
		targetID := b.targetForPath(filePath)
		root := b.rootForTarget(targetID)
		for number, line := range file.lines {
			match := todoMarker.FindStringSubmatchIndex(line)
			if match == nil {
				continue
			}
			marker := line[match[2]:match[3]]
			text := clipText(strings.TrimLeft(strings.TrimSpace(line[match[3]:]), ":- "))
			text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
			if text == "" {
				text = marker
			}
			b.add(root, Fact{
				Kind:     KindTODO,
				TargetID: targetID,
				Anchor:   &Anchor{Path: filePath, Line: number + 1},
				Key:      marker,
				Text:     text,
			}, marker, text)
		}
	}
}
