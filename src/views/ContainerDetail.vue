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
          <button v-else class="btn btn-stop" @click="handleAction('stop')" :disabled="actionLoading">
            <Square :size="14" style="margin-right: 4px" /> 停止
          </button>

          <button class="btn btn-restart" @click="handleAction('restart')" :disabled="actionLoading">
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
            <span class="value monospace">{{
              container.Config.Cmd ? container.Config.Cmd.join(' ') : '-'
            }}</span>
          </div>
        </div>

        <div class="card">
          <h3>网络</h3>
          <div
            v-for="[name, net] in Object.entries(container.NetworkSettings.Networks)"
            :key="name"
            class="info-row"
          >
            <span class="label">{{ name }}</span>
            <span class="value">{{ (net as any).IPAddress || '-' }}</span>
          </div>
          <div class="ports">
            <h4>端口映射</h4>
            <div class="tag-list">
              <div
                v-for="[port, bindings] in Object.entries(container.NetworkSettings.Ports || {})"
                :key="port"
                class="tag"
              >
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
              <span class="env-value">{{ env.slice(env.indexOf('=') + 1) }}</span>
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
