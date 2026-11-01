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

  // create(geo) 返回轨迹地图区块。geo 来自 GET /admin/api/tracks/:track_id 的 data.geo：
  // { simplified_polyline: [[lat,lng],...]（GCJ02）, start_lat/lng, end_lat/lng }。
  // 地图在元素插入文档后异步初始化；无轨迹线或地图未配置时显示提示。
  function create(geo) {
    const section = document.createElement('section');
    section.className = 'admin-track-map';
    const heading = document.createElement('div'); heading.className = 'admin-track-candidates-heading';
    const title = document.createElement('h3'); title.textContent = '轨迹地图';
    heading.appendChild(title);
    const body = document.createElement('div'); body.className = 'admin-track-map-body';
    section.append(heading, body);

    let points = [];
    if (geo && Array.isArray(geo.simplified_polyline)) {
      points = geo.simplified_polyline
        .map(p => {
          if (Array.isArray(p) && p.length >= 2) return { lat: Number(p[0]), lng: Number(p[1]) };
          if (p && typeof p === 'object') return { lat: Number(p.latitude), lng: Number(p.longitude) };
          return null;
        })
        .filter(p => p && Number.isFinite(p.lat) && Number.isFinite(p.lng));
    }
    if (!points.length) {
      const note = document.createElement('p'); note.className = 'text-secondary mb-0';
      note.textContent = '暂无地图索引轨迹线（原始轨迹未索引或文件缺失）。';
      body.appendChild(note);
      return section;
    }
    const canvas = document.createElement('div'); canvas.className = 'admin-track-map-canvas';
    body.appendChild(canvas);

    (async () => {
      try {
        const configResp = await fetch('/admin/api/tracks/map-config');
        const config = ((await configResp.json()).data) || {};
        if (!config.configured) {
          body.replaceChildren();
          const note = document.createElement('p'); note.className = 'text-secondary mb-0';
          note.textContent = '地图服务未配置。';
          body.appendChild(note);
          return;
        }
        const AMap = await loadAMapSDK(config);
        if (!canvas.isConnected) return;
        const map = new AMap.Map(canvas, { resizeEnable: true, scrollWheel: true, zoom: 13, center: [points[0].lng, points[0].lat] });
        map.addControl(new AMap.Scale());
        const path = points.map(p => [p.lng, p.lat]);
        const polyline = new AMap.Polyline({ path, strokeColor: '#2563eb', strokeWeight: 4, strokeOpacity: 0.9, lineJoin: 'round', lineCap: 'round' });
        map.add(polyline);
        const start = new AMap.Marker({ position: path[0], label: { content: '起', direction: 'top' } });
        const end = new AMap.Marker({ position: path[path.length - 1], label: { content: '终', direction: 'top' } });
        map.add(start);
        map.add(end);
        const overlays = [polyline, start, end];
        const fit = () => {
          if (!canvas.isConnected) return;
          map.resize();
          map.setFitView(overlays, true, [40, 40, 40, 40], 17);
        };
        fit();
        // 弹窗动画期间画布尺寸可能为 0，等真正可见后再校准一次视野。
        window.setTimeout(fit, 350);
        if ('IntersectionObserver' in window) {
          const observer = new IntersectionObserver(entries => {
            if (entries.some(entry => entry.isIntersecting)) {
              observer.disconnect();
              fit();
            }
          });
          observer.observe(canvas);
        }
      } catch (_) {
        if (!canvas.isConnected) return;
        body.replaceChildren();
        const note = document.createElement('p'); note.className = 'text-secondary mb-0';
        note.textContent = '地图暂时无法加载。';
        body.appendChild(note);
      }
    })();
    return section;
  }

  window.AdminTrackMapPreview = { create: create };
})();
