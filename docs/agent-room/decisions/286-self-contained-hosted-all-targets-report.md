# 286 — Self-contained hosted all-targets report

**Status:** ACTIVE (owner-authorized, 2026-08-11)

**Preserves:** D269's independent ordinary target runs, canonical target
container and terminal portfolio, sibling-page navigation v2, one canonical
`ReportData` and Manifest authority per target, provider accounting and failure
isolation; D152/D160's revision-pinned standalone GitLab/GitHub source actions;
Report 48, Manifest 18 and all semantic/provider contracts.

## Product gap

An explicit `--all-targets` hosted run still leaves the default standalone
`report.html` dependent on sibling run directories. Copying that one file to
S3 or another static object host therefore breaks its target menu even though
every ready target already owns a complete authorized standalone projection.

## Approved correction

- Only explicit `--all-targets` with exactly one GitHub or GitLab repository
  URL and more than one selected target receives the bundle. Ordinary
  multi-target selection, hosted singletons and no-host runs retain D269's
  sibling pages unchanged.
- Every ready target first completes the existing ordinary generation,
  manifest binding, portfolio finalization and revision-pinned hosted render.
  The report package returns an opaque prepared target containing only that
  target's scrubbed presentation payload and private authority.
- After every ready target is prepared, the default run's `report.html` is
  atomically replaced by one self-contained bundle. Embedded CSS/JavaScript
  assets occur once and each ready target payload occurs once. Unavailable
  targets retain only their disabled catalog descriptor; no partial payload is
  published.
- Canonical container order supplies zero-based target indices. Standalone
  target navigation v3 uses ordinary same-document links of the exact form
  `?target=INDEX#canvas`. A small bootstrap chooses the payload before the main
  application starts. Missing, repeated, non-canonical, out-of-range or
  unavailable query values select the exact container default, which need not
  be index zero. The links resolve identically from `file://` and static HTTP
  object URLs.
- The bundle embeds no sibling run ID, run directory or local absolute path.
  Every prepared payload must agree on source host, normalized repository URL,
  exact revision and presentation language authority. Source bodies remain
  stripped while useful revision-pinned source locations remain.
- The complete ready-payload aggregate has a terminal 1 GiB bound. It is never
  prefix-clipped. A sealed marker identifies the exact container/portfolio,
  default index and target counts. Recovery inspection is path-based,
  bounded-memory and verifies the streamed artifact seal; an ordinary
  pre-bundle HTML file is reported as absent rather than reinterpreted.

Standalone bundle wire v1 and target navigation v3 are presentation-only.
The typed UI catalog advances 36→37 without adding product prose. Report 48,
Manifest 18, target page portfolio v1 and D269 navigation v2 remain unchanged.

## Acceptance

Provider-free tests prove a non-zero default, exact invalid-query fallback,
disabled unavailable targets, file/static-HTTP URL resolution, source
revision pinning and local-path/source scrubbing, asset and payload uniqueness,
cross-target authority rejection, terminal aggregate bounds, atomic failure,
ordinary-report detection and tamper rejection. The owner-facing integration
then builds with `make build`, runs a real hosted `--all-targets` repository and
opens only the copied default HTML in a browser before upload acceptance.

Approved by:
    Repository owner after requesting that explicit hosted `--all-targets`
    produce one HTML file that can be uploaded directly to S3, 2026-08-11.
