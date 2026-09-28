# The term question's options

Each option of the question "For a newcomer's glossary of this program, is
`candidate` a domain concept, general vocabulary or a code element?" with its
criteria: what it is, what it includes, what it is not for, and examples. The
request sends them as each option's criteria. Only a domain concept goes into
the glossary. The options differ in where a reader finds what the name means.

## domain_concept

What: A concept of the program's field, or of a technology the program is built around, that the reader must know to follow the report and would look up in that field's or that technology's documentation.

Includes: terms of the program's business, science or product field; the concepts, operations, structures and formats of a database, storage system, protocol or standard that the program is built around; acronyms of the field; such a concept even when the program also names one of its types, fields, settings or commands after it

Not for: vocabulary that any engineer already knows; a phrase whose meaning is its words put together, such as a concept's size, frequency, statistics, operations, tests or failures, when the concept alone is the term; a name whose meaning only this program's own source or report gives

Examples:
- chargeback, in a payments program
- settlement window, in a payments program, even when one of its settings types has a settlement window field
- variant call, in a genome pipeline
- bill of lading, in a shipping program

## general_vocabulary

What: A word or phrase that an engineer already knows, whatever the program: general programming, computing, network, cloud, tooling or everyday vocabulary, or a phrase whose meaning is its known words put together.

Includes: programming and computing words; well-known protocols, formats, services, platforms and tools, and their common settings; operating-system, network, build, packaging, monitoring and testing words; a concept's name combined with an ordinary word, such as its size, growth, frequency, statistics, operations, tests or failures

Not for: a concept of the program's field, or of a technology it is built around, that an engineer would have to look up

Examples:
- function, interface, cache, logger, checksum
- HTTP, JSON, TLS, SSH
- command-line interface, configuration file, subcommand, integration test
- settlement frequency and chargeback statistics, when settlement and chargeback are the program's concepts

## code_element

What: A name whose meaning only this program's own source or report gives: a name the program made for one of its own components, types, functions, variables, settings, flags, commands, files, tests or paths, or a title or phrase of the report that describes what the program or one of its parts does.

Includes: a component or type of this program named in plain words; a count, limit, switch or path among its own settings; a command, subcommand or flag of this program; a test or scenario of this program; a heading or title of the report; a paraphrase of a longer phrase of the report

Not for: a concept of the program's field, or of a technology it is built around, that its documentation defines, even when the program names one of its types, fields, settings or commands after it; vocabulary that any engineer already knows

Examples:
- worker count, a setting of the program
- the payment client, one of the program's types described in words
- verbose flag, an option of the program's command line
- nightly export job, the title of one of the program's parts
