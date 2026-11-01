(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const form = $('accountForm');
  const modal = new tabler.Modal($('accountModal'), { backdrop: 'static' });
  let modules = [], selected = null, saving = false;
  let auditRequest = 0;

  async function api(path, options) {
    const response = await fetch('/admin/api' + path, options);
    const payload = await response.json().catch(() => ({}));
    if (response.status === 401) location.href = '/admin/login.html';
    if (!response.ok) throw new Error(payload.error || '请求失败，请重试');
    return payload.data;
  }
  function cell(row, value) { const td = document.createElement('td'); td.textContent = value; row.appendChild(td); return td; }
  function time(value) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'; }
  function showError(element, message) { element.textContent = message; element.hidden = !message; }

  async function load() {
    $('refreshAccounts').disabled = true;
    try {
      const data = await api('/accounts');
      modules = data.modules || [];
      updateAuditAccounts(data.items || []);
      $('emailMigrationHint').hidden = !(data.items || []).some(account => account.can_reset_password);
      $('accountsList').replaceChildren();
      (data.items || []).forEach(account => {
        const row = document.createElement('tr');
        cell(row, account.username); cell(row, account.display_name);
        const badge = document.createElement('span'); badge.className = 'badge ' + (account.role === 'admin' ? 'bg-purple-lt' : 'bg-blue-lt'); badge.textContent = account.role === 'admin' ? '管理员' : '操作员'; cell(row, '').appendChild(badge);
        cell(row, account.enabled ? '启用' : '停用');
        cell(row, account.recovery_email_bound ? '已验证' : '未绑定'); cell(row, time(account.last_login_at));
        const actions = cell(row, ''); actions.className = 'row-actions';
        const edit = document.createElement('button'); edit.type = 'button'; edit.className = 'btn btn-sm btn-outline-primary'; edit.textContent = '设置'; edit.onclick = () => openEditor(account);
        actions.appendChild(edit);
        if (account.can_reset_password) {
          const reset = document.createElement('button'); reset.type = 'button'; reset.className = 'btn btn-sm btn-outline-secondary'; reset.textContent = '应急重置'; reset.title = '仅供未绑定邮箱账号在迁移期恢复登录'; reset.onclick = () => resetPassword(account);
          actions.appendChild(reset);
        }
        $('accountsList').appendChild(row);
      });
      showError($('accountsError'), '');
      $('createAccount').disabled = false;
    } catch (error) { showError($('accountsError'), error.message); }
    finally { $('refreshAccounts').disabled = false; }
  }

  function updateAuditAccounts(accounts) {
    const filter = $('auditAccountFilter'), previous = filter.value;
    const all = document.createElement('option'); all.value = ''; all.textContent = '全部账号';
    filter.replaceChildren(all);
    accounts.forEach(account => {
      const option = document.createElement('option'); option.value = account.username;
      option.textContent = (account.display_name && account.display_name !== account.username ? account.display_name + '（' + account.username + '）' : account.username) + (account.enabled ? '' : '（已停用）');
      filter.appendChild(option);
    });
    filter.value = accounts.some(account => account.username === previous) ? previous : '';
    filter.disabled = false;
    if (filter.value !== previous) loadAudit();
  }

  function updateRole() {
    const admin = form.elements.role.value === 'admin';
    $('permissionMatrix').hidden = admin; $('adminPermissionHint').hidden = !admin; $('operatorPermissionHint').hidden = admin;
  }
  function openEditor(account) {
    selected = account;
    form.reset(); showError($('accountFormError'), '');
    $('accountTitle').textContent = account ? '账号设置 · ' + account.username : '新增账号';
    form.elements.username.disabled = !!account;
    form.elements.username.value = account ? account.username : '';
    form.elements.display_name.value = account ? account.display_name : '';
    form.elements.role.value = account ? account.role : 'operator';
    form.elements.enabled.value = account ? String(account.enabled) : 'true';
    form.elements.password.required = !account;
    form.elements.password.disabled = !!account;
    $('initialPasswordField').hidden = !!account;
    $('permissionRows').replaceChildren();
    modules.forEach(module => {
      const row = document.createElement('tr'); cell(row, module.name);
      const select = document.createElement('select'); select.className = 'form-select form-select-sm'; select.name = 'permission_' + module.id;
      [['none', '无权限'], ['read', '只读'], ['write', '可操作']].forEach(([value, label]) => { const option = document.createElement('option'); option.value = value; option.textContent = label; select.appendChild(option); });
      select.value = account ? ((account.permissions || {})[module.id] || 'none') : 'none';
      cell(row, '').appendChild(select); $('permissionRows').appendChild(row);
    });
    updateRole(); modal.show();
  }

  form.addEventListener('submit', async event => {
    event.preventDefault();
    if (saving || !form.reportValidity()) return;
    saving = true; $('saveAccount').disabled = true;
    const body = { username: form.elements.username.value, display_name: form.elements.display_name.value, role: form.elements.role.value, enabled: form.elements.enabled.value === 'true', permissions: {}, revision: selected ? selected.revision : 0 };
    modules.forEach(module => { body.permissions[module.id] = form.elements['permission_' + module.id].value; });
    if (!selected) body.password = form.elements.password.value;
    try {
      await api('/accounts' + (selected ? '/' + encodeURIComponent(selected.username) : ''), { method: selected ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      form.elements.password.value = '';
      saving = false; modal.hide(); AdminUI.alert('账号设置已保存', 'success');
      if (selected && selected.username === AdminAccess.identity().username && (body.role !== 'admin' || !body.enabled)) { location.href = '/admin/access.html'; return; }
      await Promise.all([load(), loadAudit()]);
    } catch (error) { showError($('accountFormError'), error.message); }
    finally { saving = false; $('saveAccount').disabled = false; }
  });
  $('accountModal').addEventListener('hide.bs.modal', event => { if (saving) event.preventDefault(); });
  $('accountModal').addEventListener('hidden.bs.modal', () => { form.elements.password.value = ''; });

  async function resetPassword(account) {
    const values = await AdminUI.form({ title: '迁移期应急重置 · ' + account.username, confirmText: '重置并退出该账号全部登录', fields: [{ name: 'password', label: '临时密码（仅限未绑定邮箱账号，下次登录需修改）', type: 'password', required: true, maxLength: 72 }] });
    if (!values) return;
    try {
      await api('/accounts/' + encodeURIComponent(account.username) + '/reset-password', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password: values.password, revision: account.revision }) });
      values.password = '';
      if (account.username === AdminAccess.identity().username) { location.href = '/admin/login.html'; return; }
      AdminUI.alert('密码已重置，该账号下次登录需要修改密码', 'success');
      await Promise.all([load(), loadAudit()]);
    } catch (error) { AdminUI.alert(error.message, 'danger'); }
  }

  async function loadAudit() {
    const request = ++auditRequest, username = $('auditAccountFilter').value;
    $('refreshAudit').disabled = true;
    showError($('auditError'), '');
    auditState('正在加载…');
    $('auditHint').textContent = '展示' + (username ? '所选账号' : '所有账号') + '最近的修改、处理和发布等操作，同一操作的发起与结果合并显示，不包含普通浏览记录。';
    try {
      const params = new URLSearchParams();
      if (username) params.set('username', username);
      const data = await api('/audit-logs' + (username ? '?' + params.toString() : ''));
      if (request !== auditRequest) return;
      const items = AdminAuditLog.visibleEntries(data.items || []), rows = document.createDocumentFragment();
      items.forEach(item => {
        const description = AdminAuditLog.describe(item), outcome = AdminAuditLog.outcome(item);
        const row = document.createElement('tr');
        cell(row, time(item.created_at)); cell(row, item.actor_id === 'email-recovery' ? '邮箱找回（未登录）' : item.username);
        const action = cell(row, description.action);
        if (item.method === 'POST' && item.path === '/admin/api/scenic-spots/provider-content/backfill') {
          const automatic = document.createElement('div'); automatic.className = 'small text-secondary';
          automatic.textContent = '景点页可自动触发 · 分批同步'; action.appendChild(automatic);
        }
        const details = document.createElement('details'); details.className = 'admin-audit-details';
        const summary = document.createElement('summary'); summary.textContent = '说明与请求详情'; details.appendChild(summary);
        [description.note, outcome.note, '请求：' + item.method + ' ' + item.path,
          'HTTP 状态：' + (item.status || '尚未记录最终响应'), '记录编号：' + item.id].filter(Boolean).forEach(text => {
          const line = document.createElement('p'); line.textContent = text; details.appendChild(line);
        });
        action.appendChild(details);
        cell(row, description.target);
        const result = cell(row, ''), badge = document.createElement('span');
        badge.className = 'badge ' + outcome.color; badge.textContent = outcome.label; badge.title = outcome.note;
        result.appendChild(badge);
        if (!item.status || item.status >= 300) {
          const reason = document.createElement('div'); reason.className = 'admin-audit-result-note'; reason.textContent = outcome.note;
          result.appendChild(reason);
        }
        rows.appendChild(row);
      });
      $('auditList').replaceChildren(rows);
      if (!items.length) auditState(username ? '该账号暂无操作记录' : '暂无操作记录');
      showError($('auditError'), '');
    } catch (error) {
      if (request !== auditRequest) return;
      showError($('auditError'), error.message); auditState('加载失败，请刷新重试');
    } finally { if (request === auditRequest) $('refreshAudit').disabled = false; }
  }
  function auditState(message) {
    const row = document.createElement('tr');
    const td = cell(row, message); td.colSpan = 5; td.className = 'text-secondary text-center';
    $('auditList').replaceChildren(row);
  }
  form.elements.role.addEventListener('change', updateRole);
  $('createAccount').disabled = true;
  $('createAccount').onclick = () => openEditor(null);
  $('refreshAccounts').onclick = load;
  $('refreshAudit').onclick = loadAudit;
  $('auditAccountFilter').addEventListener('change', loadAudit);
  load(); loadAudit();
})();
