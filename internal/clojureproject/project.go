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
)

type Target struct {
	Ref, Selector, Name, ProjectDir, ManifestPath, ManifestFileRef, CorpusSHA256 string
	Files                                                                        []corpus.Entry
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
	var targets []Target
	for dir, manifest := range manifests {
		target := Target{Selector: "clojure:" + manifest.Path, Name: name, ProjectDir: dir, ManifestPath: manifest.Path, ManifestFileRef: string(manifest.ID), CorpusSHA256: repository.SHA256()}
		if dir != "." {
			target.Name = dir
		}
		for _, entry := range repository.Entries() {
			if path.Ext(entry.Path) != ".clj" && path.Ext(entry.Path) != ".cljc" || path.Base(entry.Path) == "project.clj" {
				continue
			}
			if dir != "." && !strings.HasPrefix(entry.Path, dir+"/") {
				continue
			}
			owner := dir
			for parent := path.Dir(entry.Path); parent != dir && parent != "."; parent = path.Dir(parent) {
				if _, ok := manifests[parent]; ok {
					owner = parent
					break
				}
			}
			if owner == dir {
				target.Files = append(target.Files, entry)
			}
		}
		if len(target.Files) == 0 {
			continue
		}
		slices.SortFunc(target.Files, func(a, b corpus.Entry) int { return strings.Compare(a.Path, b.Path) })
		raw, _ := json.Marshal(target)
		target.Ref = fmt.Sprintf("clojure-%x", sha256.Sum256(raw))
		targets = append(targets, target)
	}
	slices.SortFunc(targets, func(a, b Target) int { return strings.Compare(a.Selector, b.Selector) })
	return targets, nil
}

func (target Target) Validate() error {
	if target.Ref == "" || target.Name == "" || target.Selector != "clojure:"+target.ManifestPath || len(target.Files) == 0 || target.ManifestFileRef == "" {
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
