import axios from "axios"

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

declare function useState<T>(initial: T): [T, (next: T) => void]

export function SimulationField(): number {
  const [sliderValue, setSliderValue] = useState(0)
  function animate(): void {
    setSliderValue(sliderValue + 1)
  }
  animate()
  return sliderValue
}

interface NeutralTransport {
  get(path: string): void
  post(path: string, payload: unknown): void
}

declare const neutralTransport: NeutralTransport
declare const dynamicTransport: Record<string, (path: string) => void>

export function neutralCallPatterns(levelId: string, payload: unknown): void {
  neutralTransport.get("/api/levels")
  neutralTransport.get(`/api/level/${levelId}`)
  neutralTransport.post("/api/level/run", payload)
  dynamicTransport[levelId]("/computed")
}

const localAxiosLookalike = {
  get(_path: string): void {},
}

export async function axiosBoundaryPatterns(
  levelId: string,
  payload: unknown,
  dynamicPath: string,
): Promise<void> {
  await axios.get("/api/levels")
  await axios.get(`/api/level/${levelId}`)
  await axios.post("/api/level/run", payload)
  await axios.delete("/api/levels")
  await axios.get("/api/frontend-only")
  await axios.get(dynamicPath)
  localAxiosLookalike.get("/api/lookalike")
}
