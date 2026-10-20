package jstsproject

import (
	"strconv"
	"strings"
)

// helperHeapArguments is the V8 heap ceiling the helper runs with: half the
// machine's physical memory, unless NODE_OPTIONS already sets one (either
// spelling V8 accepts), which then stands, or the memory is unknown. V8's
// own default (about 4.3 GB on a 32 GB Mac) is an engine heuristic, not an
// envelope of the evidence: orca's three project references needed ~13 GB
// and completed at 16 GB in 940 s (2026-10-20). A repository needing more
// still fails explicitly.
func helperHeapArguments(nodeOptions string) []string {
	for _, option := range strings.Fields(nodeOptions) {
		if strings.HasPrefix(option, "--max-old-space-size") || strings.HasPrefix(option, "--max_old_space_size") {
			return nil
		}
	}
	memory := physicalMemory()
	if memory == 0 {
		return nil
	}
	return []string{"--max-old-space-size=" + strconv.FormatUint(memory/2/(1<<20), 10)}
}
