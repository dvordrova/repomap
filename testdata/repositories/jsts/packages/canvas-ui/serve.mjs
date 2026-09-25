import { createServer } from "node:http"
import { drawCanvas } from "./canvas.mjs"

// The application server, run by the start script. The visual checks start it
// too; that does not make it test code.
createServer((request, response) => response.end(drawCanvas(["api", "store"]))).listen(8875)
