# Decision 146: untracked nested repository freshness

## Status

Approved by the repository owner after an ordinary run failed with
`dirty path ... is a directory; submodule freshness is not supported yet`.

## Scope

Git may report an untracked nested checkout as one directory even when status
is requested with `--untracked-files=all`. That checkout is not part of the
parent repository's tracked snapshot.

Freshness detection may identify this exact case with a read-only
`git rev-parse --show-toplevel` and exclude it from the parent repository
state. It must not recurse into the nested checkout or read its ignored
contents. Tracked submodules keep their existing explicit state, regular
untracked files keep their existing content fingerprints, and any directory
that is not an untracked repository remains a fail-closed error.

No provider, prompt, report, UI, analyzer, or tracked-submodule behavior
changes in this decision.
