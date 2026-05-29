(function () {
  // 通用：登录态校验 + 退出
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

  // 翻页：采用 cursor 栈记录每一页的入口 cursor。
  // cursorStack[i] = 第 i 页（从 0 开始）的入口 cursor；首页入口 cursor 为 null。
  var pageSize = 20;
  var cursorStack = [null];
  var pageIndex = 0;
  var lastNextCursor = null;
  var totalCount = 0;

  var tbody = document.getElementById('userList');
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
      params.set('cursor_created_at', cur.created_at);
      params.set('cursor_id', String(cur.id));
    }
    var resp = await fetch('/admin/api/users?' + params.toString());
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
      var avatar = it.avatar_url
        ? '<img src="' + esc(it.avatar_url) + '" alt="" style="width:32px;height:32px;border-radius:50%;object-fit:cover;" />'
        : '';
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + it.id + '</td>' +
        '<td>' + avatar + '</td>' +
        '<td>' + esc(it.nickname) + '</td>' +
        '<td>' + esc(it.phone || '') + '</td>' +
        '<td>' + esc(it.client_language || '') + '</td>' +
        '<td>' + esc(it.signature || '') + '</td>' +
        '<td>' + fmtTime(it.created_at) + '</td>' +
        '<td>' + fmtTime(it.updated_at) + '</td>';
      tbody.appendChild(tr);
    });

    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
    totalHint.textContent = '共 ' + totalCount + ' 个用户';
  }

  loadPage();
})();
