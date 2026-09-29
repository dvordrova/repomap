// Workers built and run the way freqtrade's trade command runs its Worker,
// beside Python's src/fixture_app/workers.py. `new` constructs: the call is
// the constructor the compiler resolves, the class's own (Worker) or the one
// it inherits (QuietWorker keeps BaseWorker's). A call on the result is the
// method of the name's declared type, an inherited one included.

class BaseWorker {
  constructor(readonly name: string) {}

  run(): string {
    return this.step()
  }

  step(): string {
    return this.name
  }
}

class Worker extends BaseWorker {
  constructor(name: string, readonly retries: number) {
    super(name)
  }

  step(): string {
    return this.name.repeat(this.retries)
  }
}

class QuietWorker extends BaseWorker {}

class Plain {}

export function startWorker(name: string): number {
  let worker: Worker | null = null
  try {
    worker = new Worker(name, 2)
    worker.run()
  } finally {
    if (worker) worker.step()
  }
  return 0
}

export function startQuiet(name: string): string {
  const quiet = new QuietWorker(name)
  return quiet.run()
}

export function makePlain(): Plain {
  return new Plain()
}
