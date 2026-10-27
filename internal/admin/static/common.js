(function () {
  fetch('/admin/api/me').then(function (response) {
    if (!response.ok) {
      window.location.href = '/admin/login.html';
      return null;
    }
    return response.json();
  }).then(function (payload) {
    const userName = document.getElementById('userName');
    if (payload && payload.data && userName) userName.textContent = payload.data.username;
  });
  const logout = document.getElementById('logoutBtn');
  if (logout) logout.addEventListener('click', async function () {
    await fetch('/admin/api/logout', { method: 'POST' });
    window.location.href = '/admin/login.html';
  });
})();
