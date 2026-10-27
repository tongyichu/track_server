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
  statusFilter.value = 'pending';
  var versionFilter = document.getElementById('versionFilter');
  var phoneFilter = document.getElementById('phoneFilter');
  var refreshBtn = document.getElementById('refreshBtn');
  var prevBtn = document.getElementById('prevBtn');
  var nextBtn = document.getElementById('nextBtn');
  var pageHint = document.getElementById('pageHint');
  var emptyDetail = document.getElementById('emptyDetail');
  var detailContent = document.getElementById('detailContent');
  var detailModal = new tabler.Modal(document.getElementById('detailModal'));
  var statusForm = document.getElementById('statusForm');
  var statusSelect = document.getElementById('statusSelect');
  var replyInput = document.getElementById('replyInput');
  var saveErr = document.getElementById('saveErr');
  var saveOk = document.getElementById('saveOk');

  statusFilter.addEventListener('change', resetAndLoad);
  refreshBtn.addEventListener('click', resetAndLoad);
  [versionFilter, phoneFilter].forEach(function (input) {
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') resetAndLoad();
    });
  });
  tbody.addEventListener('click', function (event) {
    var button = event.target && event.target.closest ? event.target.closest('button[data-id]') : null;
    if (!button) return;
    tbody.querySelectorAll('tr.selected').forEach(function (row) { row.classList.remove('selected'); });
    var selectedRow = button.closest('tr');
    if (selectedRow) selectedRow.classList.add('selected');
    loadDetail(button.dataset.id, true);
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
    if (versionFilter.value.trim()) params.set('app_version', versionFilter.value.trim());
    if (phoneFilter.value.trim()) params.set('phone', phoneFilter.value.trim());
    if (cursorStack[pageIndex]) params.set('cursor', cursorStack[pageIndex]);

    var resp = await fetch('/admin/api/feedbacks?' + params.toString());
    if (!resp.ok) {
      tbody.innerHTML = '<tr><td colspan="8">加载失败: ' + resp.status + '</td></tr>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    var items = data.items || [];
    lastNextCursor = data.next_cursor || '';
    tbody.innerHTML = '';
    if (!items.length) {
      tbody.innerHTML = '<tr><td colspan="8" class="hint">暂无反馈</td></tr>';
    }
    items.forEach(function (it) {
      var tr = document.createElement('tr');
      if (it.feedback_id === currentID) tr.className = 'selected';
      tr.innerHTML = '<td>' + esc(it.user_name || '未知用户') + '</td>' +
        '<td>' + statusBadge(it.status) + '</td>' +
        '<td class="feedback-content">' + esc(it.content) + '</td>' +
        '<td>' + ((it.images || []).length) + '</td>' +
        '<td>' + esc(it.platform || '') + '</td>' +
        '<td>' + esc(it.app_version || '') + '</td>' +
        '<td>' + fmtTime(it.created_at) + '</td>' +
        '<td><button data-id="' + esc(it.feedback_id) + '">查看</button></td>';
      tbody.appendChild(tr);
    });
    prevBtn.disabled = pageIndex <= 0;
    nextBtn.disabled = !lastNextCursor;
    pageHint.textContent = '第 ' + (pageIndex + 1) + ' 页（' + items.length + ' 条）';
  }

  async function loadDetail(id, scrollToDetail) {
    saveErr.textContent = '';
    saveOk.textContent = '';
    emptyDetail.textContent = '正在加载反馈详情…';
    emptyDetail.classList.remove('hidden');
    detailContent.classList.add('hidden');
    if (scrollToDetail) detailModal.show();
    try {
      var resp = await fetch('/admin/api/feedbacks/' + encodeURIComponent(id));
      if (!resp.ok) {
        var errorBody = await resp.json().catch(function () { return {}; });
        var errorText = errorBody.error || ('加载详情失败: ' + resp.status);
        emptyDetail.textContent = errorText;
        AdminUI.alert(errorText, 'danger');
        return;
      }
      var it = ((await resp.json()).data) || {};
      currentID = it.feedback_id || '';
      emptyDetail.classList.add('hidden');
      detailContent.classList.remove('hidden');
      setText('detailUserName', it.user_name || '未知用户');
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
      if (!(it.images || []).length) imgs.innerHTML = '<span class="hint">无图片</span>';
    } catch (err) {
      var message = '加载详情失败: ' + (err.message || err);
      emptyDetail.textContent = message;
      AdminUI.alert(message, 'danger');
    }
  }

  function setText(id, value) {
    document.getElementById(id).textContent = value == null ? '' : String(value);
  }

  function statusBadge(status) {
    var s = String(status || '');
    var labels = { pending: '待处理', processing: '处理中', resolved: '已解决', ignored: '已忽略' };
    return '<span class="badge badge-' + esc(s) + '">' + esc(labels[s] || s) + '</span>';
  }

  function fmtTime(s) {
    return esc((s || '').replace('T', ' ').slice(0, 19));
  }

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  async function loadStats() {
    try {
      var resp = await fetch('/admin/api/dashboard?period=month');
      if (!resp.ok) throw new Error(String(resp.status));
      var data = ((await resp.json()).data) || {};
      var stats = data.feedback || {};
      setText('statPending', stats.pending);
      setText('statProcessing', stats.processing);
      setText('statResolved', stats.resolved);
      setText('statIgnored', stats.ignored);
      setText('statOver24', stats.pending_over_24h);
      setText('statOver72', stats.pending_over_72h);
    } catch (_) {
      ['statPending', 'statProcessing', 'statResolved', 'statIgnored', 'statOver24', 'statOver72'].forEach(function (id) { setText(id, '—'); });
    }
  }

  loadStats();
  loadPage();
})();
