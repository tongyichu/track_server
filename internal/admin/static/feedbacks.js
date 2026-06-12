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

  var pageSize = 20;
  var cursorStack = [null];
  var pageIndex = 0;
  var lastNextCursor = '';
  var currentID = '';

  var tbody = document.getElementById('feedbackList');
  var statusFilter = document.getElementById('statusFilter');
  var refreshBtn = document.getElementById('refreshBtn');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var pageHint = document.getElementById('pageHint');
  var emptyDetail = document.getElementById('emptyDetail');
  var detailContent = document.getElementById('detailContent');
  var statusForm = document.getElementById('statusForm');
  var statusSelect = document.getElementById('statusSelect');
  var replyInput = document.getElementById('replyInput');
  var saveErr = document.getElementById('saveErr');
  var saveOk = document.getElementById('saveOk');

  statusFilter.addEventListener('change', resetAndLoad);
  refreshBtn.addEventListener('click', resetAndLoad);
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

  statusForm.addEventListener('submit', async function (e) {
    e.preventDefault();
    if (!currentID) return;
    saveErr.textContent = '';
    saveOk.textContent = '';
    var payload = {
      status: statusSelect.value,
      reply: replyInput.value || '',
    };
    try {
      var resp = await fetch('/admin/api/feedbacks/' + encodeURIComponent(currentID) + '/status', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        var j = await resp.json().catch(function () { return {}; });
        saveErr.textContent = j.error || ('保存失败: ' + resp.status);
        return;
      }
      saveOk.textContent = '已保存';
      await loadDetail(currentID);
      await loadPage();
    } catch (err) {
      saveErr.textContent = err.message || String(err);
    }
  });

  function resetAndLoad() {
    cursorStack = [null];
    pageIndex = 0;
    lastNextCursor = '';
    loadPage();
  }

  async function loadPage() {
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
    if (statusFilter.value) params.set('status', statusFilter.value);
    if (cursorStack[pageIndex]) params.set('cursor', cursorStack[pageIndex]);

    var resp = await fetch('/admin/api/feedbacks?' + params.toString());
    if (!resp.ok) {
      tbody.innerHTML = '<tr><td colspan="9">加载失败: ' + resp.status + '</td></tr>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    var items = data.items || [];
    lastNextCursor = data.next_cursor || '';
    tbody.innerHTML = '';
    if (!items.length) {
      tbody.innerHTML = '<tr><td colspan="9" class="hint">暂无反馈</td></tr>';
    }
    items.forEach(function (it) {
      var tr = document.createElement('tr');
      if (it.feedback_id === currentID) tr.className = 'selected';
      tr.innerHTML = '<td class="mono">' + esc(it.feedback_id) + '</td>' +
        '<td>' + esc(it.user_id) + '</td>' +
        '<td>' + statusBadge(it.status) + '</td>' +
        '<td class="clip" title="' + esc(it.content) + '">' + esc(it.content) + '</td>' +
        '<td>' + ((it.images || []).length) + '</td>' +
        '<td>' + esc(it.platform || '') + '</td>' +
        '<td>' + esc(it.app_version || '') + '</td>' +
        '<td>' + fmtTime(it.created_at) + '</td>' +
        '<td><button data-id="' + esc(it.feedback_id) + '">查看</button></td>';
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll('button[data-id]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        loadDetail(btn.dataset.id);
      });
    });
    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
  }

  async function loadDetail(id) {
    saveErr.textContent = '';
    saveOk.textContent = '';
    var resp = await fetch('/admin/api/feedbacks/' + encodeURIComponent(id));
    if (!resp.ok) {
      saveErr.textContent = '加载详情失败: ' + resp.status;
      return;
    }
    var it = ((await resp.json()).data) || {};
    currentID = it.feedback_id || '';
    emptyDetail.classList.add('hidden');
    detailContent.classList.remove('hidden');
    setText('detailID', it.feedback_id);
    setText('detailUserID', it.user_id);
    document.getElementById('detailStatus').innerHTML = statusBadge(it.status);
    setText('detailClient', [it.platform, it.app_version].filter(Boolean).join(' / '));
    setText('detailDevice', [it.device_model, it.system_version].filter(Boolean).join(' / '));
    setText('detailContact', it.contact || '');
    setText('detailCreated', fmtTime(it.created_at));
    setText('detailUpdated', fmtTime(it.updated_at));
    setText('detailText', it.content || '');
    statusSelect.value = it.status || 'pending';
    replyInput.value = it.reply || '';

    var imgs = document.getElementById('detailImages');
    imgs.innerHTML = '';
    (it.images || []).forEach(function (img) {
      var a = document.createElement('a');
      a.href = img.url;
      a.target = '_blank';
      a.title = img.mime_type + ' / ' + Math.round((img.size || 0) / 1024) + ' KB';
      a.innerHTML = '<img src="' + esc(img.url) + '" alt="反馈图片 ' + esc(img.image_id) + '" />';
      imgs.appendChild(a);
    });
    if (!(it.images || []).length) {
      imgs.innerHTML = '<span class="hint">无图片</span>';
    }
    loadPage();
  }

  function setText(id, value) {
    document.getElementById(id).textContent = value == null ? '' : String(value);
  }

  function statusBadge(status) {
    var s = String(status || '');
    return '<span class="badge badge-' + esc(s) + '">' + esc(s) + '</span>';
  }

  function fmtTime(s) {
    return esc((s || '').replace('T', ' ').slice(0, 19));
  }

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  loadPage();
})();
