<template>
  <DashboardLayout>
    <Teleport v-if="mounted && activeSection !== 'nodes'" to="#header-action"
      ><button
        class="btn-submit"
        :disabled="saving || loading || !!loadError || !username"
        @click="handleSave"
      >
        <Save :size="14" />{{ saving ? '正在保存…' : '保存设置' }}
      </button></Teleport
    >
    <div v-if="loading" class="settings-loading" role="status">正在加载系统设置…</div>
    <div v-else-if="loadError" class="inline-error" role="alert">
      <p>{{ loadError }}</p>
      <button class="secondary" @click="fetchConfig">重新加载设置</button>
    </div>
    <div v-else class="settings-container">
      <nav class="settings-tabs" aria-label="设置分类">
        <button
          :class="{ active: activeSection === 'general' }"
          :aria-pressed="activeSection === 'general'"
          @click="activeSection = 'general'"
        >
          <Layout :size="15" />基本设置
        </button>
        <button
          :class="{ active: activeSection === 'registries' }"
          :aria-pressed="activeSection === 'registries'"
          @click="activeSection = 'registries'"
        >
          <Database :size="15" />镜像仓库
        </button>
        <button
          v-if="systemStore.info.isController"
          :class="{ active: activeSection === 'nodes' }"
          :aria-pressed="activeSection === 'nodes'"
          @click="activeSection = 'nodes'"
        >
          <Network :size="15" />节点与任务
        </button>
      </nav>
      <section v-show="activeSection === 'general'" class="settings-card" aria-label="基本设置">
        <header>
          <h2>实例信息</h2>
          <p>设置控制台的名称、外观和管理员凭据。</p>
        </header>
        <div class="settings-field">
          <label for="hostname"><Layout :size="14" />主机名称</label
          ><input id="hostname" v-model="hostname" placeholder="留空使用系统主机名" autocomplete="off" />
          <p class="hint">显示在节点选择器中，方便区分不同实例。</p>
        </div>
        <div class="settings-field">
          <label for="admin-username"><User :size="14" />管理员用户名</label>
          <input
            id="admin-username"
            v-model="username"
            maxlength="64"
            autocomplete="username"
            placeholder="admin"
          />
          <p class="hint">1–64 位，以字母或数字开头，支持字母、数字、下划线、点、@ 和连字符。</p>
        </div>
        <div class="settings-field">
          <label for="new-password"><Lock :size="14" />管理员密码</label
          ><input
            id="new-password"
            v-model="password"
            type="password"
            placeholder="不修改请留空"
            autocomplete="new-password"
          />
          <p class="hint">留空保持现有密码。</p>
        </div>
        <div class="settings-field">
          <label><User :size="14" />控制台标志</label
          ><input
            ref="fileInputRef"
            type="file"
            accept="image/png,image/jpeg"
            aria-label="上传控制台标志"
            hidden
            @change="handleLogoUpload"
          /><button
            type="button"
            class="logo-wrapper"
            aria-label="上传新的控制台标志"
            @click="fileInputRef?.click()"
          >
            <img :src="logo || '/logo.png'" alt="当前标志" /><span><Upload :size="18" /></span>
          </button>
          <p class="hint">支持 PNG、JPG，点击图标上传，保存后生效。</p>
        </div>
      </section>
      <section v-show="activeSection === 'registries'" class="settings-card" aria-label="镜像仓库认证">
        <header>
          <h2>镜像仓库认证</h2>
          <p>配置拉取私有镜像所需的访问凭据。</p>
        </header>
        <div class="registry-list">
          <p v-if="!registries.length" class="settings-empty">尚未添加仓库认证，公开镜像无需配置。</p>
          <div v-for="(registry, index) in registries" :key="index" class="registry-item">
            <label
              >仓库域名<input v-model="registry.domain" placeholder="例如 docker.io" autocomplete="off"
            /></label>
            <label
              >用户名<input v-model="registry.username" placeholder="仓库用户名" autocomplete="off"
            /></label>
            <label
              >密码 / Token<input
                v-model="registry.password"
                type="password"
                placeholder="留空不修改"
                autocomplete="new-password"
            /></label>
            <button
              class="icon-button remove-registry"
              :aria-label="`删除仓库 ${registry.domain || index + 1}`"
              title="删除仓库"
              @click="removeRegistry(index)"
            >
              <Trash2 :size="16" />
            </button>
          </div>
          <button class="secondary" @click="addRegistry"><Plus :size="15" />添加仓库认证</button>
          <p class="hint">Agent 仅能获取授权范围内的仓库凭据。请先保存仓库，再前往“节点与任务”授权。</p>
        </div>
      </section>
      <section
        v-if="activeSection === 'nodes' && systemStore.info.isController"
        class="settings-card"
        aria-label="节点与任务"
      >
        <NodeManager :registries="registries.map((registry) => registry.domain).filter(Boolean)" />
      </section>
    </div>
  </DashboardLayout>
</template>
<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { Save, Lock, Layout, User, Upload, Plus, Trash2, Database, Network } from 'lucide-vue-next';
import NodeManager from '@/components/NodeManager.vue';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useToastStore } from '@/stores/toast';
import { useHeaderStore } from '@/stores/header';
import { useSystemStore } from '@/stores/system';
import { request } from '@/utils/request';

const toastStore = useToastStore();
const headerStore = useHeaderStore();
const systemStore = useSystemStore();

const activeSection = ref('general');
const hostname = ref('');
const username = ref('admin');
const password = ref('');
const logo = ref('');
const registries = ref<any[]>([]);
const loading = ref(true);
const loadError = ref('');
const saving = ref(false);
const mounted = ref(false);
const fileInputRef = ref<HTMLInputElement | null>(null);

const fetchConfig = async () => {
  loading.value = true;
  try {
    const data = await request('/api/settings/system', { localOnly: true });
    username.value = data.username || 'admin';
    if (data.hostname) hostname.value = data.hostname;
    if (data.logo) logo.value = data.logo;
    registries.value = (data.registries || []).map((r: any) => ({ ...r, password: '' })); // don't show existing passwords

    loadError.value = '';
    loading.value = false;
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : '加载设置失败，请重试';
    loading.value = false;
  }
};

const addRegistry = () => {
  registries.value.push({ domain: '', username: '', password: '' });
};

const removeRegistry = (index: number) => {
  registries.value.splice(index, 1);
};

const handleSave = async () => {
  saving.value = true;
  try {
    const updates: any = { username: username.value };
    if (hostname.value) updates.hostname = hostname.value;
    if (password.value) updates.password = password.value;
    if (logo.value && logo.value.startsWith('data:')) {
      updates.logo = logo.value;
    }

    updates.registries = registries.value;

    await request('/api/settings/system', {
      method: 'POST',
      localOnly: true,
      body: JSON.stringify(updates)
    });

    toastStore.addToast('设置已保存', 'success');
    await systemStore.fetchSystemInfo();
    if (password.value) password.value = '';
  } catch (err) {
    toastStore.addToast(err instanceof Error ? err.message : '保存失败', 'error');
  } finally {
    saving.value = false;
  }
};

const handleLogoUpload = (e: Event) => {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0];
  if (file) {
    // Convert to base64
    const reader = new FileReader();
    reader.onloadend = () => {
      logo.value = reader.result as string;
    };
    reader.readAsDataURL(file);
  }
};

onMounted(() => {
  mounted.value = true;
  headerStore.set('系统设置', false);
  fetchConfig();
});

onUnmounted(() => {
  headerStore.reset();
});
</script>
<style scoped>
.settings-container {
  width: 100%;
}
.settings-tabs {
  display: flex;
  gap: 8px;
  border-bottom: 1px solid var(--border);
  padding-bottom: 16px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}
.settings-tabs button {
  background: transparent;
  color: var(--system-gray);
  border-color: transparent;
  font-size: 12px;
}
.settings-tabs button.active {
  background: var(--accent-soft);
  border-color: var(--border);
  color: var(--blue);
}
.settings-card {
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
  padding: 24px;
}
.settings-card header {
  padding-bottom: 20px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 24px;
}
.settings-card h2 {
  font-size: 14px;
  font-weight: 500;
}
.settings-card header p,
.hint {
  color: var(--system-gray);
  font-size: 12px;
  line-height: 1.8;
  margin-top: 6px;
}
.settings-field {
  max-width: 600px;
  margin-bottom: 24px;
}
.settings-field:last-child {
  margin-bottom: 0;
}
.settings-field > label {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 12px;
  margin-bottom: 10px;
}
.settings-field > label svg {
  color: var(--system-gray);
}
.logo-wrapper {
  position: relative;
  width: 76px;
  height: 76px;
  padding: 6px;
  background: var(--field);
}
.logo-wrapper img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.logo-wrapper > span {
  display: none;
  position: absolute;
  inset: 0;
  place-items: center;
  background: #0008;
  color: #fff;
}
.logo-wrapper:hover > span,
.logo-wrapper:focus-visible > span {
  display: grid;
}
.registry-list {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 16px;
}
.registry-item {
  width: 100%;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr)) 34px;
  gap: 12px;
  padding: 16px;
  background: var(--window-background);
  border: 1px solid var(--border);
  border-radius: 4px;
  align-items: end;
}
.registry-item label {
  font-size: 12px;
  color: var(--system-gray);
}
.registry-item input {
  margin-top: 8px;
}
.remove-registry {
  color: var(--system-red);
}
.settings-empty,
.settings-loading {
  padding: 28px 0;
  color: var(--system-gray);
}
@media (max-width: 900px) {
  .registry-item {
    grid-template-columns: minmax(0, 1fr);
  }
  .remove-registry {
    justify-self: end;
  }
}
@media (max-width: 720px) {
  .settings-card {
    padding: 18px;
  }
  .settings-tabs button {
    flex: 1;
  }
}
</style>
