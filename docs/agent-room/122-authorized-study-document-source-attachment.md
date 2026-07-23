# Decision: Authorized Study document source attachment

Status: Approved by the repository owner through the active overnight
continuation goal after the Study audit identified already referenced documents
that were rendered as path-only cards.

## Product outcome

When a Study direction already references a document and the report is generated
with verified repository authority, the default reader should see a bounded
source excerpt for that document instead of a path-only card.

## Approved implementation scope

1. Keep canonical Study records unchanged.
2. Do not alter Study IDs, semantic/model responses, reducer inputs, or saved
   evidence.
3. Add only a presentation-time fallback during authorized report generation.
4. Use only already authorized local paths:
   - the document path must already be present in the Study bundle/openable path
     set;
   - the path must be README/doc-like documentation;
   - the file must be a regular file under the authorized analysis root.
5. Keep the same bounded document excerpt size used by Study bundle collection.
6. Do not send fallback bytes to the model.
7. Preserve graceful degradation: if the file cannot be read, keep the existing
   path-only behavior and debug coverage.

## Truth boundary

This fallback is presentation-only source attachment. It makes an already
selected document readable in the report. It does not create a new fact, claim,
direction, mechanism, evidence relation, or canonical Study identity.

## Explicit non-goals

- No prompt change.
- No model call.
- No validator weakening.
- No repository-wide analysis.
- No broader source budget increase.
- No report parser authority expansion when report generation is not authorized.

## Focused verification

- Unit test for authorized fallback attachment.
- End-to-end `GenerateAuthorized` test proving the persisted `report.json`
  gains document source, `run_manifest.json` still verifies, and the canonical
  Study record remains unchanged.
- Existing path-only diagnostic test remains valid without an authorized source
  root.
- Replay the copied repomap run with `--replay-saved`.
- Confirm path-only Study documents drop to zero in the audit.
- `git diff --check`.
