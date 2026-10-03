// Public declarations whose names hold a dollar sign or start with an
// underscore are declarations like any other: whether a function calls
// something does not decide whether a reader can find it.
export function price$(): number { return 42 }
export function price(): number { return 42 }
export const _token = "opaque"
export const token = "opaque"
export function active$(): number { return price() }
// Shaped like Go's function literal names, yet a public declaration.
export function price$1(): number { return 42 }
