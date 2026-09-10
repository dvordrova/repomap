package report

// pageOutbound is one accepted communication record. Destination and Summary
// are display prose; Address, NativeLabel and External retain source spelling.
// No dependency-group membership is required and no package import creates one.
type pageOutbound struct {
	ID                          string
	Destination, DestinationRef string
	Summary, SummaryRef         string
	Address, NativeLabel        string
	External, Basis, Source     string
	Anchor                      pageAnchor
}

func (builder *pageBuilder) fillSectionOutbound(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	for _, call := range index.Outbound {
		row := pageOutbound{
			ID:          section.ID + "-out-" + call.ID,
			Destination: call.Destination, Summary: call.Summary,
			Address: call.Address, External: call.External, Basis: call.Basis, Source: call.Source,
			Anchor: builder.links.anchor(call.Location.Path, call.Location.Line, call.Location.Column),
		}
		// Native HTTP facts may have no semantic label. Compose only their
		// supplied method and address, never a guessed destination.
		if call.Method != "" && call.Address != "" {
			row.NativeLabel = call.Method + " " + call.Address
		} else {
			row.NativeLabel = call.External
		}
		section.Outbound = append(section.Outbound, row)
	}
}

func (row pageOutbound) BasisLabel() string {
	switch row.Basis {
	case "configuration":
		return "Configured communication"
	case "dispatch":
		return "Communication call"
	default:
		return ""
	}
}
