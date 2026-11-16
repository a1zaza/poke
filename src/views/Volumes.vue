<template>
  <DashboardLayout>
    <div class="volume-manager">
      <Teleport to="#header-action" v-if="mounted">
        <button @click="handlePrune" class="btn-prune" :disabled="pruning">
             <Loader2 v-if="pruning" class="spin" :size="14" />
             <Eraser v-else :size="14" />
             清理未使用
        </button>
      </Teleport>

      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 30%">名称</th>
              <th style="width: 15%">驱动 (Driver)</th>
              <th style="width: 30%">挂载点 (Mountpoint)</th>
              <th style="width: 20%">创建时间</th>
              <th style="width: 100px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && volumes.length === 0">
              <td colspan="5" style="text-align: center">加载中...</td>
            </tr>
            <tr v-else v-for="vol in volumes" :key="vol.Name" class="row">
              <td :title="vol.Name">
                <div style="display: flex; align-items: center; gap: 6px">
                  <HardDrive :size="14" style="opacity: 0.5" />
                  {{ vol.Name.length > 50 ? vol.Name.substring(0, 20) + '...' + vol.Name.substring(vol.Name.length - 20) : vol.Name }}
                </div>
              </td>
              <td>
                <div class="tag-badge">{{ vol.Driver }}</div>
              </td>
              <td style="opacity: 0.7; font-size: 11px;" :title="vol.Mountpoint">
                {{ vol.Mountpoint.length > 40 ? '...' + vol.Mountpoint.substring(vol.Mountpoint.length - 40) : vol.Mountpoint }}
              </td>
              <td>{{ formatDate(vol.CreatedAt) }}</td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn btn-delete"
                    @click="deleteName = vol.Name"
                    :disabled="actionLoading === vol.Name"
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
        :isOpen="!!deleteName"
        title="删除存储卷"
        message="确定要删除这个存储卷吗？该操作无法撤销。请确保没有容器正在使用它。"
        @confirm="handleDelete"
        @close="deleteName = null"
        :isDanger="true"
      />
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { Trash2, HardDrive, Eraser, Loader2 } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';
import { useToastStore } from '@/stores/toast';
import { request } from '@/utils/request';

interface VolumeInfo {
    Name: string;
    Driver: string;
    Mountpoint: string;
    CreatedAt: string;
    UsageData?: {
        Size: number;
        RefCount: number;
    }
}

const toastStore = useToastStore();
const volumes = ref<VolumeInfo[]>([]);
const loading = ref(true);
const actionLoading = ref<string | null>(null);
const deleteName = ref<string | null>(null);
const pruning = ref(false);
const mounted = ref(false);

let intervalId: number;

const fetchVolumes = async () => {
    try {
        const data = await request('/api/volumes');
        // Docker volume list response has { Volumes: [...] } structure
        if (data && Array.isArray(data.Volumes)) {
             volumes.value = data.Volumes.sort((a: VolumeInfo, b: VolumeInfo) => a.Name.localeCompare(b.Name));
        } else if (Array.isArray(data)) {
             volumes.value = data.sort((a: VolumeInfo, b: VolumeInfo) => a.Name.localeCompare(b.Name));
        }
    } catch (err) {
        console.error(err);
    } finally {
        loading.value = false;
    }
};

const handlePrune = async () => {
    pruning.value = true;
    try {
        const data = await request('/api/volumes/prune', { method: 'POST' });
        // data.VolumesDeleted, data.SpaceReclaimed
        const count = data.VolumesDeleted ? data.VolumesDeleted.length : 0;

        if (count === 0) {
            toastStore.addToast('没有发现需要清理的存储卷', 'info');
        } else {
            toastStore.addToast(`清理完成: 删除了 ${count} 个存储卷`, 'success');
        }
        fetchVolumes();
    } catch (err: any) {
        toastStore.addToast(err.message, 'error');
    } finally {
        pruning.value = false;
    }
};

const handleDelete = async () => {
    if (!deleteName.value) return;

    setActionLoading(deleteName.value);
    try {
        await request(`/api/volumes/${deleteName.value}`, { method: 'DELETE' });
        toastStore.addToast('存储卷已删除', 'success');
        fetchVolumes();
    } catch (err: any) {
        toastStore.addToast(err.message, 'error');
    } finally {
        setActionLoading(null);
        deleteName.value = null;
    }
};

const setActionLoading = (id: string | null) => {
    actionLoading.value = id;
};

const formatDate = (isoString: string) => {
    if (!isoString) return '-';
    return new Date(isoString).toLocaleString();
};

onMounted(() => {
    mounted.value = true;
    fetchVolumes();
    intervalId = window.setInterval(fetchVolumes, 5000);
});

onUnmounted(() => {
    clearInterval(intervalId);
});
</script>

<style scoped>
.volume-manager {
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

.tag-badge {
    display: inline-block;
    background: var(--item-hover);
    border-radius: 4px;
    padding: 2px 6px;
    font-size: 11px;
    color: var(--foreground);
    opacity: 0.8;
}

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

.btn-delete:hover:not(:disabled) {
    background: var(--system-red);
    color: white;
}

.btn-prune {
    background: var(--item-hover);
    border: 1px solid var(--glass-border);
    border-radius: 6px;
    padding: 0 16px;
    color: var(--system-red);
    font-size: 13px;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 500;
    transition: opacity 0.2s;
    height: 32px;
}

.btn-prune:hover:not(:disabled) {
    opacity: 0.8;
}

.btn-prune:disabled {
    opacity: 0.5;
    cursor: not-allowed;
}

.spin {
    animation: spin 1s linear infinite;
}

@keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
}
</style>
