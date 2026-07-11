def role_text($role): {
  handler_candidate: "a handler candidate",
  orchestrator_candidate: "an orchestrator candidate",
  adapter_candidate: "an adapter candidate",
  wrapper_candidate: "a wrapper candidate",
  delegator_candidate: "a delegator candidate",
  validator_candidate: "a validator candidate",
  mapper_candidate: "a mapper candidate",
  unknown: "an unknown structural role"
}[$role];

def hint_text($hint): {
  validation_name: "validation-related",
  delegation_name: "delegation-related",
  error_translation_name: "error-translation-related",
  response_enrichment_name: "response-enrichment-related",
  persistence_name: "persistence-related",
  io_name: "I/O-related",
  coordination_name: "coordination-related",
  other: "other"
}[$hint];

def unknown_text($kind): {
  target_body: "The target body and control flow have not been read.",
  branch_conditions: "The branch conditions in the target body are unknown.",
  call_order: "The runtime call order is unknown.",
  error_conditions: "The error conditions are unknown.",
  side_effects: "The side effects are unknown.",
  runtime_reachability: "Runtime reachability is not established by static calls.",
  tests: "Relevant executable tests have not been identified.",
  dynamic_calls: "Dynamic calls are not represented by this static bundle.",
  build_variants: "Behavior under other build variants is unknown."
}[$kind];

($facts[0]) as $f |
($result[0]) as $r |
($r.hypothesis.aliases | map(. as $id | $r.name_signals[] | select(.id == $id))) as $used |
($r.next_action) as $action |
{
  summary: {
    statement: ("Static facts identify " + $f.target.name + " as the selected target; its source behavior remains unverified."),
    evidence_ids: [$f.target.evidence_id],
    confidence: 0.5
  },
  responsibility: {
    statement: ("Static names suggest " + $f.target.name + " may have " + role_text($r.hypothesis.role) + " with " + ($used | map(hint_text(.hint)) | join(", ")) + " collaborators; this is only a navigation hypothesis."),
    evidence_ids: $r.hypothesis.evidence_ids,
    confidence: (if $r.hypothesis.confidence == "medium" then 0.5 else 0.3 end)
  },
  files_to_read_in_order: ([{
    path: $f.target.path,
    line: $f.target.line,
    structural_role: "target",
    evidence_ids: [$f.target.evidence_id]
  }] + [
    $used[] | {
      path: .path,
      line: .line,
      structural_role: "static_callee",
      evidence_ids: [.evidence_id]
    }
  ]),
  test_evidence_ids: [],
  unknowns: [unknown_text($r.unknown)],
  next_queries: [{
    query: (
      if $action.operation == "read_target" then "read target symbol " + $f.target.name
      elif $action.operation == "read_callee" then "read static callee selected by " + $action.anchor_evidence_id
      elif $action.operation == "find_tests" then "find _test.go references to " + $f.target.name
      else "expand static callers of " + $f.target.name end
    ),
    reason: (
      if $action.operation == "read_target" then "verify source control flow before explaining behavior"
      elif $action.operation == "read_callee" then "inspect a selected collaborator behind the name-level hypothesis"
      elif $action.operation == "find_tests" then "look for executable evidence"
      else "check whether more static entry paths change the navigation hypothesis" end
    )
  }],
  warnings: ["Static symbol names and call edges do not prove source behavior or runtime execution."]
}
