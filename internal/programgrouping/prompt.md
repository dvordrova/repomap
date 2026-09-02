# Group categorized program elements

You receive one bounded shard of a language-neutral program graph from Go,
Python, JavaScript, or TypeScript. Use the same semantic contract for every
language and framework.

The request has one of two phases:

- `grouping`: form useful groups directly from categorized program subjects;
- `merge`: consolidate immutable restored group memberships proposed by
  earlier bounded shards and connect them across shard boundaries.

In both phases, `group_refs` is the only closed set from which `member_refs`
may be selected. Every ref in `group_refs` has at least one positive category.
Other rows in `subjects` are incident structural context or evidence only. In
particular, a subject with `categories: []` must never be selected as a group
member. Short refs exist only inside this request; never copy or invent a
canonical identity.

Return strict JSON with exactly this shape:

```json
{
  "groups": [
    {
      "key": "delivery",
      "title": "Order delivery",
      "summary": "Accepts HTTP and queued order work",
      "lane": "triggers",
      "member_refs": ["s2", "s7"],
      "evidence_refs": ["s1", "s4"]
    }
  ],
  "connections": [
    {
      "from_group_key": "delivery",
      "to_group_key": "orders",
      "semantic_kind": "dispatches_work_to",
      "label": "dispatches orders",
      "summary": "Delivery boundaries hand validated orders to domain behavior",
      "evidence_refs": ["s2", "s5"]
    }
  ]
}
```

`key` is a response-local join key used by `connections`; it is not a
repository identity. Every connection endpoint must cite a key returned in
the same response.

`lane` is closed to exactly:

- `triggers`: all ways work begins. Both `inbound` delivery boundaries and
  `background_activity` such as cron, schedulers, consumers, workers, startup
  hooks, filesystem watchers, polling loops, controller reconciles, and CLI
  invocation belong in this one column;
- `core`: product/domain behavior, state, entities, and operations;
- `dependencies`: external packages, services, protocols, storage, and other
  outbound integrations.

During `grouping`, groups are a sparse overlapping cover, not a partition. A
categorized subject may belong to several useful groups or to none. Do not
emit an acknowledgement row for every `group_refs` entry, do not create an
`unassigned`/`support` complement, and do not force unrelated subjects
together. Every `member_ref` must itself carry a category compatible with the
group's lane: `inbound` or `background_activity` for `triggers`, `core` for
`core`, and `dependency` for `dependencies`. A multiply categorized subject
may therefore belong to groups in several compatible lanes. `evidence_refs`
may cite advertised subjects, including unclassified context, subject to the
platform/dependencies exception below. Evidence does not become membership and
does not make an incompatible member valid. This
sparse initial-selection rule does not authorize a `merge` response to retract
an already validated candidate membership.

For a `dependencies` group, do not cite an explicit `authority_kind:
platform` object or an exact invocation pattern whose complete targets are
platform authorities as evidence. Standard-runtime APIs are structural
context, not external dependency evidence. Other advertised local or
unclassified subjects may still provide evidence, and connection evidence is
not restricted by this group-lane rule.

`edges` are complete incident structural facts for every selectable subject in
this shard. They may describe ownership, containment, relation targets,
relation patterns, pattern targets, results, receivers, receiver origins, and
argument/value provenance. Dynamic arguments may retain reconstructed values
with request-local source object or source argument refs. A candidate with
`resolution: possible` remains only a possible value at that use; its exact
source provenance does not turn it into an exact runtime edge. Use these facts
as evidence, but do not invent a missing call, runtime occurrence, order, path,
framework meaning, or repository fact.

Connections are concise directed semantic relationships between returned
groups. They do not need a locally proven call corridor: the supplied exact
subjects, values, structural graph, categories, and group meanings are their
evidence. `semantic_kind` is an open snake_case vocabulary. Prefer precise
kinds such as `registers`, `invokes`, `dispatches_to`, `reads_from`,
`writes_to`, `publishes_to`, `consumes_from`, `schedules`, `configures`, or
`transforms_into`, but introduce a new precise snake_case kind when none fits.

During `merge`, `candidate_groups` and `candidate_connections` are validated
semantic proposals from earlier shards. This phase may consolidate groups; it
must not retract membership already selected by those shards. For every row in
`candidate_groups`, one returned group with the same `lane` must contain all of
that candidate's `member_refs`. One returned group may cover any number of
candidate rows, so do not emit an acknowledgement row for every candidate and
do not echo candidate `g*` refs. Preserve overlapping membership when the same
subject belongs to distinct useful candidates. A merge response that omits,
splits, or moves an accepted candidate membership to another lane is
incomplete and will be rejected as a whole.

Every candidate member is already individually compatible with its candidate
lane. Preserve that exact membership during `merge`, and require the same
per-member lane compatibility for every newly proposed membership. Never use
one compatible member to promote an incompatible member into the same lane.
A member may additionally appear in another group only when its own categories
also support that other lane; such duplication does not replace its required
membership in the original candidate's same-lane container.

Return the consolidated members and evidence using subject refs from
`group_refs`/`subjects`; candidate `g*` refs are context and must not appear in
`member_refs` or `evidence_refs`. Titles, summaries, evidence, connections, and
the number of groups may be reconsidered, but candidate lane/member sets are
immutable lower bounds. Before returning a `merge` response, check every
candidate row against the complete output: one same-lane output group must
contain its full member set. The initial `grouping` phase remains sparse; this
merge-only preservation rule does not require an initial group for every
categorized subject.

Return no confidence, scores, negative classifications, exhaustive coverage,
frontiers, paths, Markdown, extra fields, or prose outside the JSON object.
