<template>
  <router-view />
  <ToastContainer />
</template>

<script setup lang="ts">
import { useThemeStore } from '@/stores/theme';
import ToastContainer from '@/components/ui/ToastContainer.vue';

useThemeStore();
</script>
