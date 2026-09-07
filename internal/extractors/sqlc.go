package extractors

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"go.yaml.in/yaml/v3"
)

// Only config v2 is supported. Other sqlc fields are deliberately left to
// sqlc itself: this pass reads declared file relationships, never a database
// connection or plugin executable.
type sqlcConfig struct {
	Version string `yaml:"version"`
	SQL     []struct {
		Engine  string    `yaml:"engine"`
		Schema  yaml.Node `yaml:"schema"`
		Queries yaml.Node `yaml:"queries"`
		Gen     map[string]struct {
			Out yaml.Node `yaml:"out"`
		} `yaml:"gen"`
		Codegen []struct {
			Plugin string    `yaml:"plugin"`
			Out    yaml.Node `yaml:"out"`
		} `yaml:"codegen"`
	} `yaml:"sql"`
}

// SQLC is a reference built-in extractor. It only knows sqlc's config; its
// response has the same producer-local IDs and fact shape as a command plugin.
func SQLC(ctx context.Context, request Request) (Response, error) {
	b := sqlcExtractor{request: request, nodes: make(map[string]bool), response: Response{Version: Version, Nodes: []facts.ExtractionNode{}, Links: []facts.ExtractionLink{}, Diagnostics: []facts.Diagnostic{}}}
	for _, configPath := range request.Files {
		if err := ctx.Err(); err != nil {
			return b.response, err
		}
		switch path.Base(configPath) {
		case "sqlc.yaml", "sqlc.yml", "sqlc.json":
		default:
			continue
		}
		data, err := os.ReadFile(filepath.Join(request.Root, filepath.FromSlash(configPath)))
		if err != nil {
			return b.response, fmt.Errorf("sqlc: read %s: %w", configPath, err)
		}
		var config sqlcConfig
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		err = decoder.Decode(&config)
		if err == nil {
			var extra yaml.Node
			if next := decoder.Decode(&extra); next != io.EOF {
				err = fmt.Errorf("expected one configuration document")
			}
		}
		if err != nil || config.Version != "2" || len(config.SQL) == 0 {
			b.diagnose("sqlc_configuration", configPath+": expected a readable version 2 configuration with sql blocks")
			continue
		}
		for index, block := range config.SQL {
			type declaration struct {
				label string
				node  yaml.Node
			}
			var declarations []declaration
			var blockErr error
			add := func(role, generator string, node yaml.Node) {
				if blockErr != nil {
					return
				}
				values, err := sqlcPathNodes(node, role != "output")
				if err != nil {
					blockErr = err
					return
				}
				for _, value := range values {
					label := "configured " + role + " input"
					if role == "output" {
						label = "configured output: " + generator
					}
					declarations = append(declarations, declaration{label: label, node: value})
				}
			}
			add("schema", "", block.Schema)
			add("queries", "", block.Queries)
			keys := make([]string, 0, len(block.Gen))
			for key := range block.Gen {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				add("output", key, block.Gen[key].Out)
			}
			for _, plugin := range block.Codegen {
				add("output", plugin.Plugin, plugin.Out)
			}
			if blockErr != nil || len(block.Gen)+len(block.Codegen) == 0 || block.Engine == "" {
				b.diagnose("sqlc_configuration", fmt.Sprintf("%s: sql block %d has incomplete or unsupported path declarations", configPath, index+1))
				continue
			}
			group := facts.ExtractionNode{ID: fmt.Sprintf("%s#sql[%d]", configPath, index), Name: "sqlc (" + block.Engine + ")", Path: configPath, Line: block.Schema.Line}
			b.addNode(group)
			for _, declaration := range declarations {
				item := b.sqlcPath(configPath, declaration.node)
				b.addNode(item)
				b.response.Links = append(b.response.Links, facts.ExtractionLink{From: group.ID, To: item.ID, Label: declaration.label, Path: configPath, Line: declaration.node.Line})
			}
		}
	}
	return b.response, nil
}

type sqlcExtractor struct {
	request  Request
	response Response
	nodes    map[string]bool
}

func (b *sqlcExtractor) addNode(node facts.ExtractionNode) {
	if b.nodes[node.ID] {
		return
	}
	b.nodes[node.ID] = true
	b.response.Nodes = append(b.response.Nodes, node)
}

func (b *sqlcExtractor) diagnose(kind, detail string) {
	b.response.Diagnostics = append(b.response.Diagnostics, facts.Diagnostic{Kind: kind, Detail: detail})
}

func sqlcPathNodes(node yaml.Node, allowList bool) ([]yaml.Node, error) {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		resolved := *node.Alias
		resolved.Line, resolved.Column = node.Line, node.Column
		return sqlcPathNodes(resolved, allowList)
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && strings.TrimSpace(node.Value) != "" {
		return []yaml.Node{node}, nil
	}
	if allowList && node.Kind == yaml.SequenceNode && len(node.Content) > 0 {
		var values []yaml.Node
		for _, child := range node.Content {
			items, err := sqlcPathNodes(*child, false)
			if err != nil {
				return nil, err
			}
			values = append(values, items...)
		}
		return values, nil
	}
	return nil, fmt.Errorf("expected a path string or nonempty path list")
}

func (b *sqlcExtractor) sqlcPath(config string, node yaml.Node) facts.ExtractionNode {
	item := facts.ExtractionNode{ID: "reference:" + config + ":" + node.Value, Name: node.Value}
	resolved := path.Clean(path.Join(path.Dir(config), node.Value))
	// Preserve references we cannot resolve instead of inventing their target.
	// Wildcards need sqlc's exact expansion semantics; they are not literal paths.
	if path.IsAbs(node.Value) || strings.ContainsAny(node.Value, "*?[]\\\x00") || resolved == ".." || strings.HasPrefix(resolved, "../") {
		b.diagnose("sqlc_path_unresolved", fmt.Sprintf("%s:%d: %q cannot be resolved as a literal repository path", config, node.Line, node.Value))
		return item
	}
	item.ID, item.Path, item.Name = "path:"+resolved, resolved, resolved
	return item
}
