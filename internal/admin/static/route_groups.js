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
  function apiJSON(url, options) {
    options = options || {};
    options.headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
    return fetch(url, options).then(async function (resp) {
      var body = await resp.json().catch(function () { return {}; });
      if (!resp.ok) {
        var error = new Error(body.error || ('HTTP ' + resp.status));
        error.status = resp.status;
        throw error;
      }
      return body.data;
    });
  }

  var ROUTES_PER_BRANCH = 20;
  var MEMBER_PAGE_SIZE = 5;
  var selectedGroupID = '', detailRequest = 0, detailSession = 0, detailController = null, detailLoadingID = '';
  var memberOffset = 0, memberTotal = 0, representativeWriting = false;
  var introBaseline = '', introWriting = false, introClosePrompt = false, allowIntroClose = false;
  var introStatus = '', introAIController = null, introLoadController = null;
  var selectedTrackType = '';
  var routeGroups = new Map(), areaTree = new Map();
  var listController = null, loadedQuery = '', directoryError = '';
  var areaExpansion = new Map();
  var visibleRouteCounts = new Map();
  var groupList = document.getElementById('groupList');
  var memberList = document.getElementById('memberList');
  var memberPager = document.getElementById('memberPager');
  var memberPageHint = document.getElementById('memberPageHint');
  var memberPrevBtn = document.getElementById('memberPrevBtn');
  var memberNextBtn = document.getElementById('memberNextBtn');
  var detailHint = document.getElementById('detailHint');
  var detailRetryBtn = document.getElementById('detailRetryBtn');
  var detailPanel = document.getElementById('detailPanel');
  var detailPath = document.getElementById('detailPath');
  var statusHint = document.getElementById('statusHint');
  var listSummaryHint = document.getElementById('listSummaryHint');
  var introPanel = document.getElementById('introSection');
  var introEditor = document.getElementById('introEditor');
  var introLoadState = document.getElementById('introLoadState');
  var introLoadHint = document.getElementById('introLoadHint');
  var introRetryBtn = document.getElementById('introRetryBtn');
  var cityNames = Object.create(null);
  var regionOptions;
  var detailModal = new AdminComponents.Dialog(document.getElementById('detailModal'));
  var scenicMembersModal = new AdminComponents.Dialog(document.getElementById('scenicMembersModal'));
  var scenicSelection = null, scenicController = null, scenicOffset = 0, scenicTotal = 0;
  var introFieldsModal = new AdminComponents.Dialog(document.getElementById('introFieldsModal'));
  document.getElementById('introAdvancedBtn').addEventListener('click', function () { introFieldsModal.show(); });
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
    resetIntroduction();
    selectedGroupID = '';
    detailSession++;
    detailRequest++;
    detailLoadingID = '';
    if (detailController) detailController.abort();
    renderGroupsFromSelection();
  });

  detailRetryBtn.addEventListener('click', function () { loadDetail(selectedGroupID); });
  introRetryBtn.addEventListener('click', loadIntroduction);
  memberPrevBtn.addEventListener('click', function () { loadDetail(selectedGroupID, memberOffset - MEMBER_PAGE_SIZE); });
  memberNextBtn.addEventListener('click', function () { loadDetail(selectedGroupID, memberOffset + MEMBER_PAGE_SIZE); });

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
        loadGroups();
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
    loadGroups();
  });
  citySelect.addEventListener('change', loadGroups);

  // ---------- 地区目录与分支内路线 ----------
  document.getElementById('refreshBtn').addEventListener('click', loadGroups);

  function listParams() {
    var params = new URLSearchParams({ view: 'directory' });
    if (selectedTrackType) params.set('track_type', selectedTrackType);
    if (citySelect.value) params.set('city_code', citySelect.value);
    else if (provinceSelect.value) params.set('province_code', provinceSelect.value);
    return params;
  }

  async function loadGroups() {
    if (listController) listController.abort();
    var controller = listController = new AbortController();
    var query = listParams().toString();
    if (!loadedQuery) {
      groupList.innerHTML = '<tr><td colspan="3"><div class="route-table-state">加载中…</div></td></tr>';
    }
    statusHint.textContent = loadedQuery ? '更新中…' : '';
    try {
      var data = await apiJSON('/admin/api/route-groups?' + query, { signal: controller.signal });
      if (listController !== controller) return;
      var nextGroups = new Map(data.items.map(function (group) { return [group.group_id, group]; }));
      if (data.total !== nextGroups.size) throw new Error('地区目录不完整，请刷新列表');
      var nextTree = buildAreaTree(nextGroups);
      if (loadedQuery !== query) {
        areaExpansion.clear();
        visibleRouteCounts.clear();
      }
      loadedQuery = query;
      directoryError = '';
      routeGroups = nextGroups;
      areaTree = nextTree;
      renderGroups();
      statusHint.textContent = '';
      window.dispatchEvent(new Event('admin:route-directory-updated'));
    } catch (err) {
      if (listController !== controller) return;
      if (!loadedQuery) {
        directoryError = err.message;
        groupList.innerHTML = '<tr><td colspan="3"><div class="route-table-state route-table-error">加载失败 · ' + esc(err.message) + '</div></td></tr>';
        window.dispatchEvent(new Event('admin:route-directory-updated'));
        return;
      }
      statusHint.textContent = loadedQuery === query ? '刷新失败 · 仍显示上次结果' : '筛选未更新 · 仍显示上次结果';
      AdminUI.alert(err.message, 'danger');
    } finally {
      if (listController === controller) listController = null;
    }
  }

  function routeGroupLocation(g) {
    var location = g.grouping || {};
    return {
      cityCode: location.city_code,
      cityName: location.city_name || cityNames[location.city_code] || location.city_code || '城市未标注',
      districtID: location.district_id,
      districtName: location.district_name || '区县未标注',
      scenicID: location.scenic_id,
      scenicSpotID: location.scenic_spot_id,
      scenicName: location.scenic_name || (location.scenic_id ? '未命名景点' : '')
    };
  }

  function buildAreaTree(items) {
    var root = { key: '', path: [], children: new Map() };
    function addGroup(parent, type, id, name) {
      var key = parent.key + '/' + type + ':' + encodeURIComponent(id || '');
      var bucket = parent.children.get(key);
      if (!bucket) {
        bucket = { key: key, id: id, type: type, name: name, path: parent.path.concat(name), children: new Map(), groups: [], scenicCount: 0, independentCount: 0, memberCount: 0 };
        parent.children.set(key, bucket);
      }
      return bucket;
    }
    items.forEach(function (g) {
      var location = routeGroupLocation(g);
      var city = addGroup(root, 'city', location.cityCode, location.cityName);
      var district = addGroup(city, 'district', location.districtID, location.districtName);
      var leaf = location.scenicID ? addGroup(district, 'scenic', location.scenicID, location.scenicName) : district;
      leaf.groups.push(g);
      if (location.scenicID) {
        leaf.location = location;
        leaf.memberCount += Number(g.member_count || 0);
      } else {
        district.independentCount++;
      }
    });
    root.children.forEach(function (city) {
      city.children.forEach(function (district) {
        district.scenicCount = district.children.size;
        city.scenicCount += district.scenicCount;
        city.independentCount += district.independentCount;
      });
    });
    return root.children;
  }

  function routeDirectoryCount(scenicCount, independentCount) {
    var labels = [];
    if (scenicCount) labels.push(scenicCount.toLocaleString() + ' 处景点');
    if (independentCount) labels.push(independentCount.toLocaleString() + ' 组独立路线');
    return labels.join(' · ') || '共 0 项';
  }

  function renderGroups(focusKey, focusGroupID) {
    var totals = Array.from(areaTree.values()).reduce(function (result, city) {
      result.scenic += city.scenicCount;
      result.independent += city.independentCount;
      return result;
    }, { scenic: 0, independent: 0 });
    listSummaryHint.textContent = routeDirectoryCount(totals.scenic, totals.independent);
    groupList.innerHTML = '';
    if (!routeGroups.size) {
      groupList.innerHTML = '<tr><td colspan="3"><div class="route-table-state">没有符合当前筛选的路线组</div></td></tr>';
      return;
    }
    var rows = document.createDocumentFragment();
    var focusTarget;
    var filters = new URLSearchParams(loadedQuery);
    var cityFilter = filters.get('city_code');
    var regionFiltered = cityFilter || filters.get('province_code');
    function appendBucket(bucket, level) {
      var expandedByFilter = level === 0 && regionFiltered && (bucket.id === cityFilter || areaTree.size === 1);
      var expanded = areaExpansion.has(bucket.key) ? areaExpansion.get(bucket.key) : !!expandedByFilter;
      var header = renderAreaHeader(bucket, level, expanded);
      rows.appendChild(header);
      if (bucket.key === focusKey) focusTarget = header.querySelector('.route-area-toggle');
      if (!expanded || bucket.type === 'scenic') return;
      Array.from(bucket.children.values()).sort(compareAreas).forEach(function (child) { appendBucket(child, level + 1); });
      if (bucket.type === 'district' && bucket.children.size && bucket.groups.length) {
        var divider = document.createElement('tr');
        divider.className = 'route-area-divider route-tree-depth-' + (level + 1);
        divider.innerHTML = '<td colspan="3" class="route-tree-cell">独立路线</td>';
        rows.appendChild(divider);
      }
      var visible = visibleRouteCounts.get(bucket.key) || ROUTES_PER_BRANCH;
      bucket.groups.slice(0, visible).forEach(function (g) {
        var row = renderGroupRow(g, level + 1);
        rows.appendChild(row);
        if (g.group_id === focusGroupID) focusTarget = row.querySelector('button');
      });
      if (bucket.groups.length > visible) rows.appendChild(renderBranchMore(bucket, level + 1, visible));
    }
    Array.from(areaTree.values()).sort(function (a, b) {
      return (cityFilter ? Number(b.id === cityFilter) - Number(a.id === cityFilter) : 0) || compareAreas(a, b);
    }).forEach(function (bucket) { appendBucket(bucket, 0); });
    groupList.appendChild(rows);
    if (focusTarget) focusTarget.focus({ preventScroll: true });
  }

  function compareAreas(a, b) {
    return Number(!a.id) - Number(!b.id) || a.name.localeCompare(b.name, 'zh-CN');
  }

  function renderAreaHeader(bucket, level, expanded) {
    var tr = document.createElement('tr');
    var scenic = bucket.type === 'scenic';
    tr.className = 'route-area-header route-area-level-' + bucket.type + ' route-tree-depth-' + level;
    tr.innerHTML = '<td colspan="3" class="route-tree-cell">' +
      '<button type="button" class="route-area-toggle" ' + (scenic ? 'aria-haspopup="dialog"' : 'aria-expanded="' + expanded + '"') + ' title="' + esc(bucket.name) + '">' +
      '<span class="' + (scenic ? 'route-scenic-dot' : 'route-area-chevron') + '" aria-hidden="true"></span>' +
      '<span class="route-area-title">' + esc(bucket.name) + '</span>' +
      '<span class="route-area-stats">' + (scenic ? bucket.memberCount.toLocaleString() + ' 条轨迹' : routeDirectoryCount(bucket.scenicCount, bucket.independentCount)) + '</span>' +
      '</button></td>';
    tr.querySelector('.route-area-toggle').addEventListener('click', function () {
      if (scenic) { openScenicMembers(bucket); return; }
      areaExpansion.set(bucket.key, !expanded);
      renderGroups(bucket.key);
    });
    return tr;
  }

  function renderBranchMore(bucket, level, visible) {
    var tr = document.createElement('tr');
    tr.className = 'route-branch-more route-tree-depth-' + level;
    tr.innerHTML = '<td colspan="3" class="route-tree-cell"><button type="button" class="btn btn-sm btn-outline-secondary">显示更多路线</button></td>';
    var button = tr.querySelector('button');
    button.setAttribute('aria-label', bucket.name + '：显示更多路线');
    button.addEventListener('click', function () {
      visibleRouteCounts.set(bucket.key, visible + ROUTES_PER_BRANCH);
      renderGroups('', bucket.groups[visible].group_id);
    });
    return tr;
  }

  function renderGroupRow(g, level) {
    var tr = document.createElement('tr');
    tr.className = 'route-group-row route-tree-depth-' + level;
    tr.dataset.groupId = g.group_id;
    if (g.group_id === selectedGroupID) tr.classList.add('selected');
    tr.innerHTML = '<td class="route-tree-cell route-name-cell"><button type="button" class="route-group-open" aria-haspopup="dialog"><strong title="' + esc(g.name || '') + '">' + esc(g.name || '未命名路线') + '</strong><code>' + esc(g.group_id) + '</code></button></td>' +
      '<td>' + typeBadgeHTML(g.track_type) + '</td>' +
      '<td class="route-member-count">' + Number(g.member_count || 0).toLocaleString() + '</td>';
    tr.addEventListener('click', function () {
      tr.querySelector('button').focus({ preventScroll: true });
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

  async function loadDetail(groupID, requestedOffset) {
    if (!groupID || introWriting || (requestedOffset != null && detailLoadingID === groupID)) return;
    var opening = selectedGroupID !== groupID || !detailModal.root.open;
    if (opening) {
      detailSession++;
      memberList.replaceChildren();
      memberOffset = memberTotal = 0;
      resetIntroduction();
      document.getElementById('mergeInput').value = '';
      detailPanel.hidden = true;
      var cached = routeGroups.get(groupID);
      document.getElementById('detailName').textContent = cached && cached.name || '路线组';
      document.getElementById('detailTrackType').textContent = '';
      renderDetailPath(cached);
    }
    var paging = !opening && requestedOffset != null;
    var offset = paging ? requestedOffset : memberOffset;
    var pageError = '';
    if (detailController) detailController.abort();
    var controller = detailController = new AbortController(), request = ++detailRequest;
    detailLoadingID = groupID;
    selectedGroupID = groupID;
    detailModal.show();
    renderGroupsFromSelection();
    if (!paging) showDetailState('加载中…', false);
    updateMemberPager(true);
    try {
      var url = '/admin/api/route-groups/' + encodeURIComponent(groupID) + '?limit=' + MEMBER_PAGE_SIZE + '&offset=' + offset;
      var data = await apiJSON(url, { signal: controller.signal });
      if (request !== detailRequest || controller.signal.aborted) return;
      if (!paging) renderDetail(data);
      var group = data.group || {};
      memberOffset = Number(data.member_offset || 0);
      memberTotal = Number(group.member_count || 0);
      document.getElementById('detailMemberCount').textContent = memberTotal.toLocaleString() + ' 条';
      renderMembers(data.members || [], group.representative_track_id);
      detailPanel.hidden = false;
      showDetailState('', false);
      if (!paging && introLoadState.hidden) loadIntroduction();
    } catch (err) {
      if (request === detailRequest && !controller.signal.aborted) {
        if (err.status === 404) {
          detailPanel.hidden = true;
          showDetailState(err.message, true);
        } else if (paging) {
          pageError = '翻页失败，可重试';
        } else {
          showDetailState('详情加载失败 · ' + err.message, true);
        }
      }
    } finally {
      if (request === detailRequest) {
        detailLoadingID = '';
        updateMemberPager(false, pageError);
      }
    }
  }

  function updateMemberPager(busy, message) {
    memberPager.hidden = memberTotal <= MEMBER_PAGE_SIZE;
    memberPrevBtn.disabled = busy || memberOffset === 0;
    memberNextBtn.disabled = busy || memberOffset + MEMBER_PAGE_SIZE >= memberTotal;
    memberPageHint.textContent = message || (busy ? '加载中…' : '第 ' + (Math.floor(memberOffset / MEMBER_PAGE_SIZE) + 1) + ' / ' + Math.max(1, Math.ceil(memberTotal / MEMBER_PAGE_SIZE)) + ' 页');
    memberList.setAttribute('aria-busy', String(busy));
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
    await loadGroups();
    window.dispatchEvent(new Event('admin:route-groups-changed'));
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
    introFieldsModal.hide();
    document.getElementById('introStatus').textContent = '';
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
    button.textContent = busy ? '正在生成…' : 'AI 草稿';
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
    await loadGroups();
  }

  function renderDetail(data) {
    var g = data.group || {};
    document.getElementById('detailGroupID').textContent = g.group_id || '';
    document.getElementById('detailName').textContent = g.name || '未命名路线';
    var cached = routeGroups.get(g.group_id);
    renderDetailPath(data.grouping ? { grouping: data.grouping } : cached);
    var meta = trackTypeMeta(g.track_type);
    document.getElementById('detailTrackType').textContent = meta ? meta.name : (g.track_type || '-');
    var areaLink = document.getElementById('detailAreaLink');
    var url = data.area && data.area.introduction_url || '';
    areaLink.hidden = !url.startsWith('/api/v1/track-map/areas/');
    if (!areaLink.hidden) areaLink.href = url;
    document.getElementById('renameInput').value = g.name || '';
  }

  function renderDetailPath(group) {
    detailPath.hidden = !group || !group.grouping;
    if (detailPath.hidden) return;
    var path = routeGroupLocation(group);
    detailPath.textContent = [path.cityName, path.districtName, path.scenicName || '独立路线'].join(' / ');
  }

  function memberTrackItem(track, trackID, distance, extraMetrics, representative, onOpen) {
    var title = String(track.title || '').trim() || '未命名轨迹';
    var distanceLabel = Number.isFinite(distance) && distance > 0
      ? (distance >= 1000 ? (distance / 1000).toFixed(1) + ' 公里' : Math.round(distance) + ' 米') : '—';
    var preview = track.preview_images || {};
    var variant = onOpen && ['landscape_day', 'square_day', 'landscape_night', 'square_night'].find(function (key) {
      return track.preview_variant === key || preview[key + '_url'];
    });
    var previewURL = variant ? '/admin/api/tracks/' + encodeURIComponent(trackID) + '/assets/' + variant : '';
    var item = document.createElement('li');
    item.className = 'route-member-row' + (onOpen ? ' route-member-clickable' : '');
    item.dataset.trackId = trackID;
    var content = '<span class="route-member-thumb" data-no-preview aria-hidden="true">' +
      (previewURL ? '<img alt="" loading="lazy" src="' + esc(previewURL) + '">' : '') +
      '<span' + (previewURL ? ' hidden' : '') + '>—</span></span>' +
      '<span class="route-member-name"><strong title="' + esc(title) + '">' + esc(title) + '</strong>' +
      '<small><code>' + esc(trackID) + '</code>' + (representative ? ' <span class="route-member-role">代表轨迹</span>' : '') + '</small></span>' +
      '<span class="route-member-metrics"><span>' + distanceLabel + '</span>' + extraMetrics + '</span>';
    item.innerHTML = onOpen ? '<button type="button" class="route-member-summary route-member-open" aria-haspopup="dialog">' + content + '</button>' : '<div class="route-member-summary">' + content + '</div>';
    var image = item.querySelector('img');
    image?.addEventListener('error', function () {
      image.nextElementSibling.hidden = false;
      image.remove();
    });
    if (onOpen) item.addEventListener('click', function (event) {
      if (!event.target.closest('.route-member-actions')) onOpen(trackID, item.querySelector('.route-member-open'));
    });
    return item;
  }

  function renderMembers(items, representativeTrackID) {
    memberList.replaceChildren();
    if (!items.length) {
      memberList.innerHTML = '<li class="route-member-empty">暂无成员轨迹</li>';
      return;
    }
    var canViewTrack = AdminAccess.can('tracks', false);
    items.forEach(function (row) {
      var m = row.member || {};
      var geo = row.geo_index || {};
      var track = row.track || {};
      var trackID = m.track_id;
      var isRepresentative = trackID === representativeTrackID;
      var score = m.similarity_score;
      var similarity = Number.isFinite(score) ? (score * 100).toFixed(1) + '%' : '—';
      var item = memberTrackItem(track, trackID, Number(geo.distance || track.distance), '<span>相似度 ' + similarity + '</span>', isRepresentative, canViewTrack && openMemberTrack);
      item.insertAdjacentHTML('beforeend',
        '<div class="route-member-actions" data-admin-write="route_groups">' +
        (isRepresentative ? '' : '<button type="button" class="btn btn-sm btn-outline-secondary" data-route-representative>设为代表</button>') +
        '<div class="dropdown"><button type="button" class="route-member-menu" data-admin-menu aria-haspopup="menu" aria-expanded="false" aria-label="轨迹 ' + esc(trackID) + ' 的操作">⋯</button>' +
        '<div class="dropdown-menu dropdown-menu-end" role="menu"><button type="button" class="dropdown-item" role="menuitem" data-route-remove>从组内移除</button></div></div></div>');
      item.querySelector('[data-route-representative]')?.addEventListener('click', function (event) { setRepresentative(trackID, event.currentTarget); });
      item.querySelector('[data-route-remove]').addEventListener('click', function () { removeMember(trackID); });
      memberList.appendChild(item);
    });
  }

  var scenicTracksList = document.getElementById('scenicTracksList');
  var scenicTracksState = document.getElementById('scenicTracksState');
  var scenicTracksHint = document.getElementById('scenicTracksHint');
  var scenicTracksRetry = document.getElementById('scenicTracksRetry');
  var scenicTracksPager = document.getElementById('scenicTracksPager');
  var scenicTracksPrev = document.getElementById('scenicTracksPrev');
  var scenicTracksNext = document.getElementById('scenicTracksNext');

  function openScenicMembers(bucket) {
    if (!bucket || bucket.type !== 'scenic') return;
    scenicSelection = { location: bucket.location, query: loadedQuery };
    scenicOffset = scenicTotal = 0;
    scenicTracksList.replaceChildren();
    scenicTracksPager.hidden = true;
    document.getElementById('scenicTracksCount').textContent = '';
    document.getElementById('scenicMembersName').textContent = bucket.name;
    document.getElementById('scenicMembersPath').textContent = bucket.path.slice(0, -1).join(' / ');
    var infoLink = document.getElementById('scenicMembersInfo');
    var spotID = scenicSelection.location.scenicSpotID;
    infoLink.hidden = !spotID;
    if (spotID) infoLink.href = '/admin/scenic_spot_detail.html?spot_id=' + encodeURIComponent(spotID);
    else infoLink.removeAttribute('href');
    scenicMembersModal.show();
    loadScenicMembers(0);
  }

  async function loadScenicMembers(offset) {
    if (!scenicSelection || !scenicMembersModal.root.open) return;
    if (scenicController) scenicController.abort();
    var controller = scenicController = new AbortController();
    var location = scenicSelection.location;
    var params = new URLSearchParams(scenicSelection.query);
    params.delete('view');
    params.set('scenic_id', location.scenicID);
    params.set('grouping_city_code', location.cityCode || '');
    params.set('district_id', location.districtID || '');
    params.set('limit', String(MEMBER_PAGE_SIZE));
    params.set('offset', String(Math.max(0, offset)));
    scenicTracksState.hidden = false;
    scenicTracksHint.textContent = '正在加载景点轨迹…';
    scenicTracksRetry.hidden = true;
    scenicTracksPrev.disabled = scenicTracksNext.disabled = true;
    try {
      var data = await apiJSON('/admin/api/route-groups/scenic-members?' + params.toString(), { signal: controller.signal });
      if (scenicController !== controller) return;
      scenicOffset = Number(data.member_offset || 0);
      scenicTotal = Number(data.total || 0);
      renderScenicMembers(data.items || []);
      scenicTracksState.hidden = true;
    } catch (err) {
      if (scenicController !== controller) return;
      if (err.status === 404) {
        scenicOffset = scenicTotal = 0;
        renderScenicMembers([]);
        scenicTracksState.hidden = true;
      } else {
        scenicTracksHint.textContent = '景点轨迹加载失败 · ' + err.message;
        scenicTracksRetry.hidden = false;
        scenicTracksRetry.onclick = function () { loadScenicMembers(offset); };
      }
    } finally {
      if (scenicController === controller) {
        scenicController = null;
        scenicTracksPager.hidden = scenicTotal <= MEMBER_PAGE_SIZE;
        scenicTracksPrev.disabled = scenicOffset === 0;
        scenicTracksNext.disabled = scenicOffset + MEMBER_PAGE_SIZE >= scenicTotal;
        document.getElementById('scenicTracksPage').textContent = '第 ' + (Math.floor(scenicOffset / MEMBER_PAGE_SIZE) + 1) + ' / ' + Math.max(1, Math.ceil(scenicTotal / MEMBER_PAGE_SIZE)) + ' 页';
      }
    }
  }

  function renderScenicMembers(tracks) {
    scenicTracksList.replaceChildren();
    document.getElementById('scenicTracksCount').textContent = scenicTotal.toLocaleString() + ' 条';
    if (!tracks.length) {
      scenicTracksList.innerHTML = '<li class="route-member-empty">暂无可显示的轨迹</li>';
      return;
    }
    var canViewTrack = AdminAccess.can('tracks', false);
    tracks.forEach(function (track) {
      var item = memberTrackItem(track, track.track_id, Number(track.distance), typeBadgeHTML(track.track_type), false, canViewTrack && openScenicTrack);
      item.insertAdjacentHTML('beforeend', '<div class="route-member-actions"><div class="dropdown">' +
        '<button type="button" class="route-member-menu" data-admin-menu aria-haspopup="menu" aria-expanded="false" aria-label="轨迹 ' + esc(track.track_id) + ' 的操作">⋯</button>' +
        '<div class="dropdown-menu dropdown-menu-end" role="menu"><button type="button" class="dropdown-item" role="menuitem" data-scenic-group>维护所属路线组</button></div></div></div>');
      item.querySelector('[data-scenic-group]').addEventListener('click', function () { loadDetail(track.group_id); });
      scenicTracksList.appendChild(item);
    });
  }

  async function openScenicTrack(trackID, trigger) {
    if (!AdminAccess.can('tracks', false) || !scenicMembersModal.root.open || scenicController) return;
    trigger.focus({ preventScroll: true });
    try {
      await window.AdminTrackEditor.open(trackID, { cities: regionOptions, onSaved: refreshDirectoryAfterEdit, onDeleted: refreshDirectoryAfterEdit });
    } catch (err) {
      AdminUI.alert('打开轨迹失败 · ' + err.message, 'danger');
    }
  }

  async function refreshDirectoryAfterEdit() {
    await loadGroups();
    window.dispatchEvent(new Event('admin:route-groups-changed'));
  }

  scenicTracksPrev.addEventListener('click', function () { loadScenicMembers(scenicOffset - MEMBER_PAGE_SIZE); });
  scenicTracksNext.addEventListener('click', function () { loadScenicMembers(scenicOffset + MEMBER_PAGE_SIZE); });
  scenicMembersModal.root.addEventListener('admin:dialog-closed', function () {
    if (scenicController) scenicController.abort();
    scenicController = null;
    scenicSelection = null;
  });
  window.addEventListener('admin:route-directory-updated', function () {
    if (scenicSelection && scenicMembersModal.root.open) loadScenicMembers(scenicOffset);
  });

  async function openMemberTrack(trackID, trigger) {
    if (!trackID || !selectedGroupID || !detailModal.root.open || detailLoadingID || introWriting || !AdminAccess.can('tracks', false)) return;
    var groupID = selectedGroupID, session = detailSession;
    trigger.focus({ preventScroll: true });
    var refresh = function () { return refreshGroupAfterChange(groupID, session); };
    try {
      await window.AdminTrackEditor.open(trackID, { cities: regionOptions, onSaved: refresh, onDeleted: refresh });
    } catch (err) {
      AdminUI.alert('打开轨迹失败 · ' + err.message, 'danger');
    } finally {
      if (isCurrentDetail(groupID, session) && !trigger.isConnected) {
        var item = Array.from(memberList.children).find(function (row) { return row.dataset.trackId === trackID; });
        var focus = item ? item.querySelector('.route-member-open') : detailModal.root.querySelector('.btn-close');
        focus?.focus({ preventScroll: true });
      }
    }
  }

  async function setRepresentative(trackID, button) {
    if (!selectedGroupID || !trackID || detailLoadingID || representativeWriting || !AdminAccess.can('route_groups', true)) return;
    var groupID = selectedGroupID, session = detailSession;
    representativeWriting = button.disabled = true;
    try {
      await apiJSON('/admin/api/route-groups/' + encodeURIComponent(groupID) + '/representative', {
        method: 'PUT',
        body: JSON.stringify({ track_id: trackID })
      });
      await refreshGroupAfterChange(groupID, session);
      AdminUI.alert('已设为代表轨迹', 'success');
    } catch (err) {
      AdminUI.alert(err.message, 'danger');
    } finally {
      representativeWriting = button.disabled = false;
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
      regionOptions = (items || []).filter(function (item) { return item.level === 'city'; });
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
  window.AdminRouteGroups = {
    openDetail: loadDetail,
    openScenic: openScenicMembers,
    refreshDirectory: loadGroups,
    directory: function () { return { tree: areaTree, query: loadedQuery, ready: Boolean(loadedQuery), error: directoryError }; }
  };
  loadGroups();
  loadRegions().then(function () {
    if (!routeGroups.size) return;
    areaTree = buildAreaTree(routeGroups);
    renderGroups();
    window.dispatchEvent(new Event('admin:route-directory-updated'));
  });
  var groupFromURL = new URLSearchParams(window.location.search).get('group_id');
  if (groupFromURL) loadDetail(groupFromURL);
})();
