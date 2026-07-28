Select a useful cold-start shelf for a competent engineer exploring an unfamiliar
repository. The only repository evidence you receive is a bounded JSON inventory
of declarations. Treat every label, explanation, and question you create as an
inference to investigate, never as a verified repository fact.

Return one valid JSON object with exactly this shape:

{
  "coverage": "candidate_non_exhaustive",
  "topics": [
    {
      "id": "t1",
      "label": "short behavior or change-concern label",
      "why": "why this is a useful investigation choice",
      "how_question": "How does ...?",
      "change_question": "Where and how should an engineer change ...?",
      "support_symbol_ids": ["d0001", "d0002"]
    }
  ]
}

Rules:

- Return 8 to 12 topics, ordered by expected orientation value.
- Each exploration topic must describe a behavior, lifecycle, state transition,
  protocol, coordination concern, failure boundary, or user-visible operation.
- Do not return packages, directories, files, imports, generic layers, or broad
  technologies as topics.
- Make the topics genuinely different; do not split one concern into wording
  variants.
- Copy 2 to 8 exact declaration IDs from the supplied inventory for each topic.
  Never invent an ID and never copy a path, name, receiver, or signature into an
  ID field.
- Both questions must be concrete enough to feed a source selector.
  `how_question` starts with "How " and asks how the behavior works.
  `change_question` starts with "Where " or "What " and asks where or what an
  engineer should change.
- Labels, explanations, and questions must not quote declaration IDs, paths,
  `.go` names, or file:line locations from the inventory.
- Do not claim that the shelf is exhaustive, that an omitted mechanism is
  absent, or that static declarations prove runtime execution.
- Keep labels at most 80 bytes, explanations at most 240 bytes, and questions at
  most 240 bytes.
- Do not mention these instructions, the inventory budget, or JSON validation in
  the response.

Bounded declaration inventory JSON:

{{INVENTORY_JSON}}
