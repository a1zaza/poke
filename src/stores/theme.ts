import { defineStore } from 'pinia';
import { ref, watchEffect } from 'vue';

export const useThemeStore = defineStore('theme', () => {
  const theme = ref(localStorage.getItem('theme') === 'light' ? 'light' : 'dark');

  function toggleTheme() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark';
  }

  watchEffect(() => {
    document.documentElement.setAttribute('data-theme', theme.value);
    localStorage.setItem('theme', theme.value);
  });

  return { theme, toggleTheme };
});
