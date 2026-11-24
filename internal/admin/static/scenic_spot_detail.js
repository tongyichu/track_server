(function () {
  'use strict';
  const get = id => document.getElementById(id);
  const message = get('scenicInfoMessage');
  const retry = get('scenicInfoRetry');
  let currentSpot = null, request = null;

  function photoURL(raw) {
    const prefix = '/api/v1/static/scenic_spots/';
    if (typeof raw !== 'string' || !raw.startsWith(prefix)) return '';
    const url = new URL(raw, location.origin);
    return url.pathname.startsWith(prefix) ? '/admin/api/static/scenic_spots/' + url.pathname.slice(prefix.length) : '';
  }

  function renderPhotos(spot) {
    const gallery = get('scenicInfoGallery');
    const photos = get('scenicInfoPhotos');
    const entries = (spot.photos || []).filter(photo => photo && photoURL(photo.url));
    if (!entries.length && photoURL(spot.cover_url)) entries.push({ url: spot.cover_url });
    photos.replaceChildren();
    entries.forEach(photo => {
      const figure = document.createElement('figure');
      const link = document.createElement('a');
      link.href = photoURL(photo.url);
      link.target = '_blank';
      link.rel = 'noopener';
      const image = document.createElement('img');
      image.src = link.href;
      image.alt = photo.alt_text || photo.caption || spot.name || '景点照片';
      image.loading = 'lazy';
      image.addEventListener('error', () => {
        figure.remove();
        gallery.hidden = !photos.childElementCount;
      }, { once: true });
      link.append(image);
      figure.append(link);
      if (photo.caption) {
        const caption = document.createElement('figcaption');
        caption.textContent = photo.caption;
        figure.append(caption);
      }
      photos.append(figure);
    });
    gallery.hidden = !entries.length;
  }

  function render(spot) {
    get('scenicInfoName').textContent = spot.name || '未命名景点';
    const meta = get('scenicInfoMeta');
    const status = { archived: '已归档', merged: '已合并' };
    meta.textContent = [spot.category, spot.scenic_grade, status[spot.status]].filter(Boolean).join(' · ');
    meta.hidden = !meta.textContent;
    get('scenicInfoCity').textContent = spot.city_name || '未标注';
    get('scenicInfoAddress').textContent = spot.address || '未填写';
    get('scenicInfoDescription').textContent = spot.description || '暂无简介';
    renderPhotos(spot);
    get('scenicInfoState').hidden = true;
    get('scenicInfoContent').hidden = false;
  }

  function clear() {
    request?.abort();
    request = null;
    currentSpot = null;
    get('scenicInfoState').hidden = false;
    get('scenicInfoContent').hidden = true;
    get('scenicInfoMeta').hidden = true;
    get('scenicInfoGallery').hidden = true;
    get('scenicInfoPhotos').replaceChildren();
    retry.hidden = true;
  }

  async function load(spotID, name = '景点详情') {
    clear();
    currentSpot = { spotID, name };
    get('scenicInfoName').textContent = name;
    if (!spotID || spotID.length > 128) {
      message.textContent = '景点编号无效。';
      return;
    }
    const controller = request = new AbortController();
    const { signal } = controller;
    message.textContent = '正在加载景点信息…';
    try {
      const response = await fetch('/admin/api/scenic-spots/' + encodeURIComponent(spotID), { cache: 'no-store', signal });
      if (signal.aborted) return;
      if (response.status === 404) { message.textContent = '未找到这个景点。'; return; }
      if (response.status === 403) { message.textContent = '当前账号没有景点查看权限。'; return; }
      if (response.status === 401) { message.textContent = '登录已失效，请重新登录后重试。'; return; }
      if (!response.ok) throw new Error('景点请求失败');
      const result = await response.json();
      if (signal.aborted) return;
      if (result.code !== 0 || !result.data) throw new Error('景点响应无效');
      render(result.data);
    } catch (_) {
      if (signal.aborted) return;
      message.textContent = '暂时无法加载景点信息。';
      retry.hidden = false;
    } finally {
      if (request === controller) request = null;
    }
  }

  retry.addEventListener('click', () => {
    if (currentSpot) load(currentSpot.spotID, currentSpot.name);
  });
  window.AdminScenicSpotDetail = { load, clear };
  if (document.querySelector('main.scenic-info-page')) {
    load((new URLSearchParams(location.search).get('spot_id') || '').trim());
  }
})();
