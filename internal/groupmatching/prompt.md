# Cross-target group matching

You receive one exact cross-target group-pair dossier derived from the complete, already-built GroupsIndex set. Decide only whether the two endpoint groups themselves participate on opposite sides of one direct cross-target integration boundary.

Start with the non-empty `witness_candidates` catalog. Candidate-free pairs are closed locally because no response row could satisfy the witness contract, so every pair sent to you contains at least one `j*`. Each row is a request-local joint that Go has already established mechanically between the fixed left and right endpoint groups. It names one advertised boundary edge and its owning source pattern and argument on each side. The two arguments have an equal direct or locally reconstructed literal/template value. `support_resolution` is derived locally: `exact` means both boundary anchors and the shared value are exact. `possible` means either that the shared reconstructed value is only possible, or that exactly one semantic-trigger receiver-origin anchor is an `alternatives` relation opposite an exact anchor while the shared argument value itself is exact.

A witness candidate is evidence, not a connection. Select a `j*` only when the referenced endpoint groups and boundary patterns describe the two direct sides of the same integration. Go will create no cross-target connection merely because the candidate exists. Conversely, do not reconstruct a joint yourself: the response selects closed `j*` refs and never copies argument values, edge tuples, resolutions, subject refs, or repository identities.

Some candidates also carry `required_from_group_ref` and
`required_to_group_ref`. Go emits these only for a narrowly proven orientation:
one side is a positive inbound delivery boundary and the other is a positive
dependency-category exact outbound call. Conflicting dual-role subjects do not
receive this constraint. These fields are closed direction authority for that
candidate. They do not appear for ambiguous shapes such as arbitrary
background polling or two outbound dependencies.

The endpoint groups contain their complete model-selected `member_refs` and `evidence_refs` plus deterministic `boundary_edge_refs`. Every advertised boundary edge is an existing `structural_edges` row that joins a group-owned pattern to one exact non-platform external symbol through `pattern_target`, `pattern_receiver`, or `pattern_receiver_origin`. The pattern source is either a group member or belongs to finite `owner_ref`/`container_ref` ancestry rooted at one. This allows a readable card to own a boundary performed inside its nested callback or call-result object without making that implementation object a presentation member.

Use `subjects`, `structural_edges`, and `local_connections` to understand what a candidate means. They are context, not automatic proof. A matching name, type, selector, domain concept, or graph proximity alone is insufficient. A downstream group that calls a local helper which later reaches an integration boundary is not itself the direct endpoint. Likewise, an upstream domain group used by a boundary handler is not itself the direct endpoint. Existing local connections may explain such indirect adjacency but must not be promoted into a cross-target edge.

Return strict JSON with exactly this shape:

```json
{
  "connections": [
    {
      "pair_ref": "p1",
      "from_group_ref": "g4",
      "to_group_ref": "g1",
      "semantic_kind": "uses_http_api_of",
      "label": "uses level API",
      "summary": "The frontend HTTP service uses the backend level API endpoints.",
      "witness_joint_refs": ["j1", "j2"]
    }
  ]
}
```

Rules:

- Output is sparse. Return no row unless the endpoint groups themselves have a direct candidate-backed integration. An empty `connections` array is valid. Never echo a row merely to acknowledge the pair.
- `pair_ref` must be the supplied `pair.ref`. `from_group_ref` and
  `to_group_ref` must be exactly its two endpoint group refs. Either direction
  is allowed when selected candidates carry no required direction. If any
  selected candidate has `required_from_group_ref` and
  `required_to_group_ref`, use those exact endpoints; all selected candidates
  must agree. Request order is never direction: do not default to left-to-right
  or copy the `left_group`/`right_group` order.
- Direction must agree with the grammar of all three authored fields.
  Interpret the row as the sentence `FROM semantic_kind TO`: `from_group_ref`
  is the actor/caller/client/producer and `to_group_ref` is the acted-on
  provider/server/consumer. The grammatical subject of `summary` must describe
  the `from_group_ref` endpoint. For example, if the summary says “the frontend
  HTTP service uses the backend API”, the frontend group is `from_group_ref`
  and the backend API group is `to_group_ref`, even when the backend happens to
  be `left_group`. A row whose refs, kind, label, and summary disagree on
  direction is invalid; reverse the refs or rewrite the semantics before
  returning it.
- `witness_joint_refs` must contain one or more refs from this request's `witness_candidates`. Select every advertised candidate that supports this same directed semantic connection. Unknown refs have no authority.
- Inspect the candidate's referenced patterns, external endpoints, arguments, and group roles before selecting it. Similar argument values can exist in unrelated integrations; the locally established value joint is necessary but not sufficient semantic evidence.
- `semantic_kind` is an open lowercase `snake_case` vocabulary. Kinds such as `uses_http_api_of`, `publishes_events_to`, and `invokes_command_on` are examples only. Create a more precise kind when appropriate.
- `label` is a concise readable edge label. `summary` positively explains the supported connection without claiming more than the evidence supports. Never return a row whose label or summary says that no connection, no direct relation, or only an unsupported indirect relation exists.
- Respect each selected candidate's `support_resolution`. `possible` support may justify a qualified semantic connection, but it is never proof of an exact framework binding, runtime call, request occurrence, or execution path. Do not describe it as exact.
- Select only request-local refs. Do not emit target IDs, group IDs, subject IDs, relation IDs, digests, repository paths in place of refs, or any unadvertised identifier.
- Do not return `evidence_refs`, expanded witness objects, copied values, or support resolution. Go revalidates every selected `j*`, restores bilateral pattern evidence, and derives the strongest surviving support resolution locally.
- Do not manufacture support, unassigned, complement, exhaustive, or fallback connections.

Return JSON only.
