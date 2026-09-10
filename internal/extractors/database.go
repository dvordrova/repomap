package extractors

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
)

type databaseExtractor struct {
	response     Response
	nodes        map[string]int
	tables       map[string][]string
	queries      []string
	sourceAnchor func(sqlToken) facts.Anchor
}

// Database extracts source declarations and SQL text without opening a database
// or running repository code. SQLC supplies explicit source scopes; otherwise
// each unbound source keeps its own scope rather than inventing one connection.
func Database(ctx context.Context, request Request) (Response, error) {
	sqlc, err := SQLC(ctx, request)
	if err != nil {
		return Response{}, err
	}
	return database(ctx, request, sqlc)
}
func database(ctx context.Context, request Request, sqlc Response) (Response, error) {
	b := &databaseExtractor{response: Response{Version: Version, Nodes: []facts.ExtractionNode{}, Links: []facts.ExtractionLink{}, Diagnostics: []facts.Diagnostic{}}, nodes: map[string]int{}, tables: map[string][]string{}}
	scopes := map[string][]string{}
	paths := map[string]string{}
	for _, node := range sqlc.Nodes {
		paths[node.ID] = node.Path
	}
	for _, link := range sqlc.Links {
		if link.Label != "configured schema input" && link.Label != "configured queries input" {
			continue
		}
		root := paths[link.To]
		if root == "" {
			continue
		}
		for _, file := range request.Files {
			if file == root || strings.HasPrefix(file, root+"/") {
				scopes[file] = append(scopes[file], "sqlc:"+link.From)
			}
		}
	}
	for _, file := range request.Files {
		if err := ctx.Err(); err != nil {
			return b.response, err
		}
		extension := strings.ToLower(path.Ext(file))
		switch extension {
		case ".sql", ".py", ".go", ".js", ".jsx", ".ts", ".tsx":
		default:
			continue
		}
		raw, err := os.ReadFile(filepath.Join(request.Root, filepath.FromSlash(file)))
		if err != nil {
			return b.response, fmt.Errorf("data: read %s: %w", file, err)
		}
		source := string(raw)
		fileScopes := scopes[file]
		if len(fileScopes) == 0 {
			fileScopes = []string{"source:" + file}
		}
		if extension == ".sql" {
			for _, part := range sqlStatements(source) {
				for _, scope := range fileScopes {
					b.addSQL(file, scope, part.text, part.line, part.dynamic, nil)
				}
			}
			continue
		}
		literals, mask := sourceLiterals(source, extension == ".py")
		var owners []dataOwner
		if extension == ".py" {
			owners = b.addPythonModels(file, source, mask, literals)
		}
		for _, literal := range joinSQLLiterals(source, mask, literals, extension == ".py") {
			// Explicit sqlc inputs already have SQL source authority. An
			// otherwise unbound literal needs more than a leading English verb.
			if len(scopes[file]) == 0 && !embeddedSQLStatement(literal.text) {
				continue
			}
			scope := fileScopes[0]
			if len(scopes[file]) == 0 {
				scope = fmt.Sprintf("source:%s:%d:%d", file, literal.line, literal.start)
			}
			owner := dataOwnerAt(owners, literal.line)
			var anchor *facts.Anchor
			if owner != nil {
				scope = owner.scope
				a := facts.Anchor{Path: file, Line: owner.line}
				anchor = &a
			}
			b.sourceAnchor = literalSQLAnchor(file, literal)
			id := b.addSQL(file, scope, literal.text, literal.line, literal.dynamic, anchor)
			b.sourceAnchor = nil
			if id != "" && literal.expression != "" {
				b.response.Nodes[b.nodes[id]].Data.Expression = literal.expression
			}
		}
	}
	b.linkQueries()
	return b.response, nil
}

func literalSQLAnchor(file string, literal sourceLiteral) func(sqlToken) facts.Anchor {
	return func(token sqlToken) facts.Anchor {
		line := literal.line
		for _, segment := range literal.segments {
			if token.offset >= segment.offset && token.offset < segment.offset+len(segment.text) {
				line = segment.line
				if segment.multiline {
					line += strings.Count(segment.text[:token.offset-segment.offset], "\n")
				}
				break
			}
		}
		return facts.Anchor{Path: file, Line: line}
	}
}
func (b *databaseExtractor) addNode(node facts.ExtractionNode) {
	if _, exists := b.nodes[node.ID]; exists {
		return
	}
	b.nodes[node.ID] = len(b.response.Nodes)
	b.response.Nodes = append(b.response.Nodes, node)
}
func dataTableKey(scope, name string) string { return scope + "\x00" + name }
func (b *databaseExtractor) addTable(file string, line int, data *facts.DataObject) string {
	qualified := data.Name
	if data.Schema != "" {
		qualified = data.Schema + "." + data.Name
	}
	if data.Origin != "orm" {
		tokens, _ := sqlTokens(data.Name, line)
		if len(tokens) > 0 {
			data.Name = tokens[len(tokens)-1].text
			if len(tokens) > 2 && tokens[len(tokens)-2].text == "." {
				data.Schema, _ = sqlIdentifier(tokens[:len(tokens)-2], 0)
			}
		}
	}
	id := fmt.Sprintf("table:%s:%s:%s:%d", data.Scope, qualified, file, line)
	b.addNode(facts.ExtractionNode{ID: id, Name: qualified, Path: file, Line: line, Data: data})
	key := dataTableKey(data.Scope, qualified)
	for _, known := range b.tables[key] {
		if known == id {
			return id
		}
	}
	b.tables[key] = append(b.tables[key], id)
	return id
}
func (b *databaseExtractor) linkQueries() {
	for _, id := range b.queries {
		query := b.response.Nodes[b.nodes[id]]
		for _, name := range query.Data.Tables {
			key := dataTableKey(query.Data.Scope, name)
			targets := append([]string(nil), b.tables[key]...)
			if len(targets) == 0 {
				target := b.addTable(query.Path, query.Line, &facts.DataObject{Kind: "table", Origin: "query", Scope: query.Data.Scope, Name: name, Partial: query.Data.Partial})
				targets = []string{target}
			}
			sort.Strings(targets)
			for _, target := range targets {
				b.response.Links = append(b.response.Links, facts.ExtractionLink{From: id, To: target, Label: "SQL mentions table", Path: query.Path, Line: query.Line})
			}
		}
	}
}
