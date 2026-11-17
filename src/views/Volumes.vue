<template>
  <DashboardLayout>
    <div class="resource-page">
      <ResourceToolbar
        v-model="search"
        :total="volumes.length"
        :filtered="filtered.length"
        :loading="loading"
        placeholder="搜索存储卷名称或挂载点"
        @refresh="fetchVolumes"
      />
      <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
      <Teleport to="#header-action" v-if="mounted">
        <button @click="showPruneConfirm = true" class="btn-prune" :disabled="pruning">
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
            <tr v-if="loading && !volumes.length">
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
            <tr v-else v-for="vol in filtered" :key="vol.Name" class="row">
              <td :title="vol.Name">
                <div style="display: flex; align-items: center; gap: 6px">
                  <HardDrive :size="14" style="opacity: 0.5" />
                  {{
                    vol.Name.length > 50
                      ? vol.Name.substring(0, 20) + '...' + vol.Name.substring(vol.Name.length - 20)
                      : vol.Name
                  }}
                </div>
              </td>
              <td>
                <div class="tag-badge">{{ vol.Driver }}</div>
              </td>
              <td style="opacity: 0.7; font-size: 11px" :title="vol.Mountpoint">
                {{
                  vol.Mountpoint.length > 40
                    ? '...' + vol.Mountpoint.substring(vol.Mountpoint.length - 40)
                    : vol.Mountpoint
                }}
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
        :isOpen="showPruneConfirm"
        title="清理未使用的存储卷"
        message="这会删除所有未被容器使用的存储卷，其中的数据无法恢复。确定继续吗？"
        :isDanger="true"
        @confirm="handlePrune"
        @close="showPruneConfirm = false"
      />
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
import ResourceToolbar from '@/components/ui/ResourceToolbar.vue';
import { useResourceFilter } from '@/composables/useResourceFilter';
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
  };
}

const toastStore = useToastStore();
const volumes = ref<VolumeInfo[]>([]);
const { search, filtered } = useResourceFilter(volumes, (item) => [item.Name, item.Driver, item.Mountpoint]);
const error = ref('');
let refreshing = false;
const loading = ref(true);
const actionLoading = ref<string | null>(null);
const deleteName = ref<string | null>(null);
const pruning = ref(false);
const showPruneConfirm = ref(false);
const mounted = ref(false);

let intervalId: number;

const fetchVolumes = async () => {
  if (refreshing) return;
  refreshing = true;
  loading.value = true;
  try {
    const data = await request('/api/volumes');
    // Docker volume list response has { Volumes: [...] } structure
    if (data && Array.isArray(data.Volumes)) {
      volumes.value = data.Volumes.sort((a: VolumeInfo, b: VolumeInfo) => a.Name.localeCompare(b.Name));
    } else if (Array.isArray(data)) {
      volumes.value = data.sort((a: VolumeInfo, b: VolumeInfo) => a.Name.localeCompare(b.Name));
    }
    error.value = '';
  } catch (err) {
    error.value = err instanceof Error ? err.message : '资源加载失败，请重试';
  } finally {
    loading.value = false;
    refreshing = false;
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
