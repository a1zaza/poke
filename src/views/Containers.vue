<template>
  <DashboardLayout>
    <div class="resource-page">
      <ResourceToolbar
        v-model="search"
        :total="containers.length"
        :filtered="filtered.length"
        :loading="loading"
        placeholder="搜索容器名称、镜像或 ID"
        @refresh="fetchContainers"
      />
      <Teleport v-if="mounted" to="#header-action"
        ><button
          class="secondary"
          :disabled="updates.checking.value || !containers.length"
          @click="
            updates.sync(
              containers.map((container) => container.Image),
              true
            )
          "
        >
          <RefreshCw :size="14" :class="{ spin: updates.checking.value }" />检查更新
        </button></Teleport
      >
      <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 100px">ID</th>
              <th style="width: 24%">名称</th>
              <th style="width: 32%">镜像</th>
              <th style="width: 14%">状态</th>
              <th style="width: 248px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && !containers.length">
              <td colspan="5" class="table-message">正在加载资源…</td>
            </tr>
            <tr v-else-if="!filtered.length">
              <td colspan="5" class="table-message">
                {{
                  error
                    ? '资源暂不可用，请检查连接后重试。'
                    : search
                      ? '没有匹配的资源，请尝试其他关键词。'
                      : '当前节点暂无资源。'
                }}
              </td>
            </tr>
            <tr v-else v-for="container in filtered" :key="container.Id" class="row">
              <td class="id-cell">
                <router-link :to="`/containers/${container.Id}`" :title="container.Id">{{
                  container.Id.substring(0, 12)
                }}</router-link>
              </td>
              <td>
                <router-link
                  class="container-name-cell"
                  :to="`/containers/${container.Id}`"
                  :title="getContainerName(container)"
                >
                  <span class="container-name">{{ getContainerName(container) }}</span>
                  <span
                    v-if="!container.isControlled"
                    class="uncontrolled-badge"
                    title="该容器不属于 Poke 受控目录中的 Compose 项目"
                    >不受控</span
                  >
                </router-link>
              </td>
              <td>
                <div class="container-image-cell">
                  <span class="container-image-name" :title="container.Image">{{ container.Image }}</span>
                  <span v-if="updatingIds.has(container.Id)" class="update-badge checking">更新中</span>
                  <span
                    v-else-if="!canCheckImage(container.Image)"
                    class="update-badge checking"
                    title="镜像按 ID 或摘要固定，不检测标签更新"
                    >已固定</span
                  >
                  <span
                    v-else-if="updates.states.value[container.Image]?.status === 'checking'"
                    class="update-badge checking"
                    >检测中</span
                  >
                  <button
                    v-else-if="updates.states.value[container.Image]?.status === 'available'"
                    type="button"
                    class="update-badge available"
                    title="更新此容器"
                    @click="promptUpdate(container)"
                  >
                    有更新
                  </button>
                  <span
                    v-else-if="updates.states.value[container.Image]?.status === 'latest'"
                    class="update-badge latest"
                    title="该镜像标签暂无更新"
                    >最新</span
                  >
                  <span
                    v-else-if="updates.states.value[container.Image]?.status === 'error'"
                    class="update-badge error"
                    :title="updates.states.value[container.Image]?.error"
                    >检测失败</span
                  >
                </div>
              </td>
              <td>
                <span class="status" :class="getStatusClass(container.State)"></span>
                {{ container.Status }}
              </td>
              <td>
                <div class="action-buttons">
                  <button class="btn" @click="viewDetail(container.Id)" title="查看详情">
                    <Eye :size="14" />
                  </button>
                  <button class="btn" @click="viewLogs(container.Id)" title="查看日志">
                    <FileText :size="14" />
                  </button>
                  <button
                    class="btn"
                    @click="viewExec(container.Id)"
                    :disabled="container.State !== 'running'"
                    title="打开终端"
                    :style="{ opacity: container.State !== 'running' ? 0.3 : 1 }"
                  >
                    <Terminal :size="14" />
                  </button>

                  <button
                    v-if="container.State !== 'running'"
                    class="btn btn-start"
                    @click="handleAction(container.Id, 'start')"
                    :disabled="actionLoading === container.Id"
                    title="启动"
                  >
                    <Play :size="14" fill="currentColor" />
                  </button>
                  <button
                    v-else
                    class="btn btn-stop"
                    @click="handleAction(container.Id, 'stop')"
                    :disabled="actionLoading === container.Id"
                    title="停止"
                  >
                    <Square :size="14" fill="currentColor" />
                  </button>

                  <button
                    class="btn btn-restart"
                    @click="handleAction(container.Id, 'restart')"
                    :disabled="actionLoading === container.Id"
                    title="重启"
                  >
                    <RotateCw :size="14" />
                  </button>

                  <button
                    class="btn btn-delete"
                    @click="promptDelete(container.Id)"
                    :disabled="actionLoading === container.Id || container.State === 'running'"
                    title="删除"
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
        :isOpen="!!deleteId"
        @close="deleteId = null"
        @confirm="confirmDelete"
        title="删除容器"
        message="确定要强制删除这个容器吗？此操作不可撤销。"
        :isDanger="true"
      />

      <!-- Update Modal -->
      <ConfirmModal
        v-if="updateContainer"
        :isOpen="!!updateContainer"
        @close="updateContainer = null"
        @confirm="confirmUpdate"
        title="更新容器镜像"
        :message="updateMessage"
      />

      <!-- Unmanaged Alert Modal -->
      <ConfirmModal
        :isOpen="showUnmanagedAlert"
        @close="showUnmanagedAlert = false"
        @confirm="showUnmanagedAlert = false"
        title="容器无法自动更新"
        :message="unmanagedMessage"
        :isDanger="false"
      >
        <template #footer>
          <div style="display: flex; justify-content: flex-end">
            <button class="btn-confirm" @click="showUnmanagedAlert = false">知道了</button>
          </div>
        </template>
      </ConfirmModal>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import ResourceToolbar from '@/components/ui/ResourceToolbar.vue';
import { canCheckImage, useImageUpdates } from '@/composables/useImageUpdates';
import { useResourceFilter } from '@/composables/useResourceFilter';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';
import { useToastStore } from '@/stores/toast';
import { Play, Square, RotateCw, Trash2, Eye, FileText, Terminal, RefreshCw } from 'lucide-vue-next';
import { request } from '@/utils/request';

interface Container {
  Id: string;
  Names: string[];
  Image: string;
  State: string;
  Status: string;
  Labels?: Record<string, string>;
  isControlled: boolean;
}

const router = useRouter();
const toastStore = useToastStore();

const containers = ref<Container[]>([]);
const { search, filtered } = useResourceFilter(containers, (item) => [
  item.Id,
  ...(item.Names || []),
  item.Image,
  item.State
]);
const error = ref('');
let refreshing = false;
const mounted = ref(false);
const updatingIds = ref(new Set<string>());
const updates = useImageUpdates((image, signal) =>
  request('/api/images/check-update', { method: 'POST', body: JSON.stringify({ image }), signal })
);
let refreshAfterUpdate: ReturnType<typeof setTimeout> | undefined;

const loading = ref(true);
const actionLoading = ref<string | null>(null);
const deleteId = ref<string | null>(null);
const updateContainer = ref<Container | null>(null);
const updateMessage = ref('');
const showUnmanagedAlert = ref(false); // New Alert State
const unmanagedMessage = ref('');

let intervalId: number;

const fetchContainers = async () => {
  if (refreshing) return;
  refreshing = true;
  loading.value = true;
  try {
    const data = await request('/api/containers');
    if (Array.isArray(data)) {
      const newContainers = data.sort((a, b) => a.Id.localeCompare(b.Id));
      containers.value = newContainers;
      updates.sync(newContainers.map((container) => container.Image));
    }
    error.value = '';
  } catch (err) {
    error.value = err instanceof Error ? err.message : '资源加载失败，请重试';
  } finally {
    loading.value = false;
    refreshing = false;
  }
};

onMounted(() => {
  mounted.value = true;
  fetchContainers();
  intervalId = window.setInterval(fetchContainers, 3000);
});

onUnmounted(() => {
  clearInterval(intervalId);
  if (refreshAfterUpdate) clearTimeout(refreshAfterUpdate);
  updates.stop();
});

const getContainerName = (c: Container) => {
  return c.Names && c.Names.length > 0 && c.Names[0] ? c.Names[0].replace('/', '') : c.Id.substring(0, 12);
};

const getStatusClass = (state: string) => {
  switch (state.toLowerCase()) {
    case 'running':
      return 'running';
    case 'exited':
      return 'exited';
    case 'paused':
      return 'paused';
    default:
      return 'exited';
  }
};

const viewDetail = (id: string) => router.push(`/containers/${id}`);
const viewLogs = (id: string) => router.push(`/containers/${id}/logs`);
const viewExec = (id: string) => router.push(`/containers/${id}/exec`);

const handleAction = async (id: string, action: string) => {
  actionLoading.value = id;
  try {
    await request(`/api/containers/${id}/${action}`, {
      method: 'POST'
    });

    toastStore.addToast(`容器已${action === 'delete' ? '删除' : '更新'}`, 'success');
    await fetchContainers();
  } catch (err) {
    toastStore.addToast(`操作失败: ${err}`, 'error');
  } finally {
    actionLoading.value = null;
  }
};

const promptDelete = (id: string) => {
  deleteId.value = id;
};

const confirmDelete = async () => {
  if (!deleteId.value) return;

  actionLoading.value = deleteId.value;
  try {
    await request(`/api/containers/${deleteId.value}?force=true`, {
      method: 'DELETE'
    });

    toastStore.addToast('容器已删除', 'success');
    await fetchContainers();
  } catch (err: any) {
    toastStore.addToast(`删除失败: ${err.message}`, 'error');
  } finally {
    actionLoading.value = null;
    deleteId.value = null;
  }
};

const promptUpdate = (container: Container) => {
  // 1. Frontend Pre-check: Check labels immediately
  const labels = container.Labels || {};
  const isManaged =
    labels['com.docker.compose.project'] &&
    labels['com.docker.compose.project.working_dir'] &&
    labels['com.docker.compose.service'];

  if (!isManaged) {
    unmanagedMessage.value =
      '无法自动更新：该容器未被 Docker Compose 管理，或缺少必要的标签信息。请使用命令行手动更新。';
    showUnmanagedAlert.value = true;
    return;
  }

  updateContainer.value = container;
  updateMessage.value = `检测到镜像 ${container.Image} 有新版本。确定要拉取更新并重启容器吗？`;
};

const confirmUpdate = async () => {
  if (!updateContainer.value) return;

  const id = updateContainer.value.Id;
  const image = updateContainer.value.Image;
  updatingIds.value.add(id);
  updateContainer.value = null;

  try {
    await request(`/api/containers/${id}/update`, {
      method: 'POST'
    });

    toastStore.addToast(`容器已开始更新`, 'success');
    // 等待容器状态刷新。
    updates.invalidate(image);
    refreshAfterUpdate = setTimeout(fetchContainers, 2000);
  } catch (err: any) {
    if (err.code === 'NOT_COMPOSE_MANAGED') {
      unmanagedMessage.value =
        '无法自动更新：容器对应的项目路径不存在或无法访问（可能位于宿主机或其他未挂载路径）。';
      showUnmanagedAlert.value = true;
    } else {
      toastStore.addToast(`更新失败: ${err.message}`, 'error');
    }
  } finally {
    updatingIds.value.delete(id);
  }
};
</script>
