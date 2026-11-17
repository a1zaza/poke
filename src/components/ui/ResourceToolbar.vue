<template>
  <div class="resource-toolbar">
    <label class="resource-search"
      ><Search :size="15" /><input
        :value="modelValue"
        :placeholder="placeholder"
        :aria-label="placeholder"
        type="search"
        @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    /></label>
    <span class="resource-count" aria-live="polite">{{
      modelValue ? `${filtered} / ${total}` : `共 ${total} 项`
    }}</span>
    <button class="secondary refresh-button" :disabled="loading" @click="$emit('refresh')">
      <RefreshCw :size="14" :class="{ spin: loading }" />刷新
    </button>
  </div>
</template>
<script setup lang="ts">
import { RefreshCw, Search } from 'lucide-vue-next';
defineProps<{ modelValue: string; total: number; filtered: number; loading: boolean; placeholder: string }>();
defineEmits<{ 'update:modelValue': [value: string]; refresh: [] }>();
</script>
<style scoped>
.resource-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.resource-search {
  display: flex;
  align-items: center;
  position: relative;
  width: min(100%, 340px);
}
.resource-search svg {
  position: absolute;
  left: 10px;
  color: var(--system-gray);
  pointer-events: none;
}
.resource-search input {
  padding-left: 34px !important;
  background: var(--surface) !important;
}
.resource-count {
  color: var(--system-gray);
  font-size: 12px;
  white-space: nowrap;
}
.refresh-button {
  margin-left: auto;
  font-size: 12px;
}
@media (max-width: 540px) {
  .resource-search {
    width: 100%;
  }
}
</style>
