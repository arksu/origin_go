<template>
  <div class="app-layout">
    <aside class="panel panel-left">
      <ObjectList />
    </aside>
    <main class="panel-center">
      <RenderView />
    </main>
    <aside class="panel panel-right">
      <LayerHierarchy />
    </aside>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useTerrainStore } from '@/stores/terrainStore'
import { loadTerrainFiles } from '@/loaders/terrainLoader'
import ObjectList from '@/components/ObjectList.vue'
import LayerHierarchy from '@/components/LayerHierarchy.vue'
import RenderView from '@/components/RenderView.vue'

const store = useTerrainStore()

onMounted(() => {
  const files = loadTerrainFiles()
  store.loadFiles(files)
})
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html,
body,
#app {
  width: 100%;
  height: 100%;
  overflow: hidden;
  background: #1e1e1e;
  color: #ccc;
  font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
  font-size: 14px;
}

.app-layout {
  display: flex;
  width: 100%;
  height: 100%;
}

.panel {
  background: #252525;
  border-right: 1px solid #333;
  overflow-y: auto;
}

.panel-left {
  width: 220px;
  flex-shrink: 0;
}

.panel-right {
  width: 280px;
  flex-shrink: 0;
  border-right: none;
  border-left: 1px solid #333;
}

.panel-center {
  flex: 1;
  min-width: 0;
}
</style>
