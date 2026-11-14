(function () {
  'use strict';
  const colors = ['#206bc4', '#2fb344', '#f59f00', '#ae3ec9', '#d63939', '#17a2b8', '#6f42c1', '#74b816', '#f76707', '#0ca678', '#4263eb', '#e64980', '#868e96'];
  function number(value) { return Number(value || 0); }
  function compact(value) {
    const count = number(value);
    if (count < 10000) return count.toLocaleString();
    const scaled = count / 10000;
    return (scaled >= 10 ? scaled.toFixed(0) : scaled.toFixed(1)).replace(/\.0$/, '') + 'w';
  }
  function ring(rows, total, caption, unit) {
    return {
      color: colors, aria: { enabled: true },
      tooltip: { trigger: 'item', confine: true, renderMode: 'richText', formatter: item => item.name + '\n' + number(item.value).toLocaleString() + ' ' + unit + '（' + item.percent + '%）' },
      series: [{ type: 'pie', radius: ['57%', '82%'], center: ['50%', '50%'], avoidLabelOverlap: true,
        itemStyle: { borderColor: '#fff', borderWidth: 2 }, label: { show: false },
        emphasis: { scale: true, scaleSize: 6 }, data: rows }],
      graphic: [
        { type: 'text', left: 'center', top: '41%', style: { text: number(total).toLocaleString(), fill: '#182433', fontSize: 22, fontWeight: 700, textAlign: 'center' } },
        { type: 'text', left: 'center', top: '55%', style: { text: caption, fill: '#667085', fontSize: 11, textAlign: 'center' } }
      ]
    };
  }
  function bars(rows, total, settings) {
    settings = settings || {};
    const percent = value => number(total) > 0 ? (number(value) * 100 / number(total)).toFixed(1) : '0.0';
    const suffix = value => settings.showPercent ? ' · ' + percent(value) + '%' : ' ' + settings.unit;
    const axisLabel = { color: '#667085', fontSize: 11 };
    if (settings.labelWidth) { axisLabel.width = settings.labelWidth; axisLabel.overflow = 'truncate'; }
    return {
      aria: { enabled: true },
      grid: { left: 0, right: settings.showPercent ? 82 : 52, top: 12, bottom: 8, containLabel: true },
      tooltip: { trigger: 'item', confine: true, renderMode: 'richText', formatter: settings.tooltip || (item => item.name + '\n' + number(item.value).toLocaleString() + ' ' + settings.unit + (settings.showPercent ? ' · ' + percent(item.value) + '%' : '')) },
      xAxis: { type: 'value', show: false, max: value => Math.max(1, value.max) },
      yAxis: { type: 'category', inverse: true, data: rows.map(item => item.name), axisLine: { show: false }, axisTick: { show: false }, axisLabel: axisLabel },
      series: [{ type: 'bar', data: rows, barMaxWidth: 12, showBackground: true,
        backgroundStyle: { color: '#f1f5f9', borderRadius: 4 }, itemStyle: { color: colors[0], borderRadius: 4 },
        label: { show: true, position: 'right', color: '#475569', fontSize: 10, formatter: item => compact(item.value) + suffix(item.value) } }],
      graphic: rows.length ? [] : [{ type: 'text', left: 'center', top: 'middle', style: { text: '当前周期暂无数据', fill: '#667085', fontSize: 12 } }]
    };
  }
  function legend(root, rows, totalLabel, total, palette) {
    palette = palette || colors;
    root.replaceChildren();
    function append(label, value, color, isTotal) {
      const row = document.createElement('div'), name = document.createElement('span'), count = document.createElement('strong');
      row.className = 'admin-user-legend-row' + (isTotal ? ' admin-user-legend-total' : '');
      name.className = 'admin-user-legend-label';
      if (color) { const dot = document.createElement('i'); dot.style.background = color; name.appendChild(dot); }
      name.appendChild(document.createTextNode(label)); count.textContent = number(value).toLocaleString();
      row.append(name, count); root.appendChild(row);
    }
    rows.forEach((item, index) => append(item.name, item.value, palette[index % palette.length], false));
    if (totalLabel) append(totalLabel, total, '', true);
    else if (!rows.length) { const empty = document.createElement('span'); empty.className = 'hint'; empty.textContent = '暂无数据'; root.appendChild(empty); }
  }
  window.AdminChartStyle = { colors: colors.slice(), ring: ring, bars: bars, legend: legend };
})();
