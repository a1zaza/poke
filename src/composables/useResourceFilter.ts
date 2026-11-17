import { computed, ref, type Ref } from 'vue';

/** Keep filtering local: searching must never change node scope or API requests. */
export function useResourceFilter<T>(items: Ref<T[]>, fields: (item: T) => Array<string | undefined | null>) {
  const search = ref('');
  const filtered = computed(() => {
    const query = search.value.trim().toLocaleLowerCase();
    return query
      ? items.value.filter((item) => fields(item).some((value) => value?.toLocaleLowerCase().includes(query)))
      : items.value;
  });
  return { search, filtered };
}
