# Group categorized program elements

You receive one bounded shard of a language-neutral program graph from Go,
Python, JavaScript, or TypeScript. Use the same semantic contract for every
language and framework.

The request has one of two phases:

- `grouping`: form useful groups directly from categorized program subjects;
- `consolidate`: say which of the group candidates proposed by earlier shards
  are the same thing;
- `containers`: name the parts this target has, and put every candidate in
  one of them;
- `split`: say what parts one group is made of, placing every one of its
  members in exactly one part;
- `names`: give each part of a target a title.

In both phases, `group_refs` is the only closed set from which `member_refs`
may be selected. Every ref in `group_refs` has at least one positive category.
Other rows in `subjects` are incident structural context or evidence only. In
particular, a subject with `categories: []` must never be selected as a group
member. Short refs exist only inside this request; never copy or invent a
canonical identity.

Answer with one line per selectable ref and nothing else:

```json
{
  "assign": [
    {"ref": "s2", "group": "Order delivery"},
    {"ref": "s7", "group": "Order delivery"},
    {"ref": "s4", "group": "Order storage"}
  ],
  "links": [
    {"from": "Order delivery", "to": "Order storage", "label": "stores accepted orders"}
  ]
}
```

`group` is the name of the group the ref belongs to — a responsibility a
reader would say out loud, written the same way every time it appears. Refs
carrying the same name are one group. Every ref in `group_refs` appears
exactly once: none left out, none repeated, and no ref that is not in
`group_refs`.

`links` says what one group does to another, naming the groups by the same
names used in `assign`. A group never links to itself, one pair of groups is
linked at most once, and a group links to at most three others: a list that
repeats itself is wrong however long it is.

Do not return lanes, member lists, evidence, summaries, keys, confidence or
prose. A group's lane and its evidence follow from the categories of the refs
you assign to it, and are decided here, not by you.

Every ref is placed in exactly one group, so a group's lane is decided by the
categories of what it holds: `inbound` or `background_activity` refs make a
triggers group, `core` refs a core group, `dependency` refs a dependencies
group. Refs of different lanes under one name simply become one group per
lane. You never write a lane.

Size a group for reading, not for coverage. One group is one responsibility a
reader would name out loud. `selectable` says how many refs this request has
to work with, and the answer is measured against that number and nothing
else: return at least `selectable`/20 groups and at most `selectable`/4.
For a request carrying 64 refs that is between 3 and 16 groups. No group may
hold more than a fifth of `selectable`: a group that large is several
responsibilities that happen to share a directory. A group holding one
subject is fine when that subject is its own responsibility.

`grouping` is a partition, not a sample: every ref in `group_refs` gets a
group, including the ones that look like plumbing. A ref you cannot place with
anything else is its own group of one — that is a real answer, and leaving it
out is not. Do not invent a catch-all "misc" or "support" group to park refs
you did not think about; name what they actually do.

`edges` are complete incident structural facts for every selectable subject in
this shard. They may describe ownership, containment, relation targets,
relation patterns, pattern targets, results, receivers, receiver origins, and
argument/value provenance. Dynamic arguments may retain reconstructed values
with request-local source object or source argument refs. A candidate with
`resolution: possible` remains only a possible value at that use; its exact
source provenance does not turn it into an exact runtime edge. Use these facts
as evidence for which refs belong together, but do not invent a missing call,
runtime occurrence, order, path, framework meaning, or repository fact.

A link is a concise directed relationship between two groups you named. It
does not need a locally proven call corridor: the supplied exact subjects,
values, structural graph and categories are its evidence. Its `label` says
what the first group does to the second in a few words — "registers routes",
"reads settings", "dispatches work" — and nothing more.
A `consolidate` request is a graph one level up from the subjects. Its nodes
are `candidates` — each one's title, summary, lane, how many members it holds,
and a few member names — and its edges are `connections` between them.

Read it as a graph and not as a list of names. Candidates that constantly
reach each other are usually one thing however differently they are named;
candidates that share a word and never touch usually are not. Separate shards
of one target each saw a different part of it, so several of them describe the
same thing under different words — "Middleware", "Middleware common" and
"Middleware heartbeat" are one group.

Answer with one line per candidate and nothing else:

```json
{
  "assign": [
    {"ref": "c1", "cluster": "middleware"},
    {"ref": "c2", "cluster": "middleware"},
    {"ref": "c3", "cluster": "router"}
  ]
}
```

`labels` in the request says how many distinct labels to answer with, and it
is counted from this request and not from the target: a request of forty
candidates asks for about twenty. Coming back with one label per candidate
joins nothing and coming back with a handful gathers things that are not the
same thing, and both are wrong however well the labels read.

`cluster` is a short lowercase label you choose. Candidates carrying the same
label are the same thing; a candidate that belongs with nothing else gets a
label of its own. Every `ref` in `candidates` appears exactly once, none is
left out and none is repeated. Do not return titles, summaries, lanes,
members, connections or prose — they are decided elsewhere from what you
assign here.

A `containers` request has the same shape and asks a different question, and
it is not open: `parts` lists the areas of this target, already named. Put
every candidate in the one it belongs to, spelling the name exactly as
`parts` spells it. Basic authentication and response compression are not the
same thing and both are middleware; a router and its route tree are not the
same thing and both are the router. A candidate that belongs in none of the
named parts is left out of the answer rather than given a part of its own —
a name that is not in `parts` is dropped.

Name every candidate exactly once, across all groups. In a `consolidate`
response a candidate that belongs with nothing else is a group of one, and
keeps its own title unless a better one covers it. Candidates in one group must share a `lane`: a lane follows
from a member's own categories and this phase may not move one. Aim for the fewest
groups a reader can still tell apart, up to the `labels` the request asks
for, but never gather things that are not the same thing merely to reach a
number.

Return no confidence, scores, negative classifications, exhaustive coverage,
frontiers, paths, Markdown, extra fields, or prose outside the JSON object.
