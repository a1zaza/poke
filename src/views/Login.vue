<template>
  <div class="login-page">
    <div class="login-card">
      <div class="brand">
        <div class="logo">
           <img src="/logo.png" alt="Poke" width="64" height="64" />
        </div>
        <h1>Poke</h1>
      </div>

      <p class="subtitle">请输入管理员密码以继续</p>

      <form @submit.prevent="handleLogin" class="login-form">
        <div class="input-group">
           <input
             v-model="password"
             type="password"
             placeholder="密码"
             :disabled="loading"
             ref="passwordInput"
             class="input"
             autocomplete="off"
           />
        </div>

        <button type="submit" class="btn-submit" :disabled="loading || !password">
           <span v-if="!loading">登录</span>
           <span v-else class="loader"></span>
        </button>

        <p v-if="error" class="error-msg">{{ error }}</p>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '@/stores/auth';

const password = ref('');
const loading = ref(false);
const error = ref('');
const passwordInput = ref<HTMLInputElement | null>(null);

const router = useRouter();
const authStore = useAuthStore();

const handleLogin = async () => {
    if (!password.value) return;
    loading.value = true;
    error.value = '';

    try {
        const success = await authStore.login(password.value);
        if (success) {
            router.push('/');
        } else {
            error.value = '密码错误，请重试';
        }
    } catch (e) {
        error.value = '登录失败，请检查网络';
    } finally {
        loading.value = false;
    }
};

onMounted(() => {
    passwordInput.value?.focus();
});
</script>

<style scoped>
.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100vh;
  width: 100vw;
  background: var(--background);
  color: var(--foreground);
}

.login-card {
  background: var(--window-background);
  padding: 40px;
  border-radius: 24px;
  border: 1px solid var(--glass-border);
  width: 100%;
  max-width: 400px;
  box-shadow: 0 20px 40px rgba(0,0,0,0.2);
  display: flex;
  flex-direction: column;
  align-items: center;
  backdrop-filter: blur(20px);
}

.brand {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    margin-bottom: 8px;
}

.logo {
    /* Image handles its own styling */
    display: flex;
    align-items: center;
    justify-content: center;
}

h1 {
    font-size: 24px;
    font-weight: 700;
    letter-spacing: -0.5px;
}

.subtitle {
    font-size: 14px;
    color: var(--system-gray);
    margin-bottom: 32px;
}

.login-form {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 16px;
}

.input {
    width: 100%;
    height: 48px;
    border-radius: 12px;
    border: 1px solid var(--glass-border);
    background: var(--input-background);
    color: var(--foreground);
    padding: 0 16px;
    font-size: 16px;
    outline: none;
    transition: all 0.2s;
}

.input:focus {
    border-color: var(--system-blue);
    box-shadow: 0 0 0 4px rgba(0, 122, 255, 0.1);
}

.btn-submit {
    height: 48px;
    border-radius: 12px;
    background: var(--foreground);
    color: var(--background);
    border: none;
    font-size: 16px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    display: flex;
    align-items: center;
    justify-content: center;
}

.btn-submit:hover:not(:disabled) {
    transform: translateY(-2px);
    box-shadow: 0 4px 12px rgba(0,0,0,0.1);
}

.btn-submit:disabled {
    opacity: 0.7;
    cursor: not-allowed;
}

.error-msg {
    color: var(--system-red);
    font-size: 13px;
    text-align: center;
    margin-top: 8px;
}

.loader {
    width: 20px;
    height: 20px;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
}

@keyframes spin {
    to { transform: rotate(360deg); }
}
</style>
