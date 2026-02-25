<template>
  <div class="panel-wrap">
    <h3>Diff / Save</h3>

    <div class="summary">
      <div>JSON changes: <strong>{{ store.hasUnsavedJsonChanges ? 'yes' : 'no' }}</strong></div>
      <div>Pending PNGs: <strong>{{ store.pendingGeneratedImagesForSelectedFile.length }}</strong></div>
    </div>

    <div v-if="store.pendingGeneratedImagesForSelectedFile.length > 0" class="pending-list">
      <div class="pending-title">Generated shadow images</div>
      <div
        v-for="draft in store.pendingGeneratedImagesForSelectedFile"
        :key="draft.key"
        class="pending-row"
      >
        <code>{{ draft.relPath }}</code>
      </div>
    </div>

    <div class="actions">
      <button class="refresh-btn" :disabled="store.diffLoading" @click="store.refreshDiffNow">
        {{ store.diffLoading ? 'Refreshing...' : 'Refresh Diff' }}
      </button>
      <button class="save-btn" :disabled="!store.hasUnsavedChanges || store.saving" @click="onSave">
        {{ store.saving ? 'Saving...' : 'Save' }}
      </button>
    </div>

    <div v-if="store.saveStatus" :class="['save-status', store.saveStatus.ok ? 'ok' : 'err']">
      {{ store.saveStatus.message }}
    </div>

    <pre class="diff-box">{{ store.diffText || '(no json diff)' }}</pre>
  </div>
</template>

<script setup lang="ts">
import { useObjectEditorStore } from '@/stores/objectEditorStore'

const store = useObjectEditorStore()

async function onSave(): Promise<void> {
  try {
    await store.saveCurrentFile()
  } catch {
    // status shown by store
  }
}
</script>

<style scoped>
.panel-wrap {
  padding: 8px;
  height: 100%;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

h3 {
  margin: 0;
  font-size: 13px;
  color: #aaa;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.summary {
  padding: 8px;
  border: 1px solid #333;
  border-radius: 6px;
  background: #252525;
  font-size: 12px;
  color: #ccc;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.pending-list {
  border: 1px solid #333;
  border-radius: 6px;
  background: #252525;
  padding: 8px;
}

.pending-title {
  color: #aaa;
  font-size: 11px;
  margin-bottom: 6px;
}

.pending-row {
  font-size: 11px;
  color: #ddd;
  word-break: break-all;
  margin-top: 3px;
}

.actions {
  display: flex;
  gap: 6px;
}

.refresh-btn,
.save-btn {
  flex: 1;
  border-radius: 4px;
  border: 1px solid #555;
  padding: 8px 10px;
  cursor: pointer;
  font-size: 12px;
}

.refresh-btn {
  background: #333;
  color: #ddd;
}

.save-btn {
  background: #2563eb;
  color: #fff;
}

.refresh-btn:disabled,
.save-btn:disabled {
  background: #2c2c2c;
  color: #777;
  cursor: not-allowed;
}

.save-status {
  font-size: 12px;
  padding: 6px 8px;
  border-radius: 4px;
  border: 1px solid transparent;
}

.save-status.ok {
  background: #163424;
  color: #9ae6b4;
  border-color: #1f6b46;
}

.save-status.err {
  background: #3b1919;
  color: #fecaca;
  border-color: #7f1d1d;
}

.diff-box {
  flex: 1;
  min-height: 260px;
  overflow: auto;
  margin: 0;
  padding: 10px;
  background: #181818;
  border: 1px solid #333;
  border-radius: 6px;
  color: #d1d5db;
  font-size: 11px;
  line-height: 1.4;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
