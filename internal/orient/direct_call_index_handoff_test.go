package orient

import (
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestDeliverDirectCallIndexIsLiveRunOnlyAndExact(t *testing.T) {
	want := surfacediscovery.DirectCallIndex{
		Version: surfacediscovery.DirectCallIndexVersion,
		State:   surfacediscovery.DirectCallIndexReady,
		SHA256:  "exact-index-digest",
	}
	called := 0
	var got surfacediscovery.DirectCallIndex
	opts := Options{DirectCallIndexSink: func(index surfacediscovery.DirectCallIndex) {
		called++
		got = index
	}}

	deliverDirectCallIndex(opts, nil)
	if called != 0 {
		t.Fatalf("nil index invoked sink %d time(s)", called)
	}
	deliverDirectCallIndex(opts, &want)
	if called != 1 || got.Version != want.Version || got.State != want.State || got.SHA256 != want.SHA256 {
		t.Fatalf("direct-call handoff = calls:%d index:%+v, want exact %+v", called, got, want)
	}

	// A missing consumer is an intentional no-op: ordinary local artifact
	// production remains unchanged until a live Study investigation asks for
	// the private substrate.
	deliverDirectCallIndex(Options{}, &want)
}
