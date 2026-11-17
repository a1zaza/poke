<template>
  <Teleport to="body">
    <div class="toast-container" aria-live="polite" aria-relevant="additions">
      <div v-for="toast in toasts" :key="toast.id" class="toast" :class="toast.type">
        <CheckCircle v-if="toast.type === 'success'" :size="16" color="var(--system-green)" />
        <AlertCircle v-if="toast.type === 'error'" :size="16" color="var(--system-red)" />
        <Info v-if="toast.type === 'info'" :size="16" color="var(--system-blue)" />
        <span class="toast-message">{{ toast.message }}</span>
        <button class="icon-button" aria-label="关闭通知" @click="removeToast(toast.id)">
          <X :size="14" />
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { useToastStore } from '@/stores/toast';
import { computed } from 'vue';
import { CheckCircle, AlertCircle, Info, X } from 'lucide-vue-next';

const toastStore = useToastStore();
const toasts = computed(() => toastStore.toasts);
const removeToast = toastStore.removeToast;
</script>

<style scoped>
.toast-container {
  position: fixed;
  top: 24px;
  right: 24px;
  z-index: 12000;
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-width: min(420px, calc(100vw - 32px));
}
.toast {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
  color: var(--foreground);
  padding: 10px 12px;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  box-shadow: var(--glass-shadow);
}
.toast > svg {
  flex: none;
}
.toast-message {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.toast.success {
  border-left: 3px solid var(--system-green);
}
.toast.error {
  border-left: 3px solid var(--system-red);
}
.toast.info {
  border-left: 3px solid var(--system-blue);
}
@media (max-width: 720px) {
  .toast-container {
    top: 12px;
    right: 16px;
  }
}
</style>
