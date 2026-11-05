(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  let catalog = null;
  let selectedId = '';
  let activeCountryId = '';
  let dirtyIds = new Set();
  let persistedIds = new Set();
  let persistedNodes = new Map();
  let expandedIds = new Set();
  const treeMissingElements = new Map();
  let saving = false;
  let researchDraft = null;
  let researchNodeId = '';
  let researchingNodeId = '';
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
      const currentNode = catalog.nodes.find(item => item.id === node.id);
      if (currentNode) currentNode.photos = structuredClone(photos);
      const base = persistedNodes.get(node.id);
      if (base) base.photos = structuredClone(photos);
      catalog.version = Math.max(catalog.version, result.version);
      updateTreeMissing(node);
    },
    busyChanged: updateSaveState
  });
  const fieldMap = {
    regionName: 'name', regionNameEn: 'name_en',
    regionCountryCode: 'country_code', regionSlogan: 'slogan', regionDescription: 'description'
  };

  function externalValue(node, provider, type) {
    return (node.external_ids || []).find(item => item.provider === provider && item.type === type)?.value || '';
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

  function researchStatus(message, loading, error) {
    $('regionResearchStatusText').textContent = message;
    $('regionResearchStatus').hidden = !message;
    $('regionResearchStatus').classList.toggle('text-danger', !!error);
    $('regionResearchDots').hidden = !loading;
  }

  function updateSaveState() {
    $('regionSaveNode').disabled = !editable() || saving || photoEditor.busy || !dirtyIds.has(selectedId);
    $('regionSaveNode').textContent = '保存当前节点';
    $('regionDeleteCountry').hidden = !canDeleteCountry(current());
    $('regionDeleteCountry').disabled = !editable() || saving || photoEditor.busy;
    photoEditor.sessions.get(selectedId)?.picker.refresh();
  }

  function markDirty(node, message) {
    dirtyIds.add(node.id);
    updateSaveState();
    status(message || '当前节点有未保存修改');
  }

  async function api(path, options) {
    const response = await fetch('/admin/api/regions/' + path, options);
    let body;
    try { body = await response.json(); } catch (_) { throw new Error('服务器返回无效内容'); }
    if (!response.ok || body.code !== 0) throw new Error(body.error || '操作失败');
    return body.data;
  }

  function current() { return catalog && catalog.nodes.find(node => node.id === selectedId); }
  function canDeleteCountry(node) {
    const saved = node && persistedNodes.get(node.id);
    return !!saved && saved.level === 'country' && !saved.parent_id && saved.active &&
      saved.id !== 'country:CN' && saved.region_id !== 'CN' && (saved.country_code || '').toUpperCase() !== 'CN';
  }
  function firstCountryId() {
    return children('').find(node => node.level === 'country' && node.active)?.id || '';
  }
  function children(parentId) {
    return catalog.nodes.filter(node => node.parent_id === parentId)
      .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0) || a.name.localeCompare(b.name, 'zh-CN'));
  }
  function ancestors(id) {
    const result = new Set();
    let node = catalog.nodes.find(item => item.id === id);
    while (node && node.parent_id) {
      if (result.has(node.parent_id)) break;
      result.add(node.parent_id);
      node = catalog.nodes.find(item => item.id === node.parent_id);
    }
    return result;
  }

  function countryOf(node) {
    const seen = new Set();
    while (node && !seen.has(node.id)) {
      if (!node.parent_id) return node.level === 'country' ? node : null;
      seen.add(node.id);
      node = catalog.nodes.find(item => item.id === node.parent_id);
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
      tab.className = 'region-country-tab' + (node.id === activeCountryId ? ' active' : '') + (!node.active ? ' archived' : '');
      tab.setAttribute('role', 'tab');
      tab.setAttribute('aria-selected', node.id === activeCountryId ? 'true' : 'false');
      tab.setAttribute('aria-controls', 'regionTree');
      const title = document.createElement('span');
      title.textContent = node.name;
      const code = document.createElement('small');
      code.textContent = node.country_code || node.code || '';
      tab.append(title, code);
      tab.addEventListener('click', () => {
        $('regionSearch').value = '';
        select(node.id);
        root.querySelector('[aria-selected="true"]')?.focus();
      });
      root.appendChild(tab);
    });
  }

  function updateTreeMissing(node) {
    const root = treeMissingElements.get(node.id);
    if (!root) return;
    const saved = persistedNodes.get(node.id);
    const missing = [];
    if (!saved?.slogan?.trim()) missing.push('缺口号');
    if (!saved?.description?.trim()) missing.push('缺简介');
    if (['province', 'city'].includes(node.level) &&
        !(saved?.photos || []).some(photo => photo.url?.trim())) missing.push('缺图片');
    root.replaceChildren(...missing.map(text => {
      const badge = document.createElement('span');
      badge.className = 'badge bg-orange-lt';
      badge.textContent = text;
      return badge;
    }));
  }

  function renderTree() {
    const root = $('regionTree');
    root.replaceChildren();
    treeMissingElements.clear();
    if (!catalog || !activeCountryId) return;
    const query = $('regionSearch').value.trim().toLowerCase();
    const visible = new Set();
    if (query) catalog.nodes.forEach(node => {
      if (countryOf(node)?.id === activeCountryId &&
          (node.name + ' ' + (node.name_en || '') + ' ' + (node.code || '') + ' ' +
            (node.external_ids || []).map(item => item.value).join(' ')).toLowerCase().includes(query)) {
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
        if (childNodes.length) button.setAttribute('aria-expanded', expanded ? 'true' : 'false');
        const label = document.createElement('span');
        label.className = 'region-tree-name';
        label.textContent = node.name;
        button.appendChild(label);
        const missing = document.createElement('span');
        missing.className = 'region-tree-missing';
        button.appendChild(missing);
        treeMissingElements.set(node.id, missing);
        updateTreeMissing(node);
        button.addEventListener('click', () => select(node.id));
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

  function renderResearch() {
    const available = researchDraft && researchNodeId === selectedId;
    $('regionResearchPanel').hidden = !available;
    if (!available) return;
    const fields = $('regionDraftFields');
    fields.replaceChildren();
    [['英文名称', researchDraft.name_en], ['城市宣传口号', researchDraft.slogan], ['简介', researchDraft.description]].forEach(([label, value]) => {
      const row = document.createElement('p');
      const strong = document.createElement('strong'); strong.textContent = label + '：';
      const content = document.createElement('span'); content.textContent = value || '未查到可靠资料';
      row.append(strong, content); fields.appendChild(row);
    });
    if (researchDraft.slogan_source_url) {
      const link = document.createElement('a');
      link.href = researchDraft.slogan_source_url; link.target = '_blank'; link.rel = 'noopener noreferrer';
      link.textContent = '核对宣传口号来源'; fields.appendChild(link);
    }
    const sources = $('regionResearchSources');
    sources.replaceChildren();
    const title = document.createElement('h4'); title.textContent = '资料来源'; sources.appendChild(title);
    researchDraft.sources.forEach(source => {
      const link = document.createElement('a'); link.href = source.url; link.target = '_blank'; link.rel = 'noopener noreferrer'; link.textContent = source.title;
      sources.appendChild(link);
    });
  }

  function select(id) {
    if (photoEditor.busy || saving && id !== selectedId) return;
    if (researchNodeId !== id) { researchDraft = null; researchNodeId = ''; }
    selectedId = id;
    const node = current();
    if (node) {
      const country = countryOf(node);
      if (country) activeCountryId = country.id;
      ancestors(node.id).forEach(parentId => { if (parentId !== activeCountryId) expandedIds.add(parentId); });
    } else activeCountryId = '';
    renderCountryTabs();
    $('regionEmpty').hidden = !!node;
    $('regionForm').hidden = !node;
    $('regionAddChild').disabled = !node || !node.active || !persistedIds.has(node.id) || !editable();
    photoEditor.bind(node);
    if (!node) { renderTree(); return; }
    $('regionEditorTitle').textContent = node.name || '新增地区';
    $('regionId').textContent = node.region_id || '';
    Object.entries(fieldMap).forEach(([id, key]) => { $(id).value = node[key] || ''; });
    $('regionAmapAdcode').value = externalValue(node, 'amap', 'adcode') || (node.code_system === 'cn_adcode' ? node.code || '' : '');
    $('regionGooglePlaceId').value = externalValue(node, 'google', 'place_id');
    const countryRoot = node.level === 'country' && !node.parent_id;
    const showCountryCode = countryRoot && !persistedIds.has(node.id);
    $('regionCountryCodeField').hidden = !showCountryCode;
    $('regionCountryCodeField').classList.toggle('d-none', !showCountryCode);
    $('regionCountryCodeLabel').textContent = countryRoot ? '国家缩写（两位）' : '所属国家代码';
    $('regionCountryCode').required = countryRoot && node.active;
    $('regionCountryCode').disabled = !countryRoot || persistedIds.has(node.id);
    $('regionContinentField').hidden = node.level !== 'country';
    $('regionContinent').value = node.continent || '';
    $('regionLatitude').value = node.latitude == null ? '' : String(node.latitude);
    $('regionLongitude').value = node.longitude == null ? '' : String(node.longitude);
    $('regionSelectable').checked = !!node.selectable;
    $('regionDeleteDraft').hidden = persistedIds.has(node.id);
    $('regionResearch').disabled = !editable() || !persistedIds.has(node.id) || !!researchingNodeId;
    if (researchingNodeId === node.id) researchStatus('正在联网查询城市资料', true);
    else if (researchDraft && researchNodeId === node.id) researchStatus('联网草稿已就绪，请核对后代入');
    else researchStatus('');
    updateSaveState();
    renderResearch();
    renderTree();
    if (dirtyIds.has(id)) status('当前节点有未保存修改');
  }

  function syncForm() {
    const node = current();
    if (!node) return;
    Object.entries(fieldMap).forEach(([id, key]) => { node[key] = $(id).value.trim(); });
    setExternalValue(node, 'amap', 'adcode', $('regionAmapAdcode').value.trim());
    setExternalValue(node, 'google', 'place_id', $('regionGooglePlaceId').value.trim());
    node.country_code = node.country_code.toUpperCase();
    if (node.level === 'country' && !persistedIds.has(node.id)) {
      node.code = node.country_code;
    }
    if (node.level === 'country') node.continent = $('regionContinent').value;
    if (node.code && !node.code_system) {
      const countryCode = node.country_code.toUpperCase();
      node.code_system = node.level === 'country' ? 'iso3166-1' : countryCode === 'CN' && /^\d{6}$/.test(node.code) ? 'cn_adcode' : 'local:' + (countryCode || countryOf(node)?.id || 'unknown');
    }
    node.latitude = $('regionLatitude').value === '' ? null : Number($('regionLatitude').value);
    node.longitude = $('regionLongitude').value === '' ? null : Number($('regionLongitude').value);
    node.selectable = $('regionSelectable').checked;
    $('regionEditorTitle').textContent = node.name || '新增地区';
    markDirty(node);
    const country = countryOf(node);
    if (country) activeCountryId = country.id;
    renderCountryTabs();
    renderTree();
  }

  function addNode(parent) {
    if (!catalog || saving || photoEditor.busy || parent && !persistedIds.has(parent.id)) return;
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
    if (parent) expandedIds.add(parent.id);
    markDirty(node, '已新增节点，请填写资料并保存当前节点');
    select(id);
    $('regionName').focus();
  }

  async function load() {
    status('正在加载地区目录…');
    try {
      catalog = await api('catalog');
      researchDraft = null;
      researchNodeId = '';
      persistedIds = new Set(catalog.nodes.map(node => node.id));
      persistedNodes = new Map(catalog.nodes.map(node => [node.id, structuredClone(node)]));
      expandedIds = new Set();
      dirtyIds = new Set();
      const selected = current();
      select(selected && countryOf(selected)?.active ? selectedId : firstCountryId());
      status('地区目录已加载');
    } catch (error) { status(error.message, true); }
  }

  $('regionSearch').addEventListener('input', renderTree);
  ['input', 'change'].forEach(type => $('regionForm').addEventListener(type, event => {
    if (!event.target.closest('#regionPhotos')) syncForm();
  }));
  $('regionAddCountry').addEventListener('click', () => addNode(null));
  $('regionAddChild').addEventListener('click', () => addNode(current()));
  $('regionDeleteCountry').addEventListener('click', async () => {
    const node = current();
    if (!editable() || saving || photoEditor.busy || !canDeleteCountry(node)) return;
    const removed = new Set(catalog.nodes.filter(item => countryOf(item)?.id === node.id).map(item => item.id));
    const expectedVersion = catalog.version;
    const name = persistedNodes.get(node.id).name;
    saving = true;
    $('regionForm').inert = true;
    updateSaveState();
    try {
      const confirmed = await AdminUI.confirm({
        title: '删除国家',
        message: '确认删除“' + name + '”及其 ' + (removed.size - 1) + ' 个下级地区？删除后将从城市管理和客户端目录移除，历史轨迹和图片文件保留。该国未保存的修改将被丢弃。',
        confirmText: '删除国家', danger: true
      });
      if (!confirmed) return;
      status('正在删除国家…');
      const updated = await api(encodeURIComponent(node.id), {
        method: 'DELETE', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ expected_version: expectedVersion })
      });
      const previousBases = persistedNodes;
      const pending = catalog.nodes.filter(item => !removed.has(item.id) && dirtyIds.has(item.id));
      catalog = updated;
      persistedIds = new Set(updated.nodes.map(item => item.id));
      persistedNodes = new Map(updated.nodes.map(item => [item.id, structuredClone(item)]));
      pending.forEach(draft => {
        const index = catalog.nodes.findIndex(item => item.id === draft.id);
        if (index < 0) catalog.nodes.push(draft);
        else catalog.nodes[index] = draft;
        if (previousBases.has(draft.id)) persistedNodes.set(draft.id, previousBases.get(draft.id));
        else persistedNodes.delete(draft.id);
      });
      removed.forEach(id => { dirtyIds.delete(id); expandedIds.delete(id); });
      photoEditor.remove(removed);
      researchDraft = null; researchNodeId = '';
      $('regionSearch').value = '';
      saving = false;
      select(firstCountryId());
      status('已删除“' + name + '”及其下级地区');
    } catch (error) { status(error.message, true); }
    finally {
      saving = false;
      $('regionForm').inert = false;
      updateSaveState();
    }
  });
  $('regionDeleteDraft').addEventListener('click', () => {
    const node = current();
    if (!node || persistedIds.has(node.id) || saving || photoEditor.busy) return;
    const removed = new Set([node.id]);
    catalog.nodes.forEach(item => { if (ancestors(item.id).has(node.id)) removed.add(item.id); });
    catalog.nodes = catalog.nodes.filter(item => !removed.has(item.id));
    removed.forEach(id => dirtyIds.delete(id));
    select(catalog.nodes.some(item => item.id === activeCountryId && item.active) ? activeCountryId : firstCountryId());
    status('未保存节点及下级已删除');
  });
  $('regionResearch').addEventListener('click', async () => {
    const node = current();
    if (!node || !persistedIds.has(node.id) || researchingNodeId) return;
    const nodeId = node.id, targetCatalog = catalog;
    researchingNodeId = nodeId;
    $('regionResearch').disabled = true;
    researchStatus('正在联网查询城市资料', true);
    try {
      const draft = await api('research', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id: nodeId }) });
      if (catalog !== targetCatalog || selectedId !== nodeId) return;
      researchDraft = draft; researchNodeId = nodeId;
      renderResearch(); researchStatus('联网草稿已就绪，请核对后代入');
    } catch (error) { if (selectedId === nodeId) researchStatus(error.message, false, true); }
    finally {
      researchingNodeId = '';
      $('regionResearch').disabled = !editable() || !persistedIds.has(selectedId);
    }
  });
  function applyDraft(overwrite) {
    if (!researchDraft || researchNodeId !== selectedId || !editable()) return;
    [['regionNameEn', 'name_en'], ['regionSlogan', 'slogan'], ['regionDescription', 'description']].forEach(([id, key]) => {
      if (researchDraft[key] && (overwrite || !$(id).value.trim())) $(id).value = researchDraft[key];
    });
    syncForm();
    status('文字已代入，请核对并保存当前节点');
  }
  $('regionApplyEmpty').addEventListener('click', () => applyDraft(false));
  $('regionApplyAll').addEventListener('click', () => applyDraft(true));
  $('regionSaveNode').addEventListener('click', async () => {
    const node = current();
    if (!node || !dirtyIds.has(node.id) || saving || photoEditor.busy || !editable()) return;
    if (!$('regionForm').reportValidity()) return;
    const savedId = node.id;
    const previousBases = persistedNodes;
    saving = true; $('regionForm').inert = true; updateSaveState();
    status('正在保存当前节点…');
    try {
      const updated = await api('node', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ node, base_node: previousBases.get(savedId) || null }) });
      const pending = new Map(catalog.nodes.filter(item => item.id !== savedId && dirtyIds.has(item.id)).map(item => [item.id, item]));
      catalog = updated;
      persistedIds = new Set(updated.nodes.map(item => item.id));
      persistedNodes = new Map(updated.nodes.map(item => [item.id, structuredClone(item)]));
      pending.forEach((draft, id) => {
        const index = catalog.nodes.findIndex(item => item.id === id);
        if (index < 0) catalog.nodes.push(draft);
        else catalog.nodes[index] = draft;
        if (previousBases.has(id)) persistedNodes.set(id, previousBases.get(id));
        else persistedNodes.delete(id);
      });
      dirtyIds.delete(savedId);
      select(savedId);
      status('当前节点已保存，客户端下次刷新目录时会读取新版本');
    } catch (error) { status(error.message, true); }
    finally {
      saving = false;
      $('regionForm').inert = false;
      $('regionForm').querySelector('.region-form-grid').inert = false;
      $('regionForm').querySelector('.region-editor-title').inert = false;
      $('regionResearchPanel').inert = false;
      updateSaveState();
    }
  });
  load();
})();
