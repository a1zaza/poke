<template>
  <DashboardLayout>
    <div class="container-manager">
      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 100px">ID</th>
              <th style="width: 15%">名称</th>
              <th style="width: 45%">镜像</th>
              <th style="width: 15%">状态</th>
              <th style="width: 210px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && containers.length === 0">
              <td colspan="5" style="text-align: center">加载中...</td>
            </tr>
            <tr v-else v-for="container in containers" :key="container.Id" class="row">
              <td
                class="id-cell clickable"
                @click="viewDetail(container.Id)"
              >
                {{ container.Id.substring(0, 12) }}
              </td>
              <td
                class="container-name-link"
                @click="viewDetail(container.Id)"
              >
                <div class="container-name-cell">
                  <span class="container-name">{{ getContainerName(container) }}</span>
                  <span
                    v-if="!container.isControlled"
                    class="uncontrolled-badge"
                    title="该容器不属于 Poke 受控目录中的 Compose 项目"
                  >不受控</span>
                </div>
              </td>
              <td :title="container.Image">
                {{ container.Image }}

                <span v-if="getUpdateStatus(container.Id) === 'checking'" class="update-badge checking">...</span>
                <span v-if="getUpdateStatus(container.Id) === 'available'"
                      class="update-badge available"
                      title="点击更新容器"
                      @click="promptUpdate(container)"
                >有更新</span>
                <span v-if="getUpdateStatus(container.Id) === 'updating'" class="update-badge checking">更新中</span>
                <span v-if="getUpdateStatus(container.Id) === 'latest'" class="update-badge latest" title="已是最新版本">最新</span>
                <span v-if="getUpdateStatus(container.Id) === 'error'" class="update-badge error" :title="getUpdateError(container.Id) || '检测失败'">失败</span>
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
                    title="终端/Exec"
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
            <div style="display: flex; justify-content: flex-end;">
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
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';
import { useToastStore } from '@/stores/toast';
import { Play, Square, RotateCw, Trash2, Eye, FileText, Terminal } from 'lucide-vue-next';
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
const updateStates = ref<Record<string, { status: string, error?: string }>>({});

const loading = ref(true);
const actionLoading = ref<string | null>(null);
const deleteId = ref<string | null>(null);
const updateContainer = ref<Container | null>(null);
const updateMessage = ref('');
const showUnmanagedAlert = ref(false); // New Alert State
const unmanagedMessage = ref('');


let intervalId: number;

const fetchContainers = async () => {
    try {
        const data = await request('/api/containers');
        if (Array.isArray(data)) {
            const newContainers = data.sort((a, b) => a.Id.localeCompare(b.Id));
            containers.value = newContainers;
            checkUpdates();
        }
    } catch (err) {
        console.error(err);
    } finally {
        loading.value = false;
    }
};

onMounted(() => {
    fetchContainers();
    intervalId = window.setInterval(fetchContainers, 3000);
});

onUnmounted(() => {
    clearInterval(intervalId);
});

const getContainerName = (c: Container) => {
    return c.Names && c.Names.length > 0 && c.Names[0] ? c.Names[0].replace('/', '') : c.Id.substring(0, 12);
};

const getStatusClass = (state: string) => {
    switch (state.toLowerCase()) {
        case 'running': return 'running';
        case 'exited': return 'exited';
        case 'paused': return 'paused';
        default: return 'exited';
    }
};

const viewDetail = (id: string) => router.push(`/containers/${id}`);
const viewLogs = (id: string) => router.push(`/containers/${id}/logs`);
const viewExec = (id: string) => router.push(`/containers/${id}/exec`);

const handleAction = async (id: string, action: string) => {
    actionLoading.value = id;
    try {
        await request(`/api/containers/${id}/${action}`, {
            method: 'POST',
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
    const isManaged = labels['com.docker.compose.project'] &&
                      labels['com.docker.compose.project.working_dir'] &&
                      labels['com.docker.compose.service'];

    if (!isManaged) {
        unmanagedMessage.value = "无法自动更新：该容器未被 Docker Compose 管理，或缺少必要的标签信息。请使用命令行手动更新。";
        showUnmanagedAlert.value = true;
        return;
    }

    updateContainer.value = container;
    updateMessage.value = `检测到镜像 ${container.Image} 有新版本。确定要拉取更新并重启容器吗？`;
};

const confirmUpdate = async () => {
    if (!updateContainer.value) return;

    const id = updateContainer.value.Id;
    updateStates.value[id] = { status: 'updating' };
    updateContainer.value = null;

    try {
        await request(`/api/containers/${id}/update`, {
            method: 'POST'
        });

        toastStore.addToast(`容器已开始更新`, 'success');
        // 等待容器状态刷新。
        setTimeout(fetchContainers, 2000);
    } catch (err: any) {
        if (err.code === 'NOT_COMPOSE_MANAGED') {
             unmanagedMessage.value = "无法自动更新：容器对应的项目路径不存在或无法访问（可能位于宿主机或其他未挂载路径）。";
             showUnmanagedAlert.value = true;
        } else {
             toastStore.addToast(`更新失败: ${err.message}`, 'error');
        }
        updateStates.value[id] = { status: 'available' }; // Revert
    }
};

const getUpdateStatus = (id: string) => updateStates.value[id]?.status;
const getUpdateError = (id: string) => updateStates.value[id]?.error;

const checkUpdates = async () => {
    for (const c of containers.value) {
        if (updateStates.value[c.Id]) continue;

        updateStates.value[c.Id] = { status: 'checking' };

        request('/api/images/check-update', {
            method: 'POST',
            body: JSON.stringify({ image: c.Image })
        }).then((res: any) => {
            if (res.error) {
                updateStates.value[c.Id] = { status: 'error', error: res.error };
            } else if (res.hasUpdate) {
                updateStates.value[c.Id] = { status: 'available' };
            } else {
                updateStates.value[c.Id] = { status: 'latest' };
            }
        }).catch((err: any) => {
             updateStates.value[c.Id] = { status: 'error', error: err.message || 'Network error' };
        });
    }

    // Cleanup old states
    const currentIds = new Set(containers.value.map(c => c.Id));
    for (const id in updateStates.value) {
        if (!currentIds.has(id)) {
            delete updateStates.value[id];
        }
    }
};
</script>

<style scoped>
.container-manager {
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

.clickable {
    cursor: pointer;
}

.clickable:hover {
    text-decoration: underline;
}

.container-name-link {
    cursor: pointer;
}

.container-name-link:hover .container-name {
    text-decoration: underline;
}

.id-cell {
    opacity: 0.7;
    font-size: 11px;
}

.container-name-cell {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
}

.container-name {
    overflow: hidden;
    text-overflow: ellipsis;
}

.uncontrolled-badge {
    flex: none;
    font-size: 9px;
    background: var(--item-hover);
    color: #ff9f0a;
    padding: 1px 4px;
    border-radius: 4px;
    white-space: nowrap;
}

.status {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 8px;
}

.status.running { background: var(--system-green); }
.status.exited { background: var(--system-gray); }
.status.paused { background: var(--system-blue); }

.row:hover {
    background: var(--item-hover);
}

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

.btn-start:hover:not(:disabled) { background: var(--system-green); color: white; }
.btn-stop:hover:not(:disabled) { background: var(--system-gray); color: white; }
.btn-restart:hover:not(:disabled) { background: var(--system-blue); color: white; }
.btn-delete:hover:not(:disabled) { background: var(--system-red); color: white; }

.update-badge {
    font-size: 9px;
    padding: 2px 5px;
    border-radius: 4px;
    white-space: nowrap;
    margin-left: 6px;
    font-weight: 600;
    cursor: pointer;
    display: inline-block;
}

.update-badge.available {
    background: rgba(10, 132, 255, 0.1);
    color: var(--system-blue);
}

.update-badge.latest {
    background: rgba(48, 209, 88, 0.1);
    color: var(--system-green);
    opacity: 0.8;
}

.update-badge.error {
    background: rgba(255, 69, 58, 0.1);
    color: var(--system-red);
}

.update-badge.checking {
    background: var(--item-hover);
    color: var(--system-gray);
    opacity: 0.5;
}

.btn-confirm {
    background: var(--system-blue);
    color: white;
    border: none;
    padding: 8px 16px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 13px;
}
</style>
