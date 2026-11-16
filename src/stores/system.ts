import { defineStore } from 'pinia';
import { ref } from 'vue';
import { request } from '@/utils/request';

export interface ManagedNode { id: string; name: string; status: string; is_local?: boolean; registries?: string[] }

export const useSystemStore = defineStore('system', () => {
    const info = ref({ hostname: 'Poke', logo: '/logo.png', isController: false, localEnabled: true });
    const nodes = ref<ManagedNode[]>([]);
    const currentNodeId = ref(localStorage.getItem('poke_current_node') || 'local');
    const loading = ref(false);
    const error = ref('');
    const ready = ref(false);
    let fetchPromise: Promise<void> | null = null;

    function fetchInfo(): Promise<void> {
        if (fetchPromise) return fetchPromise;
        loading.value = true;
        error.value = '';
        fetchPromise = (async () => {
            try {
                const [data, remote] = await Promise.all([
                    request('/api/settings/system', { localOnly: true }),
                    request<ManagedNode[]>('/api/nodes', { localOnly: true })
                ]);
                info.value.hostname = data.hostname || 'Poke';
                info.value.logo = data.logo || '/logo.png';
                info.value.isController = data.mode === 'controller';
                info.value.localEnabled = data.local_enabled !== false;
                nodes.value = [...(info.value.localEnabled ? [{ id: 'local', name: '当前实例', status: 'online', is_local: true }] : []), ...remote.filter(n => n.status !== 'revoked')];
                ready.value = true;
                if (!nodes.value.some(node => node.id === currentNodeId.value)) {
                    currentNodeId.value = nodes.value.find(n => n.status === 'online')?.id || '';
                    localStorage.setItem('poke_current_node', currentNodeId.value);
                }
            } catch (e) {
                console.error(e);
                error.value = '节点列表加载失败';
            } finally {
                loading.value = false;
                fetchPromise = null;
            }
        })();
        return fetchPromise;
    }
    function setNode(id: string) {
        currentNodeId.value = id;
        localStorage.setItem('poke_current_node', id);
        window.location.href = '/overview';
    }
    return { info, nodes, currentNodeId, loading, error, ready, fetchSystemInfo: fetchInfo, setNode };
});
