(function () {
  const host = document.getElementById('accessLinks');
  document.querySelectorAll('.topbar .nav a').forEach(item => {
    if (item.classList.contains('admin-permission-hidden') || item.classList.contains('mobile-only')) return;
    const link = document.createElement('a'); link.href = item.href; link.textContent = item.textContent; host.appendChild(link);
  });
  if (!host.children.length) document.getElementById('accessHint').textContent = '尚未分配页面权限，请联系管理员。';
})();
