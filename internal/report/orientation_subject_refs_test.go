package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Orientation keeps a cited member's target-qualified ref because two targets
// may both own n3. The summary and a role card must reach each member's own
// source, the way production stores subjects under subjectKey.
func TestOrientationQualifiedSubjectRefsReachEachTargetsSource(t *testing.T) {
	builder := &pageBuilder{data: &ReportData{}, links: newPageLinks(&ReportData{}), subjects: map[string]subjectRef{}}
	for _, member := range []struct{ target, path string }{{"t1", "alpha/apply.go"}, {"t2", "beta/apply.go"}} {
		subject := groupindex.Subject{ID: "n3", Object: &groupindex.ObjectFacts{Name: "Apply",
			Location: &programindex.Location{Path: member.path, Line: 3, Column: 1}}}
		builder.subjects[subjectKey(member.target, subject.ID)] = subjectRef{subject: subject, programTargetID: member.target}
	}
	builder.data.Orientation = &orientation.Result{Summary: "Both targets apply the items.", SummaryRefs: []string{"t1.n3", "t2.n3"}}
	view := &pageView{}
	builder.summary(view)
	if view.Summary == nil || len(view.Summary.Anchors) != 2 ||
		view.Summary.Anchors[0].Path != "alpha/apply.go" || view.Summary.Anchors[1].Path != "beta/apply.go" {
		t.Fatalf("qualified summary refs lost a target's member: %+v", view.Summary)
	}
	if anchors := builder.refAnchors([]string{"t9.n3"}); len(anchors) != 0 {
		t.Fatalf("an unknown qualified ref acquired an anchor: %+v", anchors)
	}
}
