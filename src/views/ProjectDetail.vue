<template>
  <DashboardLayout>
    <div v-if="loading && !project" class="loading">加载项目详情中...</div>
    <div v-else-if="!project" class="error">未找到项目信息</div>

    <div v-else class="project-detail">
      <Teleport to="#header-action" v-if="mounted">
        <div class="header-controls">
             <template v-if="project.isControlled">
                <button
                    class="btn btn-edit"
                    :class="{ active: isEditing }"
                    @click="isEditing = !isEditing"
                >
                    <component :is="isEditing ? X : FileEdit" :size="14" style="margin-right: 4px" />
                    {{ isEditing ? '取消' : '编辑配置' }}
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
                <button
                    class="btn btn-delete"
                    @click="confirmRemove"
                    :disabled="actionLoading"
                >
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
            <span>该项目位于 <code>{{ project.workingDir }}</code>，不在受控目录 <code>/poke/projects</code> 下，无法进行管理。</span>
        </div>

        <div class="card full-width">
            <div class="info-row">
                <span class="label">工作目录</span>
                <span class="value monospace">{{ project.workingDir }}</span>
            </div>
            <div class="info-row">
                <span class="label">运行状态</span>
                <span class="value">
                    {{ runningCount }} / {{ totalCount }} 容器正在运行
                </span>
            </div>
        </div>

        <div v-if="isEditing" class="card full-width editor-card">
            <div class="editor-header">
                <h3>编辑 docker-compose.yml</h3>
            </div>
            <textarea
                class="textarea"
                v-model="editedContent"
                spellcheck="false"
            ></textarea>
            <div class="editor-actions">
                <button class="btn btn-cancel" @click="isEditing = false">
                    取消
                </button>
                <button class="btn btn-save" @click="handleSaveCompose" :disabled="actionLoading">
                    <Save :size="14" style="margin-right: 6px" />
                    保存并重新部署
                    <span v-if="actionLoading" class="loader-inline"></span>
                </button>
            </div>
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
import {
    Play, Square, RotateCw, Box, Layers, HardDrive,
    FileEdit, Save, X, Trash
} from 'lucide-vue-next';
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

<style scoped>
.project-detail {
    height: 100%;
    overflow: auto;
    padding-bottom: 40px;
}

.loading, .error {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 200px;
  color: var(--system-gray);
}

.header-area {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 24px;
}

.title-group {
    display: flex;
    align-items: center;
    gap: 12px;
}

.back-btn {
    background: transparent;
    border: none;
    color: var(--foreground);
    cursor: pointer;
    opacity: 0.7;
    transition: opacity 0.2s;
    display: flex;
    align-items: center;
}

.back-btn:hover {
    opacity: 1;
}

h2 {
    font-size: 20px;
    font-weight: 600;
}

.uncontrolled-badge {
    font-size: 10px;
    background: var(--item-hover);
    color: #ff9f0a;
    padding: 2px 6px;
    border-radius: 4px;
}


.header-controls {
    display: flex;
    gap: 8px;
    align-items: center;
}

.grid {
    display: flex;
    flex-direction: column;
    gap: 16px;
}

.notice-card {
    background: var(--item-hover);
    border: 1px solid var(--card-border);
    padding: 12px 16px;
    border-radius: 8px;
    color: #ff9f0a;
    font-size: 13px;
    display: flex;
    align-items: center;
    gap: 10px;
}

.card {
    background: var(--card-background);
    border: 1px solid var(--card-border);
    border-radius: 12px;
    padding: 20px;
}

.info-row {
    display: flex;
    justify-content: space-between;
    margin-bottom: 8px;
    font-size: 14px;
}

.info-row:last-child {
    margin-bottom: 0;
}

.label { color: var(--system-gray); }
.value { font-weight: 500; }


.editor-card {
    display: flex;
    flex-direction: column;
    gap: 12px;
}

.textarea {
    width: 100%;
    min-height: 300px;
    background: var(--input-background);
    color: var(--foreground);
    border: 1px solid var(--glass-border);
    border-radius: 8px;
    padding: 12px;

    font-size: 13px;
    resize: none;
    outline: none;
}

.textarea:focus { border-color: var(--system-blue); }

.editor-actions {
    display: flex;
    justify-content: flex-end;
    gap: 12px;
}

.container-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-top: 16px;
}

.container-item {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 12px 16px;
    background: var(--window-background); /* Slightly different bg */
    border-radius: 8px;
    border: 1px solid var(--glass-border);
}

.icon-box { color: var(--system-blue); opacity: 0.8; }

.container-info { flex: 1; }

.service-name { font-weight: 600; font-size: 14px; }
.container-name {
    font-size: 11px;
    opacity: 0.4;

}

.status-col {
    text-align: right;
}

.status-badge {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
}

.dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--system-gray);
}

.status-badge.running { color: var(--system-green); }
.status-badge.running .dot { background: var(--system-green); }
.status-badge.stopped { color: var(--system-gray); }

.status-text {
    font-size: 10px;
    opacity: 0.3;
}

.extra-icon { opacity: 0.5; }
.image-name {
    font-size: 11px;
    opacity: 0.4;
    min-width: 150px;
    text-align: right;

}

/* API Button Styles */
.btn {
    border: 1px solid var(--glass-border);
    border-radius: 6px;
    padding: 0 12px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    background: var(--window-background); /* Default bg */
    color: var(--foreground);
    font-size: 13px;
    font-weight: 500;
    transition: all 0.2s;
}

.btn:hover:not(:disabled) {
    background: var(--item-hover);
    transform: translateY(-1px);
}

.btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
}

.btn-start { background: rgba(52, 199, 89, 0.1); color: var(--system-green); border-color: transparent; }
.btn-start:hover:not(:disabled) { background: rgba(52, 199, 89, 0.2); }

.btn-stop { background: rgba(255, 149, 0, 0.1); color: var(--system-orange); border-color: transparent; }
.btn-stop:hover:not(:disabled) { background: rgba(255, 149, 0, 0.2); }

.btn-restart { background: rgba(0, 122, 255, 0.1); color: var(--system-blue); border-color: transparent; }
.btn-restart:hover:not(:disabled) { background: rgba(0, 122, 255, 0.2); }

.btn-delete { background: rgba(255, 59, 48, 0.1); color: var(--system-red); border-color: transparent; padding: 0 8px; }
.btn-delete:hover:not(:disabled) { background: rgba(255, 59, 48, 0.2); }

.btn-edit { background: var(--item-hover); }
.btn-save { background: var(--system-blue); color: white; border: none; }
.btn-save:hover:not(:disabled) { background: var(--system-blue); opacity: 0.9; }

.loader-inline {
    width: 10px;
    height: 10px;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    display: inline-block;
    margin-left: 6px;
}
</style>
