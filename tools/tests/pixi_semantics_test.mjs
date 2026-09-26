// Validates the packer's atlas conventions against the REAL Pixi v8 code:
// Spritesheet parser + Texture.updateUvs + TextureMatrix + updateQuadBounds.
// No WebGL needed — everything here is the engine's actual CPU-side math.
//
// Run: node tools/tests/pixi_semantics_test.mjs
import * as cp from "node:child_process";
import { promisify } from "node:util";
import { pathToFileURL } from "node:url";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";

const execFile = promisify(cp.execFile);
const root = path.resolve(import.meta.dirname, "..", "..");

const { Spritesheet } = await import(
  pathToFileURL(path.join(root, "web_new/node_modules/pixi.js/lib/spritesheet/Spritesheet.mjs"))
);
const { Texture } = await import(
  pathToFileURL(path.join(root, "web_new/node_modules/pixi.js/lib/rendering/renderers/shared/texture/Texture.mjs"))
);
const { TextureSource } = await import(
  pathToFileURL(path.join(root, "web_new/node_modules/pixi.js/lib/rendering/renderers/shared/texture/sources/TextureSource.mjs"))
);
const { updateQuadBounds } = await import(
  pathToFileURL(path.join(root, "web_new/node_modules/pixi.js/lib/utils/data/updateQuadBounds.mjs"))
);

// 1. generate fixtures and pack them (rotations and trim enabled)
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "atlas-pixi-test-"));
const fixtures = path.join(tmp, "fixtures");
await execFile("python3", [path.join(root, "tools/tests/gen_fixtures.py"), fixtures]);
for (const algorithm of ["maxrects", "skyline"]) {
  await execFile("python3", [
    path.join(root, "tools/pack_texturepacker_atlas.py"),
    fixtures,
    path.join(tmp, `atlas_${algorithm}.json`),
    path.join(tmp, `atlas_${algorithm}.png`),
    "--algorithm", algorithm,
    "--animations-from-sequences",
  ]);
}

for (const algorithm of ["maxrects", "skyline"]) {
  const data = JSON.parse(fs.readFileSync(path.join(tmp, `atlas_${algorithm}.json`), "utf8"));
  const sheetW = data.meta.size.w;
  const source = new TextureSource({ resource: { width: sheetW, height: sheetW }, width: sheetW, height: sheetW });
  const base = new Texture({ source });
  const sheet = new Spritesheet(base, data);
  sheet.parseSync();
  const frameKeys = Object.keys(data.frames);

  for (const key of frameKeys) {
    const fr = data.frames[key];
    const tex = sheet.textures[key];
    assert.ok(tex, `${algorithm}: texture missing for ${key}`);

    // --- parser semantics (Spritesheet.mjs) ---
    const expectedRegionW = fr.rotated ? fr.frame.h : fr.frame.w;
    const expectedRegionH = fr.rotated ? fr.frame.w : fr.frame.h;
    assert.equal(tex.rotate, fr.rotated ? 2 : 0, `${algorithm}/${key}: rotate`);
    assert.deepEqual(
      [tex.frame.x, tex.frame.y, tex.frame.width, tex.frame.height],
      [fr.frame.x, fr.frame.y, expectedRegionW, expectedRegionH],
      `${algorithm}/${key}: frame must be the on-sheet region`);
    assert.deepEqual(
      [tex.orig.width, tex.orig.height],
      [fr.sourceSize.w, fr.sourceSize.h],
      `${algorithm}/${key}: orig == sourceSize`);
    if (fr.trimmed) {
      assert.deepEqual(
        [tex.trim.x, tex.trim.y, tex.trim.width, tex.trim.height],
        [fr.spriteSourceSize.x, fr.spriteSourceSize.y, fr.frame.w, fr.frame.h],
        `${algorithm}/${key}: trim in original coords`);
    } else {
      assert.equal(tex.trim, null, `${algorithm}/${key}: untrimmed has no trim`);
    }

    // --- renderer math (DefaultBatcher.packQuadAttributes + updateQuadBounds) ---
    // Sprite quads sample texture.uvs corners directly; trim only shifts the
    // quad positions (content drawn at trim.x/y within the orig rect). So the
    // fixed corner mapping must land on the region pixels our convention
    // predicts (region = content rotated 90° CW when rotated).
    const bounds = { minX: 0, maxX: 0, minY: 0, maxY: 0 };
    updateQuadBounds(bounds, { _x: 0, _y: 0 }, tex);
    const cw = fr.frame.w, ch = fr.frame.h; // content dims (upright)
    assert.deepEqual(
      [bounds.minX + 0, bounds.minY + 0, bounds.maxX - bounds.minX, bounds.maxY - bounds.minY],
      fr.trimmed ? [fr.spriteSourceSize.x, fr.spriteSourceSize.y, cw, ch] : [0, 0, cw, ch],
      `${algorithm}/${key}: display bounds`);

    // uv corner order in DefaultBatcher: v0..v3 = content TL,TR,BR,BL
    const uvCorners = [[tex.uvs.x0, tex.uvs.y0], [tex.uvs.x1, tex.uvs.y1],
                       [tex.uvs.x2, tex.uvs.y2], [tex.uvs.x3, tex.uvs.y3]];
    const contentCorners = [[0, 0], [cw, 0], [cw, ch], [0, ch]];
    for (let c = 0; c < 4; c++) {
      const [cx, cy] = contentCorners[c];
      // expected region-local pixel for content pixel (cx, cy)
      const ex = fr.rotated ? ch - cy : cx;
      const ey = fr.rotated ? cx : cy;
      assert.deepEqual(
        [Math.round(uvCorners[c][0] * sheetW - fr.frame.x), Math.round(uvCorners[c][1] * sheetW - fr.frame.y)],
        [ex, ey],
        `${algorithm}/${key}: quad corner ${c} (content ${cx},${cy}) must sample region pixel (${ex},${ey})`);
    }
  }

  // --- animations (parser: data.animations -> sheet.animations in JSON order) ---
  const animKeys = data.animations["seq/walk"];
  assert.deepEqual(animKeys, ["seq/walk.0001.png", "seq/walk.0002.png"], `${algorithm}: animation order`);
  const animTextures = sheet.animations["seq/walk"];
  assert.equal(animTextures.length, 2, `${algorithm}: animation length`);
  assert.ok(animTextures.every((t, i) => t === sheet.textures[animKeys[i]]),
    `${algorithm}: animation textures match frames in order`);

  // --- the fixture set must exercise the rotated+trimmed combo ---
  // (tile-mesh rendering relies on trim quad placement + rotated uvs together)
  const combo = frameKeys.filter((k) => data.frames[k].rotated && data.frames[k].trimmed);
  assert.ok(combo.length > 0, `${algorithm}: fixtures must include a rotated+trimmed frame`);
  console.log(`${algorithm}: ${frameKeys.length} frames validated against real Pixi v8 code`);
}
fs.rmSync(tmp, { recursive: true, force: true });
console.log("pixi_semantics_test: PASS");
