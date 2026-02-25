<template>
  <ul class="tree-list">
    <li v-for="node in nodes" :key="node.path" class="tree-item">
      <div
        class="tree-row"
        :class="{ active: node.path === selectedPath, resource: node.isResource }"
        draggable="true"
        @click.stop="$emit('select', node.path)"
        @dragstart="onDragStart(node.path, $event)"
        @dragover.prevent
        @drop.prevent="onDrop(node.path, $event)"
      >
        <span class="tree-key">{{ node.key }}</span>
        <span v-if="node.isResource" class="badge">layers</span>
      </div>

      <ObjectNodeTree
        v-if="node.children.length > 0"
        :nodes="node.children"
        :selected-path="selectedPath"
        @select="$emit('select', $event)"
        @move-as-child="$emit('move-as-child', $event)"
      />
    </li>
  </ul>
</template>

<script setup lang="ts">
import type { ObjectTreeNode } from '@/types/objectEditor'

defineOptions({ name: 'ObjectNodeTree' })

const props = defineProps<{
  nodes: ObjectTreeNode[]
  selectedPath: string
}>()

const emit = defineEmits<{
  select: [path: string]
  'move-as-child': [{ sourcePath: string; targetParentPath: string }]
}>()

function onDragStart(path: string, e: DragEvent): void {
  e.dataTransfer?.setData('text/plain', path)
  e.dataTransfer!.effectAllowed = 'move'
}

function onDrop(targetParentPath: string, e: DragEvent): void {
  const sourcePath = e.dataTransfer?.getData('text/plain')?.trim()
  if (!sourcePath || sourcePath === targetParentPath) return
  emit('move-as-child', { sourcePath, targetParentPath })
}
</script>

<style scoped>
.tree-list {
  list-style: none;
  margin: 0;
  padding: 0 0 0 12px;
}

.tree-item {
  margin: 2px 0;
}

.tree-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  border-radius: 4px;
  cursor: pointer;
  border: 1px solid transparent;
}

.tree-row:hover {
  background: #313131;
}

.tree-row.active {
  background: #1f3d31;
  border-color: #28c76f;
}

.tree-row.resource .tree-key {
  color: #d5ffe7;
}

.tree-key {
  font-size: 12px;
  color: #ddd;
  word-break: break-all;
}

.badge {
  margin-left: auto;
  font-size: 10px;
  color: #8dd4a7;
  border: 1px solid #2f6f4b;
  border-radius: 999px;
  padding: 0 6px;
}
</style>
