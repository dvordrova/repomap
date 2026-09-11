package pythonprogramindex

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePythonExplicitFacadesRetainFactoryCallbackAuthority(t *testing.T) {
	const prefix = "src/fixture_app/import_facades/"
	var sources []string
	fixture := filepath.Join("..", "..", "testdata", "repositories", "python")
	err := filepath.WalkDir(filepath.Join(fixture, filepath.FromSlash(prefix)), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(fixture, path)
		if err == nil {
			sources = append(sources, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := pythonCorpus(t, cumulativePythonSources(t, sources...))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	var factory, callback string
	for _, object := range index.Objects {
		if object.Location == nil {
			continue
		}
		if object.Location.Path == prefix+"resolver_impl.py" && object.Name == "load_exchange" {
			factory = object.ID
		}
		if object.Location.Path == prefix+"exchange_impl.py" && object.Name == "ws_connection_reset" {
			callback = object.ID
		}
	}
	if factory == "" || callback == "" {
		t.Fatal("original factory or exchange declaration missing")
	}
	var factoryCalls, callbacks, unknownCallbacks int
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != prefix+"consumer.py" {
			continue
		}
		for _, target := range relation.ToIDs {
			if target == factory && relation.Kind == programindex.RelationCalls {
				factoryCalls++
				if relation.Resolution != programindex.ResolutionAlternatives {
					t.Fatalf("facade import invented exact factory dispatch: %+v", relation)
				}
			}
		}
		for _, pattern := range relation.Patterns {
			if pattern.Selector != "do" {
				continue
			}
			for _, argument := range pattern.Arguments {
				if argument.Position != 1 {
					continue
				}
				if relation.Location.Line >= 31 {
					unknownCallbacks++
					if len(argument.ObjectIDs) != 0 || argument.Resolution != programindex.ResolutionUnresolved {
						t.Fatalf("invalid/unknown export or untyped factory borrowed callback identity: %+v", argument)
					}
					continue
				}
				callbacks++
				if len(argument.ObjectIDs) != 1 || argument.ObjectIDs[0] != callback || argument.Resolution != programindex.ResolutionAlternatives || argument.ObjectsObserved != 1 {
					t.Fatalf("explicit re-export factory lost possible original callback: %+v", argument)
				}
			}
		}
	}
	if factoryCalls != 3 || callbacks != 3 || unknownCallbacks != 12 {
		t.Fatalf("factory calls=%d callback bindings=%d unknown callbacks=%d", factoryCalls, callbacks, unknownCallbacks)
	}
	graph, err := places.Build(places.Input{Revision: strings.Repeat("a", 40), Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != prefix+"exchange_impl.py" || !strings.HasSuffix(place.Symbol.Decl.Name, "ws_connection_reset") {
			continue
		}
		if len(place.Symbol.Bindings) != 3 {
			t.Fatalf("graph lost or invented facade receiver registrations: %+v", place.Symbol.Bindings)
		}
		for _, binding := range place.Symbol.Bindings {
			if binding.Resolution != "alternatives" || binding.Path != prefix+"consumer.py" || !strings.Contains(binding.From, "Bot.__init__") || strings.Contains(binding.From, "UnknownBot") {
				t.Fatalf("graph changed original owner/authority: %+v", binding)
			}
			foundTime := false
			for _, evidence := range binding.Evidence {
				if evidence.Extractor == "registration_receiver_call" && strings.HasPrefix(evidence.Label, `at("00:0`) {
					foundTime = true
				}
			}
			if !foundTime {
				t.Fatalf("graph lost authored schedule observation: %+v", binding)
			}
		}
		return
	}
	t.Fatal("callback declaration is missing from the consuming graph")
}

func TestCumulativePythonTypedParameterKeepsPossibleOriginalMethod(t *testing.T) {
	const prefix = "src/fixture_app/import_facades/"
	sources := []string{prefix + "exchange_impl.py", prefix + "exchange/__init__.py", prefix + "__init__.py", prefix + "typed_parameters.py"}
	repository := pythonCorpus(t, cumulativePythonSources(t, sources...))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	var method string
	for _, object := range index.Objects {
		if object.Name == "ws_connection_reset" && object.Location != nil && object.Location.Path == prefix+"exchange_impl.py" {
			method = object.ID
		}
	}
	if method == "" {
		t.Fatal("original method missing")
	}
	known, unknown := 0, 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Location.Path != prefix+"typed_parameters.py" {
			continue
		}
		if relation.Location.Line == 5 {
			known++
			if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 || relation.ToIDs[0] != method {
				t.Fatalf("written local type lost original possible method: %+v", relation)
			}
		} else {
			unknown++
			if relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
				t.Fatalf("unknown/reassigned/union parameter borrowed a method: %+v", relation)
			}
		}
	}
	if known != 1 || unknown != 5 {
		t.Fatalf("typed parameter calls known=%d unknown=%d", known, unknown)
	}
}
