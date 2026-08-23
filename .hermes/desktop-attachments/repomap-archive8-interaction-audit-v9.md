# Repomap Archive 8 — interactive desktop UX and canvas audit

**Evidence:** owner-supplied `Архив 8.zip`  
**Runs inspected:** Casdoor, etcd, Telebot, Chatto, Restic  
**Method:** real Chromium interaction against each self-contained `report.html`:
mouse hover, click, drag, wheel scroll, zoom, route changes, Back, inspector
scrolling, DOM hit-testing, source affordance inventory, screenshots and
computed-style checks.

This audit is intentionally stricter than a static HTML/JSON review. A control
passes only when an ordinary pointer user can discover it, click it, obtain the
expected result, and return without losing context.

---

## Executive verdict

Archive 8 improved several earlier problems:

- Architecture edges are now passive: no role/button/tabindex/hitbox and
  `pointer-events: none`.
- Casdoor publishes a much richer partial model Architecture.
- Study cards are compact.
- Component cards have a clear hover state.
- Study route changes open at the top and browser Back restores the previous
  Study scroll position.
- Canvas panning and explicit zoom controls work.

However, the desktop interaction layer is not accepted.

The largest problems are:

1. **Source-looking text is inconsistent and often inert.**
2. **The Architecture canvas has an occluded/inaccessible primary node and a
   misleading Fit behavior.**
3. **Mouse-wheel over the large canvas traps page scrolling and silently
   changes zoom.**
4. **The inspector leaks wheel scrolling into the page behind it.**
5. **Association rows contain nested buttons; opening a source also collapses
   the parent row and loses focus.**
6. **Hover and click signifiers are inconsistent.**
7. **Architecture is still one 9k–12.6k-pixel page because the mechanism
   evidence is rendered as a long ordinal list rather than connected
   fragments.**
8. **Several semantic/data defects from Archive 7 remain and directly damage
   the diagrams: equivalent components, broad association scope and etcd
   whole-response rejection.**

The next program should be a browser-driven interaction and diagram corrective,
not another screenshot-only CSS pass.

---

# 1. Exact run matrix

| Repository | Run |
|---|---|
| Casdoor | `20260806-124454-casdoor-e3666462f0d9` |
| etcd | `20260806-124454-etcd-f8ab8fee94b3` |
| Telebot | `20260806-124454-telebot-cfeb8f440443` |
| Chatto | `20260806-124454-chatto-1000266c6770` |
| Restic | `20260806-124455-restic-d448a29fc1ae` |

Desktop document heights at 1440×1000:

| Repository | Overview | Study | Architecture |
|---|---:|---:|---:|
| Casdoor | 2,685 px | 2,991 px | 12,664 px |
| etcd | 3,542 px | 2,865 px | 10,353 px |
| Telebot | 1,701 px | 1,749 px | 2,001 px |
| Restic | 2,423 px | 2,704 px | 9,480 px |

The Architecture height is primarily caused by appending relation diagnostics,
the mechanism list and the component list below the canvas.

---

# 2. Source affordance audit

## Casdoor Overview

Good:

- ordinary Overview object cards are one coherent `<a>` target;
- the whole card changes background on hover;
- exact GitHub target includes pinned revision/path/line.

Problem:

- the primary “Open first” section has one clickable blue button, but the
  visible `main.go:36 · main.go` line is inert.
- The primary button itself has pointer cursor but no visible hover-state
  change.

This creates two competing visual conventions:

```text
source card → whole card is a link
Open first → button is a link, source text is not
```

## Study overview

Across all Archive 8 reports, every visible preview source is inert:

| Repo | Preview locations | Clickable |
|---|---:|---:|
| Casdoor | 23 | 0 |
| etcd | 23 | 0 |
| Telebot | 12 | 0 |
| Chatto | 17 | 0 |
| Restic | 22 | 0 |

Example:

```text
InitAdapter
object/ormer.go:91
```

Only the theme title is a button. The symbol/path preview is not a source action.

The card container itself has no hover state, which is correct for a
multi-zone card, but the title button also has no visual hover change. Discovery
depends almost entirely on the pointer cursor.

## Study detail

The symbol is a source link:

```text
InitAdapter
```

The adjacent exact location is plain text:

```text
object/ormer.go:91
```

Both refer to the same target but only one has action semantics. The source
reference should be one coherent control.

## Architecture relation list

Source paths are plain text:

```text
ldap/server.go:61
main.go:150
```

The raw relation kinds are also primary copy:

```text
CONFIGURES_SECURITY_BOUNDARY
STATIC_CALL_SUPPORTING_RELATION
```

## Mechanism fragment

Source locations are plain text in every report:

```text
main.go:36
ldap/server.go:61
main.go:150
```

Casdoor has five visible source-like Architecture refs in the default page and
all five are noninteractive. etcd has 28/28 noninteractive Architecture source
refs. Restic has 5/5.

## Required source-action rule

Every visible exact source identity must have one of two explicit states:

```text
actionable:
  symbol + path:line form one link/button
  hover/focus clearly changes
  exact pinned destination is verified

unavailable:
  neutral non-link styling
  adjacent reason explains why no action exists
```

Never render a source-colored/monospace path as an unexplained inert value.

---

# 3. Hover and card-signifier audit

## Good

Architecture component card:

- whole card is one button;
- pointer cursor;
- border and shadow strengthen on hover;
- click opens the inspector.

Overview object source card:

- whole card is one anchor;
- background changes on hover;
- no nested interactive descendants.

## Weak or contradictory

Primary “Open first” button:

- pointer cursor;
- no hover color, border, shadow or transform change.

Study title:

- pointer cursor;
- no visible hover change.

Study preview source:

- code/path appearance;
- no cursor/action/hover state.

Association row:

- pointer cursor;
- no hover-state change;
- no chevron or explicit “show witnesses” affordance;
- useful exact witnesses are hidden.

Association witness:

- looks like a source card;
- no additional hover-state change.

## Binding signifier model

### Whole-card action

Use only when the entire card has one destination.

- one `<a>` or `<button>`;
- entire card hover/focus state;
- visible action text or arrow;
- no interactive descendants.

### Multi-action card

Use when title/detail/source actions have different destinations.

- container is not clickable;
- container does not receive whole-card hover elevation;
- each action has its own obvious hover/focus state;
- no event delegation that turns whitespace into an ambiguous action.

### Disclosure

Use:

```html
<details>
  <summary>…count + chevron…</summary>
  <div>independent source links</div>
</details>
```

or an equivalent sibling toggle.

Never put source buttons inside a parent button.

### Static information card

- no pointer cursor;
- no action-like hover elevation;
- no hidden essential content on hover.

---

# 4. Invalid nested interaction in Architecture inspector

Current association DOM after expansion:

```html
<button class="rm-arch__association-row">
  ...
  <div class="rm-arch__association-witnesses">
    <button class="rm-arch__edge-jump">...</button>
    <button class="rm-arch__edge-jump">...</button>
    <button class="rm-arch__edge-jump">...</button>
  </div>
</button>
```

This is invalid nested interactive content.

Observed behavior:

1. Click association row → witnesses expand.
2. Click `createDatabaseForPostgres · object/ormer.go:206`.
3. A pinned GitHub source action is attempted.
4. The click bubbles to the outer association button.
5. The association collapses.
6. Focus falls to `<body>`.

The user loses the exact place they were inspecting.

Required:

- use a disclosure/toggle and sibling links;
- source click must not toggle/collapse the disclosure;
- preserve expanded state;
- preserve selected component;
- preserve map transform;
- keep focus on the source action or return target;
- never rely only on `stopPropagation` to preserve invalid nested markup.

---

# 5. Canvas geometry and hit-testing

## Casdoor initial state

Canvas viewport:

```text
x=307, y=467.95, width=1046, height=720
```

Primary node `Запуск`:

```text
x=495, y=314.95, width=320, height=132
```

Its center is outside the canvas viewport and is covered by the toolbar.

Real pointer click fails:

```text
<nav class="rm-arch__flows"> intercepts pointer events
```

The node looks like a normal clickable component in the canvas model, but the
user cannot click it.

## Casdoor Fit control

After clicking Fit:

```text
surface transform = scale(0.65) translate(...)
Запуск y ≈ 244
canvas viewport y ≈ 468
```

The node is even farther outside the viewport.

Thus “Fit” does not fit the principal graph.

## Cross-repository state

- etcd: 9 visible nodes, initial center hit-tests pass.
- Telebot: 5 pass.
- Chatto: 6 pass.
- Restic: 6 pass.
- Casdoor: only 8 of 19 component centers are initially inside the canvas
  viewport; the primary node is occluded.

## Required tests

After initial render and after Fit:

- every principal node bounding box is inside the canvas viewport, OR the UI
  explicitly switches to a documented readable/pan mode;
- every visible node center passes `document.elementFromPoint` and resolves to
  that component card;
- toolbar and overlays never intercept a component center;
- diagnostic/unclassified nodes may be outside the principal fit only when a
  clear disclosure is used.

If one scale cannot show all text legibly, implement semantic zoom:

```text
overview fit:
  all principal areas visible
  title/count only

readable zoom:
  title + description + metadata
```

Do not call a control “Fit” when it knowingly leaves principal nodes outside.

---

# 6. Canvas wheel and scroll trap

At the Casdoor canvas:

```text
initial page scrollY = 0
initial scale = 1.0

ordinary wheel down over canvas:
  page scrollY remains 0
  scale becomes 0.835

ordinary wheel up:
  scale returns to 1.0
```

The 720px-tall canvas captures ordinary page scrolling and silently zooms.

The viewport hint says “drag the map”; it does not tell the user that ordinary
wheel changes scale.

Required desktop behavior:

- ordinary wheel scrolls the page;
- Ctrl/Cmd + wheel may zoom;
- explicit +/- controls always zoom;
- drag on blank canvas pans;
- drag beginning on a node/control does not pan;
- if wheel zoom is intentionally retained, the hint must explicitly state it
  and a user preference must exist, but ordinary page scrolling remains the
  preferred contract.

---

# 7. Inspector scroll, focus and background context

Current inspector:

- fixed overlay on the right;
- scrollable internally;
- not a dialog;
- focus stays on the selected canvas component when it opens.

Observed nested-scroll behavior:

```text
page scrollY = 580
inspector scrolls to its bottom
wheel down again inside inspector
page behind moves from 580 → 1380
```

The user closes the inspector and returns to a different page position.

Closing the inspector with its close button leaves focus on `<body>`, not the
component that opened it.

Tab after opening also continues through underlying canvas nodes rather than
entering the visible inspector.

Required:

Choose one coherent model.

### Nonmodal docked inspector

- participates in the Architecture workspace layout;
- does not cover the map;
- ordinary tab order reaches it logically;
- no backdrop;
- independent scroll has `overscroll-behavior: contain`.

### Overlay drawer

- role/dialog semantics where appropriate;
- focus enters close/first control;
- Escape closes;
- focus returns to exact trigger;
- background interaction is blocked or deliberately nonmodal;
- inspector uses `overscroll-behavior: contain`.

Do not combine “visually modal overlay” with “keyboard remains entirely behind
it”.

---

# 8. Architecture page is still a long evidence wall

Casdoor Architecture is 12,664px tall.

Order:

1. canvas;
2. relation summary;
3. 47-item mechanism list;
4. unknown frontier;
5. full component list.

etcd Architecture is 10,353px; Restic 9,480px.

The mechanism heading says:

> one honest vertical fragment

but Casdoor renders:

- one process entry;
- two static handoffs;
- dozens of unrelated boundary/resource observations;
- one frontier;

as one ordinal card stack.

This still implies a story/order even though most rows say order is not
established.

Required Architecture information architecture:

```text
[Map] [Relations] [Mechanism fragments] [Component list]
```

or an equivalent segmented/disclosure workspace.

Default:

- map + selected inspector;
- compact relation count;
- compact mechanism-fragment count.

Do not append thousands of pixels of diagnostic content below the map.

---

# 9. Mechanism representation

Current transition schema still has no explicit source/target on every visible
transition. Array order remains visually dominant.

Required graph contract:

```text
source
target
claim_kind
support_mode
ordering
evidence
scenario
limitation
```

Connected adjacency only when:

```text
target(previous) == source(next)
```

or a separate exact join exists.

Current Casdoor should be rendered as connected fragments, for example:

```text
main.go:36 · main
    → exact callsite main.go:150
service.Start
    ⇢ continuation not recovered
```

and separate independent evidence:

```text
ldap/server.go:61
    → ldap.getTLSconfig
```

Boundary/resource observations that are not connected to a path are touchpoint
groups, not “next steps”.

Every source endpoint/path in a fragment is actionable.

---

# 10. Remaining data/diagram defects visible in Archive 8

These are not merely visual.

## etcd

- a useful model proposal is still rejected under `proposal.unknown_anchor_id`;
- local fallback labels/descriptions appear in English inside RU product;
- 28 visible Architecture source refs are inert.

## Telebot

Equivalent membership classes remain:

```text
Запуск бота == Обработка вебхуков
Middleware == Реактивные элементы == Компоновка
```

## Chatto

```text
TLS и безопасность
Точка входа процесса
Интерфейсы жизненного цикла
```

still resolve to the same member set.

## Restic

Six visible model components still reuse the same broad unit/member set.

## Casdoor association scope

`Запуск` still receives all 218 observations through broad/root scope.

These must remain part of the next acceptance program because they determine
whether the diagram is meaningful.

---

# 11. What already passes and must not regress

- Architecture edges are passive.
- Component card hover is clear.
- Canvas drag/pan works.
- +/- controls work.
- Study route opens at top.
- Browser Back restores previous Study scroll.
- Overview object source card is a coherent single link.
- Exact static GitHub actions use pinned revision and safe target attributes.
- Local Architecture survives model rejection.
- No mobile work is required.

---

# 12. Required real-browser exploration protocol

A browser reviewer must not stop after screenshots or DOM assertions.

For every relevant report:

1. Move the pointer across every card type.
2. Record cursor and visible hover changes.
3. Click whitespace, title, code symbol, path, badges and secondary actions
   separately.
4. Verify no action zone triggers a different sibling action.
5. Expand every disclosure type and click a child source.
6. Confirm parent disclosure remains stable.
7. Scroll body, canvas and inspector with the wheel.
8. Drag blank canvas.
9. Click +/-/Fit.
10. Hit-test every principal component after initial render and Fit.
11. Open/close inspector; check page scroll and focus restoration.
12. Navigate Study → detail → Back.
13. Check every source-looking path/symbol for a real destination or explicit
    unavailable state.
14. Capture screenshots for default, hover, expanded, selected, scrolled and
    returned states.
15. Inspect console errors.

A static browser screenshot is not an interaction PASS.
