package report

import "slices"

// destinationWritten is where the value naming what a destination's calls
// reach ends, as the code wrote it, each once in record order: an address,
// or the expression the walk stopped at (FrontierName), with the place of
// the first. The destination's reading says it under its name, as an
// input's says its registration: Redis's Primary, what redis-server's
// connect and gethostbyname reach from syncWithMaster, had read its name
// alone, and its value ends at server.masterhost. Nothing is resolved or
// composed here: a value the walk could not read names nothing, and a
// started program is named by its own word already.
func destinationWritten(rows []pageOutbound) (string, pageAnchor) {
	var values []string
	var first pageAnchor
	for _, row := range rows {
		if row.Program {
			continue
		}
		for _, use := range row.Uses {
			value := use.Value
			if value == "" {
				value = use.FrontierName()
			}
			if value == "" || slices.Contains(values, value) {
				continue
			}
			if len(values) == 0 && len(use.Steps) > 0 {
				first = use.Steps[len(use.Steps)-1].Anchor
			}
			values = append(values, value)
		}
	}
	if len(values) != 1 {
		// Several ends are several places the calls go, each read in its
		// record: no one of them says what the destination is.
		return "", pageAnchor{}
	}
	return values[0], first
}
