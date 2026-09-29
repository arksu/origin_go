import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const html = readFileSync(new URL("./index.html", import.meta.url), "utf8");
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];

function schema(preset, seed = 731, amplitude = 1.7) {
  return {
    preset, preset_directory: "/presets", seed, chunks_x: 3, chunks_y: 4,
    sizes: [{ chunks: 10, tiles: 1280 }],
    world: { name: "world", title: "World", param_key: "world", groups: [{ title: "Рельеф и вода Перлина", fields: [
      { key: "terrain_scale", label: "terrain_scale", type: "float", min: 0, range_max: 0.02, step: 0.0001, default: 0.002 },
      { key: "perlin_water_enabled", label: "perlin_water_enabled", type: "bool", default: false },
    ] }] },
    layers: [{ name: "biomes", title: "Biomes", param_key: "biomes", groups: [{ title: "Включение", fields: [
      { key: "enabled", label: "enabled", type: "bool", default: true },
      { key: "blob_enabled", label: "blob_enabled", type: "bool", default: true },
      { key: "blob_raggedness", label: "blob_raggedness", type: "float", min: 0, max: 64, step: 0.1, default: 1 },
      { key: "hnh_smoothing_passes", label: "hnh_smoothing_passes", type: "int", min: 0, range_max: 10, default: 3 },
      { key: "temperature_scale", label: "temperature_scale", type: "float", min: 0, range_max: 4, step: 0.01, default: 1 },
    ] }] }, { name: "rivers", title: "Rivers", param_key: "river", groups: [{
      title: "Channel shape", fields: [{ key: "shape_amplitude_scale", label: "Amplitude", type: "float", min: 0, max: 3, step: 0.1, default: amplitude }],
    }] }],
  };
}

async function page(address = "http://127.0.0.1:8107/?preset=variant.yaml&keep=1#map") {
  const elements = new Map();
  const requests = [];
  const presets = new Map([["base.yaml", schema("base.yaml", 42, 1)], ["variant.yaml", schema("variant.yaml")]]);
  const pending = new Map();
  let location = new URL(address);
  class Element {
    constructor() {
      this.children = [];
      this.listeners = new Map();
      this.value = "";
      this.checked = false;
      this.disabled = false;
      this.clientWidth = 1000;
      this.clientHeight = 800;
      this.classList = { add() {}, remove() {}, toggle() {} };
    }
    set id(value) { this.identifier = value; elements.set(value, this); }
    get id() { return this.identifier; }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    addEventListener(name, listener) {
      const previous = this.listeners.get(name);
      this.listeners.set(name, previous ? event => { previous(event); return listener(event); } : listener);
    }
    querySelectorAll() { return this.children.flatMap(child => child.children || []).filter(child => child.checked); }
    getContext() { return { fillRect() {}, drawImage() {} }; }
  }
  for (const [, identifier] of html.matchAll(/id="([^"]+)"/g)) {
    const element = new Element();
    element.id = identifier;
  }
  elements.get("settings").disabled = true;
  elements.get("auto").checked = true;
  const response = (payload, ok = true) => ({ ok, status: ok ? 200 : 400, json: async () => structuredClone(payload) });
  const context = vm.createContext({
    document: {
      getElementById: identifier => elements.get(identifier),
      createElement: () => new Element(),
      createTextNode: text => ({ text }),
    },
    window: {
      get location() { return location; },
      history: { replaceState: (_state, _title, value) => { location = new URL(value); } },
      addEventListener() {},
    },
    URL, URLSearchParams, AbortController,
    setTimeout: () => 1, clearTimeout() {}, performance: { now: () => 1000 },
    createImageBitmap: async blob => ({ ...blob, close() {} }),
    fetch: async (address, options = {}) => {
      const url = new URL(address, location);
      const body = options.body ? JSON.parse(options.body) : null;
      requests.push({ url, options, body });
      if (url.pathname === "/api/defaults") {
        const name = url.searchParams.get("preset") || "base.yaml";
        if (pending.has(name)) return pending.get(name);
        return presets.has(name) ? response(presets.get(name)) : response({ error: `Missing preset ${name}` }, false);
      }
      if (url.pathname === "/api/render") {
        return { ok: true, blob: async () => ({ width: body.chunks_x * 128, height: body.chunks_y * 128 }) };
      }
      assert.equal(url.pathname, "/api/preset");
      const saved = schema(body.path, body.seed, body.params.river.shape_amplitude_scale);
      for (const panel of [saved.world, ...saved.layers]) {
        for (const group of panel.groups) {
          for (const field of group.fields) field.default = body.params[panel.param_key][field.key];
        }
      }
      saved.chunks_x = body.chunks_x;
      saved.chunks_y = body.chunks_y;
      presets.set(body.path, saved);
      return response({ preset: body.path });
    },
  });
  const run = source => vm.runInContext(source, context);
  await run(script);
  return { elements, requests, presets, pending, response, run, get url() { return location; } };
}

test("page loads the queried preset, rectangular dimensions, and parameters", async () => {
  const current = await page();
  assert.equal(current.elements.get("preset").value, "variant.yaml");
  assert.equal(current.elements.get("seed").value, "731");
  assert.equal(current.elements.get("worldSize").value, "3x4");
  assert.equal(current.elements.get("f_rivers_shape_amplitude_scale").value, "1.7");
  const render = current.requests.find(request => request.url.pathname === "/api/render");
  assert.equal(render.body.preset, "variant.yaml");
  assert.equal(render.body.chunks_x, 3);
  assert.equal(render.body.chunks_y, 4);
  assert.equal(current.url.searchParams.get("keep"), "1");
  assert.equal(current.url.hash, "#map");
  assert.equal(current.elements.get("savePreset").disabled, false);
});

test("without a query parameter, the loaded preset is added to the URL", async () => {
  const current = await page("http://127.0.0.1:8107/");
  assert.equal(current.url.searchParams.get("preset"), "base.yaml");
  assert.equal(current.elements.get("preset").value, "base.yaml");
  assert.equal(current.elements.get("seed").value, "42");
});

test("typing a filename syncs the URL without loading or saving; Save creates a variant", async () => {
  const current = await page();
  const before = current.requests.length;
  current.elements.get("preset").value = "good variant.yaml";
  current.elements.get("preset").listeners.get("input")();
  assert.equal(current.url.searchParams.get("preset"), "good variant.yaml");
  assert.equal(current.requests.length, before);
  current.elements.get("seed").value = "999";
  current.elements.get("f_rivers_shape_amplitude_scale").value = "2.4";
  await current.run("savePreset()");
  const saved = current.requests.find(request => request.url.pathname === "/api/preset");
  assert.equal(saved.body.preset, "variant.yaml");
  assert.equal(saved.body.path, "good variant.yaml");
  assert.equal(saved.options.headers["Content-Type"], "application/json");
  assert.equal(current.presets.get("variant.yaml").seed, 731);
  assert.equal(current.elements.get("presetStatus").textContent, "Saved good variant.yaml");
  current.elements.get("seed").value = "1";
  await current.run("loadPreset()");
  assert.equal(current.elements.get("seed").value, "999");
  assert.equal(current.elements.get("f_rivers_shape_amplitude_scale").value, "2.4");
  assert.equal(current.elements.get("layersBox").children.length, 2);
});

test("missing presets show an error and disable Save instead of silently falling back", async () => {
  const current = await page("http://127.0.0.1:8107/?preset=missing.yaml");
  assert.match(current.elements.get("presetStatus").textContent, /Missing preset/);
  assert.equal(current.elements.get("savePreset").disabled, true);
  assert.equal(current.requests.length, 1);
});

test("an older preset load cannot overwrite a newer selection", async () => {
  const current = await page();
  let resolveFirst;
  current.pending.set("slow.yaml", new Promise(resolve => { resolveFirst = resolve; }));
  const first = current.run('els.preset.value = "slow.yaml"; loadPreset()');
  await current.run('els.preset.value = "base.yaml"; loadPreset()');
  resolveFirst(current.response(schema("slow.yaml", 987)));
  await first;
  assert.equal(current.elements.get("preset").value, "base.yaml");
  assert.equal(current.elements.get("seed").value, "42");
});

test("invalid numeric controls cannot be saved as null values", async () => {
  const current = await page();
  current.elements.get("f_rivers_shape_amplitude_scale").value = "";
  await current.run("savePreset()");
  assert.equal(current.requests.filter(request => request.url.pathname === "/api/preset").length, 0);
  assert.match(current.elements.get("presetStatus").textContent, /must be a valid number/);
  assert.equal(current.elements.get("savePreset").disabled, false);
});

test("typing during a pending load keeps the edited path and query intact", async () => {
  const current = await page();
  let resolveFirst;
  current.pending.set("slow.yaml", new Promise(resolve => { resolveFirst = resolve; }));
  const first = current.run('els.preset.value = "slow.yaml"; loadPreset()');
  current.elements.get("preset").value = "new draft.yaml";
  current.elements.get("preset").listeners.get("input")();
  resolveFirst(current.response(schema("slow.yaml", 987)));
  await first;
  assert.equal(current.elements.get("preset").value, "new draft.yaml");
  assert.equal(current.url.searchParams.get("preset"), "new draft.yaml");
  assert.equal(current.elements.get("savePreset").disabled, true);
});

test("biome and world values load, render, and save even with the biome layer hidden", async () => {
  const current = await page();
  assert.equal(current.elements.get("f_biomes_enabled").checked, true);
  assert.equal(current.elements.get("f_world_perlin_water_enabled").checked, false);
  assert.equal(current.elements.get("f_world_terrain_scale").value, "0.002");
  const biomeLayer = current.elements.get("layersBox").children.find(label => label.children[0].value === "biomes").children[0];
  biomeLayer.checked = false;
  current.elements.get("f_biomes_blob_enabled").checked = false;
  current.elements.get("f_biomes_blob_raggedness").value = "0";
  current.elements.get("f_biomes_hnh_smoothing_passes").value = "0";
  current.elements.get("f_world_terrain_scale").value = "0.00123";
  current.elements.get("preset").value = "biomes.yaml";
  await current.run("runRender()");
  const rendered = current.requests.filter(request => request.url.pathname === "/api/render").at(-1).body;
  assert.deepEqual(rendered.layers, ["rivers"]);
  assert.equal(rendered.params.biomes.enabled, true);
  assert.equal(rendered.params.biomes.blob_enabled, false);
  assert.equal(rendered.params.biomes.blob_raggedness, 0);
  assert.equal(rendered.params.biomes.hnh_smoothing_passes, 0);
  assert.equal(rendered.params.world.terrain_scale, 0.00123);
  assert.equal(rendered.params.world.perlin_water_enabled, false);
  assert.equal(current.requests.filter(request => request.url.pathname === "/api/preset").length, 0);
  await current.run("savePreset()");
  const saved = current.requests.find(request => request.url.pathname === "/api/preset").body;
  assert.deepEqual(saved.params, rendered.params);
  await current.run("loadPreset()");
  assert.equal(current.elements.get("f_biomes_blob_enabled").checked, false);
  assert.equal(current.elements.get("f_biomes_blob_raggedness").value, "0");
  assert.equal(current.elements.get("f_biomes_hnh_smoothing_passes").value, "0");
  assert.equal(current.elements.get("f_world_terrain_scale").value, "0.00123");
});

test("slider ranges include preset values without rounding or clamping numeric inputs", async () => {
  const current = await page();
  const preset = current.presets.get("variant.yaml");
  const fields = preset.layers[0].groups[0].fields;
  fields.find(field => field.key === "temperature_scale").default = 12.34567;
  fields.find(field => field.key === "hnh_smoothing_passes").default = 20;
  await current.run("loadPreset()");
  assert.equal(current.elements.get("f_biomes_temperature_scale").value, "12.34567");
  assert.equal(current.elements.get("f_biomes_hnh_smoothing_passes").value, "20");
  const ranges = current.run(`(() => {
    const wrap = buildField(state.schema.layers[0], state.schema.layers[0].groups[0].fields.find(field => field.key === "temperature_scale"));
    const number = wrap.children[0].children[1];
    const range = wrap.children[1];
    const initialMaximum = range.max;
    number.value = "30.123456";
    number.listeners.get("input")();
    return [initialMaximum, range.max, number.value];
  })()`);
  assert.deepEqual(Array.from(ranges), ["12.34567", "30.123456", "30.123456"]);
  assert.equal(current.run("collectParams().biomes.temperature_scale"), 30.123456);
});
