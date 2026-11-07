(function () {
  'use strict';

  let sdkPromise = null;

  function loadPlaceSearch(amap) {
    if (amap.PlaceSearch) return Promise.resolve(amap.PlaceSearch);
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('景点查询加载超时，请重新点击景点')), 15000);
      amap.plugin('AMap.PlaceSearch', () => {
        clearTimeout(timer);
        if (amap.PlaceSearch) resolve(amap.PlaceSearch);
        else reject(new Error('景点查询暂不可用'));
      });
    });
  }

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
    if (options.canAssignScenicSpot) {
      const hint = document.createElement('p'); hint.className = 'text-secondary mb-2';
      hint.textContent = '点击地图景点，可设为这条轨迹的景点标签。';
      body.appendChild(hint);
    }
    body.append(note, canvas); section.append(heading, body);

    const controller = new AbortController();
    let disposed = false, map = null, amap = null, overlays = [], variants = null;
    let selected = options.coordinateSystem || '', changed = false, fitted = false, fitTimer = null, observer = null;
    let currentSpot = options.primaryScenicSpot || null, expectedSpotID = options.scenicSpotID;
    let infoWindow = null, placeSearch = null, selectionSequence = 0, lookupTimer = null, assigning = false;

    function poiPosition(value) {
      if (!value) return null;
      const lng = Number(typeof value.getLng === 'function' ? value.getLng() : value.lng);
      const lat = Number(typeof value.getLat === 'function' ? value.getLat() : value.lat);
      return Number.isFinite(lng) && Number.isFinite(lat) ? [lng, lat] : null;
    }
    function plainText(value) { return typeof value === 'string' ? value : ''; }
    function isCurrentPOI(id) {
      return !!currentSpot && (currentSpot.sources || []).some(source => source.provider === 'amap' && source.provider_id === id && source.status !== 'disabled');
    }
    async function assignPOI(poi, button, detail) {
      if (disposed || assigning || !options.canAssignScenicSpot) return;
      assigning = true; button.disabled = true; button.textContent = '正在设置…';
      try {
        const response = await fetch('/admin/api/tracks/' + encodeURIComponent(options.trackID) + '/scenic-spot', {
          method: 'POST', signal: controller.signal, headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ provider_id: poi.id, name: poi.name, category: poi.category, address: poi.address,
            longitude: poi.position[0], latitude: poi.position[1], expected_spot_id: expectedSpotID })
        });
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) {
          if (response.status === 409 && typeof options.onScenicSpotRefresh === 'function') {
            await options.onScenicSpotRefresh();
            if (infoWindow) infoWindow.close();
            throw new Error('景点关联已变化或该景点不可用，请重新选择后再试');
          }
          throw new Error(payload.error || '设置景点标签失败');
        }
        if (disposed) return;
        const data = payload.data;
        section.setScenicSpot(data.primary_scenic_spot, data.scenic_spot_id);
        if (infoWindow) infoWindow.close();
        if (typeof options.onScenicSpotAssigned === 'function') {
          try { await options.onScenicSpotAssigned(data); }
          catch (_) { if (!disposed) AdminUI.alert('景点标签已设置，列表刷新失败，请手动刷新', 'warning'); }
        }
        if (!disposed) AdminUI.alert('已设为轨迹景点标签：' + data.primary_scenic_spot.name, 'success');
      } catch (error) {
        if (!disposed && error.name !== 'AbortError') {
          detail.textContent = error.message;
          AdminUI.alert('设置失败：' + error.message, 'danger');
        }
      } finally {
        assigning = false;
        button.disabled = isCurrentPOI(poi.id);
        button.textContent = button.disabled ? '当前景点标签' : '设为该轨迹景点标签';
      }
    }
    async function selectPOI(event) {
      if (disposed || assigning || !event || !map) return;
      const position = poiPosition(event.lnglat);
      if (!position) return;
      const sequence = ++selectionSequence;
      clearTimeout(lookupTimer);
      if (infoWindow) infoWindow.close();
      const popup = document.createElement('div'); popup.className = 'admin-track-map-poi';
      const name = document.createElement('strong'); name.textContent = plainText(event.name) || '地图景点';
      const detail = document.createElement('p'); detail.className = 'text-secondary';
      popup.append(name, detail);
      infoWindow = new amap.InfoWindow({ content: popup, closeWhenClickMap: true });
      infoWindow.open(map, position);
      if (!options.canAssignScenicSpot) { detail.textContent = '景点标签设置需要轨迹及景点操作权限。'; return; }
      const id = String(event.id || '').trim();
      if (!id) { detail.textContent = '此标记没有可用的 POI ID，请选择其他景点。'; return; }
      detail.textContent = '正在读取景点资料…';
      const button = document.createElement('button'); button.type = 'button'; button.className = 'btn btn-sm btn-primary';
      button.dataset.adminWrite = 'tracks'; button.textContent = '设为该轨迹景点标签'; button.disabled = true;
      popup.appendChild(button);
      const current = () => !disposed && sequence === selectionSequence && popup.isConnected;
      try {
        const PlaceSearch = await loadPlaceSearch(amap);
        if (!current()) return;
        if (!placeSearch) placeSearch = new PlaceSearch({ extensions: 'all' });
        lookupTimer = setTimeout(() => { if (current()) detail.textContent = '景点资料读取超时，请重新点击景点重试。'; }, 15000);
        placeSearch.getDetails(id, (status, result) => {
          if (!current()) return;
          clearTimeout(lookupTimer);
          const poi = status === 'complete' && result && result.poiList && result.poiList.pois && result.poiList.pois[0];
          const point = poi && poiPosition(poi.location);
          if (!poi || !point || !plainText(poi.name).trim() || String(poi.id || '') !== id) {
            detail.textContent = '景点资料读取失败，请重新点击景点重试。'; return;
          }
          name.textContent = poi.name;
          detail.textContent = [plainText(poi.type), plainText(poi.address)].filter(Boolean).join(' · ');
          const scope = document.createElement('p'); scope.className = 'text-secondary';
          scope.textContent = isCurrentPOI(id) ? '这是当前轨迹的景点标签。' : currentSpot ? '立即替换当前景点标签：' + (currentSpot.name || currentSpot.id) : '设置后立即生效。';
          popup.insertBefore(scope, button);
          button.disabled = isCurrentPOI(id);
          if (button.disabled) button.textContent = '当前景点标签';
          button.addEventListener('click', () => assignPOI({ id: id, name: poi.name, category: plainText(poi.type), address: plainText(poi.address), position: point }, button, detail));
        });
      } catch (error) { if (current()) detail.textContent = error.message; }
    }
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
    section.setScenicSpot = function (spot, spotID) { currentSpot = spot; expectedSpotID = spotID; };
    section.dispose = function () {
      disposed = true; selectionSequence++; controller.abort(); clearTimeout(fitTimer); clearTimeout(lookupTimer);
      if (observer) observer.disconnect();
      if (infoWindow) infoWindow.close();
      if (placeSearch) placeSearch.clear();
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
        map = new amap.Map(canvas, { resizeEnable: true, scrollWheel: true, isHotspot: true, zoom: 13, center: [116.397, 39.909] });
        map.addControl(new amap.Scale());
        map.on('hotspotclick', selectPOI);
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
