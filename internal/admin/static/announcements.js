(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  let items = [], cursor = '', listGeneration = 0, listController = null;
  const editor = new tabler.Modal($('noticeEditor'), { backdrop: 'static', keyboard: false });
  let editorVisible = false, editorClosing = false, leavePromise = null, saveGate = false;
  let current = null, detailGeneration = 0, detailController = null, saving = false, baseline = '';
  const date = value => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '立即';
  const kind = value => value === 'activity' ? '活动消息' : '系统消息';
  const status = notice => notice.status === 'draft' ? '草稿' : notice.status === 'withdrawn' ? '已撤回' : new Date(notice.publish_at) > new Date() ? '待定时发布' : '已发布';
  async function api(path, options = {}) {
    const response = await fetch('/admin/api/announcements' + path, { credentials: 'same-origin', ...options });
    const body = await response.json();
    if (!response.ok || body.code && body.code !== 0) throw new Error(body.error || '通知操作失败');
    return body.data;
  }
  function render() {
    const rows = items.map(notice => {
      const tr = document.createElement('tr');
      [kind(notice.kind), notice.title, notice.target_user_ids?.length ? '指定 ' + notice.target_user_ids.length + ' 人' : '全部用户', date(notice.publish_at), status(notice)].forEach((value, index) => {
        const td = document.createElement('td'); td.textContent = value;
        if (index === 1) { td.className = 'notice-title-cell'; const summary = document.createElement('small'); summary.textContent = notice.body; td.appendChild(summary); }
        tr.appendChild(td);
      });
      const action = document.createElement('td'), button = document.createElement('button');
      button.className = 'btn btn-sm btn-outline-primary'; button.textContent = AdminAccess.can('announcements', true) ? '编辑' : '查看';
      button.addEventListener('click', () => open(notice.id)); action.appendChild(button); tr.appendChild(action); return tr;
    });
    if (!rows.length) { const tr = document.createElement('tr'), td = document.createElement('td'); td.colSpan = 6; td.className = 'text-center text-secondary p-4'; td.textContent = '暂无通知'; tr.appendChild(td); rows.push(tr); }
    $('noticeRows').replaceChildren(...rows); $('moreNotices').hidden = !cursor;
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
  function localDate(value) { if (!value) return ''; const d = new Date(value); if (!Number.isFinite(d.getTime())) return ''; const pad = n => String(n).padStart(2, '0'); return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' + pad(d.getMinutes()); }
  function snapshot() { return JSON.stringify(['noticeKind', 'noticeTitle', 'noticeBody', 'noticeURL', 'noticeAudience', 'noticeUsers', 'noticePublishAt', 'noticeExpiresAt'].map(id => $(id).value)); }
  function preview() {
    $('noticeUsersLabel').hidden = $('noticeAudience').value !== 'users';
    $('previewKind').textContent = kind($('noticeKind').value); $('previewTitle').textContent = $('noticeTitle').value || '通知标题';
    $('previewBody').textContent = $('noticeBody').value || '通知正文'; $('previewTime').textContent = $('noticePublishAt').value ? date($('noticePublishAt').value) : '立即';
    $('previewLink').textContent = $('noticeURL').value.trim() ? '点击卡片打开 H5' : '';
  }
  function bind(notice) {
    current = notice; $('noticeSaveError').textContent = ''; $('noticeEditorTitle').textContent = notice.id ? '通知详情' : '新建通知';
    $('noticeKind').value = notice.kind || 'system'; $('noticeTitle').value = notice.title || ''; $('noticeBody').value = notice.body || ''; $('noticeURL').value = notice.h5_url || '';
    $('noticeAudience').value = notice.target_user_ids?.length ? 'users' : 'all'; $('noticeUsers').value = (notice.target_user_ids || []).join('\n');
    $('noticePublishAt').value = notice.publish_at ? localDate(notice.publish_at) : ''; $('noticeExpiresAt').value = localDate(notice.expires_at);
    const editable = AdminAccess.can('announcements', true) && notice.status !== 'withdrawn';
    $('noticeForm').querySelectorAll('input,textarea,select').forEach(input => { input.disabled = !editable; });
    ['noticePublishAt', 'noticeAudience', 'noticeUsers'].forEach(id => { if (notice.id && notice.status !== 'draft') $(id).disabled = true; });
    $('saveNoticeDraft').hidden = !editable || !!notice.id && notice.status !== 'draft'; $('publishNotice').hidden = !editable;
    $('publishNotice').textContent = notice.id && notice.status === 'published' ? '保存修改' : '发布通知';
    $('withdrawNotice').hidden = !editable || notice.status !== 'published'; $('noticeSavedState').textContent = notice.id ? status(notice) : '';
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
    try { await saveNotice(nextStatus); } finally { saveGate = false; }
  }
  async function saveNotice(nextStatus) {
    if (saving || !AdminAccess.can('announcements', true)) return;
    if (!$('noticeForm').reportValidity()) return;
    const targets = $('noticeAudience').value === 'users' ? [...new Set($('noticeUsers').value.split(/[\s,，]+/).filter(Boolean))] : [];
    if ($('noticeAudience').value === 'users' && (!targets.length || targets.some(id => !/^[1-9]\d*$/.test(id)))) { $('noticeSaveError').textContent = '请填写有效的用户ID'; return; }
    if ($('noticeURL').value && !/^https:\/\//i.test($('noticeURL').value.trim())) { $('noticeSaveError').textContent = 'H5链接须使用HTTPS'; return; }
    if (nextStatus === 'withdrawn' && !await AdminUI.confirm({ title: '撤回通知？', message: '撤回后客户端将隐藏此通知。', confirmText: '撤回' })) return;
    if (nextStatus === 'published' && current?.status !== 'published' && !await AdminUI.confirm({ title: '发布通知？', message: targets.length ? '将发送给指定的 ' + targets.length + ' 位用户。' : '将发送给全部用户。', confirmText: '发布' })) return;
    const owner = current; saving = true; $('noticeForm').inert = true; $('noticeSaveError').textContent = '';
    const controller = new AbortController(); detailController = controller; const timeout = setTimeout(() => controller.abort(), 30000);
    try {
      const notice = { ...owner, kind: $('noticeKind').value, title: $('noticeTitle').value.trim(), body: $('noticeBody').value.trim(), h5_url: $('noticeURL').value.trim(), status: nextStatus, target_user_ids: targets,
        publish_at: owner?.status === 'published' || owner?.status === 'withdrawn' ? owner.publish_at : $('noticePublishAt').value ? new Date($('noticePublishAt').value).toISOString() : new Date().toISOString(), expires_at: $('noticeExpiresAt').value ? new Date($('noticeExpiresAt').value).toISOString() : null };
      const result = await api(owner.id ? '/' + encodeURIComponent(owner.id) : '', { method: owner.id ? 'PUT' : 'POST', signal: controller.signal, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expected_version: owner.revision || 0, notice }) });
      if (current !== owner) return; bind(result); await load();
    } catch (error) { if (current === owner) $('noticeSaveError').textContent = error.name === 'AbortError' ? '保存超时，请刷新核对后重试' : error.message; }
    finally { clearTimeout(timeout); saving = false; $('noticeForm').inert = false; }
  }
  $('newNotice').addEventListener('click', () => open('')); $('refreshNotices').addEventListener('click', () => load()); $('moreNotices').addEventListener('click', () => load(true));
  $('closeNotice').addEventListener('click', close);
  $('noticeEditor').addEventListener('hidden.bs.modal', () => { editorVisible = false; editorClosing = false; });
  $('noticeEditor').addEventListener('keydown', event => { if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); } });
  $('noticeForm').addEventListener('input', preview); $('noticeForm').addEventListener('submit', event => { event.preventDefault(); save('published'); });
  $('saveNoticeDraft').addEventListener('click', () => save('draft')); $('withdrawNotice').addEventListener('click', () => save('withdrawn'));
  window.addEventListener('pagehide', () => { listController?.abort(); detailController?.abort(); }); load();
})();
