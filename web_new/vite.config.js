import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { execSync } from 'node:child_process';
function getGitCommitHash() {
    try {
        return execSync('git rev-parse --short HEAD').toString().trim();
    }
    catch {
        return 'unknown';
    }
}
export default defineConfig({
    plugins: [vue()],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url))
        }
    },
    define: {
        __APP_VERSION__: JSON.stringify(process.env.npm_package_version || '0.1.0'),
        __BUILD_TIME__: JSON.stringify(new Date().toISOString()),
        __COMMIT_HASH__: JSON.stringify(getGitCommitHash()),
    },
    server: {
        port: 5173,
        host: true,
        proxy: {
            '/api': {
                target: 'http://localhost:8080',
                changeOrigin: true,
                rewrite: (path) => path.replace(/^\/api/, '')
            }
        }
    },
    build: {
        target: 'esnext',
        sourcemap: true,
        chunkSizeWarningLimit: 600,
        rollupOptions: {
            output: {
                manualChunks(id) {
                    if (id.includes('node_modules/pixi.js'))
                        return 'pixi';
                    if (id.includes('/src/network/proto/'))
                        return 'network-proto';
                    if (id.includes('/src/network/handlers/'))
                        return 'network-handlers';
                    if (id.includes('/src/network/'))
                        return 'network-core';
                    if (id.includes('/src/game/terrain/'))
                        return 'game-terrain';
                    if (id.includes('/src/game/objects/'))
                        return 'game-objects';
                    if (id.includes('/src/game/'))
                        return 'game-core';
                    if (id.includes('node_modules'))
                        return 'vendor';
                }
            }
        }
    }
});
