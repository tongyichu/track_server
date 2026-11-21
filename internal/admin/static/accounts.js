(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const form = $('accountForm');
  const modal = new AdminComponents.Dialog($('accountModal'), { backdrop: 'static' });
  const resetForm = $('accountResetForm');
  const resetModal = new AdminComponents.Dialog($('accountResetModal'), { backdrop: 'static' });
  const username = AdminAccess.identity()?.username;
  const permissionGroups = [
    ['内容与路线', ['tracks', 'track_submissions', 'scenic_spots', 'regions', 'route_groups']],
    ['用户与运营', ['users', 'companions', 'feedbacks', 'announcements', 'music']],
    ['系统与工具', ['dashboard', 'realtime', 'releases', 'analytics', 'ai_usage', 'ai_tasks', 'ai_settings']]
  ];
  let modules = [], selected = null, saving = false;
  let resetTarget = null, resetting = false;
  let auditRequest = 0;
  let renderedAuditFilter = null;

  async function api(path, options) {
    const response = await fetch('/admin/api' + path, options);
    const payload = await response.json().catch(() => ({}));
    if (response.status === 401) location.href = '/admin/login.html';
    if (!response.ok) throw new Error(payload.error || '请求失败，请重试');
    return payload.data;
  }
  function cell(row, value) { const td = document.createElement('td'); td.textContent = value; row.appendChild(td); return td; }
  function time(value) { return AdminTime.dateTime(value); }
  function showError(element, message) { element.textContent = message; element.hidden = !message; }
  function listState(list, columns, message) {
    const row = document.createElement('tr');
    const status = cell(row, message);
    status.colSpan = columns;
    status.className = 'text-secondary text-center';
    list.replaceChildren(row);
  }

  function renderAccount(account) {
    const row = document.createElement('tr');
    const name = document.createElement('strong');
    name.textContent = account.display_name || account.username;
    const identity = cell(row, '');
    identity.className = 'account-identity';
    identity.appendChild(name);
    if (account.username === username) {
      const self = document.createElement('span');
      self.className = 'account-self';
      self.textContent = '本人';
      identity.appendChild(self);
    }
    const meta = document.createElement('small');
    meta.className = 'text-secondary';
    const role = account.role === 'admin' ? '管理员' : '操作员';
    meta.textContent = name.textContent === account.username ? role : account.username + ' · ' + role;
    identity.appendChild(meta);

    const status = account.enabled ? (account.must_change_password ? '待改密' : '启用') : '停用';
    const statusLabel = document.createElement('span');
    statusLabel.className = status === '待改密' ? 'account-password-pending' : (account.enabled ? '' : 'text-secondary');
    statusLabel.textContent = status;
    const statusCell = cell(row, '');
    statusCell.className = 'account-status';
    statusCell.appendChild(statusLabel);
    cell(row, account.recovery_email_bound ? '已验证' : '未绑定').className = 'account-recovery';
    cell(row, time(account.last_login_at)).className = 'account-login-time';
    const loginSource = cell(row, account.last_login_ip || '—');
    loginSource.className = 'account-login-source';
    if (account.last_login_ip) {
      const location = document.createElement('small');
      location.className = 'account-login-location text-secondary';
      location.textContent = account.last_login_location || '所在地未知';
      loginSource.append(location);
    }

    const actions = document.createElement('div');
    actions.className = 'account-row-actions';
    const actionCell = cell(row, '');
    actionCell.className = 'account-actions';
    actionCell.appendChild(actions);
    const edit = document.createElement('button');
    edit.type = 'button'; edit.className = 'btn btn-sm btn-outline-primary btn-touch'; edit.textContent = '设置';
    edit.setAttribute('aria-label', '设置账号 ' + account.username);
    edit.onclick = () => openEditor(account);
    actions.appendChild(edit);
    if (account.can_reset_password) {
      const dropdown = document.createElement('div');
      dropdown.className = 'dropdown';
      const more = document.createElement('button');
      more.type = 'button'; more.className = 'btn btn-sm btn-outline-secondary dropdown-toggle btn-touch'; more.textContent = '更多';
      more.setAttribute('data-admin-menu', '');
      more.setAttribute('aria-expanded', 'false');
      more.setAttribute('aria-label', '账号 ' + account.username + ' 的更多操作');
      const menu = document.createElement('div');
      menu.className = 'dropdown-menu dropdown-menu-end';
      const reset = document.createElement('button');
      reset.type = 'button'; reset.className = 'dropdown-item'; reset.textContent = '应急重置密码';
      reset.onclick = () => openReset(account);
      menu.appendChild(reset);
      dropdown.append(more, menu);
      actions.appendChild(dropdown);
    }
    return row;
  }

  async function load() {
    $('refreshAccounts').disabled = true;
    try {
      const data = await api('/accounts');
      modules = data.modules || [];
      const accounts = data.items || [];
      $('loginLocationAttribution').hidden = data.login_location_source !== 'db-ip';
      updateAuditAccounts(accounts);
      const ownAccount = accounts.find(account => account.username === username);
      const needsOwnEmail = ownAccount && !ownAccount.recovery_email_bound;
      const canReset = accounts.some(account => account.can_reset_password);
      $('emailMigrationHint').hidden = !canReset && !needsOwnEmail;
      $('verifyOwnRecoveryEmail').hidden = !needsOwnEmail;
      $('emailMigrationText').textContent = canReset ? '未验证邮箱的账号暂可应急重置，全部启用账号验证后永久关闭。' : '当前账号还未验证找回邮箱。';
      if (accounts.length) $('accountsList').replaceChildren(...accounts.map(renderAccount));
      else listState($('accountsList'), 6, '暂无后台账号');
      showError($('accountsError'), '');
      $('createAccount').disabled = false;
    } catch (error) {
      showError($('accountsError'), error.message);
      if ($('createAccount').disabled) listState($('accountsList'), 6, '账号加载失败，请刷新重试');
    }
    finally { $('refreshAccounts').disabled = false; }
  }

  function updateAuditAccounts(accounts) {
    const filter = $('auditAccountFilter'), previous = filter.value;
    const all = document.createElement('option'); all.value = ''; all.textContent = '全部操作人';
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
    $('permissionMatrix').hidden = admin;
    $('adminPermissionHint').hidden = !admin;
    updateConfirmation();
  }

  function updateConfirmation() {
    const checkbox = $('confirmAccountChange');
    const role = form.elements.role.value;
    const isCurrent = selected && selected.username === username;
    let message = '';
    if (isCurrent && form.elements.enabled.value === 'false') {
      message = '停用当前账号会退出该账号的全部后台登录。';
    } else if (isCurrent && selected.role === 'admin' && role !== 'admin') {
      message = '当前账号会失去管理员权限和账号管理入口。';
    } else if (role === 'admin' && selected?.role !== 'admin') {
      message = '该账号将拥有全部后台页面和操作权限，包括管理其他账号。';
    }
    $('accountChangeMessage').textContent = message ? '我确认：' + message : '';
    $('accountChangeConfirm').hidden = !message;
    checkbox.required = !!message;
    checkbox.checked = false;
  }

  function renderPermissions(account) {
    const container = $('permissionRows');
    container.replaceChildren();
    const sections = new Map(permissionGroups.map(([label]) => [label, []]));
    modules.forEach(module => {
      const section = permissionGroups.find(([, ids]) => ids.includes(module.id))?.[0] || '系统与工具';
      sections.get(section).push(module);
    });
    sections.forEach((items, title) => {
      if (!items.length) return;
      const group = document.createElement('details');
      group.open = !window.matchMedia('(max-width: 767px)').matches;
      const heading = document.createElement('summary');
      const count = document.createElement('span');
      count.className = 'account-permission-count';
      heading.append(title, count);
      group.appendChild(heading);
      items.forEach(module => {
        const label = document.createElement('label');
        label.className = 'account-permission-row';
        const name = document.createElement('span');
        name.textContent = module.name;
        const select = document.createElement('select');
        select.className = 'form-select form-select-sm';
        select.name = 'permission_' + module.id;
        select.add(new Option('无权限', 'none'));
        select.add(new Option('只读', 'read'));
        select.add(new Option('可操作', 'write'));
        select.value = account?.permissions?.[module.id] || 'none';
        label.append(name, select);
        group.appendChild(label);
      });
      function updateCount() {
        const granted = Array.from(group.querySelectorAll('select')).filter(input => input.value !== 'none').length;
        count.textContent = '已授权 ' + granted + ' / ' + items.length;
      }
      group.addEventListener('change', updateCount);
      updateCount();
      container.appendChild(group);
    });
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
    $('newUsernameHint').hidden = !!account;
    renderPermissions(account);
    updateRole(); modal.show();
  }

  function validateTemporaryPassword(input, error) {
    const password = input.value;
    if (Array.from(password).length >= 8 && new TextEncoder().encode(password).length <= 72) return true;
    showError(error, '临时密码至少 8 个字符，最多 72 字节');
    input.focus();
    return false;
  }

  form.addEventListener('submit', async event => {
    event.preventDefault();
    if (saving || !form.reportValidity()) return;
    if (!selected && !validateTemporaryPassword(form.elements.password, $('accountFormError'))) return;
    showError($('accountFormError'), '');
    saving = true; $('saveAccount').disabled = true;
    const body = { username: form.elements.username.value, display_name: form.elements.display_name.value, role: form.elements.role.value, enabled: form.elements.enabled.value === 'true', permissions: {}, revision: selected ? selected.revision : 0 };
    modules.forEach(module => { body.permissions[module.id] = form.elements['permission_' + module.id].value; });
    if (!selected) body.password = form.elements.password.value;
    try {
      await api('/accounts' + (selected ? '/' + encodeURIComponent(selected.username) : ''), { method: selected ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      form.elements.password.value = '';
      saving = false; modal.hide(); AdminUI.alert('账号设置已保存', 'success');
      if (selected && selected.username === username && (body.role !== 'admin' || !body.enabled)) {
        location.href = body.enabled ? '/admin/access.html' : '/admin/login.html';
        return;
      }
      await Promise.all([load(), loadAudit()]);
    } catch (error) { showError($('accountFormError'), error.message); }
    finally { saving = false; $('saveAccount').disabled = false; }
  });
  $('accountModal').addEventListener('admin:dialog-closing', event => { if (saving) event.preventDefault(); });
  $('accountModal').addEventListener('admin:dialog-closed', () => { form.elements.password.value = ''; });

  function openReset(account) {
    if (resetting) return;
    resetTarget = account;
    resetForm.reset();
    showError($('accountResetError'), '');
    $('accountResetTitle').textContent = '应急重置 · ' + account.username;
    resetModal.show();
  }

  resetForm.addEventListener('submit', async event => {
    event.preventDefault();
    if (resetting || !resetTarget) return;
    const password = resetForm.elements.password;
    const error = $('accountResetError');
    if (!validateTemporaryPassword(password, error)) return;
    const account = resetTarget;
    resetting = true;
    $('confirmAccountReset').disabled = true;
    showError(error, '');
    try {
      await api('/accounts/' + encodeURIComponent(account.username) + '/reset-password', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password: password.value, revision: account.revision }) });
      password.value = '';
      resetting = false;
      resetModal.hide();
      if (account.username === username) { location.href = '/admin/login.html'; return; }
      AdminUI.alert('密码已重置，该账号下次登录需要修改密码', 'success');
      await Promise.all([load(), loadAudit()]);
    } catch (cause) { showError(error, cause.message); }
    finally { resetting = false; $('confirmAccountReset').disabled = false; }
  });
  $('accountResetModal').addEventListener('admin:dialog-shown', () => resetForm.elements.password.focus());
  $('accountResetModal').addEventListener('admin:dialog-closing', event => { if (resetting) event.preventDefault(); });
  $('accountResetModal').addEventListener('admin:dialog-closed', () => {
    resetForm.reset();
    resetTarget = null;
  });

  async function loadAudit() {
    const request = ++auditRequest;
    const actor = $('auditAccountFilter').value, operation = $('auditOperationFilter').value;
    const params = new URLSearchParams();
    if (actor) params.set('username', actor);
    if (operation) params.set('operation', operation);
    const filter = params.toString();
    $('refreshAudit').disabled = true;
    showError($('auditError'), '');
    if (renderedAuditFilter !== filter) {
      renderedAuditFilter = null;
      listState($('auditList'), 5, '正在加载…');
    }
    try {
      const data = await api('/audit-logs' + (filter ? '?' + filter : ''));
      if (request !== auditRequest) return;
      const items = AdminAuditLog.visibleEntries(data.items || []), rows = document.createDocumentFragment();
      items.forEach(item => {
        const description = AdminAuditLog.describe(item, data.target_names || {}), outcome = AdminAuditLog.outcome(item);
        const row = document.createElement('tr');
        row.className = 'admin-audit-entry';
        cell(row, time(item.created_at)); cell(row, item.actor_id === 'email-recovery' ? '邮箱找回（未登录）' : item.username);
        const details = document.createElement('details'); details.className = 'admin-audit-details';
        const summary = document.createElement('summary'); summary.textContent = description.action; details.appendChild(summary);
        [description.note, outcome.note, '请求：' + item.method + ' ' + item.path,
          'HTTP 状态：' + (item.status || '尚未记录最终响应'), '记录编号：' + item.id].filter(Boolean).forEach(text => {
          const line = document.createElement('p'); line.textContent = text; details.appendChild(line);
        });
        cell(row, '').appendChild(details);
        cell(row, description.target);
        const result = cell(row, ''), badge = document.createElement('span');
        badge.className = 'badge ' + outcome.color; badge.textContent = outcome.label; badge.title = outcome.note;
        result.appendChild(badge);
        rows.appendChild(row);
      });
      $('auditList').replaceChildren(rows);
      renderedAuditFilter = filter;
      if (!items.length) listState($('auditList'), 5, filter ? '暂无符合筛选条件的操作记录' : '暂无操作记录');
      showError($('auditError'), '');
    } catch (error) {
      if (request !== auditRequest) return;
      showError($('auditError'), error.message);
      if (renderedAuditFilter !== filter) listState($('auditList'), 5, '加载失败，请刷新重试');
    } finally { if (request === auditRequest) $('refreshAudit').disabled = false; }
  }
  form.elements.role.addEventListener('change', updateRole);
  form.elements.enabled.addEventListener('change', updateConfirmation);
  $('createAccount').disabled = true;
  $('createAccount').onclick = () => openEditor(null);
  $('refreshAccounts').onclick = load;
  $('refreshAudit').onclick = loadAudit;
  $('auditAccountFilter').addEventListener('change', loadAudit);
  $('auditOperationFilter').addEventListener('change', loadAudit);
  load(); loadAudit();
})();
