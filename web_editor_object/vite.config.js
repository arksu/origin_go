import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import fs from 'node:fs';
import path from 'node:path';
function sendJson(res, status, data) {
    res.statusCode = status;
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(data));
}
function readJsonBody(req) {
    return new Promise((resolve, reject) => {
        let body = '';
        req.on('data', (chunk) => {
            body += chunk.toString();
        });
        req.on('end', () => {
            try {
                resolve(JSON.parse(body));
            }
            catch (error) {
                reject(error);
            }
        });
        req.on('error', reject);
    });
}
function isPlainObject(value) {
    return typeof value === 'object' && value !== null && !Array.isArray(value);
}
function prettyJson(value) {
    return JSON.stringify(value, null, 2) + '\n';
}
function splitLines(text) {
    const lines = text.split('\n');
    if (lines.length > 0 && lines[lines.length - 1] === '')
        lines.pop();
    return lines;
}
function generateUnifiedDiff(beforeText, afterText, fileName) {
    if (beforeText === afterText)
        return '';
    const a = splitLines(beforeText);
    const b = splitLines(afterText);
    let prefix = 0;
    while (prefix < a.length && prefix < b.length && a[prefix] === b[prefix])
        prefix++;
    let suffix = 0;
    while (suffix < a.length - prefix &&
        suffix < b.length - prefix &&
        a[a.length - 1 - suffix] === b[b.length - 1 - suffix]) {
        suffix++;
    }
    const context = 3;
    const aChangeEnd = a.length - suffix;
    const bChangeEnd = b.length - suffix;
    const aHunkStart = Math.max(0, prefix - context);
    const bHunkStart = Math.max(0, prefix - context);
    const aHunkEnd = Math.min(a.length, aChangeEnd + context);
    const bHunkEnd = Math.min(b.length, bChangeEnd + context);
    const out = [
        `--- ${fileName}.json`,
        `+++ ${fileName}.json`,
        `@@ -${aHunkStart + 1},${aHunkEnd - aHunkStart} +${bHunkStart + 1},${bHunkEnd - bHunkStart} @@`,
    ];
    for (let i = aHunkStart; i < prefix; i++)
        out.push(` ${a[i]}`);
    for (let i = prefix; i < aChangeEnd; i++)
        out.push(`-${a[i]}`);
    for (let i = prefix; i < bChangeEnd; i++)
        out.push(`+${b[i]}`);
    for (let i = aChangeEnd; i < aHunkEnd; i++)
        out.push(` ${a[i]}`);
    return out.join('\n') + '\n';
}
function listPngsRecursive(rootDir, relPrefix = '') {
    const output = [];
    if (!fs.existsSync(rootDir))
        return output;
    for (const entry of fs.readdirSync(rootDir, { withFileTypes: true })) {
        const nextFsPath = path.join(rootDir, entry.name);
        const nextRel = relPrefix ? `${relPrefix}/${entry.name}` : entry.name;
        if (entry.isDirectory()) {
            output.push(...listPngsRecursive(nextFsPath, nextRel));
            continue;
        }
        if (entry.isFile() && entry.name.toLowerCase().endsWith('.png')) {
            output.push({ relPath: nextRel.replaceAll(path.sep, '/') });
        }
    }
    return output;
}
function safeResolveInside(baseDir, relativePath) {
    const resolved = path.resolve(baseDir, relativePath);
    const normalizedBase = path.resolve(baseDir);
    if (!(resolved === normalizedBase || resolved.startsWith(normalizedBase + path.sep))) {
        throw new Error('Path escapes base directory');
    }
    return resolved;
}
function objectEditorPlugin() {
    return {
        name: 'object-editor-api',
        configureServer(server) {
            const projectRoot = __dirname;
            const webNewRoot = path.resolve(projectRoot, '../web_new');
            const objectsDir = path.resolve(webNewRoot, 'src/game/objects');
            const gameAssetsDir = path.resolve(webNewRoot, 'public/assets/game');
            const objAssetsDir = path.resolve(gameAssetsDir, 'obj');
            server.middlewares.use('/__api/object-editor/init', async (req, res) => {
                if (req.method !== 'GET') {
                    sendJson(res, 405, { error: 'Method not allowed' });
                    return;
                }
                try {
                    const files = fs.readdirSync(objectsDir)
                        .filter((name) => name.endsWith('.json'))
                        .sort((a, b) => a.localeCompare(b))
                        .map((name) => {
                        const fullPath = path.join(objectsDir, name);
                        const raw = fs.readFileSync(fullPath, 'utf-8');
                        return {
                            fileName: name.replace(/\.json$/, ''),
                            json: JSON.parse(raw),
                        };
                    });
                    const images = listPngsRecursive(objAssetsDir, 'obj').sort((a, b) => a.relPath.localeCompare(b.relPath));
                    sendJson(res, 200, { files, images });
                }
                catch (error) {
                    sendJson(res, 500, { error: String(error) });
                }
            });
            server.middlewares.use('/__api/object-editor/preview-diff', async (req, res) => {
                if (req.method !== 'POST') {
                    sendJson(res, 405, { error: 'Method not allowed' });
                    return;
                }
                try {
                    const body = await readJsonBody(req);
                    if (!body.fileName || !/^[a-zA-Z0-9_-]+$/.test(body.fileName)) {
                        sendJson(res, 400, { error: 'Invalid fileName' });
                        return;
                    }
                    if (!isPlainObject(body.json)) {
                        sendJson(res, 400, { error: 'JSON root must be an object' });
                        return;
                    }
                    const filePath = safeResolveInside(objectsDir, `${body.fileName}.json`);
                    const before = fs.readFileSync(filePath, 'utf-8');
                    const after = prettyJson(body.json);
                    const unifiedDiff = generateUnifiedDiff(before, after, body.fileName);
                    sendJson(res, 200, { unifiedDiff, hasChanges: unifiedDiff.length > 0 });
                }
                catch (error) {
                    sendJson(res, 500, { error: String(error) });
                }
            });
            server.middlewares.use('/__api/object-editor/save', async (req, res) => {
                if (req.method !== 'POST') {
                    sendJson(res, 405, { error: 'Method not allowed' });
                    return;
                }
                try {
                    const body = await readJsonBody(req);
                    if (!body.fileName || !/^[a-zA-Z0-9_-]+$/.test(body.fileName)) {
                        sendJson(res, 400, { error: 'Invalid fileName' });
                        return;
                    }
                    if (!isPlainObject(body.json)) {
                        sendJson(res, 400, { error: 'JSON root must be an object' });
                        return;
                    }
                    const jsonPath = safeResolveInside(objectsDir, `${body.fileName}.json`);
                    const generatedImages = Array.isArray(body.generatedImages) ? body.generatedImages : [];
                    const imageWrites = [];
                    for (const img of generatedImages) {
                        if (!img || typeof img.relPath !== 'string' || typeof img.pngBase64 !== 'string') {
                            throw new Error('Invalid generatedImages entry');
                        }
                        if (!img.relPath.startsWith('obj/') || !img.relPath.endsWith('.png')) {
                            throw new Error(`Invalid image relPath: ${img.relPath}`);
                        }
                        if (img.relPath.includes('..') || path.isAbsolute(img.relPath)) {
                            throw new Error(`Invalid image relPath: ${img.relPath}`);
                        }
                        const absPath = safeResolveInside(gameAssetsDir, img.relPath);
                        const bytes = Buffer.from(img.pngBase64, 'base64');
                        imageWrites.push({ relPath: img.relPath, absPath, bytes });
                    }
                    for (const img of imageWrites) {
                        fs.mkdirSync(path.dirname(img.absPath), { recursive: true });
                    }
                    for (const img of imageWrites) {
                        fs.writeFileSync(img.absPath, img.bytes);
                    }
                    fs.writeFileSync(jsonPath, prettyJson(body.json), 'utf-8');
                    sendJson(res, 200, {
                        ok: true,
                        savedJsonPath: jsonPath,
                        savedImages: imageWrites.map((w) => w.absPath),
                    });
                }
                catch (error) {
                    sendJson(res, 500, { error: String(error) });
                }
            });
        },
    };
}
export default defineConfig({
    plugins: [vue(), objectEditorPlugin()],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url)),
        },
    },
    server: {
        port: 5175,
        host: true,
    },
});
