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
    if (window.AMap && typeof window.AMap.Map === 'function') return Promise.resolve(window.AMap);
    if (sdkPromise) return sdkPromise;
    const attempt = new Promise((resolve, reject) => {
      window._AMapSecurityConfig = { serviceHost: window.location.origin + config.service_host };
      const script = document.createElement('script');
      let settled = false;
      const timeout = setTimeout(() => finish(new Error('高德地图加载超时，请恢复网络后重试')), 15000);
      function finish(error) {
        if (settled) return;
        settled = true; clearTimeout(timeout);
        script.onload = null; script.onerror = null;
        if (error) { script.remove(); reject(error); } else resolve(window.AMap);
      }
      script.async = true;
      script.src = 'https://webapi.amap.com/maps?v=2.0&key=' + encodeURIComponent(config.key) + '&plugin=AMap.Scale';
      script.onload = () => finish(window.AMap && typeof window.AMap.Map === 'function' ? null : new Error('高德地图 SDK 未就绪，请重试'));
      script.onerror = () => finish(new Error('高德地图脚本未加载，请恢复网络后重试'));
      document.head.appendChild(script);
    });
    sdkPromise = attempt;
    // A transient network failure must not poison every later map opened on this page.
    attempt.catch(() => { if (sdkPromise === attempt) sdkPromise = null; });
    return attempt;
  }

  function create(geo, options) {
    options = options || {};
    const section = document.createElement('section');
    section.className = 'admin-track-map';
    const heading = document.createElement('div'); heading.className = 'admin-track-candidates-heading';
    const title = document.createElement('h3'); title.textContent = '地图校验';
    const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-sm btn-outline-secondary'; retry.textContent = '重试预览'; retry.hidden = true;
    const retryMap = document.createElement('button'); retryMap.type = 'button'; retryMap.className = 'btn btn-sm btn-outline-secondary'; retryMap.textContent = '重试地图'; retryMap.hidden = true;
    heading.append(title, retryMap, retry);
    const body = document.createElement('div'); body.className = 'admin-track-map-body';
    const note = document.createElement('p'); note.className = 'text-secondary mb-2';
    const canvas = document.createElement('div'); canvas.className = 'admin-track-map-canvas';
    const selection = document.createElement('div'); selection.className = 'admin-track-map-poi'; selection.hidden = true;
    selection.setAttribute('aria-live', 'polite');
    body.append(note, canvas); section.append(heading, body);
    note.hidden = true;
    selection.addEventListener('click', event => event.stopPropagation());

    const controller = new AbortController();
    let disposed = false, map = null, amap = null, overlays = [], variants = null;
    let mapLoading = false;
    let selected = options.coordinateSystem || '', changed = false, fitted = false, fitTimer = null, observer = null;
    let currentSpot = options.primaryScenicSpot || null, expectedSpotID = options.scenicSpotID;
    let placeSearch = null, selectionSequence = 0, lookupTimer = null, clickTimer = null, assigning = false;
    let lastHotspotAt = 0, selectionMarker = null, infoWindow = null;

    let photos = [], photoMarkers = [], photoWindow = null, lastPhotoClick = 0;
    let endpoints = [], routeLines = [], themeObserver = null;
    const endpointSize = 25;
    if (options.strokeColor) section.style.setProperty('--track-sport-day', options.strokeColor);
    if (options.strokeColorDark) section.style.setProperty('--track-sport-night', options.strokeColorDark);

    function closePhoto() {
      if (photoWindow) { photoWindow.close(); photoWindow = null; }
    }
    function showPhotoPoint(group, position) {
      lastPhotoClick = Date.now();
      clearTimeout(clickTimer); clearTimeout(lookupTimer); selectionSequence++;
      if (infoWindow) infoWindow.close();
      if (selectionMarker) { map.remove(selectionMarker); selectionMarker = null; }
      closePhoto();
      photoWindow = new amap.InfoWindow({isCustom:true, autoMove:true, offset:new amap.Pixel(0, -12),
        content:options.photoPanel.popup(group, closePhoto)});
      photoWindow.open(map, position);
    }
    function drawPhotoPoints() {
      if (!map || !options.photoPanel) return;
      closePhoto();
      if (photoMarkers.length) map.remove(photoMarkers);
      photoMarkers = [];
      const groups = new Map();
      for (const photo of photos) {
        const position = poiPosition(photo.map_position);
        if (!position) continue;
        const key = position.join(',');
        if (!groups.has(key)) groups.set(key, {position, items:[]});
        groups.get(key).items.push(photo);
      }
      for (const {position, items} of groups.values()) {
        const icon = document.createElement('button');
        icon.type = 'button'; icon.className = 'admin-track-photo-pin';
        icon.setAttribute('aria-label', '查看此处的 ' + items.length + ' 张随拍');
        const marker = new amap.Marker({position, content:icon, anchor:'center', offset:new amap.Pixel(0, 0), bubble:false, zIndex:120});
        marker.on('click', () => showPhotoPoint(items, position));
        icon.addEventListener('keydown', event => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault(); event.stopPropagation(); showPhotoPoint(items, position);
          }
        });
        photoMarkers.push(marker);
      }
      if (photoMarkers.length) {
        map.add(photoMarkers);
        if (!overlays.length) fit();
      }
    }
    section.setPhotos = items => { photos = items; drawPhotoPoints(); };

    function routeColors() {
      const style = getComputedStyle(section);
      return [style.getPropertyValue('--track-route-border').trim(), style.getPropertyValue('--track-route-core').trim()];
    }
    function updateRouteColors() {
      const [border, core] = routeColors();
      for (const [outline, line] of routeLines) {
        outline.setOptions({strokeColor:border}); line.setOptions({strokeColor:core});
      }
    }
    function addEndpoint(position, end) {
      const icon = document.createElement('span');
      icon.className = 'admin-track-endpoint' + (end ? ' is-end' : '');
      icon.textContent = end ? '终' : '起';
      const marker = new amap.Marker({position, content:icon, anchor:'center', offset:new amap.Pixel(0, 0), title:end ? '终点' : '起点', zIndex:110});
      endpoints.push({position, marker});
      overlays.push(marker);
    }
    function separateEndpoints() {
      if (!map || endpoints.length !== 2) return;
      const [first, last] = endpoints.map(({position}) => map.lngLatToContainer(new amap.LngLat(...position)));
      if (!first || !last) return;
      const dx = first.getX() - last.getX(), dy = first.getY() - last.getY();
      const shift = Math.hypot(dx, dy) < endpointSize ? (dx <= 0 ? -1 : 1) * endpointSize / 2 : 0;
      endpoints[0].marker.setOffset(new amap.Pixel(shift, 0));
      endpoints[1].marker.setOffset(new amap.Pixel(-shift, 0));
    }

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
    function lookupFailure(result) {
      // 仅展示供应商的短错误码，避免把带密钥的请求地址或原始响应带入提示。
      const code = typeof result === 'string' ? result : result && result.info;
      return typeof code === 'string' && /^[A-Z][A-Z0-9_]{1,79}$/.test(code) && code !== 'OK' ? '（' + code + '）' : '';
    }
    function beginSelection(title, position) {
      closePhoto();
      const sequence = ++selectionSequence;
      clearTimeout(lookupTimer); clearTimeout(clickTimer);
      selection.replaceChildren(); selection.hidden = false;
      const name = document.createElement('strong'); name.textContent = title;
      const detail = document.createElement('p'); detail.className = 'text-secondary';
      const close = document.createElement('button'); close.type = 'button'; close.className = 'btn-close'; close.setAttribute('aria-label', '关闭景点浮窗');
      close.addEventListener('click', () => { selectionSequence++; clearTimeout(lookupTimer); if (infoWindow) infoWindow.close(); });
      selection.append(close, name, detail);
      if (position) openSelection(position);
      return { name, detail, current: () => !disposed && sequence === selectionSequence };
    }
    function openSelection(position) {
      if (!infoWindow) infoWindow = new amap.InfoWindow({ isCustom: true, content: selection, offset: new amap.Pixel(0, -16), autoMove: true });
      infoWindow.open(map, position);
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
      let position = poiPosition(event.lnglat || event.location);
      if (!id) { if (position) searchNearby(position); return; }
      const { name, detail, current } = beginSelection(plainText(event.name) || '地图景点', position);
      if (position) markSelection(position, name.textContent);
      detail.textContent = '正在读取景点资料…';
      const button = document.createElement('button'); button.type = 'button'; button.className = 'btn btn-sm btn-primary';
      button.dataset.adminWrite = 'tracks'; button.textContent = '设为该轨迹景点标签'; button.disabled = true;
      const actions = document.createElement('div'); actions.className = 'admin-track-poi-actions'; selection.appendChild(actions);
      actions.appendChild(button);
      const nearby = document.createElement('button'); nearby.type = 'button'; nearby.className = 'btn btn-sm btn-outline-secondary'; nearby.textContent = '附近景点';
      nearby.addEventListener('click', () => searchNearby(position));
      nearby.disabled = !position; actions.appendChild(nearby);
      const retryDetails = document.createElement('button'); retryDetails.type = 'button'; retryDetails.className = 'btn btn-sm btn-outline-secondary'; retryDetails.textContent = '重试资料'; retryDetails.hidden = true;
      retryDetails.addEventListener('click', () => selectPOI(event)); actions.appendChild(retryDetails);
      const failed = message => {
        if (current()) { detail.textContent = message; retryDetails.hidden = false; }
      };
      try {
        const PlaceSearch = await loadPlaceSearch(amap);
        if (!current()) return;
        if (!placeSearch) placeSearch = new PlaceSearch({ extensions: 'all' });
        lookupTimer = setTimeout(() => failed('景点资料读取超时，请重试。'), 15000);
        placeSearch.getDetails(id, (status, result) => {
          if (!current()) return;
          clearTimeout(lookupTimer);
          if (status !== 'complete') {
            failed('景点资料读取失败' + lookupFailure(result) + '，请重试。'); return;
          }
          const pois = result && result.poiList && result.poiList.pois;
          const poi = Array.isArray(pois) && pois.find(item => item && String(item.id || '') === id);
          const point = poi && poiPosition(poi.location);
          if (!poi) {
            failed('未返回所选景点的资料，请重试或查看附近景点。'); return;
          }
          if (!point || !plainText(poi.name).trim()) {
            failed('景点资料缺少名称或坐标，请选择其他景点。'); return;
          }
          retryDetails.hidden = true;
          position = point; nearby.disabled = false;
          name.textContent = poi.name;
          markSelection(point, poi.name);
          openSelection(point);
          detail.textContent = plainText(poi.address) || '暂无地址';
          button.disabled = isCurrentPOI(id);
          if (button.disabled) button.textContent = '当前景点标签';
          button.addEventListener('click', () => assignPOI({ id: id, name: poi.name, category: plainText(poi.type), address: plainText(poi.address), position: point }, button, detail));
        });
      } catch (error) { failed(error.message); }
    }
    async function searchNearby(position) {
      if (disposed || assigning || !position || !map || !options.canAssignScenicSpot) return;
      closePhoto();
      const sequence = ++selectionSequence;
      const current = () => !disposed && sequence === selectionSequence;
      clearTimeout(lookupTimer); clearTimeout(clickTimer);
      if (infoWindow) infoWindow.close();
      markSelection(position, '附近景点查询位置');
      const publish = state => {
        if (current() && typeof options.onNearbyResults === 'function') options.onNearbyResults(Object.assign({
          retry: () => searchNearby(position),
          select: poi => { if (!disposed) { section.scrollIntoView({ behavior: 'smooth', block: 'nearest' }); selectPOI(poi); } }
        }, state));
      };
      publish({ loading: true });
      try {
        const PlaceSearch = await loadPlaceSearch(amap);
        if (!current()) return;
        const nearbySearch = new PlaceSearch({ pageSize: 8, pageIndex: 1, extensions: 'all', type: '110000' });
        lookupTimer = setTimeout(() => publish({ error: '附近景点查询超时，请重试。' }), 15000);
        nearbySearch.searchNearBy('', position, 1500, (status, result) => {
          if (!current()) return;
          clearTimeout(lookupTimer);
          const pois = status === 'complete' && result && result.poiList && result.poiList.pois;
          const candidates = Array.isArray(pois) ? pois.filter(poi => plainText(poi.id).trim() && plainText(poi.name).trim() && poiPosition(poi.location)) : [];
          if (!candidates.length && status !== 'complete' && status !== 'no_data') {
            publish({ error: '附近景点查询失败' + lookupFailure(result) + '，请重试。' }); return;
          }
          publish({ items: candidates });
        });
      } catch (error) { publish({ error: error.message }); }
    }
    function onHotspotClick(event) {
      if (Date.now() - lastPhotoClick < 300) return;
      lastHotspotAt = Date.now(); clearTimeout(clickTimer);
      selectPOI(event);
    }
    function onMapClick(event) {
      if (disposed || Date.now() - lastPhotoClick < 300) return;
      closePhoto();
      if (!options.canAssignScenicSpot || assigning || Date.now() - lastHotspotAt < 300) return;
      const position = poiPosition(event && event.lnglat);
      if (!position) return;
      clearTimeout(clickTimer);
      // 同一次点击可能同时派发普通点击和热点点击，优先保留精确 POI。
      clickTimer = setTimeout(() => searchNearby(position), 250);
    }
    const normalize = value => String(value || 'GCJ02').toUpperCase().replace(/-/g, '');
    function pathsOf(points) {
      const paths = [];
      let path = null, segment = null;
      for (const point of Array.isArray(points) ? points : []) {
        const position = poiPosition(Array.isArray(point) ? [point[1], point[0]] : point && [point.longitude, point.latitude]);
        if (!position) { path = null; segment = null; continue; }
        const id = String(point.segment_id == null ? 0 : point.segment_id);
        if (!path || segment !== id) {
          path = []; paths.push(path); segment = id;
        }
        path.push(position);
      }
      return paths;
    }
    function fit() {
      if (disposed || !map || !canvas.isConnected) return;
      map.resize();
      const targets = overlays.length ? overlays : photoMarkers;
      if (targets.length) map.setFitView(targets, true, [64, 36, 36, 36], 17);
      separateEndpoints();
    }
    function draw() {
      if (disposed) return;
      const source = normalize(selected);
      const points = variants ? variants[source] : (!changed && geo ? geo.simplified_polyline : []);
      if (variants) { note.textContent = points ? '' : '当前历史坐标系不支持预览，请选择 WGS84、GCJ02 或 BD09。'; note.hidden = !!points; }
      if (!map) return;
      if (overlays.length) map.remove(overlays);
      overlays = []; endpoints = []; routeLines = [];
      const paths = pathsOf(points);
      if (!paths.length) return;
      const [border, core] = routeColors();
      for (const [index, path] of paths.entries()) {
        if (path.length === 1) {
          if (index > 0 && index < paths.length - 1) {
            const dot = document.createElement('span');
            dot.className = 'admin-track-isolated-point';
            overlays.push(new amap.Marker({position:path[0], content:dot, anchor:'center', offset:new amap.Pixel(0, 0), title:'轨迹点', bubble:true}));
          }
          continue;
        }
        const common = {path, lineJoin:'round', lineCap:'round', strokeOpacity:1, bubble:true};
        const outline = new amap.Polyline({...common, strokeColor:border, strokeWeight:7, zIndex:50});
        const line = new amap.Polyline({...common, strokeColor:core, strokeWeight:4, zIndex:51});
        routeLines.push([outline, line]); overlays.push(outline, line);
      }
      const start = paths[0][0], end = paths.at(-1).at(-1);
      addEndpoint(start, false);
      if (start[0] !== end[0] || start[1] !== end[1]) addEndpoint(end, true);
      map.add(overlays);
      separateEndpoints();
      // 切换坐标系保留视野，便于比较同一路段的偏移。
      if (!fitted) { fitted = true; fit(); }
    }
    async function loadVariants() {
      retry.hidden = true;
      note.textContent = '正在准备坐标系预览…'; note.hidden = false;
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
        retry.hidden = false; note.hidden = false;
      }
    }
    section.setCoordinateControl = function (group) { group.classList.add('admin-track-coordinate'); heading.insertBefore(group, retry); };
    section.setCoordinateSystem = function (value) {
      selected = value; changed = true;
      if (!variants) note.textContent = retry.hidden ? '正在准备所选坐标系预览…' : '预览加载失败，请点击“重试预览”。';
      draw();
    };
    section.setScenicSpot = function (spot, spotID) { currentSpot = spot; expectedSpotID = spotID; };
    section.dispose = function () {
      disposed = true; selectionSequence++; controller.abort(); clearTimeout(fitTimer); clearTimeout(lookupTimer); clearTimeout(clickTimer);
      if (observer) observer.disconnect();
      if (themeObserver) themeObserver.disconnect();
      closePhoto();
      if (infoWindow) infoWindow.close();
      if (placeSearch) placeSearch.clear();
      if (map) map.destroy();
    };
    retry.addEventListener('click', loadVariants);
    loadVariants();
    async function initializeMap() {
      if (disposed || mapLoading || map) return;
      mapLoading = true; retryMap.hidden = true; retryMap.disabled = true;
      canvas.textContent = '正在加载地图…';
      try {
        const response = await fetch('/admin/api/tracks/map-config', { signal: controller.signal });
        if (!response.ok) throw new Error('地图配置读取失败');
        const config = ((await response.json()).data) || {};
        if (!config.configured) throw new Error('地图服务未配置');
        amap = await loadAMapSDK(config);
        if (disposed || !canvas.isConnected) return;
        canvas.replaceChildren();
        map = new amap.Map(canvas, { resizeEnable: true, scrollWheel: true, isHotspot: true, zoom: 13, center: [116.397, 39.909] });
        map.addControl(new amap.Scale());
        if (options.canAssignScenicSpot) {
          map.on('hotspotclick', onHotspotClick);
        }
        map.on('click', onMapClick);
        map.on('zoomend', separateEndpoints);
        map.on('moveend', separateEndpoints);
        themeObserver = new MutationObserver(updateRouteColors);
        for (const element of [document.documentElement, document.body]) {
          themeObserver.observe(element, {attributes:true, attributeFilter:['class', 'data-theme', 'data-bs-theme']});
        }
        draw(); drawPhotoPoints();
        fitTimer = window.setTimeout(fit, 350);
        if ('IntersectionObserver' in window) {
          observer = new IntersectionObserver(entries => {
            if (entries.some(entry => entry.isIntersecting)) { observer.disconnect(); fit(); }
          });
          observer.observe(canvas);
        }
      } catch (error) {
        clearTimeout(fitTimer);
        if (observer) { observer.disconnect(); observer = null; }
        if (themeObserver) { themeObserver.disconnect(); themeObserver = null; }
        closePhoto();
        if (map) { map.destroy(); map = null; overlays = []; endpoints = []; photoMarkers = []; routeLines = []; fitted = false; }
        if (!disposed) {
          canvas.textContent = '地图暂时无法加载：' + error.message;
          retryMap.hidden = false;
        }
      } finally {
        mapLoading = false; retryMap.disabled = false;
      }
    }
    retryMap.addEventListener('click', initializeMap);
    initializeMap();
    return section;
  }

  window.AdminTrackMapPreview = { create: create };
})();
