(function () {

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function statusText(s) {
    if (s === 0) return '已下架';
    if (s === 1) return '公开';
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

  function trackCells(it) {
    var type = typeVisuals[it.track_type] || { color: '#64748b', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8"/><path d="M12 8v8m-4-4h8"/></svg>' };
    var screenshotURL = safeImageURL(it.preview_images && it.preview_images.landscape_day_url);
    var screenshotCell = screenshotURL
        ? '<a class="track-shot" href="' + esc(screenshotURL) + '" target="_blank" rel="noopener"><img src="' + esc(screenshotURL) + '" alt="轨迹图" loading="lazy"></a>'
        : '<span class="hint">无</span>';
    var avatarURL = safeImageURL(it.avatar_url);
    var avatar = avatarURL ? '<img class="track-avatar" src="' + esc(avatarURL) + '" alt="" loading="lazy">' : '<span class="track-avatar-fallback">' + esc((it.user_name || '用').slice(0, 1)) + '</span>';
    var statusClass = it.status === 1 ? 'bg-green-lt' : it.status === 0 ? 'bg-red-lt' : 'bg-secondary-lt';
    return '<td class="track-id">' + esc(it.id) + '</td>' +
        '<td title="' + esc(it.city_code || '') + '">' + esc(trackCityName(it)) + '</td>' +
        '<td class="track-title-cell">' + esc(it.title || '') + '</td>' +
        '<td><span class="track-type-badge" style="--track-type-color:' + type.color + '">' + type.icon + '<span>' + esc(typeNameByCode[it.track_type] || it.track_type || '未知') + '</span></span></td>' +
        '<td><span class="track-user">' + avatar + '<span>' + esc(it.user_name || '未知用户') + '</span></span></td>' +
        '<td>' + screenshotCell + '</td>' +
        '<td><span class="badge ' + statusClass + '">' + esc(statusText(it.status)) + '</span></td>' +
        '<td><button class="btn btn-sm btn-outline-primary js-edit-track" data-track-id="' + esc(it.id) + '">' + (AdminAccess.can('tracks', true) ? '编辑' : '查看') + '</button></td>';
  }

  function trackCityName(track) {
    var code = String(track.city_code || '').trim();
    var name = String(track.city_name || '').trim() || cityNameByCode[code];
    if (name) return name;
    return code ? '未匹配城市（' + code + '）' : '未填写城市';
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
  var trackTypeFilter = document.getElementById('trackTypeFilter');
  var cityFilter = document.getElementById('cityFilter');
  var provinceFilter = document.getElementById('provinceFilter');
  var statusFilter = document.getElementById('statusFilter');
  var missingFilter = document.getElementById('missingFilter');
  var pageRequest = 0;
  var pageController = null;
  var renderedTrackRows = new Map();
  var trackTypeOptionsKey = '';
  var missingCountsKey = '';
  var missingLabels = { '': '全部', place_candidate: '待确认景点', scenic_spot: '未关联景点', description: '缺简介', city: '缺城市', previews: '缺预览图' };
  function renderMissingCounts(counts) {
    Array.from(missingFilter.options).forEach(function (option) {
      var value = counts && counts[option.value || 'all'];
      var count = typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value.toLocaleString() : '—';
      var label = missingLabels[option.value] + '（' + count + '）';
      if (option.textContent !== label) option.textContent = label;
    });
  }
  var trackListError = document.getElementById('trackListError');
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
  trackTypeFilter.addEventListener('change', resetSearch);
  cityFilter.addEventListener('change', resetSearch);
  provinceFilter.addEventListener('change', function () {
    populateCityOptions(cityOptions, provinceFilter.value);
    resetSearch();
  });
  statusFilter.addEventListener('change', resetSearch);
  missingFilter.addEventListener('change', resetSearch);
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

  function populateTrackTypeOptions(types) {
    if (!Array.isArray(types)) return;
    var key = JSON.stringify(types);
    if (key === trackTypeOptionsKey) return;
    var selected = trackTypeFilter.value;
    var options = [new Option('全部运动类型', '')];
    types.forEach(function (type) {
      if (!type.code || !type.name) return;
      typeNameByCode[type.code] = type.name;
      options.push(new Option(type.name, type.code));
    });
    trackTypeFilter.replaceChildren.apply(trackTypeFilter, options);
    trackTypeFilter.value = options.some(function (option) { return option.value === selected; }) ? selected : '';
    trackTypeFilter.disabled = options.length <= 1;
    trackTypeOptionsKey = key;
  }

  function renderTrackRows(items) {
    var rows = items.map(function (item) {
      var markup = trackCells(item), cached = renderedTrackRows.get(item.id);
      if (cached && cached.markup === markup) return cached.row;
      var row = document.createElement('tr'); row.innerHTML = markup;
      renderedTrackRows.set(item.id, { markup: markup, row: row }); return row;
    });
    var ids = new Set(items.map(function (item) { return item.id; }));
    renderedTrackRows.forEach(function (_, id) { if (!ids.has(id)) renderedTrackRows.delete(id); });
    if (!rows.length) {
      var empty = document.createElement('tr'), cell = document.createElement('td');
      cell.colSpan = 8; cell.textContent = '当前筛选条件下暂无轨迹'; empty.appendChild(cell); rows.push(empty);
    }
    var wanted = new Set(rows);
    Array.from(tbody.children).forEach(function (row) { if (!wanted.has(row)) row.remove(); });
    rows.forEach(function (row, index) { if (tbody.children[index] !== row) tbody.insertBefore(row, tbody.children[index] || null); });
  }

  async function loadPage() {
    var request = ++pageRequest;
    if (pageController) pageController.abort();
    var controller = new AbortController(); pageController = controller;
    prevBtn.disabled = true; nextBtn.disabled = true;
    trackListError.hidden = true; trackListError.textContent = '';
    var hints = { scenic_spot: '未关联景点包含正常的非景区轨迹，可结合其他条件筛选。', previews: '四种日夜预览图中任意一张缺失，即视为预览图不完整。' };
    var hint = document.getElementById('missingHint');
    hint.textContent = hints[missingFilter.value] || '';
    hint.hidden = !hint.textContent;
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
    if (titleFilter.value.trim()) params.set('title', titleFilter.value.trim());
    if (userNameFilter.value.trim()) params.set('user_name', userNameFilter.value.trim());
    if (trackTypeFilter.value) params.set('track_type', trackTypeFilter.value);
    if (provinceFilter.value) params.set('province_code', provinceFilter.value);
    if (cityFilter.value) params.set('city_code', cityFilter.value);
    if (statusFilter.value) params.set('status', statusFilter.value);
    var countsKey = params.toString();
    if (missingCountsKey !== countsKey) { renderMissingCounts(null); document.getElementById('missingCountsError').hidden = true; }
    if (missingFilter.value) params.set('missing', missingFilter.value);
    var cur = cursorStack[pageIndex];
    if (cur) {
      params.set('cursor_start_time', cur.start_time);
      params.set('cursor_id', String(cur.id));
    }
    try {
      var resp = await fetch('/admin/api/tracks?' + params.toString(), { signal: controller.signal });
      var payload = await resp.json().catch(function () { return {}; });
      if (request !== pageRequest || controller.signal.aborted) return;
      if (!resp.ok) throw new Error(payload.error || String(resp.status));
      var data = payload.data || {}, items = data.items || [];
      populateTrackTypeOptions(data.track_types);
      renderMissingCounts(data.missing_counts);
      missingCountsKey = countsKey;
      document.getElementById('missingCountsError').hidden = !data.missing_counts_unavailable;
      totalCount = data.total || 0;
      lastNextCursor = data.next_cursor || null;
      renderTrackRows(items);
      prevBtn.disabled = pageIndex <= 0;
      nextBtn.disabled = !lastNextCursor;
      pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
      totalHint.textContent = statusFilter.value === '0'
        ? '共 ' + totalCount + ' 条已下架轨迹'
        : '共 ' + totalCount + ' 条轨迹（不含已下架）';
    } catch (error) {
      if (request !== pageRequest || controller.signal.aborted) return;
      trackListError.textContent = '加载失败，已保留上次列表；请搜索或刷新重试：' + (error.message || error);
      trackListError.hidden = false;
    } finally { if (request === pageRequest) pageController = null; }
  }

  window.addEventListener('pagehide', function () { if (pageController) pageController.abort(); });

  async function handleTrackEdit(ev) {
    var editBtn = ev.target && ev.target.closest ? ev.target.closest('.js-edit-track') : null;
    if (editBtn) {
      try {
        await editTrack(editBtn.getAttribute('data-track-id'));
      } catch (err) {
        AdminUI.alert('读取轨迹失败: ' + (err.message || err), 'danger');
      }
    }
  }
  tbody.addEventListener('click', handleTrackEdit);
  document.getElementById('trackImportResults').addEventListener('click', handleTrackEdit);

  var importResults = [];
  function renderImportResults() {
    var node = document.getElementById('trackImportResults');
    var okCount = importResults.filter(function (r) { return !r.error; }).length;
    node.innerHTML = '<p class="hint">共 ' + importResults.length + ' 个文件，成功 ' + okCount + ' 个，失败 ' + (importResults.length - okCount) + ' 个。</p>' +
      '<div class="table-responsive"><table class="data table track-import-result-table"><thead><tr>' +
      '<th>文件名</th><th>ID</th><th>城市</th><th>标题</th><th>类型</th><th>用户名</th><th>轨迹图</th><th>状态</th><th>操作</th><th>导入结果</th>' +
      '</tr></thead><tbody>' + importResults.map(function (r) {
        var cells = r.error
          ? '<td colspan="8" data-label="错误原因" class="track-import-result-fail">' + esc(r.error) + '</td>'
          : r.removed ? '<td colspan="8" data-label="轨迹" class="hint">轨迹已彻底删除</td>' : trackCells(r.track);
        return '<tr><td class="track-import-file-name">' + esc(r.file_name) + '</td>' + cells +
          '<td data-label="导入结果"><span class="badge ' + (r.error ? 'bg-red-lt' : 'bg-green-lt') + '">' + (r.error ? '失败' : '成功') + '</span></td></tr>';
      }).join('') + '</tbody></table></div>';
    node.hidden = false;
  }

  async function refreshImportedTrack(trackID) {
    var result = importResults.find(function (r) { return !r.error && r.track_id === trackID; });
    if (!result) return;
    try {
      var response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID));
      if (importResults.indexOf(result) < 0) return;
      if (response.status === 404) {
        result.removed = true;
      } else {
        if (!response.ok) throw new Error(String(response.status));
        var data = ((await response.json()).data) || {};
        if (importResults.indexOf(result) < 0) return;
        result.track = Object.assign({}, result.track, data.track);
        result.track.city_name = data.city_name || '';
        var previews = result.track.preview_images || {};
        result.track.preview_images = Object.assign({}, previews, {
          landscape_day_url: (previews.landscape_day_url || '').replace(/^\/api\/v1\/static\//, '/admin/api/static/')
        });
      }
      renderImportResults();
    } catch (error) {
      AdminUI.alert('导入结果刷新失败，请重新打开轨迹核实: ' + (error.message || error), 'warning');
    }
  }

  async function editTrack(trackID) {
    function refreshLists() { return Promise.all([loadPage(), refreshImportedTrack(trackID)]); }
    return window.AdminTrackEditor.open(trackID, { cities: cityOptions, onSaved: refreshLists, onDeleted: refreshLists });
  }

  function populateCityOptions(regions, provinceCode) {
    cityFilter.replaceChildren(new Option('全部城市', ''));
    regions.filter(function (item) {
      return item.level === 'city' && (!provinceCode || item.province_code === provinceCode);
    }).forEach(function (city) {
      cityFilter.add(new Option(city.name, city.code));
    });
  }

  async function loadCities() {
    try {
      var resp = await fetch('/admin/api/cities');
      if (!resp.ok) throw new Error(String(resp.status));
      var data = ((await resp.json()).data) || [];
      cityOptions = data;
      data.forEach(function (item) { cityNameByCode[item.code] = item.name; });
      data.filter(function (item) { return item.level === 'province'; })
        .forEach(function (item) { provinceFilter.add(new Option(item.name, item.code)); });
      populateCityOptions(data, '');
      AdminRegionPicker.enhance(provinceFilter, cityFilter, data);
      return data;
    } catch (err) {
      AdminUI.alert('城市列表加载失败: ' + (err.message || err), 'danger');
      return [];
    }
  }

  var citiesLoaded = loadCities();
  citiesLoaded.then(loadPage);
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

  // ── 批量导入 ────────────────────────────────────────────────
  (function setupTrackImport() {
    if (!document.getElementById('trackImportPanel')) return;
    var ownerToggle = document.getElementById('trackImportOwnerToggle');
    var ownerCurrent = document.getElementById('trackImportOwnerCurrent');
    var ownerMenu = document.getElementById('trackImportOwnerMenu');
    var typeSelect = document.getElementById('trackImportType');
    var coordSelect = document.getElementById('trackImportCoord');
    var fileInput = document.getElementById('trackImportFiles');
    var fileHint = document.getElementById('trackImportFileHint');
    var submitBtn = document.getElementById('trackImportSubmit');
    var errorNode = document.getElementById('trackImportError');
    var resultsNode = document.getElementById('trackImportResults');
    var selectedOwner = null;
    var ownersLoaded = false;
    var importing = false;

    function ownerHTML(user) {
      var avatar = user && user.avatar_url ? '<img src="' + esc(user.avatar_url) + '" alt="" class="companion-avatar" />' : '';
      return '<span class="track-import-owner-option">' + avatar + '<span>' + esc(user ? user.nickname || ('用户 ' + user.id) : '') + '</span></span>';
    }
    function updateSubmitState() {
      submitBtn.disabled = importing || !selectedOwner || !fileInput.files.length;
    }
    function closeOwnerMenu() {
      ownerMenu.hidden = true;
      ownerToggle.setAttribute('aria-expanded', 'false');
    }
    function loadOwners() {
      if (ownersLoaded) return Promise.resolve();
      ownerMenu.innerHTML = '<li class="hint">正在加载内部人员…</li>';
      return fetch('/admin/api/users?is_internal=true&limit=100')
        .then(function (resp) { if (!resp.ok) throw new Error(String(resp.status)); return resp.json(); })
        .then(function (payload) {
          ownersLoaded = true;
          var items = (payload.data && payload.data.items) || [];
          ownerMenu.innerHTML = '';
          if (!items.length) {
            ownerMenu.innerHTML = '<li class="hint">暂无内部人员，请先在用户管理标记。</li>';
            return;
          }
          items.forEach(function (user) {
            var li = document.createElement('li');
            li.setAttribute('role', 'option');
            li.innerHTML = ownerHTML(user);
            li.addEventListener('click', function () {
              selectedOwner = user;
              ownerCurrent.innerHTML = ownerHTML(user);
              closeOwnerMenu();
              updateSubmitState();
            });
            ownerMenu.appendChild(li);
          });
        })
        .catch(function () {
          ownerMenu.innerHTML = '<li class="hint">内部人员加载失败，请重新打开。</li>';
        });
    }
    ownerToggle.addEventListener('click', function () {
      var open = ownerMenu.hidden;
      closeOwnerMenu();
      if (open) {
        ownerMenu.hidden = false;
        ownerToggle.setAttribute('aria-expanded', 'true');
        loadOwners();
      }
    });
    document.addEventListener('click', function (event) {
      if (!ownerMenu.hidden && !document.getElementById('trackImportOwner').contains(event.target)) closeOwnerMenu();
    });

    fileInput.addEventListener('change', function () {
      var count = fileInput.files.length;
      fileHint.textContent = count ? '已选择 ' + count + ' 个文件' : '未选择文件';
      updateSubmitState();
    });
    submitBtn.addEventListener('click', function () {
      if (importing || !selectedOwner || !fileInput.files.length) return;
      if (fileInput.files.length > 50) {
        errorNode.textContent = '单次最多导入 50 个文件。';
        errorNode.hidden = false;
        return;
      }
      importing = true;
      updateSubmitState();
      submitBtn.textContent = '正在导入…';
      errorNode.hidden = true;
      resultsNode.hidden = true;
      importResults = [];
      var importOwner = selectedOwner;
      var importType = typeSelect.value;
      var form = new FormData();
      Array.from(fileInput.files).forEach(function (file) { form.append('files', file, file.name); });
      form.append('owner_user_id', String(importOwner.id));
      form.append('track_type', importType);
      form.append('coordinate_system', coordSelect.value);
      fetch('/admin/api/tracks/import', { method: 'POST', body: form })
        .then(function (resp) {
          return resp.json().then(function (payload) {
            if (!resp.ok) throw new Error((payload && payload.error) || ('请求失败（' + resp.status + '）'));
            return payload;
          });
        })
        .then(function (payload) {
          var results = (payload.data && payload.data.results) || [];
          var okCount = results.filter(function (r) { return !r.error; }).length;
          importResults = results.map(function (r) {
            if (!r.error) {
              r.track = Object.assign({ id: r.track_id, title: r.title, track_type: importType }, r.track, {
                user_name: importOwner.nickname || '未知用户', avatar_url: importOwner.avatar_url
              });
            }
            return r;
          });
          renderImportResults();
          if (okCount) {
            cursorStack = [null];
            pageIndex = 0;
            loadPage();
          }
        })
        .catch(function (err) {
          errorNode.textContent = '导入失败：' + (err.message || err);
          errorNode.hidden = false;
        })
        .finally(function () {
          importing = false;
          submitBtn.textContent = '开始导入';
          updateSubmitState();
        });
    });
  })();
})();
