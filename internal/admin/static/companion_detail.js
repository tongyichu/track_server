(function () {
  // 通用：登录态校验 + 退出

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function fmtTime(s) {
    return esc(AdminTime.dateTime(s));
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
  var memberTrackWarning = document.getElementById('memberTrackWarning');
  var loadController = null;
  var loadVersion = 0;
  var hasLoaded = false;

  function trackCell(track) {
    return window.AdminCompanionMemberTrack.cell(track, function (trackID) {
      window.AdminTrackEditor.open(trackID, { onSaved: load }).catch(function (error) {
        window.AdminUI.alert('打开轨迹失败：' + (error.message || error), 'danger');
      });
    });
  }

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
    var existing = new Map(Array.from(memberList.children).map(function (row) { return [row.dataset.userId, row]; }));
    (members || []).forEach(function (m, index) {
      var key = String(m.user_id);
      var signature = JSON.stringify(m);
      var tr = existing.get(key) || document.createElement('tr');
      existing.delete(key);
      if (tr.dataset.signature !== signature) {
        var avatar = m.avatar_url
          ? '<img src="' + esc(m.avatar_url) + '" alt="" style="width:32px;height:32px;border-radius:50%;object-fit:cover;" />'
          : '';
        tr.innerHTML = '<td>' + esc(m.user_id || '') + '</td>' +
          '<td>' + avatar + '</td>' +
          '<td>' + esc(m.nickname || '') + '</td>' +
          '<td>' + esc(m.role || '') + '</td>' +
          '<td>' + esc(m.member_status || '') + '</td>' +
          '<td>' + esc(m.presence_status || '') + '</td>' +
          '<td>' + fmtTime(m.joined_at) + '</td>' +
          '<td>' + fmtTime(m.last_seen_at) + '</td>';
        tr.insertBefore(trackCell(m.track), tr.children[3]);
        tr.dataset.userId = key;
        tr.dataset.signature = signature;
      }
      if (memberList.children[index] !== tr) memberList.insertBefore(tr, memberList.children[index] || null);
    });
    existing.forEach(function (row) { row.remove(); });
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
    if (loadController) loadController.abort();
    var controller = loadController = new AbortController();
    var version = ++loadVersion;
    var timer = setTimeout(function () { controller.abort(); }, 12000);
    try {
      var resp = await fetch('/admin/api/companions/' + encodeURIComponent(sessionID), { signal: controller.signal });
      if (!resp.ok) throw new Error('加载失败：' + resp.status);
      var data = ((await resp.json()).data) || {};
      if (version !== loadVersion || controller.signal.aborted) return;
      renderSession(data.session);
      renderMembers(data.members);
      renderPositions(data.live_positions);
      renderDanmakus(data.danmakus);
      memberTrackWarning.textContent = data.member_tracks_warning || '';
      memberTrackWarning.hidden = !data.member_tracks_warning;
      hasLoaded = true;
    } catch (error) {
      if (version !== loadVersion) return;
      memberHint.textContent = controller.signal.aborted ? '加载超时，请刷新重试' : error.message;
      if (!hasLoaded) sessionInfo.textContent = memberHint.textContent;
    } finally {
      clearTimeout(timer);
      if (loadController === controller) loadController = null;
    }
  }

  document.getElementById('refreshMembers').addEventListener('click', load);
  window.addEventListener('pagehide', function () { ++loadVersion; if (loadController) loadController.abort(); });
  window.addEventListener('pageshow', function (event) { if (event.persisted) load(); });
  load();
})();
