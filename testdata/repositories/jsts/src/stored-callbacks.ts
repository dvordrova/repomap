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
