(function () {
  'use strict';

  let sdkPromise = null;

  function loadAMapSDK(config) {
    if (window.AMap) return Promise.resolve(window.AMap);
    if (sdkPromise) return sdkPromise;
    sdkPromise = new Promise((resolve, reject) => {
      window._AMapSecurityConfig = { serviceHost: window.location.origin + config.service_host };
      const script = document.createElement('script');
      script.src = 'https://webapi.amap.com/maps?v=2.0&key=' + encodeURIComponent(config.key) + '&plugin=AMap.Scale';
      script.onload = () => (window.AMap ? resolve(window.AMap) : reject(new Error('高德地图 SDK 未就绪')));
      script.onerror = () => reject(new Error('高德地图 SDK 加载失败'));
      document.head.appendChild(script);
    });
    return sdkPromise;
  }

  function create(geo, options) {
    options = options || {};
    const section = document.createElement('section');
    section.className = 'admin-track-map';
    const heading = document.createElement('div'); heading.className = 'admin-track-candidates-heading';
    const title = document.createElement('h3'); title.textContent = '轨迹地图';
    const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-sm btn-outline-secondary'; retry.textContent = '重试预览'; retry.hidden = true;
    heading.append(title, retry);
    const body = document.createElement('div'); body.className = 'admin-track-map-body';
    const note = document.createElement('p'); note.className = 'text-secondary mb-2';
    const canvas = document.createElement('div'); canvas.className = 'admin-track-map-canvas';
    body.append(note, canvas); section.append(heading, body);

    const controller = new AbortController();
    let disposed = false, map = null, amap = null, overlays = [], variants = null;
    let selected = options.coordinateSystem || '', changed = false, fitted = false, fitTimer = null, observer = null;
    const normalize = value => String(value || 'GCJ02').toUpperCase().replace(/-/g, '');
    function pathOf(points) {
      return (points || []).map(p => Array.isArray(p) ? [Number(p[1]), Number(p[0])] : [Number(p.longitude), Number(p.latitude)])
        .filter(p => Number.isFinite(p[0]) && Number.isFinite(p[1]));
    }
    function fit() {
      if (disposed || !map || !canvas.isConnected) return;
      map.resize();
      if (overlays.length) map.setFitView(overlays, true, [40, 40, 40, 40], 17);
    }
    function draw() {
      if (disposed) return;
      const source = normalize(selected);
      const points = variants ? variants[source] : (!changed && geo ? geo.simplified_polyline : []);
      if (variants) note.textContent = points ? (selected ? '按 ' + selected + ' 预览；点击“保存修改”后生效。' : '未设置坐标系，按历史默认 GCJ02 预览。') : '当前历史坐标系不支持预览，请选择 WGS84、GCJ02 或 BD09。';
      if (!map) return;
      if (overlays.length) map.remove(overlays);
      overlays = [];
      const path = pathOf(points);
      if (path.length < 2) return;
      const polyline = new amap.Polyline({ path, strokeColor: '#2563eb', strokeWeight: 4, strokeOpacity: 0.9, lineJoin: 'round', lineCap: 'round' });
      const start = new amap.Marker({ position: path[0], label: { content: '起', direction: 'top' } });
      const end = new amap.Marker({ position: path[path.length - 1], label: { content: '终', direction: 'top' } });
      overlays = [polyline, start, end];
      map.add(overlays);
      // 切换坐标系保留视野，便于比较同一路段的偏移。
      if (!fitted) { fitted = true; fit(); }
    }
    async function loadVariants() {
      retry.hidden = true;
      note.textContent = '正在准备坐标系预览…';
      try {
        const response = await fetch('/admin/api/tracks/' + encodeURIComponent(options.trackID) + '/map-preview', { signal: controller.signal });
        const payload = await response.json();
        if (!response.ok) throw new Error(payload.error || '原始轨迹读取失败');
        if (disposed) return;
        variants = payload.data.variants;
        draw();
      } catch (error) {
        if (disposed) return;
        note.textContent = '坐标系预览加载失败：' + error.message + (changed ? '。' : '；暂显示已有索引。');
        retry.hidden = false;
      }
    }
    section.setCoordinateSystem = function (value) {
      selected = value; changed = true;
      if (!variants) note.textContent = retry.hidden ? '正在准备所选坐标系预览…' : '预览加载失败，请点击“重试预览”。';
      draw();
    };
    section.dispose = function () {
      disposed = true; controller.abort(); clearTimeout(fitTimer);
      if (observer) observer.disconnect();
      if (map) map.destroy();
    };
    retry.addEventListener('click', loadVariants);
    loadVariants();
    (async () => {
      try {
        const response = await fetch('/admin/api/tracks/map-config', { signal: controller.signal });
        if (!response.ok) throw new Error('地图配置读取失败');
        const config = ((await response.json()).data) || {};
        if (!config.configured) throw new Error('地图服务未配置');
        amap = await loadAMapSDK(config);
        if (disposed || !canvas.isConnected) return;
        map = new amap.Map(canvas, { resizeEnable: true, scrollWheel: true, zoom: 13, center: [116.397, 39.909] });
        map.addControl(new amap.Scale());
        draw();
        fitTimer = window.setTimeout(fit, 350);
        if ('IntersectionObserver' in window) {
          observer = new IntersectionObserver(entries => {
            if (entries.some(entry => entry.isIntersecting)) { observer.disconnect(); fit(); }
          });
          observer.observe(canvas);
        }
      } catch (error) {
        if (!disposed) { canvas.textContent = '地图暂时无法加载：' + error.message; }
      }
    })();
    return section;
  }

  window.AdminTrackMapPreview = { create: create };
})();
