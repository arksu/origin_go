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
const buildId = process.env.BUILD_ID || new Date().toISOString().replace(/[-:TZ.]/g, '');
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
                entryFileNames: `assets/[name]-${buildId}-[hash].js`,
                chunkFileNames: `assets/[name]-${buildId}-[hash].js`,
                assetFileNames: `assets/[name]-${buildId}-[hash][extname]`,
                manualChunks(id) {
                    if (!id.includes('node_modules'))
                        return;
                    if (id.includes('/node_modules/pixi.js/'))
                        return 'vendor-pixi';
                    if (id.includes('/node_modules/@esotericsoftware/'))
                        return 'vendor-spine';
                    if (id.includes('/node_modules/vue/') || id.includes('/node_modules/pinia/'))
                        return 'vendor-vue';
                    if (id.includes('/node_modules/protobufjs/') || id.includes('/node_modules/@protobufjs/'))
                        return 'vendor-proto';
                    if (id.includes('/node_modules/dayjs/') || id.includes('/node_modules/lodash'))
                        return 'vendor-utils';
                    return 'vendor';
                }
            }
        }
    }
});
