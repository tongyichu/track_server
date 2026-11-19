(function () {
  'use strict';
  const kit = window.AdminUsageCharts, $ = id => document.getElementById(id);
  let renderVersion = 0, latestStats, onOpenSpot, renderedStats;
  const disclosure = $('scenicChartsDisclosure');
  if (window.matchMedia('(max-width: 900px)').matches) disclosure.open = false;
  const map = kit.createSpatialMap({ chart: 'scenicSpatialChart', summary: 'scenicSpatialSummary', error: 'scenicSpatialError', reset: 'resetScenicMap', selection: 'scenicMapSelection', select: 'scenicMapSpot', open: 'openScenicMapSpot', idKey: 'scenic_spot_id', unit: '处' });
  function showError(message) { ['scenicSpatialError', 'scenicTrendError'].forEach(id => { $(id).textContent = message; $(id).hidden = false; }); }
  async function draw() {
    const current = ++renderVersion;
    const data = latestStats;
    if (!disclosure.open || !data) return;
    if (renderedStats === data) {
      window.echarts?.getInstanceByDom($('scenicTrendChart'))?.resize();
      map.resize();
      return;
    }
    try {
      await AdminAssets.loadCharts();
      if (current !== renderVersion || !disclosure.open) return;
      const trend = data.trend, hourly = trend.bucket === 'hour';
      kit.trend('scenicTrendChart', trend.items || [], [{ name: '新增入库', key: 'count', color: '#206bc4' }], hourly, '处');
      $('scenicTrendSummary').textContent = '新增 ' + Number(trend.total || 0).toLocaleString() + ' 处 · ' + (hourly ? '按小时' : '按天') + ' · ' + kit.date(trend.start, hourly) + ' — ' + kit.date(trend.end, true);
      $('scenicTrendError').hidden = true;
      await map.render(data.spatial || {}, onOpenSpot);
      if (current === renderVersion) renderedStats = data;
    } catch (_) {
      if (current === renderVersion) showError('图表暂不可用，请刷新重试');
    }
  }
  function render(data, openSpot) {
    latestStats = data;
    onOpenSpot = openSpot;
    return draw();
  }
  disclosure.addEventListener('toggle', draw);
  window.AdminScenicSpotCharts = { render, showError, setLoading: active => kit.loading('scenicTrendChart', active) };
})();
