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
  only until the ELK integration lands, then it is deleted. The spike and npm
  artifacts have now been removed.

## Production vertical slice

- [x] Replace the stale cached FlowProof v1 input with a small checked-in v2
  restic presentation fixture.
- [x] Add a view-only preview entrypoint and Make target that require no
  manifest, repository scan, provider, gopls, or Pyright.
- [x] Support every bounded saved task rather than one task branch. The daemon
  fixture renders eleven task branches plus shared state.
- [x] Keep fixed bounded node dimensions for ELK across the final fixtures;
  ordinary selection never relayouts the graph.
- [x] Keep report typography during checkpoints A–C and apply the product pass
  only to canvas hierarchy and evidence channels.
- [x] Make component, flow, step, and edge selection keyboard-accessible.
  Arrow-key spatial traversal remains an explicit MVP limitation.
- [x] Cover saved synthesis, invalid output, and deterministic fallback.
  Realistic fallback screenshot coverage is explicitly deferred.
- [x] Validate restic, a daemon, and a branching/backend fixture after evidence
  drill-down was complete.

## Required cleanup before completion

- [x] Delete the rejected React Flow spike and all ignored npm artifacts.
- [x] Remove `layoutRoleLanes`, inline `imports` labels, obsolete arrow marker,
  and the old canvas-only selection closure.
- [x] Demote the old separate flow presentation to the detail ledger; primary
  flow selection stays on the architecture canvas.
- [x] Remove temporary debug controls, fixed selections, and comparison-only
  CSS.
- [x] Record final screenshots, fixture metrics, verification commands, known
  limitations, and explicitly deferred items.
