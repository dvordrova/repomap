# JavaScript and TypeScript native authority

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Package ownership, compiler and dependency authority

- The current JavaScript/TypeScript slice owns every eligible `package.json`
  project. Package-source ownership is assigned to the deepest containing
  manifest; every manifest with at least one owned tracked JavaScript/TypeScript
  source contributes one required target representative. A source-less
  manifest is tooling rather than target authority and cannot suppress sibling,
  child, or ancestor package targets. TargetPortfolio retains all exact package
  targets for full analysis and decides their roles and repository default; an explicit
  `jsts:<manifest>` narrows the typed plan to that one owned package before
  TypeScript compiler execution. `package.json#name` is optional: an exact
  top-level npm lockfile name is the secondary package identity, otherwise the
  root uses `root-package` and a nested package uses its repository-relative
  project directory. These fallbacks never authorize an implicit string-form
  `package.json#bin` command. Its corpus-only scout owns the exact
  manifest/source-ownership catalog, target identities, and package candidates
  without invoking Node or the TypeScript compiler; it participates in the same
  portfolio and typed execution plan as Go and Python. Each retained or
  explicitly selected package target receives its own page, which uses
  an available TypeScript Compiler API, preferring the project compiler, to honor
  `tsconfig.json` or `jsconfig.json`, repository-confined solution-style
  project references, aliases, and module resolution. A project reference that
  stays inside the owning package extends that page's complete compiler graph;
  an exact repository-local reference outside the package is a cross-target
  boundary and does not pull the sibling package into the page. Missing
  references and references outside the analyzed repository still fail closed.
  Exact owned manifest script inputs and supported tool-config files remain
  additional compiler roots. If a config selects no owned roots, those explicit
  sources use the compiler's existing inferred defaults; JavaScript inputs are
  enabled for that additional program. This does not admit other excluded files
  or the sibling sources selected by a documentation config.
  Repomap never installs npm, yarn, pnpm, or other packages. Browser and Node
  server surfaces plus
  canonical safe, package-owned, tracked `package.json#bin` command/path pairs
  are product surfaces inside their owning page-local ProgramTarget. A CLI
  surface never invents a bin-wrapper-to-source relation: its entry refs stay
  empty, while an exact canonical `dev` or `start` script with one
  helper-selected source may independently seed that source only after CLI
  product authority exists.
  Compiler/type-resolved declarations and exact external imports are the only
  call-target authority. Every ProgramIndex external symbol carries its exact
  raw package origin plus an adapter-derived `package` or `platform` authority
  kind. Go derives it from the complete build-selected `go list -deps`
  package-origin universe (including `DepOnly`) and its `Standard` bit; Python
  derives it from the exact `sys.stdlib_module_names` set; JS/TS maps
  TypeScript default-library and Node standard-library origins to `platform`
  and npm origins to `package`. Compiler-resolved sibling JS/TS packages
  also keep their repository directory on each import, call and external
  symbol, and in a `workspace` dependency row. Exact directory-scoped export
  identities join their own declarations across targets; a same-name npm
  package or another directory never supplies that identity. Workspace calls
  remain structural evidence but cannot be classified as known external
  libraries by their names. Report jumps use that exact directory; an
  unselected sibling gains no substitute destination. Missing or unknown authority fails closed;
  shared stages never infer it from a package-path prefix. Platform package objects are not dependencies or remote participants. A source call through a platform transport may independently establish runtime communication in model review, as specified in [ProgramIndex](PROGRAM_INDEX.md#typed-graph). Local callbacks or timers remain structural evidence rather than automatic workers or integrations. Calls and
  constructions retain their distinct exact invocation authority; an
  unresolved property name remains an unresolved frontier and is never matched
  to repository declarations by name alone.
  Local compiler candidates come from `typescript` or exact npm aliases to it
  declared by the selected manifest; a nested package inherits repository-root
  candidates only when it declares none itself. Each resolves from its declaring
  package scope. Without declarations, ordinary local `typescript` resolution
  still applies. If no usable local compiler exists, the helper uses an existing
  TypeScript package from the selected Node installation or the first `tsc` on
  `PATH`; it does not search other Node installations. The installed package
  must identify itself as `typescript` and expose a supported Compiler API.
  Distinct compatible local candidates in the selected API tier fail closed;
  one stable legacy API candidate is preferred over a native-preview candidate
  when both are deliberately declared. Compiler availability does not supply
  missing project dependencies or call-target authority.
  Shared contracts are supporting code, build/migration scripts remain tools,
  and a runtime script, library, or tool-only root must never promote itself
  into an application.

## Callable JSX and declaration headers

The JSTS result and helper preserve every compiler-observed callable
JSX attribute as an anchored callback binding, including render props. The
element and attribute are source facts; neither their names nor a framework
allowlist classify an operation. Wrapped function-valued declarations retain
compiler callable identity; inline anonymous callbacks and unindexed callable
factory results remain unresolved. A factory result never becomes a binding or
method call on the factory itself. Calls in ordinary local value initializers
belong to their enclosing callable. The symbol table and operation review
can interpret these observations as user `interaction`, alongside commands,
requests, scheduled and continuous work, using the same graph and review.

The JSTS helper also preserves written class/interface headers, including type
parameters and heritage, in the existing signature. Their compiler-owned body
boundary excludes member bodies while keeping braces inside generic types.
This native kind reaches the same ProgramIndex and question evidence; Python
already preserves its class header.

## Owned fields

Direct interface property declarations now enter the same native declaration
catalogue. Their written signatures preserve optional/readonly modifiers and
nested field types; original names, locations and owners remain exact. A nested
type-literal member is not lifted into the outer interface, and inherited
members do not acquire a declaration at the derived interface. These are
type-owned variables, not callable implementations. The existing atlas member
projection carries them into question evidence. This repairs the deterministic
loss that left IGetLevelsResponse without count before any question selection;
it does not establish that a later model answer selects or explains the field.

The JSTS helper retains the written class/interface declaration header in the
existing signature, including modifiers, type parameters and heritage. The
compiler's body token ends the header; object-type braces inside generic
parameters are retained, while member bodies are excluded. Previously the
compiler-rendered type at the declaration name erased the class/interface
distinction before model input. The raw declaration, ProgramIndex and question
evidence now retain that source distinction. Python already writes its class
header; the cumulative boundary check confirms that it reaches question rows.

## Native compiler compatibility

The helper adapts the native TypeScript 7 AST predicate names and scanner
signature/end token to the same extraction contract. The cumulative JSTS
type-member check runs against a prepared native `tsc` on PATH as well as the
existing legacy compiler check, retaining the original field signatures,
owners and question evidence. The native regression was reproduced and checked
with TypeScript 7.0.2 while preparing the ordinary Webernetes acceptance run.

## Framework-neutral registrations

Compiler-observed `setInterval`, worker construction, callbacks and native source expressions enter existing observations. Constructor/helper names do not prove a persistent responsibility. A periodic callback and a supported one-shot scheduled task remain distinct legitimate operation candidates; the initializer is not automatically that work. Explicit imports and compiler-resolved barrels retain original native identities; no export or target is inferred by name alone.
