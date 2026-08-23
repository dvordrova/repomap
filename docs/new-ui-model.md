# New report UI model

The report is a repository-orientation workspace, not a browser for pipeline
artifacts. Its first vertical slice follows one interaction:

`Survey -> Choose -> Focus -> Verify`

## User questions

- What is this target responsible for?
- Which responsibility should I explore first?
- What important code and entrypoints participate in it?
- How does it connect to another responsibility?
- Where is the exact source evidence?

## First screen

The repository identity and target kind establish scope. The primary survey is
the refined CoreMap responsibility set: names and purposes are the semantic
orientation, while counts and producer coverage are not navigation.

A focused system canvas projects three existing semantic layers:
`entrypoints -> core responsibilities -> integrations`. An entrypoint connects
to a responsibility only when its exact object ID is a representative member.
An integration connects only when a selected use's exact caller ID is such a
member; its edge preserves either exact external-symbol authority or explicitly
unresolved runtime dispatch. Unbound selected rows are counted at the frontier
rather than connected heuristically. Hover and keyboard-focus details expose
purpose, signature, relation authority, and exact source actions.

Choosing a responsibility opens one focused explanation in place. Focus joins
that responsibility to exact representative symbols, matching selected
activity entrypoints, and exact ProgramView relations. Evidence is a separate
rail of source-linked symbols and files. Material unresolved dispatch is shown
beside the affected explanation; general producer diagnostics stay secondary.

The MVP intentionally omits the raw object/relation catalog, per-artifact
sections, coverage dashboards, and a generic graph of every ProgramIndex
object. The canvas is a bounded semantic projection, not another graph browser.

## Useful knowledge retained from the old UI

- exact source opening for served and standalone reports;
- switching published targets without mixing their semantic authority;
- distinct concepts for responsibilities, activity starts, integrations, and
  evidence;
- explicit uncertainty when it changes how a connection should be read.

The old overview/map duplication, dataset-by-dataset navigation, raw
ProgramIndex explorer, and browser-side revalidation of the complete backend
contract are discarded. The generated report is already validated before
publication; the new client validates only the small contract it consumes.

## Product / Pipeline Gaps

### End-to-end mechanisms and workflows

Desired capability: show how one concrete repository behavior unfolds across
registration, dispatch, state changes, and effects.

Why useful: a newcomer needs causal explanations, not only important objects.

Current pipeline: `ActivityPath` is limited to selected activity-to-integration
routes and may legitimately be empty for a library such as Chi.

Old UI: showed some paths, but not a general mechanism model.

MVP decision: do not infer a substitute. Show exact local connections and keep
the missing mechanism explicit for later pipeline work.

### Repository-level purpose

Desired capability: one grounded explanation of what the repository as a
whole is responsible for.

Why useful: this is the first orientation question.

Current pipeline: CoreMap provides responsibility names and purposes but no
single validated repository thesis.

Old UI: approximated this with project guesses and summaries.

MVP decision: summarize the responsibility set without inventing a new claim.

### One component in several behaviors

Desired capability: reveal when the same important symbol participates in
several distinct responsibilities or workflows.

Why useful: shared components are often architectural joints.

Current pipeline: repeated exact symbol membership is available; behavioral
roles and mechanism membership are only partial.

Old UI: partial support through associations and graph navigation.

MVP decision: expose repeated membership and exact relations where present;
defer behavioral role labels.

### Tests as evidence

Desired capability: lead from an explanation to tests demonstrating intended
behavior.

Why useful: tests are often the safest next reading step.

Current pipeline: tests are not a first-class semantic witness in the report.

Old UI: no reliable first-class equivalent.

MVP decision: record the gap; do not guess tests from filenames.
