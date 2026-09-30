import {test,expect} from '@playwright/test';

// A program entered stands its Inputs and Outside on its own border as
// ports (canvas.jsx PortPill; owner, 2026-09-30: variant B), the whole map
// keeping their frames: its Inputs frame is no longer drawn, each input
// kind is an icon named in one line when pointed at or reached by the
// keyboard, and a click on it reads that kind, the camera staying.
test('an entered program stands its inputs as ports on its border, named and read from there',async({page})=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  const map=page.locator('[data-map]');
  await expect(map).toHaveAttribute('data-fixture-ready','true');
  await expect(map).not.toHaveClass(/flow-initializing/);
  await expect(page.locator('[data-port]'),'the whole map keeps its frames').toHaveCount(0);
  await page.locator('[data-zoom-into="backend"]').first().click();
  const pill=page.locator('[data-port="backend-inputs"]');
  await expect(pill).toBeVisible();
  await expect(page.locator('.react-flow__node[data-id="backend-inputs"]')).toBeHidden();
  const icons=pill.locator('[data-port-end]');
  await expect(icons).not.toHaveCount(0);
  const before=await map.evaluate(map=>JSON.stringify(map.captureViewport()));
  await icons.first().hover();
  const tip=pill.locator('.flow-port-tip');
  await expect(tip).toHaveCount(1);
  expect((await tip.innerText()).trim()).toMatch(/^[^\n]+$/);
  await icons.first().click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Job processing service');
  await page.mouse.move(2,2);
  await expect(tip).toHaveCount(0);
  await pill.focus();await page.keyboard.press('Tab');
  await expect(icons.first()).toBeFocused();
  await expect(tip,'the keyboard names it too').toHaveCount(1);
  expect(await map.evaluate(map=>JSON.stringify(map.captureViewport())),'the camera stays').toBe(before);
  expect(errors).toEqual([]);
});
