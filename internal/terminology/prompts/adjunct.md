# Response envelope and repository terminology

The task instructions above describe only the computed value of result.
The final response_example supplies the single response shape, with terms
beside result. Write the task answer only inside result, never as a separate
JSON value before or after that object. Field restrictions in the task apply
inside result; term sources use only the separate catalogue below.

Within that same response, define unfamiliar terms needed to read the computed
result. Each name must occur verbatim in its prose; an input-only name
has no glossary authority. Preserve the task's distinction between observations
and interpretations. Use an empty terms array when no supported definition is
needed.

Prefer useful meanings of acronyms, domain terms, protocols, data formats and
technical concepts. Do not create a catalogue of repository names, paths,
declarations or UI components, or repeat native type explanations already
supplied in the task. A named code abstraction is useful only when its meaning
helps the reader understand the answer. Emit one entry per meaning and spelling,
combining its supporting sources. Repeat a spelling only for different meanings,
never once per input row or occurrence.

Each term contains exactly:
- name: the exact complete word or phrase in the computed answer, preserving
  its script and spelling.
- explanation: a short plain English definition in this source context.
- sources: a JSON array of supporting g* strings, e.g. ["g1", "g2"], from the
  final catalogue. A row-local source
  supports an occurrence in that same answer row; a shared source can support
  any answer row or the answer as a whole. Line 0 denotes a whole-file source.

The user suffix REPOMAP_TERMINOLOGY_CATALOG_V3 supplies the closed source refs
as sources and the final output contract. Do not invent meanings, paths or line
numbers, reconstruct omitted paths from trees, or generate occurrence pointers.
