(function () {
  'use strict';
  const kit = window.AdminUsageCharts, $ = id => document.getElementById(id);
  const map = kit.createSpatialMap({ chart: 'scenicSpatialChart', summary: 'scenicSpatialSummary', error: 'scenicSpatialError', reset: 'resetScenicMap', selection: 'scenicMapSelection', select: 'scenicMapSpot', open: 'openScenicMapSpot', idKey: 'scenic_spot_id', unit: '处' });
  function showError(message) { ['scenicSpatialError', 'scenicTrendError'].forEach(id => { $(id).textContent = message; $(id).hidden = false; }); }
  async function render(data, openSpot) {
    const trend = data.trend, hourly = trend.bucket === 'hour';
    kit.trend('scenicTrendChart', trend.items || [], [{ name: '新增入库', key: 'count', color: '#206bc4' }], hourly, '处');
    $('scenicTrendSummary').textContent = '新增 ' + Number(trend.total || 0).toLocaleString() + ' 处 · ' + (hourly ? '按小时' : '按天') + ' · ' + kit.date(trend.start, hourly) + ' — ' + kit.date(trend.end, true);
    $('scenicTrendError').hidden = true;
    await map.render(data.spatial || {}, openSpot);
  }
  window.AdminScenicSpotCharts = { render, showError, setLoading: active => kit.loading('scenicTrendChart', active) };
})();
