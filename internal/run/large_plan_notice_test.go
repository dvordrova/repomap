package run

import (
	"bytes"
	"strings"
	"testing"
)

func TestLargePlanNoticeOnlySpeaksForALargePlan(t *testing.T) {
	var buffer bytes.Buffer
	output := newRunOutput(&buffer)
	notice := largePlanNotice(output, "Program categorization", "beets")
	notice(largePlanRequests - 1)
	if buffer.Len() != 0 {
		t.Fatalf("small plan reported: %q", buffer.String())
	}
	notice(820)
	rendered := buffer.String()
	if !strings.Contains(rendered, "provider requests: 820") ||
		!strings.Contains(rendered, "Program categorization") ||
		!strings.Contains(rendered, "beets") {
		t.Fatalf("large plan notice = %q", rendered)
	}
}

func TestLargePlanNoticeWithoutOutputIsNil(t *testing.T) {
	if largePlanNotice(nil, "stage", "target") != nil {
		t.Fatal("a notice was built with nowhere to write it")
	}
}
