(function () {
  const form = document.getElementById('loginForm');
  const err = document.getElementById('err');
  form.addEventListener('submit', async function (e) {
    e.preventDefault();
    err.textContent = '';
    const fd = new FormData(form);
    try {
      const resp = await fetch('/admin/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: fd.get('username'),
          password: fd.get('password'),
        }),
      });
      if (!resp.ok) {
        const data = await resp.json().catch(function () { return {}; });
        err.textContent = data.error || ('登录失败: ' + resp.status);
        return;
      }
      window.location.href = '/admin/index.html';
    } catch (e) {
      err.textContent = '网络错误: ' + e.message;
    }
  });
})();
