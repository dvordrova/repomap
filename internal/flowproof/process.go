package flowproof

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

const SurfaceCollectorVersion = "surface-catalog-v4"

// StaticSurfaceFact is the provider-neutral subset of one accepted surface
// record that may contribute exact static evidence to a partial proof.
type StaticSurfaceFact struct {
	ID            string
	Kind          string
	Label         string
	QualifiedName string
	Location      evidence.Location
	Handler       string
	Direct        bool
	Wrappers      []StaticWrapperFact
}

type StaticWrapperFact struct {
	ID            string
	Label         string
	QualifiedName string
	Location      evidence.Location
	Callsite      evidence.Location
}

type ProcessSeed struct {
	FlowID           string
	Goal             string
	SeedSurfaceID    string
	ScenarioID       string
	CollectorVersion string
	Entrypoint       StaticSurfaceFact
	Supporting       *StaticSurfaceFact
	CurrentFrontier  string
}

// BuildProcess creates a process-level partial proof. A same-executable
// supporting surface may add static anchors and wrapper relations, but it never
// turns executable co-membership into runtime order.
func BuildProcess(seed ProcessSeed) Proof {
	proof := Proof{
		Version:         Version,
		ID:              seed.FlowID,
		Archetype:       ArchetypeProcess,
		Goal:            seed.Goal,
		Slots:           newCLISlots(),
		SeedSurfaceID:   seed.SeedSurfaceID,
		TraceQuality:    TraceQualityPartial,
		CurrentFrontier: strings.TrimSpace(seed.CurrentFrontier),
		Warnings:        []string{"static surface evidence does not establish runtime execution or order"},
	}
	entry := staticSurfaceAnchor(seed.Entrypoint, AnchorFunction)
	if entry.ID != "" && entry.Location != nil {
		proof.Anchors = append(proof.Anchors, entry)
		setSlot(&proof, SlotTrigger, SlotPartial, "process invocation targets an exact entry declaration", []string{entry.ID}, "runtime process invocation was not observed")
		setSlot(&proof, SlotEntrypoint, SlotVerified, "exact process entrypoint", []string{entry.ID}, "")
	}

	if seed.Supporting != nil && addProcessSupportingSurface(&proof, entry.ID, *seed.Supporting) {
		proof.TraceEvidenceSurfaceIDs = []string{seed.Supporting.ID}
	}
	setMissingProcessSlots(&proof)
	if proof.CurrentFrontier == "" {
		proof.CurrentFrontier = "downstream runtime handoff from the exact process entry remains unresolved"
	}
	refreshTraceState(&proof)
	return proof
}

type DescriptorSeed struct {
	FlowID           string
	Goal             string
	SeedSurfaceID    string
	Descriptor       StaticSurfaceFact
	ConsumerFrontier string
}

// BuildDescriptor creates a partial proof rooted in exact descriptor evidence.
// The descriptor is not promoted to a request entry; consumer registration is
// retained as the current frontier.
func BuildDescriptor(seed DescriptorSeed) Proof {
	frontier := strings.TrimSpace(seed.ConsumerFrontier)
	if frontier == "" {
		frontier = "runtime descriptor consumer registration remains unresolved"
	}
	proof := Proof{
		Version:         Version,
		ID:              seed.FlowID,
		Archetype:       ArchetypeProcess,
		Goal:            seed.Goal,
		Slots:           newCLISlots(),
		SeedSurfaceID:   seed.SeedSurfaceID,
		TraceQuality:    TraceQualityPartial,
		CurrentFrontier: frontier,
		Warnings:        []string{"descriptor evidence does not prove consumer registration or request execution"},
	}
	descriptor := staticSurfaceAnchor(seed.Descriptor, AnchorOperation)
	if descriptor.ID != "" && descriptor.Location != nil {
		proof.Anchors = append(proof.Anchors, descriptor)
		setSlot(&proof, SlotTrigger, SlotPartial, "exact static descriptor", []string{descriptor.ID}, frontier)
	}
	setMissingProcessSlots(&proof)
	refreshTraceState(&proof)
	return proof
}

func addProcessSupportingSurface(proof *Proof, entryID string, supporting StaticSurfaceFact) bool {
	surface := staticSurfaceAnchor(supporting, AnchorCallsite)
	if surface.ID == "" || surface.Location == nil {
		return false
	}
	transitionIDs := make([]string, 0, len(supporting.Wrappers)+1)
	previousID := entryID
	for index, wrapper := range supporting.Wrappers {
		fact := StaticSurfaceFact{
			ID: wrapper.ID, Label: wrapper.Label, QualifiedName: wrapper.QualifiedName,
			Location: wrapper.Location,
		}
		anchor := staticSurfaceAnchor(fact, AnchorFunction)
		if anchor.ID == "" || anchor.Location == nil || previousID == "" || wrapper.Callsite.Path == "" || wrapper.Callsite.Line <= 0 {
			continue
		}
		proof.Anchors = append(proof.Anchors, anchor)
		transition := Transition{
			ID:   transitionID("surface-wrapper", index, wrapper.Callsite.Path, wrapper.Callsite.Line, wrapper.ID),
			From: previousID, To: anchor.ID, Relation: evidence.RelationCalls,
			Resolution: evidence.ResolutionStatic, Invocation: evidence.InvocationSynchronous,
			Certainty: evidence.CertaintyStatic, Evidence: wrapper.Callsite, Provider: "surface_catalog",
		}
		proof.Transitions = append(proof.Transitions, transition)
		transitionIDs = append(transitionIDs, transition.ID)
		previousID = anchor.ID
	}
	proof.Anchors = append(proof.Anchors, surface)
	if previousID != "" && (previousID != entryID || supporting.Direct) {
		transition := Transition{
			ID:   transitionID("surface-terminal", len(transitionIDs), surface.Location.Path, surface.Location.Line, supporting.ID),
			From: previousID, To: surface.ID, Relation: evidence.RelationCalls,
			Resolution: evidence.ResolutionStatic, Invocation: evidence.InvocationSynchronous,
			Certainty: evidence.CertaintyStatic, Evidence: *surface.Location, Provider: "surface_catalog",
		}
		proof.Transitions = append(proof.Transitions, transition)
		transitionIDs = append(transitionIDs, transition.ID)
	}
	evidenceIDs := append(transitionIDs, surface.ID)
	missing := fmt.Sprintf("runtime ordering and handoff from process entry to %s", strings.ReplaceAll(supporting.Kind, "_", " "))
	setSlot(proof, SlotDispatch, SlotPartial, "same-executable static "+strings.ReplaceAll(supporting.Kind, "_", " ")+" evidence", evidenceIDs, missing)
	if strings.TrimSpace(supporting.Handler) != "" {
		setSlot(proof, SlotApplicationCallable, SlotPartial, "exact registered handler or callback identity", []string{surface.ID}, "runtime callback invocation")
	}
	return true
}

func setMissingProcessSlots(proof *Proof) {
	missing := map[SlotKind]string{
		SlotTrigger:             "runtime trigger observation",
		SlotEntrypoint:          "exact process entrypoint",
		SlotDispatch:            "downstream dispatch from process entry",
		SlotApplicationCallable: "application handler or callback reached from the process entry",
		SlotCoreOperation:       "first domain-level operation",
		SlotIOBoundary:          "repository, network, or storage boundary",
		SlotConcurrency:         "concurrent lifecycle tied to this trace",
		SlotTermination:         "return, shutdown, or completion path",
	}
	for _, kind := range cliSlotOrder {
		slot, _ := proof.Slot(kind)
		if slot.Status != SlotMissing || slot.Missing != "" {
			continue
		}
		setSlot(proof, kind, SlotMissing, "", nil, missing[kind])
	}
}

func staticSurfaceAnchor(fact StaticSurfaceFact, kind AnchorKind) Anchor {
	if strings.TrimSpace(fact.ID) == "" || strings.TrimSpace(fact.Location.Path) == "" || fact.Location.Line <= 0 {
		return Anchor{}
	}
	label := strings.TrimSpace(fact.Label)
	if label == "" {
		label = strings.TrimSpace(fact.Kind)
	}
	if label == "" {
		label = fact.ID
	}
	location := fact.Location
	return Anchor{
		ID: fact.ID, Kind: kind, Label: label, QualifiedName: fact.QualifiedName,
		Location: &location,
	}
}
