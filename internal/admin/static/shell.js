(function () {
  const topbar = document.querySelector('.topbar');
  if (!topbar) return;

  const path = window.location.pathname;
  const items = [
    { href: '/admin/dashboard.html', label: '工作台', icon: 'layout-dashboard', primary: true },
    { href: '/admin/track_submissions.html', label: '投稿审核', icon: 'clipboard-check', primary: true },
    { href: '/admin/tracks.html', label: '轨迹管理', icon: 'route', primary: true },
    { href: '/admin/scenic_spots.html', label: '景点管理', icon: 'map-2' },
    { href: '/admin/users.html', label: '用户管理', icon: 'users' },
    { href: '/admin/route_groups.html', label: '聚合路线', icon: 'map-2' },
    { href: '/admin/companions.html', label: '同行管理', icon: 'friends' },
    { href: '/admin/feedbacks.html', label: '反馈与纠错', icon: 'message-report' },
    { href: '/admin/index.html', label: 'App 发布', icon: 'package' },
    { href: '/admin/analytics.html', label: '埋点同步', icon: 'chart-bar' },
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
    '<div class="user"><span id="userName"></span><button id="logoutBtn" type="button">退出</button></div>'
  ].join('');
})();
