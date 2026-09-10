import { readFileSync } from "node:fs"

export function prepareReadme(filename) {
  return readFileSync(filename, "utf8")
}

prepareReadme("README.md")
