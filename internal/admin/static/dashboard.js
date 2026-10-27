(function () {
  let period = 'day';
  let chart = null;

  function comparisonText(metric) {
    const current = Number(metric.current || 0);
    const previous = Number(metric.previous || 0);
    const delta = current - previous;
    if (delta === 0) return '与上一周期持平';
    return '较上一周期 ' + (delta > 0 ? '+' : '') + delta;
  }

  function setMetric(id, metric) {
    document.getElementById(id).textContent = Number(metric.current || 0).toLocaleString('zh-CN');
    const delta = document.getElementById(id + 'Delta');
    delta.textContent = comparisonText(metric);
    delta.className = Number(metric.current || 0) >= Number(metric.previous || 0) ? 'positive' : 'negative';
  }

  function renderChart(items) {
    if (!chart) chart = echarts.init(document.getElementById('trendChart'));
    chart.setOption({
      color: ['#2563eb', '#14b8a6'],
      tooltip: { trigger: 'axis' },
      legend: { data: ['新增用户', '新增用户轨迹'], bottom: 0 },
      grid: { left: 36, right: 16, top: 24, bottom: 48, containLabel: true },
      xAxis: { type: 'category', boundaryGap: false, data: items.map(function (item) { return item.date.slice(5); }), axisLine: { lineStyle: { color: '#d8dee9' } } },
      yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#edf0f5' } } },
      series: [
        { name: '新增用户', type: 'line', smooth: true, symbolSize: 7, areaStyle: { opacity: 0.08 }, data: items.map(function (item) { return item.new_users; }) },
        { name: '新增用户轨迹', type: 'line', smooth: true, symbolSize: 7, areaStyle: { opacity: 0.08 }, data: items.map(function (item) { return item.uploaded_tracks; }) }
      ]
    });
  }

  async function loadDashboard() {
    const error = document.getElementById('dashboardError');
    error.classList.add('hidden');
    try {
      const response = await fetch('/admin/api/dashboard?period=' + encodeURIComponent(period));
      if (response.status === 401) {
        window.location.href = '/admin/login.html';
        return;
      }
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || ('加载失败: ' + response.status));
      const data = payload.data;
      setMetric('newUsers', data.metrics.new_users);
      setMetric('uploadedTracks', data.metrics.uploaded_tracks);
      setMetric('uploadUsers', data.metrics.upload_users);
      setMetric('submissions', data.metrics.submissions);
      document.getElementById('pendingSubmissions').textContent = Number(data.metrics.pending_submissions || 0).toLocaleString('zh-CN');
      document.getElementById('rangeHint').textContent = new Date(data.range.start).toLocaleDateString('zh-CN') + ' 至今';
      document.getElementById('updatedAt').textContent = '数据更新时间：' + new Date(data.updated_at).toLocaleString('zh-CN');
      renderChart(data.trend || []);
    } catch (cause) {
      error.textContent = cause.message || String(cause);
      error.classList.remove('hidden');
    }
  }

  document.querySelectorAll('[data-period]').forEach(function (button) {
    button.addEventListener('click', function () {
      period = button.dataset.period;
      document.querySelectorAll('[data-period]').forEach(function (item) { item.classList.toggle('active', item === button); });
      loadDashboard();
    });
  });
  document.getElementById('refreshDashboard').addEventListener('click', loadDashboard);
  window.addEventListener('resize', function () { if (chart) chart.resize(); });
  fetch('/admin/api/me').then(function (response) {
    if (!response.ok) {
      window.location.href = '/admin/login.html';
      return null;
    }
    return response.json();
  }).then(function (payload) {
    if (payload && payload.data) document.getElementById('userName').textContent = payload.data.username;
  });
  document.getElementById('logoutBtn').addEventListener('click', async function () {
    await fetch('/admin/api/logout', { method: 'POST' });
    window.location.href = '/admin/login.html';
  });
  loadDashboard();
})();
