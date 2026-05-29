(function () {
  // 通用：登录态校验 + 退出
  fetch('/admin/api/me').then(function (r) {
    if (!r.ok) { window.location.href = '/admin/login.html'; return null; }
    return r.json();
  }).then(function (j) {
    if (j && j.data) {
      var el = document.getElementById('userName');
      if (el) el.textContent = '👤 ' + j.data.username;
    }
  });
  document.getElementById('logoutBtn').addEventListener('click', async function () {
    await fetch('/admin/api/logout', { method: 'POST' });
    window.location.href = '/admin/login.html';
  });

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function fmtTime(s) {
    if (!s) return '';
    if (String(s).startsWith('0001-')) return '';
    return esc(String(s).replace('T', ' ').slice(0, 19));
  }
  function fmtBool(v) { return v ? '是' : '否'; }

  function getQuery(name) {
    var m = new URLSearchParams(window.location.search);
    return m.get(name) || '';
  }

  var sessionID = getQuery('session_id');
  var sessionInfo = document.getElementById('sessionInfo');
  var memberList = document.getElementById('memberList');
  var memberHint = document.getElementById('memberHint');
  var positionList = document.getElementById('positionList');
  var positionHint = document.getElementById('positionHint');
  var danmakuList = document.getElementById('danmakuList');
  var danmakuHint = document.getElementById('danmakuHint');

  if (!sessionID) {
    sessionInfo.innerHTML = '<span class="error">缺少 session_id 参数</span>';
    return;
  }

  function renderSession(s) {
    if (!s) {
      sessionInfo.innerHTML = '<span class="error">会话不存在</span>';
      return;
    }
    var rows = [
      ['SessionID', esc(s.session_id)],
      ['发起人 UserID', s.owner_user_id || 0],
      ['标题', esc(s.title || '')],
      ['类型', esc(s.track_type || '')],
      ['位置', esc(s.locate_addr || '')],
      ['状态', esc(s.status || '')],
      ['可见性', esc(s.visibility || '')],
      ['最大成员', s.max_members || 0],
      ['弹幕开关', fmtBool(s.danmaku_enabled)],
      ['开始时间', fmtTime(s.started_at)],
      ['结束时间', fmtTime(s.ended_at)],
      ['创建时间', fmtTime(s.created_at)],
      ['更新时间', fmtTime(s.updated_at)],
    ];
    var html = '<table class="data"><tbody>';
    rows.forEach(function (r) {
      html += '<tr><th style="width:140px;">' + r[0] + '</th><td>' + r[1] + '</td></tr>';
    });
    html += '</tbody></table>';
    sessionInfo.innerHTML = html;
  }

  function renderMembers(members) {
    memberList.innerHTML = '';
    (members || []).forEach(function (m) {
      var avatar = m.avatar_url
        ? '<img src="' + esc(m.avatar_url) + '" alt="" style="width:32px;height:32px;border-radius:50%;object-fit:cover;" />'
        : '';
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + (m.user_id || 0) + '</td>' +
        '<td>' + avatar + '</td>' +
        '<td>' + esc(m.nickname || '') + '</td>' +
        '<td>' + esc(m.role || '') + '</td>' +
        '<td>' + esc(m.member_status || '') + '</td>' +
        '<td>' + esc(m.presence_status || '') + '</td>' +
        '<td>' + fmtTime(m.joined_at) + '</td>' +
        '<td>' + fmtTime(m.last_seen_at) + '</td>';
      memberList.appendChild(tr);
    });
    memberHint.textContent = '共 ' + (members || []).length + ' 人';
  }

  function renderPositions(positions) {
    positionList.innerHTML = '';
    (positions || []).forEach(function (p) {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + (p.user_id || 0) + '</td>' +
        '<td>' + esc(p.latitude) + '</td>' +
        '<td>' + esc(p.longitude) + '</td>' +
        '<td>' + esc(p.coordinate_system || '') + '</td>' +
        '<td>' + esc(p.speed_kmh || 0) + '</td>' +
        '<td>' + esc(p.heading || 0) + '</td>' +
        '<td>' + esc(p.accuracy_m || 0) + '</td>' +
        '<td>' + esc(p.altitude || 0) + '</td>' +
        '<td>' + fmtTime(p.recorded_at) + '</td>' +
        '<td>' + (p.seq || 0) + '</td>' +
        '<td>' + esc(p.source || '') + '</td>';
      positionList.appendChild(tr);
    });
    positionHint.textContent = '共 ' + (positions || []).length + ' 条';
  }

  function renderDanmakus(danmakus) {
    danmakuList.innerHTML = '';
    (danmakus || []).forEach(function (d) {
      var tr = document.createElement('tr');
      tr.innerHTML = '<td>' + (d.id || 0) + '</td>' +
        '<td>' + (d.user_id || 0) + '</td>' +
        '<td>' + esc(d.content || '') + '</td>' +
        '<td>' + fmtTime(d.created_at) + '</td>';
      danmakuList.appendChild(tr);
    });
    danmakuHint.textContent = '共 ' + (danmakus || []).length + ' 条';
  }

  async function load() {
    var resp = await fetch('/admin/api/companions/' + encodeURIComponent(sessionID));
    if (!resp.ok) {
      sessionInfo.innerHTML = '<span class="error">加载失败：' + resp.status + '</span>';
      return;
    }
    var data = ((await resp.json()).data) || {};
    renderSession(data.session);
    renderMembers(data.members);
    renderPositions(data.live_positions);
    renderDanmakus(data.danmakus);
  }

  load();
})();
