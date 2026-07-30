# Decision 162: Provider-free Architecture localization adapter

## Status

Approved by the repository owner through the explicit instruction to continue
the localization night plan after Decision 161.

## Problem

Decision 161 defines a safe canonical/projection contract but deliberately has
no product-shaped consumer. The existing report cannot yet demonstrate that a
real semantic object survives an English identity projection byte-for-byte or
that a Russian projection changes only prose.

Several apparent report IDs are not suitable localization owners. Legacy
report component IDs, orientation flow IDs, and Study direction IDs currently
depend on model-authored names or questions and can therefore change with the
output language.

Architecture Canvas subsystem and component IDs are different: they are
reconstructed from validated local member sets rather than localized wording.
They are the narrowest existing semantic owners that match the allowlisted
localization contract.

## Decision

Add one provider-free compatibility adapter between `ArchitectureCanvas` and
`internal/localization`.

The adapter may include only:

- subsystem `name` and `description`;
- component `name` and `description`;
- exact structured subsystem/component/source IDs, member/parent IDs and names,
  participation flow IDs, fact values and paths, and component
  flow/surface/investigation/anchor/source IDs as typed protected terms when
  they occur in that prose.

The adapter must:

- use the existing subsystem/component IDs without deriving identity from
  names, descriptions, array positions, or locale;
- accept only a Canvas already returned by the validated local
  `ProjectArchitectureCanvas` path;
- rebuild the canonical artifact and localization input from the current
  Canvas before applying a supplied projection, rejecting stale artifacts even
  when their stable field IDs happen to match;
- reconstruct an English identity projection without changing any byte of the
  serialized Architecture Canvas;
- apply a supplied Russian projection only to the allowlisted fields;
- preserve every ID, member, relation, source location, enum, ordering, and
  non-prose field exactly;
- validate the complete projected result field set before copying or changing
  any Architecture Canvas prose;
- retain Decision 161's field-level canonical-English fallback and diagnostics
  for missing or invalid translations, while projection-envelope failures fall
  back to the complete canonical-English canvas;
- make no provider or network call.

## Explicitly not authorized

- ordinary run wiring or writing new files beside a run;
- changing `--lang`, `deepseek.Client.OutputLanguage`, semantic prompts, or
  existing cache keys;
- treating historical Russian semantic responses as canonical English;
- a Russian-to-English translation;
- localizing Study, Guided Tour, flows, surfaces, diagnostics, source, or
  static UI copy;
- changing report JSON, HTTP, run manifest, freshness, or browser behavior;
- removing the current DOM localization path;
- live provider calls or external repository runs.

## Acceptance

- a saved-shaped Architecture Canvas survives English identity projection
  byte-for-byte;
- a supplied Russian projection changes only the four allowlisted field kinds;
- technical terms present in structured Architecture data survive
  byte-for-byte;
- projection failure cannot partially mutate the input canvas;
- one invalid field falls back to canonical English without discarding valid
  Russian fields or changing non-prose data;
- field IDs are stable when prose changes;
- focused package tests and the full repository check pass provider-free.

## Migration and rollback

This decision adds no persisted artifact and no ordinary consumer. A following
decision may authorize sidecar artifacts only after this adapter passes exact
compatibility tests. Rolling back removes the adapter, its tests, and this
decision without changing existing runs or reports.
