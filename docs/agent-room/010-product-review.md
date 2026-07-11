# Product/User Advocate Review: repomap Report UX Redesign

**Reviewer**: product/user advocate
**Date**: 2026-05-24
**Verdict**: **NEEDS CHANGES** — strong foundation conceptually, but the proposed changes must be shaped around the user's real question: "what should I open next, and why?"

---

## 1. User Journey Analysis

### The onboarding moment

A developer opens `report.html` for the first time. They just ran `repomap ../etcd` — a one-liner. The browser opens. What happens next determines whether `repomap` becomes a habit or gets `rm -rf`d.

**Current experience (broken)**:
1. They see the repo name and project guess in a dark header. Good start.
2. They see flow cards with `F: 12 T: 5 P: 3`. They don't know what F, T, or P mean. These are pipeline counters masquerading as UI. **User abandons or scrolls confused.**
3. They click a flow tab and see a giant paragraph of summary text, then a "Likely Chain" section with side-by-side cards — no arrows, no temporal flow, no visual storytelling. They wonder: "Is step 1 before step 2? What connects them?"
4. They scroll down and see `S: T: D: P: E:` counters. More abbreviations. They feel like they're reading a JSON dump in HTML clothing.
5. They find `files_to_read_in_order` buried mid-page as a plain file list. They might skim it, but there's no visual cue that this is *the most important section on the page*.
6. They close the report, unsure what to do next.

**Ideal experience (target)**:
1. **Repo identity** — name, one-line guess, confidence badge. Immediately answers: "What is this?"
2. **Flow cards with purpose** — each card says what it is, how confident we are, and *why the user should care*. The first card has a **"Start Here"** indicator. User clicks it.
3. **Flow story page** — a vertical waterfall timeline of steps connected by arrows/dots. Each step has: what happens, evidence files, confidence. The user understands the flow at a glance.
4. **Read Order section** — prominent, numbered: `1. Open server.go — Entry point for gRPC Put handler`. User knows *exactly* what to open in their editor next.
5. **Known unknowns** — a sidebar or collapsible section with warnings and uncertainties. The user knows what's reliable vs. speculative.
6. **User leaves with an action** — they open the first file in their editor. `repomap` did its job.

### The emotional arc

A good dashboard makes the user feel **oriented**, not **overwhelmed**. Each section should answer a specific question:

| Section | User's Question |
|---|---|
| Header | "What did I just map?" |
| Flow cards | "What are the important runtime flows here?" |
| Flow timeline | "What happens in this flow, step by step?" |
| Read order | "What should I open next, and why?" |
| Warnings/unknowns | "Where should I be skeptical?" |
| Tests | "Where can I see this in action?" |

Every piece of UI that doesn't answer one of these questions is clutter.

---

## 2. Information Hierarchy

### Most important → least important

**Overview page (first thing user sees)**:

| Priority | Element | Prominence |
|---|---|---|
| 1 | Repo name + project guess + confidence | Full-width hero/header |
| 2 | Recommended first flow ("Start Here") | Visually distinct card with call-to-action |
| 3 | Other flow cards (sorted by confidence, then by read-order count) | Grid/flex cards, smaller |
| 4 | Global warnings (if any) | Banner below header, dismissible |
| 5 | Artifacts dir (debug info) | Footer, muted, expandable |

**Flow detail page (when user clicks a flow card)**:

| Priority | Element | Prominence |
|---|---|---|
| 1 | Flow name + confidence badge + one-line summary | Top of page, sticky header |
| 2 | Waterfall timeline (likely_chain with arrows) | Full-width, immediately visible after header |
| 3 | **Read Order** — numbered, with reasons | Prominent, below timeline. This is THE answer |
| 4 | Tests to explore | Collapsible, after read order |
| 5 | Unknowns + warnings | Sidebar or collapsible section; tied to relevant steps |
| 6 | Evidence files (per step) | Inline tooltip or expand on step click |
| 7 | Bundle stats (technical details) | Collapsed by default, in a "Technical Details" expander at the bottom |

### Anti-hierarchy (what NOT to do)

- Do NOT show bundle counters first. The user doesn't care how many files we selected. They care *which* files to open.
- Do NOT bury the read order after unknowns, warnings, and bundle stats. The read order is the answer to the user's core question.
- Do NOT show technical abbreviations (`F:`, `T:`, `S:`, `D:`, `P:`, `E:`) anywhere in the visible UI. These belong only in the hidden technical details section, fully spelled out.

---

## 3. Specific UX Decisions

### 3.1 "Start Here" / recommended-first-flow indicator

**Decision: YES, mandatory.**

The overview should visually elevate the flow with the highest confidence *and* the most complete data (longest read-order list, fewest unknowns). Label it **"Start Here"** or **"Recommended First"** with a distinct visual treatment — a subtle glow, a colored left border, or a star/arrow icon. Without this, the user faces a choice among 3–5 equally-weighted cards and experiences decision paralysis.

**Implementation guidance**: Pick the flow with `max(confidence * len(files_to_read_in_order))` to prioritize flows that are both reliable and actionable.

### 3.2 Waterfall timeline (likely_chain with connecting arrows)

**Decision: YES, this is the single most important visual upgrade.**

The current side-by-side flexbox cards fail to communicate sequence. A vertical waterfall timeline with:

- Numbered step circles/badges connected by a vertical line or downward arrows
- Each step card to the right of the timeline
- Step name (bold), what_happens (normal), confidence (small badge)
- Evidence files as expandable/collapsible detail within each step card

This transforms `likely_chain` from "here is some data" to "here is the story of what happens."

**Accessibility note**: The vertical line itself is decorative. Step numbers provide the actual sequence information. Screen readers must read steps in DOM order.

### 3.3 Read Order section: numbered list vs. table

**Decision: Numbered list, NOT a table.**

The read order must be the *most prominent section* on the flow page. It answers the user's core question: "What should I open next, and why?"

Format:

```
## Read Order — Open these files in sequence

1. server/etcdserver/server.go
   Entry point for gRPC Put handler. Contains the main request
   routing logic. Start here to understand the request lifecycle.

2. server/etcdserver/v3_server.go
   Implements the gRPC v3 API methods including Put. Contains
   the server-side handler that decodes requests.

3. ...
```

Why a numbered list over a table:
- A table implies comparison/equality. The read order is a *sequence*.
- Numbered list creates a natural reading/opening order that matches editor behavior (open file 1, then file 2).
- Each item has a path (monospace, clickable as a `file://` link) and a reason (prose, not a tag).
- Priority field from `FileItem` can map to visual hierarchy (P1 = prominent, P2 = normal, P3 = muted).

**Critical UX detail**: If supported, file paths should be clickable `file://` links that open in the user's editor/IDE. This turns "what should I open?" into "click and open." Even if the links only work in some browsers, the monospaced path is already easy to copy.

### 3.4 Bundle stats: hide behind "Technical Details"

**Decision: YES — collapsed by default, fully spelled out.**

The current `S: T: D: P: E:` section is the most egregious user-hostile element. These are internal pipeline counters that leaked into the UI.

Rules:
- Collapsed by default behind a **"Technical Details"** toggle at the very bottom of the flow page.
- When expanded, show fully spelled-out labels:
  - `Source files selected: 12`
  - `Test files selected: 5`
  - `Docs selected: 2`
  - `Packages selected: 3`
  - `Related edges: 7`
- Never use single-letter abbreviations in visible UI.

This data answers "how much data did we analyze for this flow?" — a meta-question that most users never ask. It's useful for power users debugging the pipeline, not for orientation.

### 3.5 Unknowns and warnings: placement

**Decision: Inline with the relevant chain step OR a collapsible sidebar. Not a separate dumped section.**

Unknowns and warnings mean different things depending on the flow:

- **Per-step warnings** (e.g., "Low confidence in step 3"): Attach to the step card in the waterfall timeline as a small warning icon/badge. The user should know *where* the uncertainty is, not just that uncertainty exists.
- **Global flow warnings** (e.g., "This flow may not cover the auth layer"): Show in a collapsible callout between the waterfall and the read order, styled as an amber/yellow warning banner. Not a red error — these are "FYI, be skeptical about X."
- **Unknowns** (e.g., "How does auth interact with this flow?"): Show in a "Known Unknowns" sidebar/callout, styled as an informational blue/gray section. These are *good* — they show the AI is honest about what it doesn't know.

**Do NOT** put unknowns and warnings in a single undifferentiated "Warnings" dump. This conflates internal confidence issues with genuine knowledge gaps.

### 3.6 Overview "Read Order" aggregating all flows

**Decision: NO — or only as a lightweight section.**

The overview page *should not* aggregate all read-order files from all flows. That creates a giant undifferentiated list with no narrative context. The read order is meaningful only in the context of a specific flow.

However, the overview *could* show a compact "Quick Start" section at the bottom:

```
## Quick Start — Where to look first

1. gRPC Put Request        → server/etcdserver/server.go
2. etcd Server Startup     → server/etcdserver/server.go
3. Watch Stream            → server/mvcc/watchable_store.go
```

Each line links to the corresponding flow tab. This gives the power user a way to jump directly to the most important file per flow without clicking through each tab.

### 3.7 Error states and partial data

**Decision: Graceful degradation with clear boundaries.**

Flows can have partial data (no DeepSeek response, no chain, empty read order). The UI must communicate this honestly without looking broken:

- **Flow with error (no data at all)**: Show a muted/disabled card on the overview. Label: "Unavailable — offline mode" or "Analysis failed." The card should still be clickable, but the detail page shows a clear explanation of *why* (e.g., "This flow was selected but could not be analyzed because DeepSeek returned an error. Rerun with DEEPSEEK_API_KEY set to get AI analysis."). Do NOT show an empty page with broken sections.
- **Flow with partial data (bundle only, no AI analysis)**: Show the file list and bundle summary, but clearly indicate "Basic analysis only — no AI interpretation." The timeline section should say "No AI explanation available — rerun with API key."
- **Flow with low confidence**: Still show the data. The confidence badge (yellow/red) already signals this. Do not hide or degrade the content — the user can judge for themselves.

**Error anti-pattern**: Showing raw Go error strings like `"cannot read: open flows/grpc-put-request/flow_report.json: no such file or directory"` in the UI. Translate pipeline errors into user-facing messages.

---

## 4. Anti-Patterns to Avoid

### 4.1 The "JSON dump in HTML clothing" anti-pattern

This is the most important anti-pattern. Signs:

- Single-letter abbreviations (`F:`, `T:`, `S:`, `D:`, `P:`, `E:`)
- Raw data sections without context or labels
- Field names leaking into the UI (`bundle_summary`, `selected_files_count`)
- Every piece of data shown at equal visual weight

**Fix**: Every visible element must have a human-readable label and be placed in the user's information hierarchy. If something doesn't have a clear label, hide it or expand it.

### 4.2 The "everything visible at once" anti-pattern

Showing all data on a single scrollable page with equal prominence. The current report has this problem — summary, chain, files, tests, unverified, unknowns, warnings, and bundle stats all compete for attention.

**Fix**: Progressive disclosure. Show the most important information first. Collapse secondary information behind toggles. Use visual weight (size, color, position) to guide the eye.

### 4.3 The "AI-generated wall of text" anti-pattern

The summary text from DeepSeek can be long. Showing it as a single paragraph at the top of the flow page buries the actionable information below the fold.

**Fix**: Show a one-line summary in the header/banner. Put the full summary in a collapsible "Summary" section that's expanded by default but can be collapsed once the user has read it.

### 4.4 The "debug mode by default" anti-pattern

The artifacts directory path, run IDs, and bundle stats are debug information. Showing them prominently in the main report confuses users who just want to understand a codebase.

**Fix**: Move all debug/technical information to a "Technical Details" section at the very bottom, collapsed by default.

### 4.5 The "everything is a card" anti-pattern

Not every piece of data needs to be in a card. The current overview uses cards for everything. The waterfall timeline should use a continuous visual flow, not disconnected cards.

**Fix**: Reserve cards for top-level items (flows on the overview page). Use lists, timelines, and inline elements for sub-items.

---

## 5. Visual Language

### Recommendation: Clean developer dashboard (not terminal-themed, not SaaS-bloated)

**Rationale**: `repomap` is a developer tool, not a monitoring dashboard or a marketing page. The visual language should convey "I am a thoughtful, engineering-quality tool" — not "I am a flashy startup" and not "I am a raw terminal dump."

### Color palette

| Role | Color | Usage |
|---|---|---|
| High confidence | `#16a34a` (green) | Badges, indicators |
| Medium confidence | `#ca8a04` (amber/gold) | Badges, warning accents |
| Low confidence / Error | `#dc2626` (red) | Badges, error boxes |
| Primary text | `#1a1a1a` (near-black) | Body text |
| Secondary text | `#666` (gray) | Descriptions, reasons |
| Code / paths | `#d63384` (magenta/pink) | File paths, code references — kept from current design, it works well |
| Background | `#fff` (white) or `#fafafa` (off-white) | Page background |
| Card background | `#f5f5f5` (light gray) | Card backgrounds |
| Accent / actions | `#2563eb` (blue) | Links, active tabs, "Start Here" indicator |
| Step numbers | `#1e293b` (dark slate) | Waterfall step circles |

### Typography

- System font stack: `-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif` (keep current — it's correct)
- File paths: `'SF Mono', 'Fira Code', 'Cascadia Code', monospace` — add coding-specific fonts before falling back to system monospace
- Sizes: body 14–15px, headings 16–20px, small/caption 12px

### Spacing and layout

- Max width: 960px centered (current 900px is fine, slightly wider gives more room for the waterfall)
- Card padding: 1.25rem
- Vertical rhythm: 1.5rem between major sections, 0.75rem between sub-items
- Waterfall vertical spacing: 2rem between steps (steps are meaty and need breathing room)

### "Start Here" indicator

A subtle visual treatment — not a garish badge:
- A small pill badge next to the flow card title: `▸ Start Here` in accent blue
- OR a light blue-left-border on the card with a subtle glow/shadow

### Waterfall timeline design

```
┌──────────────────────────────────────────────────────┐
│  ● Step 1: Receive gRPC Request                     │
│  │  The client sends a Put request via gRPC to the   │
│  │  etcd server. The request is intercepted by the    │
│  │  gRPC interceptor chain.                          │
│  │                                                    │
│  │  📄 Evidence: server/etcdserver/v3_server.go      │
│  │  🟢 Confidence: 90%                               │
│  ├────────────────────────────────────────────────── │
│  ● Step 2: Validate Request                         │
│  │  The request is validated for auth, quotas, and   │
│  │  lease constraints before being processed.        │
│  │  ...                                              │
│  ├────────────────────────────────────────────────── │
│  ● Step 3: ...                                       │
└──────────────────────────────────────────────────────┘
```

The vertical line connecting step circles creates the "flow" feeling. Each step is a self-contained unit of understanding.

---

## 6. Accessibility

### Color-blind users

- Confidence badges use BOTH color AND text/icons: `🟢 High (90%)`, `🟡 Medium (55%)`, `🔴 Low (30%)`
- The waterfall vertical line is decorative. Step numbers provide the actual sequence information.
- "Start Here" uses BOTH color AND a text label/icon. Never convey information through color alone.

### Screen readers

- Use semantic HTML: `<nav>` for tabs, `<ol>` for read order, `<section>` for major blocks
- Flow cards should be `<button>` or `<a>` elements (not `<div onclick="...">`) so they are keyboard-focusable
- Step numbers should use `aria-label="Step 1: Receive gRPC Request"` or similar
- Confidence percentages should have `aria-label="Confidence: 90 percent"`
- Warnings should have `role="alert"` or `aria-live="polite"` (but only for dynamic content, which this isn't)

### Keyboard navigation

- Tabs must be focusable via Tab key and activatable via Enter/Space
- Flow cards on the overview must be focusable and activatable
- All collapsible sections must be focusable and togglable via keyboard

### Small screens (mobile/tablet)

- The waterfall timeline should stack vertically (it already will, with flex/grid)
- Flow cards on the overview should stack in a single column on narrow screens
- Font sizes should not shrink below 14px
- File paths that are long URLs should `word-break: break-all` or use `overflow-wrap: break-word`

---

## 7. Success Criteria

### How do we know the UX is good?

**Quantitative (observable behaviors)**:
1. User opens `report.html` and clicks a flow card within 10 seconds
2. User scrolls past the waterfall timeline to the Read Order section
3. User copies a file path from the Read Order (indicates intent to open in editor)
4. User navigates between 2+ flow tabs (indicates they found value and are exploring)
5. Return usage: user runs `repomap` on another repo within a week

**Qualitative (user feedback indicators)**:
1. "I ran this on etcd and immediately knew where to start reading" — not "What do these abbreviations mean?"
2. "The waterfall view made the request flow obvious" — not "I couldn't tell what connected to what"
3. "I opened the first 3 files in the Read Order and understood the flow" — this is the ultimate success metric
4. User shows the report to a teammate — word-of-mouth is the best signal

**Failure indicators (what we want to avoid)**:
1. "I just looked at the JSON instead" — if the HTML is worse than raw JSON, we failed
2. "What do F, T, P, S, D, E mean?" — any confusion about abbreviations is a UX failure
3. User closes the report within 5 seconds without interacting — the overview didn't hook them
4. "It looks like a debug dump" — this means we haven't moved past the current design

---

## 8. Summary of Must-Fix Issues

### Blockers (must fix before implementation)

1. **Remove all single-letter abbreviations from visible UI**. `F: T: P: S: D: E:` are hostile to new users. Spell them out, and hide them behind "Technical Details."
2. **Make Read Order the most prominent section** on each flow page. Currently it's buried. This is the answer to the user's core question.
3. **Implement waterfall timeline** with connected vertical steps. The current side-by-side cards without connecting visual elements fail to communicate flow sequence.
4. **Add "Start Here" / recommended-first-flow indicator** on the overview page. Without guidance, users face decision paralysis among equal-looking flow cards.
5. **Collapse bundle stats behind "Technical Details"** by default. These are pipeline internals, not user-facing information.

### High priority (should fix)

6. **Tie unknowns/warnings to specific chain steps** where possible, rather than dumping them in a separate section.
7. **Translate pipeline error messages** into user-facing language. No raw Go errors in the HTML.
8. **Show empty states gracefully** — partial data or missing AI analysis should not look like a bug.
9. **Add `file://` links** to Read Order paths so users can click-to-open in their editor.
10. **Ensure keyboard accessibility** for tabs, collapsible sections, and flow cards.

### Nice to have

11. Quick Start section on the overview aggregating the first read-order file from each flow.
12. One-line flow summary in the overview card (truncated), not just counters.
13. Copy-to-clipboard button next to file paths.

---

## Conclusion

The proposed UX redesign has the right goals: answer "what should I open next, and why?" and make the report feel like a developer dashboard, not a debug dump. The current implementation is a good proof-of-concept but fails at its most important job: guiding the user to action.

The single most impactful change is elevating the Read Order section — with numbered steps, descriptive reasons, and visual prominence — because this directly answers the user's core question. Everything else (waterfall timeline, Start Here indicator, collapsed technical details) supports this primary goal.

A user who opens `repomap`'s report and immediately knows which file to open first is a user who will run `repomap` again.
