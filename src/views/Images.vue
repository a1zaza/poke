<template>
  <DashboardLayout>
    <div class="resource-page">
      <ResourceToolbar
        v-model="search"
        :total="images.length"
        :filtered="filtered.length"
        :loading="loading"
        placeholder="搜索镜像标签或 ID"
        @refresh="fetchImages"
      />
      <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
      <Teleport to="#header-action" v-if="mounted">
        <div class="header-controls">
          <div class="search-box">
            <input
              v-model="pullImageName"
              placeholder="镜像名称，例如 nginx:alpine"
              aria-label="要拉取的镜像名称"
              @keydown.enter="handlePull"
              autocomplete="off"
            />
            <button
              @click="handlePull"
              :disabled="pulling || !pullImageName"
              class="btn-pull"
              title="拉取镜像"
              aria-label="拉取镜像"
            >
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
            <tr v-if="loading && !images.length">
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
            <tr v-else v-for="image in filtered" :key="image.Id" class="row">
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
        <div style="display: flex; flex-direction: column; gap: 12px">
          <p>确定要清理未使用镜像吗？</p>
          <label style="display: flex; align-items: center; gap: 8px; cursor: pointer">
            <input type="checkbox" v-model="pruneAll" />
            <span>同时清理所有未使用的镜像 (不仅仅是无标签的悬空镜像)</span>
          </label>
          <p style="font-size: 12px; opacity: 0.7; color: var(--system-red)">
            {{
              pruneAll
                ? '⚠️ 注意：这将删除所有当前未被运行容器使用的镜像，即使它们有标签。请谨慎操作。'
                : '仅删除无标签的悬空镜像 (dangling images)。'
            }}
          </p>
        </div>
      </ConfirmModal>
    </div>
  </DashboardLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { Trash2, Download, Eraser, Loader2 } from 'lucide-vue-next';
import ResourceToolbar from '@/components/ui/ResourceToolbar.vue';
import { useResourceFilter } from '@/composables/useResourceFilter';
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
const { search, filtered } = useResourceFilter(images, (item) => [item.Id, ...(item.RepoTags || [])]);
const error = ref('');
let refreshing = false;
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
  if (refreshing) return;
  refreshing = true;
  loading.value = true;
  try {
    const data = await request('/api/images');
    if (Array.isArray(data)) {
      images.value = data.sort((a: Image, b: Image) => a.Id.localeCompare(b.Id));
    }
    error.value = '';
  } catch (err) {
    error.value = err instanceof Error ? err.message : '资源加载失败，请重试';
  } finally {
    loading.value = false;
    refreshing = false;
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
