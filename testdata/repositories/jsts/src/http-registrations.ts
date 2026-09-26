import express, {type Application, type Request, type Response} from "express"

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

// installRouteTree hands one application to two helpers. The leaves' app
// reaches the leaf through one more parameter, so its route is held by the
// express() call. The branch also hands its app to itself: following that
// parameter back through its callers comes around to the same parameter, so
// its routes have no holder.
export function installRouteTree(): void {
  const app = express()
  registerRouteLeaves(app)
  registerRouteBranch(app, false)
}

function registerRouteLeaves(app: Application): void {
  registerRouteLeaf(app)
}

function registerRouteLeaf(app: Application): void {
  app.get("/tree/leaf", emptyHealthHandler)
}

function registerRouteBranch(app: Application, nested: boolean): void {
  if (nested) {
    app.get("/tree/branch/nested", emptyHealthHandler)
    return
  }
  app.get("/tree/branch", emptyHealthHandler)
  registerRouteBranch(app, true)
}
