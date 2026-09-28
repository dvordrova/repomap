Consolidate a glossary of already accepted source-bound explanations.

Input `groups` contains closed `g*` refs. Each is one already accepted group;
its `variants` contain every original explanation, spelling
and source scope, each with a closed `v*` ref. A variant's `source_set` selects
one p* row from `source_sets`; that row's `sources` selects s* rows from `sources`,
each giving its exact repository path and line (zero means whole-file scope).
A p* row lists at most six sample observations and `count`, the real number of
observations behind that variant. These catalogues encode repeated sources once; follow the selected set for
every variant. Each request supplies its own complete catalogues, with no refs
to another window. Earlier groups are indivisible. Compare
their complete original evidence, not only their first or shortest member.

Join groups that name the same concept. The rule is:

- Groups with the same name were found in different places of the report.
  Join them when their explanations describe the same concept, even when
  their sources differ.
- Keep groups separate when their explanations describe different things,
  even when their names are equal.
- Groups with different names join only when their explanations show that
  the names are two spellings or aliases of the same concept.
- Sources tell where each explanation was observed. Different sources are
  never a reason to keep groups apart, and shared sources are never a reason
  to join them.

Return only JSON, with one assignment for EVERY input g ref, including groups that stay alone:
{"assignments":[{"ref":"g1","representative":"v1"},{"ref":"g2","representative":"v2"},{"ref":"g3","representative":"v2"}]}

Each assignment chooses one advertised v ref as its representative. Groups
that choose the same v ref are joined automatically. To keep a group separate,
choose one of its own variants. To join compatible groups, all of them must
choose the SAME variant, including the group that owns that variant. Never
create a cycle or a chain of different representatives.

Choose the original explanation that best covers the shared meaning; it will
be used verbatim. All original names, variants and sources are retained
automatically. Do not enumerate output group members: the assignments already
determine them. A group that needs no merge still needs its assignment.
Do not write explanations, names, source paths or new refs in the response.
There is no required number of groups and no incentive to make the glossary
smaller. Singleton groups are valid. This decision covers only the supplied
window; do not claim comparison with evidence outside it.

This consolidation is optional: if a window cannot be accepted, its input
entries remain separate and unchanged. A refused response supplies no grouping.
