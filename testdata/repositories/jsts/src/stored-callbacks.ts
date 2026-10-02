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

// A call through a variable calls what the stores that may reach it put
// there (owner, 2026-09-30: one known target exact, several alternatives),
// as the Python adapter calls a name a branch reassigns and Go the SSA phi of
// a function value: a constant its one function, a `let` a branch reassigns
// each function its stores put there, from the last store no branch skips; a
// later store reaches the call only around a loop. A closure calling a
// variable with several stores, a store of no plain function and a write the
// index cannot follow leave the call open; a constant holding a factory's
// result stays a call of the constant, and a parameter is joined as above.
function makeHandler(): () => void {
  return acceptClient;
}

export function callConstant(): void {
  const handler = acceptClient;
  handler();
}

export function callChosenLet(readable: boolean): void {
  let handler;
  if (readable) handler = acceptClient;
  else handler = flushReplies;
  handler();
}

export function callTypedLet(readable: boolean): void {
  let handler: () => void = flushReplies;
  if (readable) handler = acceptClient;
  handler();
}

export function callOverwritten(readable: boolean): void {
  let handler: () => void = flushReplies;
  if (readable) handler = acceptClient;
  handler = flushReplies;
  handler();
}

export function callInLoop(items: number[]): void {
  let handler: () => void = flushReplies;
  for (const item of items) {
    handler();
    if (item > 0) handler = acceptClient;
  }
}

export function callFromClosure(readable: boolean): () => void {
  let handler: () => void = flushReplies;
  if (readable) handler = acceptClient;
  return () => handler();
}

export function callAfterCompound(): void {
  let handler: (() => void) | undefined = acceptClient;
  handler ??= flushReplies;
  handler();
}

export function callFactory(): void {
  const handler = makeHandler();
  handler();
}

// An array's callback runs at its call, before any store written after it:
// a closure knows only a variable whose one store is its initializer.
export function callInMapOfConstant(items: number[]): void {
  const handler = acceptClient;
  items.map(() => handler());
}

export function callInMapBeforeStore(items: number[]): void {
  let handler!: () => void;
  items.map(() => handler());
  handler = acceptClient;
}
