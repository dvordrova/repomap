package audit

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/terminology"
)

// auditRun is one saved ordinary run as the product's own readers restore
// it: every target's ProgramIndex and hydrated GroupsIndex (with the derived
// Launch.Nested), the facts, orientation, atlas and glossary.
type auditRun struct {
	dir         string
	revision    string
	repoPath    string
	targets     []auditTarget
	facts       facts.Result
	orientation *orientation.Result
	atlas       *atlas.Atlas
	glossary    *terminology.Catalog
}

type auditTarget struct {
	id      string
	display string
	// keys name the target's program: its target key and the files its
	// program starts at (see targetKeys); never its display name.
	keys    []string
	program programindex.Index
	index   groupindex.Index
	objects map[string]programindex.Object
}

func (run *auditRun) target(id string) *auditTarget {
	for i := range run.targets {
		if run.targets[i].id == id {
			return &run.targets[i]
		}
	}
	return nil
}

// readRun restores a run through report.ReadRunReceipt: report.json with the
// files it names (the sibling target runs' ProgramIndexes included), each
// checked against the digest it was written with. Every overlay is hydrated
// again with groupindex's own Hydrate, which derives Launch.Nested.
func readRun(dir string) (*auditRun, error) {
	receipt, err := report.ReadRunReceipt(dir)
	if err != nil {
		return nil, fmt.Errorf("read run %s: %w", dir, err)
	}
	data := receipt.Data()
	if data == nil || data.GroupGraph == nil || data.ProgramPortfolio == nil || data.Facts == nil {
		return nil, fmt.Errorf("read run %s: report data, group graph, programs or facts are missing", dir)
	}
	root, err := receipt.Manifest().ResolveAnalysisRoot()
	if err != nil {
		return nil, err
	}
	run := &auditRun{
		dir: receipt.RunDir(), revision: data.CapturedRevision, repoPath: root,
		facts: *data.Facts, orientation: data.Orientation, glossary: data.Glossary,
	}
	if run.revision == "" {
		run.revision = receipt.Manifest().RepositoryState.Head
	}
	programs := map[string]programindex.Index{}
	if err := data.ProgramPortfolio.ReadProgramIndexes(func(program programindex.Index) error {
		programs[program.Target.ID] = program
		return nil
	}); err != nil {
		return nil, err
	}
	display := map[string]string{}
	if data.TargetOutcomePortfolio != nil {
		for _, outcome := range data.TargetOutcomePortfolio.Outcomes {
			if outcome.ProgramTargetID != "" {
				display[outcome.ProgramTargetID] = outcome.DisplayName
			}
		}
	}
	for _, overlay := range data.GroupGraph.Indexes {
		program, ok := programs[overlay.TargetID]
		if !ok {
			return nil, fmt.Errorf("read run %s: no ProgramIndex for %s", dir, overlay.TargetID)
		}
		index, err := overlay.Hydrate(program)
		if err != nil {
			return nil, fmt.Errorf("read run %s: hydrate %s: %w", dir, overlay.TargetID, err)
		}
		name := display[overlay.TargetID]
		if name == "" {
			name = program.Target.Name
		}
		run.targets = append(run.targets, newAuditTarget(overlay.TargetID, name, program, index))
	}
	saved, err := atlas.Read(dir)
	if err != nil {
		return nil, err
	}
	run.atlas = &saved
	return run, nil
}

func newAuditTarget(id, display string, program programindex.Index, index groupindex.Index) auditTarget {
	objects := make(map[string]programindex.Object, len(program.Objects))
	for _, object := range program.Objects {
		objects[object.ID] = object
	}
	return auditTarget{id: id, display: display, keys: targetKeys(program.Target), program: program, index: index, objects: objects}
}

// targetKeys are the ways a report target names its program: its target
// key (the selector, c:redis-server, and its last segment, redis-server)
// and each file its program starts at (a seed, src/othello/core.clj) with
// that file's directory (cmd/litestream of cmd/litestream/main.go).
func targetKeys(target programindex.Target) []string {
	var keys []string
	if target.Selector != "" {
		keys = append(keys, target.Selector)
		if i := strings.LastIndex(target.Selector, ":"); i >= 0 {
			keys = append(keys, target.Selector[i+1:])
		}
	}
	for _, seed := range target.Seeds {
		if seed.Location != nil && seed.Location.Path != "" {
			keys = append(keys, seed.Location.Path, path.Dir(seed.Location.Path))
		}
	}
	return keys
}

func locationAt(location *programindex.Location) anchorAt {
	if location == nil {
		return anchorAt{}
	}
	return anchorAt{location.Path, location.Line}
}

func factAt(anchor *facts.Anchor) anchorAt {
	if anchor == nil {
		return anchorAt{}
	}
	return anchorAt{anchor.Path, anchor.Line}
}

// reportRows lists the rows of every component's inventories: the Inputs
// collection (GroupsIndex operations), outgoing calls, data records, then
// the Entrypoints and Configuration facts.
func reportRows(run *auditRun) []*reportRow {
	var rows []*reportRow
	known := map[string]bool{}
	for i := range run.targets {
		target := &run.targets[i]
		known[target.id] = true
		index := &target.index
		names := map[string]string{}
		for _, operation := range index.Operations {
			names[operation.ID] = operation.Name
		}
		for _, operation := range index.Operations {
			family, ok := operationFamily[operation.Kind]
			if !ok {
				family = "commands"
			}
			row := &reportRow{
				id: target.id + "/" + operation.ID, target: target.id, section: "inputs", family: family,
				kind: operation.Kind, name: operation.Name,
				at:             anchorAt{operation.Location.Path, operation.Location.Line},
				handlerUnknown: operation.HandlerUnknown, valueOf: names[operation.ValueOf],
				nested: index.Launch.Nested[operation.ID],
			}
			if subject, ok := target.objects[operation.SubjectID]; ok && operation.SubjectID != "" {
				row.handler, row.handlerUnreachable = subject.Name, subject.Unreachable
			}
			if declaredBy, ok := target.objects[operation.DeclaredBy]; ok && operation.DeclaredBy != "" {
				row.declaredBy = declaredBy.Name
			}
			if operation.DeclaredOn != nil {
				row.declaredOn = operation.DeclaredOn.Text
			}
			rows = append(rows, row)
		}
		for _, call := range index.Outbound {
			row := &reportRow{
				id: target.id + "/" + call.ID, target: target.id, section: "external", family: "external",
				kind: call.Kind, name: call.Destination, destination: call.Destination, external: call.External,
				at: anchorAt{call.Location.Path, call.Location.Line},
			}
			if row.name == "" {
				row.name = call.External
			}
			for _, caller := range call.ReachedFrom {
				if caller.Location != nil {
					row.callers = append(row.callers, locationAt(caller.Location))
				}
			}
			rows = append(rows, row)
		}
		for _, record := range index.Data {
			row := &reportRow{
				id: target.id + "/" + record.ID, target: target.id, section: "data", family: "data",
				kind: "data", at: anchorAt{record.Path, record.Line},
			}
			if data := record.Data; data != nil {
				if data.Kind != "" {
					row.kind = data.Kind
				}
				row.name = data.Name
				if data.Schema != "" {
					row.name = data.Schema + "." + data.Name
				}
				// A file record claims the path as written: its literal
				// or template, and each write of the field it is read
				// from with the path that write stores. No name is a path
				// not established.
				if data.Kind == "file" && data.File != nil {
					row.unknownPath = data.Name == ""
					if data.Name != "" {
						row.paths = append(row.paths, data.Name)
					}
					for _, value := range data.File.Values {
						if value.Value != "" {
							row.paths = append(row.paths, value.Value)
						}
						row.writes = append(row.writes, fileWrite{at: anchorAt{value.Anchor.Path, value.Anchor.Line}, path: value.Value})
					}
				}
			}
			rows = append(rows, row)
		}
	}
	// A program's way in is its Entrypoints list, never an extra.
	for _, fact := range run.facts.Facts {
		if fact.Kind != facts.KindEntrypoint || !known[fact.TargetID] {
			continue
		}
		name := fact.Symbol
		if name == "" {
			name = fact.Key
		}
		rows = append(rows, &reportRow{
			id: fact.TargetID + "/" + fact.ID, target: fact.TargetID, section: "entrypoints", family: "commands",
			kind: "entry", name: name, at: factAt(fact.Anchor),
		})
	}
	// The component's Configuration table: its config reads.
	for _, fact := range run.facts.Facts {
		if fact.Kind != facts.KindConfigRead || !known[fact.TargetID] {
			continue
		}
		rows = append(rows, &reportRow{
			id: fact.TargetID + "/" + fact.ID, target: fact.TargetID, section: "config", family: "commands",
			kind: "config", name: fact.Key, at: factAt(fact.Anchor),
		})
	}
	return rows
}

// flowSteps are the saved Main flow steps with the anchor the page gives
// each: the subject's declaration, or the cited fact's anchor.
func flowSteps(run *auditRun, compOfTarget map[string]string) []flowStep {
	if run.orientation == nil {
		return nil
	}
	factByID := map[string]facts.Fact{}
	for _, fact := range run.facts.Facts {
		factByID[fact.ID] = fact
	}
	var steps []flowStep
	for i, step := range run.orientation.MainFlow.Steps {
		s := flowStep{n: i + 1, target: step.TargetID, component: compOfTarget[step.TargetID]}
		if target := run.target(step.TargetID); target != nil && step.SubjectID != "" {
			if object, ok := target.objects[step.SubjectID]; ok {
				s.at, s.subject = locationAt(object.Location), object.Name
			}
		} else if fact, ok := factByID[step.FactID]; ok && step.FactID != "" {
			s.at, s.subject = factAt(fact.Anchor), fact.Symbol
		}
		steps = append(steps, s)
	}
	return steps
}

// sourceTree is the repository at the run's revision, read through git.
type sourceTree interface {
	isPath(word string) bool
	// grep reports whether literal is written in paths (the whole tree
	// when paths is empty).
	grep(literal string, paths []string) (bool, error)
	read(path string) (string, error)
}

type gitTree struct {
	repo, revision string
	paths          map[string]bool
	dirs           map[string]bool
	suffixes       map[string]bool
	greps          map[string]bool
}

func newGitTree(repo, revision string) (*gitTree, error) {
	out, err := exec.Command("git", "-C", repo, "ls-tree", "-r", "--name-only", revision).Output()
	if err != nil {
		return nil, fmt.Errorf("list %s at %s: %w", repo, revision, err)
	}
	tree := &gitTree{repo: repo, revision: revision, paths: map[string]bool{}, dirs: map[string]bool{}, suffixes: map[string]bool{}, greps: map[string]bool{}}
	for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if path == "" {
			continue
		}
		tree.paths[path] = true
		parts := strings.Split(path, "/")
		for k := 1; k < len(parts); k++ {
			tree.dirs[strings.Join(parts[:k], "/")] = true
			tree.suffixes[strings.Join(parts[k:], "/")] = true
		}
	}
	return tree, nil
}

func (tree *gitTree) isPath(word string) bool {
	if strings.HasPrefix(word, "./") {
		word = strings.Trim(word, "./")
	}
	return tree.paths[word] || tree.dirs[word] || tree.suffixes[word]
}

func (tree *gitTree) grep(literal string, paths []string) (bool, error) {
	paths = slices.Sorted(slices.Values(paths))
	key := literal + "\x00" + strings.Join(paths, "\x00")
	if found, ok := tree.greps[key]; ok {
		return found, nil
	}
	chunks := [][]string{nil}
	if len(paths) > 0 {
		chunks = nil
		for start := 0; start < len(paths); start += 400 {
			chunks = append(chunks, paths[start:min(start+400, len(paths))])
		}
	}
	found := false
	for _, chunk := range chunks {
		args := append([]string{"-C", tree.repo, "grep", "-q", "-F", "-e", literal, tree.revision, "--"}, chunk...)
		err := exec.Command("git", args...).Run()
		var exit *exec.ExitError
		if err == nil {
			found = true
			break
		}
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return false, fmt.Errorf("git grep %q: %w", literal, err)
		}
	}
	tree.greps[key] = found
	return found, nil
}

func (tree *gitTree) read(path string) (string, error) {
	var out bytes.Buffer
	command := exec.Command("git", "-C", tree.repo, "show", tree.revision+":"+filepath.ToSlash(path))
	command.Stdout = &out
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("read %s at %s: %w", path, tree.revision, err)
	}
	return out.String(), nil
}
