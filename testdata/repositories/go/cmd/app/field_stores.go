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

// serviceConfig's URL is written by set; send reads it from one of two
// configurations no caller names: each possible write stays beside each
// unresolved configuration, never the call's address.
type serviceConfig struct {
	URL string
}

func set(c *serviceConfig) {
	c.URL = "https://service.example"
}

func send(a, b *serviceConfig, choose bool) {
	c := a
	if choose {
		c = b
	}
	_, _ = http.Get(c.URL)
}

// endpointConfig's URL is written by the very function that reads it, from
// the argument each caller hands: each caller's request reads its own
// address, never the other caller's.
type endpointConfig struct {
	URL string
}

var endpoints = map[string]*endpointConfig{}

func deliver(c *endpointConfig, u string) {
	c.URL = u
	_, _ = http.Get(c.URL)
}

func deliverBoth() {
	deliver(endpoints["a"], "https://a.example")
	deliver(endpoints["b"], "https://b.example")
}

// webhook's Endpoint copies a serviceConfig's URL: the field's write reads
// another field, whose own write is followed too.
type webhook struct {
	Endpoint string
}

func copyEndpoint(w *webhook, c *serviceConfig) {
	w.Endpoint = c.URL
}

func fire(w *webhook) {
	_, _ = http.Get(w.Endpoint)
}
