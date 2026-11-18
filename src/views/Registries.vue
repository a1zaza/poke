<template>
  <DashboardLayout>
    <Teleport v-if="mounted" to="#header-action">
      <button class="secondary" :disabled="loading || !!loadError || saving" @click="addRegistry">
        <Plus :size="14" />添加仓库
      </button>
      <button class="btn-submit" :disabled="loading || !!loadError || saving" @click="handleSave">
        <Save :size="14" />{{ saving ? '正在保存…' : '保存仓库' }}
      </button>
    </Teleport>
    <div v-if="loading" class="settings-state" role="status">正在加载镜像仓库…</div>
    <div v-else-if="loadError" class="settings-state inline-error" role="alert">
      <p>{{ loadError }}</p>
      <button class="secondary" @click="load">重新加载仓库</button>
    </div>
    <div v-else class="settings-page">
      <aside class="settings-notice">
        <strong>私有镜像访问凭据</strong>
        <p>
          公开镜像无需配置。填写仓库域名及登录凭据，密码或 Token
          留空将保留该域名已保存的凭据。删除仓库后需保存才会生效。
        </p>
        <p v-if="system.info.isController">
          保存后，请在 <router-link to="/nodes">节点与任务</router-link> 为对应 Agent 授权仓库。
        </p>
      </aside>
      <section class="settings-panel" aria-labelledby="registries-title">
        <header class="settings-panel-title">
          <h2 id="registries-title">仓库认证</h2>
          <span class="settings-count">{{ registries.length }} 个仓库</span>
        </header>
        <div v-if="!registries.length" class="settings-empty">
          <Database :size="28" />
          <h2>尚未配置镜像仓库</h2>
          <p>需要访问私有镜像时，添加仓库认证即可。</p>
          <button class="secondary" @click="addRegistry"><Plus :size="14" />添加第一个仓库</button>
        </div>
        <article v-for="(registry, index) in registries" :key="registry.key" class="registry-editor">
          <header class="registry-editor-header">
            <strong>{{ registry.domain || `新仓库 ${index + 1}` }}</strong><button
              class="icon-button"
              :disabled="saving"
              :aria-label="`删除仓库 ${registry.domain || index + 1}`"
              title="删除仓库"
              @click="registries.splice(index, 1)"
            >
              <Trash2 :size="15" />
            </button>
          </header>
          <div class="registry-editor-fields">
            <label class="settings-field"
              ><span>仓库域名</span
              ><input
                v-model="registry.domain"
                placeholder="例如 ghcr.io"
                autocomplete="off"
                :disabled="saving"
              /><small>只填域名与端口，不包含协议和镜像路径。</small></label
            >
            <label class="settings-field"
              ><span>用户名</span
              ><input
                v-model="registry.username"
                placeholder="仓库用户名"
                autocomplete="off"
                :disabled="saving"
            /></label>
            <label class="settings-field"
              ><span>密码 / Token</span
              ><input
                v-model="registry.password"
                type="password"
                :placeholder="
                  registry.originalDomain === registry.domain.trim() && registry.originalDomain
                    ? '留空保留已保存凭据'
                    : '请输入密码或访问 Token'
                "
                autocomplete="new-password"
                :disabled="saving"
            /></label>
          </div>
        </article>
      </section>
    </div>
  </DashboardLayout>
</template>
<script setup lang="ts">
import { ref } from 'vue';
import { Database, Plus, Save, Trash2 } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useSettingsPage } from '@/composables/useSettingsPage';
import { useSystemStore } from '@/stores/system';
import { useToastStore } from '@/stores/toast';
const system = useSystemStore();
const toast = useToastStore();
let nextKey = 0;
const registries = ref<
  Array<{ key: number; domain: string; username: string; password: string; originalDomain: string }>
>([]);
const { loading, loadError, saving, mounted, load, save } = useSettingsPage((data) => {
  registries.value = (data.registries || []).map((r) => ({
    ...r,
    key: nextKey++,
    password: '',
    originalDomain: r.domain
  }));
});
function addRegistry() {
  registries.value.push({ key: nextKey++, domain: '', username: '', password: '', originalDomain: '' });
}
async function handleSave() {
  const domains = registries.value.map((r) => r.domain.trim());
  if (domains.some((domain) => !domain || /[\s/]/.test(domain)) || new Set(domains).size !== domains.length) {
    toast.addToast('请填写有效且不重复的仓库域名，不包含协议和路径', 'error');
    return;
  }
  const updates = registries.value.map((r, index) => ({
    domain: domains[index],
    username: r.username,
    password: r.password
  }));
  if (await save({ registries: updates })) {
    registries.value.forEach((r, index) => {
      r.domain = domains[index]!;
      r.password = '';
      r.originalDomain = r.domain;
    });
  }
}
</script>
