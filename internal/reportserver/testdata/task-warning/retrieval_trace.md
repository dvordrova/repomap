# Task Lens retrieval trace

- Version: `1`
- Task kind: `bug`
- Task profile: `unknown`
- Gold assessment: not applied

## Extracted task terms

| Term | Normalized | Found | Weight |
| --- | --- | --- | ---: |
| failure | failure | false | 8 |
| path | path | false | 2 |
| reported | reported | false | 8 |
| reported failure | reported failure | false | 16 |
| request | request | false | 4 |
| sees | sees | false | 2 |
| Validate | validate | true | 16 |

## Candidates before ranking

| Order | Stage | Candidate | Path / symbol | Roles | Score | Components |
| ---: | --- | --- | --- | --- | ---: | --- |
| 1 | initial | anchor-2e8bd13976e95aab1a42 | internal/api.go / Handle | representative_implementation | 0 | direct_task_term_match=+10 (sum of exact retained task-term hits); distance_from_selected_anchor=+0 (no repository-wide graph distance was computed); duplicate_adjacent_only_penalty=+0; exact_relation_to_selected_anchor=+0 (relations are computed after retained anchor identity is fixed); example_test_only_penalty=+0; missing_role_fit=+0 (fit to immutable key-role reservations); production_relevance=+20; repository_role=-50 (residual repository/path score; components sum to the exact pre-ranking score); source_scope_completeness=+20; test_fixture_relevance=+0 |
| 2 | initial | anchor-cd6ed011bcd4e5333200 | internal/service.go / Validate | symptom_site | 0 | direct_task_term_match=+10 (sum of exact retained task-term hits); distance_from_selected_anchor=+0 (no repository-wide graph distance was computed); duplicate_adjacent_only_penalty=+0; exact_relation_to_selected_anchor=+0 (relations are computed after retained anchor identity is fixed); example_test_only_penalty=+0; missing_role_fit=+0 (fit to immutable key-role reservations); production_relevance=+20; repository_role=-50 (residual repository/path score; components sum to the exact pre-ranking score); source_scope_completeness=+20; test_fixture_relevance=+0 |
| 3 | initial | anchor-dbf6609b644e97fdf1f5 | internal/service_test.go / TestValidate | verification_anchor | 0 | direct_task_term_match=+10 (sum of exact retained task-term hits); distance_from_selected_anchor=+0 (no repository-wide graph distance was computed); duplicate_adjacent_only_penalty=+0; exact_relation_to_selected_anchor=+0 (relations are computed after retained anchor identity is fixed); example_test_only_penalty=+0; missing_role_fit=+0 (fit to immutable key-role reservations); production_relevance=+0; repository_role=-50 (residual repository/path score; components sum to the exact pre-ranking score); source_scope_completeness=+20; test_fixture_relevance=+20 |

## Exact local relationships

| ID | Left → right | Kind | Support | Evidence | Scope / non-guarantees |
| --- | --- | --- | --- | --- | --- |
| relation-1ab78752d33f6af5ab01 | anchor-2e8bd13976e95aab1a42 → anchor-cd6ed011bcd4e5333200 | direct_call | locally_observed | evidence-3a68ae597cd4d4218f64, evidence-a9d5ac2ef6adb4921d74 | Exact retained anchor scopes only. / A direct call expression is present in the retained caller excerpt; this does not prove runtime reachability, order, or callee behavior. |
| relation-3e503c275742ad94a86a | anchor-dbf6609b644e97fdf1f5 → anchor-cd6ed011bcd4e5333200 | direct_call | locally_observed | evidence-73de9feec3f18ff427e8, evidence-a9d5ac2ef6adb4921d74 | Exact retained anchor scopes only. / A direct call expression is present in the retained caller excerpt; this does not prove runtime reachability, order, or callee behavior. |
| relation-db0a568c6bec544bc2cf | anchor-2e8bd13976e95aab1a42 → anchor-dbf6609b644e97fdf1f5 | shared_state_alias | locally_observed | evidence-3a68ae597cd4d4218f64, evidence-73de9feec3f18ff427e8 | Exact retained anchor scopes only. / Exact retained source references only; this does not prove runtime order, reachability, or causality. |

## Selected anchors

| Rank | Candidate | Anchor | Reason |
| ---: | --- | --- | --- |
| 1 | anchor-2e8bd13976e95aab1a42 | anchor-2e8bd13976e95aab1a42 | reserved after key roles for supporting role representative_implementation |
| 2 | anchor-cd6ed011bcd4e5333200 | anchor-cd6ed011bcd4e5333200 | retained by bounded exact-term score and file diversity |
| 3 | anchor-dbf6609b644e97fdf1f5 | anchor-dbf6609b644e97fdf1f5 | retained by bounded exact-term score and file diversity |

## Dropped candidates

None.

## Source-scope completeness

| Anchor | Kind | Lines | Truncated | Match outside | Negative claims | Basis | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- |
| anchor-2e8bd13976e95aab1a42 | complete_enclosing_symbol | 10-12 / 32 | false | false | true | complete_scope |  |
| anchor-cd6ed011bcd4e5333200 | complete_enclosing_symbol | 20-22 / 32 | false | false | true | complete_scope |  |
| anchor-dbf6609b644e97fdf1f5 | complete_enclosing_symbol | 30-32 / 32 | false | false | true | complete_scope |  |

## Role coverage

| Tier | Role | Required | Anchors | Represented |
| --- | --- | ---: | --- | --- |
| supporting | representative_implementation | 1 | anchor-2e8bd13976e95aab1a42 | true |
| optional | documentation_contract | 1 |  | false |
| optional | verification_anchor | 1 | anchor-dbf6609b644e97fdf1f5 | true |

## Verification frontier

Decisive anchor: `anchor-2e8bd13976e95aab1a42`.

| Slot | ID | Authority | Anchor | Path / symbol | Evidence | Text |
| --- | --- | --- | --- | --- | --- | --- |
| anchor | verification-75f20b065992b8a94d7a | exact_existing_test | anchor-dbf6609b644e97fdf1f5 | internal/service_test.go / TestValidate | evidence-73de9feec3f18ff427e8 | Exact retained test or example calls a decisive endpoint or asserts task-observable concepts; this does not establish coverage of every task case. |

## Budget consumption

| Budget | Consumed | Bound event |
| --- | ---: | --- |
| initial_candidates | 7 | false |
| retained_anchors | 3 | false |
| read_files | 3 | false |
| read_bytes | 256 | false |
| source_scan_bytes | 0 | false |
| retained_source_bytes | 120 | false |
| gopls_queries | 1 | false |
| frontier_expansions | 1 | false |
| local_wall_millis | 42 | false |

## Limit-caused loss

| Limit | Bound | Observed | Applied | Caused loss | Exact reason |
| --- | ---: | ---: | --- | --- | --- |
| completion_expansions | 2 | 1 | false | false |  |
| initial_candidates | 40 | 7 | false | false |  |
| read_bytes | 131072 | 256 | false | false |  |
| read_files | 12 | 3 | false | false |  |
| retained_anchors | 16 | 3 | false | false |  |
| retained_source_bytes | 131072 | 120 | false | false |  |
| source_scan_bytes | 4194304 | 0 | false | false |  |
