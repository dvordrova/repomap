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
