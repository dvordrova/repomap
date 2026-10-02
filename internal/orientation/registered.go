package orientation

import (
	"slices"

	"github.com/dvordrova/repomap/internal/facts"
)

// FlowHop is one declaration on a run of calls a Main flow step names beside
// its own: how the hop before reaches it, as the walk does. Possible marks a
// hop reached through a value (one of a dispatch's alternatives, or a
// callable a call through a function value may run), Handed one handed over
// (a registration's or a passed callable's); neither, an exact call. The
// report says them in words, never as a count.
type FlowHop struct {
	SubjectID string `json:"subject_id"`
	Possible  bool   `json:"possible,omitempty"`
	Handed    bool   `json:"handed,omitempty"`
}

// FlowRegistration is one place a step's callable is registered: the
// registration fact and the run of hops reaching the function that
// registers it, from the latest earlier step of the path that reaches it,
// that function last.
type FlowRegistration struct {
	FactID string    `json:"fact_id"`
	Chain  []FlowHop `json:"chain"`
}

// FlowRunner is a function that runs a step's callable through a value,
// with the run of hops reaching it from the latest earlier step of the path
// that reaches it, the runner last.
type FlowRunner struct {
	Chain []FlowHop `json:"chain"`
}

// readRegistrations says, on every step of a walked flow whose callable a
// registration hands over, where it is registered and what runs it,
// walking the same edges the flow walks (flowGraph: exact calls, a
// dispatch's alternatives, a function value's possible callees, hand-overs)
// from the latest earlier step of the path that reaches a registering
// function, the fewest hops first, every run of that length kept (version
// 3). redis's readQueryFromClient had read "processTimeEvents → serverCron
// → syncWithMaster → createClient registers it", the replication path the
// report found by exact calls alone; aeProcessEvents may call acceptHandler,
// which calls createClient. A registration no earlier step reaches is its
// registering function alone. A runner is a function calling the callable
// through a value other than the site the step is reached from, which the
// step already names.
func readRegistrations(steps []FlowStep, before []string, graph *flowGraph, registrations []facts.Fact) {
	sites := map[string][]facts.Fact{}
	for _, fact := range registrations {
		if fact.Kind == facts.KindRegistration && fact.TargetID == graph.index.Target.ID && fact.OwnerID != "" && fact.ObjectID != "" && fact.ID != "" {
			sites[fact.ObjectID] = append(sites[fact.ObjectID], fact)
		}
	}
	readStepRegistrations(steps, before, graph, sites)
}

func readStepRegistrations(steps []FlowStep, before []string, graph *flowGraph, sites map[string][]facts.Fact) {
	shown := slices.Clone(before)
	for position := range steps {
		step := &steps[position]
		if registered := sites[step.SubjectID]; len(registered) > 0 {
			step.Registered = graph.registrationsOf(registered, shown)
			step.RunBy = graph.runnersOf(step.SubjectID, step.Site, shown)
		}
		shown = append(shown, step.SubjectID)
		for way := range step.Paths {
			readStepRegistrations(step.Paths[way].Steps, shown, graph, sites)
		}
	}
}

// registrationsOf are a callable's registrations as the path reaches them:
// from its latest step reaching any registering function, every shortest
// run of hops to each one it reaches at that length; none reached, each
// registering function alone.
func (graph *flowGraph) registrationsOf(sites []facts.Fact, shown []string) []FlowRegistration {
	owners := map[string]bool{}
	for _, site := range sites {
		owners[site.OwnerID] = true
	}
	for at := len(shown) - 1; at >= 0; at-- {
		chains := graph.shortestChains(shown[at], owners)
		if len(chains) == 0 {
			continue
		}
		var result []FlowRegistration
		for _, chain := range chains {
			owner := chain[len(chain)-1].SubjectID
			for _, site := range sites {
				if site.OwnerID == owner {
					result = append(result, FlowRegistration{FactID: site.ID, Chain: chain})
				}
			}
		}
		return result
	}
	var result []FlowRegistration
	for _, site := range sites {
		result = append(result, FlowRegistration{FactID: site.ID, Chain: []FlowHop{{SubjectID: site.OwnerID}}})
	}
	return result
}

// runnersOf are the functions calling a callable through a value, the
// step's own site aside, each with its shortest run from the path.
func (graph *flowGraph) runnersOf(callable, site string, shown []string) []FlowRunner {
	var runners []string
	for from, edges := range graph.out {
		for _, edge := range edges {
			if edge.to == callable && edge.site != "" && from != site && !slices.Contains(runners, from) {
				runners = append(runners, from)
			}
		}
	}
	slices.Sort(runners)
	var result []FlowRunner
	for _, runner := range runners {
		chain := []FlowHop{{SubjectID: runner}}
		for at := len(shown) - 1; at >= 0; at-- {
			if chains := graph.shortestChains(shown[at], map[string]bool{runner: true}); len(chains) > 0 {
				chain = chains[0]
				break
			}
		}
		result = append(result, FlowRunner{Chain: chain})
	}
	return result
}

// shortestChains are every run of hops of the fewest from start to any of
// targets over the flow's edges, start first; start itself when it is one.
func (graph *flowGraph) shortestChains(start string, targets map[string]bool) [][]FlowHop {
	if targets[start] {
		return [][]FlowHop{{{SubjectID: start}}}
	}
	type parent struct {
		from string
		hop  FlowHop
	}
	parents := map[string][]parent{}
	depth := map[string]int{start: 0}
	frontier := []string{start}
	var reached []string
	for len(frontier) > 0 && len(reached) == 0 {
		var next []string
		for _, from := range frontier {
			for _, edge := range graph.out[from] {
				hop := FlowHop{SubjectID: edge.to, Possible: edge.site != "", Handed: edge.site == "" && edge.via != "called"}
				known, seen := depth[edge.to]
				if !seen {
					depth[edge.to] = depth[from] + 1
					next = append(next, edge.to)
					if targets[edge.to] {
						reached = append(reached, edge.to)
					}
				} else if known != depth[from]+1 {
					continue
				}
				if !slices.ContainsFunc(parents[edge.to], func(p parent) bool { return p.from == from }) {
					parents[edge.to] = append(parents[edge.to], parent{from: from, hop: hop})
				}
			}
		}
		frontier = next
	}
	var chains [][]FlowHop
	var back func(at string, tail []FlowHop)
	back = func(at string, tail []FlowHop) {
		if at == start {
			chains = append(chains, append([]FlowHop{{SubjectID: start}}, tail...))
			return
		}
		for _, p := range parents[at] {
			back(p.from, append([]FlowHop{p.hop}, tail...))
		}
	}
	for _, target := range reached {
		back(target, nil)
	}
	return chains
}
