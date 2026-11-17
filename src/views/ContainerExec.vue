<template>
  <DashboardLayout>
    <div class="terminal-view">
      <Teleport to="#header-action" v-if="mounted">
        <div class="header-controls">
          <div class="status-indicator" :class="connectionStatus">
            <span class="status-dot"></span>
            {{ statusText }}
          </div>
          <button v-if="connectionStatus === 'disconnected'" class="btn-reconnect" @click="connect">
            <RefreshCw :size="14" />
            重连
          </button>
        </div>
      </Teleport>
      <div ref="terminalContainer" class="terminal-container"></div>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import { useRoute } from 'vue-router';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { RefreshCw } from 'lucide-vue-next';
import 'xterm/css/xterm.css';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useSystemStore } from '@/stores/system';
import { useHeaderStore } from '@/stores/header';
import { request } from '@/utils/request';

const route = useRoute();
const systemStore = useSystemStore();
const headerStore = useHeaderStore();
const terminalContainer = ref<HTMLElement | null>(null);
const containerId = route.params.id as string;

const connectionStatus = ref<'connecting' | 'connected' | 'disconnected'>('connecting');
const mounted = ref(false);

let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let ws: WebSocket | null = null;
let disposed = false;

const statusText = computed(() => {
  switch (connectionStatus.value) {
    case 'connecting':
      return '正在连接...';
    case 'connected':
      return '已连接';
    case 'disconnected':
      return '连接已断开';
    default:
      return '';
  }
});

const fetchDetail = async () => {
  try {
    const data = await request(`/api/containers/${containerId}`);
    const name = data.Name.replace('/', '');
    headerStore.set(`${name} - 终端`, true);
  } catch (err) {
    console.error(err);
  }
};

const connect = () => {
  if (ws) {
    ws.close();
  }

  connectionStatus.value = 'connecting';
  term?.write('\r\n\x1b[33m正在连接容器…\x1b[0m\r\n');

  // Connect WebSocket
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  let wsUrl = '';

  if (systemStore.currentNodeId && systemStore.currentNodeId !== 'local') {
    // The Controller pairs this browser connection with an outbound Agent stream.
    wsUrl = `${protocol}//${window.location.host}/ws/nodes/${systemStore.currentNodeId}/containers/${containerId}/exec`;
  } else {
    // Native WS path
    wsUrl = `${protocol}//${window.location.host}/ws/containers/${containerId}/exec`;
  }

  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    connectionStatus.value = 'connected';
    term?.write('\x1b[32m连接成功。\x1b[0m\r\n');

    // Initial resize
    if (ws && fitAddon && term) {
      const { cols, rows } = term;
      ws.send(JSON.stringify({ event: 'resize', rows, cols }));
    }
  };

  ws.onmessage = (event) => {
    term?.write(event.data);
  };

  ws.onclose = () => {
    connectionStatus.value = 'disconnected';
    term?.write('\r\n\x1b[31m连接已关闭。\x1b[0m\r\n');
  };

  ws.onerror = (err) => {
    connectionStatus.value = 'disconnected';
    term?.write(`\r\n\x1b[31m终端连接错误： ${err}\x1b[0m\r\n`);
  };
};

onMounted(async () => {
  mounted.value = true;
  fetchDetail();

  // Wait for the web fonts before xterm measures cells or caches glyphs.
  await Promise.allSettled([
    document.fonts.load('14px Consolas'),
    document.fonts.load('14px "LXGW WenKai"', '终端')
  ]);
  if (disposed || !terminalContainer.value) return;

  term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: getComputedStyle(document.documentElement).fontFamily,
    theme: {
      background: '#1e1e1e',
      foreground: '#ffffff'
    }
  });

  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(terminalContainer.value);

  // Slight delay for fit
  setTimeout(() => {
    fitAddon?.fit();
  }, 100);

  connect();

  // Terminal -> WebSocket
  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ event: 'data', data }));
    }
  });

  // Handle Window Resize
  window.addEventListener('resize', handleResize);
});

onUnmounted(() => {
  disposed = true;
  window.removeEventListener('resize', handleResize);
  headerStore.reset();
  if (ws) {
    ws.close();
  }
  if (term) {
    term.dispose();
  }
});

const handleResize = () => {
  if (fitAddon && term && ws && ws.readyState === WebSocket.OPEN) {
    fitAddon.fit();
    const { cols, rows } = term;
    ws.send(JSON.stringify({ event: 'resize', rows, cols }));
  }
};
</script>

<style scoped>
.terminal-view {
  height: max(360px, 62dvh);
  display: flex;
  flex-direction: column;
}

.terminal-container {
  flex: 1;
  background: #1e1e1e;
  padding: 10px;
  border-radius: 4px;
  overflow: hidden;
}

.header-controls {
  display: flex;
  align-items: center;
  gap: 12px;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  font-weight: 500;
  padding: 4px 10px;
  border-radius: 4px;
  background: var(--item-hover);
  color: var(--system-gray);
  transition: all 0.3s;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--system-gray);
  position: relative;
}

.status-indicator.connecting {
  color: var(--system-blue);
}
.status-indicator.connecting .status-dot {
  background: var(--system-blue);
  box-shadow: 0 0 8px var(--system-blue);
  animation: pulse 1.5s infinite;
}

.status-indicator.connected {
  color: var(--system-green);
}
.status-indicator.connected .status-dot {
  background: var(--system-green);
}

.status-indicator.disconnected {
  color: var(--system-red);
}
.status-indicator.disconnected .status-dot {
  background: var(--system-red);
}

.btn-reconnect {
  background: var(--system-blue);
  color: white;
  border: none;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 11px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
  transition: opacity 0.2s;
}

.btn-reconnect:hover {
  opacity: 0.9;
}

@keyframes pulse {
  0% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.5);
    opacity: 0.5;
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}
</style>
