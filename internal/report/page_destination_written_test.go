package report

import "testing"

// A destination's reading says where the value naming what its calls reach
// ends, as the code wrote it: Redis's Primary, reached by gethostbyname
// through server.masterhost and by a connect whose address the walk could
// not read, says server.masterhost at its line. Two ends say none, and a
// started program is named by its word already.
func TestADestinationSaysWhereItsValueEnds(t *testing.T) {
	at := pageAnchor{Path: "redis.c", Line: 7219, Href: "#redis-c-L7219"}
	primary := []pageOutbound{
		{Uses: []pageOutboundUse{{Frontier: "server.masterhost", Steps: []pageOutboundStep{{Name: "anetTcpConnect"}, {Name: "server.masterhost", Anchor: at}}}}},
		{Uses: []pageOutboundUse{{Frontier: "(struct sockaddr*)&sa", Unread: true}}},
	}
	if written, source := destinationWritten(primary); written != "server.masterhost" || source != at {
		t.Fatalf("Primary reads %q at %+v, want server.masterhost at %+v", written, source, at)
	}
	two := append(primary, pageOutbound{Uses: []pageOutboundUse{{Value: "https://api.github.com"}}})
	if written, _ := destinationWritten(two); written != "" {
		t.Fatalf("two ends read %q", written)
	}
	if written, _ := destinationWritten([]pageOutbound{{Program: true, Uses: []pageOutboundUse{{Value: "litestream"}}}}); written != "" {
		t.Fatalf("a started program reads %q", written)
	}
}
