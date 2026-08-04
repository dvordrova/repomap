# Decision 158: Recover the build-selected Cobra command inventory

## Status

Held and superseded by Decision 159.

This document preserves the exact-tree reconstruction attempt as historical
context. Implementation review showed that attributing global instances,
mutation order, wrapper reachability, and complete command hierarchy required a
fragile miniature interpreter. Its deep reconstruction contract is no longer
active scope. Decision 159 keeps the useful typed facts while replacing the
tree requirement with a generic shallow inventory.

## Attributable failure

The current command reader parses only files in a `main` package and follows a
small package-local syntax shape:

`main -> newRootCommand().Execute -> root.AddCommand(newChildCommand())`.

Real repositories commonly assemble the same exact framework differently.
For example, etcd uses:

`main -> imported MustStart -> Start -> global rootCmd.Execute`, while
`init` functions register commands through imported constructors. Named
`Run`/`RunE` handlers and nested command groups are also ordinary Cobra
patterns. Those registrations are currently absent from the report even though
the existing surface-discovery stage already loaded the required build-selected
type and SSA information.

This is a fact-collection defect, not evidence that the repository has no
commands.

## Corrective contract

- Reuse the existing build-selected `go/packages` and SSA load. Do not launch a
  second loader and do not scan ignored or unselected source.
- Recognize only the canonical `github.com/spf13/cobra.Command` type and its
  exact `AddCommand`, `Execute`, and `ExecuteContext` methods. Lookalike names
  are not Cobra evidence.
- Resolve package globals, unambiguous local aliases, command constructors,
  composite literals, nested registrations, and named or literal `Run`/`RunE`
  handlers.
- Root command publication in an exact Execute receiver attributable through a
  bounded static wrapper chain to a build-selected `main`.
- Admit `init` registrations only when they mutate a rooted command in an
  import-reachable repository package.
- Publish exact command identity, hierarchy, registration, constructor,
  handler, process entrypoint, and wrapper locations when available. A rooted
  command with a missing handler remains an honest partial entry surface.
- Keep the legacy `gofacts` command reader for no-surface and old-artifact
  compatibility, but prefer the typed record when both describe the same
  executable command.
- Use deterministic, independently visible Cobra limits. Sort before
  truncation and record every reached limit in surface coverage.
- The ordinary report must count typed Cobra records as CLI commands and make
  their exact locations available to later Study/path composition.

## Product meaning

This slice expands the factual inventory. It does not require every command to
form a complete Mechanism. A command is an Entry point; it can immediately
support an incomplete reading path, and later participate in a complete
trigger-to-effect Mechanism when sufficient downstream facts exist.

## Acceptance

- a fixture reproduces imported `MustStart -> Start -> rootCmd.Execute`;
- `init` registration, constructors, nested groups, and named `Run`/`RunE`
  handlers are recovered with exact locations;
- a fake local `Command.AddCommand` is ignored;
- ambiguous aliases remain unresolved instead of guessed;
- typed records win over equivalent legacy records without duplicate cards;
- representative etcd commands such as `get`, `put`, `lease grant`,
  `endpoint health`, `snapshot save`, `user add`, and
  `role grant-permission` are present;
- focused tests, `./scripts/check.sh`,
  `./scripts/etcd_check.sh /Users/dvordrova/git/etcd`, and
  `git diff --check` pass.

No lifecycle pairing, Mechanism orchestration, provider prompt, Search,
source snapshot, report format, or new framework/plugin API is part of this
corrective.
