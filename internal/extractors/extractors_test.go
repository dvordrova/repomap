package extractors

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
)

func TestExternalCommandAndSQLCUseTheSameFactBoundary(t *testing.T) {
	root, repository := commandCorpus(t, "success")
	extracted, err := Run(context.Background(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(extracted.Exchanges) != 3 || len(extracted.Extractions) != 3 {
		t.Fatalf("producers: %+v", extracted)
	}
	result, err := facts.Build(facts.Input{Repository: repository, Extractions: extracted.Extractions})
	if err != nil {
		t.Fatal(err)
	}
	byProducer := make(map[string]facts.Fact)
	for _, row := range result.OfKind(facts.KindEntity) {
		if row.Key == "sqlc.yaml#sql[0]" {
			byProducer[row.Extractor] = row
		}
	}
	if len(byProducer) != 2 {
		t.Fatalf("generation sources: %+v", byProducer)
	}
	if !reflect.DeepEqual(extracted.Extractions[0].Nodes, extracted.Extractions[2].Nodes) || !reflect.DeepEqual(extracted.Extractions[0].Links, extracted.Extractions[2].Links) {
		t.Fatal("same plugin result normalized differently")
	}
	if byProducer["company"].ID == byProducer["sqlc"].ID {
		t.Fatal("producer-local identities collided")
	}
	if extracted.Exchanges[2].Stderr != "plugin diagnostic\n" {
		t.Fatal("stderr not recorded")
	}
	for _, row := range result.OfKind(facts.KindRelation) {
		if row.Extractor == "database" {
			continue
		}
		if len(row.Refs) != 2 || row.Refs[0] != byProducer[row.Extractor].ID || result.ByID()[row.Refs[1]].Extractor != row.Extractor {
			t.Fatalf("reference not restored: %+v", row)
		}
	}
	// The raw response must keep local IDs; normalization may not mutate it.
	if extracted.Extractions[2].Nodes[0].ID != "sqlc.yaml#sql[0]" {
		t.Fatal("producer input mutated")
	}
}

func TestCommandFailuresKeepTheirExchangeAndHaveNoReplacement(t *testing.T) {
	for _, mode := range []string{"malformed", "failure", "wrong-version", "unknown-ref"} {
		t.Run(mode, func(t *testing.T) {
			root, repository := commandCorpus(t, mode)
			extracted, err := Run(context.Background(), root, repository)
			if mode == "unknown-ref" && err == nil {
				_, err = facts.Build(facts.Input{Repository: repository, Extractions: extracted.Extractions})
			}
			if err == nil {
				t.Fatal("failed extractor silently replaced")
			}
			if len(extracted.Exchanges) != 3 || extracted.Exchanges[2].Stdout == "" {
				t.Fatal("failed exchange lost")
			}
			if _, err := json.Marshal(extracted); err != nil {
				t.Fatal("failure cannot be saved", err)
			}
		})
	}
}

// A child copy of this test binary behaves like a separately installed plugin.
// It reads the actual stdin protocol and returns the reference SQLC response.
func TestExtractorProcess(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--repomap-plugin" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	var request Request
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(9)
	}
	var options struct {
		Marker string `json:"marker"`
	}
	if err := json.Unmarshal(request.Options, &options); err != nil || options.Marker != "delivered" {
		os.Exit(8)
	}
	working, _ := os.Getwd()
	root, _ := filepath.EvalSymlinks(request.Root)
	working, _ = filepath.EvalSymlinks(working)
	if request.Version != Version || working != root || len(request.Files) < 3 {
		os.Exit(7)
	}
	fmt.Fprintln(os.Stderr, "plugin diagnostic")
	if mode == "malformed" {
		fmt.Print("not JSON")
		os.Exit(0)
	}
	if mode == "failure" {
		fmt.Print("partial output")
		os.Exit(3)
	}
	response, err := SQLC(context.Background(), request)
	if err != nil {
		os.Exit(6)
	}
	if mode == "unknown-ref" {
		response.Links[0].To = "absent"
	}
	if mode == "wrong-version" {
		response.Version = 999
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		os.Exit(5)
	}
	os.Exit(0)
}

func commandCorpus(t *testing.T, mode string) (string, *corpus.Corpus) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	configuration, _ := json.Marshal(config{Version: Version, Extractors: []Command{{Name: "company", Command: []string{executable, "-test.run=^TestExtractorProcess$", "--", "--repomap-plugin", mode}, Options: json.RawMessage(`{"marker":"delivered"}`)}}})
	return newCorpus(t, map[string]string{
		ConfigFilename: string(configuration),
		"sqlc.yaml":    "version: '2'\nsql:\n- engine: sqlite\n  schema: schema.sql\n  queries: query.sql\n  gen:\n    go:\n      out: generated\n",
		"schema.sql":   "CREATE TABLE t (id integer);\n", "query.sql": "-- name: List :many\nSELECT * FROM t;\n",
	})
}

func TestCommandConfigHasOneCurrentVersion(t *testing.T) {
	root, repository := newCorpus(t, map[string]string{ConfigFilename: `{"version":2,"extractors":[]}`})
	if _, err := Run(context.Background(), root, repository); err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatal("old config accepted")
	}
}
