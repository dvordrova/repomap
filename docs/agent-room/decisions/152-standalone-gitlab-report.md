# Decision 152: Standalone GitLab report

## Status

Approved by the repository owner for sharing a generated report as a
self-contained static artifact, including upload to object storage.

## Product outcome

`repomap --gitlab-url <project-url>` produces one standalone `report.html`.
The report does not depend on a running repomap server. Source locations open
the analyzed repository in GitLab at the exact captured commit and line instead
of opening the local editor or a saved-source drawer.

## Contract

- `--gitlab-url` implies no-server/static report generation. The normalized
  project URL is used only to construct links; report generation does not
  contact GitLab.
- Every source link is pinned to the captured repository `HEAD`, preserves its
  exact line anchor, and includes the repository-relative analysis prefix when
  repomap analyzed a subdirectory of the GitLab project.
- Static HTML does not embed saved source bodies, source windows, or local-only
  paths and debug metadata needed by the localhost report authority.
- The existing `report.json`, run manifest, HTTP, and JSON formats remain
  unchanged. The GitLab presentation configuration is HTML-only.
- Static GitLab export requires a clean analyzed checkout. Analyzed submodule
  source is rejected because one parent-project URL and revision cannot address
  a submodule repository; per-submodule remotes are outside this slice.
- Static GitLab export and `--source-episode` are mutually exclusive because a
  source episode depends on the local authorized source authority.
- Private GitLab authentication remains the browser's responsibility. No
  credential or access token is embedded in the report.
- Repomap does not contact GitLab to prove that the captured commit is
  reachable. The user must push that commit before sharing when it is not
  already present on the supplied project.
- This mode adds no provider request, repository upload, network lookup,
  language adapter, Search surface, or hosting integration.

## Acceptance

- a generated report is one self-contained HTML file and its navigation works
  from a static origin without reportserver APIs;
- file and symbol actions construct escaped GitLab blob URLs pinned to the
  captured commit and line;
- source bodies and local-only metadata are absent from the standalone HTML;
- canonical report JSON and manifest behavior remain unchanged;
- invalid GitLab URLs, dirty checkout state, analyzed submodules, and the
  source-episode combination fail with explicit errors;
- focused tests cover URL normalization, nested analysis prefixes, static
  source navigation, and source-content stripping;
- the standalone report is browser-inspected before delivery, and full
  repository checks plus nearby etcd validation pass.
