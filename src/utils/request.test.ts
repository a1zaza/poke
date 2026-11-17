import { beforeEach, describe, expect, it, vi } from 'vitest';

const { systemStore, useSystemStoreMock } = vi.hoisted(() => ({
  systemStore: { currentNodeId: 'local', ready: true, fetchSystemInfo: vi.fn() },
  useSystemStoreMock: vi.fn()
}));

useSystemStoreMock.mockImplementation(() => systemStore);

vi.mock('@/stores/system', () => ({
  useSystemStore: useSystemStoreMock
}));

import { request } from './request';

describe('request', () => {
  beforeEach(() => {
    systemStore.currentNodeId = 'local';
    useSystemStoreMock.mockClear();
    vi.restoreAllMocks();
  });

  it('为远程节点拼接代理路径和查询参数', async () => {
    systemStore.currentNodeId = 'node-1';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    );

    await expect(request('/api/containers', { params: { state: 'running' } })).resolves.toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/nodes/node-1/api/containers?state=running',
      expect.objectContaining({
        headers: expect.objectContaining({ 'Content-Type': 'application/json' })
      })
    );
  });

  it('localOnly 为真时绕过远程代理', async () => {
    systemStore.currentNodeId = 'node-1';
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));

    await request('/api/system/info', { localOnly: true });
    expect(fetchMock).toHaveBeenCalledWith('/api/system/info', expect.any(Object));
    expect(useSystemStoreMock).not.toHaveBeenCalled();
  });

  it('将后端错误信息和错误码附加到异常', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: '项目路径无效', code: 'INVALID_PATH' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' }
      })
    );

    const error = await request('/api/projects').catch((caught) => caught as Error & { code?: string });
    expect(error).toBeInstanceOf(Error);
    expect(error.message).toBe('项目路径无效');
    expect(error.code).toBe('INVALID_PATH');
    expect(error.status).toBe(400);
  });

  it('保留限流状态码，供检测队列退避处理', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: '请求过多' }), { status: 429 })
    );
    await expect(request('/api/images/check-update', { method: 'POST' })).rejects.toMatchObject({
      status: 429,
      message: '请求过多'
    });
  });
});

describe('Agent 任务请求', () => {
  beforeEach(() => {
    systemStore.currentNodeId = 'node-1';
    systemStore.ready = true;
    vi.restoreAllMocks();
  });
  it('提交远程修改后读取持久化任务结果', async () => {
    const body = btoa(String.fromCharCode(...new TextEncoder().encode(JSON.stringify({ message: '完成' }))));
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ task_id: 'task-1' }), { status: 202 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: 'task-1', state: 'succeeded', response: { status: 200, body } }), {
          status: 200
        })
      );
    await expect(request('/api/containers/one/start', { method: 'POST' })).resolves.toEqual({
      message: '完成'
    });
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/nodes/node-1/api/containers/one/start');
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/tasks/task-1');
  });
  it('结果待确认时保留任务标识，不重放修改', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ task_id: 'uncertain' }), { status: 202 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: 'uncertain', state: 'unknown', error: '请检查节点状态' }))
      );
    await expect(request('/api/projects', { method: 'POST' })).rejects.toMatchObject({
      code: 'TASK_UNCONFIRMED',
      taskId: 'uncertain'
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
  it('节点离线时返回错误，不回退操作主节点', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify({ message: '节点离线' }), { status: 503 }));
    await expect(request('/api/containers')).rejects.toThrow('节点离线');
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
