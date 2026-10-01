Explain each type to a developer who does not know this repository's terms.
Each independent row contains its declaration and exact native-owned member
declarations, with signatures. No implementation bodies, runtime values or
execution trace were supplied.

Fill the cells advertised by fill:

- line: preferably two short, complete sentences. First explain
  what data or objects this thing represents or controls, in familiar words.
  When its members' names and signatures show the most consequential rule
  about those data (what changes, disappears or survives), state it, preferring
  it over secondary administration, bookkeeping and recovery methods. An
  inventory of methods or "manages the lifecycle" is not an explanation of a
  concept.
- alias: asked only for a name that is not written in Latin letters: a short
  English reader label, at most 40 characters, alongside the original name,
  grounded in this declaration. Translate the
  meaning, not just the sound of the name. Do not invent a concept or expand
  an unexplained acronym. Use none when the evidence does not establish a
  useful alias. The native code name stays intact.

Ground every detail in the row. Preserve the native declaration kind and member
kinds: a struct with a function-valued field is not an interface with a method.
Names and signatures establish declaration structure, not runtime effects.
When the members show no data or lifecycle rule, describe only that supplied
structure; do not invent a consequence to fill the second sentence. Omit a
lifecycle topic the members do not show entirely, rather than adding "no
deletion or expiration rule is shown" to an unrelated concept.
Explain a necessary unfamiliar term with familiar words instead of substituting
one unexplained name for another. Do not invent fields, enum values, response contents,
storage, locking, deletion or safety guarantees. Do not substitute a textbook
definition for the repository's meaning. If even the role cannot be explained
from the row, say it is not established.

The result rows contain every supplied key exactly once and only the columns
advertised by fill.
