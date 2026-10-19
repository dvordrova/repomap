// Calls on values the compiler types `any`, the way bee-queue's Queue calls
// job.computeDelay() on the job its _finishJob is handed: Python's
// duck_receivers.py in JavaScript. Queue keeps a storage its storageClass
// builds and uses enqueue, dequeue and size on it, so put calls each
// storage's enqueue; finish uses computeDelay and toData on its job, which
// only Job has. What the code names no more than the method it calls, a
// value whose shape the code tests, a caller handing an object literal and
// a class whose member of the called name is a number leave their calls
// unresolved. A subclass's use narrows only its own calls.

class Storage {
  enqueue(item) { throw new Error("abstract " + item) }
  dequeue() { throw new Error("abstract") }
  size() { return 0 }
}

class MemoryStorage extends Storage {
  constructor() { super(); this.items = [] }
  enqueue(item) { this.items.push(item) }
  dequeue() { return this.items.shift() }
}

class FileStorage extends Storage {
  constructor() { super(); this.path = "queue.txt" }
  enqueue(item) { return this.path + item }
  dequeue() { return this.path }
}

export class Queue {
  constructor(storageClass) {
    this.storageClass = storageClass
    this.storage = this.createStorage()
  }
  createStorage() { return new this.storageClass() }
  put(item) { return this.storage.enqueue(item) }
  take() { return this.storage.dequeue() }
  pending() { return this.storage.size() }
}

export class FileQueue extends Queue {
  // Only FileStorage has a path: this call is its enqueue alone.
  flush() { return this.storage.enqueue(this.storage.path) }
}

export class Consumer {
  constructor(queue) { this.queue = queue }
  loop() {
    const item = this.queue.take()
    return this.queue.put(item)
  }
}

export class Job {
  constructor(id) { this.id = id; this.status = "created" }
  computeDelay() { return this.id ? 1000 : -1 }
  toData() { return JSON.stringify({ id: this.id }) }
}

export function finish(job, failed) {
  const delay = failed ? job.computeDelay() : -1
  return delay >= 0 ? job.toData() : ""
}

export function finishNew(id) { return finish(new Job(id), true) }

class Panel {
  draw() { return "panel" }
  size() { return 1 }
}

export function render(view) {
  view.draw()
  return view.size()
}

export function renderLiteral() { return render({ draw() { return "" }, size() { return 0 } }) }

class Journal {
  write(text) { return text }
}

export class Notifier {
  constructor(sink) { this.sink = sink }
  // write is all the code uses on the sink: a method's name alone.
  notify(text) { return this.sink.write(text) }
}

export function drain(storage) {
  // The code tests the value's shape: it is not one kind of value.
  if ("size" in storage) storage.size()
  return storage.dequeue()
}

class Metronome {
  tick() { return 1 }
  reset() { return 0 }
}

class Counter {
  constructor() { this.tick = 0 }
  reset() { this.tick = 0 }
}

export class Beat {
  constructor(timer) { this.timer = timer }
  step() {
    this.timer.reset()
    // Counter has a tick, but a number: no method is claimed.
    return this.timer.tick()
  }
}

export const unused = [MemoryStorage, FileStorage, Panel, Journal, Metronome, Counter]
