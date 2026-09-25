import { createServer } from "node:http"

// Canned groups for the visual checks. Only Playwright's webServer starts this
// server, so it is test code.
createServer((request, response) => response.end(JSON.stringify(["api", "store"]))).listen(8876)
