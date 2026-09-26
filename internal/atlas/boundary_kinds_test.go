package atlas

import (
	"strings"
	"testing"
)

// The owner's rule for the kinds a model chooses among: one kind carries
// every protocol, in both directions. A raw socket connect, an RPC and an
// HTTP request sent to another service are all one outgoing kind, as every
// request a client sends in is `request`; no kind names a protocol, so none
// can call a TCP connection HTTP.
func TestBoundaryKindsNameNoProtocol(t *testing.T) {
	kinds := append(BoundaryKinds(), OutgoingBoundaryKinds()...)
	kinds = append(kinds, IncomingBoundaryKinds()...)
	for _, kind := range kinds {
		for _, protocol := range []string{"http", "grpc", "rpc", "tcp", "udp", "websocket", "socket"} {
			if strings.Contains(strings.ToLower(kind), protocol) {
				t.Fatalf("boundary kind %q names the protocol %s", kind, protocol)
			}
		}
	}
	outgoing := false
	for _, kind := range OutgoingBoundaryKinds() {
		outgoing = outgoing || kind == BoundaryClientRequest
	}
	if !outgoing {
		t.Fatalf("no outgoing kind carries a request to another service: %v", OutgoingBoundaryKinds())
	}
}
