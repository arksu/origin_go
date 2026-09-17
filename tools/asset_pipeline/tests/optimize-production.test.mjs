import assert from 'node:assert/strict'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import test from 'node:test'
import { loadRecipes } from '../catalog.ts'
import { exportAsset } from '../blender.ts'
import { optimizeExport } from '../optimize.mjs'
import { assetIO, canonicalJSON, catalogBytes, sha256, validateArtifacts } from '../report.mjs'
import { reviewTexture } from '../texture-review.mjs'
import { blenderPath, pipelineDirectory } from './blender_helpers.ts'

test('production assets preserve the full decoded contract and meet measured UASTC budgets', async () => {
  const root = resolve(import.meta.dirname, '../../..')
  const recipes = await loadRecipes(root)
  const output = join(root, `build/optimization-review-${Date.now()}`)
  await mkdir(output, { recursive: true })
  const identity = JSON.parse(await readFile(new URL('../toolchain.lock.json', import.meta.url)))
  const toolchain = { identity, paths: { toktx: join(pipelineDirectory, '.tools/ktx-4.4.2/install/usr/local/bin/toktx') } }
  const reports = {}, metrics = []
  for (const [id, recipe] of recipes) {
    const directory = join(output, id.replace('/', '-'))
    const sourceHash = sha256(await readFile(recipe.source.absolutePath))
    await exportAsset({ recipe, selectedClips: Object.keys(recipe.clips),
      resolvedDependencies: [...recipe.dependencies.export, ...recipe.dependencies.preview].map(assetId => ({
        assetId, absolutePath: recipes.get(assetId).source.absolutePath,
      })) }, directory, blenderPath)
    const result = await optimizeExport({ rawDirectory: join(directory, 'raw'), recipe, toolchain, outputDirectory: join(directory, 'optimized') })
    const report = await validateArtifacts(result, recipe)
    assert.equal(report.errors, 0)
    assert.equal(sha256(await readFile(recipe.source.absolutePath)), sourceHash)
    if (recipe.kind === 'character') assert.ok(report.nodeNames.includes('grip_l'))
    const original = await assetIO().read(join(directory, 'raw/model.glb'))
    const textureReview = []
    for (const [index, texture] of original.getRoot().listTextures().entries()) {
      const comparison = await reviewTexture(texture.getImage(), result.textures[index])
      assert.ok(comparison.compressionError.rootMeanSquareError < 8, `${id} UASTC color error`)
      textureReview.push(comparison)
    }
    metrics.push(result.metrics)
    reports[id] = { ...report, textureReview, model: result.model, rawDirectory: result.rawDirectory,
      suggestedByteBudget: Math.ceil(result.metrics.totalBytes * 1.2 / 1024) * 1024 }
  }
  const reportPath = join(output, 'report.json')
  await writeFile(reportPath, canonicalJSON({ assets: reports, catalogBytes: catalogBytes(metrics) }) + '\n')
  console.log(`OPTIMIZATION_REVIEW=${reportPath}`)
})
