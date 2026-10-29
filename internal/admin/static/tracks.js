(function () {
  fetch('/admin/api/me').then(function (r) {
    if (!r.ok) { window.location.href = '/admin/login.html'; return null; }
    return r.json();
  }).then(function (j) {
    if (j && j.data) {
      var el = document.getElementById('userName');
      if (el) el.textContent = '👤 ' + j.data.username;
    }
  });
  document.getElementById('logoutBtn').addEventListener('click', async function () {
    await fetch('/admin/api/logout', { method: 'POST' });
    window.location.href = '/admin/login.html';
  });

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function statusText(s) {
    if (s === 0) return '已删除';
    if (s === 1) return '正常';
    if (s === 2) return '私密';
    return String(s);
  }
  var cityOptions = [];
  var cityNameByCode = Object.create(null);
  var typeNameByCode = { hiking: '徒步', running: '跑步', climbing: '爬山', riding: '骑行', driving: '自驾' };
  var typeVisuals = {
    hiking: { color: '#16a34a', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m3 20 6-11 4 6 3-4 5 9H3Z"/><path d="m8 20 4-6 3 3"/></svg>' },
    running: { color: '#2563eb', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="14.5" cy="4.5" r="2"/><path d="m12 8-3 4 4 2 2 5m-6-7-4 1m7-5 4 3 3 1m-9 2-3 5"/></svg>' },
    climbing: { color: '#ea580c', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m3 20 7-14 4 8 2-4 5 10H3Z"/><path d="m9 20 3-6 3 2"/></svg>' },
    riding: { color: '#9333ea', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="6" cy="17" r="3.5"/><circle cx="18" cy="17" r="3.5"/><path d="m6 17 4-7h4l4 7m-8-7 4 7H6m5-10h3"/></svg>' },
    driving: { color: '#0f766e', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 11 1.5-5h11L19 11l2 2v5h-2v-2H5v2H3v-5l2-2Z"/><path d="M6.5 11h11M6 14h.01M18 14h.01"/></svg>' }
  };
  function safeImageURL(s) {
    s = String(s || '').trim();
    if (s.indexOf('/admin/api/static/') === 0 || s.indexOf('https://') === 0 || s.indexOf('http://') === 0) {
      return s;
    }
    return '';
  }

  var pageSize = 20;
  var cursorStack = [null];
  var pageIndex = 0;
  var lastNextCursor = null;
  var totalCount = 0;

  var tbody = document.getElementById('trackList');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var refreshBtn = document.getElementById('refreshBtn');
  var searchBtn = document.getElementById('searchBtn');
  var titleFilter = document.getElementById('titleFilter');
  var userNameFilter = document.getElementById('userNameFilter');
  var cityFilter = document.getElementById('cityFilter');
  var statusFilter = document.getElementById('statusFilter');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');

  refreshBtn.addEventListener('click', function () {
    cursorStack = [null];
    pageIndex = 0;
    loadPage();
  });
  function resetSearch() { cursorStack = [null]; pageIndex = 0; loadPage(); }
  searchBtn.addEventListener('click', resetSearch);
  titleFilter.addEventListener('keydown', function (event) { if (event.key === 'Enter') resetSearch(); });
  userNameFilter.addEventListener('keydown', function (event) { if (event.key === 'Enter') resetSearch(); });
  cityFilter.addEventListener('change', resetSearch);
  statusFilter.addEventListener('change', resetSearch);
  prevBtn.addEventListener('click', function () {
    if (pageIndex <= 0) return;
    pageIndex -= 1;
    cursorStack = cursorStack.slice(0, pageIndex + 1);
    loadPage();
  });
  nextBtn.addEventListener('click', function () {
    if (!lastNextCursor) return;
    pageIndex += 1;
    cursorStack[pageIndex] = lastNextCursor;
    loadPage();
  });

  async function loadPage() {
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
    if (titleFilter.value.trim()) params.set('title', titleFilter.value.trim());
    if (userNameFilter.value.trim()) params.set('user_name', userNameFilter.value.trim());
    if (cityFilter.value) params.set('city_code', cityFilter.value);
    if (statusFilter.value) params.set('status', statusFilter.value);
    var cur = cursorStack[pageIndex];
    if (cur) {
      params.set('cursor_start_time', cur.start_time);
      params.set('cursor_id', String(cur.id));
    }
    var resp = await fetch('/admin/api/tracks?' + params.toString());
    if (!resp.ok) {
      tbody.innerHTML = '<tr><td colspan="8">加载失败: ' + resp.status + '</td></tr>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    var items = data.items || [];
    totalCount = data.total || 0;
    lastNextCursor = data.next_cursor || null;

    tbody.innerHTML = '';
    items.forEach(function (it) {
      var tr = document.createElement('tr');
      var type = typeVisuals[it.track_type] || { color: '#64748b', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8"/><path d="M12 8v8m-4-4h8"/></svg>' };
      var screenshotURL = safeImageURL(it.preview_images && it.preview_images.landscape_day_url);
      var screenshotCell = screenshotURL
        ? '<a class="track-shot" href="' + esc(screenshotURL) + '" target="_blank" rel="noopener"><img src="' + esc(screenshotURL) + '" alt="轨迹图" loading="lazy"></a>'
        : '<span class="hint">无</span>';
      var avatarURL = safeImageURL(it.avatar_url);
      var avatar = avatarURL ? '<img class="track-avatar" src="' + esc(avatarURL) + '" alt="" loading="lazy">' : '<span class="track-avatar-fallback">' + esc((it.user_name || '用').slice(0, 1)) + '</span>';
      var statusClass = it.status === 1 ? 'bg-green-lt' : 'bg-secondary-lt';
      tr.innerHTML = '<td class="track-id">' + esc(it.id) + '</td>' +
        '<td title="' + esc(it.city_code || '') + '">' + esc(cityNameByCode[it.city_code] || '未知城市') + '</td>' +
        '<td class="track-title-cell">' + esc(it.title || '') + '</td>' +
        '<td><span class="track-type-badge" style="--track-type-color:' + type.color + '">' + type.icon + '<span>' + esc(typeNameByCode[it.track_type] || it.track_type || '未知') + '</span></span></td>' +
        '<td><span class="track-user">' + avatar + '<span>' + esc(it.user_name || '未知用户') + '</span></span></td>' +
        '<td>' + screenshotCell + '</td>' +
        '<td><span class="badge ' + statusClass + '">' + esc(statusText(it.status)) + '</span></td>' +
        '<td><button class="btn btn-sm btn-outline-primary js-edit-track" data-track-id="' + esc(it.id) + '">编辑</button></td>';
      tbody.appendChild(tr);
    });

    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
    totalHint.textContent = '共 ' + totalCount + ' 条轨迹（不含已删除）';
  }

  tbody.addEventListener('click', async function (ev) {
    var editBtn = ev.target && ev.target.closest ? ev.target.closest('.js-edit-track') : null;
    if (editBtn) {
      try {
        await editTrack(editBtn.getAttribute('data-track-id'), editBtn);
      } catch (err) {
        AdminUI.alert('读取轨迹失败: ' + (err.message || err), 'danger');
      }
    }
  });

  async function editTrack(trackID, btn) { return window.AdminTrackEditor.open(trackID, { onSaved: loadPage, onDeleted: loadPage }); }

  async function loadCities() {
    try {
      var resp = await fetch('/admin/api/cities');
      if (!resp.ok) throw new Error(String(resp.status));
      var data = ((await resp.json()).data) || [];
      cityOptions = data;
      data.forEach(function (city) {
        cityNameByCode[city.code] = city.name;
        var option = document.createElement('option'); option.value = city.code; option.textContent = city.name; cityFilter.appendChild(option);
      });
      return data;
    } catch (err) {
      AdminUI.alert('城市列表加载失败: ' + (err.message || err), 'danger');
      return [];
    }
  }

  function renderDistribution(target, items, cityLabels) {
    var body = document.getElementById(target);
    if (!items || !items.length) { body.innerHTML = '<tr><td colspan="2" class="hint">本月暂无数据</td></tr>'; return; }
    body.innerHTML = items.map(function (item) {
      var label = item.label || (cityLabels ? (cityNameByCode[item.key] || '未知城市') : (typeNameByCode[item.key] || item.key || '未知'));
      return '<tr><td>' + esc(label) + '</td><td>' + Number(item.count || 0).toLocaleString() + '</td></tr>';
    }).join('');
  }

  async function loadStats(citiesLoaded) {
    var typeBody = document.getElementById('typeStats');
    var cityBody = document.getElementById('cityStats');
    try {
      var resp = await fetch('/admin/api/dashboard?period=month');
      if (!resp.ok) throw new Error(String(resp.status));
      var data = ((await resp.json()).data) || {};
      if (citiesLoaded) await citiesLoaded;
      var content = data.content || {};
      renderDistribution('typeStats', content.types || [], false);
      renderDistribution('cityStats', content.cities || [], true);
    } catch (err) {
      typeBody.innerHTML = cityBody.innerHTML = '<tr><td colspan="2">统计加载失败</td></tr>';
    }
  }

  var citiesLoaded = loadCities();
  citiesLoaded.then(loadPage);
  loadStats(citiesLoaded);
  var linkedTrackID = new URLSearchParams(window.location.search).get('track_id');
  if (linkedTrackID) {
    if (/^NO\.[0-9A-Z]{8}$/.test(linkedTrackID)) {
      citiesLoaded.then(function () { return editTrack(linkedTrackID); }).catch(function (err) {
        AdminUI.alert('读取轨迹失败: ' + (err.message || err), 'danger');
      });
    } else {
      AdminUI.alert('轨迹 ID 无效', 'danger');
    }
  }
})();
