<template>
  <div class="select-container" ref="containerRef">
    <div class="select-trigger" @click="isOpen = !isOpen">
      <span>{{ selectedOption?.label || '' }}</span>
      <ChevronDown :size="12" style="margin-left: 6px; opacity: 0.5" />
    </div>

    <div v-if="isOpen" class="select-options" :class="placement">
      <div
        v-for="option in props.options"
        :key="option.value"
        class="select-option"
        :class="{ selected: option.value === props.modelValue }"
        @click="handleSelect(option.value)"
      >
        {{ option.label }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { ChevronDown } from 'lucide-vue-next';

interface Option {
  label: string;
  value: number | string;
}

interface Props {
  options: Option[];
  modelValue: number | string;
  placement?: 'top' | 'bottom';
}

const props = withDefaults(defineProps<Props>(), {
  placement: 'bottom'
});

const emit = defineEmits<{
  'update:modelValue': [value: number | string];
}>();

const isOpen = ref(false);
const containerRef = ref<HTMLElement | null>(null);

const selectedOption = computed(() => {
  return props.options.find(opt => opt.value === props.modelValue) || props.options[0];
});

const handleSelect = (value: number | string) => {
  emit('update:modelValue', value);
  isOpen.value = false;
};

const handleClickOutside = (event: MouseEvent) => {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    isOpen.value = false;
  }
};

onMounted(() => {
  document.addEventListener('mousedown', handleClickOutside);
});

onUnmounted(() => {
  document.removeEventListener('mousedown', handleClickOutside);
});
</script>

<style scoped>
.select-container {
  position: relative;
  display: inline-block;
  min-width: 80px;
}

.select-trigger {
  background: var(--input-background);
  border: 1px solid var(--glass-border);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--foreground);
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  transition: all 0.2s;
  user-select: none;
}

.select-trigger:hover {
  background: var(--item-hover);
}

.select-trigger:active {
  transform: translateY(1px);
}

.select-options {
  position: absolute;
  left: 0;
  right: 0;
  width: 100%;
  background: var(--window-background);
  backdrop-filter: blur(20px);
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  padding: 4px;
  z-index: 1000;
  box-shadow: var(--glass-shadow);
  display: flex;
  flex-direction: column;
  gap: 2px;
  animation: scaleIn 0.1s ease-out;
}

.select-options.bottom {
  top: 100%;
  margin-top: 4px;
  transform-origin: top right;
}

.select-options.top {
  bottom: 100%;
  margin-bottom: 8px;
  transform-origin: bottom right;
}

@keyframes scaleIn {
  from {
    transform: scale(0.95);
    opacity: 0;
  }
  to {
    transform: scale(1);
    opacity: 1;
  }
}

.select-option {
  padding: 6px 12px;
  font-size: 12px;
  color: var(--foreground);
  opacity: 0.8;
  cursor: pointer;
  border-radius: 4px;
  transition: background 0.1s;
  white-space: nowrap;
}

.select-option:hover {
  background: var(--item-hover);
  opacity: 1;
}

.select-option.selected {
  background: var(--item-hover);
  color: var(--system-blue);
  font-weight: 500;
}
</style>
