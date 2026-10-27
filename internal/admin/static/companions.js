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
      tbody.innerHTML = '<tr><td colspan="10">加载失败: ' + resp.status + '</td></tr>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    var items = data.items || [];
    totalCount = data.total || 0;
    lastNextCursor = data.next_cursor || null;

    tbody.innerHTML = '';
    items.forEach(function (it) {
      var sid = esc(it.session_id);
      var detailHref = '/admin/companion_detail.html?session_id=' + encodeURIComponent(it.session_id || '');
      var tr = document.createElement('tr');
      tr.innerHTML = '<td title="' + sid + '"><a href="' + detailHref + '">' + sid + '</a></td>' +
        '<td>' + (it.owner_user_id || 0) + '</td>' +
        '<td>' + esc(it.title || '') + '</td>' +
        '<td>' + esc(it.track_type || '') + '</td>' +
        '<td>' + esc(it.status || '') + '</td>' +
        '<td>' + esc(it.visibility || '') + '</td>' +
        '<td>' + (it.danmaku_enabled ? '开' : '关') + '</td>' +
        '<td>' + (it.max_members || 0) + '</td>' +
        '<td>' + fmtTime(it.started_at) + '</td>' +
        '<td>' + fmtTime(it.ended_at) + '</td>';
      tbody.appendChild(tr);
    });

    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
    totalHint.textContent = '共 ' + totalCount + ' 个会话';
  }

  loadCompanionStatistics();
  loadPage();
})();
