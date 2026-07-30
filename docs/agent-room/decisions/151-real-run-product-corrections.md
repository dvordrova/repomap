# Decision 151: Real-run product corrections

## Status

Approved by the repository owner through the requested fresh product review of
`fujiwara/lambroll` and `ngoduykhanh/wireguard-ui`.

## Observed failures

- Study bundle safety scanning rejected ordinary Go assignments such as
  `server.Password = options.password` and `client.PrivateKey = key.String()`.
  The right-hand sides are runtime references, not credential literals.
- An accepted Architecture component containing exact file and symbol members,
  but no package member, advertised one exact anchor and did nothing when
  clicked.
- When Study was unavailable and no complete mechanism existed, Overview
  silently substituted ranked saved-source snapshots. In Russian mode this
  fallback also exposed untranslated presentation copy and preferred an
  English repository-thesis purpose over the accepted Russian project guess.

## Corrective contract

- The credential scanner ignores an unquoted Go-style dotted selector on the
  right-hand side of a credential-shaped assignment. Quoted dotted values,
  literal tokens, private keys, and all other existing credential forms remain
  fail-closed.
- Any accepted Architecture component with an exact openable member location
  receives a compact inspector and a direct source action. Package membership
  is needed only for package-wide Study joins, not for exact member actions.
  A package-only component receives one deterministic repository-owned file
  action when no more precise source member exists.
- Ordinary Overview never renders saved source snapshots as a substitute for
  missing Study or mechanisms. Exact locations remain available through area,
  Study, and Architecture actions.
- Search is removed from the product path: no CLI switch, index construction,
  saved payload, HTML surface, route, or shipped Search assets remain.
- Russian fallback Overview prefers the accepted Russian project guess and all
  locally authored fallback presentation copy participates in the report
  language layer. Exact paths, symbols, packages, and technical terms remain
  unchanged.
- `--no-secrets` is an explicit unsafe product override that disables
  credential detection for one process run. It prints a terminal warning and
  is retained in run metadata. Tracked selected source may then reach the
  bounded provider request and debug artifacts; ignored files are still
  outside the tracked snapshot.
- No new provider call, runtime relation, language adapter, Search surface,
  source snapshot, or report format is introduced.

## Acceptance

- focused scanner tests distinguish runtime selector references from literal
  credentials;
- a focused CLI test covers the scoped unsafe override, warning, and metadata;
- ordinary CLI reports omit Search and expose no Search compatibility switch;
- a file/symbol-only Architecture component has a compact source-backed
  inspector;
- fallback Overview does not render source snapshots and its Russian UI copy is
  localized;
- fresh uncached Russian runs of `fujiwara/lambroll` and
  `ngoduykhanh/wireguard-ui` are reviewed across Overview, Study, and
  Architecture;
- full repository checks and nearby etcd validation pass.
