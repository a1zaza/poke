<template>
  <Modal :isOpen="isOpen" :title="title" :onClose="onClose">
    <div style="margin-bottom: 12px">{{ message }}</div>
    <slot />

    <template #footer>
        <button class="button cancel" @click="onClose">
            取消
        </button>
        <button
            class="button confirm"
            :class="{ danger: isDanger }"
            @click="handleConfirm"
        >
            确认
        </button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import Modal from './Modal.vue';

const props = defineProps<{
    isOpen: boolean;
    title: string;
    message?: string;
    isDanger?: boolean;
    onConfirm: () => void;
    onClose: () => void;
}>();

const handleConfirm = () => {
    props.onConfirm();
    props.onClose();
};
</script>

<style scoped>
.button {
    padding: 6px 16px;
    border-radius: 6px;
    border: none;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.2s;
}

.cancel {
    background: var(--item-hover);
    color: var(--foreground);
}

.cancel:hover {
    background: rgba(255, 255, 255, 0.2);
}

.confirm {
    background: var(--system-blue);
    color: white;
}

.confirm:hover {
    filter: brightness(110%);
}

.confirm.danger {
    background: var(--system-red);
}
</style>
