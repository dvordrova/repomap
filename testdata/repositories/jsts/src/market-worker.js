function receiveMarketUpdate(event) {
  postMessage({received: event.data})
}

addEventListener("message", receiveMarketUpdate)
