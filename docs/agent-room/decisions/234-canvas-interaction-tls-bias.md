# 234 — Canvas pointer-local interaction contract and TLS-family dominance bias

**Status:** ACTIVE (owner-authorized decision D of the Archive 9 semantic-product program)
**Supersedes:** the D230 D3 wheel clause (ordinary wheel scrolls the page;
Ctrl/Cmd+wheel zooms) ONLY to the extent the owner corrective specifies —
the canvas may own wheel/trackpad input while the pointer is over the map;
the page remains scrollable from the side gutter / outside the canvas.

## Problem (owner corrective 1, canvas)

The desktop map must be pointer-local:

- pointer over canvas: wheel/trackpad may zoom or pan according to ONE
  documented stable behavior;
- blank-space drag pans;
- explicit +, −, Fit/Overview and reset controls work;
- pointer outside canvas: ordinary wheel scrolls the report page;
- the viewport contains a persistent but quiet interaction hint that names
  the exact behavior: «Колесо — масштаб · перетаскивание — перемещение»
  (EN: "Wheel — zoom · drag — pan");
- Fit/Overview must actually expose every principal node OR switch to an
  honest semantic-overview scale;
- canvas state remains recoverable and deterministic;
- no toolbar or overlay occludes a principal node;
- inspector/source actions preserve the canvas transform;
- no acceptance blocker may be raised merely because the canvas owns wheel
  input over the map.

Fix (not page-scroll) is the target: hidden/unrecoverable behavior, broken
Fit, node occlusion, state loss.

## Problem (owner corrective 3, TLS/security-family dominance)

TLS/security-family dominance is a bias, not a product truth. Do NOT remove
TLS/certificate facts. Four-stage bias:

1. broad `tls_or_security_boundary` anchor classification;
2. declaration-family anchors as component seeds;
3. package participation as responsibility/association scope;
4. Overview/Study ranking repeats the strongest/lightest anchor family.

Grounding: when exact evidence allows, separate certificate/PKI lifecycle,
transport TLS configuration, and authentication/authorization policy;
materially distinct readings/learning outcomes get separate themes, otherwise
group under one primary theme with alternate readings. Publish an explicit
portfolio-concentration diagnostic.

## Decision

### Canvas interaction contract (architecture_canvas.js)

Wheel/trackpad over the canvas zooms (pointer-local, single documented
behavior — the canvas owns the wheel input while the pointer is over the
map, per the owner corrective). Outside the canvas the report page scrolls
normally. This replaces the D230 D3 Ctrl/Cmd+wheel-only zoom clause; Ctrl/
Cmd+wheel keeps zooming too (harmless superset, same stable behavior).

- `installViewportInteractions`: wheel over viewport → zoom (no modifier
  required); blank-space drag pans; pointer outside → untouched.
- Persistent quiet hint in the viewport: «Колесо — масштаб ·
  перетаскивание — перемещение» / "Wheel — zoom · drag — pan", non
  intrusive, aria-label present, never occludes a node.
- Fit/Overview: `fit()` exposes the landscape bounds (all principal nodes);
  a semantic-overview fallback scale is applied when bounds are degenerate.
- Inspector/source actions preserve view.x/view.y/view.scale (no reset on
  open/close).
- No toolbar/overlay covers a principal node (controls are compact,
  positioned outside the graph area).
- Canvas state is deterministic: same report + same interactions ⇒ same
  view (no randomness, no time-dependence).

### TLS/security-family dominance (bias, four stages — no fact removal)

- Stage 1 already exists: `tls_or_security_boundary` anchor classification
  in surfacediscovery.
- Stage 2/3: declaration-family anchors seed components; package
  participation is participation/support, never exclusive ownership (D231
  shared participation already encodes this).
- Stage 4: Overview/Study ranking must NOT make TLS/certificates the
  default principal area of every repository. A single broad anchor or a
  declaration-family membership does not move every package observation into
  a TLS component; principal component/theme count requires distinct scope/
  responsibility (D4-equivalence).
- Grounding separation: when exact evidence allows, certificate/PKI
  lifecycle, transport TLS configuration and authn/authz policy are
  DISTINCT themes; otherwise one primary theme with alternate readings
  (D233 co-projection).
- Portfolio-concentration diagnostic: published in the Study status (D233
  generic rule — no TLS string in the rule); a synthetic non-TLS dominant
  family proves genericity.

## Fresh review verdicts (all applied)

Product: PASS with 2 bounded defects:
- Acceptance scoping: wheel-zoom and Fit-expose-every-principal-node are
  scoped to the LANDSCAPE state (no focused flow); the flow state keeps its
  distinct scroll behavior. Scoped in acceptance + asset test.
- Hint a11y: viewportHint gains role="note" + aria-label naming the exact
  behavior; pinned in the asset test.

Red-team: PASS with 1 required finding + 2 bounded:
- F1 (required): readableFitScale clamps only the UPPER bound (FIT_MAX_SCALE
  = 1.35) — the 0.16 floor was removed so a huge landscape fits ENTIRELY
  inside the viewport (all principal node centers hit-testable); the
  semantic-overview scale keeps tiny renders honest. Huge-landscape case
  added to TestArchitectureCanvasLayoutModes; FIT_MIN_SCALE → FIT_MAX_SCALE
  in the asset pin.
- F2: casdoor not-TLS-dominated is now a quantitative assertion (generic
  concentration diagnostic does not fire on the default Study shelf; every
  tls_or_security_boundary anchor counted/reachable) — verified against the
  Archive 9 casdoor run.
- F3: the DOM harness is named (minimal elementFromPoint/addEventListener
  model in the node-runner shim); "page scroller NOT touched" is proven by
  asserting no wheel listener was installed on the page/document object.

## Version/cache/replay identities

- Canvas: client-side only; no wire/artifact/version change (D230 D3 was
  also client-side). Golden report HTML regenerates.
- TLS bias: backend/ranking client-side only; no wire change; no prompt
  change (prompts already model-free of this bias).
- No cache/replay identity change.

## Acceptance (provider-free)

- Wheel over the canvas zooms (no modifier) — proven in the node-runner
  test by invoking the wheel handler in the LANDSCAPE state (no focused
  flow) and asserting scale changed and no wheel listener was ever
  installed on the page/document object (the handler binds the viewport
  only, so the page scroller is never touched).
- Pointer outside canvas: wheel scrolls the page (handler not installed on
  the page root).
- Hint text renders in EN and RU («Колесо — масштаб · перетаскивание —
  перемещение» / "Wheel — zoom · drag — pan") in the landscape state, with
  role="note" + aria-label present (a11y contract).
- Fit/Overview exposes every principal node in the landscape state: after
  fit, all component centers are inside the viewport (hit-testable). The
  fit scale clamps only the upper bound — a huge landscape fits entirely
  inside the viewport (F1), with the semantic-overview scale keeping tiny
  renders honest.
- Inspector/source open/close preserves view.x/view.y/view.scale.
- No principal node center is covered by a toolbar/overlay (hit-test at the
  center returns the node, not the overlay); the hint is pointer-events:
  none.
- Casdoor default Overview/Study/Architecture is NOT TLS-dominated while
  every certificate/TLS fact remains present and reachable. Quantitative
  assertion: the generic concentration diagnostic does NOT fire on
  casdoor's default Study shelf (no single anchor family > half of the
  published cards), and every `tls_or_security_boundary` anchor remains
  counted/reachable (verified against the Archive 9 casdoor run in the
  D231 run dir).
- One synthetic non-TLS dominant family (logging) triggers the same
  concentration diagnostic as a TLS shelf — the rule is generic.
- TLS/security anchors remain counted and reachable (nothing deleted).
- DOM harness: the node-runner vm shim gains a minimal
  elementFromPoint/addEventListener model; "page scroller NOT touched" is
  proven by asserting no wheel listener was installed on the page/document
  object.

## Docs

- RUN_LOG.md (D231 run dir) records the corrective application.
- PROMPT_VALIDATOR_MATRIX.md — no prompt change in this decision.
