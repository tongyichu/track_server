(function (mountDashboard) {
  const workbench = document.getElementById('dashboard-workbench');
  if (workbench) workbench.addEventListener('admin:dashboard-ready', mountDashboard, { once: true });
  else mountDashboard();
})(function () {
  const periodLabels = { day: '今日', week: '本周', month: '本月' };
  const periodButtons = document.querySelectorAll('[data-period]');
  let requestedPeriod = 'day';
  let dashboardRequest = null;
  const charts = {};
  const colors = AdminChartStyle.colors;
  let latestData = null;
  let healthRequest = null;
  const tableRequests = new Map();
  const cityPanel = document.querySelector('[data-workbench-table="cities"]');
  const cityPager = cityPanel.querySelector('[data-city-pagination]');
  const cityPageButtons = cityPager.querySelectorAll('[data-city-page-step]');
  let cityPage = null;

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
      legend: { data: ['登录用户', '新增用户', '新增云端轨迹'], bottom: 0, itemWidth: 14, itemHeight: 8 },
      grid: baseGrid(34),
      xAxis: { type: 'category', boundaryGap: false, data: items.map(function (item) { return item.date.slice(5); }), axisLine: { lineStyle: { color: '#dce1e8' } }, axisTick: { show: false } },
      yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#edf0f5' } } },
      series: [
        { name: '登录用户', type: 'line', smooth: true, symbol: 'none', lineStyle: { width: 3 }, areaStyle: { opacity: .07 }, data: items.map(function (item) { return item.active_users; }) },
        { name: '新增用户', type: 'line', smooth: true, symbol: 'none', lineStyle: { width: 2 }, data: items.map(function (item) { return item.new_users; }) },
        { name: '新增云端轨迹', type: 'line', smooth: true, symbol: 'none', lineStyle: { width: 2 }, data: items.map(function (item) { return item.uploaded_tracks; }) }
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

  function renderReviewMetrics(review) {
    const reviewed = review && number(review.approved) + number(review.rejected) > 0;
    const empty = review ? '暂无样本' : '—';
    setText('approvalRate', reviewed ? (number(review.approval_rate) * 100).toFixed(1) + '%' : empty);
    setText('avgReviewHours', reviewed ? formatDurationHours(review.avg_review_hours) : empty);
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
    setText('feedbackResolvedPeriod', feedback ? formatNumber(feedback.resolved_in_period) : '—');
    const resolved = feedback && number(feedback.resolved_in_period) > 0;
    const empty = feedback ? '暂无样本' : '—';
    setText('avgResolutionHours', resolved ? formatDurationHours(feedback.avg_resolution_hours) : empty);
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
    (feedback.platforms || []).forEach(function (item) { source.push({ name: item.label || item.key, value: number(item.count) }); });
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
      cell.colSpan = 5;
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
      ['navigations', 'collections', 'likes', 'comments'].forEach(function (key) {
        row.insertCell().textContent = formatNumber(item[key]);
      });
      Array.from(row.cells).forEach(function (cell, index) {
        cell.dataset.label = body.parentElement.tHead.rows[0].cells[index].textContent;
      });
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

  function tableNumber(value) {
    return value == null ? '—' : formatNumber(value);
  }

  function cohortRate(count, total) {
    if (!total) return '—';
    return count == null ? '待观察' : formatNumber(count * 100 / total, 1) + '%';
  }

  const backlogEntries = {
    review: ['投稿待审核', '/admin/track_submissions.html?status=pending', '处理投稿'],
    feedback: ['反馈待处理或处理中', '/admin/feedbacks.html', '查看反馈'],
    index: ['待建地图索引', '/admin/route_groups.html', '查看路线组'],
    index_failed: ['地图索引失败', '/admin/route_groups.html', '查看路线组'],
    deletion_records: ['注销待清理资料'],
    deletion_files: ['注销待清理文件']
  };
  const todoSummary = document.getElementById('dashboardTodoSummary');
  const todoStatus = todoSummary.querySelector('[data-todo-status]');
  const todoLinks = todoSummary.querySelector('[data-todo-links]');
  const todoCounts = new Map();
  [
    ['review', 'track_submissions', '投稿待审核'],
    ['feedback', 'feedbacks', '未结反馈']
  ].forEach(function ([key, permission, label]) {
    const link = document.createElement('a');
    link.href = backlogEntries[key][1];
    link.dataset.adminRead = permission;
    const name = document.createElement('span'), count = document.createElement('strong');
    name.textContent = label;
    count.textContent = '—';
    link.append(name, count);
    todoLinks.append(link);
    todoCounts.set(key, count);
  });
  AdminAccess.apply();

  function renderTodoCounts(rows) {
    todoCounts.forEach(function (node, key) {
      node.textContent = tableNumber(rows.find(function (row) { return row.key === key; })?.open);
    });
  }

  const managementRows = {
    cities: function (item) {
      const delta = item.navigations - item.previous_navigations;
      return [item.name, tableNumber(item.supply), tableNumber(item.added), tableNumber(item.collectors),
        tableNumber(item.navigations), (delta > 0 ? '+' : '') + formatNumber(delta)];
    },
    cohorts: function (item) {
      return [item.week + ' 起', tableNumber(item.registered),
        cohortRate(item.first_track, item.registered), cohortRate(item.day_one_login, item.registered),
        cohortRate(item.day_seven_login, item.registered)];
    },
    versions: function (item) {
      return [item.version, item.release_recorded_at ? AdminTime.date(item.release_recorded_at) : '—',
        tableNumber(item.devices), item.share == null ? '—' : formatNumber(item.share * 100, 1) + '%',
        tableNumber(item.feedback)];
    },
    companions: function (item) {
      return [item.name, tableNumber(item.rooms), tableNumber(item.members), tableNumber(item.ready),
        tableNumber(item.pending), tableNumber(item.empty), tableNumber(item.unavailable)];
    },
    backlog: function (item, data) {
      const entry = backlogEntries[item.key];
      if (!entry) return null;
      let action = '—';
      if (entry[1]) {
        action = document.createElement('a');
        action.href = entry[1];
        action.textContent = entry[2];
      }
      const waiting = item.oldest && (Date.parse(data.range.end) - Date.parse(item.oldest)) / 3600000;
      return [entry[0], tableNumber(item.open), tableNumber(item.overdue),
        item.oldest ? formatDurationHours(Math.max(0, waiting)) : '—', action];
    }
  };

  function replaceManagementRows(panel, data) {
    const head = panel.querySelector('thead');
    const body = panel.querySelector('tbody');
    const headers = Array.from(head.rows[0].cells, function (cell) { return cell.textContent; });
    const fragment = document.createDocumentFragment();
    const format = managementRows[panel.dataset.workbenchTable];
    data.rows.forEach(function (item) {
      const values = format(item, data);
      if (!values) return;
      const row = document.createElement('tr');
      values.forEach(function (value, index) {
        const cell = row.insertCell();
        cell.dataset.label = headers[index];
        if (value instanceof Node) cell.append(value);
        else cell.textContent = value;
      });
      fragment.append(row);
    });
    if (!fragment.childElementCount) {
      const row = document.createElement('tr');
      const cell = row.insertCell();
      cell.colSpan = headers.length;
      cell.textContent = '暂无数据';
      fragment.append(row);
    }
    body.replaceChildren(fragment);
  }

  function updateCityPager() {
    if (!cityPage) return;
    cityPager.hidden = false;
    const multiplePages = cityPage.pages > 1;
    const total = '共 ' + formatNumber(cityPage.total) + ' 个城市';
    cityPager.querySelector('[data-city-page-status]').textContent =
      multiplePages ? total + ' · 第 ' + cityPage.page + ' / ' + cityPage.pages + ' 页' : total;
    cityPager.querySelector('[data-city-page-actions]').hidden = !multiplePages;
    cityPageButtons.forEach(function (button) {
      const target = cityPage.page + Number(button.dataset.cityPageStep);
      button.disabled = tableRequests.has(cityPanel) || target < 1 || target > cityPage.pages;
    });
  }

  async function loadManagementTable(panel, period, page = 1) {
    const key = panel.dataset.workbenchTable;
    const managesTodo = key === 'backlog';
    tableRequests.get(panel)?.abort();
    const controller = new AbortController();
    tableRequests.set(panel, controller);
    if (panel === cityPanel) updateCityPager();
    const status = panel.querySelector('[data-table-status]');
    const previous = panel.dataset.loadedPeriod;
    status.textContent = '正在读取…';
    if (managesTodo && !previous) todoStatus.textContent = '正在读取…';
    panel.setAttribute('aria-busy', 'true');
    try {
      const params = new URLSearchParams({ table: key, period: period });
      if (panel === cityPanel) params.set('page', page);
      const response = await fetch('/admin/api/dashboard/tables?' + params, { signal: controller.signal });
      if (controller.signal.aborted) return;
      if (response.status === 403 || (managesTodo && response.status === 401)) {
        panel.closest('section').hidden = true;
        if (managesTodo) {
          todoSummary.hidden = true;
          renderTodoCounts([]);
          todoStatus.textContent = '';
        }
        return;
      }
      if (!response.ok) throw new Error('table unavailable');
      const payload = await response.json();
      if (controller.signal.aborted) return;
      const data = payload.data;
      if (!data?.range || !Array.isArray(data.rows)) throw new Error('invalid table');
      if (panel === cityPanel && !data.pagination) throw new Error('invalid city pagination');
      replaceManagementRows(panel, data);
      panel.dataset.loadedPeriod = data.period;
      if (panel === cityPanel) cityPage = data.pagination;
      if (managesTodo) {
        renderTodoCounts(data.rows);
        todoStatus.textContent = '';
      }
      status.textContent = panel.hasAttribute('data-workbench-period') ? dateRange(data.range.start, data.range.end) : '';
    } catch (_) {
      if (controller.signal.aborted) return;
      const previousLabel = panel.hasAttribute('data-workbench-period') ? periodLabels[previous] : '上次';
      status.textContent = previous ? '读取失败，保留' + previousLabel + '结果。' : '读取失败，请刷新重试。';
      if (managesTodo) todoStatus.textContent = previous ? '刷新失败，显示上次数据' : '读取失败，请刷新重试';
      if (!previous) {
        const cell = panel.querySelector('tbody td');
        if (cell) cell.textContent = '数据暂不可用';
      }
    } finally {
      if (tableRequests.get(panel) === controller) {
        tableRequests.delete(panel);
        panel.removeAttribute('aria-busy');
        if (panel === cityPanel) updateCityPager();
      }
    }
  }

  function loadManagementTables(period, includeFixed) {
    document.querySelectorAll('[data-workbench-table]').forEach(function (panel) {
      const section = panel.closest('section');
      if (!section.hidden && !section.classList.contains('admin-permission-hidden') &&
          (includeFixed || panel.hasAttribute('data-workbench-period'))) {
        const page = panel === cityPanel && panel.dataset.loadedPeriod === period ? cityPage?.page : undefined;
        loadManagementTable(panel, period, page);
      }
    });
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
    renderReviewMetrics(data.review);
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

  cityPageButtons.forEach(function (button) {
    button.addEventListener('click', function () {
      const page = cityPage.page + Number(button.dataset.cityPageStep);
      loadManagementTable(cityPanel, cityPanel.dataset.loadedPeriod, page);
    });
  });

  periodButtons.forEach(function (button) {
    button.addEventListener('click', function () {
      loadDashboard(button.dataset.period);
      loadManagementTables(button.dataset.period, false);
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
    loadManagementTables(requestedPeriod, true);
    loadAnalyticsHealth();
  });
  window.addEventListener('pagehide', function () {
    if (dashboardRequest) dashboardRequest.abort();
    tableRequests.forEach(function (request) { request.abort(); });
    if (healthRequest) { const request = healthRequest; healthRequest = null; request.abort(); }
  });
  window.addEventListener('pageshow', function (event) {
    if (event.persisted) {
      loadDashboard();
      loadManagementTables(requestedPeriod, true);
      loadAnalyticsHealth();
    }
  });
  window.addEventListener('resize', function () { Object.keys(charts).forEach(function (key) { charts[key].resize(); }); });
  const canReview = AdminAccess.can('track_submissions', false);
  const canReadFeedback = AdminAccess.can('feedbacks', false);
  todoSummary.hidden = !canReview && !canReadFeedback;
  document.getElementById('backlogSection').hidden = ![
    'track_submissions', 'feedbacks', 'route_groups', 'users'
  ].some(function (module) { return AdminAccess.can(module, false); });
  document.getElementById('dashboardEfficiency').hidden = !canReview && !canReadFeedback;
  document.getElementById('dashboardGrowth').hidden = !canReview && !AdminAccess.can('users', false) && !AdminAccess.can('tracks', false);
  loadDashboard();
  loadManagementTables(requestedPeriod, true);
  loadAnalyticsHealth();
});
