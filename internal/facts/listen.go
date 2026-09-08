package facts

import (
	"net"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// "On which port" is a first-day question, and for a program that hard-codes
// its address the answer is in the code rather than in a manifest or an
// environment key with no default. This captures the literal a server binds
// to, at the line that binds it.
//
// Only a string literal is captured. An address computed at run time is not a
// fact about the repository, and guessing one would be worse than leaving the
// question to the configuration rows that already answer it. A numeric literal
// port, as JavaScript writes it, is not captured either: the index records no
// numeric argument value to read.

// listenSelectors maps a bind call to the argument position of its address.
var listenSelectors = map[string]int{
	"listenandserve":    1,
	"listenandservetls": 1,
	"listen":            1,
	"serve":             1,
}

// listenNetworks are the first arguments that mean the address is next, as in
// net.Listen("tcp", ":8080").
// maxTCPPort is the largest number a TCP or UDP port can be. It is the
// protocol's own bound, not a product limit.
const maxTCPPort = 65535

var listenNetworks = map[string]struct{}{
	"tcp": {}, "tcp4": {}, "tcp6": {}, "unix": {}, "unixpacket": {}, "udp": {}, "udp4": {}, "udp6": {},
}

func (b *builder) addListenAddresses(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			position, known := listenSelectors[strings.ToLower(pattern.Selector)]
			if !known || pattern.Form != programindex.PatternCall {
				continue
			}
			address, ok := listenAddress(pattern, position)
			if !ok {
				continue
			}
			anchor := target.patternAnchor(relation, pattern)
			if anchor == nil {
				continue
			}
			if !b.once(strings.Join([]string{string(KindListenAddress), anchor.String(), address}, "\x00")) {
				continue
			}
			symbol, objectID := target.enclosingSymbol(relation.FromID)
			b.add(target.root, Fact{
				Kind:       KindListenAddress,
				TargetID:   target.target.ID,
				Anchor:     anchor,
				Key:        pattern.Selector,
				Value:      address,
				Symbol:     symbol,
				ObjectID:   objectID,
				Resolution: ResolutionExact,
			}, address)
		}
	}
}

// listenAddress reads the literal address, stepping past a leading network
// name when the call names one.
func listenAddress(pattern programindex.RelationPattern, position int) (string, bool) {
	argument, found := positionalArgument(pattern, position)
	if !found {
		return "", false
	}
	value, templated, literal := literalValue(argument)
	if !literal || templated {
		return "", false
	}
	if _, network := listenNetworks[strings.ToLower(value)]; network {
		return listenAddress(pattern, position+1)
	}
	if isListenAddress(value) {
		return value, true
	}
	return "", false
}

// isListenAddress accepts what a bind call is actually given: ":8080",
// "127.0.0.1:8080", "[::1]:8080", a scoped IPv6 address, or a unix socket path.
func isListenAddress(value string) bool {
	if value == "" || strings.ContainsAny(value, " \t") {
		return false
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") {
		return true
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || !isPortNumber(port) {
		return false
	}
	return host == "" || !strings.Contains(host, "/")
}

func isPortNumber(value string) bool {
	number, err := strconv.Atoi(value)
	return err == nil && number > 0 && number <= maxTCPPort
}
