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
    const selection = document.createElement('div'); selection.className = 'admin-track-map-poi'; selection.hidden = true;
    selection.setAttribute('aria-live', 'polite');
    if (options.canAssignScenicSpot) {
      const hint = document.createElement('p'); hint.className = 'text-secondary mb-2';
      hint.textContent = '点击地图景点，在地图下方设置标签；未点中时可从附近景点中选择。';
      body.appendChild(hint);
    }
    body.append(note, canvas, selection); section.append(heading, body);

    const controller = new AbortController();
    let disposed = false, map = null, amap = null, overlays = [], variants = null;
    let selected = options.coordinateSystem || '', changed = false, fitted = false, fitTimer = null, observer = null;
    let currentSpot = options.primaryScenicSpot || null, expectedSpotID = options.scenicSpotID;
    let placeSearch = null, selectionSequence = 0, lookupTimer = null, clickTimer = null, assigning = false;
    let lastHotspotAt = 0, selectionMarker = null;

    function poiPosition(value) {
      if (!value) return null;
      if (typeof value === 'string') value = value.split(',');
      const longitude = Array.isArray(value) ? value[0] : typeof value.getLng === 'function' ? value.getLng() : value.lng;
      const latitude = Array.isArray(value) ? value[1] : typeof value.getLat === 'function' ? value.getLat() : value.lat;
      if (longitude == null || latitude == null || longitude === '' || latitude === '') return null;
      const lng = Number(longitude), lat = Number(latitude);
      return Number.isFinite(lng) && Number.isFinite(lat) && Math.abs(lng) <= 180 && Math.abs(lat) <= 90 ? [lng, lat] : null;
    }
    function plainText(value) { return typeof value === 'string' ? value : ''; }
    function beginSelection(title) {
      const sequence = ++selectionSequence;
      clearTimeout(lookupTimer); clearTimeout(clickTimer);
      selection.replaceChildren(); selection.hidden = false;
      const name = document.createElement('strong'); name.textContent = title;
      const detail = document.createElement('p'); detail.className = 'text-secondary';
      selection.append(name, detail);
      return { name, detail, current: () => !disposed && sequence === selectionSequence };
    }
    function markSelection(position, name) {
      if (selectionMarker) map.remove(selectionMarker);
      selectionMarker = new amap.Marker({ position: position, title: name, bubble: true });
      map.add(selectionMarker);
    }
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
            throw new Error('景点关联已变化或该景点不可用，请重新选择后再试');
          }
          throw new Error(payload.error || '设置景点标签失败');
        }
        if (disposed) return;
        const data = payload.data;
        section.setScenicSpot(data.primary_scenic_spot, data.scenic_spot_id);
        detail.textContent = '已设为这条轨迹的景点标签。';
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
      if (disposed || assigning || !event || !map || !options.canAssignScenicSpot) return;
      const id = String(event.id || '').trim();
      const position = poiPosition(event.lnglat || event.location);
      if (!id) { if (position) searchNearby(position); return; }
      const { name, detail, current } = beginSelection(plainText(event.name) || '地图景点');
      if (position) markSelection(position, name.textContent);
      detail.textContent = '正在读取景点资料…';
      const button = document.createElement('button'); button.type = 'button'; button.className = 'btn btn-sm btn-primary';
      button.dataset.adminWrite = 'tracks'; button.textContent = '设为该轨迹景点标签'; button.disabled = true;
      selection.appendChild(button);
      const nearby = document.createElement('button'); nearby.type = 'button'; nearby.className = 'btn btn-sm btn-outline-secondary ms-2'; nearby.textContent = '查看附近景点';
      nearby.addEventListener('click', () => searchNearby(position));
      if (position) selection.appendChild(nearby);
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
          markSelection(point, poi.name);
          detail.textContent = [plainText(poi.type), plainText(poi.address)].filter(Boolean).join(' · ');
          const scope = document.createElement('p'); scope.className = 'text-secondary';
          scope.textContent = isCurrentPOI(id) ? '这是当前轨迹的景点标签。' : currentSpot ? '立即替换当前景点标签：' + (currentSpot.name || currentSpot.id) : '设置后立即生效。';
          selection.insertBefore(scope, button);
          button.disabled = isCurrentPOI(id);
          if (button.disabled) button.textContent = '当前景点标签';
          button.addEventListener('click', () => assignPOI({ id: id, name: poi.name, category: plainText(poi.type), address: plainText(poi.address), position: point }, button, detail));
        });
      } catch (error) { if (current()) detail.textContent = error.message; }
    }
    async function searchNearby(position) {
      if (disposed || assigning || !position || !map || !options.canAssignScenicSpot) return;
      const { detail, current } = beginSelection('选择附近景点');
      detail.textContent = '正在查询点击位置附近的景点…';
      markSelection(position, '点击位置');
      const retrySearch = document.createElement('button'); retrySearch.type = 'button'; retrySearch.className = 'btn btn-sm btn-outline-secondary'; retrySearch.textContent = '重新查询'; retrySearch.hidden = true;
      retrySearch.addEventListener('click', () => searchNearby(position)); selection.appendChild(retrySearch);
      try {
        const PlaceSearch = await loadPlaceSearch(amap);
        if (!current()) return;
        const nearbySearch = new PlaceSearch({ pageSize: 8, pageIndex: 1, extensions: 'all', type: '110000' });
        lookupTimer = setTimeout(() => {
          if (current()) { detail.textContent = '附近景点查询超时，请重试。'; retrySearch.hidden = false; }
        }, 15000);
        nearbySearch.searchNearBy('', position, 1500, (status, result) => {
          if (!current()) return;
          clearTimeout(lookupTimer);
          const pois = status === 'complete' && result && result.poiList && result.poiList.pois;
          const candidates = Array.isArray(pois) ? pois.filter(poi => plainText(poi.id).trim() && plainText(poi.name).trim() && poiPosition(poi.location)) : [];
          if (!candidates.length) {
            detail.textContent = status === 'complete' || status === 'no_data' ? '附近未找到景点，请放大地图后重新点选。' : '附近景点查询失败，请重试。';
            retrySearch.hidden = false; return;
          }
          detail.textContent = '请选择目标景点，再点击设置按钮。';
          const list = document.createElement('div'); list.className = 'admin-track-map-poi-list';
          candidates.forEach(poi => {
            const item = document.createElement('button'); item.type = 'button'; item.className = 'btn btn-sm btn-outline-secondary';
            item.textContent = poi.name; item.title = [plainText(poi.type), plainText(poi.address)].filter(Boolean).join(' · ');
            item.addEventListener('click', () => selectPOI(poi)); list.appendChild(item);
          });
          selection.insertBefore(list, retrySearch);
        });
      } catch (error) {
        if (current()) { detail.textContent = error.message; retrySearch.hidden = false; }
      }
    }
    function onHotspotClick(event) {
      lastHotspotAt = Date.now(); clearTimeout(clickTimer);
      selectPOI(event);
    }
    function onMapClick(event) {
      if (!options.canAssignScenicSpot || disposed || assigning || Date.now() - lastHotspotAt < 300) return;
      const position = poiPosition(event && event.lnglat);
      if (!position) return;
      clearTimeout(clickTimer);
      // 同一次点击可能同时派发普通点击和热点点击，优先保留精确 POI。
      clickTimer = setTimeout(() => searchNearby(position), 250);
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
      const polyline = new amap.Polyline({ path, strokeColor: '#2563eb', strokeWeight: 4, strokeOpacity: 0.9, lineJoin: 'round', lineCap: 'round', bubble: true });
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
      disposed = true; selectionSequence++; controller.abort(); clearTimeout(fitTimer); clearTimeout(lookupTimer); clearTimeout(clickTimer);
      if (observer) observer.disconnect();
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
        if (options.canAssignScenicSpot) {
          map.on('hotspotclick', onHotspotClick);
          map.on('click', onMapClick);
        }
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
