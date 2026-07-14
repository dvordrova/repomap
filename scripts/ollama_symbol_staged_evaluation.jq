($facts[0]) as $f |
($result[0]) as $r |
($f.target.name | split(".") | last | ascii_downcase) as $target_operation |

def signal_priority($hint): {
  validation_name: 0,
  delegation_name: 1,
  error_translation_name: 2,
  response_enrichment_name: 3,
  persistence_name: 4,
  io_name: 5,
  coordination_name: 6,
  other: 7
}[$hint];

def source_priority($source):
  if $source == "deterministic_name_rule" then 0
  elif $source == "model" then 1
  else 2 end;

([
  $f.outgoing[] |
  (.name | ascii_downcase) as $name |
  (if ($name | test("check|validat")) then "validation_name"
   elif ($name | test("error")) then "error_translation_name"
   elif $name == $target_operation then "delegation_name"
   elif ($name | test("fill|header|enrich")) then "response_enrichment_name"
   elif ($name | test("^save|store|persist|commit")) then "persistence_name"
   elif ($name | test("sync|flush|fsync|read|write")) then "io_name"
   else null end) as $expected |
  select($expected != null) |
  (.evidence_id) as $evidence_id |
  ([$r.name_signals[] | select(.evidence_id == $evidence_id) | .hint][0] // "missing") as $actual |
  {
    name: ("name_hint:" + $evidence_id),
    passed: ($actual == $expected),
    expected: $expected,
    actual: $actual
  }
] + [
  ($r.name_signals | sort_by([source_priority(.source), signal_priority(.hint)]) | .[:3] | map(.id) | sort) as $expected |
  ($r.hypothesis.aliases | sort) as $actual |
  {
    name: "hypothesis_uses_prioritized_signals",
    passed: ($actual == $expected),
    expected: ($expected | join(",")),
    actual: ($actual | join(","))
  },
  {
    name: "hypothesis_uses_distinct_signals",
    passed: (($r.hypothesis.aliases | unique | length) == ($r.hypothesis.aliases | length)),
    expected: "distinct aliases",
    actual: ($r.hypothesis.aliases | join(","))
  }
] + [{
  name: "hypothesis_role_matches_capability",
  passed: ($hypothesis_schema[0].properties.role.enum | index($r.hypothesis.role) != null),
  expected: ($hypothesis_schema[0].properties.role.enum | join(",")),
  actual: $r.hypothesis.role
}, {
  name: "no_model_free_text",
  passed: ($r.model_free_text_fields == 0),
  expected: 0,
  actual: $r.model_free_text_fields
}, {
  name: "next_action_reads_missing_target_body",
  passed: ($r.unknown == "target_body" and $r.next_action.operation == "read_target"),
  expected: "target_body -> read_target",
  actual: ($r.unknown + " -> " + $r.next_action.operation)
}]) as $checks |
{
  version: 1,
  scope: "constrained name classification and action quality; not source or runtime truth",
  passed: ($checks | map(select(.passed)) | length),
  total: ($checks | length),
  checks: $checks
}
