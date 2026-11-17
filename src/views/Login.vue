<template>
  <main class="login-layout">
    <section class="login-story" aria-label="Poke 简介">
      <div class="login-brand">
        <img :src="'/logo.png'" alt="Poke 标志" width="38" height="38" /><strong>Poke</strong
        ><span>容器管理控制台</span>
      </div>
      <div class="story-copy">
        <span class="eyebrow">从一台主机，到每一个节点。</span>
        <h1>让容器有序运行，<br />让管理简单清晰<span>。</span></h1>
        <p>从 Compose 部署到多节点协作，<br />在一个控制台，掌握你的 Docker 环境。</p>
        <div class="login-features">
          <span><Box :size="16" />容器与终端</span><span><Layers :size="16" />应用编排</span
          ><span><Network :size="16" />多节点连接</span>
        </div>
      </div>
      <small>Poke / 独立部署 · 自主管理</small>
    </section>
    <section class="login-panel" aria-labelledby="login-title">
      <form @submit.prevent="handleLogin">
        <span class="eyebrow">控制台登录</span>
        <h2 id="login-title">欢迎回来</h2>
        <p class="login-intro">使用管理员账户，继续管理你的容器环境。</p>
        <label for="admin-username">用户名</label>
        <input
          id="admin-username"
          ref="usernameInput"
          v-model="username"
          type="text"
          placeholder="默认用户名 admin"
          maxlength="64"
          :disabled="loading"
          autocomplete="username"
          required
          :aria-invalid="!!error"
          :aria-describedby="error ? 'login-error' : undefined"
          class="username-input"
        />
        <label for="admin-password">管理员密码</label>
        <input
          id="admin-password"
          v-model="password"
          type="password"
          placeholder="请输入密码"
          :disabled="loading"
          autocomplete="current-password"
          required
          :aria-invalid="!!error"
          :aria-describedby="error ? 'login-error' : undefined"
        />
        <p v-if="error" id="login-error" class="inline-error login-error" role="alert">{{ error }}</p>
        <button type="submit" class="primary login-submit" :disabled="loading || !username || !password">
          <LoaderCircle v-if="loading" :size="16" class="spin" /><span>{{
            loading ? '正在登录…' : '进入控制台'
          }}</span
          ><ArrowRight v-if="!loading" :size="16" />
        </button>
        <div class="login-note">
          <KeyRound :size="15" /><span
            >默认用户名为 admin。首次初始化时会打印用户名和随机密码，请从服务日志获取。</span
          >
        </div>
      </form>
    </section>
  </main>
</template>
<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { Box, Layers, KeyRound, Network, ArrowRight, LoaderCircle } from 'lucide-vue-next';
import { useAuthStore } from '@/stores/auth';

const username = ref('');
const password = ref('');
const loading = ref(false);
const error = ref('');
const usernameInput = ref<HTMLInputElement | null>(null);

const router = useRouter();
const authStore = useAuthStore();

const handleLogin = async () => {
  if (!username.value || !password.value) return;
  loading.value = true;
  error.value = '';

  try {
    const success = await authStore.login(username.value, password.value);
    if (success) {
      router.push('/');
    } else {
      error.value = '用户名或密码错误，请重试';
    }
  } catch (e) {
    error.value = '登录失败，请检查网络';
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  usernameInput.value?.focus();
});
</script>
<style scoped>
.username-input {
  margin-bottom: 20px;
}
.login-layout {
  display: grid;
  grid-template-columns: 1fr 1fr;
  min-height: 100dvh;
}
.login-story {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 60px;
  padding: 48px clamp(28px, 4vw, 64px);
  border-right: 1px solid var(--border);
  background: radial-gradient(ellipse at 0% 60%, var(--accent-soft), transparent 75%), var(--surface);
}
.login-brand {
  display: flex;
  align-items: center;
  gap: 10px;
}
.login-brand strong {
  font-size: 24px;
  letter-spacing: -0.6px;
}
.login-brand > span {
  margin-left: 8px;
  padding-left: 16px;
  border-left: 1px solid var(--border);
  color: var(--system-gray);
  font-size: 12px;
}
h1 {
  font-size: clamp(30px, 3.25vw, 50px);
  line-height: 1.7;
  font-weight: 400;
  margin: 20px 0;
  letter-spacing: -0.5px;
}
h1 > span {
  color: var(--blue);
}
.story-copy p {
  color: var(--system-gray);
  line-height: 2.2;
  font-size: 13px;
}
.login-features {
  display: flex;
  flex-wrap: wrap;
  gap: 22px;
  margin-top: 40px;
  font-size: 12px;
  color: var(--system-gray);
}
.login-features span {
  display: flex;
  align-items: center;
  gap: 8px;
}
.login-features svg {
  color: var(--blue);
}
.login-story > small {
  font-size: 11px;
  color: var(--system-gray);
  letter-spacing: 1px;
}
.login-panel {
  display: grid;
  place-items: center;
  padding: 40px;
  background: var(--window-background);
}
.login-panel form {
  width: 100%;
  max-width: 350px;
}
h2 {
  font-size: 26px;
  font-weight: 500;
  margin: 12px 0 8px;
}
.login-intro {
  color: var(--system-gray);
  font-size: 12px;
  margin-bottom: 32px;
  line-height: 1.8;
}
.login-panel label {
  display: block;
  font-size: 12px;
  margin-bottom: 10px;
}
.login-panel input {
  min-height: 42px;
}
.login-submit {
  margin-top: 20px;
  width: 100%;
  min-height: 42px;
}
.login-submit span {
  flex: 1;
}
.login-error {
  margin-top: 12px;
  font-size: 12px;
}
.login-note {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  color: var(--system-gray);
  font-size: 12px;
  line-height: 1.9;
  margin-top: 28px;
  padding-top: 20px;
  border-top: 1px solid var(--border);
}
.login-note svg {
  flex: none;
  margin-top: 3px;
}
@media (max-width: 900px) {
  .login-layout {
    grid-template-columns: 1fr 1fr;
  }
  .login-story {
    padding: 32px;
  }
  h1 {
    font-size: 30px;
  }
  .login-brand > span {
    display: none;
  }
}
@media (max-width: 720px) {
  .login-layout {
    grid-template-columns: 1fr;
  }
  .login-story {
    padding: 24px;
    gap: 24px;
    border-right: 0;
    border-bottom: 1px solid var(--border);
  }
  .login-story h1 {
    font-size: 26px;
    margin: 10px 0;
  }
  .login-story > small,
  .login-features,
  .story-copy > p {
    display: none;
  }
  .login-panel {
    padding: 32px 24px 48px;
  }
  .login-panel form {
    max-width: 420px;
  }
}
</style>
