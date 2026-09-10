import express, { Router, type Request, type Response } from "express"

const root = express()
const publicRouter = Router()
const privateRouter = Router()

export function ping(_request: Request, response: Response): void {
  response.json({status: "ready"})
}

publicRouter.get("/ping", ping)
privateRouter.get("/ping", ping)
root.use("/api/v1", publicRouter)
root.use("/alternate", publicRouter)
root.use("/private", privateRouter)

const localLookalike = {use(_prefix: string, _router: unknown): void {}}
localLookalike.use("/not-a-route", publicRouter)
