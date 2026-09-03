# Group categorized program elements

You receive one bounded shard of a language-neutral program graph from Go,
Python, JavaScript, or TypeScript. Use the same semantic contract for every
language and framework.

The request has one of two phases:

- `grouping`: form useful groups directly from categorized program subjects;
- `consolidate`: say which of the group candidates proposed by earlier shards
  are the same thing;
- `containers`: name the parts this target has, and put every candidate in
  one of them.

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

`inbound` and `background_activity` are categories, never lanes. A group whose
`lane` is any string other than `triggers`, `core` or `dependencies` is
discarded whole, and every connection naming it is discarded with it.

Size a group for reading, not for coverage. One group is one responsibility a
reader would name out loud, so a target of a few hundred subjects usually has
between eight and fifteen of them. No group should hold more than about a
fifth of the subjects in `group_refs`: a group that large is several
responsibilities that happen to share a directory, and splitting it by what
each part does is always the better answer. A group holding one subject is
fine when that subject is its own responsibility.

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
does not make an incompatible member valid. The `grouping` phase is sparse:
a subject with no group is a subject this shard had nothing to say about.

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

A `consolidate` request is a graph one level up from the subjects. Its nodes
are `candidates` — each one's title, summary, lane, how many members it holds,
and a few member names — and its edges are `connections` between them. It
carries no member refs, and your answer selects none.

Read it as a graph and not as a list of names. Candidates that constantly
reach each other are usually one thing however differently they are named;
candidates that share a word and never touch usually are not. Separate shards
of one target each saw a different part of it, so several of them describe the
same thing under different words — "Middleware", "Middleware common" and
"Middleware heartbeat" are one group.

The same request shape describes every level. Whatever you return becomes the
nodes of the next one, so a group's title has to read as the name of a part of
this repository, not as a bag: "Request logging" and not "Group 3".

Return strict JSON with exactly this shape:

```json
{
  "groups": [
    {
      "title": "Middleware",
      "summary": "Request-scoped wrappers the router composes around handlers",
      "lane": "core",
      "candidate_refs": ["c3", "c7", "c12"]
    }
  ]
}
```

A `containers` request has the same shape and asks a different question. Do
not ask again which candidates are the same thing — that was already settled
and the answer will not change. Ask which **part of this target** each one
belongs to. Basic authentication and response compression are not the same
thing and both are middleware; a router and its route tree are not the same
thing and both are the router. Name four to eight parts, each holding several
candidates, and use a name a reader of this repository would recognise.
A part holding one candidate is not a part.

Name every candidate exactly once, across all groups. In a `consolidate`
response a candidate that belongs with nothing else is a group of one, and
keeps its own title unless a better one covers it. Candidates in one group must share a `lane`: a lane follows
from a member's own categories and this phase may not move one. Aim for the
fewest groups a reader can still tell apart — a target reads well at four to
fourteen — but never gather things that are not the same thing merely to
reach a number.

Return no confidence, scores, negative classifications, exhaustive coverage,
frontiers, paths, Markdown, extra fields, or prose outside the JSON object.
