<template>
  <div class="layout">
    <Sidebar />
    <main class="main">
      <header class="header">
        <div class="title-area">
          <button v-if="headerStore.showBack" @click="router.back()" class="global-back-btn">
             <ArrowLeft :size="18" />
          </button>
          <h3>{{ headerStore.title || defaultTitle }}</h3>
        </div>
        <div class="header-action" id="header-action">
          <!-- Connect custom header actions via a portal or store later -->
        </div>
      </header>
      <div class="content">
        <slot />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import Sidebar from '@/components/layout/Sidebar.vue';
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useHeaderStore } from '@/stores/header';
import { ArrowLeft } from 'lucide-vue-next';

const route = useRoute();
const router = useRouter();
const headerStore = useHeaderStore();

// Reset header on route change (optional, but good practice if views don't cleanup)
// But views usually set it onMounted.
// Let's keep a default computed title as fallback.

const defaultTitle = computed(() => {
    const path = route.path;
    const parts = path.split('/').filter(Boolean);

    if (path === '/' || path === '/containers') return '容器';
    if (path === '/overview') return '概览';
    if (path === '/projects') return '项目';
    if (path === '/images') return '镜像';
    if (path === '/networks') return '网络';
    if (path === '/volumes') return '存储卷';
    if (path === '/settings') return '设置';

    if (parts[0] === 'projects' && parts[1] === 'new') return '新建项目';

    if (parts[0] === 'containers' && parts[1]) {
        const id = parts[1].substring(0, 12);
        if (parts[2] === 'logs') return `${id} - 日志查看器`;
        if (parts[2] === 'exec') return `${id} - 终端`;
        return `${id} - 容器详情`;
    }

    if (parts[0] === 'projects' && parts[1]) {
        return `${decodeURIComponent(parts[1])}`;
    }

    return 'Poke';
});
</script>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  width: 100vw;
  background: var(--background);
  color: var(--foreground);
  transition: background 0.3s ease;
}

.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--window-background);
  overflow: hidden;
  position: relative;
}

.header {
  height: 64px;
  border-bottom: 1px solid var(--glass-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 32px;
  background: var(--window-header);
  backdrop-filter: blur(20px);
  z-index: 10;
  transition: background 0.3s ease, border-color 0.3s ease;
}

.title-area {
  display: flex;
  align-items: center;
  gap: 12px;
}

.title-area h3 {
  font-size: 16px;
  font-weight: 600;
  opacity: 0.9;
}

.global-back-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--foreground);
  display: flex;
  align-items: center;
  opacity: 0.6;
  padding: 4px;
  border-radius: 6px;
  transition: all 0.2s;
}

.global-back-btn:hover {
  opacity: 1;
  background: var(--item-hover);
}

.content {
  flex: 1;
  overflow: auto;
  padding: 24px;
}
</style>
