import { createConsumer } from "@fixture/kafka-client"
import axios from "axios"
import express, { type Request, type Response } from "express"

export interface Product {
  id: string
  name: string
}

export interface OrderEvent {
  orderId: string
}

const handledOrderIds: string[] = []

const localRouterLookalike = {
  get(_path: string, _handler: (request: Request, response: Response) => void): void {},
}

const localConsumerLookalike = {
  subscribe(_topic: string, _handler: (event: OrderEvent) => void): void {},
}

export async function loadFeaturedProduct(): Promise<Product> {
  const response = await axios.get<Product>(
    "https://catalog.example/products/featured",
  )
  return response.data
}

export async function getFeaturedProduct(
  _request: Request,
  response: Response,
): Promise<void> {
  const product = await loadFeaturedProduct()
  response.json(product)
}

export function recordOrder(event: OrderEvent): void {
  handledOrderIds.push(event.orderId)
}

export function handleOrder(event: OrderEvent): void {
  recordOrder(event)
}

export function startServer(dynamicPath: string): void {
  const app = express()
  app.get("/products/featured", getFeaturedProduct)
  app.get(dynamicPath, getFeaturedProduct)
  app.listen(3000)
  localRouterLookalike.get("/products/lookalike", getFeaturedProduct)
}

export function registerOrderConsumer(dynamicSelector: string): void {
  const consumer = createConsumer()
  consumer.subscribe("orders.created", handleOrder)
  const dynamicConsumer = consumer as unknown as Record<
    string,
    (...args: unknown[]) => void
  >
  dynamicConsumer[dynamicSelector](
    "orders.dynamic",
    handleOrder,
  )
  localConsumerLookalike.subscribe("orders.lookalike", handleOrder)
}

export function registerRuntimeOrderHandler(
  topic: string,
  handler: (event: OrderEvent) => void,
): void {
  const consumer = createConsumer()
  consumer.subscribe(topic, handler)
}

startServer("/products/runtime")
registerOrderConsumer("subscribe")

export function registerDirectOrderConsumer(): void {
  createConsumer().subscribe("orders.direct", handleOrder)
}

export function startAmbiguousServer(path: string): void {
  const app = express()
  app.get(path, getFeaturedProduct)
}

startAmbiguousServer("/products/ambiguous-one")
startAmbiguousServer("/products/ambiguous-two")

export function startReassignedServer(path: string): void {
  path = "/products/changed"
  const app = express()
  app.get(path, getFeaturedProduct)
}

startReassignedServer("/products/reassigned")
