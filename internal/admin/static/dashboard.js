(function () {
  let period = 'day';
  const charts = {};
  let chartLibraryPromise;
  const colors = ['#206bc4', '#2fb344', '#f59f00', '#ae3ec9', '#d63939', '#17a2b8', '#6f42c1', '#74b816'];

  function number(value) {
    return Number(value || 0);
  }

  function formatNumber(value, digits) {
    return number(value).toLocaleString('zh-CN', { maximumFractionDigits: digits == null ? 0 : digits });
  }

  function comparisonText(metric) {
    metric = metric || {};
    const current = number(metric.current);
    const previous = number(metric.previous);
    const delta = current - previous;
    if (delta === 0) return '与上一周期持平';
    return '较上一周期 ' + (delta > 0 ? '+' : '') + formatNumber(delta);
  }

  function setMetric(id, metric) {
    metric = metric || {};
    document.getElementById(id).textContent = formatNumber(metric.current);
    const delta = document.getElementById(id + 'Delta');
    if (!delta) return;
    delta.textContent = comparisonText(metric);
    delta.className = number(metric.current) >= number(metric.previous) ? 'positive' : 'negative';
  }

  function setText(id, value) {
    const element = document.getElementById(id);
    if (element) element.textContent = value;
  }

  function chart(id) {
    const element = document.getElementById(id);
    if (!element) return null;
    if (!charts[id]) charts[id] = echarts.init(element);
    return charts[id];
  }

  function chartOption(id, option) {
    const instance = chart(id);
    if (instance) instance.setOption(option, true);
  }

  function loadChartLibrary() {
    if (window.echarts) return Promise.resolve();
    if (chartLibraryPromise) return chartLibraryPromise;
    chartLibraryPromise = new Promise(function (resolve, reject) {
      const script = document.createElement('script');
      script.src = '/admin/static/vendor/echarts.min.js';
      script.onload = resolve;
      script.onerror = function () {
        chartLibraryPromise = null;
        reject(new Error('图表库加载失败'));
      };
      document.head.appendChild(script);
    });
    return chartLibraryPromise;
  }

  function axisTooltip() {
    return { trigger: 'axis', backgroundColor: 'rgba(17,24,39,.92)', borderWidth: 0, textStyle: { color: '#fff' } };
  }

  function baseGrid(bottom) {
    return { left: 18, right: 18, top: 24, bottom: bottom || 38, containLabel: true };
  }

  function renderTrend(items) {
    items = items || [];
    chartOption('trendChart', {
      color: [colors[0], colors[1], colors[2]],
      tooltip: axisTooltip(),
      legend: { data: ['活跃用户', '新增用户', '新增轨迹'], bottom: 0, itemWidth: 14, itemHeight: 8 },
      grid: baseGrid(52),
      xAxis: { type: 'category', boundaryGap: false, data: items.map(function (item) { return item.date.slice(5); }), axisLine: { lineStyle: { color: '#dce1e8' } }, axisTick: { show: false } },
      yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#edf0f5' } } },
      series: [
        { name: '活跃用户', type: 'line', smooth: true, symbol: 'none', lineStyle: { width: 3 }, areaStyle: { opacity: .07 }, data: items.map(function (item) { return item.active_users; }) },
        { name: '新增用户', type: 'line', smooth: true, symbol: 'none', lineStyle: { width: 2 }, data: items.map(function (item) { return item.new_users; }) },
        { name: '新增轨迹', type: 'line', smooth: true, symbol: 'none', lineStyle: { width: 2 }, data: items.map(function (item) { return item.uploaded_tracks; }) }
      ]
    });
  }

  function renderFunnel(data) {
    data = data || {};
    const registered = number(data.registered);
    const values = [
      { name: '注册', value: registered },
      { name: '上传轨迹', value: number(data.uploaded) },
      { name: '发起投稿', value: number(data.submitted) },
      { name: '审核通过', value: number(data.approved) }
    ];
    chartOption('funnelChart', {
      color: ['#206bc4', '#4299e1', '#2fb344', '#74b816'],
      tooltip: { trigger: 'item', formatter: function (item) { return item.name + '：' + formatNumber(item.value) + (registered ? '<br/>注册转化率：' + (item.value / registered * 100).toFixed(1) + '%' : ''); } },
      series: [{
        type: 'funnel', left: '6%', top: 18, bottom: 8, width: '88%', minSize: '28%', maxSize: '100%', sort: 'descending', gap: 4,
        label: { show: true, position: 'inside', color: '#fff', formatter: function (item) { return item.name + '  ' + formatNumber(item.value); } },
        labelLine: { show: false }, itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 }, data: values
      }]
    });
  }

  function pieOption(items, palette) {
    items = items || [];
    return {
      color: palette || colors,
      tooltip: { trigger: 'item', formatter: '{b}：{c}（{d}%）' },
      legend: { bottom: 0, type: 'scroll', itemWidth: 12, itemHeight: 8 },
      series: [{ type: 'pie', radius: ['48%', '70%'], center: ['50%', '43%'], minAngle: 4, avoidLabelOverlap: true, label: { show: false }, emphasis: { label: { show: true, fontWeight: 600 } }, data: items }]
    };
  }

  function distributionData(items) {
    return (items || []).map(function (item) { return { name: item.label || item.key || '未知', value: number(item.count) }; });
  }

  function renderReviewMetrics(review) {
    review = review || {};
    setText('reviewPending', formatNumber(review.pending));
    setText('reviewOver24', formatNumber(review.pending_over_24h));
    setText('reviewOver72', formatNumber(review.pending_over_72h));
    setText('approvalRate', (number(review.approval_rate) * 100).toFixed(1) + '%');
    setText('avgReviewHours', formatDurationHours(review.avg_review_hours));
    setText('oldestPending', review.oldest_pending ? new Date(review.oldest_pending).toLocaleString('zh-CN') : '暂无积压');
  }

  function renderReviewChart(review) {
    review = review || {};
    chartOption('reviewChart', pieOption([
      { name: '通过', value: number(review.approved) },
      { name: '驳回', value: number(review.rejected) }
    ], ['#2fb344', '#d63939']));
  }

  function renderFeedbackMetrics(feedback) {
    feedback = feedback || {};
    setText('feedbackPending', formatNumber(feedback.pending));
    setText('feedbackProcessing', formatNumber(feedback.processing));
    setText('feedbackOver24', formatNumber(feedback.pending_over_24h));
    setText('feedbackOver72', formatNumber(feedback.pending_over_72h));
    setText('feedbackResolvedPeriod', formatNumber(feedback.resolved_in_period));
    setText('avgResolutionHours', formatDurationHours(feedback.avg_resolution_hours));
  }

  function renderFeedbackCharts(feedback) {
    feedback = feedback || {};
    chartOption('feedbackStatusChart', pieOption([
      { name: '待处理', value: number(feedback.pending) },
      { name: '处理中', value: number(feedback.processing) },
      { name: '已解决', value: number(feedback.resolved) },
      { name: '已忽略', value: number(feedback.ignored) }
    ], ['#f59f00', '#206bc4', '#2fb344', '#adb5bd']));
    const source = [];
    (feedback.platforms || []).forEach(function (item) { source.push({ name: '平台 · ' + (item.label || item.key), value: number(item.count) }); });
    (feedback.versions || []).slice(0, 5).forEach(function (item) { source.push({ name: '版本 · ' + (item.label || item.key), value: number(item.count) }); });
    chartOption('feedbackVersionChart', horizontalBarOption(source));
  }

  function horizontalBarOption(items) {
    items = items || [];
    return {
      color: [colors[0]], tooltip: axisTooltip(), grid: { left: 8, right: 20, top: 10, bottom: 10, containLabel: true },
      xAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#edf0f5' } } },
      yAxis: { type: 'category', inverse: true, axisTick: { show: false }, axisLine: { show: false }, data: items.map(function (item) { return item.name; }), axisLabel: { width: 92, overflow: 'truncate' } },
      series: [{ type: 'bar', barMaxWidth: 16, itemStyle: { borderRadius: [0, 4, 4, 0] }, label: { show: true, position: 'right' }, data: items.map(function (item) { return item.value; }) }]
    };
  }

  function renderContentMetrics(content) {
    content = content || {};
    setText('avgDistance', formatDistance(content.avg_distance_m));
    setText('avgDuration', formatDurationSeconds(content.avg_duration_s));
    setText('staleRunning', formatNumber(content.stale_running));
    setText('zeroDistance', formatNumber(content.zero_distance));
    setText('failedMapJobs', formatNumber(content.failed_map_jobs));
    setText('pendingMapJobs', formatNumber(content.pending_map_jobs));
    setText('activeRouteGroups', formatNumber(content.active_route_groups));
  }

  function renderContentCharts(content) {
    content = content || {};
    chartOption('trackStatusChart', pieOption(distributionData(content.statuses), ['#2fb344', '#f59f00', '#d63939']));
    chartOption('trackTypeChart', pieOption(distributionData(content.types), colors));
    chartOption('trackCityChart', horizontalBarOption(distributionData(content.cities)));
  }

  function renderEngagementMetrics(engagement) {
    engagement = engagement || {};
    const current = engagement.current || {};
    const previous = engagement.previous || {};
    ['Collections', 'Navigations', 'Likes', 'Comments'].forEach(function (suffix) {
      const key = suffix.toLowerCase();
      setText('engagement' + suffix, formatNumber(current[key]));
      const delta = document.getElementById('engagement' + suffix + 'Delta');
      if (delta) {
        delta.textContent = comparisonText({ current: current[key], previous: previous[key] });
        delta.className = number(current[key]) >= number(previous[key]) ? 'positive' : 'negative';
      }
    });
    renderTopTracks(engagement.top_tracks || []);
  }

  function renderEngagementChart(engagement, trend) {
    engagement = engagement || {};
    trend = trend || [];
    chartOption('engagementChart', {
      color: [colors[0], colors[1], colors[3], colors[2]], tooltip: axisTooltip(),
      legend: { data: ['收藏', '导航', '点赞', '评论'], bottom: 0, itemWidth: 14, itemHeight: 8 }, grid: baseGrid(52),
      xAxis: { type: 'category', boundaryGap: false, data: trend.map(function (item) { return item.date.slice(5); }), axisTick: { show: false }, axisLine: { lineStyle: { color: '#dce1e8' } } },
      yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#edf0f5' } } },
      series: [
        { name: '收藏', type: 'line', smooth: true, symbol: 'none', data: trend.map(function (item) { return item.collections; }) },
        { name: '导航', type: 'line', smooth: true, symbol: 'none', data: trend.map(function (item) { return item.navigations; }) },
        { name: '点赞', type: 'line', smooth: true, symbol: 'none', data: trend.map(function (item) { return item.likes; }) },
        { name: '评论', type: 'line', smooth: true, symbol: 'none', data: trend.map(function (item) { return item.comments; }) }
      ]
    });
  }

  function renderCompanion(companion) {
    companion = companion || {};
    setText('companionActive', formatNumber(companion.active));
    setText('companionCreated', formatNumber((companion.created || {}).current));
    setText('companionEnded', formatNumber(companion.ended));
    setText('companionAutoEnded', formatNumber(companion.auto_ended));
    setText('companionParticipants', formatNumber(companion.avg_participants, 1) + ' 人');
  }

  function renderTopTracks(items) {
    const body = document.getElementById('topTracks');
    body.innerHTML = '';
    if (!items.length) {
      body.innerHTML = '<tr><td colspan="7" class="text-secondary text-center">暂无互动数据</td></tr>';
      return;
    }
    items.forEach(function (item) {
      const row = document.createElement('tr');
      row.innerHTML = '<td class="mono">' + esc(item.track_id) + '</td>' +
        '<td>' + esc(item.title || '未命名轨迹') + '</td>' +
        '<td>' + formatNumber(item.collections) + '</td>' +
        '<td>' + formatNumber(item.navigations) + '</td>' +
        '<td>' + formatNumber(item.likes) + '</td>' +
        '<td>' + formatNumber(item.comments) + '</td>' +
        '<td><strong>' + formatNumber(item.score) + '</strong></td>';
      body.appendChild(row);
    });
  }

  function formatDurationHours(value) {
    const hours = number(value);
    if (!hours) return '0 小时';
    if (hours < 1) return Math.round(hours * 60) + ' 分钟';
    return hours.toFixed(hours < 10 ? 1 : 0) + ' 小时';
  }

  function formatDurationSeconds(value) {
    const seconds = number(value);
    if (seconds >= 3600) return (seconds / 3600).toFixed(1) + ' 小时';
    return Math.round(seconds / 60) + ' 分钟';
  }

  function formatDistance(value) {
    const meters = number(value);
    if (meters >= 1000) return (meters / 1000).toFixed(1) + ' km';
    return Math.round(meters) + ' m';
  }

  function esc(value) {
    return String(value == null ? '' : value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
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
      const data = payload.data || {};
      const metrics = data.metrics || {};
      setMetric('activeUsers', metrics.active_users);
      setMetric('newUsers', metrics.new_users);
      setMetric('uploadedTracks', metrics.uploaded_tracks);
      setMetric('uploadUsers', metrics.upload_users);
      setMetric('submissions', metrics.submissions);
      setText('pendingSubmissions', formatNumber(metrics.pending_submissions));
      setText('pendingFeedback', formatNumber(metrics.pending_feedback));
      setText('activeCompanions', formatNumber(metrics.active_companions));
      setText('rangeHint', new Date(data.range.start).toLocaleDateString('zh-CN') + ' 至今');
      setText('updatedAt', '数据更新时间：' + new Date(data.updated_at).toLocaleString('zh-CN'));
      renderReviewMetrics(data.review);
      renderFeedbackMetrics(data.feedback);
      renderContentMetrics(data.content);
      renderEngagementMetrics(data.engagement);
      renderCompanion(data.companion);
      await new Promise(function (resolve) { requestAnimationFrame(resolve); });
      await loadChartLibrary();
      renderTrend(data.trend);
      renderFunnel(data.funnel);
      renderReviewChart(data.review);
      renderFeedbackCharts(data.feedback);
      renderContentCharts(data.content);
      renderEngagementChart(data.engagement, data.trend);
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
  window.addEventListener('resize', function () { Object.keys(charts).forEach(function (key) { charts[key].resize(); }); });
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
