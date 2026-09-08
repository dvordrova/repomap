package storefixture

import "net"

// These literal bind sites are inspected, never executed by analysis.
func ListenIPv6Loopback() { _, _ = net.Listen("tcp6", "[::1]:8080") }
func ListenIPv6Zone()     { _, _ = net.Listen("tcp6", "[fe80::1%eth0]:8080") }
func ListenIPv4Control()  { _, _ = net.Listen("tcp4", "127.0.0.1:8080") }
func ListenUnixControl()  { _, _ = net.Listen("unix", "/tmp/fixture.sock") }

// Invalid ports and runtime addresses do not establish a listen-address fact.
func ListenInvalidPort()                   { _, _ = net.Listen("tcp6", "[::1]:65536") }
func ListenComputedAddress(address string) { _, _ = net.Listen("tcp6", address) }
