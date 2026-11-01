(function () {


  var pageSize = 20;
  var cursorStack = [null];
  var pageIndex = 0;
  var lastNextCursor = '';
  var currentID = '';
  var detailRequest = 0;
  var listRequest = 0;
  var saving = false;
  var correction = window.AdminFeedbackCorrection;
  var currentIssue = null;
  var currentRevision = 0;
  var structuredCorrection = false;
  var serverResult = null;
  var recoveryDraft = null;
  var outcomeSelect = document.getElementById('outcomeSelect');
  var kindFilter = document.getElementById('kindFilter');
  var optionFilter = document.getElementById('optionFilter');
  var trackFilter = document.getElementById('trackFilter');
  [kindFilter, optionFilter].forEach(function (select) { select.addEventListener('change', resetAndLoad); });
  trackFilter.addEventListener('keydown', function (event) { if (event.key === 'Enter') resetAndLoad(); });
  var optionRequest = 0;
  async function loadOptions() {
    var request = ++optionRequest;
    try {
      var response = await fetch('/admin/api/feedback-correction-config', { cache: 'no-store' });
      if (!response.ok) return;
      var config = ((await response.json()).data) || {};
      if (request !== optionRequest) return;
      var selected = optionFilter.value;
      optionFilter.replaceChildren(new Option('全部问题', ''));
      (config.options || []).forEach(function (option) { optionFilter.add(new Option(option.name + (option.enabled ? '' : '（已停用）'), option.id)); });
      optionFilter.value = selected;
    } catch (_) { /* List filters remain usable if configuration fails. */ }
  }
  document.addEventListener('correction-config-saved', loadOptions);
  loadOptions();

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
    outcomeSelect.value = button.dataset.reply;
    replyInput.value = template.text;
    updateReplyRequirement();
  });
  statusSelect.addEventListener('change', updateReplyRequirement);
  outcomeSelect.addEventListener('change', function () {
    if (outcomeSelect.value) statusSelect.value = outcomeSelect.value === 'unverified' ? 'ignored' : 'resolved';
    updateReplyRequirement();
  });
  document.getElementById('detailTrackEdit').addEventListener('click', async function () {
    if (!currentIssue || saving) return;
    var id = currentID, trackID = currentIssue.trackID;
    var baseline = serverResult;
    var draft = { status: statusSelect.value, reply: replyInput.value, outcome: outcomeSelect.value };
    var source = structuredCorrection && window.AdminAccess.can('feedbacks', true) ? id : '';
    await new Promise(function (resolve) {
      function restoreDraft(draft) {
    statusSelect.value = draft.status; replyInput.value = draft.reply; outcomeSelect.value = draft.outcome;
    updateReplyRequirement();
  }
  document.getElementById('restoreDraft').addEventListener('click', function () {
    if (saving || !recoveryDraft) return;
    restoreDraft(recoveryDraft); recoveryDraft = null;
    this.classList.add('hidden'); saveErr.textContent = '';
  });
  document.getElementById('detailModal').addEventListener('hidden.bs.modal', resolve, { once: true });
      detailModal.hide();
    });
    try {
      await window.AdminTrackEditor.open(trackID, { sourceFeedbackID: source, waitForHidden: true });
    } catch (error) { AdminUI.alert(error.message || String(error), 'danger'); }
    await loadDetail(id, true);
    if (currentID === id) {
      if (JSON.stringify(baseline) === JSON.stringify(serverResult)) restoreDraft(draft);
      else {
        recoveryDraft = draft;
        document.getElementById('restoreDraft').classList.remove('hidden');
        saveErr.textContent = '处理结果已被其他操作更新，已显示最新内容。可核对后恢复刚才的草稿。';
      }
    }
  });
  document.getElementById('detailModal').addEventListener('hidden.bs.modal', function () {
    detailRequest += 1;
    currentID = '';
  });

  function updateReplyRequirement() {
    var terminal = statusSelect.value === 'resolved' || statusSelect.value === 'ignored';
    replyInput.required = statusSelect.value === 'resolved' || (structuredCorrection && terminal);
    outcomeSelect.required = structuredCorrection && terminal;
    if (!terminal) outcomeSelect.value = '';
    if (statusSelect.value === 'resolved' && outcomeSelect.value === 'unverified') outcomeSelect.value = '';
    if (statusSelect.value === 'ignored' && outcomeSelect.value !== 'unverified') outcomeSelect.value = '';
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
    if (structuredCorrection) { payload.outcome = outcomeSelect.value; payload.revision = currentRevision; }
    if (structuredCorrection && (payload.status === 'resolved' || payload.status === 'ignored') && !payload.outcome) { saveErr.textContent = '请选择处理结果'; return; }
    if ((payload.status === 'resolved' || (structuredCorrection && payload.status === 'ignored')) && !payload.reply) {
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
    if (kindFilter.value) params.set('kind', kindFilter.value);
    if (optionFilter.value) params.set('correction_option_id', optionFilter.value);
    if (trackFilter.value.trim()) params.set('track_id', trackFilter.value.trim());
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
      currentIssue = issue;
      document.getElementById('detailTrackContext').classList.toggle('hidden', !issue);
      if (issue) loadTrackContext(issue.trackID, request);
      structuredCorrection = !!it.correction;
      currentRevision = (it.correction || {}).revision || 0;
      serverResult = { status: it.status || 'pending', reply: it.reply || '', outcome: (it.correction || {}).outcome || '' };
      recoveryDraft = null;
      document.getElementById('restoreDraft').classList.add('hidden');
      outcomeSelect.value = (it.correction || {}).outcome || '';
      document.getElementById('outcomeField').classList.toggle('hidden', !structuredCorrection);
      setText('detailHandler', it.handled_by ? '处理人：' + it.handled_by + ' · ' + fmtTime((it.correction || {}).handled_at) : '');
      var change = it.track_change;
      var fieldNames = {Title:'标题',CityCode:'城市',LocateAddr:'位置',TrackType:'运动类型',Description:'路线说明',Distance:'里程',Duration:'时长',ElevationGain:'爬升',AvgSpeedKmh:'速度',CaloriesBurned:'热量',UserID:'所属用户',SessionID:'会话',SourceTag:'来源',CoordinateSystem:'坐标系',DescriptionSource:'说明来源',DescriptionRuleVersion:'说明版本',StartTime:'开始时间',EndTime:'结束时间',IsRunning:'录制状态',Status:'可见范围',RawTrackURL:'原始文件',TrackScreenshotURL:'截图',TrackNoMapBgScreenshotURL:'无底图截图',PreviewImages:'预览图'};
      setText('detailTrackChange', change ? '最近关联修改：' + (change.fields || []).map(function (field) { return fieldNames[field] || field; }).join('、') + ' · ' + change.operator + ' · ' + fmtTime(change.saved_at) : (issue ? '尚无从此纠错入口保存的资料修改记录。' : ''));
      document.getElementById('detailTrackEdit').disabled = !window.AdminAccess.can('tracks', false);
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

  async function loadTrackContext(trackID, request) {
    var panel = document.getElementById('detailTrackContext');
    if (!window.AdminAccess.can('tracks', false)) { panel.textContent = '当前账号无轨迹查看权限，可核对用户提交的信息。'; return; }
    panel.textContent = '正在读取轨迹当前资料…';
    try {
      var response = await fetch('/admin/api/tracks/' + encodeURIComponent(trackID));
      if (!response.ok) throw new Error(response.status === 404 ? '轨迹已删除或不存在' : '读取失败: ' + response.status);
      var track = ((await response.json()).data) || {};
      if (request !== detailRequest) return;
      panel.replaceChildren();
      var heading = document.createElement('strong'); heading.textContent = '轨迹当前资料'; panel.appendChild(heading);
      [track.title || trackID, [track.city_code, track.locate_addr].filter(Boolean).join(' · '),
        '里程 ' + (Number(track.distance || 0) / 1000).toFixed(2) + ' km · 时长 ' + Math.round(Number(track.duration || 0) / 60) + ' 分钟 · 爬升 ' + Number(track.elevation_gain || 0) + ' m',
        track.description].filter(Boolean).forEach(function (value) { var line = document.createElement('p'); line.textContent = value; panel.appendChild(line); });
    } catch (error) {
      if (request !== detailRequest) return;
      panel.textContent = error.message || String(error);
      var retry = document.createElement('button'); retry.type = 'button'; retry.className = 'btn btn-sm btn-outline-secondary ms-2'; retry.textContent = '重试';
      retry.addEventListener('click', function () { if (request === detailRequest) loadTrackContext(trackID, request); }); panel.appendChild(retry);
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
      var resp = await fetch('/admin/api/dashboard?period=month&scope=feedbacks');
      if (!resp.ok) throw new Error(String(resp.status));
      var data = ((await resp.json()).data) || {};
      var stats = data.feedback || {};
      setText('statPending', stats.pending);
      setText('statProcessing', stats.processing);
      setText('statResolved', stats.resolved);
      setText('statIgnored', stats.ignored);
    } catch (_) {
      ['statPending', 'statProcessing', 'statResolved', 'statIgnored'].forEach(function (id) { setText(id, '—'); });
    }
  }

  loadStats();
  loadPage();
})();
