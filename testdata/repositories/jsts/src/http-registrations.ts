import express, {type Request, type Response} from "express"

const application = express()

export function emptyHealthHandler(_request: Request, _response: Response): void {}

export function returnedHealthHandler(): typeof emptyHealthHandler {
  function emptyReturnedHandler(_request: Request, _response: Response): void {}
  return emptyReturnedHandler
}

application.get("/health", emptyHealthHandler)
application.get("/returned", returnedHealthHandler())

class RouteDefinition {
  constructor(public path: string) {}
}

function registerRoute(path: string): void {
  application.get(path, emptyHealthHandler)
}

export function installRoutes(runtimePath: string): void {
  const update = new RouteDefinition("/v1/update")
  const metrics = new RouteDefinition("/v1/metrics")
  registerRoute(update.path)
  registerRoute(metrics.path)
  registerRoute(runtimePath)
}

export function unusedRouteDefinition(): RouteDefinition {
  return new RouteDefinition("/never-registered")
}

const localRouteNames = { get(_path: string): void {} }
localRouteNames.get("/not-a-route")

class MethodArgumentApplication {
  first(next: unknown): unknown { return next }
  second(next: unknown): unknown { return next }
}

function receiveMethodArguments(...values: Array<(next: unknown) => unknown>): void {}

// JS method values are passed functions; no bound receiver execution is implied.
export function passMethodArguments(app: MethodArgumentApplication): void {
  receiveMethodArguments(app.first, app.second, app.first, (app.second))
}
