# Decision 156: Make publication decisions observable and component starts complete

## Status

Active product corrective, authorized by the repository owner after real
reports hid most exact handler and entry anchors behind one component-level
code start and required ad-hoc `jq` inspection to explain unpublished output.

## Attributable failures

The browser projection already retains a bounded runtime-surface catalog and
exact component members. The inspector nevertheless shows only one generic
component code start. A component such as clients or launch points can
therefore appear to contain one file even when several exact handlers or
registration sites survived surface discovery.

Package identities are also rendered as inert text even when the captured
repository graph contains an exact package-owned file that can use the
existing source authority.

Separately, ordinary progress output reports stage completion but does not
summarize the deterministic reductions that decide publication. Debug
artifacts contain those facts, but understanding a missing Study direction or
Architecture result should not require a custom `jq` query.

## Corrective contract

- The Architecture component inspector keeps the component-owned code start
  separate from a deterministic launch-point list.
- The launch-point list contains one exact target for every retained surface
  owned by that component, preferring its exact handler, then registration
  site, then process entry. It shows the surface/handler label and exact
  `file:line`.
- Study anchors remain Study navigation and package-only file fallbacks remain
  package context; neither is relabeled as an exact component or handler
  start.
- An exact package label is clickable only through a file proven to belong to
  that package in the captured repository graph. It reuses the existing local
  opaque-source or pinned GitLab source action and never infers a repository
  URL from an import path.
- After authorized report generation succeeds, an ordinary run emits a
  bounded, human-readable publication summary assembled from validated report
  state and locally issued stage reason codes. It reports the currently
  available found, accepted, rejected, reduced, grouped, and published counts,
  including whether legacy direction expansion was requested, targeted-research
  skip reasons, and bounded Study-review outcomes.
- The trace never prints provider prose, source contents, credentials,
  authorization data, or an unbounded list of paths or IDs.
- Existing progress lines may gain accepted/rejected counts, but provider
  requests, model prompts, cache keys, analysis budgets, canonical report
  data, manifest data, IDs, HTTP behavior, and source authority remain
  unchanged.

## Acceptance

- a provider-free multi-package fixture keeps component members, Study
  anchors, and launch points distinct and gives each package a package-owned
  source target;
- package navigation exercises the same local/static source router as existing
  file and symbol actions;
- a bounded trace fixture exposes rejection and publication reasons without
  leaking model prose;
- normal progress reports orientation acceptance and rejection separately;
- focused tests, `./scripts/check.sh`,
  `./scripts/etcd_check.sh /Users/dvordrova/git/etcd`, and
  `git diff --check` pass;
- local-server, pinned-GitLab, plain-file, and dirty-GitLab source authority
  cases prove when package and launch-point actions are actually available;
- a real saved report confirms multi-surface components expose multiple exact
  handler or registration starts.

No new analyzer, provider call, report page, report or manifest field, Search
surface, cache behavior, or compatibility layer is part of this corrective.
