(function () {
  'use strict';
  const form = document.getElementById('passwordForm');
  const error = document.getElementById('passwordError');
  if (AdminAccess.identity().must_change_password) document.getElementById('passwordHint').textContent = '请先修改临时密码，再使用后台功能。';
  form.addEventListener('submit', async event => {
    event.preventDefault(); error.textContent = '';
    if (!form.reportValidity()) return;
    if (form.elements.password.value !== form.elements.confirmation.value) { error.textContent = '两次输入的新密码不一致'; return; }
    const button = document.getElementById('changePassword'); button.disabled = true;
    try {
      const response = await fetch('/admin/api/password', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ old_password: form.elements.old_password.value, password: form.elements.password.value }) });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || '密码修改失败');
      form.reset(); location.href = '/admin/login.html';
    } catch (cause) { error.textContent = cause.message; }
    finally { button.disabled = false; }
  });
})();
