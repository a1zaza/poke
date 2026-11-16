<template>
  <section class="node-manager">
    <h3>Agent 节点</h3>
    <p>Agent 主动连接当前主节点，无需填写 Agent 地址或开放入站端口。</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <div class="enroll-form">
      <input v-model="name" placeholder="新节点名称" maxlength="100" aria-label="节点名称" />
      <label v-for="domain in registries" :key="domain">
        <input v-model="allowed" type="checkbox" :value="domain" /> 授权仓库 {{ domain }}
      </label>
      <button :disabled="busy || !name.trim()" @click="create">生成注册凭据</button>
    </div>
    <div v-if="invitation" class="invitation">
      <strong>注册凭据仅本次显示，10 分钟有效</strong>
      <p>将下方凭据保存到 Agent 主机的 /poke/enrollment-token，权限设为 0600，然后配置以下环境变量。注册成功后可移除注册文件和对应变量。</p>
      <textarea :value="invitation.token" readonly aria-label="一次性注册凭据" spellcheck="false" />
      <pre>{{ instructions }}</pre>
      <p>使用当前 Poke 镜像，将宿主机 /poke 挂载到容器 /poke，并挂载 Docker socket。Agent 模式无需映射端口。生产入口使用 HTTPS；若配置了私有 CA，通过 POKE_CA_FILE 指定受信任证书文件。</p>
      <button @click="invitation = null">关闭并隐藏凭据</button>
    </div>
    <p v-if="!nodes.length">尚未添加节点。</p>
    <div v-for="node in nodes" :key="node.id" class="node-row">
      <div><strong>{{ node.name }}</strong> · {{ statusName(node.status) }}<small>{{ node.id }}</small>
        <small>授权仓库：{{ node.registries?.join('、') || '未授权私有仓库' }}</small>
      </div>
      <details v-if="node.status !== 'revoked'">
        <summary>仓库授权</summary>
        <label v-for="domain in registries" :key="domain"><input type="checkbox" :checked="(edits[node.id] || node.registries || []).includes(domain)" @change="toggle(node, domain)" />{{ domain }}</label>
        <button :disabled="busy" @click="savePolicy(node)">保存授权</button>
      </details>
      <button :disabled="busy" @click="renew(node.id)">重新注册</button>
      <button v-if="node.status !== 'revoked'" :disabled="busy" @click="revoke(node.id)">撤销节点</button>
    </div>
    <p>重新注册会立即使旧身份失效；撤销会断开管理连接，已经运行的容器继续运行。</p>
    <h3>最近任务</h3>
    <p>页面关闭后，已接收的任务仍可能继续执行。结果待确认的任务需要检查节点实际状态。</p>
    <div v-for="task in tasks" :key="task.id" class="task-row">
      <span>{{ nodes.find(n => n.id === task.node_id)?.name || task.node_id }} · {{ task.method }} {{ task.path }} · {{ taskName(task.state) }}<small>{{ task.id }}</small><small v-if="task.error">{{ task.error }}</small></span>
      <button v-if="['queued', 'running', 'accepted'].includes(task.state)" @click="cancelTask(task.id)">请求取消</button>
    </div>
    <p v-if="!tasks.length">暂无任务。</p>
    <ConfirmModal :isOpen="!!pendingAction" :title="pendingAction?.kind === 'revoke' ? '撤销此节点？' : '重新注册此节点？'" message="现有管理连接将断开，重新接入需要新的注册凭据。运行中的容器继续运行。" @confirm="confirmAction" @close="pendingAction = null" />
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { request } from '@/utils/request';
import { useSystemStore, type ManagedNode } from '@/stores/system';
import ConfirmModal from '@/components/ui/ConfirmModal.vue';

defineProps<{ registries: string[] }>();
const store = useSystemStore();
const nodes = ref<ManagedNode[]>([]);
const tasks = ref<Array<{id: string; method: string; path: string; state: string; node_id: string; error?: string}>>([]);
const name = ref('');
const allowed = ref<string[]>([]);
const edits = ref<Record<string,string[]>>({});
const error = ref('');
const busy = ref(false);
const invitation = ref<{id: string; token: string} | null>(null);
const pendingAction = ref<{kind: 'revoke' | 'renew'; id: string} | null>(null);
let timer: ReturnType<typeof setInterval> | undefined;
let refreshing = false;
const instructions = computed(() => `POKE_MODE=agent\nPOKE_CONTROLLER_URL=${window.location.origin}\nPOKE_NODE_ID=${invitation.value?.id || ''}\nPOKE_ENROLLMENT_TOKEN_FILE=/poke/enrollment-token`);
const statusName = (s: string) => ({online: '在线', offline: '离线', pending: '等待注册', revoked: '已撤销'}[s] || s);
const taskName = (s: string) => ({queued: '等待接收', accepted: '已接收', running: '执行中', succeeded: '成功', failed: '失败', unknown: '结果待确认', cancelled: '已取消'}[s] || s);
async function refresh() {
  if (refreshing) return;
  refreshing = true;
  try {
    [nodes.value, tasks.value] = await Promise.all([request('/api/nodes', {localOnly: true}), request('/api/tasks', {localOnly: true})]);
    error.value = '';
    await store.fetchSystemInfo();
  } catch (e) { error.value = (e as Error).message; }
  finally { refreshing = false; }
}
async function create() {
  busy.value = true;
  try {
    const data = await request('/api/nodes', {method: 'POST', localOnly: true, body: JSON.stringify({name: name.value, registries: allowed.value})});
    invitation.value = {id: data.node.id, token: data.enrollment_token}; name.value = ''; await refresh();
  } catch (e) { error.value = (e as Error).message; }
  finally { busy.value = false; }
}
function toggle(node: ManagedNode, domain: string) {
  const next = [...(edits.value[node.id] || node.registries || [])];
  const index = next.indexOf(domain);
  if (index >= 0) next.splice(index, 1); else next.push(domain);
  edits.value[node.id] = next;
}
async function savePolicy(node: ManagedNode) {
  busy.value = true;
  try {
    await request(`/api/nodes/${node.id}`, {method: 'PATCH', localOnly: true, body: JSON.stringify({name: node.name, registries: edits.value[node.id] || node.registries || []})});
    delete edits.value[node.id]; await refresh();
  } catch (e) { error.value = (e as Error).message; }
  finally { busy.value = false; }
}
function revoke(id: string) { pendingAction.value = {kind: 'revoke', id}; }
function renew(id: string) { pendingAction.value = {kind: 'renew', id}; }
async function confirmAction() {
  const action = pendingAction.value; pendingAction.value = null;
  if (!action) return;
  busy.value = true;
  try {
    if (action.kind === 'revoke') await request(`/api/nodes/${action.id}`, {method: 'DELETE', localOnly: true});
    else {
      const data = await request(`/api/nodes/${action.id}/enrollment`, {method: 'POST', localOnly: true});
      invitation.value = {id: action.id, token: data.enrollment_token};
    }
    await refresh();
  } catch (e) { error.value = (e as Error).message; }
  finally { busy.value = false; }
}
async function cancelTask(id: string) {
  try { await request(`/api/tasks/${id}/cancel`, {method: 'POST', localOnly: true}); await refresh(); }
  catch (e) { error.value = (e as Error).message; }
}
onMounted(() => { void refresh(); timer = setInterval(() => void refresh(), 5000); });
onUnmounted(() => { if (timer) clearInterval(timer); invitation.value = null; });
</script>
<style scoped>
.node-manager { margin: 24px 0; padding-top: 16px; border-top: 1px solid var(--glass-border); }
h3 { font-size: 15px; } p, small { font-size: 12px; color: var(--system-gray); line-height: 1.7; }
small { display: block; overflow-wrap: anywhere; }
.enroll-form { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; }
input, textarea, button { padding: 8px; border-radius: 6px; background: var(--input-background); color: var(--foreground); border: 1px solid var(--glass-border); }
button { cursor: pointer; } button:disabled { opacity: .5; cursor: default; }
.node-row, .task-row { display: flex; gap: 12px; align-items: center; padding: 12px 0; border-bottom: 1px solid var(--glass-border); }
.node-row > div, .task-row > span { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.invitation { margin: 12px 0; padding: 16px; border: 1px solid var(--glass-border); border-radius: 8px; }
textarea { width: 100%; box-sizing: border-box; margin-top: 8px; } pre { overflow-x: auto; font-size: 12px; }
</style>
