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

// The value all a destination's walks end in stands on its card as the
// argument its calls hand over (ArgumentValue, read "Argument value"), never
// as what the code wrote for an input (Written) nor as its address: casdoor's
// Object Storage had read its object key "%s/%s" unlabelled.
func TestADestinationsWalkedValueIsItsArgumentValue(t *testing.T) {
	at := pageAnchor{Path: "util/path.go", Line: 61, Href: "#util-path-go-L61"}
	key := pageOutboundUse{Value: "%s/%s", Steps: []pageOutboundStep{{Name: "uploadFile"}, {Name: "UrlJoin", Anchor: at}}}
	view := pageView{Sections: []*pageSection{{ID: "t1", ShortLabel: "casdoor", Map: &pageMap{}, Outbound: []pageOutbound{
		{ID: "t1-out-put", Destination: "Object Storage", KindLabel: "SDK", Uses: []pageOutboundUse{key}},
		{ID: "t1-out-list", Destination: "Object Storage", KindLabel: "SDK", Uses: []pageOutboundUse{key}},
	}}}}
	for _, node := range view.SystemMap().Nodes {
		if node.Branch != "communication" || node.FullTitle != "Object Storage" || node.Children == "" {
			continue
		}
		if node.ArgumentValue != "%s/%s" || node.Written != "" || node.Source != at {
			t.Fatalf("the destination's walked value is not its argument value: %+v", node)
		}
		return
	}
	t.Fatal("no Object Storage destination on the system map")
}
