<template>
  <DashboardLayout>
    <div class="project-manager">
      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 25%">项目名称</th>
              <th style="width: 15%">状态</th>
              <th style="width: 45%">路径 / Compose 文件</th>
              <th style="width: 150px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && projects.length === 0">
              <td colspan="4" style="text-align: center">加载中...</td>
            </tr>
            <tr v-else-if="projects.length === 0">
              <td colspan="4" style="text-align: center; opacity: 0.5;">未发现 Docker Compose 项目</td>
            </tr>
            <tr v-else v-for="project in projects" :key="project.name" class="row">
              <td>
                <div
                  class="project-name-cell"
                  @click="router.push(`/projects/${project.name}`)"
                >
                  <Layers :size="14" style="color: var(--system-blue)" />
                  <div style="display: flex; align-items: center; gap: 6px">
                    <span class="project-title">{{ project.name }}</span>
                    <span v-if="!project.isControlled" class="uncontrolled-badge">不受控</span>
                  </div>
                </div>
              </td>
              <td>
                <div style="display: flex; align-items: center; gap: 8px">
                  <span class="status-dot" :class="getStatusClass(project.status)"></span>
                  {{ getStatusText(project.status) }}
                  <span style="font-size: 10px; opacity: 0.5">
                    {{ project.runningCount }}/{{ project.totalCount }}
                  </span>
                </div>
              </td>
              <td style="opacity: 0.5; font-size: 11px;">
                {{ project.workingDir }}
              </td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn"
                    @click="router.push(`/projects/${project.name}`)"
                    title="查看详情"
                  >
                    <Eye :size="14" />
                  </button>

                  <button
                    v-if="project.status !== 'running'"
                    class="btn btn-start"
                    @click="handleProjectAction(project.name, 'start')"
                    :disabled="isActing(project.name) || !project.isControlled"
                    :title="project.isControlled ? '启动全部' : '非受控项目无法操作'"
                  >
                    <Play :size="14" fill="currentColor" />
                  </button>

                  <button
                    v-if="project.status !== 'stopped'"
                    class="btn btn-stop"
                    @click="handleProjectAction(project.name, 'down')"
                    :disabled="isActing(project.name) || !project.isControlled"
                    :title="project.isControlled ? '停止并移除容器 (down)' : '非受控项目无法操作'"
                  >
                    <Square :size="14" fill="currentColor" />
                  </button>

                  <button
                    class="btn btn-restart"
                    @click="handleProjectAction(project.name, 'restart')"
                    :disabled="isActing(project.name) || !project.isControlled"
                    :title="project.isControlled ? '重启全部' : '非受控项目无法操作'"
                  >
                    <RotateCw :size="14" />
                  </button>

                  <button
                    class="btn btn-delete"
                    @click="confirmRemove(project.name)"
                    :disabled="isActing(project.name) || !project.isControlled"
                    :title="project.isControlled ? '移除项目 (删除文件夹)' : '非受控项目无法操作'"
                  >
                    <Trash2 :size="14" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <ConfirmModal
        :isOpen="showConfirmRemove"
        title="确认移除项目"
        :message="`确定要移除项目 ${pendingProjectName} 吗？这将停止容器并永久删除项目文件夹。`"
        :isDanger="true"
        @confirm="handleConfirmRemove"
        @close="showConfirmRemove = false"
      />

    <Teleport to="#header-action" v-if="mounted">
        <button @click="router.push('/projects/new')" class="btn-new">
            <Plus :size="16" />
            新建项目
        </button>
    </Teleport>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { Layers, Play, Square, RotateCw, Eye, Plus, Trash2 } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useToastStore } from '@/stores/toast';
import { request } from '@/utils/request';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';

interface ProjectContainer {
    id: string;
    name: string;
    state: string;
    status: string;
    service: string;
}

interface Project {
    name: string;
    status: 'running' | 'partial' | 'stopped';
    runningCount: number;
    totalCount: number;
    workingDir: string;
    isControlled: boolean;
    containers: ProjectContainer[];
}

const router = useRouter();
const toastStore = useToastStore();
const projects = ref<Project[]>([]);
const loading = ref(true);
const actionLoading = ref<string | null>(null);
const mounted = ref(false);

const showConfirmRemove = ref(false);
const pendingProjectName = ref('');

let intervalId: number;

const fetchProjects = async () => {
    try {
        const data = await request('/api/projects');
        if (Array.isArray(data)) {
            projects.value = data;
        }
    } catch (err) {
        console.error(err);
    } finally {
        loading.value = false;
    }
};

const handleProjectAction = async (name: string, action: string) => {
    actionLoading.value = `${name}-${action}`;
    try {
        await request(`/api/projects/${name}/action`, {
            method: 'POST',
            body: JSON.stringify({ action })
        });
        toastStore.addToast(`项目 ${name} 已执行 ${action}`, 'success');
        fetchProjects();
    } catch (err: any) {
        toastStore.addToast(`操作失败: ${err.message}`, 'error');
    } finally {
        actionLoading.value = null;
    }
};

const confirmRemove = (name: string) => {
    pendingProjectName.value = name;
    showConfirmRemove.value = true;
};

const handleConfirmRemove = () => {
    handleProjectAction(pendingProjectName.value, 'remove');
};

const isActing = (name: string) => {
    return actionLoading.value?.startsWith(name);
};

const getStatusText = (status: string) => {
    switch (status) {
        case 'running': return '全部运行中';
        case 'partial': return '部分运行中';
        case 'stopped': return '已停止';
        default: return status;
    }
};

const getStatusClass = (status: string) => {
    switch (status) {
        case 'running': return 'running';
        case 'stopped': return 'exited';
        case 'partial': return 'paused';
        default: return 'paused';
    }
};

onMounted(() => {
    mounted.value = true;
    fetchProjects();
    intervalId = window.setInterval(fetchProjects, 5000);
});

onUnmounted(() => {
    clearInterval(intervalId);
});
</script>

<style scoped>
.project-manager {
    height: 100%;
    display: flex;
    flex-direction: column;
}

.table-container {
    background: var(--card-background);
    border: 1px solid var(--card-border);
    border-radius: 12px;
    overflow: auto;
    flex: 1;
    position: relative;
    transition: background 0.3s ease, border-color 0.3s ease;
}

.table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
    table-layout: fixed;
}

.table th {
    text-align: left;
    padding: 16px;
    border-bottom: 1px solid var(--card-border);
    color: var(--system-gray);
    font-weight: 500;
    white-space: nowrap;
    position: sticky;
    top: 0;
    background: var(--window-header);
    backdrop-filter: blur(10px);
    z-index: 10;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    transition: background 0.3s ease, border-color 0.3s ease, color 0.3s ease;
}

.table td {
    padding: 12px 16px;
    border-bottom: 1px solid var(--card-border);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: border-color 0.3s ease;
}

.table tr:last-child td {
    border-bottom: none;
}

.row:hover {
    background: var(--item-hover);
}

.project-name-cell {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    width: fit-content;
    padding: 2px 4px;
    margin-left: -4px;
    border-radius: 4px;
    transition: all 0.2s;
}

.project-name-cell:hover .project-title {
    color: var(--system-blue);
    text-decoration: underline;
}

.project-title {
    font-weight: 600;
    transition: color 0.2s;
}

.uncontrolled-badge {
    font-size: 9px;
    background: var(--item-hover);
    color: #ff9f0a;
    padding: 1px 4px;
    border-radius: 4px;
    white-space: nowrap;
}

.status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--system-gray);
}

.status-dot.running { background: var(--system-green); }
.status-dot.exited { background: var(--system-gray); }
.status-dot.paused { background: var(--system-orange); }

.action-buttons {
    display: flex;
    gap: 8px;
}

.btn {
    border: 1px solid transparent;
    border-radius: 6px;
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    background: transparent;
    color: var(--foreground);
    opacity: 0.6;
}

.btn:hover:not(:disabled) {
    background: var(--item-hover);
    transform: scale(1.05);
    opacity: 1;
}

.btn:active:not(:disabled) {
    transform: scale(0.95);
}

.btn:disabled {
    opacity: 0.2;
    cursor: not-allowed;
}

.btn-start { color: var(--system-green); }
.btn-stop { color: var(--system-orange); }
.btn-restart { color: var(--system-blue); }
.btn-delete { color: var(--system-red); }

.btn-new {
    background: var(--system-blue);
    border: none;
    border-radius: 6px;
    padding: 6px 16px;
    color: white;
    font-size: 13px;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 500;
    transition: opacity 0.2s;
}

.btn-new:hover {
    opacity: 0.9;
}
</style>
