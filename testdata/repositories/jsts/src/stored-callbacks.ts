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

// A callee chosen by a condition calls one of the functions its branches
// name, the condition deciding which (C.md's conditional callee):
// alternatives through a function value. Nested conditions and parenthesised
// or asserted names are the same choice. A literal condition, or two branches
// naming one function, calls it plainly; a branch naming anything else, here
// a parameter, leaves the call open. A class a condition chooses constructs
// one of the classes, each its own constructor: the compiler's signature for
// the union is no authority.
function tickSeconds(ms: number): number {
  return ms / 1000;
}

function tickMillis(ms: number): number {
  return ms;
}

const tickTenths = (ms: number): number => ms / 100;

class SecondsClock {
  constructor(readonly ms: number) {}
}

class MillisClock {
  constructor(readonly ms: number) {}
}

export function watchTick(ms: number, seconds: boolean, tenths: boolean, held: (ms: number) => number): void {
  (seconds ? tickSeconds : tickMillis)(ms);
  (tenths ? tickTenths : seconds ? tickSeconds : (tickMillis as (ms: number) => number))(ms);
  (seconds ? tickSeconds : held)(ms);
}

export function watchTickAgain(ms: number, seconds: boolean, clock: typeof MillisClock): void {
  (true ? tickSeconds : tickMillis)(ms);
  (seconds ? tickMillis : tickMillis)(ms);
  new (seconds ? SecondsClock : MillisClock)(ms);
  new (seconds ? SecondsClock : clock)(ms);
}
