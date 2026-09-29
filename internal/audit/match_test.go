package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The five inventories a component is scored on, in table order.
var inventories = []string{"requests", "commands", "workers", "external", "data"}

// inventory is testdata/audit/<repo>/inventory.json: ground truth written from
// the code only at a pinned revision, reviewed, then changed only by dated
// errata. Fields the audit does not read are kept raw so the decoder stays
// strict about the ones it does.
type inventory struct {
	Repository    string            `json:"repository"`
	Path          string            `json:"path,omitempty"`
	Revision      string            `json:"revision"`
	Components    json.RawMessage   `json:"components,omitempty"`
	Items         []inventoryItem   `json:"items"`
	Not           []inventoryTrap   `json:"not"`
	Notes         json.RawMessage   `json:"notes,omitempty"`
	ReviewSummary json.RawMessage   `json:"review_summary,omitempty"`
	Review        json.RawMessage   `json:"review,omitempty"`
	Errata        []json.RawMessage `json:"errata,omitempty"`

	// entries are the declared components' entry files by component name
	// (components[].entry: "src/othello/core.clj:5 -main -> …").
	entries map[string]string
}

// inventoryComponent is a declared component as the audit reads it: its
// name and the entry its program starts at.
type inventoryComponent struct {
	Name  string `json:"name"`
	Entry string `json:"entry,omitempty"`
}

// inventoryItem is one thing a newcomer needs to find (must) or may find.
// AlsoComponent names components a report may equally credit it to. Flow
// marks the program's own foreground loop: it is scored against the Main
// flow, never against the workers inventory.
type inventoryItem struct {
	Component     string          `json:"component"`
	Inventory     string          `json:"inventory"`
	Kind          string          `json:"kind"`
	Name          string          `json:"name"`
	Anchor        string          `json:"anchor"`
	Handler       json.RawMessage `json:"handler"`
	Must          bool            `json:"must"`
	Why           string          `json:"why"`
	AlsoComponent []string        `json:"also_component,omitempty"`
	Flow          bool            `json:"flow,omitempty"`
	Struct        json.RawMessage `json:"struct,omitempty"`
	UseSite       json.RawMessage `json:"use_site,omitempty"`
}

// inventoryTrap is a known false row: listing it is wrong.
type inventoryTrap struct {
	Component string `json:"component"`
	Name      string `json:"name"`
	Anchor    string `json:"anchor"`
	Reason    string `json:"reason"`
	Flow      bool   `json:"flow,omitempty"`
}

func loadInventory(path string) (*inventory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeInventory(raw)
}

func decodeInventory(raw []byte) (*inventory, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var inv inventory
	if err := decoder.Decode(&inv); err != nil {
		return nil, fmt.Errorf("inventory: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("inventory: trailing data")
	}
	if inv.Revision == "" || len(inv.Items) == 0 {
		return nil, fmt.Errorf("inventory: a revision and items are required")
	}
	for i, item := range inv.Items {
		if item.Component == "" || item.Anchor == "" || item.Name == "" || !slices.Contains(inventories, item.Inventory) {
			return nil, fmt.Errorf("inventory: item %d (%q) needs a component, a name, an anchor and one of %v", i, item.Name, inventories)
		}
	}
	for i, trap := range inv.Not {
		if trap.Anchor == "" || trap.Name == "" {
			return nil, fmt.Errorf("inventory: trap %d needs a name and an anchor", i)
		}
	}
	if len(inv.Components) > 0 {
		var declared []inventoryComponent
		if err := json.Unmarshal(inv.Components, &declared); err != nil {
			return nil, fmt.Errorf("inventory: components: %w", err)
		}
		inv.entries = map[string]string{}
		for _, component := range declared {
			if entry, _, _ := strings.Cut(strings.TrimSpace(component.Entry), " "); component.Name != "" && entry != "" {
				inv.entries[component.Name] = parseAnchor(entry).path
			}
		}
	}
	return &inv, nil
}

// anchorAt is a repository path and line; line 0 is a whole file.
type anchorAt struct {
	path string
	line int
}

func (a anchorAt) String() string {
	if a.line > 0 {
		return a.path + ":" + strconv.Itoa(a.line)
	}
	return a.path
}

var anchorPattern = regexp.MustCompile(`^(.*?):(\d+)(?:-\d+)?$`)

func parseAnchor(text string) anchorAt {
	text = strings.TrimSpace(text)
	if m := anchorPattern.FindStringSubmatch(text); m != nil {
		line, _ := strconv.Atoi(m[2])
		return anchorAt{m[1], line}
	}
	return anchorAt{path: text}
}

// reportRow is one row the report shows in a component's inventories: an
// input of the Inputs collection, an outgoing call, a data record, an
// entrypoint or a configuration read. Its anchor is its own location; an
// outgoing call also carries its ReachedFrom caller sites.
type reportRow struct {
	id        string
	target    string
	component string
	mapped    bool
	section   string // inputs, external, data, entrypoints, config
	family    string // the inventory the row belongs to
	kind      string
	name      string
	at        anchorAt
	callers   []anchorAt

	destination, external string
	// paths and writes are a file record's claim (section data, kind file):
	// the path as written, and each write of the field it is read from;
	// unknownPath is a file whose path is not established.
	paths              []string
	writes             []fileWrite
	unknownPath        bool
	handlerUnknown     bool
	handlerUnreachable bool
	handler            string
	declaredBy         string
	declaredOn         string
	valueOf            string
	// nested is the product's Launch.Nested: an input listed under another
	// input (a sub-argument, a value of its words), never a row of its own.
	nested bool

	status   string // match, other, trap, extra, entry, same_destination, nested, unknown
	tier     int
	items    []int
	traps    []int
	expected []string
}

// fileWrite is one write of a file record's field: its site and the path it
// stores, empty when that is not established.
type fileWrite struct {
	at   anchorAt
	path string
}

// operationFamily is the inventory an input row of a kind belongs to.
var operationFamily = map[string]string{
	"request": "requests", "interaction": "requests",
	"command": "commands", "setting": "commands", "extension": "commands", "entry": "commands",
	"scheduled": "workers", "continuous": "workers", "consumer": "workers",
}

var (
	settingKinds   = map[string]bool{"setting": true, "env": true, "url-query": true}
	scheduledKinds = map[string]bool{"timer": true, "cron-step": true, "scheduled-job": true, "scheduler": true, "ticker": true}
	queueKinds     = map[string]bool{"queue-consumer": true, "consumer": true}
)

// expectedKinds are the row kinds an item is found with; any other kind of
// the same match is "kind differs".
func expectedKinds(item inventoryItem) map[string]bool {
	switch item.Inventory {
	case "requests":
		return map[string]bool{"request": true}
	case "commands":
		if item.Kind == "entrypoint" {
			return map[string]bool{"entry": true, "command": true, "extension": true}
		}
		if settingKinds[item.Kind] {
			return map[string]bool{"setting": true, "config": true}
		}
		return map[string]bool{"command": true, "extension": true, "entry": true}
	case "workers":
		switch {
		case scheduledKinds[item.Kind]:
			return map[string]bool{"scheduled": true}
		case queueKinds[item.Kind]:
			return map[string]bool{"consumer": true}
		}
		return map[string]bool{"continuous": true}
	case "external":
		return map[string]bool{"outbound": true}
	}
	return map[string]bool{"data": true}
}

func rowKindClass(row *reportRow) string {
	switch row.section {
	case "external":
		return "outbound"
	case "data":
		return "data"
	}
	return row.kind
}

var (
	componentPrefix = regexp.MustCompile(`^\./`)
	componentSuffix = regexp.MustCompile(`\.(py|rb|sh|tcl|ps1|go|js|ts)$`)
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]`)
)

// normComponent is how a report target's key or program path meets an
// inventory component: lower case, letters and digits only, no script
// extension.
func normComponent(name string) string {
	s := strings.TrimSpace(strings.ToLower(name))
	s = componentPrefix.ReplaceAllString(s, "")
	s = componentSuffix.ReplaceAllString(s, "")
	return nonAlphanumeric.ReplaceAllString(s, "")
}

var (
	trailingQualifier = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
	qualifierText     = regexp.MustCompile(`\(([^)]*)\)\s*$`)
	nameSeparator     = regexp.MustCompile(`\s+/\s+|,\s*|\s+\|\s+`)
	flagSeparator     = regexp.MustCompile(`[/|]`)
)

// nameAlternatives are the written names an item accepts: "a / b", "a, b",
// "-c/--config" and a trailing "(qualifier)" give alternatives; a data item
// also accepts its qualifier as a table name.
func nameAlternatives(name string, data bool) map[string]bool {
	s := strings.TrimSpace(name)
	out := []string{s}
	if base := strings.TrimSpace(trailingQualifier.ReplaceAllString(s, "")); base != "" {
		out = append(out, base)
	}
	if data {
		if m := qualifierText.FindStringSubmatch(s); m != nil {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	for _, piece := range slices.Clone(out) {
		for _, part := range nameSeparator.Split(piece, -1) {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	for _, piece := range slices.Clone(out) {
		if strings.HasPrefix(piece, "-") {
			for _, part := range flagSeparator.Split(piece, -1) {
				if part = strings.TrimSpace(part); part != "" {
					out = append(out, part)
				}
			}
		}
	}
	result := map[string]bool{}
	for _, x := range out {
		if x != "" {
			result[strings.ToLower(x)] = true
		}
	}
	return result
}

// pathAlternatives are the paths a data item names for a file: its name, its
// name without a trailing "(qualifier)" and that base's "a / b" or "a, b"
// pieces. A qualifier describes the file ("logfile (default stdout)"), so
// its words are no path: "restore output sidecars (-wal, -shm, -journal)"
// names no file "-wal".
func pathAlternatives(name string) map[string]bool {
	s := strings.TrimSpace(name)
	base := strings.TrimSpace(trailingQualifier.ReplaceAllString(s, ""))
	result := map[string]bool{s: true}
	if base != "" {
		result[base] = true
		for _, part := range nameSeparator.Split(base, -1) {
			if part = strings.TrimSpace(part); part != "" {
				result[part] = true
			}
		}
	}
	return result
}

// canonicalDestination is the report's grouping of outgoing calls: the
// destination text before a parenthesis.
func canonicalDestination(text string) string {
	base := strings.TrimSpace(text)
	if i := strings.Index(base, "("); i > 0 {
		base = strings.TrimSpace(base[:i])
	}
	return base
}

// candidate is an item or a trap a row may match.
type candidate struct {
	trap       bool
	ref        int
	component  string
	also       []string
	at         anchorAt
	names      map[string]bool
	paths      map[string]bool // a data item's file paths (pathAlternatives)
	nameFamily string          // inputs, data or none
	scope      string          // for a trap: the directory or file it covers entirely
	entrypoint bool
	external   bool
}

func rowNameFamily(row *reportRow) string {
	switch {
	case row.section == "inputs" || row.section == "config":
		return "inputs"
	case row.section == "data" && row.kind == "table":
		return "data"
	}
	return ""
}

// Match tiers, best first: 0 same line and same name, 1 same line, 2 same
// written name in the catalogue (or any entrypoint of the component), 3 one
// line away, 4 two lines away; 5 to 7 are the same positions of an outgoing
// call's ReachedFrom caller site. A row keeps only its best tier, so a row
// on an item's exact line is never also credited to that item's neighbours.
const noTier = -1

// callerTier ranks a ReachedFrom caller site's position after the row's own.
var callerTier = map[int]int{1: 5, 3: 6, 4: 7}

func positionTier(at anchorAt, cand candidate, sameName bool) int {
	if at.path != cand.at.path || at.line <= 0 {
		return noTier
	}
	switch d := abs(at.line - cand.at.line); d {
	case 0:
		if sameName {
			return 0
		}
		return 1
	case 1:
		return 3
	case 2:
		return 4
	}
	return noTier
}

func better(best, tier int) int {
	if tier != noTier && (best == noTier || tier < best) {
		return tier
	}
	return best
}

func tierFor(row *reportRow, cand candidate) int {
	best := noTier
	rowName := strings.ToLower(strings.TrimSpace(row.name))
	sameName := len(cand.names) > 0 && cand.names[rowName] && cand.nameFamily == rowNameFamily(row)
	if cand.scope != "" && row.at.path != "" &&
		(row.at.path == cand.scope || strings.HasPrefix(row.at.path, strings.TrimRight(cand.scope, "/")+"/")) {
		best = 1
	}
	if cand.at.path != "" && cand.at.line > 0 {
		best = better(best, positionTier(row.at, cand, sameName))
		// The one exception to "a row's own location only": an outgoing
		// call's ReachedFrom caller site (GroupsIndex 26). An external item
		// anchored where the program connects from states the same claim,
		// "X connects from caller C", as the row reached from C. It ranks
		// after every own-location tier (5 same line, 6 ±1, 7 ±2), so a row
		// is never taken from an item written at its own location.
		if !cand.trap && cand.external && row.section == "external" {
			for _, caller := range row.callers {
				if tier := positionTier(caller, cand, false); tier != noTier {
					best = better(best, callerTier[tier])
				}
			}
		}
	}
	if !cand.trap && cand.entrypoint && row.section == "entrypoints" &&
		(row.component == cand.component || slices.Contains(cand.also, row.component)) {
		// A program's way in: any entrypoint fact of the item's component.
		best = better(best, 2)
	}
	if sameName && (row.at.path == cand.at.path || !cand.trap && cand.nameFamily == "data") {
		// The same catalogue: a row written in the item's (trap's) file; a
		// table's catalogue is the component's data.
		best = better(best, 2)
	}
	if !cand.trap && cand.nameFamily == "data" && row.section == "data" && row.kind == "file" {
		best = better(best, fileTier(row, cand))
	}
	return best
}

// fileTier matches a file record to a data item by the file's written path,
// the same claim as the item's: the same path literal, exactly (redis's
// dump.rdb, the path server.dbfilename's write stores), is the item's
// anywhere in the component, as a table's name is (2); the write storing it
// is the item's own line (the inventory anchors dump.rdb at that write,
// redis.c:1493), the same line and name when that write stores the item's
// path (0), the same line when its path is not established (1:
// /tmp/redis-%p.vm, stored through zstrdup at redis.c:1503). No
// neighbouring line of a write counts.
func fileTier(row *reportRow, cand candidate) int {
	best := noTier
	named := func(path string) bool { return path != "" && cand.paths[strings.TrimSpace(path)] }
	if slices.ContainsFunc(row.paths, named) {
		best = 2
	}
	for _, write := range row.writes {
		if cand.at.line > 0 && write.at == cand.at {
			if named(write.path) {
				return 0
			}
			best = better(best, 1)
		}
	}
	return best
}

func bestCandidates(row *reportRow, cands []candidate) (int, []candidate) {
	best, chosen := noTier, []candidate(nil)
	for _, cand := range cands {
		tier := tierFor(row, cand)
		switch {
		case tier == noTier:
		case best == noTier || tier < best:
			best, chosen = tier, []candidate{cand}
		case tier == best:
			chosen = append(chosen, cand)
		}
	}
	return best, chosen
}

var trapToken = regexp.MustCompile(`[A-Za-z0-9_.\-/*]+`)

// trapScope is the directory or file a trap covers entirely: a path written
// in its name, or its component when that is no inventory component (tests/,
// mock/, _examples/), provided it contains the trap's anchor.
func trapScope(trap inventoryTrap, components map[string]bool) string {
	path := parseAnchor(trap.Anchor).path
	if path == "" {
		return ""
	}
	component := strings.TrimRight(trap.Component, "/")
	var tokens []string
	for _, token := range trapToken.FindAllString(trap.Name, -1) {
		if strings.Contains(strings.Trim(token, "/"), "/") || strings.HasSuffix(token, "/") {
			tokens = append(tokens, token)
		}
	}
	if component != "" && !components[component] {
		tokens = append(tokens, component)
	}
	scope := ""
	for _, token := range tokens {
		token = strings.TrimRight(strings.Trim(token, "`'\"()"), "/")
		if token == "" {
			continue
		}
		if (path == token || strings.HasPrefix(path, token+"/")) && len(token) > len(scope) {
			scope = token
		}
	}
	return scope
}

type hit struct {
	row  *reportRow
	tier int
	own  bool
}

type itemResult struct {
	index  int
	item   inventoryItem
	status string // found, kind_differs, other_component, missed
	hits   []hit
}

// flowStep is one saved Main flow step as the page shows it: its subject's
// declaration, or the anchor of the fact it cites.
type flowStep struct {
	n         int
	target    string
	component string
	at        anchorAt
	subject   string
}

type flowResult struct {
	item   inventoryItem
	status string // found, other_component, missed
	steps  []int
}

type cell struct {
	component, inventory                                              string
	must, found, kindDiffers, otherComponent, missed                  int
	may, mayFound, mayKindDiffers                                     int
	extras, trapHits, wrongKindRows, sameDestination, nested, unknown int
	missedItems, kindItems, otherItems                                []*itemResult
	extraRows, trapRows, wrongRows, nestedRows, unknownRows           []*reportRow
}

func (c *cell) empty() bool {
	return c.must == 0 && c.may == 0 && c.extras == 0 && c.trapHits == 0 && c.wrongKindRows == 0 && c.nested == 0 && c.unknown == 0
}

type scoreResult struct {
	inv          *inventory
	rows         []*reportRow
	results      []*itemResult
	steps        []flowStep
	flow         []*flowResult
	cells        []*cell
	components   []string
	compOfTarget map[string]string
	unmapped     []string
}

// score matches the report's rows against the inventory (design A4). It is
// deterministic: anchors and written names only, never a handler, a
// secondary anchor or a fuzzy match.
func score(run *auditRun, inv *inventory, rows []*reportRow) *scoreResult {
	var invComponents []string
	for _, item := range inv.Items {
		if !slices.Contains(invComponents, item.Component) {
			invComponents = append(invComponents, item.Component)
		}
	}
	componentSet := map[string]bool{}
	for _, component := range invComponents {
		componentSet[component] = true
	}
	compOfTarget := map[string]string{}
	mappedComponents := map[string]bool{}
	for _, target := range run.targets {
		if component := componentOf(target, invComponents, inv.entries); component != "" {
			compOfTarget[target.id] = component
			mappedComponents[component] = true
		}
	}
	display := map[string]string{}
	for _, target := range run.targets {
		display[target.id] = target.display
	}
	for _, row := range rows {
		if component := compOfTarget[row.target]; component != "" {
			row.component, row.mapped = component, true
		} else {
			row.component = "(report only) " + display[row.target]
		}
	}

	var itemCands []candidate
	for i, item := range inv.Items {
		if item.Flow {
			continue
		}
		family := ""
		switch item.Inventory {
		case "requests", "commands":
			family = "inputs"
		case "data":
			family = "data"
		}
		var names, paths map[string]bool
		if family != "" {
			names = nameAlternatives(item.Name, item.Inventory == "data")
		}
		if item.Inventory == "data" {
			paths = pathAlternatives(item.Name)
		}
		itemCands = append(itemCands, candidate{
			ref: i, component: item.Component, also: item.AlsoComponent, at: parseAnchor(item.Anchor),
			names: names, paths: paths, nameFamily: family, entrypoint: item.Kind == "entrypoint", external: item.Inventory == "external",
		})
	}
	var trapCands []candidate
	for j, trap := range inv.Not {
		// A trap names rows of any input-like family by name, in its own file.
		trapCands = append(trapCands, candidate{
			trap: true, ref: j, component: trap.Component, at: parseAnchor(trap.Anchor),
			names: nameAlternatives(trap.Name, false), nameFamily: "inputs", scope: trapScope(trap, componentSet),
		})
	}
	ownItem := func(cand candidate, component string) bool {
		return cand.component == component || slices.Contains(cand.also, component)
	}
	trapApplies := func(cand candidate, row *reportRow) bool {
		return !row.mapped || cand.component == row.component || !mappedComponents[cand.component]
	}

	itemHits := map[int][]hit{}
	for _, row := range rows {
		var own []candidate
		for _, cand := range itemCands {
			if ownItem(cand, row.component) {
				own = append(own, cand)
			}
		}
		if row.section != "entrypoints" {
			for _, cand := range trapCands {
				if trapApplies(cand, row) {
					own = append(own, cand)
				}
			}
		}
		if tier, chosen := bestCandidates(row, own); len(chosen) > 0 {
			var items, traps []int
			for _, cand := range chosen {
				if !cand.trap {
					items = append(items, cand.ref)
				}
			}
			// A tie goes to the item: a flag declared on the line its values
			// are written on is that flag, not its values.
			if len(items) == 0 {
				for _, cand := range chosen {
					traps = append(traps, cand.ref)
				}
			}
			for _, i := range items {
				itemHits[i] = append(itemHits[i], hit{row, tier, true})
			}
			row.tier, row.items, row.traps = tier, items, traps
			row.status = "match"
			if len(traps) > 0 {
				row.status = "trap"
			}
			continue
		}
		var other []candidate
		for _, cand := range itemCands {
			if !ownItem(cand, row.component) {
				other = append(other, cand)
			}
		}
		if tier, chosen := bestCandidates(row, other); len(chosen) > 0 {
			for _, cand := range chosen {
				itemHits[cand.ref] = append(itemHits[cand.ref], hit{row, tier, false})
				row.items = append(row.items, cand.ref)
			}
			row.tier, row.status = tier, "other"
			continue
		}
		row.tier = noTier
		row.status = "extra"
		if row.section == "entrypoints" {
			row.status = "entry"
		}
	}

	// An outgoing call to a destination already matched in the component is
	// that destination's call, not an extra.
	found := map[string]map[string]bool{}
	for _, row := range rows {
		if row.section == "external" && row.status == "match" && row.destination != "" {
			if found[row.component] == nil {
				found[row.component] = map[string]bool{}
			}
			found[row.component][canonicalDestination(row.destination)] = true
		}
	}
	for _, row := range rows {
		if row.section == "external" && row.status == "extra" && row.destination != "" && found[row.component][canonicalDestination(row.destination)] {
			row.status = "same_destination"
		}
	}
	// Fix 0: an input the product lists under another input (Launch.Nested)
	// is that input's value or check, not a row: it may still show an item
	// the reader finds under its parent, but it is never an extra or a trap
	// hit of its own.
	for _, row := range rows {
		if row.nested && (row.status == "extra" || row.status == "trap") {
			row.status = "nested"
		}
	}
	// A file whose path is not established claims no file: an honest
	// unknown, counted apart, never an extra.
	for _, row := range rows {
		if row.unknownPath && row.status == "extra" {
			row.status = "unknown"
		}
	}

	result := &scoreResult{inv: inv, rows: rows, compOfTarget: compOfTarget}
	for i, item := range inv.Items {
		if item.Flow {
			continue
		}
		hits := itemHits[i]
		expected := expectedKinds(item)
		var own, right int
		for _, h := range hits {
			if h.own {
				own++
				if expected[rowKindClass(h.row)] {
					right++
				}
			}
		}
		status := "missed"
		switch {
		case right > 0:
			status = "found"
		case own > 0:
			status = "kind_differs"
		case len(hits) > 0:
			status = "other_component"
		}
		result.results = append(result.results, &itemResult{index: i, item: item, status: status, hits: hits})
	}
	result.steps = flowSteps(run, compOfTarget)
	result.flow = scoreFlow(inv, result.steps)

	components := slices.Clone(invComponents)
	for _, row := range rows {
		if !slices.Contains(components, row.component) {
			components = append(components, row.component)
		}
	}
	cells := map[[2]string]*cell{}
	var order []*cell
	for _, component := range components {
		for _, name := range inventories {
			c := &cell{component: component, inventory: name}
			cells[[2]string{component, name}] = c
			order = append(order, c)
		}
	}
	for _, res := range result.results {
		c := cells[[2]string{res.item.Component, res.item.Inventory}]
		if !res.item.Must {
			c.may++
			switch res.status {
			case "found":
				c.mayFound++
			case "kind_differs":
				c.mayKindDiffers++
			}
			continue
		}
		c.must++
		switch res.status {
		case "found":
			c.found++
		case "kind_differs":
			c.kindDiffers++
			c.kindItems = append(c.kindItems, res)
		case "other_component":
			c.otherComponent++
			c.otherItems = append(c.otherItems, res)
		case "missed":
			c.missed++
			c.missedItems = append(c.missedItems, res)
		}
	}
	for _, row := range rows {
		c := cells[[2]string{row.component, row.family}]
		switch row.status {
		case "extra":
			c.extras++
			c.extraRows = append(c.extraRows, row)
		case "trap":
			c.trapHits++
			c.trapRows = append(c.trapRows, row)
		case "same_destination":
			c.sameDestination++
		case "nested":
			c.nested++
			c.nestedRows = append(c.nestedRows, row)
		case "unknown":
			c.unknown++
			c.unknownRows = append(c.unknownRows, row)
		case "match":
			if row.nested || len(row.items) == 0 {
				continue
			}
			wrong := true
			var expected []string
			for _, i := range row.items {
				if expectedKinds(inv.Items[i])[rowKindClass(row)] {
					wrong = false
				}
				expected = append(expected, inv.Items[i].Inventory+":"+inv.Items[i].Kind)
			}
			if wrong {
				slices.Sort(expected)
				row.expected = slices.Compact(expected)
				c.wrongKindRows++
				c.wrongRows = append(c.wrongRows, row)
			}
		}
	}
	for _, c := range order {
		if !c.empty() {
			result.cells = append(result.cells, c)
		}
	}
	result.components = components
	for _, component := range invComponents {
		if !mappedComponents[component] {
			result.unmapped = append(result.unmapped, component)
		}
	}
	return result
}

// componentOf is the inventory component whose program a report target is,
// by the target's key or its program's path, never its display name. A
// component's name is a program key or path (redis-server, freqtrade's
// build_helpers/create_command_partials.py, litestream's cmd/litestream);
// a declared component also names its entry file (othello-desktop starts at
// src/othello/core.clj, deps.edn's :run program, which the report calls
// "othello"). The first component in inventory order that one of the
// target's keys names is the target's.
func componentOf(target auditTarget, components []string, entries map[string]string) string {
	keys := map[string]bool{}
	for _, key := range target.keys {
		if key = normComponent(key); key != "" {
			keys[key] = true
		}
	}
	for _, component := range components {
		for _, name := range []string{component, entries[component]} {
			if key := normComponent(name); key != "" && keys[key] {
				return component
			}
		}
	}
	return ""
}

// scoreFlow scores the items marked flow against the saved Main flow: a
// step of the item's component within two lines of its anchor shows it.
func scoreFlow(inv *inventory, steps []flowStep) []*flowResult {
	var results []*flowResult
	for _, item := range inv.Items {
		if !item.Flow {
			continue
		}
		at := parseAnchor(item.Anchor)
		cand := candidate{at: at}
		res := &flowResult{item: item, status: "missed"}
		best, bestOther := noTier, noTier
		var own, other []int
		for _, step := range steps {
			tier := positionTier(step.at, cand, false)
			if tier == noTier || at.line <= 0 {
				continue
			}
			if step.component == item.Component || slices.Contains(item.AlsoComponent, step.component) {
				if best == noTier || tier < best {
					best, own = tier, nil
				}
				if tier == best {
					own = append(own, step.n)
				}
			} else {
				if bestOther == noTier || tier < bestOther {
					bestOther, other = tier, nil
				}
				if tier == bestOther {
					other = append(other, step.n)
				}
			}
		}
		switch {
		case len(own) > 0:
			res.status, res.steps = "found", own
		case len(other) > 0:
			res.status, res.steps = "other_component", other
		}
		results = append(results, res)
	}
	return results
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
