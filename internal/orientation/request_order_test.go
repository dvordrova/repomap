package orientation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
)

func TestQualifiedCompactRefsUseNaturalOrder(t *testing.T) {
	refs := []string{"t10.g1", "t2.g10", "t2.g2", "t1.g20", "t1.g3"}
	sort.Slice(refs, func(i, j int) bool { return compactRefLess(refs[i], refs[j]) })
	want := []string{"t1.g3", "t1.g20", "t2.g2", "t2.g10", "t10.g1"}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("qualified refs = %v, want %v", refs, want)
	}
}

// Independent original comparator: invalid later segments must not change an
// earlier numeric decision, and overflowing/zero-prefixed refs stay lexical.
func originalCompactRefLess(left, right string) bool {
	segment := func(value string) (string, int, bool) {
		first := strings.IndexFunc(value, func(r rune) bool { return r >= '0' && r <= '9' })
		if first <= 0 {
			return "", 0, false
		}
		n, err := strconv.Atoi(value[first:])
		if err != nil || n <= 0 || value[:first]+strconv.Itoa(n) != value {
			return "", 0, false
		}
		return value[:first], n, true
	}
	a, b := strings.Split(left, "."), strings.Split(right, ".")
	for i := 0; i < min(len(a), len(b)); i++ {
		ap, an, aok := segment(a[i])
		bp, bn, bok := segment(b[i])
		if !aok || !bok {
			return left < right
		}
		if ap != bp {
			return ap < bp
		}
		if an != bn {
			return an < bn
		}
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return left < right
}

func TestCompactRefOrderingPreservesOriginalPairDecisions(t *testing.T) {
	refs := []string{"", ".", "t", "1", "t0", "t01", "t+1", "t-1", "t1.", "t1..g2", "t2.bad", "t10.bad", "t1.g0", "t1.g01", "t1.g1.", "α2.n10", "α2.n2", "t9999999999999999999999999", "t1.g9999999999999999999999999"}
	for _, target := range []int{1, 2, 9, 10, 99, 100, 450} {
		for _, member := range []int{1, 2, 10, 100} {
			refs = append(refs, "t"+strconv.Itoa(target), "t"+strconv.Itoa(target)+".n"+strconv.Itoa(member), "t"+strconv.Itoa(target)+".g"+strconv.Itoa(member)+".n2")
		}
	}
	for _, a := range refs {
		for _, b := range refs {
			if got, want := compactRefLess(a, b), originalCompactRefLess(a, b); got != want {
				t.Fatalf("%q < %q = %v, original %v", a, b, got, want)
			}
		}
	}
}

func BenchmarkQualifiedCompactRefComparison(b *testing.B) {
	for _, item := range []struct {
		name string
		less func(string, string) bool
	}{{"original", originalCompactRefLess}, {"current", compactRefLess}} {
		b.Run(item.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if !item.less("t450.g20.n2", "t450.g100.n2") {
					b.Fatal("wrong natural order")
				}
			}
		})
	}
}

// Refs are numbered in an order the graph fixes, not in the order a caller
// listed indexes, groups, members or connections.
func TestRequestBytesDoNotDependOnGroupMemberOrConnectionOrder(t *testing.T) {
	fixture := newFixture(t)
	canonical := encodeOverview(t, fixture.input)
	if !bytes.Contains(canonical, []byte(`"Apply@10`)) {
		t.Fatal("fixture carries no member calls to order")
	}
	shuffled := fixture.input
	shuffled.Groups = append([]groupindex.Index(nil), fixture.input.Groups...)
	slices.Reverse(shuffled.Groups)
	for i := range shuffled.Groups {
		index := &shuffled.Groups[i]
		index.Groups = append([]groupindex.Group(nil), index.Groups...)
		slices.Reverse(index.Groups)
		for g := range index.Groups {
			members := append([]string(nil), index.Groups[g].MemberSubjectIDs...)
			slices.Reverse(members)
			index.Groups[g].MemberSubjectIDs = members
		}
		index.Connections = append([]groupindex.Connection(nil), index.Connections...)
		slices.Reverse(index.Connections)
	}
	if reordered := encodeOverview(t, shuffled); !bytes.Equal(canonical, reordered) {
		t.Fatalf("request bytes follow the caller's order:\n%s\n%s", canonical, reordered)
	}
}

// A presentation change must not invent a second request-local identity for
// an already built group.
func TestGroupLaneChangeKeepsEveryOtherRefInPlace(t *testing.T) {
	fixture := newFixture(t)
	before := decodeRequest(t, fixture.input)
	moved := fixture.input
	moved.Groups = append([]groupindex.Index(nil), fixture.input.Groups...)
	// Both fixture targets hold a group of this title; this test changes the
	// presentation field on the already identified group.
	const title = "Execution triggers"
	changedGroups := 0
	for i := range moved.Groups {
		index := &moved.Groups[i]
		index.Groups = append([]groupindex.Group(nil), index.Groups...)
		for g := range index.Groups {
			group := &index.Groups[g]
			if group.Title != title {
				continue
			}
			group.Lane = groupindex.LaneCore
			changedGroups++
		}
	}
	if changedGroups != len(moved.Groups) {
		t.Fatalf("relaned %d groups across %d indexes", changedGroups, len(moved.Groups))
	}
	after := decodeRequest(t, moved)
	if !reflect.DeepEqual(groupRefsByTitle(before), groupRefsByTitle(after)) {
		t.Fatalf("group refs moved with one lane:\n%v\n%v", groupRefsByTitle(before), groupRefsByTitle(after))
	}
	changed := 0
	for i := range before.Groups {
		if before.Groups[i].Lane == after.Groups[i].Lane {
			continue
		}
		changed++
		if before.Groups[i].Title != title || after.Groups[i].Lane != string(groupindex.LaneCore) {
			t.Fatalf("another group changed lane: %+v -> %+v", before.Groups[i], after.Groups[i])
		}
	}
	if changed != changedGroups {
		t.Fatalf("lane changes = %d, want %d", changed, changedGroups)
	}
}

func decodeRequest(t *testing.T, input Input) overviewRequest {
	t.Helper()
	var decoded overviewRequest
	if err := json.Unmarshal(encodeOverview(t, input), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func groupRefsByTitle(wire overviewRequest) map[string]string {
	refs := make(map[string]string, len(wire.Groups))
	for _, group := range wire.Groups {
		refs[group.Target+" "+group.Title] = group.Ref
	}
	return refs
}
