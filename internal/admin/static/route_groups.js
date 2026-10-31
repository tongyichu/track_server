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
    return (g.city_codes || []).map(function (code) { return cityNames[code] || code; }).join(', ');
  }
  function areaTypeName(type) {
    if (type === 'scenic_spot') return '景区';
    if (type === 'district') return '区县';
    return type || '';
  }
  function areaInfoHTML(area) {
    if (!area || !area.name) return '<div class="route-area-overview"><span class="route-area-label">关联区域</span><strong>未关联区域</strong><span class="hint">未命中区域目录</span></div>';
    var html = '<div class="route-area-overview"><span class="route-area-label">关联区域</span><div class="route-area-value"><strong>' + esc(area.name) + '</strong>';
    var typeName = areaTypeName(area.type);
    if (typeName) html += ' <span class="route-type-badge route-type-other">' + esc(typeName) + '</span>';
    html += '</div>';
    if (area.city_name) html += '<span class="route-area-city">' + esc(area.city_name) + '</span>';
    if (area.introduction_url && area.introduction_url.indexOf('/api/v1/track-map/areas/') === 0) {
      html += '<a class="admin-link" href="' + esc(area.introduction_url) + '" target="_blank" rel="noopener">查看区域介绍</a>';
    }
    return html + '</div>';
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
  var cityNames = Object.create(null);
  var detailModal = new tabler.Modal(document.getElementById('detailModal'));

  document.querySelectorAll('[data-route-tab]').forEach(function (button) {
    button.addEventListener('click', function () {
      document.querySelectorAll('[data-route-tab]').forEach(function (item) { item.classList.toggle('active', item === button); });
      document.querySelectorAll('[data-route-panel]').forEach(function (panel) { panel.hidden = panel.dataset.routePanel !== button.dataset.routeTab; });
    });
  });

  document.getElementById('refreshBtn').addEventListener('click', loadGroups);
  document.getElementById('renameBtn').addEventListener('click', async function () {
    if (!selectedGroupID) return;
    var name = document.getElementById('renameInput').value.trim();
    if (!name) { AdminUI.alert('请输入新名称', 'danger'); return; }
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(selectedGroupID) + '/name', {
        method: 'PUT',
        body: JSON.stringify({ name: name })
      });
      await loadDetail(selectedGroupID);
      await loadGroups();
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    }
  });
  document.getElementById('mergeBtn').addEventListener('click', async function () {
    if (!selectedGroupID) return;
    var sourceGroupID = document.getElementById('mergeInput').value.trim();
    if (!sourceGroupID) { AdminUI.alert('请输入源路线组ID', 'danger'); return; }
    if (!await AdminUI.confirm({ title: '合并路线组', message: '确认把 ' + sourceGroupID + ' 合并到 ' + selectedGroupID + '？', confirmText: '确认合并', danger: true })) return;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(selectedGroupID) + '/merge', {
        method: 'POST',
        body: JSON.stringify({ source_group_id: sourceGroupID })
      });
      document.getElementById('mergeInput').value = '';
      await loadDetail(selectedGroupID);
      await loadGroups();
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    }
  });
  document.getElementById('introSaveBtn').addEventListener('click', saveIntroduction);
  document.getElementById('introPublishBtn').addEventListener('click', function () { setIntroductionPublished(true); });
  document.getElementById('introUnpublishBtn').addEventListener('click', function () { setIntroductionPublished(false); });

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
      groupList.innerHTML = '<tr><td colspan="7"><div class="route-table-state route-table-error">加载失败：' + esc(err.message) + '</div></td></tr>';
      statusHint.textContent = '';
    }
  }

  function renderGroups(items) {
    totalHint.textContent = '本次显示 ' + items.length + ' 组';
    groupList.innerHTML = '';
    if (!items.length) {
      groupList.innerHTML = '<tr><td colspan="7"><div class="route-table-state"><strong>没有找到路线组</strong><span>可以调整运动类型或城市条件后重新查询。</span></div></td></tr>';
      return;
    }
    items.forEach(function (g) {
      var tr = document.createElement('tr');
      tr.dataset.groupId = g.group_id;
      if (g.group_id === selectedGroupID) tr.className = 'selected';
      var areaName = g.area && g.area.name ? g.area.name : '未关联区域';
      var trackTypeName = ({hiking: '徒步', running: '跑步', climbing: '爬山', riding: '骑行', driving: '自驾'})[g.track_type] || g.track_type || '-';
      var typeClass = ({hiking: 'hiking', running: 'running', climbing: 'climbing', riding: 'riding', driving: 'driving'})[g.track_type] || 'other';
      var cityLabel = groupCities(g) || (g.area && g.area.city_name) || '城市未标注';
      var representativeID = g.representative_track_id || '';
      tr.innerHTML = '<td class="route-name-cell"><strong title="' + esc(g.name || '') + '">' + esc(g.name || '未命名路线') + '</strong><small title="' + esc(g.group_id) + '">路线组 · ' + esc(g.group_id) + '</small></td>' +
        '<td class="route-area-cell"><strong title="' + esc(areaName) + '">' + esc(areaName) + '</strong><small title="' + esc(cityLabel) + '">' + esc(cityLabel) + '</small></td>' +
        '<td><span class="route-type-badge route-type-' + typeClass + '">' + esc(trackTypeName) + '</span></td>' +
        '<td><span class="route-member-count">' + Number(g.member_count || 0).toLocaleString() + '</span><small class="route-cell-caption">条轨迹</small></td>' +
        '<td class="route-representative-cell" title="' + esc(representativeID) + '">' + (representativeID ? '<code>' + esc(representativeID) + '</code>' : '<span class="hint">未设置</span>') + '</td>' +
        '<td class="route-updated-cell">' + (g.updated_at ? fmtTime(g.updated_at) : '<span class="hint">—</span>') + '</td>' +
        '<td><button class="btn btn-sm btn-outline-primary route-manage-button" data-id="' + esc(g.group_id) + '">管理</button></td>';
      if (g.group_id === selectedGroupID) tr.className = 'selected';
      tr.querySelector('button').addEventListener('click', function () {
        loadDetail(g.group_id);
      });
      groupList.appendChild(tr);
    });
  }

  async function loadDetail(groupID) {
    selectedGroupID = groupID;
    document.getElementById('detailPanel').hidden = false;
    document.getElementById('detailName').textContent = '路线组管理';
    detailModal.show();
    document.querySelectorAll('[data-route-tab]').forEach(function (item) { item.classList.toggle('active', item.dataset.routeTab === 'members'); });
    document.querySelectorAll('[data-route-panel]').forEach(function (panel) { panel.hidden = panel.dataset.routePanel !== 'members'; });
    renderGroupsFromSelection();
    detailHint.textContent = '加载中...';
    try {
      var data = await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '?limit=100');
      renderDetail(data);
      await loadIntroduction(groupID);
      detailHint.textContent = '';
    } catch (err) {
      detailHint.textContent = '加载失败: ' + err.message;
    }
  }

  function renderGroupsFromSelection() {
    var selected = selectedGroupID;
    var rows = groupList.querySelectorAll('tr[data-group-id]');
    rows.forEach(function (row) { row.classList.toggle('selected', row.dataset.groupId === selected); });
  }

  function lines(id) { return document.getElementById(id).value.split('\n').map(function (s) { return s.trim(); }).filter(Boolean); }
  function putLines(id, values) { document.getElementById(id).value = (values || []).join('\n'); }
  async function loadIntroduction(groupID) {
    var intro = await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/introduction');
    var zh = intro.zh || {}, en = intro.en || {};
    document.getElementById('introZhName').value = zh.name || '';
    document.getElementById('introEnName').value = en.name || '';
    document.getElementById('introZhSummary').value = zh.summary || '';
    document.getElementById('introEnSummary').value = en.summary || '';
    putLines('introZhDescription', zh.description); putLines('introEnDescription', en.description);
    putLines('introZhHighlights', zh.highlights); putLines('introEnHighlights', en.highlights);
    putLines('introZhTips', zh.tips); putLines('introEnTips', en.tips);
    document.getElementById('introDifficulty').value = intro.difficulty || '';
    document.getElementById('introBestSeasons').value = (intro.best_seasons || []).join(',');
    document.getElementById('introDurationMin').value = intro.estimated_duration_min || 0;
    document.getElementById('introDurationMax').value = intro.estimated_duration_max || 0;
    document.getElementById('introStatus').textContent = '状态：' + (intro.status || 'draft') + '；版本：' + (intro.content_version || 0);
    var link = document.getElementById('introPreviewLink');
    link.hidden = intro.status !== 'published';
    link.href = '/api/v1/track-map/groups/' + encodeURIComponent(groupID) + '/introduction.html?v=' + (intro.content_version || 1);
  }
  function introductionPayload() {
    return {zh:{name:document.getElementById('introZhName').value.trim(),summary:document.getElementById('introZhSummary').value.trim(),description:lines('introZhDescription'),highlights:lines('introZhHighlights'),tips:lines('introZhTips')},
      en:{name:document.getElementById('introEnName').value.trim(),summary:document.getElementById('introEnSummary').value.trim(),description:lines('introEnDescription'),highlights:lines('introEnHighlights'),tips:lines('introEnTips')},
      difficulty:document.getElementById('introDifficulty').value.trim(),best_seasons:document.getElementById('introBestSeasons').value.split(',').map(function(s){return s.trim();}).filter(Boolean),
      estimated_duration_min:Number(document.getElementById('introDurationMin').value||0),estimated_duration_max:Number(document.getElementById('introDurationMax').value||0)};
  }
  async function saveIntroduction() {
    if (!selectedGroupID) return;
    try { await apiJSON('/admin/api/route-groups/'+encodeURIComponent(selectedGroupID)+'/introduction',{method:'PUT',body:JSON.stringify(introductionPayload())}); await loadIntroduction(selectedGroupID); }
    catch(err){ AdminUI.alert(err.message, 'danger'); }
  }
  async function setIntroductionPublished(published) {
    if (!selectedGroupID) return;
    try { await apiJSON('/admin/api/route-groups/'+encodeURIComponent(selectedGroupID)+'/introduction/'+(published?'publish':'unpublish'),{method:'POST'}); await loadIntroduction(selectedGroupID); await loadGroups(); }
    catch(err){ AdminUI.alert(err.message, 'danger'); }
  }

  function renderDetail(data) {
    var g = data.group || {};
    document.getElementById('detailGroupID').textContent = g.group_id || '';
    document.getElementById('detailName').textContent = g.name || '未命名路线';
    document.getElementById('detailTrackType').textContent = ({hiking: '徒步', running: '跑步', climbing: '爬山', riding: '骑行', driving: '自驾'})[g.track_type] || g.track_type || '-';
    document.getElementById('detailCities').textContent = groupCities(g) || (data.area && data.area.city_name) || '-';
    document.getElementById('detailAreaID').textContent = g.area_id || '-';
    document.getElementById('detailAreaInfo').innerHTML = areaInfoHTML(data.area);
    document.getElementById('detailSource').textContent = g.source || '-';
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
        '<td>' + esc(cityNames[geo.city_code] || geo.city_code || '') + '</td>' +
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
      AdminUI.alert(err.message, 'danger');
    }
  }

  async function removeMember(trackID) {
    if (!selectedGroupID || !trackID) return;
    if (!await AdminUI.confirm({ title: '移除成员轨迹', message: '确认从当前路线组移除轨迹 ' + trackID + '？', confirmText: '确认移除', danger: true })) return;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(selectedGroupID) + '/members/' + encodeURIComponent(trackID), {
        method: 'DELETE'
      });
      await loadDetail(selectedGroupID);
      await loadGroups();
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    }
  }

  async function loadCities() {
    try {
      var items = await apiJSON('/admin/api/cities');
      var select = document.getElementById('cityCodeInput');
      (items || []).forEach(function (city) {
        cityNames[city.code] = city.name;
        var option = document.createElement('option');
        option.value = city.code;
        option.textContent = city.name;
        select.appendChild(option);
      });
    } catch (_) { /* 城市目录不可用时保留“全部城市”，不阻塞路线组管理。 */ }
  }

  window.AdminRouteGroups = { openDetail: loadDetail, refresh: loadGroups };
  loadCities().then(loadGroups);
})();
