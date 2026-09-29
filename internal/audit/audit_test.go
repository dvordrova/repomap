// Package audit scores saved ordinary runs against reviewed ground truth
// (docs/contracts/DEVELOPMENT.md, Claim audit). It is test-only: no command,
// no provider call, no threshold.
//
// TestClaimAudit reads each run named in REPOMAP_AUDIT_RUNS through the
// product's own readers (report.ReadRunReceipt, groupindex Hydrate, atlas),
// matches its rows against testdata/audit/<repo>/inventory.json and writes
// <repo>.md and <repo>.json to REPOMAP_AUDIT_OUT. Recall and precision are
// reported, never asserted. The test fails only on a deterministic falsity:
// a Main flow step no earlier step leads to, a code name the repository does
// not have, a quote its cited file does not write, or a broken artifact
// chain.
//
// Matching is deterministic (design A4). A row keeps its best tier: exact
// line (with the same name first), the same written name in the item's
// catalogue, one line away, two lines away. Items win ties with traps. An
// entrypoint item matches any entrypoint of its component; a table name
// matches across the component's files. No handler, call chain or other
// secondary anchor is matched, with one exception: an outgoing call's
// ReachedFrom caller site (GroupsIndex 26) may match an external item
// anchored there, since "X connects from caller C" is the same claim either
// way; it ranks after every own-location tier. An input the product nests
// under another (Launch.Nested: a value or a sub-argument) is no row of its
// own: it may show an item, never an extra or a trap hit. Items marked flow
// are scored against the Main flow steps, not the workers inventory.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
)

type auditPair struct{ repo, dir string }

// parseAuditRuns reads "repo=run-dir" pairs separated by commas, colons or
// white space.
func parseAuditRuns(spec string) ([]auditPair, error) {
	var pairs []auditPair
	fields := strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == os.PathListSeparator || unicode.IsSpace(r) })
	for _, field := range fields {
		repo, dir, ok := strings.Cut(field, "=")
		if !ok || repo == "" || dir == "" {
			return nil, fmt.Errorf("REPOMAP_AUDIT_RUNS: %q is not repo=run-dir", field)
		}
		pairs = append(pairs, auditPair{repo, dir})
	}
	return pairs, nil
}

// resolved is path made absolute with the symbolic links of its longest
// existing ancestor resolved, so a directory not created yet compares too.
func resolved(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	rest := ""
	for dir := absolute; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, rest)
		}
		if filepath.Dir(dir) == dir {
			return absolute
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// auditOutputDir is REPOMAP_AUDIT_OUT, or a new temporary directory; never a
// run directory, the directory holding the runs, or the repomap cache.
func auditOutputDir(pairs []auditPair) (string, error) {
	out := os.Getenv("REPOMAP_AUDIT_OUT")
	if out == "" {
		return os.MkdirTemp("", "repomap-audit-")
	}
	out = resolved(out)
	var forbidden []string
	if cache, err := os.UserCacheDir(); err == nil {
		forbidden = append(forbidden, resolved(filepath.Join(cache, "repomap")))
	}
	for _, pair := range pairs {
		forbidden = append(forbidden, filepath.Dir(resolved(pair.dir)))
	}
	for _, dir := range forbidden {
		if within(out, dir) {
			return "", fmt.Errorf("REPOMAP_AUDIT_OUT %s is inside %s: the audit never writes into runs or the cache", out, dir)
		}
	}
	return out, os.MkdirAll(out, 0o755)
}

func TestClaimAudit(t *testing.T) {
	spec := os.Getenv("REPOMAP_AUDIT_RUNS")
	if strings.TrimSpace(spec) == "" {
		t.Skip("REPOMAP_AUDIT_RUNS=<repo>=<run dir>[,…] names the saved runs to audit")
	}
	pairs, err := parseAuditRuns(spec)
	if err != nil {
		t.Fatal(err)
	}
	out, err := auditOutputDir(pairs)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("claim audit output: %s", out)
	for _, pair := range pairs {
		t.Run(pair.repo, func(t *testing.T) {
			inv, err := loadInventory(filepath.Join("..", "..", "testdata", "audit", pair.repo, "inventory.json"))
			if err != nil {
				t.Fatalf("%s: %v", pair.repo, err)
			}
			run, err := readRun(pair.dir)
			if err != nil {
				t.Fatal(err)
			}
			if run.revision == "" || !strings.HasPrefix(run.revision, inv.Revision) && !strings.HasPrefix(inv.Revision, run.revision) {
				t.Fatalf("run %s is at revision %q, the %s inventory is pinned to %s", run.dir, run.revision, pair.repo, inv.Revision)
			}
			source, err := newGitTree(run.repoPath, run.revision)
			if err != nil {
				t.Fatalf("the audit reads the source at the run's revision: %v", err)
			}
			report, err := runAudit(pair.repo, run, inv, source)
			if err != nil {
				t.Fatal(err)
			}
			if err := report.write(out); err != nil {
				t.Fatal(err)
			}
			for _, line := range report.headline() {
				t.Log(line)
			}
			for _, falsity := range report.falsities() {
				t.Errorf("%s: %s", pair.repo, falsity)
			}
		})
	}
}

// auditReport is one repository's scored run and claim checks.
type auditReport struct {
	repo    string
	run     *auditRun
	score   *scoreResult
	flow    flowCheck
	names   namesCheck
	quotes  []quoteCheck
	recipes []recipeCheck
	unbound []*unboundComponent
}

func runAudit(repo string, run *auditRun, inv *inventory, source sourceTree) (*auditReport, error) {
	rows := reportRows(run)
	report := &auditReport{repo: repo, run: run, score: score(run, inv, rows), flow: checkFlow(run), unbound: checkUnbound(rows)}
	var err error
	if report.names, err = checkNames(run, source); err != nil {
		return nil, err
	}
	if report.quotes, err = checkQuotes(run, source); err != nil {
		return nil, err
	}
	if report.recipes, err = checkRecipes(run, source, rows); err != nil {
		return nil, err
	}
	return report, nil
}

// falsities are the deterministic wrongs: the only failures of the audit.
func (report *auditReport) falsities() []string {
	var result []string
	for _, link := range report.flow.links {
		if link.status == "BROKEN" || link.status == "NO_PROGRAM_INDEX" {
			result = append(result, fmt.Sprintf("Main flow step %d (%s %s): %s — %q", link.n, link.target, link.subject, link.status, link.text))
		}
	}
	for _, miss := range report.names.misses {
		result = append(result, fmt.Sprintf("invented name %q in %s %s: %q", miss.token, miss.typ, miss.where, miss.text))
	}
	for _, quote := range report.quotes {
		if quote.how == "MISSING" {
			result = append(result, fmt.Sprintf("run recipe %s quotes %q, which %v does not write", quote.target, quote.quote, quote.files))
		}
	}
	return result
}

type tally struct {
	must, found, kindDiffers, otherComponent, missed, may, mayFound, mayKindDiffers int
	extras, trapHits, wrongKindRows, nested, reportOnlyExtras, reportOnlyTrapHits   int
}

func (report *auditReport) inventoryTally(name string) tally {
	var t tally
	for _, c := range report.score.cells {
		if c.inventory != name {
			continue
		}
		if strings.HasPrefix(c.component, "(report only)") {
			t.reportOnlyExtras += c.extras
			t.reportOnlyTrapHits += c.trapHits
			continue
		}
		t.must += c.must
		t.found += c.found
		t.kindDiffers += c.kindDiffers
		t.otherComponent += c.otherComponent
		t.missed += c.missed
		t.may += c.may
		t.mayFound += c.mayFound
		t.mayKindDiffers += c.mayKindDiffers
		t.extras += c.extras
		t.trapHits += c.trapHits
		t.wrongKindRows += c.wrongKindRows
		t.nested += c.nested
	}
	return t
}

func (report *auditReport) headline() []string {
	var lines []string
	for _, name := range inventories {
		t := report.inventoryTally(name)
		lines = append(lines, fmt.Sprintf("%-9s must %4d found %4d kind≠ %3d other %3d missed %4d extras %4d traps %3d wrong-kind rows %3d nested %3d",
			name, t.must, t.found, t.kindDiffers, t.otherComponent, t.missed, t.extras, t.trapHits, t.wrongKindRows, t.nested))
	}
	found := 0
	for _, f := range report.score.flow {
		if f.status == "found" {
			found++
		}
	}
	lines = append(lines, fmt.Sprintf("main flow items %d/%d; flow links %v; wording flags %d; names %v; quotes %d",
		found, len(report.score.flow), report.flow.tally, len(report.flow.wording), report.names.counts, len(report.quotes)))
	return lines
}

func mdEscape(text string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(text)
}

func percent(n, d int) string {
	if d == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(d))
}

func itemLine(res *itemResult) string {
	extra := ""
	if len(res.item.AlsoComponent) > 0 {
		extra = " (also " + strings.Join(res.item.AlsoComponent, ", ") + ")"
	}
	return fmt.Sprintf("%s [%s] `%s`%s", mdEscape(res.item.Name), res.item.Kind, res.item.Anchor, extra)
}

func rowLine(row *reportRow, traps []inventoryTrap) string {
	s := fmt.Sprintf("%s/%s \"%s\" `%s`", row.section, row.kind, shorten(row.name, 60), row.at)
	if row.valueOf != "" {
		s += " (a value of " + row.valueOf + ")"
	}
	if row.handlerUnknown {
		s += " (handler unknown)"
	}
	if row.section == "external" && row.external != "" {
		s += " via " + row.external
	}
	if len(row.traps) > 0 && traps != nil {
		var parts []string
		for _, j := range row.traps {
			parts = append(parts, fmt.Sprintf("%s `%s`: %s", mdEscape(traps[j].Name), traps[j].Anchor, shorten(traps[j].Reason, 90)))
		}
		s += " — trap: " + strings.Join(parts, "; ")
	}
	return s
}

func hitLine(h hit) string {
	return fmt.Sprintf("%s · %s/%s \"%s\" `%s`", h.row.component, h.row.section, h.row.kind, shorten(h.row.name, 40), h.row.at)
}

func nearestInFile(res *itemResult, rows []*reportRow) string {
	at := parseAnchor(res.item.Anchor)
	if at.path == "" || at.line <= 0 {
		return ""
	}
	var best *reportRow
	bestD := -1
	for _, row := range rows {
		if row.at.path == at.path && row.at.line > 0 {
			if d := abs(row.at.line - at.line); bestD < 0 || d < bestD {
				best, bestD = row, d
			}
		}
	}
	if best == nil {
		return "no report row in this file"
	}
	return fmt.Sprintf("nearest row in file: %s/%s \"%s\" at :%d (Δ%d, %s)", best.section, best.kind, shorten(best.name, 40), best.at.line, bestD, best.component)
}

func sortedRows(rows []*reportRow) []*reportRow {
	out := slices.Clone(rows)
	slices.SortStableFunc(out, func(a, b *reportRow) int {
		if a.at.path != b.at.path {
			return strings.Compare(a.at.path, b.at.path)
		}
		return a.at.line - b.at.line
	})
	return out
}

func (report *auditReport) markdown() string {
	s, run, inv := report.score, report.run, report.score.inv
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	must, flowItems := 0, 0
	for _, item := range inv.Items {
		if item.Must {
			must++
		}
		if item.Flow {
			flowItems++
		}
	}
	w("# Claim audit: %s", report.repo)
	w("")
	w("- Run: `%s`", run.dir)
	w("- Revision: `%s` (inventory `%s`)", run.revision, inv.Revision)
	w("- Inventory: %d items (%d must, %d scored against the Main flow), %d traps, %d errata", len(inv.Items), must, flowItems, len(inv.Not), len(inv.Errata))
	var mapping []string
	for _, target := range run.targets {
		component := s.compOfTarget[target.id]
		if component == "" {
			component = "(no component: report only)"
		}
		mapping = append(mapping, fmt.Sprintf("%s %s → %s", target.id, target.display, component))
	}
	w("- Report targets → inventory components: %s", strings.Join(mapping, "; "))
	if len(s.unmapped) > 0 {
		w("- Inventory components with no report target (found only through `also_component` or another component): %s", strings.Join(s.unmapped, ", "))
	}
	w("- Written by internal/audit (TestClaimAudit); no provider call. Rows: the Inputs collection, Configuration reads, Entrypoints, outgoing calls and data records of each target.")
	w("- Matching: same component (or `also_component`); a row's own anchor in the item's file within 2 lines, or for requests/commands the same written name in the item's file (tables: the same name anywhere in the component). A row keeps only its best tier (exact line > same name > ±1 > ±2). An item wins a tie with a trap. An outgoing call's ReachedFrom caller site may match an external item anchored there, ranked after the row's own location. No handler or call-chain anchors. Outgoing calls to a destination already matched in the component are not extras.")
	w("- Nested: inputs the product lists under another input (Launch.Nested) are no rows of their own; they may show an item but are never extras or trap hits.")
	w("")
	w("## Headline per inventory")
	w("")
	w("Inventory components only; rows of report targets that map to no inventory component are totalled in the last two columns.")
	w("")
	w("| Inventory | must | found | kind differs | other component | missed | recall | may found (kind≠) / may | extras | trap hits | wrong-kind rows | nested | report-only extras | report-only trap hits |")
	w("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, name := range inventories {
		t := report.inventoryTally(name)
		w("| %s | %d | %d | %d | %d | %d | %s | %d (%d) / %d | %d | %d | %d | %d | %d | %d |", name, t.must, t.found, t.kindDiffers, t.otherComponent, t.missed,
			percent(t.found, t.must), t.mayFound, t.mayKindDiffers, t.may, t.extras, t.trapHits, t.wrongKindRows, t.nested, t.reportOnlyExtras, t.reportOnlyTrapHits)
	}
	w("")
	w("## Component × inventory")
	w("")
	w("| Component | Inventory | must | found | kind differs | other component | missed | may found (kind≠) / may | extras | trap hits | wrong-kind rows | nested |")
	w("|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, c := range s.cells {
		w("| %s | %s | %d | %d | %d | %d | %d | %d (%d) / %d | %d | %d | %d | %d |", mdEscape(c.component), c.inventory, c.must, c.found, c.kindDiffers, c.otherComponent, c.missed,
			c.mayFound, c.mayKindDiffers, c.may, c.extras, c.trapHits, c.wrongKindRows, c.nested)
	}
	w("")
	w("## Main flow items")
	w("")
	w("Items marked `flow` are the program's own foreground loops: a Main flow step of their component within two lines of the anchor shows them.")
	w("")
	w("| Component | Item | Anchor | must | Status | Steps |")
	w("|---|---|---|---|---|---|")
	for _, f := range s.flow {
		var steps []string
		for _, n := range f.steps {
			steps = append(steps, fmt.Sprint(n))
		}
		w("| %s | %s | `%s` | %v | %s | %s |", mdEscape(f.item.Component), mdEscape(f.item.Name), f.item.Anchor, f.item.Must, f.status, strings.Join(steps, ", "))
	}
	w("")
	w("## Details per table")
	w("")
	for _, c := range s.cells {
		if len(c.missedItems)+len(c.kindItems)+len(c.otherItems)+len(c.extraRows)+len(c.trapRows)+len(c.wrongRows)+len(c.nestedRows)+c.sameDestination == 0 {
			continue
		}
		w("### %s · %s", c.component, c.inventory)
		w("")
		if len(c.missedItems) > 0 {
			w("**Missed must items (%d)**", len(c.missedItems))
			w("")
			for _, res := range c.missedItems {
				w("- %s — %s", itemLine(res), nearestInFile(res, s.rows))
			}
			w("")
		}
		for _, list := range []struct {
			title string
			items []*itemResult
		}{{fmt.Sprintf("Found with the wrong kind (%d must) — expected %s", len(c.kindItems), c.inventory), c.kindItems},
			{fmt.Sprintf("Found only in another component (%d must)", len(c.otherItems)), c.otherItems}} {
			if len(list.items) == 0 {
				continue
			}
			w("**%s**", list.title)
			w("")
			for _, res := range list.items {
				var hits []string
				for _, h := range res.hits[:min(3, len(res.hits))] {
					hits = append(hits, hitLine(h))
				}
				w("- %s → %s", itemLine(res), strings.Join(hits, "; "))
			}
			w("")
		}
		if len(c.trapRows) > 0 {
			w("**Trap hits (%d rows)**", len(c.trapRows))
			w("")
			for _, row := range sortedRows(c.trapRows) {
				w("- %s", rowLine(row, inv.Not))
			}
			w("")
		}
		if len(c.wrongRows) > 0 {
			w("**Report rows matched to an inventory item of another kind: %d rows**", len(c.wrongRows))
			w("")
			type group struct {
				key  string
				rows []*reportRow
			}
			var groups []*group
			for _, row := range sortedRows(c.wrongRows) {
				key := row.section + "/" + row.kind + ", inventory says " + strings.Join(row.expected, ", ")
				i := slices.IndexFunc(groups, func(g *group) bool { return g.key == key })
				if i < 0 {
					groups = append(groups, &group{key: key})
					i = len(groups) - 1
				}
				groups[i].rows = append(groups[i].rows, row)
			}
			for _, g := range groups {
				var names []string
				for _, row := range g.rows[:min(10, len(g.rows))] {
					names = append(names, mdEscape(shorten(row.name, 30)))
				}
				more := ""
				if len(g.rows) > 10 {
					more = ", …"
				}
				w("- %d × report %s: %s%s (`%s`…`%s`)", len(g.rows), g.key, strings.Join(names, ", "), more, g.rows[0].at, g.rows[len(g.rows)-1].at)
			}
			w("")
		}
		if len(c.nestedRows) > 0 {
			w("**Listed under another input, not counted (%d rows)**", len(c.nestedRows))
			w("")
			for _, row := range sortedRows(c.nestedRows) {
				w("- %s", rowLine(row, inv.Not))
			}
			w("")
		}
		if c.sameDestination > 0 {
			w("_%d more report rows call a destination already matched in this component; not counted as extras._", c.sameDestination)
			w("")
		}
		if len(c.extraRows) > 0 {
			type group struct {
				key     string
				n       int
				anchors []string
				row     *reportRow
			}
			var groups []*group
			for _, row := range sortedRows(c.extraRows) {
				key := row.section + "/" + row.kind + " \"" + shorten(row.name, 60) + "\""
				switch {
				case row.section == "external":
					name := row.destination
					if name == "" {
						name = row.external
					}
					key = row.section + "/" + row.kind + " \"" + shorten(name, 60) + "\""
				case row.section == "data" && row.kind != "table":
					key = row.section + "/" + row.kind + " \"" + row.at.path + "\""
				}
				i := slices.IndexFunc(groups, func(g *group) bool { return g.key == key })
				if i < 0 {
					groups = append(groups, &group{key: key, row: row})
					i = len(groups) - 1
				}
				groups[i].n++
				if len(groups[i].anchors) < 3 {
					groups[i].anchors = append(groups[i].anchors, row.at.String())
				}
			}
			suffix := ""
			if len(groups) > 40 {
				suffix = " (first 40 groups)"
			}
			w("**Extras: %d rows in %d groups%s**", len(c.extraRows), len(groups), suffix)
			w("")
			for _, g := range groups[:min(40, len(groups))] {
				label := g.key
				if g.row.valueOf != "" {
					label += " (value of " + g.row.valueOf + ")"
				}
				if g.row.handlerUnknown && g.n == 1 {
					label += " (handler unknown)"
				}
				w("- %s ×%d `%s`", mdEscape(label), g.n, strings.Join(g.anchors, ", "))
			}
			w("")
		}
	}
	w("## Deterministic claim checks")
	w("")
	w("### AUTO-1 Main flow link — %v", report.flow.tally)
	w("")
	w("| # | Target | Subject | Anchor | Link to an earlier step |")
	w("|---|---|---|---|---|")
	for _, link := range report.flow.links {
		w("| %d | %s | %s | `%s` | %s |", link.n, link.target, mdEscape(link.subject), link.anchor, link.status)
	}
	w("")
	w("### AUTO-3 \"calls\" wording versus the code relation — %d flagged", len(report.flow.wording))
	w("")
	for _, flag := range report.flow.wording {
		w("- step %d (%s): says \"%s\", code: %s. \"%s\"", flag.n, flag.subject, flag.says, flag.code, shorten(flag.text, 160))
	}
	w("")
	w("### AUTO-2 Names exist — %v", report.names.counts)
	w("")
	w("| Type | Resolved as |")
	w("|---|---|")
	for _, typ := range report.names.types {
		w("| %s | %v |", typ, report.names.perType[typ])
	}
	w("")
	if len(report.names.misses) > 0 {
		w("Unresolved (%d):", len(report.names.misses))
		w("")
		for _, miss := range report.names.misses {
			w("- %s %s: `%s` — \"%s\"", miss.typ, miss.where, miss.token, mdEscape(miss.text))
		}
		w("")
	}
	w("### AUTO-4 Quotes in run recipe notes — %d", len(report.quotes))
	w("")
	for _, quote := range report.quotes {
		w("- %s \"%s\": %s in %s", quote.target, mdEscape(quote.quote), quote.how, strings.Join(quote.files, ", "))
	}
	w("")
	w("### AUTO-5 Run recipe")
	w("")
	for _, recipe := range report.recipes {
		var words []string
		for _, word := range recipe.words {
			words = append(words, fmt.Sprintf("`%s` %s", word.word, word.resolved))
		}
		text := strings.Join(words, "; ")
		if text == "" {
			text = "no flags or words"
		}
		w("- %s `%s` — program `%s`: %s; %s", recipe.target, recipe.command, recipe.program, recipe.programResolved, text)
	}
	w("")
	w("### AUTO-6 Inputs with no handler")
	w("")
	w("| Component | Inputs | With handler | Handler unknown, by kind | Unreachable handlers |")
	w("|---|---|---|---|---|")
	for _, entry := range report.unbound {
		w("| %s | %d | %d | %v | %d |", mdEscape(entry.component), entry.inputs, entry.withHandler, entry.handlerUnknown, len(entry.unreachable))
	}
	w("")
	flagged := false
	for _, entry := range report.unbound {
		for _, c := range entry.catalogues {
			if c.kinds["request"] > 0 || c.kinds["interaction"] > 0 {
				if !flagged {
					w("Flagged for the verifier: requests/interactions whose registration hands over no callable, by catalogue:")
					w("")
					flagged = true
				}
				w("- %s: %d rows %v — %s `%s` e.g. %s", mdEscape(entry.component), c.n, c.kinds, mdEscape(c.name), c.first, mdEscape(strings.Join(c.names, ", ")))
			}
		}
	}
	if flagged {
		w("")
	}
	type otherCatalogue struct {
		component string
		c         *catalogue
	}
	var others []otherCatalogue
	for _, entry := range report.unbound {
		for _, c := range entry.catalogues {
			if c.kinds["request"] == 0 && c.kinds["interaction"] == 0 {
				others = append(others, otherCatalogue{entry.component, c})
			}
		}
	}
	if len(others) > 0 {
		slices.SortStableFunc(others, func(a, b otherCatalogue) int { return b.c.n - a.c.n })
		w("Other handler-unknown catalogues (%d), largest first:", len(others))
		w("")
		for _, other := range others[:min(25, len(others))] {
			w("- %s: %d %v — %s `%s`", mdEscape(other.component), other.c.n, other.c.kinds, mdEscape(other.c.name), other.c.first)
		}
		w("")
	}
	for _, entry := range report.unbound {
		if len(entry.unreachable) == 0 {
			continue
		}
		var parts []string
		for _, row := range entry.unreachable[:min(20, len(entry.unreachable))] {
			parts = append(parts, fmt.Sprintf("%s %s → %s `%s`", row.kind, row.name, row.handler, row.at))
		}
		w("Unreachable handlers in %s: %s", entry.component, strings.Join(parts, "; "))
		w("")
	}
	return b.String()
}

// summary is <repo>.json: the counts and item statuses, for comparing two
// audits mechanically.
func (report *auditReport) summary() any {
	type itemOut struct {
		Component, Inventory, Kind, Name, Anchor, Status string
		Must                                             bool
		Rows                                             []string `json:",omitempty"`
	}
	type cellOut struct {
		Component, Inventory                                                     string
		Must, Found, KindDiffers, OtherComponent, Missed, May, MayFound, MayKind int
		Extras, TrapHits, WrongKindRows, SameDestination, Nested                 int
		ExtraRows, TrapRows, NestedRows                                          []string `json:",omitempty"`
	}
	rowID := func(row *reportRow) string {
		return fmt.Sprintf("%s %s/%s %q %s", row.target, row.section, row.kind, row.name, row.at)
	}
	var items []itemOut
	for _, res := range report.score.results {
		out := itemOut{res.item.Component, res.item.Inventory, res.item.Kind, res.item.Name, res.item.Anchor, res.status, res.item.Must, nil}
		for _, h := range res.hits {
			out.Rows = append(out.Rows, rowID(h.row))
		}
		items = append(items, out)
	}
	for _, f := range report.score.flow {
		items = append(items, itemOut{f.item.Component, "main flow", f.item.Kind, f.item.Name, f.item.Anchor, f.status, f.item.Must, nil})
	}
	var cells []cellOut
	for _, c := range report.score.cells {
		out := cellOut{c.component, c.inventory, c.must, c.found, c.kindDiffers, c.otherComponent, c.missed, c.may, c.mayFound, c.mayKindDiffers,
			c.extras, c.trapHits, c.wrongKindRows, c.sameDestination, c.nested, nil, nil, nil}
		for _, row := range c.extraRows {
			out.ExtraRows = append(out.ExtraRows, rowID(row))
		}
		for _, row := range c.trapRows {
			out.TrapRows = append(out.TrapRows, rowID(row))
		}
		for _, row := range c.nestedRows {
			out.NestedRows = append(out.NestedRows, rowID(row))
		}
		cells = append(cells, out)
	}
	return map[string]any{
		"repo": report.repo, "run_dir": report.run.dir, "revision": report.run.revision,
		"cells": cells, "items": items, "flow_links": report.flow.tally, "wording_flags": len(report.flow.wording),
		"names": report.names.counts, "falsities": report.falsities(),
	}
}

func (report *auditReport) write(dir string) error {
	if err := os.WriteFile(filepath.Join(dir, report.repo+".md"), []byte(report.markdown()), 0o644); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report.summary(), "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, report.repo+".json"), append(encoded, '\n'), 0o644)
}
