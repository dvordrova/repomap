Evidence vocabulary. Every value below comes from an extractor, never from a model; a field absent from a row was not observed.
- `invocation`, how a call site is made: `synchronous` (default, left out); `goroutine` a Go `go` statement; `deferred` a Go
  `defer`. A prefixed form names the dynamic joint, then the invocation: `interface_invoke:` a call through an interface method
  with no statically known implementation; `declared_interface_dispatch:` a call of a method declared on an external interface,
  the declared method being the callee; `function_value_call:` a call of a function value (closure, variable or field);
  `callback_transfer:` a callable value passed into a named function or a declared interface method, transferred, not run;
  `callable_binding:field` a callable assigned to a struct field, not an execution. `dynamic`, `non_static` and `depth_bound`
  are unresolved sites counted in `detail`: a dynamic interface invoke, a non-static call, a call beyond the traversal depth.
- `resolution`: `exact` (default, left out) one known callee; `alternatives` several possible callees; `unresolved` none known.
- `kind` of a declaration: `function`, `method`, `type`, `variable`, `lambda`. Of a call: `calls` a repository declaration;
  `invokes_external` a symbol outside the indexed code; `executes` a process or command launch; `passes_callback`, `reads`,
  `writes`, `sources`, `decorates`, `implements` other observed relations, by name.
- Origins (`receiver_value`, `source_arguments[].origin`, `result_value`) are source expressions, not runtime values: a node
  has a `kind`, a `text` and at most two levels of `parts`. `literal` (text is the value); `parameter` (of the enclosing
  callable, by name); `receiver` (its method receiver); `field` (a field selection, parts[0] the value it is selected from,
  `initializer` an observed assignment when the instance cannot be followed); `call_result` (the result of another call);
  `record` (a composite literal of `field_value` nodes, each named by text, value in parts); `concat` (a string composed of
  its parts); `index` (container, then key); `alternatives` (several possible sources); `unknown` (not followed).
- `dispatch_observations` are the native views of one call site; `witnesses[].kind`: `go_ssa_dynamic_handoff` (Go SSA saw a
  callable or interface value cross to this site); `go_declared_interface_dispatch` (a Go call of a method declared on an
  external interface); `callable_receiver_field` (a literal field on the registering receiver object);
  `interface_field_assignment` (an observed assignment of a candidate implementation to the interface-typed field).
- `extractor` in `source_evidence.by_ref`: `control_context` (the label names the statement whose body holds the call:
  `for body`, `for body without condition`, `range body`, `range body over channel`, `select without default`,
  `select with default`); `callback_registration` (the call that received this callable, label `receiving call: <callee>`);
  `registration_receiver_call` (a call in the fluent chain producing that registration's receiver, with its literal
  arguments); `registration_result_use` (a later call on the registration's result); `callable_receiver_field` and
  `interface_field_assignment` as above. `evidence_refs` are the `e*` refs of `by_ref` supporting a call or binding.
- `callable_bindings`, where a callable is supplied or received: `from` supplies, `to` receives, `detail` names the receiving
  API or slot, `path`/`line` locate the binding, `arguments` are literal arguments of the receiving call (`position` or
  `keyword`; `kind` `literal_string`, or `string_template` with `{param}` for a hole; `value`). One binding is one object;
  several share a table: `columns` name the per-row values in order, `shared` the fields equal on every row, `rows` the rest.
- `has_repository_callee_candidate`: true when an observed callee candidate is indexed in this repository, absent otherwise;
  it does not settle the dispatch. `callers`: how many declarations call this one, a count. `local_calls`: exact repository
  callees of this declaration, one line each, `name@line`, a non-default invocation, and in parentheses the control
  statements holding the call; context, never selectable. A `$N` suffix, as in `serve$1`, marks the N-th function literal
  (closure) inside that declaration: its code, not a declaration of its own. `values` are literal arguments observed at a
  call, `arguments` repository declarations passed to it, `api` the exact external symbol, `detail` the extractor's note.
