<template>
  <DashboardLayout>
    <div class="resource-page">
      <ResourceToolbar
        v-model="search"
        :total="networks.length"
        :filtered="filtered.length"
        :loading="loading"
        placeholder="搜索网络名称、驱动或子网"
        @refresh="fetchNetworks"
      />
      <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 120px">ID</th>
              <th style="width: 25%">名称</th>
              <th style="width: 15%">驱动 (Driver)</th>
              <th style="width: 10%">范围</th>
              <th style="width: 20%">子网 (Subnet)</th>
              <th style="width: 100px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && !networks.length">
              <td colspan="6" class="table-message">正在加载资源…</td>
            </tr>
            <tr v-else-if="!filtered.length">
              <td colspan="6" class="table-message">
                {{
                  error
                    ? '资源暂不可用，请检查连接后重试。'
                    : search
                      ? '没有匹配的资源，请尝试其他关键词。'
                      : '当前节点暂无资源。'
                }}
              </td>
            </tr>
            <tr v-else v-for="net in filtered" :key="net.Id" class="row">
              <td class="id-cell" :title="net.Id">{{ net.Id.substring(0, 12) }}</td>
              <td>
                <div style="display: flex; align-items: center; gap: 6px">
                  <Network :size="14" style="opacity: 0.5" />
                  {{ net.Name }}
                </div>
              </td>
              <td>
                <div class="tag-badge">{{ net.Driver }}</div>
              </td>
              <td style="opacity: 0.7">{{ net.Scope }}</td>
              <td style="font-size: 12px">
                {{ net.IPAM?.Config?.[0]?.Subnet || '-' }}
              </td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn btn-delete"
                    @click="promptDelete(net.Id, net.Name)"
                    :disabled="actionLoading === net.Id"
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
        title="删除网络"
        message="确定要删除这个网络吗？如果有容器正在使用它，删除可能会失败。"
        @confirm="handleDelete"
        @close="deleteId = null"
        :isDanger="true"
      />
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { Trash2, Network } from 'lucide-vue-next';
import ResourceToolbar from '@/components/ui/ResourceToolbar.vue';
import { useResourceFilter } from '@/composables/useResourceFilter';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';
import { useToastStore } from '@/stores/toast';
import { request } from '@/utils/request';

interface NetworkInfo {
  Id: string;
  Name: string;
  Driver: string;
  Scope: string;
  IPAM?: {
    Config?: Array<{
      Subnet?: string;
      Gateway?: string;
    }>;
  };
  Created: string;
}

const toastStore = useToastStore();
const networks = ref<NetworkInfo[]>([]);
const { search, filtered } = useResourceFilter(networks, (item) => [
  item.Name,
  item.Id,
  item.Driver,
  item.IPAM?.Config?.[0]?.Subnet
]);
const error = ref('');
let refreshing = false;
const loading = ref(true);
const actionLoading = ref<string | null>(null);
const deleteId = ref<string | null>(null);

let intervalId: number;

const fetchNetworks = async () => {
  if (refreshing) return;
  refreshing = true;
  loading.value = true;
  try {
    const data = await request('/api/networks');
    if (Array.isArray(data)) {
      networks.value = data.sort((a: NetworkInfo, b: NetworkInfo) => a.Id.localeCompare(b.Id));
    }
    error.value = '';
  } catch (err) {
    error.value = err instanceof Error ? err.message : '资源加载失败，请重试';
  } finally {
    loading.value = false;
    refreshing = false;
  }
};

const promptDelete = (id: string, name: string) => {
  // Prevent deleting predefined networks
  if (['bridge', 'host', 'none'].includes(name)) {
    toastStore.addToast('无法删除系统默认网络', 'error');
    return;
  }
  deleteId.value = id;
};

const handleDelete = async () => {
  if (!deleteId.value) return;

  setActionLoading(deleteId.value);
  try {
    await request(`/api/networks/${deleteId.value}`, { method: 'DELETE' });
    toastStore.addToast('网络已删除', 'success');
    fetchNetworks();
  } catch (err: any) {
    toastStore.addToast(err.message, 'error');
  } finally {
    setActionLoading(null);
    deleteId.value = null;
  }
};

const setActionLoading = (id: string | null) => {
  actionLoading.value = id;
};

onMounted(() => {
  fetchNetworks();
  intervalId = window.setInterval(fetchNetworks, 5000);
});

onUnmounted(() => {
  clearInterval(intervalId);
});
</script>
