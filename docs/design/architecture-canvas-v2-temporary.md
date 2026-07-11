# Architecture canvas v2 temporary-work checklist

This file tracks shortcuts used to shorten the visual iteration loop. A checked
item is removed or generalized; an open item must be resolved or explicitly
deferred before the final canvas-v2 handoff.

## Technology spike

- [x] **Hard-coded restic-like graph.** Preview-only. It compared layout and
  interaction libraries without repository analysis. It will not enter the
  production analysis path.
- [x] **Fixed 1280×720 viewport.** Preview-only. It was sufficient for the
  library decision; production is still tested at one narrower desktop width.
- [x] **Post-layout group boxes in the first comparison.** Preview-only. A
  second ELK spike verified real compound groups and explicit ports.
- [x] **Simplified node content.** Preview-only. It proved progressive
  disclosure; production node content comes from the saved canvas contract.
- [x] **Fixed Backup flow.** Preview-only. Production flow selection uses saved
  stable IDs and shows one selected flow at a time.
- [x] **Temporary React Flow comparison.** Rejected. The ignored spike remains
  only until the ELK integration lands, then it is deleted.

## Production vertical slice

- [ ] Replace the stale cached FlowProof v1 input with a small checked-in v2
  restic presentation fixture.
- [ ] Add a view-only preview entrypoint and Make target that require no
  manifest, repository scan, provider, gopls, or Pyright.
- [ ] Initially support one task branch per flow. This is production-visible
  and must render additional tasks as explicit unprojected frontiers until the
  bounded projection supports them.
- [ ] Use fixed initial node dimensions for ELK. Generalize only to the bounded
  component and selected-flow node variants used by the final fixtures.
- [ ] Keep existing report typography and colors during checkpoints A–C. Do a
  product pass only after landscape → flow → evidence works.
- [ ] Keyboard navigation may initially cover component/flow/edge selection
  without arrow-key graph traversal. Document any remaining limitation at the
  final checkpoint.
- [ ] The first model-synthesis fixture may cover restic only. Add invalid
  output and deterministic fallback before checkpoint D completes.
- [ ] Validate restic continuously; add the daemon and branching/backend
  fixtures only after evidence drill-down is complete.

## Required cleanup before completion

- [ ] Delete the rejected React Flow spike and all ignored npm artifacts.
- [ ] Remove `layoutRoleLanes`, inline `imports` labels, obsolete arrow marker,
  and the old canvas-only selection closure.
- [ ] Demote or remove the old separate flow presentation after the same proof
  remains available from the canvas and detail ledger.
- [ ] Remove temporary debug controls, fixed selections, and comparison-only
  CSS.
- [ ] Record final screenshots, fixture metrics, verification commands, known
  limitations, and explicitly deferred items.
