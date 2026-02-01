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
                manualChunks: {
                    'pixi': ['pixi.js'],
                    'game-system': [
                        '@/game',
                        '@/network'
                    ],
                    'vendor': [
                        'vue',
                        'vue-router',
                        'pinia',
                        'axios',
                        'protobufjs'
                    ]
                }
            }
        }
    }
});
