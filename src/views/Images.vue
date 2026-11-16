<template>
  <DashboardLayout>
    <div class="container-manager">
      <Teleport to="#header-action" v-if="mounted">
        <div class="header-controls">
          <div class="search-box">
            <input
              v-model="pullImageName"
              placeholder="拉取镜像 (e.g. nginx)"
              @keydown.enter="handlePull"
              autocomplete="off"
            />
            <button @click="handlePull" :disabled="pulling || !pullImageName" class="btn-pull">
               <Loader2 v-if="pulling" class="spin" :size="14" />
               <Download v-else :size="14" />
            </button>
          </div>

          <button @click="showPruneConfirm = true" class="btn-prune" :disabled="pruning">
             <Loader2 v-if="pruning" class="spin" :size="14" />
             <Eraser v-else :size="14" />
             清理未使用
          </button>
        </div>
      </Teleport>

      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 120px">ID</th>
              <th style="width: 30%">标签 (Tag)</th>
              <th style="width: 20%">大小</th>
              <th style="width: 25%">创建时间</th>
              <th style="width: 100px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && images.length === 0">
              <td colspan="5" style="text-align: center">加载中...</td>
            </tr>
            <tr v-else v-for="image in images" :key="image.Id" class="row">
              <td class="id-cell" :title="image.Id">
                {{ image.Id.split(':')[1]?.substring(0, 12) || image.Id.substring(0, 12) }}
              </td>
              <td>
                <template v-if="image.RepoTags && image.RepoTags.length > 0">
                  <div v-for="tag in image.RepoTags" :key="tag" class="tag-badge">
                    {{ tag }}
                  </div>
                </template>
                <span v-else style="opacity: 0.5">&lt;none&gt;</span>
              </td>
              <td>{{ formatSize(image.Size) }}</td>
              <td>{{ formatDate(image.Created) }}</td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn btn-delete"
                    @click="deleteId = image.Id"
                    :disabled="actionLoading === image.Id"
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
        title="删除镜像"
        message="确定要删除这个镜像吗？操作无法撤销。"
        @confirm="handleDelete"
        @close="deleteId = null"
        :isDanger="true"
      />

      <ConfirmModal
        :isOpen="showPruneConfirm"
        title="清理镜像"
        @confirm="handlePrune"
        @close="showPruneConfirm = false"
        :isDanger="true"
      >
        <div style="display: flex; flex-direction: column; gap: 12px;">
            <p>确定要清理未使用镜像吗？</p>
            <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
                <input type="checkbox" v-model="pruneAll" />
                <span>同时清理所有未使用的镜像 (不仅仅是无标签的悬空镜像)</span>
            </label>
            <p style="font-size: 12px; opacity: 0.7; color: var(--system-red);">
                {{ pruneAll ? '⚠️ 注意：这将删除所有当前未被运行容器使用的镜像，即使它们有标签。请谨慎操作。' : '仅删除无标签的悬空镜像 (dangling images)。' }}
            </p>
        </div>
      </ConfirmModal>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { Trash2, Download, Eraser, Loader2 } from 'lucide-vue-next';
import DashboardLayout from '@/layouts/DashboardLayout.vue';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';
import { useToastStore } from '@/stores/toast';
import { request } from '@/utils/request';

interface Image {
    Id: string;
    RepoTags: string[];
    Size: number;
    Created: number;
}

const toastStore = useToastStore();
const images = ref<Image[]>([]);
const loading = ref(true);
const actionLoading = ref<string | null>(null);
const deleteId = ref<string | null>(null);
const mounted = ref(false);

// Pull & Prune State
const pullImageName = ref('');
const pulling = ref(false);
const pruning = ref(false);
const showPruneConfirm = ref(false);
const pruneAll = ref(false);

let intervalId: number;

const fetchImages = async () => {
    try {
        const data = await request('/api/images');
        if (Array.isArray(data)) {
            images.value = data.sort((a: Image, b: Image) => a.Id.localeCompare(b.Id));
        }
    } catch (err) {
        console.error(err);
    } finally {
        loading.value = false;
    }
};

const handlePull = async () => {
    if (!pullImageName.value.trim()) return;
    pulling.value = true;
    toastStore.addToast(`开始拉取镜像: ${pullImageName.value}`, 'info');

    try {
        await request('/api/images/pull', {
            method: 'POST',
            body: JSON.stringify({ image: pullImageName.value })
        });
        toastStore.addToast('镜像拉取成功', 'success');
        pullImageName.value = '';
        fetchImages();
    } catch (err: any) {
        toastStore.addToast(err.message, 'error');
    } finally {
        pulling.value = false;
    }
};

const handlePrune = async () => {
    pruning.value = true;
    try {
        await request(`/api/images/prune?all=${pruneAll.value}`, { method: 'POST' });
        toastStore.addToast('清理命令已发送', 'success');
        fetchImages();
    } catch (err: any) {
        toastStore.addToast(err.message, 'error');
    } finally {
        pruning.value = false;
        showPruneConfirm.value = false;
    }
};

const handleDelete = async () => {
    if (!deleteId.value) return;

    setActionLoading(deleteId.value);
    try {
        await request(`/api/images/${deleteId.value}?force=true`, { method: 'DELETE' });
        toastStore.addToast('镜像已删除', 'success');
        fetchImages();
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

const formatSize = (bytes: number) => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) return '0 Byte';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return Math.round(bytes / Math.pow(1024, i)) + ' ' + sizes[i];
};

const formatDate = (timestamp: number) => {
    return new Date(timestamp * 1000).toLocaleString();
};

onMounted(() => {
    mounted.value = true;
    fetchImages();
    intervalId = window.setInterval(fetchImages, 5000);
});

onUnmounted(() => {
    clearInterval(intervalId);
});
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
    margin-right: 4px;
    margin-bottom: 2px;
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

.header-controls {
    display: flex;
    gap: 12px;
    align-items: center;
}

.search-box {
    display: flex;
    gap: 0;
    background: var(--input-background);
    border-radius: 6px;
    overflow: hidden;
    border: 1px solid var(--glass-border);
    height: 32px;
}

.search-box input {
    background: transparent;
    border: none;
    padding: 0 12px;
    color: var(--foreground);
    width: 200px;
    outline: none;
    font-size: 13px;
    caret-color: var(--system-blue);
    height: 100%;
}

.btn-pull {
    background: var(--system-blue);
    border: none;
    padding: 0 12px;
    color: white;
    cursor: pointer;
    display: flex;
    align-items: center;
    height: 100%;
    transition: opacity 0.2s;
}

.btn-pull:disabled {
    opacity: 0.5;
    cursor: not-allowed;
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
