import { layout } from "./layout.mjs"

export function drawCanvas(groups) {
  return layout(groups).map((box) => `${box.name}@${box.x},${box.y}`).join("\n")
}
