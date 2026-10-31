(function () {
  'use strict';
  const $ = id => document.getElementById(id), kit = window.AdminUsageCharts;
  async function read(path) {
    const response = await fetch('/admin/api/' + path, { cache: 'no-store' });
    if (response.status === 401) { location.href = '/admin/login.html'; throw new Error('登录已失效'); }
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || '统计加载失败');
    return result.data;
  }
  if ($('routeSpatialChart')) {
    const map = kit.createSpatialMap({ chart: 'routeSpatialChart', summary: 'routeSpatialSummary', error: 'routeSpatialError', reset: 'resetRouteMap', selection: 'routeMapSelection', select: 'routeMapGroup', open: 'openRouteMapGroup', idKey: 'id', unit: '组' });
    let request = 0;
    async function load() {
      const current = ++request, button = $('refreshMapStatistics');
      button.disabled = true; button.textContent = '刷新中…'; $('mapStatisticsError').hidden = true;
      kit.loading('routeSizeChart', true);
      try {
        const data = await read('route-groups/statistics');
        if (current !== request) return;
        $('mapStatisticsScope').textContent = '全量有效路线组 ' + Number(data.group_count || 0).toLocaleString() + ' 组 · 覆盖 ' + Number(data.city_count || 0).toLocaleString() + ' 个城市';
        kit.bars('routeSizeChart', data.distribution || [], '组');
        await map.render(data, id => window.AdminRouteGroups.openDetail(id));
      } catch (error) {
        if (current === request) { $('mapStatisticsError').textContent = '统计刷新失败，请重试：' + error.message; $('mapStatisticsError').hidden = false; }
      } finally { if (current === request) { button.disabled = false; button.textContent = '刷新数据'; kit.loading('routeSizeChart', false); } }
    }
    $('refreshMapStatistics').addEventListener('click', () => { load(); window.AdminRouteGroups.refresh(); });
    load();
  }
  if ($('companionTrendChart')) {
    let days = 30, loadedDays = 30, request = 0;
    const buttons = Array.from(document.querySelectorAll('[data-usage-days]'));
    function select() { buttons.forEach(button => { const active = Number(button.dataset.usageDays) === days; button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active)); }); }
    async function load() {
      const current = ++request;
      $('companionUsageError').hidden = true;
      ['companionTrendChart', 'companionSizeChart'].forEach(id => kit.loading(id, true));
      try {
        const data = await read('companions/usage-statistics?days=' + days);
        if (current !== request) return;
        loadedDays = data.days;
        $('companionUsageScope').textContent = kit.date(data.start, data.bucket === 'hour') + ' — ' + kit.date(data.end, true) + ' · ' + (data.bucket === 'hour' ? '按小时' : '按天');
        kit.trend('companionTrendChart', data.trend || [], [{ name: '创建', key: 'created', color: '#206bc4' }, { name: '结束', key: 'ended', color: '#2b8a6e' }], data.bucket === 'hour', '个房间');
        kit.bars('companionSizeChart', data.participants || [], '个房间');
      } catch (error) {
        if (current !== request) return;
        days = loadedDays; select();
        $('companionUsageError').textContent = '统计刷新失败，请重试：' + error.message; $('companionUsageError').hidden = false;
      } finally { if (current === request) ['companionTrendChart', 'companionSizeChart'].forEach(id => kit.loading(id, false)); }
    }
    buttons.forEach(button => button.addEventListener('click', () => { const value = Number(button.dataset.usageDays); if (value !== days) { days = value; select(); load(); } }));
    window.AdminCompanionUsage = { refresh: load };
    load();
  }
})();
