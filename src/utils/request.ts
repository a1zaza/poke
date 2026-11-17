import { useSystemStore } from '@/stores/system';

export interface RequestOptions extends RequestInit {
  params?: Record<string, string>;
  localOnly?: boolean;
}

export interface TaskResult {
  id: string;
  state: string;
  error?: string;
  response?: { status: number; body?: string; content_type?: string };
}

function failure(data: any, status: number): Error {
  const error = new Error(data?.error || data?.message || `请求失败（HTTP ${status}）`);
  return Object.assign(error, { code: data?.code, status });
}

async function pause(signal?: AbortSignal | null) {
  if (signal?.aborted) throw new DOMException('请求已取消', 'AbortError');
  await new Promise<void>((resolve, reject) => {
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException('请求已取消', 'AbortError'));
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, 500);
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}

export async function waitForTask<T>(id: string, signal?: AbortSignal | null): Promise<T> {
  while (true) {
    const response = await fetch(`/api/tasks/${encodeURIComponent(id)}`, { signal });
    const task = (await response.json()) as TaskResult;
    if (!response.ok) throw failure(task, response.status);
    if (task.state === 'succeeded' || task.state === 'failed') {
      const bytes = Uint8Array.from(atob(task.response?.body || ''), (c) => c.charCodeAt(0));
      const text = new TextDecoder().decode(bytes);
      const data = text ? JSON.parse(text) : {};
      if (task.state === 'failed') throw failure(data, task.response?.status || 500);
      return data as T;
    }
    if (task.state === 'unknown' || task.state === 'cancelled') {
      throw Object.assign(new Error(task.error || '任务未完成，请在节点管理中确认执行结果'), {
        code: 'TASK_UNCONFIRMED',
        taskId: id
      });
    }
    await pause(signal);
  }
}

export const request = async <T = any>(path: string, options: RequestOptions = {}): Promise<T> => {
  const { localOnly, params, ...init } = options;
  let finalPath = path;
  if (!localOnly) {
    const store = useSystemStore();
    if (!store.ready) await store.fetchSystemInfo();
    if (!store.currentNodeId) throw new Error('请先在系统设置中注册并选择在线节点');
    if (store.currentNodeId && store.currentNodeId !== 'local') {
      finalPath = `/api/nodes/${encodeURIComponent(store.currentNodeId)}${path}`;
    }
  }
  if (params) {
    finalPath += (finalPath.includes('?') ? '&' : '?') + new URLSearchParams(params).toString();
  }
  const response = await fetch(finalPath, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init.headers }
  });
  if (response.status === 204) return {} as T;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw failure(data, response.status);
  if (response.status === 202 && data.task_id) return waitForTask<T>(data.task_id, init.signal);
  return data as T;
};
