(function () {

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  var TRACK_TYPES = [
    { type: 'hiking', name: '徒步', icon: 'mingcute-hiking-line-v1.svg', color: '#00875A' },
    { type: 'running', name: '跑步', icon: 'mingcute-run-line-v1.svg', color: '#F26A4B' },
    { type: 'climbing', name: '爬山', icon: 'mingcute-mountain-2-line-v1.svg', color: '#6C4CE1' },
    { type: 'riding', name: '骑行', icon: 'mingcute-bike-line-v1.svg', color: '#2F80ED' },
    { type: 'driving', name: '自驾', icon: 'mingcute-car-line-v1.svg', color: '#F5A623' }
  ];
  function trackTypeMeta(type) {
    for (var i = 0; i < TRACK_TYPES.length; i++) {
      if (TRACK_TYPES[i].type === type) return TRACK_TYPES[i];
    }
    return null;
  }
  function typeIconHTML(meta, className) {
    if (!meta) return '';
    var url = '/admin/static/track_type_icon/' + meta.icon;
    if (window.AdminAssets && AdminAssets.url) url = AdminAssets.url(url);
    return '<span class="' + className + '" style="background-color:' + meta.color + ';-webkit-mask-image:url(\'' + url + '\');mask-image:url(\'' + url + '\')" aria-hidden="true"></span>';
  }
  function typeBadgeHTML(type) {
    var meta = trackTypeMeta(type);
    var typeClass = meta ? meta.type : 'other';
    var name = meta ? meta.name : (type || '-');
    return '<span class="route-type-badge route-type-' + typeClass + '">' + typeIconHTML(meta, 'route-type-badge-icon') + esc(name) + '</span>';
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
    if (!area || !area.name) return '<div class="route-area-overview"><span class="route-area-label">离线聚合区域</span><strong>未关联区域</strong><span class="hint">未命中区域目录</span></div>';
    var html = '<div class="route-area-overview"><span class="route-area-label">离线聚合区域</span><div class="route-area-value"><strong>' + esc(area.name) + '</strong>';
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

  var PAGE_SIZE = 50;
  var selectedGroupID = '', detailRequest = 0, detailSession = 0, detailController = null, detailLoadingID = '';
  var introBaseline = '', introWriting = false, introClosePrompt = false, allowIntroClose = false;
  var introStatus = '', introAIController = null, introLoadController = null;
  var selectedTrackType = '';
  var loadedGroups = [];
  var nextOffset = 0;
  var totalCount = 0;
  var listLoading = false, listController = null;
  var listLoadSeq = 0;
  var areaExpansion = new Map();
  var groupList = document.getElementById('groupList');
  var memberList = document.getElementById('memberList');
  var memberMatchDetails = document.getElementById('memberMatchDetails');
  var routeMemberTable = document.getElementById('routeMemberTable');
  var introAdvancedFields = document.getElementById('introAdvancedFields');
  var detailHint = document.getElementById('detailHint');
  var detailRetryBtn = document.getElementById('detailRetryBtn');
  var detailPanel = document.getElementById('detailPanel');
  var detailPath = document.getElementById('detailPath');
  var statusHint = document.getElementById('statusHint');
  var listSummaryHint = document.getElementById('listSummaryHint');
  var loadMoreBtn = document.getElementById('loadMoreBtn');
  var introPanel = document.querySelector('[data-route-panel="introduction"]');
  var introEditor = document.getElementById('introEditor');
  var introLoadState = document.getElementById('introLoadState');
  var introLoadHint = document.getElementById('introLoadHint');
  var introRetryBtn = document.getElementById('introRetryBtn');
  var cityNames = Object.create(null);
  var detailModal = new AdminComponents.Dialog(document.getElementById('detailModal'));
  function showMemberMatchDetails(show) {
    memberMatchDetails.checked = show;
    routeMemberTable.classList.toggle('show-match-details', show);
  }
  memberMatchDetails.addEventListener('change', function () { showMemberMatchDetails(memberMatchDetails.checked); });
  document.getElementById('detailModal').addEventListener('admin:dialog-closing', function (event) {
    if (introWriting) { event.preventDefault(); return; }
    if (!allowIntroClose && introDirty()) {
      event.preventDefault();
      if (introClosePrompt) return;
      introClosePrompt = true;
      AdminUI.confirm({ title: '放弃未保存的路线介绍？', message: '当前编辑的路线介绍尚未保存。', confirmText: '放弃修改', waitForHidden: true }).then(function (confirmed) {
        if (confirmed) { allowIntroClose = true; detailModal.hide(); }
      }).finally(function () { introClosePrompt = false; });
      return;
    }
    allowIntroClose = false;
    showMemberMatchDetails(false);
    introAdvancedFields.open = false;
    resetIntroduction();
    selectedGroupID = '';
    detailSession++;
    detailRequest++;
    detailLoadingID = '';
    if (detailController) detailController.abort();
    renderGroupsFromSelection();
  });

  function selectRouteTab(tab) {
    document.querySelectorAll('[data-route-tab]').forEach(function (button) {
      var selected = button.dataset.routeTab === tab;
      button.classList.toggle('active', selected);
      button.setAttribute('aria-pressed', String(selected));
    });
    document.querySelectorAll('[data-route-panel]').forEach(function (panel) { panel.hidden = panel.dataset.routePanel !== tab; });
    if (tab === 'introduction') loadIntroduction();
  }
  document.querySelectorAll('[data-route-tab]').forEach(function (button) {
    button.addEventListener('click', function () { selectRouteTab(button.dataset.routeTab); });
  });
  detailRetryBtn.addEventListener('click', function () { loadDetail(selectedGroupID); });
  introRetryBtn.addEventListener('click', loadIntroduction);

  // ---------- 运动类型下拉（icon + 文字） ----------
  var typeToggle = document.getElementById('trackTypeToggle');
  var typeMenu = document.getElementById('trackTypeMenu');
  var typePopup;

  function typeOptionHTML(value, iconMeta, label) {
    return '<button type="button" class="route-type-option' + (value === selectedTrackType ? ' active' : '') + '" role="option" data-type-value="' + esc(value) + '" aria-selected="' + (value === selectedTrackType) + '">' +
      (iconMeta ? typeIconHTML(iconMeta, 'route-type-option-icon') : '<span class="route-type-option-icon route-type-option-icon-all" aria-hidden="true"></span>') +
      '<span>' + esc(label) + '</span></button>';
  }
  function renderTypeToggle() {
    var meta = trackTypeMeta(selectedTrackType);
    typeToggle.innerHTML = (meta ? typeIconHTML(meta, 'route-type-option-icon') : '<span class="route-type-option-icon route-type-option-icon-all" aria-hidden="true"></span>') +
      '<span class="route-type-toggle-label">' + esc(meta ? meta.name : '全部类型') + '</span>';
  }
  function renderTypeMenu() {
    var html = typeOptionHTML('', null, '全部类型');
    TRACK_TYPES.forEach(function (meta) { html += typeOptionHTML(meta.type, meta, meta.name); });
    typeMenu.innerHTML = html;
    typeMenu.querySelectorAll('.route-type-option').forEach(function (option) {
      option.addEventListener('click', function () {
        selectedTrackType = option.dataset.typeValue || '';
        renderTypeToggle();
        closeTypeMenu();
        resetAndLoad();
      });
    });
  }
  function openTypeMenu() {
    if (!typeMenu.hidden) return;
    renderTypeMenu();
    typeMenu.hidden = false;
    typePopup = AdminComponents.anchorPopup(typeToggle, typeMenu, { matchWidth: true });
    typeToggle.setAttribute('aria-expanded', 'true');
  }
  function closeTypeMenu() {
    typePopup?.close();
    typePopup = null;
    typeMenu.hidden = true;
    typeToggle.setAttribute('aria-expanded', 'false');
  }
  typeToggle.addEventListener('click', function () {
    if (typeMenu.hidden) { openTypeMenu(); } else { closeTypeMenu(); }
  });
  typeToggle.addEventListener('keydown', function (event) {
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      openTypeMenu();
      var current = typeMenu.querySelector('.route-type-option.active') || typeMenu.querySelector('.route-type-option');
      if (current) current.focus();
    }
  });
  typeMenu.addEventListener('keydown', function (event) {
    var options = Array.prototype.slice.call(typeMenu.querySelectorAll('.route-type-option'));
    var index = options.indexOf(document.activeElement);
    if (event.key === 'ArrowDown' && index < options.length - 1) { event.preventDefault(); options[index + 1].focus(); }
    if (event.key === 'ArrowUp' && index > 0) { event.preventDefault(); options[index - 1].focus(); }
    if (event.key === 'Escape') { closeTypeMenu(); typeToggle.focus(); }
  });
  document.addEventListener('click', function (event) {
    if (!typeMenu.hidden && !document.getElementById('trackTypeSelect').contains(event.target)) closeTypeMenu();
  });
  window.addEventListener('pagehide', closeTypeMenu);

  // ---------- 省 / 市级联 ----------
  var provinceSelect = document.getElementById('provinceCodeInput');
  var citySelect = document.getElementById('cityCodeInput');
  var citiesByProvince = Object.create(null);

  function renderCityOptions(provinceCode) {
    var options = '<option value="">全部城市</option>';
    var list = (provinceCode && citiesByProvince[provinceCode]) || Object.keys(citiesByProvince)
      .reduce(function (acc, key) { return acc.concat(citiesByProvince[key]); }, []);
    list.forEach(function (city) {
      options += '<option value="' + esc(city.code) + '">' + esc(city.name) + '</option>';
    });
    citySelect.innerHTML = options;
  }
  provinceSelect.addEventListener('change', function () {
    renderCityOptions(provinceSelect.value);
    resetAndLoad();
  });
  citySelect.addEventListener('change', function () { resetAndLoad(); });

  // ---------- 列表加载与分页 ----------
  document.getElementById('refreshBtn').addEventListener('click', reloadKeepingPage);
  loadMoreBtn.addEventListener('click', function () { loadGroups(false); });

  function resetAndLoad() {
    loadedGroups = [];
    nextOffset = 0;
    totalCount = 0;
    areaExpansion.clear();
    loadGroups(true);
  }

  function listParams(offset, limit) {
    var params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (selectedTrackType) params.set('track_type', selectedTrackType);
    if (citySelect.value) params.set('city_code', citySelect.value);
    else if (provinceSelect.value) params.set('province_code', provinceSelect.value);
    return params;
  }

  async function loadGroups(reset) {
    if (listLoading && !reset) return;
    if (listController) listController.abort();
    var controller = listController = new AbortController();
    listLoading = true;
    var seq = ++listLoadSeq;
    if (reset) {
      groupList.innerHTML = '<tr><td colspan="4"><div class="route-table-state">加载中…</div></td></tr>';
      listSummaryHint.textContent = '';
    }
    statusHint.textContent = '加载中...';
    loadMoreBtn.hidden = true;
    var params = listParams(reset ? 0 : nextOffset, PAGE_SIZE);
    try {
      var data = await apiJSON('/admin/api/route-groups?' + params.toString(), { signal: controller.signal });
      if (seq !== listLoadSeq) return;
      replaceLoadedGroups(reset ? [] : loadedGroups, data.items || []);
      totalCount = Number(data.total || 0);
      nextOffset = Number(data.next_offset || loadedGroups.length);
      renderGroups();
      statusHint.textContent = '';
      loadMoreBtn.hidden = !data.has_more;
    } catch (err) {
      if (seq !== listLoadSeq) return;
      if (reset) {
        groupList.innerHTML = '<tr><td colspan="4"><div class="route-table-state route-table-error">加载失败 · ' + esc(err.message) + '</div></td></tr>';
      } else {
        AdminUI.alert('加载失败：' + err.message, 'danger');
        loadMoreBtn.hidden = false;
      }
      statusHint.textContent = '';
    } finally {
      if (seq === listLoadSeq) listLoading = false;
    }
  }

  function replaceLoadedGroups(previous, incoming) {
    loadedGroups = Array.from(new Map(previous.concat(incoming).map(function (group) {
      return [group.group_id, group];
    })).values());
  }

  function routeGroupLocation(g) {
    var location = g.grouping || {};
    return {
      cityCode: location.city_code,
      cityName: location.city_name || cityNames[location.city_code] || location.city_code || '城市未标注',
      districtID: location.district_id,
      districtName: location.district_name || '区县未标注',
      scenicID: location.scenic_id,
      scenicName: location.scenic_name || (location.scenic_id ? '未命名景点' : '')
    };
  }

  function buildAreaTree(items) {
    var root = { key: '', children: new Map() };
    function addGroup(parent, type, id, name) {
      var key = parent.key + '/' + type + ':' + encodeURIComponent(id || '');
      var bucket = parent.children.get(key);
      if (!bucket) {
        bucket = { key: key, id: id, type: type, name: name, children: new Map(), groups: [], count: 0 };
        parent.children.set(key, bucket);
      }
      bucket.count++;
      return bucket;
    }
    items.forEach(function (g) {
      var location = routeGroupLocation(g);
      var city = addGroup(root, 'city', location.cityCode, location.cityName);
      var district = addGroup(city, 'district', location.districtID, location.districtName);
      var leaf = location.scenicID ? addGroup(district, 'scenic', location.scenicID, location.scenicName) : district;
      leaf.groups.push(g);
    });
    return root.children;
  }

  function renderGroups(focusKey) {
    var partial = nextOffset < totalCount;
    listSummaryHint.textContent = partial ? '已加载 ' + loadedGroups.length + ' / ' + totalCount + ' 组' : '共 ' + totalCount + ' 组';
    groupList.innerHTML = '';
    if (!loadedGroups.length) {
      groupList.innerHTML = '<tr><td colspan="4"><div class="route-table-state">没有符合当前筛选的路线组</div></td></tr>';
      return;
    }
    var rows = document.createDocumentFragment();
    var focusedToggle;
    var cities = buildAreaTree(loadedGroups);
    var cityFilter = citySelect.value;
    var regionFiltered = cityFilter || provinceSelect.value;
    function appendBucket(bucket, level) {
      var expandedByFilter = level === 0 && regionFiltered && (bucket.id === cityFilter || cities.size === 1);
      var expanded = areaExpansion.has(bucket.key) ? areaExpansion.get(bucket.key) : !!expandedByFilter;
      var header = renderAreaHeader(bucket, level, expanded, partial);
      rows.appendChild(header);
      if (bucket.key === focusKey) focusedToggle = header.querySelector('.route-area-toggle');
      if (!expanded) return;
      Array.from(bucket.children.values()).sort(compareAreas).forEach(function (child) { appendBucket(child, level + 1); });
      if (bucket.type === 'district' && bucket.children.size && bucket.groups.length) {
        var divider = document.createElement('tr');
        divider.className = 'route-area-divider route-tree-depth-' + (level + 1);
        divider.innerHTML = '<td colspan="4" class="route-tree-cell">独立路线</td>';
        rows.appendChild(divider);
      }
      bucket.groups.forEach(function (g) { rows.appendChild(renderGroupRow(g, level + 1)); });
    }
    Array.from(cities.values()).sort(function (a, b) {
      return (cityFilter ? Number(b.id === cityFilter) - Number(a.id === cityFilter) : 0) || compareAreas(a, b);
    }).forEach(function (bucket) { appendBucket(bucket, 0); });
    groupList.appendChild(rows);
    if (focusedToggle) focusedToggle.focus({ preventScroll: true });
  }

  function compareAreas(a, b) {
    return Number(!a.id) - Number(!b.id) || a.name.localeCompare(b.name, 'zh-CN');
  }

  function renderAreaHeader(bucket, level, expanded, partial) {
    var tr = document.createElement('tr');
    tr.className = 'route-area-header route-area-level-' + bucket.type + ' route-tree-depth-' + level;
    tr.innerHTML = '<td colspan="4" class="route-tree-cell">' +
      '<button type="button" class="route-area-toggle" aria-expanded="' + expanded + '" title="' + esc(bucket.name) + '">' +
      '<span class="route-area-chevron" aria-hidden="true"></span>' +
      '<span class="route-area-title">' + esc(bucket.name) + '</span>' +
      '<span class="route-area-stats">' + (partial ? '已加载 ' : '') + bucket.count.toLocaleString() + ' 组</span>' +
      '</button></td>';
    tr.querySelector('.route-area-toggle').addEventListener('click', function () {
      areaExpansion.set(bucket.key, !expanded);
      renderGroups(bucket.key);
    });
    return tr;
  }

  function renderGroupRow(g, level) {
    var tr = document.createElement('tr');
    tr.className = 'route-group-row route-tree-depth-' + level;
    tr.dataset.groupId = g.group_id;
    if (g.group_id === selectedGroupID) tr.classList.add('selected');
    tr.innerHTML = '<td class="route-tree-cell route-name-cell"><strong title="' + esc(g.name || '') + '">' + esc(g.name || '未命名路线') + '</strong><code>' + esc(g.group_id) + '</code></td>' +
      '<td>' + typeBadgeHTML(g.track_type) + '</td>' +
      '<td class="route-member-count">' + Number(g.member_count || 0).toLocaleString() + '</td>' +
      '<td><button type="button" class="route-manage-button">管理</button></td>';
    tr.querySelector('button').addEventListener('click', function () {
      loadDetail(g.group_id);
    });
    return tr;
  }

  document.getElementById('renameBtn').addEventListener('click', async function () {
    if (!selectedGroupID || detailLoadingID) return;
    var groupID = selectedGroupID, session = detailSession;
    var name = document.getElementById('renameInput').value.trim();
    if (!name) { AdminUI.alert('请输入新名称', 'danger'); return; }
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/name', {
        method: 'PUT',
        body: JSON.stringify({ name: name })
      });
      await refreshGroupAfterChange(groupID, session);
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    }
  });
  document.getElementById('mergeBtn').addEventListener('click', async function () {
    if (!selectedGroupID || detailLoadingID) return;
    var groupID = selectedGroupID, session = detailSession;
    var sourceGroupID = document.getElementById('mergeInput').value.trim();
    if (!sourceGroupID) { AdminUI.alert('请输入源路线组ID', 'danger'); return; }
    if (!await AdminUI.confirm({ title: '合并路线组', message: '确认把 ' + sourceGroupID + ' 合并到 ' + groupID + '？', confirmText: '确认合并', danger: true })) return;
    if (!isCurrentDetail(groupID, session)) return;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/merge', {
        method: 'POST',
        body: JSON.stringify({ source_group_id: sourceGroupID })
      });
      if (isCurrentDetail(groupID, session)) document.getElementById('mergeInput').value = '';
      await refreshGroupAfterChange(groupID, session);
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    }
  });
  document.getElementById('introSaveBtn').addEventListener('click', function () { writeIntroduction('draft'); });
  document.getElementById('introGenerateBtn').addEventListener('click', generateIntroductionDraft);
  document.getElementById('introPublishBtn').addEventListener('click', function () { writeIntroduction('publish'); });
  document.getElementById('introUnpublishBtn').addEventListener('click', function () { writeIntroduction('unpublish'); });

  // 变更后重新加载已加载的全部页，保持列表与详情一致。
  async function reloadKeepingPage() {
    if (listLoading) return;
    var controller = listController = new AbortController();
    listLoading = true;
    var seq = ++listLoadSeq;
    statusHint.textContent = '刷新中…';
    try {
      var target = Math.max(nextOffset, PAGE_SIZE);
      var batch = [], offset = 0, data;
      do {
        var params = listParams(offset, Math.min(PAGE_SIZE, target - offset));
        data = await apiJSON('/admin/api/route-groups?' + params.toString(), { signal: controller.signal });
        if (seq !== listLoadSeq) return;
        batch = batch.concat(data.items || []);
        var next = Number(data.next_offset || batch.length);
        if (next <= offset) break;
        offset = next;
      } while (data.has_more && offset < target);
      replaceLoadedGroups([], batch);
      totalCount = Number(data.total || 0);
      nextOffset = offset;
      renderGroups();
      loadMoreBtn.hidden = !data.has_more;
      statusHint.textContent = '';
    } catch (err) {
      if (seq === listLoadSeq) {
        statusHint.textContent = '';
        AdminUI.alert('刷新失败：' + err.message, 'danger');
      }
    } finally {
      if (seq === listLoadSeq) listLoading = false;
    }
  }

  async function loadDetail(groupID) {
    if (!groupID || detailLoadingID === groupID || introWriting) return;
    if (selectedGroupID !== groupID || !detailModal.root.open) {
      detailSession++;
      memberList.replaceChildren();
      showMemberMatchDetails(false);
      introAdvancedFields.open = false;
      resetIntroduction();
      detailPanel.hidden = true;
      var cached = loadedGroups.find(function (group) { return group.group_id === groupID; });
      document.getElementById('detailName').textContent = cached && cached.name || '路线组';
      renderDetailPath(cached);
      selectRouteTab('members');
    }
    if (detailController) detailController.abort();
    var controller = detailController = new AbortController(), request = ++detailRequest;
    detailLoadingID = groupID;
    selectedGroupID = groupID;
    detailModal.show();
    renderGroupsFromSelection();
    showDetailState('加载中…', false);
    try {
      var data = await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '?limit=100', { signal: controller.signal });
      if (request !== detailRequest || controller.signal.aborted) return;
      renderDetail(data);
      detailPanel.hidden = false;
      showDetailState('', false);
    } catch (err) {
      if (request === detailRequest && !controller.signal.aborted) {
        showDetailState('详情加载失败 · ' + err.message, true);
      }
    } finally {
      if (request === detailRequest) detailLoadingID = '';
    }
  }

  function showDetailState(message, retry) {
    detailHint.textContent = message;
    detailHint.hidden = !message;
    detailRetryBtn.hidden = !retry;
  }

  function isCurrentDetail(groupID, session) {
    return detailModal.root.open && selectedGroupID === groupID && detailSession === session;
  }

  async function refreshGroupAfterChange(groupID, session) {
    if (isCurrentDetail(groupID, session)) await loadDetail(groupID);
    await reloadKeepingPage();
  }

  function renderGroupsFromSelection() {
    var selected = selectedGroupID;
    var rows = groupList.querySelectorAll('tr[data-group-id]');
    rows.forEach(function (row) { row.classList.toggle('selected', row.dataset.groupId === selected); });
  }

  function lines(id) { return document.getElementById(id).value.split('\n').map(function (s) { return s.trim(); }).filter(Boolean); }
  function putLines(id, values) { document.getElementById(id).value = (values || []).join('\n'); }
  function resetIntroduction() {
    if (introLoadController) introLoadController.abort();
    introLoadController = null;
    cancelIntroGeneration();
    introBaseline = '';
    introEditor.hidden = true;
    introLoadState.hidden = true;
    showIntroError('');
  }

  async function loadIntroduction() {
    if (!selectedGroupID || introBaseline || introLoadController) return;
    var groupID = selectedGroupID;
    var controller = introLoadController = new AbortController();
    introLoadHint.textContent = '正在读取路线介绍…';
    introLoadState.hidden = false;
    introRetryBtn.hidden = true;
    try {
      var intro = await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/introduction', { signal: controller.signal });
      if (introLoadController !== controller) return;
      renderIntroduction(intro, groupID);
      introEditor.hidden = false;
      introLoadState.hidden = true;
    } catch (err) {
      if (introLoadController !== controller) return;
      introLoadHint.textContent = '路线介绍加载失败 · ' + err.message;
      introRetryBtn.hidden = false;
    } finally {
      if (introLoadController === controller) introLoadController = null;
    }
  }
  function renderIntroduction(intro, groupID) {
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
    introStatus = intro.status;
    var published = introStatus === 'published';
    var statusText = published ? '已发布' : introStatus === 'automatic' ? '自动简介' : '草稿';
    document.getElementById('introStatus').textContent = statusText + (intro.content_version ? ' · 第 ' + intro.content_version + ' 版' : '');
    document.getElementById('introUnpublishBtn').hidden = !published;
    var link = document.getElementById('introPreviewLink');
    link.hidden = !published && introStatus !== 'automatic';
    link.href = '/api/v1/track-map/groups/' + encodeURIComponent(groupID) + '/introduction.html' + (published ? '?v=' + intro.content_version : '');
    introBaseline = JSON.stringify(introductionPayload());
  }
  function introductionPayload() {
    return {zh:{name:document.getElementById('introZhName').value.trim(),summary:document.getElementById('introZhSummary').value.trim(),description:lines('introZhDescription'),highlights:lines('introZhHighlights'),tips:lines('introZhTips')},
      en:{name:document.getElementById('introEnName').value.trim(),summary:document.getElementById('introEnSummary').value.trim(),description:lines('introEnDescription'),highlights:lines('introEnHighlights'),tips:lines('introEnTips')},
      difficulty:document.getElementById('introDifficulty').value.trim(),best_seasons:document.getElementById('introBestSeasons').value.split(',').map(function(s){return s.trim();}).filter(Boolean),
      estimated_duration_min:Number(document.getElementById('introDurationMin').value||0),estimated_duration_max:Number(document.getElementById('introDurationMax').value||0)};
  }
  function introDirty() {
    return !!introBaseline && JSON.stringify(introductionPayload()) !== introBaseline;
  }
  function showIntroError(message) {
    var node = document.getElementById('introWriteError');
    node.textContent = message;
    node.hidden = !message;
  }

  function setIntroGenerationBusy(busy) {
    var button = document.getElementById('introGenerateBtn');
    button.disabled = busy || !AdminAccess.can('route_groups', true);
    button.textContent = busy ? '正在生成…' : 'AI 生成草稿';
  }

  function cancelIntroGeneration() {
    if (!introAIController) return;
    introAIController.abort();
    introAIController = null;
    setIntroGenerationBusy(false);
  }

  async function generateIntroductionDraft() {
    if (!selectedGroupID || !introBaseline || detailLoadingID || introWriting || introAIController || !AdminAccess.can('route_groups', true)) return;
    var groupID = selectedGroupID, session = detailSession;
    var field = document.getElementById('introZhSummary');
    var previous = field.value;
    if (previous.trim() && (introStatus !== 'automatic' || previous.trim() !== JSON.parse(introBaseline).zh.summary)) {
      if (!await AdminUI.confirm({ title: '替换当前中文摘要？', message: 'AI 草稿会填入中文摘要，保存或发布前可以继续编辑。', confirmText: '生成草稿', waitForHidden: true })) return;
    }
    if (selectedGroupID !== groupID || session !== detailSession || introAIController || introWriting || previous !== field.value) return;
    var controller = introAIController = new AbortController();
    showIntroError('');
    setIntroGenerationBusy(true);
    try {
      var result = await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/introduction/generate', { method: 'POST', signal: controller.signal });
      if (introAIController !== controller) return;
      if (field.value !== previous) {
        showIntroError('生成期间摘要已修改，已保留当前内容。');
        return;
      }
      field.value = result.summary;
      field.focus({ preventScroll: true });
    } catch (err) {
      if (!controller.signal.aborted) showIntroError(err.message);
    } finally {
      if (introAIController === controller) {
        introAIController = null;
        setIntroGenerationBusy(false);
      }
    }
  }

  async function writeIntroduction(action) {
    if (!selectedGroupID || !introBaseline || detailLoadingID || introWriting || !AdminAccess.can('route_groups', true)) return;
    if (action === 'unpublish' && introDirty()) {
      showIntroError('路线介绍有未保存修改。保存草稿会同时下线，或撤销修改后再下线。');
      return;
    }
    var groupID = selectedGroupID, session = detailSession;
    if (action === 'draft' && introStatus === 'published') {
      if (!await AdminUI.confirm({ title: '下线并保存草稿？', message: '公开页面会改为展示自动简介。', confirmText: '下线并保存', waitForHidden: true })) return;
      if (groupID !== selectedGroupID || session !== detailSession || introWriting) return;
    }
    cancelIntroGeneration();
    var path = '/admin/api/route-groups/' + encodeURIComponent(groupID) + '/introduction';
    var options = { method: action === 'draft' ? 'PUT' : 'POST' };
    if (action !== 'unpublish') options.body = JSON.stringify(introductionPayload());
    showIntroError('');
    introWriting = introPanel.inert = true;
    try {
      var intro = await apiJSON(path + (action === 'draft' ? '' : '/' + action), options);
      renderIntroduction(intro, groupID);
    } catch (err) {
      showIntroError(err.message);
      return;
    } finally {
      introWriting = introPanel.inert = false;
    }
    detailModal.hide();
    AdminUI.alert(({ draft: '草稿已保存', publish: '路线介绍已发布', unpublish: '路线介绍已下线' })[action], 'success');
    await reloadKeepingPage();
  }

  function renderDetail(data) {
    var g = data.group || {};
    document.getElementById('detailGroupID').textContent = g.group_id || '';
    document.getElementById('detailName').textContent = g.name || '未命名路线';
    var cached = loadedGroups.find(function (group) { return group.group_id === g.group_id; });
    renderDetailPath(data.grouping ? { grouping: data.grouping } : cached);
    var meta = trackTypeMeta(g.track_type);
    document.getElementById('detailTrackType').textContent = meta ? meta.name : (g.track_type || '-');
    document.getElementById('detailCities').textContent = (data.coverage_city_names || []).join('、') || groupCities(g) || (data.area && data.area.city_name) || '—';
    document.getElementById('detailAreaID').textContent = g.area_id || '-';
    document.getElementById('detailAreaInfo').innerHTML = areaInfoHTML(data.area);
    document.getElementById('detailSource').textContent = g.source || '-';
    document.getElementById('detailMemberCount').textContent = String(g.member_count || 0);
    document.getElementById('detailRepresentative').textContent = g.representative_track_id || '';
    document.getElementById('renameInput').value = g.name || '';
    renderMembers(data.members || [], g.representative_track_id || '', Number(g.member_count || 0));
  }

  function renderDetailPath(group) {
    detailPath.hidden = !group || !group.grouping;
    if (detailPath.hidden) return;
    var path = routeGroupLocation(group);
    detailPath.textContent = [path.cityName, path.districtName, path.scenicName || '独立路线'].join(' / ');
  }

  function renderMembers(items, representativeTrackID, total) {
    memberList.innerHTML = '';
    var hint = document.getElementById('memberListHint');
    hint.textContent = items.length < total ? '已显示 ' + items.length + ' / ' + total + ' 条' : '';
    hint.hidden = !hint.textContent;
    if (!items.length) {
      memberList.innerHTML = '<tr class="admin-empty-row"><td colspan="8">暂无成员轨迹</td></tr>';
      return;
    }
    items.forEach(function (row) {
      var m = row.member || {};
      var geo = row.geo_index || {};
      var track = row.track || {};
      var isRepresentative = m.track_id === representativeTrackID;
      var title = String(track.title || '').trim() || '未命名轨迹';
      var distance = Number(geo.distance || track.distance);
      var distanceLabel = Number.isFinite(distance) && distance > 0
        ? (distance >= 1000 ? (distance / 1000).toFixed(1) + ' 公里' : Math.round(distance) + ' 米') : '—';
      var tr = document.createElement('tr');
      tr.innerHTML = '<td class="route-member-name"><strong title="' + esc(title) + '">' + esc(title) + '</strong>' +
        '<small><code>' + esc(m.track_id) + '</code>' + (isRepresentative ? ' <span class="route-member-role">代表轨迹</span>' : '') + '</small></td>' +
        '<td>' + distanceLabel + '</td>' +
        '<td class="route-member-diagnostic">' + esc(({ forward: '正向', reverse: '反向' })[m.match_direction] || m.match_direction || '—') + '</td>' +
        '<td class="route-member-diagnostic">' + (m.similarity_score != null ? Number(m.similarity_score).toFixed(3) : '—') + '</td>' +
        '<td class="route-member-diagnostic">' + esc(cityNames[geo.city_code] || geo.city_code || '—') + '</td>' +
        '<td class="route-member-diagnostic">' + esc(geo.point_count || '—') + '</td>' +
        '<td class="route-member-diagnostic">' + esc(m.source || '—') + '</td>' +
        '<td class="row-actions"></td>';
      var actions = tr.querySelector('.row-actions');
      actions.innerHTML = '<a class="admin-link route-member-view" data-admin-read="tracks" href="/admin/tracks.html?track_id=' + encodeURIComponent(m.track_id) + '" aria-label="查看轨迹 ' + esc(m.track_id) + '">查看</a>' +
        '<div class="dropdown" data-admin-write="route_groups">' +
        '<button type="button" class="route-member-menu" data-admin-menu aria-haspopup="menu" aria-expanded="false" aria-label="轨迹 ' + esc(m.track_id) + ' 的操作">⋯</button>' +
        '<div class="dropdown-menu dropdown-menu-end" role="menu">' +
        (isRepresentative ? '' : '<button type="button" class="dropdown-item" role="menuitem" data-route-representative>设为代表轨迹</button>') +
        '<button type="button" class="dropdown-item" role="menuitem" data-route-remove>从组内移除</button></div></div>';
      actions.querySelector('a').addEventListener('click', guardTrackLink);
      actions.querySelector('[data-route-representative]')?.addEventListener('click', function () { setRepresentative(m.track_id); });
      actions.querySelector('[data-route-remove]').addEventListener('click', function () { removeMember(m.track_id); });
      memberList.appendChild(tr);
    });
  }

  async function guardTrackLink(event) {
    if (!introDirty() || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    var url = event.currentTarget.href;
    var groupID = selectedGroupID, session = detailSession;
    var confirmed = await AdminUI.confirm({ title: '放弃未保存的路线介绍？', message: '前往轨迹详情前，请确认是否放弃当前修改。', confirmText: '放弃并查看', waitForHidden: true });
    if (confirmed && isCurrentDetail(groupID, session)) window.location.assign(url);
  }

  async function setRepresentative(trackID) {
    if (!selectedGroupID || !trackID || detailLoadingID || !AdminAccess.can('route_groups', true)) return;
    var groupID = selectedGroupID, session = detailSession;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/representative', {
        method: 'PUT',
        body: JSON.stringify({ track_id: trackID })
      });
      await refreshGroupAfterChange(groupID, session);
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    }
  }

  async function removeMember(trackID) {
    if (!selectedGroupID || !trackID || detailLoadingID || !AdminAccess.can('route_groups', true)) return;
    var groupID = selectedGroupID, session = detailSession;
    if (!await AdminUI.confirm({ title: '移除成员轨迹', message: '确认从当前路线组移除轨迹 ' + trackID + '？', confirmText: '确认移除', danger: true, waitForHidden: true })) return;
    if (!isCurrentDetail(groupID, session)) return;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/members/' + encodeURIComponent(trackID), {
        method: 'DELETE'
      });
      await refreshGroupAfterChange(groupID, session);
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    }
  }

  async function loadRegions() {
    try {
      var items = await apiJSON('/admin/api/cities');
      var provinceOptions = '';
      (items || []).forEach(function (item) {
        if (item.level === 'province') {
          provinceOptions += '<option value="' + esc(item.code) + '">' + esc(item.name) + '</option>';
          return;
        }
        if (item.level === 'city') {
          cityNames[item.code] = item.name;
          var provinceCode = item.province_code || '';
          if (!citiesByProvince[provinceCode]) citiesByProvince[provinceCode] = [];
          citiesByProvince[provinceCode].push({ code: item.code, name: item.name });
        }
      });
      provinceSelect.innerHTML = '<option value="">全部省份</option>' + provinceOptions;
      renderCityOptions('');
      AdminRegionPicker.enhance(provinceSelect, citySelect, items, { label: '覆盖地区' });
    } catch (_) { /* 省市目录不可用时保留“全部”，不阻塞路线组管理。 */ }
  }

  renderTypeToggle();
  window.AdminRouteGroups = { openDetail: loadDetail };
  resetAndLoad();
  loadRegions().then(function () { if (loadedGroups.length) renderGroups(); });
  var groupFromURL = new URLSearchParams(window.location.search).get('group_id');
  if (groupFromURL) loadDetail(groupFromURL);
})();
