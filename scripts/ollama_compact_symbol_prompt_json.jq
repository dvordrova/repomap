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
  "Both interpretation statements must explicitly say likely, suggests, inferred, or based on static facts.",
  "Choose at most four reading evidence IDs. No test evidence is supplied, so test_evidence_ids must be empty.",
  "Return exactly one concrete missing fact, one local query that would reduce it, and at most one warning.",
  "",
  ("TARGET: " + .target.evidence_id + " | " + entity(.target.entity)),
  "",
  "INCOMING STATIC CALLS:",
  (incoming[]),
  "",
  "OUTGOING STATIC CALLS:",
  (outgoing[])
] | join("\n")
