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

func TestEmbeddedSQLRequiresStructureBeyondALeadingEnglishVerb(t *testing.T) {
	for _, source := range []string{
		"SELECT 1", "SELECT -1", "SELECT 'ready'", `SELECT "거래"`, "SELECT count(*)", "SELECT {projection}", "SELECT ${projection}",
		"SELECT id FROM trades", "SELECT id alias FROM trades", "SELECT id AS alias",
		"SELECT DISTINCT id, pair FROM trades", "SELECT CASE WHEN x=1 THEN 2 ELSE 3 END",
		"SELECT CASE status WHEN 1 THEN 'open' ELSE 'closed' END FROM public.orders",
		`SELECT note COLLATE "C" FROM public.orders`, "SELECT CURRENT_USER UNION SELECT note FROM public.orders",
		"CREATE TABLE trades(id INT PRIMARY KEY)", "CREATE TABLE backup AS SELECT * FROM trades",
		"CREATE UNIQUE INDEX trade_idx ON trades(id)", "CREATE OR REPLACE VIEW active AS SELECT * FROM trades",
		"CREATE TABLE IF NOT EXISTS {table}(id INT)", `CREATE TABLE "unterminated`,
		"ALTER SEQUENCE trades_id_seq RESTART WITH 10", `ALTER SEQUENCE "{sequence}" RENAME TO "{backup}"`,
		"ALTER TABLE trades ADD COLUMN note TEXT", "DROP INDEX IF EXISTS trade_idx",
		"INSERT INTO trades VALUES(1)", "INSERT OR REPLACE INTO {table} SELECT * FROM trades",
		"DELETE FROM trades WHERE id=1", "UPDATE trades SET id=2", "UPDATE {table} SET id=2",
		"UPDATE public.orders AS o SET id=2", "UPDATE public.orders o SET id=2",
		"WITH recent AS (SELECT id FROM trades) SELECT * FROM recent",
		"WITH RECURSIVE recent(id) AS NOT MATERIALIZED (SELECT id FROM trades) SELECT * FROM recent",
		"PRAGMA journal_mode", "PRAGMA journal_mode=wal", "PRAGMA table_info(trades)",
	} {
		if !embeddedSQLStatement(source) {
			t.Errorf("supported SQL source rejected: %s", source)
		}
	}
	for _, source := range []string{
		"create-userdir", "Create a new strategy from a template", "Create user-data directory.",
		"Select Trading mode", "SELECT id", "Insert Exchange API Key", "Insert values from Arguments",
		"Update trades from arguments", "Delete files from cache", "With values from Arguments",
		"CREATE", "INSERT", "UPDATE", "DELETE", "WITH", "SELECT", "PRAGMA some ordinary words",
		"CREATE 'TABLE' trades", "INSERT 'INTO' trades", "WITH name 'AS' (SELECT 1)",
		`SELECT + ""`, "SELECT - ``", "SELECT - []",
	} {
		if embeddedSQLStatement(source) {
			t.Errorf("unbound prose/ambiguous literal acquired SQL authority: %s", source)
		}
	}
}

func TestCumulativeSQLAdmissionRetainsSourcesAndDropsProseBeforeRelations(t *testing.T) {
	for _, language := range []string{"go", "python", "jsts"} {
		t.Run(language, func(t *testing.T) {
			response, err := Database(t.Context(), cumulativeDataRequest(t, language))
			if err != nil {
				t.Fatal(err)
			}
			queries := map[string]facts.ExtractionNode{}
			explicitBare := false
			for _, node := range response.Nodes {
				if node.Path == "data/schema.sql" && node.Data.Kind == "query" && strings.Contains(node.Data.SQL, "SELECT trading mode") {
					explicitBare = true
				}
				if !strings.Contains(node.Path, "sql_literals") && !strings.Contains(node.Path, "sql-literals") {
					continue
				}
				if node.Data.Kind == "query" {
					queries[node.Data.SQL] = node
				} else if node.Name != "public.orders" {
					t.Fatalf("literal prose/value invented a table: %+v", node)
				}
			}
			if language != "python" && !explicitBare || len(queries) != 11 {
				t.Fatalf("SQL source context lost or prose admitted: explicit=%v queries=%+v", explicitBare, queries)
			}
			joined := queries["SELECT id FROM public.orders WHERE note = 'JOIN imaginary_table'"]
			if joined.ID == "" || joined.Line < 1 || joined.Data.Expression == "" || joined.Data.Partial || !reflect.DeepEqual(joined.Data.Tables, []string{"public.orders"}) {
				t.Fatalf("joined statement/physical anchor/value isolation lost: %+v", joined)
			}
			malformed := queries[`SELECT * FROM "unterminated`]
			if malformed.ID == "" || !malformed.Data.Partial || len(malformed.Data.Tables) != 0 {
				t.Fatalf("recognized malformed SQL lost its partial source: %+v", malformed)
			}
			projection := queries["SELECT {projection}"]
			if projection.ID == "" || !projection.Data.Partial || len(projection.Data.Tables) != 0 {
				t.Fatalf("dynamic projection lost its partial source: %+v", projection)
			}
			for _, source := range []string{
				"SELECT 1", "SELECT 'ready'", "WITH recent AS (SELECT id FROM public.orders) SELECT id FROM recent", "UPDATE public.orders SET note = 'updated'",
				"UPDATE public.orders AS o SET id=2", "SELECT CASE status WHEN 1 THEN 'open' ELSE 'closed' END FROM public.orders",
				`SELECT note COLLATE "C" FROM public.orders`, "SELECT CURRENT_USER UNION SELECT note FROM public.orders",
			} {
				if queries[source].ID == "" {
					t.Errorf("supported source missing: %s", source)
				}
			}
		})
	}
}

func TestSQLCSourceScopeRetainsAmbiguousBareQuery(t *testing.T) {
	request := cumulativeDataRequest(t, "python")
	file := "src/fixture_app/sql_literals.py"
	request.Files = []string{file}
	// sqlc owns the explicit input assignment; the database producer consumes
	// that existing contract without reclassifying its declared source.
	sqlc := Response{
		Nodes: []facts.ExtractionNode{{ID: "config", Path: "sqlc.yaml"}, {ID: "input", Path: file}},
		Links: []facts.ExtractionLink{{From: "config", To: "input", Label: "configured queries input"}},
	}
	response, err := database(t.Context(), request, sqlc)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range response.Nodes {
		if node.Data.Kind == "query" && node.Data.SQL == "SELECT id" && node.Data.Scope == "sqlc:config" {
			return
		}
	}
	t.Fatal("sqlc's explicit source authority was replaced by unbound-literal admission")
}
