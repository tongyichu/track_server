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

  var pageSize = 20;
  var cursorStack = [null];
  var pageIndex = 0;
  var lastNextCursor = null;
  var totalCount = 0;

  var tbody = document.getElementById('trackList');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var refreshBtn = document.getElementById('refreshBtn');
  var pageHint = document.getElementById('pageHint');
  var totalHint = document.getElementById('totalHint');

  refreshBtn.addEventListener('click', function () {
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

  async function loadPage() {
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
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
      tr.innerHTML = '<td>' + esc(it.id) + '</td>' +
        '<td>' + it.user_id + '</td>' +
        '<td>' + esc(it.title || '') + '</td>' +
        '<td>' + esc(it.track_type || '') + '</td>' +
        '<td>' + esc(it.city_code || '') + '</td>' +
        '<td>' + (it.distance != null ? Number(it.distance).toFixed(1) : '') + '</td>' +
        '<td>' + (it.duration || 0) + '</td>' +
        '<td>' + esc(statusText(it.status)) + '</td>' +
        '<td>' + fmtTime(it.start_time) + '</td>' +
        '<td>' + fmtTime(it.end_time) + '</td>' +
        '<td><button class="danger js-delete-track" data-track-id="' + esc(it.id) + '">删除</button></td>';
      tbody.appendChild(tr);
    });

    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
    totalHint.textContent = '共 ' + totalCount + ' 条轨迹（不含已删除）';
  }

  tbody.addEventListener('click', async function (ev) {
    var btn = ev.target && ev.target.closest ? ev.target.closest('.js-delete-track') : null;
    if (!btn) return;
    var trackID = btn.getAttribute('data-track-id') || '';
    if (!trackID) return;
    if (!window.confirm('确认删除轨迹 ' + trackID + '？删除后会标记为已删除，并清理收藏和地图索引。')) {
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
        window.alert(msg);
        btn.disabled = false;
        btn.textContent = '删除';
        return;
      }
      await loadPage();
    } catch (err) {
      window.alert('删除失败: ' + (err && err.message ? err.message : err));
      btn.disabled = false;
      btn.textContent = '删除';
    }
  });

  loadPage();
})();
