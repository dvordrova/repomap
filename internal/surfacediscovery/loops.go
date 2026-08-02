package surfacediscovery

import (
	"go/token"
	"sort"

	"github.com/dvordrova/repomap/internal/semantics/catalog"
	"golang.org/x/tools/go/ssa"
)

type loopDescriptor struct {
	blocks map[*ssa.BasicBlock]bool
	signal LoopSignal
}

func (a *analyzer) loops(function *ssa.Function) []loopDescriptor {
	if cached, ok := a.loopCache[function]; ok {
		return cached
	}
	result := []loopDescriptor{}
	seen := map[string]struct{}{}
	if function == nil || function.Blocks == nil {
		a.loopCache[function] = result
		return result
	}
	for _, tail := range function.Blocks {
		for _, header := range tail.Succs {
			if !header.Dominates(tail) {
				continue
			}
			blocks := naturalLoop(header, tail)
			kind := "control_flow_loop"
			detail := "bounded SSA control-flow cycle"
			hasReceive := false
			hasSelect := false
			for block := range blocks {
				for _, instruction := range block.Instrs {
					switch current := instruction.(type) {
					case *ssa.Select:
						hasSelect = true
					case *ssa.UnOp:
						if current.Op == token.ARROW {
							hasReceive = true
						}
					}
				}
			}
			switch {
			case hasSelect:
				kind = "select_event_loop"
				detail = "control-flow loop contains a select; cancellation and runtime branch choice remain unproven"
			case hasReceive:
				kind = "channel_receive_loop"
				detail = "control-flow loop contains a channel receive; channel ownership and runtime lifetime remain unproven"
			}
			descriptor := loopDescriptor{
				blocks: blocks,
				signal: LoopSignal{
					Kind:       kind,
					FunctionID: a.functionID(function),
					Location:   a.blockLocation(header, function),
					Detail:     detail,
					Certainty:  "static",
				},
			}
			key := descriptor.signal.Kind + "\x00" + descriptor.signal.FunctionID + "\x00" + locationKey(descriptor.signal.Location)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, descriptor)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return locationKey(result[i].signal.Location) < locationKey(result[j].signal.Location)
	})
	a.loopCache[function] = result
	return result
}

func naturalLoop(header, tail *ssa.BasicBlock) map[*ssa.BasicBlock]bool {
	result := map[*ssa.BasicBlock]bool{header: true, tail: true}
	work := []*ssa.BasicBlock{tail}
	for len(work) > 0 {
		last := len(work) - 1
		block := work[last]
		work = work[:last]
		for _, predecessor := range block.Preds {
			if result[predecessor] {
				continue
			}
			result[predecessor] = true
			if predecessor != header {
				work = append(work, predecessor)
			}
		}
	}
	return result
}

func (a *analyzer) registrationLoop(
	call ssa.CallInstruction,
	seed catalog.Seed,
) (LoopSignal, bool) {
	for _, loop := range a.loops(call.Parent()) {
		if !loop.blocks[call.Block()] {
			continue
		}
		return LoopSignal{
			Kind:         "registration_loop",
			FunctionID:   a.functionID(call.Parent()),
			Location:     a.location(call.Pos()),
			TerminalSeed: seed.ID,
			Detail:       "configured registration sink occurs inside a control-flow loop; registration cardinality may be dynamic",
			Certainty:    "static",
		}, true
	}
	return LoopSignal{}, false
}

func (a *analyzer) addLoopSignal(signal LoopSignal) {
	key := signal.Kind + "\x00" + signal.FunctionID + "\x00" + locationKey(signal.Location) + "\x00" + signal.TerminalSeed
	if a.loopSeen[key] {
		return
	}
	a.loopSeen[key] = true
	a.result.Coverage.LoopSignals = append(a.result.Coverage.LoopSignals, signal)
}

func (a *analyzer) blockLocation(block *ssa.BasicBlock, function *ssa.Function) Location {
	for _, instruction := range block.Instrs {
		if instruction.Pos().IsValid() {
			return a.location(instruction.Pos())
		}
	}
	return a.location(function.Pos())
}
