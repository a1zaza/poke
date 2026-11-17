<template>
  <aside id="console-sidebar" class="sidebar" :class="{ open }" aria-label="主导航" @keydown="keepFocus">
    <router-link to="/overview" class="brand" @click="emit('navigate')">
      <img :src="info.logo" alt="Poke 标志" width="36" height="36" />
      <div><strong>Poke</strong><small>容器与节点管理</small></div>
    </router-link>
    <div class="node-panel">
      <span class="node-caption">当前节点</span>
      <span v-if="systemLoading && !systemStore.ready" class="node-hint">正在加载节点…</span>
      <button v-else-if="systemError" class="node-retry" @click="systemStore.fetchSystemInfo">
        加载失败，点击重试
      </button>
      <Select
        v-else-if="nodes.length"
        v-model="currentNodeId"
        label="选择管理节点"
        :options="
          nodes.map((node) => ({
            label: `${node.is_local ? info.hostname || node.name : node.name} · ${node.status === 'online' ? '在线' : '离线'}`,
            value: node.id
          }))
        "
        @update:modelValue="handleNodeSwitch($event as string)"
      />
      <router-link v-else class="node-hint" to="/settings" @click="emit('navigate')"
        >添加你的第一个节点 →</router-link
      >
    </div>
    <nav class="menu-scroll" aria-label="功能导航">
      <div v-for="group in menuGroups" :key="group.label" class="menu-group">
        <div class="menu-label">{{ group.label }}</div>
        <router-link
          v-for="item in group.items"
          :key="item.id"
          :to="item.href"
          class="menu-item"
          :class="{ active: isActive(item.href) }"
          :aria-current="isActive(item.href) ? 'page' : undefined"
          @click="emit('navigate')"
        >
          <component :is="item.icon" :size="17" :stroke-width="1.65" /><span>{{ item.label }}</span>
        </router-link>
      </div>
    </nav>
    <div class="sidebar-footer">
      <div class="account">
        <span class="avatar"><ShieldCheck :size="16" /></span>
        <div>
          管理员<small>{{ info.isController ? '多节点管理' : '本机管理' }}</small>
        </div>
      </div>
      <button
        class="icon-button"
        @click="toggleTheme"
        :title="theme === 'dark' ? '切换浅色主题' : '切换深色主题'"
        :aria-label="theme === 'dark' ? '切换浅色主题' : '切换深色主题'"
      >
        <Sun v-if="theme === 'dark'" :size="17" /><Moon v-else :size="17" />
      </button>
      <button class="icon-button" @click="handleLogout" title="退出登录" aria-label="退出登录">
        <LogOut :size="17" />
      </button>
    </div>
    <ConfirmModal
      :isOpen="showLogoutConfirm"
      title="退出登录"
      message="退出后需要重新输入管理员密码才能访问。"
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
  ShieldCheck,
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

const props = defineProps<{ open: boolean; mobile: boolean }>();
const emit = defineEmits<{ navigate: [] }>();
const keepFocus = (event: KeyboardEvent) => {
  if (!props.mobile || !props.open || event.key !== 'Tab') return;
  const controls = [
    ...(event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>(
      'a, button:not(:disabled), [tabindex="0"]'
    )
  ];
  const first = controls[0],
    last = controls[controls.length - 1];
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first?.focus();
  }
};
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

watch(
  () => systemStore.currentNodeId,
  (id) => {
    currentNodeId.value = id;
  }
);

let nodeRefresh: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  void systemStore.fetchSystemInfo();
  nodeRefresh = setInterval(() => void systemStore.fetchSystemInfo(), 10000);
});
onUnmounted(() => {
  if (nodeRefresh) clearInterval(nodeRefresh);
});

const handleLogout = () => {
  showLogoutConfirm.value = true;
};

const executeLogout = async () => {
  try {
    await authStore.logout();
  } catch {
    toastStore.addToast('退出请求未确认，请重试或关闭页面', 'error');
  } finally {
    void router.replace('/login');
  }
};

const handleNodeSwitch = async (id: string) => {
  if (id === 'local') {
    systemStore.setNode(id);
    return;
  }

  const node = nodes.value.find((n) => n.id === id);
  if (node?.status === 'online') {
    systemStore.setNode(id);
  } else {
    toastStore.addToast('节点离线，请等待 Agent 主动连接', 'error');
    currentNodeId.value = systemStore.currentNodeId;
  }
};

const toggleTheme = themeStore.toggleTheme;

const menuGroups = computed(() => {
  const groups = [
    {
      label: '集群',
      items: [
        { id: 'overview', href: '/overview', label: '运行概览', icon: LayoutDashboard },
        { id: 'projects', href: '/projects', label: 'Compose 项目', icon: Layers }
      ]
    },
    {
      label: '资源',
      items: [
        { id: 'containers', href: '/containers', label: '容器管理', icon: Box },
        { id: 'images', href: '/images', label: '镜像管理', icon: HardDrive },
        { id: 'networks', href: '/networks', label: '网络管理', icon: Network },
        { id: 'volumes', href: '/volumes', label: '存储卷管理', icon: HardDrive }
      ]
    },
    {
      label: '系统',
      items: [
        // { id: 'logs', href: '/logs', label: '日志', icon: Activity },
        { id: 'settings', href: '/settings', label: '系统设置', icon: Settings }
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
  flex: none;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 26px 14px 0;
  border-right: 1px solid var(--border);
  background: var(--sidebar-background);
}
.brand {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 0 8px;
  color: var(--foreground);
  text-decoration: none;
}
.brand img {
  flex: none;
}
.brand strong {
  display: block;
  font-size: 22px;
  font-weight: 600;
  letter-spacing: -0.6px;
}
.brand small {
  display: block;
  color: var(--system-gray);
  font-size: 11px;
  margin-top: 2px;
}
.node-panel {
  margin: 24px 0 20px;
  padding: 12px 8px;
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 9px;
}
.node-caption {
  font-size: 11px;
  color: var(--system-gray);
}
.node-hint {
  font-size: 12px;
  color: var(--system-gray);
}
.node-retry {
  text-align: left;
  font-size: 12px;
  color: var(--system-red);
}
.menu-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.menu-group {
  margin-bottom: 22px;
}
.menu-label {
  padding: 0 12px 9px;
  font-size: 11px;
  color: var(--system-gray);
  letter-spacing: 0.7px;
}
.menu-item {
  display: flex;
  gap: 11px;
  align-items: center;
  padding: 9px 12px;
  min-height: 38px;
  color: var(--system-gray);
  font-size: 12px;
  border-left: 2px solid transparent;
  border-radius: 0 4px 4px 0;
  text-decoration: none;
  margin-bottom: 4px;
}
.menu-item:hover {
  color: var(--foreground);
  background: var(--item-hover);
}
.menu-item.active {
  border-left-color: var(--system-blue);
  background: var(--accent-soft);
  color: var(--foreground);
}
.menu-item.active svg {
  color: var(--blue);
}
.sidebar-footer {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: auto;
  padding: 16px 0;
  border-top: 1px solid var(--border);
}
.account {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  font-size: 12px;
}
.account > div {
  min-width: 0;
}
.account small {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: block;
  color: var(--system-gray);
  font-size: 10px;
  margin-top: 2px;
}
.avatar {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 1px solid var(--border);
  background: var(--field);
  border-radius: 6px;
  color: var(--blue);
  flex: none;
}
@media (max-width: 1100px) and (min-width: 721px) {
  .sidebar {
    width: 205px;
  }
}
@media (max-width: 720px) {
  .sidebar {
    position: fixed;
    top: 0;
    bottom: 0;
    left: 0;
    width: min(290px, calc(100vw - 48px));
    z-index: 40;
    transform: translateX(-100%);
    transition: transform 0.18s;
    padding-top: 24px;
  }
  .sidebar.open {
    transform: translateX(0);
  }
  .menu-item {
    min-height: 44px;
    font-size: 13px;
  }
}
</style>
