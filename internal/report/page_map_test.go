package report

import (
	"reflect"
	"strings"
	"testing"
)

func TestMapTitleWrapsAndOnlyCutsWhatCannotFit(t *testing.T) {
	for _, test := range []struct {
		title string
		want  []string
	}{
		{"Domain models", []string{"Domain models"}},
		{"Application bootstrap and routing", []string{"Application bootstrap and", "routing"}},
		{"Simulation rendering and animation", []string{"Simulation rendering and", "animation"}},
		{"", []string{""}},
		{
			"Averyveryverylongsinglewordthatcannotfit",
			[]string{"Averyveryverylongsinglewo…"},
		},
	} {
		got := mapTitle(test.title)
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("mapTitle(%q) = %#v, want %#v", test.title, got, test.want)
		}
		for _, line := range got {
			if len([]rune(line)) > mapTitleBudget {
				t.Fatalf("mapTitle(%q) line %q is %d runes, over the %d budget",
					test.title, line, len([]rune(line)), mapTitleBudget)
			}
		}
		if len(got) > mapTitleLines {
			t.Fatalf("mapTitle(%q) used %d lines", test.title, len(got))
		}
	}
}

// TestMapTitleMarksWhatItDropped keeps a cut visible: a reader must be able to
// tell a whole name from a shortened one.
func TestMapTitleMarksWhatItDropped(t *testing.T) {
	long := "Level drawing and rendering for the simulation canvas"
	got := mapTitle(long)
	if len(got) != mapTitleLines {
		t.Fatalf("mapTitle(%q) = %#v", long, got)
	}
	if !strings.HasSuffix(got[len(got)-1], "…") {
		t.Fatalf("a shortened title does not say so: %#v", got)
	}
}

// TestAddressValueSeparatesAPortFromATime keeps "where to reach them" from
// listing something that merely has the shape of an address.
func TestAddressValueSeparatesAPortFromATime(t *testing.T) {
	for _, value := range []string{
		":8080", "localhost:8080", "0.0.0.0:3000", "http://localhost:8080",
		"https://example.test:443/path", "127.0.0.1:65535",
	} {
		if !addressValue(value) {
			t.Fatalf("addressValue(%q) = false, want true", value)
		}
	}
	for _, value := range []string{
		"", "12:30", "8080", "localhost", "x:0", "x:65536", "a b:80", "1:2",
	} {
		if addressValue(value) {
			t.Fatalf("addressValue(%q) = true, want false", value)
		}
	}
}
