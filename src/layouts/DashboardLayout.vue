<template>
  <div class="console-shell" @keydown.esc="closeNavigation">
    <button
      v-if="navigationOpen && mobile"
      class="sidebar-mask"
      aria-label="关闭导航"
      @click="closeNavigation"
    />
    <Sidebar
      :open="navigationOpen"
      :mobile="mobile"
      :inert="mobile && !navigationOpen"
      @navigate="closeNavigation"
    />
    <main id="main-content" class="console-main" :inert="mobile && navigationOpen">
      <header class="topbar">
        <div class="breadcrumb">
          <button
            ref="menuButton"
            class="icon-button mobile-menu"
            aria-label="打开导航"
            :aria-expanded="navigationOpen"
            aria-controls="console-sidebar"
            @click="openNavigation"
          >
            <Menu :size="19" />
          </button>
          <span class="breadcrumb-root">控制台</span><ChevronRight :size="13" />
          <strong>{{ sectionTitle }}</strong>
        </div>
        <div class="topbar-actions">
          <div id="header-action" class="header-actions" />
        </div>
      </header>
      <div class="workspace-content">
        <section class="page-heading">
          <div class="page-title">
            <span class="eyebrow">{{ sectionLabel }}</span>
            <div class="page-title-row">
              <button
                v-if="headerStore.showBack"
                class="icon-button"
                aria-label="返回上一页"
                @click="router.back()"
              >
                <ArrowLeft :size="18" />
              </button>
              <h1>{{ headerStore.title || defaultTitle }}</h1>
            </div>
            <p>{{ description }}</p>
          </div>
        </section>
        <div class="page-body"><slot /></div>
        <footer class="console-footer">
          <span>Poke · Docker 管理控制台</span
          ><span>{{ systemStore.info.isController ? '多节点管理' : '本机管理' }}</span>
        </footer>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft, ChevronRight, Menu } from 'lucide-vue-next';
import Sidebar from '@/components/layout/Sidebar.vue';
import { useHeaderStore } from '@/stores/header';
import { useSystemStore } from '@/stores/system';

const route = useRoute();
const router = useRouter();
const headerStore = useHeaderStore();
const systemStore = useSystemStore();
const navigationOpen = ref(false);
const mobile = ref(false);
const menuButton = ref<HTMLButtonElement | null>(null);
const sections: Record<string, { title: string; label: string; description: string }> = {
  overview: {
    title: '运行概览',
    label: '节点概览',
    description: '了解当前节点的资源使用与 Docker 运行情况。'
  },
  containers: {
    title: '容器管理',
    label: '容器资源',
    description: '查看运行状态，管理容器生命周期、日志与终端。'
  },
  projects: {
    title: 'Compose 项目',
    label: '应用编排',
    description: '从配置到部署，集中管理你的 Compose 应用。'
  },
  images: { title: '镜像管理', label: '镜像资源', description: '拉取、检查和清理当前节点的容器镜像。' },
  networks: { title: '网络管理', label: '网络资源', description: '查看 Docker 网络、驱动及子网配置。' },
  volumes: { title: '存储卷管理', label: '存储资源', description: '查看持久化数据卷及其挂载信息。' },
  settings: { title: '基本设置', label: '控制台管理', description: '设置实例外观与管理员登录凭据。' },
  registries: { title: '镜像仓库', label: '访问认证', description: '集中管理私有镜像仓库的登录凭据。' },
  nodes: { title: '节点与任务', label: '集群管理', description: '接入 Agent、分配仓库授权并跟踪远程任务。' },
  logs: { title: '系统日志', label: '运行记录', description: '查看实例的运行记录与事件。' }
};
const section = computed(() => sections[route.path.split('/')[1] || 'overview'] || sections.overview!);
const sectionTitle = computed(() => section.value.title);
const sectionLabel = computed(() => section.value.label);
const description = computed(() => section.value.description);
const defaultTitle = computed(() => {
  const parts = route.path.split('/').filter(Boolean);
  if (parts[0] === 'projects' && parts[1] === 'new') return '新建 Compose 项目';
  if (parts[0] === 'projects' && parts[1]) return parts[1];
  if (parts[0] === 'containers' && parts[1]) {
    return `${parts[1].slice(0, 12)} · ${parts[2] === 'logs' ? '容器日志' : parts[2] === 'exec' ? '交互终端' : '容器详情'}`;
  }
  return sectionTitle.value;
});
const closeNavigation = () => {
  const wasOpen = navigationOpen.value;
  navigationOpen.value = false;
  if (wasOpen) void nextTick(() => menuButton.value?.focus());
};
const openNavigation = async () => {
  navigationOpen.value = true;
  await nextTick();
  document.querySelector<HTMLElement>('#console-sidebar a')?.focus();
};
let query: MediaQueryList;
const updateViewport = () => {
  mobile.value = query.matches;
  if (!mobile.value) navigationOpen.value = false;
};
onMounted(() => {
  query = window.matchMedia('(max-width: 720px)');
  updateViewport();
  query.addEventListener('change', updateViewport);
});
onUnmounted(() => query?.removeEventListener('change', updateViewport));
</script>

<style scoped>
.console-shell {
  display: flex;
  margin: 20px;
  width: calc(100% - 40px);
  height: calc(100dvh - 40px);
  min-height: 0;
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
  box-shadow: 0 12px 45px #0002;
}
.console-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--window-background);
}
.topbar {
  min-height: 58px;
  flex: none;
  padding: 10px 26px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  border-bottom: 1px solid var(--border);
  background: var(--window-header);
}
.topbar-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 16px;
  min-width: 0;
  max-width: 100%;
  margin-left: auto;
}
.breadcrumb {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  font-size: 12px;
  color: var(--system-gray);
}
.breadcrumb strong {
  color: var(--foreground);
  font-weight: 400;
  white-space: nowrap;
}
.workspace-content {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 30px max(28px, calc((100% - var(--content-max-width)) / 2)) 18px;
  display: flex;
  flex-direction: column;
  scrollbar-gutter: stable;
}
.page-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 24px;
  flex: none;
}
.page-title {
  min-width: 0;
}
.page-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 7px;
}
.page-title-row > .icon-button {
  margin-left: -6px;
}
h1 {
  font-size: 24px;
  font-weight: 500;
  letter-spacing: -0.5px;
  overflow-wrap: anywhere;
  line-height: 1.5;
}
.page-title p {
  color: var(--system-gray);
  font-size: 12px;
  margin-top: 5px;
  line-height: 1.8;
}
.header-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
  justify-content: flex-end;
  max-width: 100%;
}
.header-actions:empty {
  display: none;
}
.header-actions :deep(.header-controls),
.header-actions :deep(.controls) {
  justify-content: flex-end;
  max-width: 100%;
}
.page-body {
  flex: 1;
  min-width: 0;
}
.console-footer {
  display: flex;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  color: var(--system-gray);
  font-size: 11px;
  margin-top: 32px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}
.mobile-menu,
.sidebar-mask {
  display: none;
}
@media (max-width: 1100px) {
  .page-heading {
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .topbar-actions {
    flex-wrap: wrap;
    gap: 8px;
  }
  .workspace-content {
    padding: 24px 20px 16px;
  }
}
@media (max-width: 720px) {
  .console-shell {
    margin: 0;
    width: 100%;
    height: 100dvh;
    border: 0;
    border-radius: 0;
  }
  .topbar {
    padding: 6px 12px;
    gap: 8px;
  }
  .mobile-menu {
    display: inline-flex;
  }
  .breadcrumb-root {
    display: none;
  }
  .workspace-content {
    padding: 22px 16px 16px;
  }
  .page-heading {
    gap: 16px;
    margin-bottom: 20px;
  }
  h1 {
    font-size: 22px;
  }
  .header-actions :deep(.header-controls),
  .header-actions :deep(.controls) {
    display: contents;
  }
  .sidebar-mask {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 39;
    background: #0008;
    border: 0;
    border-radius: 0;
  }
}
</style>
