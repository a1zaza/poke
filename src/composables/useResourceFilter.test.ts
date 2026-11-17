import { describe, expect, it } from 'vitest';
import { ref } from 'vue';
import { useResourceFilter } from './useResourceFilter';

describe('本地资源搜索', () => {
  it('按名称、标签及路径匹配，忽略大小写和首尾空白', () => {
    const items = ref([
      { name: '前端应用', tags: ['GHCR.io/example/Web:1'], path: '/poke/projects/web' },
      { name: '数据库', tags: null, path: '/poke/projects/db' }
    ]);
    const { search, filtered } = useResourceFilter(items, (item) => [
      item.name,
      ...(item.tags || []),
      item.path
    ]);
    search.value = '  ghcr.IO  ';
    expect(filtered.value.map((item) => item.name)).toEqual(['前端应用']);
    search.value = '数据库';
    expect(filtered.value).toEqual([items.value[1]]);
    search.value = '/projects/db';
    expect(filtered.value).toEqual([items.value[1]]);
    expect(items.value).toHaveLength(2);
  });

  it('跟随后台列表更新，空搜索显示全部，未匹配时显示空列表', () => {
    const items = ref([{ name: 'api' }]);
    const { search, filtered } = useResourceFilter(items, (item) => [item.name]);
    search.value = 'worker';
    expect(filtered.value).toEqual([]);
    items.value = [{ name: 'worker' }];
    expect(filtered.value).toEqual([{ name: 'worker' }]);
    search.value = '   ';
    expect(filtered.value).toEqual(items.value);
  });
});
