package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An address walk ending at a value its adapter could not read says the
// address is not established from code, never the expression: Redis's
// connect had read "Address passes through (struct sockaddr*)&sa". The
// chain keeps its steps.
func TestAnUnreadAddressIsNotEstablishedFromCode(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Outbound: []groupindex.OutboundCall{{
		ID: "b121", Kind: "client_request", External: "socket.h.connect", Location: programindex.Location{Path: "anet.c", Line: 158, Column: 9},
		Uses: []atlas.DestinationUse{{Frontier: "(struct sockaddr*)&sa", Unread: true, Steps: []atlas.DestinationStep{
			{SubjectID: "n787", Name: "anetTcpGenericConnect", Path: "anet.c", Line: 158, Column: 9},
			{SubjectID: "n787", Name: "(struct sockaddr*)&sa", Path: "anet.c", Line: 158, Column: 20},
		}}},
	}}}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionOutbound(section)
	row := section.Outbound[0]
	if uses := row.InformativeUses(); len(uses) != 1 || uses[0].FrontierName() != "" || !uses[0].Unread {
		t.Fatalf("an unread end is lost or named: %+v", uses)
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "outbound-row", row); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	if strings.Contains(page, "passes through") || !strings.Contains(page, "Argument value not established from code") || !strings.Contains(page, "anetTcpGenericConnect") {
		t.Fatalf("the unread end reads wrong:\n%s", page)
	}
}
