import test from "node:test"
import assert from "node:assert/strict"
import { layout } from "./layout.mjs"

test("places groups in one row", () => {
  assert.deepEqual(layout(["api", "store"]).map((box) => box.x), [0, 200])
})
