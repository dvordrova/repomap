// Calls through a value of an interface this repository declares, as Go's
// RegisterTicketHandlerServer calls through its server interface: the
// targets are the methods of the repository's classes assignable to the
// interface, one exact, several alternatives (basis implements). TypeScript
// is structural, so MemoryStore implements Store without saying so; an
// abstract class is never the value, its subclass is, keeping the method it
// inherits; a generic class is not enumerated; ReadOnlyStore lacks put. An
// interface no class implements leaves its call unresolved.

export interface Store {
  put(key: string): void
  get(key: string): string
}

class DiskStore implements Store {
  put(key: string) { void key }
  get(key: string) { return key }
}

class MemoryStore {
  put(key: string) { void key }
  get(key: string) { return "" + key }
}

class ReadOnlyStore {
  get(key: string) { return key }
}

abstract class BaseStore implements Store {
  abstract put(key: string): void
  get(key: string) { return "base " + key }
}

class CachedStore extends BaseStore {
  put(key: string) { void key }
}

export function save(store: Store) {
  store.put("k")
  return store.get("k")
}

export class StoreService {
  constructor(private readonly store: Store) {}
  run() { return this.store.get("a") }
}

export interface Receipt {
  print(): string
}

class PaperReceipt implements Receipt {
  print() { return "paper" }
}

class TemplateReceipt<T> implements Receipt {
  constructor(private readonly value: T) {}
  print() { return String(this.value) }
}

export function printReceipt(receipt: Receipt) {
  return receipt.print()
}

export interface Ledger {
  record(entry: string): void
}

export function book(ledger: Ledger) {
  ledger.record("entry")
}

export const stores = [DiskStore, MemoryStore, ReadOnlyStore, CachedStore, PaperReceipt, TemplateReceipt]
