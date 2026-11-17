<template>
  <DashboardLayout>
    <div class="project-new">
      <Teleport to="#header-action" v-if="mounted">
        <button @click="handleCreate" class="btn-submit" :disabled="loading || !name || !content">
          <Play v-if="!loading" :size="14" fill="currentColor" />
          <span>{{ loading ? '正在创建...' : '部署并启动' }}</span>
        </button>
      </Teleport>

      <form class="form" @submit.prevent="handleCreate">
        <div class="card">
          <div class="section">
            <label class="label" for="project-name">
              <CheckCircle2 :size="14" class="icon" />
              项目名称
            </label>
            <input
              id="project-name"
              v-model="name"
              type="text"
              class="input"
              placeholder="例如：my-web-app"
              :disabled="loading"
              autocomplete="off"
            />
            <p class="hint">
              项目将保存在 <code>/poke/projects/{{ name || '...' }}</code> 目录下
            </p>
          </div>

          <div class="section">
            <label class="label" for="project-compose">
              <FileCode :size="14" class="icon" />
              Docker Compose 配置 (YAML)
            </label>
            <textarea
              id="project-compose"
              v-model="content"
              class="textarea"
              placeholder="services:&#10;  web:&#10;    image: nginx&#10;    ports:&#10;      - '80:80'"
              spellcheck="false"
              :disabled="loading"
            ></textarea>
          </div>

          <div class="info">
            <AlertCircle :size="14" />
            <span>创建后将自动执行 <code>docker compose up -d</code></span>
          </div>
        </div>
      </form>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { Play, CheckCircle2, FileCode, AlertCircle } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useToastStore } from '@/stores/toast';
import { useHeaderStore } from '@/stores/header';
import { request } from '@/utils/request';

const router = useRouter();
const toastStore = useToastStore();
const name = ref('');
const content = ref('services:\n  ');
const loading = ref(false);
const mounted = ref(false);

const handleCreate = async () => {
  if (!name.value || !content.value) return;

  // Simple validation
  if (!/^[a-zA-Z0-9-_]+$/.test(name.value)) {
    toastStore.addToast('项目名称只能包含字母、数字、下划线和连字符', 'error');
    return;
  }

  loading.value = true;
  try {
    await request('/api/projects', {
      method: 'POST',
      body: JSON.stringify({ name: name.value, content: content.value })
    });

    toastStore.addToast('项目创建成功', 'success');
    router.push('/projects');
  } catch (err: any) {
    toastStore.addToast(err.message, 'error');
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  mounted.value = true;
  useHeaderStore().set('新建项目', true);
});

onUnmounted(() => {
  useHeaderStore().reset();
});
</script>
