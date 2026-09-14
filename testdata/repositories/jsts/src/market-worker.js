function receiveMarketUpdate(event) {
  postMessage({received: event.data})
}

addEventListener("message", receiveMarketUpdate)

/** @param {import("./facade-exports/exchange").Exchange[]} exchanges */
function typedJSIteration(exchanges) {
  for (const exchange of exchanges) {
    exchange.resetStream()
  }
}
function unknownJSIteration(exchanges) {
  for (const exchange of exchanges) {
    exchange.resetStream()
  }
}

import { paintColor as READ_COLOR } from "../shared/contracts"
export function jsValueReferences() {
  return { READ_COLOR }
}
export function jsReadShadow(READ_COLOR) { return READ_COLOR }

/** @param {import("./facade-exports/exchange").Exchange} _ */
function typedJSUnderscore(_) { _.resetStream() }
function unknownJSUnderscore(_) { _.resetStream() }
