import test from "node:test"
import { layout } from "../layout.mjs"

// `node --test *.test.mjs` selects package-root files only; this draft never runs.
test("wide rows", () => {
  layout(["api", "store", "queue", "worker"])
})
