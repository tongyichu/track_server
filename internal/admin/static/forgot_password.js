(function () {
  'use strict';
  const $ = id => document.getElementById(id), request = $('recoveryRequestForm'), complete = $('recoveryCompleteForm');
  let available = false, busy = false, finished = false, deadline = 0, identity = null;
  function message(id, text) { $(id).textContent = text; $(id).hidden = !text; }
  function render() {
    const remaining = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
    $('requestRecoveryCode').disabled = !available || busy || finished || remaining > 0;
    $('requestRecoveryCode').textContent = remaining ? remaining + ' 秒后可重发' : '发送找回验证码';
    $('completeRecovery').disabled = !available || busy || finished;
    $('changeRecoveryIdentity').disabled = busy;
    request.elements.username.readOnly = busy || !!identity;
    request.elements.email.readOnly = busy || !!identity;
    complete.querySelectorAll('input').forEach(input => { input.readOnly = busy || finished; });
  }
  async function api(path, body) {
    const response = await fetch('/admin/api/password-recovery/' + path, body ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body), cache: 'no-store' } : { cache: 'no-store' });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(result.error || '操作失败，请重试');
    return result.data || {};
  }
  request.addEventListener('submit', async event => {
    event.preventDefault();
    if (!available || busy || finished || Date.now() < deadline || !request.reportValidity()) return;
    message('recoveryError', ''); busy = true; render();
    const submitted = { username: request.elements.username.value.trim(), email: request.elements.email.value.trim().toLowerCase() };
    try {
      const result = await api('request', submitted);
      identity = submitted; deadline = Date.now() + Number(result.resend_after || 60) * 1000;
      request.elements.username.readOnly = true; request.elements.email.readOnly = true;
      $('changeRecoveryIdentity').hidden = false;
      complete.reset(); complete.hidden = false;
      $('recoveryTarget').textContent = '登录名：' + identity.username + ' · 邮箱：' + identity.email + '。请输入最新收到的 8 位验证码，10 分钟内有效。';
      message('recoveryMessage', result.message);
    } catch (error) { message('recoveryError', error.message); }
    finally { busy = false; render(); }
  });
  $('changeRecoveryIdentity').addEventListener('click', () => {
    identity = null; complete.reset(); complete.hidden = true;
    request.elements.username.readOnly = false; request.elements.email.readOnly = false;
    $('changeRecoveryIdentity').hidden = true; message('recoveryMessage', ''); request.elements.username.focus();
  });
  complete.addEventListener('submit', async event => {
    event.preventDefault();
    if (!available || busy || finished || !identity || !complete.reportValidity()) return;
    message('recoveryError', '');
    if (complete.elements.password.value !== complete.elements.confirmation.value) { message('recoveryError', '两次输入的新密码不一致'); return; }
    busy = true; render();
    try {
      await api('complete', { ...identity, code: complete.elements.code.value.trim(), password: complete.elements.password.value, confirmation: complete.elements.confirmation.value });
      finished = true; identity = null; request.reset(); complete.reset(); request.hidden = true; complete.hidden = true;
      message('recoveryMessage', '新密码已设置，全部旧登录已退出。请点击下方“返回登录”，使用新密码登录。');
    } catch (error) { message('recoveryError', error.message); }
    finally { busy = false; render(); }
  });
  api('status').then(data => {
    available = !!data.enabled;
    $('recoveryAvailability').textContent = available ? '验证码只发送到事先绑定并验证的邮箱。未绑定的账号暂不能通过邮件找回。' : '邮件找回暂不可用，请联系后台负责人完成发信配置。';
    render();
  }).catch(() => { $('recoveryAvailability').textContent = '邮件服务状态加载失败，请刷新页面重试。'; render(); });
  const timer = setInterval(render, 1000);
  window.addEventListener('pagehide', () => { clearInterval(timer); request.reset(); complete.reset(); identity = null; });
  window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });
})();
