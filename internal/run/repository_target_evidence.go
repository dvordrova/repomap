package run

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

type repositoryNativeEvidence struct {
	Root             string
	Observations     []targetportfolio.Observation
	SeedOwners       []repositoryTargetKey
	SameLaunchOwners []repositoryTargetKey
	Consumers        []repositoryTargetKey
}

type repositoryNativeCandidate struct {
	Target    repositoryTypedTarget
	Row       targetportfolio.NativeCandidate
	Consumers []repositoryTargetKey
}

// repositoryToolingDirectories hold what surrounds a repository's programs,
// not the programs: agent hooks, CI workflows and actions, editor settings.
// A script found there is no target hypothesis. Morfeu's
// .claude/hooks/obsidian-session-context.py carries a main guard, reached
// the portfolio as a native target (launch_root, root ".") and ended every
// run with WARN "Target not analyzed".
var repositoryToolingDirectories = []string{".claude", ".github", ".vscode"}

// repositoryToolingPath reports whether a repository-relative file lies in a
// tooling directory at any depth.
func repositoryToolingPath(filePath string) bool {
	for _, segment := range strings.Split(path.Dir(filePath), "/") {
		if slices.Contains(repositoryToolingDirectories, segment) {
			return true
		}
	}
	return false
}

func repositoryToolingFileRef(repository *corpus.Corpus, fileRef corpus.FileID) bool {
	if repository == nil {
		return false
	}
	info, ok := repository.Info(fileRef)
	return ok && repositoryToolingPath(info.Entry.Path)
}

// withoutRepositoryToolingCandidates keeps the file hypotheses the portfolio
// may see: none from a tooling directory.
func withoutRepositoryToolingCandidates(repository *corpus.Corpus, candidates []analysistarget.FileCandidate) []analysistarget.FileCandidate {
	kept := make([]analysistarget.FileCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !repositoryToolingFileRef(repository, candidate.FileRef) {
			kept = append(kept, candidate)
		}
	}
	return kept
}

func withoutRepositoryToolingRefs(repository *corpus.Corpus, refs []corpus.FileID) []corpus.FileID {
	kept := make([]corpus.FileID, 0, len(refs))
	for _, ref := range refs {
		if !repositoryToolingFileRef(repository, ref) {
			kept = append(kept, ref)
		}
	}
	return kept
}

// repositoryNativeCandidates restores every exact native target and offers
// those whose representative file lies outside the tooling directories. A
// kept target may still name an excluded one as a seed owner; that owner is
// simply not offered, as it is no candidate the model could choose.
func repositoryNativeCandidates(repository *corpus.Corpus, discovery repositoryTargetDiscovery) ([]repositoryNativeCandidate, error) {
	var result []repositoryNativeCandidate
	tooling := make(map[repositoryTargetKey]struct{})
	for _, adapter := range discovery.adapters {
		rows, err := adapter.RestoreFiles(adapter.RequiredFileRefs)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if len(row.FileRefs) == 0 {
				return nil, fmt.Errorf("native target has no representative")
			}
			representative := row.FileRefs[0]
			for _, ref := range adapter.RequiredFileRefs {
				if slices.Contains(row.FileRefs, ref) {
					representative = ref
					break
				}
			}
			if repositoryToolingFileRef(repository, representative) {
				tooling[row.Target.Key] = struct{}{}
				continue
			}
			result = append(result, repositoryNativeCandidate{Target: row.Target, Row: targetportfolio.NativeCandidate{
				FileRef: representative, Language: string(adapter.Key), Kind: string(row.Target.Scope), Name: row.Target.Display,
			}})
		}
	}
	sort.Slice(result, func(i, j int) bool { return repositoryTypedTargetLess(result[i].Target, result[j].Target) })
	byKey := make(map[repositoryTargetKey]targetportfolio.NativeOwner)
	for i := range result {
		row := &result[i].Row
		row.Ref = "t" + strconv.Itoa(i+1)
		byKey[result[i].Target.Key] = targetportfolio.NativeOwner{Ref: row.Ref, Name: row.Name, Kind: row.Kind}
	}
	documents := guidanceCommandObservations(discovery.guidance)
	for i := range result {
		candidate := &result[i]
		adapter := discovery.byKey[candidate.Target.Key.Adapter]
		var facts repositoryNativeEvidence
		if adapter.NativeEvidence != nil {
			var err error
			facts, err = adapter.NativeEvidence(candidate.Target)
			if err != nil {
				return nil, err
			}
		}
		candidate.Row.Root = facts.Root
		candidate.Row.Evidence = facts.Observations
		candidate.Consumers = facts.Consumers
		for _, key := range facts.SeedOwners {
			if _, excluded := tooling[key]; excluded {
				continue
			}
			owner, ok := byKey[key]
			if !ok {
				return nil, fmt.Errorf("seed owner is outside the exact native catalogue")
			}
			owner.SameLaunch = slices.Contains(facts.SameLaunchOwners, key)
			candidate.Row.SeedOwners = append(candidate.Row.SeedOwners, owner)
		}
		for _, document := range documents {
			dir := path.Dir(document.Path)
			// A guide in this project or an ancestor is context, not an asserted
			// launch-to-target mapping. The model must interpret the quotation.
			if dir == "." || dir == facts.Root || strings.HasPrefix(facts.Root, dir+"/") {
				candidate.Row.Evidence = append(candidate.Row.Evidence, document)
			}
		}
	}
	return result, nil
}

// guidanceCommandObservations preserves complete fenced shell statements and
// import lines with their original address. Nothing is executed or promoted
// to a runtime fact, and continuation lines remain in the same statement.
func guidanceCommandObservations(guidance readmetargetscout.GuidanceSnapshot) []targetportfolio.Observation {
	var result []targetportfolio.Observation
	for _, document := range guidance.Documents {
		lines := strings.Split(document.Content, "\n")
		fence, language, heading := "", "", ""
		for i := 0; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if fence == "" && strings.HasPrefix(trimmed, "#") {
				heading = trimmed
			}
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				if fence != "" {
					if strings.HasPrefix(trimmed, fence) {
						fence, language = "", ""
					}
				} else {
					mark := trimmed[0]
					n := 0
					for n < len(trimmed) && trimmed[n] == mark {
						n++
					}
					fence, language = trimmed[:n], strings.ToLower(strings.TrimSpace(trimmed[n:]))
				}
				continue
			}
			if fence == "" || trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			isImport := strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "import(") || strings.HasPrefix(trimmed, "from ") || strings.Contains(trimmed, "require(")
			isCommand := false
			switch language {
			case "sh", "shell", "bash", "zsh", "console", "terminal", "shell-session", "cmd", "powershell":
				isCommand = true
			}
			for _, prefix := range []string{"$ ", "python ", "python3 ", "go run ", "go install ", "npm ", "yarn ", "pipenv run ", "uv run "} {
				isCommand = isCommand || strings.HasPrefix(trimmed, prefix)
			}
			if !isImport && !isCommand {
				continue
			}
			start, text := i+1, lines[i]
			for (strings.HasSuffix(strings.TrimSpace(lines[i]), "\\") || isImport && guidanceImportOpen(text)) && i+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), fence) {
				i++
				text += "\n" + lines[i]
			}
			kind := "documented_command"
			if isImport {
				kind = "documented_import"
			}
			result = append(result, targetportfolio.Observation{Kind: kind, Path: document.Path, Line: start, Values: []string{heading, text}})
		}
	}
	return result
}

// Fenced examples commonly group Go/Python imports or spread JS import names
// over several lines. Preserve that complete statement, including its module
// string. Brackets inside quoted names and comments do not extend the excerpt.
func guidanceImportOpen(text string) bool {
	depth := 0
	var quote rune
	escaped, comment := false, false
	chars := []rune(text)
	for i, char := range chars {
		if comment {
			comment = char != '\n'
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"', '`':
			quote = char
		case '#':
			comment = true
		case '/':
			comment = i+1 < len(chars) && chars[i+1] == '/'
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
	}
	return depth > 0
}

func pythonNativeEvidence(target pythontarget.Target, catalog pythontarget.Catalog, repository *corpus.Corpus) (repositoryNativeEvidence, error) {
	result := repositoryNativeEvidence{Root: target.ProjectDir}
	for _, basis := range target.Basis {
		result.Observations = append(result.Observations, targetportfolio.Observation{Kind: string(basis.Kind), Path: basis.Path, Line: basis.Line, Values: []string{basis.Label}})
	}
	for _, root := range target.Roots {
		result.Observations = append(result.Observations, targetportfolio.Observation{Kind: "launch_root", Path: root.Path, Line: root.Line, Values: []string{string(root.Kind), root.Module, root.Qualname}})
		fileRef, _ := repository.ID(root.Path)
		info, ok := repository.Info(fileRef)
		if !ok {
			return repositoryNativeEvidence{}, fmt.Errorf("Python launch source %q is absent from corpus", root.Path)
		}
		result.Observations = append(result.Observations, targetportfolio.Observation{Kind: "launch_file_executable", Path: root.Path, Values: []string{strconv.FormatBool(info.Entry.Executable)}})
		for _, module := range target.Modules {
			if module.Path == root.Path {
				result.Observations = append(result.Observations, targetportfolio.Observation{Kind: "declared_distribution_membership", Path: root.Path, Values: []string{module.Name, strconv.FormatBool(target.DeclaresModule(module))}})
				break
			}
		}
	}
	if entry, found := pythontarget.LaunchEntry(target); found {
		result.Observations = append(result.Observations, targetportfolio.Observation{
			Kind: "launch_callable", Path: entry.Path, Line: entry.Line,
			Values: []string{entry.Module, entry.Qualname, "arguments=none"},
		})
		for _, call := range target.LaunchCalls {
			result.Observations = append(result.Observations, targetportfolio.Observation{
				Kind: "launch_call_site", Path: call.Path, Line: call.Line,
				Values: []string{call.Entry.Module, call.Entry.Qualname, "arguments=none"},
			})
		}
	}
	for _, imported := range target.RelativeImports {
		result.Observations = append(result.Observations, targetportfolio.Observation{Kind: "module_level_relative_import", Path: imported.Path, Line: imported.Line, Values: []string{strings.Repeat(".", imported.Level) + imported.Module, imported.Name}})
	}
	for _, declaration := range target.DeclaredPackages {
		for _, column := range []struct {
			name   string
			values []string
		}{{"packages", declaration.Packages}, {"where", declaration.Where}, {"include", declaration.Include}, {"exclude", declaration.Exclude}} {
			result.Observations = append(result.Observations, targetportfolio.Observation{Kind: "distribution_" + declaration.Kind + "_" + column.name, Path: declaration.Path, Line: declaration.Line, Values: column.values})
		}
	}
	for _, owner := range catalog.Entries {
		if pythontarget.CanSeed(owner, target) {
			result.SeedOwners = append(result.SeedOwners, repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: owner.Ref})
			if pythontarget.SameLaunch(owner, target) {
				result.SameLaunchOwners = append(result.SameLaunchOwners, repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: owner.Ref})
			}
		}
	}
	return result, nil
}

func goNativeEvidence(target analysistarget.Target, facts gofacts.Facts, catalog analysistarget.TargetCatalog) repositoryNativeEvidence {
	result := repositoryNativeEvidence{Root: target.ModuleDir}
	for _, root := range target.Roots {
		result.Observations = append(result.Observations, targetportfolio.Observation{Kind: "go_main", Path: root.Path, Line: root.Line, Values: []string{target.PackagePath}})
	}
	if target.Kind != analysistarget.KindModuleLibrary {
		return result
	}
	public := make(map[string]bool)
	for _, pkg := range target.LibraryPackages {
		public[pkg.PackagePath] = true
	}
	imports := make(map[string][]string)
	for _, edge := range facts.InternalEdges {
		imports[edge.From] = append(imports[edge.From], edge.To)
	}
	uses := func(start string) bool {
		seen := make(map[string]bool)
		queue := append([]string(nil), imports[start]...)
		for len(queue) > 0 {
			next := queue[0]
			queue = queue[1:]
			if seen[next] {
				continue
			}
			seen[next] = true
			if public[next] {
				return true
			}
			queue = append(queue, imports[next]...)
		}
		return false
	}
	var mains, consumers, external []string
	for _, entry := range catalog.Entries {
		other := entry.Candidate.Target
		if other.Kind != analysistarget.KindExecutablePackage || !uses(other.PackagePath) {
			continue
		}
		result.Consumers = append(result.Consumers, repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: other.Ref})
		if other.ModuleID == target.ModuleID {
			consumers = append(consumers, other.PackagePath)
		}
	}
	for _, pkg := range facts.Packages {
		if pkg.ModuleID == target.ModuleID && pkg.Name == "main" {
			mains = append(mains, pkg.CanonicalPath)
		}
		if pkg.ModuleID != target.ModuleID {
			for _, imported := range imports[pkg.CanonicalPath] {
				if public[imported] {
					external = append(external, pkg.ModulePath+": "+pkg.CanonicalPath+" imports "+imported)
				}
			}
		}
	}
	for _, row := range []struct {
		name   string
		values []string
	}{{"own_main_packages", mains}, {"own_main_consumers", consumers}, {"other_module_imports", external}} {
		sort.Strings(row.values)
		result.Observations = append(result.Observations, targetportfolio.Observation{Kind: row.name, Path: path.Join(target.ModuleDir, "go.mod"), Values: append([]string{"count=" + strconv.Itoa(len(row.values))}, row.values...)})
	}
	return result
}

func nativeCandidateRows(candidates []repositoryNativeCandidate) []targetportfolio.NativeCandidate {
	rows := make([]targetportfolio.NativeCandidate, len(candidates))
	for i, candidate := range candidates {
		rows[i] = candidate.Row
	}
	return rows
}
