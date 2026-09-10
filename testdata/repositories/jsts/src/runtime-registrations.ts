export function recordWallet(): void {
  console.log("record wallet balances")
}

export function installWalletSchedule(): number {
  return setInterval(recordWallet, 60_000)
}

export function startMarketWorker(): Worker {
  return new Worker("./market-worker.js", {type: "module"})
}

export function boundedRetry(): void {
  for (let attempt = 0; attempt < 3; attempt++) recordWallet()
}

export function createOnce(): Promise<void> {
  return new Promise((resolve) => { recordWallet(); resolve() })
}

const localTimer = {setInterval(callback: () => void): void {callback()}}
localTimer.setInterval(boundedRetry)
