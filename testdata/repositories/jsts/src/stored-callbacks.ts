// An event loop stores a handler in one of two properties, chosen by a flag.
// The calls through those properties stay unresolved: neither property is
// given the handler registered for the other.
export class EventLoop {
  onRead: () => void = () => {};
  onWrite: () => void = () => {};

  register(readable: boolean, handler: () => void): void {
    if (readable) {
      this.onRead = handler;
    } else {
      this.onWrite = handler;
    }
  }

  fire(): void {
    this.onRead();
    this.onWrite();
  }
}

export function acceptClient(): void {}
export function flushReplies(): void {}

export function runEventLoop(): void {
  const loop = new EventLoop();
  loop.register(true, acceptClient);
  loop.register(false, flushReplies);
  loop.fire();
}

// A function calling its own parameter calls what every call into it hands
// there, as the Python, Go and C adapters join a parameter's callers: run's
// two calls hand throttle two methods, the call's alternatives, startOnce
// hands runOnce one function, its exact target, and the test's arrow
// function (market.test.ts) is none of the program's values.
export class Throttle {
  run(running: boolean): void {
    if (running) {
      this.throttle(this.processRunning, 1);
    } else {
      this.throttle(this.processStopped, 1);
    }
  }

  throttle(step: () => void, secs: number): void {
    step();
  }

  processRunning(): void {}

  processStopped(): void {}
}

function runOnce(job: () => void): void {
  job();
}

export function startOnce(): void {
  runOnce(acceptClient);
}

// A call handing any other value leaves the call open, the function another
// call hands its witness.
function runAny(job: () => void): void {
  job();
}

export function startAny(job: () => void): void {
  runAny(flushReplies);
  runAny(job);
}
