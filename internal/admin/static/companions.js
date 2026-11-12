(function () {

  function esc(s) {
    return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function fmtTime(s) {
    return AdminTime.dateTime(s);
  }
  var labels = {
    active: '进行中', ended: '已结束', public: '公开', private: '私密',
    hiking: '徒步', running: '跑步', climbing: '爬山', riding: '骑行', driving: '自驾',
    owner: '房主', member: '成员', joined: '已加入', left: '已离开', kicked: '已移出',
    online: '在线', offline: '离线', auto_close: '自动结束', member_flow: '成员离开',
    owner_ended: '房主结束', inactive_timeout: '长时间无活动',
    max_duration_exceeded: '达到最长持续时间', all_members_left: '所有成员已离开'
  };
  function label(value) { return labels[value] || value || '—'; }
  function badge(value) {
    var color = value === 'active' || value === 'online' ? 'bg-green-lt text-green' : 'bg-secondary-lt text-secondary';
    return '<span class="badge ' + color + '">' + esc(label(value)) + '</span>';
  }
  var typeVisuals = {
    hiking: { color: '#16a34a', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m3 20 6-11 4 6 3-4 5 9H3Z"/><path d="m8 20 4-6 3 3"/></svg>' },
    running: { color: '#2563eb', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="14.5" cy="4.5" r="2"/><path d="m12 8-3 4 4 2 2 5m-6-7-4 1m7-5 4 3 3 1m-9 2-3 5"/></svg>' },
    climbing: { color: '#ea580c', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m3 20 7-14 4 8 2-4 5 10H3Z"/><path d="m9 20 3-6 3 2"/></svg>' },
    riding: { color: '#9333ea', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="6" cy="17" r="3.5"/><circle cx="18" cy="17" r="3.5"/><path d="m6 17 4-7h4l4 7m-8-7 4 7H6m5-10h3"/></svg>' },
    driving: { color: '#0f766e', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 11 1.5-5h11L19 11l2 2v5h-2v-2H5v2H3v-5l2-2Z"/><path d="M6.5 11h11M6 14h.01M18 14h.01"/></svg>' }
  };
  var fallbackTypeVisual = { color: '#64748b', icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8"/><path d="M12 8v8m-4-4h8"/></svg>' };
  function typeBadge(trackType) {
    var type = typeVisuals[trackType] || fallbackTypeVisual;
    return '<span class="track-type-badge" style="--track-type-color:' + type.color + '">' + type.icon + '<span>' + esc(label(trackType)) + '</span></span>';
  }
  function fmtDuration(start, end) {
    var startTime = new Date(start).getTime();
    if (Number.isNaN(startTime)) return '—';
    var endTime = end && !String(end).startsWith('0001-') ? new Date(end).getTime() : Date.now();
    if (Number.isNaN(endTime) || endTime < startTime) return '—';
    var minutes = Math.floor((endTime - startTime) / 60000);
    var days = Math.floor(minutes / 1440), hours = Math.floor((minutes % 1440) / 60), mins = minutes % 60;
    var text = days > 0 ? days + ' 天 ' + hours + ' 小时' : (hours > 0 ? hours + ' 小时 ' + mins + ' 分' : mins + ' 分钟');
    return end && !String(end).startsWith('0001-') ? text : text + '（进行中）';
  }
  function safeImageURL(s) {
    s = String(s || '').trim();
    return (s.indexOf('/admin/api/static/') === 0 || s.indexOf('https://') === 0 || s.indexOf('http://') === 0) ? s : '';
  }
  async function apiJSON(url) {
    var response = await fetch(url);
    if (response.status === 401) { window.location.href = '/admin/login.html'; throw new Error('登录已失效'); }
    var payload = await response.json().catch(function () { return {}; });
    if (!response.ok) throw new Error(payload.error || ('请求失败（' + response.status + '）'));
    return payload.data || {};
  }

  var statsRequest = 0;
  async function loadCompanionStatistics() {
    var request = ++statsRequest;
    var errorNode = document.getElementById('companionStatsError');
    errorNode.hidden = true;
    try {
      var payload = await apiJSON('/admin/api/companions/statistics?period=month');
      if (request !== statsRequest) return;
      var stats = payload.stats || {};
      document.getElementById('companionActive').textContent = Number(stats.active || 0).toLocaleString();
      document.getElementById('companionCreated').textContent = Number((stats.created || {}).current || 0).toLocaleString();
      document.getElementById('companionCreatedCompare').textContent = '上期 ' + Number((stats.created || {}).previous || 0).toLocaleString();
      document.getElementById('companionEnded').textContent = Number(stats.ended || 0).toLocaleString();
      document.getElementById('companionAutoEnded').textContent = '其中自动结束 ' + Number(stats.auto_ended || 0).toLocaleString();
      var average = Number(stats.avg_participants || 0);
      document.getElementById('companionParticipants').textContent = average > 0 ? average.toFixed(1) : '—';
    } catch (_) {
      if (request !== statsRequest) return;
      ['companionActive', 'companionCreated', 'companionEnded', 'companionParticipants'].forEach(function (id) {
        document.getElementById(id).textContent = '—';
      });
      document.getElementById('companionCreatedCompare').textContent = '上期 —';
      document.getElementById('companionAutoEnded').textContent = '其中自动结束 —';
      errorNode.textContent = '统计加载失败，请点击“刷新数据”重试。';
      errorNode.hidden = false;
    }
  }

  var cursorStack = [null], pageIndex = 0, lastNextCursor = null, listRequest = 0;
  var applied = { title: '', ownerName: '', trackType: '', provinceCode: '', cityCode: '' };
  var tbody = document.getElementById('companionList');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var titleFilter = document.getElementById('titleFilter');
  var ownerNameFilter = document.getElementById('ownerNameFilter');
  var provinceFilter = document.getElementById('provinceFilter');
  var cityFilter = document.getElementById('cityFilter');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');
  var resetBtn = document.getElementById('resetBtn');
  var detailModal = document.getElementById('companionDetailModal');
  var detailRequest = 0, currentSessionID = '';

  // 运动类型下拉（icon + 文字）
  var selectedType = '';
  var typeToggle = document.getElementById('trackTypeToggle');
  var typeCurrent = document.getElementById('trackTypeCurrent');
  var typeMenu = document.getElementById('trackTypeMenu');
  function typeOptionHTML(code) {
    if (!code) return '<span class="companion-type-any">全部类型</span>';
    var type = typeVisuals[code] || fallbackTypeVisual;
    return '<span class="track-type-badge" style="--track-type-color:' + type.color + '">' + type.icon + '<span>' + esc(label(code)) + '</span></span>';
  }
  function renderTypeMenu() {
    typeMenu.innerHTML = '';
    [''].concat(Object.keys(typeVisuals)).forEach(function (code) {
      var li = document.createElement('li');
      li.setAttribute('role', 'option');
      li.setAttribute('aria-selected', String(code === selectedType));
      li.dataset.value = code;
      li.innerHTML = typeOptionHTML(code);
      li.addEventListener('click', function () { selectType(code); });
      typeMenu.appendChild(li);
    });
  }
  function selectType(code) {
    selectedType = code;
    typeCurrent.innerHTML = typeOptionHTML(code);
    closeTypeMenu();
  }
  function closeTypeMenu() {
    typeMenu.hidden = true;
    typeToggle.setAttribute('aria-expanded', 'false');
  }
  typeToggle.addEventListener('click', function () {
    var open = typeMenu.hidden;
    closeTypeMenu();
    if (open) {
      typeMenu.hidden = false;
      typeToggle.setAttribute('aria-expanded', 'true');
    }
  });
  document.addEventListener('click', function (event) {
    if (!typeMenu.hidden && !document.getElementById('trackTypeFilter').contains(event.target)) closeTypeMenu();
  });
  typeToggle.addEventListener('keydown', function (event) {
    if (event.key === 'Escape') closeTypeMenu();
  });
  renderTypeMenu();

  // 省份 / 城市级联（数据源 /admin/api/cities）
  var regionsPromise = null;
  function loadRegions() {
    if (!regionsPromise) {
      regionsPromise = apiJSON('/admin/api/cities').then(function (payload) {
        return Array.isArray(payload) ? payload : (payload.items || []);
      });
    }
    return regionsPromise;
  }
  function populateProvinceOptions(regions) {
    provinceFilter.replaceChildren(new Option('全部省份', ''));
    regions.filter(function (item) { return item.level === 'province'; })
      .forEach(function (item) { provinceFilter.add(new Option(item.name, item.code)); });
  }
  function populateCityOptions(regions, provinceCode) {
    cityFilter.replaceChildren(new Option('全部城市', ''));
    regions.filter(function (item) { return item.level === 'city' && item.province_code === provinceCode; })
      .forEach(function (item) { cityFilter.add(new Option(item.name, item.code)); });
    cityFilter.disabled = !provinceCode;
  }
  loadRegions().then(function (regions) {
    populateProvinceOptions(regions);
    populateCityOptions(regions, '');
    AdminRegionPicker.enhance(provinceFilter, cityFilter, regions, { label: '发起人所在地' });
  }).catch(function () { /* 省市选项加载失败时保留输入筛选可用 */ });
  provinceFilter.addEventListener('change', function () {
    loadRegions().then(function (regions) { populateCityOptions(regions, provinceFilter.value); }).catch(function () {});
  });

  function hasActiveFilter() {
    return !!(applied.title || applied.ownerName || applied.trackType || applied.provinceCode || applied.cityCode);
  }
  function pendingFilterChange() {
    return titleFilter.value.trim() !== applied.title || ownerNameFilter.value.trim() !== applied.ownerName ||
      selectedType !== applied.trackType || provinceFilter.value !== applied.provinceCode || cityFilter.value !== applied.cityCode;
  }
  function resetSearch() {
    applied = {
      title: titleFilter.value.trim(),
      ownerName: ownerNameFilter.value.trim(),
      trackType: selectedType,
      provinceCode: provinceFilter.value,
      cityCode: cityFilter.value
    };
    resetBtn.hidden = !hasActiveFilter();
    cursorStack = [null];
    pageIndex = 0;
    loadPage();
  }
  document.getElementById('companionSearch').addEventListener('submit', function (event) {
    event.preventDefault();
    resetSearch();
  });
  resetBtn.addEventListener('click', function () {
    titleFilter.value = '';
    ownerNameFilter.value = '';
    selectType('');
    provinceFilter.value = '';
    cityFilter.value = '';
    loadRegions().then(function (regions) { populateCityOptions(regions, ''); }).catch(function () {});
    resetSearch();
  });
  [titleFilter, ownerNameFilter].forEach(function (input) {
    input.addEventListener('input', function () { resetBtn.hidden = !hasActiveFilter() && !pendingFilterChange() && !input.value; });
  });
  document.getElementById('refreshBtn').addEventListener('click', function () {
    loadCompanionStatistics();
    if (window.AdminCompanionUsage) window.AdminCompanionUsage.refresh();
    cursorStack = [null];
    pageIndex = 0;
    loadPage();
  });
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

  function showEmpty(body, columns, message) {
    body.innerHTML = '<tr><td colspan="' + columns + '" class="companion-table-state">' + esc(message) + '</td></tr>';
  }
  async function loadPage() {
    var request = ++listRequest;
    var params = new URLSearchParams({ limit: '20' });
    if (applied.title) params.set('title', applied.title);
    if (applied.ownerName) params.set('owner_name', applied.ownerName);
    if (applied.trackType) params.set('track_type', applied.trackType);
    if (applied.provinceCode) params.set('province_code', applied.provinceCode);
    if (applied.cityCode) params.set('city_code', applied.cityCode);
    var cur = cursorStack[pageIndex];
    if (cur) {
      params.set('cursor_started_at', cur.started_at);
      params.set('cursor_session_id', cur.session_id);
    }
    prevBtn.disabled = nextBtn.disabled = true;
    lastNextCursor = null;
    totalHint.textContent = '';
    pageHint.textContent = '正在加载…';
    tbody.setAttribute('aria-busy', 'true');
    showEmpty(tbody, 8, '正在加载房间…');
    try {
      var data = await apiJSON('/admin/api/companions?' + params.toString());
      if (request !== listRequest) return;
      var items = data.items || [];
      lastNextCursor = data.next_cursor || null;
      tbody.replaceChildren();
      items.forEach(function (it) {
        var ownerAvatar = safeImageURL(it.owner_avatar_url);
        var tr = document.createElement('tr');
        tr.className = 'companion-row';
        tr.tabIndex = 0;
        tr.setAttribute('aria-label', '查看房间：' + (it.title || it.session_id));
        tr.innerHTML = '<td><div class="companion-room-cell"><strong>' + esc(it.title || '未命名房间') + '</strong></div></td>' +
          '<td>' + typeBadge(it.track_type) + '</td>' +
          '<td><div class="companion-user">' + (ownerAvatar ? '<img src="' + esc(ownerAvatar) + '" alt="" class="companion-avatar" />' : '') +
          '<span><strong>' + esc(it.owner_name || '未知用户') + '</strong></span></div></td>' +
          '<td><div class="companion-cell-stack">' + badge(it.status) + '<small>' + esc(label(it.visibility)) + '房间</small></div></td>' +
          '<td>' + esc(fmtTime(it.started_at)) + '</td>' +
          '<td>' + esc(fmtDuration(it.started_at, it.status === 'active' ? null : it.ended_at)) + '</td>' +
          '<td>' + esc(Number(it.member_count || 0) + ' / ' + (it.max_members || 0)) + '</td>' +
          '<td><button type="button" class="btn btn-sm btn-outline-primary companion-view">查看详情</button></td>';
        tr.addEventListener('click', function () { loadDetail(it.session_id); });
        tr.addEventListener('keydown', function (event) {
          if (event.target === tr && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); loadDetail(it.session_id); }
        });
        tr.querySelector('.companion-view').addEventListener('click', function (event) { event.stopPropagation(); loadDetail(it.session_id); });
        tbody.appendChild(tr);
      });
      if (!items.length) showEmpty(tbody, 8, hasActiveFilter() ? '未找到匹配的房间，请调整筛选条件。' : '暂无同行房间。');
      nextBtn.disabled = !lastNextCursor;
      totalHint.textContent = '共 ' + Number(data.total || 0).toLocaleString() + ' 个房间';
      pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页 · 本页 ' + items.length + ' 个房间';
    } catch (error) {
      if (request !== listRequest) return;
      showEmpty(tbody, 8, '加载失败：' + error.message + '。请点击“刷新数据”重试。');
      pageHint.textContent = '加载失败';
    } finally {
      if (request === listRequest) {
        tbody.removeAttribute('aria-busy');
        prevBtn.disabled = pageIndex <= 0;
      }
    }
  }

  function setDetailRows(id, items) {
    var container = document.getElementById(id);
    container.replaceChildren();
    items.forEach(function (item) {
      var cell = document.createElement('div'), name = document.createElement('strong'), value = document.createElement('span');
      name.textContent = item[0];
      value.textContent = item[1] == null || item[1] === '' ? '—' : String(item[1]);
      cell.append(name, value);
      container.appendChild(cell);
    });
  }
  var tabs = Array.from(document.querySelectorAll('[data-companion-tab]'));
  function selectTab(tab) {
    tabs.forEach(function (item) {
      var selected = item === tab;
      item.setAttribute('aria-selected', String(selected));
      item.tabIndex = selected ? 0 : -1;
    });
    document.querySelectorAll('[data-companion-panel]').forEach(function (panel) {
      panel.hidden = panel.dataset.companionPanel !== tab.dataset.companionTab;
    });
  }
  tabs.forEach(function (tab, index) {
    tab.addEventListener('click', function () { selectTab(tab); });
    tab.addEventListener('keydown', function (event) {
      var target;
      if (event.key === 'ArrowRight') target = tabs[(index + 1) % tabs.length];
      if (event.key === 'ArrowLeft') target = tabs[(index + tabs.length - 1) % tabs.length];
      if (event.key === 'Home') target = tabs[0];
      if (event.key === 'End') target = tabs[tabs.length - 1];
      if (target) { event.preventDefault(); selectTab(target); target.focus(); }
    });
  });
  function userHTML(user, id) {
    user = user || {};
    return '<div class="companion-user">' + (user.avatar_url ? '<img src="' + esc(user.avatar_url) + '" alt="" class="companion-avatar" />' : '') +
      '<span><strong>' + esc(user.nickname || ('用户 ' + id)) + '</strong><small>ID ' + esc(id) + '</small></span></div>';
  }
  function renderCompanionDetail(data) {
    var s = data.session || {}, members = data.members || [], users = new Map();
    members.forEach(function (m) { users.set(m.user_id, m); });
    var owner = users.get(s.owner_user_id);
    document.getElementById('companionDetailTitle').textContent = s.title || '未命名房间';
    document.getElementById('companionDetailSubtitle').textContent = '房间 ID：' + (s.session_id || '—');
    setDetailRows('companionSessionInfo', [
      ['房主', owner && owner.nickname ? owner.nickname + '（ID ' + s.owner_user_id + '）' : '用户 ' + s.owner_user_id],
      ['房间状态', label(s.status)], ['运动类型', label(s.track_type)],
      ['房间可见性', label(s.visibility)], ['开始时间', fmtTime(s.started_at)], ['结束时间', fmtTime(s.ended_at)]
    ]);
    document.getElementById('companionLocation').textContent = s.locate_addr ? '活动地点：' + s.locate_addr : '活动地点未填写';
    var endSources = { owner: '房主结束', member_flow: '成员离开', auto_close: '自动结束' };
    setDetailRows('companionSettingsInfo', [
      ['人数上限', s.max_members], ['弹幕', s.danmaku_enabled ? '已开启' : '已关闭'],
      ['创建时间', fmtTime(s.created_at)], ['更新时间', fmtTime(s.updated_at)],
      ['结束方式', endSources[s.end_source] || s.end_source || '—'], ['结束原因', label(s.end_reason)],
      ['结束操作人 ID', s.end_operator_user_id || '—']
    ]);
    document.getElementById('companionPresenceHint').textContent = s.status === 'ended' ? '房间已结束，连接状态为最后保存的记录。' : '连接状态为最近保存的记录，点击“刷新详情”更新。';
    var memberBody = document.getElementById('companionMemberList');
    memberBody.replaceChildren();
    members.forEach(function (m) {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + userHTML(m, m.user_id) + '</td><td>' + esc(label(m.role)) + '</td><td>' + esc(label(m.member_status)) + '</td><td>' + badge(m.presence_status) + '</td><td>' + esc(fmtTime(m.joined_at)) + '</td><td>' + esc(fmtTime(m.last_seen_at)) + '</td>';
      memberBody.appendChild(tr);
    });
    document.getElementById('companionMemberHint').textContent = members.length;
    var positions = data.live_positions || [], positionBody = document.getElementById('companionPositionList');
    positionBody.replaceChildren();
    positions.forEach(function (p) {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + userHTML(users.get(p.user_id), p.user_id) + '</td>' +
        '<td><div class="companion-cell-stack"><span>' + esc(p.longitude) + ', ' + esc(p.latitude) + '</span><small>' + esc(p.coordinate_system || '—') + '</small></div></td>' +
        '<td><div class="companion-cell-stack"><span>' + esc(p.speed_kmh) + ' km/h</span><small>方向 ' + esc(p.heading) + '°</small></div></td>' +
        '<td><div class="companion-cell-stack"><span>精度 ' + esc(p.accuracy_m) + ' m</span><small>海拔 ' + esc(p.altitude) + ' m</small></div></td>' +
        '<td>' + esc(fmtTime(p.recorded_at)) + '</td><td><div class="companion-cell-stack"><span>' + esc(p.source || '—') + '</span><small>序号 ' + esc(p.seq) + '</small></div></td>';
      positionBody.appendChild(tr);
    });
    document.getElementById('companionPositionHint').textContent = positions.length;
    var danmakus = data.danmakus || [], danmakuBody = document.getElementById('companionDanmakuList');
    danmakuBody.replaceChildren();
    danmakus.forEach(function (d) {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + userHTML(users.get(d.user_id), d.user_id) + '</td><td class="companion-danmaku-content">' + esc(d.content) + '</td><td>' + esc(fmtTime(d.created_at)) + '</td>';
      danmakuBody.appendChild(tr);
    });
    document.getElementById('companionDanmakuHint').textContent = danmakus.length;
    if (!members.length) showEmpty(memberBody, 6, '暂无同行成员');
    if (!positions.length) showEmpty(positionBody, 6, '暂无位置记录');
    if (!danmakus.length) showEmpty(danmakuBody, 3, '暂无弹幕记录');
  }
  async function loadDetail(sessionID, preserveTab) {
    var request = ++detailRequest;
    currentSessionID = sessionID;
    if (!preserveTab) {
      selectTab(tabs[0]);
      document.querySelector('.companion-settings').open = false;
    }
    document.getElementById('companionDetailTitle').textContent = '正在加载详情…';
    document.getElementById('companionDetailSubtitle').textContent = '房间 ID：' + sessionID;
    document.getElementById('companionDetailError').hidden = true;
    document.getElementById('companionDetailBody').hidden = true;
    document.getElementById('companionDetailLoading').hidden = false;
    document.getElementById('refreshCompanionDetail').disabled = true;
    if (!detailModal.open) detailModal.showModal();
    try {
      var data = await apiJSON('/admin/api/companions/' + encodeURIComponent(sessionID));
      if (request !== detailRequest) return;
      renderCompanionDetail(data);
      document.getElementById('companionDetailBody').hidden = false;
    } catch (error) {
      if (request !== detailRequest) return;
      var errorNode = document.getElementById('companionDetailError');
      errorNode.textContent = '详情加载失败：' + error.message + '。请点击“刷新详情”重试。';
      errorNode.hidden = false;
      document.getElementById('companionDetailTitle').textContent = '同行房间详情';
    } finally {
      if (request === detailRequest) {
        document.getElementById('companionDetailLoading').hidden = true;
        document.getElementById('refreshCompanionDetail').disabled = false;
      }
    }
  }
  document.getElementById('refreshCompanionDetail').addEventListener('click', function () {
    if (currentSessionID) loadDetail(currentSessionID, true);
  });
  document.getElementById('closeCompanionDetail').addEventListener('click', function () { detailModal.close(); });
  detailModal.addEventListener('close', function () { detailRequest += 1; currentSessionID = ''; });
  detailModal.addEventListener('click', function (event) { if (event.target === detailModal) detailModal.close(); });

  loadCompanionStatistics();
  loadPage();
})();
