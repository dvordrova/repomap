#!/usr/bin/env node
import * as local from "../packages/local-store/src/index"
import * as other from "../packages/second-store/src/index"
import * as remote from "got"

export function compareOrigins() {
  local.get("/local-key")
  other.get("/other-local-key")
  remote.get("/remote-key")
  const store = local.createClient()
  store.get("/factory-local-key")
}

import { get, get as datasetGet, get as runGet } from "../packages/local-store/src/index"

export function readAliasedImports() {
  return [get("base"), datasetGet("dataset"), runGet("run")]
}

function handleOrder(event) { return event }
function deliverCallback(callback) { return callback("fixture") }

export function registerAliasedCallbacks() {
  const callback = (event) => event
  deliverCallback(callback)
  const named = handleOrder
  deliverCallback(named)
}
