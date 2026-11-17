<template>
  <Modal :isOpen="isOpen" :title="title" :onClose="onClose">
    <p v-if="message" style="margin-bottom: 12px">{{ message }}</p>
    <slot />

    <template #footer>
      <slot name="footer">
        <button class="button cancel" @click="onClose">取消</button>
        <button class="button confirm" :class="{ danger: isDanger }" @click="handleConfirm">确认</button>
      </slot>
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
  min-width: 76px;
}
.confirm {
  background: var(--system-blue);
  border-color: var(--system-blue);
  color: #fff;
}
.confirm:hover {
  background: #1687cc;
}
.confirm.danger {
  background: #ad3c38;
  border-color: #ad3c38;
  color: #fff;
}
.confirm.danger:hover {
  background: #c34843;
}
</style>
