import { writeFileSync } from "node:fs"
import { drawCanvas } from "./canvas.mjs"

writeFileSync("canvas.txt", drawCanvas(["api", "store"]))
