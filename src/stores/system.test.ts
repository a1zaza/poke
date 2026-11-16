import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { requestMock } = vi.hoisted(() => ({ requestMock: vi.fn() }));

vi.mock('@/utils/request', () => ({ request: requestMock }));

import { useSystemStore } from './system';

describe('系统节点信息', () => {
    const storage = new Map<string, string>();

    beforeEach(() => {
        storage.clear();
        vi.stubGlobal('localStorage', {
            getItem: (key: string) => storage.get(key) ?? null,
            setItem: (key: string, value: string) => storage.set(key, value)
        });
        setActivePinia(createPinia());
        requestMock.mockReset();
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
    });

    afterEach(() => {
        vi.restoreAllMocks();
        vi.unstubAllGlobals();
    });

    it('创建 Store 时不会在登录前自动请求节点列表', () => {
        const store = useSystemStore();

        expect(requestMock).not.toHaveBeenCalled();
        expect(store.loading).toBe(false);
        expect(store.info.hostname).toBe('Poke');
    });

    it('登录后显式加载本地节点列表', async () => {
        requestMock.mockImplementation((path: string) => Promise.resolve(path === '/api/nodes' ? [] : {
            hostname: '主节点', logo: '/logo.png', mode: 'controller'
        }));
        const store = useSystemStore();

        const loading = store.fetchSystemInfo();
        expect(store.loading).toBe(true);
        await loading;

        expect(requestMock).toHaveBeenCalledWith('/api/settings/system', { localOnly: true });
        expect(store.loading).toBe(false);
        expect(store.error).toBe('');
        expect(store.info.hostname).toBe('主节点');
        expect(store.nodes).toHaveLength(1);
    });

    it('加载失败后结束 loading 并允许重试', async () => {
        requestMock.mockRejectedValueOnce(new Error('Unauthorized'));
        const store = useSystemStore();

        await store.fetchSystemInfo();
        expect(store.loading).toBe(false);
        expect(store.error).toBe('节点列表加载失败');

        requestMock.mockImplementation((path: string) => Promise.resolve(path === '/api/nodes' ? [] : {hostname: '重试成功'}));
        await store.fetchSystemInfo();
        expect(requestMock).toHaveBeenCalledTimes(4);
        expect(store.error).toBe('');
        expect(store.info.hostname).toBe('重试成功');
    });
    it('Controller 无需本地 Docker，空节点列表保留注册入口', async () => {
        requestMock.mockImplementation((path: string) => Promise.resolve(path === '/api/nodes' ? [] : {mode: 'controller', local_enabled: false}));
        const store = useSystemStore();
        await store.fetchSystemInfo();
        expect(store.info.isController).toBe(true);
        expect(store.info.localEnabled).toBe(false);
        expect(store.nodes).toEqual([]);
        expect(store.currentNodeId).toBe('');
    });

});
