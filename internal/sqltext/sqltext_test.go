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
		// A table filled in by a printf verb (Go, Python) or an adjacent
		// piece of a name, as real migration and store code writes it.
		"INSERT INTO %s (version, dirty) VALUES (?, ?)", "CREATE TABLE IF NOT EXISTS %s (version uint64, dirty bool)",
		"DELETE FROM %s", "DROP TABLE %s", "INSERT INTO %s(%s, %s) VALUES($1, $2)",
		"UPDATE %s SET %s = $2 WHERE %s = $1", "DELETE FROM %s WHERE %s = $1", "CREATE SCHEMA %s;",
		"UPDATE %s t SET %s = nextval('%s') FROM (SELECT ctid FROM %s WHERE %s IS NULL LIMIT $1) sub WHERE t.ctid = sub.ctid",
		"ALTER TABLE partitions.manifests_p_%d DROP CONSTRAINT IF EXISTS fk_manifests", "ALTER TABLE %[1]s ENABLE TRIGGER %[2]s",
		"DROP TABLE %(table)s", "CREATE DATABASE IF NOT EXISTS %s default charset utf8mb4", "CREATE EXTENSION IF NOT EXISTS %q",
		"UPDATE %s SET {assignments} WHERE id = ?",
		// Dialect forms, including sqlc-generated MySQL and PostgreSQL queries.
		"-- name: BarExists :one\nSELECT EXISTS (\n  SELECT 1 FROM bar where id = $1\n)\n",
		"-- name: BarNotExists :one\nSELECT NOT EXISTS (\n  SELECT 1 FROM bar where id = ?\n)\n",
		"-- name: UpdateJoin :exec\nUPDATE join_table as jt\nJOIN primary_table as pt\n  ON jt.primary_table_id = pt.id\nSET jt.is_active = ?\nWHERE jt.id = ?",
		"-- name: UpdateXWithY :exec\nUPDATE x INNER JOIN y ON y.a = x.a SET x.b = y.b",
		"-- name: DeleteAuthor :exec\nUPDATE authors, books SET authors.deleted_at = now(), books.deleted_at = now() WHERE books.is_amazing = 1",
		"-- name: DeleteJoin :exec\nDELETE jt.*, pt.* FROM join_table as jt JOIN primary_table as pt ON jt.primary_table_id = pt.id WHERE jt.id = ?",
		"-- name: RemoveAll :exec\nDELETE author_book FROM author_book INNER JOIN book ON book.id = author_book.book_id",
		"INSERT IGNORE INTO users (id) VALUES (?)", "SELECT @@server_id", "SELECT :name", "UPDATE OR IGNORE trades SET id=2",
		"CREATE MATERIALIZED VIEW totals AS SELECT 1", "CREATE TYPE mood AS ENUM ('sad', 'ok')", "DROP MATERIALIZED VIEW IF EXISTS totals",
		"ALTER TABLE ONLY public.orders ADD CONSTRAINT orders_pk PRIMARY KEY (id)", "ALTER TABLE trades MODIFY note TEXT",
		"ALTER TABLE trades ENABLE ROW LEVEL SECURITY", "DELETE FROM ONLY trades WHERE id = 1",
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
		// A hole or a table-like word does not make a message a statement:
		// SQL never writes ':' after an object name, and SET assigns.
		"create table %s: %w", "drop table %q: %w", "create table t: %v", "create {kind} table {name}: {err}",
		"create table with nil name", "create database failed: %w", "select: empty target list", "select user %d name",
		"select %s", "update %s set to %v", "Update method is set to never so we won't check for an update",
		"update venue mismatch: %s", "delete %s from cache", "insert: missing relation", "alter schema owner returned exit code %d",
	} {
		if Statement(source) {
			t.Errorf("unbound prose/ambiguous literal acquired SQL authority: %s", source)
		}
	}
}

func TestTablesSkipClauseWordsAndRuntimeNames(t *testing.T) {
	for source, want := range map[string][]string{
		"DROP TABLE IF EXISTS batched_background_migration_jobs CASCADE":                {"batched_background_migration_jobs"},
		"DROP TABLE IF EXISTS authors":                                                  {"authors"},
		"ALTER TABLE IF EXISTS users ADD COLUMN x int":                                  {"users"},
		"CREATE TABLE IF NOT EXISTS audit (id int)":                                     {"audit"},
		"ALTER TABLE ONLY public.orders ADD CONSTRAINT pk PRIMARY KEY (id)":             {"public.orders"},
		"SELECT COUNT(*) FROM table WHERE status = $1":                                  {"table"},
		"INSERT INTO authors (name, bio) VALUES (?, ?) ON DUPLICATE KEY UPDATE bio = ?": {"authors"},
		"INSERT INTO kv (k) VALUES ($1) ON CONFLICT (k) DO UPDATE SET k = $1":           {"kv"},
		// A name filled in at run time is not a written table name.
		"DELETE FROM %s WHERE id IN (SELECT id FROM kept)":         {"kept"},
		"ALTER TABLE partitions.manifests_p_%d DROP CONSTRAINT fk": nil,
		`SELECT FIRST 1 version FROM "%v"`:                         nil,
		"UPDATE shard_{n} SET id = 1":                              nil,
	} {
		tokens, _ := Tokens(source, 1)
		if got := Tables(tokens); len(got) != len(want) || len(want) > 0 && !reflect.DeepEqual(got, want) {
			t.Errorf("Tables(%q) = %v, want %v", source, got, want)
		}
	}
	for source, want := range map[string]bool{
		"DELETE FROM %s WHERE id = $1":          true,
		"UPDATE shard_{n} SET id = 1":           true,
		`SELECT version FROM "%v"`:              true,
		"SELECT id FROM t WHERE name LIKE '%s'": false,
		"INSERT INTO t (a) VALUES (%s)":         false,
		"SELECT a % b FROM t":                   false,
	} {
		tokens, _ := Tokens(source, 1)
		if got := RuntimeTable(tokens); got != want {
			t.Errorf("RuntimeTable(%q) = %v, want %v", source, got, want)
		}
	}
}
