import { fileURLToPath, URL } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import fs from 'node:fs'
import path from 'node:path'

function terrainSavePlugin(): Plugin {
  return {
    name: 'terrain-save',
    configureServer(server) {
      server.middlewares.use('/__api/save-terrain', (req, res) => {
        if (req.method !== 'POST') {
          res.statusCode = 405
          res.end('Method not allowed')
          return
        }

        let body = ''
        req.on('data', (chunk: Buffer) => { body += chunk.toString() })
        req.on('end', () => {
          try {
            const { fileName, config } = JSON.parse(body) as { fileName: string; config: unknown }

            if (!fileName || !config) {
              res.statusCode = 400
              res.end(JSON.stringify({ error: 'Missing fileName or config' }))
              return
            }

            if (fileName.includes('..') || fileName.includes('/')) {
              res.statusCode = 400
              res.end(JSON.stringify({ error: 'Invalid fileName' }))
              return
            }

            const terrainDir = path.resolve(__dirname, 'src/terrain')
            const filePath = path.join(terrainDir, `${fileName}.json`)
            const realPath = fs.realpathSync(filePath)

            const json = JSON.stringify(config, null, 2) + '\n'
            fs.writeFileSync(realPath, json, 'utf-8')

            res.setHeader('Content-Type', 'application/json')
            res.end(JSON.stringify({ ok: true, path: realPath }))
          } catch (e) {
            res.statusCode = 500
            res.end(JSON.stringify({ error: String(e) }))
          }
        })
      })
    },
  }
}

export default defineConfig({
  plugins: [vue(), terrainSavePlugin()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5174,
    host: true,
  },
})
