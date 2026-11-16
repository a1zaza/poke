import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { requestMock } = vi.hoisted(() => ({ requestMock: vi.fn() }));

vi.mock('@/utils/request', () => ({ request: requestMock }));

import { useAuthStore } from './auth';

describe('认证状态', () => {
    beforeEach(() => {
        setActivePinia(createPinia());
        requestMock.mockReset();
        vi.spyOn(console, 'error').mockImplementation(() => undefined);
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('登录成功后设置认证状态', async () => {
        requestMock.mockResolvedValue({ success: true });
        const store = useAuthStore();

        await expect(store.login('password')).resolves.toBe(true);
        expect(store.isAuthenticated).toBe(true);
        expect(requestMock).toHaveBeenCalledWith('/api/auth/login', expect.objectContaining({ localOnly: true }));
    });

    it('登录失败时保持未认证状态', async () => {
        requestMock.mockRejectedValue(new Error('登录失败'));
        const store = useAuthStore();

        await expect(store.login('wrong')).resolves.toBe(false);
        expect(store.isAuthenticated).toBe(false);
    });

    it('会话检查失败时清除认证状态', async () => {
        const store = useAuthStore();
        requestMock.mockResolvedValueOnce({ success: true });
        await store.login('password');
        requestMock.mockRejectedValueOnce(new Error('会话失效'));

        await expect(store.checkAuth()).resolves.toBe(false);
        expect(store.isAuthenticated).toBe(false);
    });
    it('退出调用服务端并清理客户端登录状态', async () => {
        requestMock.mockResolvedValue({});
        const store = useAuthStore();
        await store.login('password');
        await store.logout();
        expect(store.isAuthenticated).toBe(false);
        expect(requestMock).toHaveBeenCalledWith('/api/auth/logout', {method: 'POST', localOnly: true});
    });

});
