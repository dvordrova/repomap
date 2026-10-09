package contracttest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// kvcli's connect is written in net.c's netConnect, and the client reaches
// it two ways, from two parts: main's one command (kvcli.c) and the
// interactive mode's lines (repl.c), as Redis's anet.c connect is reached
// from syncWithMaster, cliConnect and createClient. Read end to end with the
// kvd preset (one part per file), the outgoing call names both callers with
// where each calls into net.c; the server, which never connects, has none.
func TestCFixtureConnectIsReachedFromTheClientsTwoParts(t *testing.T) {
	pair := readKvdPair(t)
	client, program := pair.indexes["kvcli"], pair.client
	fixture := sharedCFixture(t)
	mainLine, _ := fixture.at(t, "kvcli.c", "fd = netConnect(", "")
	replLine, _ := fixture.at(t, "repl.c", "fd = netConnect(host, port)", "")
	var connect *groupindex.OutboundCall
	for position := range client.Outbound {
		if strings.HasSuffix(client.Outbound[position].External, "socket.h.connect") {
			connect = &client.Outbound[position]
		}
	}
	if connect == nil {
		t.Fatalf("kvcli's connect is no outgoing call: %+v", client.Outbound)
	}
	got := reachedFromNames(client, program, *connect)
	want := []string{fmt.Sprintf("kvcli.c: main at kvcli.c:%d", mainLine), fmt.Sprintf("repl.c: repl at repl.c:%d", replLine)}
	if !slices.Equal(got, want) {
		t.Fatalf("kvcli's connect is reached from %q, want %q", got, want)
	}
	for _, call := range pair.indexes["kvd"].Outbound {
		if len(call.ReachedFrom) > 0 && call.Location.Path == "net.c" {
			t.Fatalf("the server reaches a net.c call it never makes: %+v", call)
		}
	}
	// The question naming what the connect reaches, asked once for its
	// destination, is told the same callers by name before the parts are
	// drawn: the first callers outside net.c.
	pair.preset.mu.Lock()
	asked := slices.Clone(pair.preset.reachedFrom)
	pair.preset.mu.Unlock()
	if want := "netConnect in net.c [main repl]"; !slices.Equal(asked, []string{want}) {
		t.Fatalf("the connect's destination was asked with %q, want %q", asked, want)
	}
}

// The same shape in each language's cumulative fixture, over its real
// ProgramIndex, with the parts a reader would draw given by the test: a
// helper that sends, in a part of its own with the rest of its file, called
// from two functions each in a part of its own. Each language already had
// the helper: Go's DestinationRequest, Python's and TypeScript's dispatch.
// The Clojure fixture has no function that sends and two callers of one
// (its only outgoing call, `revision`'s git, is called by nothing): a
// recorded missing equivalent.
func TestEveryLanguageReachesASendingHelperFromItsCallersParts(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		writePublishedGoFixtureModule(t, root)
		authorities := sharedGoFixtureAuthorities(t, root, repository, goFixtureAppPackage, "reached from")
		input, err := goadapter.BuildInput(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		// DestinationHandler's returned function makes its call: the
		// function stands in its maker's part.
		expectReachedFromParts(t, index, "internal/storefixture/destinations.go", "DestinationRequest",
			map[string]string{"DestinationHandler": "Price handlers", "DestinationApplication": "Application"})
	})
	t.Run("python", func(t *testing.T) {
		_, repository := materializeFixtureRepository(t, "python")
		catalog, err := pythontarget.Discover(t.Context(), repository)
		if err != nil {
			t.Fatal(err)
		}
		input, err := sharedPythonFixtureInput(t, repository, pythonFixtureTarget(t, catalog))
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		expectReachedFromParts(t, index, "src/fixture_app/destinations.py", "dispatch",
			map[string]string{"exchange_prices": "Exchange prices", "notify_orders": "Notifications"})
	})
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := sharedJSTSFixture(t, repository, root)
		if err != nil {
			t.Fatal(err)
		}
		expectReachedFromParts(t, index, "src/destinations.ts", "dispatch",
			map[string]string{"exchangePrices": "Exchange prices", "notifyOrders": "Notifications"})
	})
}

// expectReachedFromParts draws helper's file as one part without the named
// callers, each caller (with what it owns) as a part of its own, and every
// other declaration off the map, and checks that the helper's outgoing
// call is reached from exactly the callers' parts, each once.
func expectReachedFromParts(t *testing.T, index programindex.Index, path, helper string, callers map[string]string) {
	t.Helper()
	var helperObject programindex.Object
	parts := map[string][]string{}
	owned := map[string]bool{}
	for _, object := range index.Objects {
		// A function a caller returns stands in its part: Go names it
		// after its maker (DestinationHandler$1).
		name, _, _ := strings.Cut(object.Name, "$")
		if title, caller := callers[name]; caller && object.Kind == programindex.ObjectFunction && object.Location != nil && object.Location.Path == path {
			parts[title] = append(parts[title], object.ID)
			owned[object.ID] = true
		}
	}
	for _, object := range index.Objects {
		// What a caller owns stands in its part.
		if object.Location == nil || object.Location.Path != path || owned[object.ID] || owned[object.OwnerID] || owned[object.ContainerID] {
			continue
		}
		if object.Kind == programindex.ObjectModule {
			continue
		}
		if object.Name == helper && object.Kind == programindex.ObjectFunction {
			helperObject = object
		}
		parts["Sending"] = append(parts["Sending"], object.ID)
	}
	if helperObject.ID == "" || len(parts) != len(callers)+1 {
		t.Fatalf("%s: helper %s or its callers %v are not declared: %v", path, helper, callers, parts)
	}
	titles := make([]string, 0, len(parts))
	for title := range parts {
		titles = append(titles, title)
	}
	slices.Sort(titles)
	target := atlas.Target{ID: index.Target.ID, Name: index.Target.Name, Language: index.Target.Language, Kind: index.Target.Kind, Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}}
	for position, title := range titles {
		target.Boxes = append(target.Boxes, atlas.Box{ID: fmt.Sprintf("b%d", position), Dir: ".", Title: title, Side: atlas.SideMid, MemberIDs: parts[title],
			Files: []atlas.File{{Path: path, Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}})
		if title == "Sending" {
			target.Boundaries = []atlas.Boundary{{ID: "send", ObjectID: helperObject.ID, BoxID: target.Boxes[position].ID, Direction: atlas.DirectionOut,
				Kind: atlas.BoundaryClientRequest, Destination: "Price service", Source: "model", Path: path, LineNo: helperObject.Location.Line + 1, Column: 1,
				Values: []string{}, Line: "Sends the request."}}
		}
	}
	value := atlas.Atlas{Version: atlas.Version, Repository: "fixture", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, value)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 || len(indexes[0].Outbound) != 1 {
		t.Fatalf("the helper's call is not one outgoing call: %+v", indexes)
	}
	var reached []string
	titleOf := map[string]string{}
	for _, group := range indexes[0].Groups {
		titleOf[group.ID] = group.Title
	}
	for _, caller := range indexes[0].Outbound[0].ReachedFrom {
		if caller.Location == nil || caller.Location.Path != path {
			t.Fatalf("a caller is not a call into %s's part: %+v", helper, caller)
		}
		reached = append(reached, titleOf[caller.GroupID])
	}

	want := slices.Sorted(func(yield func(string) bool) {
		for _, title := range callers {
			if !yield(title) {
				return
			}
		}
	})
	slices.Sort(reached)
	if !slices.Equal(reached, want) {
		t.Fatalf("%s's call is reached from the parts %q, want %q", helper, reached, want)
	}
}

// reachedFromNames reads a call's callers as "part: name at path:line".
func reachedFromNames(index groupindex.Index, program programindex.Index, call groupindex.OutboundCall) []string {
	names := map[string]string{}
	for _, object := range program.Objects {
		names[object.ID] = object.Name
	}
	titles := map[string]string{}
	for _, group := range index.Groups {
		titles[group.ID] = group.Title
	}
	var result []string
	for _, caller := range call.ReachedFrom {
		at := "no site"
		if caller.Location != nil {
			at = fmt.Sprintf("%s:%d", caller.Location.Path, caller.Location.Line)
		}
		result = append(result, strings.TrimSpace(titles[caller.GroupID]+": "+names[caller.SubjectID]+" at "+at))
	}
	slices.Sort(result)
	return result
}
