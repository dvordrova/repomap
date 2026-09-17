package storefixture

import (
	"net"
	"testing"
)

// These literal bind sites are inspected, never executed by analysis.
func ListenIPv6Loopback() { _, _ = net.Listen("tcp6", "[::1]:8080") }
func ListenIPv6Zone()     { _, _ = net.Listen("tcp6", "[fe80::1%eth0]:8080") }
func ListenIPv4Control()  { _, _ = net.Listen("tcp4", "127.0.0.1:8080") }
func ListenUnixControl()  { _, _ = net.Listen("unix", "/tmp/fixture.sock") }

// Invalid ports and runtime addresses do not establish a listen-address fact.
func ListenInvalidPort()                   { _, _ = net.Listen("tcp6", "[::1]:65536") }
func ListenComputedAddress(address string) { _, _ = net.Listen("tcp6", address) }

// A framework receives a command as a value of its own type: the callable in
// one field, the name in another. Go constructs where other languages call.
func BuildCommands() []testing.InternalTest {
	return []testing.InternalTest{{Name: "restore-state", F: restoreStateCommand}}
}

func restoreStateCommand(*testing.T) {}
