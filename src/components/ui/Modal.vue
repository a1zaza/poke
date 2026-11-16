<template>
  <Teleport to="body">
    <div v-if="isOpen" class="overlay" @click.self="onClose">
      <div class="modal">
        <div class="header">{{ title }}</div>
        <div class="body">
          <slot />
        </div>
        <div v-if="$slots.footer" class="footer">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">


const props = defineProps<{
  isOpen: boolean;
  title: string;
  onClose: () => void;
}>();
</script>

<style scoped>
.overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
  -webkit-backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
}

.modal {
  background: var(--window-background);
  backdrop-filter: blur(40px);
  -webkit-backdrop-filter: blur(40px);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  width: 500px;
  color: var(--foreground);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4);
  animation: scaleIn 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  overflow: hidden;
}

@keyframes scaleIn {
  from {
    transform: scale(0.9);
    opacity: 0;
  }
  to {
    transform: scale(1);
    opacity: 1;
  }
}

.header {
  padding: 16px 20px;
  border-bottom: 1px solid var(--glass-border);
  font-weight: 600;
  font-size: 15px;
}

.body {
  padding: 24px 20px;
  font-size: 14px;
  color: var(--foreground);
  line-height: 1.5;
}

.footer {
  padding: 16px 20px;
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  background: var(--item-hover);
}
</style>
