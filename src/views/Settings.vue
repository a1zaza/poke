<template>
  <DashboardLayout>
    <Teleport v-if="mounted" to="#header-action">
      <button
        class="btn-submit"
        :disabled="saving || loading || !!loadError || !username.trim()"
        @click="handleSave"
      >
        <Save :size="14" />{{ saving ? '正在保存…' : '保存设置' }}
      </button>
    </Teleport>
    <div v-if="loading" class="settings-state" role="status">正在加载基本设置…</div>
    <div v-else-if="loadError" class="settings-state inline-error" role="alert">
      <p>{{ loadError }}</p>
      <button class="secondary" @click="load">重新加载设置</button>
    </div>
    <div v-else class="settings-page">
      <section class="settings-panel settings-split" aria-labelledby="instance-title">
        <header class="settings-intro">
          <Layout :size="20" />
          <h2 id="instance-title">实例与外观</h2>
          <p>为控制台设置名称和标志，让不同环境更容易辨认。</p>
        </header>
        <div class="settings-fields">
          <label class="settings-field" for="hostname"
            ><span>主机名称</span
            ><input
              id="hostname"
              v-model="hostname"
              placeholder="留空使用系统主机名"
              autocomplete="off"
              :disabled="saving"
            /><small>显示在左侧节点选择器中，留空后恢复系统主机名。</small></label
          >
          <div class="settings-field">
            <span>控制台标志</span>
            <div class="settings-logo-row">
              <img :src="logo || '/logo.png'" alt="控制台标志预览" />
              <div>
                <input
                  ref="fileInput"
                  type="file"
                  accept="image/png,image/jpeg"
                  hidden
                  aria-label="上传控制台标志"
                  @change="uploadLogo"
                />
                <button class="secondary" :disabled="saving" @click="fileInput?.click()">
                  <Upload :size="14" />上传图片
                </button>
                <p class="settings-hint">支持 PNG、JPG，保存后生效。</p>
              </div>
            </div>
          </div>
        </div>
      </section>
      <section class="settings-panel settings-split" aria-labelledby="account-title">
        <header class="settings-intro">
          <ShieldCheck :size="20" />
          <h2 id="account-title">管理员账号</h2>
          <p>管理控制台登录凭据，修改后请使用新凭据登录。</p>
        </header>
        <div class="settings-fields">
          <label class="settings-field" for="admin-username"
            ><span>用户名</span
            ><input
              id="admin-username"
              v-model="username"
              maxlength="128"
              autocomplete="username"
              :disabled="saving"
            /><small>1–64 个字符，以中文、字母或数字开头，支持中文、字母、数字及 _ . @ -。</small></label
          >
          <label class="settings-field" for="new-password"
            ><span>新密码</span
            ><input
              id="new-password"
              v-model="password"
              type="password"
              placeholder="不修改请留空"
              autocomplete="new-password"
              :disabled="saving"
            /><small>留空保持现有密码，保存后不会显示已设置的密码。</small></label
          >
        </div>
      </section>
      <p class="settings-hint">以上设置对整个控制台生效，与当前选择的节点无关。</p>
    </div>
  </DashboardLayout>
</template>
<script setup lang="ts">
import { ref } from 'vue';
import { Save, Layout, Upload, ShieldCheck } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useSettingsPage } from '@/composables/useSettingsPage';
import { useSystemStore } from '@/stores/system';
import { useToastStore } from '@/stores/toast';
const hostname = ref('');
const username = ref('');
const password = ref('');
const logo = ref('');
const fileInput = ref<HTMLInputElement | null>(null);
const system = useSystemStore();
const toast = useToastStore();
const { loading, loadError, saving, mounted, load, save } = useSettingsPage((data) => {
  hostname.value = data.hostname || '';
  username.value = data.username || 'admin';
  logo.value = data.logo || '/logo.png';
});
async function handleSave() {
  const updates: Record<string, unknown> = { hostname: hostname.value, username: username.value };
  if (password.value) updates.password = password.value;
  if (logo.value.startsWith('data:')) updates.logo = logo.value;
  if (await save(updates)) {
    password.value = '';
    await system.fetchSystemInfo();
  }
}
function uploadLogo(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  if (!['image/png', 'image/jpeg'].includes(file.type)) {
    toast.addToast('请选择 PNG 或 JPG 图片', 'error');
    input.value = '';
    return;
  }
  const reader = new FileReader();
  reader.onload = () => {
    logo.value = reader.result as string;
  };
  reader.onerror = () => toast.addToast('图片读取失败，请重新选择', 'error');
  reader.readAsDataURL(file);
  input.value = '';
}
</script>
