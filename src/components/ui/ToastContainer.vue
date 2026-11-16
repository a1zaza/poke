<template>
  <Teleport to="body">
    <div class="toast-container">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        class="toast"
        :class="toast.type"
      >
        <CheckCircle v-if="toast.type === 'success'" :size="16" color="var(--system-green)" />
        <AlertCircle v-if="toast.type === 'error'" :size="16" color="var(--system-red)" />
        <Info v-if="toast.type === 'info'" :size="16" color="var(--system-blue)" />
        <span style="flex: 1">{{ toast.message }}</span>
        <X
            :size="14"
            style="cursor: pointer; opacity: 0.5"
            @click="removeToast(toast.id)"
        />
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
    z-index: 10000;
    display: flex;
    flex-direction: column;
    gap: 10px;
}

.toast {
    background: var(--window-background);
    backdrop-filter: blur(20px);
    -webkit-backdrop-filter: blur(20px);
    border: 1px solid var(--glass-border);
    color: var(--foreground);
    padding: 12px 16px;
    border-radius: 8px;
    font-size: 13px;
    display: flex;
    align-items: center;
    gap: 10px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    animation: slideIn 0.3s ease forwards;
    min-width: 260px;
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

@keyframes slideIn {
    from {
        opacity: 0;
        transform: translateX(20px);
    }
    to {
        opacity: 1;
        transform: translateX(0);
    }
}
</style>
