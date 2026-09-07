package facts

import (
	"fmt"
	"sort"
	"strings"
)

// Extraction is the shared extension boundary. Producers supply local names,
// locations and labeled relationships; repomap owns identities and inventory.
type Extraction struct {
	Name        string           `json:"name"`
	Nodes       []ExtractionNode `json:"nodes"`
	Links       []ExtractionLink `json:"links"`
	Diagnostics []Diagnostic     `json:"diagnostics,omitempty"`
}

type ExtractionNode struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

type ExtractionLink struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
	// A more precise source than the origin node, when the producer has one.
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

func (b *builder) addExtractions() error {
	names := make(map[string]bool)
	paths := b.source.paths()
	filesAt := make(map[string][]Anchor)
	for _, extraction := range b.input.Extractions {
		if !validText(extraction.Name) || names[extraction.Name] {
			return fmt.Errorf("facts: missing or duplicate extractor name %q", extraction.Name)
		}
		names[extraction.Name] = true
		nodes := append([]ExtractionNode{}, extraction.Nodes...)
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
		refs := make(map[string]Fact, len(nodes))
		for _, node := range nodes {
			if !validText(node.ID) || refs[node.ID].ID != "" || node.Path == "" && !validText(node.Name) || node.Line < 0 {
				return fmt.Errorf("extractor %s: each node needs a unique id and a path or name", extraction.Name)
			}
			if node.Path != "" && node.Path != "." {
				if err := validateRepositoryPath(node.Path); err != nil {
					return err
				}
			}
			row := Fact{Kind: KindEntity, Key: node.ID, Symbol: node.Name, Path: node.Path, Extractor: extraction.Name, Value: "reference"}
			if node.Path != "" {
				row.Value = "not_in_corpus"
				row.TargetID = b.targetForPath(node.Path)
				if node.Path != "." {
					row.Anchor = &Anchor{Path: node.Path, Line: max(1, node.Line)}
				}
				members, known := filesAt[node.Path]
				if !known {
					for _, file := range pathsBeneath(paths, node.Path) {
						members = append(members, Anchor{Path: file, Line: 1})
					}
					filesAt[node.Path] = members
				}
				row.Evidence = members
				if len(members) > 0 {
					row.Value = "present"
				}
			}
			refs[node.ID] = b.add(b.rootForTarget(row.TargetID), row, extraction.Name, node.ID)
		}
		links := append([]ExtractionLink{}, extraction.Links...)
		sort.SliceStable(links, func(i, j int) bool {
			a, c := links[i], links[j]
			if a.From != c.From {
				return a.From < c.From
			}
			if a.To != c.To {
				return a.To < c.To
			}
			if a.Label != c.Label {
				return a.Label < c.Label
			}
			if a.Path != c.Path {
				return a.Path < c.Path
			}
			return a.Line < c.Line
		})
		for _, link := range links {
			from, fromOK := refs[link.From]
			to, toOK := refs[link.To]
			if !fromOK || !toOK || !validText(link.Label) || link.Line < 0 {
				return fmt.Errorf("extractor %s: link needs known node ids and a label", extraction.Name)
			}
			anchor := from.Anchor
			if link.Path != "" {
				anchor = &Anchor{Path: link.Path, Line: max(1, link.Line)}
			}
			if anchor == nil {
				return fmt.Errorf("extractor %s: link %s to %s needs a source path", extraction.Name, link.From, link.To)
			}
			b.add(b.rootForTarget(from.TargetID), Fact{Kind: KindRelation, Key: link.Label, Anchor: anchor,
				Refs: []string{from.ID, to.ID}, TargetID: from.TargetID, Extractor: extraction.Name,
			}, extraction.Name, link.From, link.To, link.Label)
		}
		for _, diagnostic := range extraction.Diagnostics {
			b.diagnose(extraction.Name+":"+diagnostic.Kind, diagnostic.Detail)
		}
	}
	return nil
}

// The corpus is sorted. A producer with many nodes should not rescan every
// repository file for every node; repeated directory references share lookup.
func pathsBeneath(paths []string, root string) []string {
	if root == "." {
		return paths
	}
	i := sort.SearchStrings(paths, root)
	if i < len(paths) && paths[i] == root {
		return paths[i : i+1]
	}
	prefix := root + "/"
	start := sort.SearchStrings(paths, prefix)
	end := start
	for end < len(paths) && strings.HasPrefix(paths[end], prefix) {
		end++
	}
	return paths[start:end]
}
