package groupindex

// Phases of a program. Runtime is what any input's handler reaches (its
// Reach); initialization is what the target's seeds reach over the same
// execution edges and no input's handler does: the launch and the main
// loop around the work. A subject both reach is both. A subject neither
// reaches has no phase, and is never quiet: until what stores it becomes an
// input, a stored callback is simply not established. A program with no
// input handled by a declaration has no phases at all: it does all its work
// from its launch. A connection has the phase of its source subject. A
// connection into a helper is derived beside it from the saved helper
// interpretations, so a hydrated index says what the projected one does.
const (
	PhaseInit    = "init"
	PhaseRuntime = "runtime"
	PhaseBoth    = "both"
)

// phases derives the phases, the helper mark and the quiet flag from the
// reaches.
func (graph *reachGraph) phases() {
	index := graph.index
	runtime := make([]bool, len(index.Subjects))
	serves := false
	for _, reach := range index.Reach {
		for _, subject := range reach.Subjects {
			runtime[graph.position[subject.SubjectID]] = true
			serves = true
		}
	}
	init := make([]bool, len(index.Subjects))
	if serves {
		var queue []int
		for _, seed := range index.Target.Seeds {
			if position, known := graph.position[seed.ObjectID]; known && !init[position] {
				init[position] = true
				queue = append(queue, position)
			}
		}
		for next := 0; next < len(queue); next++ {
			for _, edge := range graph.exec[queue[next]] {
				if to := graph.to[edge]; !init[to] {
					init[to] = true
					queue = append(queue, to)
				}
			}
		}
	}
	phase := func(id string) string {
		position, known := graph.position[id]
		switch {
		case !known || id == "":
			return ""
		case init[position] && runtime[position]:
			return PhaseBoth
		case init[position]:
			return PhaseInit
		case runtime[position]:
			return PhaseRuntime
		}
		return ""
	}
	for position := range index.Subjects {
		index.Subjects[position].Phase = phase(index.Subjects[position].ID)
	}
	helpers := make(map[string]bool)
	for _, subject := range index.Subjects {
		if subject.Interpretation != nil && subject.Interpretation.Helper {
			helpers[subject.ID] = true
		}
	}
	// A connection is quiet, drawn only while one of its ends is looked at,
	// when it is wiring or a call into a helper in a program that serves
	// something: a program with no handled input does all its work from its
	// launch, and Redis's client, benchmark and dump checker had drawn none
	// of their arrows. A call into a helper is quiet even on an input's
	// path: every command handler calls its reply helpers. One exception,
	// over the program's own connections: when every one of them would be
	// quiet, its calls into helpers are drawn, so quieting them never empties
	// its map.
	everyQuiet := true
	for position := range index.Connections {
		connection := &index.Connections[position]
		connection.Phase = phase(connection.FromSubjectID)
		connection.ToHelper = connection.To.TargetID == index.Target.ID && helpers[connection.ToSubjectID]
		connection.Quiet = serves && (connection.Phase == PhaseInit || connection.ToHelper)
		everyQuiet = everyQuiet && connection.Quiet
	}
	if everyQuiet {
		for position := range index.Connections {
			if connection := &index.Connections[position]; connection.Phase != PhaseInit {
				connection.Quiet = false
			}
		}
	}
}
