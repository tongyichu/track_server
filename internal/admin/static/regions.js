(function () {
  'use strict';
  const $ = id => document.getElementById(id);
  let catalog = null;
  let selectedId = '';
  let dirty = false;
  let persistedIds = new Set();
  const fieldMap = {
    regionName: 'name', regionNameEn: 'name_en', regionLevel: 'level', regionParent: 'parent_id',
    regionCode: 'code', regionCodeSystem: 'code_system', regionCountryCode: 'country_code',
    regionContinent: 'continent', regionSlogan: 'slogan', regionDescription: 'description', regionCover: 'cover_url'
  };

  function status(message, error) {
    $('regionStatus').textContent = message;
    $('regionStatus').className = error ? 'text-danger' : 'text-secondary';
  }

  async function api(path, options) {
    const response = await fetch('/admin/api/regions/' + path, options);
    let body;
    try { body = await response.json(); } catch (_) { throw new Error('服务器返回无效内容'); }
    if (!response.ok || body.code !== 0) throw new Error(body.error || '操作失败');
    return body.data;
  }

  function current() { return catalog && catalog.nodes.find(node => node.id === selectedId); }
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

  function renderTree() {
    const root = $('regionTree');
    root.replaceChildren();
    if (!catalog) return;
    const query = $('regionSearch').value.trim().toLowerCase();
    const visible = new Set();
    if (query) catalog.nodes.forEach(node => {
      if ((node.name + ' ' + (node.name_en || '') + ' ' + (node.code || '')).toLowerCase().includes(query)) {
        visible.add(node.id);
        ancestors(node.id).forEach(id => visible.add(id));
      }
    });
    function add(parentId, depth) {
      children(parentId).forEach(node => {
        if (query && !visible.has(node.id)) return;
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'region-tree-item' + (node.id === selectedId ? ' selected' : '') + (!node.active ? ' archived' : '');
        button.style.paddingLeft = (12 + depth * 16) + 'px';
        button.setAttribute('role', 'treeitem');
        button.setAttribute('aria-selected', node.id === selectedId ? 'true' : 'false');
        const label = document.createElement('span');
        label.textContent = node.name;
        const code = document.createElement('small');
        code.textContent = node.code || node.level;
        button.append(label, code);
        button.addEventListener('click', () => select(node.id));
        root.appendChild(button);
        add(node.id, depth + 1);
      });
    }
    add('', 0);
    if (!root.childElementCount) root.textContent = '没有匹配的地区';
  }

  function parentOptions(node) {
    const select = $('regionParent');
    select.replaceChildren();
    const excluded = new Set([node.id]);
    catalog.nodes.forEach(item => {
      if (ancestors(item.id).has(node.id)) excluded.add(item.id);
    });
    const root = new Option('无（国家根节点）', '');
    select.add(root);
    catalog.nodes.filter(item => !excluded.has(item.id) && item.active).forEach(item => {
      select.add(new Option(item.name + (item.code ? ' · ' + item.code : ''), item.id));
    });
    select.value = node.parent_id || '';
  }

  function preview(url) {
    const image = $('regionCoverPreview');
    if (!url) { image.hidden = true; image.removeAttribute('src'); return; }
    image.hidden = false;
    image.src = url.startsWith('/api/v1/static/regions/')
      ? '/admin/api/static/regions/' + url.split('/').pop() : url;
  }

  function select(id) {
    selectedId = id;
    const node = current();
    $('regionEmpty').hidden = !!node;
    $('regionForm').hidden = !node;
    $('regionAddChild').disabled = !node || !(window.AdminAccess && AdminAccess.can('regions', true));
    if (!node) { renderTree(); return; }
    $('regionEditorTitle').textContent = node.name || '新增地区';
    $('regionId').textContent = '内部 ID：' + node.id;
    parentOptions(node);
    Object.entries(fieldMap).forEach(([id, key]) => { $(id).value = node[key] || ''; });
    $('regionLatitude').value = node.latitude == null ? '' : String(node.latitude);
    $('regionLongitude').value = node.longitude == null ? '' : String(node.longitude);
    $('regionSort').value = node.sort_order == null ? '0' : String(node.sort_order);
    $('regionPostalCodes').value = (node.postal_codes || []).join(', ');
    $('regionSelectable').checked = !!node.selectable;
    $('regionActive').checked = !!node.active;
    $('regionArchive').textContent = node.active ? '归档节点' : '恢复节点';
    $('regionDeleteDraft').hidden = persistedIds.has(node.id);
    preview(node.cover_url);
    renderTree();
  }

  function syncForm() {
    const node = current();
    if (!node) return;
    Object.entries(fieldMap).forEach(([id, key]) => { node[key] = $(id).value.trim(); });
    node.latitude = $('regionLatitude').value === '' ? null : Number($('regionLatitude').value);
    node.longitude = $('regionLongitude').value === '' ? null : Number($('regionLongitude').value);
    node.sort_order = Number($('regionSort').value || 0);
    node.postal_codes = $('regionPostalCodes').value.split(/[,，\n]/).map(value => value.trim()).filter(Boolean);
    node.selectable = $('regionSelectable').checked;
    node.active = $('regionActive').checked;
    $('regionEditorTitle').textContent = node.name || '新增地区';
    $('regionArchive').textContent = node.active ? '归档节点' : '恢复节点';
    dirty = true;
    status('有未保存修改');
    preview(node.cover_url);
    renderTree();
  }

  function addNode(parent) {
    if (!catalog) return;
    const country = !parent;
    let id;
    do {
      const values = crypto.getRandomValues(new Uint8Array(5));
      id = 'R' + Array.from(values, value => (value % 36).toString(36).toUpperCase()).join('');
    } while (catalog.nodes.some(node => node.id === id));
    const node = {
      id, parent_id: parent ? parent.id : '', name: country ? '新国家' : '新地区',
      level: country ? 'country' : 'city', code: '', code_system: '',
      country_code: parent ? parent.country_code || '' : '', continent: parent ? parent.continent || '' : '',
      selectable: false, active: true, sort_order: children(parent ? parent.id : '').length,
      postal_codes: []
    };
    catalog.nodes.push(node);
    dirty = true;
    select(id);
    status('已新增节点，请填写资料并保存');
  }

  async function load() {
    status('正在加载地区目录…');
    try {
      catalog = await api('catalog');
      persistedIds = new Set(catalog.nodes.map(node => node.id));
      dirty = false;
      $('regionVersion').textContent = '目录版本 ' + catalog.version + ' · ' + catalog.nodes.length + ' 个节点';
      select(catalog.nodes.some(node => node.id === selectedId) ? selectedId : '');
      status('地区目录已加载');
    } catch (error) { status(error.message, true); }
  }

  $('regionSearch').addEventListener('input', renderTree);
  $('regionForm').addEventListener('input', syncForm);
  $('regionForm').addEventListener('change', syncForm);
  $('regionAddCountry').addEventListener('click', () => addNode(null));
  $('regionAddChild').addEventListener('click', () => addNode(current()));
  $('regionArchive').addEventListener('click', () => {
    const node = current();
    if (!node) return;
    const active = !node.active;
    node.active = active;
    if (!active) catalog.nodes.forEach(item => { if (ancestors(item.id).has(node.id)) item.active = false; });
    dirty = true;
    select(node.id);
    status(active ? '节点已恢复，尚未保存' : '节点及下级已归档，尚未保存');
  });
  $('regionDeleteDraft').addEventListener('click', () => {
    const node = current();
    if (!node || persistedIds.has(node.id)) return;
    const removed = new Set([node.id]);
    catalog.nodes.forEach(item => { if (ancestors(item.id).has(node.id)) removed.add(item.id); });
    catalog.nodes = catalog.nodes.filter(item => !removed.has(item.id));
    dirty = true;
    select('');
    status('未保存节点及下级已删除');
  });
  $('regionReload').addEventListener('click', load);
  $('regionSave').addEventListener('click', async () => {
    if (!catalog || !dirty) return;
    $('regionSave').disabled = true;
    status('正在保存…');
    try {
      catalog = await api('catalog', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(catalog) });
      persistedIds = new Set(catalog.nodes.map(node => node.id));
      dirty = false;
      $('regionVersion').textContent = '目录版本 ' + catalog.version + ' · ' + catalog.nodes.length + ' 个节点';
      select(selectedId);
      status('保存成功，客户端下次刷新目录时会读取新版本');
    } catch (error) { status(error.message, true); }
    finally { $('regionSave').disabled = false; }
  });
  $('regionImageFile').addEventListener('change', async () => {
    const file = $('regionImageFile').files[0];
    if (!file) return;
    if (file.size > 8 * 1024 * 1024) { status('图片不能超过 8 MiB', true); return; }
    const nodeId = selectedId;
    const form = new FormData();
    form.append('image', file);
    status('正在上传图片…');
    try {
      const result = await api('images', { method: 'POST', body: form });
      const node = catalog.nodes.find(item => item.id === nodeId);
      if (node) node.cover_url = result.cover_url;
      if (selectedId === nodeId) { $('regionCover').value = result.cover_url; preview(result.cover_url); }
      dirty = true;
      status('图片已上传，请保存地区资料');
    } catch (error) { status(error.message, true); }
    $('regionImageFile').value = '';
  });
  window.addEventListener('beforeunload', event => { if (dirty) { event.preventDefault(); event.returnValue = ''; } });
  load();
})();
