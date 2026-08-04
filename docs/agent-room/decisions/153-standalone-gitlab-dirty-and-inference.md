# Decision 153: Standalone GitLab dirty checkout and project inference

## Status

Approved by the repository owner after product use exposed two unnecessary
requirements in Decision 152.

## Product correction

`--gitlab-url` no longer requires a clean checkout. Repomap may publish a
standalone report from a stable dirty working tree because repository freshness
already fingerprints local contents before analysis and confirms that the same
contents remain at publication.

A host-only value such as `--gitlab-url https://gitlab.company` derives the
namespace/project path from the credential-free repository-local Git `origin`
identity. A complete project URL remains accepted and takes precedence.

## Contract

- The repository `HEAD` and captured working-tree contents must remain
  unchanged during the run.
- Source paths unchanged from `HEAD` link to the captured GitLab commit and
  exact line as before.
- Modified, added, deleted, renamed, conflicted, or untracked source paths do
  not link to `HEAD`: that would misrepresent local contents and line numbers.
  They remain visible as local-only locations in the static report.
- The standalone HTML carries only a global dirty marker and the bounded
  analysis-relative intersection of dirty paths with openable source paths.
  It carries no dirty content or content digest.
- Canonical `report.json`, manifest, HTTP, and JSON formats remain unchanged.
- Host-only inference uses the sanitized repository-local `origin` identity,
  never a module name or manifest name. The inferred remote host must match the
  supplied GitLab host.
- A missing, local-only, unsafe, or host-mismatched remote requires the caller
  to provide the complete GitLab project URL.
- Analyzed submodule source and `--source-episode` remain unsupported in
  standalone GitLab mode.
- No GitLab request, provider request, synthetic commit, push, or source
  snapshot is introduced.

## Acceptance

- stable dirty checkout publication succeeds;
- a working-tree change during the run still rejects publication;
- clean source links remain SHA-pinned and dirty source locations have no
  external href or local API fallback;
- the report explains dirty/local-only behavior in English and Russian;
- host-only GitLab URLs resolve through repository-local HTTPS or SSH/scp
  remotes without retaining credentials;
- complete project URLs remain independent of remote configuration;
- standalone HTML still omits source bodies, local absolute paths, and
  authority-only metadata;
- focused, full, and nearby etcd checks pass.
