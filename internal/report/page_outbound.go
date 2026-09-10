package report

import "strings"

// pageOutbound is one accepted communication record. Destination and Summary
// are display prose; Address, NativeLabel and External retain source spelling.
// No dependency-group membership is required and no package import creates one.
type pageOutbound struct {
	KindLabel                   string
	DestinationCount            int
	Uses                        []pageOutboundUse
	ID                          string
	Destination, DestinationRef string
	Summary, SummaryRef         string
	Address, NativeLabel        string
	External, Basis, Source     string
	Anchor                      pageAnchor
}

type pageOutboundUse struct {
	Address, Frontier, Method string
	Steps                     []pageOutboundStep
}

type pageOutboundStep struct {
	Name   string
	Anchor pageAnchor
}

// AddressText expands the destination reader's leading configuration notation
// for display only. It never resolves a setting or changes the saved address.
type pageOutboundAddress struct {
	Text, Setting, SettingLabel, Suffix string
}

func outboundAddressText(address string) pageOutboundAddress {
	result := pageOutboundAddress{Text: address}
	prefix, label := "{--", "Address from command-line option"
	if strings.HasPrefix(address, "{env:") {
		prefix, label = "{env:", "Address from environment variable"
	}
	if !strings.HasPrefix(address, prefix) {
		return result
	}
	name, suffix, found := strings.Cut(strings.TrimPrefix(address, prefix), "}")
	if !found || name == "" || strings.ContainsAny(name, "{} \t\r\n") || strings.ContainsAny(suffix, "{}") {
		return result
	}
	result.Setting, result.SettingLabel, result.Suffix = name, label, suffix
	if prefix == "{--" {
		result.Setting = "--" + name
	}
	return result
}

func (row pageOutbound) AddressText() pageOutboundAddress    { return outboundAddressText(row.Address) }
func (use pageOutboundUse) AddressText() pageOutboundAddress { return outboundAddressText(use.Address) }

func (builder *pageBuilder) fillSectionOutbound(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	for _, call := range index.Outbound {
		row := pageOutbound{
			KindLabel:   outboundKindLabel(call.Kind),
			ID:          section.ID + "-out-" + call.ID,
			Destination: call.Destination, Summary: call.Summary,
			Address: call.Address, External: call.External, Basis: call.Basis, Source: call.Source,
			Anchor: builder.links.anchor(call.Location.Path, call.Location.Line, call.Location.Column),
		}
		destinations := make(map[string]bool)
		for _, use := range call.Uses {
			destinations[use.Address+"\x00"+use.Frontier] = true
			value := pageOutboundUse{Address: use.Address, Frontier: use.Frontier, Method: use.Method}
			for _, step := range use.Steps {
				name := step.Name
				// Existing operation subjects already own their interpreted
				// names. Matching the original declaration binds this source
				// chain to that operation without inventing another label.
				for _, operation := range index.Operations {
					if step.SubjectID != "" && operation.SubjectID == step.SubjectID {
						name = operation.Name
						break
					}
				}
				value.Steps = append(value.Steps, pageOutboundStep{Name: name, Anchor: builder.links.anchor(step.Path, step.Line, step.Column)})
			}
			row.Uses = append(row.Uses, value)
		}
		row.DestinationCount = len(destinations)
		if len(call.Uses) > 0 {
			row.Address = ""
			if len(destinations) == 1 {
				row.Address = call.Uses[0].Address
			}
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

func outboundKindLabel(kind string) string {
	switch kind {
	case "http_client":
		return "HTTP"
	case "db":
		return "Database"
	case "queue_producer", "queue_consumer":
		return "Queue"
	case "sdk":
		return "SDK"
	default:
		return "External communication"
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
