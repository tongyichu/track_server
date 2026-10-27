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
  var detailRequest = 0;
  var listRequest = 0;
  var saving = false;
  var correction = window.AdminFeedbackCorrection;

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
  var quickReplies = {
    corrected: { status: 'resolved', text: '已修正相关信息，感谢你的反馈。' },
    unverified: { status: 'ignored', text: '暂时无法核实该问题，欢迎补充说明或图片后再次反馈。' },
    unchanged: { status: 'ignored', text: '经核实，当前信息无需修改。感谢你的反馈。' }
  };

  document.getElementById('correctionReplies').addEventListener('click', function (event) {
    var button = event.target.closest('button[data-reply]');
    var template = button && quickReplies[button.dataset.reply];
    if (!template || saving) return;
    statusSelect.value = template.status;
    replyInput.value = template.text;
    updateReplyRequirement();
  });
  statusSelect.addEventListener('change', updateReplyRequirement);
  document.getElementById('detailModal').addEventListener('hidden.bs.modal', function () {
    detailRequest += 1;
    currentID = '';
  });

  function updateReplyRequirement() {
    replyInput.required = statusSelect.value === 'resolved';
  }

  statusFilter.addEventListener('change', resetAndLoad);
  refreshBtn.addEventListener('click', resetAndLoad);
  [versionFilter, phoneFilter].forEach(function (input) {
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') resetAndLoad();
    });
  });
  tbody.addEventListener('click', function (event) {
    var button = event.target && event.target.closest ? event.target.closest('button[data-id]') : null;
    if (!button || saving) return;
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
    if (!currentID || saving) return;
    var feedbackID = currentID;
    saveErr.textContent = '';
    saveOk.textContent = '';
    var payload = {
      status: statusSelect.value,
      reply: replyInput.value.trim(),
    };
    if (payload.status === 'resolved' && !payload.reply) {
      saveErr.textContent = '请填写处理回复';
      return;
    }
    saving = true;
    Array.from(statusForm.elements).forEach(function (input) { input.disabled = true; });
    try {
      var resp = await fetch('/admin/api/feedbacks/' + encodeURIComponent(feedbackID) + '/status', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!resp.ok) {
        var j = await resp.json().catch(function () { return {}; });
        saveErr.textContent = j.error || ('保存失败: ' + resp.status);
        return;
      }
      AdminUI.alert('已保存处理结果', 'success');
      if (currentID === feedbackID) await loadDetail(feedbackID);
      await Promise.all([loadPage(), loadStats()]);
    } catch (err) {
      saveErr.textContent = err.message || String(err);
    } finally {
      saving = false;
      Array.from(statusForm.elements).forEach(function (input) { input.disabled = false; });
    }
  });

  function resetAndLoad() {
    cursorStack = [null];
    pageIndex = 0;
    lastNextCursor = '';
    loadPage();
  }

  async function loadPage() {
    var request = ++listRequest;
    prevBtn.disabled = true;
    nextBtn.disabled = true;
    var params = new URLSearchParams();
    params.set('limit', String(pageSize));
    if (statusFilter.value) params.set('status', statusFilter.value);
    if (versionFilter.value.trim()) params.set('app_version', versionFilter.value.trim());
    if (phoneFilter.value.trim()) params.set('phone', phoneFilter.value.trim());
    if (cursorStack[pageIndex]) params.set('cursor', cursorStack[pageIndex]);

    var data;
    try {
      var resp = await fetch('/admin/api/feedbacks?' + params.toString());
      if (!resp.ok) throw new Error(String(resp.status));
      data = ((await resp.json()).data) || {};
    } catch (err) {
      if (request !== listRequest) return;
      tbody.innerHTML = '<tr><td colspan="9">加载失败: ' + esc(err.message || err) + '</td></tr>';
      prevBtn.disabled = pageIndex <= 0;
      return;
    }
    if (request !== listRequest) return;
    var items = data.items || [];
    lastNextCursor = data.next_cursor || '';
    tbody.innerHTML = '';
    if (!items.length) {
      tbody.innerHTML = '<tr><td colspan="9" class="hint">暂无反馈</td></tr>';
    }
    items.forEach(function (it) {
      var tr = document.createElement('tr');
      var issue = correction.parse(it);
      if (it.feedback_id === currentID) tr.className = 'selected';
      tr.innerHTML = '<td>' + esc(it.user_name || '未知用户') + '</td>' +
        '<td>' + (issue ? '<span class="badge bg-blue-lt text-blue">轨迹纠错</span>' : '意见反馈') + '</td>' +
        '<td>' + statusBadge(it.status) + '</td>' +
        '<td class="feedback-content">' + feedbackContent(it.content, issue) + '</td>' +
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
    var request = ++detailRequest;
    currentID = '';
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
        if (request !== detailRequest) return;
        var errorText = errorBody.error || ('加载详情失败: ' + resp.status);
        emptyDetail.textContent = errorText;
        AdminUI.alert(errorText, 'danger');
        return;
      }
      var it = ((await resp.json()).data) || {};
      if (request !== detailRequest) return;
      var issue = correction.parse(it);
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
      setText('detailTitle', issue ? '轨迹纠错' : '反馈详情');
      document.getElementById('detailCorrection').classList.toggle('hidden', !issue);
      document.getElementById('correctionReplies').classList.toggle('hidden', !issue);
      var trackLink = document.getElementById('detailTrackLink');
      trackLink.removeAttribute('href');
      if (issue) {
        setText('detailCorrectionReason', issue.reason);
        setText('detailTrackTitle', issue.title);
        setText('detailTrackID', issue.trackID);
        trackLink.href = correction.trackURL(issue.trackID);
      }
      setText('detailTextLabel', issue ? '补充说明' : '反馈内容');
      setText('detailText', issue ? issue.notes : it.content || '');
      document.getElementById('detailTextBlock').classList.toggle('hidden', !!issue && !issue.notes);
      statusSelect.value = it.status || 'pending';
      replyInput.value = it.reply || '';
      updateReplyRequirement();

      var imgs = document.getElementById('detailImages');
      imgs.innerHTML = '';
      (it.images || []).forEach(function (img) {
        var a = document.createElement('a');
        a.href = img.url;
        a.target = '_blank';
        a.rel = 'noopener';
        a.title = img.mime_type + ' / ' + Math.round((img.size || 0) / 1024) + ' KB';
        a.innerHTML = '<img src="' + esc(img.url) + '" alt="反馈图片 ' + esc(img.image_id) + '" />';
        imgs.appendChild(a);
      });
      document.getElementById('detailImagesBlock').classList.toggle('hidden', !!issue && !(it.images || []).length);
      if (!(it.images || []).length) imgs.innerHTML = '<span class="hint">无图片</span>';
    } catch (err) {
      if (request !== detailRequest) return;
      var message = '加载详情失败: ' + (err.message || err);
      emptyDetail.textContent = message;
      AdminUI.alert(message, 'danger');
    }
  }

  function feedbackContent(content, issue) {
    if (!issue) return esc(content);
    return '<div class="feedback-correction-context"><span class="badge bg-blue-lt text-blue">' + esc(issue.reason) + '</span>' +
      '<a href="' + esc(correction.trackURL(issue.trackID)) + '" target="_blank" rel="noopener">' + esc(issue.title) + '</a>' +
      '<span class="hint mono">' + esc(issue.trackID) + '</span></div>' +
      (issue.notes ? '<p class="feedback-correction-notes">' + esc(issue.notes) + '</p>' : '');
  }

  function setText(id, value) {
    document.getElementById(id).textContent = value == null ? '' : String(value);
  }

  function statusBadge(status) {
    var s = String(status || '');
    var labels = { pending: '待处理', processing: '处理中', resolved: '已处理', ignored: '已关闭' };
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
