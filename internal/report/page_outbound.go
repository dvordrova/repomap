package report

import (
	"sort"
	"strings"
)

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

// displayCallable drops the program index's platform notation from a
// callable's name: "platform:javascript.WebSocket" reads WebSocket on the
// page while the saved atlas keeps the original spelling.
func displayCallable(name string) string {
	rest, ok := strings.CutPrefix(name, "platform:")
	if !ok {
		return name
	}
	if _, after, found := strings.Cut(rest, "."); found && after != "" {
		return after
	}
	return rest
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
			Address: call.Address, External: displayCallable(call.External), Basis: call.Basis, Source: call.Source,
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
			row.NativeLabel = displayCallable(call.External)
		}
		section.Outbound = append(section.Outbound, row)
	}
}

// pageOutboundGroup presents every record naming one destination as one
// row: the destination, how many records name it, their shared kind, basis
// and address. The records are compact lines nested beneath it, three in
// view and the rest under one disclosure; each opens its own purpose,
// address and source. Nothing is merged in the data.
type pageOutboundGroup struct {
	Destination, NativeLabel, KindLabel string
	Basis, Source, Address              string
	Addresses                           int
	Rows                                []pageOutbound
}

func (group pageOutboundGroup) BasisLabel() string {
	return pageOutbound{Basis: group.Basis}.BasisLabel()
}

func (group pageOutboundGroup) AddressText() pageOutboundAddress {
	return outboundAddressText(group.Address)
}

// outboundGroupPreview is how many call records a destination shows before
// the rest wait under one "Expand" disclosure. Every record is rendered.
const outboundGroupPreview = 3

func (group pageOutboundGroup) First() []pageOutbound {
	if len(group.Rows) <= outboundGroupPreview {
		return group.Rows
	}
	return group.Rows[:outboundGroupPreview]
}

func (group pageOutboundGroup) Rest() []pageOutbound {
	if len(group.Rows) <= outboundGroupPreview {
		return nil
	}
	return group.Rows[outboundGroupPreview:]
}

// Brief is one record's line beneath its destination when the source names
// no method, address or callable: the first sentence of its purpose, at most
// 90 runes. The full purpose, address, basis and source chain open under it.
func (row pageOutbound) Brief() string {
	if text := strings.TrimSpace(row.Summary); text != "" {
		return leadSentence(text, 90)
	}
	return ""
}

func leadSentence(text string, limit int) string {
	if end := strings.Index(text, ". "); end > 0 {
		text = text[:end+1]
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	cut := limit - 1
	for cut > limit/2 && runes[cut] != ' ' {
		cut--
	}
	return strings.TrimRight(string(runes[:cut]), " ,;:") + "…"
}

// groupOutbound groups records by their destination text (case-insensitive),
// or by native label or kind when the model named no destination. Groups
// with more records come first; equal counts keep record order. Grouping is
// a rendering step over translated rows, so it changes no saved data.
func groupOutbound(rows []pageOutbound) []pageOutboundGroup {
	var groups []pageOutboundGroup
	position := make(map[string]int)
	for _, row := range rows {
		key := "k\x00" + row.KindLabel
		switch {
		case strings.TrimSpace(row.Destination) != "":
			key = "d\x00" + strings.ToLower(strings.TrimSpace(row.Destination))
		case row.NativeLabel != "":
			key = "n\x00" + row.NativeLabel
		}
		at, known := position[key]
		if !known {
			at = len(groups)
			position[key] = at
			groups = append(groups, pageOutboundGroup{Destination: strings.TrimSpace(row.Destination), NativeLabel: row.NativeLabel,
				KindLabel: row.KindLabel, Basis: row.Basis, Source: row.Source})
		}
		group := &groups[at]
		if group.KindLabel != row.KindLabel {
			group.KindLabel = "External communication"
		}
		if group.Basis != row.Basis {
			group.Basis = ""
		}
		if group.Source != row.Source {
			group.Source = "model"
		}
		group.Rows = append(group.Rows, row)
	}
	for i := range groups {
		addresses := make(map[string]bool)
		for _, row := range groups[i].Rows {
			if row.Address != "" {
				addresses[row.Address] = true
			}
			if row.DestinationCount > 1 {
				for _, use := range row.Uses {
					if use.Address != "" {
						addresses[use.Address] = true
					}
				}
			}
		}
		groups[i].Addresses = len(addresses)
		if len(addresses) == 1 {
			for address := range addresses {
				groups[i].Address = address
			}
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return len(groups[i].Rows) > len(groups[j].Rows) })
	return groups
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
