(function () {
  'use strict';
  let geometryPromise;
  const charts = new Map(), maps = [];
  function esc(value) { return String(value == null ? '' : value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;'); }
  function date(value, hourly) {
    return AdminTime.short(value, hourly);
  }
  function chart(id) {
    if (!window.echarts) throw new Error('图表组件暂不可用');
    if (!charts.has(id)) charts.set(id, echarts.init(document.getElementById(id)));
    return charts.get(id);
  }
  function loading(id, active) {
    document.getElementById(id).setAttribute('aria-busy', String(active));
    if (!charts.has(id)) return;
    if (active) charts.get(id).showLoading({ text: '加载中…', color: '#206bc4' });
    else charts.get(id).hideLoading();
  }
  function bars(id, rows, unit) {
    const total = rows.reduce((sum, row) => sum + Number(row.count || 0), 0);
    chart(id).setOption({
      aria: { enabled: true }, grid: { left: 6, right: 90, top: 20, bottom: 12, containLabel: true },
      tooltip: { trigger: 'item', confine: true, formatter: item => esc(item.name) + '<br>' + item.value.toLocaleString() + ' ' + esc(unit) + ' · ' + (total ? item.value * 100 / total : 0).toFixed(1) + '%' },
      xAxis: { type: 'value', show: false, max: value => Math.max(1, value.max) },
      yAxis: { type: 'category', inverse: true, data: rows.map(row => row.label || row.key), axisTick: { show: false }, axisLine: { show: false }, axisLabel: { color: '#667085', fontSize: 12 } },
      series: [{ type: 'bar', barMaxWidth: 16, showBackground: true, backgroundStyle: { color: '#f1f5f9', borderRadius: 4 }, itemStyle: { borderRadius: 4 }, data: rows.map(row => ({ value: Number(row.count || 0), itemStyle: { color: row.key === 'unknown' || row.key === 'zero' ? '#94a3b8' : '#206bc4' } })), label: { show: true, position: 'right', color: '#475569', fontSize: 11, formatter: item => item.value.toLocaleString() + ' · ' + (total ? item.value * 100 / total : 0).toFixed(1) + '%' } }]
    }, true);
  }
  function trend(id, items, series, hourly, unit) {
    chart(id).setOption({
      aria: { enabled: true }, animationDurationUpdate: 180,
      legend: { show: series.length > 1, top: 0, data: series.map(item => item.name), textStyle: { color: '#667085', fontSize: 11 } },
      grid: { left: 8, right: 18, top: 38, bottom: 12, containLabel: true },
      tooltip: { trigger: 'axis', confine: true, formatter: values => {
        if (!values.length) return '';
        return esc(date(items[values[0].dataIndex].at, hourly)) + values.map(value => '<br>' + value.marker + esc(value.seriesName) + ' ' + Number(value.value).toLocaleString() + ' ' + esc(unit)).join('');
      } },
      xAxis: { type: 'category', boundaryGap: false, data: items.map(item => item.label), axisLabel: { color: '#667085', fontSize: 11, hideOverlap: true }, axisLine: { lineStyle: { color: '#dce3eb' } }, axisTick: { show: false } },
      yAxis: { type: 'value', name: unit, min: 0, minInterval: 1, nameTextStyle: { color: '#667085', fontSize: 11 }, axisLabel: { color: '#667085', fontSize: 11 }, splitLine: { lineStyle: { color: '#eef1f5' } } },
      series: series.map(item => ({ name: item.name, type: 'line', data: items.map(point => Number(point[item.key] || 0)), showSymbol: items.length <= 7, symbolSize: 6, smooth: false, lineStyle: { color: item.color, width: 2 }, itemStyle: { color: item.color } }))
    }, true);
  }
  function loadGeometry() {
    if (!geometryPromise) geometryPromise = fetch(AdminAssets.url('/admin/static/china_provinces.json')).then(response => {
      if (!response.ok) throw new Error('地图底图加载失败');
      return response.json();
    }).then(geo => { echarts.registerMap('admin-spatial-china', { type: 'FeatureCollection', features: geo.features.filter(feature => String(feature.properties.adcode) !== '100000_JD') }); }).catch(error => { geometryPromise = null; throw error; });
    return geometryPromise;
  }
  function spatialGeo(maxZoom) {
    return { map: 'admin-spatial-china', roam: true, zoom: 1, center: [104.5, 35.5], boundingCoords: [[73, 54], [136, 17]], layoutCenter: ['50%', '50%'], layoutSize: '100%', scaleLimit: { min: 1, max: maxZoom }, label: { show: false }, itemStyle: { areaColor: '#edf3fa', borderColor: '#fff', borderWidth: 1 }, emphasis: { label: { show: true, color: '#475569', fontSize: 11 }, itemStyle: { areaColor: '#dbeafe' } } };
  }
  function createSpatialMap(options) {
    const el = key => document.getElementById(options[key]);
    let instance, points = [], unlocated = 0, frame = 0, version = 0, open = function () {};
    function compact(n) { return n >= 10000 ? (n / 10000).toFixed(1).replace(/\.0$/, '') + 'w' : n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n); }
    function schedule() { if (frame) cancelAnimationFrame(frame); frame = requestAnimationFrame(() => { frame = 0; draw(); }); }
    function draw() {
      if (!instance) return;
      const cells = new Map();
      let visible = 0;
      points.forEach(point => {
        const pixel = instance.convertToPixel({ geoIndex: 0 }, [point.longitude, point.latitude]);
        if (!pixel || !Number.isFinite(pixel[0]) || !Number.isFinite(pixel[1]) || pixel[0] < 0 || pixel[0] > instance.getWidth() || pixel[1] < 0 || pixel[1] > instance.getHeight()) return;
        const key = Math.floor(pixel[0] / 32) + ':' + Math.floor(pixel[1] / 32);
        if (!cells.has(key)) cells.set(key, []);
        cells.get(key).push(point); visible++;
      });
      instance.setOption({ series: [{ id: 'spatial-points', data: Array.from(cells.values()).map(members => ({ name: members.length === 1 ? members[0].name : members.length + ' ' + options.unit, value: [members.reduce((n, p) => n + p.longitude, 0) / members.length, members.reduce((n, p) => n + p.latitude, 0) / members.length, members.length], members })) }] });
      el('summary').textContent = '当前视野 ' + visible.toLocaleString() + ' / 已定位 ' + points.length.toLocaleString() + ' ' + options.unit + ' · 缺坐标 ' + unlocated.toLocaleString();
    }
    function initialize() {
      instance = echarts.init(el('chart'));
      instance.setOption({
        animation: false, aria: { enabled: true },
        tooltip: { trigger: 'item', confine: true, formatter: item => {
          const members = item.data && item.data.members;
          if (!members) return '';
          if (members.length === 1) return esc(members[0].name || '未命名') + (members[0].city ? '<br>' + esc(members[0].city) : '') + '<br>点击查看详情';
          return members.length.toLocaleString() + ' ' + esc(options.unit) + '<br>' + members.slice(0, 3).map(point => esc(point.name || '未命名')).join('<br>') + '<br>点击放大并选择';
        } },
        geo: spatialGeo(32),
        series: [{ id: 'spatial-points', type: 'scatter', coordinateSystem: 'geo', data: [], symbolSize: value => value[2] === 1 ? 8 : Math.min(44, 22 + Math.log2(value[2]) * 3), itemStyle: { color: '#206bc4', opacity: .88 }, label: { show: true, position: 'inside', color: '#fff', fontSize: 11, formatter: item => item.value[2] > 1 ? compact(item.value[2]) : '' }, zlevel: 1 }]
      });
      instance.on('georoam', schedule);
      instance.on('click', item => {
        const members = item.data && item.data.members;
        if (!members || !members.length) return;
        if (members.length === 1) { open(members[0][options.idKey]); return; }
        el('select').replaceChildren();
        members.forEach(point => el('select').add(new Option(point.name || '未命名', point[options.idKey])));
        el('selection').hidden = false;
        const zoom = instance.getOption().geo[0].zoom;
        if (zoom < 32) { instance.setOption({ geo: { center: item.value.slice(0, 2), zoom: Math.min(32, zoom * 2) } }); schedule(); }
      });
    }
    el('reset').addEventListener('click', () => { if (instance) { instance.setOption({ geo: { center: [104.5, 35.5], zoom: 1 } }); el('selection').hidden = true; schedule(); } });
    el('open').addEventListener('click', () => { if (el('select').value) open(el('select').value); });
    const api = {
      async render(data, onOpen) {
        const current = ++version;
        open = onOpen;
        try {
          await AdminAssets.loadCharts();
          await loadGeometry();
          if (current !== version) return;
          points = data.points || []; unlocated = Number(data.unlocated || 0);
          el('selection').hidden = true;
          if (!instance) initialize();
          el('error').hidden = true; schedule();
        } catch (_) { if (current === version) { el('error').textContent = '地图暂不可用，请刷新重试。'; el('error').hidden = false; } }
      },
      resize() { if (instance) { instance.resize(); schedule(); } }
    };
    maps.push(api); return api;
  }

  function routeCoverageLayers(tree) {
    const layers = { city: [], district: [], places: [] };
    let missing = 0;
    function coordinate(group) {
      const p = group.map_point;
      return p && Number.isFinite(p.longitude) && Number.isFinite(p.latitude) && (p.longitude || p.latitude)
        ? [p.longitude, p.latitude] : null;
    }
    function center(points) {
      if (!points.length) return null;
      const sum = points.reduce((total, p) => [total[0] + p[0], total[1] + p[1]], [0, 0]);
      return sum.map(value => value / points.length);
    }
    function regionMarker(bucket, points) {
      if (!points.length) return;
      const parts = [];
      if (bucket.scenicCount) parts.push(bucket.scenicCount.toLocaleString() + ' 处景点');
      if (bucket.independentCount) parts.push(bucket.independentCount.toLocaleString() + ' 组独立路线');
      layers[bucket.type].push({ id: bucket.key, type: bucket.type, name: bucket.name, path: bucket.path, value: center(points), note: parts.join(' · ') });
    }
    tree.forEach(city => {
      const cityPoints = [];
      city.children.forEach(district => {
        const districtPoints = [];
        district.children.forEach(scenic => {
          const value = center(scenic.groups.map(coordinate).filter(Boolean));
          if (!value) { missing++; return; }
          districtPoints.push(value);
          layers.places.push({ id: scenic.key, type: 'scenic', name: scenic.name, path: scenic.path, value, scenic, note: scenic.memberCount.toLocaleString() + ' 条轨迹' });
        });
        district.groups.forEach(group => {
          const value = coordinate(group);
          if (!value) { missing++; return; }
          districtPoints.push(value);
          const name = group.name || group.group_id;
          layers.places.push({ id: group.group_id, type: 'route', name, path: district.path.concat(name), value, groupID: group.group_id, note: Number(group.member_count || 0).toLocaleString() + ' 条轨迹' });
        });
        regionMarker(district, districtPoints);
        districtPoints.forEach(point => cityPoints.push(point));
      });
      regionMarker(city, cityPoints);
    });
    return { layers, missing };
  }

  function createRouteCoverageMap(options) {
    const el = key => document.getElementById(options[key]);
    const districtZoom = 12, placesZoom = 48;
    const levelNames = { city: '城市', district: '区县', places: '景点与独立路线' };
    const colors = { city: '#206bc4', district: '#206bc4', scenic: '#267568', route: '#206bc4' };
    let instance, data, filterQuery, frame = 0, version = 0, visible = new Map(), optionKey = '';
    function schedule() {
      if (frame) return;
      frame = requestAnimationFrame(() => { frame = 0; draw(); });
    }
    function reset() {
      if (!instance) return;
      instance.setOption({ geo: { center: [104.5, 35.5], zoom: 1 } });
      schedule();
    }
    function follow(marker) {
      if (!marker) return;
      if (marker.type === 'scenic') { options.openScenic(marker.scenic); return; }
      if (marker.type === 'route') { options.openRoute(marker.groupID); return; }
      instance.setOption({ geo: { center: marker.value, zoom: marker.type === 'city' ? 24 : 96 } });
      schedule();
    }
    function draw() {
      if (!instance || !data) return;
      const zoom = Number(instance.getOption().geo[0].zoom || 1);
      const level = zoom < districtZoom ? 'city' : zoom < placesZoom ? 'district' : 'places';
      const markers = data.layers[level].filter(marker => {
        const p = instance.convertToPixel({ geoIndex: 0 }, marker.value);
        return p && p[0] >= 0 && p[0] <= instance.getWidth() && p[1] >= 0 && p[1] <= instance.getHeight();
      });
      visible = new Map(markers.map(marker => [marker.id, marker]));
      instance.setOption({ series: [{ id: 'route-coverage-points', data: markers.map(marker => ({
        id: marker.id, name: marker.name, value: marker.value, itemStyle: { color: colors[marker.type] }, symbolSize: marker.type === 'route' ? 9 : 13
      })) }] });
      const key = markers.map(marker => marker.id + '\n' + marker.path.join('/')).join('\n');
      if (key !== optionKey) {
        optionKey = key;
        const select = el('select'), previous = select.value;
        select.replaceChildren(new Option('选择地图位置', ''), ...markers.map(marker => new Option(marker.path.join(' / '), marker.id)));
        select.value = visible.has(previous) ? previous : '';
      }
      el('selection').hidden = markers.length === 0;
      el('open').disabled = !el('select').value;
      el('summary').textContent = levelNames[level] + ' · 当前视野 ' + markers.length.toLocaleString() + ' 个点' +
        (data.missing ? ' · 缺坐标 ' + data.missing.toLocaleString() + ' 项' : '');
    }
    function initialize() {
      instance = echarts.init(el('chart'));
      instance.setOption({
        animation: false,
        aria: { enabled: true },
        tooltip: { trigger: 'item', confine: true, formatter: info => {
          const marker = info.data && visible.get(info.data.id);
          if (!marker) return '';
          const action = marker.type === 'scenic' ? '查看景点轨迹' : marker.type === 'route' ? '查看路线组' : '点击进入下一级';
          return esc(marker.path.join(' / ')) + '<br>' + esc(marker.note) + '<br>' + action;
        } },
        geo: spatialGeo(256),
        series: [{ id: 'route-coverage-points', type: 'scatter', coordinateSystem: 'geo', data: [],
          label: { show: true, position: 'right', color: '#334155', fontSize: 11, formatter: '{b}' },
          labelLayout: { hideOverlap: true }, zlevel: 1 }]
      });
      instance.on('georoam', schedule);
      instance.on('click', 'series.scatter', event => follow(event.data && visible.get(event.data.id)));
    }
    el('reset').addEventListener('click', reset);
    el('select').addEventListener('change', () => { el('open').disabled = !el('select').value; });
    el('open').addEventListener('click', () => follow(visible.get(el('select').value)));
    const api = {
      async render(directory) {
        const current = ++version;
        el('error').hidden = true;
        if (!directory.ready) {
          el('summary').textContent = directory.error ? '' : '正在读取路线目录…';
          if (directory.error) { el('error').textContent = '路线目录加载失败，请刷新统计。'; el('error').hidden = false; }
          return;
        }
        try {
          await AdminAssets.loadCharts();
          await loadGeometry();
          if (current !== version) return;
          data = routeCoverageLayers(directory.tree);
          if (!instance) initialize();
          if (filterQuery !== directory.query) reset();
          filterQuery = directory.query;
          schedule();
        } catch (_) {
          if (current === version) { el('error').textContent = '地图暂不可用，请刷新统计。'; el('error').hidden = false; }
        }
      },
      resize() { if (instance) { instance.resize(); schedule(); } }
    };
    maps.push(api);
    return api;
  }
  function resize() { charts.forEach(value => value.resize()); maps.forEach(value => value.resize()); }
  window.addEventListener('resize', resize);
  window.AdminUsageCharts = { createSpatialMap, createRouteCoverageMap, bars, trend, loading, date, resize };
})();
