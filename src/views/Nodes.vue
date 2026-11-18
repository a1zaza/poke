<template>
  <DashboardLayout>
    <div v-if="loading" class="settings-state" role="status">正在加载节点配置…</div>
    <div v-else-if="loadError" class="settings-state inline-error" role="alert">
      <p>{{ loadError }}</p>
      <button class="secondary" @click="load">重新加载配置</button>
    </div>
    <NodeManager v-else-if="controller" :registries="registries" />
    <div v-else class="settings-panel settings-empty">
      <Network :size="28" />
      <h2>当前为本机管理模式</h2>
      <p>节点接入与远程任务管理仅在 Controller 模式下提供。</p>
      <router-link class="button secondary" to="/overview">返回运行概览</router-link>
    </div>
  </DashboardLayout>
</template>
<script setup lang="ts">
import { ref } from 'vue';
import { Network } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import NodeManager from '@/components/NodeManager.vue';
import { useSettingsPage } from '@/composables/useSettingsPage';
const controller = ref(false);
const registries = ref<string[]>([]);
const { loading, loadError, load } = useSettingsPage((data) => {
  controller.value = data.mode === 'controller';
  registries.value = (data.registries || []).map((r) => r.domain).filter(Boolean);
});
</script>
