# Add an external client — experiment result

The experiment answers one repository task: how a newcomer can add an outbound
client without first reconstructing the whole service.

## Result

| Layer | Verdict | What it established |
| --- | --- | --- |
| H0 dependency/import baseline | PARTIAL | Found all four real boundaries, but also admitted two critical false positives and could not establish roles, completeness, evidence, or a best example. |
| H1 typed structural footprint | PASS | Recovered 4 production boundaries, 32 of 32 expected role assignments, 43 of 43 evidence locators, all 6 exclusions, and exact `10 = 4 + 6` accounting. |
| H2 presentation copy | Non-authoritative | Adds bounded titles, purposes, and summaries for six short step refs and four short example refs. It cannot change roles, examples, completeness, recommendation, evidence, or exclusions. |

Three boundaries are complete. Kubernetes is the deterministic recommended
example because it is complete, covers all required and common roles, and wins
the local stable tie-break. Notifier remains incomplete with exactly three
missing roles: verification, observability, and failure policy.

## Demo

Serve this experiment directory on loopback and open
`preview/report.html`. The standalone file contains all CSS, JavaScript, and
validated browser data; it has no external assets or network calls.

The UI starts with the task picker. Open **Add an external client** to see six
repository-specific steps and three complete examples. **Show all 4** reveals
Notifier. Source paths are not created in the DOM until **View evidence** is
chosen. **Candidate audit** is a separate disclosure with the six rejected
candidates and their closed reasons.

## Honest limits

- Evidence comes from one deliberately crafted Go service, not a corpus of real repositories.
- H1 is a test-only extraction hypothesis and is not integrated into the ordinary repomap pipeline.
- H2 is presentation-only and is tested with an injected deterministic provider; there is no live-provider acceptance claim.
- The HTML is a task prototype, not a change to the production report, schema, CLI, or current ADR.
- No user study was run. The manual protocol records expected observable behavior only.
