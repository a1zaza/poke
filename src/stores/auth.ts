import { defineStore } from 'pinia';
import { ref } from 'vue';
import { request } from '@/utils/request';

export const useAuthStore = defineStore('auth', () => {
    const user = ref(null);
    const isAuthenticated = ref(false);

    async function login(password: string) {
        try {
            await request('/api/auth/login', {
                method: 'POST',
                body: JSON.stringify({ password }),
                localOnly: true
            });

            isAuthenticated.value = true;
            return true;
        } catch (e) {
            console.error(e);
            return false;
        }
    }

    async function checkAuth() {
        try {
            // Session validation is independent of local Docker availability.
            await request('/api/auth/session', { localOnly: true });
            isAuthenticated.value = true;
            return true;
        } catch (e) {
            isAuthenticated.value = false;
            return false;
        }
    }

    async function logout() {
        try { await request('/api/auth/logout', {method: 'POST', localOnly: true}); }
        finally { user.value = null; isAuthenticated.value = false; }
    }

    return { user, isAuthenticated, login, checkAuth, logout };
});
