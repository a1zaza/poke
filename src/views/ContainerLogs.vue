<template>
  <DashboardLayout>
    <div class="container-logs">
      <Teleport to="#header-action" v-if="mounted">
        <div class="controls">
          <Select
            :options="[
              { label: '100 行', value: 100 },
              { label: '500 行', value: 500 },
              { label: '1000 行', value: 1000 },
              { label: '2000 行', value: 2000 }
            ]"
            v-model="tailLines"
          />

          <button
            class="btn"
            :class="{ active: autoRefresh }"
            @click="autoRefresh = !autoRefresh"
            :title="autoRefresh ? '暂停自动刷新' : '开启自动刷新'"
          >
            <Pause v-if="autoRefresh" :size="14" />
            <Play v-else :size="14" />
            {{ autoRefresh ? '自动刷新' : '已暂停' }}
          </button>

          <button class="btn" @click="fetchLogs" title="立即刷新">
            <RefreshCw :size="14" /> 刷新
          </button>

          <button
            class="btn"
            :class="{ active: showTimestamps }"
            @click="showTimestamps = !showTimestamps"
            :title="showTimestamps ? '隐藏时间' : '显示时间'"
          >
            <Clock :size="14" />
            时间
          </button>

          <button class="btn" @click="handleDownload" title="下载日志">
            <Download :size="14" /> 下载
          </button>
        </div>
      </Teleport>

      <div class="log-window">
        <div v-if="loading && !logs" style="color: var(--system-gray)">加载中...</div>
        <div v-else-if="!logs" style="color: var(--system-gray)">暂无日志</div>
        <div v-else v-for="(line, index) in renderLogs()" :key="index" class="log-line" v-html="line"></div>
        <div ref="logEndRef"></div>
      </div>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue';
import { useRoute } from 'vue-router';
import { Pause, Play, RefreshCw, Download, Clock } from 'lucide-vue-next';
import Anser from 'anser';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import Select from '@/components/ui/Select.vue';
import { useToastStore } from '@/stores/toast';
import { useHeaderStore } from '@/stores/header';
import { request } from '@/utils/request';

const route = useRoute();
const toastStore = useToastStore();
const headerStore = useHeaderStore();

const id = route.params.id as string;
const logs = ref('');
const container = ref<any>(null);
const loading = ref(true);
const autoRefresh = ref(true);
const showTimestamps = ref(false);
const tailLines = ref(100);
const logEndRef = ref<HTMLElement | null>(null);
const mounted = ref(false);

let intervalId: number;

const fetchDetail = async () => {
    try {
        const data = await request(`/api/containers/${id}`);
        container.value = data;

        // Update header title
        const name = data.Name.replace('/', '');
        headerStore.set(`${name} - 日志`, true);
    } catch (err) {
        console.error(err);
    }
};

const fetchLogs = async () => {
    try {
        const data = await request(`/api/containers/${id}/logs?tail=${tailLines.value}&timestamps=${showTimestamps.value}`);
        logs.value = data.logs || '';
    } catch (err: any) {
        console.error(err);
        if (loading.value) toastStore.addToast('获取日志失败', 'error');
    } finally {
        loading.value = false;
    }
};

const handleDownload = () => {
    const blob = new Blob([logs.value], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `container-${id}-logs.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
};

const renderLogs = () => {
    if (!logs.value) return [];

    return logs.value.split('\n').map((line) => {
        let efficientLine = line;
        // Remove Docker multiplexing header if present
        if (efficientLine.length > 8 && efficientLine.charCodeAt(0) < 32) {
            efficientLine = efficientLine.substring(8);
        }

        // Parse ANSI codes
        const ansiData = Anser.ansiToJson(efficientLine, { use_classes: false });

        // Build HTML string
        return ansiData.map((chunk: any) => {
            const styles = [];
            if (chunk.fg) styles.push(`color: rgb(${chunk.fg})`);
            if (chunk.bg) styles.push(`background-color: rgb(${chunk.bg})`);
            const styleAttr = styles.length > 0 ? ` style="${styles.join('; ')}"` : '';
            return `<span${styleAttr}>${escapeHtml(chunk.content)}</span>`;
        }).join('');
    });
};

const escapeHtml = (text: string) => {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
};

// Watch for changes in tailLines and showTimestamps
watch([tailLines, showTimestamps], () => {
    fetchLogs();
});

// Watch logs and auto-scroll
watch(logs, () => {
    if (autoRefresh.value && logEndRef.value) {
        setTimeout(() => {
            logEndRef.value?.scrollIntoView({ behavior: 'smooth' });
        }, 100);
    }
});

onMounted(() => {
    mounted.value = true;
    fetchDetail();
    fetchLogs();

    intervalId = window.setInterval(() => {
        if (autoRefresh.value) fetchLogs();
    }, 2000);
});

onUnmounted(() => {
    clearInterval(intervalId);
    headerStore.reset();
});
</script>

<style scoped>
.container-logs {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.controls {
  display: flex;
  gap: 8px;
  align-items: center;
}

.btn {
  background: var(--item-hover);
  border: none;
  border-radius: 6px;
  padding: 6px 12px;
  color: var(--foreground);
  font-size: 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  transition: all 0.2s;
}

.btn:hover {
  background: rgba(255, 255, 255, 0.2);
}

.btn.active {
  background: var(--system-blue);
  color: white;
}

.log-window {
  flex: 1;
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 16px;
  border-radius: 8px;
  overflow-y: auto;

  font-size: 13px;
  line-height: 1.6;
}

.log-line {
  margin-bottom: 2px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
