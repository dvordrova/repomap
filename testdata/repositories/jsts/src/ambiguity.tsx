import { createConsumer } from "@fixture/kafka-client"
import { useQuery } from "@tanstack/react-query"
import express, { type Request, type Response } from "express"
import { Route } from "wouter"

export function requestMiddleware(
  _request: Request,
  _response: Response,
): void {}

export function ambiguousHandler(request: Request, response: Response): void
export function ambiguousHandler(
  request: Request,
  response: Response,
): Promise<void>
export function ambiguousHandler(
  _request: Request,
  response: Response,
): void | Promise<void> {
  response.json({ ok: true })
}

export function registerAmbiguousExpress(): void {
  const app = express()
  app.get("/products/ambiguous", requestMiddleware, ambiguousHandler)
}

export interface mixedOrderCallback {
  marker: string
}

export function mixedOrderCallback(_event: unknown): void {}

export function registerMixedOrderCallback(): void {
  const consumer = createConsumer()
  consumer.subscribe("orders.mixed", mixedOrderCallback)
}

export function AmbiguousPage(): null
export function AmbiguousPage(_props: { mode?: string }): null
export function AmbiguousPage(_props?: { mode?: string }): null {
  return null
}

export function AmbiguousRouter(): unknown {
  return <Route path="/ambiguous" component={AmbiguousPage} />
}

export function mixedQueryKey(userId: string): void {
  useQuery({ queryKey: ["users", userId] })
}

export function fetchMethodAuthority(dynamicMethod: string): void {
  fetch("/products/duplicate-method", { method: "POST", method: "PATCH" })
  fetch("/products/dynamic-method", { method: dynamicMethod })
}
