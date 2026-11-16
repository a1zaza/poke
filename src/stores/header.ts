import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useHeaderStore = defineStore('header', () => {
    const title = ref('');
    const showBack = ref(false);

    function setTitle(newTitle: string) {
        title.value = newTitle;
    }

    function setBack(show: boolean) {
        showBack.value = show;
    }

    function set(newTitle: string, back: boolean = false) {
        title.value = newTitle;
        showBack.value = back;
    }

    function reset() {
        title.value = '';
        showBack.value = false;
    }

    return { title, showBack, setTitle, setBack, set, reset };
});
