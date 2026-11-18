<template>
  <DashboardLayout>
    <div class="resource-page">
      <ResourceToolbar
        v-model="search"
        :total="projects.length"
        :filtered="filtered.length"
        :loading="loading"
        placeholder="搜索项目名称或路径"
        @refresh="fetchProjects"
      />
      <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 25%">项目名称</th>
              <th style="width: 15%">状态</th>
              <th style="width: 45%">路径 / Compose 文件</th>
              <th style="width: 212px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && !projects.length">
              <td colspan="4" class="table-message">正在加载资源…</td>
            </tr>
            <tr v-else-if="!filtered.length">
              <td colspan="4" class="table-message">
                {{
                  error
                    ? '资源暂不可用，请检查连接后重试。'
                    : search
                      ? '没有匹配的资源，请尝试其他关键词。'
                      : '当前节点暂无资源。'
                }}
              </td>
            </tr>
            <tr v-else v-for="project in filtered" :key="project.name" class="row">
              <td>
                <router-link class="project-name-cell" :to="`/projects/${project.name}`">
                  <Layers :size="14" style="color: var(--system-blue)" />
                  <div style="display: flex; align-items: center; gap: 6px">
                    <span class="project-title">{{ project.name }}</span>
                    <span v-if="!project.isControlled" class="uncontrolled-badge">不受控</span>
                  </div>
                </router-link>
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
              <td style="opacity: 0.5; font-size: 11px">
                {{ project.workingDir }}
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn" @click="router.push(`/projects/${project.name}`)" title="查看详情">
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
                    :title="project.isControlled ? '移除项目' : '非受控项目无法操作'"
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
        :message="`确定要移除项目 ${pendingProjectName} 吗？这将停止容器并移除受管配置；远程项目会保留宿主机业务文件，本机项目会删除项目目录。`"
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
import ResourceToolbar from '@/components/ui/ResourceToolbar.vue';
import { useResourceFilter } from '@/composables/useResourceFilter';
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
  status: 'running' | 'partial' | 'stopped' | 'unknown';
  actualStateAvailable?: boolean;
  runningCount: number;
  totalCount: number;
  workingDir: string;
  isControlled: boolean;
  containers: ProjectContainer[];
}

const router = useRouter();
const toastStore = useToastStore();
const projects = ref<Project[]>([]);
const { search, filtered } = useResourceFilter(projects, (item) => [item.name, item.workingDir, item.status]);
const error = ref('');
let refreshing = false;
const loading = ref(true);
const actionLoading = ref<string | null>(null);
const mounted = ref(false);

const showConfirmRemove = ref(false);
const pendingProjectName = ref('');

let intervalId: number;

const fetchProjects = async () => {
  if (refreshing) return;
  refreshing = true;
  loading.value = true;
  try {
    const data = await request('/api/projects');
    if (Array.isArray(data)) {
      projects.value = data;
    }
    error.value = '';
  } catch (err) {
    error.value = err instanceof Error ? err.message : '资源加载失败，请重试';
  } finally {
    loading.value = false;
    refreshing = false;
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
  if (status === 'unknown') return '状态待确认';
  switch (status) {
    case 'running':
      return '全部运行中';
    case 'partial':
      return '部分运行中';
    case 'stopped':
      return '已停止';
    default:
      return status;
  }
};

const getStatusClass = (status: string) => {
  switch (status) {
    case 'running':
      return 'running';
    case 'stopped':
      return 'exited';
    case 'partial':
      return 'paused';
    default:
      return 'paused';
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
