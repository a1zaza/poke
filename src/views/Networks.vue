<template>
  <DashboardLayout>
    <div class="network-manager">
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
            <tr v-if="loading && networks.length === 0">
              <td colspan="6" style="text-align: center">加载中...</td>
            </tr>
            <tr v-else v-for="net in networks" :key="net.Id" class="row">
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
const loading = ref(true);
const actionLoading = ref<string | null>(null);
const deleteId = ref<string | null>(null);

let intervalId: number;

const fetchNetworks = async () => {
    try {
        const data = await request('/api/networks');
        if (Array.isArray(data)) {
            networks.value = data.sort((a: NetworkInfo, b: NetworkInfo) => a.Id.localeCompare(b.Id));
        }
    } catch (err) {
        console.error(err);
    } finally {
        loading.value = false;
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

<style scoped>
.network-manager {
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

.id-cell {

    opacity: 0.7;
    font-size: 11px;
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
</style>
