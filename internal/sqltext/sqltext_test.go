package sqltext

import (
	"reflect"
	"testing"
)

func TestSQLMalformedAndQuotedInputsRemainPartialOrLiteral(t *testing.T) {
	tokens, partial := Tokens("SELECT * FROM \"orders\" WHERE note='JOIN imaginary'", 3)
	if partial || !reflect.DeepEqual(Tables(tokens), []string{"orders"}) {
		t.Fatalf("quoted SQL evidence: %+v %v", tokens, partial)
	}
	_, partial = Tokens("SELECT * FROM {table} JOIN real_table ON x=1", 1)
	if !partial {
		t.Fatal("dynamic identifier became complete")
	}
	_, partial = Tokens("SELECT * FROM \"unterminated", 1)
	if !partial {
		t.Fatal("malformed identifier became complete")
	}
	tokens, _ = Tokens("INSERT INTO audit_log (id) VALUES ($1); UPDATE users SET seen = now()", 1)
	if got := Tables(tokens); !reflect.DeepEqual(got, []string{"audit_log", "users"}) {
		t.Fatalf("tables of two statements = %v", got)
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
		"MERGE INTO trades t USING incoming i ON t.id = i.id WHEN MATCHED THEN UPDATE SET note = i.note",
		"REPLACE INTO trades VALUES(1)", "-- name: GetUser :one\nSELECT id, email, name FROM users WHERE id = $1\n",
	} {
		if !Statement(source) {
			t.Errorf("supported SQL source rejected: %s", source)
		}
	}
	for _, source := range []string{
		"create-userdir", "Create a new strategy from a template", "Create user-data directory.",
		"Select Trading mode", "SELECT id", "Insert Exchange API Key", "Insert values from Arguments",
		"Update trades from arguments", "Delete files from cache", "With values from Arguments",
		"CREATE", "INSERT", "UPDATE", "DELETE", "WITH", "SELECT", "PRAGMA some ordinary words",
		"CREATE 'TABLE' trades", "INSERT 'INTO' trades", "WITH name 'AS' (SELECT 1)",
		`SELECT + ""`, "SELECT - ``", "SELECT - []", "MERGE", "REPLACE",
		// Messages, flag help and keyword comparisons seen as call arguments
		// in a real Go repository.
		"create %s dir: %w", "create {param} dir", "with", "merge", "select",
		"merge repository target hypotheses: %w", "update settings: %w", "replace %s with %s",
		"create a standalone report with GitLab source links; does not select a repository",
		"Create .repomap.conf if absent, then open it in the configured editor.",
	} {
		if Statement(source) {
			t.Errorf("unbound prose/ambiguous literal acquired SQL authority: %s", source)
		}
	}
}
