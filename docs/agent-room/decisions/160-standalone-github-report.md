# Decision 160: Standalone GitHub report

## Status

Approved by the repository owner in the current session.

## Decision

Add `--github-url` as the GitHub-hosted equivalent of the existing standalone
GitLab report mode.

- A complete repository URL or a host-only URL inferred from the sanitized
  repository-local `origin` identity is accepted.
- The report is static and self-contained. Source actions open the exact
  captured commit, file, and line in GitHub; no GitHub request is made.
- Stable dirty repositories remain supported. Changed source paths stay
  local-only instead of linking to the captured remote commit.
- GitHub and GitLab modes are mutually exclusive and keep their different blob
  routes and line-range fragment syntax.
- Saved source bodies and local filesystem authority are not embedded.
  Canonical `report.json` and manifest formats remain host-neutral.
- Existing local-server behavior, model stages, report content, and GitLab
  behavior are unchanged.

## Verification

- URL normalization, origin inference, and unsafe/non-root rejection.
- Offline CLI generation without a local server.
- Exact GitHub line and line-range links across static report source actions.
- Dirty source paths remain unlinked and static mode does not call localhost.
- GitHub routing is present only in standalone HTML.
