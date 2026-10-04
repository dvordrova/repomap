package reading

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// consults is what one reading of a field's writes looked at in the path
// it was read from: the caller each function it read a parameter of was
// bound to (chosenCallers), each alternative already chosen
// (chooseDestinationPart), and whether it met a value, field or object the
// walk was already reading (a cycle) or heard an undecided call: cut. A reading met by a later walk with the same
// bindings and choices gives the same paths after its own (writesMemo).
type consults struct {
	bindings map[string]string
	choices  map[string]int
	cut      bool
}

// writesReading is one reading of a field's writes: what it consulted,
// the targets it was read for, and what it added after the path it was
// read from.
type writesReading struct {
	consulted consults
	targets   string
	tails     []destinationPath
}

func bindingKey(callees []string, formal *sourcevalue.Anchor) string {
	key := strings.Join(callees, "\x01")
	if formal != nil {
		key += "\x02" + formal.Path + ":" + strconv.Itoa(formal.Line) + ":" + strconv.Itoa(formal.Column)
	}
	return key
}

func bindingValue(chosen map[sourcevalue.Anchor]bool) string {
	var anchors []string
	for anchor := range chosen {
		anchors = append(anchors, anchor.Path+":"+strconv.Itoa(anchor.Line)+":"+strconv.Itoa(anchor.Column))
	}
	slices.Sort(anchors)
	return strings.Join(anchors, " ")
}

func (d *DestinationReader) consultBinding(callees []string, formal *sourcevalue.Anchor, chosen map[sourcevalue.Anchor]bool) {
	if len(d.frames) == 0 {
		return
	}
	key, value := bindingKey(callees, formal), bindingValue(chosen)
	for _, frame := range d.frames {
		if _, seen := frame.bindings[key]; !seen {
			frame.bindings[key] = value
		}
	}
}

func (d *DestinationReader) consultChoice(key string, chosen int, ok bool) {
	if len(d.frames) == 0 {
		return
	}
	if !ok {
		chosen = -1
	}
	for _, frame := range d.frames {
		if _, seen := frame.choices[key]; !seen {
			frame.choices[key] = chosen
		}
	}
}

func (d *DestinationReader) consultCut() {
	for _, frame := range d.frames {
		frame.cut = true
	}
}

// pushConsults begins recording what a reading of a field's writes
// consults.
func (d *DestinationReader) pushConsults() *consults {
	frame := &consults{bindings: make(map[string]string), choices: make(map[string]int)}
	d.frames = append(d.frames, frame)
	return frame
}

// popConsults ends a reading and keeps it for reuse, unless it met a value
// the walk was already reading (a cycle cut: what it gave depends on the
// walk it was read in) or heard an undecided call (a later consumer must
// hear it too, which a reuse would not replay).
func (d *DestinationReader) popConsults(frame *consults, key string, use destinationPath, result []destinationPath) {
	d.frames = d.frames[:len(d.frames)-1]
	if frame.cut {
		return
	}
	reading := writesReading{consulted: *frame, targets: strings.Join(use.TargetIDs, "\x00")}
	for _, path := range result {
		tail := cloneDestinationPath(path)
		if len(tail.Steps) >= len(use.Steps) {
			tail.Steps = tail.Steps[len(use.Steps):]
		}
		for choice, value := range use.choices {
			if tail.choices[choice] == value {
				delete(tail.choices, choice)
			}
		}
		reading.tails = append(reading.tails, tail)
	}
	if d.writesMemo == nil {
		d.writesMemo = make(map[string][]writesReading)
	}
	d.writesMemo[key] = append(d.writesMemo[key], reading)
}

// reuseWrites gives what an earlier reading of the same field's writes
// gave, when the walk binds every function it consulted to the same
// callers, has made the same choices and reads for the same targets: the
// earlier reading's paths after this walk's own. Its consults are this
// walk's too.
func (d *DestinationReader) reuseWrites(key string, use destinationPath) ([]destinationPath, bool) {
	targets := strings.Join(use.TargetIDs, "\x00")
	for _, reading := range d.writesMemo[key] {
		if reading.targets != targets || !d.sameConsults(reading.consulted, use) {
			continue
		}
		for _, frame := range d.frames {
			for binding, value := range reading.consulted.bindings {
				if _, seen := frame.bindings[binding]; !seen {
					frame.bindings[binding] = value
				}
			}
			for choice, value := range reading.consulted.choices {
				if _, seen := frame.choices[choice]; !seen {
					frame.choices[choice] = value
				}
			}
		}
		result := make([]destinationPath, 0, len(reading.tails))
		for _, tail := range reading.tails {
			path := cloneDestinationPath(use)
			for _, step := range tail.Steps {
				path.Steps = appendDestinationStep(path.Steps, step)
			}
			maps.Copy(path.choices, tail.choices)
			path.TargetIDs = slices.Clone(tail.TargetIDs)
			path.Address, path.Frontier, path.Unread, path.Method = tail.Address, tail.Frontier, tail.Unread, tail.Method
			result = append(result, path)
		}
		return result, true
	}
	return nil, false
}

func (d *DestinationReader) sameConsults(consulted consults, use destinationPath) bool {
	for choice, value := range consulted.choices {
		chosen, ok := use.choices[choice]
		if !ok {
			chosen = -1
		}
		if chosen != value {
			return false
		}
	}
	for binding, value := range consulted.bindings {
		callees, formal := parseBindingKey(binding)
		if bindingValue(d.bindingOf(use, callees, formal)) != value {
			return false
		}
	}
	return true
}

func parseBindingKey(key string) ([]string, *sourcevalue.Anchor) {
	head, anchor, hasFormal := strings.Cut(key, "\x02")
	callees := strings.Split(head, "\x01")
	if !hasFormal {
		return callees, nil
	}
	parts := strings.Split(anchor, ":")
	if len(parts) < 3 {
		return callees, nil
	}
	column, line := parts[len(parts)-1], parts[len(parts)-2]
	path := strings.Join(parts[:len(parts)-2], ":")
	return callees, &sourcevalue.Anchor{Path: path, Line: number(line), Column: number(column)}
}

func number(text string) int {
	value, _ := strconv.Atoi(text)
	return value
}
