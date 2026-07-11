// Package flowproof models a bounded, replayable proof for one user-facing
// repository flow. It keeps deterministic evidence separate from model prose
// and from the presentation that renders it.
package flowproof

import "github.com/dvordrova/repomap/internal/evidence"

const Version = 1

type Archetype string

const ArchetypeCLI Archetype = "cli"

type SlotKind string

const (
	SlotTrigger             SlotKind = "trigger"
	SlotEntrypoint          SlotKind = "entrypoint"
	SlotDispatch            SlotKind = "dispatch"
	SlotApplicationCallable SlotKind = "application_callable"
	SlotCoreOperation       SlotKind = "core_operation"
	SlotIOBoundary          SlotKind = "io_boundary"
	SlotConcurrency         SlotKind = "concurrency"
	SlotTermination         SlotKind = "termination"
)

var cliSlotOrder = []SlotKind{
	SlotTrigger,
	SlotEntrypoint,
	SlotDispatch,
	SlotApplicationCallable,
	SlotCoreOperation,
	SlotIOBoundary,
	SlotConcurrency,
	SlotTermination,
}

type SlotStatus string

const (
	SlotMissing       SlotStatus = "missing"
	SlotPartial       SlotStatus = "partial"
	SlotVerified      SlotStatus = "verified"
	SlotUnresolved    SlotStatus = "unresolved"
	SlotNotApplicable SlotStatus = "not_applicable"
)

// ApplicabilityReason is a bounded, machine-readable explanation for why a
// slot does not belong to the selected flow scope. It is policy input, not an
// inference made from a collector returning no facts.
type ApplicabilityReason string

const (
	ApplicabilityNoConcurrentLifecycleInScope ApplicabilityReason = "no_concurrent_lifecycle_in_scope"
)

type Slot struct {
	Kind                SlotKind            `json:"kind"`
	Status              SlotStatus          `json:"status"`
	Summary             string              `json:"summary,omitempty"`
	EvidenceIDs         []string            `json:"evidence_ids,omitempty"`
	Missing             string              `json:"missing,omitempty"`
	ApplicabilityReason ApplicabilityReason `json:"applicability_reason,omitempty"`
}

type AnchorKind string

const (
	AnchorCommand   AnchorKind = "command"
	AnchorFunction  AnchorKind = "function"
	AnchorMethod    AnchorKind = "method"
	AnchorCallsite  AnchorKind = "callsite"
	AnchorOperation AnchorKind = "operation"
	AnchorTask      AnchorKind = "task"
)

type Anchor struct {
	ID            string             `json:"id"`
	Kind          AnchorKind         `json:"kind"`
	Label         string             `json:"label"`
	QualifiedName string             `json:"qualified_name,omitempty"`
	Location      *evidence.Location `json:"location,omitempty"`
}

type Transition struct {
	ID         string                  `json:"id"`
	From       string                  `json:"from"`
	To         string                  `json:"to"`
	Relation   evidence.RelationKind   `json:"relation"`
	Resolution evidence.ResolutionKind `json:"resolution"`
	Invocation evidence.InvocationMode `json:"invocation,omitempty"`
	Condition  *evidence.Condition     `json:"condition,omitempty"`
	Certainty  evidence.Certainty      `json:"certainty"`
	Evidence   evidence.Location       `json:"evidence"`
	Provider   string                  `json:"provider"`
}

type Proof struct {
	Version     int          `json:"version"`
	ID          string       `json:"id"`
	Archetype   Archetype    `json:"archetype"`
	Goal        string       `json:"goal"`
	Command     string       `json:"command,omitempty"`
	Slots       []Slot       `json:"slots"`
	Anchors     []Anchor     `json:"anchors"`
	Transitions []Transition `json:"transitions"`
	Warnings    []string     `json:"warnings,omitempty"`
}

func (p Proof) Slot(kind SlotKind) (Slot, bool) {
	for _, slot := range p.Slots {
		if slot.Kind == kind {
			return slot, true
		}
	}
	return Slot{}, false
}

func (p Proof) Anchor(id string) (Anchor, bool) {
	for _, anchor := range p.Anchors {
		if anchor.ID == id {
			return anchor, true
		}
	}
	return Anchor{}, false
}

func (p Proof) Transition(id string) (Transition, bool) {
	for _, transition := range p.Transitions {
		if transition.ID == id {
			return transition, true
		}
	}
	return Transition{}, false
}

// Satisfied reports whether every slot required by the proof archetype has an
// honest terminal outcome. Missing slots are unsatisfied; omission never means
// that a collector proved non-applicability.
func (p Proof) Satisfied() bool {
	for _, kind := range cliSlotOrder {
		slot, ok := p.Slot(kind)
		if !ok || !slotSatisfied(p.Archetype, slot) {
			return false
		}
	}
	return len(cliSlotOrder) > 0
}

func slotSatisfied(archetype Archetype, slot Slot) bool {
	switch slot.Status {
	case SlotVerified:
		return slot.Missing == ""
	case SlotNotApplicable:
		return slotMayBeNotApplicable(archetype, slot.Kind) && validApplicabilityReason(slot.ApplicabilityReason)
	default:
		return false
	}
}

func slotMayBeNotApplicable(archetype Archetype, kind SlotKind) bool {
	return archetype == ArchetypeCLI && kind == SlotConcurrency
}

func validApplicabilityReason(reason ApplicabilityReason) bool {
	return reason == ApplicabilityNoConcurrentLifecycleInScope
}
