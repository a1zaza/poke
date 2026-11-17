import { computed, ref } from 'vue';

export type ImageUpdateStatus = 'checking' | 'available' | 'latest' | 'error';
interface UpdateState {
  status: ImageUpdateStatus;
  error?: string;
  checkedAt?: number;
}
interface UpdateResult {
  hasUpdate?: boolean;
  error?: string;
}
export const canCheckImage = (image: string) =>
  !!image && !image.startsWith('sha256:') && !image.includes('@');

/** One queue per page/node. Polling only schedules missing or expired images. */
export function useImageUpdates(check: (image: string, signal: AbortSignal) => Promise<UpdateResult>) {
  const states = ref<Record<string, UpdateState>>({});
  const active = ref(0);
  const queued = ref(0);
  const pending = new Set<string>();
  const requests = new Map<string, AbortController>();
  let wanted = new Set<string>();
  let stopped = false;
  let blockedUntil = 0;
  let wake: ReturnType<typeof setTimeout> | undefined;
  const checking = computed(() => active.value > 0 || queued.value > 0);

  function pump() {
    if (stopped || !pending.size) return;
    if (Date.now() < blockedUntil) {
      if (!wake)
        wake = setTimeout(() => {
          wake = undefined;
          pump();
        }, blockedUntil - Date.now());
      return;
    }
    while (active.value < 2 && pending.size) {
      const image = pending.values().next().value!;
      pending.delete(image);
      queued.value = pending.size;
      const controller = new AbortController();
      requests.set(image, controller);
      active.value++;
      void run(image, controller);
    }
  }

  async function run(image: string, controller: AbortController) {
    try {
      const result = await check(image, controller.signal);
      if (stopped || controller.signal.aborted || !wanted.has(image)) return;
      states.value[image] = result.error
        ? { status: 'error', error: result.error, checkedAt: Date.now() }
        : { status: result.hasUpdate ? 'available' : 'latest', checkedAt: Date.now() };
    } catch (error) {
      if (stopped || controller.signal.aborted || !wanted.has(image)) return;
      const limited = (error as { status?: number })?.status === 429;
      if (limited) blockedUntil = Date.now() + 30_000;
      states.value[image] = {
        status: 'error',
        checkedAt: Date.now(),
        error: limited
          ? '检测请求繁忙，已暂停后续检测，稍后重试'
          : error instanceof Error
            ? error.message
            : '更新检测失败'
      };
    } finally {
      if (requests.get(image) === controller) requests.delete(image);
      active.value--;
      pump();
    }
  }

  function sync(images: string[], force = false) {
    if (stopped) return;
    wanted = new Set(images.filter(canCheckImage));
    for (const image of Object.keys(states.value)) {
      if (!wanted.has(image)) {
        requests.get(image)?.abort();
        pending.delete(image);
        delete states.value[image];
      }
    }
    for (const image of wanted) {
      if (requests.has(image) || pending.has(image)) continue;
      const state = states.value[image];
      const ttl = state?.status === 'error' ? 60_000 : 300_000;
      if (!force && state?.checkedAt !== undefined && Date.now() - state.checkedAt < ttl) continue;
      states.value[image] = { status: 'checking' };
      pending.add(image);
    }
    queued.value = pending.size;
    pump();
  }

  function invalidate(image: string) {
    requests.get(image)?.abort();
    pending.delete(image);
    queued.value = pending.size;
    delete states.value[image];
  }

  function stop() {
    stopped = true;
    if (wake) clearTimeout(wake);
    pending.clear();
    queued.value = 0;
    for (const controller of requests.values()) controller.abort();
  }

  return { states, checking, sync, invalidate, stop };
}
