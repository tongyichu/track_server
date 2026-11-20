(function () {
  const periodLabels = { day: '今日', week: '本周', month: '本月' };
  const periodButtons = document.querySelectorAll('[data-period]');
  let requestedPeriod = 'day';
  let dashboardRequest = null;
  const charts = {};
  const colors = AdminChartStyle.colors;
  let latestData = null;
  let healthRequest = null;

  async function loadAnalyticsHealth() {
    const node = document.getElementById('analyticsHealth');
    if (!AdminAccess.can('analytics', false)) { node.hidden = true; return; }
    node.hidden = false;
    if (healthRequest) healthRequest.abort();
    const controller = new AbortController();
    healthRequest = controller;
    const timeout = setTimeout(function () { controller.abort(); }, 8000);
    const link = document.getElementById('analyticsHealthLink');
    const label = document.getElementById('analyticsHealthLabel');
    try {
      const response = await fetch('/admin/api/analytics/health', { signal: controller.signal });
      if (healthRequest !== controller) return;
      if (response.status === 401 || response.status === 403) { node.hidden = true; return; }
      if (!response.ok) throw new Error('health unavailable');
      const payload = await response.json();
      if (healthRequest !== controller) return;
      const data = payload.data;
      if (!data || !data.label || !['normal', 'warning', 'error', 'pending', 'disabled', 'unknown'].includes(data.state)) throw new Error('invalid health');
      link.dataset.state = data.state;
      label.textContent = '埋点 · ' + data.label;
      const details = [data.message || ''];
      if (data.last_finished_at) details.push('最近完成：' + AdminTime.dateTime(data.last_finished_at));
      if (data.next_due_at) details.push('计划同步：' + AdminTime.dateTime(data.next_due_at));
      details.push('查看同步日志');
      link.title = details.join('\n');
      link.setAttribute('aria-label', label.textContent + '。' + details.join('。'));
    } catch (cause) {
      if (healthRequest !== controller) return;
      link.dataset.state = 'unknown';
      label.textContent = '埋点 · 状态读取失败';
      link.title = '暂时无法获取归档状态，请刷新重试或查看同步日志';
      link.setAttribute('aria-label', label.textContent + '，查看同步日志');
    } finally {
      clearTimeout(timeout);
      if (healthRequest === controller) healthRequest = null;
    }
  }

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
    if (delta === 0) return '与上期持平';
    return '较上期 ' + (delta > 0 ? '+' : '') + formatNumber(delta);
  }

  function setMetric(id, metric) {
    setText(id, metric ? formatNumber(metric.current) : '—');
    const delta = document.getElementById(id + 'Delta');
    if (!delta) return;
    delta.textContent = metric ? comparisonText(metric) : '';
    delta.className = metric && number(metric.current) < number(metric.previous) ? 'negative' : 'positive';
  }

  function setText(id, value) {
    const element = document.getElementById(id);
    if (element) element.textContent = value;
  }

  function chart(id) {
    const element = document.getElementById(id);
    if (!element || !element.clientWidth || !element.clientHeight || element.closest('.admin-permission-hidden')) return null;
    if (!charts[id]) charts[id] = echarts.init(element);
    return charts[id];
  }

  function chartOption(id, option) {
    const instance = chart(id);
    if (!instance) return;
    instance.setOption(option, true);
    const legend = document.getElementById(id + 'Legend');
    if (legend && option.series[0].type === 'pie') {
      const rows = option.series[0].data;
      AdminChartStyle.legend(legend, rows, '', rows.reduce(function (sum, item) { return sum + number(item.value); }, 0), option.color);
    }
  }

  function axisTooltip() {
    return { trigger: 'axis', confine: true, renderMode: 'richText', backgroundColor: 'rgba(17,24,39,.92)', borderWidth: 0, textStyle: { color: '#fff' } };
  }

  function baseGrid(bottom) {
    return { left: 18, right: 18, top: 12, bottom: bottom || 26, containLabel: true };
  }

  function renderTrend(items) {
    items = items || [];
    chartOption('trendChart', {
      color: [colors[0], colors[1], colors[2]],
      tooltip: axisTooltip(),
      legend: { data: ['活跃用户', '新增用户', '新增轨迹'], bottom: 0, itemWidth: 14, itemHeight: 8 },
      grid: baseGrid(34),
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
    chartOption('funnelChart', AdminChartStyle.bars(values, registered, { unit: '人', showPercent: true, labelWidth: 78 }));
  }

  function pieOption(items, palette) {
    items = items || [];
    const total = items.reduce(function (sum, item) { return sum + number(item.value); }, 0);
    const option = AdminChartStyle.ring(items, total, '总计', '条');
    if (palette) option.color = palette;
    return option;
  }

  function distributionData(items) {
    return (items || []).map(function (item) { return { name: item.label || item.key || '未知', value: number(item.count) }; });
  }

  function renderBacklog(id, label, stats, oldest) {
    const element = document.getElementById(id);
    const over72 = number(stats?.pending_over_72h);
    const between24And72 = Math.max(0, number(stats?.pending_over_24h) - over72);
    element.hidden = !between24And72 && !over72 && !oldest;
    element.replaceChildren(label);
    function addAge(text, className) {
      const age = document.createElement('span');
      age.textContent = text;
      age.className = className;
      element.append(' · ', age);
    }
    if (between24And72) addAge('24–72 小时 ' + formatNumber(between24And72) + ' 条', 'backlog-warning');
    if (over72) addAge('超 72 小时 ' + formatNumber(over72) + ' 条', 'backlog-danger');
    if (oldest) element.append(' · 最早 ' + oldest);
  }

  function renderReviewMetrics(review, pendingSubmissions) {
    const pending = review?.pending ?? pendingSubmissions;
    const reviewed = review && number(review.approved) + number(review.rejected) > 0;
    const empty = review ? '暂无样本' : '—';
    setText('reviewPending', pending == null ? '—' : formatNumber(pending));
    setText('approvalRate', reviewed ? (number(review.approval_rate) * 100).toFixed(1) + '%' : empty);
    setText('avgReviewHours', reviewed ? formatDurationHours(review.avg_review_hours) : empty);
    renderBacklog('reviewBacklog', '投稿积压', review, AdminTime.dateTime(review?.oldest_pending, ''));
    document.getElementById('reviewBreakdown').hidden = !review;
  }

  function renderReviewChart(review) {
    review = review || {};
    chartOption('reviewChart', pieOption([
      { name: '通过', value: number(review.approved) },
      { name: '驳回', value: number(review.rejected) }
    ], ['#2fb344', '#d63939']));
  }

  function renderFeedbackMetrics(feedback) {
    setText('feedbackPending', feedback ? formatNumber(feedback.pending) : '—');
    setText('feedbackProcessing', feedback ? formatNumber(feedback.processing) : '—');
    setText('feedbackResolvedPeriod', feedback ? formatNumber(feedback.resolved_in_period) : '—');
    const resolved = feedback && number(feedback.resolved_in_period) > 0;
    const empty = feedback ? '暂无样本' : '—';
    setText('avgResolutionHours', resolved ? formatDurationHours(feedback.avg_resolution_hours) : empty);
    renderBacklog('feedbackBacklog', '反馈积压', feedback);
    document.getElementById('feedbackBreakdown').hidden = !feedback;
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
    const total = items.reduce(function (sum, item) { return sum + number(item.value); }, 0);
    return AdminChartStyle.bars(items, total, { unit: '条', labelWidth: 84 });
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
      legend: { data: ['收藏', '导航', '点赞', '评论'], bottom: 0, itemWidth: 14, itemHeight: 8 }, grid: baseGrid(34),
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
    body.replaceChildren();
    if (!items.length) {
      const cell = body.insertRow().insertCell();
      cell.colSpan = 6;
      cell.className = 'text-secondary text-center';
      cell.textContent = '暂无互动数据';
      return;
    }
    items.forEach(function (item) {
      const row = body.insertRow();
      const track = row.insertCell();
      const link = document.createElement('a');
      link.href = '/admin/tracks.html?track_id=' + encodeURIComponent(item.track_id);
      link.textContent = item.title || '未命名轨迹';
      const id = document.createElement('small');
      id.className = 'dashboard-track-id mono text-secondary';
      id.textContent = item.track_id;
      track.append(link, id);
      ['collections', 'navigations', 'likes', 'comments'].forEach(function (key) {
        row.insertCell().textContent = formatNumber(item[key]);
      });
      const score = document.createElement('strong');
      score.textContent = formatNumber(item.score);
      row.insertCell().appendChild(score);
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

  function dateRange(start, end) {
    const from = AdminTime.date(start);
    const to = AdminTime.date(end);
    return from === to ? from : from + ' 至 ' + to;
  }

  function renderDashboard(data) {
    const metrics = data.metrics || {};
    setMetric('activeUsers', metrics.active_users);
    setMetric('newUsers', metrics.new_users);
    setMetric('uploadedTracks', metrics.uploaded_tracks);
    setMetric('uploadUsers', metrics.upload_users);
    setMetric('submissions', metrics.submissions);
    setText('rangeHint', periodLabels[data.period] + ' · ' + dateRange(data.range.start, data.range.end));
    setText('updatedAt', '最近更新 ' + AdminTime.dateTime(data.updated_at) + '（北京时间）');
    const trend = data.trend || [];
    const trendRange = trend.length ? dateRange(trend[0].date, trend[trend.length - 1].date) : '';
    setText('trendRangeHint', trendRange);
    setText('engagementRangeHint', trendRange ? '每日趋势 · ' + trendRange : '');
    renderReviewMetrics(data.review, metrics.pending_submissions);
    renderFeedbackMetrics(data.feedback);
    renderContentMetrics(data.content);
    renderEngagementMetrics(data.engagement);
    renderCompanion(data.companion);
    periodButtons.forEach(function (button) {
      const active = button.dataset.period === data.period;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
  }

  function showChartError() {
    const error = document.getElementById('dashboardChartError');
    error.textContent = '图表加载失败，请刷新重试';
    error.classList.remove('hidden');
  }

  async function renderDashboardCharts(data) {
    document.getElementById('dashboardChartError').classList.add('hidden');
    try {
      await AdminAssets.loadCharts();
      if (data !== latestData) return;
      renderTrend(data.trend);
      if (data.funnel) renderFunnel(data.funnel);
      if (data.review && document.getElementById('reviewBreakdown').open) renderReviewChart(data.review);
      if (data.feedback && document.getElementById('feedbackBreakdown').open) renderFeedbackCharts(data.feedback);
      if (data.content) renderContentCharts(data.content);
      if (data.engagement) renderEngagementChart(data.engagement, data.trend);
    } catch (_) {
      if (data === latestData) showChartError();
    }
  }

  async function loadDashboard(period = requestedPeriod) {
    requestedPeriod = period;
    if (dashboardRequest) dashboardRequest.abort();
    const controller = new AbortController();
    dashboardRequest = controller;
    const status = document.getElementById('dashboardStatus');
    const error = document.getElementById('dashboardError');
    status.textContent = '正在读取' + periodLabels[period] + '数据';
    status.hidden = false;
    error.classList.add('hidden');
    try {
      const response = await fetch('/admin/api/dashboard?period=' + encodeURIComponent(period), { signal: controller.signal });
      if (controller.signal.aborted) return;
      if (response.status === 401) {
        window.location.href = '/admin/login.html';
        return;
      }
      const payload = await response.json();
      if (controller.signal.aborted) return;
      const data = payload.data;
      if (!response.ok || !data?.range || !Object.hasOwn(periodLabels, data.period)) throw new Error('统计数据暂不可用');
      renderDashboard(data);
      latestData = data;
      renderDashboardCharts(data);
    } catch (_) {
      if (controller.signal.aborted) return;
      const displayed = latestData ? '，当前显示' + periodLabels[latestData.period] + '数据' : '，请刷新重试';
      error.textContent = periodLabels[period] + '数据读取失败' + displayed;
      error.classList.remove('hidden');
    } finally {
      if (dashboardRequest === controller) {
        dashboardRequest = null;
        status.hidden = true;
      }
    }
  }

  periodButtons.forEach(function (button) {
    button.addEventListener('click', function () {
      loadDashboard(button.dataset.period);
    });
  });
  ['reviewBreakdown', 'feedbackBreakdown'].forEach(function (id) {
    const details = document.getElementById(id);
    AdminStats.onExpand(details, async function () {
      const data = latestData;
      if (!data) return;
      try {
        await AdminAssets.loadCharts();
        if (!details.open || data !== latestData) return;
        if (id === 'reviewBreakdown') renderReviewChart(data.review); else renderFeedbackCharts(data.feedback);
        Object.keys(charts).forEach(function (key) { charts[key].resize(); });
      } catch (_) {
        if (details.open && data === latestData) showChartError();
      }
    });
  });
  document.getElementById('refreshDashboard').addEventListener('click', function () {
    loadDashboard();
    loadAnalyticsHealth();
  });
  window.addEventListener('pagehide', function () {
    if (dashboardRequest) dashboardRequest.abort();
    if (healthRequest) { const request = healthRequest; healthRequest = null; request.abort(); }
  });
  window.addEventListener('pageshow', function (event) {
    if (event.persisted) {
      loadDashboard();
      loadAnalyticsHealth();
    }
  });
  window.addEventListener('resize', function () { Object.keys(charts).forEach(function (key) { charts[key].resize(); }); });
  const canReview = AdminAccess.can('track_submissions', false);
  const canReadFeedback = AdminAccess.can('feedbacks', false);
  document.getElementById('dashboardTodos').hidden = !canReview && !canReadFeedback;
  document.getElementById('dashboardEfficiency').hidden = !canReview && !canReadFeedback;
  document.getElementById('dashboardGrowth').hidden = !canReview && !AdminAccess.can('users', false) && !AdminAccess.can('tracks', false);
  loadDashboard();
  loadAnalyticsHealth();
})();
