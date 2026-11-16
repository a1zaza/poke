import { defineStore } from 'pinia';
import { ref } from 'vue';

export type ToastType = 'success' | 'error' | 'info';

interface Toast {
    id: string;
    message: string;
    type: ToastType;
}

export const useToastStore = defineStore('toast', () => {
    const toasts = ref<Toast[]>([]);

    function addToast(message: string, type: ToastType = 'info') {
        const id = Math.random().toString(36).substring(7);
        toasts.value.push({ id, message, type });

        setTimeout(() => {
            toasts.value = toasts.value.filter(t => t.id !== id);
        }, 4000);
    }

    function removeToast(id: string) {
        toasts.value = toasts.value.filter(t => t.id !== id);
    }

    return { toasts, addToast, removeToast };
});
