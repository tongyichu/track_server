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
    const routes = window.AdminRouteGroups;
    const map = kit.createRouteCoverageMap({
      chart: 'routeSpatialChart', summary: 'routeSpatialSummary', error: 'routeSpatialError', reset: 'resetRouteMap',
      selection: 'routeMapSelection', select: 'routeMapGroup', open: 'openRouteMapGroup',
      openRoute: routes.openDetail, openScenic: routes.openScenic
    });
    const details = $('routeStatistics');
    let request = 0, loaded = false;
    async function load() {
      if (!details.open) return;
      const current = ++request, button = $('refreshMapStatistics');
      button.disabled = true; button.textContent = '刷新中…'; $('mapStatisticsError').hidden = true;
      kit.loading('routeSizeChart', true);
      try {
        const data = await read('route-groups/statistics');
        if (current !== request || !details.open) return;
        $('mapStatisticsScope').textContent = '全量有效路线组 ' + Number(data.group_count || 0).toLocaleString() + ' 组 · 覆盖 ' + Number(data.city_count || 0).toLocaleString() + ' 个城市';
        await AdminAssets.loadCharts();
        if (current !== request || !details.open) return;
        kit.bars('routeSizeChart', data.distribution || [], '组');
        loaded = true;
      } catch (error) {
        if (current === request) { $('mapStatisticsError').textContent = '统计刷新失败，请重试：' + error.message; $('mapStatisticsError').hidden = false; }
      } finally { if (current === request) { button.disabled = false; button.textContent = '刷新数据'; kit.loading('routeSizeChart', false); } }
    }
    function renderCoverage() { if (details.open) map.render(routes.directory()); }
    $('refreshMapStatistics').addEventListener('click', () => { routes.refreshDirectory(); load(); });
    window.addEventListener('admin:route-groups-changed', () => { loaded = false; load(); });
    window.addEventListener('admin:route-directory-updated', renderCoverage);
    AdminStats.onExpand(details, () => { renderCoverage(); if (loaded) kit.resize(); else load(); });
    if (details.open) { renderCoverage(); load(); }
  }
  if ($('companionTrendChart')) {
    const details = $('companionUsage');
    let days = 30, loaded = false, request = 0;
    const buttons = Array.from(document.querySelectorAll('[data-usage-days]'));
    function select() { buttons.forEach(button => { const active = Number(button.dataset.usageDays) === days; button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active)); }); }
    async function load(targetDays = days) {
      if (!details.open) return;
      const current = ++request;
      $('companionUsageError').hidden = true;
      ['companionTrendChart', 'companionSizeChart'].forEach(id => kit.loading(id, true));
      try {
        const data = await read('companions/usage-statistics?days=' + targetDays);
        if (current !== request || !details.open) return;
        await AdminAssets.loadCharts();
        if (current !== request || !details.open) return;
        kit.trend('companionTrendChart', data.trend || [], [{ name: '创建', key: 'created', color: '#206bc4' }, { name: '结束', key: 'ended', color: '#2b8a6e' }], data.bucket === 'hour', '个房间');
        kit.bars('companionSizeChart', data.participants || [], '个房间');
        $('companionUsageScope').textContent = kit.date(data.start, data.bucket === 'hour') + ' 至 ' + kit.date(data.end, true) + ' · ' + (data.bucket === 'hour' ? '按小时' : '按天');
        days = data.days;
        loaded = true;
        select();
      } catch (error) {
        if (current !== request) return;
        $('companionUsageError').textContent = '统计刷新失败，请重试：' + error.message; $('companionUsageError').hidden = false;
      } finally { if (current === request) ['companionTrendChart', 'companionSizeChart'].forEach(id => kit.loading(id, false)); }
    }
    buttons.forEach(button => button.addEventListener('click', () => load(Number(button.dataset.usageDays))));
    window.AdminCompanionUsage = { refresh: load };
    AdminStats.onExpand(details, () => { if (loaded) kit.resize(); else load(); });
    if (details.open) load();
  }
})();
