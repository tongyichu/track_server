(function () {
  'use strict';
  var $ = function (id) { return document.getElementById(id); };
  var selected = null, detailController = null, detailRequest = 0, detailID = '', loadingID = '', reviewing = false;
  var listController = null, listRequest = 0;
  var modalClosing = false, pendingDetailID = '';
  var typeOptions = Object.create(null), optionLabels = Object.create(null);
  var modalRoot = $('submissionDetailModal'), modal = new tabler.Modal(modalRoot);
  var statusLabels = { pending: '待审核', approved: '已通过', rejected: '已驳回', withdrawn: '已撤回', invalidated: '已失效' };
  var statusColors = { pending: 'bg-yellow-lt', approved: 'bg-green-lt', rejected: 'bg-red-lt', withdrawn: 'bg-secondary-lt', invalidated: 'bg-secondary-lt' };
  var eventLabels = { submitted: '提交投稿', resubmitted: '更新并重新投稿', approved: '审核通过', rejected: '审核驳回', withdrawn: '撤回投稿', invalidated: '投稿失效' };
  function esc(value) {
    return String(value == null ? '' : value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function timeText(value) {
    if (!value || String(value).startsWith('0001-')) return '—';
    var date = new Date(value);
    return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false });
  }
  function statusText(value) { return statusLabels[value] || '未知状态'; }
  function badge(text, color) { return '<span class="badge ' + (color || 'bg-secondary-lt') + '">' + esc(text) + '</span>'; }
  function statusBadge(value) { return badge(statusText(value), statusColors[value]); }
  function readMetadata(data) {
    (data.track_types || []).forEach(function (item) { typeOptions[item.code] = item; });
    Object.entries(data.options || {}).forEach(function (entry) {
      if (!Array.isArray(entry[1])) return;
      var names = optionLabels[entry[0]] = Object.create(null);
      entry[1].forEach(function (item) { names[item.code] = item.name; });
    });
  }
  function optionText(group, code) { return code ? (optionLabels[group] && optionLabels[group][code] || '未识别') : '未填写'; }
  function typeBadge(code) {
    var type = typeOptions[code], color = type && /^#[a-f0-9]{6}$/i.test(type.color) ? type.color : '#64748b', icon = '';
    if (type && /^[a-z0-9-]+\.svg$/.test(type.icon)) {
      var url = AdminAssets.url('/admin/static/track_type_icon/' + type.icon);
      icon = '<span class="submission-type-icon" style="--submission-type-icon:url(' + esc(url) + ')" aria-hidden="true"></span>';
    }
    return '<span class="track-type-badge" style="--track-type-color:' + color + '">' + icon + '<span>' + esc(type ? type.name : '其他类型') + '</span></span>';
  }
  function riskBadge(code) {
    var colors = { none: 'bg-secondary-lt', low: 'bg-green-lt', medium: 'bg-yellow-lt', high: 'bg-red-lt' };
    return badge(optionText('risk_level_options', code), colors[code]);
  }
  function errorText(error) {
    if (error.status === 409) return '投稿状态或版本已变化，请关闭弹窗重新查看后再审核。';
    if (error.status === 401) return '登录已失效，请刷新页面重新登录。';
    if (error.status === 403) return '当前账号没有执行此操作的权限。';
    if (error.status === 404) return '投稿或关联轨迹不存在，请刷新列表。';
    return /[\u3400-\u9fff]/.test(error.message || '') ? error.message : '请求失败，请稍后重试。';
  }
  async function api(path, options) {
    var response = await fetch(path, options), body = await response.json();
    if (!response.ok || body.code !== 0) {
      var error = new Error(body.error || '请求失败'); error.status = response.status; throw error;
    }
    return body.data || {};
  }
  function listState(message) { $('list').innerHTML = '<tr><td colspan="9" class="submission-table-state">' + esc(message) + '</td></tr>'; }
  async function load() {
    if (listController) listController.abort();
    var controller = listController = new AbortController(), request = ++listRequest;
    $('refreshBtn').disabled = true; $('list').setAttribute('aria-busy', 'true'); listState('正在加载投稿…');
    try {
      var data = await api('/admin/api/track-submissions?limit=100&status=' + encodeURIComponent($('status').value), { signal: controller.signal });
      if (request !== listRequest || controller.signal.aborted) return;
      readMetadata(data); $('list').replaceChildren();
      (data.items || []).forEach(function (item) {
        var row = document.createElement('tr');
        row.innerHTML = '<td>' + esc(item.track_id) + '</td><td>' + esc(item.title || '未命名路线') + '</td><td>' + typeBadge(item.track_type) + '</td><td>' +
          badge(optionText('difficulty_options', item.difficulty)) + '</td><td>' + riskBadge(item.risk_level) + '</td><td>' + statusBadge(item.status) + '</td><td>第 ' +
          esc(item.revision) + ' 版</td><td>' + esc(timeText(item.submitted_at)) + '</td><td><button type="button" class="btn btn-sm btn-outline-primary">查看</button></td>';
        row.querySelector('button').addEventListener('click', function () { show(item.submission_id); });
        $('list').appendChild(row);
      });
      if (!(data.items || []).length) listState('暂无符合条件的投稿');
    } catch (error) {
      if (request === listRequest && !controller.signal.aborted) listState('加载失败：' + errorText(error));
    } finally {
      if (request === listRequest) { $('refreshBtn').disabled = false; $('list').removeAttribute('aria-busy'); }
    }
  }
  async function loadStats() {
    var ids = ['submissionPending', 'submissionOver24', 'submissionOver72', 'submissionApproved', 'submissionRejected', 'submissionApprovalRate', 'submissionAvgHours'];
    try {
      var data = await api('/admin/api/dashboard?period=month&scope=track_submissions'), review = data.review || {};
      var reviewed = Number(review.approved || 0) + Number(review.rejected || 0);
      function set(id, value) { $(id).textContent = value; }
      set('submissionPending', Number(review.pending || 0).toLocaleString());
      set('submissionOver24', Number(review.pending_over_24h || 0).toLocaleString());
      set('submissionOver72', Number(review.pending_over_72h || 0).toLocaleString());
      set('submissionApproved', Number(review.approved || 0).toLocaleString());
      set('submissionRejected', Number(review.rejected || 0).toLocaleString());
      set('submissionApprovalRate', reviewed ? (Number(review.approval_rate || 0) * 100).toFixed(1) + '%' : '—');
      set('submissionAvgHours', reviewed ? Number(review.avg_review_hours || 0).toFixed(1) + ' 小时' : '—');
    } catch (_) { ids.forEach(function (id) { $(id).textContent = '—'; }); }
  }
  function fact(label, content) { return '<div><dt>' + esc(label) + '</dt><dd>' + content + '</dd></div>'; }
  function tags(group, values) {
    return '<span class="submission-inline-tags">' + ((values || []).map(function (value) { return badge(optionText(group, value)); }).join('') || '未填写') + '</span>';
  }
  function durationText(seconds) {
    if (seconds == null || !Number.isFinite(Number(seconds))) return '—';
    seconds = Math.max(0, Number(seconds));
    if (seconds < 60) return Math.floor(seconds) + ' 秒';
    var minutes = Math.floor(seconds / 60), hours = Math.floor(minutes / 60);
    return hours ? hours + ' 小时 ' + minutes % 60 + ' 分' : minutes + ' 分钟';
  }
  function renderPhotos(images) {
    var photos = (images || []).filter(function (image) { return image && String(image.url || '').startsWith('/admin/api/static/submission_images/'); });
    if (!photos.length) return '<p class="text-secondary mb-0">暂无投稿图片</p>';
    return '<div class="submission-photo-grid">' + photos.map(function (image, index) {
      var caption = image.caption || '投稿图片 ' + (index + 1);
      return '<figure><a href="' + esc(image.url) + '" target="_blank" rel="noopener noreferrer"><img src="' + esc(image.url) + '" alt="' + esc(caption) + '" loading="lazy"></a><figcaption>' + esc(caption) + '</figcaption></figure>';
    }).join('') + '</div>';
  }
  function renderHistory(events) {
    var rows = (events || []).slice().sort(function (a, b) { return new Date(b.created_at) - new Date(a.created_at); });
    var history = rows.map(function (event) {
      var operator = event.operator_type === 'admin' ? (event.operator || '管理员') : event.operator_type === 'user' ? '投稿用户' : '系统';
      var status = event.from_status ? statusText(event.from_status) + ' → ' + statusText(event.to_status) : statusText(event.to_status);
      return '<li><header><strong>' + esc(eventLabels[event.event_type] || '状态更新') + '</strong><time>' + esc(timeText(event.created_at)) + '</time></header><small>' +
        esc(operator) + ' · ' + esc(status) + ' · 第 ' + esc(event.revision) + ' 版</small>' + (event.reason ? '<p>' + esc(event.reason) + '</p>' : '') + '</li>';
    }).join('');
    return '<details class="submission-detail-section submission-history"><summary>审核记录（' + rows.length + '）</summary>' +
      (history ? '<ol class="submission-history-list">' + history + '</ol>' : '<p class="text-secondary mt-3 mb-0">暂无审核记录</p>') + '</details>';
  }
  function renderDetail(data) {
    var item = selected, track = data.track;
    var months = (item.suitable_months || []).filter(function (month) { return Number.isInteger(month) && month >= 1 && month <= 12; }).map(function (month) { return month + ' 月'; }).join('、') || '未填写';
    var distance = track && track.distance != null ? (Number(track.distance) / 1000).toFixed(2) + ' 公里' : '—';
    var elevation = track && track.elevation_gain != null ? Number(track.elevation_gain).toLocaleString() + ' 米' : '—';
    var visibility = track ? (track.is_running ? '录制中' : ({ 0: '已下架', 1: '公开', 2: '私密' }[track.status] || '未知')) : '暂无信息';
    $('detailTitle').textContent = item.title || '投稿详情';
    $('detailSubtitle').textContent = '轨迹编号：' + item.track_id + ' · 第 ' + item.revision + ' 版';
    $('detail').innerHTML = '<section class="submission-overview"><div class="submission-detail-badges">' + typeBadge(item.track_type) + statusBadge(item.status) +
      badge(optionText('difficulty_options', item.difficulty)) + riskBadge(item.risk_level) + '</div><div class="submission-track-metrics"><div><span>路线里程</span><strong>' + esc(distance) +
      '</strong></div><div><span>运动用时</span><strong>' + esc(durationText(track && track.duration)) + '</strong></div><div><span>累计爬升</span><strong>' + esc(elevation) + '</strong></div></div></section>' +
      '<div class="submission-detail-grid"><div class="submission-detail-column"><section class="submission-detail-section"><h3>路线介绍</h3><p class="submission-detail-copy">' + esc(item.description || '暂无路线介绍') +
      '</p></section><section class="submission-detail-section"><h3>投稿图片</h3>' + renderPhotos(item.images) + '</section></div><div class="submission-detail-column"><section class="submission-detail-section"><h3>出行信息</h3><dl class="submission-detail-facts">' +
      fact('适宜月份', esc(months)) + fact('路面地形', tags('surface_type_options', item.surface_types)) + fact('交通方式', tags('transport_mode_options', item.transport_modes)) +
      '</dl><p class="submission-detail-copy mt-3">' + esc(item.transport_description || '暂无交通补充说明') + '</p></section><section class="submission-detail-section"><h3>投稿与审核</h3><dl class="submission-detail-facts">' +
      fact('投稿状态', statusBadge(item.status)) + fact('提交时间', esc(timeText(item.submitted_at))) + fact('轨迹状态', esc(visibility)) + fact('运动时间', esc(timeText(track && track.start_time))) +
      fact('审核时间', esc(timeText(item.reviewed_at || item.approved_at))) + fact('审核人', esc(item.reviewed_by || '—')) + '</dl>' + (item.review_reason ? '<p class="submission-detail-copy mt-3">审核意见：' + esc(item.review_reason) + '</p>' : '') +
      '</section></div></div>' + renderHistory(data.events);
    $('detail').hidden = false;
  }
  function canReview() { return !!selected && selected.status === 'pending' && !!window.AdminAccess && AdminAccess.can('track_submissions', true); }
  function updateReviewState() {
    var allowed = canReview(); $('reviewSection').hidden = !allowed;
    ['approveBtn', 'rejectBtn'].forEach(function (id) { $(id).hidden = !allowed; $(id).disabled = !allowed || reviewing; });
    $('approveBtn').textContent = reviewing ? '正在提交…' : '通过投稿';
    $('reviewReason').disabled = !allowed || reviewing;
    modalRoot.querySelectorAll('[data-bs-dismiss="modal"]').forEach(function (button) { button.disabled = reviewing; });
  }
  async function show(id, retry) {
    if (modalClosing) { pendingDetailID = id; return; }
    if (reviewing || !id || (!retry && detailID === id && (loadingID || selected))) return;
    if (detailController) detailController.abort();
    var controller = detailController = new AbortController(), request = ++detailRequest;
    detailID = loadingID = id; selected = null;
    $('detailTitle').textContent = '投稿详情'; $('detailSubtitle').textContent = '';
    $('detail').hidden = true; $('detail').replaceChildren(); $('detailLoading').hidden = false;
    $('detailError').hidden = true; $('reviewError').hidden = true; $('reviewReason').value = '';
    updateReviewState(); modal.show();
    try {
      var data = await api('/admin/api/track-submissions/' + encodeURIComponent(id), { signal: controller.signal });
      if (request !== detailRequest || controller.signal.aborted) return;
      if (!data.submission) throw new Error('未找到投稿详情');
      readMetadata(data); selected = data.submission; renderDetail(data);
    } catch (error) {
      if (request === detailRequest && !controller.signal.aborted) { selected = null; $('detailErrorText').textContent = errorText(error); $('detailError').hidden = false; }
    } finally {
      if (request === detailRequest) { loadingID = ''; $('detailLoading').hidden = true; updateReviewState(); }
    }
  }
  async function review(decision) {
    if (!canReview() || reviewing) return;
    var item = selected, reason = $('reviewReason').value.trim();
    $('reviewError').hidden = true;
    if (decision === 'rejected' && !reason) { $('reviewError').textContent = '请先填写驳回原因。'; $('reviewError').hidden = false; $('reviewReason').focus(); return; }
    reviewing = true; updateReviewState();
    try {
      await api('/admin/api/track-submissions/' + encodeURIComponent(item.submission_id) + '/review', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ decision: decision, reason: reason, expected_revision: item.revision })
      });
      selected = null; reviewing = false; updateReviewState(); modal.hide();
      AdminUI.alert(decision === 'approved' ? '投稿已通过' : '投稿已驳回', 'success');
      load(); loadStats();
    } catch (error) { $('reviewError').textContent = errorText(error); $('reviewError').hidden = false; }
    finally { reviewing = false; updateReviewState(); }
  }
  modalRoot.addEventListener('hide.bs.modal', function (event) {
    if (reviewing) { event.preventDefault(); return; }
    modalClosing = true;
    detailRequest++; if (detailController) detailController.abort();
    selected = null; detailID = loadingID = '';
  });
  modalRoot.addEventListener('hidden.bs.modal', function () {
    modalClosing = false;
    var id = pendingDetailID; pendingDetailID = '';
    if (id) show(id);
  });
  $('retryDetail').addEventListener('click', function () { show(detailID, true); });
  $('approveBtn').addEventListener('click', function () { review('approved'); });
  $('rejectBtn').addEventListener('click', function () { review('rejected'); });
  $('refreshBtn').addEventListener('click', function () { load(); loadStats(); });
  $('status').addEventListener('change', load);
  load(); loadStats();
})();
