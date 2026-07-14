($facts[0]) as $f |
($triage[0]) as $t |
($hypothesis[0]) as $h |
($action_result[0]) as $a |
($actions[0][] | select(.id == $a.choice)) as $chosen |
{
  protocol_version: "local-symbol-v2",
  target: $f.target,
  name_signals: [
    $t.signals[] as $s |
    ($f.outgoing[] | select(.alias == $s.id)) as $fact |
    $s + {
      evidence_id: $fact.evidence_id,
      symbol: $fact.name,
      path: $fact.path,
      line: $fact.line
    }
  ],
  hypothesis: {
    role: $h.role,
    confidence: $h.confidence,
    aliases: $h.use,
    evidence_ids: [
      $h.use[] as $id |
      $f.outgoing[] | select(.alias == $id) | .evidence_id
    ]
  },
  unknown: $a.unknown,
  next_action: $chosen,
  model_free_text_fields: 0,
  limitation: "symbol names and static calls are navigation evidence, not source or runtime truth"
}
