package pythonprogramindex

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// cumulativeCalls names every call written in one fixture file, by its
// source line: its resolution and the declarations or outside symbols it
// names, a class's method as Class.method.
func cumulativeCalls(t *testing.T, index programindex.Index, files map[string]string, path string) map[string][]string {
	t.Helper()
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	lines := strings.Split(files[path], "\n")
	got := map[string][]string{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		if relation.Location == nil || relation.Location.Path != path {
			continue
		}
		row := string(relation.Resolution)
		for _, id := range relation.ToIDs {
			object := objects[id]
			name := object.Name
			if owner, ok := objects[object.OwnerID]; ok && object.Kind == programindex.ObjectMethod {
				name = owner.Name + "." + name
			}
			row += " " + name
		}
		line := strings.TrimSpace(lines[relation.Location.Line-1])
		got[line] = append(got[line], row)
	}
	for line := range got {
		slices.Sort(got[line])
	}
	return got
}

// inherited_clients.py keeps its clients the way freqtrade's Exchange keeps
// its ccxt client: a call on self is the method the class declares or
// inherits along its chain of single repository bases, and a field stored
// once holds the outside type its store gives it, the declared return type
// of a repository factory or a parameter's annotation, in the subclasses
// too. A store in a class deriving from it, a union, two bases and a base
// outside the repository each leave the call unresolved.
func TestCumulativePythonSelfCallsAndFieldsFollowTheBaseChain(t *testing.T) {
	const path = "src/fixture_app/inherited_clients.py"
	files := cumulativePythonSources(t, path)
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	got := cumulativeCalls(t, index, files, path)
	want := map[string][]string{
		// Prices and MaybePrices write the same line, one call each.
		"self.client = self._make_client(base)":                {"exact MaybePrices._make_client", "exact Prices._make_client"},
		"return httpx.Client(base_url=base)":                   {"exact httpx.Client"},
		`return self.client.get("/prices/" + pair)`:            {"exact httpx.Client.get"},
		"self.ask(pair)":                                       {"exact Prices.ask"},
		`return self.client.get("/funding/" + pair)`:           {"exact httpx.Client.get"},
		`return self.client.get("/stream")`:                    {"exact httpx.Client.get"},
		"self.client = httpx.Client(base_url=base)":            {"exact httpx.Client"},
		`return self.client.get("/last")`:                      {"unresolved"},
		"self.client = httpx.AsyncClient(base_url=base)":       {"exact httpx.AsyncClient"},
		"return httpx.Client(base_url=base) if base else None": {"exact httpx.Client"},
		`return self.client.get("/prices")`:                    {"unresolved"},
		`return self.ask("BTC")`:                               {"unresolved"},
		"return self.start()":                                  {"unresolved"},
		"return httpx.post(self.url, json=payload)":            {"exact httpx.post"},
		`return self.send({"content": text})`:                  {"exact Webhook.send"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls in inherited_clients.py:\n have %q\n want %q", got, want)
	}
}

// Webhook.send reads self.url, which Webhook and Discord each store in
// their __init__: the address httpx.post is given is one of the two stores,
// each listed with its own line, neither chosen, as freqtrade's Webhook
// sends to its own or to Discord's URL.
func TestCumulativePythonBaseReadTakesEachSubclassStore(t *testing.T) {
	const path = "src/fixture_app/inherited_clients.py"
	repository := pythonCorpus(t, cumulativePythonSources(t, path))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	reader := reading.NewDestinationReader(graph.Places)
	found := false
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Symbol.Decl.Name != "Webhook.send" {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API == nil || call.API.Package != "httpx" || call.API.Name != "post" {
				continue
			}
			found = true
			var origin *sourcevalue.Value
			for _, argument := range call.SourceArguments {
				if argument.Position == 1 {
					origin = argument.Origin
				}
			}
			if origin == nil || origin.Kind != "field" || origin.Text != "url" || origin.Initializer == nil || origin.Initializer.Kind != "alternatives" {
				t.Fatalf("self.url lost its stores: %+v", origin)
			}
			var stores []int
			for _, store := range origin.Initializer.Parts {
				if store.Kind != "field_value" || store.Anchor == nil || store.Parts[0].Kind != "index" {
					t.Fatalf("store is not the written value: %+v", store)
				}
				stores = append(stores, store.Anchor.Line)
			}
			if !slices.Equal(stores, []int{86, 94}) {
				t.Fatalf("stores of self.url at lines %v, want Webhook's 86 and Discord's 94", stores)
			}
			var frontiers []string
			for _, use := range reader.Read(place, call) {
				frontiers = append(frontiers, use.Frontier)
			}
			slices.Sort(frontiers)
			if len(frontiers) != 2 || !strings.Contains(frontiers[0], "discord") || !strings.Contains(frontiers[1], "webhook") {
				t.Fatalf("destination reading of self.url: %q, want Discord's and Webhook's address", frontiers)
			}
		}
	}
	if !found {
		t.Fatal("Webhook.send has no httpx.post call")
	}
}

// outside_results.py builds its bot the way freqtrade's Telegram does: a
// call on a call's result is a member of what that call produces, a field
// stored from an untyped repository factory holds the outside call its one
// return statement returns, and functools.partial(f, ...) hands f over. A
// factory with two returns gives no type. runtime_registrations.py's
// scheduler chain continues the outside symbol through an attribute.
func TestCumulativePythonCallsOnCallResultsAndPartial(t *testing.T) {
	const path = "src/fixture_app/outside_results.py"
	const registrations = "src/fixture_app/runtime_registrations.py"
	files := cumulativePythonSources(t, path, registrations, "src/fixture_app/workers.py")
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	got := cumulativeCalls(t, index, files, path)
	const builder = "telegram.ext.Application.builder"
	want := map[string][]string{
		"with Path(name).open() as handle:": {"exact pathlib.Path", "exact pathlib.Path.open"},
		"return handle.read()":              {"unresolved"},
		// The class call, the __init__ Worker declares, and the run it
		// inherits from BaseWorker.
		"return Worker(name, 1).run()":                                                               {"exact BaseWorker.run", "exact Worker", "exact Worker.__init__"},
		"self._app = self._build_app(token)":                                                         {"exact Bot._build_app"},
		"self._fallback = self._either_app(token, False)":                                            {"exact Bot._either_app"},
		"return Application.builder().token(token).build()":                                          {"exact " + builder, "exact " + builder, "exact " + builder + ".token", "exact " + builder + ".token", "exact " + builder + ".token.build", "exact " + builder + ".token.build"},
		"return Application.builder().token(token).local_mode(True).build()":                         {"exact " + builder, "exact " + builder + ".token", "exact " + builder + ".token.local_mode", "exact " + builder + ".token.local_mode.build"},
		`self._app.add_handler(CommandHandler("forcebuy", partial(self._force_enter, side="long")))`: {"exact functools.partial", "exact " + builder + ".token.build.add_handler", "exact telegram.ext.CommandHandler"},
		`self._app.add_handler(CommandHandler("status", self._status))`:                              {"exact " + builder + ".token.build.add_handler", "exact telegram.ext.CommandHandler"},
		"return self._app.bot.send_message(chat, text)":                                              {"exact " + builder + ".token.build.bot.send_message"},
		"return self._fallback.bot.send_message(chat, text)":                                         {"unresolved"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls in outside_results.py:\n have %q\n want %q", got, want)
	}
	chain := cumulativeCalls(t, index, files, registrations)[`scheduler.every().day.at("00:07").do(persist_wallet)`]
	if !slices.Equal(chain, []string{"exact schedule.Scheduler.every", "exact schedule.Scheduler.every.day.at", "exact schedule.Scheduler.every.day.at.do"}) {
		t.Fatalf("scheduler chain: %q", chain)
	}

	result, err := facts.Build(facts.Input{Repository: repository, TrackedPaths: repository.VisiblePaths(), Targets: []facts.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	handlers := map[string][]string{}
	for _, registration := range result.OfKind(facts.KindRegistration) {
		if registration.Anchor == nil || registration.Anchor.Path != path || registration.ObjectID == "" {
			continue
		}
		name := objectByID(t, index, registration.ObjectID).Name
		handlers[name] = append(handlers[name], registration.Text+" "+registration.Path)
	}
	for name := range handlers {
		slices.Sort(handlers[name])
	}
	if !slices.Contains(handlers["_force_enter"], "telegram.ext.CommandHandler forcebuy") ||
		!slices.Contains(handlers["_status"], "telegram.ext.CommandHandler status") {
		t.Fatalf("CommandHandler hands over %v; want _force_enter through partial, as _status directly", handlers)
	}
}
