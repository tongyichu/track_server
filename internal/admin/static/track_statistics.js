(function () {
  'use strict';
  const colors = ['#16a34a', '#2563eb', '#ea580c', '#9333ea', '#0f766e', '#64748b'];
  const typeColors = { hiking: colors[0], running: colors[1], climbing: colors[2], riding: colors[3], driving: colors[4] };
  const periodNames = { day: '今天', week: '本周', month: '本月' };
  const pending = new Map(), cards = [];
  function number(value) { return Number(value || 0).toLocaleString('zh-CN'); }
  function date(value) {
    return new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
  }
  function read(period) {
    if (!pending.has(period)) {
      const controller = new AbortController();
      const promise = fetch('/admin/api/tracks/statistics?period=' + encodeURIComponent(period), { signal: controller.signal, cache: 'no-store' })
        .then(async response => {
          if (response.status === 401) { window.location.href = '/admin/login.html'; throw new Error('登录已失效'); }
          const payload = await response.json();
          if (!response.ok) throw new Error(payload.error || '统计加载失败');
          return payload.data;
        }).finally(() => pending.delete(period));
      pending.set(period, { controller: controller, promise: promise });
    }
    return pending.get(period).promise;
  }
  function option(kind, items) {
    const base = {
      color: colors, aria: { enabled: true },
      tooltip: { trigger: 'item', confine: true, renderMode: 'richText' },
      graphic: items.length ? [] : [{ type: 'text', left: 'center', top: 'middle', style: { text: '当前周期暂无数据', fill: '#667085', fontSize: 14 } }]
    };
    if (kind === 'types') {
      base.tooltip.formatter = item => item.name + '\n' + number(item.value) + ' 条 · ' + item.percent + '%';
      base.legend = { type: 'scroll', bottom: 4, left: 'center', textStyle: { color: '#667085' } };
      base.series = [{ type: 'pie', radius: ['0%', '66%'], center: ['50%', '44%'],
        label: { show: false }, emphasis: { label: { show: true, formatter: '{b}\n{d}%', fontWeight: 600 } },
        data: items.map(item => ({ name: item.label || item.key || '未知类型', value: item.count, itemStyle: { color: typeColors[item.key] || colors[5] } }))
      }];
    } else {
      base.tooltip.formatter = item => {
        const user = items[item.dataIndex];
        return (user.nickname || '未命名用户') + '\n用户 ID：' + user.user_id + '\n贡献轨迹：' + number(user.count) + ' 条';
      };
      base.grid = { left: 12, right: 52, top: 12, bottom: 28, containLabel: true };
      base.xAxis = { type: 'value', min: 0, minInterval: 1, axisLabel: { color: '#667085' }, splitLine: { lineStyle: { color: '#eef1f5' } } };
      base.yAxis = { type: 'category', inverse: true,
        data: items.map((item, index) => (index + 1) + '. ' + (item.nickname || '未命名用户')),
        axisTick: { show: false }, axisLine: { show: false },
        axisLabel: { color: '#475467', width: 100, overflow: 'truncate' }
      };
      base.series = [{ type: 'bar', barMaxWidth: 20,
        itemStyle: { color: '#2563eb', borderRadius: [0, 4, 4, 0] },
        label: { show: true, position: 'right', color: '#475467', formatter: item => number(item.value) },
        data: items.map(item => item.count)
      }];
    }
    return base;
  }
  document.querySelectorAll('[data-track-stat]').forEach(element => {
    const kind = element.dataset.trackStat, buttons = Array.from(element.querySelectorAll('[data-stat-period]'));
    const status = element.querySelector('[data-stat-status]'), error = element.querySelector('[data-stat-error]');
    const refresh = element.querySelector('[data-stat-refresh]'), chartNode = element.querySelector('.track-stat-chart');
    let period = 'month', loadedPeriod = 'month', request = 0, chart;
    function select() {
      buttons.forEach(button => {
        const active = button.dataset.statPeriod === period;
        button.classList.toggle('active', active);
        button.setAttribute('aria-pressed', String(active));
      });
    }
    let lastStatus = '';
    async function load() {
      const current = ++request, requestedPeriod = period;
      error.hidden = true; refresh.disabled = true; element.setAttribute('aria-busy', 'true');
      status.textContent = periodNames[period] + '统计加载中…';
      try {
        const data = await read(requestedPeriod);
        if (current !== request) return;
        await AdminAssets.loadCharts();
        if (current !== request) return;
        const items = data[kind] || [];
        if (!chart) chart = echarts.init(chartNode);
        chart.setOption(option(kind, items), true);
        loadedPeriod = requestedPeriod;
        lastStatus = periodNames[loadedPeriod] + ' · ' + date(data.start) + ' — ' + date(data.end);
        status.textContent = lastStatus;
        chartNode.setAttribute('aria-label', (kind === 'types' ? '轨迹类型：' : '用户贡献轨迹量前十：') +
          (items.length ? items.map(item => (item.label || item.nickname || item.key || '未命名用户') + ' ' + number(item.count) + ' 条').join('，') : '当前周期暂无数据'));
      } catch (failure) {
        if (current !== request || failure.name === 'AbortError') return;
        period = loadedPeriod; select();
        status.textContent = lastStatus || '统计暂不可用';
        error.textContent = '统计加载失败，请点击刷新重试：' + failure.message;
        error.hidden = false;
      } finally {
        if (current === request) { refresh.disabled = false; element.setAttribute('aria-busy', 'false'); }
      }
    }
    buttons.forEach(button => button.addEventListener('click', () => {
      if (button.dataset.statPeriod === period) return;
      period = button.dataset.statPeriod; select(); load();
    }));
    refresh.addEventListener('click', load);
    const observer = new ResizeObserver(() => { if (chart) chart.resize(); });
    observer.observe(chartNode);
    cards.push({ load: load, dispose: () => { ++request; observer.disconnect(); if (chart) chart.dispose(); } });
    load();
  });
  const refreshPage = document.getElementById('refreshBtn');
  if (refreshPage) refreshPage.addEventListener('click', () => cards.forEach(card => card.load()));
  window.addEventListener('pagehide', event => {
    if (event.persisted) return;
    pending.forEach(item => item.controller.abort());
    cards.forEach(card => card.dispose());
  });
})();
