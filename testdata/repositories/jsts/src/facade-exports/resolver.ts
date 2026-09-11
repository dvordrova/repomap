import { Market } from "./exchange-index";

export class ExchangeResolver {
  static load(): Market {
    return new Market();
  }
}
