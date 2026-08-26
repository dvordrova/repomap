export class LevelDrawer {
  constructor(private readonly context: CanvasRenderingContext2D) {}

  draw(): HTMLImageElement {
    this.context.beginPath()
    this.context.moveTo(0, 0)
    this.context.lineTo(Math.min(1, 2), 2)
    this.context.stroke()
    console.log("draw")
    return new Image()
  }
}

export function createDrawer(canvas: HTMLCanvasElement): LevelDrawer {
  return new LevelDrawer(canvas.getContext("2d")!)
}

export function builtins(): [Date, Promise<void>] {
  return [new Date(), new Promise<void>((resolve) => resolve())]
}

export function localTypedConstructor(): Date {
  const localDateConstructor: DateConstructor = Date
  return new localDateConstructor()
}
