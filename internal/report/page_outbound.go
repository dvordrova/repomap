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
	Method                      string
	// LineWithType keeps the callable's type on the line because another
	// record of the same destination has the same member on another type:
	// "PodInterface.Patch" and "DeploymentInterface.Patch", not "Patch, Patch".
	LineWithType bool
	Anchor       pageAnchor
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
			Address: call.Address, External: displayCallable(call.External), Basis: call.Basis, Source: call.Source, Method: call.Method,
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

// genericCallables are member names that say nothing without their type:
// "Ping" reads as "Pool.Ping", while "ExchangeDeclare" stands on its own.
var genericCallables = map[string]bool{"New": true, "Close": true, "Ping": true, "Get": true, "Set": true, "Do": true, "Run": true, "Start": true, "Stop": true,
	"Connect": true, "Open": true, "Exec": true, "Query": true, "Call": true, "Send": true, "Write": true, "Read": true, "Begin": true, "Commit": true,
	"Rollback": true, "Publish": true, "Consume": true, "Dial": true, "Delete": true, "Update": true, "Create": true, "List": true, "Put": true, "Post": true,
	"Up": true, "Down": true, "Version": true, "Save": true, "Load": true, "Fetch": true, "Store": true, "Add": true, "Remove": true, "Find": true,
	"Patch": true, "Watch": true, "Apply": true, "Scan": true, "Push": true, "Pull": true, "Insert": true, "Select": true, "Subscribe": true, "Unsubscribe": true, "Emit": true, "On": true}

// Line is one record's line beneath its destination: a native HTTP fact
// keeps its method and address; a callable drops the package the group
// already implies ("amqp091-go.Channel.Confirm" under RabbitMQ reads
// "Confirm") and keeps its type only when the member name alone is generic
// ("Pool.Ping", "Migrate.Up"). The full callable stays in the record's body.
func (row pageOutbound) Line() string {
	if row.Method != "" && row.Address != "" {
		return row.NativeLabel
	}
	if row.External != "" {
		return shortCallable(row.External, row.LineWithType)
	}
	// An unresolved interface call has no callable of its own; the function
	// that makes it is the next best name for the line.
	for _, use := range row.Uses {
		for _, step := range use.Steps {
			if step.Name != "" {
				return shortCallable(step.Name, false)
			}
		}
	}
	return ""
}

// InformativeUses are the destination chains worth a line: ones that reach
// an address, stop at a named frontier, or pass through more than one
// step. A chain of one step at the record's own location says nothing the
// record's anchor does not.
func (row pageOutbound) InformativeUses() []pageOutboundUse {
	var uses []pageOutboundUse
	for _, use := range row.Uses {
		if use.Address != "" || use.Frontier != "" || len(use.Steps) > 1 {
			uses = append(uses, use)
		}
	}
	return uses
}

func shortCallable(external string, keepType bool) string {
	parts := strings.Split(external, ".")
	if len(parts) < 2 {
		return external
	}
	member := parts[len(parts)-1]
	if (keepType || genericCallables[member]) && len(parts) >= 3 {
		return parts[len(parts)-2] + "." + member
	}
	return member
}

// callableParts splits "pkg.Type.Member" into its type (empty for a
// package-level function) and member.
func callableParts(external string) (string, string) {
	parts := strings.Split(external, ".")
	if len(parts) < 3 {
		return "", parts[len(parts)-1]
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

// Brief is the note printed right after the call on its line: the first
// sentence of the record's purpose, at most 120 runes. With the telegraphic
// boundaries note that is the whole purpose; a longer purpose keeps its
// full text under the disclosure (see MoreThanBrief).
func (row pageOutbound) Brief() string {
	if text := strings.TrimSpace(row.Summary); text != "" {
		return leadSentence(text, 120)
	}
	return ""
}

// MoreThanBrief reports a purpose longer than the note on the line, which
// the disclosure then repeats in full.
func (row pageOutbound) MoreThanBrief() bool {
	return strings.TrimSpace(row.Summary) != row.Brief()
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
		destination := canonicalDestination(row.Destination)
		switch {
		case destination != "":
			key = "d\x00" + strings.ToLower(destination)
		case row.NativeLabel != "":
			key = "n\x00" + row.NativeLabel
		}
		at, known := position[key]
		if !known {
			at = len(groups)
			position[key] = at
			groups = append(groups, pageOutboundGroup{Destination: destination, NativeLabel: row.NativeLabel,
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
	// Within one destination a member shared by several types keeps its
	// type on the line; a member used by one type reads alone.
	for i := range groups {
		types := make(map[string]map[string]bool)
		for _, row := range groups[i].Rows {
			if row.External == "" {
				continue
			}
			typ, member := callableParts(row.External)
			if types[member] == nil {
				types[member] = make(map[string]bool)
			}
			types[member][typ] = true
		}
		for j := range groups[i].Rows {
			row := &groups[i].Rows[j]
			if row.External == "" {
				continue
			}
			_, member := callableParts(row.External)
			row.LineWithType = len(types[member]) > 1
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return len(groups[i].Rows) > len(groups[j].Rows) })
	return groups
}

// knownSystems maps a word found in a destination to the name its group
// carries. One run named one broker "RabbitMQ broker", "AMQP broker
// (RabbitMQ)", "RabbitMQ broker (queue topology)" and four more ways; the
// reader wants one row per system. Records keep their own wording.
var knownSystems = []struct{ word, name string }{
	{"rabbitmq", "RabbitMQ"}, {"amqp", "RabbitMQ"}, {"kafka", "Kafka"}, {"nats", "NATS"},
	{"redis", "Redis"}, {"memcache", "Memcached"},
	{"postgres", "PostgreSQL"}, {"pgx", "PostgreSQL"}, {"mysql", "MySQL"}, {"mariadb", "MariaDB"},
	{"sqlite", "SQLite"}, {"mongo", "MongoDB"}, {"clickhouse", "ClickHouse"}, {"elasticsearch", "Elasticsearch"},
	{"minio", "S3 storage"}, {"s3", "S3 storage"}, {"github", "GitHub"}, {"google", "Google"},
	{"slack", "Slack"}, {"telegram", "Telegram"}, {"stripe", "Stripe"}, {"sentry", "Sentry"},
	{"otlp", "OpenTelemetry collector"}, {"opentelemetry", "OpenTelemetry collector"},
	{"kubernetes", "Kubernetes API server"}, {"docker", "Docker daemon"},
}

// canonicalDestination is the group name for a destination text: the known
// system it names, else the text without its parenthetical qualifier.
// "remote PostgreSQL database" and "PostgreSQL database" are one group;
// "Cache store (concrete implementation unresolved)" stays "Cache store",
// not Redis, because nothing in it names Redis.
func canonicalDestination(text string) string {
	base := strings.TrimSpace(text)
	if i := strings.Index(base, "("); i > 0 {
		base = strings.TrimSpace(base[:i])
	}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	for _, system := range knownSystems {
		for _, word := range words {
			if word == system.word || len(system.word) >= 5 && strings.HasPrefix(word, system.word) {
				return system.name
			}
		}
	}
	return base
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
