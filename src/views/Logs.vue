<template>
  <DashboardLayout>
    <div v-if="loading && events.length === 0" class="loading">加载中...</div>

    <div v-else class="logs-container">
      <div class="timeline">
        <div v-if="events.length === 0" class="empty">
          最近 24 小时内没有发现 Docker 事件
        </div>

        <div
          v-for="(event, idx) in events"
          :key="`${event.timeNano}-${idx}`"
          class="event-item"
        >
          <div class="timeline-col">
            <div
              class="dot"
              :style="{ borderColor: getActionColor(event.Action) }"
            ></div>
            <div v-if="idx < events.length - 1" class="line"></div>
          </div>

          <div class="event-content">
            <div class="event-header">
              <span class="time">
                <Clock :size="12" />
                {{ formatTime(event.time) }}
              </span>
              <span class="type-badge">
                <component :is="getTypeIcon(event.Type)" :size="14" />
                {{ event.Type }}
              </span>
            </div>

            <div class="event-body">
              <span
                class="action"
                :style="{ color: getActionColor(event.Action) }"
              >
                {{ event.Action }}
              </span>
              <span class="target" :title="event.id">
                {{ getEventName(event) }}
              </span>
            </div>

            <div
              v-if="hasExtraAttributes(event)"
              class="attributes"
            >
              <span
                v-for="[key, value] in getExtraAttributes(event)"
                :key="key"
                class="attr"
              >
                {{ key }}: {{ value }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { Clock, Box, HardDrive, Network, Database, Activity } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useHeaderStore } from '@/stores/header';
import { request } from '@/utils/request';

interface DockerEvent {
  status: string;
  id: string;
  from: string;
  Type: string;
  Action: string;
  Actor: {
    ID: string;
    Attributes: Record<string, string>;
  };
  time: number;
  timeNano: number;
}

const headerStore = useHeaderStore();
const events = ref<DockerEvent[]>([]);
const loading = ref(true);

let intervalId: number;

const fetchEvents = async () => {
  try {
    const data = await request('/api/system/events');
    if (Array.isArray(data)) {
      events.value = data;
    }
  } catch (err) {
    console.error('Failed to fetch events:', err);
  } finally {
    loading.value = false;
  }
};

const formatTime = (seconds: number) => {
  return new Date(seconds * 1000).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  });
};

const getTypeIcon = (type: string) => {
  switch (type) {
    case 'container': return Box;
    case 'image': return HardDrive;
    case 'network': return Network;
    case 'volume': return Database;
    default: return Activity;
  }
};

const getActionColor = (action: string) => {
  const a = action.toLowerCase();
  if (['start', 'create', 'pull', 'unpause'].some(k => a.includes(k))) {
    return 'var(--system-green)';
  }
  if (['stop', 'die', 'kill', 'destroy', 'delete', 'prune', 'remove'].some(k => a.includes(k))) {
    return 'var(--system-red)';
  }
  if (['restart', 'update', 'rename'].some(k => a.includes(k))) {
    return 'var(--system-blue)';
  }
  return 'var(--system-gray)';
};

const getEventName = (event: DockerEvent) => {
  return event.Actor.Attributes.name ||
         event.Actor.Attributes.image ||
         event.id?.substring(0, 12) ||
         'unknown';
};

const hasExtraAttributes = (event: DockerEvent) => {
  return Object.keys(event.Actor.Attributes).length > 1;
};

const getExtraAttributes = (event: DockerEvent) => {
  return Object.entries(event.Actor.Attributes)
    .filter(([key]) => key !== 'name' && key !== 'image')
    .slice(0, 3);
};

onMounted(() => {
  headerStore.set('系统日志', false);
  fetchEvents();
  // Refresh every 30 seconds
  intervalId = window.setInterval(fetchEvents, 30000);
});

onUnmounted(() => {
  clearInterval(intervalId);
  headerStore.reset();
});
</script>

<style scoped>
.logs-container {
  height: 100%;
  overflow-y: auto;
  padding-right: 10px;
  padding-bottom: 40px;
}

.logs-container::-webkit-scrollbar {
  width: 6px;
}

.logs-container::-webkit-scrollbar-track {
  background: transparent;
}

.logs-container::-webkit-scrollbar-thumb {
  background: var(--card-border);
  border-radius: 3px;
}

.logs-container::-webkit-scrollbar-thumb:hover {
  background: var(--system-gray);
}

.loading,
.empty {
  padding: 40px;
  text-align: center;
  opacity: 0.5;
  font-size: 14px;
  color: var(--system-gray);
}

.timeline {
  display: flex;
  flex-direction: column;
}

.event-item {
  display: flex;
  gap: 20px;
}

.timeline-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 20px;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid;
  background: var(--background);
  z-index: 1;
  margin-top: 6px;
  flex-shrink: 0;
}

.line {
  width: 2px;
  background: var(--card-border);
  flex: 1;
}

.event-content {
  flex: 1;
  padding-bottom: 24px;
}

.event-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.time {
  font-size: 12px;
  opacity: 0.5;
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--foreground);
}

.type-badge {
  font-size: 10px;
  text-transform: uppercase;
  background: var(--item-hover);
  padding: 2px 6px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  gap: 4px;
  opacity: 0.8;
  color: var(--foreground);
}

.event-body {
  display: flex;
  align-items: center;
  gap: 12px;
}

.action {
  font-weight: 600;
  font-size: 14px;
  text-transform: capitalize;
}

.target {

  font-size: 13px;
  padding: 2px 8px;
  background: var(--input-background);
  border-radius: 4px;
  color: var(--foreground);
}

.attributes {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.attr {
  font-size: 11px;
  color: var(--system-gray);
  opacity: 0.8;
  background: var(--item-hover);
  padding: 1px 6px;
  border-radius: 3px;
}
</style>
