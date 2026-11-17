<template>
  <DashboardLayout>
    <div v-if="loading && !project" class="detail-state">加载项目详情中...</div>
    <div v-else-if="!project" class="detail-state error-state">未找到项目信息</div>

    <div v-else class="project-detail">
      <Teleport to="#header-action" v-if="mounted">
        <div class="header-controls">
          <template v-if="project.isControlled">
            <button class="btn btn-edit" :class="{ active: isEditing }" @click="isEditing = !isEditing">
              <component :is="isEditing ? X : FileEdit" :size="14" style="margin-right: 4px" />
              {{ isEditing ? '取消' : '编辑配置' }}
            </button>
            <button
              v-if="isEditing"
              class="btn btn-save"
              @click="handleSaveCompose"
              :disabled="actionLoading"
            >
              <Save :size="14" />
              {{ actionLoading ? '正在部署…' : '保存并重新部署' }}
            </button>
          </template>

          <template v-if="runningCount < totalCount">
            <button
              class="btn btn-start"
              @click="handleAction('start')"
              :disabled="actionLoading || !project.isControlled"
            >
              <Play :size="14" style="margin-right: 4px" /> 启动
            </button>
          </template>

          <template v-if="runningCount > 0">
            <button
              class="btn btn-stop"
              @click="confirmDown"
              :disabled="actionLoading || !project.isControlled"
            >
              <Square :size="14" fill="currentColor" style="margin-right: 4px" /> 停止
            </button>
          </template>

          <template v-if="project.isControlled">
            <button class="btn btn-delete" @click="confirmRemove" :disabled="actionLoading">
              <Trash :size="14" style="margin-right: 4px" /> 移除
            </button>
          </template>

          <template v-if="runningCount > 0">
            <button
              class="btn btn-restart"
              @click="handleAction('restart')"
              :disabled="actionLoading || !project.isControlled"
            >
              <RotateCw :size="14" style="margin-right: 4px" /> 重启
            </button>
          </template>
        </div>
      </Teleport>

      <div class="grid">
        <div v-if="!project.isControlled" class="notice-card">
          <Layers :size="16" />
          <span
            >该项目位于 <code>{{ project.workingDir }}</code>，不在受控目录
            <code>/poke/projects</code> 下，无法进行管理。</span
          >
        </div>

        <div class="card full-width">
          <div class="info-row">
            <span class="label">工作目录</span>
            <span class="value monospace">{{ project.workingDir }}</span>
          </div>
          <div class="info-row">
            <span class="label">运行状态</span>
            <span class="value"> {{ runningCount }} / {{ totalCount }} 容器正在运行 </span>
          </div>
        </div>

        <div v-if="isEditing" class="card full-width editor-card">
          <div class="editor-header">
            <h3>编辑 docker-compose.yml</h3>
          </div>
          <textarea class="textarea" v-model="editedContent" spellcheck="false"></textarea>
        </div>

        <div class="card full-width">
          <h3>服务容器 ({{ totalCount }})</h3>
          <div class="container-list">
            <div v-for="c in project.containers" :key="c.id" class="container-item">
              <Box :size="18" class="icon-box" />
              <div class="container-info">
                <div class="service-name">{{ c.service }}</div>
                <div class="container-name">{{ c.name }}</div>
              </div>
              <div class="status-col">
                <div class="status-badge" :class="c.state === 'running' ? 'running' : 'stopped'">
                  <span class="dot"></span>
                  {{ c.state }}
                </div>
                <div class="status-text">{{ c.status }}</div>
              </div>
              <div class="extra-icon"><HardDrive :size="14" /></div>
              <div class="image-name">{{ c.image }}</div>
            </div>
          </div>
        </div>
      </div>

      <ConfirmModal
        :isOpen="showConfirmDown"
        title="确认停止"
        message="确定要停止此项目并移除所有容器吗？(docker compose down)"
        :isDanger="true"
        @confirm="handleConfirmDown"
        @close="showConfirmDown = false"
      />

      <ConfirmModal
        :isOpen="showConfirmRemove"
        title="确认移除项目"
        message="确定要移除此项目吗？这将停止容器并永久删除项目文件夹及 docker-compose.yml 文件。"
        :isDanger="true"
        @confirm="handleConfirmRemove"
        @close="showConfirmRemove = false"
      />
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Play, Square, RotateCw, Box, Layers, HardDrive, FileEdit, Save, X, Trash } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import { useToastStore } from '@/stores/toast';
import { useHeaderStore } from '@/stores/header';
import { request } from '@/utils/request';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';

const route = useRoute();
const router = useRouter();
const toastStore = useToastStore();
const headerStore = useHeaderStore();

const name = route.params.name as string;
const project = ref<any>(null);
const loading = ref(true);
const actionLoading = ref(false);
const isEditing = ref(false);
const editedContent = ref('');
const mounted = ref(false);

const showConfirmDown = ref(false);
const showConfirmRemove = ref(false);

let intervalId: number;

const runningCount = computed(() => {
  if (!project.value) return 0;
  return project.value.containers.filter((c: any) => c.state === 'running').length;
});

const totalCount = computed(() => {
  if (!project.value) return 0;
  return project.value.containers.length;
});

const fetchDetail = async () => {
  try {
    const data = await request(`/api/projects/${name}`);
    project.value = data;

    // Update header title
    headerStore.set(data.name, true);

    // Initial load content if not editing
    if (!isEditing.value && data.isControlled) {
      editedContent.value = data.composeContent || '';
    }
  } catch (err: any) {
    toastStore.addToast(err.message, 'error');
  } finally {
    loading.value = false;
  }
};

const handleAction = async (action: string) => {
  actionLoading.value = true;
  try {
    await request(`/api/projects/${name}/action`, {
      method: 'POST',
      body: JSON.stringify({ action })
    });
    toastStore.addToast(`项目操作成功: ${action}`, 'success');
    fetchDetail();
  } catch (err: any) {
    toastStore.addToast(err.message, 'error');
  } finally {
    actionLoading.value = false;
  }
};

const handleSaveCompose = async () => {
  actionLoading.value = true;
  try {
    await request(`/api/projects/${name}`, {
      method: 'PUT',
      body: JSON.stringify({ content: editedContent.value })
    });
    toastStore.addToast('项目配置已更新并重新部署', 'success');
    isEditing.value = false;
    fetchDetail();
  } catch (err: any) {
    toastStore.addToast(err.message, 'error');
  } finally {
    actionLoading.value = false;
  }
};

const confirmDown = () => {
  showConfirmDown.value = true;
};

const handleConfirmDown = () => {
  showConfirmDown.value = false;
  handleAction('down');
};

const confirmRemove = () => {
  showConfirmRemove.value = true;
};

const handleConfirmRemove = async () => {
  showConfirmRemove.value = false;
  actionLoading.value = true;
  try {
    await request(`/api/projects/${name}/action`, {
      method: 'POST',
      body: JSON.stringify({ action: 'remove' })
    });
    toastStore.addToast('项目已移除', 'success');
    // Redirect to projects list
    // Redirect to projects list
    router.push('/projects');
  } catch (err: any) {
    toastStore.addToast(err.message, 'error');
  } finally {
    actionLoading.value = false;
  }
};

onMounted(() => {
  mounted.value = true;
  fetchDetail();
  intervalId = window.setInterval(() => {
    if (!isEditing.value) fetchDetail();
  }, 5000);
});

onUnmounted(() => {
  clearInterval(intervalId);
  headerStore.reset();
});
</script>
