import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { canCheckImage, useImageUpdates } from './useImageUpdates';

const flush = () => vi.advanceTimersByTimeAsync(0);

describe('镜像更新检测队列', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
  });
  afterEach(() => vi.useRealTimers());

  it('同镜像只检查一次，同时最多两个请求，轮询不会重复加入进行中的请求', async () => {
    const finish = new Map<string, () => void>();
    let active = 0,
      peak = 0;
    const check = vi.fn(async (image: string) => {
      active++;
      peak = Math.max(peak, active);
      await new Promise<void>((resolve) => finish.set(image, resolve));
      active--;
      return { hasUpdate: true };
    });
    const queue = useImageUpdates(check);
    queue.sync(['a', 'a', 'b', 'c', 'b']);
    queue.sync(['a', 'b', 'c']);
    expect(check).toHaveBeenCalledTimes(2);
    finish.get('a')!();
    await flush();
    expect(check).toHaveBeenCalledTimes(3);
    finish.get('b')!();
    finish.get('c')!();
    await flush();
    expect(peak).toBe(2);
    expect(queue.checking.value).toBe(false);
    expect(queue.states.value.a?.status).toBe('available');
    queue.stop();
  });

  it('普通列表刷新复用五分钟结果，显式检查和缓存过期才重新检测', async () => {
    const check = vi.fn().mockResolvedValue({ hasUpdate: false });
    const queue = useImageUpdates(check);
    queue.sync(['a']);
    await flush();
    for (let i = 0; i < 10; i++) {
      await vi.advanceTimersByTimeAsync(3000);
      queue.sync(['a']);
    }
    expect(check).toHaveBeenCalledTimes(1);
    queue.sync(['a'], true);
    await flush();
    expect(check).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(300001);
    queue.sync(['a']);
    await flush();
    expect(check).toHaveBeenCalledTimes(3);
    expect(queue.states.value.a?.status).toBe('latest');
    queue.stop();
  });

  it('429 会暂停后续队列，错误冷却期间不会被三秒轮询反复重试', async () => {
    const check = vi
      .fn()
      .mockRejectedValueOnce(Object.assign(new Error('队列已满'), { status: 429 }))
      .mockResolvedValue({ hasUpdate: false });
    const queue = useImageUpdates(check);
    queue.sync(['a', 'b', 'c']);
    await flush();
    expect(check).toHaveBeenCalledTimes(2);
    expect(queue.states.value.a?.status).toBe('error');
    await vi.advanceTimersByTimeAsync(29999);
    queue.sync(['a', 'b', 'c']);
    expect(check).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(check).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(30001);
    queue.sync(['a', 'b', 'c']);
    await flush();
    expect(check).toHaveBeenCalledTimes(4);
    queue.stop();
  });

  it('离开页面后终止在途请求，迟到结果不再写入，也不继续队列', async () => {
    const finish: Array<() => void> = [],
      signals: AbortSignal[] = [];
    const check = vi.fn(async (_image: string, signal: AbortSignal) => {
      signals.push(signal);
      await new Promise<void>((resolve) => finish.push(resolve));
      return { hasUpdate: false };
    });
    const queue = useImageUpdates(check);
    queue.sync(['a', 'b', 'c']);
    queue.stop();
    expect(signals.every((signal) => signal.aborted)).toBe(true);
    finish.forEach((resolve) => resolve());
    await flush();
    expect(check).toHaveBeenCalledTimes(2);
    expect(queue.states.value.a?.status).toBe('checking');
    queue.sync(['d']);
    expect(check).toHaveBeenCalledTimes(2);
  });

  it('删除或更换镜像时丢弃旧检测结果，更新镜像后可失效缓存', async () => {
    let finishOld!: () => void;
    const check = vi.fn(async (image: string) => {
      if (image === 'old')
        await new Promise<void>((resolve) => {
          finishOld = resolve;
        });
      return { hasUpdate: false };
    });
    const queue = useImageUpdates(check);
    queue.sync(['old']);
    queue.sync(['new']);
    await flush();
    finishOld();
    await flush();
    expect(queue.states.value.old).toBeUndefined();
    expect(queue.states.value.new?.status).toBe('latest');
    queue.invalidate('new');
    queue.sync(['new']);
    await flush();
    expect(check).toHaveBeenCalledTimes(3);
    queue.stop();
  });

  it('固定摘要或镜像 ID 不检测标签更新，业务错误不能标记为最新', async () => {
    expect(canCheckImage('sha256:abc')).toBe(false);
    expect(canCheckImage('registry.example/app@sha256:abc')).toBe(false);
    const check = vi.fn().mockResolvedValue({ error: '仓库认证失败' });
    const queue = useImageUpdates(check);
    queue.sync(['sha256:abc', 'app@sha256:def', 'app:latest']);
    await flush();
    expect(check).toHaveBeenCalledTimes(1);
    expect(queue.states.value['app:latest']?.status).toBe('error');
    queue.stop();
  });
});
