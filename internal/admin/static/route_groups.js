(function () {
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
    return esc((s || '').replace('T', ' ').slice(0, 19));
  }
  function groupCities(g) {
    return (g.city_codes || []).join(', ');
  }
  function apiJSON(url, options) {
    options = options || {};
    options.headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
    return fetch(url, options).then(async function (resp) {
      var body = await resp.json().catch(function () { return {}; });
      if (!resp.ok) {
        throw new Error(body.error || ('HTTP ' + resp.status));
      }
      return body.data;
    });
  }

  var selectedGroupID = '';
  var groupList = document.getElementById('groupList');
  var memberList = document.getElementById('memberList');
  var totalHint = document.getElementById('totalHint');
  var detailHint = document.getElementById('detailHint');
  var statusHint = document.getElementById('statusHint');

  document.getElementById('refreshBtn').addEventListener('click', loadGroups);
  document.getElementById('renameBtn').addEventListener('click', async function () {
    if (!selectedGroupID) return;
    var name = document.getElementById('renameInput').value.trim();
    if (!name) { alert('请输入新名称'); return; }
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(selectedGroupID) + '/name', {
        method: 'PUT',
        body: JSON.stringify({ name: name })
      });
      await loadDetail(selectedGroupID);
      await loadGroups();
    } catch (err) {
      alert(err.message);
    }
  });
  document.getElementById('mergeBtn').addEventListener('click', async function () {
    if (!selectedGroupID) return;
    var sourceGroupID = document.getElementById('mergeInput').value.trim();
    if (!sourceGroupID) { alert('请输入源路线组ID'); return; }
    if (!confirm('确认把 ' + sourceGroupID + ' 合并到 ' + selectedGroupID + '？')) return;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(selectedGroupID) + '/merge', {
        method: 'POST',
        body: JSON.stringify({ source_group_id: sourceGroupID })
      });
      document.getElementById('mergeInput').value = '';
      await loadDetail(selectedGroupID);
      await loadGroups();
    } catch (err) {
      alert(err.message);
    }
  });

  async function loadGroups() {
    statusHint.textContent = '加载中...';
    var params = new URLSearchParams();
    params.set('limit', '100');
    var trackType = document.getElementById('trackTypeInput').value.trim();
    var cityCode = document.getElementById('cityCodeInput').value.trim();
    if (trackType) params.set('track_type', trackType);
    if (cityCode) params.set('city_code', cityCode);
    try {
      var data = await apiJSON('/admin/api/route-groups?' + params.toString());
      renderGroups(data.items || []);
      statusHint.textContent = '';
    } catch (err) {
      groupList.innerHTML = '<tr><td colspan="9">加载失败: ' + esc(err.message) + '</td></tr>';
      statusHint.textContent = '';
    }
  }

  function renderGroups(items) {
    totalHint.textContent = '共 ' + items.length + ' 条路线组';
    groupList.innerHTML = '';
    items.forEach(function (g) {
      var tr = document.createElement('tr');
      if (g.group_id === selectedGroupID) tr.className = 'selected';
      tr.innerHTML = '<td class="mono">' + esc(g.group_id) + '</td>' +
        '<td class="clip">' + esc(g.name || '') + '</td>' +
        '<td>' + esc(g.track_type || '') + '</td>' +
        '<td>' + esc(groupCities(g)) + '</td>' +
        '<td>' + (g.member_count || 0) + '</td>' +
        '<td>' + esc(g.source || '') + '</td>' +
        '<td class="mono">' + esc(g.representative_track_id || '') + '</td>' +
        '<td>' + fmtTime(g.updated_at) + '</td>' +
        '<td><button data-id="' + esc(g.group_id) + '">查看</button></td>';
      tr.querySelector('button').addEventListener('click', function () {
        loadDetail(g.group_id);
      });
      groupList.appendChild(tr);
    });
  }

  async function loadDetail(groupID) {
    selectedGroupID = groupID;
    detailHint.textContent = '加载中...';
    try {
      var data = await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '?limit=100');
      renderDetail(data);
      detailHint.textContent = '';
    } catch (err) {
      detailHint.textContent = '加载失败: ' + err.message;
    }
  }

  function renderDetail(data) {
    var g = data.group || {};
    document.getElementById('detailGroupID').textContent = g.group_id || '';
    document.getElementById('detailName').textContent = g.name || '';
    document.getElementById('detailTrackType').textContent = g.track_type || '';
    document.getElementById('detailCities').textContent = groupCities(g);
    document.getElementById('detailMemberCount').textContent = String(g.member_count || 0);
    document.getElementById('detailRepresentative').textContent = g.representative_track_id || '';
    document.getElementById('renameInput').value = g.name || '';
    renderMembers(data.members || [], g.representative_track_id || '');
  }

  function renderMembers(items, representativeTrackID) {
    memberList.innerHTML = '';
    items.forEach(function (row) {
      var m = row.member || {};
      var geo = row.geo_index || {};
      var tr = document.createElement('tr');
      tr.innerHTML = '<td class="mono">' + esc(m.track_id) + '</td>' +
        '<td>' + esc(m.role || '') + '</td>' +
        '<td>' + esc(m.match_direction || '') + '</td>' +
        '<td>' + (m.similarity_score != null ? Number(m.similarity_score).toFixed(3) : '') + '</td>' +
        '<td>' + esc(geo.city_code || '') + '</td>' +
        '<td>' + (geo.distance != null ? Number(geo.distance).toFixed(1) : '') + '</td>' +
        '<td>' + (geo.point_count || '') + '</td>' +
        '<td>' + esc(m.source || '') + '</td>' +
        '<td class="row-actions"></td>';
      var actions = tr.querySelector('.row-actions');
      if (m.track_id !== representativeTrackID) {
        var repBtn = document.createElement('button');
        repBtn.textContent = '设代表';
        repBtn.addEventListener('click', function () { setRepresentative(m.track_id); });
        actions.appendChild(repBtn);
      }
      var removeBtn = document.createElement('button');
      removeBtn.className = 'danger';
      removeBtn.textContent = '移除';
      removeBtn.addEventListener('click', function () { removeMember(m.track_id); });
      actions.appendChild(removeBtn);
      memberList.appendChild(tr);
    });
  }

  async function setRepresentative(trackID) {
    if (!selectedGroupID || !trackID) return;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(selectedGroupID) + '/representative', {
        method: 'PUT',
        body: JSON.stringify({ track_id: trackID })
      });
      await loadDetail(selectedGroupID);
      await loadGroups();
    } catch (err) {
      alert(err.message);
    }
  }

  async function removeMember(trackID) {
    if (!selectedGroupID || !trackID) return;
    if (!confirm('确认从当前路线组移除轨迹 ' + trackID + '？')) return;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(selectedGroupID) + '/members/' + encodeURIComponent(trackID), {
        method: 'DELETE'
      });
      await loadDetail(selectedGroupID);
      await loadGroups();
    } catch (err) {
      alert(err.message);
    }
  }

  loadGroups();
})();
