<template>
  <DashboardLayout>
    <section v-if="system.ready && !system.nodes.length" class="overview-state">
      <Server :size="32" />
      <h2>连接你的第一个节点</h2>
      <p>在系统设置中注册 Agent，即可开始查看资源和管理容器。</p>
      <router-link class="button primary" to="/nodes">前往添加节点<ArrowUpRight :size="15" /></router-link>
    </section>
    <div v-else-if="loading && !stats" class="overview-state" role="status">
      <LoaderCircle class="spin" :size="24" />
      <p>正在获取节点运行情况…</p>
    </div>
    <section v-else-if="error" class="overview-state" role="alert">
      <Server :size="28" />
      <h2>暂时无法连接节点</h2>
      <p>{{ error }}</p>
      <div class="header-controls">
        <button class="secondary" @click="fetchStats"><RefreshCw :size="14" />重试连接</button
        ><router-link to="/nodes" class="button secondary">管理节点</router-link>
      </div>
    </section>
    <div v-else-if="stats" class="dashboard-container">
      <section class="overview-banner">
        <div class="banner-icon"><Server :size="22" /></div>
        <div>
          <h2>{{ stats.hostname }}</h2>
          <p>{{ stats.platform }} · {{ stats.cpus.count }} 核处理器 · Docker 资源概览</p>
        </div>
        <span class="connection-badge"><span class="status-dot running" />节点已连接</span>
      </section>
      <div class="stats-grid">
        <button class="stat-card" @click="router.push('/containers')">
          <div class="stat-top"><span>容器</span><Box :size="17" /></div>
          <strong>{{ stats.docker.containers.total }}</strong>
          <div class="stat-bottom">
            <span class="running">{{ stats.docker.containers.running }} 运行中</span
            ><ArrowUpRight :size="14" />
          </div>
        </button>
        <button class="stat-card" @click="router.push('/images')">
          <div class="stat-top"><span>镜像</span><HardDrive :size="17" /></div>
          <strong>{{ stats.docker.images }}</strong>
          <div class="stat-bottom"><span>本地镜像</span><ArrowUpRight :size="14" /></div>
        </button>
        <button class="stat-card" @click="router.push('/networks')">
          <div class="stat-top"><span>网络</span><Network :size="17" /></div>
          <strong>{{ stats.docker.networks }}</strong>
          <div class="stat-bottom"><span>Docker 网络</span><ArrowUpRight :size="14" /></div>
        </button>
        <button class="stat-card" @click="router.push('/volumes')">
          <div class="stat-top"><span>存储卷</span><Database :size="17" /></div>
          <strong>{{ stats.docker.volumes }}</strong>
          <div class="stat-bottom"><span>持久化数据卷</span><ArrowUpRight :size="14" /></div>
        </button>
      </div>
      <div class="overview-grid">
        <section class="overview-card">
          <header>
            <Info :size="16" />
            <h2>系统信息</h2>
          </header>
          <dl class="system-info">
            <div>
              <dt>主机名称</dt>
              <dd>{{ stats.hostname }}</dd>
            </div>
            <div>
              <dt>操作系统</dt>
              <dd>{{ stats.platform }} ({{ stats.release }})</dd>
            </div>
            <div>
              <dt>处理器</dt>
              <dd>{{ stats.cpus.model }}</dd>
            </div>
            <div>
              <dt>核心数量</dt>
              <dd>{{ stats.cpus.count }} 核</dd>
            </div>
          </dl>
        </section>
        <section class="overview-card">
          <header>
            <Activity :size="16" />
            <h2>资源使用</h2>
          </header>
          <div class="resource-meter">
            <div class="resource-heading">
              <span><MemoryStick :size="15" />内存使用</span><strong>{{ stats.memory.usagePercent }}%</strong>
            </div>
            <div
              class="meter-track"
              role="progressbar"
              aria-label="内存使用率"
              :aria-valuenow="stats.memory.usagePercent"
              :aria-valuemin="0"
              :aria-valuemax="100"
            >
              <div
                :style="{
                  width: `${Math.min(100, stats.memory.usagePercent)}%`,
                  background: stats.memory.usagePercent > 80 ? 'var(--system-red)' : 'var(--system-blue)'
                }"
              />
            </div>
            <div class="meter-detail">
              <span>已用 {{ formatBytes(stats.memory.used) }}</span
              ><span>总量 {{ formatBytes(stats.memory.total) }}</span>
            </div>
          </div>
          <div class="runtime-summary">
            <span
              >运行中容器 <strong>{{ stats.docker.containers.running }}</strong></span
            ><span
              >已停止容器 <strong>{{ stats.docker.containers.stopped }}</strong></span
            >
          </div>
        </section>
        <section class="overview-card ports-card">
          <header>
            <Globe :size="16" />
            <h2>服务端口</h2>
            <span class="muted">{{ stats.ports?.length || 0 }} 项映射</span>
          </header>
          <div class="port-table-container">
            <table class="table port-table">
              <thead>
                <tr>
                  <th style="width: 80px">状态</th>
                  <th>容器名称</th>
                  <th>绑定 IP</th>
                  <th>宿主机 → 容器</th>
                  <th style="width: 80px">协议</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!stats.ports?.length">
                  <td colspan="5" class="table-message">暂无活跃的端口映射</td>
                </tr>
                <tr v-for="(port, index) in stats.ports" :key="index" class="row">
                  <td>
                    <span
                      class="status-dot"
                      :class="port.state"
                      :title="port.state"
                      :aria-label="port.state === 'running' ? '运行中' : '已停止'"
                    />
                  </td>
                  <td :title="port.container">{{ port.container }}</td>
                  <td>{{ port.public ? port.ip || '0.0.0.0' : '—' }}</td>
                  <td>
                    <span v-if="port.public" class="port-number">{{ port.public }} → </span>{{ port.private }}
                  </td>
                  <td>{{ port.type }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </div>
  </DashboardLayout>
</template>
<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import {
  Box,
  HardDrive,
  Network,
  Database,
  Info,
  Activity,
  MemoryStick,
  Globe,
  Server,
  ArrowUpRight,
  RefreshCw,
  LoaderCircle
} from 'lucide-vue-next';
import { useSystemStore } from '@/stores/system';
import DashboardLayout from '@/layouts/DashboardLayout.vue';

interface SystemStats {
  hostname: string;
  platform: string;
  release: string;
  memory: {
    total: number;
    free: number;
    used: number;
    usagePercent: number;
  };
  cpus: {
    model: string;
    count: number;
  };
  docker: {
    containers: {
      total: number;
      running: number;
      stopped: number;
    };
    images: number;
    volumes: number;
    networks: number;
  };
  ports: Array<{
    container: string;
    ip: string;
    public: number;
    private: number;
    type: string;
    state: string;
  }>;
}

import { request } from '@/utils/request';

const router = useRouter();
const system = useSystemStore();
const stats = ref<SystemStats | null>(null);
const loading = ref(true);
const error = ref('');
let intervalId: number;

const fetchStats = async () => {
  try {
    const data = await request('/api/system/info');
    stats.value = data;
    error.value = '';
  } catch (err) {
    stats.value = null;
    error.value = (err as Error).message || '节点连接失败';
  } finally {
    loading.value = false;
  }
};

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

onMounted(() => {
  fetchStats();
  intervalId = window.setInterval(fetchStats, 10000);
});

onUnmounted(() => {
  clearInterval(intervalId);
});
</script>
<style scoped>
.dashboard-container {
  display: flex;
  flex-direction: column;
  gap: 22px;
}
.overview-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: linear-gradient(100deg, var(--accent-soft), var(--surface) 85%);
  padding: 19px 21px;
}
.banner-icon {
  width: 42px;
  height: 42px;
  display: grid;
  place-items: center;
  color: var(--blue);
  border: 1px solid var(--border);
  border-radius: 6px;
  flex: none;
}
.overview-banner > div:nth-child(2) {
  min-width: 0;
}
.overview-banner h2 {
  font-size: 15px;
  font-weight: 500;
  overflow-wrap: anywhere;
}
.overview-banner p {
  font-size: 12px;
  margin-top: 5px;
  color: var(--system-gray);
}
.connection-badge {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  white-space: nowrap;
  border: 1px solid var(--border);
  color: var(--system-green);
  padding: 4px 8px;
  font-size: 11px;
  border-radius: 4px;
}
.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}
.stat-card {
  display: block;
  width: 100%;
  text-align: left;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 18px 20px;
}
.stat-card:hover {
  border-color: var(--blue);
}
.stat-top,
.stat-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--system-gray);
  font-size: 12px;
}
.stat-top svg {
  color: var(--system-gray);
}
.stat-card > strong {
  display: block;
  font-size: 32px;
  line-height: 1.3;
  font-weight: 400;
  margin: 12px 0;
}
.stat-bottom .running {
  color: var(--system-green);
}
.overview-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
}
.overview-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
  min-width: 0;
}
.overview-card > header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
}
.overview-card header svg {
  color: var(--system-gray);
}
.overview-card h2 {
  font-size: 13px;
  font-weight: 500;
}
.overview-card header .muted {
  margin-left: auto;
  font-size: 11px;
}
.system-info {
  padding: 8px 20px;
  margin: 0;
  font-size: 12px;
}
.system-info > div {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 20px;
  padding: 10px 0;
}
.system-info dt {
  color: var(--system-gray);
  flex: none;
}
.system-info dd {
  margin: 0;
  text-align: right;
  overflow-wrap: anywhere;
}
.resource-meter {
  padding: 24px 20px;
}
.resource-heading,
.meter-detail {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
}
.resource-heading > span {
  display: flex;
  gap: 8px;
  align-items: center;
}
.resource-heading strong {
  color: var(--blue);
  font-size: 18px;
  font-weight: 400;
}
.meter-track {
  height: 5px;
  background: var(--progress-bg);
  border-radius: 3px;
  overflow: hidden;
  margin: 16px 0 12px;
}
.meter-track > div {
  height: 100%;
  border-radius: inherit;
}
.meter-detail {
  color: var(--system-gray);
  font-size: 11px;
}
.runtime-summary {
  display: flex;
  gap: 24px;
  padding: 16px 20px;
  border-top: 1px solid var(--border);
  color: var(--system-gray);
  font-size: 12px;
}
.runtime-summary strong {
  color: var(--foreground);
  margin-left: 10px;
  font-weight: 500;
}
.ports-card {
  grid-column: 1 / -1;
}
.port-table-container {
  overflow: auto;
}
.port-number {
  color: var(--blue);
}
.overview-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  min-height: 360px;
  padding: 32px;
  border: 1px dashed var(--border);
  border-radius: 6px;
  text-align: center;
}
.overview-state > svg {
  color: var(--system-gray);
}
.overview-state h2 {
  font-size: 17px;
  font-weight: 500;
}
.overview-state p {
  color: var(--system-gray);
  max-width: 480px;
  overflow-wrap: anywhere;
}
@media (max-width: 1100px) {
  .stats-grid {
    gap: 12px;
  }
  .stat-card {
    padding: 16px;
  }
}
@media (max-width: 800px) {
  .stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .overview-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 540px) {
  .overview-banner {
    flex-wrap: wrap;
    padding: 16px;
    gap: 12px;
  }
  .connection-badge {
    margin-left: 0;
  }
  .system-info > div {
    gap: 12px;
  }
  .runtime-summary {
    flex-wrap: wrap;
    gap: 12px;
  }
}
</style>
