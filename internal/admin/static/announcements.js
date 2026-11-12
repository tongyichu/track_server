(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  let items = [], cursor = '', listGeneration = 0, listController = null;
  const editor = new tabler.Modal($('noticeEditor'), { backdrop: 'static', keyboard: false });
  let editorVisible = false, editorClosing = false, leavePromise = null, saveGate = false;
  let current = null, detailGeneration = 0, detailController = null, saving = false, baseline = '';
  const date = value => AdminTime.dateTime(value, value ? '—' : '立即');
  const kind = value => value === 'activity' ? '活动消息' : '系统消息';
  const phase = notice => notice.status === 'published' && !notice.published_at && new Date(notice.publish_at) > new Date() ? 'scheduled' : notice.status;
  const status = notice => phase(notice) === 'published' && notice.delivery_ends_at && new Date(notice.delivery_ends_at) <= new Date() ? '已结束发放' : ({ draft: '草稿', scheduled: '待发布', withdrawn: '已撤回', expired: '已结束', published: '已上线' })[phase(notice)] || '草稿';
  const coverage = notice => notice.delivery_mode === 'persistent' ? '持续公告' : notice.delivery_mode === 'once' ? '一次性通知' : '原有通知';
  const historyActions = { edited: '保存修改', draft: '保存草稿', scheduled: '安排定时发布', published: '上线', withdrawn: '撤回', cancelled: '取消定时', expired: '到期未发放' };
  const renderedRows = new Map();
  async function api(path, options = {}) {
    const response = await fetch('/admin/api/announcements' + path, { credentials: 'same-origin', ...options });
    const body = await response.json();
    if (!response.ok || body.code && body.code !== 0) throw new Error(body.error || '通知操作失败');
    return body.data;
  }
  function render() {
    const rows = items.map(notice => {
      const values = [kind(notice.kind), notice.title, coverage(notice), notice.target_user_ids?.length ? '指定 ' + notice.target_user_ids.length + ' 人' : '全部用户', date(notice.publish_at), status(notice)];
      const fingerprint = JSON.stringify([values, notice.body, AdminAccess.can('announcements', true)]);
      const cached = renderedRows.get(notice.id);
      if (cached?.fingerprint === fingerprint) return cached.row;
      const tr = document.createElement('tr');
      values.forEach((value, index) => {
        const td = document.createElement('td'); td.textContent = value;
        if (index === 1) { td.className = 'notice-title-cell'; const summary = document.createElement('small'); summary.textContent = notice.body; td.appendChild(summary); }
        tr.appendChild(td);
      });
      const action = document.createElement('td'), button = document.createElement('button');
      button.className = 'btn btn-sm btn-outline-primary'; button.textContent = AdminAccess.can('announcements', true) ? '编辑' : '查看';
      button.addEventListener('click', () => open(notice.id)); action.appendChild(button); tr.appendChild(action);
      renderedRows.set(notice.id, { fingerprint, row: tr }); return tr;
    });
    const ids = new Set(items.map(notice => notice.id));
    for (const id of renderedRows.keys()) if (!ids.has(id)) renderedRows.delete(id);
    if (!rows.length) { const tr = document.createElement('tr'), td = document.createElement('td'); td.colSpan = 7; td.className = 'text-center text-secondary p-4'; td.textContent = '暂无通知'; tr.appendChild(td); rows.push(tr); }
    const parent = $('noticeRows'), wanted = new Set(rows);
    for (const row of [...parent.children]) if (!wanted.has(row)) row.remove();
    rows.forEach((row, index) => { if (parent.children[index] !== row) parent.insertBefore(row, parent.children[index] || null); });
    $('moreNotices').hidden = !cursor;
  }
  async function load(append = false) {
    listController?.abort(); const controller = new AbortController(), generation = ++listGeneration; listController = controller;
    $('refreshNotices').disabled = true; $('moreNotices').disabled = true; $('noticeListError').textContent = '';
    try {
      const page = await api('?limit=30' + (append && cursor ? '&cursor=' + encodeURIComponent(cursor) : ''), { signal: controller.signal });
      if (generation !== listGeneration) return;
      const merged = append ? [...items, ...(page.items || [])] : page.items || [];
      items = [...new Map(merged.map(item => [item.id, item])).values()]; cursor = page.has_more ? page.next_cursor || '' : ''; render();
    } catch (error) { if (!controller.signal.aborted) $('noticeListError').textContent = error.message; }
    finally { if (generation === listGeneration) { $('refreshNotices').disabled = false; $('moreNotices').disabled = false; } }
  }
  function localDate(value) { return AdminTime.toInput(value); }
  function inputTime(value, original) { return value && value === localDate(original) ? original : AdminTime.fromInput(value); }
  function snapshot() { return JSON.stringify(['noticeKind', 'noticeTitle', 'noticeBody', 'noticeURL', 'noticeAudience', 'noticeUsers', 'noticePublishAt', 'noticeExpiresAt', 'noticeDeliveryMode', 'noticeDeliveryWindowMode', 'noticeDeliveryEndsAt', 'noticePublishMode'].map(id => $(id).value)); }
  function preview() {
    $('noticeUsersLabel').hidden = $('noticeAudience').value !== 'users';
    const persistent = $('noticeDeliveryMode').value === 'persistent';
    const scheduled = $('noticePublishMode').value === 'scheduled';
    $('noticeDeliveryWindow').hidden = !persistent;
    $('noticeDeliveryEndLabel').hidden = !persistent || $('noticeDeliveryWindowMode').value !== 'until';
    $('noticeDeliveryEndsAt').required = !$('noticeDeliveryEndLabel').hidden;
    $('noticePublishAtLabel').hidden = !scheduled;
    $('noticePublishAt').required = scheduled;
    $('noticeDeliveryHint').textContent = persistent ? '有效期内覆盖符合范围的新老用户，每人每轮提醒一次。' : $('noticeDeliveryMode').value === 'legacy' ? '保留原有接收与已读规则；撤回重新发布后可选择新的覆盖方式。' : '仅面向发布时间之前已注册的目标用户，离线后回来仍可接收。';
    $('publishNotice').textContent = phase(current || {}) === 'published' ? '保存修改' : scheduled ? (phase(current || {}) === 'scheduled' ? '保存定时安排' : '确认定时发布') : (current?.publish_round > 0 ? '重新发布' : '发布通知');
    $('previewKind').textContent = kind($('noticeKind').value); $('previewTitle').textContent = $('noticeTitle').value || '通知标题';
    $('previewBody').textContent = $('noticeBody').value || '通知正文';
    try { $('previewTime').textContent = scheduled && $('noticePublishAt').value ? date(inputTime($('noticePublishAt').value, current?.publish_at)) : '立即发布'; }
    catch (_) { $('previewTime').textContent = '请输入有效的北京时间'; }
    $('previewLink').textContent = $('noticeURL').value.trim() ? '点击卡片打开 H5' : '';
  }
  function bind(notice) {
    current = notice; $('noticeSaveError').textContent = ''; $('noticeEditorTitle').textContent = notice.id ? '通知详情' : '新建通知';
    $('noticeKind').value = notice.kind || 'system'; $('noticeTitle').value = notice.title || ''; $('noticeBody').value = notice.body || ''; $('noticeURL').value = notice.h5_url || '';
    $('noticeAudience').value = notice.target_user_ids?.length ? 'users' : 'all'; $('noticeUsers').value = (notice.target_user_ids || []).join('\n');
    $('noticePublishAt').value = notice.publish_at ? localDate(notice.publish_at) : ''; $('noticeExpiresAt').value = localDate(notice.expires_at);
    $('noticeDeliveryMode').value = notice.delivery_mode || (phase(notice) === 'published' ? 'legacy' : 'once');
    $('noticeDeliveryMode').querySelector('[value="legacy"]').hidden = $('noticeDeliveryMode').value !== 'legacy';
    $('noticeDeliveryWindowMode').value = notice.delivery_ends_at ? 'until' : 'unlimited';
    $('noticeDeliveryEndsAt').value = localDate(notice.delivery_ends_at);
    $('noticePublishMode').value = phase(notice) === 'scheduled' ? 'scheduled' : notice.publish_mode || 'immediate';
    const editable = AdminAccess.can('announcements', true), live = phase(notice) === 'published';
    $('noticeForm').querySelectorAll('input,textarea,select').forEach(input => { input.disabled = !editable; });
    ['noticePublishMode', 'noticePublishAt', 'noticeAudience', 'noticeUsers', 'noticeDeliveryMode', 'noticeDeliveryWindowMode', 'noticeDeliveryEndsAt'].forEach(id => { if (live) $(id).disabled = true; });
    $('saveNoticeDraft').hidden = !editable || live || phase(notice) === 'scheduled'; $('publishNotice').hidden = !editable;
    $('withdrawNotice').hidden = !editable || !live;
    $('cancelNoticeSchedule').hidden = !editable || phase(notice) !== 'scheduled';
    $('noticeSavedState').textContent = notice.id ? status(notice) + (notice.publish_round ? ' · 第' + notice.publish_round + '轮' : '') : '';
    $('noticeActualPublishedAt').hidden = !notice.published_at;
    $('noticeActualPublishedAt').textContent = notice.published_at ? '实际上线：' + date(notice.published_at) : '';
    $('noticeHistory').hidden = !notice.history?.length;
    $('noticeHistoryRows').replaceChildren(...[...(notice.history || [])].reverse().map(entry => {
      const li = document.createElement('li'); li.textContent = date(entry.at) + ' · ' + (historyActions[entry.action] || entry.action) + ' · ' + entry.actor + (entry.round ? ' · 第' + entry.round + '轮' : '') + ' · ' + entry.title + (entry.publish_at ? ' · 发布时间：' + date(entry.publish_at) : ''); return li;
    }));
    preview(); baseline = snapshot(); if (!editorVisible) { editorVisible = true; editor.show(); } AdminAccess.apply();
  }
  async function canLeave() {
    if (saving || saveGate) return false;
    if (!current || snapshot() === baseline) return true;
    if (!leavePromise) leavePromise = AdminUI.confirm({ title: '放弃未保存修改？', message: '通知尚未保存。', confirmText: '放弃修改' });
    try { return await leavePromise; } finally { leavePromise = null; }
  }
  async function open(id) {
    if (editorClosing || !await canLeave() || editorClosing) return;
    detailController?.abort(); const controller = new AbortController(), generation = ++detailGeneration; detailController = controller;
    if (!id) { bind({}); return; }
    try { const notice = await api('/' + encodeURIComponent(id), { signal: controller.signal }); if (generation === detailGeneration && !controller.signal.aborted) bind(notice); }
    catch (error) { if (!controller.signal.aborted) AdminUI.alert(error.message, 'danger'); }
  }
  async function close() { if (!await canLeave()) return; detailGeneration++; detailController?.abort(); current = null; if (editorVisible) { editorClosing = true; editor.hide(); } }
  async function save(nextStatus) {
    if (saveGate || saving) return;
    saveGate = true;
    try { await saveNotice(nextStatus); } catch (error) { $('noticeSaveError').textContent = error.message || '请检查通知内容和时间'; } finally { saveGate = false; }
  }
  async function saveNotice(nextStatus) {
    if (saving || !AdminAccess.can('announcements', true)) return;
    const cancelling = nextStatus === 'cancelled';
    $('noticePublishAt').required = nextStatus === 'published' && phase(current || {}) !== 'published' && $('noticePublishMode').value === 'scheduled';
    $('noticeDeliveryEndsAt').required = nextStatus === 'published' && !$('noticeDeliveryEndLabel').hidden;
    if (!cancelling && !$('noticeForm').reportValidity()) return;
    const targets = $('noticeAudience').value === 'users' ? [...new Set($('noticeUsers').value.split(/[\s,，]+/).filter(Boolean))] : [];
    if (!cancelling && $('noticeAudience').value === 'users' && (!targets.length || targets.some(id => !/^[1-9]\d*$/.test(id)))) { $('noticeSaveError').textContent = '请填写有效的用户ID'; return; }
    if (!cancelling && $('noticeURL').value && !/^https:\/\//i.test($('noticeURL').value.trim())) { $('noticeSaveError').textContent = 'H5链接须使用HTTPS'; return; }
    if (nextStatus === 'withdrawn' && !await AdminUI.confirm({ title: '撤回通知？', message: '撤回后客户端将隐藏此通知。', confirmText: '撤回' })) return;
    const live = phase(current || {}) === 'published', scheduled = $('noticePublishMode').value === 'scheduled';
    if (cancelling && !await AdminUI.confirm({ title: '取消定时发布？', message: '取消后保留为草稿，当前未保存的修改不生效。', confirmText: '取消定时' })) return;
    if (nextStatus === 'published' && !live) {
      let when = '立即发布';
      if (scheduled) {
        const planned = inputTime($('noticePublishAt').value, current?.publish_at);
        if (!planned || new Date(planned) <= new Date()) { $('noticeSaveError').textContent = '定时发布时间已过去，请重新选择'; return; }
        when = date(planned) + '（北京时间）上线';
      }
      if (!await AdminUI.confirm({ title: scheduled ? '确认定时发布？' : '发布通知？', message: when + '，接收范围：' + (targets.length ? '指定' + targets.length + '位用户' : '全部用户') + '。', confirmText: scheduled ? '确认定时' : '发布' })) return;
    }
    const owner = current; saving = true; $('noticeForm').inert = true; $('noticeSaveError').textContent = '';
    const controller = new AbortController(); detailController = controller; const timeout = setTimeout(() => controller.abort(), 30000);
    try {
      const notice = cancelling ? {
        kind: owner.kind, title: owner.title, body: owner.body, h5_url: owner.h5_url, status: 'draft', target_user_ids: owner.target_user_ids,
        publish_at: owner.publish_at, publish_mode: owner.publish_mode || 'scheduled', expires_at: owner.expires_at,
        delivery_mode: owner.delivery_mode || 'once', delivery_ends_at: owner.delivery_ends_at
      } : {
        kind: $('noticeKind').value, title: $('noticeTitle').value.trim(), body: $('noticeBody').value.trim(), h5_url: $('noticeURL').value.trim(), status: nextStatus,
        target_user_ids: targets, delivery_mode: $('noticeDeliveryMode').value,
        publish_mode: live ? owner.publish_mode || '' : $('noticePublishMode').value,
        publish_at: live ? owner.publish_at : scheduled ? inputTime($('noticePublishAt').value, owner?.publish_at) : null,
        delivery_ends_at: $('noticeDeliveryMode').value === 'persistent' && $('noticeDeliveryWindowMode').value === 'until' ? inputTime($('noticeDeliveryEndsAt').value, owner?.delivery_ends_at) : null,
        expires_at: inputTime($('noticeExpiresAt').value, owner?.expires_at) || null
      };
      const result = await api(owner.id ? '/' + encodeURIComponent(owner.id) : '', { method: owner.id ? 'PUT' : 'POST', signal: controller.signal, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expected_version: owner.revision || 0, notice }) });
      if (current !== owner) return; bind(result); await load();
    } catch (error) { if (current === owner) $('noticeSaveError').textContent = error.name === 'AbortError' ? '保存超时，请刷新核对后重试' : error.message; }
    finally { clearTimeout(timeout); saving = false; $('noticeForm').inert = false; }
  }
  $('newNotice').addEventListener('click', () => open('')); $('refreshNotices').addEventListener('click', () => load()); $('moreNotices').addEventListener('click', () => load(true));
  $('closeNotice').addEventListener('click', close);
  $('noticeEditor').addEventListener('hidden.bs.modal', () => { editorVisible = false; editorClosing = false; });
  $('noticeEditor').addEventListener('keydown', event => { if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); } });
  $('noticeForm').addEventListener('change', preview);
  $('noticeForm').addEventListener('input', preview); $('noticeForm').addEventListener('submit', event => { event.preventDefault(); save('published'); });
  $('cancelNoticeSchedule').addEventListener('click', () => save('cancelled'));
  $('saveNoticeDraft').addEventListener('click', () => save('draft')); $('withdrawNotice').addEventListener('click', () => save('withdrawn'));
  window.addEventListener('pagehide', () => { listController?.abort(); detailController?.abort(); }); load();
})();
