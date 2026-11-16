<template>
  <DashboardLayout>
    <div v-if="loading && !container" class="loading">加载详情中...</div>
    <div v-else-if="!container" class="error">未找到容器信息</div>

    <div v-else class="container-detail">
      <Teleport to="#header-action" v-if="mounted">
        <div class="header-controls">
          <button
            v-if="!isRunning"
            class="btn btn-start"
            @click="handleAction('start')"
            :disabled="actionLoading"
          >
            <Play :size="14" style="margin-right: 4px" /> 启动
          </button>
          <button
            v-else
            class="btn btn-stop"
            @click="handleAction('stop')"
            :disabled="actionLoading"
          >
            <Square :size="14" style="margin-right: 4px" /> 停止
          </button>

          <button
            class="btn btn-restart"
            @click="handleAction('restart')"
            :disabled="actionLoading"
          >
            <RotateCw :size="14" style="margin-right: 4px" /> 重启
          </button>

          <button
            class="btn"
            @click="router.push(`/containers/${id}/exec`)"
            :disabled="actionLoading || !isRunning"
          >
            <Terminal :size="14" style="margin-right: 4px" /> 终端
          </button>
        </div>
      </Teleport>

      <div class="grid">
        <div class="card">
          <h3>基本信息</h3>
          <div class="info-row">
            <span class="label">镜像</span>
            <span class="value">{{ container.Config.Image }}</span>
          </div>
          <div class="info-row">
            <span class="label">创建时间</span>
            <span class="value">{{ formatDate(container.Created) }}</span>
          </div>
          <div class="info-row">
            <span class="label">状态</span>
            <span class="value">{{ container.State.Status }}</span>
          </div>
          <div class="info-row">
            <span class="label">命令</span>
            <span class="value monospace">{{ container.Config.Cmd ? container.Config.Cmd.join(' ') : '-' }}</span>
          </div>
        </div>

        <div class="card">
          <h3>网络</h3>
          <div v-for="([name, net]) in Object.entries(container.NetworkSettings.Networks)" :key="name" class="info-row">
            <span class="label">{{ name }}</span>
            <span class="value">{{ (net as any).IPAddress || '-' }}</span>
          </div>
          <div class="ports">
            <h4>端口映射</h4>
            <div class="tag-list">
              <div v-for="([port, bindings]) in Object.entries(container.NetworkSettings.Ports || {})" :key="port" class="tag">
                {{ bindings ? `${(bindings as any)[0].HostPort}:${port}` : port }}
              </div>
            </div>
          </div>
        </div>

        <div class="card full-width">
          <h3>环境变量</h3>
          <div class="env-grid">
            <div v-for="env in container.Config.Env" :key="env" class="env-item">
              <span class="env-key">{{ env.split('=')[0] }}</span>
              <span class="env-value">{{ env.split('=')[1] }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Play, Square, RotateCw, Terminal } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useToastStore } from '@/stores/toast';
import { useHeaderStore } from '@/stores/header';
import { request } from '@/utils/request';

const route = useRoute();
const router = useRouter();
const toastStore = useToastStore();
const headerStore = useHeaderStore();

const id = route.params.id as string;
const container = ref<any>(null);
const loading = ref(true);
const actionLoading = ref(false);
const mounted = ref(false);

let intervalId: number;

const isRunning = computed(() => container.value?.State?.Running || false);

const fetchDetail = async () => {
    try {
        const data = await request(`/api/containers/${id}`);
        container.value = data;

        // Update header title
        const name = data.Name.replace('/', '');
        headerStore.set(name, true);
    } catch (err: any) {
        toastStore.addToast(err.message, 'error');
    } finally {
        loading.value = false;
    }
};

const handleAction = async (action: string) => {
    actionLoading.value = true;
    try {
        await request(`/api/containers/${id}/${action}`, {
            method: 'POST'
        });
        toastStore.addToast(`操作成功: ${action}`, 'success');
        fetchDetail();
    } catch (err: any) {
        toastStore.addToast(err.message, 'error');
    } finally {
        actionLoading.value = false;
    }
};

const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString();
};

onMounted(() => {
    mounted.value = true;
    fetchDetail();
    intervalId = window.setInterval(fetchDetail, 3000);
});

onUnmounted(() => {
    clearInterval(intervalId);
    headerStore.reset();
});
</script>

<style scoped>
.container-detail {
  height: 100%;
  overflow: auto;
  padding-bottom: 40px;
}

.loading, .error {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
  color: var(--system-gray);
}

.header-controls {
  display: flex;
  gap: 8px;
  align-items: center;
}

.grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 24px;
}

@media (max-width: 900px) {
  .grid {
    grid-template-columns: 1fr;
  }
}

.card {
  background: var(--card-background);
  border: 1px solid var(--card-border);
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.02);
}

.full-width {
  grid-column: 1 / -1;
}

.card h3 {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 20px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--card-border);
}

.card h4 {
  font-size: 14px;
  font-weight: 500;
  margin-top: 16px;
  margin-bottom: 12px;
  color: var(--system-gray);
}

.info-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 12px;
  font-size: 14px;
}

.info-row:last-child {
  margin-bottom: 0;
}

.label {
  color: var(--system-gray);
  font-weight: 500;
}

.value {
  font-weight: 500;
  text-align: right;
}

.monospace {
  font-size: 13px;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tag {
  display: inline-block;
  background: var(--item-hover);
  border-radius: 4px;
  padding: 4px 8px;
  font-size: 12px;
  color: var(--foreground);

}

.env-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 12px;
}

.env-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 12px;
  background: var(--input-background);
  border-radius: 6px;
  font-size: 12px;
}

.env-key {
  color: var(--system-blue);
  font-weight: 600;
}

.env-value {
  opacity: 0.8;
  word-break: break-all;
}

/* Button Styles */
.btn {
  border: 1px solid var(--glass-border);
  border-radius: 6px;
  padding: 0 12px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  background: var(--window-background);
  color: var(--foreground);
  font-size: 13px;
  font-weight: 500;
  transition: all 0.2s;
}

.btn:hover:not(:disabled) {
  background: var(--item-hover);
  transform: translateY(-1px);
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-start {
  background: rgba(52, 199, 89, 0.1);
  color: var(--system-green);
  border-color: transparent;
}

.btn-start:hover:not(:disabled) {
  background: rgba(52, 199, 89, 0.2);
}

.btn-stop {
  background: rgba(255, 149, 0, 0.1);
  color: var(--system-orange);
  border-color: transparent;
}

.btn-stop:hover:not(:disabled) {
  background: rgba(255, 149, 0, 0.2);
}

.btn-restart {
  background: rgba(0, 122, 255, 0.1);
  color: var(--system-blue);
  border-color: transparent;
}

.btn-restart:hover:not(:disabled) {
  background: rgba(0, 122, 255, 0.2);
}
</style>
