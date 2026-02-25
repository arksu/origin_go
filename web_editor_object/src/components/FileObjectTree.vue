<template>
  <div class="panel-wrap">
    <h3>Object Files</h3>
    <ul class="file-list">
      <li
        v-for="(file, idx) in store.files"
        :key="file.fileName"
        :class="{ active: idx === store.selectedFileIndex }"
        @click="store.selectFile(idx)"
      >
        {{ file.fileName }}.json
      </li>
    </ul>

    <template v-if="store.selectedWorkingRoot">
      <div class="toolbar">
        <button class="small-btn" @click="moveSelectedToRoot">Move To Root</button>
        <button class="small-btn" @click="store.flattenSelectedWrapper">Flatten Wrapper</button>
      </div>
      <div class="subpath-row">
        <input
          v-model="subPathInput"
          class="subpath-input"
          placeholder="add sub path (e.g. runestone)"
          @keydown.enter.prevent="onAddSubPath"
        />
        <button class="small-btn" @click="onAddSubPath">Add</button>
      </div>
      <div class="rename-row">
        <input
          v-model="renameInput"
          class="subpath-input"
          placeholder="rename selected object key"
          :disabled="!store.selectedObjectPath"
          @keydown.enter.prevent="onRenameSelected"
        />
        <button class="small-btn" :disabled="!store.selectedObjectPath" @click="onRenameSelected">Rename</button>
      </div>
      <div class="path-line">
        <span class="label">Selected:</span>
        <code>{{ store.selectedObjectPath || '(none)' }}</code>
      </div>
      <div class="drop-root" @dragover.prevent @drop.prevent="onDropRoot">
        Drop here to move node to file root
      </div>
      <ObjectNodeTree
        :nodes="store.selectedTree"
        :selected-path="store.selectedObjectPath"
        @select="store.selectObjectPath"
        @move-as-child="onMoveAsChild"
      />
    </template>

    <div v-if="store.validationIssues.length > 0" class="issues">
      <div v-for="issue in store.validationIssues" :key="`${issue.code}:${issue.message}`" class="issue">
        {{ issue.message }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useObjectEditorStore } from '@/stores/objectEditorStore'
import ObjectNodeTree from '@/components/ObjectNodeTree.vue'

const store = useObjectEditorStore()
const subPathInput = ref('')
const renameInput = ref('')

watch(
  () => store.selectedObjectPath,
  (path) => {
    const parts = path.split('.').filter(Boolean)
    renameInput.value = parts[parts.length - 1] ?? ''
  },
  { immediate: true },
)

function onAddSubPath(): void {
  const value = subPathInput.value.trim()
  if (!value) return
  store.addSubPathToSelected(value)
  subPathInput.value = ''
}

function moveSelectedToRoot(): void {
  if (!store.selectedObjectPath) return
  store.moveNodeToRoot(store.selectedObjectPath)
}

function onMoveAsChild(payload: { sourcePath: string; targetParentPath: string }): void {
  store.moveNodeAsChild(payload.sourcePath, payload.targetParentPath)
}

function onDropRoot(e: DragEvent): void {
  const sourcePath = e.dataTransfer?.getData('text/plain')?.trim()
  if (!sourcePath) return
  store.moveNodeToRoot(sourcePath)
}

function onRenameSelected(): void {
  if (!store.selectedObjectPath) return
  store.renameSelectedObject(renameInput.value)
}
</script>

<style scoped>
.panel-wrap {
  padding: 8px;
  height: 100%;
  overflow: auto;
}

h3 {
  margin: 0 0 8px;
  color: #aaa;
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.file-list {
  list-style: none;
  padding: 0;
  margin: 0 0 12px;
}

.file-list li {
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  color: #ccc;
}

.file-list li:hover {
  background: #333;
}

.file-list li.active {
  background: #2563eb;
  color: #fff;
}

.toolbar,
.subpath-row,
.rename-row {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.small-btn {
  border: 1px solid #555;
  background: #333;
  color: #ddd;
  border-radius: 4px;
  padding: 5px 8px;
  font-size: 12px;
  cursor: pointer;
}

.small-btn:hover {
  background: #3d3d3d;
}

.small-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.subpath-input {
  flex: 1;
  min-width: 0;
  background: #1e1e1e;
  border: 1px solid #555;
  border-radius: 4px;
  color: #ddd;
  padding: 5px 8px;
  font-size: 12px;
}

.path-line {
  margin-bottom: 8px;
  font-size: 11px;
  color: #aaa;
  word-break: break-all;
}

.path-line .label {
  margin-right: 4px;
}

.drop-root {
  border: 1px dashed #555;
  border-radius: 4px;
  color: #888;
  padding: 6px;
  font-size: 11px;
  text-align: center;
  margin-bottom: 8px;
}

.issues {
  margin-top: 10px;
  border-top: 1px solid #333;
  padding-top: 8px;
}

.issue {
  font-size: 11px;
  color: #fca5a5;
  margin-top: 4px;
}
</style>
