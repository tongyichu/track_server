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
  function fmtTime(s) {
    return esc((s || '').replace('T', ' ').slice(0, 19));
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
    var cur = cursorStack[pageIndex];
    if (cur) {
      params.set('cursor_start_time', cur.start_time);
      params.set('cursor_id', String(cur.id));
    }
    var resp = await fetch('/admin/api/tracks?' + params.toString());
    if (!resp.ok) {
      tbody.innerHTML = '<tr><td colspan="11">加载失败: ' + resp.status + '</td></tr>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    var items = data.items || [];
    totalCount = data.total || 0;
    lastNextCursor = data.next_cursor || null;

    tbody.innerHTML = '';
    items.forEach(function (it) {
      var tr = document.createElement('tr');
      var screenshotURL = safeImageURL(it.preview_images && it.preview_images.landscape_day_url);
      var screenshotCell = screenshotURL
        ? '<a class="track-shot" href="' + esc(screenshotURL) + '" target="_blank" rel="noopener"><img src="' + esc(screenshotURL) + '" alt="轨迹截图" loading="lazy"></a>'
        : '<span class="hint">无</span>';
      tr.innerHTML = '<td>' + screenshotCell + '</td>' +
        '<td>' + esc(it.id) + '</td>' +
        '<td>' + it.user_id + '</td>' +
        '<td>' + esc(it.title || '') + '</td>' +
        '<td>' + esc(typeNameByCode[it.track_type] || it.track_type || '') + '</td>' +
        '<td title="' + esc(it.city_code || '') + '">' + esc(cityNameByCode[it.city_code] || '未知城市') + '</td>' +
        '<td>' + (it.distance != null ? Number(it.distance).toFixed(1) : '') + '</td>' +
        '<td>' + (it.duration || 0) + '</td>' +
        '<td>' + esc(statusText(it.status)) + '</td>' +
        '<td>' + fmtTime(it.start_time) + '</td>' +
        '<td><button class="js-edit-track" data-track-id="' + esc(it.id) + '">编辑</button> ' +
        '<button class="danger js-delete-track" data-track-id="' + esc(it.id) + '">删除</button></td>';
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
      await editTrack(editBtn);
      return;
    }
    var btn = ev.target && ev.target.closest ? ev.target.closest('.js-delete-track') : null;
    if (!btn) return;
    var trackID = btn.getAttribute('data-track-id') || '';
    if (!trackID) return;
    if (!await AdminUI.confirm({
      title: '删除轨迹',
      message: '确认删除轨迹 ' + trackID + '？删除后会标记为已删除，并清理收藏和地图索引。',
      confirmText: '删除轨迹',
      danger: true
    })) {
      return;
    }
    btn.disabled = true;
    btn.textContent = '删除中';
    try {
      var resp = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), { method: 'DELETE' });
      if (!resp.ok) {
        var msg = '删除失败: ' + resp.status;
        try {
          var body = await resp.json();
          if (body && body.error) msg += ' ' + body.error;
        } catch (_) {}
        AdminUI.alert(msg, 'danger');
        btn.disabled = false;
        btn.textContent = '删除';
        return;
      }
      await loadPage();
    } catch (err) {
      AdminUI.alert('删除失败: ' + (err && err.message ? err.message : err), 'danger');
      btn.disabled = false;
      btn.textContent = '删除';
    }
  });

  async function editTrack(btn) {
    var trackID = btn.getAttribute('data-track-id') || '';
    if (!trackID) return;
    var detailResp = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID));
    if (!detailResp.ok) { AdminUI.alert('读取轨迹详情失败: ' + detailResp.status, 'danger'); return; }
    var track = ((await detailResp.json()).data) || {};
    var cities = cityOptions.slice();
    if (track.city_code && !cityNameByCode[track.city_code]) cities.push({ code: track.city_code, name: '未知城市 (' + track.city_code + ')' });
    var cityFields = [{ value: '', label: '未设置' }].concat(cities.map(function (city) { return { value: city.code, label: city.name }; }));
    var typeFields = [
      { value: 'hiking', label: '徒步' }, { value: 'running', label: '跑步' }, { value: 'climbing', label: '爬山' },
      { value: 'riding', label: '骑行' }, { value: 'driving', label: '自驾' }
    ];
    if (track.track_type && !typeNameByCode[track.track_type]) typeFields.push({ value: track.track_type, label: track.track_type });
    function timeValue(value) {
      if (!value) return '';
      var date = new Date(value);
      if (isNaN(date.getTime())) return '';
      var pad = function (num) { return String(num).padStart(2, '0'); };
      return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + 'T' + pad(date.getHours()) + ':' + pad(date.getMinutes());
    }
    var values = await AdminUI.form({
      title: '编辑轨迹 ' + trackID,
      confirmText: '保存修改',
      wide: true,
      fields: [
        { name: 'user_id', label: '用户 ID', type: 'number', value: track.user_id, min: 0 },
        { name: 'session_id', label: '同行会话 ID', value: track.session_id },
        { name: 'title', label: '轨迹标题', value: track.title, maxLength: 128 },
        { name: 'city_code', label: '城市', type: 'select', value: track.city_code, options: cityFields },
        { name: 'locate_addr', label: '位置描述', value: track.locate_addr, maxLength: 512 },
        { name: 'track_type', label: '轨迹类型', type: 'select', value: track.track_type, options: typeFields },
        { name: 'source_tag', label: '来源标签', value: track.source_tag, maxLength: 128 },
        { name: 'coordinate_system', label: '坐标系', value: track.coordinate_system, maxLength: 32 },
        { name: 'description', label: '轨迹描述', type: 'textarea', value: track.description, maxLength: 4000, full: true },
        { name: 'description_source', label: '描述来源', value: track.description_source, maxLength: 64 },
        { name: 'description_rule_version', label: '描述规则版本', type: 'number', value: track.description_rule_version, min: 0 },
        { name: 'start_time', label: '开始时间', type: 'datetime-local', value: timeValue(track.start_time), required: true },
        { name: 'end_time', label: '结束时间', type: 'datetime-local', value: timeValue(track.end_time) },
        { name: 'distance', label: '距离（米）', type: 'number', value: track.distance, min: 0, step: 'any' },
        { name: 'duration', label: '时长（秒）', type: 'number', value: track.duration, min: 0 },
        { name: 'calories_burned', label: '消耗热量（千卡）', type: 'number', value: track.calories_burned, min: 0, step: 'any' },
        { name: 'elevation_gain', label: '累计爬升（米）', type: 'number', value: track.elevation_gain },
        { name: 'avg_speed_kmh', label: '平均速度（公里/小时）', type: 'number', value: track.avg_speed_kmh, min: 0, step: 'any' },
        { name: 'is_running', label: '轨迹进行状态', type: 'select', value: track.is_running, options: [{ value: 'false', label: '已结束' }, { value: 'true', label: '进行中' }] },
        { name: 'status', label: '可见状态', type: 'select', value: track.status, options: [{ value: 1, label: '正常' }, { value: 2, label: '私密' }] },
        { name: 'raw_track_url', label: '原始轨迹文件 URL', value: track.raw_track_url, maxLength: 255, full: true },
        { name: 'track_screenshot_url', label: '旧版截图 URL', value: track.track_screenshot_url, maxLength: 255, full: true },
        { name: 'track_no_map_bg_screenshot_url', label: '无地图截图 URL', value: track.track_no_map_bg_screenshot_url, maxLength: 255, full: true },
        { name: 'preview_square_day', label: '方形日间预览图 URL', value: track.preview_images && track.preview_images.square_day_url, maxLength: 2048, full: true },
        { name: 'preview_square_night', label: '方形夜间预览图 URL', value: track.preview_images && track.preview_images.square_night_url, maxLength: 2048, full: true },
        { name: 'preview_landscape_day', label: '横版日间 4:3 预览图 URL', value: track.preview_images && track.preview_images.landscape_day_url, maxLength: 2048, full: true },
        { name: 'preview_landscape_night', label: '横版夜间 4:3 预览图 URL', value: track.preview_images && track.preview_images.landscape_night_url, maxLength: 2048, full: true }
      ]
    });
    if (!values) return;
    var body = {
      user_id: Number(values.user_id), session_id: values.session_id, title: values.title, city_code: values.city_code,
      locate_addr: values.locate_addr, track_type: values.track_type, source_tag: values.source_tag,
      coordinate_system: values.coordinate_system, description: values.description, description_source: values.description_source,
      description_rule_version: Number(values.description_rule_version || 0), start_time: values.start_time,
      end_time: values.end_time, distance: Number(values.distance || 0), duration: Number(values.duration || 0),
      calories_burned: Number(values.calories_burned || 0), elevation_gain: Number(values.elevation_gain || 0),
      avg_speed_kmh: Number(values.avg_speed_kmh || 0), is_running: values.is_running === 'true', status: Number(values.status),
      raw_track_url: values.raw_track_url, track_screenshot_url: values.track_screenshot_url,
      track_no_map_bg_screenshot_url: values.track_no_map_bg_screenshot_url,
      preview_images: {
        square_day_url: values.preview_square_day, square_night_url: values.preview_square_night,
        landscape_day_url: values.preview_landscape_day, landscape_night_url: values.preview_landscape_night
      }
    };
    btn.disabled = true;
    btn.textContent = '保存中';
    try {
      var resp = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      if (!resp.ok) {
        var msg = '保存失败: ' + resp.status;
        try {
          var body = await resp.json();
          if (body && body.error) msg += ' ' + body.error;
        } catch (_) {}
        AdminUI.alert(msg, 'danger');
        btn.disabled = false;
        btn.textContent = '编辑';
        return;
      }
      await loadPage();
    } catch (err) {
      AdminUI.alert('保存失败: ' + (err && err.message ? err.message : err), 'danger');
      btn.disabled = false;
      btn.textContent = '编辑';
    }
  }

  async function loadCities() {
    try {
      var resp = await fetch('/admin/api/cities');
      if (!resp.ok) throw new Error(String(resp.status));
      var data = ((await resp.json()).data) || [];
      cityOptions = data;
      data.forEach(function (city) { cityNameByCode[city.code] = city.name; });
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
  loadPage();
  loadStats(citiesLoaded);
})();
