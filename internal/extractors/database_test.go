package extractors

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
)

func cumulativeDataRequest(t *testing.T, language string) Request {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "../../testdata/repositories", language)
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return Request{Version: Version, Root: root, Files: paths, Options: json.RawMessage(`{}`)}
}
func TestCumulativeDataPreservesORMOwnershipUnknownScopesAndSQLSources(t *testing.T) {
	for _, language := range []string{"python", "go", "jsts"} {
		t.Run(language, func(t *testing.T) {
			response, err := Database(context.Background(), cumulativeDataRequest(t, language))
			if err != nil {
				t.Fatal(err)
			}
			var declarations, queries int
			scopes := map[string]bool{}
			partial := false
			joined, adjacent, aliases, schema := false, false, false, false
			for _, node := range response.Nodes {
				if node.Data == nil {
					continue
				}
				if err := node.Data.Validate(); err != nil {
					t.Fatal(err)
				}
				if node.Data.Kind == "query" {
					queries++
					partial = partial || node.Data.Partial
					joined = joined || strings.Contains(node.Data.Expression, "+ table +") && node.Data.Partial && reflect.DeepEqual(node.Data.Tables, []string{"orders"})
					adjacent = adjacent || strings.Contains(node.Data.Expression, "\n") && strings.Contains(node.Data.SQL, "SELECT id FROM trades") && !node.Data.Partial
				}
				if node.Data.Kind == "table" && node.Data.Origin != "query" {
					declarations++
					if node.Data.Name == "trades" {
						scopes[node.Data.Scope] = true
						schema = schema || node.Data.Schema == "archive"
					}
					if strings.Contains(node.Name, "not_a_table") {
						t.Fatal("docstring became schema")
					}
					if len(node.Data.Columns) == 0 {
						t.Fatalf("columns missing: %+v", node)
					}
					if node.Name == "trades" && len(node.Data.Columns) > 1 {
						found := map[string]facts.DataColumn{}
						for _, column := range node.Data.Columns {
							found[column.Name] = column
						}
						aliases = found["exchange_name"].Name != "" && found["alias_order"].ForeignKey == "orders.id" && found["qualified_order"].ForeignKey == "orders.id"
					}
				}
			}
			if language == "python" {
				if len(scopes) != 2 || !partial || !joined || !adjacent || !aliases || !schema {
					t.Fatalf("ORM/SQL evidence lost: scopes%v partial%v joined%v adjacent%v aliases%v", scopes, partial, joined, adjacent, aliases)
				}
			} else if declarations != 1 {
				t.Fatalf("SQL declarations=%d", declarations)
			}
			if queries == 0 || declarations == 0 || len(response.Links) == 0 {
				t.Fatalf("missing source data: %+v", response)
			}
			again, err := Database(context.Background(), cumulativeDataRequest(t, language))
			if err != nil || !reflect.DeepEqual(response, again) {
				t.Fatal("data extraction nondeterministic", err)
			}
		})
	}
}
func TestSQLParserRetainsNamedSchemaKeysAndDoesNotInventForeignKeysFromJoin(t *testing.T) {
	b := &databaseExtractor{response: Response{Version: Version, Nodes: []facts.ExtractionNode{}, Links: []facts.ExtractionLink{}}, nodes: map[string]int{}, tables: map[string][]string{}}
	b.addSQL("schema.sql", "sqlc:one", `CREATE TABLE public.orders (id INT PRIMARY KEY, customer_id INT REFERENCES customers(id));`, 1, false, nil)
	b.addSQL("queries.sql", "sqlc:one", "-- name: ReadOrders :many\nSELECT id FROM public.orders JOIN customers ON customers.id = orders.customer_id", 4, false, nil)
	b.addSQL("other.sql", "sqlc:two", "CREATE TABLE public.orders (code TEXT)", 1, false, nil)
	b.linkQueries()
	var found bool
	for _, node := range b.response.Nodes {
		if node.Data.Kind == "query" && node.Name == "ReadOrders" {
			found = true
			if node.Line != 5 {
				t.Fatal("lost original SQL line")
			}
		}
		if node.Data.Kind == "table" && node.Data.Scope == "sqlc:one" && node.Name == "public.orders" {
			if node.Data.Schema != "public" || len(node.Data.Columns) != 2 || !node.Data.Columns[0].PrimaryKey || node.Data.Columns[1].ForeignKey != "customers.id" {
				t.Fatalf("DDL facts missing: %+v", node.Data)
			}
		}
		if node.Data.Kind == "table" && node.Name == "customers" && (node.Data.Origin != "query" || len(node.Data.Columns) > 0) {
			t.Fatal("JOIN invented schema or FK")
		}
	}
	if !found {
		t.Fatal("sqlc query name missing")
	}
	for _, link := range b.response.Links {
		from, to := b.response.Nodes[b.nodes[link.From]], b.response.Nodes[b.nodes[link.To]]
		if from.Data.Scope != to.Data.Scope {
			t.Fatal("cross-connection table relation")
		}
	}
}
func TestSQLMalformedAndQuotedInputsRemainPartialOrLiteral(t *testing.T) {
	tokens, partial := sqlTokens("SELECT * FROM \"orders\" WHERE note='JOIN imaginary'", 3)
	if partial || !reflect.DeepEqual(sqlTables(tokens), []string{"orders"}) {
		t.Fatalf("quoted SQL evidence: %+v %v", tokens, partial)
	}
	_, partial = sqlTokens("SELECT * FROM {table} JOIN real_table ON x=1", 1)
	if !partial {
		t.Fatal("dynamic identifier became complete")
	}
	_, partial = sqlTokens("SELECT * FROM \"unterminated", 1)
	if !partial {
		t.Fatal("malformed identifier became complete")
	}
}

func TestSQLStatementCommentsQuotedNamesAndCompositeKeys(t *testing.T) {
	source := "-- name: ReadOrders :many; this comment is not a statement\nSELECT * FROM \"거래 내역\" WHERE note='literal; value';\n/* also ; */\nCREATE TABLE orders (a INT, b INT, PRIMARY KEY(a,b), CONSTRAINT owner FOREIGN KEY(a,b) REFERENCES peers(x,y));"
	parts := sqlStatements(source)
	if len(parts) != 2 || parts[1].line != 2 {
		t.Fatalf("SQL statement boundaries lost: %+v", parts)
	}
	tokens, partial := sqlTokens(parts[0].text, parts[0].line)
	if partial || !reflect.DeepEqual(sqlTables(tokens), []string{"거래 내역"}) {
		t.Fatal("quoted native table name changed")
	}
	tokens, partial = sqlTokens(parts[1].text, parts[1].line)
	columns := parseSQLColumns(tokens, "schema.sql")
	if partial || len(columns) != 2 || !columns[0].PrimaryKey || !columns[1].PrimaryKey || columns[0].ForeignKey != "peers.x" || columns[1].ForeignKey != "peers.y" {
		t.Fatalf("composite source constraints lost: %+v", columns)
	}
	for _, source := range []string{"x = 'SELECT * FROM a'\ny = 'JOIN b'", "call('SELECT * FROM a', 'JOIN b')"} {
		literals, mask := sourceLiterals(source, true)
		joined := joinSQLLiterals(source, mask, literals, true)
		if len(joined) != 1 || strings.Contains(joined[0].text, "JOIN") {
			t.Fatal("independent SQL expressions were combined")
		}
	}
}

func TestEmbeddedSQLAnchorsFollowPhysicalLiteralLines(t *testing.T) {
	for _, tc := range []struct {
		source string
		python bool
		want   int
	}{
		{`query := "CREATE TABLE trades (\nid INTEGER PRIMARY KEY)"`, false, 1},
		{"query = (\"CREATE TABLE trades (\"\n         \"id INTEGER PRIMARY KEY)\")", true, 2},
	} {
		literals, mask := sourceLiterals(tc.source, tc.python)
		joined := joinSQLLiterals(tc.source, mask, literals, tc.python)
		if len(joined) != 1 {
			t.Fatal("SQL expression missing")
		}
		tokens, _ := sqlTokens(joined[0].text, joined[0].line)
		columns := parseSQLColumns(tokens, "models.py", literalSQLAnchor("models.py", joined[0]))
		if len(columns) != 1 || columns[0].Anchor.Line != tc.want {
			t.Fatalf("SQL text line became a wrong physical source line: %+v", columns)
		}
	}
}
