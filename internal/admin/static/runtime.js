(function () {
  'use strict';
  const timeZone = 'Asia/Shanghai';
  const timeFormatter = new Intl.DateTimeFormat('zh-CN', {
    timeZone, year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23'
  });
  function timeParts(value) {
    // Instants must carry an offset; calendar dates and form inputs have separate paths.
    if (value == null || value === '' || typeof value === 'string' &&
        (value.startsWith('0001-') || !/(?:Z|[+-]\d{2}:\d{2})$/i.test(value))) return null;
    const date = new Date(value);
    if (!Number.isFinite(date.getTime())) return null;
    const parts = {};
    timeFormatter.formatToParts(date).forEach(part => { parts[part.type] = part.value; });
    return parts;
  }
  const datePart = parts => parts.year + '-' + parts.month + '-' + parts.day;
  const clockPart = parts => parts.hour + ':' + parts.minute + ':' + parts.second;
  function dateTime(value, fallback = '—') {
    const parts = timeParts(value);
    return parts ? datePart(parts) + ' ' + clockPart(parts) : fallback;
  }
  function calendarDate(value, fallback = '—') {
    if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
    const parts = timeParts(value);
    return parts ? datePart(parts) : fallback;
  }
  function clock(value, fallback = '—') {
    const parts = timeParts(value);
    return parts ? clockPart(parts) : fallback;
  }
  function shortDate(value, withTime, fallback = '—') {
    if (!withTime && typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)) return value.slice(5);
    const parts = timeParts(value);
    return parts ? parts.month + '-' + parts.day + (withTime ? ' ' + parts.hour + ':' + parts.minute : '') : fallback;
  }
  function toInput(value) {
    const parts = timeParts(value);
    return parts ? datePart(parts) + 'T' + parts.hour + ':' + parts.minute : '';
  }
  function fromInput(value) {
    if (!value) return '';
    const match = /^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})(?::(\d{2})(?:\.(\d{1,3}))?)?$/.exec(value);
    if (!match) throw new Error('请输入有效的北京时间');
    const date = new Date(value + '+08:00');
    const canonical = match[1] + ' ' + match[2] + ':' + (match[3] || '00');
    if (dateTime(date, '') !== canonical) throw new Error('请输入有效的北京时间');
    return date.toISOString();
  }
  function shiftDate(value, days) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || !Number.isInteger(days)) return '';
    // UTC is only a calendar arithmetic anchor here, never a display conversion.
    const date = new Date(value + 'T00:00:00Z');
    if (!Number.isFinite(date.getTime()) || date.toISOString().slice(0, 10) !== value) return '';
    date.setUTCDate(date.getUTCDate() + days);
    return date.toISOString().slice(0, 10);
  }
  window.AdminTime = Object.freeze({ timeZone, dateTime, date: calendarDate, time: clock, short: shortDate, toInput, fromInput, shiftDate });
  const version = document.querySelector('meta[name="admin-assets-version"]');
  let chartsPromise;
  function url(path) {
    const result = new URL(path, location.origin);
    if (version && result.origin === location.origin && result.pathname.startsWith('/admin/static/')) {
      result.searchParams.set('v', version.content);
    }
    return result.href;
  }
  function loadCharts() {
    if (window.echarts) return Promise.resolve();
    if (!chartsPromise) {
      chartsPromise = new Promise(function (resolve, reject) {
        const script = document.createElement('script');
        script.src = url('/admin/static/vendor/echarts.min.js');
        script.async = true;
        script.onload = function () {
          if (window.echarts) resolve();
          else { script.remove(); chartsPromise = null; reject(new Error('图表组件暂不可用')); }
        };
        script.onerror = function () {
          script.remove(); chartsPromise = null;
          reject(new Error('图表库加载失败，请重试'));
        };
        document.head.appendChild(script);
      });
    }
    return chartsPromise;
  }
  window.AdminAssets = { url: url, loadCharts: loadCharts };
})();
