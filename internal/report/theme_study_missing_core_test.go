package report

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestDeriveMissingCoreAreasUsesExactDeclarationAndSharedMemberLocations(t *testing.T) {
	t.Parallel()

	locatedMember := func(id, path string, line int) componentmap.Candidate {
		return componentmap.Candidate{
			ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: id},
			Role: componentmap.CandidateRoleConceptualMember,
			Name: id,
			Facts: []componentmap.LocalFact{{
				Kind:      componentmap.FactDeclaration,
				Value:     id,
				Location:  &evidence.Location{Path: path, Line: line},
				Certainty: evidence.CertaintyStatic,
			}},
		}
	}
	data := &ReportData{ArchitectureCanvas: &ArchitectureCanvas{
		LocalRemainderComponentID: "remainder",
		Components: []ArchitectureComponent{
			{
				ID:      "application",
				Name:    "Application Entry",
				Members: []componentmap.Candidate{locatedMember("app", "app.go", 35)},
			},
			{
				ID:            "shared-config",
				Name:          "Configuration Parsing",
				SharedMembers: []componentmap.Candidate{locatedMember("config", "config/parse.go", 14)},
			},
			{
				ID:      "storage",
				Name:    "Storage Engine",
				Members: []componentmap.Candidate{locatedMember("store", "store/db.go", 21)},
			},
			{
				ID:         "hypothesis",
				Name:       "Hypothesis",
				Hypothesis: true,
			},
			{ID: "remainder", Name: "Local remainder"},
		},
	}}
	themes := themestudy.StudyThemes{Cards: []themestudy.ThemeCard{
		{Readings: []themestudy.Reading{{Path: "app.go"}}},
		{AlternateReadings: []themestudy.Reading{{Path: "config/parse.go"}}},
	}}

	count, names := deriveMissingCoreAreas(themes, data)
	if count != 1 || !reflect.DeepEqual(names, []string{"Storage Engine"}) {
		t.Fatalf("missing core areas = %d %#v, want only Storage Engine", count, names)
	}
}

func TestDeriveMissingCoreAreasKeepsExactCountWithBoundedNames(t *testing.T) {
	t.Parallel()

	components := make([]ArchitectureComponent, 0, 14)
	for index := 0; index < 14; index++ {
		components = append(components, ArchitectureComponent{
			ID:   componentmap.ComponentID(fmt.Sprintf("component-%02d", index)),
			Name: fmt.Sprintf("Component %02d", index),
		})
	}
	count, names := deriveMissingCoreAreas(
		themestudy.StudyThemes{},
		&ReportData{ArchitectureCanvas: &ArchitectureCanvas{Components: components}},
	)
	if count != 14 || len(names) != 12 {
		t.Fatalf("missing core areas = %d names=%d, want exact 14 and bounded 12", count, len(names))
	}
}
