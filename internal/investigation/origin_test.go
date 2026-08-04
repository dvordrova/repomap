package investigation

import (
	"reflect"
	"testing"
)

func TestReducePreservesOrientationOriginOnSameRevisionRedirect(t *testing.T) {
	t.Parallel()

	origin := validOrientationOrigin()
	session := startWithOrigin(t, origin)
	if session.Origin == nil || !reflect.DeepEqual(*session.Origin, origin) {
		t.Fatalf("started origin = %#v, want %#v", session.Origin, origin)
	}

	session = reduceFixture(t, session, Event{
		Kind: EventRedirected,
		Redirect: &RedirectInput{
			Goal:     Goal{Text: "follow the selected flow through delete"},
			Focus:    Focus{Kind: FocusSymbol, Symbol: "kvServer.DeleteRange"},
			Revision: "revision-1",
		},
	})
	if session.Origin == nil || !reflect.DeepEqual(*session.Origin, origin) {
		t.Fatalf("redirected origin = %#v, want %#v", session.Origin, origin)
	}
}

func TestReduceClearsOrientationOriginOnRepositoryChange(t *testing.T) {
	t.Parallel()

	session := startWithOrigin(t, validOrientationOrigin())
	session = reduceFixture(t, session, Event{
		Kind:     EventRepositoryChanged,
		Revision: "revision-2",
	})
	if session.Origin != nil {
		t.Fatalf("repository-changed origin = %#v, want nil", session.Origin)
	}
}

func TestReduceAcceptsComponentAnchorOriginWithoutFlow(t *testing.T) {
	t.Parallel()

	origin := validOrientationOrigin()
	origin.Kind = OriginOrientationComponent
	origin.FlowID = ""
	origin.FlowName = ""
	origin.ComponentID = "component-7bfa93f359a9"
	origin.AnchorID = "anchor-1d759c749297"
	session := startWithOrigin(t, origin)
	if session.Origin == nil || !reflect.DeepEqual(*session.Origin, origin) {
		t.Fatalf("component origin = %#v, want %#v", session.Origin, origin)
	}
}

func TestReduceAcceptsCanonicalSlashRepositoryIdentity(t *testing.T) {
	t.Parallel()

	origin := validOrientationOrigin()
	origin.RepoName = "example.com/owner/repository"
	session := startWithOrigin(t, origin)
	if session.Origin == nil || session.Origin.RepoName != origin.RepoName {
		t.Fatalf("canonical origin = %#v, want repository identity %q", session.Origin, origin.RepoName)
	}
}

func TestReduceRejectsInvalidOrientationOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Origin)
	}{
		{
			name: "kind",
			mutate: func(origin *Origin) {
				origin.Kind = "other"
			},
		},
		{
			name: "status",
			mutate: func(origin *Origin) {
				origin.Status = "confirmed"
			},
		},
		{
			name: "report hash",
			mutate: func(origin *Origin) {
				origin.ReportSHA256 = "not-a-sha256"
			},
		},
		{
			name: "repository traversal",
			mutate: func(origin *Origin) {
				origin.RepoName = "../repo"
			},
		},
		{
			name: "repository absolute path",
			mutate: func(origin *Origin) {
				origin.RepoName = "/parent/repo"
			},
		},
		{
			name: "repository backslash",
			mutate: func(origin *Origin) {
				origin.RepoName = `parent\repo`
			},
		},
		{
			name: "flow id",
			mutate: func(origin *Origin) {
				origin.FlowID = "HTTP Flow"
			},
		},
		{
			name: "flow name",
			mutate: func(origin *Origin) {
				origin.FlowName = "request\nresponse"
			},
		},
		{
			name: "accepted revision",
			mutate: func(origin *Origin) {
				origin.AcceptedRevision = "revision-2"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			origin := validOrientationOrigin()
			test.mutate(&origin)
			next, actions, err := Reduce(Session{}, Event{
				Kind: EventStarted,
				Start: &StartInput{
					Goal:       Goal{Text: "investigate selected flow"},
					Repository: Repository{Path: "/repo", Revision: "revision-1"},
					Focus:      Focus{Kind: FocusSymbol, Symbol: "kvServer.Put"},
					Origin:     &origin,
				},
			})
			if err == nil {
				t.Fatal("Reduce(start) error = nil")
			}
			if !reflect.DeepEqual(next, Session{}) || len(actions) != 0 {
				t.Fatalf("rejected start returned session=%#v actions=%#v", next, actions)
			}
		})
	}
}

func TestReduceRejectsRedirectThatChangesRepositoryRevision(t *testing.T) {
	t.Parallel()

	session := startWithOrigin(t, validOrientationOrigin())
	original := cloneFixture(t, session)
	next, actions, err := Reduce(session, Event{
		Kind: EventRedirected,
		Redirect: &RedirectInput{
			Goal:     Goal{Text: "follow the selected flow through delete"},
			Focus:    Focus{Kind: FocusSymbol, Symbol: "kvServer.DeleteRange"},
			Revision: "revision-2",
		},
	})
	if err == nil {
		t.Fatal("Reduce(redirect) error = nil")
	}
	if len(actions) != 0 || !reflect.DeepEqual(next, original) || !reflect.DeepEqual(session, original) {
		t.Fatal("rejected redirect mutated the session")
	}
}

func startWithOrigin(t *testing.T, origin Origin) Session {
	t.Helper()

	session, actions, err := Reduce(Session{}, Event{
		Kind: EventStarted,
		Start: &StartInput{
			Goal:       Goal{Text: "investigate selected flow"},
			Repository: Repository{Path: "/repo", Revision: "revision-1"},
			Focus:      Focus{Kind: FocusSymbol, Symbol: "kvServer.Put"},
			Origin:     &origin,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || !reflect.DeepEqual(actions, session.Next) {
		t.Fatalf("actions = %#v session.next = %#v", actions, session.Next)
	}
	return session
}

func validOrientationOrigin() Origin {
	return Origin{
		Kind:             OriginOrientationFlow,
		Status:           OriginCandidate,
		ReportSHA256:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RepoName:         "etcd",
		FlowID:           "flow-http-request",
		FlowName:         "HTTP/gRPC request",
		AcceptedRevision: "revision-1",
	}
}
