def location($entity):
  ($entity.location.path + ":" + ($entity.location.line | tostring));

def entity($entity):
  ($entity.name + " [" + $entity.kind + "] @ " + location($entity));

def incoming:
  if (.incoming_calls | length) == 0 then
    ["(none)"]
  else
    [.incoming_calls[] |
      (.evidence_id + " | " + entity(.caller) + " -> TARGET | callsite=" +
       (.callsite.path + ":" + (.callsite.line | tostring)))]
  end;

def outgoing:
  if (.outgoing_calls | length) == 0 then
    ["(none)"]
  else
    [.outgoing_calls[] |
      (.evidence_id + " | TARGET -> " + entity(.callee) + " | callsite=" +
       (.callsite.path + ":" + (.callsite.line | tostring)))]
  end;

[
  "Analyze one selected Go symbol using only the static facts below.",
  "Names and static calls suggest behavior but do not prove runtime execution or source semantics.",
  "Every interpretation must cite listed evidence IDs. Never invent paths, tests, behavior, or IDs.",
  "",
  ("TARGET: " + .target.evidence_id + " | " + entity(.target.entity)),
  "",
  "INCOMING STATIC CALLS:",
  (incoming[]),
  "",
  "OUTGOING STATIC CALLS:",
  (outgoing[]),
  "",
  "Return only KEY: VALUE lines. Do not return JSON, Markdown, code, or headings.",
  "Both SUMMARY and RESPONSIBILITY must explicitly say likely, suggests, inferred, or based on static facts.",
  "READ values are evidence IDs. Do not emit TEST because no test evidence is supplied.",
  "UNKNOWN must name a concrete missing fact. NEXT_QUERY must contain a real local query and reason separated by ||.",
  "",
  "SUMMARY: ...",
  "SUMMARY_EVIDENCE: comma-separated evidence IDs",
  "SUMMARY_CONFIDENCE: number from 0 to 0.75",
  "RESPONSIBILITY: ...",
  "RESPONSIBILITY_EVIDENCE: comma-separated evidence IDs",
  "RESPONSIBILITY_CONFIDENCE: number from 0 to 0.75",
  "READ: one evidence ID",
  "UNKNOWN: one missing fact",
  "NEXT_QUERY: local query || why it reduces uncertainty",
  "WARNING: one limitation"
] | join("\n")
