import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';

// The canvas is a window on the map (owner, 2026-10-14): nothing that takes
// room in a box — a font's size, a line's height, a width, a height, a
// margin or a gap — follows the camera's zoom, or the words rewrap and the
// boxes' contents move as the reader zooms. Opacity, strokes and a border's
// width (its room taken back from the padding) may.
test('no room-taking style follows the zoom',async()=>{
  for(const file of ['scene.css','canvas.css']){
    const css=await readFile(new URL(file,import.meta.url),'utf8');
    for(const [,body] of css.matchAll(/\{([^{}]*)\}/g))
      for(const declaration of body.split(';')){
        const [property,value=' ']=declaration.split(/:(.*)/s);
        if(/--flow-zoom/.test(value)&&/^\s*(font-size|font|line-height|width|height|min-width|min-height|max-width|max-height|margin[\w-]*|gap|row-gap|column-gap)\s*$/.test(property))
          assert.fail(`${file}: ${declaration.trim()}`);
      }
  }
});
