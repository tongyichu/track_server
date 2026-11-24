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
    'scenic_spots.html': 'scenic_spots', 'regions.html': 'regions', 'users.html': 'users', 'route_groups.html': 'route_groups',
    'music.html': 'music', 'companions.html': 'companions', 'companion_detail.html': 'companions', 'feedbacks.html': 'feedbacks',
    'announcements.html': 'announcements', 'realtime.html': 'realtime', 'ai_usage.html': 'ai_usage', 'index.html': 'releases', 'analytics.html': 'analytics', 'accounts.html': 'accounts'
  };
  function linkModule(link) {
    const url = new URL(link.getAttribute('href'), location.origin);
    if (url.origin !== location.origin || !url.pathname.startsWith('/admin/')) return '';
    return modulesByPage[url.pathname.split('/').pop()] || '';
  }
  function permitted(value, write) { return value.split(/\s+/).every(module => can(module, write)); }
  function select(root, selector) {
    const elements = Array.from(root.querySelectorAll(selector));
    if (root.matches && root.matches(selector)) elements.unshift(root);
    return elements;
  }
  function applyPermissionsTo(root) {
    select(root, 'a[href]').forEach(link => {
      const module = linkModule(link);
      if (module && !link.classList.contains('brand')) {
        const primaryEntry = link.dataset.adminFallbackFor;
        link.classList.toggle('admin-permission-hidden', !can(module, false) || (!!primaryEntry && can(primaryEntry, false)));
      }
    });
    select(root, '[data-admin-read], [data-admin-write]').forEach(element => {
      const write = element.hasAttribute('data-admin-write');
      element.classList.toggle('admin-permission-hidden', !permitted(element.getAttribute(write ? 'data-admin-write' : 'data-admin-read'), write));
    });
    select(root, '[data-related-spot-id]').forEach(element => element.classList.toggle('admin-permission-hidden', !can('tracks', false)));
    select(root, 'input,select,textarea,button[data-admin-input]').forEach(input => {
      const container = input.closest('[data-admin-fields]');
      if (!input.dataset.adminInput && !container && !input.dataset.adminLocked) return;
      const locked = (input.dataset.adminInput && !permitted(input.dataset.adminInput, true)) ||
        (container && !permitted(container.dataset.adminFields, true));
      if (locked) { input.dataset.adminLocked = 'true'; input.disabled = true; }
      else if (input.dataset.adminLocked) { delete input.dataset.adminLocked; input.disabled = false; }
    });
  }
  function observePermissions() {
    observer.observe(document.body, { childList: true, subtree: true, attributes: true,
      attributeFilter: ['disabled', 'href', 'data-admin-read', 'data-admin-write', 'data-admin-fields', 'data-admin-input'] });
  }
  function applyPermissions() {
    observer.disconnect();
    applyPermissionsTo(document);
    observePermissions();
  }
  const observer = new MutationObserver(mutations => {
    const roots = new Set();
    mutations.forEach(mutation => {
      if (mutation.type === 'attributes') roots.add(mutation.target);
      mutation.addedNodes.forEach(node => { if (node.nodeType === Node.ELEMENT_NODE) roots.add(node); });
    });
    observer.disconnect();
    roots.forEach(root => {
      if (!root.isConnected) return;
      for (let parent = root.parentElement; parent; parent = parent.parentElement) {
        if (roots.has(parent)) return;
      }
      applyPermissionsTo(root);
    });
    observePermissions();
  });
  window.AdminAccess = { can: can, apply: applyPermissions, identity: () => identity };
  const topbar = document.querySelector('.topbar');
  if (!topbar) return;

  const path = window.location.pathname;
  const dashboardPath = '/admin/dashboard.html';
  const morePath = '/admin/more.html';
  const navigation = [
    { items: [{ href: dashboardPath, label: '工作台', icon: 'layout-dashboard', primary: true }] },
    { label: '运营', items: [
      { href: '/admin/track_submissions.html', label: '内容审核', icon: 'clipboard-check', primary: true },
      { href: '/admin/feedbacks.html', label: '反馈与纠错', icon: 'message-report' },
      { href: '/admin/announcements.html', label: '活动&通知', icon: 'bell' },
      { href: '/admin/users.html', label: '用户管理', icon: 'users' }
    ] },
    { label: '轨迹与地图', items: [
      { href: '/admin/tracks.html', label: '轨迹管理', icon: 'route', primary: true },
      { href: '/admin/route_groups.html', label: '地图模式', icon: 'map-2' },
      { href: '/admin/scenic_spots.html', label: '景点管理', icon: 'mountain' },
      { href: '/admin/regions.html', label: '城市管理', icon: 'building-community' }
    ] },
    { label: '应用功能', items: [
      { href: '/admin/companions.html', label: '与友同行', icon: 'friends' },
      { href: '/admin/music.html', label: '背景音乐', icon: 'music' }
    ] },
    { label: '维护', items: [
      { href: '/admin/realtime.html', label: '实时通信', icon: 'cloud-upload' },
      { href: '/admin/index.html', label: '发布管理', icon: 'package' },
      { href: '/admin/ai_usage.html', label: 'AI 使用', icon: 'robot' },
      { href: '/admin/analytics.html', label: '埋点同步日志', icon: 'chart-bar', desktopOnly: true, fallbackFor: 'dashboard' },
      { href: '/admin/accounts.html', label: '账号管理', icon: 'shield-lock' }
    ] },
    { items: [{ href: morePath, label: '更多', icon: 'dots', mobileOnly: true, primary: true }] }
  ];

  const sidebarParts = document.getElementById('admin-sidebar-parts')?.content;
  function sidebarPart(part, tag) {
    const prototype = sidebarParts?.querySelector('[data-sidebar="' + part + '"]');
    if (prototype) return prototype.cloneNode(false);
    const element = document.createElement(tag);
    element.dataset.slot = 'sidebar-' + part;
    element.dataset.sidebar = part;
    return element;
  }

  const nav = sidebarPart('content', 'nav');
  nav.id = 'adminSidebarNavigation';
  nav.classList.add('nav');
  nav.setAttribute('aria-label', '管理后台导航');
  for (const section of navigation) {
    const group = sidebarPart('group', 'div');
    group.classList.add('admin-nav-section');
    if (section.label) {
      group.setAttribute('role', 'group');
      group.setAttribute('aria-label', section.label);
      const label = sidebarPart('group-label', 'div');
      label.classList.add('admin-nav-label');
      label.setAttribute('aria-hidden', 'true');
      label.textContent = section.label;
      group.append(label);
    }
    const menu = sidebarPart('menu', 'ul');
    menu.setAttribute('role', 'list');
    for (const item of section.items) {
      const row = sidebarPart('menu-item', 'li');
      const link = sidebarPart('menu-button', 'a');
      link.href = item.href;
      link.classList.toggle('nav-primary', !!item.primary);
      link.classList.toggle('mobile-only', !!item.mobileOnly);
      link.classList.toggle('desktop-only', !!item.desktopOnly);
      if (item.fallbackFor) link.dataset.adminFallbackFor = item.fallbackFor;
      const icon = document.createElement('img');
      icon.className = 'nav-icon';
      icon.src = AdminAssets.url('/admin/static/vendor/tabler-icons/' + item.icon + '.svg');
      icon.alt = '';
      const label = document.createElement('span');
      label.textContent = item.label;
      link.append(icon, label);
      row.append(link);
      menu.append(row);
    }
    group.append(menu);
    nav.append(group);
  }

  function accountIcon(content) {
    return '<svg class="admin-account-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + content + '</svg>';
  }
  const settingsIcon = accountIcon('<path d="M9.5 2h5l.5 2.2 2 1.2 2.2-.7 2.5 4.3-1.7 1.6v2.8l1.7 1.6-2.5 4.3-2.2-.7-2 1.2-.5 2.2h-5l-.5-2.2-2-1.2-2.2.7-2.5-4.3 1.7-1.6v-2.8L2.3 9l2.5-4.3 2.2.7 2-1.2Z"/><circle cx="12" cy="12" r="3.2"/>');
  const passwordIcon = accountIcon('<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V6a4 4 0 0 1 8 0v4M12 15v2"/>');
  const logoutIcon = accountIcon('<path d="M9 5H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4M12 12h9m-4-4 4 4-4 4"/>');

  topbar.innerHTML = [
    '<a class="brand" href="/admin/dashboard.html" aria-label="时光轨迹控制台"><img class="brand-mark" src="' + AdminAssets.url('/admin/static/favicon.png') + '" width="32" height="32" alt="" data-no-preview /><span><b>时光轨迹</b><small>控制台</small></span></a>',
    '<button type="button" class="btn btn-icon btn-ghost-secondary admin-sidebar-trigger" data-slot="sidebar-trigger" aria-controls="adminSidebarNavigation" aria-expanded="true" aria-label="收起侧栏" title="收起侧栏">' + accountIcon('<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>') + '</button>',
    '<div class="spacer"></div>',
    '<div class="user dropdown dropup admin-account-footer">',
    '<div class="admin-identity"><span id="userName"></span><span id="adminRole" class="badge"></span></div>',
    '<button id="accountSettingsBtn" type="button" class="btn btn-icon btn-ghost-secondary admin-settings-toggle" data-admin-menu data-admin-menu-anchor="parent" data-admin-menu-close="outside" aria-expanded="false" aria-controls="accountMenu" aria-label="账号设置" title="账号设置">' + settingsIcon + '</button>',
    '<div id="accountMenu" class="dropdown-menu admin-account-menu" aria-labelledby="accountSettingsBtn">',
    '<div class="admin-account-menu-heading">当前账号</div>',
    '<dl class="admin-account-details"><div><dt>登录账号</dt><dd id="accountMenuUsername"></dd></div><div><dt>名称</dt><dd id="accountMenuName"></dd></div><div><dt>身份</dt><dd id="accountMenuRole"></dd></div><div><dt>最近登录</dt><dd id="accountMenuLastLogin"></dd></div></dl>',
    '<div class="dropdown-divider"></div>',
    '<a href="/admin/password.html" class="dropdown-item">' + passwordIcon + '<span>账号安全</span></a>',
    '<button id="logoutBtn" type="button" class="dropdown-item text-danger">' + logoutIcon + '<span>退出登录</span></button>',
    '<p id="accountMenuError" class="admin-account-menu-error" role="alert" hidden></p>',
    '</div></div>'
  ].join('');
  topbar.dataset.slot = 'sidebar';
  topbar.dataset.variant = 'sidebar';
  const sidebarTrigger = topbar.querySelector('.admin-sidebar-trigger');
  const accountFooter = topbar.querySelector('.admin-account-footer');
  const sidebarHeader = sidebarPart('header', 'div');
  const sidebarFooter = sidebarPart('footer', 'div');
  sidebarHeader.append(topbar.querySelector('.brand'), sidebarTrigger);
  sidebarFooter.append(accountFooter);
  topbar.prepend(sidebarHeader, nav);
  topbar.append(sidebarFooter);
  const accountToggle = document.getElementById('accountSettingsBtn');
  const accountMenu = document.getElementById('accountMenu');
  const mobileLayout = window.matchMedia('(max-width: 900px)');
  const navLinks = Array.from(nav.querySelectorAll('a'));
  const sidebarPreferenceKey = 'track-admin-sidebar';

  function savedSidebarState() {
    try { return localStorage.getItem(sidebarPreferenceKey) === 'collapsed'; }
    catch (_) { return null; }
  }

  function setSidebarState(collapsed) {
    const state = collapsed ? 'collapsed' : 'expanded';
    if (topbar.dataset.state === state) return;
    topbar.dataset.state = state;
    topbar.dataset.collapsible = collapsed ? 'icon' : '';
    const label = collapsed ? '展开侧栏' : '收起侧栏';
    sidebarTrigger.setAttribute('aria-expanded', String(!collapsed));
    sidebarTrigger.setAttribute('aria-label', label);
    sidebarTrigger.title = label;
    navLinks.forEach(link => {
      if (collapsed) link.title = link.textContent;
      else link.removeAttribute('title');
    });
  }

  setSidebarState(savedSidebarState() ?? false);
  sidebarTrigger.addEventListener('click', function () {
    const collapsed = topbar.dataset.state !== 'collapsed';
    positionAccountMenu();
    setSidebarState(collapsed);
    try { localStorage.setItem(sidebarPreferenceKey, topbar.dataset.state); }
    catch (_) { /* Sidebar state can still change for the current page. */ }
    // Existing charts and maps respond to the same event as viewport resizing.
    requestAnimationFrame(() => window.dispatchEvent(new Event('resize')));
  });
  window.addEventListener('pageshow', function (event) {
    const collapsed = event.persisted ? savedSidebarState() : null;
    if (collapsed === null || collapsed === (topbar.dataset.state === 'collapsed')) return;
    setSidebarState(collapsed);
    requestAnimationFrame(() => window.dispatchEvent(new Event('resize')));
  });

  function revealActiveNavigation() {
    if (mobileLayout.matches) return;
    const active = nav.querySelector('a.active');
    if (!active) return;
    const viewport = nav.getBoundingClientRect(), row = active.getBoundingClientRect();
    if (row.top < viewport.top) nav.scrollTop -= viewport.top - row.top;
    else if (row.bottom > viewport.bottom) nav.scrollTop += row.bottom - viewport.bottom;
  }

  function syncNavigation() {
    let visibilityChanged = false;
    const visibleLink = mobileLayout.matches ? 'a.nav-primary:not(.desktop-only)' : 'a:not(.mobile-only)';
    nav.querySelectorAll('.admin-nav-section').forEach(section => {
      const hidden = !section.querySelector(visibleLink + ':not(.admin-permission-hidden)');
      if (section.hidden !== hidden) {
        section.hidden = hidden;
        visibilityChanged = true;
      }
    });
    let target = path;
    if (path === '/admin/companion_detail.html') target = '/admin/companions.html';
    if (path === '/admin/analytics.html' && can('dashboard', false)) target = dashboardPath;
    let active = navLinks.find(link => link.pathname === target && !link.classList.contains('admin-permission-hidden'));
    if (active && mobileLayout.matches && !active.classList.contains('nav-primary')) {
      active = navLinks.find(link => link.pathname === morePath);
    } else if (active && !mobileLayout.matches && active.classList.contains('mobile-only')) {
      active = null;
    }
    const activeChanged = nav.querySelector('a.active') !== (active || null);
    navLinks.forEach(link => {
      const current = link === active;
      link.classList.toggle('active', current);
      if (link.dataset.active !== String(current)) link.dataset.active = String(current);
      if (current) link.setAttribute('aria-current', link.pathname === path ? 'page' : 'location');
      else link.removeAttribute('aria-current');
    });
    if (activeChanged || visibilityChanged) requestAnimationFrame(revealActiveNavigation);
  }

  if (path === '/admin/password.html') {
    accountToggle.classList.add('is-current');
    accountMenu.querySelector('[href="/admin/password.html"]').setAttribute('aria-current', 'page');
  }
  function positionAccountMenu() {
    const dropdown = AdminComponents.Dropdown.getInstance(accountToggle);
    if (dropdown) dropdown.hide();
    accountFooter.classList.toggle('dropup', !mobileLayout.matches);
    accountMenu.classList.toggle('dropdown-menu-end', mobileLayout.matches);
  }
  positionAccountMenu();
  mobileLayout.addEventListener('change', function () {
    positionAccountMenu();
    syncNavigation();
  });
  window.addEventListener('resize', revealActiveNavigation);
  accountFooter.addEventListener('admin:menu-opening', function () {
    const error = document.getElementById('accountMenuError');
    error.textContent = ''; error.hidden = true;
    refreshIdentity();
  });
  function renderIdentity() {
    if (!identity) return;
    document.getElementById('userName').textContent = identity.display_name || identity.username;
    document.getElementById('userName').title = identity.display_name || identity.username;
    document.getElementById('accountMenuUsername').textContent = identity.username;
    document.getElementById('accountMenuName').textContent = identity.display_name || identity.username;
    document.getElementById('accountMenuRole').textContent = identity.role === 'admin' ? '管理员' : '操作员';
    const lastLogin = identity.account && identity.account.last_login_at;
    document.getElementById('accountMenuLastLogin').textContent = AdminTime.dateTime(lastLogin);
    const role = document.getElementById('adminRole');
    role.textContent = identity.role === 'admin' ? '管理员' : '操作员';
    role.className = 'badge ' + (identity.role === 'admin' ? 'bg-purple-lt' : 'bg-blue-lt');
    topbar.querySelector('.brand').href = identity.home || '/admin/access.html';
    const currentModule = modulesByPage[location.pathname.split('/').pop()];
    document.querySelectorAll('.admin-readonly-notice').forEach(node => node.remove());
    if (currentModule && can(currentModule, false) && !can(currentModule, true) && !(currentModule === 'ai_usage' && (can('ai_tasks', true) || can('ai_settings', true)))) {
      const note = document.createElement('p');
      note.className = 'alert alert-info admin-readonly-notice';
      note.textContent = '当前页面为只读权限，可以查看资料。';
      const main = document.querySelector('main');
      if (main) main.prepend(note);
    }
    applyPermissions();
    syncNavigation();
  }
  document.getElementById('logoutBtn').addEventListener('click', async function () {
    const button = document.getElementById('logoutBtn'), error = document.getElementById('accountMenuError');
    if (button.disabled) return;
    button.disabled = true; error.hidden = true;
    try {
      const response = await fetch('/admin/api/logout', { method: 'POST' });
      if (!response.ok && response.status !== 401) throw new Error('logout failed');
      location.href = '/admin/login.html';
    } catch (_) {
      error.textContent = '退出失败，请稍后重试'; error.hidden = false;
    } finally { button.disabled = false; }
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
