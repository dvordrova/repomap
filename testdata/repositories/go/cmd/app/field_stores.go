package main

import (
	"database/sql"
	"flag"
	"net/http"
)

// itemStore keeps the database a caller connected it to.
type itemStore struct {
	db *sql.DB
}

// guardedStore copies the database of the store it guards in a composite
// literal.
type guardedStore struct {
	db *sql.DB
}

func guard(s *itemStore) *guardedStore {
	return &guardedStore{db: s.db}
}

func (g *guardedStore) count() {
	_, _ = g.db.Exec("SELECT count(*) FROM items")
}

// countItems connects a store to the items database and counts through
// its guard: the walk follows the instance from the count to sql.Open.
func countItems() {
	db, err := sql.Open("sqlite", "file:items.db")
	if err != nil {
		return
	}
	guard(&itemStore{db: db}).count()
}

// poolConfig's DB is connected for one instance; pingPool is handed some
// other configuration, which no caller names: its ping reaches what any
// write of DB stored only as a possible origin, never as its address.
type poolConfig struct {
	DB *sql.DB
}

func connectPool() *poolConfig {
	x := &poolConfig{}
	x.DB, _ = sql.Open("sqlite", "file:pool.db")
	x.DB = nil
	return x
}

func pingPool(c *poolConfig) {
	_ = c.DB.Ping()
}

// statusEndpoint's url is its default, or whatever the flag package writes
// through the address the code hands it.
type statusEndpoint struct {
	url string
}

func configuredStatus() *statusEndpoint {
	e := &statusEndpoint{url: "http://localhost:8080/status"}
	flag.StringVar(&e.url, "status", e.url, "the status endpoint")
	return e
}

func (e *statusEndpoint) ping() {
	_, _ = http.Get(e.url)
}

// pingStatus pings the configured status endpoint.
func pingStatus() {
	configuredStatus().ping()
}
