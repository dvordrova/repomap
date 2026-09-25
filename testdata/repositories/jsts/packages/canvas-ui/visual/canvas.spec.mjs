import { expect, test } from "@playwright/test"
import { openCanvas } from "./fixture.mjs"

test("draws both groups", async ({ page }) => {
  await openCanvas(page)
  await expect(page.locator("body")).toContainText("store@200,0")
})
