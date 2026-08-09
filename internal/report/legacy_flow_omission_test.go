package report

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestReportDataOmitsEmptyLegacyFlowsAndPreservesNonEmptyValues(t *testing.T) {
	t.Parallel()

	t.Run("nil fields are absent", func(t *testing.T) {
		fields := reportDataJSONFields(t, mustMarshalReportData(t, ReportData{}))
		assertLegacyFlowFieldPresence(t, fields, false)
	})

	t.Run("nonempty fields are preserved", func(t *testing.T) {
		fields := reportDataJSONFields(t, mustMarshalReportData(t, ReportData{
			CandidateFlows: []string{"legacy-candidate"},
			Flows:          []FlowData{{ID: "legacy-flow"}},
		}))
		assertLegacyFlowFieldPresence(t, fields, true)

		var candidates []string
		if err := json.Unmarshal(fields["candidate_flows"], &candidates); err != nil {
			t.Fatalf("decode candidate_flows: %v", err)
		}
		if len(candidates) != 1 || candidates[0] != "legacy-candidate" {
			t.Fatalf("candidate_flows = %#v, want preserved legacy value", candidates)
		}

		var flows []FlowData
		if err := json.Unmarshal(fields["flows"], &flows); err != nil {
			t.Fatalf("decode flows: %v", err)
		}
		if len(flows) != 1 || flows[0].ID != "legacy-flow" {
			t.Fatalf("flows = %#v, want preserved legacy value", flows)
		}
	})
}

func TestRenderHTMLOmitsEmptyLegacyFlowsAndPreservesNonEmptyValues(t *testing.T) {
	t.Parallel()

	t.Run("nil fields are absent from embedded data", func(t *testing.T) {
		html := mustRenderReportHTML(t, &ReportData{})
		fields := reportDataJSONFields(t, embeddedReportDataJSON(t, html))
		assertLegacyFlowFieldPresence(t, fields, false)
	})

	t.Run("nonempty fields are present in embedded data", func(t *testing.T) {
		html := mustRenderReportHTML(t, &ReportData{
			CandidateFlows: []string{"legacy-candidate"},
			Flows:          []FlowData{{ID: "legacy-flow"}},
		})
		fields := reportDataJSONFields(t, embeddedReportDataJSON(t, html))
		assertLegacyFlowFieldPresence(t, fields, true)
	})
}

func mustMarshalReportData(t *testing.T, data ReportData) []byte {
	t.Helper()

	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal ReportData: %v", err)
	}
	return payload
}

func mustRenderReportHTML(t *testing.T, data *ReportData) []byte {
	t.Helper()

	html, err := RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML(): %v", err)
	}
	return html
}

func embeddedReportDataJSON(t *testing.T, html []byte) []byte {
	t.Helper()

	const opening = `<script type="application/json" id="rm-report-data">`
	start := bytes.Index(html, []byte(opening))
	if start < 0 {
		t.Fatal("rendered HTML is missing rm-report-data")
	}
	start += len(opening)
	endOffset := bytes.Index(html[start:], []byte("</script>"))
	if endOffset < 0 {
		t.Fatal("rendered HTML has an unterminated rm-report-data payload")
	}
	return html[start : start+endOffset]
}

func reportDataJSONFields(t *testing.T, payload []byte) map[string]json.RawMessage {
	t.Helper()

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode ReportData JSON: %v", err)
	}
	return fields
}

func assertLegacyFlowFieldPresence(
	t *testing.T,
	fields map[string]json.RawMessage,
	want bool,
) {
	t.Helper()

	for _, field := range []string{"candidate_flows", "flows"} {
		_, exists := fields[field]
		if exists != want {
			t.Errorf("field %q presence = %t, want %t", field, exists, want)
		}
	}
}
