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

function preserveCallable<T extends (...args: any[]) => any>(value: T): T {
  return value
}

declare function ActionPanel(props: {
  onActivate: (event: unknown) => void
  renderValue: () => string
  wrappedValue?: (event: unknown) => void
  label: string
}): unknown

export function ActionPage() {
  const execute = preserveCallable((_event: unknown) => {
    fetch("/products/run", { method: "POST" })
  })
  const renderValue = () => "Ready"
  return <ActionPanel onActivate={execute} renderValue={renderValue} wrappedValue={preserveCallable(execute)} label="Run" />
}

export function OtherActionPage() {
  const execute = () => console.log("different declaration")
  return <ActionPanel onActivate={execute} renderValue={() => "Other"} label="Other" />
}

import { paintColor as READ_COLOR, ReadTile as Tile } from "../shared/contracts"
import * as palette from "./destinations"

export function valueReferences() {
  const alias = READ_COLOR
  const shorthand = { READ_COLOR }
  const keyed = { [READ_COLOR]: palette.surfaceColor }
  return [alias, shorthand, keyed, READ_COLOR, READ_COLOR]
}
export function localReadShadow() {
  const READ_COLOR = "local"
  return READ_COLOR
}
export function parameterReadShadow(READ_COLOR: string) { return READ_COLOR }
export function typeOnlyRead(_value: typeof READ_COLOR): typeof READ_COLOR { return "indigo" }
export type ColorShape = typeof READ_COLOR
export { READ_COLOR }

export function writeOnlyReferences() {
  let counter = 0
  counter = 1
  ;[counter] = [2]
  ;({ counter } = { counter: 3 })
  return { counter: "a property name" }
}
export function readModifyReferences() {
  let counter = 0
  counter += 1
  counter++
  return counter
}
export function ElementReferences() {
  return <><Tile /><palette.Tile label={READ_COLOR}></palette.Tile></>
}
export function TagShadow(Tile: any) { return <Tile /> }
export function directTileCall() { Tile() }
export function deferredRead() {
  queueMicrotask(() => { void READ_COLOR })
}

export interface ReadPalette { color: string }
export const currentPalette: ReadPalette = { color: READ_COLOR }
export function propertyRead() { return currentPalette.color }
