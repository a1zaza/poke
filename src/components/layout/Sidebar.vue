<template>
  <aside class="sidebar">
    <div class="brand">
      <img :src="info.logo" alt="Poke 标志" width="32" height="32" class="logo" />
      <div class="brand-info">
        <span class="brand-text">Poke</span>
      </div>
    </div>

    <div class="menu-scroll">
      <div v-for="(group, index) in menuGroups" :key="index" class="menu-group">
        <div class="menu-label">{{ group.label }}</div>
        <router-link
          v-for="item in group.items"
          :key="item.id"
          :to="item.href"
          class="menu-item"
          :class="{ active: isActive(item.href) }"
        >
          <component :is="item.icon" :size="18" class="icon" />
          {{ item.label }}
        </router-link>
      </div>
    </div>

    <div class="sidebar-footer">
      <div class="footer-actions">
        <button class="action-btn" @click="handleLogout" title="退出登录">
          <LogOut :size="16" />
        </button>
        <button class="action-btn" @click="toggleTheme" title="切换主题">
          <Sun v-if="theme === 'dark'" :size="18" />
          <Moon v-else :size="18" />
        </button>
      </div>

      <span v-if="systemLoading" class="hostname">正在加载节点…</span>
      <button v-else-if="systemError" class="backend-retry" @click="systemStore.fetchSystemInfo">
        节点加载失败，点击重试
      </button>
      <div v-else-if="nodes && nodes.some(n => !n.is_local)" class="backend-selector">
        <Select
          v-model="currentNodeId"
          :options="nodes.map(b => ({
            label: b.is_local ? (info.hostname || b.name) : `${b.name}（${b.status === 'online' ? '在线' : '离线'}）`,
            value: b.id
          }))"
          placement="top"
          @update:modelValue="handleNodeSwitch($event as string)"
        />
      </div>
    </div>

    <ConfirmModal
      :isOpen="showLogoutConfirm"
      title="确定退出登录吗？"
      message="退出后需要重新输入密码才能访问。"
      @confirm="executeLogout"
      @close="showLogoutConfirm = false"
    />
  </aside>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useThemeStore } from '@/stores/theme';
import { useSystemStore } from '@/stores/system';
import { useToastStore } from '@/stores/toast';
import { useAuthStore } from '@/stores/auth';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';
import {
  LayoutDashboard,
  Layers,
  Box,
  HardDrive,
  Network,
  Settings,
  LogOut,
  Sun,
  Moon
} from 'lucide-vue-next';
import Select from '@/components/ui/Select.vue';

const route = useRoute();
const router = useRouter();
const themeStore = useThemeStore();
const systemStore = useSystemStore();
const toastStore = useToastStore();
const authStore = useAuthStore();

const theme = computed(() => themeStore.theme);
const info = computed(() => systemStore.info);
const nodes = computed(() => systemStore.nodes);
const currentNodeId = ref(systemStore.currentNodeId);
const systemLoading = computed(() => systemStore.loading);
const systemError = computed(() => systemStore.error);
const showLogoutConfirm = ref(false);

watch(() => systemStore.currentNodeId, (id) => {
    currentNodeId.value = id;
});

let nodeRefresh: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
    void systemStore.fetchSystemInfo();
    nodeRefresh = setInterval(() => void systemStore.fetchSystemInfo(), 10000);
});
onUnmounted(() => { if (nodeRefresh) clearInterval(nodeRefresh); });

const handleLogout = () => {
    showLogoutConfirm.value = true;
};

const executeLogout = async () => {
    try { await authStore.logout(); }
    catch { toastStore.addToast('退出请求未确认，请重试或关闭页面', 'error'); }
    finally { void router.replace('/login'); }
};

const handleNodeSwitch = async (id: string) => {
    if (id === 'local') {
        systemStore.setNode(id);
        return;
    }

    const node = nodes.value.find(n => n.id === id);
    if (node?.status === 'online') {
        systemStore.setNode(id);
    } else {
        toastStore.addToast('节点离线，请等待 Agent 主动连接', 'error');
        currentNodeId.value = systemStore.currentNodeId;
    }
}

const toggleTheme = themeStore.toggleTheme;

const menuGroups = computed(() => {
    const groups = [
      {
        label: '集群',
        items: [
          { id: 'overview', href: '/overview', label: '概览', icon: LayoutDashboard },
          { id: 'projects', href: '/projects', label: '项目', icon: Layers },
        ]
      },
      {
        label: '资源',
        items: [
          { id: 'containers', href: '/containers', label: '容器', icon: Box },
          { id: 'images', href: '/images', label: '镜像', icon: HardDrive },
          { id: 'networks', href: '/networks', label: '网络', icon: Network },
          { id: 'volumes', href: '/volumes', label: '存储卷', icon: HardDrive },
        ]
      },
      {
        label: '系统',
        items: [
          // { id: 'logs', href: '/logs', label: '日志', icon: Activity },
          { id: 'settings', href: '/settings', label: '设置', icon: Settings },
        ]
      }
    ];

    return groups;
});

const isActive = (href: string) => {
  if (href === '/containers' && route.path === '/') return true;
  if (route.path === href) return true;
  if (route.path.startsWith(href + '/')) return true;
  return false;
};
</script>

<style scoped>
.sidebar {
  width: 220px;
  background: var(--sidebar-background);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-right: 1px solid var(--glass-border);
  display: flex;
  flex-direction: column;
  padding: 12px;
  height: 100%;
  color: var(--foreground);
  transition: background 0.3s ease, border-color 0.3s ease;
}

.brand {
  display: flex;
  align-items: center;
  padding: 12px 12px;
  margin-bottom: 12px;
  height: 52px;
  gap: 12px;
}

.logo {
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
}

.brand-info {
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.brand-text {
  font-weight: 600;
  font-size: 14px;
  letter-spacing: -0.5px;
  line-height: 1.2;
}

.menu-scroll {
  flex: 1;
  overflow-y: auto;
  padding-right: 4px;
}

.menu-group {
  margin-bottom: 24px;
}

.menu-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--system-gray);
  padding: 0 12px;
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  color: var(--foreground);
  opacity: 0.7;
  transition: all 0.2s;
  margin-bottom: 2px;
  text-decoration: none;
}

.menu-item:hover {
  background: var(--item-hover);
  color: var(--foreground);
  opacity: 1;
}

.menu-item.active {
  background: var(--system-blue) !important;
  color: #fff !important;
  opacity: 1 !important;
}

.icon {
  opacity: 0.9;
}

.sidebar-footer {
  margin-top: auto;
  margin-left: -12px;
  margin-right: -12px;
  margin-bottom: -12px;
  padding: 12px 12px;
  border-top: 1px solid var(--glass-border);
  display: flex;
  align-items: center;
  transition: border-color 0.3s ease;
  overflow: visible;
}

.backend-selector {
  flex: 1;
  min-width: 0;
  margin-left: 12px;
  display: flex;
}

.backend-selector :deep(.select-container) {
  width: 100%;
}

.backend-selector :deep(.select-trigger) {
  background: var(--item-hover);
  border: none;
  padding: 0 8px;
  font-size: 11px;
  height: 26px;
  border-radius: 4px;
}

.hostname {
  font-size: 10px;
  color: var(--system-gray);
  font-weight: 500;
}

.backend-retry {
  margin-left: 12px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--system-red);
  font-size: 10px;
  cursor: pointer;
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.action-btn {
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--system-gray);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  transition: all 0.2s;
}

.action-btn:hover {
  background: var(--item-hover);
  color: var(--foreground);
}
</style>
