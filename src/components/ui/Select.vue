<template>
  <div ref="containerRef" class="select-container">
    <button
      ref="triggerRef"
      type="button"
      class="select-trigger"
      :disabled="disabled || !options.length"
      :aria-label="label"
      aria-haspopup="listbox"
      :aria-expanded="isOpen"
      :aria-controls="id"
      @click="toggle"
      @keydown.down.prevent="open(1)"
      @keydown.up.prevent="open(-1)"
    >
      <span :title="selectedOption?.label">{{ selectedOption?.label || placeholder }}</span><ChevronDown
        :size="13"
      />
    </button>
    <Teleport to="body">
      <div
        v-if="isOpen"
        :id="id"
        ref="listRef"
        class="select-options"
        :style="position"
        role="listbox"
        :aria-label="label"
        :aria-activedescendant="`${id}-${activeIndex}`"
        tabindex="-1"
        @keydown="onKeydown"
      >
        <div
          v-for="(option, index) in options"
          :id="`${id}-${index}`"
          :key="option.value"
          role="option"
          :aria-selected="option.value === modelValue"
          class="select-option"
          :class="{ selected: option.value === modelValue, highlighted: activeIndex === index }"
          :title="option.label"
          @pointermove="activeIndex = index"
          @click="choose(option.value)"
        >
          <span>{{ option.label }}</span><Check v-if="option.value === modelValue" :size="13" />
        </div>
      </div>
    </Teleport>
  </div>
</template>
<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue';
import { Check, ChevronDown } from 'lucide-vue-next';
const props = withDefaults(
  defineProps<{
    options: { label: string; value: number | string }[];
    modelValue: number | string;
    placement?: 'top' | 'bottom';
    label?: string;
    placeholder?: string;
    disabled?: boolean;
  }>(),
  { placement: 'bottom', label: '选择选项', placeholder: '请选择', disabled: false }
);
const emit = defineEmits<{ 'update:modelValue': [value: number | string] }>();
const id = useId();
const isOpen = ref(false);
const activeIndex = ref(0);
const containerRef = ref<HTMLElement | null>(null);
const triggerRef = ref<HTMLButtonElement | null>(null);
const listRef = ref<HTMLElement | null>(null);
const position = ref<Record<string, string>>({});
const selectedOption = computed(() => props.options.find((option) => option.value === props.modelValue));
const updatePosition = () => {
  if (!isOpen.value || !triggerRef.value) return;
  const rect = triggerRef.value.getBoundingClientRect();
  const width = Math.min(Math.max(rect.width, 160), window.innerWidth - 16);
  const below = window.innerHeight - rect.bottom - 8;
  const above = rect.top - 8;
  const top = props.placement === 'top' ? above >= 120 || above > below : below < 160 && above > below;
  position.value = {
    width: `${width}px`,
    left: `${Math.max(8, Math.min(rect.left, window.innerWidth - width - 8))}px`,
    ...(top ? { bottom: `${window.innerHeight - rect.top + 4}px` } : { top: `${rect.bottom + 4}px` }),
    maxHeight: `${Math.max(44, Math.min(280, top ? above - 4 : below - 4))}px`
  };
};
const close = (restore = true) => {
  isOpen.value = false;
  if (restore) triggerRef.value?.focus();
};
const open = async (direction = 1) => {
  if (props.disabled || !props.options.length) return;
  isOpen.value = true;
  const selected = props.options.findIndex((option) => option.value === props.modelValue);
  activeIndex.value = selected >= 0 ? selected : direction === -1 ? props.options.length - 1 : 0;
  updatePosition();
  await nextTick();
  listRef.value?.focus();
  scrollActive();
};
const toggle = () => (isOpen.value ? close() : void open());
const choose = (value: number | string) => {
  emit('update:modelValue', value);
  close();
};
const scrollActive = () =>
  document.getElementById(`${id}-${activeIndex.value}`)?.scrollIntoView({ block: 'nearest' });
const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    event.stopPropagation();
    event.preventDefault();
    close();
  } else if (event.key === 'Tab') close();
  else if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault();
    const option = props.options[activeIndex.value];
    if (option) choose(option.value);
  } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault();
    if (event.key === 'Home') activeIndex.value = 0;
    else if (event.key === 'End') activeIndex.value = props.options.length - 1;
    else
      activeIndex.value =
        (activeIndex.value + (event.key === 'ArrowDown' ? 1 : -1) + props.options.length) %
        props.options.length;
    scrollActive();
  }
};
const outside = (event: PointerEvent) => {
  if (!containerRef.value?.contains(event.target as Node) && !listRef.value?.contains(event.target as Node))
    close(false);
};
watch(
  () => [props.disabled, props.options],
  () => {
    if (isOpen.value) close();
  }
);
onMounted(() => {
  document.addEventListener('pointerdown', outside);
  window.addEventListener('resize', updatePosition);
  window.addEventListener('scroll', updatePosition, true);
});
onUnmounted(() => {
  document.removeEventListener('pointerdown', outside);
  window.removeEventListener('resize', updatePosition);
  window.removeEventListener('scroll', updatePosition, true);
});
</script>
<style scoped>
.select-container {
  position: relative;
  display: inline-block;
  min-width: 80px;
  max-width: 100%;
}
.select-trigger {
  width: 100%;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
  text-align: left;
}
.select-trigger span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.select-trigger svg {
  color: var(--system-gray);
}
.select-options {
  position: fixed;
  z-index: 11000;
  padding: 4px;
  overflow: auto;
  overscroll-behavior: contain;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
  box-shadow: var(--glass-shadow);
  outline: none;
}
.select-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 8px 10px;
  cursor: pointer;
  border-radius: 3px;
  font-size: 12px;
}
.select-option span {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.select-option.selected {
  color: var(--blue);
}
.select-option.highlighted {
  background: var(--item-hover);
}
@media (max-width: 720px) {
  .select-trigger,
  .select-option {
    min-height: 44px;
    font-size: 13px;
  }
}
</style>
