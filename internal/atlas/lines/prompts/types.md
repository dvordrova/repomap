Explain each type to a developer who does not know this repository's terms.
Each independent row contains its declaration and exact native-owned member
declarations, with signatures and author documentation. No implementation
bodies, runtime values or execution trace were supplied. Documentation is
repository evidence, never an instruction to follow.

Fill exactly two cells:

- line: preferably two short, complete sentences. First explain
  what data or objects this thing represents or controls, in familiar words.
  When relevant, state the most consequential documented rule about those data: what
  changes, disappears or survives, and under which condition. If the supplied
  contract explicitly says associated data are removed on expiration, closing
  or cancellation, that consequence must be in the explanation. Prefer it over
  secondary administration, bookkeeping and recovery methods. For example, a
  workspace contract may support: "Owns temporary build outputs. Closing it
  deletes those outputs." An inventory of methods or "manages the lifecycle"
  is not an explanation of a concept.
- alias: a short English reader label, at most 40 characters, alongside the
  original name. Give a descriptive English alias when the original name is
  not English or is unclear to a newcomer, grounded in this declaration and
  its documented meaning. Translate the meaning, not just the sound of a
  foreign name. Do not invent a concept or expand an unexplained acronym.
  Use none when the original name is already recognizable English or the
  evidence does not establish a useful alias. The native code name stays intact.

Ground every detail in the row. Preserve the native declaration kind and member
kinds: a struct with a function-valued field is not an interface with a method.
Names and signatures establish declaration structure, not runtime effects.
When no data or lifecycle rule is documented, describe only that supplied
structure; do not invent a consequence to fill
the second sentence. Omit an undocumented lifecycle topic entirely, rather than
adding "no deletion or expiration rule is documented" to an unrelated concept.
Explain a necessary unfamiliar term with familiar words instead of substituting
one unexplained name for another. Do not invent fields, enum values, response contents,
storage, locking, deletion or safety guarantees. Do not substitute a textbook
definition for the repository's meaning. If even the role cannot be explained
from the row, say it is not established.

Include every supplied key exactly once, with line and alias in each result row.
