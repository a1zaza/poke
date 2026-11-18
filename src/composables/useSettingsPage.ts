import { onMounted, ref } from 'vue';
import { request } from '@/utils/request';
import { useToastStore } from '@/stores/toast';

export interface RegistrySetting {
  domain: string;
  username: string;
  password?: string;
}
export interface SystemSettings {
  username: string;
  hostname: string;
  logo: string;
  mode: string;
  registries: RegistrySetting[];
}

export function useSettingsPage(apply: (data: SystemSettings) => void) {
  const loading = ref(true);
  const loadError = ref('');
  const saving = ref(false);
  const mounted = ref(false);
  const toast = useToastStore();
  async function load() {
    loading.value = true;
    loadError.value = '';
    try {
      apply(await request<SystemSettings>('/api/settings/system', { localOnly: true }));
    } catch (error) {
      loadError.value = error instanceof Error ? error.message : '加载设置失败，请重试';
    } finally {
      loading.value = false;
    }
  }
  async function save(updates: Record<string, unknown>): Promise<boolean> {
    if (saving.value || loading.value || loadError.value) return false;
    saving.value = true;
    try {
      await request('/api/settings/system', {
        method: 'POST',
        localOnly: true,
        body: JSON.stringify(updates)
      });
      toast.addToast('设置已保存', 'success');
      return true;
    } catch (error) {
      toast.addToast(error instanceof Error ? error.message : '保存失败', 'error');
      return false;
    } finally {
      saving.value = false;
    }
  }
  onMounted(() => {
    mounted.value = true;
    void load();
  });
  return { loading, loadError, saving, mounted, load, save };
}
