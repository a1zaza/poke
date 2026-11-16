<template>
  <DashboardLayout>
    <Teleport to="#header-action" v-if="mounted">
      <button
        @click="handleSave"
        class="btn-submit"
        :disabled="saving"
      >
        <Save :size="14" fill="currentColor" />
        {{ saving ? '正在保存...' : '保存设置' }}
      </button>
    </Teleport>

    <div v-if="loading" class="loading">加载中...</div>

    <div v-else class="settings-container">
      <div class="card">
        <!-- Hostname Section -->
        <div class="section">
          <label class="label">
            <Layout :size="14" class="icon" />
            主机名称 (Hostname)
          </label>
          <input
            type="text"
            class="input"
            :placeholder="hostname || 'System Hostname'"
            v-model="hostname"
            autocomplete="off"
          />
          <p class="hint">设置自定义主机名，留空则使用系统默认</p>
        </div>

        <!-- Password Section -->
        <div class="section">
          <label class="label">
            <Lock :size="14" class="icon" />
            修改管理员密码
          </label>
          <input
            type="password"
            class="input"
            placeholder="如果不修改请留空"
            v-model="password"
            autocomplete="off"
          />
          <p class="hint">留空保持原密码不变</p>
        </div>

        <!-- Logo Section -->
        <div class="section">
          <label class="label">
            <User :size="14" class="icon" />
            页面 Logo
          </label>

          <div style="margin-top: 12px">
            <input
              type="file"
              accept="image/*"
              @change="handleLogoUpload"
              ref="fileInputRef"
              style="display: none"
            />

            <div
              class="logo-wrapper"
              @click="fileInputRef?.click()"
              title="点击上传新 Logo"
            >
              <img
                :src="logo || '/logo.png'"
                alt="Current Logo"
                class="logo-image"
              />
              <div class="upload-overlay">
                <Upload :size="24" />
              </div>
            </div>

            <p class="hint" style="margin-top: 8px">
              支持 PNG, JPG。点击图片上传，保存后生效。
            </p>
          </div>
        </div>

        <NodeManager v-if="systemStore.info.isController" :registries="registries.map(r => r.domain).filter(Boolean)" />

        <!-- Registry Auth Section -->
        <div class="section">
          <label class="label">
            <Database :size="14" class="icon" />
            私有镜像仓库认证 (Registry Auth)
          </label>
          <div class="backend-manager">
            <div v-for="(r, index) in registries" :key="index" class="backend-item">
              <Database :size="14" class="icon-small" />
              <div class="backend-inputs">
                <input v-model="r.domain" placeholder="仓库域名 (e.g. docker.io)" class="input slim" autocomplete="off" />
                <input v-model="r.username" placeholder="用户名" class="input slim" autocomplete="off" />
                <input v-model="r.password" type="password" placeholder="密码/Token (留空不修改)" class="input slim" autocomplete="off" />
              </div>
              <button @click="removeRegistry(index)" class="btn-remove">
                <Trash2 :size="14" />
              </button>
            </div>
            <button @click="addRegistry" class="btn-add">
              <Plus :size="14" />
              添加认证信息
            </button>
          </div>
          <p class="hint">配置私有仓库的访问凭证。Agent 仅能在授权范围内，为当前操作获取这些凭证。请先保存仓库，再在节点注册时选择授权。</p>
        </div>
      </div>
    </div>

  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { Save, Lock, Layout, User, Upload, Plus, Trash2, Database } from 'lucide-vue-next';
import NodeManager from '@/components/NodeManager.vue';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useToastStore } from '@/stores/toast';
import { useHeaderStore } from '@/stores/header';
import { useSystemStore } from '@/stores/system';
import { request } from '@/utils/request';

const toastStore = useToastStore();
const headerStore = useHeaderStore();
const systemStore = useSystemStore();

const hostname = ref('');
const password = ref('');
const logo = ref('');
const registries = ref<any[]>([]);
const loading = ref(true);
const saving = ref(false);
const mounted = ref(false);
const fileInputRef = ref<HTMLInputElement | null>(null);

const fetchConfig = async () => {
  try {
    const data = await request('/api/settings/system', { localOnly: true });
    if (data.hostname) hostname.value = data.hostname;
    if (data.logo) logo.value = data.logo;
    registries.value = (data.registries || [])
      .map((r: any) => ({ ...r, password: '' })); // don't show existing passwords

    loading.value = false;
  } catch (err) {
    toastStore.addToast('加载配置失败', 'error');
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
    const updates: any = {};
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
    toastStore.addToast('保存失败', 'error');
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
  padding: 0;
  max-width: 800px;
  margin: 0 auto;
  animation: fadeIn 0.3s ease-out;
}

.loading {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
  color: var(--system-gray);
}

.card {
  background: var(--card-background);
  border: 1px solid var(--card-border);
  border-radius: 12px;
  padding: 32px;
  backdrop-filter: blur(20px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.section {
  margin-bottom: 24px;
  padding-bottom: 24px;
  border-bottom: 1px solid var(--glass-border);
}

.section:last-child {
  border-bottom: none;
  padding-bottom: 0;
  margin-bottom: 0;
}

.label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 12px;
  color: var(--system-gray);
}

.icon {
  color: var(--system-blue);
  opacity: 0.8;
}

.icon-small {
  color: var(--system-gray);
  margin-top: 10px;
}

.input {
  width: 100%;
  background: var(--input-background);
  border: 1px solid var(--card-border);
  color: var(--foreground);
  padding: 12px 16px;
  border-radius: 8px;
  font-size: 14px;
  transition: all 0.2s;
  outline: none;
}

.input:focus {
  border-color: var(--system-blue);
}

.hint {
  margin-top: 8px;
  font-size: 12px;
  color: var(--system-gray);
  opacity: 0.8;
}

.logo-wrapper {
  width: 80px;
  height: 80px;
  border-radius: 12px;
  border: 1px solid var(--card-border);
  position: relative;
  overflow: hidden;
  cursor: pointer;
  background: var(--card-background);
}

.logo-wrapper:hover .upload-overlay {
  opacity: 1;
}

.upload-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
  color: white;
}

.logo-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  padding: 8px;
}

.btn-submit {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--system-blue);
  color: white;
  border: none;
  padding: 6px 16px;
  border-radius: 6px;
  font-weight: 500;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
  height: 32px;
}

.btn-submit:hover:not(:disabled) {
  filter: brightness(1.1);
  transform: translateY(-1px);
}

.backend-manager {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.backend-item {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 12px;
  background: var(--item-hover);
  border-radius: 8px;
  border: 1px solid var(--glass-border);
}

.backend-inputs {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 2fr 1.5fr;
  gap: 8px;
}

.input.slim {
  padding: 8px 12px;
  font-size: 13px;
}

.btn-remove {
  background: transparent;
  border: none;
  color: var(--system-red);
  cursor: pointer;
  padding: 8px;
  opacity: 0.6;
  transition: opacity 0.2s;
  margin-top: 2px;
}

.btn-remove:hover {
  opacity: 1;
}

.btn-add {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  background: transparent;
  border: 1px dashed var(--system-blue);
  color: var(--system-blue);
  padding: 10px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
  margin-top: 8px;
}

.btn-add:hover {
  background: rgba(0, 122, 255, 0.05);
}

.btn-submit:disabled {
  opacity: 0.7;
  cursor: not-allowed;
  transform: none;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
