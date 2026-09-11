import { Resolver as Factory } from "./index";
import * as providers from "./index";

export class Bot {
  private exchange = Factory.load();
  private second = providers.Resolver.load();

  start(): void {
    setInterval(this.exchange.resetStream, 60000);
    setInterval(this.second.resetStream, 120000);
  }
}

export function startUnknown(receiver: any): void {
  setInterval(receiver.resetStream, 30000);
}

export class Exchange {
  resetStream(): void {
    throw new Error("same name does not own the callback");
  }
}

import { Market } from "./exchange-index";
export function typedParameter(exchange: Market): void {
  exchange.resetStream();
}
export function unknownParameter(exchange: any): void {
  exchange.resetStream();
}
