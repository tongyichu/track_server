(function () {
  'use strict';
  const $ = id => document.getElementById(id), binding = $('emailBindingForm'), verification = $('emailVerificationForm');
  if (!AdminAccess.identity() || AdminAccess.identity().must_change_password) { $('emailSecurityPanel').hidden = true; return; }
  const buttons = [$('sendCurrentEmailCode'), $('sendNewEmailCode'), $('verifyRecoveryEmail')];
  const titles = buttons.map(button => button.textContent), deadlines = new Map();
  let busy = false, emailBound = false, finished = false;
  function message(id, text) { $(id).textContent = text; $(id).hidden = !text; }
  function renderButtons() {
    document.querySelectorAll('#emailBindingForm input, #emailVerificationForm input').forEach(input => { input.readOnly = busy || finished; });
    buttons.forEach((button, index) => {
      const remaining = Math.max(0, Math.ceil(((deadlines.get(button.id) || 0) - Date.now()) / 1000));
      button.disabled = busy || finished || remaining > 0;
      button.textContent = remaining ? remaining + ' 秒后可重发' : titles[index];
    });
  }
  async function read(path, body) {
    const response = await fetch('/admin/api' + path, body ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body), cache: 'no-store' } : { cache: 'no-store' });
    const result = await response.json().catch(() => ({}));
    if (response.status === 401) location.href = '/admin/login.html';
    if (!response.ok) throw new Error(result.error || '操作失败，请重试');
    return result.data || {};
  }
  function pending(email) {
    $('pendingEmailHint').textContent = '正在验证：' + email + '。请输入发到这个邮箱的 8 位验证码，10 分钟内有效；只有验证通过才会保存该邮箱。';
    verification.hidden = false;
  }
  async function load() {
    try {
      const data = await read('/security');
      emailBound = data.email_verified;
      $('emailSecurityState').textContent = emailBound ? '已验证的找回邮箱：' + data.email : '尚未绑定找回邮箱，忘记密码时暂不能通过邮件恢复。';
      if (!data.email_recovery_enabled) {
        message('emailSecurityError', '邮件服务暂未配置，绑定和邮箱找回尚不可用。请联系后台负责人完成发信配置；本人修改密码仍可正常使用。');
        return;
      }
      $('emailSecurityForms').hidden = false;
      $('currentEmailCodeGroup').hidden = !emailBound;
      binding.elements.current_code.required = !!emailBound;
      if (data.pending_email) pending(data.pending_email);
    } catch (error) { message('emailSecurityError', '邮箱信息加载失败，请刷新重试：' + error.message); }
  }
  async function send(proveCurrent) {
    if (busy || finished) return;
    message('emailSecurityError', ''); message('emailSecuritySuccess', '');
    if (!binding.elements.password.reportValidity()) return;
    if (!proveCurrent && !binding.reportValidity()) return;
    busy = true; renderButtons();
    try {
      const email = binding.elements.email.value.trim().toLowerCase();
      const result = await read('/security/email/' + (proveCurrent ? 'current-code' : 'send-code'), { password: binding.elements.password.value, email: email, current_code: binding.elements.current_code.value.trim() });
      const buttonID = proveCurrent ? 'sendCurrentEmailCode' : 'sendNewEmailCode';
      deadlines.set(buttonID, Date.now() + Number(result.resend_after || 60) * 1000);
      if (!proveCurrent) {
        verification.reset(); pending(email);
        binding.elements.password.value = '';
        binding.elements.current_code.value = '';
      }
      message('emailSecuritySuccess', proveCurrent ? '原邮箱验证码已提交发送，请查收后填写，再验证新的邮箱。' : result.message);
    } catch (error) { message('emailSecurityError', error.message); }
    finally { busy = false; renderButtons(); }
  }
  $('sendCurrentEmailCode').addEventListener('click', () => send(true));
  binding.addEventListener('submit', event => { event.preventDefault(); send(false); });
  verification.addEventListener('submit', async event => {
    event.preventDefault();
    if (busy || finished || !verification.reportValidity()) return;
    message('emailSecurityError', ''); busy = true; renderButtons();
    try {
      await read('/security/email/verify', { code: verification.elements.code.value.trim() });
      finished = true; binding.reset(); verification.reset(); $('emailSecurityForms').hidden = true;
      message('emailSecuritySuccess', '找回邮箱已验证并保存，全部旧登录已退出，正在返回登录页…');
      setTimeout(() => { location.href = '/admin/login.html'; }, 1200);
    } catch (error) { message('emailSecurityError', error.message); }
    finally { busy = false; renderButtons(); }
  });
  const timer = setInterval(renderButtons, 1000);
  window.addEventListener('pagehide', () => { clearInterval(timer); binding.reset(); verification.reset(); $('passwordForm').reset(); });
  window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });
  load();
})();
