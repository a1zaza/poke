<template>
  <div class="node-manager">
    <Teleport v-if="mounted" to="#header-action">
      <button class="secondary" :disabled="refreshing" @click="refresh">
        <RefreshCw :size="14" />刷新节点与任务
      </button>
    </Teleport>
    <div class="settings-stats" aria-label="节点与任务概况">
      <div class="settings-stat">
        <span>管理节点</span><strong>{{ nodes.filter((node) => node.status !== 'revoked').length }}</strong>
      </div>
      <div class="settings-stat">
        <span>在线节点</span
        ><strong class="online-count">{{ nodes.filter((node) => node.status === 'online').length }}</strong>
      </div>
      <div class="settings-stat">
        <span>进行中任务</span
        ><strong>{{
          tasks.filter((task) =>
            ['queued', 'prepared', 'execution_granted', 'accepted', 'running'].includes(task.state)
          ).length
        }}</strong>
      </div>
    </div>
    <p v-if="initialLoading" class="settings-hint" role="status">正在加载节点与任务…</p>
    <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
    <section class="enrollment" aria-label="添加新节点">
      <div class="enrollment-intro">
        <Server :size="20" />
        <h3>接入新节点</h3>
        <p>Agent 主动连接 Controller，无需开放入站端口。生成长期 Token，按提示完成接入。</p>
      </div>
      <div class="enrollment-fields">
        <div class="enroll-form">
          <label>节点名称<input v-model="name" placeholder="例如：生产环境 · 上海" maxlength="100" /></label
          ><button class="primary" :disabled="busy || !name.trim()" @click="create">
            <Plus :size="15" />生成节点 Token
          </button>
        </div>
        <div v-if="registries.length" class="registry-policy">
          <span>授权镜像仓库</span
          ><label v-for="domain in registries" :key="domain"
            ><input v-model="allowed" type="checkbox" :value="domain" />{{ domain }}</label
          >
        </div>
      </div>
    </section>
    <section v-if="invitation" class="invitation" aria-label="长期节点 Token">
      <header>
        <strong>节点 Token 仅本次显示，请长期保留</strong
        ><button class="icon-button" aria-label="关闭并隐藏凭据" @click="invitation = null">
          <X :size="16" />
        </button>
      </header>
      <p>在 Agent 主机配置以下环境变量，节点 Token 填入 <code>POKE_NODE_TOKEN</code>。</p>
      <textarea :value="invitation.token" readonly aria-label="长期节点 Token" spellcheck="false" />
      <pre tabindex="0" aria-label="Agent 环境变量">{{ instructions }}</pre>
      <p>
        仅需挂载 Docker socket，业务目录使用宿主机绝对路径。使用当前 Poke 镜像，生产连接使用 HTTPS，私有 CA 可通过
        <code>POKE_CA_FILE</code> 指定。POKE_NODE_TOKEN 需长期保留；相同 ID 与 Token 仅允许一个活动 Agent
        实例。
      </p>
    </section>
    <section class="settings-panel node-list-panel" aria-labelledby="node-list-title">
      <header class="settings-panel-title">
        <h2 id="node-list-title">Agent 节点</h2>
        <span class="settings-count">每 5 秒自动刷新</span>
      </header>
      <div v-if="!initialLoading && !error && !nodes.length" class="node-empty">
        <Server :size="26" />
        <p>还没有 Agent 节点</p>
        <small>为新节点生成节点 Token，然后按提示完成接入。</small>
      </div>
      <div class="node-grid">
        <article v-for="node in nodes" :key="node.id" class="node-card">
          <header>
            <Server :size="16" /><strong :title="node.name">{{ node.name }}</strong
            ><span class="node-status" :class="node.status"><i />{{ statusName(node.status) }}</span>
          </header>
          <dl>
            <div>
              <dt>节点 ID</dt>
              <dd>{{ node.id }}</dd>
            </div>
            <div>
              <dt>仓库授权</dt>
              <dd>{{ node.registries?.join('、') || '未授权私有仓库' }}</dd>
            </div>
            <div v-if="node.boot_id">
              <dt>当前实例</dt>
              <dd>{{ node.boot_id }}</dd>
            </div>
            <div v-if="node.duplicate_attempts">
              <dt>重复连接</dt>
              <dd>{{ node.duplicate_attempts }} 次，已拒绝重复实例</dd>
            </div>
          </dl>
          <p v-if="node.blocked" class="node-warning">存在结果待确认的任务，已暂停此节点的修改操作。</p>
          <details v-if="node.status !== 'revoked'" class="policy-editor">
            <summary>编辑仓库授权</summary>
            <p v-if="!registries.length">请先在“镜像仓库”中添加并保存仓库认证。</p>
            <label v-for="domain in registries" :key="domain"
              ><input
                type="checkbox"
                :checked="(edits[node.id] || node.registries || []).includes(domain)"
                @change="toggle(node, domain)"
              />{{ domain }}</label
            ><button class="secondary" :disabled="busy" @click="savePolicy(node)">保存授权</button>
          </details>
          <footer>
            <button class="secondary" :disabled="busy" @click="renew(node.id)">
              <RefreshCw :size="13" />轮换 Token</button
            ><button
              v-if="node.status !== 'revoked'"
              class="revoke-button"
              :disabled="busy"
              @click="revoke(node.id)"
            >
              <ShieldX :size="13" />撤销节点
            </button>
          </footer>
        </article>
      </div>
      <p class="section-note">轮换 Token 会使旧身份失效；撤销会断开管理连接，已运行的容器继续运行。</p>
    </section>
    <section class="tasks-section settings-panel" aria-labelledby="tasks-title">
      <header class="section-heading">
        <h2 id="tasks-title"><ListTodo :size="16" />最近任务</h2>
        <p>结果待确认的任务，请先检查节点实际状态，避免重复执行。</p>
      </header>
      <div v-if="!initialLoading && !error && !tasks.length" class="task-empty">暂无任务记录。</div>
      <article v-for="task in tasks" :key="task.id" class="task-row">
        <div>
          <div class="task-title">
            <strong>{{ nodes.find((node) => node.id === task.node_id)?.name || task.node_id }}</strong><span
              class="task-state"
              :class="task.state"
              >{{ taskName(task.state) }}</span
            >
          </div>
          <p class="task-path">{{ task.method }} {{ task.path }}</p>
          <small>{{ task.id }}</small>
          <p v-if="task.error" class="task-error">{{ task.error }}</p>
        </div>
        <button
          v-if="['queued', 'prepared', 'execution_granted', 'running', 'accepted'].includes(task.state)"
          class="secondary"
          @click="cancelTask(task.id)"
        >
          请求取消
        </button>
        <button
          v-if="task.state === 'unknown' && !task.resolved"
          class="secondary"
          :disabled="busy"
          @click="pendingResolution = task.id"
        >
          确认已核实
        </button>
        <span v-else-if="task.state === 'unknown' && task.resolved" class="settings-hint">已人工核实</span>
      </article>
    </section>
    <ConfirmModal
      :isOpen="!!pendingResolution"
      title="确认已核实执行结果？"
      message="请先停止原执行实例，确认其不再修改 Docker，并核实资源实际状态。本操作仅解除该任务的修改阻塞，不会重试任务或将其标记为成功。"
      @confirm="resolveTask"
      @close="pendingResolution = null"
    />
    <ConfirmModal
      :isOpen="!!pendingAction"
      :title="pendingAction?.kind === 'revoke' ? '撤销此节点？' : '轮换此节点的 Token？'"
      message="现有管理连接将断开，后续连接需要新的节点 Token。运行中的容器继续运行。"
      @confirm="confirmAction"
      @close="pendingAction = null"
    />
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { request } from '@/utils/request';
import { useSystemStore, type ManagedNode } from '@/stores/system';
import { Plus, RefreshCw, ShieldX, Server, ListTodo, X } from 'lucide-vue-next';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';

defineProps<{ registries: string[] }>();
const store = useSystemStore();
const nodes = ref<ManagedNode[]>([]);
const tasks = ref<
  Array<{
    id: string;
    method: string;
    path: string;
    state: string;
    node_id: string;
    error?: string;
    resolved?: boolean;
  }>
>([]);
const name = ref('');
const allowed = ref<string[]>([]);
const edits = ref<Record<string, string[]>>({});
const error = ref('');
const busy = ref(false);
const invitation = ref<{ id: string; token: string } | null>(null);
const pendingResolution = ref<string | null>(null);
const pendingAction = ref<{ kind: 'revoke' | 'renew'; id: string } | null>(null);
let timer: ReturnType<typeof setInterval> | undefined;
const refreshing = ref(false);
const initialLoading = ref(true);
const mounted = ref(false);
const instructions = computed(
  () =>
    `POKE_MODE=agent\nPOKE_CONTROLLER_URL=${window.location.origin}\nPOKE_NODE_ID=${invitation.value?.id || ''}\nPOKE_NODE_TOKEN=${invitation.value?.token || ''}`
);
const statusName = (s: string) =>
  ({ online: '在线', offline: '离线', pending: '等待连接', revoked: '已撤销' })[s] || s;
const taskName = (s: string) =>
  ({
    queued: '等待接收',
    accepted: '已接收',
    prepared: '准备执行',
    execution_granted: '已授权执行',
    running: '执行中',
    succeeded: '成功',
    failed: '失败',
    unknown: '结果待确认',
    cancelled: '已取消'
  })[s] || s;
async function refresh() {
  if (refreshing.value) return;
  refreshing.value = true;
  try {
    [nodes.value, tasks.value] = await Promise.all([
      request('/api/nodes', { localOnly: true }),
      request('/api/tasks', { localOnly: true })
    ]);
    error.value = '';
    await store.fetchSystemInfo();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    refreshing.value = false;
    initialLoading.value = false;
  }
}
async function create() {
  busy.value = true;
  try {
    const data = await request('/api/nodes', {
      method: 'POST',
      localOnly: true,
      body: JSON.stringify({ name: name.value, registries: allowed.value })
    });
    invitation.value = { id: data.node.id, token: data.node_token };
    name.value = '';
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
function toggle(node: ManagedNode, domain: string) {
  const next = [...(edits.value[node.id] || node.registries || [])];
  const index = next.indexOf(domain);
  if (index >= 0) next.splice(index, 1);
  else next.push(domain);
  edits.value[node.id] = next;
}
async function savePolicy(node: ManagedNode) {
  busy.value = true;
  try {
    await request(`/api/nodes/${node.id}`, {
      method: 'PATCH',
      localOnly: true,
      body: JSON.stringify({ name: node.name, registries: edits.value[node.id] || node.registries || [] })
    });
    delete edits.value[node.id];
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
function revoke(id: string) {
  pendingAction.value = { kind: 'revoke', id };
}
function renew(id: string) {
  pendingAction.value = { kind: 'renew', id };
}
async function confirmAction() {
  const action = pendingAction.value;
  pendingAction.value = null;
  if (!action) return;
  busy.value = true;
  try {
    if (action.kind === 'revoke')
      await request(`/api/nodes/${action.id}`, { method: 'DELETE', localOnly: true });
    else {
      const data = await request(`/api/nodes/${action.id}/token`, { method: 'POST', localOnly: true });
      invitation.value = { id: action.id, token: data.node_token };
    }
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function cancelTask(id: string) {
  try {
    await request(`/api/tasks/${id}/cancel`, { method: 'POST', localOnly: true });
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function resolveTask() {
  const id = pendingResolution.value;
  pendingResolution.value = null;
  if (!id) return;
  busy.value = true;
  try {
    await request(`/api/tasks/${id}/resolve`, {
      method: 'POST',
      localOnly: true,
      body: JSON.stringify({ confirmed: true })
    });
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
onMounted(() => {
  mounted.value = true;
  void refresh();
  timer = setInterval(() => void refresh(), 5000);
});
onUnmounted(() => {
  if (timer) clearInterval(timer);
  invitation.value = null;
});
</script>
<style scoped>
.node-manager {
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.section-heading h2 {
  font-size: 14px;
  font-weight: 500;
  display: flex;
  gap: 8px;
  align-items: center;
}
.section-heading p,
.section-note {
  font-size: 12px;
  color: var(--system-gray);
  line-height: 1.8;
  margin-top: 6px;
}
.enrollment {
  padding: 24px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  display: grid;
  grid-template-columns: minmax(190px, 1fr) minmax(0, 2fr);
  gap: 40px;
}
h3 {
  font-size: 13px;
  font-weight: 500;
  margin-bottom: 16px;
}
.enroll-form {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 12px;
}
.enroll-form > label {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  max-width: none;
  min-width: min(100%, 200px);
  color: var(--system-gray);
  font-size: 12px;
}
.registry-policy {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
  font-size: 12px;
}
.registry-policy > span {
  color: var(--system-gray);
}
.registry-policy label,
.policy-editor label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  overflow-wrap: anywhere;
}
.node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr));
  gap: 16px;
  padding: 20px;
}
.node-card {
  max-width: none;
  min-width: 0;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--window-background);
  display: flex;
  flex-direction: column;
}
.node-card > header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px;
  border-bottom: 1px solid var(--border);
}
.node-card > header > svg {
  color: var(--system-gray);
  flex: none;
}
.node-card strong {
  font-size: 13px;
  font-weight: 500;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.node-status {
  margin-left: auto;
  flex: none;
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 11px;
  color: var(--system-gray);
}
.node-status.online {
  color: var(--system-green);
}
.node-status.pending {
  color: var(--system-orange);
}
.node-status.revoked {
  color: var(--system-red);
}
.node-status i {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}
dl {
  margin: 0;
  padding: 8px 16px;
  font-size: 12px;
}
dl > div {
  padding: 7px 0;
  display: flex;
  gap: 14px;
  align-items: baseline;
}
dt {
  color: var(--system-gray);
  flex: none;
}
dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.node-card > footer {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding: 14px 16px;
  border-top: 1px solid var(--border);
  margin-top: auto;
}
.node-card button {
  font-size: 12px;
}
.revoke-button {
  color: var(--system-red);
}
.policy-editor {
  padding: 0 16px 16px;
  font-size: 12px;
}
.policy-editor summary {
  cursor: pointer;
  color: var(--blue);
  padding: 8px 0;
}
.policy-editor label {
  margin: 4px 12px 8px 0;
}
.policy-editor button {
  display: flex;
  margin-top: 8px;
}
.policy-editor p {
  color: var(--system-gray);
  line-height: 1.8;
}
.invitation {
  border: 1px solid var(--blue);
  border-radius: 4px;
  padding: 16px;
  background: var(--accent-soft);
}
.invitation header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.invitation strong {
  font-size: 13px;
  font-weight: 500;
}
.invitation p {
  font-size: 12px;
  line-height: 1.8;
  color: var(--system-gray);
}
.invitation textarea {
  margin: 12px 0;
  min-height: 64px;
}
.invitation pre {
  background: var(--surface);
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 4px;
  overflow-x: auto;
  font-size: 12px;
  margin: 0 0 12px;
}
.node-empty {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: center;
  border: 0;
  border-radius: 4px;
  padding: 28px;
  color: var(--system-gray);
  text-align: center;
  margin: 20px;
}
.tasks-section {
  border-top: 1px solid var(--border);
  padding-top: 24px;
  padding: 0 24px 8px;
}
.tasks-section .section-heading {
  margin-bottom: 12px;
  padding: 20px 0 16px;
  margin: 0;
  border-bottom: 1px solid var(--border);
}
.task-row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px 0;
  border-bottom: 1px solid var(--border);
  font-size: 12px;
}
.task-row > div {
  flex: 1;
  min-width: 0;
}
.task-title {
  display: flex;
  justify-content: flex-start;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.task-title strong {
  font-weight: 500;
  overflow-wrap: anywhere;
}
.task-title span,
.task-row small,
.task-empty {
  color: var(--system-gray);
}
.task-path {
  overflow-wrap: anywhere;
  margin: 6px 0;
}
.task-row small {
  display: block;
  overflow-wrap: anywhere;
  font-size: 11px;
}
.task-error {
  color: var(--system-red);
  margin-top: 6px;
  overflow-wrap: anywhere;
}
.task-empty {
  padding: 24px 0;
  font-size: 12px;
}
@media (max-width: 540px) {
  .enroll-form > label,
  .enroll-form > button {
    max-width: none;
    width: 100%;
  }
  .task-row {
    flex-wrap: wrap;
  }
  .task-row > div {
    flex-basis: 100%;
  }
}
.enrollment-intro > svg {
  color: var(--blue);
  margin-bottom: 12px;
}
.enrollment-intro h3 {
  margin: 0 0 8px;
  font-size: 14px;
}
.enrollment-intro p {
  color: var(--system-gray);
  font-size: 12px;
  line-height: 1.8;
  max-width: 280px;
}
.enrollment-fields {
  min-width: 0;
}
.node-grid:empty {
  display: none;
}
.section-note {
  margin: 0;
  padding: 0 20px 20px;
}
.task-row:last-child {
  border-bottom: 0;
}
.task-state {
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--field);
  font-size: 11px;
}
.task-state.succeeded {
  color: var(--system-green);
}
.task-state.failed,
.task-state.unknown {
  color: var(--system-red);
}
.task-state.running,
.task-state.accepted,
.task-state.queued {
  color: var(--system-orange);
}
.online-count {
  color: var(--system-green);
}
@media (max-width: 1000px) {
  .enrollment {
    grid-template-columns: 1fr;
    gap: 20px;
  }
  .enrollment-intro p {
    max-width: none;
  }
}
@media (max-width: 720px) {
  .enrollment,
  .node-grid {
    padding: 18px;
  }
  .tasks-section {
    padding: 0 18px 8px;
  }
}
.node-warning {
  color: var(--system-orange);
  padding: 0 16px 16px;
  font-size: 12px;
  line-height: 1.8;
}
.task-state.execution_granted {
  color: var(--system-orange);
}
</style>
