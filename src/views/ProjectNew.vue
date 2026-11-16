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
            <label class="label">
              <CheckCircle2 :size="14" class="icon" />
              项目名称
            </label>
            <input
              v-model="name"
              type="text"
              class="input"
              placeholder="例如：my-web-app"
              :disabled="loading"
              autocomplete="off"
            />
            <p class="hint">项目将保存在 <code>/poke/projects/{{ name || '...' }}</code> 目录下</p>
          </div>

          <div class="section">
            <label class="label">
              <FileCode :size="14" class="icon" />
              Docker Compose 配置 (YAML)
            </label>
            <textarea
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
const content = ref("services:\n  ");
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

<style scoped>
.project-new {
    padding: 0;
    max-width: 1200px;
    margin: 0 auto;
    animation: fadeIn 0.3s ease-out;
}

.form {
    width: 100%;
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
}

.section:last-of-type {
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

.textarea {
    width: 100%;
    height: 400px;
    background: var(--input-background);
    border: 1px solid var(--card-border);
    color: var(--foreground);
    padding: 16px;
    border-radius: 8px;
    font-size: 13px;

    transition: all 0.2s;
    outline: none;
    resize: none;
    line-height: 1.6;
}

.textarea:focus {
    border-color: var(--system-blue);
}

.hint {
    margin-top: 8px;
    font-size: 12px;
    color: var(--system-gray);
}

.hint code {
    color: var(--system-blue);
    background: rgba(0, 122, 255, 0.1);
    padding: 2px 4px;
    border-radius: 4px;

}

.info {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--system-gray);
    opacity: 0.6;
    margin-top: 24px;
}

.info code {
    color: var(--system-blue);
    background: rgba(0, 122, 255, 0.1);
    padding: 2px 4px;
    border-radius: 4px;

}

.btn-submit {
    display: flex;
    align-items: center;
    gap: 6px;
    background: var(--system-green);
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
