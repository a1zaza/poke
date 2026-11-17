<template>
  <Teleport to="body">
    <div v-if="isOpen" class="dialog-overlay" @click.self="onClose">
      <section
        ref="dialog"
        class="dialog"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="id"
        tabindex="-1"
        @keydown="handleKeydown"
      >
        <header class="dialog-header">
          <h2 :id="id">{{ title }}</h2>
          <button type="button" class="icon-button" aria-label="关闭弹窗" @click="onClose">
            <X :size="18" />
          </button>
        </header>
        <div class="dialog-body"><slot /></div>
        <footer v-if="$slots.footer" class="dialog-footer"><slot name="footer" /></footer>
      </section>
    </div>
  </Teleport>
</template>
<script lang="ts">
const openDialogs: string[] = [];
</script>
<script setup lang="ts">
import { nextTick, onUnmounted, ref, useId, watch } from 'vue';
import { X } from 'lucide-vue-next';
const props = defineProps<{ isOpen: boolean; title: string; onClose: () => void }>();
const id = useId();
const dialog = ref<HTMLElement | null>(null);
let previousFocus: HTMLElement | null = null;
const release = () => {
  const index = openDialogs.indexOf(id);
  if (index >= 0) openDialogs.splice(index, 1);
  if (previousFocus?.isConnected) previousFocus.focus();
  previousFocus = null;
};
watch(
  () => props.isOpen,
  async (open) => {
    if (!open) {
      release();
      return;
    }
    previousFocus = document.activeElement as HTMLElement | null;
    openDialogs.push(id);
    await nextTick();
    if (props.isOpen) dialog.value?.focus();
  },
  { immediate: true, flush: 'post' }
);
const handleKeydown = (event: KeyboardEvent) => {
  if (openDialogs[openDialogs.length - 1] !== id) return;
  // A listbox rendered in a portal handles Escape before it reaches the dialog.
  if (event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    props.onClose();
  }
  if (event.key !== 'Tab' || !dialog.value) return;
  const elements = [
    ...dialog.value.querySelectorAll<HTMLElement>(
      'button:not(:disabled), a[href], input:not(:disabled):not([type="hidden"]), textarea:not(:disabled), [tabindex="0"]'
    )
  ].filter((element) => element.getClientRects().length);
  const first = elements[0],
    last = elements[elements.length - 1];
  if (!first) {
    event.preventDefault();
    return;
  }
  if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.value)) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
};
onUnmounted(release);
</script>
<style scoped>
.dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: 10000;
  background: #0009;
  display: grid;
  place-items: center;
  padding: 24px;
}
.dialog {
  display: flex;
  flex-direction: column;
  width: min(560px, 100%);
  max-height: calc(100dvh - 48px);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  color: var(--foreground);
  box-shadow: 0 16px 60px #0005;
  outline: none;
}
.dialog-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--border);
  padding: 16px 20px;
  flex: none;
}
.dialog-header h2 {
  font-size: 14px;
  font-weight: 500;
  padding: 7px 0;
  overflow-wrap: anywhere;
  max-height: 20dvh;
  overflow: auto;
}
.dialog-header button {
  flex: none;
  margin-right: -6px;
}
.dialog-body {
  padding: 22px 20px;
  overflow: auto;
  overscroll-behavior: contain;
  font-size: 13px;
  line-height: 1.8;
  min-height: 0;
  overflow-wrap: anywhere;
}
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  padding: 14px 20px;
  border-top: 1px solid var(--border);
  flex: none;
}
@media (max-width: 720px) {
  .dialog-overlay {
    padding: 12px;
  }
  .dialog {
    max-height: calc(100dvh - 24px);
  }
  .dialog-header,
  .dialog-body,
  .dialog-footer {
    padding: 16px;
  }
}
</style>
