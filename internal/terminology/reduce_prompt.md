Consolidate a glossary of already accepted source-bound explanations.

Input `groups` contains closed `g*` refs. Each is one already accepted group;
its `variants` contain every original explanation, spelling
and source, each with a closed `v*` ref. Earlier groups are indivisible. Compare
their complete original evidence, not only their first or shortest member.

Join groups only when their meanings and source scope agree. Equal spellings
alone do not establish equal meanings. Different spellings may be aliases only
when the supplied explanations and sources establish the same meaning. Keep
uncertain or different senses separate.

Return only JSON:
{"groups":[{"members":["g1","g2"],"representative":"v1"}]}

Every input group belongs to exactly one output group. `representative` selects
one original variant from that group's members. Its existing explanation will
be used verbatim; choose the explanation that best covers the shared meaning.
All original names, variants and sources are retained automatically.
Do not write explanations, names, source paths or new refs in the response.
There is no required number of groups and no incentive to make the glossary
smaller. Singleton groups are valid. This decision covers only the supplied
window; do not claim comparison with evidence outside it.

This consolidation is optional: if a window cannot be accepted, its input
entries remain separate and unchanged. A refused response supplies no grouping.
