(function () {
  'use strict';
  const rows = document.getElementById('placeCandidateRows');
  const refreshButton = document.getElementById('refreshPlaceCandidatesBtn');
  const moreButton = document.getElementById('loadMorePlaceCandidatesBtn');
  const typeNames = { hiking: '徒步', running: '跑步', climbing: '爬山', riding: '骑行', driving: '自驾' };
  let cursor = '';
  let loading = false;
  let cityNames = null;

  async function request(path, options) {
    const response = await fetch('/admin/api' + path, Object.assign({ credentials: 'same-origin' }, options || {}));
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.code && payload.code !== 0) throw new Error(payload.error || ('请求失败：' + response.status));
    return payload.data;
  }
  function message(text, tone) {
    if (window.AdminUI) window.AdminUI.alert(text, tone || 'success');
    else window.alert(text);
  }
  async function loadCities() {
    if (cityNames) return cityNames;
    try {
      const cities = await request('/cities');
      cityNames = Object.fromEntries((cities || []).map(city => [city.code, city.name]));
    } catch (_) {
      cityNames = {};
    }
    return cityNames;
  }
  function timeText(value) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false });
  }
  function setRowCount() {
    const count = rows.querySelectorAll('tr[data-track-id]').length;
    const title = document.querySelector('#placeCandidatePanel h2');
    if (title) title.textContent = count ? '景点识别候选（' + count + '）' : '景点识别候选';
  }
  function appendCandidate(item) {
    const row = document.createElement('tr');
    row.dataset.trackId = item.track_id;

    const trackCell = document.createElement('td');
    const trackButton = document.createElement('button');
    trackButton.type = 'button';
    trackButton.className = 'btn btn-sm btn-outline-primary';
    trackButton.textContent = item.title || item.track_id;
    trackButton.title = item.track_id;
    trackButton.addEventListener('click', () => {
      window.AdminTrackEditor.open(item.track_id, {}).catch(error => message('读取轨迹失败：' + error.message, 'danger'));
    });
    const meta = document.createElement('div');
    meta.className = 'scenic-hint mt-1';
    meta.textContent = [item.track_id, (cityNames && cityNames[item.city_code]) || '未知城市', typeNames[item.track_type] || item.track_type || '未知类型'].join(' · ');
    trackCell.append(trackButton, meta);

    const placesCell = document.createElement('td');
    const list = document.createElement('div');
    list.className = 'd-flex gap-2 flex-wrap';
    (item.places || []).forEach(place => {
      const confirmButton = document.createElement('button');
      confirmButton.type = 'button';
      confirmButton.className = 'btn btn-sm btn-outline-success';
      confirmButton.textContent = '确认：' + (place.name || place.provider_id || place.id);
      confirmButton.title = [place.category, place.provider, place.provider_id].filter(Boolean).join(' · ');
      confirmButton.addEventListener('click', async () => {
        confirmButton.disabled = true;
        try {
          await request('/place-candidates/' + encodeURIComponent(item.track_id) + '/confirm', {
            method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ place_id: place.id })
          });
          row.remove();
          setRowCount();
          message('已确认景点：' + (place.name || place.id));
        } catch (error) {
          confirmButton.disabled = false;
          message('确认失败：' + error.message, 'danger');
        }
      });
      list.appendChild(confirmButton);
    });
    placesCell.appendChild(list);

    const timeCell = document.createElement('td');
    timeCell.textContent = timeText(item.updated_at);
    const actionCell = document.createElement('td');
    const dismiss = document.createElement('button');
    dismiss.type = 'button';
    dismiss.className = 'btn btn-sm btn-outline-secondary';
    dismiss.textContent = '忽略';
    dismiss.addEventListener('click', async () => {
      const confirmed = window.AdminUI ? await window.AdminUI.confirm({ title: '忽略候选', message: '确定忽略 ' + item.track_id + ' 的景点候选？', confirmText: '忽略' }) : window.confirm('确定忽略这条候选？');
      if (!confirmed) return;
      dismiss.disabled = true;
      try {
        await request('/place-candidates/' + encodeURIComponent(item.track_id) + '/dismiss', { method: 'POST' });
        row.remove();
        setRowCount();
      } catch (error) {
        dismiss.disabled = false;
        message('忽略失败：' + error.message, 'danger');
      }
    });
    actionCell.appendChild(dismiss);
    row.append(trackCell, placesCell, timeCell, actionCell);
    rows.appendChild(row);
  }
  async function load(reset) {
    if (loading) return;
    loading = true;
    refreshButton.disabled = moreButton.disabled = true;
    if (reset) {
      cursor = '';
      rows.innerHTML = '<tr><td colspan="4" class="p-3 scenic-hint">正在加载候选…</td></tr>';
      moreButton.hidden = true;
    }
    try {
      await loadCities();
      const params = new URLSearchParams({ limit: '20' });
      if (cursor) params.set('cursor', cursor);
      const data = await request('/place-candidates?' + params, { cache: 'no-store' });
      if (reset) rows.replaceChildren();
      (data.items || []).forEach(appendCandidate);
      cursor = data.next_cursor || '';
      moreButton.hidden = !data.has_more || !cursor;
      if (!rows.querySelector('tr[data-track-id]')) {
        rows.innerHTML = '<tr><td colspan="4" class="p-3 scenic-hint">当前没有待确认候选</td></tr>';
      }
      setRowCount();
    } catch (error) {
      if (reset) rows.innerHTML = '<tr><td colspan="4" class="p-3 scenic-hint">加载失败：' + error.message + '</td></tr>';
      message('候选加载失败：' + error.message, 'danger');
    } finally {
      loading = false;
      refreshButton.disabled = moreButton.disabled = false;
    }
  }

  refreshButton.addEventListener('click', () => load(true));
  moreButton.addEventListener('click', () => load(false));
  load(true);
})();
