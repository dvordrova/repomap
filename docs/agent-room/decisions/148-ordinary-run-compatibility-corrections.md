# Decision 148: Ordinary-run compatibility corrections

## Status

Approved by the repository owner while validating Decision 147 on real
repositories, with the explicit instruction to combine the required fixes for
delivery through Git.

## Observed failures

The Russian Chatto run retained a `go/packages` diagnostic position relative
to its nested Go module as though it were relative to the package directory.
That invented `cli/cmd/cmd/license.go`, made the early source-catalog
preflight fail, and skipped every optional model stage.

The `github.com/devodev/go-office365/v0` run was locally coherent but refused
remote orientation because its README documents an all-zero `ClientSecret`
placeholder.

## Corrective contract

- Relative package diagnostic positions resolve only to files already listed
  by the loaded package. Module-relative paths are joined exactly once;
  unknown diagnostic paths remain non-openable.
- An assignment value made entirely of at least eight zeroes is an explicit
  placeholder. Mixed numeric values and all existing credential shapes remain
  fail-closed.
- The `/v0` module suffix remains part of module, package, symbol, surface,
  and report identities.

No provider request shape, report format, HTTP behavior, source authority, or
ordinary in-bound English behavior changes.

## Acceptance

- focused tests cover module-relative diagnostic positions and reject an
  unknown diagnostic file;
- secret scanning ignores the all-zero placeholder but still detects an
  almost-identical mixed numeric credential;
- full provider-backed Chatto and `go-office365/v0` runs reach optional model
  stages and publish authorized reports.
