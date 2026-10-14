import {test,expect} from '@playwright/test';

test.skip(!process.env.REPOMAP_REAL_RUN?.includes('sqlite'),'REPOMAP_REAL_RUN names the completed ordinary SQLite run');

test('keyboard entry cannot scroll screen markers away from the world camera',async({page})=>{
  test.setTimeout(180_000);
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:90_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:90_000});
  await page.locator('[data-zoom-into="system-component-t1"]').press('Enter');
  await expect(map).toHaveAttribute('data-scene-level','system-component-t1');
  const overlay=page.locator('.scene-overlay');
  const outside=await overlay.locator('button').evaluateAll(buttons=>buttons.find(button=>{
    const r=button.getBoundingClientRect(),p=button.parentElement.getBoundingClientRect();
    return r.left<p.left||r.top<p.top||r.right>p.right||r.bottom>p.bottom;
  })?.getAttribute('data-zoom-into'));
  test.skip(!outside,'this component fits every magnifier in the canvas');
  const before=await overlay.evaluate(e=>[e.scrollLeft,e.scrollTop]);
  const button=page.locator(`[data-zoom-into="${outside}"]`);
  await expect(button).toHaveAttribute('tabindex','-1');
  // Focus alone used to scroll overflow:hidden without moving the camera.
  // A harmless key reaches that focus before the entry action changes it.
  await button.press('Shift');
  expect(await overlay.evaluate(e=>[e.scrollLeft,e.scrollTop])).toEqual(before);
  await button.press('Enter');
  await expect(map).toHaveAttribute('data-scene-level',`system-component-t1/${outside}`);
  expect(await overlay.evaluate(e=>[e.scrollLeft,e.scrollTop])).toEqual([0,0]);
});
