export function dispatch(address: string): Promise<Response> {
  const request = new Request(address, { method: "GET" });
  return fetch(request);
}

export function exchangePrices(base: string): Promise<Response> {
  const endpoint = `${base}/시세`;
  return dispatch(endpoint);
}

export function notifyOrders(config: { notifications: { url: string } }): Promise<Response> {
  return dispatch(config.notifications["url"]);
}

export function capturedExchange(base: string): () => Promise<Response> {
  function refresh(): Promise<Response> { return dispatch(base + "/candles"); }
  return refresh;
}

export function directPriceFeed(): Promise<Response> {
  return fetch("https://시세.example/prices%20latest");
}

export function chooseFeed(preferred: string, fallback: string, usePreferred: boolean): Promise<Response> {
  return dispatch(usePreferred ? preferred : fallback);
}

export class ExchangeAdapter {
  constructor(private config: { exchange: { url: string } }) {}
  refresh(): Promise<Response> { return dispatch(this.config.exchange.url + "/candles"); }
}

export function reassignedAddress(replacement: string): Promise<Response> {
  let address = "https://old.example/";
  address = replacement;
  return dispatch(address);
}

export function buildExchange(config: { exchange: { url: string } }): ExchangeAdapter {
  return new ExchangeAdapter(config);
}

export function runExchange(config: { exchange: { url: string } }): Promise<Response> {
  const adapter = buildExchange(config);
  return adapter.refresh();
}

export class MutableAdapter {
  private url: string;
  constructor() { this.url = "https://old.example/"; }
  replace(url: string): void { this.url = url; }
  send(): Promise<Response> { return fetch(this.url); }
}

export function mutableAdapter(): MutableAdapter { return new MutableAdapter(); }

export class LiteralAdapter {
  constructor(private base: string) {}
  refresh(): Promise<Response> { return fetch(this.base + "/candles"); }
}

export function runLiteralAdapter(): Promise<Response> {
  const adapter = new LiteralAdapter("https://exchange.example");
  return adapter.refresh();
}

export function unusedLiteralAdapter(): LiteralAdapter {
  return new LiteralAdapter("https://unrelated.example");
}
