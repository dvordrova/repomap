package report

import (
	"maps"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A destination the reading chose as one of the report's programs is that
// program with no integration connection: freqtrade-client's one generic
// request reaches freqtrade, where it had stood as "Freqtrade Server"
// outside. Another destination keeps what it names.
func TestADestinationChosenAsAProgramJoinsIt(t *testing.T) {
	client := groupindex.Index{Target: programindex.Target{ID: "t7"}, Outbound: []groupindex.OutboundCall{
		{ID: "b1", SubjectID: "n1", Kind: atlas.BoundaryClientRequest, External: "requests.Session.request", Destination: "freqtrade", DestinationTarget: "t1", Location: programindex.Location{Path: "ft_client/freqtrade_client/ft_rest_client.py", Line: 61, Column: 34}, Source: "model"},
		{ID: "b2", SubjectID: "n2", Kind: atlas.BoundaryClientRequest, External: "requests.get", Destination: "GitHub", Location: programindex.Location{Path: "ft_client/freqtrade_client/ft_client.py", Line: 9, Column: 5}, Source: "model"},
	}}
	server := groupindex.Index{Target: programindex.Target{ID: "t1"}}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{server, client}}
	serverSection := &pageSection{ID: "t1", programTargetID: "t1", ShortLabel: "freqtrade"}
	clientSection := &pageSection{ID: "t7", programTargetID: "t7", ShortLabel: "freqtrade-client"}
	builder.sections = []*pageSection{serverSection, clientSection}
	builder.byProgram = map[string]*pageSection{"t1": serverSection, "t7": clientSection}
	builder.fillSectionOutbound(clientSection)
	got := map[string]string{}
	for _, row := range clientSection.Outbound {
		var reached []string
		for _, program := range row.Runs {
			reached = append(reached, program.Title)
		}
		got[row.ID] = strings.Join(reached, ",")
	}
	if want := map[string]string{"t7-out-b1": "freqtrade", "t7-out-b2": ""}; !maps.Equal(got, want) {
		t.Fatalf("records reach %v\nwant %v", got, want)
	}
}
