Explain unfamiliar concepts that are needed to read the supplied, already
accepted prose. This is optional glossary work, not another analysis of
repository code. The original answers are settled and must not be reproduced
or rewritten.

Each prose row has one p ref and exact accepted text. Define only a complete
name that occurs verbatim in a selected row's text. Select those prose rows;
their original source attribution is retained locally. No unseen file contents
are supplied. Do not invent details absent from the prose.

The reader is an engineer new to this repository. Skip general engineering,
computing and version-control vocabulary an engineer already knows, such as
repository, commit, package, interface, cache, JSON or HTTP; explain only terms
of this repository's own field or ones an engineer would have to look up.

Explain domain and concept terms only. Do not define names that the code itself
declares or reads: functions, methods, types, classes, variables, constants,
packages, modules, files, paths, environment variables, configuration keys,
command-line flags, headers, commands, rule or ticket codes. The reader opens
the source for those; they are not glossary entries. A word, acronym, protocol
or format that code also uses as a name, such as order, candle or ROI, is still
a concept to explain; skip only a code spelling such as OrderBook, max_retries
or config/app.yaml.

Each definition has name, kind, explanation and rows. Keep the original
spelling and script of name, give a short plain-English contextual definition,
select supporting p refs in rows, and choose exactly one kind:

- acronym: an abbreviation or initialism that stands for a longer name.
- domain: a business, product or scientific concept of the repository's field.
- protocol: a communication or interaction protocol, standard or convention between systems.
- format: a data, file, message or serialization format.

Use an empty terms array when no unfamiliar concept needs explaining.

Emit one definition per meaning and spelling, combining supporting prose rows.
Never repeat a definition once per row or occurrence. Distinct meanings of the
same spelling remain separate. Unsupported terms should be omitted; do not add
a second response object, a restatement of the prose or another glossary pass.
