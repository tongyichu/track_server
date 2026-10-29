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
    if (!s) return '';
    // 已结束的 session 才有 ended_at；零值 RFC3339 形如 "0001-01-01T00:00:00Z"
    if (String(s).startsWith('0001-')) return '';
    return esc(String(s).replace('T', ' ').slice(0, 19));
  }

  async function loadCompanionStatistics() {
    try {
      var response = await fetch('/admin/api/companions/statistics?period=month');
      if (!response.ok) throw new Error('统计加载失败');
      var payload = (await response.json()).data || {};
      var stats = payload.stats || {};
      document.getElementById('companionActive').textContent = Number(stats.active || 0).toLocaleString();
      document.getElementById('companionCreated').textContent = Number((stats.created || {}).current || 0).toLocaleString();
      document.getElementById('companionCreatedCompare').textContent = '上期 ' + Number((stats.created || {}).previous || 0).toLocaleString();
      document.getElementById('companionEnded').textContent = Number(stats.ended || 0).toLocaleString();
      document.getElementById('companionAutoEnded').textContent = Number(stats.auto_ended || 0).toLocaleString();
      document.getElementById('companionParticipants').textContent = Number(stats.avg_participants || 0).toFixed(1);
    } catch (_) {
      ['companionActive', 'companionCreated', 'companionEnded', 'companionAutoEnded', 'companionParticipants'].forEach(function (id) {
        document.getElementById(id).textContent = '—';
      });
    }
  }

  var pageSize = 20;
  var cursorStack = [null];
  var pageIndex = 0;
  var lastNextCursor = null;
  var totalCount = 0;

  var tbody = document.getElementById('companionList');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var refreshBtn = document.getElementById('refreshBtn');
  var searchBtn = document.getElementById('searchBtn');
  var titleFilter = document.getElementById('titleFilter');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');
  var detailModal = document.getElementById('companionDetailModal');
  var detailRequest = 0;

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
      params.set('cursor_started_at', cur.started_at);
      params.set('cursor_session_id', cur.session_id);
    }
    var resp = await fetch('/admin/api/companions?' + params.toString());
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
      var sid = esc(it.session_id);
      var tr = document.createElement('tr');
      tr.className = 'companion-row';
      tr.tabIndex = 0;
      tr.innerHTML = '<td title="' + sid + '">' + sid + '</td>' +
        '<td>' + (it.owner_user_id || 0) + '</td>' +
        '<td>' + esc(it.title || '') + '</td>' +
        '<td>' + esc(it.track_type || '') + '</td>' +
        '<td>' + esc(it.status || '') + '</td>' +
        '<td>' + esc(it.visibility || '') + '</td>' +
        '<td>' + (it.danmaku_enabled ? '开' : '关') + '</td>' +
        '<td>' + (it.max_members || 0) + '</td>' +
        '<td>' + fmtTime(it.started_at) + '</td>' +
        '<td>' + fmtTime(it.ended_at) + '</td>' +
        '<td><button type="button" class="btn btn-sm btn-outline-primary companion-view">查看详情</button></td>';
      tr.addEventListener('click', function () { loadDetail(it.session_id); });
      tr.addEventListener('keydown', function (event) { if (event.target === tr && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); loadDetail(it.session_id); } });
      tr.querySelector('.companion-view').addEventListener('click', function (event) { event.stopPropagation(); loadDetail(it.session_id); });
      tbody.appendChild(tr);
    });

    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
    totalHint.textContent = '共 ' + totalCount + ' 个会话';
  }

  function setDetailRows(items) {
    var container = document.getElementById('companionSessionInfo');
    container.replaceChildren();
    items.forEach(function (item) {
      var cell = document.createElement('div');
      var label = document.createElement('strong');
      var value = document.createElement('span');
      label.textContent = item[0];
      value.textContent = item[1] == null ? '' : String(item[1]);
      cell.append(label, value);
      container.appendChild(cell);
    });
  }
  function showEmpty(tbody, columns, message) {
    tbody.innerHTML = '<tr><td colspan="' + columns + '" class="hint">' + esc(message) + '</td></tr>';
  }
  function renderCompanionDetail(data) {
    var s = data.session || {};
    document.getElementById('companionDetailTitle').textContent = s.title || '同行会话详情';
    document.getElementById('companionDetailSubtitle').textContent = s.session_id || '';
    setDetailRows([
      ['发起人 UserID', s.owner_user_id || 0], ['类型', s.track_type || ''], ['位置', s.locate_addr || ''],
      ['状态', s.status || ''], ['可见性', s.visibility || ''], ['最大成员', s.max_members || 0],
      ['弹幕开关', s.danmaku_enabled ? '开' : '关'], ['开始时间', fmtTime(s.started_at)],
      ['结束时间', fmtTime(s.ended_at)], ['创建时间', fmtTime(s.created_at)], ['更新时间', fmtTime(s.updated_at)]
    ]);
    var members = data.members || [], memberBody = document.getElementById('companionMemberList');
    memberBody.innerHTML = '';
    members.forEach(function (m) {
      var avatar = m.avatar_url ? '<img src="' + esc(m.avatar_url) + '" alt="" class="companion-avatar" />' : '';
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + (m.user_id || 0) + '</td><td>' + avatar + '</td><td>' + esc(m.nickname || '') + '</td><td>' + esc(m.role || '') + '</td><td>' + esc(m.member_status || '') + '</td><td>' + esc(m.presence_status || '') + '</td><td>' + fmtTime(m.joined_at) + '</td><td>' + fmtTime(m.last_seen_at) + '</td>';
      memberBody.appendChild(tr);
    });
    document.getElementById('companionMemberHint').textContent = '共 ' + members.length + ' 人';
    var positions = data.live_positions || [], positionBody = document.getElementById('companionPositionList');
    positionBody.innerHTML = '';
    positions.forEach(function (p) {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + (p.user_id || 0) + '</td><td>' + esc(p.latitude) + '</td><td>' + esc(p.longitude) + '</td><td>' + esc(p.coordinate_system || '') + '</td><td>' + esc(p.speed_kmh || 0) + '</td><td>' + esc(p.heading || 0) + '</td><td>' + esc(p.accuracy_m || 0) + '</td><td>' + esc(p.altitude || 0) + '</td><td>' + fmtTime(p.recorded_at) + '</td><td>' + (p.seq || 0) + '</td><td>' + esc(p.source || '') + '</td>';
      positionBody.appendChild(tr);
    });
    document.getElementById('companionPositionHint').textContent = '共 ' + positions.length + ' 条';
    var danmakus = data.danmakus || [], danmakuBody = document.getElementById('companionDanmakuList');
    danmakuBody.innerHTML = '';
    danmakus.forEach(function (d) {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + (d.id || 0) + '</td><td>' + (d.user_id || 0) + '</td><td>' + esc(d.content || '') + '</td><td>' + fmtTime(d.created_at) + '</td>';
      danmakuBody.appendChild(tr);
    });
    document.getElementById('companionDanmakuHint').textContent = '共 ' + danmakus.length + ' 条';
    if (!members.length) showEmpty(memberBody, 8, '暂无同行人员');
    if (!positions.length) showEmpty(positionBody, 11, '暂无实时位置');
    if (!danmakus.length) showEmpty(danmakuBody, 4, '暂无弹幕');
  }
  async function loadDetail(sessionID) {
    var requestID = ++detailRequest;
    document.getElementById('companionDetailTitle').textContent = '正在加载详情…';
    document.getElementById('companionDetailSubtitle').textContent = sessionID || '';
    document.getElementById('companionDetailError').hidden = true;
    setDetailRows([]);
    detailModal.showModal();
    try {
      var response = await fetch('/admin/api/companions/' + encodeURIComponent(sessionID));
      if (!response.ok) throw new Error('详情加载失败：' + response.status);
      var payload = (await response.json()).data || {};
      if (requestID === detailRequest) renderCompanionDetail(payload);
    } catch (error) {
      if (requestID !== detailRequest) return;
      var errorNode = document.getElementById('companionDetailError');
      errorNode.textContent = error.message || '详情加载失败';
      errorNode.hidden = false;
      document.getElementById('companionDetailTitle').textContent = '同行会话详情';
    }
  }
  document.getElementById('closeCompanionDetail').addEventListener('click', function () { detailModal.close(); });
  detailModal.addEventListener('click', function (event) { if (event.target === detailModal) detailModal.close(); });

  loadCompanionStatistics();
  loadPage();
})();
