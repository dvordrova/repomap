# 278 — Target label and freshness truth corrective

**Status:** ACTIVE (owner-authorized final UI truth corrective, 2026-08-10)

**Preserves:** Decisions 269 and 271, exact target refs and display paths,
target navigation v1, report45, manifest18, repository-freshness authority,
provider contracts, routes and sibling-page links.

## Product defects

A module-root target was rendered with the exact repository-relative path `.`.
That path is valid backend identity, but it is implementation notation rather
than a useful target name. In Telebot it made a complete four-package portfolio
look like one unnamed target plus three packages.

The `fresh` repository-authority state means that the current checkout still
matches the captured snapshot. The UI translated it as `clean` / `чистый`,
which falsely claimed a clean Git working tree for a stable dirty checkout.

## Approved corrective

The rail keeps `display_path == "."` unchanged in transient navigation
authority and validation, but presents the terminal module name as its visible
label. A semantic-import major suffix (`/vN` or `.vN`) is presentation-only and
is removed, so `gopkg.in/telebot.v3`, `go.etcd.io/etcd/v3` and
`github.com/moby/moby/v2` render as `telebot`, `etcd` and `moby`. Non-root target
paths, `go.mod` grouping, exact titles,
target refs, default selection and sibling links remain unchanged.

The freshness label becomes `current` / `актуален`. When the already-published
static source authority marks the captured working tree dirty, provenance also
shows `local changes captured` / `локальные изменения учтены`. No Git state is
inferred in the browser and no new persisted field is added.

UI catalog identity advances UI24→UI25. No report, manifest, target-navigation
or provider identity changes.

## Acceptance

- Telebot's module-root target is visibly `telebot`, while its exact path stays
  `.` and the group header stays `go.mod`.
- A fresh clean checkout is labelled `current`, never `clean`.
- A stable dirty checkout is labelled `current` and separately says that local
  changes were captured.
- Non-root targets retain their exact repository-relative labels.

Approved by:
    Repository owner through the final product-completion authority and the
    explicit post-reload request to verify nothing remained unfinished,
    2026-08-10.
