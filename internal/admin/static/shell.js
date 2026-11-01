(function () {
  'use strict';
  const sessionNode = document.getElementById('admin-session');
  let identity = sessionNode ? JSON.parse(sessionNode.textContent) : null;
  function can(module, write) {
    if (!identity || identity.must_change_password) return false;
    if (identity.role === 'admin') return true;
    if (module === 'accounts') return false;
    const level = (identity.permissions || {})[module];
    return level === 'write' || (!write && level === 'read');
  }
  const modulesByPage = {
    'dashboard.html': 'dashboard', 'track_submissions.html': 'track_submissions', 'tracks.html': 'tracks',
    'scenic_spots.html': 'scenic_spots', 'users.html': 'users', 'route_groups.html': 'route_groups',
    'companions.html': 'companions', 'companion_detail.html': 'companions', 'feedbacks.html': 'feedbacks',
    'index.html': 'releases', 'analytics.html': 'analytics', 'accounts.html': 'accounts'
  };
  function linkModule(link) {
    const url = new URL(link.getAttribute('href'), location.origin);
    if (url.origin !== location.origin || !url.pathname.startsWith('/admin/')) return '';
    return modulesByPage[url.pathname.split('/').pop()] || '';
  }
  function permitted(value, write) { return value.split(/\s+/).every(module => can(module, write)); }
  function applyPermissions() {
    observer.disconnect();
    document.querySelectorAll('a[href]').forEach(link => {
      const module = linkModule(link);
      if (module && !link.classList.contains('brand')) link.classList.toggle('admin-permission-hidden', !can(module, false));
    });
    document.querySelectorAll('[data-admin-read], [data-admin-write]').forEach(element => {
      const write = element.hasAttribute('data-admin-write');
      element.classList.toggle('admin-permission-hidden', !permitted(element.getAttribute(write ? 'data-admin-write' : 'data-admin-read'), write));
    });
    document.querySelectorAll('[data-related-spot-id]').forEach(element => element.classList.toggle('admin-permission-hidden', !can('tracks', false)));
    document.querySelectorAll('[data-admin-input]').forEach(input => {
      const locked = !permitted(input.dataset.adminInput, true);
      if (locked) { input.dataset.adminLocked = 'true'; input.disabled = true; }
      else if (input.dataset.adminLocked) { delete input.dataset.adminLocked; input.disabled = false; }
    });
    document.querySelectorAll('[data-admin-fields]').forEach(container => {
      const locked = !permitted(container.dataset.adminFields, true);
      container.querySelectorAll('input,select,textarea').forEach(input => {
        if (locked) { input.dataset.adminLocked = 'true'; input.disabled = true; }
        else if (input.dataset.adminLocked) { delete input.dataset.adminLocked; input.disabled = false; }
      });
    });
    observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['disabled'] });
  }
  const observer = new MutationObserver(applyPermissions);
  window.AdminAccess = { can: can, apply: applyPermissions, identity: () => identity };
  const topbar = document.querySelector('.topbar');
  if (!topbar) return;

  const path = window.location.pathname;
  const items = [
    { href: '/admin/dashboard.html', label: '工作台', icon: 'layout-dashboard', primary: true },
    { href: '/admin/track_submissions.html', label: '投稿审核', icon: 'clipboard-check', primary: true },
    { href: '/admin/tracks.html', label: '轨迹管理', icon: 'route', primary: true },
    { href: '/admin/scenic_spots.html', label: '景点管理', icon: 'map-2' },
    { href: '/admin/users.html', label: '用户管理', icon: 'users' },
    { href: '/admin/route_groups.html', label: '地图模式', icon: 'map-2' },
    { href: '/admin/companions.html', label: '与友同行', icon: 'friends' },
    { href: '/admin/feedbacks.html', label: '反馈与纠错', icon: 'message-report' },
    { href: '/admin/index.html', label: 'App 发布', icon: 'package' },
    { href: '/admin/analytics.html', label: '埋点同步', icon: 'chart-bar' },
    { href: '/admin/accounts.html', label: '账号管理', icon: 'users' },
    { href: '/admin/more.html', label: '更多', icon: 'dots', mobileOnly: true, primary: true },
  ];

  function isActive(item) {
    if (path === item.href) return true;
    return path === '/admin/companion_detail.html' && item.href === '/admin/companions.html';
  }

  topbar.innerHTML = [
    '<a class="brand" href="/admin/dashboard.html"><span class="brand-mark">T</span><span><b>Track</b><small>管理中心</small></span></a>',
    '<nav class="nav" aria-label="管理后台导航">',
    items.map(function (item) {
      const classes = [item.primary ? 'nav-primary' : 'nav-secondary', item.mobileOnly ? 'mobile-only' : '', isActive(item) ? 'active' : ''].filter(Boolean).join(' ');
      return '<a class="' + classes + '" href="' + item.href + '"><img class="nav-icon" src="/admin/static/vendor/tabler-icons/' + item.icon + '.svg" alt="" /><span>' + item.label + '</span></a>';
    }).join(''),
    '</nav>',
    '<div class="spacer"></div>',
    '<div class="user"><div class="admin-identity"><span id="userName"></span><span id="adminRole" class="badge"></span></div><a href="/admin/password.html" class="admin-password-link">修改密码</a><button id="logoutBtn" type="button">退出</button></div>'
  ].join('');
  function renderIdentity() {
    if (!identity) return;
    document.getElementById('userName').textContent = identity.display_name || identity.username;
    const role = document.getElementById('adminRole');
    role.textContent = identity.role === 'admin' ? '管理员' : '操作员';
    role.className = 'badge ' + (identity.role === 'admin' ? 'bg-purple-lt' : 'bg-blue-lt');
    topbar.querySelector('.brand').href = identity.home || '/admin/access.html';
    const currentModule = modulesByPage[location.pathname.split('/').pop()];
    document.querySelectorAll('.admin-readonly-notice').forEach(node => node.remove());
    if (currentModule && can(currentModule, false) && !can(currentModule, true)) {
      const note = document.createElement('p');
      note.className = 'alert alert-info admin-readonly-notice';
      note.textContent = '当前页面为只读权限，可以查看资料。';
      const main = document.querySelector('main');
      if (main) main.prepend(note);
    }
    applyPermissions();
  }
  document.getElementById('logoutBtn').addEventListener('click', async function () {
    try { await fetch('/admin/api/logout', { method: 'POST' }); }
    finally { location.href = '/admin/login.html'; }
  });
  let refreshing = false;
  async function refreshIdentity() {
    if (refreshing) return;
    refreshing = true;
    try {
      const response = await fetch('/admin/api/me', { cache: 'no-store' });
      if (response.status === 401) { location.href = '/admin/login.html'; return; }
      if (!response.ok) return;
      identity = (await response.json()).data;
      if (identity.must_change_password && location.pathname !== '/admin/password.html') { location.href = '/admin/password.html'; return; }
      const module = modulesByPage[location.pathname.split('/').pop()];
      if (module && !can(module, false)) { location.href = '/admin/access.html'; return; }
      renderIdentity();
    } catch (_) {} finally { refreshing = false; }
  }
  window.addEventListener('focus', refreshIdentity);
  renderIdentity();
})();
