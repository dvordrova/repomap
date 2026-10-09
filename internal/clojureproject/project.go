// Package clojureproject adapts clj-kondo's native JVM Clojure analysis to the
// ordinary ProgramIndex. ClojureScript is a different execution view; .cljc
// contributes only its :clj branch here.
package clojureproject

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	p "github.com/dvordrova/repomap/internal/programindex"
)

type Target struct {
	Ref, Selector, Name, ProjectDir, ManifestPath, ManifestFileRef, CorpusSHA256 string
	Files                                                                        []corpus.Entry
	// TestDirs are the directories the build description runs as tests
	// (Manifest.TestDirectories).
	TestDirs []string `json:",omitempty"`
	// Platform is "cljs" for a shadow-cljs build, whose view is its
	// .cljs sources and the :cljs branch of its .cljc sources; empty for
	// the JVM project.
	Platform string `json:",omitempty"`
	// Build is the shadow-cljs build's id and Entries what it starts from.
	Build   string  `json:",omitempty"`
	Entries []Entry `json:",omitempty"`
}

// manifestNames are the build descriptions that bound a Clojure project's
// sources.
func manifestName(base string) bool {
	return base == "deps.edn" || base == "project.clj" || base == "shadow-cljs.edn"
}

// ownedFiles are the sources with one of the extensions under dir whose
// nearest enclosing directory holding a manifest is dir itself.
func ownedFiles(repository *corpus.Corpus, dir string, boundaries map[string]bool, extensions ...string) []corpus.Entry {
	var files []corpus.Entry
	for _, entry := range repository.Entries() {
		if !slices.Contains(extensions, path.Ext(entry.Path)) || path.Base(entry.Path) == "project.clj" {
			continue
		}
		if dir != "." && !strings.HasPrefix(entry.Path, dir+"/") {
			continue
		}
		owner := dir
		for parent := path.Dir(entry.Path); parent != dir && parent != "."; parent = path.Dir(parent) {
			if boundaries[parent] {
				owner = parent
				break
			}
		}
		if owner == dir {
			files = append(files, entry)
		}
	}
	slices.SortFunc(files, func(a, b corpus.Entry) int { return strings.Compare(a.Path, b.Path) })
	return files
}

// testDirectories are the test directories the deps.edn and project.clj of
// dir name.
func testDirectories(repository *corpus.Corpus, dir string) ([]string, error) {
	var dirs []string
	for _, name := range []string{"deps.edn", "project.clj"} {
		ref, ok := repository.ID(path.Join(dir, name))
		if !ok {
			continue
		}
		content, err := repository.ReadFileAll(ref)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, ReadManifest(name, content.Bytes).TestDirectories(name, dir)...)
	}
	slices.Sort(dirs)
	return slices.Compact(dirs), nil
}

func (target *Target) seal() {
	raw, _ := json.Marshal(target)
	target.Ref = fmt.Sprintf("clojure-%x", sha256.Sum256(raw))
}

func Scout(repository *corpus.Corpus, name string) ([]Target, error) {
	if repository == nil {
		return nil, fmt.Errorf("Clojure: repository is required")
	}
	// deps.edn and project.clj in the same directory describe one project.
	manifests := map[string]corpus.Entry{}
	for _, entry := range repository.Entries() {
		base := path.Base(entry.Path)
		if base != "deps.edn" && base != "project.clj" {
			continue
		}
		dir := path.Dir(entry.Path)
		if previous, ok := manifests[dir]; !ok || path.Base(previous.Path) != "deps.edn" {
			manifests[dir] = entry
		}
	}
	boundaries := map[string]bool{}
	for dir := range manifests {
		boundaries[dir] = true
	}
	var targets []Target
	for dir, manifest := range manifests {
		target := Target{Selector: "clojure:" + manifest.Path, Name: name, ProjectDir: dir, ManifestPath: manifest.Path, ManifestFileRef: string(manifest.ID), CorpusSHA256: repository.SHA256()}
		if dir != "." {
			target.Name = dir
		}
		target.Files = ownedFiles(repository, dir, boundaries, ".clj", ".cljc")
		if len(target.Files) == 0 {
			continue
		}
		dirs, err := testDirectories(repository, dir)
		if err != nil {
			return nil, err
		}
		target.TestDirs = dirs
		target.seal()
		targets = append(targets, target)
	}
	slices.SortFunc(targets, func(a, b Target) int { return strings.Compare(a.Selector, b.Selector) })
	return targets, nil
}

// ScoutShadow finds the ClojureScript programs a shadow-cljs.edn builds:
// each build naming what it starts from (a module's :init-fn or :entries,
// a build's :entries, a node script's :main) is a target of its own, whose view is the .cljs
// sources and the :cljs branch of the .cljc sources the shadow-cljs.edn's
// directory owns, up to a nested build description.
func ScoutShadow(repository *corpus.Corpus) ([]Target, error) {
	if repository == nil {
		return nil, fmt.Errorf("Clojure: repository is required")
	}
	boundaries := map[string]bool{}
	var configs []corpus.Entry
	for _, entry := range repository.Entries() {
		if manifestName(path.Base(entry.Path)) {
			boundaries[path.Dir(entry.Path)] = true
		}
		if path.Base(entry.Path) == "shadow-cljs.edn" {
			configs = append(configs, entry)
		}
	}
	var targets []Target
	for _, config := range configs {
		content, err := repository.ReadFileAll(config.ID)
		if err != nil {
			return nil, err
		}
		dir := path.Dir(config.Path)
		files := ownedFiles(repository, dir, boundaries, ".cljs", ".cljc")
		if len(files) == 0 {
			continue
		}
		dirs, err := testDirectories(repository, dir)
		if err != nil {
			return nil, err
		}
		for _, build := range ReadManifest("shadow-cljs.edn", content.Bytes).Builds {
			if len(build.Entries) == 0 || !p.ValidName(build.ID) {
				continue
			}
			target := Target{Selector: "clojure:" + config.Path + ":" + build.ID, Name: path.Join(dir, build.ID), ProjectDir: dir,
				ManifestPath: config.Path, ManifestFileRef: string(config.ID), CorpusSHA256: repository.SHA256(),
				Files: files, TestDirs: dirs, Platform: "cljs", Build: build.ID, Entries: build.Entries}
			target.seal()
			targets = append(targets, target)
		}
	}
	slices.SortFunc(targets, func(a, b Target) int { return strings.Compare(a.Selector, b.Selector) })
	return targets, nil
}

func (target Target) Validate() error {
	selector := "clojure:" + target.ManifestPath
	if target.Build != "" {
		selector += ":" + target.Build
	}
	if target.Ref == "" || target.Name == "" || target.Selector != selector || len(target.Files) == 0 || target.ManifestFileRef == "" ||
		(target.Platform == "cljs") != (target.Build != "") || target.Platform != "" && target.Platform != "cljs" {
		return fmt.Errorf("Clojure: invalid project target")
	}
	return nil
}

func (target Target) ValidateAgainst(repository *corpus.Corpus) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if repository == nil || repository.SHA256() != target.CorpusSHA256 {
		return fmt.Errorf("Clojure: corpus binding mismatch")
	}
	ref, ok := repository.ID(target.ManifestPath)
	if !ok || string(ref) != target.ManifestFileRef {
		return fmt.Errorf("Clojure: manifest binding mismatch")
	}
	for _, entry := range target.Files {
		ref, ok := repository.ID(entry.Path)
		if !ok || ref != entry.ID {
			return fmt.Errorf("Clojure: source binding mismatch: %s", entry.Path)
		}
	}
	return nil
}

// Build keeps one complete native view for both graph and dependency projection.
func Build(ctx context.Context, root string, repository *corpus.Corpus, target Target) (*Result, error) {
	if err := target.ValidateAgainst(repository); err != nil {
		return nil, err
	}
	analysis, err := analyze(ctx, root, target.Files)
	if err != nil {
		return nil, err
	}
	return project(repository, target, analysis)
}
