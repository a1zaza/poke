<template>
  <DashboardLayout>
    <div v-if="loading && !stats" class="loading">加载中...</div>

    <div v-else-if="error" class="loading" role="alert">
      <p>{{ error }}</p>
      <button @click="fetchStats">重试连接</button>
      <router-link to="/settings">前往系统设置管理节点</router-link>
    </div>
    <div v-else-if="stats" class="dashboard-container">
      <!-- Quick Stats Grid -->
      <div class="stats-grid">
        <div class="stat-card" @click="router.push('/containers')">
          <div class="stat-icon blue">
            <Box :size="24" />
          </div>
          <div class="stat-info">
            <div class="stat-label">容器</div>
            <div class="stat-value">{{ stats.docker.containers.total }}</div>
            <div class="stat-sub">
              <span class="running">{{ stats.docker.containers.running }} 运行中</span>
            </div>
          </div>
        </div>

        <div class="stat-card" @click="router.push('/images')">
          <div class="stat-icon green">
            <HardDrive :size="24" />
          </div>
          <div class="stat-info">
            <div class="stat-label">镜像</div>
            <div class="stat-value">{{ stats.docker.images }}</div>
            <div class="stat-sub">本地已存储</div>
          </div>
        </div>

        <div class="stat-card" @click="router.push('/networks')">
          <div class="stat-icon orange">
            <Network :size="24" />
          </div>
          <div class="stat-info">
            <div class="stat-label">网络</div>
            <div class="stat-value">{{ stats.docker.networks }}</div>
            <div class="stat-sub">虚拟网桥</div>
          </div>
        </div>

        <div class="stat-card" @click="router.push('/volumes')">
          <div class="stat-icon purple">
            <Database :size="24" />
          </div>
          <div class="stat-info">
            <div class="stat-label">存储卷</div>
            <div class="stat-value">{{ stats.docker.volumes }}</div>
            <div class="stat-sub">持久化数据</div>
          </div>
        </div>
      </div>

      <div class="main-grid">
        <!-- System Info -->
        <div class="card">
          <div class="card-header">
            <Info :size="18" />
            系统信息
          </div>
          <div class="info-list">
            <div class="info-item">
              <span class="info-label">主机名</span>
              <span class="info-value">{{ stats.hostname }}</span>
            </div>
            <div class="info-item">
              <span class="info-label">平台</span>
              <span class="info-value">{{ stats.platform }} ({{ stats.release }})</span>
            </div>
            <div class="info-item">
              <span class="info-label">处理器</span>
              <span class="info-value">{{ stats.cpus.count }} 核 - {{ stats.cpus.model }}</span>
            </div>
          </div>
        </div>

        <!-- Resource Usage -->
        <div class="card">
          <div class="card-header">
            <Activity :size="18" />
            资源占用
          </div>
          <div class="resource-group">
            <div class="resource-header">
              <div class="resource-title">
                <MemoryStick :size="16" />
                内存使用
              </div>
              <div class="resource-value">{{ stats.memory.usagePercent }}%</div>
            </div>
            <div class="progress-container">
              <div
                class="progress-bar"
                :style="{ width: `${stats.memory.usagePercent}%`, background: stats.memory.usagePercent > 80 ? 'var(--system-red)' : 'var(--system-blue)' }"
              />
            </div>
            <div class="resource-details">
              <span>已用 {{ formatBytes(stats.memory.used) }}</span>
              <span>总量 {{ formatBytes(stats.memory.total) }}</span>
            </div>
          </div>
        </div>

        <!-- Port Mappings -->
        <div class="card full-width">
           <div class="card-header">
             <Globe :size="18" />
             服务端口映射
           </div>
           <div v-if="stats.ports && stats.ports.length === 0" class="empty-ports">
               暂无活跃的端口映射
           </div>
           <div v-else-if="stats.ports" class="port-table-container">
               <table class="port-table">
                   <thead>
                       <tr>
                           <th>状态</th>
                           <th>容器名称</th>
                           <th>绑定 IP</th>
                           <th>端口映射 (宿主机:容器)</th>
                           <th>协议</th>
                       </tr>
                   </thead>
                   <tbody>
                       <tr v-for="(p, idx) in stats.ports" :key="idx" class="port-row">
                           <td class="status-cell">
                               <div class="status-dot" :class="p.state"></div>
                           </td>
                           <td class="container-cell" :title="p.container">{{ p.container }}</td>
                           <td class="ip-cell">
                               <span v-if="p.public">{{ p.ip || '0.0.0.0' }}</span>
                               <span v-else style="opacity: 0.3">-</span>
                           </td>
                           <td class="port-cell">
                               <span v-if="p.public"><span class="highlight">{{ p.public }}</span>:</span>
                               <span v-else></span>
                               {{ p.private }}
                           </td>
                           <td class="proto-cell">{{ p.type }}</td>
                       </tr>
                   </tbody>
               </table>
           </div>
        </div>
      </div>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { Box, HardDrive, Network, Database, Info, Activity, MemoryStick, Globe } from 'lucide-vue-next';
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
  padding-bottom: 32px;
}

.loading {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
  color: var(--system-gray);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 24px;
  margin-bottom: 32px;
}

@media (max-width: 1200px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }
}

.stat-card {
  background: var(--card-background);
  border: 1px solid var(--card-border);
  border-radius: 16px;
  padding: 24px;
  display: flex;
  align-items: center;
  gap: 20px;
  cursor: pointer;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05); /* Soft shadow */
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 20px -3px rgba(0, 0, 0, 0.1);
  border-color: var(--system-blue);
}

.stat-icon {
  width: 56px;
  height: 56px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.stat-icon.blue { background: rgba(0, 122, 255, 0.1); color: var(--system-blue); }
.stat-icon.green { background: rgba(52, 199, 89, 0.1); color: var(--system-green); }
.stat-icon.orange { background: rgba(255, 149, 0, 0.1); color: var(--system-orange); }
.stat-icon.purple { background: rgba(175, 82, 222, 0.1); color: var(--system-purple); }

.stat-info {
  flex: 1;
}

.stat-label {
  font-size: 13px;
  color: var(--system-gray);
  margin-bottom: 4px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--foreground);
  margin-bottom: 4px;

}

.stat-sub {
  font-size: 12px;
  color: var(--system-gray);
  opacity: 0.8;
}

.running {
  color: var(--system-green);
  font-weight: 600;
}

.main-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 24px;
}

@media (max-width: 900px) {
  .main-grid {
    grid-template-columns: 1fr;
  }
}

.card {
  background: var(--card-background);
  border: 1px solid var(--card-border);
  border-radius: 16px;
  padding: 24px;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.02);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 24px;
  color: var(--foreground);
  padding-bottom: 16px;
  border-bottom: 1px solid var(--card-border);
}

.info-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.info-label {
  color: var(--system-gray);
  font-size: 14px;
}

.info-value {
  font-weight: 500;

  font-size: 14px;
}

.resource-group {
  margin-bottom: 24px;
}

.resource-group:last-child {
  margin-bottom: 0;
}

.resource-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.resource-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 500;
}

.resource-value {
  font-weight: 600;

  color: var(--system-blue);
}

.progress-container {
  height: 8px;
  background: var(--input-background);
  border-radius: 4px;
  overflow: hidden;
  margin-bottom: 8px;
}

.progress-bar {
  height: 100%;
  border-radius: 4px;
  transition: width 0.5s ease, background 0.3s ease;
}

.resource-details {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: var(--system-gray);

}

.full-width {
  grid-column: span 2;
}

@media (max-width: 900px) {
  .full-width {
    grid-column: span 1;
  }
}

.empty-ports {
  text-align: center;
  padding: 20px;
  color: var(--system-gray);
  font-size: 13px;
  opacity: 0.6;
}

.port-table-container {
  overflow-x: auto;
}

.port-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.port-table th {
  text-align: left;
  padding: 12px 16px;
  color: var(--system-gray);
  font-weight: 500;
  border-bottom: 1px solid var(--glass-border);
  font-size: 11px;
  text-transform: uppercase;
  white-space: nowrap;
}

.port-table td {
  padding: 10px 16px;
  border-bottom: 1px solid var(--glass-border);
  color: var(--foreground);
}

.port-table tr:last-child td {
  border-bottom: none;
}

.port-row:hover {
  background: var(--item-hover);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--system-gray);
}

.status-dot.running {
  background: var(--system-green);
  box-shadow: 0 0 8px rgba(52, 199, 89, 0.4);
}

.container-cell {
  font-weight: 500;
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip-cell, .port-cell, .proto-cell {

  font-size: 12px;
}

.ip-cell {
  color: var(--system-gray);
}

.port-cell .highlight {
  color: var(--system-blue);
  font-weight: 600;
}

.proto-cell {
  text-transform: uppercase;
  font-size: 10px;
  opacity: 0.7;
}
</style>
