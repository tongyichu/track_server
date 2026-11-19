(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  let catalog = null;
  let selectedId = '';
  let activeCountryId = '';
  const dirtyIds = new Set();
  let persistedIds = new Set();
  let persistedNodes = new Map();
  let expandedIds = new Set();
  const nodesById = new Map();
  const childrenByParent = new Map();
  const treeButtons = new Map();
  const treeMissingElements = new Map();
  const treeDraftElements = new Map();
  const saveNotes = new Map();
  const conflicts = new Map();
  let operationMessage = '';
  let conflictReview = null;
  let latestController = null;
  let loadingCatalog = false;
  let treeNeedsRender = true;
  let saving = false;
  const photoEditor = new RegionPhotosEditor($('regionPhotos'), {
    editable,
    locked: () => saving,
    saved: node => persistedIds.has(node.id),
    savePhotos: (node, photos, base, signal) => api(encodeURIComponent(node.id) + '/photos', {
      method: 'PUT', headers: { 'Content-Type': 'application/json' }, signal,
      body: JSON.stringify({ photos, base_photos: base })
    }),
    photosSaved: (node, result) => {
      const photos = structuredClone(result.node.photos || []);
      node.photos = photos;
      const currentNode = nodesById.get(node.id);
      if (currentNode) currentNode.photos = structuredClone(photos);
      const base = persistedNodes.get(node.id);
      if (base) base.photos = structuredClone(photos);
      catalog.version = Math.max(catalog.version, result.version);
      updateTreeMissing(node);
      if (node.id === selectedId) showMissingSummary(node);
    },
    busyChanged: updateSaveState
  });
  const fieldMap = {
    regionName: 'name', regionNameEn: 'name_en',
    regionCountryCode: 'country_code', regionContinent: 'continent',
    regionSlogan: 'slogan', regionDescription: 'description'
  };
  const editableFields = [
    ['name', '名称'], ['name_en', '英文名称'], ['country_code', '国家缩写'], ['continent', '所属洲'],
    ['slogan', '宣传口号'], ['description', '简介'], ['latitude', '纬度'], ['longitude', '经度']
  ].map(([key, label]) => ({ key, label, value: node => node?.[key] ?? '' }));
  editableFields.push(
    { label: '高德 adcode', provider: 'amap', type: 'adcode', value: node => node ? amapValue(node) : '' },
    { label: 'Google 地区 Place ID', provider: 'google', type: 'place_id', value: node => node ? externalValue(node, 'google', 'place_id') : '' }
  );

  function externalValue(node, provider, type) {
    return (node.external_ids || []).find(item => item.provider === provider && item.type === type)?.value || '';
  }

  function amapValue(node) {
    return externalValue(node, 'amap', 'adcode') || (node.code_system === 'cn_adcode' ? node.code || '' : '');
  }

  function setExternalValue(node, provider, type, value) {
    node.external_ids = (node.external_ids || []).filter(item => item.provider !== provider || item.type !== type);
    if (value && !(provider === 'amap' && type === 'adcode' && node.code_system === 'cn_adcode' && node.code === value)) {
      node.external_ids.push({ provider, type, value });
    }
  }

  function status(message, error) {
    $('regionStatus').textContent = message;
    $('regionStatus').className = error ? 'text-danger' : 'text-secondary';
  }

  function updateSaveState() {
    const node = current(), busy = saving || photoEditor.busy;
    const conflict = conflicts.get(selectedId), note = saveNotes.get(selectedId);
    $('regionSaveNode').disabled = !editable() || !node?.active || busy || !!conflict || !dirtyIds.has(selectedId);
    const canDelete = editable() && canDeleteCountry(node);
    const canDiscard = editable() && node && !persistedIds.has(node.id);
    $('regionDeleteCountry').hidden = !canDelete;
    $('regionDeleteCountry').disabled = busy || !!conflict;
    $('regionDeleteDraft').hidden = !canDiscard;
    $('regionDeleteDraft').disabled = busy;
    $('regionDangerZone').hidden = !canDelete && !canDiscard;
    $('regionAddChild').disabled = !editable() || busy || !node?.active || !persistedIds.has(selectedId);
    $('regionAddCountry').disabled = !editable() || !catalog || busy;
    $('regionViewLatest').hidden = !conflict;
    $('regionViewLatest').disabled = busy || !!latestController;
    const feedback = $('regionSaveStatus');
    feedback.classList.toggle('text-danger', !busy && !!(conflict || note?.error));
    feedback.textContent = saving ? operationMessage : photoEditor.busy ? '宣传图片处理中…' :
      conflict || note?.message || (dirtyIds.has(selectedId) ? '有未保存修改' : '');
    if (!busy) $('regionNavigationStatus').textContent = '';
    photoEditor.sessions.get(selectedId)?.picker.refresh();
  }

  function setSaveNote(id, message, error = false) {
    saveNotes.set(id, { message, error });
    updateSaveState();
  }

  function refreshDraftIndicators() {
    const count = dirtyIds.size;
    $('regionDraftCount').textContent = count ? count + ' 个地区有未保存修改' : '';
    const dirtyCountries = new Set();
    for (const id of dirtyIds) {
      const country = countryOf(nodesById.get(id));
      if (country) dirtyCountries.add(country.id);
    }
    $('regionCountryTabs').querySelectorAll('[data-draft-country]').forEach(marker => {
      marker.hidden = !dirtyCountries.has(marker.dataset.draftCountry);
    });
  }

  function fieldChanged(field, one, another) {
    return field.value(one) !== field.value(another);
  }

  function markDirty(node) {
    const base = persistedNodes.get(node.id);
    if (!base || editableFields.some(field => fieldChanged(field, node, base))) dirtyIds.add(node.id);
    else dirtyIds.delete(node.id);
    saveNotes.delete(node.id);
    const marker = treeDraftElements.get(node.id);
    if (marker) marker.hidden = !dirtyIds.has(node.id);
    refreshDraftIndicators();
    updateSaveState();
  }

  async function api(path, options) {
    const response = await fetch('/admin/api/regions/' + path, options);
    let body;
    try { body = await response.json(); } catch (_) { throw new Error('服务器返回无效内容'); }
    if (!response.ok || body.code !== 0) {
      const error = new Error(body.error || '操作失败');
      error.status = response.status;
      throw error;
    }
    return body.data;
  }

  function current() { return nodesById.get(selectedId); }
  function canDeleteCountry(node) {
    const saved = node && persistedNodes.get(node.id);
    return !!saved && saved.level === 'country' && !saved.parent_id && saved.active &&
      saved.id !== 'country:CN' && saved.region_id !== 'CN' && (saved.country_code || '').toUpperCase() !== 'CN';
  }
  function firstCountryId() {
    return children('').find(node => node.level === 'country' && node.active)?.id || '';
  }
  const nameOrder = new Intl.Collator('zh-CN');
  function sortChildren(parentId) {
    childrenByParent.get(parentId)?.sort((a, b) =>
      (a.sort_order || 0) - (b.sort_order || 0) || nameOrder.compare(a.name, b.name));
  }
  function indexCatalog() {
    treeNeedsRender = true;
    nodesById.clear();
    childrenByParent.clear();
    catalog.nodes.forEach(node => {
      nodesById.set(node.id, node);
      if (!childrenByParent.has(node.parent_id)) childrenByParent.set(node.parent_id, []);
      childrenByParent.get(node.parent_id).push(node);
    });
    childrenByParent.forEach((_, parentId) => sortChildren(parentId));
  }
  function children(parentId) { return childrenByParent.get(parentId) || []; }
  function ancestors(id) {
    const result = new Set();
    let node = nodesById.get(id);
    while (node && node.parent_id) {
      if (result.has(node.parent_id)) break;
      result.add(node.parent_id);
      node = nodesById.get(node.parent_id);
    }
    return result;
  }

  function countryOf(node) {
    const seen = new Set();
    while (node && !seen.has(node.id)) {
      if (!node.parent_id) return node.level === 'country' ? node : null;
      seen.add(node.id);
      node = nodesById.get(node.parent_id);
    }
    return null;
  }

  function renderCountryTabs() {
    const root = $('regionCountryTabs');
    root.replaceChildren();
    if (!catalog) return;
    children('').filter(node => node.level === 'country' && node.active).forEach(node => {
      const tab = document.createElement('button');
      tab.type = 'button';
      tab.className = 'region-country-tab' + (node.id === activeCountryId ? ' active' : '');
      tab.setAttribute('role', 'tab');
      tab.setAttribute('aria-selected', node.id === activeCountryId ? 'true' : 'false');
      tab.setAttribute('aria-controls', 'regionTree');
      const title = document.createElement('span');
      title.textContent = node.name;
      const code = document.createElement('small');
      code.textContent = node.country_code || node.code || '';
      const draft = document.createElement('small');
      draft.dataset.draftCountry = node.id;
      draft.textContent = '有草稿';
      draft.hidden = true;
      tab.append(title, code, draft);
      tab.addEventListener('click', () => {
        if (select(node.id, {clearSearch: true})) root.querySelector('[aria-selected="true"]')?.focus();
      });
      root.appendChild(tab);
    });
  }

  function missingFields(saved) {
    if (!saved) return [];
    const missing = [];
    if (!saved.slogan?.trim()) missing.push('口号');
    if (!saved.description?.trim()) missing.push('简介');
    if (['province', 'city'].includes(saved.level) &&
        !(saved.photos || []).some(photo => photo.url?.trim())) missing.push('宣传图');
    return missing;
  }

  function updateTreeMissing(node) {
    const indicator = treeMissingElements.get(node.id);
    if (!indicator) return;
    const missing = missingFields(persistedNodes.get(node.id));
    indicator.textContent = missing.length ? '缺 ' + missing.length + ' 项' : '';
    indicator.title = missing.length ? '缺少' + missing.join('、') : '';
  }

  function showMissingSummary(node) {
    const missing = missingFields(persistedNodes.get(node.id));
    const summary = $('regionMissing');
    summary.hidden = !missing.length;
    summary.textContent = missing.length ? '已保存资料缺少' + missing.join('、') : '';
  }

  function renderTree() {
    treeNeedsRender = false;
    const root = $('regionTree');
    root.replaceChildren();
    treeMissingElements.clear();
    treeDraftElements.clear();
    treeButtons.clear();
    if (!catalog || !activeCountryId) return;
    const query = $('regionSearch').value.trim().toLowerCase();
    const visible = new Set();
    if (query) catalog.nodes.forEach(node => {
      if (countryOf(node)?.id === activeCountryId &&
          searchText(node).includes(query)) {
        visible.add(node.id);
        ancestors(node.id).forEach(id => visible.add(id));
      }
    });
    function add(parentId, list) {
      children(parentId).forEach(node => {
        if (query && !visible.has(node.id)) return;
        const childNodes = children(node.id).filter(item => !query || visible.has(item.id));
        const expanded = childNodes.length > 0 && (query ? true : expandedIds.has(node.id));
        const item = document.createElement('li');
        item.className = 'region-tree-node' + (!node.active ? ' archived' : '');
        item.setAttribute('role', 'none');
        const row = document.createElement('div');
        row.className = 'region-tree-row';
        if (childNodes.length) {
          const toggle = document.createElement('button');
          toggle.type = 'button';
          toggle.className = 'region-tree-toggle';
          toggle.textContent = expanded ? '−' : '+';
          toggle.setAttribute('aria-label', (expanded ? '折叠' : '展开') + node.name);
          toggle.setAttribute('aria-expanded', expanded ? 'true' : 'false');
          toggle.disabled = !!query;
          toggle.addEventListener('click', () => {
            if (expandedIds.has(node.id)) expandedIds.delete(node.id);
            else expandedIds.add(node.id);
            renderTree();
          });
          row.appendChild(toggle);
        } else {
          const spacer = document.createElement('span');
          spacer.className = 'region-tree-toggle-spacer';
          row.appendChild(spacer);
        }
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'region-tree-item' + (node.id === selectedId ? ' selected' : '');
        button.setAttribute('role', 'treeitem');
        button.setAttribute('aria-selected', node.id === selectedId ? 'true' : 'false');
        treeButtons.set(node.id, button);
        if (childNodes.length) button.setAttribute('aria-expanded', expanded ? 'true' : 'false');
        const label = document.createElement('span');
        label.className = 'region-tree-name';
        label.textContent = node.name;
        label.title = node.name;
        button.appendChild(label);
        const missing = document.createElement('span');
        missing.className = 'region-tree-missing';
        button.appendChild(missing);
        treeMissingElements.set(node.id, missing);
        updateTreeMissing(node);
        const draft = document.createElement('span');
        draft.className = 'region-tree-draft';
        draft.textContent = '未保存';
        draft.hidden = !dirtyIds.has(node.id);
        treeDraftElements.set(node.id, draft);
        button.appendChild(draft);
        button.addEventListener('click', () => select(node.id, {revealEditor: true}));
        row.appendChild(button);
        item.appendChild(row);
        if (expanded) {
          const nested = document.createElement('ul');
          nested.className = 'region-tree-children';
          nested.setAttribute('role', 'group');
          add(node.id, nested);
          item.appendChild(nested);
        }
        list.appendChild(item);
      });
    }
    const tree = document.createElement('ul');
    tree.className = 'region-tree-list';
    add(activeCountryId, tree);
    if (tree.childElementCount) root.appendChild(tree);
    else root.textContent = query ? '没有匹配的地区' : '当前国家还没有下级地区';
  }

  function editable() { return !!(window.AdminAccess && AdminAccess.can('regions', true)); }

  function updateEditorIdentity(node) {
    $('regionEditorTitle').textContent = node.name || '新增地区';
    const parents = Array.from(ancestors(node.id)).reverse().map(id => nodesById.get(id)?.name).filter(Boolean);
    $('regionPath').textContent = parents.length ? parents.concat(node.name || '新增地区').join(' / ') : '';
  }

  function revealEditor() {
    if (!window.matchMedia('(max-width: 900px)').matches) return;
    $('regionEditor').scrollIntoView({ block: 'start' });
    $('regionEditorTitle').focus({ preventScroll: true });
  }

  function select(id, options = {}) {
    if (saving || photoEditor.busy) {
      $('regionNavigationStatus').textContent = saving ? '请等当前操作完成后再切换地区' : '请等宣传图片处理完成后再切换地区';
      return false;
    }
    if (id !== selectedId) {
      latestController?.abort();
      $('regionLocation').open = false;
    }
    if (options.clearSearch) {
      treeNeedsRender = treeNeedsRender || !!$('regionSearch').value;
      $('regionSearch').value = '';
    }
    const previousId = selectedId;
    const previousCountryId = activeCountryId;
    let treeChanged = false;
    selectedId = id;
    const node = current();
    if (node) {
      const country = countryOf(node);
      if (country) activeCountryId = country.id;
      ancestors(node.id).forEach(parentId => {
        if (parentId !== activeCountryId && !expandedIds.has(parentId)) {
          expandedIds.add(parentId);
          treeChanged = true;
        }
      });
    } else activeCountryId = '';
    renderCountryTabs();
    refreshDraftIndicators();
    $('regionEmpty').hidden = !!node;
    $('regionForm').hidden = !node;
    photoEditor.bind(node);
    if (!node) {
      renderTree();
      updateSaveState();
      return true;
    }
    updateEditorIdentity(node);
    $('regionId').textContent = node.region_id || '';
    Object.entries(fieldMap).forEach(([id, key]) => { $(id).value = node[key] || ''; });
    $('regionAmapAdcode').value = amapValue(node);
    $('regionGooglePlaceId').value = externalValue(node, 'google', 'place_id');
    const countryRoot = node.level === 'country' && !node.parent_id;
    const showCountryCode = countryRoot && !persistedIds.has(node.id);
    $('regionCountryCodeField').hidden = !showCountryCode;
    $('regionCountryCode').required = showCountryCode && node.active;
    $('regionCountryCode').disabled = !showCountryCode;
    $('regionContinentField').hidden = node.level !== 'country';
    $('regionLatitude').value = node.latitude == null ? '' : String(node.latitude);
    $('regionLongitude').value = node.longitude == null ? '' : String(node.longitude);
    showMissingSummary(node);
    updateSaveState();
    if (treeNeedsRender || treeChanged || previousCountryId !== activeCountryId) renderTree();
    else {
      [previousId, selectedId].forEach(nodeId => {
        const button = treeButtons.get(nodeId);
        if (!button) return;
        button.classList.toggle('selected', nodeId === selectedId);
        button.setAttribute('aria-selected', nodeId === selectedId ? 'true' : 'false');
      });
    }
    if (options.revealEditor) revealEditor();
    return true;
  }

  function searchText(node) {
    return (node.name + ' ' + (node.name_en || '') + ' ' + (node.code || '') + ' ' +
      (node.external_ids || []).map(item => item.value).join(' ')).toLowerCase();
  }

  function syncForm(input) {
    const node = current();
    if (!node || saving || !editable()) return;
    const previousName = node.name;
    const previousCountryCode = node.country_code;
    const previousSearchText = $('regionSearch').value.trim() ? searchText(node) : '';
    const value = input.value.trim();
    switch (input.id) {
      case 'regionCountryCode':
        if (node.level !== 'country' || persistedIds.has(node.id)) return;
        node.country_code = value.toUpperCase();
        node.code = node.country_code;
        node.code_system = node.code ? 'iso3166-1' : '';
        break;
      case 'regionAmapAdcode':
        setExternalValue(node, 'amap', 'adcode', value);
        break;
      case 'regionGooglePlaceId':
        setExternalValue(node, 'google', 'place_id', value);
        break;
      case 'regionLatitude':
      case 'regionLongitude':
        node[input.id === 'regionLatitude' ? 'latitude' : 'longitude'] = value === '' ? null : Number(value);
        break;
      default:
        if (!Object.hasOwn(fieldMap, input.id)) return;
        node[fieldMap[input.id]] = value;
    }
    markDirty(node);
    if (previousName !== node.name) {
      updateEditorIdentity(node);
      sortChildren(node.parent_id);
    }
    if (node.level === 'country' && (previousName !== node.name || previousCountryCode !== node.country_code)) {
      renderCountryTabs();
      refreshDraftIndicators();
    }
    if (previousName !== node.name || (previousSearchText && previousSearchText !== searchText(node))) renderTree();
  }

  function addNode(parent) {
    if (!editable() || !catalog || saving || photoEditor.busy || parent && (!parent.active || !persistedIds.has(parent.id))) return;
    const country = !parent;
    let id;
    do {
      const values = crypto.getRandomValues(new Uint8Array(5));
      id = 'R' + Array.from(values, value => (value % 36).toString(36).toUpperCase()).join('');
    } while (catalog.nodes.some(node => node.id === id));
    const siblings = children(parent ? parent.id : '');
    const china = parent && (parent.country_code === 'CN' || parent.id === 'country:CN');
    const node = {
      id, parent_id: parent ? parent.id : '', name: country ? '新国家' : '新地区',
      level: country ? 'country' : china && parent.level === 'country' ? 'province' : china && parent.level === 'city' ? 'district' : 'city',
      code: '', code_system: '',
      country_code: parent ? parent.country_code || '' : '', continent: parent ? parent.continent || '' : '',
      selectable: false, active: true, sort_order: siblings.reduce((max, item) => Math.max(max, item.sort_order || 0), -1) + 1
    };
    catalog.nodes.push(node);
    indexCatalog();
    if (parent) expandedIds.add(parent.id);
    markDirty(node);
    select(id, { revealEditor: true });
    $('regionName').focus();
  }

  function applyCatalog(updated, replacedIds = new Set()) {
    const pending = catalog?.nodes.filter(node => dirtyIds.has(node.id) && !replacedIds.has(node.id)) || [];
    const previousBases = persistedNodes;
    catalog = updated;
    persistedNodes = new Map(updated.nodes.map(node => [node.id, structuredClone(node)]));
    persistedIds = new Set(persistedNodes.keys());
    const positions = new Map(updated.nodes.map((node, index) => [node.id, index]));
    for (const draft of pending) {
      const index = positions.get(draft.id);
      if (index === undefined) catalog.nodes.push(draft);
      else catalog.nodes[index] = draft;
      const baseline = previousBases.get(draft.id);
      if (baseline) {
        persistedNodes.set(draft.id, baseline);
        persistedIds.add(draft.id);
      } else {
        persistedNodes.delete(draft.id);
        persistedIds.delete(draft.id);
      }
    }
    indexCatalog();
  }

  function forgetDraft(id) {
    dirtyIds.delete(id);
    conflicts.delete(id);
    saveNotes.delete(id);
  }

  function setConflict(id, message) {
    conflicts.set(id, message);
    updateSaveState();
  }

  async function load() {
    if (catalog || loadingCatalog) return;
    loadingCatalog = true;
    $('regionRetryCatalog').hidden = true;
    status('正在加载地区目录…');
    updateSaveState();
    try {
      applyCatalog(await api('catalog'));
      select(firstCountryId());
      status('');
    } catch (error) {
      status(error.message, true);
      $('regionRetryCatalog').hidden = false;
    } finally {
      loadingCatalog = false;
      updateSaveState();
    }
  }

  function renderConflictReview(review) {
    const { remote, draft, baseline, latest } = review;
    const canCompare = !!baseline && !!latest?.active && !!draft.active;
    const canKeepNew = !baseline && !latest && (!draft.parent_id ||
      remote.nodes.some(node => node.id === draft.parent_id && node.active));
    const changed = canCompare ? editableFields.filter(field =>
      fieldChanged(field, draft, latest) &&
      (fieldChanged(field, draft, baseline) || fieldChanged(field, latest, baseline))) : [];
    const rows = $('regionConflictRows');
    rows.replaceChildren();
    for (const field of changed) {
      const row = document.createElement('tr');
      const label = document.createElement('th');
      label.scope = 'row';
      label.textContent = field.label;
      row.appendChild(label);
      for (const node of [draft, latest]) {
        const value = document.createElement('td');
        const content = field.value(node);
        value.textContent = content === '' ? '未填写' : String(content);
        row.appendChild(value);
      }
      rows.appendChild(row);
    }
    const mayAdoptVersion = !!baseline;
    const mayKeepDraft = canKeepNew || (canCompare && dirtyIds.has(draft.id));
    let summary;
    if (canKeepNew) summary = '目录已有更新。保留草稿后可以重新保存。';
    else if (!baseline) summary = latest ? '该编号已被占用。可以返回查看草稿，核对后重新新建。' :
      '上级地区已从目录移除。可以返回查看草稿。';
    else if (!latest?.active) summary = '此地区已从目录移除。可以返回查看草稿，或采用最新版本并放弃这条草稿。';
    else if (changed.length) summary = '请核对两版资料。保留我的修改会把你编辑过的字段应用到最新版本，确认后仍需手动保存。';
    else summary = '目录已有更新。两版的文字与位置资料没有差异，可以更新到最新版本后继续。';
    $('regionConflictSummary').textContent = summary;
    $('regionConflictDiff').hidden = changed.length === 0;
    $('regionConflictUseLatest').hidden = !mayAdoptVersion;
    $('regionConflictKeepDraft').hidden = !mayKeepDraft;
    conflictReview = { remote, draft, baseline, latest, mayKeepDraft, mayAdoptVersion };
    const dialog = $('regionConflictDialog');
    dialog.returnValue = '';
    dialog.showModal();
  }

  async function viewLatest() {
    const draft = current();
    if (!draft || !editable() || !conflicts.has(draft.id) || latestController || saving || photoEditor.busy) return;
    const controller = new AbortController();
    latestController = controller;
    updateSaveState();
    try {
      const remote = await api('catalog', { signal: controller.signal });
      if (controller.signal.aborted || current() !== draft) return;
      if (saving || photoEditor.busy || remote.version < catalog.version) {
        setConflict(draft.id, '目录有新更新，请重新查看最新版本');
        return;
      }
      renderConflictReview({
        remote, draft, baseline: persistedNodes.get(draft.id),
        latest: remote.nodes.find(node => node.id === draft.id)
      });
    } catch (error) {
      if (!controller.signal.aborted && current() === draft) setConflict(draft.id, '读取最新版本失败，请重试。' + error.message);
    } finally {
      latestController = null;
      updateSaveState();
    }
  }

  function resolveConflict() {
    const review = conflictReview;
    conflictReview = null;
    const action = $('regionConflictDialog').returnValue;
    if (!review || !editable() || !['latest', 'mine'].includes(action)) return;
    const { remote, draft, baseline, latest, mayKeepDraft, mayAdoptVersion } = review;
    if (current() !== draft || !conflicts.has(draft.id) || saving || photoEditor.busy) return;
    if (remote.version < catalog.version) {
      setConflict(draft.id, '目录有新更新，请重新查看最新版本');
      return;
    }
    if ((action === 'latest' && !mayAdoptVersion) || (action === 'mine' && !mayKeepDraft)) return;
    const id = draft.id;
    const keepNew = action === 'mine' && !baseline;
    if (keepNew) {
      conflicts.delete(id);
      applyCatalog(remote);
    } else {
      forgetDraft(id);
      applyCatalog(remote, new Set([id]));
    }
    if (action === 'mine' && latest) {
      for (const field of editableFields) {
        if (!fieldChanged(field, draft, baseline)) continue;
        if (field.provider) setExternalValue(latest, field.provider, field.type, field.value(draft));
        else latest[field.key] = draft[field.key];
      }
      sortChildren(latest.parent_id);
      markDirty(latest);
    }
    select(keepNew || latest?.active ? id : firstCountryId());
    const message = dirtyIds.has(id) ? '已保留你的修改，核对后可保存' : '已采用最新版本';
    if (selectedId) setSaveNote(selectedId, message);
    else status(message);
  }

  $('regionSearch').addEventListener('input', renderTree);
  $('regionRetryCatalog').addEventListener('click', load);
  $('regionViewLatest').addEventListener('click', viewLatest);
  $('regionConflictDialog').addEventListener('close', resolveConflict);
  window.addEventListener('pagehide', () => latestController?.abort());
  ['input', 'change'].forEach(type => $('regionForm').addEventListener(type, event => {
    if (event.target.matches('input, select, textarea') && !event.target.closest('#regionPhotos')) syncForm(event.target);
  }));
  $('regionPhotos').addEventListener('keydown', event => {
    if (event.key === 'Enter' && event.target.matches('input:not([type="file"])') && !event.defaultPrevented) {
      event.preventDefault();
      event.target.blur();
    }
  });
  $('regionAddCountry').addEventListener('click', () => addNode(null));
  $('regionAddChild').addEventListener('click', () => addNode(current()));
  $('regionDeleteCountry').addEventListener('click', async () => {
    const node = current();
    if (!editable() || saving || photoEditor.busy || !canDeleteCountry(node) || conflicts.has(node.id)) return;
    const removed = new Set(catalog.nodes.filter(item => countryOf(item)?.id === node.id).map(item => item.id));
    const expectedVersion = catalog.version;
    const name = persistedNodes.get(node.id).name;
    saving = true;
    operationMessage = '';
    $('regionEditorFields').inert = true;
    updateSaveState();
    try {
      const confirmed = await AdminUI.confirm({
        title: '删除国家',
        message: '确认删除“' + name + '”及其 ' + (removed.size - 1) + ' 个下级地区？删除后将从城市管理和客户端目录移除，历史轨迹和图片文件保留。该国未保存的修改将被丢弃。',
        confirmText: '删除国家', danger: true
      });
      if (!confirmed) return;
      operationMessage = '正在删除国家…';
      updateSaveState();
      const updated = await api(encodeURIComponent(node.id), {
        method: 'DELETE', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ expected_version: expectedVersion })
      });
      applyCatalog(updated, removed);
      removed.forEach(id => { forgetDraft(id); expandedIds.delete(id); });
      photoEditor.remove(removed);
      saving = false;
      select(firstCountryId(), { clearSearch: true });
      const message = '已删除“' + name + '”及其下级地区';
      if (selectedId) setSaveNote(selectedId, message);
      else status(message);
    } catch (error) {
      if (error.status === 409) setConflict(node.id, '目录已更新，请查看最新版本后再删除');
      else setSaveNote(node.id, error.message, true);
    }
    finally {
      saving = false;
      $('regionEditorFields').inert = false;
      updateSaveState();
    }
  });
  $('regionDeleteDraft').addEventListener('click', () => {
    const node = current();
    if (!node || !editable() || persistedIds.has(node.id) || saving || photoEditor.busy) return;
    const removed = new Set([node.id]);
    catalog.nodes.forEach(item => { if (ancestors(item.id).has(node.id)) removed.add(item.id); });
    catalog.nodes = catalog.nodes.filter(item => !removed.has(item.id));
    indexCatalog();
    removed.forEach(forgetDraft);
    select(catalog.nodes.some(item => item.id === activeCountryId && item.active) ? activeCountryId : firstCountryId());
    if (selectedId) setSaveNote(selectedId, '已删除未保存节点');
    else status('已删除未保存节点');
  });
  $('regionForm').addEventListener('submit', async event => {
    event.preventDefault();
    const node = current();
    if (!node?.active || !dirtyIds.has(node.id) || saving || photoEditor.busy || !editable() || conflicts.has(node.id)) return;
    if ($('regionLocation').querySelector(':invalid')) $('regionLocation').open = true;
    if (!$('regionForm').reportValidity()) return;
    const savedId = node.id;
    saving = true;
    operationMessage = '正在保存当前地区…';
    $('regionEditorFields').inert = true;
    updateSaveState();
    try {
      const updated = await api('node', {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ node, base_node: persistedNodes.get(savedId) || null })
      });
      forgetDraft(savedId);
      applyCatalog(updated);
      saving = false;
      select(savedId);
      setSaveNote(savedId, '已保存');
    } catch (error) {
      if (error.status === 409) setConflict(savedId, '资料已有更新，草稿仍在');
      else setSaveNote(savedId, error.message, true);
    }
    finally {
      saving = false;
      $('regionEditorFields').inert = false;
      updateSaveState();
    }
  });
  load();
})();
